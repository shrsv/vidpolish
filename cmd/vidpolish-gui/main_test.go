package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestStatusCapturingWriterPassesThroughFlush guards against a regression
// where wrapping the handler for request logging silently breaks the SSE
// endpoints (cell run progress, YouTube login), which type-assert the
// ResponseWriter for http.Flusher and give up if that fails.
func TestStatusCapturingWriterPassesThroughFlush(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusCapturingWriter{ResponseWriter: rec, status: http.StatusOK}

	if _, ok := any(sw).(http.Flusher); !ok {
		t.Fatal("statusCapturingWriter does not implement http.Flusher")
	}
	sw.Flush()
	if !rec.Flushed {
		t.Fatal("expected Flush() to reach the underlying ResponseRecorder")
	}
}

func TestStatusCapturingWriterRecordsStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	sw := &statusCapturingWriter{ResponseWriter: rec, status: http.StatusOK}
	sw.WriteHeader(http.StatusNotFound)
	if sw.status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", sw.status)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("underlying recorder code = %d, want 404", rec.Code)
	}
}
