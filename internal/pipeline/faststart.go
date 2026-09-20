package pipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RemuxFaststart rewrites the MP4/MOV container at path so its moov atom
// (the index a player needs before it can show duration, seek, or even
// paint a first frame) sits at the front instead of wherever the original
// writer left it - typically the very end for a raw, just-finished screen
// recording. This is a pure stream copy (-c copy, no re-encoding), so for
// a several-hundred-MB file it's an I/O-bound rewrite taking seconds, not
// the minutes a real re-encode would.
//
// Without this, a browser's <video> element has to make one or more Range
// requests near the end of the file just to locate the moov atom before
// it can report a duration or play anything at all - on a large file this
// can appear to hang indefinitely with no visible progress, which is
// exactly what an unhelpful still-loading spinner over an otherwise
// perfectly-uploaded video looks like.
//
// path is rewritten in place (via a temp file + rename, so a failure or
// interruption never leaves path itself corrupted); on any failure the
// original file is left untouched and the error is returned for the
// caller to log - not something an otherwise-successful upload should
// fail over. duration (input's length in seconds, 0 if unknown) drives
// report's real fractional progress (0..1), the same "-progress pipe:1"
// mechanism every other pipeline stage uses.
func RemuxFaststart(ffmpegPath, path string, duration float64, report func(local float64)) error {
	// The temp output must keep path's real extension (.mp4/.mov/...) -
	// ffmpeg picks its output muxer from the destination filename, and a
	// plain ".tmp" suffix leaves it unable to guess one at all ("Unable to
	// choose an output format"), failing the remux outright every time.
	ext := filepath.Ext(path)
	tmp := strings.TrimSuffix(path, ext) + ".faststart" + ext
	defer os.Remove(tmp) // no-op once renamed away

	// Runs immediately after the raw upload's big write was just closed -
	// on Windows, antivirus real-time scanning can hold a just-written
	// file locked for a brief moment, which would otherwise show up here
	// as an unexplained open/read failure. One short retry rides that out.
	var lastErr error
	for attempt := range 2 {
		if attempt > 0 {
			time.Sleep(500 * time.Millisecond)
		}
		args := []string{"-i", path, "-c", "copy", "-movflags", "+faststart", tmp}
		if err := runFFmpegWithProgress(ffmpegPath, args, duration, report); err != nil {
			lastErr = err
			continue
		}
		lastErr = nil
		break
	}
	if lastErr != nil {
		return lastErr
	}

	info, err := os.Stat(tmp)
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("faststart remux produced no output")
	}
	return os.Rename(tmp, path)
}
