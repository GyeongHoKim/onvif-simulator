package rpicamera

import (
	"reflect"
	"testing"
)

func TestStderrTailBelowCapacity(t *testing.T) {
	t.Parallel()
	tail := newStderrTail(4)
	tail.push("a")
	tail.push("b")
	if got, want := tail.snapshot(), []string{"a", "b"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot=%v want=%v", got, want)
	}
}

func TestStderrTailWrapsAroundCapacity(t *testing.T) {
	t.Parallel()
	tail := newStderrTail(3)
	for _, line := range []string{"a", "b", "c", "d", "e"} {
		tail.push(line)
	}
	if got, want := tail.snapshot(), []string{"c", "d", "e"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot=%v want=%v", got, want)
	}
}

func TestStderrTailZeroCapacityPromotesToOne(t *testing.T) {
	t.Parallel()
	tail := newStderrTail(0)
	tail.push("x")
	tail.push("y")
	if got, want := tail.snapshot(), []string{"y"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot=%v want=%v", got, want)
	}
}

func TestStderrTailSnapshotIsCopy(t *testing.T) {
	t.Parallel()
	tail := newStderrTail(2)
	tail.push("a")
	snap := tail.snapshot()
	tail.push("b")
	tail.push("c")
	if got, want := snap, []string{"a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot mutated under writer: %v want %v", got, want)
	}
}
