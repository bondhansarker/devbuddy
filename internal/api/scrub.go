package api

import (
	"os"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/bondhansarker/devbuddy/internal/config"
)

// What leaves the server for the browser is scrubbed of the secrets DevBuddy holds: an error message, the
// output of a Redis console command or an Elasticsearch query could echo a password or API key back (a server
// that prints its own config, a proxy that repeats a header). Secrets shorter than minSecret are left alone
// because replacing a short string would mangle ordinary text. The home directory shows as ~.
const (
	minSecret = 6
	mask      = "••••••"
)

var scrubber atomic.Pointer[strings.Replacer]

func setScrubber(cfg config.Config) {
	var secrets []string
	add := func(s string) {
		if len(s) >= minSecret {
			secrets = append(secrets, s)
		}
	}
	for _, r := range cfg.Redis {
		add(r.Password)
	}
	for _, e := range cfg.Elasticsearch {
		add(e.Password)
		add(e.APIKey)
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) }) // longest first: the replacer takes the first match
	var pairs []string
	for _, s := range secrets {
		pairs = append(pairs, s, mask)
	}
	if h, err := os.UserHomeDir(); err == nil && len(h) > 1 {
		pairs = append(pairs, h, "~")
	}
	if len(pairs) == 0 {
		scrubber.Store(nil)
		return
	}
	scrubber.Store(strings.NewReplacer(pairs...))
}

func scrub(s string) string {
	if r := scrubber.Load(); r != nil {
		return r.Replace(s)
	}
	return s
}
