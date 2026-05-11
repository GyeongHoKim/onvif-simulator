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
	if contentLength <= 0 {
		return nil, errMissingContentLength
	}

	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(br, buf); err != nil {
		return nil, mapReaderErr(err)
	}
	return buf, nil
}

// mapReaderErr converts io.EOF (clean stream end) into errReaderClosed
// so callers can distinguish a graceful shutdown from a parse failure.
func mapReaderErr(err error) error {
	if errors.Is(err, io.EOF) {
		return errReaderClosed
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return errReaderClosed
	}
	return fmt.Errorf("ffmpeg: read mpjpeg: %w", err)
}
