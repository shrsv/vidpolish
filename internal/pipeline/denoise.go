package pipeline

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// denoise runs deep-filter on inputWav, writing its cleaned output into
// outDir, and returns the path to the resulting wav file. deep-filter names
// its output based on the model it ran (e.g. "<name>_DeepFilterNet3.wav"),
// so rather than hardcoding that suffix we just look for the wav file that
// shows up in outDir after the run.
func denoise(deepFilterPath, inputWav, outDir string) (string, error) {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", fmt.Errorf("creating denoise output dir: %w", err)
	}

	before, err := wavFiles(outDir)
	if err != nil {
		return "", err
	}

	cmd := exec.Command(deepFilterPath, inputWav, "-o", outDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("deep-filter failed: %w\n%s", err, out)
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
