// Package pipeline implements the vidpolish processing stages: split
// video/audio, denoise the audio with DeepFilterNet, then cut silence with
// auto-editor. Each stage shells out to an external binary resolved via
// internal/binmgr.
package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"vidpolish/internal/binmgr"
)

// Options configures a Process run.
type Options struct {
	Input     string // path to the raw input video
	OutputDir string // directory the final polished video is written to
	KeepTemp  bool   // if true, don't delete the intermediate working dir
}

// Process runs the full split -> denoise -> auto-edit pipeline on
// opts.Input and returns the path to the final output file.
func Process(opts Options) (string, error) {
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

	workDir, err := os.MkdirTemp("", "vidpolish-*")
	if err != nil {
		return "", fmt.Errorf("creating work dir: %w", err)
	}
	if !opts.KeepTemp {
		defer os.RemoveAll(workDir)
	} else {
		fmt.Fprintf(os.Stderr, "vidpolish: keeping temp dir %s\n", workDir)
	}

	videoOnly := filepath.Join(workDir, "video.mp4")
	rawAudio := filepath.Join(workDir, "audio.wav")

	fmt.Println("==> splitting video and audio")
	if err := splitVideoAudio(ffmpegPath, opts.Input, videoOnly, rawAudio); err != nil {
		return "", err
	}

	fmt.Println("==> denoising audio")
	denoisedDir := filepath.Join(workDir, "denoised")
	denoisedAudio, err := denoise(deepFilterPath, rawAudio, denoisedDir)
	if err != nil {
		return "", err
	}

	fmt.Println("==> remuxing cleaned audio with video")
	remuxed := filepath.Join(workDir, "remuxed.mp4")
	if err := muxVideoAudio(ffmpegPath, videoOnly, denoisedAudio, remuxed); err != nil {
		return "", err
	}

	if err := os.MkdirAll(opts.OutputDir, 0o755); err != nil {
		return "", fmt.Errorf("creating output dir: %w", err)
	}
	base := strings.TrimSuffix(filepath.Base(opts.Input), filepath.Ext(opts.Input))
	finalOut := filepath.Join(opts.OutputDir, base+"-polished.mp4")

	fmt.Println("==> cutting silence with auto-editor")
	if err := autoEdit(autoEditorPath, remuxed, finalOut); err != nil {
		return "", err
	}

	return finalOut, nil
}
