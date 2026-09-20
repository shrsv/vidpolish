package pipeline

import (
	"fmt"
	"strconv"
)

// resizeVideo re-encodes input's video stream to a new resolution and/or
// bitrate, copying audio through unchanged. width/height of 0 means "keep
// that dimension as ffmpeg's scale filter derives it"; when only one of
// the two is set the other is computed with -2 (even, aspect-preserving).
// bitrateKbps of 0 means "let the encoder pick its default quality"
// (CRF-like behavior) rather than a hard bitrate cap. encoder, if set (see
// hwaccel.go), re-encodes on a hardware encoder instead of software
// libx264; maxThreads, if >0, caps libx264's own -threads (ignored - and
// harmless to pass - for hardware encoders). The output always sets
// -movflags +faststart (moov atom moved to the front) so the browser
// preview player can start playing/seeking off the first bytes instead of
// needing an extra round-trip for metadata at the end of a large file.
func resizeVideo(ffmpegPath, input, output string, width, height, bitrateKbps int, encoder string, maxThreads int, duration float64, report func(local float64)) error {
	spec := videoEncoderArgs(encoder)
	args := append([]string{}, spec.preInput...)
	args = append(args, "-i", input)

	filter := ""
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
		filter = fmt.Sprintf("scale=%s:%s", w, h)
	}
	filter = mergeFilters(filter, spec.filter)
	if filter != "" {
		args = append(args, "-vf", filter)
	}

	if len(spec.codecArgs) > 0 {
		args = append(args, spec.codecArgs...)
	} else {
		args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p")
		if bitrateKbps == 0 {
			args = append(args, "-crf", "18", "-preset", "medium")
		}
	}
	if bitrateKbps > 0 {
		args = append(args, "-b:v", fmt.Sprintf("%dk", bitrateKbps))
	}
	if maxThreads > 0 {
		args = append(args, "-threads", strconv.Itoa(maxThreads))
	}
	args = append(args, "-c:a", "copy", "-movflags", "+faststart", output)

	if err := runFFmpegWithProgress(ffmpegPath, args, duration, report); err != nil {
		return fmt.Errorf("ffmpeg resize failed: %w", err)
	}
	return nil
}
