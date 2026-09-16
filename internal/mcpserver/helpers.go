package mcpserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"vidpolish/internal/binmgr"
	"vidpolish/internal/pipeline"
	"vidpolish/internal/store"
)

// jobTracker prevents a cell from being run twice concurrently, mirroring
// internal/server's jobTracker for the same reason: running many
// different cells in parallel is fine, running the same one twice is not.
type jobTracker struct {
	mu      sync.Mutex
	running map[string]bool
}

func newJobTracker() *jobTracker {
	return &jobTracker{running: make(map[string]bool)}
}

func (j *jobTracker) start(cellID string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.running[cellID] {
		return false
	}
	j.running[cellID] = true
	return true
}

func (j *jobTracker) finish(cellID string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	delete(j.running, cellID)
}

func formatETA(fraction float64, eta time.Duration) string {
	etaStr := "..."
	if eta > 0 {
		etaStr = eta.Round(time.Second).String()
	}
	return fmt.Sprintf("uploading: %.0f%% (ETA %s)", fraction*100, etaStr)
}

// projectResponse and cellResponse mirror the JSON shapes internal/server's
// REST API returns, so an MCP client sees the same project/cell model the
// web UI does.
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
	OutputPath     string `json:"outputPath,omitempty"`
	SourceFilename string `json:"sourceFilename,omitempty"`
	YouTubeURL     string `json:"youtubeUrl,omitempty"`
	Position       int    `json:"position"`
	CreatedAt      int64  `json:"createdAt"`
	UpdatedAt      int64  `json:"updatedAt"`
}

func cellToResponse(c *store.Cell) *cellResponse {
	r := &cellResponse{
		ID:        c.ID,
		Seq:       c.Seq,
		Name:      c.DisplayName(),
		Kind:      c.Kind,
		Status:    c.Status,
		Position:  c.Position,
		CreatedAt: c.CreatedAt.Unix(),
		UpdatedAt: c.UpdatedAt.Unix(),
	}
	if c.ParentCellID != nil {
		r.ParentCellID = *c.ParentCellID
	}
	if c.StatusMessage != nil {
		r.StatusMessage = *c.StatusMessage
	}
	if c.OutputPath != nil {
		r.OutputPath = *c.OutputPath
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

// resolveCellRef resolves ref to a cell within project, trying it first as
// a raw cell ID and falling back to seq/name matching (store.GetCellByRef).
func (s *Server) resolveCellRef(projectID, ref string) (*store.Cell, error) {
	if c, err := s.db.GetCell(ref); err == nil && c.ProjectID == projectID {
		return c, nil
	}
	return s.db.GetCellByRef(projectID, ref)
}

// outputDirFor returns the directory a directly-processed video (i.e. one
// not run through a project's edit cell) is written to when the caller
// didn't specify one: the input file's own directory.
func outputDirFor(inputPath string) (string, error) {
	abs, err := filepath.Abs(inputPath)
	if err != nil {
		return "", fmt.Errorf("resolving input path: %w", err)
	}
	return filepath.Dir(abs), nil
}

// projectDir returns (creating it) ~/.vidpolish/projects/<id>, the same
// layout internal/server uses for a project's source video and edit-cell
// outputs, so cells created via MCP interoperate with the web UI.
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

// cellThumbnailPath returns the deterministic path an upload cell's
// generated thumbnail is written to and read back from.
func cellThumbnailPath(cell *store.Cell) (string, error) {
	dir, err := projectDir(cell.ProjectID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "cells", cell.ID, "thumbnail.png"), nil
}

// secretField mirrors internal/server's redaction shape: an MCP client is
// an LLM, and config.toml stores the YouTube client secret and refresh
// token in plaintext, so neither should ever appear verbatim in a tool
// result.
type secretField struct {
	Set     bool   `json:"set"`
	Preview string `json:"preview,omitempty"`
}

func redactSecret(v string) secretField {
	if v == "" {
		return secretField{Set: false}
	}
	preview := v
	if len(v) > 8 {
		preview = v[:4] + "…" + v[len(v)-4:]
	}
	return secretField{Set: true, Preview: preview}
}

// clamp constrains n to [min, max], substituting def for a non-positive
// (unset) n first.
func clamp(n, min, max, def int) int {
	if n <= 0 {
		n = def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}

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

type mediaComparison struct {
	SizeChangePct       float64 `json:"sizeChangePct"`
	DurationChangePct   float64 `json:"durationChangePct"`
	BitrateChangePct    float64 `json:"bitrateChangePct"`
	ResolutionChangePct float64 `json:"resolutionChangePct"`
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
