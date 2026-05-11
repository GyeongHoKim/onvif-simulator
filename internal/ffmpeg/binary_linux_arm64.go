//go:build linux && arm64

package ffmpeg

import "embed"

//go:embed binaries/linux_arm64
var binariesFS embed.FS
