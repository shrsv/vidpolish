package binmgr

import (
	"fmt"
	"runtime"
)

// deepFilterAsset returns the GitHub release download URL for the
// deep-filter CLI binary matching the current OS/arch. DeepFilterNet ships
// plain binaries (no archive) for each platform.
func deepFilterAsset(version string) (url, archiveExt string, err error) {
	return deepFilterAssetFor(runtime.GOOS, runtime.GOARCH, version)
}

func deepFilterAssetFor(goos, goarch, version string) (url, archiveExt string, err error) {
	var target string
	switch goos {
	case "linux":
		switch goarch {
		case "amd64":
			target = "x86_64-unknown-linux-musl"
		case "arm64":
			target = "aarch64-unknown-linux-gnu"
		default:
			return "", "", fmt.Errorf("unsupported linux arch %s", goarch)
		}
	case "darwin":
		switch goarch {
		case "amd64":
			target = "x86_64-apple-darwin"
		case "arm64":
			target = "aarch64-apple-darwin"
		default:
			return "", "", fmt.Errorf("unsupported darwin arch %s", goarch)
		}
	case "windows":
		target = "x86_64-pc-windows-msvc.exe"
	default:
		return "", "", fmt.Errorf("unsupported OS %s", goos)
	}

	name := fmt.Sprintf("deep-filter-%s-%s", version, target)
	url = fmt.Sprintf("https://github.com/Rikorose/DeepFilterNet/releases/download/v%s/%s", version, name)
	return url, "", nil
}

func deepFilterBinName() string {
	if runtime.GOOS == "windows" {
		return "deep-filter.exe"
	}
	return "deep-filter"
}

// autoEditorAsset returns the GitHub release download URL for the
// auto-editor CLI binary matching the current OS/arch.
func autoEditorAsset(version string) (url, archiveExt string, err error) {
	return autoEditorAssetFor(runtime.GOOS, runtime.GOARCH, version)
}

func autoEditorAssetFor(goos, goarch, version string) (url, archiveExt string, err error) {
	var suffix string
	switch goos {
	case "linux":
		switch goarch {
		case "amd64":
			suffix = "linux-x86_64"
		case "arm64":
			suffix = "linux-aarch64"
		default:
			return "", "", fmt.Errorf("unsupported linux arch %s", goarch)
		}
	case "darwin":
		switch goarch {
		case "amd64":
			suffix = "macos-x86_64"
		case "arm64":
			suffix = "macos-arm64"
		default:
			return "", "", fmt.Errorf("unsupported darwin arch %s", goarch)
		}
	case "windows":
		suffix = "windows-x86_64.exe"
	default:
		return "", "", fmt.Errorf("unsupported OS %s", goos)
	}

	name := fmt.Sprintf("auto-editor-%s", suffix)
	url = fmt.Sprintf("https://github.com/WyattBlue/auto-editor/releases/download/%s/%s", version, name)
	return url, "", nil
}

func autoEditorBinName() string {
	if runtime.GOOS == "windows" {
		return "auto-editor.exe"
	}
	return "auto-editor"
}

// resvgAsset returns the GitHub release download URL for the resvg CLI
// binary matching the current OS/arch. resvg only publishes prebuilt
// Windows binaries up to v0.47.0 (dropped starting v0.48.0 - see
// resvgWindowsVersion); linux/arm64 has never had one and is expected to
// have resvg installed manually and on PATH.
func resvgAsset(version string) (url, archiveExt string, err error) {
	return resvgAssetFor(runtime.GOOS, runtime.GOARCH, version)
}

func resvgAssetFor(goos, goarch, version string) (url, archiveExt string, err error) {
	var name, ext string
	switch goos {
	case "linux":
		if goarch != "amd64" {
			return "", "", fmt.Errorf("resvg publishes no linux/%s binary", goarch)
		}
		name, ext = "resvg-linux-x86_64.tar.gz", "tar.gz"
	case "darwin":
		switch goarch {
		case "amd64":
			name, ext = "resvg-macos-x86_64.zip", "zip"
		case "arm64":
			name, ext = "resvg-macos-aarch64.zip", "zip"
		default:
			return "", "", fmt.Errorf("resvg publishes no darwin/%s binary", goarch)
		}
	case "windows":
		if goarch != "amd64" {
			return "", "", fmt.Errorf("resvg publishes no windows/%s binary", goarch)
		}
		name, ext = "resvg-win64.zip", "zip"
	default:
		return "", "", fmt.Errorf("resvg publishes no %s binary", goos)
	}

	url = fmt.Sprintf("https://github.com/linebender/resvg/releases/download/v%s/%s", version, name)
	return url, ext, nil
}

func resvgBinName() string {
	if runtime.GOOS == "windows" {
		return "resvg.exe"
	}
	return "resvg"
}

// ffmpegWindowsAsset returns the download URL for a Windows ffmpeg+ffprobe
// build (gyan.dev's "essentials" build - ffmpeg/ffprobe/ffplay only, no
// extra codec docs, smaller than "full").
//
// This is a stable alias, not a version-pinned URL: gyan.dev only keeps
// dated/versioned package archives (under builds/packages/) for a limited
// time before removing them, so a hardcoded version number here would
// eventually 404 (this happened - see git history). The alias always
// redirects (HTTP 303) to whichever build is current, which Go's
// http.Client follows automatically. The zip contains ffmpeg.exe/
// ffprobe.exe under a version-numbered top-level folder, extracted by
// suffix match (see extractNamedSuffixFromZip) so the exact folder name
// doesn't matter.
func ffmpegWindowsAsset() (url string) {
	return "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"
}
