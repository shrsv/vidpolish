// Package pipeline implements the vidpolish processing stages: split
// video/audio, denoise the audio with DeepFilterNet, then cut silence with
// auto-editor. Each stage shells out to an external binary resolved via
// internal/binmgr. Intermediate artifacts are kept in a persistent,
// content-addressed cache (internal/cache) so re-running with different
// edit parameters doesn't redo the expensive split/denoise/remux stages.
package pipeline

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"vidpolish/internal/binmgr"
	"vidpolish/internal/cache"
)

// Options configures a Process run.
type Options struct {
	Input     string  // path to the raw input video
	OutputDir string  // directory the final polished video is written to
	Margin    string  // auto-editor --margin value, e.g. "0.2s"
	Speed     float64 // playback speed multiplier for kept/spoken segments; 1.0 = unchanged
	NoCache   bool    // if true, ignore existing cache entries and recompute everything
}

const editMethod = "audio"

// Process runs the full split -> denoise -> auto-edit pipeline on
// opts.Input and returns the path to the final output file.
func Process(opts Options) (string, error) {
	if opts.Margin == "" {
		opts.Margin = "0.2s"
	}
	if opts.Speed == 0 {
		opts.Speed = 1.0
	}

	ffmpegPath, err := binmgr.Resolve(binmgr.FFmpeg)
	if err != nil {
		return "", err
	}
	deepFilterPath, err := binmgr.Resolve(binmgr.DeepFilter)
	if err != nil {
		return "", err
	}
	autoEditorPath, err := binmgr.Resolve(binmgr.AutoEditor)
	if err != nil {
		return "", err
	}

	if root, err := cache.Root(); err == nil {
		if err := cache.Sweep(root, cache.DefaultTTL); err != nil {
			fmt.Fprintf(os.Stderr, "vidpolish: cache sweep warning: %v\n", err)
		}
	}

	fp, err := cache.Fingerprint(opts.Input)
	if err != nil {
		return "", fmt.Errorf("fingerprinting input: %w", err)
	}
	dir, err := cache.Dir(fp)
	if err != nil {
		return "", err
	}
	fmt.Println("==> cache dir:", dir)

	videoOnly := filepath.Join(dir, "video.mp4")
	rawAudio := filepath.Join(dir, "audio.wav")
	denoisedDir := filepath.Join(dir, "denoised")
	remuxed := filepath.Join(dir, "remuxed.mp4")

	if opts.NoCache || !exists(videoOnly) || !exists(rawAudio) {
		fmt.Println("==> splitting video and audio")
		if err := splitVideoAudio(ffmpegPath, opts.Input, videoOnly, rawAudio); err != nil {
			return "", err
		}
	} else {
		fmt.Println("==> using cached split video/audio")
	}

	var denoisedAudio string
	if opts.NoCache || !dirHasWav(denoisedDir) {
		fmt.Println("==> denoising audio")
		denoisedAudio, err = denoise(deepFilterPath, rawAudio, denoisedDir)
		if err != nil {
			return "", err
		}
	} else {
		fmt.Println("==> using cached denoised audio")
		denoisedAudio, err = firstWavIn(denoisedDir)
		if err != nil {
			return "", err
		}
	}

	if opts.NoCache || !exists(remuxed) {
		fmt.Println("==> remuxing cleaned audio with video")
		if err := muxVideoAudio(ffmpegPath, videoOnly, denoisedAudio, remuxed); err != nil {
			return "", err
		}
	} else {
		fmt.Println("==> using cached remux")
	}

	if err := cache.Touch(dir); err != nil {
		fmt.Fprintf(os.Stderr, "vidpolish: cache touch warning: %v\n", err)
	}

	paramsKey := cache.ParamsKey(
		"margin="+opts.Margin,
		"speed="+strconv.FormatFloat(opts.Speed, 'f', -1, 64),
		"edit="+editMethod,
	)
	final := filepath.Join(dir, "final-"+paramsKey+".mp4")

	if opts.NoCache || !exists(final) {
		fmt.Println("==> cutting silence with auto-editor")
		if err := autoEdit(autoEditorPath, remuxed, final, opts.Margin, opts.Speed); err != nil {
			return "", err
		}
	} else {
		fmt.Println("==> using cached auto-edit result")
	}

	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}
	base := strings.TrimSuffix(filepath.Base(opts.Input), filepath.Ext(opts.Input))
	finalOut := filepath.Join(opts.OutputDir, base+"-polished.mp4")
	if err := copyFile(final, finalOut); err != nil {
		return "", fmt.Errorf("copying result to output dir: %w", err)
	}

	return finalOut, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dirHasWav(dir string) bool {
	files, err := wavFiles(dir)
	return err == nil && len(files) > 0
}

func firstWavIn(dir string) (string, error) {
	files, err := wavFiles(dir)
	if err != nil {
		return "", err
	}
	for name := range files {
		return filepath.Join(dir, name), nil
	}
	return "", fmt.Errorf("no wav file found in %s", dir)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
