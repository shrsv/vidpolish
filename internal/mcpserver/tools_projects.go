package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"vidpolish/internal/binmgr"
	"vidpolish/internal/config"
	"vidpolish/internal/pipeline"
	server "vidpolish/internal/server"
	"vidpolish/internal/store"
	"vidpolish/internal/thumbnail"
	"vidpolish/internal/ytauth"
	"vidpolish/internal/ytmeta"
	"vidpolish/internal/ytupload"
)

// addProjectTools registers the "Projects" notebook model: one project per
// video, holding a source cell plus any number of edit/upload/text cells —
// the same model internal/server's REST API exposes to the web UI, here
// reached by calling internal/store directly instead of over HTTP.
func (s *Server) addProjectTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_list_projects",
		Description: "List every vidpolish project (one per video being worked on), each with its cells.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		projects, err := s.db.ListProjects()
		if err != nil {
			return toolError(err)
		}
		out := make([]*projectResponse, 0, len(projects))
		for _, p := range projects {
			cells, err := s.db.ListCellsByProject(p.ID)
			if err != nil {
				return toolError(err)
			}
			out = append(out, projectToResponse(p, cells))
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_create_project",
		Description: "Create a new vidpolish project (a notebook for one video). Returns the project and its initial source cell.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name string `json:"name" jsonschema:"the project's display name"`
	}) (*mcp.CallToolResult, any, error) {
		p, source, err := s.db.CreateProject(args.Name)
		if err != nil {
			return toolError(err)
		}
		return nil, projectToResponse(p, []*store.Cell{source}), nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_get_project",
		Description: "Get a project and all of its cells.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		ProjectID string `json:"projectId"`
	}) (*mcp.CallToolResult, any, error) {
		p, err := s.db.GetProject(args.ProjectID)
		if err != nil {
			return toolError(err)
		}
		cells, err := s.db.ListCellsByProject(args.ProjectID)
		if err != nil {
			return toolError(err)
		}
		return nil, projectToResponse(p, cells), nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_delete_project",
		Description: "Delete a project and all of its cells.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		ProjectID string `json:"projectId"`
	}) (*mcp.CallToolResult, any, error) {
		if err := s.db.DeleteProject(args.ProjectID); err != nil {
			return toolError(err)
		}
		return nil, map[string]bool{"deleted": true}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name: "vidpolish_set_source",
		Description: "Copy a local video file onto a project's source cell, the video every edit cell in " +
			"that project is run against. Overwrites any video already set on it.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		ProjectID string `json:"projectId"`
		Path      string `json:"path" jsonschema:"local path to the video file to use as the project's source"`
	}) (*mcp.CallToolResult, any, error) {
		cells, err := s.db.ListCellsByProject(args.ProjectID)
		if err != nil {
			return toolError(err)
		}
		var source *store.Cell
		for _, c := range cells {
			if c.Kind == store.KindSource {
				source = c
				break
			}
		}
		if source == nil {
			return toolError(errf("project %s has no source cell", args.ProjectID))
		}
		dir, err := projectDir(source.ProjectID)
		if err != nil {
			return toolError(err)
		}
		ext := filepath.Ext(args.Path)
		if ext == "" {
			ext = ".mp4"
		}
		dest := filepath.Join(dir, "source"+ext)
		if err := copyFile(args.Path, dest); err != nil {
			return toolError(err)
		}
		if err := s.db.SetSourceCellVideo(source.ID, dest, filepath.Base(args.Path)); err != nil {
			return toolError(err)
		}
		c, err := s.db.GetCell(source.ID)
		if err != nil {
			return toolError(err)
		}
		return nil, cellToResponse(c), nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name: "vidpolish_create_cell",
		Description: "Add a cell to a project: an 'edit' cell (parent must be the source cell), an " +
			"'upload' cell (parent must be an edit cell), or a 'text' note cell (parent must be the " +
			"source cell). params is kind-specific: edit takes {margin, speed, width, height, " +
			"bitrateKbps}; upload takes {title, description, tags, privacy, language, thumbnailMode, " +
			"thumbnailPath}; text takes {markdown}.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		ProjectID    string          `json:"projectId"`
		Kind         string          `json:"kind" jsonschema:"'edit', 'upload', or 'text'"`
		ParentCellID string          `json:"parentCellId" jsonschema:"the parent cell's id, seq number, or display name"`
		Name         string          `json:"name,omitempty"`
		Params       json.RawMessage `json:"params,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		if args.Kind != store.KindEdit && args.Kind != store.KindUpload && args.Kind != store.KindText {
			return toolError(errf("kind must be 'edit', 'upload', or 'text'"))
		}
		parent, err := s.resolveCellRef(args.ProjectID, args.ParentCellID)
		if err != nil {
			return toolError(errf("parentCellId: %w", err))
		}
		switch args.Kind {
		case store.KindEdit:
			if parent.Kind != store.KindSource {
				return toolError(errf("an edit cell's parent must be the project's source cell"))
			}
		case store.KindUpload:
			if parent.Kind != store.KindEdit {
				return toolError(errf("an upload cell's parent must be an edit cell"))
			}
		case store.KindText:
			if parent.Kind != store.KindSource {
				return toolError(errf("a text cell's parent must be the project's source cell"))
			}
		}
		params := string(args.Params)
		if params == "" {
			params = "{}"
		}
		seq, err := s.db.NextSeq(args.ProjectID)
		if err != nil {
			return toolError(err)
		}
		cells, err := s.db.ListCellsByProject(args.ProjectID)
		if err != nil {
			return toolError(err)
		}
		var namePtr *string
		if args.Name != "" {
			namePtr = &args.Name
		}
		c, err := s.db.CreateCell(args.ProjectID, seq, parent.ID, namePtr, args.Kind, params, len(cells))
		if err != nil {
			return toolError(err)
		}
		return nil, cellToResponse(c), nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_update_cell",
		Description: "Rename a cell and/or replace its params. Only sensible while the cell is idle.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CellID string          `json:"cellId"`
		Name   *string         `json:"name,omitempty"`
		Params json.RawMessage `json:"params,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		c, err := s.db.GetCell(args.CellID)
		if err != nil {
			return toolError(err)
		}
		if c.Status == store.StatusRunning {
			return toolError(errf("cannot edit a running cell"))
		}
		if args.Name != nil {
			if err := s.db.RenameCell(args.CellID, *args.Name); err != nil {
				return toolError(err)
			}
		}
		if len(args.Params) > 0 {
			if err := s.db.UpdateCellParams(args.CellID, string(args.Params)); err != nil {
				return toolError(err)
			}
		}
		c, err = s.db.GetCell(args.CellID)
		if err != nil {
			return toolError(err)
		}
		return nil, cellToResponse(c), nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_get_cell",
		Description: "Get one cell by id.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CellID string `json:"cellId"`
	}) (*mcp.CallToolResult, any, error) {
		c, err := s.db.GetCell(args.CellID)
		if err != nil {
			return toolError(err)
		}
		return nil, cellToResponse(c), nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_delete_cell",
		Description: "Delete a cell. A project's source cell cannot be deleted.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CellID string `json:"cellId"`
	}) (*mcp.CallToolResult, any, error) {
		c, err := s.db.GetCell(args.CellID)
		if err != nil {
			return toolError(err)
		}
		if c.Kind == store.KindSource {
			return toolError(errf("cannot delete a project's source cell"))
		}
		if err := s.db.DeleteCell(args.CellID); err != nil {
			return toolError(err)
		}
		return nil, map[string]bool{"deleted": true}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_reorder_cells",
		Description: "Reorder a project's edit, upload, or text cells among themselves. seq numbers are untouched.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		ProjectID      string   `json:"projectId"`
		Kind           string   `json:"kind" jsonschema:"'edit', 'upload', or 'text'"`
		OrderedCellIDs []string `json:"orderedCellIds"`
	}) (*mcp.CallToolResult, any, error) {
		if args.Kind != store.KindEdit && args.Kind != store.KindUpload && args.Kind != store.KindText {
			return toolError(errf("kind must be 'edit', 'upload', or 'text'"))
		}
		if err := s.db.ReorderCells(args.ProjectID, args.Kind, args.OrderedCellIDs); err != nil {
			return toolError(err)
		}
		return nil, map[string]bool{"reordered": true}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name: "vidpolish_run_cell",
		Description: "Run an edit or upload cell and block until it finishes. An edit cell processes its " +
			"parent source video through the vidpolish pipeline; an upload cell pushes its parent edit " +
			"cell's output to YouTube (requires vidpolish_youtube_login first). Progress is reported via " +
			"MCP progress notifications if the client requested one. Fails if the cell is already running.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CellID string `json:"cellId"`
	}) (*mcp.CallToolResult, any, error) {
		cell, err := s.db.GetCell(args.CellID)
		if err != nil {
			return toolError(err)
		}
		if cell.Kind == store.KindSource {
			return toolError(errf("the source cell runs implicitly via vidpolish_set_source"))
		}
		if cell.Kind == store.KindText {
			return toolError(errf("text cells don't run"))
		}
		if !s.jobs.start(args.CellID) {
			return toolError(errf("cell is already running"))
		}
		defer s.jobs.finish(args.CellID)

		s.db.UpdateCellStatus(args.CellID, store.StatusRunning, "starting...")
		log := progressLogger(ctx, req)
		log("starting...")

		var runErr error
		switch cell.Kind {
		case store.KindEdit:
			runErr = s.runEditCell(cell, log)
		case store.KindUpload:
			runErr = s.runUploadCell(cell, log)
		}
		if runErr != nil {
			s.db.UpdateCellStatus(args.CellID, store.StatusError, runErr.Error())
			return toolError(runErr)
		}
		if cell.Kind == store.KindUpload {
			s.db.UpdateCellStatus(args.CellID, store.StatusDone, "done")
		}
		c, err := s.db.GetCell(args.CellID)
		if err != nil {
			return toolError(err)
		}
		return nil, cellToResponse(c), nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_cell_info",
		Description: "Get a cell's video properties (resolution, duration, bitrate, size) plus, for an edit cell, its source's properties and a percent-change comparison.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CellID string `json:"cellId"`
	}) (*mcp.CallToolResult, any, error) {
		cell, err := s.db.GetCell(args.CellID)
		if err != nil {
			return toolError(err)
		}
		ffprobePath, err := binmgr.Resolve(binmgr.FFprobe)
		if err != nil {
			return toolError(err)
		}
		var resp struct {
			Self             *pipeline.VideoInfo `json:"self,omitempty"`
			Source           *pipeline.VideoInfo `json:"source,omitempty"`
			ComparedToSource *mediaComparison    `json:"comparedToSource,omitempty"`
		}
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
		return nil, resp, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_export_gif",
		Description: "Convert a cell's output video to an animated GIF and write it to outPath (default: alongside the cell's output, as <name>.gif).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		CellID  string `json:"cellId"`
		FPS     int    `json:"fps,omitempty" jsonschema:"frames per second, 1-30; defaults to 12"`
		Width   int    `json:"width,omitempty" jsonschema:"output width in pixels; 0 keeps the source's original width"`
		OutPath string `json:"outPath,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		cell, err := s.db.GetCell(args.CellID)
		if err != nil {
			return toolError(err)
		}
		if cell.OutputPath == nil || *cell.OutputPath == "" {
			return toolError(errf("cell has no output yet"))
		}
		fps := clamp(args.FPS, 1, 30, 12)
		width := clamp(args.Width, 0, 1920, 0)

		ffmpegPath, err := binmgr.Resolve(binmgr.FFmpeg)
		if err != nil {
			return toolError(err)
		}
		out := args.OutPath
		if out == "" {
			base := *cell.OutputPath
			out = base[:len(base)-len(filepath.Ext(base))] + ".gif"
		}
		if err := pipeline.ExportGIF(ffmpegPath, *cell.OutputPath, out, fps, width); err != nil {
			return toolError(err)
		}
		return nil, map[string]string{"path": out}, nil
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func (s *Server) runEditCell(cell *store.Cell, log func(string)) error {
	var params server.EditParams
	if err := json.Unmarshal([]byte(cell.ParamsJSON), &params); err != nil {
		return errf("invalid edit params: %w", err)
	}
	if params.Speed == 0 {
		params.Speed = 1.0
	}
	source, err := s.db.GetCell(*cell.ParentCellID)
	if err != nil {
		return err
	}
	if source.OutputPath == nil || *source.OutputPath == "" {
		return errf("source cell has no video yet")
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
		Log:         log,
	})
	if err != nil {
		return err
	}
	return s.db.SetCellOutput(cell.ID, out)
}

func (s *Server) runUploadCell(cell *store.Cell, log func(string)) error {
	var params server.UploadParams
	if err := json.Unmarshal([]byte(cell.ParamsJSON), &params); err != nil {
		return errf("invalid upload params: %w", err)
	}
	editCell, err := s.db.GetCell(*cell.ParentCellID)
	if err != nil {
		return err
	}
	if editCell.OutputPath == nil || *editCell.OutputPath == "" {
		return errf("edit cell %s has no output yet; run it first", editCell.DisplayName())
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.YouTube.RefreshToken == "" {
		return errf("not logged in to YouTube; run vidpolish_youtube_login first")
	}

	title := params.Title
	if title == "" {
		title = filepath.Base(*editCell.OutputPath)
		title = title[:len(title)-len(filepath.Ext(title))]
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
		description = ytmeta.BuildDescription(cfg.YouTube.DescriptionTemplate, title, log)
	}

	thumbnailPath := ""
	switch params.ThumbnailMode {
	case "custom":
		thumbnailPath = params.ThumbnailPath
	case "none":
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
				return errf("generating thumbnail: %w", err)
			}
			log("==> generated thumbnail preview")
		}
	}

	accessToken, err := ytauth.AccessToken(cfg.YouTube.ClientID, cfg.YouTube.ClientSecret, cfg.YouTube.RefreshToken)
	if err != nil {
		return errf("refreshing YouTube access token: %w", err)
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
		Log:           log,
		Progress: func(fraction float64, eta time.Duration) {
			log(formatETA(fraction, eta))
		},
		OnUploaded: func(result *ytupload.Result) {
			s.db.SetCellYouTubeURL(cell.ID, result.URL)
			log("uploaded: " + result.URL)
		},
	})
	return err
}
