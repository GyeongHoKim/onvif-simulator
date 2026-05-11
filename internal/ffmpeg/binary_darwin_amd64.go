//go:build darwin && amd64

package ffmpeg

import "embed"

//go:embed binaries/darwin_amd64
var binariesFS embed.FS
