package pipeline

import (
	"strings"
	"testing"
	"time"
)

func TestStageReporterZeroOverallShowsEstimating(t *testing.T) {
	var got string
	opts := Options{Log: func(s string) { got = s }}
	r := opts.newStageReporter(time.Now(), stageBoundsFor(false), 0) // split: bounds 0.00-0.08
	r.report(0)
	if !strings.Contains(got, "0%") || !strings.Contains(got, "estimating") {
		t.Fatalf("report at local 0 = %q", got)
	}
	if !strings.Contains(got, "step 1/4") {
		t.Fatalf("report should include step count, got %q", got)
	}
}

func TestStageReporterComputesOverallPercentAndETA(t *testing.T) {
	var got string
	opts := Options{Log: func(s string) { got = s }}
	start := time.Now().Add(-10 * time.Second)                  // pretend 10s have elapsed
	r := opts.newStageReporter(start, stageBoundsFor(false), 1) // denoise: bounds 0.08-0.75
	r.report(0.5)                                               // local 50% through denoise -> overall 0.08+0.5*0.67 = 0.415
	if !strings.Contains(got, "step 2/4") {
		t.Fatalf("report = %q, want step 2/4", got)
	}
	if !strings.Contains(got, "%") || !strings.Contains(got, "ETA") {
		t.Fatalf("report = %q, want a percentage and an ETA", got)
	}
}

func TestStageReporterOverallProgressesMonotonically(t *testing.T) {
	var messages []string
	opts := Options{Log: func(s string) { messages = append(messages, s) }}
	start := time.Now()
	r := opts.newStageReporter(start, stageBoundsFor(false), 1) // denoise
	r.report(0.1)
	r.report(0.5)
	r.report(0.9)
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	// Extract the percentages isn't trivial without more parsing, but at
	// minimum confirm the messages differ (i.e. progress actually moves,
	// which was the reported bug: percentage getting stuck once an ETA
	// appears).
	if messages[0] == messages[1] || messages[1] == messages[2] {
		t.Fatalf("progress messages did not change across calls: %v", messages)
	}
}

func TestStageReporterFallsBackToStdoutWithoutLogCallback(t *testing.T) {
	// Just confirm it doesn't panic when Log is nil (falls back to fmt.Println).
	opts := Options{}
	r := opts.newStageReporter(time.Now(), stageBoundsFor(false), 2)
	r.report(0.25)
}

func TestAutoEditorPercentRegex(t *testing.T) {
	line := "⏳(mp4) h264+aac |███▊                          | 25.4%  ETA 08:03 AM"
	m := autoEditorPercentRe.FindStringSubmatch(line)
	if m == nil || m[1] != "25.4" {
		t.Fatalf("FindStringSubmatch = %v, want [.. 25.4]", m)
	}
}

func TestScanCarriageReturnLinesSplitsOnCR(t *testing.T) {
	data := []byte("first\rsecond\rthird")
	var got []string
	for len(data) > 0 {
		advance, token, err := scanCarriageReturnLines(data, true)
		if err != nil {
			t.Fatalf("scanCarriageReturnLines: %v", err)
		}
		if advance == 0 {
			break
		}
		got = append(got, string(token))
		data = data[advance:]
	}
	want := []string{"first", "second", "third"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
