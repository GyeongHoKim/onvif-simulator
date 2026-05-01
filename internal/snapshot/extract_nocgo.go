//go:build !cgo

package snapshot

// Supported reports whether this binary can decode mp4 keyframes locally.
// False for CGO_ENABLED=0 builds (e.g. goreleaser cross-compiled CLI
// release artifacts). The simulator still runs everything else — RTSP,
// ONVIF SOAP services, WS-Discovery — but GetSnapshotUri behaves as if
// no MediaFilePath were configured.
const Supported = false

// Extract is a stub for non-cgo builds. The full libavcodec-backed
// implementation lives in extract_cgo.go and is selected automatically
// when CGO_ENABLED=1.
func Extract(_ string) ([]byte, error) {
	return nil, ErrSnapshotUnsupported
}
