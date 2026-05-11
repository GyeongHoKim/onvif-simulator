//go:build windows && arm64

package ffmpeg

import "embed"

//go:embed binaries/windows_arm64
var binariesFS embed.FS
