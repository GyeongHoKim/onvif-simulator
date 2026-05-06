package rtsp

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
)

func TestHasH264IDR(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		au   [][]byte
		want bool
	}{
		{"empty", nil, false},
		{"single non-IDR", [][]byte{{0x41, 0x9a}}, false},
		{"sps no idr", [][]byte{{0x67, 0x42}, {0x68, 0xce}}, false},
		{"idr present", [][]byte{{0x41, 0x9a}, {0x65, 0xb8}}, true},
		{"empty nal skipped", [][]byte{{}, {0x65, 0xb8}}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := hasH264IDR(tc.au); got != tc.want {
				t.Fatalf("hasH264IDR=%v want %v", got, tc.want)
			}
		})
	}
}

func TestLiveSourceFillsH264ParameterSetsFromFirstIDR(t *testing.T) {
	t.Parallel()
	sps := []byte{0x67, 0x42, 0xc0, 0x1e}
	pps := []byte{0x68, 0xce, 0x3c, 0x80}
	idr := []byte{0x65, 0xb8, 0x00, 0x01}

	cases := []struct {
		name    string
		probe   *ProbeResult
		nals    [][]byte
		wantSPS []byte
		wantPPS []byte
	}{
		{
			name:    "writes SPS+PPS when probe is empty",
			probe:   &ProbeResult{Codec: CodecH264},
			nals:    [][]byte{sps, pps, idr},
			wantSPS: sps,
			wantPPS: pps,
		},
		{
			name:    "missing PPS leaves PPS untouched",
			probe:   &ProbeResult{Codec: CodecH264},
			nals:    [][]byte{sps, idr},
			wantSPS: sps,
			wantPPS: nil,
		},
		{
			name:    "preserves probe-supplied SPS+PPS",
			probe:   &ProbeResult{Codec: CodecH264, SPS: []byte{0xAA}, PPS: []byte{0xBB}},
			nals:    [][]byte{sps, pps, idr},
			wantSPS: []byte{0xAA},
			wantPPS: []byte{0xBB},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ls := NewLiveSource(tc.probe, nil)
			h := &format.H264{
				PayloadTyp:        96,
				PacketizationMode: 1,
				SPS:               tc.probe.SPS,
				PPS:               tc.probe.PPS,
			}
			media := &description.Media{
				Type:    description.MediaTypeVideo,
				Formats: []format.Format{h},
			}
			ls.AttachStream(nil, media)

			ls.fillH264ParameterSets(tc.nals)

			if !bytes.Equal(h.SPS, tc.wantSPS) {
				t.Fatalf("SPS=%x want %x", h.SPS, tc.wantSPS)
			}
			if !bytes.Equal(h.PPS, tc.wantPPS) {
				t.Fatalf("PPS=%x want %x", h.PPS, tc.wantPPS)
			}
		})
	}
}

func TestLiveSourceFillH264ParameterSetsNoMediaIsNoOp(t *testing.T) {
	t.Parallel()
	ls := NewLiveSource(&ProbeResult{Codec: CodecH264}, nil)
	ls.fillH264ParameterSets([][]byte{{0x67, 0x42}, {0x68, 0xce}})
}

func TestLiveSourceReadyBeforeIDRBlocks(t *testing.T) {
	t.Parallel()
	probe := &ProbeResult{Codec: CodecH264, Width: 1920, Height: 1080, FPS: 30}
	ls := NewLiveSource(probe, nil)

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	err := ls.Ready(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded before any AU, got %v", err)
	}
}

func TestLiveSourceDescribe(t *testing.T) {
	t.Parallel()
	probe := &ProbeResult{Codec: CodecH264, Width: 640, Height: 480, FPS: 15}
	ls := NewLiveSource(probe, nil)
	if got := ls.Describe(); got != probe {
		t.Fatalf("Describe returned %p, want %p", got, probe)
	}
}

func TestLiveSourcePushDoesNotBlockWhenFull(t *testing.T) {
	t.Parallel()
	ls := NewLiveSource(&ProbeResult{Codec: CodecH264}, nil)

	// Saturate the buffer plus a few extras; the overflow must be dropped
	// silently so a slow consumer does not stall the producer.
	for i := range liveBufferSize + 5 {
		ls.Push(AccessUnit{PTS: int64(i), NTP: time.Now(), NALs: [][]byte{{0x41}}})
	}
}

// TestLiveSourceStreamsAfterIDR exercises the full LiveSource lifecycle
// (Run + writeAU + AttachStream + Ready releasing on first IDR + Close)
// against a real gortsplib server and client. Pre-IDR access units are
// dropped; the IDR releases DESCRIBE; subsequent frames flow as RTP.
func TestLiveSourceStreamsAfterIDR(t *testing.T) {
	port := freePort(t)
	s := New(port)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	probe := &ProbeResult{Codec: CodecH264, Width: 1920, Height: 1080, FPS: 30}
	live := NewLiveSource(probe, nil)
	if _, err := s.AddSource("live", live); err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	// Tiny non-IDR + IDR + non-IDR sequence is enough — the encoder only
	// needs raw NAL bytes; the client just counts packets it receives.
	preIDR := [][]byte{{0x41, 0x9a, 0x00}}     // non-IDR slice
	idr := [][]byte{{0x65, 0xb8, 0x00, 0x01}}  // IDR slice
	tail := [][]byte{{0x41, 0x9a, 0x00, 0x02}} // non-IDR slice

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ts := int64(0)
		// Push a pre-IDR frame first to exercise the drop-pre-IDR branch.
		live.Push(AccessUnit{PTS: ts, NTP: time.Now(), NALs: preIDR})
		ts += 3000
		// IDR releases Ready and starts the stream.
		live.Push(AccessUnit{PTS: ts, NTP: time.Now(), NALs: idr})
		ts += 3000
		for {
			select {
			case <-stop:
				return
			default:
			}
			live.Push(AccessUnit{PTS: ts, NTP: time.Now(), NALs: tail})
			ts += 3000
			time.Sleep(20 * time.Millisecond)
		}
	}()

	if got := readRTP(t, port, "live", 3*time.Second); got == 0 {
		close(stop)
		wg.Wait()
		t.Fatal("expected at least one RTP packet from live source")
	}
	close(stop)
	wg.Wait()
}
