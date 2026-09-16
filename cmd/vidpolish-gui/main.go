// Command vidpolish-gui wraps vidpolish's existing local web UI
// (internal/server, the same server `vidpolish ui` starts and opens in a
// browser) in a native window via Wails, so Windows users can install and
// run vidpolish without a terminal or a browser tab.
//
// The existing web/ frontend talks to internal/server over a plain
// REST+SSE API, unchanged - this just hands that server's http.Handler
// straight to Wails as a custom AssetServer, no frontend rewrite needed.
// The one addition is App (see app.go): a small bound Go struct used only
// for native file selection, because WebView2's browser-side File API is
// unreliable for a file picked/dropped inside this custom-scheme-hosted
// page (see app.go's doc comment for the specifics).
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"vidpolish/internal/server"
	"vidpolish/internal/store"
)

// version is the vidpolish release version; overridden via
// -ldflags "-X main.version=X.Y.Z" by the release build, same as
// cmd/vidpolish. The exe/installer icon comes from build/appicon.png via
// the Wails build tooling, not from this binary.
var version = "dev"

func main() {
	logger, closeLog := openGUILog()
	defer closeLog()

	db, err := store.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "vidpolish-gui: opening store:", err)
		os.Exit(1)
	}
	defer db.Close()

	srv := server.New(db)
	handler := loggingHandler(srv.Handler(), logger)

	app := &App{}

	err = wails.Run(&options.App{
		Title:     "vidpolish",
		Width:     1280,
		Height:    800,
		MinWidth:  960,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Handler: handler,
		},
		OnStartup: app.startup,
		Bind:      []any{app},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "vidpolish-gui: error:", err)
		os.Exit(1)
	}
}

// openGUILog opens (creating if needed) a small log at
// <user cache dir>/vidpolish/gui.log - on Windows,
// %LocalAppData%\vidpolish\gui.log - so a request that behaves oddly
// inside the WebView2 window (which has no visible terminal to print to,
// and production Wails builds disable DevTools by default) can still be
// diagnosed: send us that file's contents.
//
// It also repoints the process's os.Stderr (and os.Stdout) at that same
// file. A GUI app launched by double-click has no console to write to -
// os.Stderr writes from anywhere in the codebase (binmgr's download
// progress, the pipeline's stage logs, this file's own diagnostics)
// otherwise just vanish. Redirecting the actual file descriptor, not
// just this package's own logger, is what makes every one of those
// existing fmt.Fprintln(os.Stderr, ...) calls elsewhere in the codebase
// show up here too, for free.
func openGUILog() (*log.Logger, func()) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return log.New(os.Stderr, "", log.LstdFlags), func() {}
	}
	dir := filepath.Join(cacheDir, "vidpolish")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return log.New(os.Stderr, "", log.LstdFlags), func() {}
	}
	f, err := os.OpenFile(filepath.Join(dir, "gui.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return log.New(os.Stderr, "", log.LstdFlags), func() {}
	}

	origStderr := os.Stderr
	os.Stderr = f
	os.Stdout = f

	return log.New(io.MultiWriter(f, origStderr), "", log.LstdFlags), func() { f.Close() }
}

// statusCapturingWriter records the status code and response headers
// actually sent, so they can be logged after the handler runs.
type statusCapturingWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusCapturingWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Flush makes statusCapturingWriter satisfy http.Flusher when the
// underlying ResponseWriter does, so it doesn't break the SSE endpoints
// (cell run progress, YouTube login) that type-assert for it.
func (w *statusCapturingWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// loggingHandler logs every request's method/path/Range header alongside
// the response's status/Content-Type/Content-Length, specifically to
// diagnose media (video/thumbnail) serving through Wails' custom
// AssetServer on Windows - e.g. whether a Range request actually reaches
// the handler, and what Content-Type/status comes back for it.
func loggingHandler(next http.Handler, logger *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusCapturingWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Printf("%s %s range=%q -> status=%d content-type=%q content-length=%q (%s)",
			r.Method, r.URL.Path, r.Header.Get("Range"), sw.status,
			sw.Header().Get("Content-Type"), sw.Header().Get("Content-Length"), time.Since(start))
	})
}
