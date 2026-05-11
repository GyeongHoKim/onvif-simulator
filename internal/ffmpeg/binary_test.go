package ffmpeg

import (
	"errors"
	"runtime"
	"testing"
)

func TestAvailable_PlaceholderReturnsErrUnsupported(t *testing.T) {
	t.Parallel()
	// The placeholder binaries committed to git are 1 byte each, so the
	// availability check should return ErrUnsupported on every platform
	// where the project ships placeholders. CI swaps real binaries in
	// before release builds; this test confirms the placeholder gate.
	err := Available()
	if err == nil {
		t.Skip("real ffmpeg binary fetched in this environment; placeholder test is no-op")
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected ErrUnsupported wrapping, got %v", err)
	}
}

func TestBinaryName_ContainsPlatformSegment(t *testing.T) {
	t.Parallel()
	got := binaryName()
	wantContains := runtime.GOOS + "_" + runtime.GOARCH
	if !contains(got, wantContains) {
		t.Errorf("binaryName() = %q, expected to contain %q", got, wantContains)
	}
	if runtime.GOOS == "windows" && !contains(got, ".exe") {
		t.Errorf("binaryName() on windows = %q, expected .exe suffix", got)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
