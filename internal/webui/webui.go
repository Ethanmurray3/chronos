// Package webui embeds the static frontend so the whole app ships as one
// binary with no external files.
package webui

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
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

// Handler serves the embedded assets with cache validators. Embedded files
// have no modtime, so a bare FileServerFS response carries no ETag or
// Last-Modified and browsers may heuristically cache individual ES modules —
// after a deploy a page can then run a stale mix of old and new modules. We
// set Cache-Control: no-cache (revalidate every load) plus a single ETag
// derived from the whole asset tree, so unchanged builds answer with cheap
// 304s and any rebuild invalidates every module at once.
func Handler() http.Handler {
	fsys := FS()
	etag := computeETag(fsys)
	fileServer := http.FileServerFS(fsys)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("ETag", etag)
		fileServer.ServeHTTP(w, r) // ServeContent handles If-None-Match → 304
	})
}

// computeETag hashes every embedded file (WalkDir is lexical, so the result
// is deterministic for a given build).
func computeETag(fsys fs.FS) string {
	h := sha256.New()
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		h.Write([]byte(path))
		h.Write(b)
		return nil
	})
	if err != nil {
		panic(err) // embed.FS reads cannot fail at runtime
	}
	return `"` + hex.EncodeToString(h.Sum(nil))[:16] + `"`
}
