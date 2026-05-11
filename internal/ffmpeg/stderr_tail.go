package ffmpeg

import "sync"

// stderrTail is a bounded ring buffer of recent stderr lines. It exists
// so a terminal ffmpeg failure can attach the most recent diagnostic
// context to the WARN log line without unbounded memory growth during
// normal noisy operation.
type stderrTail struct {
	mu    sync.Mutex
	lines []string
	cap   int
	pos   int
	full  bool
}

func newStderrTail(capacity int) *stderrTail {
	if capacity <= 0 {
		capacity = 1
	}
	return &stderrTail{
		lines: make([]string, capacity),
		cap:   capacity,
	}
}

func (t *stderrTail) push(line string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lines[t.pos] = line
	t.pos = (t.pos + 1) % t.cap
	if t.pos == 0 {
		t.full = true
	}
}

// snapshot returns the ring contents in chronological order.
func (t *stderrTail) snapshot() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.full {
		out := make([]string, t.pos)
		copy(out, t.lines[:t.pos])
		return out
	}
	out := make([]string, 0, t.cap)
	out = append(out, t.lines[t.pos:]...)
	out = append(out, t.lines[:t.pos]...)
	return out
}
