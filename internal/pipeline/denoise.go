package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"vidpolish/internal/procutil"
)

// assumedDenoiseRTF is a conservative (slightly pessimistic) estimate of
// deep-filter's real-time factor, i.e. seconds of processing per second
// of audio, used to drive a live progress estimate since deep-filter
// itself prints no progress at all while running (confirmed by running
// it and capturing its output: only a start log line and a single
// "done in Xs" summary at the very end). A real run on this project's
// test clip measured an RTF of ~0.11; 0.15 leaves headroom so the
// estimate doesn't finish before the process actually does.
const assumedDenoiseRTF = 0.15

// denoise runs deep-filter on inputWav, writing its cleaned output into
// outDir, and returns the path to the resulting wav file. deep-filter names
// its output based on the model it ran (e.g. "<name>_DeepFilterNet3.wav"),
// so rather than hardcoding that suffix we just look for the wav file that
// shows up in outDir after the run. Since deep-filter reports no progress
// of its own, report (if non-nil) is driven by a ticker extrapolating from
// duration and assumedDenoiseRTF — an estimate, not a measurement, and
// capped short of 100% until the process actually exits.
func denoise(deepFilterPath, inputWav, outDir string, duration float64, report func(local float64)) (string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("creating denoise output dir: %w", err)
	}

	before, err := wavFiles(outDir)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(deepFilterPath, inputWav, "-o", outDir)
	procutil.HideWindow(cmd)

	stop := make(chan struct{})
	if report != nil && duration > 0 {
		go func() {
			estTotal := duration * assumedDenoiseRTF
			start := time.Now()
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					local := time.Since(start).Seconds() / estTotal
					if local > 0.98 {
						local = 0.98
					}
					report(local)
				}
			}
		}()
	}

	out, err := cmd.CombinedOutput()
	close(stop)
	if err != nil {
		// deep-filter may have written a partial wav before failing;
		// remove anything new so a later run's dirHasWav cache check
		// doesn't mistake it for a completed denoise.
		if after, aferr := wavFiles(outDir); aferr == nil {
			if newFile := diffNewFile(before, after); newFile != "" {
				os.Remove(filepath.Join(outDir, newFile))
			}
		}
		return "", fmt.Errorf("deep-filter failed: %w\n%s", err, out)
	}
	if report != nil {
		report(1.0)
	}

	after, err := wavFiles(outDir)
	if err != nil {
		return "", err
	}

	newFile := diffNewFile(before, after)
	if newFile == "" {
		return "", fmt.Errorf("deep-filter did not produce a new wav file in %s", outDir)
	}
	return filepath.Join(outDir, newFile), nil
}

func wavFiles(dir string) (map[string]bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("listing %s: %w", dir, err)
	}
	found := make(map[string]bool)
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".wav" {
			found[e.Name()] = true
		}
	}
	return found, nil
}

func diffNewFile(before, after map[string]bool) string {
	for name := range after {
		if !before[name] {
			return name
		}
	}
	return ""
}
