package api

import (
	"errors"
	"net/http"
)

type forwardReq struct {
	Cluster       string `json:"cluster"`
	Namespace     string `json:"namespace"`
	Pod           string `json:"pod"`
	ContainerPort int    `json:"container_port"`
	LocalPort     int    `json:"local_port"`
}

func (s *Server) startForward(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReadOnly {
		fail(w, http.StatusForbidden, errReadOnly)
		return
	}
	var p forwardReq
	if err := decodeBody(w, r, &p, maxBody); err != nil || p.Namespace == "" || p.Pod == "" {
		fail(w, http.StatusBadRequest, errors.New("namespace, pod and container_port required"))
		return
	}
	k, okc := s.clusterNamed(w, p.Cluster)
	if !okc {
		return
	}
	f, err := k.StartForward(r.Context(), s.fw, p.Namespace, p.Pod, p.ContainerPort, p.LocalPort)
	if err != nil {
		fail(w, http.StatusBadGateway, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	ok(w, f)
}

func (s *Server) forwards(w http.ResponseWriter, r *http.Request) { ok(w, s.fw.List()) }

// stopForwards stops every forward of ?cluster= (all of them without it): switching a connection off closes what it opened.
func (s *Server) stopForwards(w http.ResponseWriter, r *http.Request) {
	ok(w, map[string]int{"stopped": s.fw.StopCluster(r.URL.Query().Get("cluster"))})
}

func (s *Server) stopForward(w http.ResponseWriter, r *http.Request) {
	if !s.fw.Stop(r.PathValue("id")) {
		fail(w, http.StatusNotFound, errors.New("forward not found"))
		return
	}
	ok(w, map[string]string{"message": "forward stopped"})
}
