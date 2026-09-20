package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"vidpolish/internal/binmgr"
	"vidpolish/internal/config"
	"vidpolish/internal/pipeline"
	"vidpolish/internal/procutil"
	"vidpolish/internal/store"
	"vidpolish/internal/thumbnail"
	"vidpolish/internal/ytauth"
	"vidpolish/internal/ytmeta"
	"vidpolish/internal/ytupload"
)

// handleUploadSource streams a dragged-in/picked video onto a project's
// source cell, reading it from the request body (the plain-browser
// `vidpolish ui` upload path: an <input type="file">/drag-drop File
// object read via fetch/XHR).
func (s *Server) handleUploadSource(w http.ResponseWriter, r *http.Request) {
	cell, err := s.getSourceCell(w, r)
	if err != nil {
		return
	}

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	file, header, err := r.FormFile("video")
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing 'video' file field: %w", err))
		return
	}
	defer file.Close()

	if err := s.writeSourceVideo(cell, file, header.Filename, header.Size); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	c, _ := s.db.GetCell(cell.ID)
	writeJSON(w, http.StatusOK, cellToResponse(c))
}

// handleUploadSourceFromPath sets a project's source cell from a file
// already sitting on the local disk, given its absolute path - read
// directly by the Go backend via os.Open, with no browser file-content
// API involved at all.
//
// This exists because a file picked or dropped inside the Wails GUI's
// WebView2-hosted page can come back with an unreadable/empty browser
// File object (confirmed: a real multi-megabyte video picked via
// <input type="file"> logged header.Size=0 in handleUploadSource).
// cmd/vidpolish-gui's native file dialog and drag-and-drop bridge hand
// back a real OS path instead, which this endpoint reads directly -
// sidestepping that WebView2 limitation entirely rather than working
// around its symptoms.
func (s *Server) handleUploadSourceFromPath(w http.ResponseWriter, r *http.Request) {
	cell, err := s.getSourceCell(w, r)
	if err != nil {
		return
	}

	var body struct {
		Path string `json:"path"`
	}
	if err := readJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if body.Path == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("missing \"path\""))
		return
	}

	src, err := os.Open(body.Path)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("opening %s: %w", body.Path, err))
		return
	}
	defer src.Close()

	var size int64
	if info, statErr := src.Stat(); statErr == nil {
		size = info.Size()
	}
	if err := s.writeSourceVideo(cell, src, filepath.Base(body.Path), size); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	c, _ := s.db.GetCell(cell.ID)
	writeJSON(w, http.StatusOK, cellToResponse(c))
}

// getSourceCell fetches the cell named in the request path and confirms
// it's a source cell, writing an error response and returning a non-nil
// error itself if not (the caller should return immediately in that
// case).
func (s *Server) getSourceCell(w http.ResponseWriter, r *http.Request) (*store.Cell, error) {
	cellID := r.PathValue("id")
	cell, err := s.db.GetCell(cellID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return nil, err
	}
	if cell.Kind != store.KindSource {
		err := fmt.Errorf("cell is not a source cell")
		writeError(w, http.StatusBadRequest, err)
		return nil, err
	}
	return cell, nil
}

// copyProgressWriter is an io.Writer (fed via io.TeeReader) that calls
// report with a 0-100 percentage as bytes flow through, throttled to
// avoid hammering the DB/SSE hub with an update per chunk on a fast local
// copy.
type copyProgressWriter struct {
	total      int64
	written    int64
	lastReport time.Time
	report     func(pct float64)
}

func (p *copyProgressWriter) Write(b []byte) (int, error) {
	p.written += int64(len(b))
	if now := time.Now(); now.Sub(p.lastReport) >= 300*time.Millisecond {
		p.lastReport = now
		p.report(min(100, float64(p.written)/float64(p.total)*100))
	}
	return len(b), nil
}

// sourceUploadSteps is the full, fixed step list every source upload goes
// through - published upfront (see stepProgressReporter) so the frontend
// can render the whole stepper immediately instead of only learning the
// total step count as each one happens.
var sourceUploadSteps = []string{
	"Copying video",
	"Optimizing for playback",
	"Generating poster",
	"Generating preview",
}

