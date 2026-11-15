package redisview

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	goredis "github.com/redis/go-redis/v9"
)

// Commands a read-only console may run; a nil list allows every subcommand. Anything else is refused.
var readOnlyCmds = map[string][]string{
	"get": nil, "mget": nil, "strlen": nil, "getrange": nil, "exists": nil, "type": nil, "ttl": nil, "pttl": nil,
	"expiretime": nil, "pexpiretime": nil, "hget": nil, "hgetall": nil, "hmget": nil, "hkeys": nil, "hvals": nil,
	"hlen": nil, "hexists": nil, "hstrlen": nil, "hscan": nil, "lrange": nil, "llen": nil, "lindex": nil, "lpos": nil,
	"smembers": nil, "scard": nil, "sismember": nil, "smismember": nil, "srandmember": nil, "sscan": nil,
	"sinter": nil, "sunion": nil, "sdiff": nil, "zrange": nil, "zrangebyscore": nil, "zrevrange": nil,
	"zrevrangebyscore": nil, "zcard": nil, "zscore": nil, "zrank": nil, "zrevrank": nil, "zcount": nil, "zscan": nil,
	"xrange": nil, "xrevrange": nil, "xlen": nil, "scan": nil, "dbsize": nil, "randomkey": nil, "ping": nil,
	"echo": nil, "time": nil, "info": nil, "lastsave": nil, "bitcount": nil, "getbit": nil, "pfcount": nil,
	"geopos": nil, "geodist": nil,
	"memory": {"usage", "stats"}, "object": {"encoding", "freq", "idletime", "refcount"},
	"slowlog": {"get", "len"}, "client": {"list", "info", "getname"}, "config": {"get"},
	"xinfo": {"stream", "groups", "consumers"},
}

// Never run from the console: they change the connection's state (the pool shares connections), stream
// forever, or stop the server.
var neverCmds = map[string]bool{
	"monitor": true, "subscribe": true, "psubscribe": true, "ssubscribe": true, "unsubscribe": true,
	"punsubscribe": true, "sunsubscribe": true, "sync": true, "psync": true, "shutdown": true, "debug": true,
	"replicaof": true, "slaveof": true, "multi": true, "exec": true, "discard": true, "watch": true,
	"unwatch": true, "select": true, "auth": true, "hello": true, "quit": true, "reset": true,
}

var neverClientSub = map[string]bool{"reply": true, "setname": true, "setinfo": true, "tracking": true, "caching": true, "no-evict": true, "no-touch": true}

var ErrCommand = errors.New("command not allowed")

