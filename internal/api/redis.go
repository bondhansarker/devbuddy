package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/bondhansarker/devbuddy/internal/redisview"
)

// redisCtx bounds one Redis call so a dead server cannot hang a request.
func redisCtx(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 15*time.Second)
}

// redisFail maps the package errors to HTTP statuses, anything else is a failure of the server behind it.
func redisFail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, redisview.ErrUnknown), errors.Is(err, redisview.ErrNotFound):
		fail(w, http.StatusNotFound, err)
	case errors.Is(err, redisview.ErrCommand):
		fail(w, http.StatusForbidden, err)
	case errors.Is(err, redisview.ErrDB), errors.Is(err, redisview.ErrBadArg):
		fail(w, http.StatusBadRequest, err)
	default:
		fail(w, http.StatusBadGateway, err)
	}
}

func dbParam(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("db"))
	return n
}

func (s *Server) redisList(w http.ResponseWriter, r *http.Request) { ok(w, s.rd.Instances()) }

func (s *Server) redisDBs(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := redisCtx(r)
	defer cancel()
	dbs, n, err := s.rd.DBs(ctx, r.PathValue("name"))
	if err != nil {
		redisFail(w, err)
		return
	}
	ok(w, map[string]any{"dbs": dbs, "databases": n})
}

func (s *Server) redisKeys(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := redisCtx(r)
	defer cancel()
	q := r.URL.Query()
	cursor, _ := strconv.ParseUint(q.Get("cursor"), 10, 64)
	limit, _ := strconv.Atoi(q.Get("count"))
	next, keys, err := s.rd.Scan(ctx, r.PathValue("name"), dbParam(r), q.Get("match"), q.Get("type"), cursor, limit)
	if err != nil {
		redisFail(w, err)
		return
	}
	ok(w, map[string]any{"cursor": strconv.FormatUint(next, 10), "keys": keys})
}

func (s *Server) redisKey(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := redisCtx(r)
	defer cancel()
	v, err := s.rd.Get(ctx, r.PathValue("name"), dbParam(r), r.URL.Query().Get("key"))
	if err != nil {
		redisFail(w, err)
		return
	}
	ok(w, v)
}

func (s *Server) redisDelete(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReadOnly {
		fail(w, http.StatusForbidden, errReadOnly)
		return
	}
	var body struct {
		DB   int      `json:"db"`
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, errors.New("body must be {db, keys}"))
		return
	}
	ctx, cancel := redisCtx(r)
	defer cancel()
	n, err := s.rd.Del(ctx, r.PathValue("name"), body.DB, body.Keys)
	if err != nil {
		redisFail(w, err)
		return
	}
	ok(w, map[string]int64{"deleted": n})
}

// redisCommand runs one console line. Whether it may run (and a read_only server's limits) is
// decided by redisview, not by the page.
func (s *Server) redisCommand(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DB      int    `json:"db"`
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, errors.New("body must be {db, command}"))
		return
	}
	args, err := redisview.ParseCommand(body.Command)
	if err != nil {
		fail(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := redisCtx(r)
	defer cancel()
	out, err := s.rd.Exec(ctx, r.PathValue("name"), body.DB, args, s.cfg.ReadOnly)
	if err != nil {
		redisFail(w, err)
		return
	}
	ok(w, map[string]string{"output": out})
}

func (s *Server) redisSet(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReadOnly {
		fail(w, http.StatusForbidden, errReadOnly)
		return
	}
	var body struct {
		DB    int    `json:"db"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20)).Decode(&body); err != nil || body.Key == "" {
		fail(w, http.StatusBadRequest, errors.New("body must be {db, key, value}"))
		return
	}
	ctx, cancel := redisCtx(r)
	defer cancel()
	if err := s.rd.SetString(ctx, r.PathValue("name"), body.DB, body.Key, body.Value); err != nil {
		redisFail(w, err)
		return
	}
	ok(w, map[string]bool{"ok": true})
}

// redisExport only reads, so it works under read_only too.
func (s *Server) redisExport(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DB   int      `json:"db"`
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		fail(w, http.StatusBadRequest, errors.New("body must be {db, keys}"))
		return
	}
	ctx, cancel := redisCtx(r)
	defer cancel()
	items, err := s.rd.Export(ctx, r.PathValue("name"), body.DB, body.Keys)
	if err != nil {
		redisFail(w, err)
		return
	}
	ok(w, items)
}
