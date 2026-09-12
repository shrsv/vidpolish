package server

import "sync"

// jobTracker prevents a cell from being run twice concurrently. It does
// not otherwise limit concurrency: running many cells in parallel (the
// whole point of the parallel edit/upload cells feature) is fine.
type jobTracker struct {
	mu      sync.Mutex
	running map[string]bool
}

func newJobTracker() *jobTracker {
	return &jobTracker{running: make(map[string]bool)}
}

// start marks cellID as running, returning false if it already was.
func (j *jobTracker) start(cellID string) bool {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.running[cellID] {
		return false
	}
	j.running[cellID] = true
	return true
}

// finish marks cellID as no longer running.
func (j *jobTracker) finish(cellID string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	delete(j.running, cellID)
}
