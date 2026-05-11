package ffmpeg

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// placeholderThreshold is the smallest valid ffmpeg binary size we expect
// from any usable build. Real release builds land in the 30–170 MiB
// range across platforms; 1 MiB is a comfortable lower bound that
// cleanly distinguishes placeholders (a few bytes) from real binaries.
const placeholderThreshold = 1 << 20 // 1 MiB

// goosWindows / executableMode are extracted as constants so the linter
// is happy and a future port can rename them in one place.
const (
	goosWindows    = "windows"
	executableMode = 0o755
)

// binariesFS is set by exactly one build-tag-gated file in this package
// (binary_<goos>_<goarch>.go) so the compiled binary only carries its
// target platform's ffmpeg blob, not the whole matrix. Other platforms
// fall through to a stub that always reports ErrUnsupported.
//
// The actual `//go:embed` directive lives in those per-platform files
// because go:embed paths must be statically resolvable at compile time.

// binaryName returns the embedded path for the running platform.
func binaryName() string {
	exe := "ffmpeg"
	if runtime.GOOS == goosWindows {
		exe = "ffmpeg.exe"
	}
	return fmt.Sprintf("binaries/%s_%s/%s", runtime.GOOS, runtime.GOARCH, exe)
}

// availableImpl is the implementation of public Available(). It checks
// the embedded blob size — a placeholder is too small to be a real
// ffmpeg binary, so we return ErrUnsupported rather than attempting to
// extract and exec a 1-byte file.
func availableImpl() error {
	path := binaryName()
	info, err := fs.Stat(binariesFS, path)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrUnsupported, path)
	}
	if info.Size() < placeholderThreshold {
		return fmt.Errorf("%w: %s is a placeholder (%d bytes); run scripts/fetch-ffmpeg.sh",
			ErrUnsupported, path, info.Size())
	}
	return nil
}

// extractBinary copies the embedded ffmpeg into a writable temporary
// directory, chmod's it executable (no-op on windows), and returns the
// path. The caller is responsible for removing the directory via the
// returned cleanup func when the Transcoder shuts down.
func extractBinary() (path string, cleanup func(), err error) {
	if availErr := availableImpl(); availErr != nil {
		return "", nil, availErr
	}

	data, err := binariesFS.ReadFile(binaryName())
	if err != nil {
		return "", nil, fmt.Errorf("ffmpeg: read embedded binary: %w", err)
	}

	dir, err := os.MkdirTemp("", "onvif-simulator-ffmpeg-*")
	if err != nil {
		return "", nil, fmt.Errorf("ffmpeg: create temp dir: %w", err)
	}
	cleanup = func() {
		_ = os.RemoveAll(dir) //nolint:errcheck // best-effort cleanup
	}

	exe := "ffmpeg"
	if runtime.GOOS == goosWindows {
		exe = "ffmpeg.exe"
	}
	out := filepath.Join(dir, exe)

	if writeErr := os.WriteFile(out, data, executableMode); writeErr != nil {
		cleanup()
		return "", nil, fmt.Errorf("ffmpeg: write binary to %s: %w", out, writeErr)
	}

	return out, cleanup, nil
}
