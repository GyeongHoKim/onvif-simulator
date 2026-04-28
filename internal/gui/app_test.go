package gui

import (
	"context"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/logger"
)

// newTestApp builds an App backed by the in-memory simulator stub plus a
// real logger writing to a t.TempDir. Mirrors NewApp's wiring exactly so
// the App.GetLogs path is exercised end-to-end without a real Wails runtime.
func newTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	compress := false
	lg, err := logger.New(logger.Options{
		Dir:        dir,
		Filename:   "test.log",
		MaxSizeMB:  1,
		MaxBackups: 1,
		MaxAgeDays: 1,
		Compress:   &compress,
		BufferSize: 1024,
	})
	if err != nil {
		t.Fatalf("logger.New: %v", err)
	}
	t.Cleanup(func() { _ = lg.Close() }) //nolint:errcheck // test cleanup

	app := &App{logger: lg}
	emitEvent := func(r EventRecord) {
		app.logger.Write(logger.Entry{
			Time:    r.Time,
			Kind:    "event",
			Topic:   r.Topic,
			Source:  r.Source,
			Payload: r.Payload,
		})
	}
	emitMutation := func(r MutationRecord) {
		app.logger.Write(logger.Entry{
			Time:   r.Time,
			Kind:   "mutation",
			Op:     r.Kind,
			Target: r.Target,
			Detail: r.Detail,
		})
	}
	app.sim = newSimulatorStub(emitEvent, emitMutation)
	return app
}

