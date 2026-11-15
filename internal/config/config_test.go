package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const full = `{"addr":"127.0.0.1:7777","refresh_seconds":5,"log_tail":200,"read_only":true}`

func write(t *testing.T, body string) string {
	p := filepath.Join(t.TempDir(), "c.json")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadFullConfig(t *testing.T) {
	c, _, err := Load(write(t, full))
	if err != nil || c.Addr != "127.0.0.1:7777" || !c.ReadOnly || len(c.Kubernetes) != 0 {
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
		strings.Replace(full, `127.0.0.1:7777`, `0.0.0.0:7777`, 1), // not loopback
		strings.Replace(full, `127.0.0.1:7777`, `192.168.1.5:7777`, 1),
		strings.Replace(full, `}`, `,"kubeconfig":"~/.kube/config","context":"dev"}`, 1), // the old single-cluster keys are gone
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
	base := Config{Addr: "127.0.0.1:9100", RefreshSeconds: 5}
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

func TestCredentialsDoNotBelongInAddresses(t *testing.T) {
	base := Config{Addr: "127.0.0.1:9100", RefreshSeconds: 5}
	for name, c := range map[string]Config{
		"es userinfo":    {Addr: base.Addr, RefreshSeconds: 5, Elasticsearch: []Elasticsearch{{Name: "e", Addr: "https://elastic:hunter22@es.example.com:9200"}}},
		"es user only":   {Addr: base.Addr, RefreshSeconds: 5, Elasticsearch: []Elasticsearch{{Name: "e", Addr: "http://elastic@es.example.com:9200"}}},
		"redis userinfo": {Addr: base.Addr, RefreshSeconds: 5, Redis: []Redis{{Name: "r", Addr: "user:pw@10.0.0.5:6379"}}},
		"redis url":      {Addr: base.Addr, RefreshSeconds: 5, Redis: []Redis{{Name: "r", Addr: "redis://10.0.0.5:6379"}}},
	} {
		err := c.Validate()
		if err == nil {
			t.Errorf("%s: accepted", name)
		} else if strings.Contains(err.Error(), "hunter22") || strings.Contains(err.Error(), "pw@") {
			t.Errorf("%s: the error repeats the credential: %v", name, err)
		}
	}
	ok := base
	ok.Elasticsearch = []Elasticsearch{{Name: "e", Addr: "https://es.example.com:9200", Username: "elastic", Password: "x"}}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestWarnsAboutAConfigOthersCanRead(t *testing.T) {
	dir := t.TempDir()
	write := func(mode os.FileMode) string {
		p := filepath.Join(dir, "c.json")
		os.Remove(p)
		if err := os.WriteFile(p, []byte("{}"), mode); err != nil {
			t.Fatal(err)
		}
		os.Chmod(p, mode)
		return p
	}
	withPW := Config{Redis: []Redis{{Name: "r", Addr: "h:1", Password: "pw"}}}
	without := Config{Redis: []Redis{{Name: "r", Addr: "h:1"}}}
	if w := Warnings(write(0o644), withPW); len(w) != 1 || !strings.Contains(w[0], "chmod 600") {
		t.Errorf("0644 with a password: %v", w)
	}
	if w := Warnings(write(0o640), Config{Elasticsearch: []Elasticsearch{{Name: "e", Addr: "h", APIKey: "k"}}}); len(w) != 1 {
		t.Errorf("0640 with an api key: %v", w)
	}
	if w := Warnings(write(0o600), withPW); len(w) != 0 {
		t.Errorf("0600 is fine: %v", w)
	}
	if w := Warnings(write(0o644), without); len(w) != 0 {
		t.Errorf("no secrets, nothing to warn about: %v", w)
	}
}
