package pipeline

import "strconv"

// GeneratePreviewProxy transcodes input into a small, standardized,
// browser-safe MP4 at output: capped at 720p tall, a widely-supported
// H.264 Main-profile/yuv420p/AAC combination, and faststart. It's meant to
// be what the UI's <video> preview actually plays, never what editing or
// download operates on.
//
// This exists because a raw source recording can have quirks (odd
// dimensions not divisible by 16, unusual timestamp/GOP pacing, a
// profile/level combination the recorder's own encoder chose) that a
// browser's media pipeline can stall or hang on indefinitely under plain
// progressive download - confirmed on a real file that reproduced the
// same stuck-decode symptom in both an embedded WebView2 window and a
// full desktop browser, i.e. not a WebView2-specific bug. Re-encoding
// once into a deliberately unexceptional container/codec profile sidesteps
// whatever the raw file's specific issue was, rather than chasing it
// per-file. It also happens to be dramatically smaller (tens of MB instead
// of hundreds+), so it loads and seeks fast regardless.
//
// encoder, if set (see hwaccel.go), uses a hardware encoder instead of
// software libx264; maxThreads, if >0, caps libx264's own -threads.
// duration (input's length in seconds, 0 if unknown) drives report's real
// fractional progress (0..1), the same ffmpeg "-progress pipe:1" mechanism
// every other pipeline stage uses - without it, a several-minute source
// re-encoding at "generating preview..." with no percentage/ETA looks
// exactly like the app being stuck again, which is the whole thing this
// proxy step was added to stop happening.
func GeneratePreviewProxy(ffmpegPath, input, output, encoder string, maxThreads int, duration float64, report func(local float64)) error {
	spec := videoEncoderArgs(encoder)

	args := append([]string{}, spec.preInput...)
	args = append(args, "-i", input)

	filter := mergeFilters("scale=-2:min(720\\,ih)", spec.filter)
	args = append(args, "-vf", filter)

	if len(spec.codecArgs) > 0 {
		args = append(args, spec.codecArgs...)
	} else {
		args = append(args, "-c:v", "libx264", "-profile:v", "main", "-preset", "veryfast", "-crf", "23")
	}
	args = append(args, "-pix_fmt", "yuv420p")
	if maxThreads > 0 {
		args = append(args, "-threads", strconv.Itoa(maxThreads))
	}
	args = append(args, "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", output)

	return runFFmpegWithProgress(ffmpegPath, args, duration, report)
}