// writeSourceVideo copies src into cell's project directory and records
// it as the source cell's video. filename is only used for its extension
// and to record what the original file was called. totalSize, if known
// (0 if not), drives real "Copying video" progress; without it, a large
// upload (particularly the Wails desktop app's path-based upload, which
// otherwise blocks a single request/response with zero feedback for
// however long the copy takes) looks indistinguishable from a hung app.
// Every stage of this multi-step process (copy, faststart remux, poster,
// preview) reports through one shared stepProgressReporter, so the
// frontend can show a real stepper - all steps up front, ticks for
// finished ones, a live percentage/ETA for whichever is active - the same
// as an edit cell's own run.
func (s *Server) writeSourceVideo(cell *store.Cell, src io.Reader, filename string, totalSize int64) error {
	pr := s.newStepProgressReporter(cell.ID, sourceUploadSteps)
	pr.report(0, 0) // show the full stepper immediately, before any work starts

	dir, err := projectDir(cell.ProjectID)
	if err != nil {
		return err
	}
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".mp4"
	}
	dest := filepath.Join(dir, "source"+ext)

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}

	var written int64
	if totalSize > 0 {
		written, err = io.Copy(out, io.TeeReader(src, &copyProgressWriter{
			total:  totalSize,
			report: func(pct float64) { pr.report(0, pct) },
		}))
	} else {
		written, err = io.Copy(out, src)
	}
	// Deliberately logged unconditionally (not gated on a Debug flag):
	// this is the one piece of evidence that tells apart "the source
	// handed us an empty file" from "we received it fine but something
	// after that lost the bytes".
	fmt.Fprintf(os.Stderr, "vidpolish: writing source video %s: written=%d err=%v\n", filename, written, err)
	if err != nil {
		out.Close()
		return err
	}
	// Close (and check the error) synchronously here, before returning,
	// rather than via defer: a deferred Close only runs once the calling
	// handler function returns, which is after it has already responded
	// - the client can react to that (e.g. the video element requesting
	// this file back) before the deferred Close actually commits the
	// write. On Windows/NTFS in particular, a file's reported size can
	// lag behind its written bytes until Close/flush commits the size
	// metadata, so a Stat() from that immediate follow-up request can
	// genuinely see size 0 - closing first eliminates the race instead
	// of just usually getting away with it.
	if err := out.Close(); err != nil {
		return err
	}
	pr.report(0, 100)

	// A raw screen recording typically has its moov atom (the index a
	// player needs before it can even report a duration) at the very end
	// of the file, not the front - so without this, the browser's <video>
	// element has to fetch near the tail of a several-hundred-MB file
	// just to learn how long it is, which can look exactly like an
	// indefinitely stuck loading spinner over an otherwise fine upload.
	// This is a stream copy, not a re-encode, so it's seconds of I/O, not
	// minutes - status is reported the same way an edit cell's progress
	// is, so it isn't invisible.
	pr.report(1, 0)
	ffmpegPath, ffmpegErr := binmgr.Resolve(binmgr.FFmpeg)
	var duration float64
	if ffmpegErr == nil {
		if ffprobePath, err := binmgr.Resolve(binmgr.FFprobe); err == nil {
			if info, err := pipeline.ProbeVideoInfo(ffprobePath, dest); err == nil {
				duration = info.DurationSec
			}
		}
		if err := pipeline.RemuxFaststart(ffmpegPath, dest, duration, func(local float64) { pr.report(1, local*100) }); err != nil {
			// Some exotic source container could plausibly fail this -
			// worth logging, not worth failing an otherwise-successful
			// upload over; the video just keeps its original (slower to
			// preview) layout.
			fmt.Fprintf(os.Stderr, "vidpolish: faststart remux warning for source cell %s: %v\n", cell.ID, err)
		}
	}
	pr.report(1, 100)

	// Poster/preview generation both happen before SetSourceCellVideo marks
	// the cell done (mirroring runEditCell's own ordering) - marking it
	// done first and then continuing to publish "running"-style status
	// updates during preview generation left the cell stuck showing
	// "generating preview..." indefinitely instead of ever settling back
	// to done, since nothing set it back afterward.
	pr.report(2, 0)
	if err := s.generateCellPoster(cell, dest); err != nil {
		// Best-effort, same as an edit cell's poster: worth logging, not
		// worth failing an otherwise-successful upload over.
		fmt.Fprintf(os.Stderr, "vidpolish: poster generation warning for source cell %s: %v\n", cell.ID, err)
	}
	pr.report(2, 100)

	pr.report(3, 0)
	if err := s.generateCellPreview(cell, dest, "", 0, func(local float64) { pr.report(3, local*100) }); err != nil {
		// Best-effort: without a preview proxy, the frontend just falls
		// back to playing the raw file directly (see cellToResponse) -
		// slower/riskier to preview, not a failed upload.
		fmt.Fprintf(os.Stderr, "vidpolish: preview generation warning for source cell %s: %v\n", cell.ID, err)
	}
	pr.report(3, 100)

	return s.db.SetSourceCellVideo(cell.ID, dest, filename)
}

