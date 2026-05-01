package snapshot

import "errors"

// ErrNoVideoStream means the input file has no decodable video stream we can
// snapshot from. Defined in both the cgo and no-cgo builds because callers
// outside this package check it via errors.Is.
var ErrNoVideoStream = errors.New("snapshot: no decodable video stream")

// ErrSnapshotUnsupported is returned by Extract when the binary was built
// with CGO_ENABLED=0 (the goreleaser cross-compile path used for CLI release
// artifacts). The simulator falls back to "no snapshot available" for
// profiles that would otherwise auto-derive their URL from MediaFilePath.
//
// Local builds via `make cli` / `make gui` enable cgo by default and link
// libavcodec at build time, so this error never surfaces there.
var ErrSnapshotUnsupported = errors.New("snapshot: cgo support not compiled in")
