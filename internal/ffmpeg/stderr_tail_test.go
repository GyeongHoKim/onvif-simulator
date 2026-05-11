package ffmpeg

import (
	"reflect"
	"testing"
)

func TestStderrTail_PushBelowCapacity(t *testing.T) {
	t.Parallel()
	tail := newStderrTail(4)
	tail.push("a")
	tail.push("b")
	got := tail.snapshot()
	want := []string{"a", "b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("snapshot = %v, want %v", got, want)
	}
}

func TestStderrTail_WrapsAtCapacity(t *testing.T) {
	t.Parallel()
	tail := newStderrTail(3)
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		tail.push(s)
	}
	// Expect the last 3 lines in chronological order.
	got := tail.snapshot()
	want := []string{"c", "d", "e"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("snapshot after wrap = %v, want %v", got, want)
	}
}

func TestStderrTail_ZeroCapacityClampsToOne(t *testing.T) {
	t.Parallel()
	tail := newStderrTail(0)
	tail.push("a")
	tail.push("b")
	got := tail.snapshot()
	if len(got) != 1 || got[0] != "b" {
		t.Errorf("zero-capacity tail should clamp to 1; got %v", got)
	}
}

func TestStderrTail_EmptySnapshot(t *testing.T) {
	t.Parallel()
	tail := newStderrTail(4)
	if got := tail.snapshot(); len(got) != 0 {
		t.Errorf("expected empty snapshot, got %v", got)
	}
}