// handleRunCell starts a cell's job in the background if it isn't already
// running, and returns immediately; progress/completion is delivered over
// GET /api/cells/{id}/events.
func (s *Server) handleRunCell(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cell, err := s.db.GetCell(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if cell.Kind == store.KindSource {
		writeError(w, http.StatusBadRequest, fmt.Errorf("the source cell runs implicitly when a video is uploaded"))
		return
	}
	if cell.Kind == store.KindText {
		writeError(w, http.StatusBadRequest, fmt.Errorf("text cells don't run"))
		return
	}
	if !s.jobs.start(id) {
		writeError(w, http.StatusConflict, fmt.Errorf("cell is already running"))
		return
	}

	s.db.UpdateCellStatus(id, store.StatusRunning, "starting...")
	s.hub.Publish(id, "starting...")

	go func() {
		defer s.jobs.finish(id)
		var runErr error
		switch cell.Kind {
		case store.KindEdit:
			runErr = s.runEditCell(cell)
		case store.KindUpload:
			runErr = s.runUploadCell(cell)
		}
		if runErr != nil {
			s.db.UpdateCellStatus(id, store.StatusError, runErr.Error())
			s.hub.Publish(id, "error: "+runErr.Error())
		} else {
			// Edit cells: SetCellOutput already marked it done. Upload
			// cells: mark done now (youtube_url was already set as soon
			// as it was known, via onUploaded below).
			if cell.Kind == store.KindUpload {
				s.db.UpdateCellStatus(id, store.StatusDone, "done")
			}
			s.hub.Publish(id, "__done__")
		}
	}()

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) log(cellID string) func(string) {
	return func(msg string) {
		s.db.UpdateCellStatus(cellID, store.StatusRunning, msg)
		s.hub.Publish(cellID, msg)
	}
}

// stderrLog returns a plain-text logger that only writes to stderr (the
// GUI build redirects this to gui.log - see cmd/vidpolish-gui/main.go),
// never to a cell's status/SSE channel. Used as pipeline.Options.Log
// wherever OnProgress is also wired up: the pipeline's occasional
// non-per-tick plain-text lines (a cache hit, the final "done" line)
// would otherwise still publish on that same channel and briefly
// overwrite a structured stepper's JSON with plain text each time one
// fires.
func stderrLog(prefix string) func(string) {
	return func(msg string) {
		fmt.Fprintf(os.Stderr, "vidpolish: %s: %s\n", prefix, msg)
	}
}

func (s *Server) runEditCell(cell *store.Cell) error {
	var params EditParams
	if err := json.Unmarshal([]byte(cell.ParamsJSON), &params); err != nil {
		return fmt.Errorf("invalid edit params: %w", err)
	}
	if params.Speed == 0 {
		params.Speed = 1.0
	}

	source, err := s.db.GetCell(*cell.ParentCellID)
	if err != nil {
		return err
	}
	if source.OutputPath == nil || *source.OutputPath == "" {
		return fmt.Errorf("source cell has no video yet")
	}

	dir, err := projectDir(cell.ProjectID)
	if err != nil {
		return err
	}
	outDir := filepath.Join(dir, "cells", cell.ID)

	// Several edit cells (e.g. speed variants of the same large source)
	// can be started at once; running them all concurrently is what
	// actually pegs every CPU core and saturates disk I/O enough to drag
	// down the whole OS on a big file. Acquire the shared pipeline
	// semaphore before doing any real work - extras queue here instead of
	// starting immediately, with a status update explaining why.
	select {
	case s.pipelineSem <- struct{}{}:
	default:
		s.db.UpdateCellStatus(cell.ID, store.StatusRunning, "queued...")
		s.hub.Publish(cell.ID, "queued...")
		s.pipelineSem <- struct{}{}
	}
	defer func() { <-s.pipelineSem }()

	cfg, _ := config.Load() // nil is fine: pipeline.ResolveEncoder and the thread/cache-cap below all handle a nil/missing config as "use defaults"

	ffmpegPath, err := binmgr.Resolve(binmgr.FFmpeg)
	if err != nil {
		return err
	}
	encoder, err := pipeline.ResolveEncoder(cfg, ffmpegPath)
	if err != nil {
		// A GPU-detection hiccup shouldn't fail the whole edit - just fall
		// back to software encoding for this run.
		s.log(cell.ID)(fmt.Sprintf("==> GPU detection failed, using software encoding: %v", err))
		encoder = ""
	}

	maxJobs := config.DefaultMaxConcurrentJobs()
	maxCacheGB := config.DefaultMaxCacheGB
	if cfg != nil {
		maxJobs = cfg.Performance.MaxConcurrentJobs
		maxCacheGB = cfg.Performance.MaxCacheGB
	}
	maxThreads := max(1, runtime.NumCPU()/max(1, maxJobs))

	// The full step list is known upfront from the same params that decide
	// pipeline.Process's own stage plan (see pipeline.StageLabels), plus
	// the two post-processing steps this handler runs after it - so the
	// frontend can render every step immediately (with the pipeline's own
	// real sub-stage progress, not just one opaque "processing" step),
	// rather than only learning the total count as each stage happens.
	needsResize := params.Width > 0 || params.Height > 0 || params.BitrateKbps > 0
	pipelineSteps := pipeline.StageLabels(!params.SkipDenoise, needsResize)
	steps := append(append([]string{}, pipelineSteps...), "Generating poster", "Generating preview")
	posterStep, previewStep := len(pipelineSteps), len(pipelineSteps)+1

	pr := s.newStepProgressReporter(cell.ID, steps)
	pr.report(0, 0) // show the full stepper immediately, before any work starts

	out, err := pipeline.Process(pipeline.Options{
		Input:         *source.OutputPath,
		OutputDir:     outDir,
		Margin:        params.Margin,
		Speed:         params.Speed,
		Width:         params.Width,
		Height:        params.Height,
		BitrateKbps:   params.BitrateKbps,
		Denoise:       !params.SkipDenoise,
		Encoder:       encoder,
		MaxThreads:    maxThreads,
		MaxCacheBytes: int64(maxCacheGB) << 30,
		// A stderr-only logger, not s.log(cell.ID): with OnProgress wired
		// up, per-tick progress already goes out as structured JSON below
		// - the pipeline's other, occasional plain-text lines (a cache
		// hit, the final "done" line) publishing on that same per-cell
		// status/SSE channel would otherwise still briefly overwrite the
		// stepper with plain text each time one fires.
		Log:        stderrLog("edit cell " + cell.ID),
		OnProgress: func(stepIndex int, stepPct float64) { pr.report(stepIndex, stepPct) },
	})
	if err != nil {
		return err
	}

	pr.report(posterStep, 0)
	if err := s.generateCellPoster(cell, out); err != nil {
		// Best-effort: a missing poster just means the <video> falls back
		// to showing nothing until played, not a failed run.
		fmt.Fprintf(os.Stderr, "vidpolish: poster generation warning for cell %s: %v\n", cell.ID, err)
	}
	pr.report(posterStep, 100)

	pr.report(previewStep, 0)
	if err := s.generateCellPreview(cell, out, encoder, maxThreads, func(local float64) { pr.report(previewStep, local*100) }); err != nil {
		// Best-effort: without a preview proxy, the frontend just falls
		// back to playing the real output directly - not a failed run.
		fmt.Fprintf(os.Stderr, "vidpolish: preview generation warning for cell %s: %v\n", cell.ID, err)
	}
	pr.report(previewStep, 100)
	return s.db.SetCellOutput(cell.ID, out)
}

// generateCellPoster grabs a single still frame from an edit cell's
// finished output and writes it to cellPosterPath, for use as the
// frontend's <video poster> (see handleCellPoster).
func (s *Server) generateCellPoster(cell *store.Cell, videoPath string) error {
	out, err := cellPosterPath(cell)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	ffmpegPath, err := binmgr.Resolve(binmgr.FFmpeg)
	if err != nil {
		return err
	}

	// Runs right after a large file was just written and closed (the raw
	// upload copy, or a faststart remux) - on Windows in particular,
	// antivirus real-time scanning can hold a newly-written file locked
	// for a brief moment right after close, which shows up here as an
	// otherwise-unexplained open/read failure. One short retry rides that
	// out instead of leaving the poster permanently missing over a
	// few-hundred-ms timing issue.
	var lastErr error
	for attempt := range 2 {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		// -update 1: this ffmpeg build's image2 muxer otherwise refuses to
		// write a single literal filename at all ("does not contain an
		// image sequence pattern"), treating -frames:v 1 alone as not
		// enough of a signal that this isn't a %03d-style sequence.
		cmd := exec.Command(ffmpegPath, "-y", "-ss", "1", "-i", videoPath, "-frames:v", "1", "-update", "1", "-vf", "scale=480:-1", out)
		procutil.HideWindow(cmd)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			lastErr = fmt.Errorf("ffmpeg poster grab failed: %w\n%s", err, stderr.String())
			continue
		}
		return nil
	}
	return lastErr
}

