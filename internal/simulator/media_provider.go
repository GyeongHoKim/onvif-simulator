package simulator

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/mediasvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/rtsp"
)

// mediaProvider implements mediasvc.Provider for the simulator.
// Profiles are read-only at the Media API surface for MVP — add/remove go
// through the simulator facade (which writes config.*).
type mediaProvider struct {
	sim *Simulator
}

func newMediaProvider(s *Simulator) *mediaProvider {
	return &mediaProvider{sim: s}
}

func (*mediaProvider) ServiceCapabilities(context.Context) (mediasvc.ServiceCapabilities, error) {
	return mediasvc.ServiceCapabilities{
		SnapshotURI:             true,
		RTPTCP:                  true,
		RTPRTSPTCP:              true,
		MaximumNumberOfProfiles: 16,
	}, nil
}

func (p *mediaProvider) Profiles(context.Context) ([]mediasvc.Profile, error) {
	profiles := p.sim.snapshotEffectiveProfiles()
	out := make([]mediasvc.Profile, 0, len(profiles))
	for i := range profiles {
		out = append(out, profileFromConfig(&profiles[i]))
	}
	return out, nil
}

func (p *mediaProvider) Profile(_ context.Context, token string) (mediasvc.Profile, error) {
	profiles := p.sim.snapshotEffectiveProfiles()
	for i := range profiles {
		if profiles[i].Token == token {
			return profileFromConfig(&profiles[i]), nil
		}
	}
	return mediasvc.Profile{}, fmt.Errorf("%w: %s", mediasvc.ErrProfileNotFound, token)
}

// CreateProfile adds a new media profile and persists it. ONVIF Media Service
// §5.2.1 specifies CreateProfile as creating an "empty" profile; our schema
// allows MediaFilePath to be empty until the operator points the profile at
// an mp4 file via the GUI/TUI or by editing the config.
func (p *mediaProvider) CreateProfile(_ context.Context, name, token string) (mediasvc.Profile, error) {
	if strings.TrimSpace(name) == "" {
		return mediasvc.Profile{}, fmt.Errorf("%w: Name is required", mediasvc.ErrInvalidArgs)
	}
	if token == "" {
		token = generateProfileToken(p.sim, name)
	}
	profile := config.ProfileConfig{Name: name, Token: token}
	if err := p.sim.AddProfile(profile); err != nil {
		if errors.Is(err, config.ErrProfileAlreadyExists) {
			return mediasvc.Profile{}, fmt.Errorf("%w: %w", mediasvc.ErrInvalidArgs, err)
		}
		return mediasvc.Profile{}, err
	}
	return profileFromConfig(&profile), nil
}

// DeleteProfile removes a media profile by token and persists.
func (p *mediaProvider) DeleteProfile(_ context.Context, token string) error {
	if err := p.sim.RemoveProfile(token); err != nil {
		if errors.Is(err, config.ErrProfileNotFound) {
			return fmt.Errorf("%w: %s", mediasvc.ErrProfileNotFound, token)
		}
		return err
	}
	return nil
}

// generateProfileToken derives a unique token from name when the caller did
// not supply one. Format: "profile_<sanitized name>" with a numeric suffix on
// collision.
func generateProfileToken(s *Simulator, name string) string {
	cfg := s.snapshotConfig()
	taken := make(map[string]bool, len(cfg.Media.Profiles))
	for i := range cfg.Media.Profiles {
		taken[cfg.Media.Profiles[i].Token] = true
	}
	base := "profile_" + sanitizeTokenChars(name)
	if !taken[base] {
		return base
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s_%d", base, n)
		if !taken[candidate] {
			return candidate
		}
	}
}

// sanitizeTokenChars folds characters outside [a-zA-Z0-9_] into underscores so
// the result matches the ReferenceToken character set expected by ONVIF
// clients.
func sanitizeTokenChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

func (p *mediaProvider) VideoSources(context.Context) ([]mediasvc.VideoSource, error) {
	cfg := p.sim.snapshotConfig()
	seen := make(map[string]bool)
	out := make([]mediasvc.VideoSource, 0, len(cfg.Media.Profiles))
	for i := range cfg.Media.Profiles {
		tok := profileVideoSourceToken(&cfg.Media.Profiles[i])
		if seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, mediasvc.VideoSource{
			Token:      tok,
			Framerate:  cfg.Media.Profiles[i].FPS,
			Resolution: mediasvc.Resolution{Width: cfg.Media.Profiles[i].Width, Height: cfg.Media.Profiles[i].Height},
		})
	}
	return out, nil
}

