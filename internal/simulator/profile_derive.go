package simulator

import (
	"strings"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

// mjpegSiblingSuffix is appended to a user profile's Token (and Name) to
// produce the MJPEG sibling exposed via Profile S §7.9. The pair stays
// readable in client UIs: a Main profile becomes "Main" + "Main_MJPEG".
const (
	mjpegSiblingTokenSuffix = "_JPEG"
	mjpegSiblingNameSuffix  = " (MJPEG)"
)

// IsMJPEGSiblingToken reports whether token names an auto-generated MJPEG
// sibling rather than a user-declared profile. The handful of mediasvc
// code paths that need to discriminate (e.g. SetVideoEncoderConfiguration
// must not mutate persistence for a derived profile) use this predicate.
func IsMJPEGSiblingToken(token string) bool {
	return strings.HasSuffix(token, mjpegSiblingTokenSuffix)
}

// MJPEGSiblingParentToken returns the user profile token a sibling
// derives from, or the empty string when token is not a sibling.
func MJPEGSiblingParentToken(token string) string {
	if !IsMJPEGSiblingToken(token) {
		return ""
	}
	return strings.TrimSuffix(token, mjpegSiblingTokenSuffix)
}

// deriveMJPEGSiblings produces one in-memory MJPEG sibling per user
// profile with a usable source. The siblings copy width/height/fps from
// the (already-probed) parent so clients see consistent dimensions
// across both codecs; encoding is fixed at "MJPEG" so the media provider
// can pick the JPEG branch when answering GetVideoEncoderConfiguration.
// The returned slice is independent of the input — callers may mutate.
func deriveMJPEGSiblings(profiles []config.ProfileConfig) []config.ProfileConfig {
	out := make([]config.ProfileConfig, 0, len(profiles))
	for i := range profiles {
		src := profiles[i]
		if !src.HasSource() {
			continue
		}
		// Skip if a sibling already exists in the input. Defensive — the
		// simulator never persists siblings, but a caller that mistakenly
		// re-derives over an already-derived list should not double up.
		if IsMJPEGSiblingToken(src.Token) {
			continue
		}
		sibling := config.ProfileConfig{
			Name:             src.Name + mjpegSiblingNameSuffix,
			Token:            src.Token + mjpegSiblingTokenSuffix,
			Kind:             src.Kind, // shared source kind drives RTSP wiring
			MediaFilePath:    src.MediaFilePath,
			RPICam:           src.RPICam,
			Encoding:         "MJPEG",
			Width:            src.Width,
			Height:           src.Height,
			FPS:              src.FPS,
			SnapshotURI:      src.SnapshotURI,
			VideoSourceToken: src.VideoSourceToken,
			// Bitrate and GOPLength are encoder-specific to H264/H265 —
			// JPEG's quality is conveyed via VideoEncoderConfiguration's
			// Quality field, not bitrate. Leave them zero so the JPEG
			// VideoEncoderConfiguration omits them.
		}
		out = append(out, sibling)
	}
	return out
}
