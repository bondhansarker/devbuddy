package kube

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// the polled lists must ask for resourceVersion=0 so the API server answers from its cache
func TestListsAskTheAPIServerCache(t *testing.T) {
	seen := map[string]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path] = r.URL.Query().Get("resourceVersion")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()
	cs, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	c, ctx := NewWith(cs), context.Background()
	c.Namespaces(ctx)
	c.Pods(ctx, "")
	c.Services(ctx, "")
	c.Deployments(ctx, "")
	c.Jobs(ctx, "")
	for _, p := range []string{"/api/v1/namespaces", "/api/v1/pods", "/api/v1/services", "/apis/apps/v1/deployments", "/apis/batch/v1/jobs"} {
		if seen[p] != "0" {
			t.Errorf("%s: resourceVersion=%q, want 0", p, seen[p])
		}
	}
}
