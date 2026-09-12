package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"vidpolish/internal/cache"
	"vidpolish/internal/store"
)

func newTestServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	db, err := store.OpenAt(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("OpenAt: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	s := New(db)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return s, ts
}

func doJSON(t *testing.T, method, url string, body any) (*http.Response, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp, out
}

func TestCreateProjectViaAPI(t *testing.T) {
	_, ts := newTestServer(t)

	resp, body := doJSON(t, http.MethodPost, ts.URL+"/api/projects", map[string]string{"name": "My Project"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, body = %v", resp.StatusCode, body)
	}
	if body["name"] != "My Project" {
		t.Fatalf("name = %v", body["name"])
	}
	cells := body["cells"].([]any)
	if len(cells) != 1 {
		t.Fatalf("expected 1 (source) cell, got %v", cells)
	}
	source := cells[0].(map[string]any)
	if source["kind"] != "source" || source["seq"] != float64(1) {
		t.Fatalf("source cell = %v", source)
	}
}

func TestCreateProjectRequiresName(t *testing.T) {
	_, ts := newTestServer(t)
	resp, _ := doJSON(t, http.MethodPost, ts.URL+"/api/projects", map[string]string{"name": ""})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSourceUploadAndEditCellCreation(t *testing.T) {
	_, ts := newTestServer(t)

	_, proj := doJSON(t, http.MethodPost, ts.URL+"/api/projects", map[string]string{"name": "P"})
	projectID := proj["id"].(string)
	sourceID := proj["cells"].([]any)[0].(map[string]any)["id"].(string)

	// Upload a fake "video" (content doesn't matter for this handler-level test).
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("video", "clip.mp4")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	fw.Write([]byte("fake video bytes"))
	mw.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/cells/"+sourceID+"/source", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload request: %v", err)
	}
	defer resp.Body.Close()
	var sourceResp map[string]any
	json.NewDecoder(resp.Body).Decode(&sourceResp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d, body = %v", resp.StatusCode, sourceResp)
	}
	if sourceResp["status"] != "done" || sourceResp["mediaUrl"] == nil {
		t.Fatalf("source cell not marked done with media: %v", sourceResp)
	}
	if sourceResp["sourceFilename"] != "clip.mp4" {
		t.Fatalf("sourceFilename = %v", sourceResp["sourceFilename"])
	}

	// Serve it back via /api/media.
	mediaResp, err := http.Get(ts.URL + sourceResp["mediaUrl"].(string))
	if err != nil {
		t.Fatalf("media get: %v", err)
	}
	defer mediaResp.Body.Close()
	if mediaResp.StatusCode != http.StatusOK {
		t.Fatalf("media status = %d", mediaResp.StatusCode)
	}

	// Now create an edit cell referencing the source cell.
	resp2, editResp := doJSON(t, http.MethodPost, ts.URL+"/api/projects/"+projectID+"/cells", map[string]any{
		"kind":         "edit",
		"parentCellId": sourceID,
		"params":       map[string]any{"margin": "0.2s", "speed": 1.25},
	})
	if resp2.StatusCode != http.StatusCreated {
		t.Fatalf("create edit cell status = %d, body = %v", resp2.StatusCode, editResp)
	}
	if editResp["seq"] != float64(2) || editResp["name"] != "Edit #2" {
		t.Fatalf("edit cell = %v", editResp)
	}

	// An edit cell cannot be created under another edit cell as parent.
	editID := editResp["id"].(string)
	resp3, badResp := doJSON(t, http.MethodPost, ts.URL+"/api/projects/"+projectID+"/cells", map[string]any{
		"kind":         "edit",
		"parentCellId": editID,
		"params":       map[string]any{},
	})
	if resp3.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for edit-under-edit, got %d: %v", resp3.StatusCode, badResp)
	}
}