func (p *mediaProvider) VideoSourceConfigurations(context.Context) ([]mediasvc.VideoSourceConfiguration, error) {
	cfg := p.sim.snapshotConfig()
	seen := make(map[string]bool)
	out := make([]mediasvc.VideoSourceConfiguration, 0, len(cfg.Media.Profiles))
	for i := range cfg.Media.Profiles {
		tok := profileVideoSourceToken(&cfg.Media.Profiles[i])
		if seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, vsConfigFromProfile(&cfg.Media.Profiles[i], tok))
	}
	return out, nil
}

func (p *mediaProvider) VideoSourceConfiguration(
	_ context.Context, token string,
) (mediasvc.VideoSourceConfiguration, error) {
	cfg := p.sim.snapshotConfig()
	for i := range cfg.Media.Profiles {
		tok := profileVideoSourceToken(&cfg.Media.Profiles[i])
		if tok == token {
			return vsConfigFromProfile(&cfg.Media.Profiles[i], tok), nil
		}
	}
	return mediasvc.VideoSourceConfiguration{}, fmt.Errorf("%w: %s", mediasvc.ErrConfigNotFound, token)
}

func (*mediaProvider) SetVideoSourceConfiguration(context.Context, mediasvc.VideoSourceConfiguration) error {
	return nil
}

func (*mediaProvider) AddVideoSourceConfiguration(context.Context, string, string) error {
	return nil
}

func (*mediaProvider) RemoveVideoSourceConfiguration(context.Context, string) error {
	return nil
}

func (p *mediaProvider) CompatibleVideoSourceConfigurations(
	ctx context.Context, _ string,
) ([]mediasvc.VideoSourceConfiguration, error) {
	return p.VideoSourceConfigurations(ctx)
}

func (*mediaProvider) VideoSourceConfigurationOptions(
	context.Context, string, string,
) (mediasvc.VideoSourceConfigurationOptions, error) {
	return mediasvc.VideoSourceConfigurationOptions{MaximumNumberOfProfiles: 16}, nil
}

func (p *mediaProvider) VideoEncoderConfigurations(context.Context) ([]mediasvc.VideoEncoderConfiguration, error) {
	profiles := p.sim.snapshotEffectiveProfiles()
	out := make([]mediasvc.VideoEncoderConfiguration, 0, len(profiles))
	for i := range profiles {
		out = append(out, veConfigFromProfile(&profiles[i]))
	}
	return out, nil
}

func (p *mediaProvider) VideoEncoderConfiguration(
	_ context.Context, token string,
) (mediasvc.VideoEncoderConfiguration, error) {
	profiles := p.sim.snapshotEffectiveProfiles()
	for i := range profiles {
		if profiles[i].Token == token {
			return veConfigFromProfile(&profiles[i]), nil
		}
	}
	return mediasvc.VideoEncoderConfiguration{}, fmt.Errorf("%w: %s", mediasvc.ErrConfigNotFound, token)
}

func (*mediaProvider) SetVideoEncoderConfiguration(context.Context, mediasvc.VideoEncoderConfiguration) error {
	return nil
}

func (*mediaProvider) AddVideoEncoderConfiguration(context.Context, string, string) error {
	return nil
}

func (*mediaProvider) RemoveVideoEncoderConfiguration(context.Context, string) error {
	return nil
}

func (p *mediaProvider) CompatibleVideoEncoderConfigurations(
	ctx context.Context, _ string,
) ([]mediasvc.VideoEncoderConfiguration, error) {
	return p.VideoEncoderConfigurations(ctx)
}

