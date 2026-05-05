package rpicamera

// upstreamParams mirrors the parameter struct mtxrpicam expects on its
// control pipe. Field names and order are part of the wire contract with
// the binary released by mediamtx; do not rename.
type upstreamParams struct {
	LogLevel              string
	CameraID              uint32
	Width                 uint32
	Height                uint32
	HFlip                 bool
	VFlip                 bool
	Brightness            float32
	Contrast              float32
	Saturation            float32
	Sharpness             float32
	Exposure              string
	AWB                   string
	AWBGainRed            float32
	AWBGainBlue           float32
	Denoise               string
	Shutter               uint32
	Metering              string
	Gain                  float32
	EV                    float32
	ROI                   string
	HDR                   bool
	TuningFile            string
	Mode                  string
	FPS                   float32
	AfMode                string
	AfRange               string
	AfSpeed               string
	LensPosition          float32
	AfWindow              string
	FlickerPeriod         uint32
	TextOverlayEnable     bool
	TextOverlay           string
	Codec                 string
	IDRPeriod             uint32
	Bitrate               uint32
	HardwareH264Profile   string
	HardwareH264Level     string
	SoftwareH264Profile   string
	SoftwareH264Level     string
	SecondaryWidth        uint32
	SecondaryHeight       uint32
	SecondaryFPS          float32
	SecondaryMJPEGQuality uint32
}

// hydrate fills an upstreamParams with the simulator-facing subset and
// sensible defaults for everything else. The defaults match mediamtx's
// stock configuration so an operator who tunes only Width/Height/FPS gets
// the same image quality they would on a stock mediamtx install.
func (p Params) hydrate() upstreamParams {
	up := upstreamParams{
		LogLevel:            "warn",
		CameraID:            p.CameraID,
		Width:               p.Width,
		Height:              p.Height,
		HFlip:               p.HFlip,
		VFlip:               p.VFlip,
		Brightness:          p.Brightness,
		Contrast:            p.Contrast,
		Saturation:          p.Saturation,
		Sharpness:           p.Sharpness,
		Exposure:            "normal",
		AWB:                 "auto",
		AWBGainRed:          0,
		AWBGainBlue:         0,
		Denoise:             "off",
		Shutter:             0,
		Metering:            "centre", //nolint:misspell // mtxrpicam wire value
		Gain:                0,
		EV:                  0,
		ROI:                 "",
		HDR:                 false,
		TuningFile:          "",
		Mode:                "",
		FPS:                 p.FPS,
		AfMode:              "continuous",
		AfRange:             "normal",
		AfSpeed:             "normal",
		LensPosition:        0,
		AfWindow:            "",
		FlickerPeriod:       0,
		TextOverlayEnable:   false,
		TextOverlay:         "",
		Codec:               "auto",
		IDRPeriod:           p.IDRPeriod,
		Bitrate:             p.Bitrate,
		HardwareH264Profile: "main",
		HardwareH264Level:   "4.1",
		SoftwareH264Profile: "main",
		SoftwareH264Level:   "4.1",
	}
	if up.Width == 0 {
		up.Width = 1920
	}
	if up.Height == 0 {
		up.Height = 1080
	}
	if up.FPS == 0 {
		up.FPS = 30
	}
	if up.IDRPeriod == 0 {
		up.IDRPeriod = 60
	}
	if up.Bitrate == 0 {
		up.Bitrate = 5_000_000
	}
	return up
}