func TestDeleteSourceCellRefused(t *testing.T) {
	_, ts := newTestServer(t)
	_, proj := doJSON(t, http.MethodPost, ts.URL+"/api/projects", map[string]string{"name": "P"})
	sourceID := proj["cells"].([]any)[0].(map[string]any)["id"].(string)

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/cells/"+sourceID, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestConfigRedactsSecretsAndPartialUpdate(t *testing.T) {
	// Config lives under the real HOME via os.UserHomeDir; isolate it.
	t.Setenv("HOME", t.TempDir())
	_, ts := newTestServer(t)

	resp, cfg := doJSON(t, http.MethodGet, ts.URL+"/api/config", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get config status = %d", resp.StatusCode)
	}
	yt := cfg["youtube"].(map[string]any)
	secret := yt["clientSecret"].(map[string]any)
	if secret["set"] != false {
		t.Fatalf("fresh config should have no client secret set: %v", secret)
	}

	resp2, cfg2 := doJSON(t, http.MethodPut, ts.URL+"/api/config", map[string]any{
		"youtube": map[string]any{
			"clientId":     "abc",
			"clientSecret": "shh-secret-value",
		},
	})
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("put config status = %d, body=%v", resp2.StatusCode, cfg2)
	}
	yt2 := cfg2["youtube"].(map[string]any)
	if yt2["clientId"] != "abc" {
		t.Fatalf("clientId not updated: %v", yt2)
	}
	secret2 := yt2["clientSecret"].(map[string]any)
	if secret2["set"] != true {
		t.Fatalf("clientSecret should now be set: %v", secret2)
	}
	body, _ := json.Marshal(cfg2)
	if bytes.Contains(body, []byte("shh-secret-value")) {
		t.Fatalf("raw secret value leaked into API response: %s", body)
	}

	// A second PUT that omits clientSecret must leave it unchanged (still set).
	resp3, cfg3 := doJSON(t, http.MethodPut, ts.URL+"/api/config", map[string]any{
		"youtube": map[string]any{"privacy": "public"},
	})
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("put config status = %d", resp3.StatusCode)
	}
	yt3 := cfg3["youtube"].(map[string]any)
	if yt3["privacy"] != "public" {
		t.Fatalf("privacy not updated: %v", yt3)
	}
	if yt3["clientSecret"].(map[string]any)["set"] != true {
		t.Fatalf("clientSecret should remain set after unrelated update: %v", yt3)
	}
}

func TestCacheEndpoints(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, ts := newTestServer(t)

	// Manually create a cache entry the way internal/cache would.
	home, _ := os.UserHomeDir()
	cacheDir := filepath.Join(home, ".vidpolish", "cache", "abc123")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "video.mp4"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/cache")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	var entries []map[string]any
	json.NewDecoder(resp.Body).Decode(&entries)
	if len(entries) != 1 || entries[0]["fingerprint"] != "abc123" {
		t.Fatalf("entries = %v", entries)
	}

	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/cache/abc123", nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete status = %d", delResp.StatusCode)
	}
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Fatalf("cache dir should be removed, stat err: %v", err)
	}
}

func TestHubPublishSubscribe(t *testing.T) {
	h := newHub()
	ch, cancel := h.Subscribe("cell1")
	defer cancel()

	h.Publish("cell1", "hello")
	h.Publish("other", "should not arrive")

	select {
	case msg := <-ch:
		if msg != "hello" {
			t.Fatalf("msg = %q", msg)
		}
	default:
		t.Fatal("expected a message on the subscribed channel")
	}
}

func TestJobTrackerPreventsDoubleStart(t *testing.T) {
	j := newJobTracker()
	if !j.start("c1") {
		t.Fatal("first start should succeed")
	}
	if j.start("c1") {
		t.Fatal("second concurrent start should be refused")
	}
	j.finish("c1")
	if !j.start("c1") {
		t.Fatal("start after finish should succeed again")
	}
}

func TestReorderCellsViaAPI(t *testing.T) {
	_, ts := newTestServer(t)
	_, proj := doJSON(t, http.MethodPost, ts.URL+"/api/projects", map[string]string{"name": "P"})
	projectID := proj["id"].(string)
	sourceID := proj["cells"].([]any)[0].(map[string]any)["id"].(string)

	var editIDs []string
	for range 2 {
		_, c := doJSON(t, http.MethodPost, ts.URL+"/api/projects/"+projectID+"/cells", map[string]any{
			"kind": "edit", "parentCellId": sourceID, "params": map[string]any{},
		})
		editIDs = append(editIDs, c["id"].(string))
	}
	reversed := []string{editIDs[1], editIDs[0]}

	resp, body := doJSON(t, http.MethodPost, ts.URL+"/api/projects/"+projectID+"/reorder", map[string]any{
		"kind": "edit", "orderedCellIds": reversed,
	})
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("reorder status = %d, body = %v", resp.StatusCode, body)
	}

	_, refreshed := doJSON(t, http.MethodGet, ts.URL+"/api/projects/"+projectID, nil)
	var gotOrder []string
	for _, c := range refreshed["cells"].([]any) {
		cell := c.(map[string]any)
		if cell["kind"] == "edit" {
			gotOrder = append(gotOrder, cell["id"].(string))
		}
	}
	if len(gotOrder) != 2 || gotOrder[0] != reversed[0] || gotOrder[1] != reversed[1] {
		t.Fatalf("edit cell order = %v, want %v", gotOrder, reversed)
	}
}

func TestCacheListShowsOwningProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, ts := newTestServer(t)

	_, proj := doJSON(t, http.MethodPost, ts.URL+"/api/projects", map[string]string{"name": "Linked Project"})
	sourceID := proj["cells"].([]any)[0].(map[string]any)["id"].(string)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("video", "clip.mp4")
	fw.Write([]byte("some video bytes"))
	mw.Close()
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/cells/"+sourceID+"/source", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	uploadResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("source upload: %v", err)
	}
	var sourceCell map[string]any
	json.NewDecoder(uploadResp.Body).Decode(&sourceCell)
	uploadResp.Body.Close()
	sourcePath := sourceCell["sourceFilename"] // just to ensure decode worked
	if sourcePath == nil {
		t.Fatal("source upload did not return sourceFilename")
	}

	// Manually create a cache entry keyed by this exact source file's
	// fingerprint, the way pipeline.Process would.
	home, _ := os.UserHomeDir()
	projectFile := filepath.Join(home, ".vidpolish", "projects", proj["id"].(string), "source.mp4")
	fp, err := cache.Fingerprint(projectFile)
	if err != nil {
		t.Fatalf("Fingerprint: %v", err)
	}
	cacheDir := filepath.Join(home, ".vidpolish", "cache", fp)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "video.mp4"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	resp, err := http.Get(ts.URL + "/api/cache")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	var entries []map[string]any
	json.NewDecoder(resp.Body).Decode(&entries)
	if len(entries) != 1 {
		t.Fatalf("entries = %v", entries)
	}
	if entries[0]["projectName"] != "Linked Project" {
		t.Fatalf("projectName = %v, want 'Linked Project': %v", entries[0]["projectName"], entries[0])
	}
}

func TestConfigBackupsListAndRestore(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, ts := newTestServer(t)

	doJSON(t, http.MethodPut, ts.URL+"/api/config", map[string]any{
		"youtube": map[string]any{"clientId": "first"},
	})
	doJSON(t, http.MethodPut, ts.URL+"/api/config", map[string]any{
		"youtube": map[string]any{"clientId": "second"},
	})

	resp, err := http.Get(ts.URL + "/api/config/backups")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	var backups []map[string]any
	json.NewDecoder(resp.Body).Decode(&backups)
	if len(backups) == 0 {
		t.Fatal("expected at least one backup after two saves")
	}

	ts0 := backups[len(backups)-1]["timestamp"].(string) // oldest = pre-first-save (empty client id)
	restoreResp, restored := doJSON(t, http.MethodPost, ts.URL+"/api/config/backups/"+ts0+"/restore", map[string]any{})
	if restoreResp.StatusCode != http.StatusOK {
		t.Fatalf("restore status = %d, body = %v", restoreResp.StatusCode, restored)
	}
	yt := restored["youtube"].(map[string]any)
	if yt["clientId"] != "" {
		t.Fatalf("clientId after restoring oldest backup = %v, want empty", yt["clientId"])
	}
}

func TestConfigPutRejectsInvalidPrivacy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, ts := newTestServer(t)

	resp, body := doJSON(t, http.MethodPut, ts.URL+"/api/config", map[string]any{
		"youtube": map[string]any{"privacy": "nonsense"},
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %v, want 400", resp.StatusCode, body)
	}
}

func TestCellThumbnailNotFoundBeforeGeneration(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, ts := newTestServer(t)
	_, proj := doJSON(t, http.MethodPost, ts.URL+"/api/projects", map[string]string{"name": "P"})
	sourceID := proj["cells"].([]any)[0].(map[string]any)["id"].(string)

	resp, err := http.Get(ts.URL + "/api/cells/" + sourceID + "/thumbnail")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 before any thumbnail exists", resp.StatusCode)
	}
}

func TestCellThumbnailServesGeneratedFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, ts := newTestServer(t)
	_, proj := doJSON(t, http.MethodPost, ts.URL+"/api/projects", map[string]string{"name": "P"})
	projectID := proj["id"].(string)
	sourceID := proj["cells"].([]any)[0].(map[string]any)["id"].(string)

	_, editResp := doJSON(t, http.MethodPost, ts.URL+"/api/projects/"+projectID+"/cells", map[string]any{
		"kind": "edit", "parentCellId": sourceID, "params": map[string]any{},
	})
	editID := editResp["id"].(string)
	_, uploadResp := doJSON(t, http.MethodPost, ts.URL+"/api/projects/"+projectID+"/cells", map[string]any{
		"kind": "upload", "parentCellId": editID, "params": map[string]any{},
	})
	uploadID := uploadResp["id"].(string)

	home, _ := os.UserHomeDir()
	thumbDir := filepath.Join(home, ".vidpolish", "projects", projectID, "cells", uploadID)
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(thumbDir, "thumbnail.png"), []byte("fake png bytes"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// GET /api/projects/{id} should now advertise a thumbnailUrl for this cell.
	_, refreshed := doJSON(t, http.MethodGet, ts.URL+"/api/projects/"+projectID, nil)
	var thumbURL string
	for _, c := range refreshed["cells"].([]any) {
		cell := c.(map[string]any)
		if cell["id"] == uploadID {
			thumbURL, _ = cell["thumbnailUrl"].(string)
		}
	}
	if thumbURL == "" {
		t.Fatal("expected thumbnailUrl once the thumbnail file exists")
	}

	resp, err := http.Get(ts.URL + thumbURL)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(data) != "fake png bytes" {
		t.Fatalf("status=%d body=%q", resp.StatusCode, data)
	}
}
