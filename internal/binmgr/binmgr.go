// Package binmgr resolves the external tool binaries vidpolish shells out to
// (ffmpeg/ffprobe, deep-filter, auto-editor), downloading pinned per-platform
// releases into a local cache when they aren't already available.
package binmgr

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

// httpClient bounds the total time a download may take, so a stalled
// connection fails with a clear error instead of hanging silently forever.
var httpClient = &http.Client{Timeout: 10 * time.Minute}

const (
	deepFilterVersion = "0.5.6"
	autoEditorVersion = "31.6.0"
	resvgVersion      = "0.48.1"
	// resvgWindowsVersion is pinned separately: linebender/resvg dropped
	// prebuilt Windows binaries starting v0.48.0, so this is the last
	// release that still published resvg-win64.zip.
	resvgWindowsVersion = "0.47.0"
	interVersion        = "4.1"
	// ffmpegCacheTag namespaces the Windows ffmpeg cache dir. Not a real
	// pinned version - see ffmpegWindowsAsset for why - just a fixed label
	// so re-running "vidpolish deps" reuses the same cache dir.
	ffmpegCacheTag = "release-essentials"
)

// Progress optionally reports live download activity for a Resolve*/
// WithProgress call, in addition to the fixed stderr output every Resolve
// call has always produced (log always still goes to stderr regardless,
// so plain CLI usage is unaffected). Either field may be nil. Used by
// internal/server to stream per-tool progress/logs to the local UI over
// SSE instead of leaving a request looking stuck during a big download.
type Progress struct {
	// OnLog receives one-line status messages ("downloading X from Y",
	// "installing X", etc).
	OnLog func(string)
	// OnProgress receives periodic byte-count updates while a download is
	// in flight. total is 0 if the server didn't report Content-Length.
	OnProgress func(written, total int64)
}

func (p *Progress) log(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintln(os.Stderr, msg)
	if p != nil && p.OnLog != nil {
		p.OnLog(msg)
	}
}

func (p *Progress) progress(written, total int64) {
	if p != nil && p.OnProgress != nil {
		p.OnProgress(written, total)
	}
}

// Tool identifies an external binary vidpolish depends on.
type Tool string

const (
	FFmpeg     Tool = "ffmpeg"
	FFprobe    Tool = "ffprobe"
	DeepFilter Tool = "deep-filter"
	AutoEditor Tool = "auto-editor"
	Resvg      Tool = "resvg"
)

// Resolve returns the absolute path to the requested tool's binary,
// downloading and caching it first if necessary. ffmpeg/ffprobe are expected
// to already be installed on the system on Linux/macOS (they're
// near-universally preinstalled there); on Windows, where that's not a safe
// assumption, they're fetched automatically (see resolveFFmpegWindows).
// deep-filter, auto-editor, and resvg are fetched from their GitHub
// releases into ~/.cache/vidpolish/bin on every platform.
func Resolve(tool Tool) (string, error) {
	return ResolveWithProgress(tool, nil)
}

// ResolveWithProgress is Resolve, additionally reporting live progress/log
// activity through prog (which may be nil - equivalent to Resolve).
func ResolveWithProgress(tool Tool, prog *Progress) (string, error) {
	switch tool {
	case FFmpeg, FFprobe:
		if path, err := exec.LookPath(string(tool)); err == nil {
			return path, nil
		}
		if runtime.GOOS == "windows" {
			return resolveFFmpegWindows(tool, prog)
		}
		return "", fmt.Errorf("%s not found on PATH: install ffmpeg (e.g. `apt install ffmpeg`) and retry", tool)
	case DeepFilter:
		return resolveDownloaded(tool, deepFilterVersion, deepFilterAsset, deepFilterBinName, prog)
	case AutoEditor:
		return resolveDownloaded(tool, autoEditorVersion, autoEditorAsset, autoEditorBinName, prog)
	case Resvg:
		// resvg publishes binaries for linux/amd64, darwin, and (up to
		// v0.47.0 only) windows/amd64; on other platforms (linux/arm64)
		// fall back to whatever the user has installed themselves (e.g.
		// `cargo install resvg` or a package manager).
		if path, err := exec.LookPath("resvg"); err == nil {
			return path, nil
		}
		path, err := resolveDownloaded(tool, resvgResolveVersion(), resvgAsset, resvgBinName, prog)
		if err != nil {
			return "", fmt.Errorf("resvg has no prebuilt binary for %s/%s: install it yourself (e.g. `cargo install resvg`) and ensure it's on PATH: %w", runtime.GOOS, runtime.GOARCH, err)
		}
		return path, nil
	default:
		return "", fmt.Errorf("unknown tool %q", tool)
	}
}

