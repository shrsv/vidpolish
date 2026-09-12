package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"vidpolish/internal/cache"
)

type cacheEntryResponse struct {
	Fingerprint string `json:"fingerprint"`
	SizeBytes   int64  `json:"sizeBytes"`
	CachedAt    int64  `json:"cachedAt,omitempty"`
}

func (s *Server) handleListCache(w http.ResponseWriter, r *http.Request) {
	root, err := cache.Root()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	entries, err := cache.ListEntries(root)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]cacheEntryResponse, 0, len(entries))
	for _, e := range entries {
		r := cacheEntryResponse{Fingerprint: e.Fingerprint, SizeBytes: e.SizeBytes}
		if !e.CachedAt.IsZero() {
			r.CachedAt = e.CachedAt.Unix()
		}
		out = append(out, r)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleDeleteCacheEntry(w http.ResponseWriter, r *http.Request) {
	fingerprint := r.PathValue("fingerprint")
	root, err := cache.Root()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	dir, err := cache.Dir(fingerprint)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// cache.Dir creates the dir if missing; guard against deleting
	// outside the cache root and against a bogus/empty fingerprint.
	if filepath.Dir(dir) != root || fingerprint == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid fingerprint"))
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCleanCache(w http.ResponseWriter, r *http.Request) {
	root, err := cache.Root()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := cache.Sweep(root, 0); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
