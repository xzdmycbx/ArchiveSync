// Package webui embeds the built Vue single-page admin panel so the Go binary
// can serve it without external files. Run `npm run build` in web/ first; the
// output lands in web/dist which is embedded here.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// FS returns the SPA filesystem rooted at the dist directory.
func FS() (fs.FS, error) {
	return fs.Sub(distFS, "dist")
}
