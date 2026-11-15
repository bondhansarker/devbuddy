// Package redisview reads and deletes keys on the Redis servers named in the config.
// It never runs KEYS: keys are listed with SCAN, a page at a time.
package redisview

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	goredis "github.com/redis/go-redis/v9"

	"github.com/bondhansarker/devbuddy/internal/config"
)

var (
	ErrUnknown  = errors.New("unknown redis server")
	ErrNotFound = errors.New("key not found")
	ErrDB       = errors.New("db index out of range")
	ErrBadArg   = errors.New("bad argument")
)

const (
	maxString = 1 << 20 // bytes of a string value sent to the UI
	maxItems  = 500     // elements of a hash, list, set, zset or stream sent to the UI
	maxDel    = 1000    // keys per delete call
)

type Instance struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
	TLS  bool   `json:"tls"`
}

type Manager struct {
	cfg     map[string]config.Redis
	order   []Instance
	mu      sync.Mutex
	clients map[string]*goredis.Client // "name/db"
}

func New(list []config.Redis) *Manager {
	m := &Manager{cfg: map[string]config.Redis{}, clients: map[string]*goredis.Client{}}
	for _, r := range list {
		m.cfg[r.Name] = r
		m.order = append(m.order, Instance{r.Name, r.Addr, r.TLS})
	}
	return m
}

func (m *Manager) Instances() []Instance {
	if m == nil || m.order == nil {
		return []Instance{}
	}
	return m.order
}

// client returns the (cached) connection pool for one server and db index.
func (m *Manager) client(name string, db int) (*goredis.Client, error) {
	r, ok := m.cfg[name]
	if !ok {
		return nil, ErrUnknown
	}
	if db < 0 || db > 1023 {
		return nil, ErrDB
	}
	key := fmt.Sprintf("%s/%d", name, db)
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.clients[key]; c != nil {
		return c, nil
	}
	o := &goredis.Options{Addr: r.Addr, Username: r.Username, Password: r.Password, DB: db, PoolSize: 4}
	if r.TLS {
		host := r.Addr[:strings.LastIndex(r.Addr, ":")]
		o.TLSConfig = &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
	}
	c := goredis.NewClient(o)
	m.clients[key] = c
	return c, nil
}

type DB struct {
	DB      int   `json:"db"`
	Keys    int64 `json:"keys"`
	Expires int64 `json:"expires"`
}

// DBs lists the db indexes that hold keys, and how many indexes the server has.
func (m *Manager) DBs(ctx context.Context, name string) ([]DB, int, error) {
	c, err := m.client(name, 0)
	if err != nil {
		return nil, 0, err
	}
	if err := c.Ping(ctx).Err(); err != nil { // a wrong address or password shows up here
		return nil, 0, err
	}
	out := []DB{}
	info, _ := c.Info(ctx, "keyspace").Result() // INFO may be restricted: then no counts, the indexes still work
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "db") || !strings.Contains(line, ":") {
			continue
		}
		idx, rest, _ := strings.Cut(line[2:], ":")
		n, err := strconv.Atoi(idx)
		if err != nil {
			continue
		}
		d := DB{DB: n}
		for _, kv := range strings.Split(rest, ",") {
			k, v, _ := strings.Cut(kv, "=")
			x, _ := strconv.ParseInt(v, 10, 64)
			switch k {
			case "keys":
				d.Keys = x
			case "expires":
				d.Expires = x
			}
		}
		out = append(out, d)
	}
	databases := 16
	if cfg, err := c.ConfigGet(ctx, "databases").Result(); err == nil { // CONFIG may be disabled (managed Redis)
		if n, err := strconv.Atoi(cfg["databases"]); err == nil && n > 0 {
			databases = n
		}
	}
	return out, databases, nil
}

type KeyInfo struct {
	Key  string `json:"key"`
	Type string `json:"type"`
	TTL  int64  `json:"ttl"` // seconds; -1 = no expiry
}

// pattern turns plain text into a "contains" match; text with glob characters is used as given.
func pattern(match string) string {
	switch {
	case match == "":
		return "*"
	case strings.ContainsAny(match, "*?[\\"):
		return match
	}
	return "*" + match + "*"
}

var keyTypes = map[string]bool{"string": true, "hash": true, "list": true, "set": true, "zset": true, "stream": true}

