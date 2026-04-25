package simulator

import (
	"testing"
	"time"
)

func TestRingFIFOOldestFirstOut(t *testing.T) {
	r := newEventRing(3)
	for i, topic := range []string{"a", "b", "c", "d"} {
		r.push(EventRecord{Time: time.Unix(int64(i), 0), Topic: topic})
	}
	got := r.snapshot()
	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(got))
	}
	wantOrder := []string{"d", "c", "b"} // most-recent-first
	for i, w := range wantOrder {
		if got[i].Topic != w {
			t.Fatalf("entry %d: want %s, got %s", i, w, got[i].Topic)
		}
	}
}

func TestRingPartialFill(t *testing.T) {
	r := newEventRing(8)
	r.push(EventRecord{Topic: "x"})
	r.push(EventRecord{Topic: "y"})
	got := r.snapshot()
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Topic != "y" || got[1].Topic != "x" {
		t.Fatalf("unexpected order: %+v", got)
	}
}
