package esview

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bondhansarker/devbuddy/internal/config"
)

func TestSearchCapsTheCountedHits(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		body = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"took":1,"hits":{"total":{"value":10000,"relation":"gte"},"hits":[]}}`))
	}))
	defer srv.Close()
	m := New([]config.Elasticsearch{{Name: "e", Addr: srv.URL}})
	res, err := m.Search(context.Background(), "e", "", "", 0, 50, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"track_total_hits":10000`) {
		t.Fatalf("an exact count was requested: %s", body)
	}
	if res.Total != 10000 || res.Relation != "gte" {
		t.Fatalf("got %+v (the UI reads gte as ≥)", res)
	}
}
