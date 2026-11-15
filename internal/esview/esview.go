// Package esview is a read-only viewer for Elasticsearch / OpenSearch clusters named in the config:
// list indices, search documents, read one document or a mapping, and run read-only REST calls.
// It speaks plain HTTP+JSON, so it works with both products and needs no client library.
package esview

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bondhansarker/devbuddy/internal/config"
)

var (
	ErrUnknown  = errors.New("unknown elasticsearch cluster")
	ErrNotFound = errors.New("not found")
	ErrBadArg   = errors.New("bad argument")
	ErrDenied   = errors.New("not allowed: this page is read-only")
)

const (
	maxBody      = 8 << 20 // bytes read back from a cluster
	maxSize      = 200     // documents per search page
	maxReqBody   = 1 << 20 // bytes accepted for a console request body
	maxFromSize  = 10000   // Elasticsearch's default result window
	maxTotalHits = 10000   // the count stops here (the UI shows "≥ 10,000"): an exact count over a big index is costly
)

type Instance struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
}

type conn struct {
	cfg    config.Elasticsearch
	base   string
	client *http.Client
}

type Manager struct {
	conns map[string]*conn
	order []Instance
}

func New(list []config.Elasticsearch) *Manager {
	m := &Manager{conns: map[string]*conn{}}
	for _, e := range list {
		tr := &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConnsPerHost: 4, IdleConnTimeout: 60 * time.Second}
		if e.Insecure {
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // the user asked for it in the config
		}
		m.conns[e.Name] = &conn{cfg: e, base: config.ESBaseURL(e.Addr), client: &http.Client{Transport: tr, Timeout: 20 * time.Second}}
		m.order = append(m.order, Instance{e.Name, config.ESBaseURL(e.Addr)})
	}
	return m
}

func (m *Manager) Instances() []Instance {
	if m == nil || m.order == nil {
		return []Instance{}
	}
	return m.order
}

