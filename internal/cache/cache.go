// Package cache manages vidpolish's persistent pipeline-artifact cache at
// ~/.vidpolish/cache, keyed by a fast content fingerprint of the source
// video so repeated runs (e.g. trying different --margin/--speed values)
// can skip the expensive split/denoise/remux stages.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultTTL is how long a cache entry is kept before Sweep removes it.
const DefaultTTL = 7 * 24 * time.Hour

const fingerprintSampleSize = 1 << 20 // 1MiB

// Root returns ~/.vidpolish/cache, creating it if necessary.
func Root() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	root := filepath.Join(home, ".vidpolish", "cache")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", fmt.Errorf("creating cache root %s: %w", root, err)
	}
	return root, nil
}

// Fingerprint returns a fast content fingerprint for path: a sha256 over
// its size, modification time, and up to the first and last 1MiB of
// content. This is not a full-file hash (too slow for multi-hundred-MB
// recordings on every run) but is enough to detect a re-recording or edit
// of the source file in normal use.
func Fingerprint(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", fmt.Errorf("statting %s: %w", path, err)
	}

	h := sha256.New()
	fmt.Fprintf(h, "size=%d;mtime=%d;", info.Size(), info.ModTime().UnixNano())

	head := make([]byte, fingerprintSampleSize)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return "", fmt.Errorf("reading head of %s: %w", path, err)
	}
	h.Write(head[:n])

	if info.Size() > fingerprintSampleSize {
		tailStart := max(info.Size()-fingerprintSampleSize, int64(n))
		if _, err := f.Seek(tailStart, io.SeekStart); err != nil {
			return "", fmt.Errorf("seeking tail of %s: %w", path, err)
		}
		tail := make([]byte, info.Size()-tailStart)
		if _, err := io.ReadFull(f, tail); err != nil && err != io.EOF {
			return "", fmt.Errorf("reading tail of %s: %w", path, err)
		}
		h.Write(tail)
	}

	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// Dir returns the cache directory for the given fingerprint, creating it if
// necessary.
func Dir(fingerprint string) (string, error) {
	root, err := Root()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, fingerprint)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("creating cache dir %s: %w", dir, err)
	}
	return dir, nil
}

// Touch records the current time as this cache entry's last-used time, for
// TTL bookkeeping by Sweep.
func Touch(dir string) error {
	marker := filepath.Join(dir, ".cached-at")
	return os.WriteFile(marker, []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0o644)
}

// Sweep removes cache entry directories under root whose .cached-at marker
// is older than ttl. It's best-effort: individual removal failures are
// collected but don't stop the sweep.
func Sweep(root string, ttl time.Duration) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("listing cache root %s: %w", root, err)
	}

	cutoff := time.Now().Add(-ttl)
	var errs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		cachedAt, err := readCachedAt(dir)
		if err != nil || cachedAt.Before(cutoff) {
			if err := os.RemoveAll(dir); err != nil {
				errs = append(errs, err.Error())
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("sweep errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func readCachedAt(dir string) (time.Time, error) {
	data, err := os.ReadFile(filepath.Join(dir, ".cached-at"))
	if err != nil {
		return time.Time{}, err
	}
	sec, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(sec, 0), nil
}

// ParamsKey returns a short, deterministic key derived from fields, used to
// name cache artifacts that depend on edit parameters (margin, speed, ...)
// rather than just the source file.
func ParamsKey(fields ...string) string {
	h := sha256.Sum256([]byte(strings.Join(fields, "|")))
	return hex.EncodeToString(h[:])[:12]
}
