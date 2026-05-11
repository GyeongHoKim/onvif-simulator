//go:build !(linux && (amd64 || arm64 || arm)) && !(darwin && (amd64 || arm64)) && !(windows && (amd64 || arm64))

package ffmpeg

import "embed"

// binariesFS is empty on unsupported platforms. availableImpl reports
// ErrUnsupported on this build, so Open never reaches extractBinary.
//
//go:embed binaries/README.md
var binariesFS embed.FS
