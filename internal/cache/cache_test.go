package cache

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFingerprintStableForUnchangedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.mp4")
	if err := os.WriteFile(path, []byte("hello world, this is a fake video file"), 0o644); err != nil {
		t.Fatal(err)
	}

	a, err := Fingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Fingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("fingerprint changed across calls on the same file: %s vs %s", a, b)
	}
}

func TestFingerprintChangesWithContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.mp4")

	if err := os.WriteFile(path, []byte("original content"), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := Fingerprint(path)
	if err != nil {
		t.Fatal(err)
	}

	// Ensure a different mtime even on coarse filesystem clocks.
	future := time.Now().Add(time.Second)
	if err := os.WriteFile(path, []byte("different content, re-recorded"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}

	after, err := Fingerprint(path)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Error("fingerprint did not change when file content/size/mtime changed")
	}
}

func TestParamsKeyDeterministicAndSensitiveToInputs(t *testing.T) {
	a := ParamsKey("margin=0.2s", "speed=1.00")
	b := ParamsKey("margin=0.2s", "speed=1.00")
	if a != b {
		t.Errorf("ParamsKey not deterministic: %s vs %s", a, b)
	}

	c := ParamsKey("margin=0.3s", "speed=1.00")
	if a == c {
		t.Error("ParamsKey did not change when a field changed")
	}
}

func TestSweepRemovesStaleEntriesOnly(t *testing.T) {
	root := t.TempDir()

	fresh := filepath.Join(root, "fresh")
	stale := filepath.Join(root, "stale")
	for _, d := range []string{fresh, stale} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := Touch(fresh); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, ".cached-at"), []byte("1"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Sweep(root, DefaultTTL); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(fresh); err != nil {
		t.Errorf("fresh entry should survive sweep, got: %v", err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale entry should be removed, stat err: %v", err)
	}
}

func TestListEntriesReportsSizeAndCachedAt(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "abc123")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "video.mp4"), make([]byte, 1000), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Touch(dir); err != nil {
		t.Fatal(err)
	}

	entries, err := ListEntries(root)
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Fingerprint != "abc123" {
		t.Errorf("Fingerprint = %q", e.Fingerprint)
	}
	// SizeBytes includes the .cached-at marker file too, so just check it's
	// at least as large as the video file we wrote.
	if e.SizeBytes < 1000 {
		t.Errorf("SizeBytes = %d, want >= 1000", e.SizeBytes)
	}
	if e.CachedAt.IsZero() {
		t.Error("CachedAt should be set after Touch")
	}
}

func TestListEntriesOnMissingRootReturnsEmpty(t *testing.T) {
	entries, err := ListEntries(filepath.Join(t.TempDir(), "does-not-exist"))
	if err != nil {
		t.Fatalf("ListEntries: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no entries, got %v", entries)
	}
}

func TestLockSerializesSameFingerprintConcurrentAccess(t *testing.T) {
	var (
		mu        sync.Mutex
		active    int
		maxActive int
	)

	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := Lock("same-fingerprint")
			defer unlock()

			mu.Lock()
			active++
			if active > maxActive {
				maxActive = active
			}
			mu.Unlock()

			time.Sleep(10 * time.Millisecond)

			mu.Lock()
			active--
			mu.Unlock()
		}()
	}
	wg.Wait()

	if maxActive != 1 {
		t.Errorf("max concurrent holders of the same fingerprint lock = %d, want 1", maxActive)
	}
}

func TestLockDoesNotSerializeDifferentFingerprints(t *testing.T) {
	var running int32
	var maxRunning int32
	var wg sync.WaitGroup

	for i := range 5 {
		fp := "fingerprint-" + string(rune('a'+i))
		wg.Add(1)
		go func(fp string) {
			defer wg.Done()
			unlock := Lock(fp)
			defer unlock()

			n := atomic.AddInt32(&running, 1)
			for {
				old := atomic.LoadInt32(&maxRunning)
				if n <= old || atomic.CompareAndSwapInt32(&maxRunning, old, n) {
					break
				}
			}
			time.Sleep(20 * time.Millisecond)
			atomic.AddInt32(&running, -1)
		}(fp)
	}
	wg.Wait()

	if maxRunning < 2 {
		t.Errorf("expected different fingerprints to run concurrently, max concurrent = %d", maxRunning)
	}
}
