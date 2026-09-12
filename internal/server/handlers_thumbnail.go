package server

import (
	"net/http"
	"os"
	"path/filepath"

	"vidpolish/internal/config"
	"vidpolish/internal/thumbnail"
)

// handleThumbnailPreview renders the configured thumbnail style for an
// arbitrary, not-yet-saved title, so an upload cell's title field can show
// a live, accurate preview as it's typed instead of only after a real run.
// It reuses the exact same rendering path a real run would use, just
// writing to a throwaway temp file instead of the cell's own thumbnail
// path.
func (s *Server) handleThumbnailPreview(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title string `json:"title"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	cfg, err := config.Load()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	tmpDir, err := os.MkdirTemp("", "vidpolish-thumb-preview-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer os.RemoveAll(tmpDir)

	out, err := thumbnail.Generate(thumbnail.Options{
		Title:           req.Title,
		LogoPath:        cfg.Thumbnail.LogoPath,
		BackgroundColor: cfg.Thumbnail.BackgroundColor,
		AccentColor:     cfg.Thumbnail.AccentColor,
		TextColor:       cfg.Thumbnail.TextColor,
		OutPath:         filepath.Join(tmpDir, "preview.png"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	serveFile(w, r, out)
}
