// Package web bundles the offline browser interface into the Go executable.
package web

import (
	"embed"
	"io/fs"
)

// Assets contains the checked-in production bundle. Regenerate with npm run build.
//
//go:embed all:dist
var Assets embed.FS

// FS returns the production bundle with index.html at its root.
func FS() fs.FS {
	assets, err := fs.Sub(Assets, "dist")
	if err != nil {
		panic("embedded Tracelet UI is missing")
	}
	return assets
}
