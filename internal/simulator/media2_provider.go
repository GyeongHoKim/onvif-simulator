package simulator

import (
	"context"
	"fmt"
	"net"
	"strconv"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/media2svc"
)

// media2Provider implements media2svc.Provider for the simulator.
type media2Provider struct {
	sim *Simulator
}

func newMedia2Provider(s *Simulator) *media2Provider {
	return &media2Provider{sim: s}
}

func (p *media2Provider) ServiceCapabilities(_ context.Context) (media2svc.ServiceCapabilities, error) {
	cfg := p.sim.snapshotConfig()
	hasH265 := false
	for i := range cfg.Media.Profiles {
		if cfg.Media.Profiles[i].Encoding == "H265" {
			hasH265 = true
			break
		}
	}
	return media2svc.ServiceCapabilities{
		H264:                    true,
		H265:                    hasH265,
		JPEG:                    true,
		SnapshotURI:             true,
		MaximumNumberOfProfiles: 16,
		RTPMulticast:            false,
		RTPTCP:                  true,
		RTPRTSPTCP:              false,
		NonAggregateControl:     true,
		NoRTSPStreaming:         false,
	}, nil
}

func (p *media2Provider) Profiles(_ context.Context) ([]media2svc.Profile, error) {
	cfg := p.sim.snapshotConfig()
	out := make([]media2svc.Profile, 0, len(cfg.Media.Profiles))
	for i := range cfg.Media.Profiles {
		prof := cfg.Media.Profiles[i]
		enc := prof.Encoding
		if enc == "" {
			enc = "H264"
		}
		out = append(out, media2svc.Profile{
			Token: prof.Token,
			Name:  prof.Name,
			VideoEncoderConfigurations: []media2svc.VideoEncoderConfiguration{
				{
					Token:     prof.Token + "_venc",
					Name:      prof.Name + "_encoder",
					Encoding:  enc,
					Width:     prof.Width,
					Height:    prof.Height,
					FrameRate: prof.FPS,
					Bitrate:   prof.Bitrate,
				},
			},
		})
	}
	return out, nil
}

func (p *media2Provider) VideoEncoderConfigurations(_ context.Context) ([]media2svc.VideoEncoderConfiguration, error) {
	cfg := p.sim.snapshotConfig()
	out := make([]media2svc.VideoEncoderConfiguration, 0, len(cfg.Media.Profiles))
	for i := range cfg.Media.Profiles {
		prof := cfg.Media.Profiles[i]
		enc := prof.Encoding
		if enc == "" {
			enc = "H264"
		}
		out = append(out, media2svc.VideoEncoderConfiguration{
			Token:     prof.Token + "_venc",
			Name:      prof.Name + "_encoder",
			Encoding:  enc,
			Width:     prof.Width,
			Height:    prof.Height,
			FrameRate: prof.FPS,
			Bitrate:   prof.Bitrate,
		})
	}
	return out, nil
}

func (p *media2Provider) VideoEncoderConfiguration(
	_ context.Context, token string,
) (media2svc.VideoEncoderConfiguration, error) {
	cfgs, err := p.VideoEncoderConfigurations(context.TODO())
	if err != nil {
		return media2svc.VideoEncoderConfiguration{}, err
	}
	for _, cfg := range cfgs {
		if cfg.Token == token {
			return cfg, nil
		}
	}
	return media2svc.VideoEncoderConfiguration{},
		fmt.Errorf("%w: token %s", media2svc.ErrInvalidArgs, token)
}

func (*media2Provider) VideoEncoderConfigurationOptions(
	_ context.Context, _ string,
) (media2svc.VideoEncoderConfigurationOptions, error) {
	return media2svc.VideoEncoderConfigurationOptions{
		H264: &media2svc.H264Options{
			ResolutionsAvailable: []media2svc.Resolution{
				{Width: 1920, Height: 1080},
				{Width: 1280, Height: 720},
				{Width: 640, Height: 480},
			},
			FrameRateRange: media2svc.IntRange{Min: 1, Max: 30},
			BitrateRange:   media2svc.IntRange{Min: 100000, Max: 8000000},
		},
		H265: &media2svc.H265Options{
			ResolutionsAvailable: []media2svc.Resolution{
				{Width: 3840, Height: 2160},
				{Width: 1920, Height: 1080},
				{Width: 1280, Height: 720},
			},
			FrameRateRange: media2svc.IntRange{Min: 1, Max: 60},
			BitrateRange:   media2svc.IntRange{Min: 100000, Max: 16000000},
		},
		JPEG: &media2svc.JPEGOptions{
			ResolutionsAvailable: []media2svc.Resolution{
				{Width: 1920, Height: 1080},
				{Width: 1280, Height: 720},
			},
			FrameRateRange: media2svc.IntRange{Min: 1, Max: 15},
		},
	}, nil
}

func (*media2Provider) SetVideoEncoderConfiguration(
	_ context.Context, _ string, _ *media2svc.VideoEncoderConfiguration,
) error {
	return nil
}

func (p *media2Provider) StreamURI(_ context.Context, profileToken, _ string) (string, error) {
	cfg := p.sim.snapshotConfig()
	host := localAddrForXAddr()
	port := cfg.Network.RTSPPortOrDefault()
	return "rtsp://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/" + profileToken, nil
}

func (p *media2Provider) SnapshotURI(_ context.Context, profileToken string) (string, error) {
	cfg := p.sim.snapshotConfig()
	host := localAddrForXAddr()
	port := cfg.Network.HTTPPort
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/snapshot/" + profileToken, nil
}
