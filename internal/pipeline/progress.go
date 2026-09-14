package pipeline

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// probeDuration returns the duration of a media file in seconds, used to
// turn ffmpeg's byte/time progress output into a fraction, and to
// estimate deep-filter's processing time (which reports no progress of
// its own).
func probeDuration(ffprobePath, path string) (float64, error) {
	cmd := exec.Command(ffprobePath, "-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path)
	out, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("probing duration of %s: %w", path, err)
	}
	d, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		return 0, fmt.Errorf("parsing duration of %s: %w", path, err)
	}
	return d, nil
}

// VideoInfo holds the properties of a video needed to show "original"
// values in the edit UI and to decide whether a resize/bitrate pass is a
// no-op.
type VideoInfo struct {
	Width       int `json:"width"`
	Height      int `json:"height"`
	BitrateKbps int `json:"bitrateKbps"` // 0 if ffprobe couldn't determine it (e.g. some containers omit stream bit_rate)
}

// ProbeVideoInfo reads width, height, and bitrate off a video's first
// video stream via ffprobe. Bitrate falls back to the container-level
// bit_rate (format.bit_rate) when the stream doesn't report its own, which
// happens for some inputs.
func ProbeVideoInfo(ffprobePath, path string) (VideoInfo, error) {
	cmd := exec.Command(ffprobePath, "-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,bit_rate:format=bit_rate",
		"-of", "default=noprint_wrappers=1", path)
	out, err := cmd.Output()
	if err != nil {
		return VideoInfo{}, fmt.Errorf("probing video info of %s: %w", path, err)
	}

	var info VideoInfo
	var streamBitrate, formatBitrate int
	bitRateSeen := 0
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		n, _ := strconv.Atoi(val)
		switch key {
		case "width":
			info.Width = n
		case "height":
			info.Height = n
		case "bit_rate":
			// -show_entries lists the stream section before the format
			// section, so the first bit_rate= line is the stream's own
			// (may be "N/A" -> 0), the second is the format-level fallback.
			bitRateSeen++
			if bitRateSeen == 1 {
				streamBitrate = n
			} else {
				formatBitrate = n
			}
		}
	}
	if streamBitrate > 0 {
		info.BitrateKbps = streamBitrate / 1000
	} else if formatBitrate > 0 {
		info.BitrateKbps = formatBitrate / 1000
	}
	return info, nil
}

// stageReporter maps one pipeline stage's local progress (0..1) into the
// overall job's progress and ETA, and emits it as a single formatted log
// line — giving a live-moving percentage, a countdown for the whole job
// (not just the current stage), and a "step N of M" counter.
type stageReporter struct {
	opts                     Options
	jobStart                 time.Time
	stepIndex, stepTotal     int
	label                    string
	overallStart, overallEnd float64
}

func (r *stageReporter) report(local float64) {
	isFinal := local >= 1
	if local < 0 {
		local = 0
	}
	if local > 1 {
		local = 1
	}
	overall := r.overallStart + local*(r.overallEnd-r.overallStart)
	// Never display 100% until the job is genuinely finished (local == 1
	// on the very last stage): a stage capped just short of "done" (e.g.
	// auto-editor's own progress bar reaching ~99% before its container
	// finalization actually completes) can still round up to "100%" at
	// %.0f precision on a narrow stage window, which looks exactly like
	// the stuck-progress bug this is meant to fix.
	if !isFinal && overall > 0.99 {
		overall = 0.99
	}

	eta := "estimating..."
	if overall > 0.02 {
		elapsed := time.Since(r.jobStart).Seconds()
		etaSeconds := elapsed / overall * (1 - overall)
		eta = "ETA ~" + time.Duration(etaSeconds*float64(time.Second)).Round(time.Second).String()
	}
	r.opts.logf("==> %s (step %d/%d, %.0f%% overall, %s)", r.label, r.stepIndex, r.stepTotal, overall*100, eta)
}

// runFFmpegWithProgress runs ffmpeg with the given args (which must
// include an output path as the last element), reporting real fractional
// progress against totalDuration by parsing ffmpeg's own
// "-progress pipe:1" machine-readable output. report may be nil.
func runFFmpegWithProgress(ffmpegPath string, args []string, totalDuration float64, report func(local float64)) error {
	fullArgs := append([]string{"-y", "-progress", "pipe:1", "-nostats"}, args...)
	cmd := exec.Command(ffmpegPath, fullArgs...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting ffmpeg: %w", err)
	}

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 4096), 1<<20)
	for scanner.Scan() {
		line := scanner.Text()
		val, ok := strings.CutPrefix(line, "out_time_us=")
		if !ok || totalDuration <= 0 || report == nil {
			continue
		}
		us, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			continue
		}
		report(float64(us) / 1e6 / totalDuration)
	}
	// Drain remaining stdout, if any, to avoid blocking Wait().
	io.Copy(io.Discard, stdout)

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w\n%s", err, stderrBuf.String())
	}
	return nil
}

// autoEditorPercentRe matches auto-editor's own progress bar lines, e.g.
// "⏳(mp4) h264+aac |███▊ | 25.4%  ETA 08:03 AM". auto-editor's ETA is a
// wall-clock time, which is awkward to turn back into a countdown
// reliably (AM/PM, midnight rollover); its percentage is real and
// accurate, so that's the only part reused — the caller derives its own
// ETA from overall job progress instead, consistent with every other
// stage.
var autoEditorPercentRe = regexp.MustCompile(`(\d+(?:\.\d+)?)%`)

// scanCarriageReturnLines behaves like bufio.Scanner's default line
// splitting, but also splits on '\r' — needed because auto-editor
// updates its progress bar in place with carriage returns rather than
// printing a new line each time.
func scanCarriageReturnLines(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[0:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
