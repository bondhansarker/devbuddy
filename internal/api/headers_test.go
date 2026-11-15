package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bondhansarker/devbuddy/internal/config"
)

func TestSecurityHeadersOnEveryAnswer(t *testing.T) {
	h := newServer(config.Config{Addr: "127.0.0.1:9100"})
	for _, c := range []struct{ path, host string }{{"/", "127.0.0.1:9100"}, {"/api/info", "127.0.0.1:9100"}, {"/api/info", "evil.example"}, {"/api/nope", "127.0.0.1:9100"}} {
		r := httptest.NewRequest("GET", c.path, nil)
		r.Host = c.host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		got := w.Header()
		if !strings.Contains(got.Get("Content-Security-Policy"), "script-src 'self'") || !strings.Contains(got.Get("Content-Security-Policy"), "frame-ancestors 'none'") ||
			got.Get("X-Frame-Options") != "DENY" || got.Get("X-Content-Type-Options") != "nosniff" || got.Get("Referrer-Policy") != "no-referrer" {
			t.Errorf("%s (host %s, %d): missing headers %v", c.path, c.host, w.Code, got)
		}
		if strings.HasPrefix(c.path, "/api/") && got.Get("Cache-Control") != "no-store" {
			t.Errorf("%s: API answers must not be cached, got %q", c.path, got.Get("Cache-Control"))
		}
	}
	if strings.Contains(csp, "unsafe-inline") && strings.Contains(strings.Split(csp, "style-src")[0], "unsafe-inline") {
		t.Error("scripts must not allow unsafe-inline")
	}
}

func TestStaticFilesRevalidateWithETag(t *testing.T) {
	h := newTestServer()
	w := do(h, "GET", "/index.html", "")
	tag := w.Header().Get("ETag")
	if tag == "" || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("no validator: %v", w.Header())
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("If-None-Match", tag)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, r)
	if w2.Code != 304 || w2.Body.Len() != 0 {
		t.Fatalf("want an empty 304, got %d (%d bytes)", w2.Code, w2.Body.Len())
	}
	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("If-None-Match", `"other"`)
	w3 := httptest.NewRecorder()
	h.ServeHTTP(w3, r)
	if w3.Code != 200 || w3.Body.String() != "hi" {
		t.Fatalf("a changed file is sent again: %d %q", w3.Code, w3.Body)
	}
}

func TestOversizedBodiesAreRefused(t *testing.T) {
	_, call := redisServer(t, false)
	big := `{"db":0,"command":"ECHO ` + strings.Repeat("a", 2<<20) + `"}`
	if code, _ := call("POST", "/api/redis/cache/command", big); code != 400 {
		t.Fatalf("a 2 MB command body want 400, got %d", code)
	}
	if code, _ := call("POST", "/api/redis/cache/command", `{"db":0,"command":"PING"}`); code != 200 {
		t.Fatalf("a normal one still works, got %d", code)
	}
}

// a recorder that counts flushes
type flushCounter struct {
	*httptest.ResponseRecorder
	mu sync.Mutex
	n  int
}

func (f *flushCounter) Flush() { f.mu.Lock(); f.n++; f.mu.Unlock(); f.ResponseRecorder.Flush() }
func (f *flushCounter) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

func TestLogLinesAreFlushedInBatches(t *testing.T) {
	w := &flushCounter{ResponseRecorder: httptest.NewRecorder()}
	s := newSSE(w)
	for i := 0; i < 5000; i++ {
		s.Data("line")
	}
	if n := w.count(); n > 3 {
		t.Errorf("5000 quick lines caused %d flushes, want a handful", n)
	}
	time.Sleep(3 * flushEvery)
	if w.count() == 0 {
		t.Error("buffered lines must still go out without more traffic")
	}
	s.Data("last")
	s.Event("end", "")
	s.Close()
	if got := strings.Count(w.Body.String(), "data: line\n\n"); got != 5000 || !strings.Contains(w.Body.String(), "data: last") || !strings.HasSuffix(w.Body.String(), "event: end\ndata: \n\n") {
		t.Errorf("lines lost or reordered: %d lines, tail %q", got, w.Body.String()[len(w.Body.String())-40:])
	}
}

var _ http.Flusher = (*flushCounter)(nil)
