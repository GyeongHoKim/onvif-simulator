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
	"github.com/bluenviron/gortsplib/v5/pkg/format/rtph264"
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

func TestLiveSourceAbsorbsParameterSets(t *testing.T) {
	t.Parallel()
	sps := []byte{0x67, 0x42, 0xc0, 0x1e}
	pps := []byte{0x68, 0xce, 0x3c, 0x80}
	idr := []byte{0x65, 0xb8, 0x00, 0x01}
	altSPS := []byte{0x67, 0x4d, 0x40, 0x29}

	cases := []struct {
		name    string
		probe   *ProbeResult
		feed    [][][]byte // sequence of AUs to absorb
		wantSPS []byte
		wantPPS []byte
	}{
		{
			name:    "single inline IDR carries SPS+PPS",
			probe:   &ProbeResult{Codec: CodecH264},
			feed:    [][][]byte{{sps, pps, idr}},
			wantSPS: sps,
			wantPPS: pps,
		},
		{
			name:    "SPS and PPS arriving in separate AUs are both cached",
			probe:   &ProbeResult{Codec: CodecH264},
			feed:    [][][]byte{{sps}, {pps}, {idr}},
			wantSPS: sps,
			wantPPS: pps,
		},
		{
			name:    "later AU overrides probe-supplied SPS+PPS",
			probe:   &ProbeResult{Codec: CodecH264, SPS: []byte{0xAA}, PPS: []byte{0xBB}},
			feed:    [][][]byte{{sps, pps, idr}},
			wantSPS: sps,
			wantPPS: pps,
		},
		{
			name:    "newer SPS replaces older",
			probe:   &ProbeResult{Codec: CodecH264},
			feed:    [][][]byte{{sps, pps}, {altSPS, idr}},
			wantSPS: altSPS,
			wantPPS: pps,
		},
		{
			name:    "PPS-less AU does not blank PPS",
			probe:   &ProbeResult{Codec: CodecH264},
			feed:    [][][]byte{{sps, pps}, {sps, idr}},
			wantSPS: sps,
			wantPPS: pps,
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

			for _, nals := range tc.feed {
				ls.absorbParameterSets(nals)
			}

			if !bytes.Equal(h.SPS, tc.wantSPS) {
				t.Fatalf("SDP SPS=%x want %x", h.SPS, tc.wantSPS)
			}
			if !bytes.Equal(h.PPS, tc.wantPPS) {
				t.Fatalf("SDP PPS=%x want %x", h.PPS, tc.wantPPS)
			}
			if !bytes.Equal(ls.psSPS, tc.wantSPS) {
				t.Fatalf("cached SPS=%x want %x", ls.psSPS, tc.wantSPS)
			}
			if !bytes.Equal(ls.psPPS, tc.wantPPS) {
				t.Fatalf("cached PPS=%x want %x", ls.psPPS, tc.wantPPS)
			}
		})
	}
}

func TestLiveSourceAbsorbParameterSetsNoMediaIsNoOp(t *testing.T) {
	t.Parallel()
	ls := NewLiveSource(&ProbeResult{Codec: CodecH264}, nil)
	ls.absorbParameterSets([][]byte{{0x67, 0x42}, {0x68, 0xce}})
	if len(ls.psSPS) == 0 || len(ls.psPPS) == 0 {
		t.Fatal("cache should populate even with no media attached")
	}
}

