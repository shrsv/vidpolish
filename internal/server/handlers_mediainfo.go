package server

import (
	"fmt"
	"net/http"

	"vidpolish/internal/binmgr"
	"vidpolish/internal/pipeline"
	"vidpolish/internal/store"
)

// handleCellSourceInfo returns the original width/height/bitrate of the
// video an edit cell would run against (its parent source cell's video),
// so the UI can show "original: 1920x1080, 8000kbps" placeholders instead
// of the resize/bitrate fields looking arbitrary.
func (s *Server) handleCellSourceInfo(w http.ResponseWriter, r *http.Request) {
	cell, err := s.db.GetCell(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if cell.Kind != store.KindEdit {
		writeError(w, http.StatusBadRequest, fmt.Errorf("only edit cells have source video info"))
		return
	}
	source, err := s.db.GetCell(*cell.ParentCellID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if source.OutputPath == nil || *source.OutputPath == "" {
		writeError(w, http.StatusNotFound, fmt.Errorf("source cell has no video yet"))
		return
	}

	ffprobePath, err := binmgr.Resolve(binmgr.FFprobe)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	info, err := pipeline.ProbeVideoInfo(ffprobePath, *source.OutputPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}
