package server

import (
	"fmt"
	"net/http"

	"vidpolish/internal/binmgr"
)

// toolStatus is one row of GET /api/tools. Exactly one of Path/Error is
// set when Downloading is false; while Downloading is true, Written/Total
// (when the server reported a size) and Log (the most recent status line)
// describe the download in progress. Poll this endpoint to watch a
// download to completion - there's no separate events stream (see
// toolTracker for why).
//
// Managed reports whether vidpolish manages this tool's binary itself
// (downloaded into its own cache) as opposed to using one already on the
// system PATH - only a managed tool can be usefully redownloaded, since
// there's nothing for vidpolish to redownload for e.g. a system ffmpeg
// install.
type toolStatus struct {
	Name        string `json:"name"`
	Path        string `json:"path,omitempty"`
	Error       string `json:"error,omitempty"`
	Managed     bool   `json:"managed"`
	Downloading bool   `json:"downloading,omitempty"`
	Written     int64  `json:"written,omitempty"`
	Total       int64  `json:"total,omitempty"`
	Log         string `json:"log,omitempty"`
}

var allTools = []binmgr.Tool{binmgr.FFmpeg, binmgr.FFprobe, binmgr.DeepFilter, binmgr.AutoEditor, binmgr.Resvg}

// notDownloadedMsg is shown for a tool that's simply not fetched yet (not
// an actual failure) and has no more specific last-attempt error on
// record. Click "Download" to fetch it.
const notDownloadedMsg = "not downloaded yet - click Download to fetch"

// isKnownToolName reports whether name is a real binmgr.Tool or "font".
func isKnownToolName(name string) bool {
	if name == "font" {
		return true
	}
	for _, t := range allTools {
		if string(t) == name {
			return true
		}
	}
	return false
}

// resolveToolStatuses reports each tool's current status. For a tool
// that's downloading right now (per s.tools), that's a live progress
// snapshot with no filesystem access at all; otherwise it's a plain
// on-PATH/in-cache check via binmgr.Status/FontStatus (no download, so
// this always stays fast) with the last download error attached if there
// was one. Actually starting a download is handleResolveTool/
// handleResolveAllTools's job.
func (s *Server) resolveToolStatuses() []toolStatus {
	out := make([]toolStatus, 0, len(allTools)+1)
	for _, t := range allTools {
		out = append(out, s.oneToolStatus(string(t)))
	}
	out = append(out, s.oneToolStatus("font"))
	return out
}

func (s *Server) oneToolStatus(name string) toolStatus {
	if p, downloading := s.tools.snapshot(name); downloading {
		return toolStatus{Name: name, Downloading: true, Managed: true, Written: p.Written, Total: p.Total, Log: p.Log}
	}

	st := toolStatus{Name: name}
	var available bool
	if name == "font" {
		regular, bold, ok := binmgr.FontStatus()
		available = ok
		st.Managed = true // the Inter font is always vidpolish-managed
		if ok {
			st.Path = regular + ", " + bold
		}
	} else {
		path, ok, managed := binmgr.Status(binmgr.Tool(name))
		available = ok
		st.Managed = managed
		if ok {
			st.Path = path
		}
	}
	if !available {
		if lastErr, ok := s.tools.lastError(name); ok {
			st.Error = lastErr
		} else {
			st.Error = notDownloadedMsg
		}
	}
	return st
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.resolveToolStatuses())
}

// jobKeyForTool namespaces the shared jobTracker (also used for cell runs)
// so a tool name can never collide with a cell ID.
func jobKeyForTool(name string) string { return "tool-download:" + name }

