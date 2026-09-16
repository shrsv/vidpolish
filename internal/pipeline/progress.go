package pipeline

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"vidpolish/internal/procutil"
)

// probeDuration returns the duration of a media file in seconds, used to
// turn ffmpeg's byte/time progress output into a fraction, and to
// estimate deep-filter's processing time (which reports no progress of
// its own).
func probeDuration(ffprobePath, path string) (float64, error) {
	cmd := exec.Command(ffprobePath, "-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", path)
	procutil.HideWindow(cmd)
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
// values and full media details in the UI, decide whether a resize/
// bitrate pass is a no-op, and estimate an edit's output size before
// running it.
type VideoInfo struct {
	Width            int     `json:"width"`
	Height           int     `json:"height"`
	BitrateKbps      int     `json:"bitrateKbps"`      // video stream bitrate; 0 if ffprobe couldn't determine it
	AudioBitrateKbps int     `json:"audioBitrateKbps"` // 0 if no audio stream or it didn't report a bitrate
	DurationSec      float64 `json:"durationSec"`
	FrameRate        float64 `json:"frameRate"`
	SizeBytes        int64   `json:"sizeBytes"`
}

// probeJSON is the shape of `ffprobe -of json` output this package reads
// from; only the fields ProbeVideoInfo needs are declared.
type probeJSON struct {
	Streams []struct {
		CodecType  string `json:"codec_type"`
		Width      int    `json:"width"`
		Height     int    `json:"height"`
		BitRate    string `json:"bit_rate"`
		RFrameRate string `json:"r_frame_rate"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
	} `json:"format"`
}

// ProbeVideoInfo reads a video's key properties via ffprobe: resolution,
// framerate, and video/audio bitrates off the respective streams (falling
// back to the container-level bit_rate when a stream doesn't report its
// own), plus duration from the container and file size from the
// filesystem directly (more reliable than the container's own declared
// size for a file that might still be mid-write).
func ProbeVideoInfo(ffprobePath, path string) (VideoInfo, error) {
	cmd := exec.Command(ffprobePath, "-v", "error",
		"-show_entries", "stream=codec_type,width,height,bit_rate,r_frame_rate:format=duration,bit_rate",
		"-of", "json", path)
	procutil.HideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return VideoInfo{}, fmt.Errorf("probing video info of %s: %w", path, err)
	}

	var probed probeJSON
	if err := json.Unmarshal(out, &probed); err != nil {
		return VideoInfo{}, fmt.Errorf("parsing ffprobe output for %s: %w", path, err)
	}

	var info VideoInfo
	for _, s := range probed.Streams {
		switch s.CodecType {
		case "video":
			if info.Width == 0 && info.Height == 0 {
				info.Width = s.Width
				info.Height = s.Height
				info.FrameRate = parseFrameRate(s.RFrameRate)
				info.BitrateKbps = atoiOr(s.BitRate, 0) / 1000
			}
		case "audio":
			if info.AudioBitrateKbps == 0 {
				info.AudioBitrateKbps = atoiOr(s.BitRate, 0) / 1000
			}
		}
	}
	if info.BitrateKbps == 0 {
		info.BitrateKbps = atoiOr(probed.Format.BitRate, 0) / 1000
	}
	info.DurationSec, _ = strconv.ParseFloat(probed.Format.Duration, 64)

	if stat, err := os.Stat(path); err == nil {
		info.SizeBytes = stat.Size()
	}

	return info, nil
}

// parseFrameRate turns ffprobe's r_frame_rate ("30/1", "30000/1001", or
// "0/0" when unknown) into a plain float.
func parseFrameRate(s string) float64 {
	num, den, ok := strings.Cut(s, "/")
	if !ok {
		return 0
	}
	n, errN := strconv.ParseFloat(num, 64)
	d, errD := strconv.ParseFloat(den, 64)
	if errN != nil || errD != nil || d == 0 {
		return 0
	}
	return n / d
}

// atoiOr parses s as an int, returning def if s is empty, "N/A", or
// otherwise unparseable — ffprobe reports missing numeric fields that way
// rather than omitting them.
func atoiOr(s string, def int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
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
	procutil.HideWindow(cmd)

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
		// ffmpeg opens/truncates its output file before it starts
		// encoding, so a failure partway through still leaves a
		// (typically empty) file behind at the output path. Remove it
		// so a later run doesn't mistake it for a valid cached result.
		if len(fullArgs) > 0 {
			os.Remove(fullArgs[len(fullArgs)-1])
		}
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
