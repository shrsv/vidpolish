package pipeline

import (
	"fmt"
	"os/exec"
)

// muxVideoAudio combines a (silent) video stream with a separate audio
// track into a single container for auto-editor to operate on, without
// re-encoding either stream.
func muxVideoAudio(ffmpegPath, videoIn, audioIn, out string) error {
	cmd := exec.Command(ffmpegPath, "-y",
		"-i", videoIn, "-i", audioIn,
		"-map", "0:v", "-map", "1:a",
		"-c:v", "copy", "-c:a", "aac",
		"-shortest", out)
	if o, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg mux failed: %w\n%s", err, o)
	}
	return nil
}

// autoEdit runs auto-editor on input, cutting silence with sensible
// defaults for talking-head recordings, and writes the rendered result to
// output.
func autoEdit(autoEditorPath, input, output string) error {
	cmd := exec.Command(autoEditorPath, input,
		"--edit", "audio",
		"--margin", "0.15sec",
		"--export", "default",
		"--output", output,
		"--no-open",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("auto-editor failed: %w\n%s", err, out)
	}
	return nil
}
