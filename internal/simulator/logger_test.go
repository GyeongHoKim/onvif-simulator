package simulator

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/devicesvc"
)

// writeTestConfig drops a minimal valid config at a fresh temp path with the
// log file redirected into the same temp directory so simulator.New does not
// pollute the user's real cache directory. Returns (cfgPath, logPath).
func writeTestConfig(t *testing.T) (cfgPath, logPath string) {
	t.Helper()
	dir := t.TempDir()
	cfgPath = filepath.Join(dir, config.FileName)
	logPath = filepath.Join(dir, "simulator.log")
	cfg := config.Config{
		Version: config.CurrentVersion,
		Device: config.DeviceConfig{
			UUID:         "urn:uuid:00000000-0000-4000-8000-000000000099",
			Manufacturer: "Test",
			Model:        "SimCam",
			Serial:       "SN-Logger",
			Scopes:       []string{"onvif://www.onvif.org/name/logger-test"},
		},
		Network: config.NetworkConfig{HTTPPort: 18099},
		Media: config.MediaConfig{Profiles: []config.ProfileConfig{{
			Name: "main", Token: "profile_main",
		}}},
		Logging: config.LoggingConfig{File: logPath},
	}
	data, err := json.MarshalIndent(&cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return cfgPath, logPath
}

func TestNewWithoutLoggerAutoBuilds(t *testing.T) {
	cfgPath, logPath := writeTestConfig(t)
	prior := config.Path()
	t.Cleanup(func() { config.SetPath(prior) })

	// Logger left nil; simulator must auto-build a file logger from cfg.
	sim, err := New(Options{ConfigPath: cfgPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Stop(context.Background()) }) //nolint:errcheck // best-effort shutdown in tests

	if sim.rootLogger == nil || sim.logger == nil || sim.logState == nil {
		t.Fatal("simulator did not auto-build logger when Options.Logger was nil")
	}

	// The simulator emits "ready" on startup; verify it landed in the file.
	if err := sim.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	contents, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if !strings.Contains(string(contents), `"msg":"simulator: ready"`) {
		t.Errorf("ready record missing in log file: %s", contents)
	}
}

func TestNewExplicitLoggerOverrides(t *testing.T) {
	cfgPath, _ := writeTestConfig(t)
	prior := config.Path()
	t.Cleanup(func() { config.SetPath(prior) })

	captured := newCaptureHandler()
	root := slog.New(captured)

	sim, err := New(Options{ConfigPath: cfgPath, Logger: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Stop(context.Background()) }) //nolint:errcheck // best-effort shutdown in tests

	if sim.logState != nil {
		t.Error("simulator should not own LogState when Options.Logger is set")
	}
	if !captured.hasMessage("simulator: ready") {
		t.Errorf("ready record missing in injected logger: %v", captured.messages())
	}
	// Component attr must be on every record so the operator can filter.
	if !captured.hasAttr("simulator: ready", "component", "simulator") {
		t.Error("ready record missing component=simulator")
	}

	// Force an event publish so the broker's child logger fires with
	// component=event (disabled topic emits at debug).
	captured.reset()
	sim.broker.Publish("tns1:Device/Trigger/DigitalInput", `<tt:Message/>`)
	found := false
	for _, rec := range captured.snapshot() {
		if comp, ok := rec.attrs["component"]; ok && comp == "event" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("broker did not log with component=event: %v", captured.messages())
	}
}

func TestRequestMiddlewareWrapsAllSOAPHandlers(t *testing.T) {
	cfgPath, _ := writeTestConfig(t)
	prior := config.Path()
	t.Cleanup(func() { config.SetPath(prior) })

	captured := newCaptureHandler()
	sim, err := New(Options{ConfigPath: cfgPath, Logger: slog.New(captured)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Stop(context.Background()) }) //nolint:errcheck // best-effort shutdown in tests

	srv := buildHTTPServer(sim)
	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		devicesvc.DeviceServicePath,
		strings.NewReader(""),
	)
	srv.Handler.ServeHTTP(rr, req)

	if rr.Header().Get(obs.RequestIDHeader) == "" {
		t.Errorf("device endpoint missing request-id header: %v", rr.Header())
	}
	if !captured.hasMessage("soap_request") {
		t.Errorf("middleware summary log missing: %v", captured.messages())
	}
}

