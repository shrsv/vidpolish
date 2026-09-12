package server

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:webdist
var webFS embed.FS

// webHandler serves the built frontend (Vite's output committed at
// webdist/) at "/". This is a single-page app with in-memory view state,
// not client-side URL routing, so no SPA-fallback is needed.
func webHandler() http.Handler {
	sub, err := fs.Sub(webFS, "webdist")
	if err != nil {
		// Only possible if webdist/ is missing entirely at build time,
		// which would already be a build failure; panic is appropriate
		// during server construction.
		panic("internal/server: webdist embed missing: " + err.Error())
	}
	return http.FileServerFS(sub)
}