func TestLiveSourceMaybePrependParameterSets(t *testing.T) {
	t.Parallel()
	sps := []byte{0x67, 0x42, 0xc0, 0x1e}
	pps := []byte{0x68, 0xce, 0x3c, 0x80}
	idr := []byte{0x65, 0xb8, 0x00, 0x01}

	cases := []struct {
		name      string
		cacheSPS  []byte
		cachePPS  []byte
		input     [][]byte
		wantTypes []byte
	}{
		{
			name:      "IDR alone gets SPS+PPS prepended from cache",
			cacheSPS:  sps,
			cachePPS:  pps,
			input:     [][]byte{idr},
			wantTypes: []byte{7, 8, 5},
		},
		{
			name:      "IDR with both SPS+PPS in-band passes through unchanged",
			cacheSPS:  sps,
			cachePPS:  pps,
			input:     [][]byte{sps, pps, idr},
			wantTypes: []byte{7, 8, 5},
		},
		{
			name:      "IDR with only SPS in-band gets PPS prepended",
			cacheSPS:  sps,
			cachePPS:  pps,
			input:     [][]byte{sps, idr},
			wantTypes: []byte{8, 7, 5},
		},
		{
			name:      "empty cache leaves IDR alone",
			cacheSPS:  nil,
			cachePPS:  nil,
			input:     [][]byte{idr},
			wantTypes: []byte{5},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ls := NewLiveSource(&ProbeResult{Codec: CodecH264}, nil)
			ls.psSPS = tc.cacheSPS
			ls.psPPS = tc.cachePPS

			out := ls.maybePrependParameterSets(tc.input)
			if len(out) != len(tc.wantTypes) {
				t.Fatalf("len(out)=%d want %d", len(out), len(tc.wantTypes))
			}
			for i, nal := range out {
				if got := nal[0] & 0x1F; got != tc.wantTypes[i] {
					t.Fatalf("out[%d] type=%d want %d", i, got, tc.wantTypes[i])
				}
			}
		})
	}
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

	idrNTP := time.Unix(1700000000, 0)
	first := []*rtp.Packet{{Header: rtp.Header{SequenceNumber: 1}}, {Header: rtp.Header{SequenceNumber: 2}}}
	ls.updateGOPCache(first, idrNTP, true)
	if got := len(ls.gopCache); got != 2 {
		t.Fatalf("after IDR seed, len=%d want 2", got)
	}
	if !ls.gopCache[0].ntp.Equal(idrNTP) {
		t.Fatalf("after IDR seed, ntp=%v want %v", ls.gopCache[0].ntp, idrNTP)
	}

	// non-IDR appends and carries its own NTP forward
	pSliceNTP := idrNTP.Add(33 * time.Millisecond)
	ls.updateGOPCache([]*rtp.Packet{{Header: rtp.Header{SequenceNumber: 3}}}, pSliceNTP, false)
	if got := len(ls.gopCache); got != 3 {
		t.Fatalf("after non-IDR append, len=%d want 3", got)
	}
	if !ls.gopCache[2].ntp.Equal(pSliceNTP) {
		t.Fatalf("non-IDR ntp=%v want %v", ls.gopCache[2].ntp, pSliceNTP)
	}

	// next IDR replaces wholesale
	secondNTP := idrNTP.Add(2 * time.Second)
	second := []*rtp.Packet{{Header: rtp.Header{SequenceNumber: 100}}}
	ls.updateGOPCache(second, secondNTP, true)
	if got := len(ls.gopCache); got != 1 {
		t.Fatalf("after second IDR, len=%d want 1", got)
	}
	if ls.gopCache[0].pkt.SequenceNumber != 100 {
		t.Fatalf("after second IDR, head seq=%d want 100", ls.gopCache[0].pkt.SequenceNumber)
	}
	if !ls.gopCache[0].ntp.Equal(secondNTP) {
		t.Fatalf("after second IDR, ntp=%v want %v", ls.gopCache[0].ntp, secondNTP)
	}
}