// Status reports whether tool is already available (on PATH or already
// downloaded into the cache) without downloading or installing anything -
// safe to call from a status/health-check endpoint that must return
// quickly. Call Resolve instead to actually fetch a missing tool.
//
// managed reports whether vidpolish itself is the one managing that
// binary (downloaded into its own cache) as opposed to a system install
// found on PATH - only a managed tool can be usefully redownloaded (see
// Redownload); forcing a "redownload" of the system's own ffmpeg install,
// for instance, wouldn't do anything.
func Status(tool Tool) (path string, available bool, managed bool) {
	switch tool {
	case FFmpeg, FFprobe:
		if p, err := exec.LookPath(string(tool)); err == nil {
			return p, true, false
		}
		if runtime.GOOS != "windows" {
			return "", false, false
		}
		dir, err := cacheDir(FFmpeg, ffmpegCacheTag)
		if err != nil {
			return "", false, true
		}
		name := "ffmpeg.exe"
		if tool == FFprobe {
			name = "ffprobe.exe"
		}
		p := filepath.Join(dir, name)
		if exists(p) {
			return p, true, true
		}
		return "", false, true
	case DeepFilter:
		p, ok := cachedStatus(tool, deepFilterVersion, deepFilterBinName)
		return p, ok, true
	case AutoEditor:
		p, ok := cachedStatus(tool, autoEditorVersion, autoEditorBinName)
		return p, ok, true
	case Resvg:
		if p, err := exec.LookPath("resvg"); err == nil {
			return p, true, false
		}
		p, ok := cachedStatus(tool, resvgResolveVersion(), resvgBinName)
		return p, ok, true
	default:
		return "", false, false
	}
}

// Redownload forces tool to be fetched again even if it's already cached
// (e.g. to recover from a corrupted download), by removing the cached
// file first and then resolving normally. It refuses tools resolved from
// the system PATH rather than vidpolish's own cache (see Status) -
// there's nothing for vidpolish to redownload in that case.
func Redownload(tool Tool) (string, error) {
	return RedownloadWithProgress(tool, nil)
}

// RedownloadWithProgress is Redownload, additionally reporting live
// progress/log activity through prog (which may be nil).
func RedownloadWithProgress(tool Tool, prog *Progress) (string, error) {
	if _, _, managed := Status(tool); !managed {
		if path, err := exec.LookPath(string(tool)); err == nil {
			return "", fmt.Errorf("%s is provided by your system PATH (%s), not managed by vidpolish - nothing to redownload", tool, path)
		}
	}

	switch tool {
	case FFmpeg, FFprobe:
		dir, err := cacheDir(FFmpeg, ffmpegCacheTag)
		if err != nil {
			return "", err
		}
		os.Remove(filepath.Join(dir, "ffmpeg.exe"))
		os.Remove(filepath.Join(dir, "ffprobe.exe"))
		return resolveFFmpegWindows(tool, prog)
	case DeepFilter:
		removeCached(tool, deepFilterVersion, deepFilterBinName)
		return resolveDownloaded(tool, deepFilterVersion, deepFilterAsset, deepFilterBinName, prog)
	case AutoEditor:
		removeCached(tool, autoEditorVersion, autoEditorBinName)
		return resolveDownloaded(tool, autoEditorVersion, autoEditorAsset, autoEditorBinName, prog)
	case Resvg:
		version := resvgResolveVersion()
		removeCached(tool, version, resvgBinName)
		path, err := resolveDownloaded(tool, version, resvgAsset, resvgBinName, prog)
		if err != nil {
			return "", fmt.Errorf("resvg has no prebuilt binary for %s/%s: install it yourself (e.g. `cargo install resvg`) and ensure it's on PATH: %w", runtime.GOOS, runtime.GOARCH, err)
		}
		return path, nil
	default:
		return "", fmt.Errorf("unknown tool %q", tool)
	}
}