// do sends one request to the cluster. path is an absolute path with an optional query string; it can
// not reach another host because only the configured base URL is ever used.
func (m *Manager) do(ctx context.Context, name, method, path string, body []byte) (int, []byte, error) {
	c, ok := m.conns[name]
	if !ok {
		return 0, nil, ErrUnknown
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	switch {
	case c.cfg.APIKey != "":
		req.Header.Set("Authorization", "ApiKey "+c.cfg.APIKey)
	case c.cfg.Username != "":
		req.SetBasicAuth(c.cfg.Username, c.cfg.Password)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	return resp.StatusCode, data, err
}

// esError turns a failed response into an error with the cluster's own reason.
func esError(status int, data []byte) error {
	var e struct {
		Error json.RawMessage `json:"error"`
	}
	msg := strings.TrimSpace(string(data))
	if json.Unmarshal(data, &e) == nil && len(e.Error) > 0 {
		var obj struct {
			Type, Reason string
		}
		if json.Unmarshal(e.Error, &obj) == nil && obj.Reason != "" {
			msg = obj.Type + ": " + obj.Reason
		} else {
			msg = string(e.Error)
		}
	}
	if len(msg) > 500 {
		msg = msg[:500] + "…"
	}
	switch {
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, msg)
	case status >= 400 && status < 500 && status != 401 && status != 403:
		return fmt.Errorf("%w: %s", ErrBadArg, msg)
	}
	return fmt.Errorf("elasticsearch answered %d: %s", status, msg)
}

type Info struct {
	Cluster string `json:"cluster"`
	Version string `json:"version"`
	Flavor  string `json:"flavor"`
	Health  string `json:"health"` // green, yellow, red; empty when _cluster/health is not allowed
}

func (m *Manager) Info(ctx context.Context, name string) (Info, error) {
	st, data, err := m.do(ctx, name, "GET", "/", nil)
	if err != nil {
		return Info{}, err
	}
	if st >= 400 {
		return Info{}, esError(st, data)
	}
	var r struct {
		ClusterName string `json:"cluster_name"`
		Version     struct {
			Number       string `json:"number"`
			Distribution string `json:"distribution"`
		} `json:"version"`
	}
	_ = json.Unmarshal(data, &r)
	in := Info{Cluster: r.ClusterName, Version: r.Version.Number, Flavor: "Elasticsearch"}
	if r.Version.Distribution == "opensearch" {
		in.Flavor = "OpenSearch"
	}
	if st, data, err := m.do(ctx, name, "GET", "/_cluster/health", nil); err == nil && st < 400 {
		var h struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(data, &h)
		in.Health = h.Status
	}
	return in, nil
}

type Index struct {
	Index  string `json:"index"`
	Health string `json:"health"`
	Status string `json:"status"`
	Docs   int64  `json:"docs"`
	Bytes  int64  `json:"bytes"`
	Pri    string `json:"pri"`
	Rep    string `json:"rep"`
}

func (m *Manager) Indices(ctx context.Context, name string) ([]Index, error) {
	st, data, err := m.do(ctx, name, "GET", "/_cat/indices?format=json&bytes=b&h=index,health,status,docs.count,store.size,pri,rep", nil)
	if err != nil {
		return nil, err
	}
	if st >= 400 {
		return nil, esError(st, data)
	}
	var rows []map[string]any
	if err := json.Unmarshal(data, &rows); err != nil {
		return nil, fmt.Errorf("unexpected answer from the cluster: %w", err)
	}
	num := func(v any) int64 { n, _ := strconv.ParseInt(fmt.Sprint(v), 10, 64); return n }
	str := func(v any) string {
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
	out := make([]Index, 0, len(rows))
	for _, r := range rows {
		out = append(out, Index{str(r["index"]), str(r["health"]), str(r["status"]), num(r["docs.count"]), num(r["store.size"]), str(r["pri"]), str(r["rep"])})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

var indexRe = regexp.MustCompile(`^[A-Za-z0-9._*,:\-]+$`) // names, wildcards and comma lists; no slash, so no path tricks

// cleanIndex validates an index expression; empty means every index.
func cleanIndex(index string) (string, error) {
	if index == "" {
		return "_all", nil
	}
	if !indexRe.MatchString(index) || strings.Contains(index, "..") {
		return "", fmt.Errorf("%w: index %q", ErrBadArg, index)
	}
	return index, nil
}

type Hit struct {
	Index  string          `json:"index"`
	ID     string          `json:"id"`
	Score  *float64        `json:"score"`
	Source json.RawMessage `json:"source"`
}

type SearchResult struct {
	Total    int64  `json:"total"`
	Relation string `json:"relation"` // "eq", or "gte" when Elasticsearch stopped counting
	Took     int64  `json:"took"`
	Hits     []Hit  `json:"hits"`
}

// Search runs a query_string search (Lucene syntax; empty = every document) on one or more indices.
func (m *Manager) Search(ctx context.Context, name, index, q string, from, size int, sortSpec string) (SearchResult, error) {
	idx, err := cleanIndex(index)
	if err != nil {
		return SearchResult{}, err
	}
	if size < 1 || size > maxSize {
		size = 50
	}
	if from < 0 || from+size > maxFromSize {
		return SearchResult{}, fmt.Errorf("%w: results are limited to the first %d (narrow the search)", ErrBadArg, maxFromSize)
	}
	body := map[string]any{"from": from, "size": size, "track_total_hits": maxTotalHits}
	if strings.TrimSpace(q) == "" {
		body["query"] = map[string]any{"match_all": map[string]any{}}
	} else {
		body["query"] = map[string]any{"query_string": map[string]any{"query": q, "default_operator": "AND", "lenient": true}}
	}
	if f, dir, ok := strings.Cut(sortSpec, ":"); ok && f != "" && (dir == "asc" || dir == "desc") {
		body["sort"] = []any{map[string]any{f: map[string]any{"order": dir, "unmapped_type": "keyword"}}}
	}
	b, _ := json.Marshal(body)
	st, data, err := m.do(ctx, name, "POST", "/"+url.PathEscape(idx)+"/_search?ignore_unavailable=true", b)
	if err != nil {
		return SearchResult{}, err
	}
	if st >= 400 {
		return SearchResult{}, esError(st, data)
	}
	var r struct {
		Took int64 `json:"took"`
		Hits struct {
			Total struct {
				Value    int64  `json:"value"`
				Relation string `json:"relation"`
			} `json:"total"`
			Hits []struct {
				Index  string          `json:"_index"`
				ID     string          `json:"_id"`
				Score  *float64        `json:"_score"`
				Source json.RawMessage `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return SearchResult{}, fmt.Errorf("unexpected answer from the cluster: %w", err)
	}
	out := SearchResult{Total: r.Hits.Total.Value, Relation: r.Hits.Total.Relation, Took: r.Took, Hits: []Hit{}}
	for _, h := range r.Hits.Hits {
		out.Hits = append(out.Hits, Hit{h.Index, h.ID, h.Score, h.Source})
	}
	return out, nil
}

// Doc returns one document as the cluster stores it (_source and metadata).
func (m *Manager) Doc(ctx context.Context, name, index, id string) (json.RawMessage, error) {
	idx, err := cleanIndex(index)
	if err != nil || index == "" || id == "" {
		return nil, fmt.Errorf("%w: index and id are required", ErrBadArg)
	}
	st, data, err := m.do(ctx, name, "GET", "/"+url.PathEscape(idx)+"/_doc/"+url.PathEscape(id), nil)
	if err != nil {
		return nil, err
	}
	if st >= 400 && st != http.StatusNotFound {
		return nil, esError(st, data)
	}
	if st == http.StatusNotFound {
		return nil, ErrNotFound
	}
	return data, nil
}

// Mapping returns the mapping of the index (or indices).
func (m *Manager) Mapping(ctx context.Context, name, index string) (json.RawMessage, error) {
	idx, err := cleanIndex(index)
	if err != nil {
		return nil, err
	}
	st, data, err := m.do(ctx, name, "GET", "/"+url.PathEscape(idx)+"/_mapping", nil)
	if err != nil {
		return nil, err
	}
	if st >= 400 {
		return nil, esError(st, data)
	}
	return data, nil
}

// POST is allowed only on these read endpoints (anything else that writes is not on the list).
var readPost = map[string]bool{"_search": true, "_count": true, "_msearch": true, "_mget": true, "_analyze": true,
	"_field_caps": true, "_validate": true, "_explain": true, "_termvectors": true, "_mtermvectors": true, "_sql": true,
	"_knn_search": true, "_search_shards": true, "_eql": true, "_rank_eval": true}

// AllowedQuery decides whether a console request may run: every GET (GET only reads), and POST only on
// the read endpoints above. PUT, DELETE, HEAD, PATCH and the rest are refused.
func AllowedQuery(method, path string) error {
	switch strings.ToUpper(method) {
	case "GET":
		return nil
	case "POST":
		p := path
		if i := strings.IndexByte(p, '?'); i >= 0 {
			p = p[:i]
		}
		for _, seg := range strings.Split(p, "/") {
			if readPost[seg] {
				return nil
			}
		}
		return fmt.Errorf("%w: POST %s writes or is unknown (POST is allowed on _search, _count, _msearch, _mget, _analyze, …)", ErrDenied, path)
	}
	return fmt.Errorf("%w: %s", ErrDenied, strings.ToUpper(method))
}

// Query runs a read-only REST call and returns the status and the raw JSON (or text) body.
func (m *Manager) Query(ctx context.Context, name, method, path string, body []byte) (int, []byte, error) {
	if err := AllowedQuery(method, path); err != nil {
		return 0, nil, err
	}
	if len(body) > maxReqBody {
		return 0, nil, fmt.Errorf("%w: request body over 1 MB", ErrBadArg)
	}
	u, err := url.Parse("/" + strings.TrimLeft(path, "/"))
	if err != nil || u.Host != "" || u.Scheme != "" || strings.Contains(u.Path, "..") || strings.Contains(u.Path, "//") {
		return 0, nil, fmt.Errorf("%w: path", ErrBadArg)
	}
	return m.do(ctx, name, strings.ToUpper(method), u.RequestURI(), body)
}

var exactIndexRe = regexp.MustCompile(`^[A-Za-z0-9._:\-]+$`) // one concrete index: no wildcard, no comma list

type DocRef struct {
	Index string `json:"index"`
	ID    string `json:"id"`
}

type DeleteFailure struct {
	Index string `json:"index"`
	ID    string `json:"id"`
	Error string `json:"error"`
}

type DeleteResult struct {
	Deleted  int             `json:"deleted"`
	NotFound int             `json:"not_found"` // already gone
	Failed   []DeleteFailure `json:"failed"`
}

const maxDelete = 500

// Delete removes documents by index and id with one _bulk call that can only carry delete actions. Each
// document is deleted from the exact index it was listed in: wildcards and comma lists are refused.
func (m *Manager) Delete(ctx context.Context, name string, docs []DocRef) (DeleteResult, error) {
	res := DeleteResult{Failed: []DeleteFailure{}}
	if len(docs) == 0 || len(docs) > maxDelete {
		return res, fmt.Errorf("%w: give between 1 and %d documents", ErrBadArg, maxDelete)
	}
	var b bytes.Buffer
	for _, d := range docs {
		if !exactIndexRe.MatchString(d.Index) || strings.Contains(d.Index, "..") || d.ID == "" {
			return res, fmt.Errorf("%w: each document needs one exact index and an id", ErrBadArg)
		}
		line, _ := json.Marshal(map[string]any{"delete": map[string]string{"_index": d.Index, "_id": d.ID}})
		b.Write(line)
		b.WriteByte('\n')
	}
	st, data, err := m.do(ctx, name, "POST", "/_bulk?refresh=wait_for", b.Bytes())
	if err != nil {
		return res, err
	}
	if st >= 400 {
		return res, esError(st, data)
	}
	var r struct {
		Items []map[string]struct {
			Status int             `json:"status"`
			Result string          `json:"result"`
			Error  json.RawMessage `json:"error"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return res, fmt.Errorf("unexpected answer from the cluster: %w", err)
	}
	for i, it := range r.Items {
		d := it["delete"]
		switch {
		case d.Result == "deleted":
			res.Deleted++
		case d.Result == "not_found" || d.Status == http.StatusNotFound:
			res.NotFound++
		default:
			msg := string(d.Error)
			var e struct{ Type, Reason string }
			if json.Unmarshal(d.Error, &e) == nil && e.Reason != "" {
				msg = e.Type + ": " + e.Reason
			}
			f := DeleteFailure{Error: msg}
			if i < len(docs) {
				f.Index, f.ID = docs[i].Index, docs[i].ID
			}
			res.Failed = append(res.Failed, f)
		}
	}
	return res, nil
}
