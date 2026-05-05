package rpicamera

// HydratedParams is the test-visible projection of the internal upstream
// parameter struct. It carries only the subset the tests assert on so the
// upstream wire-format stays internal to the package.
type HydratedParams struct {
	Width     uint32
	Height    uint32
	FPS       float32
	Bitrate   uint32
	IDRPeriod uint32
	HFlip     bool
	VFlip     bool
}

// HydrateForTest is the export hatch used by api_test.go to verify default
// fill-in without leaking the wire-format struct into the public API.
func HydrateForTest(p Params) HydratedParams {
	up := p.hydrate()
	return HydratedParams{
		Width:     up.Width,
		Height:    up.Height,
		FPS:       up.FPS,
		Bitrate:   up.Bitrate,
		IDRPeriod: up.IDRPeriod,
		HFlip:     up.HFlip,
		VFlip:     up.VFlip,
	}
}
