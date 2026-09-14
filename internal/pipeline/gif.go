package pipeline

import (
	"bytes"
	"fmt"
	"os/exec"
)

// ExportGIF renders input to an animated GIF at output using ffmpeg's
// two-pass palette approach (palettegen+paletteuse): a naive direct GIF
// encode is limited to a fixed 256-color web-safe-ish palette and looks
// noticeably banded, while generating a palette from the actual clip
// first gives a much closer color match.
//
// fps controls playback smoothness vs. file size (lower = smaller);
// width resizes proportionally (height computed via -1 to preserve
// aspect ratio) and 0 keeps the input's original width.
func ExportGIF(ffmpegPath, input, output string, fps, width int) error {
	filter := fmt.Sprintf("fps=%d", fps)
	if width > 0 {
		filter += fmt.Sprintf(",scale=%d:-1:flags=lanczos", width)
	}
	filter += ",split[s0][s1];[s0]palettegen[p];[s1][p]paletteuse"

	cmd := exec.Command(ffmpegPath, "-y", "-i", input, "-vf", filter, output)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg gif export failed: %w\n%s", err, stderr.String())
	}
	return nil
}
