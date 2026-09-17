package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"vidpolish/internal/binmgr"
	"vidpolish/internal/pipeline"
	"vidpolish/internal/store"
)

type profileResponse struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Params    any    `json:"params"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

func profileToResponse(p *store.Profile) *profileResponse {
	r := &profileResponse{ID: p.ID, Name: p.Name, CreatedAt: p.CreatedAt.Unix(), UpdatedAt: p.UpdatedAt.Unix()}
	var params any
	if err := json.Unmarshal([]byte(p.ParamsJSON), &params); err == nil {
		r.Params = params
	}
	return r
}

// sourceResolutionFor probes and returns cell's ultimate source resolution
// (its parent's output, for an edit cell), or 0,0 if it isn't probeable yet
// — callers treat that as "unknown" and fall back to "keep original".
func sourceResolutionFor(db *store.DB, cell *store.Cell) (int, int) {
	if cell.Kind != store.KindEdit || cell.ParentCellID == nil {
		return 0, 0
	}
	parent, err := db.GetCell(*cell.ParentCellID)
	if err != nil || parent.OutputPath == nil || *parent.OutputPath == "" {
		return 0, 0
	}
	ffprobePath, err := binmgr.Resolve(binmgr.FFprobe)
	if err != nil {
		return 0, 0
	}
	info, err := pipeline.ProbeVideoInfo(ffprobePath, *parent.OutputPath)
	if err != nil {
		return 0, 0
	}
	return info.Width, info.Height
}

func (s *Server) handleListProfiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := s.db.ListProfiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	out := make([]*profileResponse, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, profileToResponse(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleCreateProfile saves the given edit cell's current params as a new
// named profile, converting its absolute resize (if any) into a percentage
// of its source resolution.
func (s *Server) handleCreateProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string `json:"name"`
		CellID string `json:"cellId"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}
	cell, err := s.db.GetCell(req.CellID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if cell.Kind != store.KindEdit {
		writeError(w, http.StatusBadRequest, fmt.Errorf("cellId must refer to an edit cell"))
		return
	}
	var ep EditParams
	if err := json.Unmarshal([]byte(cell.ParamsJSON), &ep); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("parsing cell params: %w", err))
		return
	}
	srcW, srcH := sourceResolutionFor(s.db, cell)
	pp := DeriveProfileParams(ep, srcW, srcH)
	paramsJSON, err := json.Marshal(pp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	p, err := s.db.CreateProfile(req.Name, string(paramsJSON))
	if err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusCreated, profileToResponse(p))
}

// handlePatchProfile renames and/or replaces a profile's params. Both
// fields are optional; whichever is omitted keeps its current value,
// mirroring handleUpdateCell's partial-update shape.
func (s *Server) handlePatchProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.db.GetProfile(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
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
	name := p.Name
	if req.Name != nil && *req.Name != "" {
		name = *req.Name
	}
	paramsJSON := p.ParamsJSON
	if len(req.Params) > 0 {
		paramsJSON = string(req.Params)
	}
	if err := s.db.UpdateProfile(id, name, paramsJSON); err != nil {
		writeError(w, http.StatusConflict, err)
		return
	}
	updated, err := s.db.GetProfile(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, profileToResponse(updated))
}

func (s *Server) handleDeleteProfile(w http.ResponseWriter, r *http.Request) {
	if err := s.db.DeleteProfile(r.PathValue("id")); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleApplyProfile resolves a saved profile's scale percentage against
// the target cell's source resolution and overwrites the cell's params.
func (s *Server) handleApplyProfile(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ProfileID string `json:"profileId"`
	}
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	profile, err := s.db.GetProfile(req.ProfileID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	cell, err := s.db.GetCell(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if cell.Kind != store.KindEdit {
		writeError(w, http.StatusBadRequest, fmt.Errorf("cell must be an edit cell"))
		return
	}
	if cell.Status == store.StatusRunning {
		writeError(w, http.StatusConflict, fmt.Errorf("cannot edit a running cell"))
		return
	}
	var pp ProfileParams
	if err := json.Unmarshal([]byte(profile.ParamsJSON), &pp); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("parsing profile params: %w", err))
		return
	}
	srcW, srcH := sourceResolutionFor(s.db, cell)
	ep := ResolveProfile(pp, srcW, srcH)
	epJSON, err := json.Marshal(ep)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := s.db.UpdateCellParams(cell.ID, string(epJSON)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	c, err := s.db.GetCell(cell.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, cellToResponse(c))
}
