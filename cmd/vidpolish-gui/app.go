package main

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App exposes a few native, Go-side operations to the frontend via Wails'
// JS<->Go binding layer (window.go.main.App.<Method> - injected
// automatically into every page this app serves, even through our custom
// AssetServer.Handler).
//
// It exists specifically to work around a WebView2 limitation: a picked
// or dropped file's browser-side File object can come back with size 0
// inside a WebView2-hosted page loaded from a custom scheme (confirmed:
// vidpolish's own upload handler logged header.Size=0 for a real,
// multi-megabyte video picked via <input type="file">). Wails' native
// file dialog and drag-and-drop bridge hand back real OS file paths
// instead, which the Go backend then reads directly off disk - no
// browser file-content API involved at all.
type App struct {
	ctx context.Context
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// PickVideoFile opens a native "Open File" dialog for a video file and
// returns the chosen absolute path, or "" if the user cancelled.
func (a *App) PickVideoFile() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select a video",
		Filters: []runtime.FileFilter{
			{DisplayName: "Video files", Pattern: "*.mp4;*.mov;*.mkv;*.webm;*.avi;*.m4v"},
			{DisplayName: "All files", Pattern: "*.*"},
		},
	})
}
