package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bondhansarker/devbuddy/internal/config"
)

const (
	redisPW = "r3dis-s3cret-pw"
	esPW    = "es-p4ssword-xyz"
	esKey   = "QUJDREVGRzpISUpLTE1OTw=="
)

func secretServer(t *testing.T, esURL string) http.Handler {
	t.Helper()
	mr, _ := redisServer(t, false) // a miniredis the console can talk to
	mr.RequireAuth(redisPW)
	return newServer(config.Config{
		Redis:         []config.Redis{{Name: "cache", Addr: mr.Addr(), Username: "default", Password: redisPW}},
		Elasticsearch: []config.Elasticsearch{{Name: "logs", Addr: esURL, Username: "elastic", Password: esPW}, {Name: "keyed", Addr: esURL, APIKey: esKey}},
	})
}

func get(h http.Handler, method, path, body string) (int, string) {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
	return w.Code, w.Body.String()
}

// no response that lists connections or settings may carry a credential
func TestNoCredentialInTheListsTheBrowserReads(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{}`)) }))
	defer srv.Close()
	h := secretServer(t, srv.URL)
	for _, p := range []string{"/api/info", "/api/redis", "/api/es"} {
		code, body := get(h, "GET", p, "")
		if code != 200 {
			t.Fatalf("%s: %d", p, code)
		}
		for _, s := range []string{redisPW, esPW, esKey, "elastic", "default"} {
			if strings.Contains(body, s) {
				t.Errorf("%s leaks %q: %s", p, s, body)
			}
		}
	}
}

// an upstream error that repeats a secret reaches the browser masked
func TestErrorsAreScrubbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte(`{"error":{"type":"security_exception","reason":"bad credentials ` + esPW + ` and ` + esKey + `"}}`))
	}))
	defer srv.Close()
	h := secretServer(t, srv.URL)
	code, body := get(h, "GET", "/api/es/logs/indices", "")
	if code == 200 || strings.Contains(body, esPW) || strings.Contains(body, esKey) || !strings.Contains(body, mask) {
		t.Fatalf("%d %s", code, body)
	}
}

func TestQueryAndConsoleOutputAreScrubbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"echo":"` + esPW + `"}`))
	}))
	defer srv.Close()
	h := secretServer(t, srv.URL)
	code, body := get(h, "POST", "/api/es/logs/query", `{"method":"GET","path":"_cluster/settings"}`)
	if code != 200 || strings.Contains(body, esPW) || !strings.Contains(body, mask) {
		t.Fatalf("es query: %d %s", code, body)
	}
	code, body = get(h, "POST", "/api/redis/cache/command", `{"db":0,"command":"ECHO `+redisPW+`"}`)
	if code != 200 || strings.Contains(body, redisPW) || !strings.Contains(body, mask) {
		t.Fatalf("console: %d %s", code, body)
	}
}

func TestShortSecretsAreLeftAlone(t *testing.T) {
	setScrubber(config.Config{Redis: []config.Redis{{Name: "r", Addr: "h:1", Password: "x"}}})
	if got := scrub("exists: x"); got != "exists: x" {
		t.Fatalf("a one-letter password would mangle text: %q", got)
	}
	setScrubber(config.Config{})
}
