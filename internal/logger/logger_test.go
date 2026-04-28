package logger

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func newTestLogger(t *testing.T) (logger *Logger, dir string) {
	t.Helper()
	dir = t.TempDir()
	compress := false // keep tests fast and deterministic
	l, err := New(Options{
		Dir:        dir,
		Filename:   "test.log",
		MaxSizeMB:  1,
		MaxBackups: 5,
		MaxAgeDays: 1,
		Compress:   &compress,
		BufferSize: 4096, // generous so tests don't drop entries
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() }) //nolint:errcheck // test cleanup
	return l, dir
}

// writeAndSync enqueues each entry then polls Read() until the total reflects
// every write. This is robust against drain-goroutine timing AND surfaces
// buffer-overflow drops as test failures (rather than silent corruption).
func writeAndSync(t *testing.T, l *Logger, entries ...Entry) {
	t.Helper()
	for i := range entries {
		l.Write(entries[i])
	}
	want := len(entries)
	deadline := time.Now().Add(3 * time.Second)
	var lastTotal int
	for time.Now().Before(deadline) {
		_, total, err := l.Read(0, 0)
		if err != nil {
			t.Fatalf("Read while waiting for flush: %v", err)
		}
		lastTotal = total
		if total >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("drain did not flush %d entries in time (last total=%d, dropped=%d)",
		want, lastTotal, l.DroppedCount())
}

func TestWriteReadRoundTrip(t *testing.T) {
	l, _ := newTestLogger(t)

	t0 := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	writeAndSync(t, l,
		Entry{Time: t0, Kind: "event", Topic: "tns1:T1", Source: "S1", Payload: "p1"},
		Entry{Time: t0.Add(1 * time.Second), Kind: "mutation", Op: "AddProfile", Target: "tok", Detail: "d"},
		Entry{Time: t0.Add(2 * time.Second), Kind: "event", Topic: "tns1:T2", Source: "S2", Payload: "p2"},
	)

	got, total, err := l.Read(0, 100)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if total != 3 {
		t.Errorf("total=%d want 3", total)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	// Newest-first.
	if got[0].Topic != "tns1:T2" {
		t.Errorf("got[0].Topic=%q want tns1:T2", got[0].Topic)
	}
	if got[1].Op != "AddProfile" {
		t.Errorf("got[1].Op=%q want AddProfile", got[1].Op)
	}
	if got[2].Topic != "tns1:T1" {
		t.Errorf("got[2].Topic=%q want tns1:T1", got[2].Topic)
	}
}

func TestPagination(t *testing.T) {
	l, _ := newTestLogger(t)

	base := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	entries := make([]Entry, 50)
	for i := range entries {
		entries[i] = Entry{
			Time:    base.Add(time.Duration(i) * time.Second),
			Kind:    "event",
			Topic:   "tns1:T",
			Source:  "S",
			Payload: "i" + string(rune('0'+i%10)),
		}
	}
	writeAndSync(t, l, entries...)

	got, total, err := l.Read(10, 20)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if total != 50 {
		t.Errorf("total=%d want 50", total)
	}
	if len(got) != 20 {
		t.Fatalf("len=%d want 20", len(got))
	}
	// Newest-first means index 0 corresponds to entries[49]; offset 10 → entries[39].
	wantTime := entries[39].Time
	if !got[0].Time.Equal(wantTime) {
		t.Errorf("got[0].Time=%v want %v", got[0].Time, wantTime)
	}
}

func TestRotationReadsBackups(t *testing.T) {
	l, dir := newTestLogger(t)

	// MaxSize=1MB; write enough to force a rotation.
	big := strings.Repeat("x", 4096)
	base := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	entries := make([]Entry, 400)
	for i := range entries {
		entries[i] = Entry{
			Time:    base.Add(time.Duration(i) * time.Millisecond),
			Kind:    "event",
			Topic:   "tns1:T",
			Payload: big,
		}
	}
	writeAndSync(t, l, entries...)

	// Confirm at least one backup exists.
	matches, err := filepath.Glob(filepath.Join(dir, "test-*.log"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected at least one rotated backup; none found")
	}

	got, total, err := l.Read(0, 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if total != 400 {
		t.Errorf("total=%d want 400 (active + backup combined)", total)
	}
	if len(got) != 400 {
		t.Errorf("len=%d want 400", len(got))
	}
}

func TestClearRotatesAndPreservesBackup(t *testing.T) {
	l, dir := newTestLogger(t)

	t0 := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	writeAndSync(t, l,
		Entry{Time: t0, Kind: "event", Topic: "tns1:T1"},
		Entry{Time: t0.Add(time.Second), Kind: "event", Topic: "tns1:T2"},
	)

	if err := l.Clear(); err != nil {
		t.Fatalf("Clear: %v", err)
	}

	// Active file should be empty after Clear (backup retains content).
	active := filepath.Join(dir, "test.log")
	info, err := os.Stat(active)
	if err == nil && info.Size() != 0 {
		t.Errorf("active file size=%d after Clear; want 0", info.Size())
	}

	matches, err := filepath.Glob(filepath.Join(dir, "test-*.log"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected backup after Clear; none found")
	}
}

func TestWriteIsNonBlockingOnOverflow(t *testing.T) {
	dir := t.TempDir()
	compress := false
	l, err := New(Options{
		Dir:        dir,
		Filename:   "test.log",
		MaxSizeMB:  10,
		MaxBackups: 1,
		MaxAgeDays: 1,
		Compress:   &compress,
		BufferSize: 4, // tiny buffer to force overflow
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() }) //nolint:errcheck // test cleanup

	// Hammer the channel faster than the drain can process. Each Write must
	// return quickly even when the buffer is full.
	t0 := time.Date(2026, 4, 28, 10, 0, 0, 0, time.UTC)
	start := time.Now()
	for range 10_000 {
		l.Write(Entry{Time: t0, Kind: "event", Topic: "tns1:T"})
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("10_000 Writes took %v; should be sub-200ms (Write must not block)", elapsed)
	}

	if l.DroppedCount() == 0 {
		t.Errorf("expected DroppedCount > 0 with tiny buffer; got 0")
	}
}

func TestRFC3339NanoTimeFormat(t *testing.T) {
	l, dir := newTestLogger(t)

	t0 := time.Date(2026, 4, 28, 10, 0, 0, 123456789, time.UTC)
	writeAndSync(t, l, Entry{Time: t0, Kind: "event", Topic: "tns1:T"})

	// Read the raw active file and verify the time field is RFC3339Nano.
	data, err := os.ReadFile(filepath.Join(dir, "test.log"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	if !scanner.Scan() {
		t.Fatalf("no log line written")
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(scanner.Text()), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got, ok := raw["time"].(string)
	if !ok {
		t.Fatalf("time field missing or not a string: %v", raw["time"])
	}
	// RFC3339Nano: 2026-04-28T10:00:00.123456789Z
	rx := regexp.MustCompile(`^2026-04-28T10:00:00\.\d+Z$`)
	if !rx.MatchString(got) {
		t.Errorf("time=%q not RFC3339Nano", got)
	}
	// Round-trip parse.
	if _, err := time.Parse(time.RFC3339Nano, got); err != nil {
		t.Errorf("time=%q not parseable as RFC3339Nano: %v", got, err)
	}
}
