package api

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/bondhansarker/devbuddy/internal/config"
	"github.com/bondhansarker/devbuddy/internal/kube"
)

func nsCluster(name, ns string) Cluster {
	cs := fake.NewSimpleClientset(&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "d-" + ns, Namespace: ns}})
	return Cluster{Name: name, Context: name + "-ctx", Client: kube.NewWith(cs)}
}

func serve(clusters []Cluster) func(method, path string) (int, string) {
	h := New(clusters, fstest.MapFS{"index.html": {Data: []byte("hi")}}, config.Config{})
	return func(method, path string) (int, string) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w.Code, w.Body.String()
	}
}

func TestSeveralKubernetesConnections(t *testing.T) {
	call := serve([]Cluster{nsCluster("dev", "dev-ns"), nsCluster("prod", "prod-ns"), {Name: "bad", Context: "bad-ctx", Err: errors.New("kubeconfig missing")}})
	for _, c := range []struct {
		path string
		code int
		want string
	}{
		{"/api/namespaces", 200, "dev-ns"}, // no ?cluster= : the first connection
		{"/api/namespaces?cluster=dev", 200, "dev-ns"},
		{"/api/namespaces?cluster=prod", 200, "prod-ns"}, // a different cluster, different data
		{"/api/deployments?cluster=prod", 200, "d-prod-ns"},
		{"/api/namespaces?cluster=nope", 404, "unknown Kubernetes connection"},
		{"/api/namespaces?cluster=bad", 503, "kubeconfig missing"}, // set up failed: not connected
		{"/api/kubernetes/ping?cluster=prod", 200, "version"},
		{"/api/info", 200, `"name":"prod"`},
		{"/api/info", 200, `"error":"kubeconfig missing"`},
	} {
		code, body := call("GET", c.path)
		if code != c.code || !strings.Contains(body, c.want) {
			t.Fatalf("%s: %d %s (want %d %s)", c.path, code, body, c.code, c.want)
		}
	}
	if _, body := call("GET", "/api/namespaces?cluster=prod"); strings.Contains(body, "dev-ns") {
		t.Fatalf("prod answered with dev data: %s", body)
	}
}

func TestNoKubernetesConfiguredIsNotConnected(t *testing.T) {
	call := serve(nil)
	for _, p := range []string{"/api/namespaces", "/api/pods", "/api/services", "/api/deployments", "/api/jobs", "/api/kubernetes/ping"} {
		if code, body := call("GET", p); code != 503 || !strings.Contains(body, "not connected") {
			t.Fatalf("%s: %d %s", p, code, body)
		}
	}
	if code, body := call("GET", "/api/info"); code != 200 || !strings.Contains(body, `"clusters":[]`) {
		t.Fatalf("info must still work without Kubernetes: %d %s", code, body)
	}
	if code, _ := call("GET", "/api/redis"); code != 200 { // the data stores do not depend on Kubernetes
		t.Fatalf("redis list: %d", code)
	}
}
