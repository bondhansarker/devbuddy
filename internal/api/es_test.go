package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bondhansarker/devbuddy/internal/config"
)

// fakeES answers like a small Elasticsearch and records what it was asked.
var bulks []string

func fakeES(t *testing.T) (*httptest.Server, *[]string) {
	var seen []string
	bulks = nil
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/":
			w.Write([]byte(`{"cluster_name":"logs","version":{"number":"8.13.0"}}`))
		case r.URL.Path == "/_cluster/health":
			w.Write([]byte(`{"status":"yellow"}`))
		case r.URL.Path == "/_cat/indices":
			w.Write([]byte(`[{"index":"orders","health":"green","status":"open","docs.count":"42","store.size":"2048","pri":"1","rep":"1"},{"index":".kibana","health":"green","status":"open","docs.count":"3","store.size":"10","pri":"1","rep":"0"}]`))
		case strings.HasSuffix(r.URL.Path, "/_search"):
			w.Write([]byte(`{"took":3,"hits":{"total":{"value":2,"relation":"eq"},"hits":[{"_index":"orders","_id":"1","_score":1.2,"_source":{"user":"alice"}},{"_index":"orders","_id":"a/b","_score":null,"_source":{"user":"bob"}}]}}`))
		case strings.HasPrefix(r.URL.Path, "/orders/_doc/"):
			if strings.HasSuffix(r.URL.Path, "/missing") {
				w.WriteHeader(404)
				w.Write([]byte(`{"found":false}`))
				return
			}
			w.Write([]byte(`{"_index":"orders","_id":"1","found":true,"_source":{"user":"alice"}}`))
		case r.URL.Path == "/_bulk":
			b, _ := io.ReadAll(r.Body)
			bulks = append(bulks, string(b))
			items := []string{}
			for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
				res := `{"delete":{"status":200,"result":"deleted"}}`
				if strings.Contains(line, `"gone"`) {
					res = `{"delete":{"status":404,"result":"not_found"}}`
				}
				if strings.Contains(line, `"locked"`) {
					res = `{"delete":{"status":403,"error":{"type":"cluster_block_exception","reason":"index is read-only"}}}`
				}
				items = append(items, res)
			}
			w.Write([]byte(`{"errors":false,"items":[` + strings.Join(items, ",") + `]}`))
		case r.URL.Path == "/bad/_search":
			w.WriteHeader(400)
		default:
			w.WriteHeader(404)
			w.Write([]byte(`{"error":{"type":"index_not_found_exception","reason":"no such index"}}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func esServer(t *testing.T) (func(method, path, body string) (int, string), *[]string) {
	return esServerCfg(t, false)
}

func esServerCfg(t *testing.T, readOnly bool) (func(method, path, body string) (int, string), *[]string) {
	srv, seen := fakeES(t)
	h := newServer(config.Config{ReadOnly: readOnly, Elasticsearch: []config.Elasticsearch{{Name: "logs", Addr: srv.URL, Username: "u", Password: "p"}}})
	return func(method, path, body string) (int, string) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, strings.NewReader(body)))
		return w.Code, w.Body.String()
	}, seen
}

func TestElasticsearchViewer(t *testing.T) {
	call, seen := esServer(t)
	for _, c := range []struct{ path, want string }{
		{"/api/es", `"name":"logs"`},
		{"/api/es/logs/info", `"health":"yellow"`},
		{"/api/es/logs/indices", `"index":".kibana"`},
		{"/api/es/logs/search?index=orders&q=alice", `"user":"alice"`},
		{"/api/es/logs/search?index=orders", `"total":2`},
		{"/api/es/logs/doc?index=orders&id=1", `"found":true`},
	} {
		if code, body := call("GET", c.path, ""); code != 200 || !strings.Contains(body, c.want) {
			t.Fatalf("%s: %d %s (want %s)", c.path, code, body, c.want)
		}
	}
	if code, _ := call("GET", "/api/es/logs/doc?index=orders&id=missing", ""); code != 404 {
		t.Fatalf("missing doc: %d", code)
	}
	if code, _ := call("GET", "/api/es/nope/info", ""); code != 404 {
		t.Fatalf("unknown cluster: %d", code)
	}
	for _, bad := range []string{"orders/../_all", "a%20b", "x%2F..", "a/b"} {
		if code, _ := call("GET", "/api/es/logs/search?index="+bad, ""); code != 400 {
			t.Fatalf("index %q accepted: %d", bad, code)
		}
	}
	// an id with a slash is escaped, and the search carries the user's text as a query_string
	call("GET", "/api/es/logs/doc?index=orders&id=a/b", "")
	found := false
	for _, s := range *seen {
		if strings.Contains(s, "/orders/_doc/a%2Fb") {
			found = true
		}
	}
	if !found {
		t.Fatalf("id not escaped: %v", *seen)
	}
}

func TestElasticsearchConsoleIsReadOnly(t *testing.T) {
	call, seen := esServer(t)
	for _, c := range []struct {
		method, path string
		want         int
	}{
		{"GET", "_cat/indices", 200}, {"POST", "orders/_search", 200}, {"POST", "_count", 200},
		{"PUT", "orders", 403}, {"DELETE", "orders", 403}, {"POST", "orders/_doc", 403},
		{"POST", "_bulk", 403}, {"POST", "orders/_delete_by_query", 403}, {"POST", "orders/_update/1", 403},
		{"POST", "orders/_doc/1/_update", 403}, {"HEAD", "orders", 403}, {"PATCH", "x", 403},
		{"GET", "http://evil.example/x", 400}, {"GET", "a/../b", 400},
	} {
		body := `{"method":"` + c.method + `","path":"` + c.path + `","body":""}`
		if code, out := call("POST", "/api/es/logs/query", body); code != c.want {
			t.Fatalf("%s %s: got %d %s, want %d", c.method, c.path, code, out, c.want)
		}
	}
	for _, s := range *seen {
		if strings.HasPrefix(s, "PUT") || strings.HasPrefix(s, "DELETE") || strings.Contains(s, "_bulk") || strings.Contains(s, "evil") {
			t.Fatalf("a refused request reached the cluster: %s", s)
		}
	}
	// a 404 from the cluster is an answer, not a failure
	if code, out := call("POST", "/api/es/logs/query", `{"method":"GET","path":"nosuch/_mapping","body":""}`); code != 200 || !strings.Contains(out, `"status":404`) {
		t.Fatalf("%d %s", code, out)
	}
}

func TestElasticsearchDelete(t *testing.T) {
	call, _ := esServer(t)
	code, body := call("POST", "/api/es/logs/delete", `{"items":[{"index":"orders","id":"1"},{"index":"orders","id":"gone"},{"index":"orders","id":"locked"},{"index":"users","id":"a/b"}]}`)
	if code != 200 || !strings.Contains(body, `"deleted":2`) || !strings.Contains(body, `"not_found":1`) || !strings.Contains(body, "index is read-only") {
		t.Fatalf("%d %s", code, body)
	}
	if len(bulks) != 1 || strings.Count(bulks[0], `"delete"`) != 4 || strings.Contains(bulks[0], `"index"`+`:{`) || strings.Contains(bulks[0], `"update"`) {
		t.Fatalf("bulk body must carry only delete actions: %v", bulks)
	}
	for _, bad := range []string{
		`{"items":[{"index":"*","id":"1"}]}`, `{"items":[{"index":"a,b","id":"1"}]}`, `{"items":[{"index":"orders","id":""}]}`,
		`{"items":[{"index":"../x","id":"1"}]}`, `{"items":[]}`, `{"items":"nope"}`,
	} {
		if code, _ := call("POST", "/api/es/logs/delete", bad); code != 400 {
			t.Fatalf("%s accepted: %d", bad, code)
		}
	}
	if len(bulks) != 1 {
		t.Fatalf("a refused delete reached the cluster: %v", bulks)
	}
	// the console is still read-only
	if code, _ := call("POST", "/api/es/logs/query", `{"method":"POST","path":"_bulk","body":""}`); code != 403 {
		t.Fatalf("console _bulk: %d", code)
	}
}

func TestElasticsearchDeleteRefusedWhenReadOnly(t *testing.T) {
	call, _ := esServerCfg(t, true)
	if code, _ := call("POST", "/api/es/logs/delete", `{"items":[{"index":"orders","id":"1"}]}`); code != 403 {
		t.Fatalf("got %d", code)
	}
	if len(bulks) != 0 {
		t.Fatalf("read_only still deleted: %v", bulks)
	}
	if code, _ := call("GET", "/api/es/logs/indices", ""); code != 200 {
		t.Fatal("reads must still work")
	}
}
