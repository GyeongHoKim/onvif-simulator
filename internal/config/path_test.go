package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

// TestResolveExplicitPathWins covers the CLI -config flag flow.
func TestResolveExplicitPathWins(t *testing.T) {
	t.Parallel()
	got, err := config.Resolve("/tmp/explicit/onvif-simulator.json")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "/tmp/explicit/onvif-simulator.json" {
		t.Fatalf("expected explicit override, got %q", got)
	}
}

// TestResolveFallsBackToDefaultPath covers the GUI .app double-click flow:
// no override → result must come from os.UserConfigDir.
func TestResolveFallsBackToDefaultPath(t *testing.T) {
	// Not parallel: t.Setenv is incompatible with t.Parallel.
	dir := t.TempDir()
	switch runtime.GOOS {
	case "linux":
		t.Setenv("XDG_CONFIG_HOME", dir)
	case "darwin":
		t.Setenv("HOME", dir)
	case "windows":
		t.Setenv("AppData", dir)
	default:
		t.Skipf("unsupported test platform %q", runtime.GOOS)
	}

	got, err := config.Resolve("")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("expected absolute path, got %q", got)
	}
	if filepath.Base(got) != config.FileName {
		t.Fatalf("expected basename %q, got %q", config.FileName, got)
	}
	if filepath.Base(filepath.Dir(got)) != config.DirName {
		t.Fatalf("expected parent dir %q, got %q", config.DirName, filepath.Dir(got))
	}
}

// TestEnsureExistsCreatesDefault covers the simulator's first-run flow.
func TestEnsureExistsCreatesDefault(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", config.FileName)

	created, err := config.EnsureExists(path)
	if err != nil {
		t.Fatalf("EnsureExists: %v", err)
	}
	if !created {
		t.Fatal("expected created=true on first run")
	}

	// File must exist and parse back to a valid Config.
	t.Cleanup(func() { config.SetPath("") })
	config.SetPath(path)
	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load after EnsureExists: %v", err)
	}
	if err := config.Validate(&got); err != nil {
		t.Fatalf("default config failed Validate: %v", err)
	}
}

// TestEnsureExistsLeavesExistingUntouched guards against EnsureExists
// clobbering a user-edited config file.
func TestEnsureExistsLeavesExistingUntouched(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, config.FileName)

	const sentinel = `{"keep":"me"}`
	if err := os.WriteFile(path, []byte(sentinel), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	created, err := config.EnsureExists(path)
	if err != nil {
		t.Fatalf("EnsureExists: %v", err)
	}
	if created {
		t.Fatal("expected created=false when file already exists")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != sentinel {
		t.Fatalf("file was overwritten: got %q", got)
	}
}

// TestSetPathRoundTrip exercises Save→Load through SetPath, the path GUI
// and TUI consumers will use.
func TestSetPathRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, config.FileName)
	t.Cleanup(func() { config.SetPath("") })
	config.SetPath(path)

	if got := config.Path(); got != path {
		t.Fatalf("Path: got %q, want %q", got, path)
	}

	want := config.Default()
	if err := config.Save(&want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at SetPath location: %v", err)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Device.UUID != want.Device.UUID {
		t.Fatalf("round-trip UUID mismatch: got %q, want %q", got.Device.UUID, want.Device.UUID)
	}
}

// TestDefaultPassesValidate locks in that the baseline config returned by
// Default is itself a valid configuration — otherwise EnsureExists would
// write a file the simulator immediately rejects.
func TestDefaultPassesValidate(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	if err := config.Validate(&cfg); err != nil {
		t.Fatalf("Default config failed Validate: %v", err)
	}
}

// TestEnsureExistsStatError covers the "exists but stat failed for another
// reason" branch by passing a path under a file (not a directory), which
// produces a not-a-directory error rather than ErrNotExist.
func TestEnsureExistsStatError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}

	// blocker/onvif-simulator.json — Stat returns ENOTDIR (not ErrNotExist).
	_, err := config.EnsureExists(filepath.Join(blocker, config.FileName))
	if err == nil {
		t.Fatal("expected stat error when parent is a regular file")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected non-ErrNotExist error, got %v", err)
	}
}