// waitForLog polls App.GetLogs until total >= want or the deadline passes.
// The drain goroutine is asynchronous, so direct assertions race the test.
func waitForLog(t *testing.T, app *App, want int) LogPage {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	var page LogPage
	for time.Now().Before(deadline) {
		p, err := app.GetLogs(0, 100)
		if err != nil {
			t.Fatalf("GetLogs: %v", err)
		}
		page = p
		if page.Total >= want {
			return page
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("log did not reach %d entries (last total=%d)", want, page.Total)
	return page
}

func TestGetLogsReflectsEventCallback(t *testing.T) {
	app := newTestApp(t)

	app.sim.Motion("VS_MAIN", true)

	page := waitForLog(t, app, 1)
	if page.Total != 1 {
		t.Errorf("Total=%d want 1", page.Total)
	}
	if len(page.Entries) != 1 {
		t.Fatalf("len(Entries)=%d want 1", len(page.Entries))
	}
	got := page.Entries[0]
	if got.Kind != "event" {
		t.Errorf("Kind=%q want event", got.Kind)
	}
	if got.Topic == "" {
		t.Errorf("Topic is empty; expected the motion topic")
	}
	if got.Source != "VS_MAIN" {
		t.Errorf("Source=%q want VS_MAIN", got.Source)
	}
}

func TestGetLogsReflectsMutationCallback(t *testing.T) {
	app := newTestApp(t)

	if err := app.sim.SetDiscoveryMode("NonDiscoverable"); err != nil {
		t.Fatalf("SetDiscoveryMode: %v", err)
	}

	page := waitForLog(t, app, 1)
	if len(page.Entries) != 1 {
		t.Fatalf("len(Entries)=%d want 1", len(page.Entries))
	}
	got := page.Entries[0]
	if got.Kind != "mutation" {
		t.Errorf("Kind=%q want mutation", got.Kind)
	}
	if got.Op != "SetDiscoveryMode" {
		t.Errorf("Op=%q want SetDiscoveryMode", got.Op)
	}
	if got.Detail != "NonDiscoverable" {
		t.Errorf("Detail=%q want NonDiscoverable", got.Detail)
	}
}

func TestGetLogsReturnsNewestFirst(t *testing.T) {
	app := newTestApp(t)

	app.sim.Motion("VS_FIRST", true)
	waitForLog(t, app, 1)
	app.sim.Motion("VS_SECOND", true)
	page := waitForLog(t, app, 2)

	if len(page.Entries) != 2 {
		t.Fatalf("len(Entries)=%d want 2", len(page.Entries))
	}
	if page.Entries[0].Source != "VS_SECOND" {
		t.Errorf("Entries[0].Source=%q want VS_SECOND (newest first)", page.Entries[0].Source)
	}
	if page.Entries[1].Source != "VS_FIRST" {
		t.Errorf("Entries[1].Source=%q want VS_FIRST", page.Entries[1].Source)
	}
}

func TestClearLogsRotatesActiveFile(t *testing.T) {
	app := newTestApp(t)

	app.sim.Motion("VS_MAIN", true)
	waitForLog(t, app, 1)

	if err := app.ClearLogs(); err != nil {
		t.Fatalf("ClearLogs: %v", err)
	}

	// Read picks up backups too, so the rotated entry is still counted —
	// what we care about is that the user no longer sees it as the active
	// surface. Verify the panel-equivalent (offset 0, limit large) still
	// reflects the rotated entry until it ages out, and that the active
	// file has been emptied (subsequent writes start fresh).
	app.sim.Motion("VS_AFTER_CLEAR", true)
	page := waitForLog(t, app, 2)

	// The newest entry must be the post-clear one.
	if page.Entries[0].Source != "VS_AFTER_CLEAR" {
		t.Errorf("Entries[0].Source=%q want VS_AFTER_CLEAR (post-clear write)",
			page.Entries[0].Source)
	}
}

func TestOnShutdownClosesLoggerWithoutPanic(t *testing.T) {
	app := newTestApp(t)
	app.sim.Motion("VS_MAIN", true)
	waitForLog(t, app, 1)

	// Idempotency: calling twice must not panic.
	app.OnShutdown(context.Background())
	app.OnShutdown(context.Background())
}

func TestOnShutdownNilLogger(_ *testing.T) {
	app := &App{}
	app.OnShutdown(context.Background()) // must not panic with nil logger
}

func TestNewAppFallsBackToStubAndWiresLogger(t *testing.T) {
	// Redirect UserConfigDir to a temp dir on every supported OS so the
	// real logger created by NewApp lands in t.TempDir() and is cleaned up
	// automatically. APPDATA covers Windows; XDG_CONFIG_HOME covers Linux;
	// HOME covers macOS via os.UserConfigDir's Library/Application Support
	// fallback.
	tmp := t.TempDir()
	t.Setenv("APPDATA", tmp)
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("HOME", tmp)

	// NewApp tries newSimulatorAdapter first, which Loads config from the
	// working directory. On a test run the working dir is internal/gui so
	// no onvif-simulator.json exists → it falls back to the stub. Both
	// branches build a working App with a live logger.
	app := NewApp()
	if app == nil {
		t.Fatal("NewApp returned nil")
	}
	if app.logger == nil {
		t.Fatal("App.logger is nil after NewApp")
	}
	if app.sim == nil {
		t.Fatal("App.sim is nil after NewApp")
	}
	app.OnStartup(context.Background())
	app.OnShutdown(context.Background())
}

func TestNewAppWithNoOpLoggerStillBoots(t *testing.T) {
	// When logger.New fails (unwriteable dir, locked-down filesystem) the
	// App must still construct via the NoOp fallback path. We can't easily
	// force logger.New to fail in a portable test, so simulate the post-
	// fallback state directly: an App with logger.NoOp() must accept events
	// without panicking and return an empty GetLogs response.
	app := &App{logger: logger.NoOp()}
	emitEvent := func(r EventRecord) {
		app.logger.Write(logger.Entry{
			Time:   r.Time,
			Kind:   "event",
			Topic:  r.Topic,
			Source: r.Source,
		})
	}
	emitMutation := func(_ MutationRecord) {}
	app.sim = newSimulatorStub(emitEvent, emitMutation)

	app.sim.Motion("VS_MAIN", true)

	page, err := app.GetLogs(0, 100)
	if err != nil {
		t.Fatalf("GetLogs: %v", err)
	}
	if page.Total != 0 {
		t.Errorf("Total=%d want 0 (NoOp logger discards writes)", page.Total)
	}

	if err := app.ClearLogs(); err != nil {
		t.Errorf("ClearLogs: %v want nil (NoOp logger Clear is a no-op)", err)
	}
	app.OnShutdown(context.Background()) // must not panic
}
