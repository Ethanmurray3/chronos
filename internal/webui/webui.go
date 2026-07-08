// Package webui embeds the static frontend so the whole app ships as one
// binary with no external files.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:assets
var assets embed.FS

// FS returns the embedded asset tree rooted at the assets directory, ready to
// hand to http.FileServerFS.
func FS() fs.FS {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err) // build-time embed guarantees this never fails
	}
	return sub
}
