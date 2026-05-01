// Package snapshot extracts a single JPEG frame from a local mp4 file and
// serves it via an HTTP endpoint that the embedded ONVIF simulator hosts on
// its main HTTP listener.
//
// The simulator decodes one keyframe per profile at startup (via go-astiav /
// libavcodec) and caches the resulting JPEG bytes in memory. GetSnapshotUri
// returns http://<host>:<httpport>/onvif/snapshot/<token>.jpg, and a GET on
// that URL replays the cached bytes — no per-request decoding cost.
//
// Extract is the only piece of this package that touches ffmpeg; everything
// else is pure Go and can be linted/tested without the cgo dependency. Tests
// that exercise the decode path live alongside testdata/sample.mp4.
package snapshot
