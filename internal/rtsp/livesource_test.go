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
	"github.com/pion/rtp"
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

func TestLiveSourceUpdateGOPCacheReplacesOnIDR(t *testing.T) {
	t.Parallel()
	ls := NewLiveSource(&ProbeResult{Codec: CodecH264}, nil)

	first := []*rtp.Packet{{Header: rtp.Header{SequenceNumber: 1}}, {Header: rtp.Header{SequenceNumber: 2}}}
	ls.updateGOPCache(first, true)
	if got := len(ls.gopCache); got != 2 {
		t.Fatalf("after IDR seed, len=%d want 2", got)
	}

	// non-IDR appends
	ls.updateGOPCache([]*rtp.Packet{{Header: rtp.Header{SequenceNumber: 3}}}, false)
	if got := len(ls.gopCache); got != 3 {
		t.Fatalf("after non-IDR append, len=%d want 3", got)
	}

	// next IDR replaces wholesale
	second := []*rtp.Packet{{Header: rtp.Header{SequenceNumber: 100}}}
	ls.updateGOPCache(second, true)
	if got := len(ls.gopCache); got != 1 {
		t.Fatalf("after second IDR, len=%d want 1", got)
	}
	if ls.gopCache[0].SequenceNumber != 100 {
		t.Fatalf("after second IDR, head seq=%d want 100", ls.gopCache[0].SequenceNumber)
	}
}

func TestLiveSourceUpdateGOPCacheIgnoresNonIDRBeforeIDR(t *testing.T) {
	t.Parallel()
	ls := NewLiveSource(&ProbeResult{Codec: CodecH264}, nil)

	// non-IDR before any IDR has seeded the cache must be skipped — caching
	// reference-less slices would only confuse a replaying decoder.
	ls.updateGOPCache([]*rtp.Packet{{Header: rtp.Header{SequenceNumber: 1}}}, false)
	if got := len(ls.gopCache); got != 0 {
		t.Fatalf("non-IDR before IDR seeded cache: len=%d want 0", got)
	}
}

func TestLiveSourceUpdateGOPCacheCapDropsAppend(t *testing.T) {
	t.Parallel()
	ls := NewLiveSource(&ProbeResult{Codec: CodecH264}, nil)

	idr := make([]*rtp.Packet, maxGOPCachePackets)
	for i := range idr {
		idr[i] = &rtp.Packet{}
	}
	ls.updateGOPCache(idr, true)
	if got := len(ls.gopCache); got != maxGOPCachePackets {
		t.Fatalf("seeded cache len=%d want %d", got, maxGOPCachePackets)
	}

	// One more non-IDR packet would overflow → append must be dropped.
	ls.updateGOPCache([]*rtp.Packet{{}}, false)
	if got := len(ls.gopCache); got != maxGOPCachePackets {
		t.Fatalf("after overflow append, len=%d want %d (drop, not grow)",
			got, maxGOPCachePackets)
	}

	// Next IDR must still reset.
	ls.updateGOPCache([]*rtp.Packet{{Header: rtp.Header{SequenceNumber: 1}}}, true)
	if got := len(ls.gopCache); got != 1 {
		t.Fatalf("post-cap reset len=%d want 1", got)
	}
}

func TestLiveSourceReplayGOPNoOpWithoutMedia(t *testing.T) {
	t.Parallel()
	ls := NewLiveSource(&ProbeResult{Codec: CodecH264}, nil)
	// Sanity: with l.media nil, ReplayGOP must not panic on a nil session
	// either, because the media-nil guard short-circuits before the
	// session is touched.
	ls.ReplayGOP(nil)
}

// TestLiveSourceMidGoPJoinerReceivesKeyframe verifies that a client which
// connects after the first IDR — and crucially while the producer is paused
// (no fresh live frames between PLAY and the deadline) — still receives RTP
// packets, because OnPlay replays the cached GOP into the session before the
// stream broadcast resumes.
//
// Without the cache, this test would observe zero packets: the live source
// would have nothing to broadcast until the next IDR, which never arrives
// in the test window.
func TestLiveSourceMidGoPJoinerReceivesKeyframe(t *testing.T) {
	port := freePort(t)
	s := New(port)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	probe := &ProbeResult{Codec: CodecH264, Width: 1280, Height: 720, FPS: 30}
	live := NewLiveSource(probe, nil)
	if _, err := s.AddSource("live", live); err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	sps := []byte{0x67, 0x42, 0xc0, 0x1e}
	pps := []byte{0x68, 0xce, 0x3c, 0x80}
	idr := []byte{0x65, 0xb8, 0x00, 0x01}
	pSlice := []byte{0x41, 0x9a, 0x00, 0x02}

	// IDR access unit (SPS+PPS+IDR together — the inline-IDR shape that
	// fillH264ParameterSets relies on) seeds the cache, then a few P-slices
	// extend it. After this Push burst the producer goes idle for the rest
	// of the test, so any packets the client receives must come from the
	// cache replay path, not the live broadcast.
	live.Push(AccessUnit{PTS: 0, NTP: time.Now(), NALs: [][]byte{sps, pps, idr}})
	live.Push(AccessUnit{PTS: 3000, NTP: time.Now(), NALs: [][]byte{pSlice}})
	live.Push(AccessUnit{PTS: 6000, NTP: time.Now(), NALs: [][]byte{pSlice}})

	// Give Run() time to drain the queue and populate the cache before the
	// client connects.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		live.gopMu.Lock()
		populated := len(live.gopCache) > 0
		live.gopMu.Unlock()
		if populated {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := readRTP(t, port, "live", time.Second); got == 0 {
		t.Fatal("mid-GoP joiner saw zero packets — GOP replay did not fire")
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
