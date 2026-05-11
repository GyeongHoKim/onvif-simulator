package ffmpeg

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// makeMPJPEG returns a synthetic mpjpeg byte stream containing the given
// frames separated by the ffmpeg boundary. Mirrors the format the real
// `ffmpeg -f mpjpeg` muxer emits so the parser is exercised on
// production-shape input.
func makeMPJPEG(frames ...[]byte) []byte {
	var b bytes.Buffer
	for _, f := range frames {
		b.WriteString("--ffmpeg\r\n")
		b.WriteString("Content-type: image/jpeg\r\n")
		b.WriteString("Content-length: ")
		b.WriteString(itoa(len(f)))
		b.WriteString("\r\n\r\n")
		b.Write(f)
		b.WriteString("\r\n")
	}
	return b.Bytes()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func TestReadMPJPEGFrame_SingleFrame(t *testing.T) {
	t.Parallel()
	want := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0xDE, 0xAD, 0xBE, 0xEF, 0xFF, 0xD9}
	stream := makeMPJPEG(want)
	br := bufio.NewReader(bytes.NewReader(stream))

	got, err := readMPJPEGFrame(context.Background(), br)
	if err != nil {
		t.Fatalf("readMPJPEGFrame returned error: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("frame mismatch: got %x, want %x", got, want)
	}
}

func TestReadMPJPEGFrame_MultipleFrames(t *testing.T) {
	t.Parallel()
	f1 := bytes.Repeat([]byte{0x01}, 16)
	f2 := bytes.Repeat([]byte{0x02}, 32)
	stream := makeMPJPEG(f1, f2)
	br := bufio.NewReader(bytes.NewReader(stream))

	got1, err := readMPJPEGFrame(context.Background(), br)
	if err != nil {
		t.Fatalf("first frame error: %v", err)
	}
	if !bytes.Equal(got1, f1) {
		t.Errorf("first frame mismatch: got %x, want %x", got1, f1)
	}
	got2, err := readMPJPEGFrame(context.Background(), br)
	if err != nil {
		t.Fatalf("second frame error: %v", err)
	}
	if !bytes.Equal(got2, f2) {
		t.Errorf("second frame mismatch")
	}
}

func TestReadMPJPEGFrame_StreamEnd(t *testing.T) {
	t.Parallel()
	br := bufio.NewReader(strings.NewReader(""))
	_, err := readMPJPEGFrame(context.Background(), br)
	if !errors.Is(err, errReaderClosed) {
		t.Fatalf("expected errReaderClosed on empty stream, got %v", err)
	}
}

func TestReadMPJPEGFrame_InvalidContentLengthValue(t *testing.T) {
	t.Parallel()
	bad := "--ffmpeg\r\nContent-type: image/jpeg\r\nContent-length: not-a-number\r\n\r\n"
	br := bufio.NewReader(strings.NewReader(bad))
	_, err := readMPJPEGFrame(context.Background(), br)
	if err == nil {
		t.Fatal("expected error for non-numeric Content-Length")
	}
	if errors.Is(err, errReaderClosed) {
		t.Fatalf("parse error should not map to errReaderClosed: %v", err)
	}
}

func TestReadMPJPEGFrame_MissingContentLength(t *testing.T) {
	t.Parallel()
	bad := "--ffmpeg\r\nContent-type: image/jpeg\r\n\r\n"
	br := bufio.NewReader(strings.NewReader(bad))
	_, err := readMPJPEGFrame(context.Background(), br)
	if err == nil {
		t.Fatal("expected error for missing Content-Length")
	}
	if errors.Is(err, errReaderClosed) {
		t.Fatalf("missing Content-Length should not be treated as clean close: %v", err)
	}
}

func TestReadMPJPEGFrame_HonorsContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stream := makeMPJPEG([]byte{0xFF, 0xD8, 0xFF, 0xD9})
	br := bufio.NewReader(bytes.NewReader(stream))
	_, err := readMPJPEGFrame(ctx, br)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestReadMPJPEGFrame_TruncatedBody(t *testing.T) {
	t.Parallel()
	// Advertise a 64-byte body but only supply 16 bytes so io.ReadFull
	// returns io.ErrUnexpectedEOF. The parser should map this to a
	// truncation error distinct from errReaderClosed so callers can flag
	// it as a transport failure rather than a graceful close.
	body := bytes.Repeat([]byte{0xAB}, 16)
	stream := "--ffmpeg\r\nContent-type: image/jpeg\r\nContent-length: 64\r\n\r\n" + string(body)
	br := bufio.NewReader(strings.NewReader(stream))
	_, err := readMPJPEGFrame(context.Background(), br)
	if err == nil {
		t.Fatal("expected error for truncated frame body")
	}
	if errors.Is(err, errReaderClosed) {
		t.Fatalf("truncated body must not map to errReaderClosed: %v", err)
	}
	if !errors.Is(err, errFrameTruncated) {
		t.Fatalf("expected errFrameTruncated, got %v", err)
	}
}

func TestReadMPJPEGFrame_RejectsOversizedContentLength(t *testing.T) {
	t.Parallel()
	// Content-Length far above maxMPJPEGFrameSize must be rejected before
	// the parser allocates memory.
	huge := "--ffmpeg\r\nContent-type: image/jpeg\r\nContent-length: 999999999\r\n\r\n"
	br := bufio.NewReader(strings.NewReader(huge))
	_, err := readMPJPEGFrame(context.Background(), br)
	if !errors.Is(err, errFrameTooLarge) {
		t.Fatalf("expected errFrameTooLarge, got %v", err)
	}
}

func TestReadMPJPEGFrame_TolerantOfLFOnlyEndings(t *testing.T) {
	t.Parallel()
	// Some ffmpeg builds emit `\n` rather than `\r\n` for the multipart
	// line endings; the parser tolerates both.
	payload := []byte{0xFF, 0xD8, 0xAA, 0xBB, 0xFF, 0xD9}
	stream := "--ffmpeg\nContent-type: image/jpeg\nContent-length: " + itoa(len(payload)) + "\n\n" + string(payload) + "\n"
	br := bufio.NewReader(strings.NewReader(stream))
	got, err := readMPJPEGFrame(context.Background(), br)
	if err != nil {
		t.Fatalf("LF-only frame parse: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("LF-only frame mismatch: got %x, want %x", got, payload)
	}
}
