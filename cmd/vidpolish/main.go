// Command vidpolish is a CLI that turns a raw screen recording into a
// polished video: split audio/video, denoise the audio with DeepFilterNet,
// and cut silence with auto-editor.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"vidpolish/internal/binmgr"
	"vidpolish/internal/browseropen"
	"vidpolish/internal/cache"
	"vidpolish/internal/config"
	"vidpolish/internal/mcpserver"
	"vidpolish/internal/pipeline"
	"vidpolish/internal/server"
	"vidpolish/internal/store"
	"vidpolish/internal/thumbnail"
	"vidpolish/internal/ytauth"
	"vidpolish/internal/ytmeta"
	"vidpolish/internal/ytupload"
)

// version is the vidpolish release version. It defaults to "dev" for
// locally built binaries (plain `go build` / `make build`); release builds
// override it via `-ldflags "-X main.version=X.Y.Z"` (see
// scripts/release-build.sh).
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "process":
		runProcess(os.Args[2:])
	case "deps":
		runDeps()
	case "cache":
		runCache(os.Args[2:])
	case "config":
		runConfig(os.Args[2:])
	case "youtube":
		runYouTube(os.Args[2:])
	case "upload":
		runUpload(os.Args[2:])
	case "ui":
		runUI(os.Args[2:])
	case "mcp":
		runMCP(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("vidpolish", version)
	default:
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `vidpolish - split, denoise, and auto-cut raw recordings

Usage:
  vidpolish process <input.mp4> [flags]
  vidpolish deps
  vidpolish cache clean
  vidpolish config init
  vidpolish youtube login
  vidpolish upload <video.mp4> [flags]
  vidpolish ui [--port 7890] [--no-open]
  vidpolish mcp
  vidpolish version`)
}

