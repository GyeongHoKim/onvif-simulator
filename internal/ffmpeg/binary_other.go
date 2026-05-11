//go:build !(linux && (amd64 || arm64 || arm)) && !(darwin && (amd64 || arm64)) && !(windows && (amd64 || arm64))

package ffmpeg

import "embed"

// binariesFS carries only the binaries/README.md asset on unsupported
// platforms — no platform binary is embedded here. availableImpl reports
// ErrUnsupported on this build, so Open returns early and extractBinary is
// never invoked. The README embed exists solely so //go:embed has a valid
// path on every platform.
//
//go:embed binaries/README.md
var binariesFS embed.FS