// generateCellPreview transcodes a source/edit cell's finished output into
// a small, standardized preview proxy (see pipeline.GeneratePreviewProxy)
// for the frontend's <video> to actually play, so the browser is never
// handed a raw, arbitrarily large/idiosyncratic recording directly.
// encoder/maxThreads are passed through as-is (empty/0 is fine - falls
// back to software libx264, uncapped threads). report, if non-nil,
// receives real fractional progress (0..1) - the caller wires this into
// its own multi-step progress reporter (see stepProgressReporter) rather
// than this function publishing status on its own, so a several-minute
// re-encode shows up as one step of a larger stepper instead of its own
// disconnected status line.
func (s *Server) generateCellPreview(cell *store.Cell, videoPath, encoder string, maxThreads int, report func(local float64)) error {
	out, err := cellPreviewPath(cell)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	ffmpegPath, err := binmgr.Resolve(binmgr.FFmpeg)
	if err != nil {
		return err
	}

	// Duration drives real percentage progress instead of a single static
	// "generating preview..." message for however long a multi-minute
	// source's re-encode takes - a probe failure just means progress
	// falls back to no percentage within this step, not a failed preview.
	var duration float64
	if ffprobePath, err := binmgr.Resolve(binmgr.FFprobe); err == nil {
		if info, err := pipeline.ProbeVideoInfo(ffprobePath, videoPath); err == nil {
			duration = info.DurationSec
		}
	}

	// Same transient-Windows-file-lock reasoning as generateCellPoster.
	var lastErr error
	for attempt := range 2 {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		if err := pipeline.GeneratePreviewProxy(ffmpegPath, videoPath, out, encoder, maxThreads, duration, report); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func (s *Server) runUploadCell(cell *store.Cell) error {
	var params UploadParams
	if err := json.Unmarshal([]byte(cell.ParamsJSON), &params); err != nil {
		return fmt.Errorf("invalid upload params: %w", err)
	}

	editCell, err := s.db.GetCell(*cell.ParentCellID)
	if err != nil {
		return err
	}
	if editCell.OutputPath == nil || *editCell.OutputPath == "" {
		return fmt.Errorf("edit cell %s has no output yet; run it first", editCell.DisplayName())
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.YouTube.RefreshToken == "" {
		return fmt.Errorf("not logged in to YouTube; connect it from the config panel first")
	}

	// The full step list is known upfront (whether thumbnail generation
	// runs is decided by params/config, already both in hand) - see
	// writeSourceVideo/runEditCell for the same reasoning: show the whole
	// stepper immediately rather than only learning the step count as
	// each stage happens. NoWait is hardcoded false below, so "Processing
	// on YouTube" always applies.
	needsThumbnailGen := (params.ThumbnailMode == "" || params.ThumbnailMode == "auto") && cfg.Thumbnail.Enabled
	var steps []string
	if needsThumbnailGen {
		steps = append(steps, "Generating thumbnail")
	}
	steps = append(steps, "Uploading to YouTube", "Processing on YouTube")
	uploadStep, processingStep := len(steps)-2, len(steps)-1

	pr := s.newStepProgressReporter(cell.ID, steps)
	pr.report(0, 0) // show the full stepper immediately, before any work starts

	title := params.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(*editCell.OutputPath), filepath.Ext(*editCell.OutputPath))
	}
	privacy := params.Privacy
	if privacy == "" {
		privacy = cfg.YouTube.Privacy
	}
	language := params.Language
	if language == "" {
		language = cfg.YouTube.DefaultLanguage
	}
	tags := ytmeta.MergeTags(append(params.Tags, cfg.YouTube.DefaultTags...))
	description := params.Description
	if description == "" {
		description = ytmeta.BuildDescription(cfg.YouTube.DescriptionTemplate, title, s.log(cell.ID))
	}

	thumbnailPath := ""
	switch params.ThumbnailMode {
	case "custom":
		thumbnailPath = params.ThumbnailPath
	case "none":
		// no thumbnail
	default: // "auto" or unset
		if cfg.Thumbnail.Enabled {
			pr.report(0, 0)
			out, err := cellThumbnailPath(cell)
			if err != nil {
				return err
			}
			thumbnailPath, err = thumbnail.Generate(thumbnail.Options{
				Title:           title,
				LogoPath:        cfg.Thumbnail.LogoPath,
				BackgroundColor: cfg.Thumbnail.BackgroundColor,
				AccentColor:     cfg.Thumbnail.AccentColor,
				TextColor:       cfg.Thumbnail.TextColor,
				OutPath:         out,
			})
			if err != nil {
				return fmt.Errorf("generating thumbnail: %w", err)
			}
			s.log(cell.ID)("==> generated thumbnail preview")
			pr.report(0, 100)
		}
	}

	accessToken, err := ytauth.AccessToken(cfg.YouTube.ClientID, cfg.YouTube.ClientSecret, cfg.YouTube.RefreshToken)
	if err != nil {
		return fmt.Errorf("refreshing YouTube access token: %w", err)
	}

	_, err = ytupload.Upload(*editCell.OutputPath, ytupload.Options{
		AccessToken:   accessToken,
		Title:         title,
		Description:   description,
		Tags:          tags,
		Privacy:       privacy,
		Language:      language,
		ThumbnailPath: thumbnailPath,
		NoWait:        false,
		Log:           s.log(cell.ID),
		Progress: func(fraction float64, eta time.Duration) {
			pr.report(uploadStep, fraction*100)
		},
		OnUploaded: func(result *ytupload.Result) {
			// Record and broadcast the link immediately, the same
			// "don't wait for processing to show it" behavior the CLI
			// has, so it shows up in the UI right away. Published
			// directly (not through pr): Cell.jsx specifically watches
			// for a raw "uploaded: " SSE message to show the link live,
			// separate from - and not overwritten by - the JSON step
			// progress in status_message.
			s.db.SetCellYouTubeURL(cell.ID, result.URL)
			s.hub.Publish(cell.ID, "uploaded: "+result.URL)
			pr.report(uploadStep, 100)
			pr.report(processingStep, 0)
		},
	})
	if err != nil {
		return err
	}
	pr.report(processingStep, 100)
	return nil
}

func formatETA(d time.Duration) string {
	if d <= 0 {
		return "..."
	}
	return d.Round(time.Second).String()
}

// handleCellEvents streams a cell's progress lines as Server-Sent Events.
// "__done__" is a sentinel telling the client the job finished (success
// or error, distinguished by then re-fetching GET /api/cells/{id}).
func (s *Server) handleCellEvents(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming unsupported"))
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, cancel := s.hub.Subscribe(id)
	defer cancel()

	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", strconv.Quote(msg))
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
