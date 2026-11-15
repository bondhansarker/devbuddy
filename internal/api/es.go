package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/bondhansarker/devbuddy/internal/esview"
)

func esCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 25*time.Second)
}

func esFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, esview.ErrUnknown), errors.Is(err, esview.ErrNotFound):
		fail(w, http.StatusNotFound, err)
	case errors.Is(err, esview.ErrBadArg):
		fail(w, http.StatusBadRequest, err)
	case errors.Is(err, esview.ErrDenied):
		fail(w, http.StatusForbidden, err)
	default:
		fail(w, http.StatusBadGateway, err)
	}
}

func (s *Server) esList(w http.ResponseWriter, r *http.Request) { ok(w, s.es.Instances()) }

func (s *Server) esInfo(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := esCtx(r)
	defer cancel()
	in, err := s.es.Info(ctx, r.PathValue("name"))
	if err != nil {
		esFail(w, err)
		return
	}
	ok(w, in)
}

func (s *Server) esIndices(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := esCtx(r)
	defer cancel()
	l, err := s.es.Indices(ctx, r.PathValue("name"))
	if err != nil {
		esFail(w, err)
		return
	}
	ok(w, l)
}

func (s *Server) esSearch(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := esCtx(r)
	defer cancel()
	q := r.URL.Query()
	from, _ := strconv.Atoi(q.Get("from"))
	size, _ := strconv.Atoi(q.Get("size"))
	res, err := s.es.Search(ctx, r.PathValue("name"), q.Get("index"), q.Get("q"), from, size, q.Get("sort"))
	if err != nil {
		esFail(w, err)
		return
	}
	ok(w, res)
}

func (s *Server) esDoc(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := esCtx(r)
	defer cancel()
	q := r.URL.Query()
	raw, err := s.es.Doc(ctx, r.PathValue("name"), q.Get("index"), q.Get("id"))
	if err != nil {
		esFail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

func (s *Server) esMapping(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := esCtx(r)
	defer cancel()
	raw, err := s.es.Mapping(ctx, r.PathValue("name"), r.URL.Query().Get("index"))
	if err != nil {
		esFail(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(raw)
}

// esQuery runs a read-only REST call. The cluster's status and body come back as data: a 404 from
// the cluster is an answer to show, not a failure of this request.
func (s *Server) esQuery(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Method string `json:"method"`
		Path   string `json:"path"`
		Body   string `json:"body"`
	}
	if err := decodeBody(w, r, &body, maxBody); err != nil {
		fail(w, http.StatusBadRequest, errors.New("body must be {method, path, body}"))
		return
	}
	ctx, cancel := esCtx(r)
	defer cancel()
	status, data, err := s.es.Query(ctx, r.PathValue("name"), body.Method, body.Path, []byte(body.Body))
	if err != nil {
		esFail(w, err)
		return
	}
	ok(w, map[string]any{"status": status, "body": scrub(string(data))})
}

// esDelete deletes documents by index and id (never by pattern), unless read_only is set. The Query tab
// stays read-only: this endpoint is the only write path, and it can only delete documents.
func (s *Server) esDelete(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReadOnly {
		fail(w, http.StatusForbidden, errReadOnly)
		return
	}
	var body struct {
		Items []esview.DocRef `json:"items"`
	}
	if err := decodeBody(w, r, &body, maxBody); err != nil {
		fail(w, http.StatusBadRequest, errors.New("body must be {items: [{index, id}]}"))
		return
	}
	ctx, cancel := esCtx(r)
	defer cancel()
	res, err := s.es.Delete(ctx, r.PathValue("name"), body.Items)
	if err != nil {
		esFail(w, err)
		return
	}
	ok(w, res)
}
