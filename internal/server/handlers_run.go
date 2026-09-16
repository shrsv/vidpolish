package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"vidpolish/internal/config"
	"vidpolish/internal/pipeline"
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

	if err := s.writeSourceVideo(cell, file, header.Filename); err != nil {
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

	if err := s.writeSourceVideo(cell, src, filepath.Base(body.Path)); err != nil {
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

// writeSourceVideo copies src into cell's project directory and records
// it as the source cell's video. filename is only used for its extension
// and to record what the original file was called.
func (s *Server) writeSourceVideo(cell *store.Cell, src io.Reader, filename string) error {
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
	written, err := io.Copy(out, src)
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

	out, err := pipeline.Process(pipeline.Options{
		Input:       *source.OutputPath,
		OutputDir:   outDir,
		Margin:      params.Margin,
		Speed:       params.Speed,
		Width:       params.Width,
		Height:      params.Height,
		BitrateKbps: params.BitrateKbps,
		Denoise:     !params.SkipDenoise,
		Log:         s.log(cell.ID),
	})
	if err != nil {
		return err
	}
	return s.db.SetCellOutput(cell.ID, out)
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
			msg := fmt.Sprintf("uploading: %.0f%% (ETA %s)", fraction*100, formatETA(eta))
			s.db.UpdateCellStatus(cell.ID, store.StatusRunning, msg)
			s.hub.Publish(cell.ID, msg)
		},
		OnUploaded: func(result *ytupload.Result) {
			// Record and broadcast the link immediately, the same
			// "don't wait for processing to show it" behavior the CLI
			// has, so it shows up in the UI right away.
			s.db.SetCellYouTubeURL(cell.ID, result.URL)
			s.hub.Publish(cell.ID, "uploaded: "+result.URL)
		},
	})
	return err
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