// removeCached deletes tool@version's cached binary, if present, so the
// next resolveDownloaded call is forced to fetch it fresh.
func removeCached(tool Tool, version string, binNameFn func() string) {
	dir, err := cacheDir(tool, version)
	if err != nil {
		return
	}
	os.Remove(filepath.Join(dir, binNameFn()))
}

// resvgResolveVersion returns the resvg release version to fetch for the
// current platform - see resvgWindowsVersion for why Windows differs.
func resvgResolveVersion() string {
	if runtime.GOOS == "windows" {
		return resvgWindowsVersion
	}
	return resvgVersion
}

// cachedStatus reports whether tool@version is already sitting in the
// cache dir, without downloading it.
func cachedStatus(tool Tool, version string, binNameFn func() string) (string, bool) {
	dir, err := cacheDir(tool, version)
	if err != nil {
		return "", false
	}
	p := filepath.Join(dir, binNameFn())
	if exists(p) {
		return p, true
	}
	return "", false
}

// FontStatus mirrors Status for the cached Inter font files, without
// downloading them.
func FontStatus() (regular, bold string, available bool) {
	userCache, err := os.UserCacheDir()
	if err != nil {
		return "", "", false
	}
	dir := filepath.Join(userCache, "vidpolish", "fonts", "inter-"+interVersion)
	regular = filepath.Join(dir, "Inter-Regular.ttf")
	bold = filepath.Join(dir, "Inter-Bold.ttf")
	if exists(regular) && exists(bold) {
		return regular, bold, true
	}
	return "", "", false
}

// ResolveFont returns the paths to the cached Inter Regular and Bold TTF
// files used for thumbnail text rendering, downloading them from Inter's
// GitHub release on first use.
func ResolveFont() (regular, bold string, err error) {
	return ResolveFontWithProgress(nil)
}

// ResolveFontWithProgress is ResolveFont, additionally reporting live
// progress/log activity through prog (which may be nil).
func ResolveFontWithProgress(prog *Progress) (regular, bold string, err error) {
	userCache, err := os.UserCacheDir()
	if err != nil {
		return "", "", fmt.Errorf("resolving user cache dir: %w", err)
	}
	dir := filepath.Join(userCache, "vidpolish", "fonts", "inter-"+interVersion)
	regular = filepath.Join(dir, "Inter-Regular.ttf")
	bold = filepath.Join(dir, "Inter-Bold.ttf")

	if exists(regular) && exists(bold) {
		return regular, bold, nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("creating font cache dir: %w", err)
	}

	url := fmt.Sprintf("https://github.com/rsms/inter/releases/download/v%s/Inter-%s.zip", interVersion, interVersion)
	prog.log("vidpolish: downloading Inter font %s from %s", interVersion, url)

	tmp, err := downloadToTemp(url, dir, prog)
	if err != nil {
		return "", "", fmt.Errorf("downloading font: %w", err)
	}
	defer os.Remove(tmp)

	if err := extractNamedFromZip(tmp, "extras/ttf/Inter-Regular.ttf", regular); err != nil {
		return "", "", fmt.Errorf("extracting Inter-Regular.ttf: %w", err)
	}
	if err := extractNamedFromZip(tmp, "extras/ttf/Inter-Bold.ttf", bold); err != nil {
		return "", "", fmt.Errorf("extracting Inter-Bold.ttf: %w", err)
	}
	return regular, bold, nil
}

// RedownloadFont forces the Inter font files to be fetched again even if
// already cached (e.g. to recover from a corrupted download).
func RedownloadFont() (regular, bold string, err error) {
	return RedownloadFontWithProgress(nil)
}

// RedownloadFontWithProgress is RedownloadFont, additionally reporting
// live progress/log activity through prog (which may be nil).
func RedownloadFontWithProgress(prog *Progress) (regular, bold string, err error) {
	userCache, err := os.UserCacheDir()
	if err != nil {
		return "", "", fmt.Errorf("resolving user cache dir: %w", err)
	}
	dir := filepath.Join(userCache, "vidpolish", "fonts", "inter-"+interVersion)
	os.Remove(filepath.Join(dir, "Inter-Regular.ttf"))
	os.Remove(filepath.Join(dir, "Inter-Bold.ttf"))
	return ResolveFontWithProgress(prog)
}

