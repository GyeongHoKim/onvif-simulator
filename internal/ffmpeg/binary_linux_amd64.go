//go:build linux && amd64

package ffmpeg

import "embed"

// Embed the whole platform subdirectory so the file is visible whether
// the worktree only carries the `placeholder` file (clean clone) or a
// real `ffmpeg` blob alongside it (after `just ffmpeg-fetch`). The
// runtime in binary.go reads the specific binaryName() filename and
// reports ErrUnsupported when only the placeholder is present.
//
//go:embed binaries/linux_amd64
var binariesFS embed.FS
