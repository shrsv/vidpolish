package pipeline

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strconv"
)

// muxVideoAudio combines a (silent) video stream with a separate audio
// track into a single container for auto-editor to operate on. The audio
// is re-encoded to AAC (not a pure stream copy), so this is a real
// encoding pass worth reporting progress for, not an instant operation.
func muxVideoAudio(ffmpegPath, videoIn, audioIn, out string, duration float64, report func(local float64)) error {
	args := []string{
		"-i", videoIn, "-i", audioIn,
		"-map", "0:v", "-map", "1:a",
		"-c:v", "copy", "-c:a", "aac",
		"-shortest", out,
	}
	if err := runFFmpegWithProgress(ffmpegPath, args, duration, report); err != nil {
		return fmt.Errorf("ffmpeg mux failed: %w", err)
	}
	return nil
}

// autoEdit runs auto-editor on input, cutting silence, and writes the
// rendered result to output. margin is passed straight through as
// auto-editor's --margin (e.g. "0.2s"); speed, if not 1.0, speeds up the
// kept/spoken segments via --when-normal speed:N (silent segments are still
// fully cut). auto-editor prints its own real, accurate progress
// percentage as a carriage-return-updated line (e.g.
// "⏳(mp4) h264+aac |███▊ | 25.4%  ETA 08:03 AM"); report (if non-nil) is
// fed that percentage. Its own ETA is a wall-clock time, awkward to turn
// back into a reliable countdown, so it's not used — the caller derives
// an ETA from overall job progress instead, same as every other stage.
func autoEdit(autoEditorPath, input, output, margin string, speed float64, report func(local float64)) error {
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
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("auto-editor: %w", err)
	}
	cmd.Stderr = cmd.Stdout // combine, since auto-editor's progress stream isn't documented as stdout vs stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting auto-editor: %w", err)
	}

	var lastErrTail []byte
	lastReported := -1.0
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 4096), 1<<20)
	scanner.Split(scanCarriageReturnLines)
	for scanner.Scan() {
		line := scanner.Bytes()
		lastErrTail = append(lastErrTail[:0], line...) // keep the most recent line for error context
		if report == nil {
			continue
		}
		m := autoEditorPercentRe.FindSubmatch(line)
		if m == nil {
			continue
		}
		pct, err := strconv.ParseFloat(string(m[1]), 64)
		if err != nil || pct == lastReported {
			continue
		}
		lastReported = pct
		// auto-editor's own progress bar can reach 100% before the
		// process actually exits (container finalization still
		// pending), which would otherwise show as "100%" stuck for
		// several seconds — cap short of that until report(1.0) below,
		// once the process has genuinely finished.
		local := pct / 100
		if local > 0.99 {
			local = 0.99
		}
		report(local)
	}
	io.Copy(io.Discard, stdout)

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("auto-editor failed: %w\n%s", err, lastErrTail)
	}
	if report != nil {
		report(1.0)
	}
	return nil
}
