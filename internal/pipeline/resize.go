package pipeline

import "fmt"

// resizeVideo re-encodes input's video stream to a new resolution and/or
// bitrate, copying audio through unchanged. width/height of 0 means "keep
// that dimension as ffmpeg's scale filter derives it"; when only one of
// the two is set the other is computed with -2 (even, aspect-preserving).
// bitrateKbps of 0 means "let libx264 pick its default quality" (CRF-like
// behavior) rather than a hard bitrate cap.
func resizeVideo(ffmpegPath, input, output string, width, height, bitrateKbps int, duration float64, report func(local float64)) error {
	args := []string{"-i", input}

	if width > 0 || height > 0 {
		// libx264 with yuv420p requires even dimensions; an explicit
		// width/height from the caller (e.g. an aspect-locked UI field)
		// can land on an odd value, so round it down to the nearest even
		// number instead of passing it straight to ffmpeg.
		w, h := "-2", "-2"
		if width > 0 {
			w = fmt.Sprintf("trunc(%d/2)*2", width)
		}
		if height > 0 {
			h = fmt.Sprintf("trunc(%d/2)*2", height)
		}
		args = append(args, "-vf", fmt.Sprintf("scale=%s:%s", w, h))
	}

	args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p")
	if bitrateKbps > 0 {
		args = append(args, "-b:v", fmt.Sprintf("%dk", bitrateKbps))
	} else {
		args = append(args, "-crf", "18", "-preset", "medium")
	}
	args = append(args, "-c:a", "copy", output)

	if err := runFFmpegWithProgress(ffmpegPath, args, duration, report); err != nil {
		return fmt.Errorf("ffmpeg resize failed: %w", err)
	}
	return nil
}
