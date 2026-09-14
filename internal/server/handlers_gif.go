package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"vidpolish/internal/binmgr"
	"vidpolish/internal/pipeline"
)

// handleExportGif converts a cell's output video to an animated GIF with
// caller-chosen fps/width and streams it back directly — there's no
// reason to persist it server-side, it's a one-off export the browser
// saves wherever the user points it.
func (s *Server) handleExportGif(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cell, err := s.db.GetCell(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if cell.OutputPath == nil || *cell.OutputPath == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("cell has no output yet"))
		return
	}

	var req struct {
		FPS   int `json:"fps"`
		Width int `json:"width"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	fps := clamp(req.FPS, 1, 30, 12)
	width := clamp(req.Width, 0, 1920, 0) // 0 keeps the source's original width

	ffmpegPath, err := binmgr.Resolve(binmgr.FFmpeg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	tmpDir, err := os.MkdirTemp("", "vidpolish-gif-*")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer os.RemoveAll(tmpDir)

	out := filepath.Join(tmpDir, "export.gif")
	if err := pipeline.ExportGIF(ffmpegPath, *cell.OutputPath, out, fps, width); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	serveFile(w, r, out)
}

// clamp constrains n to [min, max], substituting def for a non-positive
// (unset) n first — used to sanitize caller-supplied GIF export options
// without rejecting the request outright for an out-of-range value.
func clamp(n, min, max, def int) int {
	if n <= 0 {
		n = def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
