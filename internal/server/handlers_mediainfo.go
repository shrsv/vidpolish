package server

import (
	"net/http"

	"vidpolish/internal/binmgr"
	"vidpolish/internal/pipeline"
	"vidpolish/internal/store"
)

// cellMediaInfoResponse is what GET /api/cells/{id}/info returns: the
// cell's own video properties (if it has run), the properties of the
// video it was/would be run against (an edit cell's parent source; nil
// for a source cell, since there's nothing "before" it), and — only when
// both are known for an edit cell — a same-shape percent-change summary
// so the UI doesn't have to compute it.
type cellMediaInfoResponse struct {
	Self             *pipeline.VideoInfo `json:"self,omitempty"`
	Source           *pipeline.VideoInfo `json:"source,omitempty"`
	ComparedToSource *mediaComparison    `json:"comparedToSource,omitempty"`
}

// mediaComparison is Self vs. Source expressed as percent change (e.g.
// SizeChangePct: -60 means the edit is 60% smaller than the source).
// ResolutionChangePct compares pixel area (width*height), since width and
// height individually don't collapse to one meaningful number.
type mediaComparison struct {
	SizeChangePct       float64 `json:"sizeChangePct"`
	DurationChangePct   float64 `json:"durationChangePct"`
	BitrateChangePct    float64 `json:"bitrateChangePct"`
	ResolutionChangePct float64 `json:"resolutionChangePct"`
}

// handleCellInfo probes and returns a cell's video properties, plus (for
// edit cells) its parent source's properties and a comparison between the
// two — powering the per-cell media-info display and the "original"
// placeholders in the resize/bitrate form. Probing is best-effort per
// video: a video that isn't ready yet, or that briefly fails to probe,
// just comes back as a nil field rather than failing the whole request.
func (s *Server) handleCellInfo(w http.ResponseWriter, r *http.Request) {
	cell, err := s.db.GetCell(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	ffprobePath, err := binmgr.Resolve(binmgr.FFprobe)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	var resp cellMediaInfoResponse
	if cell.OutputPath != nil && *cell.OutputPath != "" {
		if info, err := pipeline.ProbeVideoInfo(ffprobePath, *cell.OutputPath); err == nil {
			resp.Self = &info
		}
	}

	if cell.Kind == store.KindEdit && cell.ParentCellID != nil {
		if parent, err := s.db.GetCell(*cell.ParentCellID); err == nil &&
			parent.OutputPath != nil && *parent.OutputPath != "" {
			if info, err := pipeline.ProbeVideoInfo(ffprobePath, *parent.OutputPath); err == nil {
				resp.Source = &info
				if resp.Self != nil {
					resp.ComparedToSource = compareVideoInfo(*resp.Self, info)
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func compareVideoInfo(self, source pipeline.VideoInfo) *mediaComparison {
	pct := func(a, b float64) float64 {
		if b == 0 {
			return 0
		}
		return (a - b) / b * 100
	}
	selfArea := float64(self.Width * self.Height)
	sourceArea := float64(source.Width * source.Height)
	return &mediaComparison{
		SizeChangePct:       pct(float64(self.SizeBytes), float64(source.SizeBytes)),
		DurationChangePct:   pct(self.DurationSec, source.DurationSec),
		BitrateChangePct:    pct(float64(self.BitrateKbps), float64(source.BitrateKbps)),
		ResolutionChangePct: pct(selfArea, sourceArea),
	}
}