func (*mediaProvider) VideoEncoderConfigurationOptions(
	context.Context, string, string,
) (mediasvc.VideoEncoderConfigurationOptions, error) {
	// Profile S §7.9.1: device "shall declare MJPEG Option in
	// VideoEncoderConfigurationOptions". The H264 block covers §7.5; the
	// JPEG block is what the §7.9 client-side compliance check looks for.
	// Both lists include the same baseline resolutions because the
	// simulator can serve every codec at every advertised dimension —
	// for H.264 by passing the source through, for MJPEG by transcoding
	// or by the rpicam secondary stream.
	resolutions := []mediasvc.ResolutionOptions{
		{Width: 1920, Height: 1080},
		{Width: 1280, Height: 720},
		{Width: 640, Height: 480},
		// Media Spec §5.1 mandates JPEG QVGA support; we list it
		// explicitly so a strictly-compliant client never has to derive it.
		{Width: 320, Height: 240},
	}
	return mediasvc.VideoEncoderConfigurationOptions{
		QualityRange: mediasvc.IntRange{Min: 1, Max: 5},
		H264: mediasvc.H264Options{
			ResolutionsAvailable:  resolutions,
			GovLengthRange:        mediasvc.IntRange{Min: 1, Max: 120},
			FrameRateRange:        mediasvc.IntRange{Min: 1, Max: 60},
			EncodingIntervalRange: mediasvc.IntRange{Min: 1, Max: 1},
			H264ProfilesSupported: []string{"Baseline", "Main", "High"},
		},
		JPEG: mediasvc.JPEGOptions{
			ResolutionsAvailable:  resolutions,
			FrameRateRange:        mediasvc.IntRange{Min: 1, Max: 60},
			EncodingIntervalRange: mediasvc.IntRange{Min: 1, Max: 1},
		},
	}, nil
}

func (p *mediaProvider) StreamURI(
	_ context.Context, profileToken string, _ mediasvc.StreamSetup,
) (mediasvc.MediaURI, error) {
	cfg := p.sim.snapshotConfig()
	profiles := p.sim.snapshotEffectiveProfiles()
	for i := range profiles {
		prof := &profiles[i]
		if prof.Token != profileToken {
			continue
		}
		uri := streamURIFor(&cfg, prof)
		return mediasvc.MediaURI{URI: uri, Timeout: mediaTimeoutPT0S}, nil
	}
	return mediasvc.MediaURI{}, fmt.Errorf("%w: %s", mediasvc.ErrProfileNotFound, profileToken)
}

// streamURIFor computes the URI returned by GetStreamUri. The simulator
// hosts the RTSP endpoint itself, so the URI always points at this device:
// rtsp://<advertised host>:<RTSPPort>/<token>. The MediaFilePath of the
// profile decides whether a stream is actually served — Start refuses to
// register an RTSP source when MediaFilePath is empty, so a client that
// connects to a URI for a fileless profile will get a 404 from gortsplib.
func streamURIFor(cfg *config.Config, p *config.ProfileConfig) string {
	host := localAddrForXAddr()
	port := cfg.Network.RTSPPortOrDefault()
	return "rtsp://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/" + p.Token
}

func (p *mediaProvider) SnapshotURI(_ context.Context, profileToken string) (mediasvc.MediaURI, error) {
	profiles := p.sim.snapshotEffectiveProfiles()
	for i := range profiles {
		if profiles[i].Token == profileToken {
			if profiles[i].SnapshotURI == "" {
				return mediasvc.MediaURI{}, fmt.Errorf("%w: %s", mediasvc.ErrNoSnapshot, profileToken)
			}
			return mediasvc.MediaURI{URI: profiles[i].SnapshotURI, Timeout: mediaTimeoutPT0S}, nil
		}
	}
	return mediasvc.MediaURI{}, fmt.Errorf("%w: %s", mediasvc.ErrProfileNotFound, profileToken)
}

func (p *mediaProvider) GuaranteedNumberOfVideoEncoderInstances(_ context.Context, _ string) (int, error) {
	cfg := p.sim.snapshotConfig()
	if cfg.Media.MaxVideoEncoderInstances > 0 {
		return cfg.Media.MaxVideoEncoderInstances, nil
	}
	return 1, nil
}

func (p *mediaProvider) MetadataConfigurations(context.Context) ([]mediasvc.MetadataConfiguration, error) {
	cfg := p.sim.snapshotConfig()
	out := make([]mediasvc.MetadataConfiguration, 0, len(cfg.Media.MetadataConfigurations))
	for _, m := range cfg.Media.MetadataConfigurations {
		out = append(out, mediasvc.MetadataConfiguration{
			Token:     m.Token,
			Name:      m.Name,
			Analytics: m.Analytics,
			PTZStatus: m.PTZStatus,
			Events:    m.Events,
		})
	}
	return out, nil
}

