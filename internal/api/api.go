// Package api is the HTTP layer: JSON for lists and actions, SSE for logs, WebSocket for the shell.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"

	"github.com/bondhansarker/devbuddy/internal/config"
	"github.com/bondhansarker/devbuddy/internal/esview"
	"github.com/bondhansarker/devbuddy/internal/kube"
	"github.com/bondhansarker/devbuddy/internal/redisview"
)

// Cluster is one Kubernetes connection from the config; Err says why it could not be set up.
type Cluster struct {
	Name    string
	Context string
	Client  *kube.Client
	Err     error
}

type Server struct {
	clusters []*Cluster
	byName   map[string]*Cluster
	fw       *kube.Forwards
	cfg      config.Config

	shMu   sync.Mutex
	shells map[string]*shell
	rd     *redisview.Manager
	es     *esview.Manager
}

func New(clusters []Cluster, web fs.FS, cfg config.Config) http.Handler {
	s := &Server{byName: map[string]*Cluster{}, fw: kube.NewForwards(), cfg: cfg, shells: map[string]*shell{}, rd: redisview.New(cfg.Redis), es: esview.New(cfg.Elasticsearch)}
	for i := range clusters {
		c := &clusters[i]
		if c.Client != nil {
			c.Client.Name = c.Name
		}
		s.clusters = append(s.clusters, c)
		s.byName[c.Name] = c
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/info", s.info)
	mux.HandleFunc("GET /api/kubernetes/ping", s.ping)
	mux.HandleFunc("GET /api/namespaces", s.namespaces)
	mux.HandleFunc("POST /api/namespaces", s.createNamespace)
	mux.HandleFunc("DELETE /api/namespaces/{name}", s.deleteNamespace)
	// ?namespace= narrows to one namespace; without it these list every namespace.
	mux.HandleFunc("GET /api/pods", s.pods)
	mux.HandleFunc("GET /api/pods/metrics", s.podMetrics)
	mux.HandleFunc("GET /api/pods/{ns}/{pod}/describe", s.describePod)
	mux.HandleFunc("POST /api/deployments/{ns}/{name}/scale", s.scaleDeployment)
	mux.HandleFunc("POST /api/deployments/{ns}/{name}/restart", s.restartDeployment)
	mux.HandleFunc("GET /api/services", s.services)
	mux.HandleFunc("GET /api/deployments", s.deployments)
	mux.HandleFunc("GET /api/jobs", s.jobs)
	mux.HandleFunc("GET /api/pods/{ns}/{pod}/logs", s.logs)
	mux.HandleFunc("GET /api/deployments/{ns}/{name}/logs", s.workloadLogs("deployments"))
	mux.HandleFunc("GET /api/services/{ns}/{name}/logs", s.workloadLogs("services"))
	mux.HandleFunc("GET /api/jobs/{ns}/{name}/logs", s.workloadLogs("jobs"))
	mux.HandleFunc("DELETE /api/jobs/{ns}/{name}", s.deleteJob)
	mux.HandleFunc("POST /api/jobs/{ns}/{name}/trigger", s.triggerJob)
	mux.HandleFunc("GET /api/deployments/{ns}/{name}/pod", s.podFor("deployments"))
	mux.HandleFunc("GET /api/services/{ns}/{name}/pod", s.podFor("services"))
	mux.HandleFunc("GET /api/pods/{ns}/{pod}/exec", s.exec)
	mux.HandleFunc("POST /api/pods/{ns}/{pod}/restart", s.restart)
	mux.HandleFunc("GET /api/shells", s.shellList)
	mux.HandleFunc("DELETE /api/shells", s.shellKill)
	mux.HandleFunc("GET /api/redis", s.redisList)
	mux.HandleFunc("GET /api/redis/{name}/dbs", s.redisDBs)
	mux.HandleFunc("GET /api/redis/{name}/keys", s.redisKeys)
	mux.HandleFunc("GET /api/redis/{name}/key", s.redisKey)
	mux.HandleFunc("POST /api/redis/{name}/delete", s.redisDelete)
	mux.HandleFunc("POST /api/redis/{name}/set", s.redisSet)
	mux.HandleFunc("POST /api/redis/{name}/export", s.redisExport)
	mux.HandleFunc("POST /api/redis/{name}/command", s.redisCommand)
	mux.HandleFunc("GET /api/es", s.esList)
	mux.HandleFunc("GET /api/es/{name}/info", s.esInfo)
	mux.HandleFunc("GET /api/es/{name}/indices", s.esIndices)
	mux.HandleFunc("GET /api/es/{name}/search", s.esSearch)
	mux.HandleFunc("GET /api/es/{name}/doc", s.esDoc)
	mux.HandleFunc("GET /api/es/{name}/mapping", s.esMapping)
	mux.HandleFunc("POST /api/es/{name}/query", s.esQuery)
	mux.HandleFunc("POST /api/es/{name}/delete", s.esDelete)
	mux.HandleFunc("GET /api/forwards", s.forwards)
	mux.HandleFunc("POST /api/forwards", s.startForward)
	mux.HandleFunc("DELETE /api/forwards/{id}", s.stopForward)
	mux.Handle("/", http.FileServerFS(web))
	h := sameOrigin(mux)
	if loopbackAddr(cfg.Addr) {
		h = loopbackHost(h) // the listener is loopback only (config.Validate); an unset Addr (tests) skips the check
	}
	return h
}

// loopbackAddr reports whether a listen address is localhost or a loopback IP.
func loopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	return err == nil && isLoopbackHost(host)
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// loopbackHost refuses a request whose Host header is not localhost or a loopback IP. Without it a
// malicious site could use DNS rebinding (its own name resolving to 127.0.0.1) to pass the Origin
// check and read or drive this API from the browser.
func loopbackHost(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if !isLoopbackHost(strings.Trim(host, "[]")) {
			fail(w, http.StatusForbidden, errors.New("unexpected Host header: open DevBuddy at http://localhost or http://127.0.0.1"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// sameOrigin refuses a browser request from another site (cross-site WebSocket
// hijack included): exec here is cluster access. No Origin = not a browser.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o := r.Header.Get("Origin"); o != "" {
			if u, err := url.Parse(o); err != nil || u.Host != r.Host {
				fail(w, http.StatusForbidden, errors.New("cross-origin request refused"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func ok(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// kubeFail maps an API server status to the same HTTP status, else 502.
func kubeFail(w http.ResponseWriter, err error) {
	var st apierrors.APIStatus
	if errors.As(err, &st) {
		fail(w, int(st.Status().Code), err)
		return
	}
	fail(w, http.StatusBadGateway, err)
}

var errReadOnly = errors.New("read_only is set in the config; this action is disabled")

// info is what the page needs at start: the UI settings and the Kubernetes connections.
func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	type cl struct {
		Name    string `json:"name"`
		Context string `json:"context"`
		Error   string `json:"error"`
	}
	list := []cl{}
	for _, c := range s.clusters {
		e := ""
		if c.Err != nil {
			e = c.Err.Error()
		}
		list = append(list, cl{c.Name, c.Context, e})
	}
	ok(w, struct {
		Clusters       []cl `json:"clusters"`
		RefreshSeconds int  `json:"refresh_seconds"`
		LogTail        int  `json:"log_tail"`
		ReadOnly       bool `json:"read_only"`
	}{list, s.cfg.RefreshSeconds, s.cfg.LogTail, s.cfg.ReadOnly})
}

// cluster picks the connection named by ?cluster= (the first one when it is missing). With none configured,
// or one that could not be set up, it answers 503 so the page can say "not connected".
func (s *Server) cluster(w http.ResponseWriter, r *http.Request) (*kube.Client, bool) {
	return s.clusterNamed(w, r.URL.Query().Get("cluster"))
}

func (s *Server) clusterNamed(w http.ResponseWriter, name string) (*kube.Client, bool) {
	if len(s.clusters) == 0 {
		fail(w, http.StatusServiceUnavailable, errors.New(`Kubernetes is not connected: add a "kubernetes" entry to the config`))
		return nil, false
	}
	c := s.clusters[0]
	if name != "" {
		if c = s.byName[name]; c == nil {
			fail(w, http.StatusNotFound, fmt.Errorf("unknown Kubernetes connection %q", name))
			return nil, false
		}
	}
	if c.Err != nil || c.Client == nil {
		fail(w, http.StatusServiceUnavailable, fmt.Errorf("Kubernetes connection %q is not connected: %v", c.Name, c.Err))
		return nil, false
	}
	return c.Client, true
}

// ping proves the chosen connection reaches its API server.
func (s *Server) ping(w http.ResponseWriter, r *http.Request) {
	k, ok2 := s.cluster(w, r)
	if !ok2 {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	v, err := k.Version(ctx)
	if err != nil {
		fail(w, http.StatusBadGateway, err)
		return
	}
	ok(w, map[string]string{"version": v})
}
