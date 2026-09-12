package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"vidpolish/internal/store"
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
	serveFile(w, r, *c.OutputPath)
}

// cellThumbnailPath returns the deterministic path a generated thumbnail
// for an upload cell is written to and read back from.
func cellThumbnailPath(cell *store.Cell) (string, error) {
	dir, err := projectDir(cell.ProjectID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cells", cell.ID, "thumbnail.png"), nil
}

// handleCellThumbnail serves an upload cell's generated thumbnail
// preview, if one has been generated yet.
func (s *Server) handleCellThumbnail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.db.GetCell(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	path, err := cellThumbnailPath(c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("no thumbnail generated yet"))
		return
	}
	serveFile(w, r, path)
}

func serveFile(w http.ResponseWriter, r *http.Request, path string) {
	f, err := os.Open(path)
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
	http.ServeContent(w, r, path, info.ModTime(), f)
}
