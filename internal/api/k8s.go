package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/bondhansarker/devbuddy/internal/kube"
)

func (s *Server) namespaces(w http.ResponseWriter, r *http.Request) {
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	l, err := k.Namespaces(r.Context())
	if err != nil {
		kubeFail(w, err)
		return
	}
	ok(w, l)
}

// namespaces reads ?namespace=a,b. Each one is listed on its own, so a role
// limited to those namespaces works; none given means every namespace.
func namespaces(r *http.Request) []string {
	var out []string
	for _, n := range strings.Split(r.URL.Query().Get("namespace"), ",") {
		if n = strings.TrimSpace(n); n != "" {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func listIn[T any](r *http.Request, list func(ctx context.Context, ns string) ([]T, error)) ([]T, error) {
	out := []T{}
	for _, ns := range namespaces(r) {
		l, err := list(r.Context(), ns)
		if err != nil {
			return nil, err
		}
		out = append(out, l...)
	}
	return out, nil
}

func (s *Server) pods(w http.ResponseWriter, r *http.Request) {
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	l, err := listIn(r, k.Pods)
	if err != nil {
		kubeFail(w, err)
		return
	}
	ok(w, l)
}

func (s *Server) services(w http.ResponseWriter, r *http.Request) {
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	l, err := listIn(r, k.Services)
	if err != nil {
		kubeFail(w, err)
		return
	}
	ok(w, l)
}

func (s *Server) deployments(w http.ResponseWriter, r *http.Request) {
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	l, err := listIn(r, k.Deployments)
	if err != nil {
		kubeFail(w, err)
		return
	}
	ok(w, l)
}

func (s *Server) jobs(w http.ResponseWriter, r *http.Request) {
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	l, err := listIn(r, k.Jobs)
	if err != nil {
		kubeFail(w, err)
		return
	}
	ok(w, l)
}

func (s *Server) restart(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReadOnly {
		fail(w, http.StatusForbidden, errReadOnly)
		return
	}
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	if err := k.Restart(r.Context(), r.PathValue("ns"), r.PathValue("pod")); err != nil {
		kubeFail(w, err)
		return
	}
	ok(w, map[string]string{"message": "pod deleted"})
}

func (s *Server) triggerJob(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReadOnly {
		fail(w, http.StatusForbidden, errReadOnly)
		return
	}
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	name, err := k.TriggerJob(r.Context(), r.PathValue("ns"), r.PathValue("name"))
	if err != nil {
		kubeFail(w, err)
		return
	}
	ok(w, map[string]string{"name": name})
}

func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReadOnly {
		fail(w, http.StatusForbidden, errReadOnly)
		return
	}
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	if err := k.DeleteJob(r.Context(), r.PathValue("ns"), r.PathValue("name")); err != nil {
		kubeFail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}

// podFor tells the UI which pod to open a shell or port forward on for a deployment or service.
func (s *Server) podFor(kind string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		k, okc := s.cluster(w, r)
		if !okc {
			return
		}
		pod, err := k.PodFor(r.Context(), kind, r.PathValue("ns"), r.PathValue("name"))
		if err != nil {
			kubeFail(w, err)
			return
		}
		// the pod's details come with its name, so Shell and Forward do not depend on the Pods page having been loaded
		details, err := k.GetPod(r.Context(), r.PathValue("ns"), pod)
		if err != nil {
			kubeFail(w, err)
			return
		}
		ok(w, map[string]any{"pod": pod, "details": details})
	}
}

func (s *Server) describePod(w http.ResponseWriter, r *http.Request) {
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	d, err := k.Describe(r.Context(), r.PathValue("ns"), r.PathValue("pod"))
	if err != nil {
		kubeFail(w, err)
		return
	}
	ok(w, d)
}

// podMetrics is optional: 404 tells the UI this cluster has no metrics-server, so it hides the columns.
func (s *Server) podMetrics(w http.ResponseWriter, r *http.Request) {
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	l, err := listIn(r, k.PodMetrics)
	if errors.Is(err, kube.ErrNoMetrics) {
		fail(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		kubeFail(w, err)
		return
	}
	ok(w, l)
}

func (s *Server) scaleDeployment(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReadOnly {
		fail(w, http.StatusForbidden, errReadOnly)
		return
	}
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	var body struct {
		Replicas *int32 `json:"replicas"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.Replicas == nil || *body.Replicas < 0 || *body.Replicas > 10000 {
		fail(w, http.StatusBadRequest, errors.New(`body must be {"replicas": 0..10000}`))
		return
	}
	if err := k.ScaleDeployment(r.Context(), r.PathValue("ns"), r.PathValue("name"), *body.Replicas); err != nil {
		kubeFail(w, err)
		return
	}
	ok(w, map[string]int32{"replicas": *body.Replicas})
}

func (s *Server) restartDeployment(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReadOnly {
		fail(w, http.StatusForbidden, errReadOnly)
		return
	}
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	if err := k.RestartDeployment(r.Context(), r.PathValue("ns"), r.PathValue("name")); err != nil {
		kubeFail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}

func (s *Server) createNamespace(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReadOnly {
		fail(w, http.StatusForbidden, errReadOnly)
		return
	}
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&body); err != nil || body.Name == "" {
		fail(w, http.StatusBadRequest, errors.New(`body must be {"name": "..."}`))
		return
	}
	if err := k.CreateNamespace(r.Context(), body.Name); err != nil {
		if errors.Is(err, kube.ErrBadNamespace) {
			fail(w, http.StatusBadRequest, err)
			return
		}
		kubeFail(w, err)
		return
	}
	ok(w, map[string]string{"name": body.Name})
}

func (s *Server) deleteNamespace(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReadOnly {
		fail(w, http.StatusForbidden, errReadOnly)
		return
	}
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	if err := k.DeleteNamespace(r.Context(), r.PathValue("name")); err != nil {
		if errors.Is(err, kube.ErrBadNamespace) || errors.Is(err, kube.ErrProtectedNamespace) {
			fail(w, http.StatusBadRequest, err)
			return
		}
		kubeFail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}

// nodes is the cluster-wide health view: every node with readiness, pressure, capacity, requests and use.
func (s *Server) nodes(w http.ResponseWriter, r *http.Request) {
	k, okc := s.cluster(w, r)
	if !okc {
		return
	}
	n, err := k.Nodes(r.Context())
	if err != nil {
		kubeFail(w, err)
		return
	}
	ok(w, n)
}
