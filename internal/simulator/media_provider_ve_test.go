package simulator

import (
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/rtsp"
)

func TestVeConfigFromProfile_mapsMJPEGToJPEG(t *testing.T) {
	t.Parallel()
	p := &config.ProfileConfig{
		Token:    "t1",
		Name:     "Main",
		Encoding: rtsp.CodecMJPEG,
		Width:    640,
		Height:   480,
		FPS:      30,
		Bitrate:  1000,
	}
	ve := veConfigFromProfile(p)
	if ve.Encoding != "JPEG" {
		t.Fatalf("Encoding=%q want JPEG (ONVIF enum for MJPEG RTP)", ve.Encoding)
	}
}
