package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"vidpolish/internal/config"
	"vidpolish/internal/procutil"
)

// hwEncoderSpec describes how to swap a hardware encoder into an ffmpeg
// command in place of software libx264: preInput args (device init, for
// vaapi), an extra -vf filter term to merge with any existing filter
// chain (vaapi needs its frames uploaded to the device), and the -c:v plus
// quality-equivalent flags themselves.
type hwEncoderSpec struct {
	preInput  []string
	filter    string
	codecArgs []string
}

var hwEncoders = map[string]hwEncoderSpec{
	"h264_nvenc":        {codecArgs: []string{"-c:v", "h264_nvenc", "-preset", "p4", "-rc", "vbr", "-cq", "18"}},
	"h264_qsv":          {codecArgs: []string{"-c:v", "h264_qsv", "-preset", "veryfast", "-global_quality", "18"}},
	"h264_videotoolbox": {codecArgs: []string{"-c:v", "h264_videotoolbox", "-q:v", "55"}},
	"h264_vaapi": {
		preInput:  []string{"-vaapi_device", "/dev/dri/renderD128"},
		filter:    "format=nv12,hwupload",
		codecArgs: []string{"-c:v", "h264_vaapi", "-qp", "20"},
	},
}

// candidatesByGOOS lists, per OS, which hardware encoders are worth
// probing and in what priority order. vaapi is the most failure-prone of
// the four (wrong render node, missing permissions) so it's tried after
// nvenc on Linux, not first.
var candidatesByGOOS = map[string][]string{
	"windows": {"h264_nvenc", "h264_qsv"},
	"linux":   {"h264_nvenc", "h264_vaapi", "h264_qsv"},
	"darwin":  {"h264_videotoolbox"},
}

// DetectBestEncoder probes ffmpegPath for a working hardware H.264
// encoder, returning its name (an hwEncoders key) or "" if none works -
// meaning stay on software libx264. Being listed in `ffmpeg -encoders`
// only means ffmpeg was compiled with support for an encoder, not that a
// working driver/device is actually present, so each candidate is
// confirmed with a real (tiny, few-frame) smoke-test encode rather than
// trusted on the listing alone.
func DetectBestEncoder(ffmpegPath string) (string, error) {
	candidates := candidatesByGOOS[runtime.GOOS]
	if len(candidates) == 0 {
		return "", nil
	}

	out, err := exec.Command(ffmpegPath, "-hide_banner", "-encoders").Output()
	if err != nil {
		return "", fmt.Errorf("listing ffmpeg encoders: %w", err)
	}
	compiled := string(out)

	for _, name := range candidates {
		if !strings.Contains(compiled, name) {
			continue
		}
		if smokeTestEncoder(ffmpegPath, name) {
			return name, nil
		}
	}
	return "", nil
}

// smokeTestEncoder confirms name actually opens and encodes on this
// machine, not just that ffmpeg was compiled with support for it.
func smokeTestEncoder(ffmpegPath, name string) bool {
	spec := hwEncoders[name]

	args := append([]string{"-y"}, spec.preInput...)
	args = append(args, "-f", "lavfi", "-i", "color=c=black:s=64x64:d=1")
	if spec.filter != "" {
		args = append(args, "-vf", spec.filter)
	}
	args = append(args, spec.codecArgs...)
	args = append(args, "-frames:v", "2", "-f", "null", "-")

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	procutil.HideWindow(cmd)
	return cmd.Run() == nil
}

// ffmpegIdentity is a cheap fingerprint (path + mtime) of the ffmpeg
// binary at path, used to tell whether a cached DetectBestEncoder result
// is still trustworthy or the binary has since changed (e.g. binmgr
// fetched a newer build).
func ffmpegIdentity(ffmpegPath string) (string, error) {
	info, err := os.Stat(ffmpegPath)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s@%d", ffmpegPath, info.ModTime().UnixNano()), nil
}

// ResolveEncoder decides which video encoder a Process run should use,
// consulting and updating cfg's cached probe result so DetectBestEncoder's
// smoke test only ever runs once per ffmpeg binary rather than on every
// job. cfg may be nil (e.g. the CLI running with no config.toml at all),
// in which case GPU acceleration is attempted but its result isn't
// persisted anywhere.
func ResolveEncoder(cfg *config.Config, ffmpegPath string) (string, error) {
	if cfg != nil && cfg.Performance.GPUAcceleration == "off" {
		return "", nil
	}

	identity, err := ffmpegIdentity(ffmpegPath)
	if err != nil {
		return "", err
	}

	if cfg != nil && cfg.Performance.DetectedForFFmpeg == identity {
		return cfg.Performance.DetectedEncoder, nil
	}

	encoder, err := DetectBestEncoder(ffmpegPath)
	if err != nil {
		return "", err
	}

	if cfg != nil {
		cfg.Performance.DetectedEncoder = encoder
		cfg.Performance.DetectedForFFmpeg = identity
		if err := config.Save(cfg); err != nil {
			return encoder, fmt.Errorf("saving detected encoder: %w", err)
		}
	}
	return encoder, nil
}

// InvalidateEncoder clears a cached hardware-encoder detection (e.g. after
// a run using it actually failed at runtime - a driver could have been
// uninstalled/disabled since the last probe), so the next ResolveEncoder
// call re-probes instead of repeatedly hitting the same broken encoder.
func InvalidateEncoder(cfg *config.Config) error {
	if cfg == nil {
		return nil
	}
	cfg.Performance.DetectedEncoder = ""
	cfg.Performance.DetectedForFFmpeg = ""
	return config.Save(cfg)
}

// videoEncoderArgs returns the ffmpeg args needed to select encoder in
// place of software libx264 (preInput args, an extra filter term to merge
// into any existing -vf chain, and the -c:v plus quality flags). encoder
// == "" (or unrecognized) returns the zero value, meaning "no change -
// caller's existing libx264 args apply".
func videoEncoderArgs(encoder string) hwEncoderSpec {
	return hwEncoders[encoder]
}

// mergeFilters joins two -vf filter chain fragments (either may be
// empty) with a comma, ffmpeg's filtergraph separator.
func mergeFilters(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "," + b
	}
}