func TestApplyLoggingConfigOnReload(t *testing.T) {
	cfgPath, logPath := writeTestConfig(t)
	prior := config.Path()
	t.Cleanup(func() { config.SetPath(prior) })

	// Auto-built logger so the simulator owns the LogState that reload
	// mutates. No LogLevel override → cfg.Logging.Level wins.
	sim, err := New(Options{ConfigPath: cfgPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Stop(context.Background()) }) //nolint:errcheck // best-effort shutdown in tests

	// Mutate logging.level on disk and trigger a reload.
	mutated := sim.ConfigSnapshot()
	mutated.Logging.Level = "warn"
	out, marshalErr := json.MarshalIndent(&mutated, "", "  ")
	if marshalErr != nil {
		t.Fatalf("marshal: %v", marshalErr)
	}
	if writeErr := os.WriteFile(cfgPath, out, 0o600); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}

	if reloadErr := sim.reloadFromDisk(); reloadErr != nil {
		t.Fatalf("reloadFromDisk: %v", reloadErr)
	}
	if got := sim.logState.Level(); got != slog.LevelWarn {
		t.Errorf("LogState.Level=%v want warn", got)
	}

	// Emit one info and one warn; only warn should reach the file.
	sim.logger.Info("info-suppressed")
	sim.logger.Warn("warn-kept")

	if err := sim.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	contents, readErr := os.ReadFile(logPath)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	out2 := string(contents)
	if strings.Contains(out2, "info-suppressed") {
		t.Error("info record leaked through warn level after reload")
	}
	if !strings.Contains(out2, "warn-kept") {
		t.Errorf("warn record missing after reload: %s", out2)
	}
}

func TestReloadFromDiskInvalidJSON(t *testing.T) {
	cfgPath, _ := writeTestConfig(t)
	prior := config.Path()
	t.Cleanup(func() { config.SetPath(prior) })

	sim, err := New(Options{ConfigPath: cfgPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Stop(context.Background()) }) //nolint:errcheck // best-effort shutdown in tests

	if writeErr := os.WriteFile(cfgPath, []byte("{not-json"), 0o600); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}
	if reloadErr := sim.reloadFromDisk(); reloadErr == nil {
		t.Fatal("expected reload error for invalid config JSON")
	}
}

func TestReloadFromDiskApplyLoggingFails(t *testing.T) {
	cfgPath, logPath := writeTestConfig(t)
	prior := config.Path()
	t.Cleanup(func() { config.SetPath(prior) })

	sim, err := New(Options{ConfigPath: cfgPath})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Stop(context.Background()) }) //nolint:errcheck // best-effort shutdown in tests

	mutated := sim.ConfigSnapshot()
	mutated.Logging.File = "/bad\x00path/log.log"
	out, marshalErr := json.MarshalIndent(&mutated, "", "  ")
	if marshalErr != nil {
		t.Fatalf("marshal: %v", marshalErr)
	}
	if writeErr := os.WriteFile(cfgPath, out, 0o600); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}
	if reloadErr := sim.reloadFromDisk(); reloadErr != nil {
		t.Fatalf("reloadFromDisk: %v", reloadErr)
	}
	// Logging Apply fails but reload continues; prior log file should still exist.
	if _, statErr := os.Stat(logPath); statErr != nil {
		t.Fatalf("expected prior log file to remain: %v", statErr)
	}
}

