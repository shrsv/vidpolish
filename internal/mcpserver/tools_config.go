package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"vidpolish/internal/binmgr"
	"vidpolish/internal/cache"
	"vidpolish/internal/config"
	"vidpolish/internal/store"
)

// configResponse mirrors internal/server's redacted config shape: the
// YouTube client secret and refresh token are never returned verbatim,
// since a tool result is read by an LLM, not rendered in a trusted UI.
type configResponse struct {
	YouTube struct {
		ClientID            string      `json:"clientId"`
		ClientSecret        secretField `json:"clientSecret"`
		RefreshToken        secretField `json:"refreshToken"`
		Privacy             string      `json:"privacy"`
		DefaultLanguage     string      `json:"defaultLanguage"`
		DefaultTags         []string    `json:"defaultTags"`
		DescriptionTemplate string      `json:"descriptionTemplate"`
	} `json:"youtube"`
	Thumbnail struct {
		Enabled         bool   `json:"enabled"`
		LogoPath        string `json:"logoPath"`
		BackgroundColor string `json:"backgroundColor"`
		AccentColor     string `json:"accentColor"`
		TextColor       string `json:"textColor"`
	} `json:"thumbnail"`
}

func toConfigResponse(cfg *config.Config) *configResponse {
	var out configResponse
	out.YouTube.ClientID = cfg.YouTube.ClientID
	out.YouTube.ClientSecret = redactSecret(cfg.YouTube.ClientSecret)
	out.YouTube.RefreshToken = redactSecret(cfg.YouTube.RefreshToken)
	out.YouTube.Privacy = cfg.YouTube.Privacy
	out.YouTube.DefaultLanguage = cfg.YouTube.DefaultLanguage
	out.YouTube.DefaultTags = cfg.YouTube.DefaultTags
	out.YouTube.DescriptionTemplate = cfg.YouTube.DescriptionTemplate
	out.Thumbnail.Enabled = cfg.Thumbnail.Enabled
	out.Thumbnail.LogoPath = cfg.Thumbnail.LogoPath
	out.Thumbnail.BackgroundColor = cfg.Thumbnail.BackgroundColor
	out.Thumbnail.AccentColor = cfg.Thumbnail.AccentColor
	out.Thumbnail.TextColor = cfg.Thumbnail.TextColor
	return &out
}

func loadOrInitConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err == nil {
		return cfg, nil
	}
	if _, initErr := config.Init(); initErr != nil {
		return nil, initErr
	}
	return config.Load()
}

type cacheEntryResponse struct {
	Fingerprint string `json:"fingerprint"`
	SizeBytes   int64  `json:"sizeBytes"`
	CachedAt    int64  `json:"cachedAt,omitempty"`
	ProjectID   string `json:"projectId,omitempty"`
	ProjectName string `json:"projectName,omitempty"`
}

// fingerprintToProject best-effort maps each cache fingerprint to the
// project whose source video produced it, the same way internal/server
// does for the web UI's cache panel.
func (s *Server) fingerprintToProject() map[string]*store.Project {
	out := make(map[string]*store.Project)
	projects, err := s.db.ListProjects()
	if err != nil {
		return out
	}
	for _, p := range projects {
		cells, err := s.db.ListCellsByProject(p.ID)
		if err != nil {
			continue
		}
		for _, c := range cells {
			if c.Kind != store.KindSource || c.OutputPath == nil || *c.OutputPath == "" {
				continue
			}
			if fp, err := cache.Fingerprint(*c.OutputPath); err == nil {
				out[fp] = p
			}
		}
	}
	return out
}

