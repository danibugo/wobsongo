//go:build !dev

// Package ui embeds the compiled static assets (CSS, JS) for production builds.
// In development (-tags dev) the files are served directly from disk instead.
package ui

import (
	"embed"
	"io/fs"
)

//go:embed static
var staticEmbed embed.FS

// StaticFS is an fs.FS rooted at the static content directory.
// Paths are relative to static/: e.g. "css/output.css", "js/dialog.min.js".
var StaticFS fs.FS

func init() {
	sub, err := fs.Sub(staticEmbed, "static")
	if err != nil {
		panic("ui: sub static FS: " + err.Error())
	}
	StaticFS = sub
}
