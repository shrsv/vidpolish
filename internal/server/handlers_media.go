package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"vidpolish/internal/store"
)

// mimeByExt is a small, deterministic extension -> Content-Type map for
// the file types vidpolish ever serves via serveFile. We set this
// ourselves instead of leaning on Go's mime.TypeByExtension (which
// ServeContent falls back to automatically): on Windows that function
// reads the type from the registry (HKEY_CLASSES_ROOT\<ext>) rather than
// a built-in table, and plenty of real Windows installs have no entry (or
// the wrong one) for .mp4/.webm, so ServeContent ends up sending no
// Content-Type at all - and WebView2/Chromium's <video> element just
// silently refuses to play a response with no Content-Type, which looks
// exactly like "the preview never loads" with no visible error.
var mimeByExt = map[string]string{
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mov":  "video/quicktime",
	".mkv":  "video/x-matroska",
	".gif":  "image/gif",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".wav":  "audio/wav",
}

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
	if ct, ok := mimeByExt[strings.ToLower(filepath.Ext(path))]; ok {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeContent(w, r, path, info.ModTime(), f)
}
