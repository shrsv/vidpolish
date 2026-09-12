package server

import (
	"fmt"
	"net/http"
	"os"
)

// handleMedia serves a cell's output video/thumbnail for in-browser
// playback, with Range support (seeking) via http.ServeContent.
func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.db.GetCell(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if c.OutputPath == nil || *c.OutputPath == "" {
		writeError(w, http.StatusNotFound, fmt.Errorf("cell has no output yet"))
		return
	}
	f, err := os.Open(*c.OutputPath)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	http.ServeContent(w, r, *c.OutputPath, info.ModTime(), f)
}
