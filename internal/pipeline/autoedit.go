package pipeline

import (
	"fmt"
	"os/exec"
	"strconv"
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

// autoEdit runs auto-editor on input, cutting silence, and writes the
// rendered result to output. margin is passed straight through as
// auto-editor's --margin (e.g. "0.2s"); speed, if not 1.0, speeds up the
// kept/spoken segments via --when-normal speed:N (silent segments are still
// fully cut).
func autoEdit(autoEditorPath, input, output, margin string, speed float64) error {
	args := []string{input,
		"--edit", "audio",
		"--margin", margin,
		"--export", "default",
		"--output", output,
		"--no-open",
	}
	if speed != 1.0 {
		args = append(args, "--when-normal", "speed:"+strconv.FormatFloat(speed, 'f', -1, 64))
	}

	cmd := exec.Command(autoEditorPath, args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("auto-editor failed: %w\n%s", err, out)
	}
	return nil
}
