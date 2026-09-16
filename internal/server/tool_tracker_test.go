package server

import "testing"

func TestToolTrackerLifecycle(t *testing.T) {
	tr := newToolTracker()

	if _, downloading := tr.snapshot("ffmpeg"); downloading {
		t.Fatal("expected not downloading before starting")
	}

	tr.starting("ffmpeg")
	if _, downloading := tr.snapshot("ffmpeg"); !downloading {
		t.Fatal("expected downloading immediately after starting")
	}

	tr.setBytes("ffmpeg", 10, 100)
	tr.setLog("ffmpeg", "downloading...")
	p, downloading := tr.snapshot("ffmpeg")
	if !downloading || p.Written != 10 || p.Total != 100 || p.Log != "downloading..." {
		t.Fatalf("unexpected snapshot: %+v downloading=%v", p, downloading)
	}

	tr.finished("ffmpeg", "")
	if _, downloading := tr.snapshot("ffmpeg"); downloading {
		t.Fatal("expected not downloading after finished with no error")
	}
	if _, ok := tr.lastError("ffmpeg"); ok {
		t.Fatal("expected no last error after a successful finish")
	}
}

func TestToolTrackerErrorPersistsAcrossPolls(t *testing.T) {
	tr := newToolTracker()

	tr.starting("resvg")
	tr.finished("resvg", "boom: network unreachable")

	// The error must still be there on a poll well after the download
	// ended - this is what lets the UI explain *why* a tool is still
	// missing after a failed attempt, instead of just reverting to the
	// generic "not downloaded yet" message.
	msg, ok := tr.lastError("resvg")
	if !ok || msg != "boom: network unreachable" {
		t.Fatalf("lastError = %q, %v; want the recorded error", msg, ok)
	}

	// Starting a new attempt clears the stale error immediately (not just
	// on success), so a retry-in-progress never shows the previous
	// failure's message.
	tr.starting("resvg")
	if _, ok := tr.lastError("resvg"); ok {
		t.Fatal("expected lastError cleared once a new attempt starts")
	}
}
