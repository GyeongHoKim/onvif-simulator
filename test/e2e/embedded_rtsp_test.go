//go:build e2e

// embedded_rtsp_test.go verifies that GetStreamUri on a simulator configured
// with a local mp4 (MediaFilePath) returns a URI pointing at the simulator's
// own RTSP server, and that the URI actually serves RTP packets.
//
// The test skips when:
//   - The simulator is configured for the legacy passthrough RTSP model
//     (GetStreamUri returns a URL pointing somewhere other than the
//     simulator's host).
//   - GetStreamUri itself errors with NoRTSPStreaming (Profile S optional).
//
// To exercise the embedded path, run:
//
//	ffmpeg -y -f lavfi -i testsrc=duration=2:size=320x240:rate=15 \
//	    -c:v libx264 -pix_fmt yuv420p sample.mp4
//	# point the simulator at sample.mp4 via media_file_path in onvif-simulator.json
//	./bin/onvif-simulator serve &
//	make e2e
package e2e

import (
	"context"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	media "github.com/use-go/onvif/media"
	sdkmedia "github.com/use-go/onvif/sdk/media"
	onviftypes "github.com/use-go/onvif/xsd/onvif"

	"github.com/bluenviron/gortsplib/v5"
	"github.com/bluenviron/gortsplib/v5/pkg/base"
	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/bluenviron/gortsplib/v5/pkg/format"
	"github.com/pion/rtp"
)

func TestEmbeddedRTSPStreamPlayback(t *testing.T) {
	dev := newDevice(t)
	ctx, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// Pick the first profile and ask for an RTSP-Unicast/RTSP transport URI
	// — the same combo most ONVIF clients use first.
	prof := mediaFirstProfile(t, ctx, dev)
	uriResp, err := sdkmedia.Call_GetStreamUri(ctx, dev, media.GetStreamUri{
		ProfileToken: prof.Token,
		StreamSetup: onviftypes.StreamSetup{
			Stream: "RTP-Unicast",
			Transport: onviftypes.Transport{
				Protocol: "RTSP",
			},
		},
	})
	if err != nil {
		t.Skipf("GetStreamUri unsupported on this simulator: %v", err)
	}
	streamURI := string(uriResp.MediaUri.Uri)
	if streamURI == "" {
		t.Fatal("GetStreamUri returned empty URI")
	}

	parsed, err := url.Parse(streamURI)
	if err != nil {
		t.Fatalf("parse %q: %v", streamURI, err)
	}
	host := envOrDefault("ONVIF_HOST", "localhost:8080")
	deviceHost := strings.SplitN(host, ":", 2)[0]
	if !strings.Contains(parsed.Host, deviceHost) && !strings.HasPrefix(parsed.Host, "127.") {
		t.Skipf(
			"GetStreamUri %q does not point at the simulator (%s) — "+
				"likely a passthrough config; embedded RTSP test skipped",
			streamURI, deviceHost,
		)
	}

	got := readRTPPackets(t, streamURI, 5*time.Second)
	if got == 0 {
		t.Fatalf("no RTP packets received from %s within 5s", streamURI)
	}
	t.Logf("received %d RTP packets from embedded RTSP server at %s", got, streamURI)
}

// readRTPPackets connects an RTSP client to streamURI, performs DESCRIBE /
// SETUP / PLAY, and waits up to deadline for at least one RTP packet to
// arrive. Returns the packet count.
func readRTPPackets(t *testing.T, streamURI string, deadline time.Duration) int {
	t.Helper()

	parsedURL, err := base.ParseURL(streamURI)
	if err != nil {
		t.Fatalf("rtsp ParseURL %q: %v", streamURI, err)
	}

	c := &gortsplib.Client{}
	c.Scheme = parsedURL.Scheme
	c.Host = parsedURL.Host
	if startErr := c.Start(); startErr != nil {
		t.Fatalf("rtsp client Start: %v", startErr)
	}
	defer c.Close()

	desc, _, err := c.Describe(parsedURL)
	if err != nil {
		t.Fatalf("rtsp DESCRIBE: %v", err)
	}
	if len(desc.Medias) == 0 {
		t.Fatal("rtsp DESCRIBE returned zero medias")
	}
	if err := c.SetupAll(desc.BaseURL, desc.Medias); err != nil {
		t.Fatalf("rtsp SETUP: %v", err)
	}

	var seen atomic.Int32
	c.OnPacketRTPAny(func(_ *description.Media, _ format.Format, _ *rtp.Packet) {
		seen.Add(1)
	})

	if _, err := c.Play(nil); err != nil {
		t.Fatalf("rtsp PLAY: %v", err)
	}

	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if seen.Load() > 0 {
			return int(seen.Load())
		}
		time.Sleep(50 * time.Millisecond)
	}
	return int(seen.Load())
}
