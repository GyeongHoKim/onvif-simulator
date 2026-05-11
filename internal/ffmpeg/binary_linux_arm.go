//go:build linux && arm

package ffmpeg

import "embed"

//go:embed binaries/linux_arm
var binariesFS embed.FS
