package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const full = `{"addr":"127.0.0.1:7777","kubeconfig":"~/.kube/config","context":"dev","refresh_seconds":5,"log_tail":200,"read_only":true}`

func write(t *testing.T, body string) string {
	p := filepath.Join(t.TempDir(), "c.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadFullConfig(t *testing.T) {
	c, _, err := Load(write(t, full))
	home, _ := os.UserHomeDir()
	if err != nil || c.Addr != "127.0.0.1:7777" || !c.ReadOnly || c.Context != "dev" || c.Kubeconfig != filepath.Join(home, ".kube/config") {
		t.Fatalf("%+v %v", c, err)
	}
}

func TestEveryKeyIsRequired(t *testing.T) {
	for _, k := range required {
		body := full
		// drop one key: rebuild the object without it
		parts := strings.Split(strings.Trim(full, "{}"), ",")
		var kept []string
		for _, p := range parts {
			if !strings.HasPrefix(p, `"`+k+`"`) {
				kept = append(kept, p)
			}
		}
		body = "{" + strings.Join(kept, ",") + "}"
		if _, _, err := Load(write(t, body)); err == nil || !strings.Contains(err.Error(), `missing key "`+k+`"`) {
			t.Fatalf("without %s: got %v", k, err)
		}
	}
}

func TestLoadRejectsTypoAndBadValues(t *testing.T) {
	for _, body := range []string{
		strings.Replace(full, `"addr"`, `"adr"`, 1) + ``, // typo: also leaves addr missing
		strings.Replace(full, `"refresh_seconds":5`, `"refresh_seconds":0`, 1),
		strings.Replace(full, `127.0.0.1:7777`, `nope`, 1),
		strings.Replace(full, `"log_tail":200`, `"log_tail":-1`, 1),
		strings.Replace(full, `"context":"dev"`, `"context":""`, 1),
		strings.Replace(full, `}`, `,"extra":1}`, 1),
	} {
		if _, _, err := Load(write(t, body)); err == nil {
			t.Fatalf("%s: want error", body)
		}
	}
}

func TestLoadMissingFileFails(t *testing.T) {
	if _, _, err := Load(filepath.Join(t.TempDir(), "nope.json")); err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("got %v", err)
	}
}

func TestRedisEntriesAreCheckedAndOptional(t *testing.T) {
	base := Config{Addr: "127.0.0.1:9100", Kubeconfig: "k", Context: "c", RefreshSeconds: 5}
	if err := base.Validate(); err != nil {
		t.Fatalf("no redis servers must be fine: %v", err)
	}
	good := base
	good.Redis = []Redis{{Name: "cache", Addr: "10.0.0.5:6379"}, {Name: "queue", Addr: "redis.internal:6379", Password: "x", TLS: true}}
	if err := good.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, list := range map[string][]Redis{
		"no name":   {{Addr: "h:6379"}},
		"duplicate": {{Name: "a", Addr: "h:1"}, {Name: "a", Addr: "h:2"}},
		"no port":   {{Name: "a", Addr: "h"}},
	} {
		c := base
		c.Redis = list
		if c.Validate() == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}

func TestKubernetesConnections(t *testing.T) {
	multi := `{"addr":"127.0.0.1:7777","refresh_seconds":5,"log_tail":200,"read_only":false,"kubernetes":[
		{"name":"dev","kubeconfig":"~/.kube/config","context":"dev-ctx"},{"name":"prod","kubeconfig":"/k/prod","context":"prod-ctx"}]}`
	c, _, err := Load(write(t, multi))
	home, _ := os.UserHomeDir()
	if err != nil || len(c.Kubernetes) != 2 || c.Kubernetes[0].Kubeconfig != filepath.Join(home, ".kube/config") || c.Kubernetes[1].Context != "prod-ctx" {
		t.Fatalf("%+v %v", c.Kubernetes, err)
	}
	// none configured is fine: the Kubernetes pages just say "not connected"
	none := `{"addr":"127.0.0.1:7777","refresh_seconds":5,"log_tail":200,"read_only":false}`
	if c, _, err := Load(write(t, none)); err != nil || len(c.Kubernetes) != 0 {
		t.Fatalf("%+v %v", c.Kubernetes, err)
	}
	// the old two-key form becomes one connection named after the context
	if c, _, err := Load(write(t, full)); err != nil || len(c.Kubernetes) != 1 || c.Kubernetes[0].Name != "dev" {
		t.Fatalf("legacy: %+v %v", c.Kubernetes, err)
	}
	for name, body := range map[string]string{
		"duplicate name": strings.Replace(multi, `"name":"prod"`, `"name":"dev"`, 1),
		"no context":     strings.Replace(multi, `"context":"prod-ctx"`, `"context":""`, 1),
		"no name":        strings.Replace(multi, `"name":"prod"`, `"name":""`, 1),
		"unknown key":    strings.Replace(multi, `"name":"prod"`, `"nam":"prod"`, 1),
	} {
		if _, _, err := Load(write(t, body)); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}