// resolveFFmpegWindows returns the cached path to ffmpeg.exe or ffprobe.exe
// on Windows, downloading gyan.dev's essentials build once and extracting
// both binaries together (they ship in the same archive) if the cache
// doesn't already have them.
func resolveFFmpegWindows(tool Tool, prog *Progress) (string, error) {
	dir, err := cacheDir(FFmpeg, ffmpegCacheTag)
	if err != nil {
		return "", err
	}
	ffmpegPath := filepath.Join(dir, "ffmpeg.exe")
	ffprobePath := filepath.Join(dir, "ffprobe.exe")

	if !exists(ffmpegPath) || !exists(ffprobePath) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("creating cache dir %s: %w", dir, err)
		}

		url := ffmpegWindowsAsset()
		prog.log("vidpolish: downloading ffmpeg from %s", url)
		tmp, err := downloadToTemp(url, dir, prog)
		if err != nil {
			return "", fmt.Errorf("downloading ffmpeg: %w", err)
		}
		defer os.Remove(tmp)

		if err := extractNamedSuffixFromZip(tmp, "/bin/ffmpeg.exe", ffmpegPath); err != nil {
			return "", fmt.Errorf("extracting ffmpeg.exe: %w", err)
		}
		if err := extractNamedSuffixFromZip(tmp, "/bin/ffprobe.exe", ffprobePath); err != nil {
			return "", fmt.Errorf("extracting ffprobe.exe: %w", err)
		}
		if err := os.Chmod(ffmpegPath, 0o755); err != nil {
			return "", err
		}
		if err := os.Chmod(ffprobePath, 0o755); err != nil {
			return "", err
		}
	}

	if tool == FFprobe {
		return ffprobePath, nil
	}
	return ffmpegPath, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func cacheDir(tool Tool, version string) (string, error) {
	userCache, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolving user cache dir: %w", err)
	}
	return filepath.Join(userCache, "vidpolish", "bin", string(tool), version), nil
}

// resolveDownloaded returns the cached binary path for tool@version,
// downloading it via assetFn/binNameFn if not already present.
func resolveDownloaded(tool Tool, version string, assetFn func(version string) (url, archiveExt string, err error), binNameFn func() string, prog *Progress) (string, error) {
	dir, err := cacheDir(tool, version)
	if err != nil {
		return "", err
	}
	binPath := filepath.Join(dir, binNameFn())

	if _, err := os.Stat(binPath); err == nil {
		return binPath, nil
	}

	url, archiveExt, err := assetFn(version)
	if err != nil {
		return "", fmt.Errorf("no known %s release asset for %s/%s: %w", tool, runtime.GOOS, runtime.GOARCH, err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating cache dir %s: %w", dir, err)
	}

	prog.log("vidpolish: downloading %s %s from %s", tool, version, url)
	if err := downloadAndInstall(url, archiveExt, binPath, prog); err != nil {
		return "", fmt.Errorf("installing %s: %w", tool, err)
	}

	return binPath, nil
}

// downloadAndInstall fetches url and installs it as an executable at
// binPath. archiveExt is "" for a plain binary asset, "zip" to extract the
// single expected binary out of a zip archive, or "tar.gz" for a gzipped
// tarball.
func downloadAndInstall(url, archiveExt, binPath string, prog *Progress) error {
	tmpPath, err := downloadToTemp(url, filepath.Dir(binPath), prog)
	if err != nil {
		return err
	}
	defer os.Remove(tmpPath)

	switch archiveExt {
	case "":
		if err := os.Rename(tmpPath, binPath); err != nil {
			return err
		}
	case "zip":
		if err := extractSingleFromZip(tmpPath, binPath); err != nil {
			return err
		}
	case "tar.gz":
		if err := extractSingleFromTarGz(tmpPath, binPath); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported archive type %q", archiveExt)
	}

	return os.Chmod(binPath, 0o755)
}

// downloadToTemp fetches url into a temp file under dir, printing a live
// progress line to stderr (percentage/size/speed/ETA when the server
// reports Content-Length, otherwise just bytes-so-far and speed), and
// returns the temp file's path. The caller is responsible for removing it.
func downloadToTemp(url, dir string, prog *Progress) (string, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w (check your network connection and try again)", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching %s: unexpected status %s", url, resp.Status)
	}

	tmp, err := os.CreateTemp(dir, "download-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()

	pw := &progressWriter{total: resp.ContentLength, start: time.Now(), prog: prog}
	if _, err := io.Copy(tmp, io.TeeReader(resp.Body, pw)); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		fmt.Fprintln(os.Stderr)
		return "", fmt.Errorf("downloading %s: %w (check your network connection and try again)", url, err)
	}
	pw.finish()
	tmp.Close()
	return tmpPath, nil
}

