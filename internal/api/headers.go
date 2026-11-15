package api

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// Everything the app loads is its own: scripts and styles come from this server, nothing is inlined into a
// script, and the page cannot be put in a frame. If a log line or a Redis value ever slipped past the escaping
// in the UI, the browser would still refuse to run it.
const csp = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; " +
	"connect-src 'self' ws://localhost:* ws://127.0.0.1:*; object-src 'none'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store") // lists and values are never kept by the browser
		}
		next.ServeHTTP(w, r)
	})
}

// staticHandler serves the embedded UI with an ETag per file (a hash of its content): the browser asks
// "changed?" on every load and gets a 304 with no body, instead of downloading the scripts again.
func staticHandler(web fs.FS) http.Handler {
	tags := map[string]string{}
	_ = fs.WalkDir(web, ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if b, err := fs.ReadFile(web, p); err == nil {
				sum := sha256.Sum256(b)
				tags[p] = `"` + hex.EncodeToString(sum[:8]) + `"`
			}
		}
		return nil
	})
	files := http.FileServerFS(web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if tag, ok := tags[p]; ok {
			w.Header().Set("ETag", tag)
			w.Header().Set("Cache-Control", "no-cache") // keep, but check first
			if r.Header.Get("If-None-Match") == tag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}

// maxBody caps a request body; the endpoints that expect more say so.
const maxBody = 1 << 20

// decodeBody reads a JSON body of at most max bytes.
func decodeBody(w http.ResponseWriter, r *http.Request, v any, max int64) error {
	return jsonDecode(http.MaxBytesReader(w, r.Body, max), v)
}
