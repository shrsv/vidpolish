package server

import (
	"encoding/json"
	"fmt"
	"time"

	"vidpolish/internal/store"
)

// progress is the structured, multi-step status vidpolish publishes for
// any cell task that has more than one stage (a source upload, an edit
// cell's run, a YouTube upload cell's run). It's JSON-encoded straight
// into the same status_message/SSE channel a cell's plain-text status
// already used (see (*Server).log) - the frontend tries to parse it as
// JSON and falls back to showing it as plain text (e.g. "queued...",
// "error: ...") when it isn't.
//
// Steps is the FULL ordered list of stages this run will go through,
// computed once up front before the run starts (see e.g.
// pipeline.StageLabels) - so a progress UI can render every stage
// immediately, with ticks for the ones already done, instead of only
// finding out the total step count as each one happens.
type progress struct {
	Steps      []string `json:"steps"`
	StepIndex  int      `json:"stepIndex"`
	StepPct    float64  `json:"stepPct"`
	OverallPct float64  `json:"overallPct"`
	ETA        string   `json:"eta,omitempty"`
}

// stepProgressReporter publishes throttled progress updates across a
// fixed, known-upfront list of steps for one cell's running task,
// computing overall percentage/ETA from a step index + that step's own
// local percentage.
type stepProgressReporter struct {
	s       *Server
	cellID  string
	steps   []string
	start   time.Time
	lastPub time.Time
}

func (s *Server) newStepProgressReporter(cellID string, steps []string) *stepProgressReporter {
	return &stepProgressReporter{s: s, cellID: cellID, steps: steps, start: time.Now()}
}

// report publishes progress for stepIndex (0-based, into r.steps) at
// stepPct (0-100) through that step. Throttled to avoid hammering the
// DB/SSE hub on a fast-moving local ffmpeg progress stream, except the
// very first call (so a stepper has the full step list immediately) and
// a step boundary/completion (stepPct 0 or 100) always goes through.
func (r *stepProgressReporter) report(stepIndex int, stepPct float64) {
	stepPct = min(100, max(0, stepPct))
	now := time.Now()
	force := r.lastPub.IsZero() || stepPct == 0 || stepPct == 100
	if !force && now.Sub(r.lastPub) < 300*time.Millisecond {
		return
	}
	r.lastPub = now

	total := len(r.steps)
	if total == 0 {
		return
	}
	overall := (float64(stepIndex) + stepPct/100) / float64(total) * 100
	overall = min(100, max(0, overall))

	eta := ""
	if overall > 2 && overall < 100 {
		elapsed := now.Sub(r.start).Seconds()
		etaSeconds := elapsed / overall * (100 - overall)
		eta = time.Duration(etaSeconds * float64(time.Second)).Round(time.Second).String()
	}

	p := progress{
		Steps:      r.steps,
		StepIndex:  stepIndex,
		StepPct:    stepPct,
		OverallPct: overall,
		ETA:        eta,
	}
	data, err := json.Marshal(p)
	if err != nil {
		return
	}
	msg := string(data)
	r.s.db.UpdateCellStatus(r.cellID, store.StatusRunning, msg)
	r.s.hub.Publish(r.cellID, msg)
}

// reportText publishes a plain-text status line instead of structured
// step progress - for phases with no fractional progress of their own
// (e.g. waiting on YouTube's processing status). The frontend falls back
// to showing this as a single line the same way it always has.
func (s *Server) reportText(cellID, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	s.db.UpdateCellStatus(cellID, store.StatusRunning, msg)
	s.hub.Publish(cellID, msg)
}
