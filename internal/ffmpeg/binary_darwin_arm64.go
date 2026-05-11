//go:build darwin && arm64

package ffmpeg

import "embed"

//go:embed binaries/darwin_arm64
var binariesFS embed.FS