func (p *mediaProvider) MetadataConfiguration(_ context.Context, token string) (mediasvc.MetadataConfiguration, error) {
	cfg := p.sim.snapshotConfig()
	for _, m := range cfg.Media.MetadataConfigurations {
		if m.Token == token {
			return mediasvc.MetadataConfiguration{
				Token:     m.Token,
				Name:      m.Name,
				Analytics: m.Analytics,
				PTZStatus: m.PTZStatus,
				Events:    m.Events,
			}, nil
		}
	}
	return mediasvc.MetadataConfiguration{}, fmt.Errorf("%w: %s", mediasvc.ErrConfigNotFound, token)
}

func (*mediaProvider) AddMetadataConfiguration(context.Context, string, string) error {
	return nil
}

func (*mediaProvider) RemoveMetadataConfiguration(context.Context, string) error {
	return nil
}

func (*mediaProvider) SetMetadataConfiguration(_ context.Context, cfg mediasvc.MetadataConfiguration) error {
	return config.UpsertMetadataConfig(config.MetadataConfig{
		Token:     cfg.Token,
		Name:      cfg.Name,
		Analytics: cfg.Analytics,
		PTZStatus: cfg.PTZStatus,
		Events:    cfg.Events,
	})
}

func (p *mediaProvider) CompatibleMetadataConfigurations(
	ctx context.Context, _ string,
) ([]mediasvc.MetadataConfiguration, error) {
	return p.MetadataConfigurations(ctx)
}

func (*mediaProvider) MetadataConfigurationOptions(
	context.Context, string, string,
) (mediasvc.MetadataConfigurationOptions, error) {
	return mediasvc.MetadataConfigurationOptions{
		AnalyticsSupported: true,
		EventsSupported:    true,
		PTZStatusSupported: true,
	}, nil
}

// ---------- mapping helpers ------------------------------------------------------

func profileVideoSourceToken(p *config.ProfileConfig) string {
	if p.VideoSourceToken != "" {
		return p.VideoSourceToken
	}
	return config.DefaultVideoSourceToken
}

func profileFromConfig(p *config.ProfileConfig) mediasvc.Profile {
	tok := profileVideoSourceToken(p)
	vs := vsConfigFromProfile(p, tok)
	ve := veConfigFromProfile(p)
	return mediasvc.Profile{
		Token:        p.Token,
		Name:         p.Name,
		Fixed:        false,
		VideoSource:  &vs,
		VideoEncoder: &ve,
	}
}

func vsConfigFromProfile(p *config.ProfileConfig, sourceToken string) mediasvc.VideoSourceConfiguration {
	return mediasvc.VideoSourceConfiguration{
		Token:       sourceToken + "_cfg",
		Name:        sourceToken,
		UseCount:    1,
		SourceToken: sourceToken,
		Bounds:      mediasvc.Rectangle{X: 0, Y: 0, Width: p.Width, Height: p.Height},
	}
}

func veConfigFromProfile(p *config.ProfileConfig) mediasvc.VideoEncoderConfiguration {
	// ONVIF Encoding values: "H264" | "H265" | "JPEG" — note that MJPEG
	// flows through ONVIF as the bare "JPEG" enum value even though the
	// stream is RTP/Motion-JPEG. Map our internal CodecMJPEG token to it.
	encoding := p.Encoding
	if encoding == rtsp.CodecMJPEG {
		encoding = "JPEG"
	}
	cfg := mediasvc.VideoEncoderConfiguration{
		Token:      p.Token + "_enc",
		Name:       p.Name + "_enc",
		UseCount:   1,
		Encoding:   encoding,
		Resolution: mediasvc.Resolution{Width: p.Width, Height: p.Height},
		Quality:    4,
		RateControl: mediasvc.VideoRateControl{
			FrameRateLimit:   p.FPS,
			EncodingInterval: 1,
			BitrateLimit:     p.Bitrate,
		},
		SessionTimeout: mediaTimeoutPT0S,
	}
	if p.Encoding == rtsp.CodecH264 {
		cfg.H264 = mediasvc.H264Configuration{
			GOVLength:   p.GOPLength,
			H264Profile: "Main",
		}
	}
	return cfg
}
