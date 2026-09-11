// Package binmgr resolves the external tool binaries vidpolish shells out to
// (ffmpeg/ffprobe, deep-filter, auto-editor), downloading pinned per-platform
// releases into a local cache when they aren't already available.
package binmgr

import (
	"archive/zip"
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
)

// Tool identifies an external binary vidpolish depends on.
type Tool string

const (
	FFmpeg     Tool = "ffmpeg"
	FFprobe    Tool = "ffprobe"
	DeepFilter Tool = "deep-filter"
	AutoEditor Tool = "auto-editor"
)

// Resolve returns the absolute path to the requested tool's binary,
// downloading and caching it first if necessary. ffmpeg/ffprobe are expected
// to already be installed on the system (they're near-universally
// preinstalled); deep-filter and auto-editor are fetched from their GitHub
// releases into ~/.cache/vidpolish/bin.
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
	default:
		return "", fmt.Errorf("unknown tool %q", tool)
	}
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
// binPath. archiveExt is "" for a plain binary asset, or "zip" to extract
// the single expected binary out of a zip archive.
func downloadAndInstall(url, archiveExt, binPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: unexpected status %s", url, resp.Status)
	}

	tmp, err := os.CreateTemp(filepath.Dir(binPath), "download-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return fmt.Errorf("saving download: %w", err)
	}
	tmp.Close()

	switch archiveExt {
	case "":
		if err := os.Rename(tmpPath, binPath); err != nil {
			return err
		}
	case "zip":
		if err := extractSingleFromZip(tmpPath, binPath); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported archive type %q", archiveExt)
	}

	return os.Chmod(binPath, 0o755)
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