func TestLogLevelOverrideStickyAcrossReload(t *testing.T) {
	cfgPath, logPath := writeTestConfig(t)
	prior := config.Path()
	t.Cleanup(func() { config.SetPath(prior) })

	// Operator pinned debug via flag/env; reload must not revert to cfg's
	// default info level.
	sim, err := New(Options{ConfigPath: cfgPath, LogLevel: "debug"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = sim.Stop(context.Background()) }) //nolint:errcheck // best-effort shutdown in tests

	if got := sim.logState.Level(); got != slog.LevelDebug {
		t.Fatalf("initial level=%v want debug", got)
	}

	// Mutate cfg.Logging.Level to error and reload — override should win.
	mutated := sim.ConfigSnapshot()
	mutated.Logging.Level = "error"
	out, marshalErr := json.MarshalIndent(&mutated, "", "  ")
	if marshalErr != nil {
		t.Fatalf("marshal: %v", marshalErr)
	}
	if writeErr := os.WriteFile(cfgPath, out, 0o600); writeErr != nil {
		t.Fatalf("write: %v", writeErr)
	}
	if reloadErr := sim.reloadFromDisk(); reloadErr != nil {
		t.Fatalf("reloadFromDisk: %v", reloadErr)
	}
	if got := sim.logState.Level(); got != slog.LevelDebug {
		t.Errorf("post-reload level=%v want debug (override should be sticky)", got)
	}
	_ = logPath // silence unused if we extend the test later
}

// captureHandler records every record passed through together with the
// attributes baked in via WithAttrs / WithGroup so tests can assert on
// attached fields like "component".
type captureHandler struct {
	mu       sync.Mutex
	parent   *captureHandler
	attrs    map[string]string
	records  *[]capturedRecord
	rootInit bool
}

type capturedRecord struct {
	level slog.Level
	msg   string
	attrs map[string]string
}

func newCaptureHandler() *captureHandler {
	recs := make([]capturedRecord, 0, 16)
	return &captureHandler{
		attrs:    map[string]string{},
		records:  &recs,
		rootInit: true,
	}
}

func (*captureHandler) Enabled(context.Context, slog.Level) bool { return true }

//nolint:gocritic // slog.Handler interface mandates value receiver
func (c *captureHandler) Handle(_ context.Context, r slog.Record) error {
	merged := make(map[string]string, len(c.attrs))
	for k, v := range c.attrs {
		merged[k] = v
	}
	r.Attrs(func(a slog.Attr) bool {
		merged[a.Key] = a.Value.String()
		return true
	})
	c.mu.Lock()
	*c.records = append(*c.records, capturedRecord{level: r.Level, msg: r.Message, attrs: merged})
	c.mu.Unlock()
	return nil
}

func (c *captureHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make(map[string]string, len(c.attrs)+len(attrs))
	for k, v := range c.attrs {
		merged[k] = v
	}
	for _, a := range attrs {
		merged[a.Key] = a.Value.String()
	}
	return &captureHandler{parent: c, attrs: merged, records: c.records}
}

func (c *captureHandler) WithGroup(string) slog.Handler { return c }

func (c *captureHandler) snapshot() []capturedRecord {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]capturedRecord, len(*c.records))
	copy(out, *c.records)
	return out
}

func (c *captureHandler) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	*c.records = (*c.records)[:0]
}

func (c *captureHandler) messages() []string {
	snap := c.snapshot()
	out := make([]string, 0, len(snap))
	for _, r := range snap {
		out = append(out, r.msg)
	}
	return out
}

func (c *captureHandler) hasMessage(msg string) bool {
	for _, r := range c.snapshot() {
		if r.msg == msg {
			return true
		}
	}
	return false
}

func (c *captureHandler) hasAttr(msg, key, value string) bool {
	for _, r := range c.snapshot() {
		if r.msg != msg {
			continue
		}
		if v, ok := r.attrs[key]; ok && v == value {
			return true
		}
	}
	return false
}
