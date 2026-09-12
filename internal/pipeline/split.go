package pipeline

import "fmt"

// splitVideoAudio demuxes input into a video-only file (stream copy, no
// re-encode) and a mono 48kHz PCM wav (DeepFilterNet's expected input
// format). report, if non-nil, receives real progress (0..1) parsed from
// ffmpeg's own machine-readable output, split evenly between the two
// passes (video split, then audio split).
func splitVideoAudio(ffmpegPath, input, videoOut, audioOut string, duration float64, report func(local float64)) error {
	videoArgs := []string{"-i", input, "-map", "0:v", "-c", "copy", videoOut}
	if err := runFFmpegWithProgress(ffmpegPath, videoArgs, duration, func(local float64) {
		if report != nil {
			report(local * 0.5)
		}
	}); err != nil {
		return fmt.Errorf("video split: %w", err)
	}

	audioArgs := []string{"-i", input, "-map", "0:a", "-vn", "-ac", "1", "-ar", "48000", audioOut}
	if err := runFFmpegWithProgress(ffmpegPath, audioArgs, duration, func(local float64) {
		if report != nil {
			report(0.5 + local*0.5)
		}
	}); err != nil {
		return fmt.Errorf("audio split: %w", err)
	}

	return nil
}
