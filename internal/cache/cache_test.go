package cache

import (
	"os"
	"path/filepath"
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
