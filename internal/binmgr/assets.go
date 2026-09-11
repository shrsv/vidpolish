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
