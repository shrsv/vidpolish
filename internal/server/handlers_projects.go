package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"vidpolish/internal/store"
)

type projectResponse struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	CreatedAt int64           `json:"createdAt"`
	UpdatedAt int64           `json:"updatedAt"`
	Cells     []*cellResponse `json:"cells"`
}

type cellResponse struct {
	ID             string `json:"id"`
	Seq            int    `json:"seq"`
	ParentCellID   string `json:"parentCellId,omitempty"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	Params         any    `json:"params"`
	Status         string `json:"status"`
	StatusMessage  string `json:"statusMessage,omitempty"`
	MediaURL       string `json:"mediaUrl,omitempty"`
	ThumbnailURL   string `json:"thumbnailUrl,omitempty"`
	SourceFilename string `json:"sourceFilename,omitempty"`
	YouTubeURL     string `json:"youtubeUrl,omitempty"`
	Position       int    `json:"position"`
}

func cellToResponse(c *store.Cell) *cellResponse {
	r := &cellResponse{
		ID:       c.ID,
		Seq:      c.Seq,
		Name:     c.DisplayName(),
		Kind:     c.Kind,
		Status:   c.Status,
		Position: c.Position,
	}
	if c.ParentCellID != nil {
		r.ParentCellID = *c.ParentCellID
	}
	if c.StatusMessage != nil {
		r.StatusMessage = *c.StatusMessage
	}
	if c.OutputPath != nil && *c.OutputPath != "" {
		r.MediaURL = "/api/media/" + c.ID
	}
	if c.Kind == store.KindUpload {
		if path, err := cellThumbnailPath(c); err == nil {
			if _, err := os.Stat(path); err == nil {
				r.ThumbnailURL = "/api/cells/" + c.ID + "/thumbnail"
			}
		}
	}
	if c.SourceFilename != nil {
		r.SourceFilename = *c.SourceFilename
	}
	if c.YouTubeURL != nil {
		r.YouTubeURL = *c.YouTubeURL
	}
	var params any
	if err := json.Unmarshal([]byte(c.ParamsJSON), &params); err == nil {
		r.Params = params
	}
	return r
}

func projectToResponse(p *store.Project, cells []*store.Cell) *projectResponse {
	r := &projectResponse{
		ID:        p.ID,
		Name:      p.Name,
		CreatedAt: p.CreatedAt.Unix(),
		UpdatedAt: p.UpdatedAt.Unix(),
	}
	for _, c := range cells {
		r.Cells = append(r.Cells, cellToResponse(c))
	}
	return r
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.db.ListProjects()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]*projectResponse, 0, len(projects))
	for _, p := range projects {
		cells, err := s.db.ListCellsByProject(p.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		out = append(out, projectToResponse(p, cells))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	p, source, err := s.db.CreateProject(req.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, projectToResponse(p, []*store.Cell{source}))
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.db.GetProject(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	cells, err := s.db.ListCellsByProject(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, projectToResponse(p, cells))
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.db.DeleteProject(id); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolveCellRef resolves ref to a cell within project, trying it first
// as a raw cell ID and falling back to seq/name matching.
func (s *Server) resolveCellRef(projectID, ref string) (*store.Cell, error) {
	if c, err := s.db.GetCell(ref); err == nil && c.ProjectID == projectID {
		return c, nil
	}
	return s.db.GetCellByRef(projectID, ref)
}

func (s *Server) handleCreateCell(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	var req struct {
		Kind         string          `json:"kind"`
		Name         string          `json:"name,omitempty"`
		ParentCellID string          `json:"parentCellId"`
		Params       json.RawMessage `json:"params"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Kind != store.KindEdit && req.Kind != store.KindUpload {
		writeError(w, http.StatusBadRequest, fmt.Errorf("kind must be 'edit' or 'upload'"))
		return
	}
	parent, err := s.resolveCellRef(projectID, req.ParentCellID)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("parentCellId: %w", err))
		return
	}
	if req.Kind == store.KindEdit && parent.Kind != store.KindSource {
		writeError(w, http.StatusBadRequest, fmt.Errorf("an edit cell's parent must be the project's source cell"))
		return
	}
	if req.Kind == store.KindUpload && parent.Kind != store.KindEdit {
		writeError(w, http.StatusBadRequest, fmt.Errorf("an upload cell's parent must be an edit cell"))
		return
	}

	params := string(req.Params)
	if params == "" {
		params = "{}"
	}
	seq, err := s.db.NextSeq(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	cells, err := s.db.ListCellsByProject(projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var namePtr *string
	if req.Name != "" {
		namePtr = &req.Name
	}
	c, err := s.db.CreateCell(projectID, seq, parent.ID, namePtr, req.Kind, params, len(cells))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, cellToResponse(c))
}

func (s *Server) handleUpdateCell(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.db.GetCell(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if c.Status == store.StatusRunning {
		writeError(w, http.StatusConflict, fmt.Errorf("cannot edit a running cell"))
		return
	}
	var req struct {
		Name   *string         `json:"name"`
		Params json.RawMessage `json:"params"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name != nil {
		if err := s.db.RenameCell(id, *req.Name); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	if len(req.Params) > 0 {
		if err := s.db.UpdateCellParams(id, string(req.Params)); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	c, err = s.db.GetCell(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, cellToResponse(c))
}

func (s *Server) handleGetCell(w http.ResponseWriter, r *http.Request) {
	c, err := s.db.GetCell(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, cellToResponse(c))
}

func (s *Server) handleDeleteCell(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	c, err := s.db.GetCell(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if c.Kind == store.KindSource {
		writeError(w, http.StatusBadRequest, fmt.Errorf("cannot delete a project's source cell"))
		return
	}
	if err := s.db.DeleteCell(id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleReorderCells persists a new display order for all cells of one
// kind within a project (edit cells among themselves, upload cells among
// themselves). seq numbers are untouched.
func (s *Server) handleReorderCells(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("id")
	var req struct {
		Kind           string   `json:"kind"`
		OrderedCellIDs []string `json:"orderedCellIds"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Kind != store.KindEdit && req.Kind != store.KindUpload {
		writeError(w, http.StatusBadRequest, fmt.Errorf("kind must be 'edit' or 'upload'"))
		return
	}
	if err := s.db.ReorderCells(projectID, req.Kind, req.OrderedCellIDs); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
