package server

import "sync"

// hub is a minimal per-cell publish/subscribe fan-out for Server-Sent
// Events: live progress lines and status transitions while a cell (or the
// YouTube login flow) is running.
type hub struct {
	mu   sync.Mutex
	subs map[string][]chan string
}

func newHub() *hub {
	return &hub{subs: make(map[string][]chan string)}
}

// Subscribe returns a channel that receives every message Published for
// key from now on, and a cancel func to stop receiving and release it.
func (h *hub) Subscribe(key string) (<-chan string, func()) {
	ch := make(chan string, 32)
	h.mu.Lock()
	h.subs[key] = append(h.subs[key], ch)
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		subs := h.subs[key]
		for i, c := range subs {
			if c == ch {
				h.subs[key] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		close(ch)
	}
	return ch, cancel
}

// Publish sends msg to every current subscriber of key, non-blocking
// (slow/gone subscribers just miss messages rather than stalling the
// publisher).
func (h *hub) Publish(key, msg string) {
	h.mu.Lock()
	subs := append([]chan string(nil), h.subs[key]...)
	h.mu.Unlock()

	for _, ch := range subs {
		select {
		case ch <- msg:
		default:
		}
	}
}