// runMCP serves vidpolish's pipeline, project/cell notebook, YouTube
// upload, and config/cache/tool-status surfaces as an MCP server over
// stdio, so an MCP client (e.g. Claude Code/Desktop) can drive vidpolish
// directly. See internal/mcpserver for the tool implementations.
func runMCP(_ []string) {
	db, err := store.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := mcpserver.Run(ctx, db, version); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runUI(args []string) {
	fs := flag.NewFlagSet("ui", flag.ExitOnError)
	port := fs.Int("port", 7890, "port to serve the local UI on")
	noOpen := fs.Bool("no-open", false, "don't open a browser automatically")
	fs.Parse(args)

	db, err := store.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer db.Close()

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	srv := &http.Server{Addr: addr, Handler: server.New(db).Handler()}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	url := "http://" + addr
	fmt.Println("==> vidpolish UI listening on", url)
	if !*noOpen {
		if err := browseropen.Open(url); err != nil {
			fmt.Println("==> could not open a browser automatically; open the URL above manually")
		}
	}

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func runProcess(args []string) {
	fs := flag.NewFlagSet("process", flag.ExitOnError)
	outputDir := fs.String("output-dir", "output", "directory to write the final polished video to")
	margin := fs.String("margin", "0.2s", "auto-editor margin around kept speech (e.g. 0.2s, 0.3s)")
	speed := fs.Float64("speed", 1.0, "playback speed multiplier for kept/spoken segments (e.g. 1.25, 1.5, 1.75)")
	width := fs.Int("width", 0, "resize output to this width in pixels (default: keep original); pair with --height or omit it to preserve aspect ratio")
	height := fs.Int("height", 0, "resize output to this height in pixels (default: keep original); pair with --width or omit it to preserve aspect ratio")
	bitrate := fs.Int("bitrate", 0, "target video bitrate in kbps (default: keep original quality, no re-encode for bitrate)")
	noCache := fs.Bool("no-cache", false, "ignore cached intermediate artifacts and recompute everything")
	upload := fs.Bool("upload", false, "upload the polished result to YouTube after processing")
	title := fs.String("title", "", "YouTube title if --upload is set (default: input filename)")
	privacy := fs.String("privacy", "", "YouTube privacy if --upload is set: public, unlisted, or private (default: from config)")
	language := fs.String("language", "", "YouTube video language if --upload is set (default: from config)")
	noWait := fs.Bool("no-wait", false, "if --upload is set, skip waiting for YouTube processing status")
	thumbnailPath := fs.String("thumbnail", "", "if --upload is set, use this image as the thumbnail instead of auto-generating one")
	noThumbnail := fs.Bool("no-thumbnail", false, "if --upload is set, don't set a thumbnail even if auto-generation is enabled in config")
	// flag.Parse stops at the first non-flag argument, so reorder to let the
	// input path appear anywhere on the command line (before or after flags).
	fs.Parse(reorderFlags(args))

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: vidpolish process <input.mp4> [flags]")
		os.Exit(1)
	}
	if *speed < 0.5 || *speed > 4.0 {
		fmt.Fprintln(os.Stderr, "error: --speed must be between 0.5 and 4.0")
		os.Exit(1)
	}
	if *width < 0 || *height < 0 || *bitrate < 0 {
		fmt.Fprintln(os.Stderr, "error: --width, --height, and --bitrate must not be negative")
		os.Exit(1)
	}

	out, err := pipeline.Process(pipeline.Options{
		Input:       fs.Arg(0),
		OutputDir:   *outputDir,
		Margin:      *margin,
		Speed:       *speed,
		Width:       *width,
		Height:      *height,
		BitrateKbps: *bitrate,
		Denoise:     true,
		NoCache:     *noCache,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println("done:", out)

	if *upload {
		doUpload(out, uploadFlags{
			Title:         *title,
			Privacy:       *privacy,
			Language:      *language,
			NoWait:        *noWait,
			ThumbnailPath: *thumbnailPath,
			NoThumbnail:   *noThumbnail,
		})
	}
}

func runConfig(args []string) {
	if len(args) != 1 || args[0] != "init" {
		fmt.Fprintln(os.Stderr, "usage: vidpolish config init")
		os.Exit(1)
	}
	path, err := config.Init()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println("config ready at", path)
	fmt.Println("fill in [youtube] client_id / client_secret, then run: vidpolish youtube login")
}

func runYouTube(args []string) {
	if len(args) != 1 || args[0] != "login" {
		fmt.Fprintln(os.Stderr, "usage: vidpolish youtube login")
		os.Exit(1)
	}
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if cfg.YouTube.ClientID == "" || cfg.YouTube.ClientSecret == "" {
		fmt.Fprintln(os.Stderr, "error: set client_id and client_secret in the config first (see: vidpolish config init)")
		os.Exit(1)
	}

	refreshToken, err := ytauth.Login(cfg.YouTube.ClientID, cfg.YouTube.ClientSecret)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	cfg.YouTube.RefreshToken = refreshToken
	if err := config.Save(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "error saving config:", err)
		os.Exit(1)
	}
	fmt.Println("logged in, refresh token saved")
}

func runUpload(args []string) {
	fs := flag.NewFlagSet("upload", flag.ExitOnError)
	title := fs.String("title", "", "YouTube title (default: input filename)")
	privacy := fs.String("privacy", "", "YouTube privacy: public, unlisted, or private (default: from config)")
	language := fs.String("language", "", "YouTube video language, e.g. en, en-IN (default: from config)")
	noWait := fs.Bool("no-wait", false, "skip waiting for YouTube processing status after upload")
	thumbnailPath := fs.String("thumbnail", "", "use this image as the thumbnail instead of auto-generating one")
	noThumbnail := fs.Bool("no-thumbnail", false, "don't set a thumbnail even if auto-generation is enabled in config")
	fs.Parse(reorderFlags(args))

	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: vidpolish upload <video.mp4> [flags]")
		os.Exit(1)
	}
	doUpload(fs.Arg(0), uploadFlags{
		Title:         *title,
		Privacy:       *privacy,
		Language:      *language,
		NoWait:        *noWait,
		ThumbnailPath: *thumbnailPath,
		NoThumbnail:   *noThumbnail,
	})
}

// uploadFlags carries the upload-related CLI flags shared by "process
// --upload" and "upload" so both funnel through the same doUpload logic.
type uploadFlags struct {
	Title         string
	Privacy       string
	Language      string
	NoWait        bool
	ThumbnailPath string
	NoThumbnail   bool
}

func doUpload(path string, f uploadFlags) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if cfg.YouTube.RefreshToken == "" {
		fmt.Fprintln(os.Stderr, "error: not logged in, run: vidpolish youtube login")
		os.Exit(1)
	}

	title := f.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	privacy := f.Privacy
	if privacy == "" {
		privacy = cfg.YouTube.Privacy
	}
	if privacy != "public" && privacy != "unlisted" && privacy != "private" {
		fmt.Fprintln(os.Stderr, "error: --privacy must be public, unlisted, or private")
		os.Exit(1)
	}
	language := f.Language
	if language == "" {
		language = cfg.YouTube.DefaultLanguage
	}

	tags := ytmeta.MergeTags(cfg.YouTube.DefaultTags)
	description := ytmeta.BuildDescription(cfg.YouTube.DescriptionTemplate, title, func(msg string) {
		fmt.Fprintln(os.Stderr, "warning:", msg)
	})

	thumbnailPath := f.ThumbnailPath
	if thumbnailPath == "" && !f.NoThumbnail && cfg.Thumbnail.Enabled {
		thumbnailPath, err = thumbnail.Generate(thumbnail.Options{
			Title:           title,
			LogoPath:        cfg.Thumbnail.LogoPath,
			BackgroundColor: cfg.Thumbnail.BackgroundColor,
			AccentColor:     cfg.Thumbnail.AccentColor,
			TextColor:       cfg.Thumbnail.TextColor,
			OutPath:         strings.TrimSuffix(path, filepath.Ext(path)) + "-thumbnail.png",
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "error generating thumbnail:", err)
			os.Exit(1)
		}
		fmt.Println("==> generated thumbnail:", thumbnailPath)
	}

	accessToken, err := ytauth.AccessToken(cfg.YouTube.ClientID, cfg.YouTube.ClientSecret, cfg.YouTube.RefreshToken)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error refreshing access token:", err)
		os.Exit(1)
	}

	fmt.Println("==> uploading", path, "as", privacy)
	result, err := ytupload.Upload(path, ytupload.Options{
		AccessToken:   accessToken,
		Title:         title,
		Description:   description,
		Tags:          tags,
		Privacy:       privacy,
		Language:      language,
		ThumbnailPath: thumbnailPath,
		NoWait:        f.NoWait,
		Progress: func(fraction float64, eta time.Duration) {
			fmt.Printf("\ruploading: %.0f%% (ETA %s)   ", fraction*100, formatETA(eta))
		},
		OnUploaded: func(result *ytupload.Result) {
			// The link is live and shareable as soon as the upload itself
			// finishes; it does not need to wait for YouTube's processing
			// to complete, so report it immediately.
			fmt.Println()
			fmt.Println("uploaded:", result.URL)
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if !f.NoWait {
		fmt.Println("==> done, video is fully processed:", result.URL)
	}
}

func formatETA(d time.Duration) string {
	if d <= 0 {
		return "..."
	}
	d = d.Round(time.Second)
	return d.String()
}

// formatElapsed renders a completed duration, always as a concrete value
// (unlike formatETA, which uses "..." for a not-yet-estimable remaining
// time) — sub-second work shows as "<1s" rather than "0s".
func formatElapsed(d time.Duration) string {
	d = d.Round(time.Second)
	if d <= 0 {
		return "<1s"
	}
	return d.String()
}

func runCache(args []string) {
	if len(args) != 1 || args[0] != "clean" {
		fmt.Fprintln(os.Stderr, "usage: vidpolish cache clean")
		os.Exit(1)
	}
	root, err := cache.Root()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if err := cache.Sweep(root, 0); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println("cleaned", root)
}

// reorderFlags moves the positional argument to the front (before any
// flags), since Go's flag package otherwise stops parsing at the first
// non-flag token.
func reorderFlags(args []string) []string {
	boolFlags := map[string]bool{
		"-no-cache": true, "--no-cache": true,
		"-upload": true, "--upload": true,
		"-no-wait": true, "--no-wait": true,
		"-no-thumbnail": true, "--no-thumbnail": true,
	}
	valueFlags := map[string]bool{
		"-output-dir": true, "--output-dir": true,
		"-margin": true, "--margin": true,
		"-speed": true, "--speed": true,
		"-width": true, "--width": true,
		"-height": true, "--height": true,
		"-bitrate": true, "--bitrate": true,
		"-title": true, "--title": true,
		"-privacy": true, "--privacy": true,
		"-language": true, "--language": true,
		"-thumbnail": true, "--thumbnail": true,
	}

	var flags, positional []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case boolFlags[a]:
			flags = append(flags, a)
		case valueFlags[a]:
			flags = append(flags, a)
			if i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		case len(a) > 0 && a[0] == '-':
			flags = append(flags, a)
		default:
			positional = append(positional, a)
		}
	}
	return append(flags, positional...)
}

