//go:build cgo

package snapshot

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/asticode/go-astiav"
)

// Supported reports whether this binary can decode mp4 keyframes locally.
// True when built with CGO_ENABLED=1 (the default for `make cli`/`make gui`).
const Supported = true

// errAllocFormatContext means libavformat refused to allocate an
// AVFormatContext — practically only happens under OOM.
var errAllocFormatContext = errors.New("snapshot: alloc format context")

// jpegQuality is the encoder quality used for cached JPEGs. 80 is the
// usual sweet spot — visually indistinguishable from the source frame for
// camera-style content while keeping the byte payload small enough to ship
// over slow management networks.
const jpegQuality = 80

// Extract decodes the first decodable video frame from the mp4 file at path
// and returns it as JPEG bytes. The decode pipeline is:
//
//	mp4 → libavformat demux → libavcodec decode → libswscale to RGBA → image/jpeg
//
// All ffmpeg objects (format context, codec context, packet, frame, sws
// context) are owned by this function and freed before it returns.
func Extract(path string) ([]byte, error) {
	silenceFFmpegLogs()

	pkt := astiav.AllocPacket()
	defer pkt.Free()
	srcFrame := astiav.AllocFrame()
	defer srcFrame.Free()
	dstFrame := astiav.AllocFrame()
	defer dstFrame.Free()

	fc := astiav.AllocFormatContext()
	if fc == nil {
		return nil, errAllocFormatContext
	}
	defer fc.Free()

	if err := fc.OpenInput(path, nil, nil); err != nil {
		return nil, fmt.Errorf("snapshot: open %s: %w", path, err)
	}
	defer fc.CloseInput()

	if err := fc.FindStreamInfo(nil); err != nil {
		return nil, fmt.Errorf("snapshot: find stream info: %w", err)
	}

	stream, decCtx, err := openVideoDecoder(fc)
	if err != nil {
		return nil, err
	}
	defer decCtx.Free()

	frame, err := readFirstFrame(fc, decCtx, pkt, srcFrame, stream.Index())
	if err != nil {
		return nil, err
	}

	img, err := frameToRGBAImage(frame, dstFrame)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if encErr := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); encErr != nil {
		return nil, fmt.Errorf("snapshot: encode jpeg: %w", encErr)
	}
	return buf.Bytes(), nil
}

// openVideoDecoder finds the first video stream and returns an opened decoder
// context for it. The caller owns and frees the returned CodecContext.
func openVideoDecoder(fc *astiav.FormatContext) (*astiav.Stream, *astiav.CodecContext, error) {
	for _, s := range fc.Streams() {
		params := s.CodecParameters()
		if params.MediaType() != astiav.MediaTypeVideo {
			continue
		}
		codec := astiav.FindDecoder(params.CodecID())
		if codec == nil {
			continue
		}
		ctx := astiav.AllocCodecContext(codec)
		if ctx == nil {
			continue
		}
		if err := params.ToCodecContext(ctx); err != nil {
			ctx.Free()
			return nil, nil, fmt.Errorf("snapshot: copy codec params: %w", err)
		}
		if err := ctx.Open(codec, nil); err != nil {
			ctx.Free()
			return nil, nil, fmt.Errorf("snapshot: open codec: %w", err)
		}
		return s, ctx, nil
	}
	return nil, nil, ErrNoVideoStream
}

// readFirstFrame pumps packets from fc into the decoder until ReceiveFrame
// returns a frame, then returns that frame. The packet and frame buffers are
// caller-owned; this function unrefs them on every iteration.
func readFirstFrame(
	fc *astiav.FormatContext,
	decCtx *astiav.CodecContext,
	pkt *astiav.Packet,
	frame *astiav.Frame,
	streamIndex int,
) (*astiav.Frame, error) {
	for {
		if err := fc.ReadFrame(pkt); err != nil {
			if errors.Is(err, astiav.ErrEof) {
				if got, drainErr := drainDecoder(decCtx, frame); drainErr != nil {
					return nil, drainErr
				} else if got {
					return frame, nil
				}
				return nil, ErrNoVideoStream
			}
			return nil, fmt.Errorf("snapshot: read frame: %w", err)
		}

		if pkt.StreamIndex() != streamIndex {
			pkt.Unref()
			continue
		}

		err := decCtx.SendPacket(pkt)
		pkt.Unref()
		if err != nil {
			return nil, fmt.Errorf("snapshot: send packet: %w", err)
		}

		recvErr := decCtx.ReceiveFrame(frame)
		if recvErr == nil {
			return frame, nil
		}
		if errors.Is(recvErr, astiav.ErrEagain) {
			continue
		}
		if errors.Is(recvErr, astiav.ErrEof) {
			return nil, ErrNoVideoStream
		}
		return nil, fmt.Errorf("snapshot: receive frame: %w", recvErr)
	}
}

// drainDecoder flushes the decoder after the input is exhausted and returns
// (true, nil) when one final frame is available. Used when the source is
// shorter than the decoder's reorder window.
func drainDecoder(decCtx *astiav.CodecContext, frame *astiav.Frame) (bool, error) {
	if err := decCtx.SendPacket(nil); err != nil {
		return false, fmt.Errorf("snapshot: flush decoder: %w", err)
	}
	err := decCtx.ReceiveFrame(frame)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, astiav.ErrEof) || errors.Is(err, astiav.ErrEagain) {
		return false, nil
	}
	return false, fmt.Errorf("snapshot: drain receive: %w", err)
}

// frameToRGBAImage scales src (typically YUV420P) into dst (RGBA) and
// returns an image.Image view over dst's pixel buffer. The dst frame buffer
// is allocated by libswscale on the first scale call; it must outlive the
// returned image since image.Image aliases it.
func frameToRGBAImage(src, dst *astiav.Frame) (image.Image, error) {
	swsCtx, err := astiav.CreateSoftwareScaleContext(
		src.Width(), src.Height(), src.PixelFormat(),
		src.Width(), src.Height(), astiav.PixelFormatRgba,
		astiav.NewSoftwareScaleContextFlags(astiav.SoftwareScaleContextFlagBilinear),
	)
	if err != nil {
		return nil, fmt.Errorf("snapshot: create sws context: %w", err)
	}
	defer swsCtx.Free()

	if scaleErr := swsCtx.ScaleFrame(src, dst); scaleErr != nil {
		return nil, fmt.Errorf("snapshot: scale frame: %w", scaleErr)
	}
	img, err := dst.Data().GuessImageFormat()
	if err != nil {
		return nil, fmt.Errorf("snapshot: guess image format: %w", err)
	}
	if err := dst.Data().ToImage(img); err != nil {
		return nil, fmt.Errorf("snapshot: copy frame to image: %w", err)
	}
	return img, nil
}

// silenceFFmpegLogs suppresses ffmpeg's chatty stderr output. Without this,
// every Extract call writes parser warnings (e.g. "non-existing PPS 0 referenced")
// straight to the simulator's terminal.
func silenceFFmpegLogs() {
	astiav.SetLogLevel(astiav.LogLevelQuiet)
}
