package pipeline

import "fmt"

// splitVideoAudio demuxes input into a video-only file and a mono 48kHz
// PCM wav (DeepFilterNet's expected input format). report, if non-nil,
// receives real progress (0..1) parsed from ffmpeg's own machine-readable
// output, split evenly between the two passes (video split, then audio
// split).
//
// The video pass re-encodes rather than stream-copying, specifically to
// strip B-frames (-bf 0): auto-editor's muxer can fail outright ("Could
// not write packet: Invalid argument") on B-frame footage once a cut
// lands inside a GOP and reordered frames no longer decode cleanly.
// Encoding without B-frames makes decode order match presentation order,
// so every cut boundary is safe. This costs the "split is a pure stream
// copy" speed that a plain remux would have, but it's a one-time cost per
// source (cached, same as the other stages) and correctness wins over it.
func splitVideoAudio(ffmpegPath, input, videoOut, audioOut string, duration float64, report func(local float64)) error {
	videoArgs := []string{
		"-i", input, "-map", "0:v",
		"-c:v", "libx264", "-bf", "0", "-crf", "18", "-preset", "veryfast",
		videoOut,
	}
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
