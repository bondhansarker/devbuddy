package api

import (
	"context"
	"net/http"
	"strings"
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
		ok(w, map[string]string{"pod": pod})
	}
}
