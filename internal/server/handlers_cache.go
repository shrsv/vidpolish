package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"vidpolish/internal/cache"
	"vidpolish/internal/store"
)

type cacheEntryResponse struct {
	Fingerprint string `json:"fingerprint"`
	SizeBytes   int64  `json:"sizeBytes"`
	CachedAt    int64  `json:"cachedAt,omitempty"`
	ProjectID   string `json:"projectId,omitempty"`
	ProjectName string `json:"projectName,omitempty"`
}

// fingerprintToProject best-effort maps each cache fingerprint to the
// project whose source video produced it, by recomputing the same
// fingerprint pipeline.Process uses for every project's source cell. A
// cache entry with no match just means its source video isn't tracked by
// any current project (e.g. it was deleted, predates this project, or
// came from a bare CLI run).
func (s *Server) fingerprintToProject() map[string]*store.Project {
	out := make(map[string]*store.Project)
	projects, err := s.db.ListProjects()
	if err != nil {
		return out
	}
	for _, p := range projects {
		cells, err := s.db.ListCellsByProject(p.ID)
		if err != nil {
			continue
		}
		for _, c := range cells {
			if c.Kind != store.KindSource || c.OutputPath == nil || *c.OutputPath == "" {
				continue
			}
			if fp, err := cache.Fingerprint(*c.OutputPath); err == nil {
				out[fp] = p
			}
		}
	}
	return out
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
	byFingerprint := s.fingerprintToProject()

	out := make([]cacheEntryResponse, 0, len(entries))
	for _, e := range entries {
		r := cacheEntryResponse{Fingerprint: e.Fingerprint, SizeBytes: e.SizeBytes}
		if !e.CachedAt.IsZero() {
			r.CachedAt = e.CachedAt.Unix()
		}
		if p, ok := byFingerprint[e.Fingerprint]; ok {
			r.ProjectID = p.ID
			r.ProjectName = p.Name
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