// progressWriter renders a single, periodically-updated progress line to
// stderr as bytes flow through it (used via io.TeeReader around a download
// body), so a large fetch never looks like it's simply hung. It also
// forwards the same byte counts to prog.OnProgress, if set, at the same
// throttled rate.
type progressWriter struct {
	total     int64
	written   int64
	start     time.Time
	lastPrint time.Time
	prog      *Progress
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += int64(n)

	now := time.Now()
	if now.Sub(pw.lastPrint) < 200*time.Millisecond {
		return n, nil
	}
	pw.lastPrint = now
	pw.render(now)
	pw.prog.progress(pw.written, pw.total)
	return n, nil
}

func (pw *progressWriter) render(now time.Time) {
	elapsed := now.Sub(pw.start).Seconds()
	var speed float64
	if elapsed > 0 {
		speed = float64(pw.written) / elapsed
	}

	if pw.total > 0 {
		pct := float64(pw.written) / float64(pw.total) * 100
		eta := "..."
		if speed > 0 {
			remaining := float64(pw.total - pw.written)
			eta = time.Duration(remaining / speed * float64(time.Second)).Round(time.Second).String()
		}
		fmt.Fprintf(os.Stderr, "\r  %5.1f%%  %s / %s  %s/s  ETA %-8s", pct,
			humanize.Bytes(uint64(pw.written)), humanize.Bytes(uint64(pw.total)),
			humanize.Bytes(uint64(speed)), eta)
	} else {
		fmt.Fprintf(os.Stderr, "\r  %s downloaded  %s/s", humanize.Bytes(uint64(pw.written)), humanize.Bytes(uint64(speed)))
	}
}

// finish renders one last, complete progress line and moves to a new line.
func (pw *progressWriter) finish() {
	pw.render(time.Now())
	fmt.Fprintln(os.Stderr)
	pw.prog.progress(pw.written, pw.total)
}

// extractSingleFromZip extracts the first regular file it finds in the zip
// at zipPath to destPath.
func extractSingleFromZip(zipPath, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		return copyErr
	}
	return fmt.Errorf("no files found in zip %s", zipPath)
}

// extractSingleFromTarGz extracts the first regular file it finds in the
// gzipped tarball at tarGzPath to destPath.
func extractSingleFromTarGz(tarGzPath, destPath string) error {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return fmt.Errorf("opening tar.gz: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("opening gzip stream: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("no files found in tar.gz %s", tarGzPath)
		}
		if err != nil {
			return fmt.Errorf("reading tar.gz: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(out, tr)
		out.Close()
		return copyErr
	}
}

// extractNamedSuffixFromZip extracts the first zip entry whose name ends
// with wantSuffix to destPath. Used for archives (like gyan.dev's ffmpeg
// builds) whose entries sit under a versioned top-level folder we don't
// want to hardcode.
func extractNamedSuffixFromZip(zipPath, wantSuffix, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if !strings.HasSuffix(f.Name, wantSuffix) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		return copyErr
	}
	return fmt.Errorf("no entry ending in %q found in zip %s", wantSuffix, zipPath)
}

// extractNamedFromZip extracts the zip entry matching wantName exactly to
// destPath.
func extractNamedFromZip(zipPath, wantName, destPath string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("opening zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name != wantName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		rc.Close()
		out.Close()
		return copyErr
	}
	return fmt.Errorf("entry %q not found in zip %s", wantName, zipPath)
}
