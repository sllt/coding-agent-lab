// Package webui locates the built single-page app.
//
// Lookup order, first match wins:
//  1. $AGENTLAB_WEB_DIR (must contain index.html)
//  2. ./web/dist relative to the working directory (development)
//  3. <executable dir>/web/dist, then <executable dir>/dist (release tarball)
//  4. the copy embedded at build time with `-tags webembed`
//
// A binary built with the webembed tag is self-contained; one built without it
// needs one of the directories above.
package webui

import (
	"io/fs"
	"os"
	"path/filepath"
)

// FS returns the web root and a short label naming where it came from. It
// returns nil when no build is available.
func FS() (fs.FS, string) {
	if dir := os.Getenv("AGENTLAB_WEB_DIR"); dir != "" {
		if hasIndex(dir) {
			return os.DirFS(dir), "env:AGENTLAB_WEB_DIR"
		}
	}
	if hasIndex(filepath.Join("web", "dist")) {
		abs, _ := filepath.Abs(filepath.Join("web", "dist"))
		return os.DirFS(abs), "dir:web/dist"
	}
	if exe, err := os.Executable(); err == nil {
		base := filepath.Dir(exe)
		for _, rel := range []string{filepath.Join("web", "dist"), "dist"} {
			if dir := filepath.Join(base, rel); hasIndex(dir) {
				return os.DirFS(dir), "exe:" + rel
			}
		}
	}
	if root := embedded(); root != nil {
		if _, err := fs.Stat(root, "index.html"); err == nil {
			return root, "embedded"
		}
	}
	return nil, "none"
}

func hasIndex(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil && !info.IsDir()
}
