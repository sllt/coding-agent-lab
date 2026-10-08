//go:build webembed

package webui

import (
	"embed"
	"io/fs"
)

// dist is filled by scripts/build-web.sh, which copies web/dist here.
//
//go:embed all:dist
var dist embed.FS

func embedded() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		return nil
	}
	return sub
}
