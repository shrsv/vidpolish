package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestServeFileSetsContentTypeExplicitly guards against relying on
// mime.TypeByExtension's OS-dependent fallback (on Windows it reads from
// the registry and can come back empty), which left the Windows GUI's
// video preview unable to play anything: WebView2/Chromium's <video>
// silently refuses a response with no Content-Type at all.
func TestServeFileSetsContentTypeExplicitly(t *testing.T) {
	cases := map[string]string{
		"clip.mp4":  "video/mp4",
		"clip.webm": "video/webm",
		"clip.mov":  "video/quicktime",
		"shot.png":  "image/png",
		"anim.gif":  "image/gif",
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			if err := os.WriteFile(path, []byte("fake media bytes"), 0o644); err != nil {
				t.Fatalf("WriteFile: %v", err)
			}
			req := httptest.NewRequest(http.MethodGet, "/media", nil)
			rec := httptest.NewRecorder()
			serveFile(rec, req, path)
			if got := rec.Header().Get("Content-Type"); got != want {
				t.Errorf("Content-Type = %q, want %q", got, want)
			}
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rec.Code)
			}
		})
	}
}
