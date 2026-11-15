// Package config loads DevBuddy's JSON config file. There are no defaults: every
// key must be set, so nobody runs against a cluster by accident. Keys are
// snake_case; an unknown key is an error so a typo does not go unnoticed.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Addr string `json:"addr"` // listen address, must be loopback (localhost or 127.0.0.1)
	// Kubernetes connections (optional: with none, the Kubernetes pages show "not connected").
	Kubernetes     []Kubernetes `json:"kubernetes"`
	RefreshSeconds int          `json:"refresh_seconds"` // how often the open view reloads
	LogTail        int          `json:"log_tail"`        // lines the log viewer starts with
	ReadOnly       bool         `json:"read_only"`       // no shell, restart or port-forward start

	// Optional, the only key that may be left out: Redis servers for the Redis page. The app connects
	// straight to each addr, so it must be reachable from this machine. Passwords live here, in the
	// gitignored config file.
	Redis []Redis `json:"redis"`

	// Optional too: Elasticsearch / OpenSearch clusters for the read-only Elasticsearch page.
	Elasticsearch []Elasticsearch `json:"elasticsearch"`
}

type Elasticsearch struct {
	Name     string `json:"name"`                 // shown in the UI, unique
	Addr     string `json:"addr"`                 // http(s)://host:port, http:// when no scheme is given
	Username string `json:"username"`             // optional, basic auth
	Password string `json:"password"`             // optional
	APIKey   string `json:"api_key"`              // optional, the base64 "id:key" value (used instead of basic auth)
	Insecure bool   `json:"insecure_skip_verify"` // optional: do not verify the TLS certificate (self-signed clusters)
}

type Kubernetes struct {
	Name       string `json:"name"`       // shown in the UI, unique
	Kubeconfig string `json:"kubeconfig"` // path to the kubeconfig file; ~/ is expanded
	Context    string `json:"context"`    // kubeconfig context to connect to
}

type Redis struct {
	Name     string `json:"name"`     // shown in the UI, unique
	Addr     string `json:"addr"`     // host:port
	Username string `json:"username"` // optional (Redis 6 ACL)
	Password string `json:"password"` // optional
	TLS      bool   `json:"tls"`      // optional
}

var required = []string{"addr", "refresh_seconds", "log_tail", "read_only"}

// Load reads path; with path "" it uses ./.config.json, then
// ~/.config/devbuddy/config.json. No file, or a missing key, is an error.
// The returned string is the file used.
func Load(path string) (Config, string, error) {
	var c Config
	if path == "" {
		for _, p := range candidates() {
			if _, err := os.Stat(p); err == nil {
				path = p
				break
			}
		}
	}
	if path == "" {
		return c, "", fmt.Errorf("no config file found (looked for %s); copy .config.example.json and fill it in, or pass --config",
			strings.Join(candidates(), ", "))
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return c, "", err
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		return c, path, fmt.Errorf("%s: %w", path, err)
	}
	for _, k := range required {
		if _, ok := keys[k]; !ok {
			return c, path, fmt.Errorf("%s: missing key %q (every key must be set, there are no defaults)", path, k)
		}
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return c, path, fmt.Errorf("%s: %w", path, err)
	}
	for i := range c.Kubernetes {
		c.Kubernetes[i].Kubeconfig = expandHome(c.Kubernetes[i].Kubeconfig)
	}
	return c, path, c.Validate()
}

func (c Config) Validate() error {
	host, _, err := net.SplitHostPort(c.Addr)
	if err != nil {
		return fmt.Errorf("addr: %w", err)
	}
	if ip := net.ParseIP(host); host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return fmt.Errorf("addr %q: only localhost or a loopback address is allowed (a shell here is cluster access and there is no login)", c.Addr)
	}
	seenK := map[string]bool{}
	for i, k := range c.Kubernetes {
		if k.Name == "" || seenK[k.Name] {
			return fmt.Errorf("kubernetes[%d]: name must be set and unique", i)
		}
		seenK[k.Name] = true
		if k.Kubeconfig == "" || k.Context == "" {
			return fmt.Errorf("kubernetes %q: kubeconfig and context must be set", k.Name)
		}
	}
	if c.RefreshSeconds < 1 {
		return errors.New("refresh_seconds must be at least 1")
	}
	if c.LogTail < 0 {
		return errors.New("log_tail must not be negative")
	}
	seen := map[string]bool{}
	for i, r := range c.Redis {
		if r.Name == "" || seen[r.Name] {
			return fmt.Errorf("redis[%d]: name must be set and unique", i)
		}
		seen[r.Name] = true
		if _, _, err := net.SplitHostPort(r.Addr); err != nil || strings.ContainsAny(r.Addr, "@/") {
			return fmt.Errorf("redis %q addr must be host:port (the password has its own key)", r.Name)
		}
	}
	seenES := map[string]bool{}
	for i, e := range c.Elasticsearch {
		if e.Name == "" || seenES[e.Name] {
			return fmt.Errorf("elasticsearch[%d]: name must be set and unique", i)
		}
		seenES[e.Name] = true
		u, err := url.Parse(ESBaseURL(e.Addr))
		if err != nil || u.Host == "" {
			return fmt.Errorf("elasticsearch %q addr must look like http://host:9200", e.Name)
		}
		if u.User != nil { // an address is shown in the UI: credentials belong in username / password / api_key
			return fmt.Errorf("elasticsearch %q: do not put credentials in addr, use the username and password (or api_key) keys", e.Name)
		}
	}
	return nil
}

// ESBaseURL adds http:// when the address has no scheme and drops a trailing slash.
func ESBaseURL(addr string) string {
	if !strings.Contains(addr, "://") {
		addr = "http://" + addr
	}
	return strings.TrimRight(addr, "/")
}

func candidates() []string {
	out := []string{".config.json"}
	if h, err := os.UserHomeDir(); err == nil {
		out = append(out, filepath.Join(h, ".config", "devbuddy", "config.json"))
	}
	return out
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

// HasSecrets reports whether the config holds a password or API key.
func (c Config) HasSecrets() bool {
	for _, r := range c.Redis {
		if r.Password != "" {
			return true
		}
	}
	for _, e := range c.Elasticsearch {
		if e.Password != "" || e.APIKey != "" {
			return true
		}
	}
	return false
}

// Warnings lists problems that do not stop the app: a config file with passwords that other users on
// the machine can read.
func Warnings(path string, c Config) []string {
	var out []string
	if fi, err := os.Stat(path); err == nil && c.HasSecrets() && fi.Mode().Perm()&0o077 != 0 {
		out = append(out, fmt.Sprintf("%s holds passwords or API keys and is readable by other users (mode %04o): run chmod 600 on it", path, fi.Mode().Perm()))
	}
	return out
}
