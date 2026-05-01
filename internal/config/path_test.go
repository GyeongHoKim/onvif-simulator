package config_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

// TestDefaultProfilesIsEmptyArrayNotNull pins the JSON shape of
// Default(): media.profiles must marshal to "[]" rather than "null"
// so ONVIF clients (and json-schema-style consumers) always see an
// array. A nil slice would emit "null" and break that contract.
func TestDefaultProfilesIsEmptyArrayNotNull(t *testing.T) {
	t.Parallel()
	cfg := config.Default()
	if cfg.Media.Profiles == nil {
		t.Fatal("Default().Media.Profiles is nil; expected an empty slice")
	}
	if len(cfg.Media.Profiles) != 0 {
		t.Fatalf("Default().Media.Profiles is not empty: %+v", cfg.Media.Profiles)
	}
	data, err := json.Marshal(cfg.Media)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"profiles":[]`) {
		t.Errorf("expected profiles to marshal as [], got: %s", string(data))
	}
}

// TestDefaultUUIDIsUnique guards against the original hardcoded
// urn:uuid:00000000-0000-4000-8000-000000000001 sneaking back in. Two
// successive Default() calls must not collide; ONVIF clients (and
// WS-Discovery) treat the URN as a unique device identity.
func TestDefaultUUIDIsUnique(t *testing.T) {
	t.Parallel()
	a := config.Default().Device.UUID
	b := config.Default().Device.UUID
	if a == b {
		t.Fatalf("Default() returned the same UUID twice: %q", a)
	}
	const prefix = "urn:uuid:"
	for _, u := range []string{a, b} {
		if len(u) <= len(prefix) || u[:len(prefix)] != prefix {
			t.Errorf("UUID %q missing %q prefix", u, prefix)
		}
	}
}

// TestEnsureExistsConcurrentRaceSingleWriter guards the TOCTOU fix in
// EnsureExists: when N goroutines race to create the same path, exactly
// one must report created=true and the persisted file must contain that
// goroutine's content untouched. Before the fix this could silently
// overwrite, because writeAtomic used os.Rename (which clobbers).
func TestEnsureExistsConcurrentRaceSingleWriter(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, config.FileName)

	const n = 16
	created := make([]bool, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})
	for i := range n {
		go func(i int) {
			defer wg.Done()
			<-start
			created[i], errs[i] = config.EnsureExists(path)
		}(i)
	}
	close(start)
	wg.Wait()

	wins := 0
	for i, e := range errs {
		if e != nil {
			t.Errorf("goroutine %d: EnsureExists: %v", i, e)
		}
		if created[i] {
			wins++
		}
	}
	if wins != 1 {
		t.Errorf("expected exactly one winner, got %d", wins)
	}

	// File must be readable by Load (i.e. the winner's content survived).
	t.Cleanup(func() { config.SetPath("") })
	config.SetPath(path)
	if _, err := config.Load(); err != nil {
		t.Fatalf("Load after race: %v", err)
	}
}

// TestEnsureExistsStatError verifies EnsureExists surfaces an error when
// its target's parent is a regular file. The exact error class is
// platform-dependent (see comment inside) so the strict ENOTDIR-style
// assertion only runs where it applies.
func TestEnsureExistsStatError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed blocker: %v", err)
	}

	// The path points "under" a regular file. EnsureExists must surface a
	// non-nil error in this case across all OSes, but the failing call
	// differs:
	//
	//   Linux/macOS: os.Stat itself fails with ENOTDIR — distinct from
	//                ErrNotExist — exercising the stat-error branch in
	//                EnsureExists. We can assert on the error class.
	//   Windows:     os.Stat reports ErrNotExist for paths under a file,
	//                so EnsureExists falls through to writeTempFile, whose
	//                MkdirAll fails (Windows wraps that error as
	//                ErrNotExist as well). The function still returns an
	//                error, just via a different branch — so we only
	//                assert that an error came back.
	_, err := config.EnsureExists(filepath.Join(blocker, config.FileName))
	if err == nil {
		t.Fatal("expected error when parent is a regular file")
	}
	if runtime.GOOS == "windows" {
		return
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected non-ErrNotExist error on %s, got %v", runtime.GOOS, err)
	}
}
