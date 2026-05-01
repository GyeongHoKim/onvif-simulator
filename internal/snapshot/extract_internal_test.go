//go:build cgo

package snapshot

import (
	"testing"

	"github.com/asticode/go-astiav"
)

// TestDrainDecoder_EmptyDecoderReturnsFalse covers the drainDecoder helper.
// Triggering it through Extract requires a corner-case mp4 (clip shorter
// than the decoder's reorder buffer) that's tedious to vendor — here we
// drive the helper with an opened-but-never-fed H.264 decoder, which
// always returns (false, nil) because there is nothing in the queue.
func TestDrainDecoder_EmptyDecoderReturnsFalse(t *testing.T) {
	t.Parallel()

	codec := astiav.FindDecoder(astiav.CodecIDH264)
	if codec == nil {
		t.Skip("h264 decoder not available")
	}
	ctx := astiav.AllocCodecContext(codec)
	if ctx == nil {
		t.Fatal("alloc codec context returned nil")
	}
	defer ctx.Free()
	if err := ctx.Open(codec, nil); err != nil {
		t.Fatalf("open codec: %v", err)
	}
	frame := astiav.AllocFrame()
	defer frame.Free()

	got, err := drainDecoder(ctx, frame)
	if err != nil {
		t.Fatalf("drainDecoder: %v", err)
	}
	if got {
		t.Fatalf("drainDecoder reported a frame from an empty decoder")
	}
}
