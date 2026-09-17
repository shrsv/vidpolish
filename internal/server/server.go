// Package server implements vidpolish's embedded local web UI: a small
// HTTP API (backed by internal/store) plus the static Preact frontend,
// served from the same process as everything else vidpolish does. It
// binds to 127.0.0.1 only — this is a local-trust tool with no login
// page, the same trust model as running the vidpolish CLI itself.
package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"vidpolish/internal/store"
)

// Server holds the shared state behind the API: the database, the SSE
// hub for live cell/YouTube-login progress, the job tracker preventing
// double-runs, and the tool-download tracker (polled, not SSE - see
// toolTracker).
type Server struct {
	db    *store.DB
	hub   *hub
	jobs  *jobTracker
	tools *toolTracker
	mux   *http.ServeMux
}

// New builds a Server backed by db and wires up all routes.
func New(db *store.DB) *Server {
	s := &Server{
		db:    db,
		hub:   newHub(),
		jobs:  newJobTracker(),
		tools: newToolTracker(),
		mux:   http.NewServeMux(),
	}
	s.routes()
	return s
}

// Handler returns the http.Handler serving both the API and the embedded
// frontend.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/projects", s.handleListProjects)
	s.mux.HandleFunc("POST /api/projects", s.handleCreateProject)
	s.mux.HandleFunc("GET /api/projects/{id}", s.handleGetProject)
	s.mux.HandleFunc("DELETE /api/projects/{id}", s.handleDeleteProject)
	s.mux.HandleFunc("POST /api/projects/{id}/cells", s.handleCreateCell)
	s.mux.HandleFunc("POST /api/projects/{id}/reorder", s.handleReorderCells)

	s.mux.HandleFunc("POST /api/cells/{id}/source", s.handleUploadSource)
	s.mux.HandleFunc("POST /api/cells/{id}/source-path", s.handleUploadSourceFromPath)
	s.mux.HandleFunc("PATCH /api/cells/{id}", s.handleUpdateCell)
	s.mux.HandleFunc("POST /api/cells/{id}/run", s.handleRunCell)
	s.mux.HandleFunc("GET /api/cells/{id}", s.handleGetCell)
	s.mux.HandleFunc("GET /api/cells/{id}/info", s.handleCellInfo)
	s.mux.HandleFunc("POST /api/cells/{id}/export-gif", s.handleExportGif)
	s.mux.HandleFunc("GET /api/cells/{id}/events", s.handleCellEvents)
	s.mux.HandleFunc("DELETE /api/cells/{id}", s.handleDeleteCell)
	s.mux.HandleFunc("POST /api/cells/{id}/apply-profile", s.handleApplyProfile)

	s.mux.HandleFunc("GET /api/profiles", s.handleListProfiles)
	s.mux.HandleFunc("POST /api/profiles", s.handleCreateProfile)
	s.mux.HandleFunc("PATCH /api/profiles/{id}", s.handlePatchProfile)
	s.mux.HandleFunc("DELETE /api/profiles/{id}", s.handleDeleteProfile)

	s.mux.HandleFunc("GET /api/media/{id}", s.handleMedia)
	s.mux.HandleFunc("GET /api/cells/{id}/thumbnail", s.handleCellThumbnail)
	s.mux.HandleFunc("POST /api/thumbnail/preview", s.handleThumbnailPreview)

	s.mux.HandleFunc("GET /api/config", s.handleGetConfig)
	s.mux.HandleFunc("PUT /api/config", s.handlePutConfig)
	s.mux.HandleFunc("GET /api/config/backups", s.handleListConfigBackups)
	s.mux.HandleFunc("POST /api/config/backups/{timestamp}/restore", s.handleRestoreConfigBackup)
	s.mux.HandleFunc("POST /api/youtube/login", s.handleYouTubeLogin)
	s.mux.HandleFunc("GET /api/youtube/login/events", s.handleYouTubeLoginEvents)

	s.mux.HandleFunc("GET /api/tools", s.handleListTools)
	s.mux.HandleFunc("POST /api/tools/resolve-all", s.handleResolveAllTools)
	s.mux.HandleFunc("POST /api/tools/{name}/resolve", s.handleResolveTool)
	s.mux.HandleFunc("POST /api/tools/{name}/redownload", s.handleRedownloadTool)

	s.mux.HandleFunc("GET /api/cache", s.handleListCache)
	s.mux.HandleFunc("DELETE /api/cache/{fingerprint}", s.handleDeleteCacheEntry)
	s.mux.HandleFunc("POST /api/cache/clean", s.handleCleanCache)

	s.mux.Handle("/", webHandler())
}

// projectDir returns (creating it) ~/.vidpolish/projects/<id>, where a
// project's source video and edit-cell outputs live.
func projectDir(projectID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	dir := filepath.Join(home, ".vidpolish", "projects", projectID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating project dir: %w", err)
	}
	return dir, nil
}
