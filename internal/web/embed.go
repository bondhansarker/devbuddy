// Package web holds the browser UI: plain HTML, CSS and JavaScript (no framework, no build step),
// embedded into the binary.
package web

import (
	"embed"
	"io/fs"
)

//go:embed static
var files embed.FS

// FS returns the UI files with index.html at the root.
func FS() fs.FS {
	sub, err := fs.Sub(files, "static")
	if err != nil {
		panic(err) // static/ is embedded at build time: cannot be missing
	}
	return sub
}
