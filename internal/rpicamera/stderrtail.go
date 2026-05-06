package rpicamera

import "sync"

// stderrTail keeps the most recent N lines emitted by the mtxrpicam helper on
// its stderr stream. It is a ring buffer so a noisy helper cannot grow the
// simulator's memory footprint, and Snapshot returns lines in arrival order so
// a post-mortem WARN reads top-down like a tail of the helper log.
type stderrTail struct {
	mu    sync.Mutex
	cap   int
	buf   []string
	start int
	size  int
}

func newStderrTail(capacity int) *stderrTail {
	if capacity <= 0 {
		capacity = 1
	}
	return &stderrTail{cap: capacity, buf: make([]string, capacity)}
}

func (t *stderrTail) push(line string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.size < t.cap {
		t.buf[(t.start+t.size)%t.cap] = line
		t.size++
		return
	}
	t.buf[t.start] = line
	t.start = (t.start + 1) % t.cap
}

func (t *stderrTail) snapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, t.size)
	for i := range t.size {
		out[i] = t.buf[(t.start+i)%t.cap]
	}
	return out
}
