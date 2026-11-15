package api

import (
	"net/http"
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

func newTestServer() http.Handler { return newServer(config.Config{}) }

func newServer(cfg config.Config) http.Handler {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet"}}},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "app", Image: "ghcr.io/example/app:v1", Ports: []corev1.ContainerPort{{ContainerPort: 8080}}}}},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"},
		Spec: corev1.ServiceSpec{Type: corev1.ServiceTypeClusterIP, ClusterIP: "10.0.0.1",
			Ports: []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP}}},
	}
	dep := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "default"}}
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "default"}}
	return New([]Cluster{{Name: "test", Context: "test", Client: kube.NewWith(fake.NewSimpleClientset(pod, svc, dep, ns))}}, fstest.MapFS{"index.html": {Data: []byte("hi")}}, cfg)
}

func do(h http.Handler, method, path, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, nil)
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestPodsListsPortsAndOwner(t *testing.T) {
	w := do(newTestServer(), "GET", "/api/pods?namespace=default", "")
	b := w.Body.String()
	if w.Code != 200 || !strings.Contains(b, `"container_port":8080`) || !strings.Contains(b, `"images":["ghcr.io/example/app:v1"]`) || !strings.Contains(b, `"owner_kind":"ReplicaSet"`) {
		t.Fatalf("got %d %s", w.Code, b)
	}
}

func TestServicesDeploymentsNamespaces(t *testing.T) {
	h := newTestServer()
	for path, want := range map[string]string{
		"/api/namespaces":                    `"name":"default"`,
		"/api/services?namespace=default":    `"ports":["80/TCP"]`,
		"/api/deployments?namespace=default": `"ready":"0/1"`,
	} {
		w := do(h, "GET", path, "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), want) {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
	}
	if w := do(h, "GET", "/api/services?namespace=default", ""); !strings.Contains(w.Body.String(), `"external_ip":null`) {
		t.Fatalf("unset external_ip must be null: %s", w.Body)
	}
}

func TestListsAcrossAllNamespaces(t *testing.T) {
	h := newTestServer()
	for _, path := range []string{"/api/pods", "/api/services", "/api/deployments"} {
		w := do(h, "GET", path, "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), `"namespace":"default"`) {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
	}
	if w := do(h, "GET", "/api/pods?namespace=other", ""); strings.Contains(w.Body.String(), `"web"`) {
		t.Fatalf("namespace filter ignored: %s", w.Body)
	}
}

func TestSeveralNamespaces(t *testing.T) {
	h := newTestServer()
	w := do(h, "GET", "/api/pods?namespace=default,other", "")
	if w.Code != 200 || strings.Count(w.Body.String(), `"name":"web"`) != 1 {
		t.Fatalf("%d %s", w.Code, w.Body)
	}
	if w := do(h, "GET", "/api/pods?namespace=other,another", ""); strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("want empty list, got %s", w.Body)
	}
}

func TestRestartDeletesPod(t *testing.T) {
	h := newTestServer()
	if w := do(h, "POST", "/api/pods/default/web/restart", ""); w.Code != 200 {
		t.Fatalf("restart: %d %s", w.Code, w.Body)
	}
	if w := do(h, "POST", "/api/pods/default/web/restart", ""); w.Code != 404 {
		t.Fatalf("second restart want 404, got %d", w.Code)
	}
}

func TestReadOnlyBlocksWrites(t *testing.T) {
	c := config.Config{ReadOnly: true}
	h := newServer(c)
	for _, path := range []string{"/api/pods/default/web/restart", "/api/forwards"} {
		if w := do(h, "POST", path, ""); w.Code != 403 {
			t.Fatalf("%s: want 403, got %d", path, w.Code)
		}
	}
	if w := do(h, "GET", "/api/pods/default/web/exec", ""); w.Code != 403 {
		t.Fatalf("exec: want 403, got %d", w.Code)
	}
	if w := do(h, "GET", "/api/pods?namespace=default", ""); w.Code != 200 {
		t.Fatalf("reads must still work, got %d", w.Code)
	}
}

func TestCrossOriginRefused(t *testing.T) {
	h := newTestServer()
	if w := do(h, "POST", "/api/pods/default/web/restart", "http://evil.example"); w.Code != 403 {
		t.Fatalf("want 403, got %d", w.Code)
	}
	if w := do(h, "GET", "/api/namespaces", "http://example.com"); w.Code != 200 {
		t.Fatalf("same origin want 200, got %d", w.Code)
	}
}
