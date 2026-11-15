package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"

	"github.com/bondhansarker/devbuddy/internal/config"
)

func redisServer(t *testing.T, readOnly bool) (*miniredis.Miniredis, func(method, path, body string) (int, string)) {
	mr := miniredis.RunT(t)
	mr.Set("user:1", "alice")
	mr.Set("user:2", "bob")
	mr.Set("session:9", "x")
	mr.HSet("profile", "name", "alice", "city", "paris")
	mr.RPush("queue", "a", "b")
	mr.Select(3)
	mr.Set("other:1", "in db 3")
	h := newServer(config.Config{ReadOnly: readOnly, Redis: []config.Redis{{Name: "cache", Addr: mr.Addr()}}})
	return mr, func(method, path, body string) (int, string) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w.Code, w.Body.String()
	}
}

func TestRedisBrowseSearchAndRead(t *testing.T) {
	_, call := redisServer(t, false)
	for _, c := range []struct{ path, want string }{
		{"/api/redis", `"name":"cache"`},
		{"/api/redis/cache/dbs", `"databases":16`},
		{"/api/redis/cache/keys?db=0&match=user", `"key":"user:1"`},         // plain text = contains
		{"/api/redis/cache/keys?db=3", `"key":"other:1"`},                   // another db index
		{"/api/redis/cache/key?db=0&key=user:1", `"string":"alice"`},        // string
		{"/api/redis/cache/key?db=0&key=profile", `"k":"city","v":"paris"`}, // hash
		{"/api/redis/cache/key?db=0&key=queue", `"type":"list"`},
	} {
		code, body := call("GET", c.path, "")
		if code != 200 || !strings.Contains(body, c.want) {
			t.Fatalf("%s: %d %s (want %s)", c.path, code, body, c.want)
		}
	}
	if _, body := call("GET", "/api/redis/cache/keys?db=0&match=user", ""); strings.Contains(body, "session:9") || strings.Contains(body, "other:1") {
		t.Fatalf("match or db ignored: %s", body)
	}
	// type filter: only hashes, only strings, and a bad type is refused
	if _, body := call("GET", "/api/redis/cache/keys?db=0&type=hash", ""); !strings.Contains(body, `"key":"profile"`) || strings.Contains(body, `"user:1"`) {
		t.Fatalf("type=hash: %s", body)
	}
	if _, body := call("GET", "/api/redis/cache/keys?db=0&type=string&match=user", ""); !strings.Contains(body, `"user:1"`) || strings.Contains(body, `"profile"`) {
		t.Fatalf("type=string: %s", body)
	}
	if code, _ := call("GET", "/api/redis/cache/keys?db=0&type=bogus", ""); code != 400 {
		t.Fatalf("bad type accepted: %d", code)
	}
	if code, _ := call("GET", "/api/redis/cache/key?db=0&key=nope", ""); code != 404 {
		t.Fatalf("missing key: %d", code)
	}
	if code, _ := call("GET", "/api/redis/nope/dbs", ""); code != 404 {
		t.Fatalf("unknown server: %d", code)
	}
}

func TestRedisDeleteOnlyInTheGivenDB(t *testing.T) {
	mr, call := redisServer(t, false)
	code, body := call("POST", "/api/redis/cache/delete", `{"db":0,"keys":["user:1","nope"]}`)
	if code != 200 || !strings.Contains(body, `"deleted":1`) {
		t.Fatalf("%d %s", code, body)
	}
	mr.Select(0)
	if mr.Exists("user:1") || !mr.Exists("user:2") {
		t.Fatal("wrong key deleted")
	}
	mr.Select(3)
	if !mr.Exists("other:1") {
		t.Fatal("db 3 touched")
	}
}

func TestRedisDeleteRefusedWhenReadOnly(t *testing.T) {
	mr, call := redisServer(t, true)
	if code, _ := call("POST", "/api/redis/cache/delete", `{"db":0,"keys":["user:1"]}`); code != 403 {
		t.Fatalf("got %d", code)
	}
	mr.Select(0)
	if !mr.Exists("user:1") {
		t.Fatal("deleted in read-only mode")
	}
	if code, _ := call("GET", "/api/redis/cache/keys?db=0", ""); code != 200 {
		t.Fatal("reads must still work")
	}
}

func TestRedisConsole(t *testing.T) {
	mr, call := redisServer(t, false)
	for _, c := range []struct{ cmd, want string }{
		{`GET user:1`, `\"alice\"`},
		{`SET "my key" "two words"`, `OK`},
		{`GET "my key"`, `two words`},
		{`HGETALL profile`, `city`},
		{`LRANGE queue 0 -1`, `1) `},
		{`GET nope`, `(nil)`},
		{`DBSIZE`, `(integer)`},
		{`LPUSH user:1 x`, `WRONGTYPE`},         // a server error is output, not a failed request
		{`GET profile`, `try: HGETALL profile`}, // WRONGTYPE comes with a hint
		{`GET queue`, `try: LRANGE queue 0 -1`},
		{`LPUSH user:1 x`, `the key is a string, try: GET user:1`},
	} {
		code, body := call("POST", "/api/redis/cache/command", `{"db":0,"command":`+strconvQuote(c.cmd)+`}`)
		if code != 200 || !strings.Contains(body, c.want) {
			t.Fatalf("%s: %d %s (want %s)", c.cmd, code, body, c.want)
		}
	}
	mr.Select(0)
	if v, _ := mr.Get("my key"); v != "two words" {
		t.Fatalf("quoted argument lost: %q", v)
	}
	for _, cmd := range []string{"MONITOR", "SELECT 1", "SUBSCRIBE x", "CLIENT REPLY OFF", "FLUSHALL extra \"unterminated"} {
		if code, _ := call("POST", "/api/redis/cache/command", `{"db":0,"command":`+strconvQuote(cmd)+`}`); code != 403 && code != 400 {
			t.Fatalf("%s must be refused, got %d", cmd, code)
		}
	}
}

func TestRedisConsoleReadOnly(t *testing.T) {
	mr, call := redisServer(t, true)
	for cmd, want := range map[string]int{"GET user:1": 200, "DEL user:1": 403, "FLUSHALL": 403, "CONFIG SET maxmemory 1": 403, "CONFIG GET maxmemory": 200, "get user:1": 200} {
		if code, body := call("POST", "/api/redis/cache/command", `{"db":0,"command":`+strconvQuote(cmd)+`}`); code != want {
			t.Fatalf("%s: got %d %s, want %d", cmd, code, body, want)
		}
	}
	mr.Select(0)
	if !mr.Exists("user:1") {
		t.Fatal("a read-only console deleted a key")
	}
}

func strconvQuote(s string) string { b, _ := json.Marshal(s); return string(b) }