// startToolResolve kicks off name's download/resolve (or, if force is
// true, a forced redownload even though it's already cached - see
// binmgr.Redownload) in a background goroutine, publishing live progress
// to s.tools for GET /api/tools to poll, and returns false (without
// starting anything) if it's already in flight. Used by
// handleResolveTool, handleRedownloadTool, and handleResolveAllTools -
// each name gets its own goroutine, so resolving several different tools
// concurrently is the normal case, not a special one.
func (s *Server) startToolResolve(name string, force bool) bool {
	if !s.jobs.start(jobKeyForTool(name)) {
		return false
	}
	s.tools.starting(name)
	go func() {
		defer s.jobs.finish(jobKeyForTool(name))
		prog := &binmgr.Progress{
			OnLog: func(msg string) { s.tools.setLog(name, msg) },
			OnProgress: func(written, total int64) {
				s.tools.setBytes(name, written, total)
			},
		}

		var err error
		switch {
		case name == "font" && force:
			_, _, err = binmgr.RedownloadFontWithProgress(prog)
		case name == "font":
			_, _, err = binmgr.ResolveFontWithProgress(prog)
		case force:
			_, err = binmgr.RedownloadWithProgress(binmgr.Tool(name), prog)
		default:
			_, err = binmgr.ResolveWithProgress(binmgr.Tool(name), prog)
		}

		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		s.tools.finished(name, errMsg)
	}()
	return true
}

// handleResolveTool starts fetching a missing tool (or re-verifying an
// already-resolved one) in the background and returns immediately -
// binmgr.Resolve for a real download can take anywhere from seconds to a
// few minutes, so the caller polls GET /api/tools for live progress
// instead of waiting on this response. Calling this again for a tool
// that's already downloading returns 409 rather than starting a
// redundant second download of the same file.
func (s *Server) handleResolveTool(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !isKnownToolName(name) {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown tool %q", name))
		return
	}
	if !s.startToolResolve(name, false) {
		writeError(w, http.StatusConflict, fmt.Errorf("%s is already being resolved", name))
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// handleRedownloadTool is handleResolveTool's "force" counterpart: it
// re-fetches name even if it's already present (e.g. to recover from a
// corrupted download), refusing only if name is provided by the system
// PATH rather than vidpolish's own cache (see binmgr.Redownload) - there
// vidpolish has nothing to redownload.
func (s *Server) handleRedownloadTool(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if !isKnownToolName(name) {
		writeError(w, http.StatusNotFound, fmt.Errorf("unknown tool %q", name))
		return
	}
	// Fail fast and synchronously for a tool vidpolish doesn't manage
	// (system PATH), rather than accepting the request and only
	// discovering async - inside startToolResolve's goroutine - that
	// binmgr.Redownload was always going to refuse it. The UI shouldn't
	// offer this button for an unmanaged tool anyway (see the Managed
	// field on toolStatus), so reaching here at all means something else
	// is calling the API directly.
	if name != "font" {
		if _, available, managed := binmgr.Status(binmgr.Tool(name)); available && !managed {
			writeError(w, http.StatusBadRequest, fmt.Errorf("%s is provided by your system PATH, not managed by vidpolish - nothing to redownload", name))
			return
		}
	}
	if !s.startToolResolve(name, true) {
		writeError(w, http.StatusConflict, fmt.Errorf("%s is already being resolved", name))
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// resolveAllResult reports, per tool, whether handleResolveAllTools
// actually kicked off a download for it.
type resolveAllResult struct {
	Name    string `json:"name"`
	Started bool   `json:"started"`
}

// handleResolveAllTools starts downloading every tool that isn't already
// available, all in parallel (one goroutine per tool, via
// startToolResolve) - a one-click "download everything missing" action
// instead of clicking Download on each row one at a time. Tools that are
// already resolved, or already mid-download from a previous call, are
// skipped and reported with started=false. It never forces a redownload
// of an already-present tool - that's handleRedownloadTool's job, one
// tool at a time, since it's specifically for recovering a suspected-bad
// binary rather than routine setup.
func (s *Server) handleResolveAllTools(w http.ResponseWriter, r *http.Request) {
	results := make([]resolveAllResult, 0, len(allTools)+1)

	for _, t := range allTools {
		name := string(t)
		if _, available, _ := binmgr.Status(t); available {
			results = append(results, resolveAllResult{Name: name, Started: false})
			continue
		}
		results = append(results, resolveAllResult{Name: name, Started: s.startToolResolve(name, false)})
	}

	if _, _, available := binmgr.FontStatus(); available {
		results = append(results, resolveAllResult{Name: "font", Started: false})
	} else {
		results = append(results, resolveAllResult{Name: "font", Started: s.startToolResolve("font", false)})
	}

	writeJSON(w, http.StatusAccepted, results)
}
