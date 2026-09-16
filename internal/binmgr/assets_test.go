package binmgr

import "testing"

func TestDeepFilterAssetLinuxAmd64(t *testing.T) {
	url, ext, err := deepFilterAssetFor("linux", "amd64", "0.5.6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://github.com/Rikorose/DeepFilterNet/releases/download/v0.5.6/deep-filter-0.5.6-x86_64-unknown-linux-musl"
	if url != want {
		t.Errorf("url = %q, want %q", url, want)
	}
	if ext != "" {
		t.Errorf("ext = %q, want empty (plain binary asset)", ext)
	}
}

func TestAutoEditorAssetDarwinArm64(t *testing.T) {
	url, _, err := autoEditorAssetFor("darwin", "arm64", "31.6.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://github.com/WyattBlue/auto-editor/releases/download/31.6.0/auto-editor-macos-arm64"
	if url != want {
		t.Errorf("url = %q, want %q", url, want)
	}
}

func TestUnsupportedArchReturnsError(t *testing.T) {
	if _, _, err := deepFilterAssetFor("linux", "riscv64", "0.5.6"); err == nil {
		t.Error("expected error for unsupported arch, got nil")
	}
}

func TestResvgAssetLinuxAmd64(t *testing.T) {
	url, ext, err := resvgAssetFor("linux", "amd64", "0.48.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://github.com/linebender/resvg/releases/download/v0.48.1/resvg-linux-x86_64.tar.gz"
	if url != want {
		t.Errorf("url = %q, want %q", url, want)
	}
	if ext != "tar.gz" {
		t.Errorf("ext = %q, want tar.gz", ext)
	}
}

func TestResvgAssetDarwinArm64(t *testing.T) {
	url, ext, err := resvgAssetFor("darwin", "arm64", "0.48.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://github.com/linebender/resvg/releases/download/v0.48.1/resvg-macos-aarch64.zip"
	if url != want {
		t.Errorf("url = %q, want %q", url, want)
	}
	if ext != "zip" {
		t.Errorf("ext = %q, want zip", ext)
	}
}

func TestResvgAssetUnsupportedPlatformsReturnError(t *testing.T) {
	if _, _, err := resvgAssetFor("linux", "arm64", "0.48.1"); err == nil {
		t.Error("expected error for linux/arm64 (resvg publishes no such binary), got nil")
	}
	if _, _, err := resvgAssetFor("windows", "arm64", "0.47.0"); err == nil {
		t.Error("expected error for windows/arm64 (resvg publishes no such binary), got nil")
	}
}

func TestResvgAssetWindowsAmd64(t *testing.T) {
	url, ext, err := resvgAssetFor("windows", "amd64", "0.47.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://github.com/linebender/resvg/releases/download/v0.47.0/resvg-win64.zip"
	if url != want {
		t.Errorf("url = %q, want %q", url, want)
	}
	if ext != "zip" {
		t.Errorf("ext = %q, want zip", ext)
	}
}

func TestFFmpegWindowsAsset(t *testing.T) {
	url := ffmpegWindowsAsset()
	want := "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"
	if url != want {
		t.Errorf("url = %q, want %q", url, want)
	}
}
