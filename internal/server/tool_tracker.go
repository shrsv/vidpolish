package server

import "sync"

// toolProgress is a snapshot of one tool's in-flight download, kept only
// while it's actually running.
type toolProgress struct {
	Written int64
	Total   int64
	Log     string
}

// toolTracker holds live download progress for tools currently being
// fetched, plus the last error for a tool that failed. GET /api/tools
// polls this (see resolveToolStatuses) instead of streaming updates over
// SSE: a plain poll is what actually keeps working across a page
// navigation or a webview that doesn't reliably deliver a long-lived
// text/event-stream response, since each poll is just an ordinary
// request answered from server-side state rather than a client-held
// subscription that can silently die.
type toolTracker struct {
	mu       sync.Mutex
	progress map[string]toolProgress
	lastErr  map[string]string
}

func newToolTracker() *toolTracker {
	return &toolTracker{
		progress: make(map[string]toolProgress),
		lastErr:  make(map[string]string),
	}
}

// starting marks name as now downloading, with no progress yet, and
// clears any previous error for it.
func (t *toolTracker) starting(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.progress[name] = toolProgress{}
	delete(t.lastErr, name)
}

func (t *toolTracker) setLog(name, msg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.progress[name]
	p.Log = msg
	t.progress[name] = p
}

func (t *toolTracker) setBytes(name string, written, total int64) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := t.progress[name]
	p.Written = written
	p.Total = total
	t.progress[name] = p
}

// finished marks name as no longer downloading. errMsg is "" on success;
// on failure it's kept until the next starting() call so a poll after
// the tool disappears from "downloading" still explains why it's still
// missing instead of just silently reverting to "not downloaded yet".
func (t *toolTracker) finished(name string, errMsg string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.progress, name)
	if errMsg != "" {
		t.lastErr[name] = errMsg
	}
}

// snapshot returns name's current progress and whether it's downloading
// right now.
func (t *toolTracker) snapshot(name string) (toolProgress, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	p, ok := t.progress[name]
	return p, ok
}

// lastError returns the error from name's most recent failed attempt, if
// any, and whether there was one.
func (t *toolTracker) lastError(name string) (string, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	msg, ok := t.lastErr[name]
	return msg, ok
}
