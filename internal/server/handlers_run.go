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

// handleUploadSource streams a dragged-in video onto a project's source
// cell.
func (s *Server) handleUploadSource(w http.ResponseWriter, r *http.Request) {
	cellID := r.PathValue("id")
	cell, err := s.db.GetCell(cellID)
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if cell.Kind != store.KindSource {
		writeError(w, http.StatusBadRequest, fmt.Errorf("cell is not a source cell"))
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

	dir, err := projectDir(cell.ProjectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".mp4"
	}
	dest := filepath.Join(dir, "source"+ext)

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	if err := s.db.SetSourceCellVideo(cell.ID, dest, header.Filename); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	c, _ := s.db.GetCell(cell.ID)
	writeJSON(w, http.StatusOK, cellToResponse(c))
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
