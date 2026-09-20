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

// cellPreviewPath returns the deterministic path a source/edit cell's
// generated small, browser-safe preview proxy (see
// pipeline.GeneratePreviewProxy) is written to and read back from.
func cellPreviewPath(cell *store.Cell) (string, error) {
	dir, err := projectDir(cell.ProjectID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cells", cell.ID, "preview.mp4"), nil
}

// handleCellPreview serves a cell's generated preview proxy, if one has
// been generated yet (see (*Server).generateCellPreview). The frontend's
// <video> element plays this, never the real output directly - the real
// output can be a raw, arbitrarily large and idiosyncratically-encoded
// recording that a browser's media pipeline can stall on indefinitely,
// where this proxy is a small, deliberately unexceptional re-encode.
func (s *Server) handleCellPreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.db.GetCell(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	path, err := cellPreviewPath(c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("no preview generated yet"))
		return
	}
	serveFile(w, r, path)
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

// cellPosterPath returns the deterministic path an edit cell's generated
// preview-frame poster (a single still frame grabbed from its finished
// output) is written to and read back from.
func cellPosterPath(cell *store.Cell) (string, error) {
	dir, err := projectDir(cell.ProjectID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cells", cell.ID, "poster.png"), nil
}

// handleCellPoster serves an edit cell's generated poster frame, if one
// has been generated yet (see generateCellPoster, called after a
// successful run). The frontend uses this as the <video poster> so a
// project page with several finished edit cells loads several small PNGs
// instead of each <video> immediately opening its own Range-request chain
// against the real (large) output file just to paint a first frame.
func (s *Server) handleCellPoster(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.db.GetCell(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	path, err := cellPosterPath(c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("no poster generated yet"))
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
