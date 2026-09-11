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
)

const (
	deepFilterVersion = "0.5.6"
	autoEditorVersion = "31.6.0"
	resvgVersion      = "0.48.1"
	interVersion      = "4.1"
)

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
// to already be installed on the system (they're near-universally
// preinstalled); deep-filter, auto-editor, and resvg are fetched from their
// GitHub releases into ~/.cache/vidpolish/bin.
func Resolve(tool Tool) (string, error) {
	switch tool {
	case FFmpeg, FFprobe:
		path, err := exec.LookPath(string(tool))
		if err != nil {
			return "", fmt.Errorf("%s not found on PATH: install ffmpeg (e.g. `apt install ffmpeg`) and retry", tool)
		}
		return path, nil
	case DeepFilter:
		return resolveDownloaded(tool, deepFilterVersion, deepFilterAsset, deepFilterBinName)
	case AutoEditor:
		return resolveDownloaded(tool, autoEditorVersion, autoEditorAsset, autoEditorBinName)
	case Resvg:
		// resvg only publishes binaries for linux/amd64 and darwin; on
		// other platforms (Windows, linux/arm64) fall back to whatever
		// the user has installed themselves (e.g. `cargo install resvg`
		// or a package manager).
		if path, err := exec.LookPath("resvg"); err == nil {
			return path, nil
		}
		path, err := resolveDownloaded(tool, resvgVersion, resvgAsset, resvgBinName)
		if err != nil {
			return "", fmt.Errorf("resvg has no prebuilt binary for %s/%s: install it yourself (e.g. `cargo install resvg`) and ensure it's on PATH: %w", runtime.GOOS, runtime.GOARCH, err)
		}
		return path, nil
	default:
		return "", fmt.Errorf("unknown tool %q", tool)
	}
}

// ResolveFont returns the paths to the cached Inter Regular and Bold TTF
// files used for thumbnail text rendering, downloading them from Inter's
// GitHub release on first use.
func ResolveFont() (regular, bold string, err error) {
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
	fmt.Fprintf(os.Stderr, "vidpolish: downloading Inter font %s from %s\n", interVersion, url)

	tmp, err := downloadToTemp(url, dir)
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
func resolveDownloaded(tool Tool, version string, assetFn func(version string) (url, archiveExt string, err error), binNameFn func() string) (string, error) {
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

	fmt.Fprintf(os.Stderr, "vidpolish: downloading %s %s from %s\n", tool, version, url)
	if err := downloadAndInstall(url, archiveExt, binPath); err != nil {
		return "", fmt.Errorf("installing %s: %w", tool, err)
	}

	return binPath, nil
}

// downloadAndInstall fetches url and installs it as an executable at
// binPath. archiveExt is "" for a plain binary asset, "zip" to extract the
// single expected binary out of a zip archive, or "tar.gz" for a gzipped
// tarball.
func downloadAndInstall(url, archiveExt, binPath string) error {
	tmpPath, err := downloadToTemp(url, filepath.Dir(binPath))
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

// downloadToTemp fetches url into a temp file under dir and returns its
// path. The caller is responsible for removing it.
func downloadToTemp(url, dir string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching %s: %w", url, err)
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

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", fmt.Errorf("saving download: %w", err)
	}
	tmp.Close()
	return tmpPath, nil
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
