package simulator

import "sync"

// eventRing is a fixed-capacity, most-recent-first ring buffer of EventRecord.
type eventRing struct {
	mu   sync.Mutex
	buf  []EventRecord
	size int
	head int
	full bool
}

func newEventRing(size int) *eventRing {
	return &eventRing{size: size, buf: make([]EventRecord, size)}
}

// push appends an event, evicting the oldest when full.
func (r *eventRing) push(e EventRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.head] = e
	r.head = (r.head + 1) % r.size
	if r.head == 0 {
		r.full = true
	}
}

// snapshot returns the buffer content, most-recent-first.
func (r *eventRing) snapshot() []EventRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := r.head
	if r.full {
		n = r.size
	}
	out := make([]EventRecord, 0, n)
	for i := range n {
		idx := (r.head - 1 - i + r.size) % r.size
		out = append(out, r.buf[idx])
	}
	return out
}
