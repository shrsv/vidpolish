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
