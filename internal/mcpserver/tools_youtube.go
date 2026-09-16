package mcpserver

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"vidpolish/internal/config"
	"vidpolish/internal/thumbnail"
	"vidpolish/internal/ytauth"
	"vidpolish/internal/ytmeta"
	"vidpolish/internal/ytupload"
)

// loginState tracks the one in-flight (or last-completed) YouTube OAuth
// login, since the flow is started by one tool call (which must return
// immediately with the consent URL for a human to open) and finishes in
// the background once that human completes it in their browser.
type loginState struct {
	mu     sync.Mutex
	status string // "none" | "pending" | "done" | "error"
	err    string
}

func (l *loginState) set(status, err string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.status, l.err = status, err
}

func (l *loginState) get() (string, string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.status, l.err
}

var ytLogin = &loginState{status: "none"}

func (s *Server) addYouTubeTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name: "vidpolish_youtube_login",
		Description: "Start YouTube OAuth login (requires client_id/client_secret already set via " +
			"vidpolish_set_config). Returns a consent URL that a human must open in a browser to " +
			"approve; the flow completes in the background. Poll vidpolish_youtube_login_status for " +
			"the result, then uploads will work.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		cfg, err := config.Load()
		if err != nil {
			return toolError(err)
		}
		if cfg.YouTube.ClientID == "" || cfg.YouTube.ClientSecret == "" {
			return toolError(errf("set client_id and client_secret first, via vidpolish_set_config"))
		}
		authURL, wait, err := ytauth.StartLogin(cfg.YouTube.ClientID, cfg.YouTube.ClientSecret)
		if err != nil {
			return toolError(err)
		}
		ytLogin.set("pending", "")
		go func() {
			refreshToken, err := wait()
			if err != nil {
				ytLogin.set("error", err.Error())
				return
			}
			cfg, loadErr := config.Load()
			if loadErr != nil {
				ytLogin.set("error", loadErr.Error())
				return
			}
			cfg.YouTube.RefreshToken = refreshToken
			if saveErr := config.Save(cfg); saveErr != nil {
				ytLogin.set("error", saveErr.Error())
				return
			}
			ytLogin.set("done", "")
		}()
		return nil, map[string]string{"authUrl": authURL}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_youtube_login_status",
		Description: "Check the status of a YouTube OAuth login started with vidpolish_youtube_login: 'none', 'pending', 'done', or 'error'.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		status, errMsg := ytLogin.get()
		out := map[string]string{"status": status}
		if errMsg != "" {
			out["error"] = errMsg
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name: "vidpolish_upload_youtube",
		Description: "Upload a video file straight to YouTube, without going through a project's " +
			"upload cell (mirrors `vidpolish upload`). Requires vidpolish_youtube_login to have " +
			"completed first. Progress is reported via MCP progress notifications if the client " +
			"requested one.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Path          string   `json:"path" jsonschema:"local path to the video file to upload"`
		Title         string   `json:"title,omitempty" jsonschema:"defaults to the input filename"`
		Description   string   `json:"description,omitempty" jsonschema:"defaults to config's description template"`
		Tags          []string `json:"tags,omitempty"`
		Privacy       string   `json:"privacy,omitempty" jsonschema:"'public', 'unlisted', or 'private'; defaults to config"`
		Language      string   `json:"language,omitempty" jsonschema:"BCP-47 language code, e.g. 'en'; defaults to config"`
		ThumbnailPath string   `json:"thumbnailPath,omitempty" jsonschema:"use this image instead of auto-generating a thumbnail"`
		NoThumbnail   bool     `json:"noThumbnail,omitempty" jsonschema:"skip thumbnail generation even if enabled in config"`
		NoWait        bool     `json:"noWait,omitempty" jsonschema:"skip waiting for YouTube processing status after upload"`
	}) (*mcp.CallToolResult, any, error) {
		cfg, err := config.Load()
		if err != nil {
			return toolError(err)
		}
		if cfg.YouTube.RefreshToken == "" {
			return toolError(errf("not logged in; run vidpolish_youtube_login first"))
		}

		title := args.Title
		if title == "" {
			title = filepath.Base(args.Path)
			title = title[:len(title)-len(filepath.Ext(title))]
		}
		privacy := args.Privacy
		if privacy == "" {
			privacy = cfg.YouTube.Privacy
		}
		if privacy != "public" && privacy != "unlisted" && privacy != "private" {
			return toolError(errf("privacy must be 'public', 'unlisted', or 'private'"))
		}
		language := args.Language
		if language == "" {
			language = cfg.YouTube.DefaultLanguage
		}
		log := progressLogger(ctx, req)
		tags := ytmeta.MergeTags(append(args.Tags, cfg.YouTube.DefaultTags...))
		description := args.Description
		if description == "" {
			description = ytmeta.BuildDescription(cfg.YouTube.DescriptionTemplate, title, log)
		}

		thumbnailPath := args.ThumbnailPath
		if thumbnailPath == "" && !args.NoThumbnail && cfg.Thumbnail.Enabled {
			thumbnailPath, err = thumbnail.Generate(thumbnail.Options{
				Title:           title,
				LogoPath:        cfg.Thumbnail.LogoPath,
				BackgroundColor: cfg.Thumbnail.BackgroundColor,
				AccentColor:     cfg.Thumbnail.AccentColor,
				TextColor:       cfg.Thumbnail.TextColor,
				OutPath:         args.Path[:len(args.Path)-len(filepath.Ext(args.Path))] + "-thumbnail.png",
			})
			if err != nil {
				return toolError(errf("generating thumbnail: %w", err))
			}
			log("==> generated thumbnail: " + thumbnailPath)
		}

		accessToken, err := ytauth.AccessToken(cfg.YouTube.ClientID, cfg.YouTube.ClientSecret, cfg.YouTube.RefreshToken)
		if err != nil {
			return toolError(errf("refreshing access token: %w", err))
		}

		result, err := ytupload.Upload(args.Path, ytupload.Options{
			AccessToken:   accessToken,
			Title:         title,
			Description:   description,
			Tags:          tags,
			Privacy:       privacy,
			Language:      language,
			ThumbnailPath: thumbnailPath,
			NoWait:        args.NoWait,
			Log:           log,
			Progress: func(fraction float64, eta time.Duration) {
				log(formatETA(fraction, eta))
			},
		})
		if err != nil {
			return toolError(err)
		}
		return nil, result, nil
	})
}