func runDeps() {
	tools := []binmgr.Tool{binmgr.FFmpeg, binmgr.FFprobe, binmgr.DeepFilter, binmgr.AutoEditor, binmgr.Resvg}
	total := len(tools) + 1 // +1 for the font, checked separately below
	start := time.Now()
	failed := 0

	fmt.Printf("==> Checking %d dependencies (this only downloads what's missing)\n", total)

	for i, tool := range tools {
		fmt.Printf("[%d/%d] %s...\n", i+1, total, tool)
		toolStart := time.Now()
		path, err := binmgr.Resolve(tool)
		if err != nil {
			failed++
			fmt.Fprintf(os.Stderr, "[%d/%d] %-12s ERROR: %v\n", i+1, total, tool, err)
			continue
		}
		fmt.Printf("[%d/%d] %-12s ready (%s) in %s\n", i+1, total, tool, path, formatElapsed(time.Since(toolStart)))
	}

	fmt.Printf("[%d/%d] font...\n", total, total)
	fontStart := time.Now()
	if regular, bold, err := binmgr.ResolveFont(); err != nil {
		failed++
		fmt.Fprintf(os.Stderr, "[%d/%d] %-12s ERROR: %v\n", total, total, "font", err)
	} else {
		fmt.Printf("[%d/%d] %-12s ready (%s, %s) in %s\n", total, total, "font", regular, bold, formatElapsed(time.Since(fontStart)))
	}

	elapsed := time.Since(start).Round(time.Second)
	if failed > 0 {
		fmt.Printf("==> done in %s, but %d/%d dependencies failed (see ERROR lines above)\n", elapsed, failed, total)
		os.Exit(1)
	}
	fmt.Printf("==> all dependencies ready in %s\n", elapsed)
}
