//go:build dev

// Package ui serves static assets from disk in development builds (-tags dev).
// CSS changes made by the Tailwind watcher are picked up immediately without
// rebuilding the binary.
package ui

import (
	"io/fs"
	"os"
)

// StaticFS is an fs.FS rooted at ui/static on disk.
// The working directory must be the repository root when running the server.
var StaticFS fs.FS = os.DirFS("ui/static")
