package server

import (
	"fmt"
	"net/http"

	"vidpolish/internal/binmgr"
)

type toolStatus struct {
	Name  string `json:"name"`
	Path  string `json:"path,omitempty"`
	Error string `json:"error,omitempty"`
}

var allTools = []binmgr.Tool{binmgr.FFmpeg, binmgr.FFprobe, binmgr.DeepFilter, binmgr.AutoEditor, binmgr.Resvg}

func resolveToolStatuses() []toolStatus {
	out := make([]toolStatus, 0, len(allTools)+1)
	for _, t := range allTools {
		path, err := binmgr.Resolve(t)
		st := toolStatus{Name: string(t)}
		if err != nil {
			st.Error = err.Error()
		} else {
			st.Path = path
		}
		out = append(out, st)
	}
	regular, bold, err := binmgr.ResolveFont()
	st := toolStatus{Name: "font"}
	if err != nil {
		st.Error = err.Error()
	} else {
		st.Path = regular + ", " + bold
	}
	out = append(out, st)
	return out
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, resolveToolStatuses())
}

func (s *Server) handleResolveTool(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "font" {
		regular, bold, err := binmgr.ResolveFont()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, toolStatus{Name: "font", Path: regular + ", " + bold})
		return
	}
	for _, t := range allTools {
		if string(t) == name {
			path, err := binmgr.Resolve(t)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			writeJSON(w, http.StatusOK, toolStatus{Name: name, Path: path})
			return
		}
	}
	writeError(w, http.StatusNotFound, fmt.Errorf("unknown tool %q", name))
}
