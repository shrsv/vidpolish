package pipeline

import (
	"fmt"
	"os/exec"
)

// splitVideoAudio demuxes input into a video-only file (stream copy, no
// re-encode) and a mono 48kHz PCM wav (DeepFilterNet's expected input
// format).
func splitVideoAudio(ffmpegPath, input, videoOut, audioOut string) error {
	videoCmd := exec.Command(ffmpegPath, "-y", "-i", input,
		"-map", "0:v", "-c", "copy", videoOut)
	if out, err := videoCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg video split failed: %w\n%s", err, out)
	}

	audioCmd := exec.Command(ffmpegPath, "-y", "-i", input,
		"-map", "0:a", "-vn", "-ac", "1", "-ar", "48000", audioOut)
	if out, err := audioCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg audio split failed: %w\n%s", err, out)
	}

	return nil
}