// Scan returns about limit keys matching (of one type when typ is set), and the cursor to continue
// from (0 = done). The type filter is done by the server (SCAN ... TYPE, Redis 6+), so a rare type
// does not mean paging through everything on the client.
func (m *Manager) Scan(ctx context.Context, name string, db int, match, typ string, cursor uint64, limit int) (uint64, []KeyInfo, error) {
	if typ != "" && !keyTypes[typ] {
		return 0, nil, fmt.Errorf("%w: unknown type %q", ErrBadArg, typ)
	}
	c, err := m.client(name, db)
	if err != nil {
		return 0, nil, err
	}
	if limit < 1 || limit > 1000 {
		limit = 200
	}
	var keys []string
	for i := 0; i < 100 && len(keys) < limit; i++ { // a sparse match needs several SCAN rounds
		var ks []string
		var next uint64
		var err error
		if typ == "" {
			ks, next, err = c.Scan(ctx, cursor, pattern(match), 500).Result()
		} else {
			ks, next, err = c.ScanType(ctx, cursor, pattern(match), 500, typ).Result()
		}
		if err != nil {
			if typ != "" && strings.Contains(strings.ToLower(err.Error()), "syntax") {
				return 0, nil, errors.New("this Redis does not support filtering SCAN by type (needs Redis 6.0 or newer)")
			}
			return 0, nil, err
		}
		keys = append(keys, ks...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	out := make([]KeyInfo, len(keys))
	if len(keys) > 0 {
		pipe := c.Pipeline()
		types := make([]*goredis.StatusCmd, len(keys))
		ttls := make([]*goredis.DurationCmd, len(keys))
		for i, k := range keys {
			types[i], ttls[i] = pipe.Type(ctx, k), pipe.TTL(ctx, k)
		}
		if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, goredis.Nil) {
			return 0, nil, err
		}
		for i, k := range keys {
			ttl := int64(-1)
			if d := ttls[i].Val(); d > 0 {
				ttl = int64(d.Seconds())
			}
			out[i] = KeyInfo{Key: clean(k), Type: types[i].Val(), TTL: ttl}
		}
		if typ != "" {
			kept := out[:0]
			for _, k := range out {
				if k.Type == typ {
					kept = append(kept, k)
				}
			}
			out = kept
		}
	}
	return cursor, out, nil
}

type Item struct {
	K string `json:"k"` // hash field, list index, zset score or stream id; empty for a set
	V string `json:"v"`
}

type Value struct {
	Key       string `json:"key"`
	Type      string `json:"type"`
	TTL       int64  `json:"ttl"`
	Size      int64  `json:"size"` // bytes of a string, else the number of elements
	Truncated bool   `json:"truncated"`
	Binary    bool   `json:"binary"` // the value is not valid UTF-8; shown with replacement characters
	String    string `json:"string"`
	Items     []Item `json:"items"`
}

func clean(s string) string { return strings.ToValidUTF8(s, "�") }

// Get reads a key of any type, capped so a huge key cannot flood the browser.
func (m *Manager) Get(ctx context.Context, name string, db int, key string) (Value, error) {
	c, err := m.client(name, db)
	if err != nil {
		return Value{}, err
	}
	typ, err := c.Type(ctx, key).Result()
	if err != nil {
		return Value{}, err
	}
	if typ == "none" {
		return Value{}, ErrNotFound
	}
	v := Value{Key: clean(key), Type: typ, TTL: -1, Items: []Item{}}
	if d, err := c.TTL(ctx, key).Result(); err == nil && d > 0 {
		v.TTL = int64(d.Seconds())
	}
	bad := func(s string) string {
		if !utf8.ValidString(s) {
			v.Binary = true
		}
		return clean(s)
	}
	switch typ {
	case "string":
		n, _ := c.StrLen(ctx, key).Result()
		s, err := c.GetRange(ctx, key, 0, maxString-1).Result()
		if err != nil {
			return v, err
		}
		v.Size, v.Truncated, v.String = n, n > maxString, bad(s)
	case "hash":
		v.Size, _ = c.HLen(ctx, key).Result()
		fields, _, err := c.HScan(ctx, key, 0, "*", maxItems).Result()
		if err != nil {
			return v, err
		}
		for i := 0; i+1 < len(fields); i += 2 {
			v.Items = append(v.Items, Item{bad(fields[i]), bad(fields[i+1])})
		}
		v.Truncated = v.Size > int64(len(v.Items))
	case "list":
		v.Size, _ = c.LLen(ctx, key).Result()
		vals, err := c.LRange(ctx, key, 0, maxItems-1).Result()
		if err != nil {
			return v, err
		}
		for i, s := range vals {
			v.Items = append(v.Items, Item{strconv.Itoa(i), bad(s)})
		}
		v.Truncated = v.Size > int64(len(v.Items))
	case "set":
		v.Size, _ = c.SCard(ctx, key).Result()
		vals, _, err := c.SScan(ctx, key, 0, "*", maxItems).Result()
		if err != nil {
			return v, err
		}
		for _, s := range vals {
			v.Items = append(v.Items, Item{"", bad(s)})
		}
		v.Truncated = v.Size > int64(len(v.Items))
	case "zset":
		v.Size, _ = c.ZCard(ctx, key).Result()
		zs, err := c.ZRangeWithScores(ctx, key, 0, maxItems-1).Result()
		if err != nil {
			return v, err
		}
		for _, z := range zs {
			v.Items = append(v.Items, Item{strconv.FormatFloat(z.Score, 'f', -1, 64), bad(fmt.Sprint(z.Member))})
		}
		v.Truncated = v.Size > int64(len(v.Items))
	case "stream":
		v.Size, _ = c.XLen(ctx, key).Result()
		es, err := c.XRangeN(ctx, key, "-", "+", maxItems).Result()
		if err != nil {
			return v, err
		}
		for _, e := range es {
			var b strings.Builder
			for k, val := range e.Values {
				fmt.Fprintf(&b, "%s=%v ", k, val)
			}
			v.Items = append(v.Items, Item{e.ID, bad(strings.TrimSpace(b.String()))})
		}
		v.Truncated = v.Size > int64(len(v.Items))
	default:
		return v, fmt.Errorf("type %q cannot be shown", typ)
	}
	return v, nil
}

// Del removes keys with UNLINK (the memory is freed in the background) and returns how many existed.
func (m *Manager) Del(ctx context.Context, name string, db int, keys []string) (int64, error) {
	c, err := m.client(name, db)
	if err != nil {
		return 0, err
	}
	if len(keys) == 0 || len(keys) > maxDel {
		return 0, fmt.Errorf("give between 1 and %d keys", maxDel)
	}
	return c.Unlink(ctx, keys...).Result()
}

// SetString replaces the value of an existing string key and keeps its TTL. Other types are not editable here.
func (m *Manager) SetString(ctx context.Context, name string, db int, key, value string) error {
	c, err := m.client(name, db)
	if err != nil {
		return err
	}
	if len(value) > maxString {
		return fmt.Errorf("%w: value is over %d bytes", ErrBadArg, maxString)
	}
	typ, err := c.Type(ctx, key).Result()
	if err != nil {
		return err
	}
	switch typ {
	case "none":
		return ErrNotFound
	case "string":
	default:
		return fmt.Errorf("%w: only string keys can be edited, this one is a %s", ErrBadArg, typ)
	}
	return c.Set(ctx, key, value, goredis.KeepTTL).Err()
}

const maxExport = 200 // keys per export call

// ExportItem is one key with its whole (capped) value as plain JSON.
type ExportItem struct {
	DB        int    `json:"db"`
	Key       string `json:"key"`
	Type      string `json:"type,omitempty"`
	TTL       int64  `json:"ttl"`
	Truncated bool   `json:"truncated,omitempty"`
	Value     any    `json:"value,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Export reads several keys (of any type) for download.
func (m *Manager) Export(ctx context.Context, name string, db int, keys []string) ([]ExportItem, error) {
	if len(keys) == 0 || len(keys) > maxExport {
		return nil, fmt.Errorf("%w: give between 1 and %d keys", ErrBadArg, maxExport)
	}
	out := make([]ExportItem, 0, len(keys))
	for _, k := range keys {
		v, err := m.Get(ctx, name, db, k)
		if err != nil {
			if errors.Is(err, ErrUnknown) || errors.Is(err, ErrDB) {
				return nil, err
			}
			out = append(out, ExportItem{DB: db, Key: k, TTL: -1, Error: err.Error()})
			continue
		}
		out = append(out, ExportItem{DB: db, Key: v.Key, Type: v.Type, TTL: v.TTL, Truncated: v.Truncated, Value: v.plain()})
	}
	return out, nil
}

// plain turns a value into what reads naturally as JSON: a hash is an object, a list or set an array.
func (v Value) plain() any {
	switch v.Type {
	case "string":
		return v.String
	case "hash":
		o := map[string]string{}
		for _, i := range v.Items {
			o[i.K] = i.V
		}
		return o
	case "zset":
		l := make([]map[string]any, 0, len(v.Items))
		for _, i := range v.Items {
			s, _ := strconv.ParseFloat(i.K, 64)
			l = append(l, map[string]any{"member": i.V, "score": s})
		}
		return l
	case "stream":
		l := make([]map[string]string, 0, len(v.Items))
		for _, i := range v.Items {
			l = append(l, map[string]string{"id": i.K, "fields": i.V})
		}
		return l
	}
	l := make([]string, 0, len(v.Items))
	for _, i := range v.Items {
		l = append(l, i.V)
	}
	return l
}