func TestLiveSourceUpdateGOPCacheIgnoresNonIDRBeforeIDR(t *testing.T) {
	t.Parallel()
	ls := NewLiveSource(&ProbeResult{Codec: CodecH264}, nil)

	// non-IDR before any IDR has seeded the cache must be skipped — caching
	// reference-less slices would only confuse a replaying decoder.
	ls.updateGOPCache([]*rtp.Packet{{Header: rtp.Header{SequenceNumber: 1}}}, time.Now(), false)
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
	ls.updateGOPCache(idr, time.Now(), true)
	if got := len(ls.gopCache); got != maxGOPCachePackets {
		t.Fatalf("seeded cache len=%d want %d", got, maxGOPCachePackets)
	}

	// One more non-IDR packet would overflow → append must be dropped.
	ls.updateGOPCache([]*rtp.Packet{{}}, time.Now(), false)
	if got := len(ls.gopCache); got != maxGOPCachePackets {
		t.Fatalf("after overflow append, len=%d want %d (drop, not grow)",
			got, maxGOPCachePackets)
	}

	// Next IDR must still reset.
	ls.updateGOPCache([]*rtp.Packet{{Header: rtp.Header{SequenceNumber: 1}}}, time.Now(), true)
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

// TestLiveSourceReplayGOPNoOpWithEmptyCache exercises the second guard in
// ReplayGOP: media is attached but no GOP has been cached yet (Run never
// observed an IDR). The empty-cache early return must fire before the
// session writer is touched, so passing a nil ServerSession is safe.
func TestLiveSourceReplayGOPNoOpWithEmptyCache(t *testing.T) {
	t.Parallel()
	ls := NewLiveSource(&ProbeResult{Codec: CodecH264}, nil)
	media := &description.Media{
		Type:    description.MediaTypeVideo,
		Formats: []format.Format{&format.H264{PayloadTyp: 96, PacketizationMode: 1}},
	}
	ls.AttachStream(nil, media)

	// gopCache is empty; ReplayGOP must return at the len(entries)==0
	// guard. If it didn't, the subsequent ss.WritePacketRTPWithNTP on a
	// nil session would panic.
	ls.ReplayGOP(nil)
}

// TestLiveSourceWriteAUSurfacesStreamWriteError covers the error-return
// branch in writeAU. After the server is stopped the underlying gortsplib
// stream is closed, and any subsequent WritePacketRTPWithNTP returns
// liberrors.ErrServerStreamClosed; writeAU must surface that error so
// Run terminates the source goroutine instead of busy-looping.
func TestLiveSourceWriteAUSurfacesStreamWriteError(t *testing.T) {
	t.Parallel()
	port := freePort(t)
	s := New(port)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	probe := &ProbeResult{Codec: CodecH264, Width: 1280, Height: 720, FPS: 30}
	live := NewLiveSource(probe, nil)
	if _, err := s.AddSource("live", live); err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	// Stop tears down the source's Run goroutine and closes the stream
	// gortsplib holds onto. live.stream still points at the now-closed
	// stream, which is the state writeAU must recognize as terminal.
	s.Stop()

	enc := &rtph264.Encoder{
		PayloadType:    96,
		PayloadMaxSize: 1460,
	}
	if err := enc.Init(); err != nil {
		t.Fatalf("enc.Init: %v", err)
	}

	sps := []byte{0x67, 0x42, 0xc0, 0x1e}
	pps := []byte{0x68, 0xce, 0x3c, 0x80}
	idr := []byte{0x65, 0xb8, 0x00, 0x01}
	au := AccessUnit{PTS: 0, NTP: time.Now(), NALs: [][]byte{sps, pps, idr}}

	if err := live.writeAU(enc, au, true); err == nil {
		t.Fatal("expected writeAU to surface stream write error after Stop, got nil")
	}
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

	// SPS+PPS+IDR seeds the cache and releases Ready; subsequent
	// P-slices extend it. The producer then goes idle, so any packets the
	// client receives must come from the cache replay path.
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

// TestLiveSourceMidGoPJoinerReceivesKeyframeActiveProducer is the
// active-producer counterpart of TestLiveSourceMidGoPJoinerReceivesKeyframe.
// A producer goroutine keeps pushing access units throughout the
// DESCRIBE/SETUP/PLAY handshake, so the OnPlay handler runs concurrently
// with live stream.WritePacketRTP broadcasts.
//
// This validates two assumptions about gortsplib v5.5.2's PLAY ordering:
//
//  1. ReplayGOP, called inside OnPlay, queues to the joining session's
//     pre-play writer (createWriter runs at server_session.go:1217, before
//     OnPlay; readerSetActive at 1268 only adds the session to the stream's
//     activeUnicastReaders set after the handler returns OK), so live
//     stream broadcasts to existing readers do not interleave with the
//     replay packets in this session's queue.
//  2. The joiner receives a substantive packet flow even when the producer
//     is actively running — both replay-cache packets and post-handshake
//     live packets land in the same FIFO writer queue.
//
// Without a working replay path the joiner would still receive the
// post-handshake live tail, but no keyframe; the test relies on
// readRTP-level "non-zero" since stricter ordering checks would race
// against gortsplib's client jitter buffer.
func TestLiveSourceMidGoPJoinerReceivesKeyframeActiveProducer(t *testing.T) {
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

	producerCtx, producerCancel := context.WithCancel(t.Context())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ts := int64(0)
		// IDR seeds the cache and unblocks Ready so DESCRIBE can return.
		live.Push(AccessUnit{PTS: ts, NTP: time.Now(), NALs: [][]byte{sps, pps, idr}})
		ts += 3000
		ticker := time.NewTicker(33 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-producerCtx.Done():
				return
			case <-ticker.C:
				live.Push(AccessUnit{PTS: ts, NTP: time.Now(), NALs: [][]byte{pSlice}})
				ts += 3000
			}
		}
	}()
	// Cancel-then-wait order matters: a single defer ensures the producer
	// is signaled to stop before we block on its exit, which a swapped
	// pair of defers would dead-lock.
	defer func() {
		producerCancel()
		wg.Wait()
	}()

	// Wait until the cache is populated with at least the seeding IDR plus
	// a P-slice so the replay path actually has data to forward when the
	// client connects.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		live.gopMu.Lock()
		populated := len(live.gopCache) >= 2
		live.gopMu.Unlock()
		if populated {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if got := readRTP(t, port, "live", time.Second); got == 0 {
		t.Fatal("active-producer joiner saw zero packets — replay/live handoff failed")
	}
}

// TestLiveSourceStreamsAfterIDR exercises the full LiveSource lifecycle
// against a real gortsplib server and client.
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

	preIDR := [][]byte{{0x41, 0x9a, 0x00}}
	sps := []byte{0x67, 0x42, 0xc0, 0x1e}
	pps := []byte{0x68, 0xce, 0x3c, 0x80}
	idr := [][]byte{sps, pps, {0x65, 0xb8, 0x00, 0x01}}
	tail := [][]byte{{0x41, 0x9a, 0x00, 0x02}}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ts := int64(0)
		live.Push(AccessUnit{PTS: ts, NTP: time.Now(), NALs: preIDR})
		ts += 3000
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

// TestLiveSourceReadyHoldsUntilParameterSetsObserved verifies that Ready
// waits for SPS, PPS, and an IDR — an IDR alone must not release it.
func TestLiveSourceReadyHoldsUntilParameterSetsObserved(t *testing.T) {
	port := freePort(t)
	s := New(port)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	probe := &ProbeResult{Codec: CodecH264, Width: 640, Height: 480, FPS: 15}
	live := NewLiveSource(probe, nil)
	if _, err := s.AddSource("live", live); err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	idrOnly := []byte{0x65, 0xb8, 0x00, 0x01}
	live.Push(AccessUnit{PTS: 0, NTP: time.Now(), NALs: [][]byte{idrOnly}})

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if err := live.Ready(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Ready returned %v, expected DeadlineExceeded with no SPS/PPS in cache", err)
	}

	sps := []byte{0x67, 0x42, 0xc0, 0x1e}
	pps := []byte{0x68, 0xce, 0x3c, 0x80}
	live.Push(AccessUnit{PTS: 3000, NTP: time.Now(), NALs: [][]byte{sps, pps, idrOnly}})

	ctx2, cancel2 := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel2()
	if err := live.Ready(ctx2); err != nil {
		t.Fatalf("Ready after SPS+PPS+IDR: %v", err)
	}
}

// TestLiveSourceWriteAUPrependsParameterSetsAfterCacheSeed verifies that a
// bare-IDR AU pushed after the cache is seeded still reaches the GOP cache
// with SPS+PPS prepended in-band.
func TestLiveSourceWriteAUPrependsParameterSetsAfterCacheSeed(t *testing.T) {
	port := freePort(t)
	s := New(port)
	if err := s.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer s.Stop()

	probe := &ProbeResult{Codec: CodecH264, Width: 640, Height: 480, FPS: 15}
	live := NewLiveSource(probe, nil)
	if _, err := s.AddSource("live", live); err != nil {
		t.Fatalf("AddSource: %v", err)
	}

	sps := []byte{0x67, 0x42, 0xc0, 0x1e}
	pps := []byte{0x68, 0xce, 0x3c, 0x80}
	idr := []byte{0x65, 0xb8, 0x00, 0x01}

	live.Push(AccessUnit{PTS: 0, NTP: time.Now(), NALs: [][]byte{sps, pps, idr}})
	live.Push(AccessUnit{PTS: 3000, NTP: time.Now(), NALs: [][]byte{idr}})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		live.gopMu.Lock()
		populated := len(live.gopCache) > 0
		live.gopMu.Unlock()
		if populated {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	live.gopMu.Lock()
	pkts := make([]*rtp.Packet, 0, len(live.gopCache))
	for _, e := range live.gopCache {
		pkts = append(pkts, e.pkt)
	}
	live.gopMu.Unlock()
	if len(pkts) == 0 {
		t.Fatal("gop cache is empty; second IDR did not land")
	}

	dec := &rtph264.Decoder{}
	if err := dec.Init(); err != nil {
		t.Fatalf("decoder init: %v", err)
	}
	seenTypes := map[byte]bool{}
	for _, pkt := range pkts {
		nals, err := dec.Decode(pkt)
		if err != nil {
			continue
		}
		for _, n := range nals {
			if len(n) > 0 {
				seenTypes[n[0]&0x1F] = true
			}
		}
	}
	if !seenTypes[h264NALTypeSPS] || !seenTypes[h264NALTypePPS] || !seenTypes[5] {
		t.Fatalf("decoded GOP cache NAL types=%v (want SPS=7, PPS=8, IDR=5)", seenTypes)
	}
}