func (s *Server) addConfigTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_get_config",
		Description: "Get vidpolish's YouTube upload and thumbnail config. Secrets are redacted to a set/unset flag plus a short preview.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		cfg, err := loadOrInitConfig()
		if err != nil {
			return toolError(err)
		}
		return nil, toConfigResponse(cfg), nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name: "vidpolish_set_config",
		Description: "Update vidpolish's YouTube upload and/or thumbnail config. Only fields you " +
			"provide are changed; omit a field to leave it as-is.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		YouTube *struct {
			ClientID            *string   `json:"clientId,omitempty"`
			ClientSecret        *string   `json:"clientSecret,omitempty"`
			Privacy             *string   `json:"privacy,omitempty" jsonschema:"'public', 'unlisted', or 'private'"`
			DefaultLanguage     *string   `json:"defaultLanguage,omitempty"`
			DefaultTags         *[]string `json:"defaultTags,omitempty"`
			DescriptionTemplate *string   `json:"descriptionTemplate,omitempty"`
		} `json:"youtube,omitempty"`
		Thumbnail *struct {
			Enabled         *bool   `json:"enabled,omitempty"`
			LogoPath        *string `json:"logoPath,omitempty"`
			BackgroundColor *string `json:"backgroundColor,omitempty"`
			AccentColor     *string `json:"accentColor,omitempty"`
			TextColor       *string `json:"textColor,omitempty"`
		} `json:"thumbnail,omitempty"`
	}) (*mcp.CallToolResult, any, error) {
		cfg, err := loadOrInitConfig()
		if err != nil {
			return toolError(err)
		}
		if yt := args.YouTube; yt != nil {
			if yt.ClientID != nil {
				cfg.YouTube.ClientID = *yt.ClientID
			}
			if yt.ClientSecret != nil {
				cfg.YouTube.ClientSecret = *yt.ClientSecret
			}
			if yt.Privacy != nil {
				cfg.YouTube.Privacy = *yt.Privacy
			}
			if yt.DefaultLanguage != nil {
				cfg.YouTube.DefaultLanguage = *yt.DefaultLanguage
			}
			if yt.DefaultTags != nil {
				cfg.YouTube.DefaultTags = *yt.DefaultTags
			}
			if yt.DescriptionTemplate != nil {
				cfg.YouTube.DescriptionTemplate = *yt.DescriptionTemplate
			}
		}
		if th := args.Thumbnail; th != nil {
			if th.Enabled != nil {
				cfg.Thumbnail.Enabled = *th.Enabled
			}
			if th.LogoPath != nil {
				cfg.Thumbnail.LogoPath = *th.LogoPath
			}
			if th.BackgroundColor != nil {
				cfg.Thumbnail.BackgroundColor = *th.BackgroundColor
			}
			if th.AccentColor != nil {
				cfg.Thumbnail.AccentColor = *th.AccentColor
			}
			if th.TextColor != nil {
				cfg.Thumbnail.TextColor = *th.TextColor
			}
		}
		if err := config.Validate(cfg); err != nil {
			return toolError(err)
		}
		if err := config.Save(cfg); err != nil {
			return toolError(err)
		}
		return nil, toConfigResponse(cfg), nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_list_config_backups",
		Description: "List config.toml backups, taken automatically on every save, most recent first.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		backups, err := config.ListBackups()
		if err != nil {
			return toolError(err)
		}
		out := make([]map[string]string, 0, len(backups))
		for _, b := range backups {
			out = append(out, map[string]string{"timestamp": strconv.FormatInt(b.Timestamp, 10)})
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_restore_config_backup",
		Description: "Restore config.toml from a prior backup timestamp (from vidpolish_list_config_backups). The restore itself is backed up too, so it can be undone.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Timestamp string `json:"timestamp"`
	}) (*mcp.CallToolResult, any, error) {
		ts, err := strconv.ParseInt(args.Timestamp, 10, 64)
		if err != nil {
			return toolError(errf("invalid timestamp"))
		}
		if err := config.RestoreBackup(ts); err != nil {
			return toolError(err)
		}
		cfg, err := config.Load()
		if err != nil {
			return toolError(err)
		}
		return nil, toConfigResponse(cfg), nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_list_tools",
		Description: "Check resolved paths for vidpolish's external tool dependencies (ffmpeg, ffprobe, deep-filter, auto-editor, resvg, font), same as `vidpolish deps`.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		return nil, resolveToolStatuses(), nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_resolve_tool",
		Description: "Resolve (downloading if needed) the path to one tool dependency by name: ffmpeg, ffprobe, deep-filter, auto-editor, resvg, or font.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Name string `json:"name"`
	}) (*mcp.CallToolResult, any, error) {
		if args.Name == "font" {
			regular, bold, err := binmgr.ResolveFont()
			if err != nil {
				return toolError(err)
			}
			return nil, toolStatus{Name: "font", Path: regular + ", " + bold}, nil
		}
		for _, t := range allTools {
			if string(t) == args.Name {
				path, err := binmgr.Resolve(t)
				if err != nil {
					return toolError(err)
				}
				return nil, toolStatus{Name: args.Name, Path: path}, nil
			}
		}
		return toolError(errf("unknown tool %q", args.Name))
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_list_cache",
		Description: "List cached pipeline intermediate artifacts (one entry per source video fingerprint), with size and the project they belong to, if any.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		root, err := cache.Root()
		if err != nil {
			return toolError(err)
		}
		entries, err := cache.ListEntries(root)
		if err != nil {
			return toolError(err)
		}
		byFingerprint := s.fingerprintToProject()
		out := make([]cacheEntryResponse, 0, len(entries))
		for _, e := range entries {
			r := cacheEntryResponse{Fingerprint: e.Fingerprint, SizeBytes: e.SizeBytes}
			if !e.CachedAt.IsZero() {
				r.CachedAt = e.CachedAt.Unix()
			}
			if p, ok := byFingerprint[e.Fingerprint]; ok {
				r.ProjectID = p.ID
				r.ProjectName = p.Name
			}
			out = append(out, r)
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_delete_cache_entry",
		Description: "Delete one cache entry by fingerprint. Never deletes the project itself, only cached intermediate artifacts.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct {
		Fingerprint string `json:"fingerprint"`
	}) (*mcp.CallToolResult, any, error) {
		root, err := cache.Root()
		if err != nil {
			return toolError(err)
		}
		dir, err := cache.Dir(args.Fingerprint)
		if err != nil {
			return toolError(err)
		}
		if filepath.Dir(dir) != root || args.Fingerprint == "" {
			return toolError(errf("invalid fingerprint"))
		}
		if err := os.RemoveAll(dir); err != nil {
			return toolError(err)
		}
		return nil, map[string]bool{"deleted": true}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "vidpolish_clean_cache",
		Description: "Sweep and remove stale cache entries past their TTL.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		root, err := cache.Root()
		if err != nil {
			return toolError(err)
		}
		if err := cache.Sweep(root, 0); err != nil {
			return toolError(err)
		}
		return nil, map[string]bool{"cleaned": true}, nil
	})
}
