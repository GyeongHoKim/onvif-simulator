//go:build windows && amd64

package ffmpeg

import "embed"

//go:embed binaries/windows_amd64
var binariesFS embed.FS
