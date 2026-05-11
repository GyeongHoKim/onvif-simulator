package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// errReaderClosed is returned by readMPJPEGFrame when the underlying
// stream ends cleanly. Distinct from a parse error so callers can stop
// without flagging it as a failure.
var errReaderClosed = errors.New("ffmpeg: mpjpeg stream closed")

// errMissingContentLength is returned when an mpjpeg frame is missing
// the Content-Length header — without it we cannot read the JPEG body.
var errMissingContentLength = errors.New("ffmpeg: mpjpeg frame missing Content-Length")

// errFrameTooLarge is returned when the Content-Length header advertises a
// frame larger than maxMPJPEGFrameSize. The cap defends against a malformed
// or hostile producer driving the parser to allocate unbounded memory.
var errFrameTooLarge = errors.New("ffmpeg: mpjpeg frame exceeds size cap")

// errFrameTruncated is returned when the underlying reader ends part-way
// through a frame body. Distinct from errReaderClosed (clean stream end)
// so callers can flag truncation as a transport failure.
var errFrameTruncated = errors.New("ffmpeg: mpjpeg frame truncated")

// maxMPJPEGFrameSize caps the JPEG body size readMPJPEGFrame will allocate.
// 32 MiB comfortably covers 4K JPEG frames at high quality and leaves a wide
// margin over the simulator's expected 1080p/4K MJPEG output.
const maxMPJPEGFrameSize = 32 << 20

// mpjpegBoundary matches the boundary literal ffmpeg emits when invoked
// with `-f mpjpeg`. The muxer hard-codes "ffmpeg" — we do not parse the
// Content-Type header that announces the boundary because it appears
// only in the stream prelude, not before every frame.
var mpjpegBoundary = []byte("--ffmpeg")

// readMPJPEGFrame reads the next JPEG frame from an mpjpeg stream
// produced by `ffmpeg -f mpjpeg pipe:1`. The stream format is one or
// more multipart/x-mixed-replace bodies separated by a boundary line:
//
//	--ffmpeg
//	Content-type: image/jpeg
//	Content-length: 12345
//
//	<12345 raw JPEG bytes>
//	--ffmpeg
//	...
//
// Boundaries use plain `\n` or `\r\n` line endings depending on the
// ffmpeg version; the parser tolerates both. Content-length is always
// present when ffmpeg writes the mpjpeg muxer, which lets us read the
// exact JPEG payload without scanning for SOI/EOI markers.
func readMPJPEGFrame(ctx context.Context, br *bufio.Reader) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Skip lines until we find the next boundary. ffmpeg may emit a
	// leading blank line and an HTTP-like Content-Type prelude before
	// the first boundary; ignoring everything until the marker keeps
	// the parser tolerant.
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			return nil, mapReaderErr(err)
		}
		if bytes.HasPrefix(bytes.TrimSpace(line), mpjpegBoundary) {
			break
		}
	}

	// Read headers until empty line. Capture Content-Length.
	var contentLength int
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			return nil, mapReaderErr(err)
		}
		trimmed := strings.TrimRight(string(line), "\r\n")
		if trimmed == "" {
			break
		}
		k, v, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(k), "Content-Length") {
			n, parseErr := strconv.Atoi(strings.TrimSpace(v))
			if parseErr != nil {
				return nil, fmt.Errorf("ffmpeg: parse mpjpeg content-length %q: %w", v, parseErr)
			}
			contentLength = n
		}
	}
	return readFrameBody(br, contentLength)
}

// readFrameBody allocates and fills the exact Content-Length-sized JPEG
// payload, after validating that the advertised size is within the cap.
// Splitting this out keeps readMPJPEGFrame under the cyclomatic limit.
func readFrameBody(br *bufio.Reader, contentLength int) ([]byte, error) {
	if contentLength <= 0 {
		return nil, errMissingContentLength
	}
	if contentLength > maxMPJPEGFrameSize {
		return nil, fmt.Errorf("%w: %d > %d", errFrameTooLarge, contentLength, maxMPJPEGFrameSize)
	}
	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(br, buf); err != nil {
		// io.ReadFull returns io.EOF (zero bytes read) or io.ErrUnexpectedEOF
		// (partial read) when the stream ends mid-frame. Both are truncation
		// here because we already committed to consuming Content-Length bytes.
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, fmt.Errorf("%w: %w", errFrameTruncated, err)
		}
		return nil, mapReaderErr(err)
	}
	return buf, nil
}

// mapReaderErr converts io.EOF (clean stream end) into errReaderClosed
// so callers can distinguish a graceful shutdown from a parse failure.
// io.ErrUnexpectedEOF is left to the caller — see readMPJPEGFrame's
// frame-body branch, which wraps it as errFrameTruncated.
func mapReaderErr(err error) error {
	if errors.Is(err, io.EOF) {
		return errReaderClosed
	}
	return fmt.Errorf("ffmpeg: read mpjpeg: %w", err)
}