// ParseCommand splits a console line into arguments: spaces separate, "double" and 'single' quotes group,
// a backslash escapes inside quotes.
func ParseCommand(line string) ([]string, error) {
	var args []string
	var cur strings.Builder
	in, quote := false, rune(0)
	esc := false
	for _, r := range line {
		switch {
		case esc:
			switch r {
			case 'n':
				cur.WriteRune('\n')
			case 't':
				cur.WriteRune('\t')
			case 'r':
				cur.WriteRune('\r')
			default:
				cur.WriteRune(r)
			}
			esc = false
		case quote != 0:
			switch {
			case r == '\\':
				esc = true
			case r == quote:
				quote = 0
			default:
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote, in = r, true
		case r == ' ' || r == '\t':
			if in {
				args = append(args, cur.String())
				cur.Reset()
				in = false
			}
		default:
			cur.WriteRune(r)
			in = true
		}
	}
	if quote != 0 || esc {
		return nil, errors.New("unterminated quote")
	}
	if in {
		args = append(args, cur.String())
	}
	return args, nil
}

// Allowed reports why a command may not run, or nil.
func Allowed(args []string, readOnly bool) error {
	if len(args) == 0 {
		return errors.New("empty command")
	}
	cmd := strings.ToLower(args[0])
	sub := ""
	if len(args) > 1 {
		sub = strings.ToLower(args[1])
	}
	if neverCmds[cmd] || (cmd == "client" && neverClientSub[sub]) {
		return fmt.Errorf("%w: %s cannot run in the console", ErrCommand, strings.ToUpper(cmd))
	}
	if !readOnly {
		return nil
	}
	subs, ok := readOnlyCmds[cmd]
	if !ok {
		return fmt.Errorf("%w: %s changes data (read_only is set)", ErrCommand, strings.ToUpper(cmd))
	}
	if subs != nil {
		for _, s := range subs {
			if s == sub {
				return nil
			}
		}
		return fmt.Errorf("%w: %s %s (read_only is set)", ErrCommand, strings.ToUpper(cmd), strings.ToUpper(sub))
	}
	return nil
}

const maxReply = 256 << 10

// Exec runs one command on the db and returns the reply formatted like redis-cli. A reply error
// from the server (WRONGTYPE, unknown command, ...) is part of the text; only a failure to reach
// the server is an error.
func (m *Manager) Exec(ctx context.Context, name string, db int, args []string, readOnly bool) (string, error) {
	if err := Allowed(args, readOnly); err != nil {
		return "", err
	}
	c, err := m.client(name, db)
	if err != nil {
		return "", err
	}
	in := make([]any, len(args))
	for i, a := range args {
		in[i] = a
	}
	v, err := c.Do(ctx, in...).Result()
	switch {
	case errors.Is(err, goredis.Nil):
		return "(nil)", nil
	case err != nil:
		var re goredis.Error
		if errors.As(err, &re) {
			msg := "(error) " + re.Error()
			if strings.HasPrefix(re.Error(), "WRONGTYPE") && len(args) > 1 {
				msg += typeHint(ctx, c, args[1])
			}
			return msg, nil
		}
		return "", err
	}
	out := format(v, true)
	if len(out) > maxReply {
		out = out[:maxReply] + "\n… output cut at 256 KB"
	}
	return out, nil
}

// typeHint says what type the key really is and how to read it, for a WRONGTYPE reply.
func typeHint(ctx context.Context, c *goredis.Client, key string) string {
	t, err := c.Type(ctx, key).Result()
	if err != nil || t == "none" {
		return ""
	}
	k := key
	if strings.ContainsAny(key, " \t\"'") {
		k = strconv.Quote(key)
	}
	try := map[string]string{
		"string": "GET " + k, "hash": "HGETALL " + k, "list": "LRANGE " + k + " 0 -1", "set": "SMEMBERS " + k,
		"zset": "ZRANGE " + k + " 0 -1 WITHSCORES", "stream": "XRANGE " + k + " - +",
	}[t]
	if try == "" {
		return "\n(hint) the key is a " + t
	}
	return "\n(hint) the key is a " + t + ", try: " + try
}

func format(v any, top bool) string {
	switch x := v.(type) {
	case nil:
		return "(nil)"
	case string:
		if top && (x == "OK" || x == "PONG") { // status replies print bare, like redis-cli
			return "OK"
		}
		return strconv.Quote(clean(x))
	case int64:
		return "(integer) " + strconv.FormatInt(x, 10)
	case float64:
		return "(double) " + strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return "(" + strconv.FormatBool(x) + ")"
	case []any:
		if len(x) == 0 {
			return "(empty array)"
		}
		lines := make([]string, len(x))
		for i, e := range x {
			p := strconv.Itoa(i+1) + ") "
			lines[i] = p + strings.ReplaceAll(format(e, false), "\n", "\n"+strings.Repeat(" ", len(p)))
		}
		return strings.Join(lines, "\n")
	case map[any]any:
		if len(x) == 0 {
			return "(empty map)"
		}
		lines := make([]string, 0, len(x))
		for k, e := range x {
			lines = append(lines, fmt.Sprint(k)+" => "+strings.ReplaceAll(format(e, false), "\n", "\n    "))
		}
		sort.Strings(lines)
		return strings.Join(lines, "\n")
	}
	return fmt.Sprint(v)
}
