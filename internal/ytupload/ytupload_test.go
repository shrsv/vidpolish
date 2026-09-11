package ytupload

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestUploadChunksReportsProgressAndReturnsVideo drives uploadChunks
// against a fake resumable-upload server that requires two chunks (308
// then 200), verifying Content-Range headers, progress callbacks, and the
// parsed final video ID.
func TestUploadChunksReportsProgressAndReturnsVideo(t *testing.T) {
	const size = chunkSize + 1024 // forces a second, smaller chunk

	var mu sync.Mutex
	var ranges []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		ranges = append(ranges, r.Header.Get("Content-Range"))
		n := len(ranges)
		mu.Unlock()

		if n < 2 {
			w.WriteHeader(308)
			return
		}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(map[string]string{"id": "abc123"})
	}))
	defer server.Close()

	f, err := os.CreateTemp(t.TempDir(), "video-*.mp4")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}
	if _, err := f.Write(make([]byte, size)); err != nil {
		t.Fatalf("writing temp file: %v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seeking temp file: %v", err)
	}
	defer f.Close()

	var progressCalls []float64
	opts := Options{
		Progress: func(fraction float64, _ time.Duration) {
			progressCalls = append(progressCalls, fraction)
		},
	}

	video, err := uploadChunks(server.URL, f, size, opts)
	if err != nil {
		t.Fatalf("uploadChunks: %v", err)
	}
	if video.ID != "abc123" {
		t.Fatalf("video ID = %q, want abc123", video.ID)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(ranges) != 2 {
		t.Fatalf("got %d requests, want 2: %v", len(ranges), ranges)
	}
	wantFirst := fmt.Sprintf("bytes 0-%d/%d", chunkSize-1, size)
	if ranges[0] != wantFirst {
		t.Fatalf("first Content-Range = %q, want %q", ranges[0], wantFirst)
	}
	wantSecond := fmt.Sprintf("bytes %d-%d/%d", chunkSize, size-1, size)
	if ranges[1] != wantSecond {
		t.Fatalf("second Content-Range = %q, want %q", ranges[1], wantSecond)
	}

	if len(progressCalls) == 0 {
		t.Fatal("expected at least one progress callback")
	}
	last := progressCalls[len(progressCalls)-1]
	if last != 1.0 {
		t.Fatalf("final progress fraction = %v, want 1.0", last)
	}
}

func TestStartSessionSendsLanguageAndPrivacy(t *testing.T) {
	origInsertURL := insertURL
	t.Cleanup(func() { insertURL = origInsertURL })

	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decoding request body: %v", err)
		}
		if got := r.Header.Get("X-Upload-Content-Type"); got != "video/mp4" {
			t.Errorf("X-Upload-Content-Type = %q, want video/mp4", got)
		}
		if got := r.Header.Get("X-Upload-Content-Length"); got != "12345" {
			t.Errorf("X-Upload-Content-Length = %q, want 12345", got)
		}
		w.Header().Set("Location", "https://example.invalid/session")
		w.WriteHeader(200)
	}))
	defer server.Close()
	insertURL = server.URL

	loc, err := startSession(Options{
		Title:    "t",
		Privacy:  "unlisted",
		Language: "en-IN",
	}, 12345)
	if err != nil {
		t.Fatalf("startSession: %v", err)
	}
	if loc != "https://example.invalid/session" {
		t.Fatalf("session URI = %q", loc)
	}

	snippet, ok := gotBody["snippet"].(map[string]any)
	if !ok {
		t.Fatalf("no snippet in body: %v", gotBody)
	}
	if snippet["defaultLanguage"] != "en-IN" {
		t.Fatalf("defaultLanguage not set in body: %v", gotBody)
	}
	if snippet["defaultAudioLanguage"] != "en-IN" {
		t.Fatalf("defaultAudioLanguage not set in body: %v", gotBody)
	}
	status, ok := gotBody["status"].(map[string]any)
	if !ok {
		t.Fatalf("no status in body: %v", gotBody)
	}
	if status["privacyStatus"] != "unlisted" {
		t.Fatalf("privacyStatus not set in body: %v", gotBody)
	}
}

func TestStartSessionMissingLocationErrors(t *testing.T) {
	origInsertURL := insertURL
	t.Cleanup(func() { insertURL = origInsertURL })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200) // no Location header
	}))
	defer server.Close()
	insertURL = server.URL

	if _, err := startSession(Options{}, 1); err == nil {
		t.Fatal("expected error for missing Location header")
	}
}

func TestSetThumbnailSendsPNGBytes(t *testing.T) {
	origThumbnailSetURL := thumbnailSetURL
	t.Cleanup(func() { thumbnailSetURL = origThumbnailSetURL })

	want := []byte{0x89, 'P', 'N', 'G', 1, 2, 3}
	var gotBody []byte
	var gotContentType, gotVideoIDQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotVideoIDQuery = r.URL.Query().Get("videoId")
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.WriteHeader(200)
	}))
	defer server.Close()
	thumbnailSetURL = server.URL + "?videoId="

	path := filepath.Join(t.TempDir(), "thumb.png")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("writing test thumbnail: %v", err)
	}

	if err := setThumbnail("token123", "vid1", path); err != nil {
		t.Fatalf("setThumbnail: %v", err)
	}
	if string(gotBody) != string(want) {
		t.Fatalf("uploaded body = %v, want %v", gotBody, want)
	}
	if gotContentType != "image/png" {
		t.Fatalf("Content-Type = %q, want image/png", gotContentType)
	}
	if gotVideoIDQuery != "vid1" {
		t.Fatalf("videoId query = %q, want vid1", gotVideoIDQuery)
	}
}

func TestSetThumbnailSurfacesAPIError(t *testing.T) {
	origThumbnailSetURL := thumbnailSetURL
	t.Cleanup(func() { thumbnailSetURL = origThumbnailSetURL })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"error":{"message":"youtubeSignupRequired: phone verification required"}}`))
	}))
	defer server.Close()
	thumbnailSetURL = server.URL + "?videoId="

	path := filepath.Join(t.TempDir(), "thumb.png")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("writing test thumbnail: %v", err)
	}

	err := setThumbnail("token123", "vid1", path)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "phone verification") {
		t.Fatalf("error %q should surface the raw API message naming phone verification", err.Error())
	}
}
