package mediasvc

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
)

// ---------- GetServiceCapabilities ----------

func (h *Handler) handleGetServiceCapabilities(ctx context.Context) ([]byte, error) {
	caps, err := h.provider.ServiceCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getServiceCapabilitiesResponse{
		XMLNS: MediaNamespace,
		Capabilities: mediaCapabilitiesEnvelope{
			SnapshotURI:      caps.SnapshotURI,
			Rotation:         caps.Rotation,
			VideoSourceMode:  caps.VideoSourceMode,
			OSD:              caps.OSD,
			TemporaryOSDText: caps.TemporaryOSDText,
			EXICompression:   caps.EXICompression,
			ProfileCapabilities: profileCapabilitiesEnvelope{
				MaximumNumberOfProfiles: caps.MaximumNumberOfProfiles,
			},
			StreamingCapabilities: streamingCapabilitiesEnvelope{
				RTPMulticast:        caps.RTPMulticast,
				RTPTCP:              caps.RTPTCP,
				RTPRTSPTCP:          caps.RTPRTSPTCP,
				NonAggregateControl: caps.NonAggregateControl,
				NoRTSPStreaming:     caps.NoRTSPStreaming,
			},
		},
	})
}

// ---------- Profile management ----------

func (h *Handler) handleGetProfiles(ctx context.Context) ([]byte, error) {
	profiles, err := h.provider.Profiles(ctx)
	if err != nil {
		return nil, err
	}
	env := make([]profileEnvelope, len(profiles))
	for i := range profiles {
		env[i] = profileToEnvelope(&profiles[i])
	}
	return xml.Marshal(getProfilesResponse{
		XMLNS:    MediaNamespace,
		XMLNSTT:  SchemaNamespace,
		Profiles: env,
	})
}

//nolint:dupl // dispatch handlers follow the same ONVIF request/response shape by design
func (h *Handler) handleGetProfile(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string `xml:"ProfileToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("mediasvc: decode GetProfile: %w", err))
	}
	profile, err := h.provider.Profile(ctx, req.ProfileToken)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getProfileResponse{
		XMLNS:   MediaNamespace,
		XMLNSTT: SchemaNamespace,
		Profile: profileToEnvelope(&profile),
	})
}

func (h *Handler) handleCreateProfile(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		Name  string `xml:"Name"`
		Token string `xml:"Token"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("mediasvc: decode CreateProfile: %w", err))
	}
	profile, err := h.provider.CreateProfile(ctx, req.Name, req.Token)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(createProfileResponse{
		XMLNS:   MediaNamespace,
		XMLNSTT: SchemaNamespace,
		Profile: profileToEnvelope(&profile),
	})
}

func (h *Handler) handleDeleteProfile(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string `xml:"ProfileToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("mediasvc: decode DeleteProfile: %w", err))
	}
	if err := h.provider.DeleteProfile(ctx, req.ProfileToken); err != nil {
		return nil, err
	}
	return xml.Marshal(deleteProfileResponse{XMLNS: MediaNamespace})
}

// ---------- VideoSource ----------

func (h *Handler) handleGetVideoSources(ctx context.Context) ([]byte, error) {
	sources, err := h.provider.VideoSources(ctx)
	if err != nil {
		return nil, err
	}
	env := make([]videoSourceEnvelope, len(sources))
	for i := range sources {
		env[i] = videoSourceEnvelope{
			Token:      sources[i].Token,
			Framerate:  sources[i].Framerate,
			Resolution: resolutionEnvelope{Width: sources[i].Resolution.Width, Height: sources[i].Resolution.Height},
		}
	}
	return xml.Marshal(getVideoSourcesResponse{
		XMLNS:        MediaNamespace,
		XMLNSTT:      SchemaNamespace,
		VideoSources: env,
	})
}

func (h *Handler) handleGetVideoSourceConfigurations(ctx context.Context) ([]byte, error) {
	cfgs, err := h.provider.VideoSourceConfigurations(ctx)
	if err != nil {
		return nil, err
	}
	env := make([]videoSourceConfigurationEnvelope, len(cfgs))
	for i := range cfgs {
		env[i] = vsConfigToEnvelope(&cfgs[i])
	}
	return xml.Marshal(getVideoSourceConfigurationsResponse{
		XMLNS:          MediaNamespace,
		XMLNSTT:        SchemaNamespace,
		Configurations: env,
	})
}

//nolint:dupl // dispatch handlers follow the same ONVIF request/response shape by design
func (h *Handler) handleGetVideoSourceConfiguration(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ConfigurationToken string `xml:"ConfigurationToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("mediasvc: decode GetVideoSourceConfiguration: %w", err))
	}
	cfg, err := h.provider.VideoSourceConfiguration(ctx, req.ConfigurationToken)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getVideoSourceConfigurationResponse{
		XMLNS:         MediaNamespace,
		XMLNSTT:       SchemaNamespace,
		Configuration: vsConfigToEnvelope(&cfg),
	})
}

func (h *Handler) handleSetVideoSourceConfiguration(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		Configuration videoSourceConfigurationEnvelope `xml:"Configuration"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("mediasvc: decode SetVideoSourceConfiguration: %w", err))
	}
	cfg := envelopeToVSConfig(&req.Configuration)
	if err := h.provider.SetVideoSourceConfiguration(ctx, cfg); err != nil {
		return nil, err
	}
	return xml.Marshal(setVideoSourceConfigurationResponse{XMLNS: MediaNamespace})
}

func (h *Handler) handleAddVideoSourceConfiguration(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken       string `xml:"ProfileToken"`
		ConfigurationToken string `xml:"ConfigurationToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("mediasvc: decode AddVideoSourceConfiguration: %w", err))
	}
	if err := h.provider.AddVideoSourceConfiguration(ctx, req.ProfileToken, req.ConfigurationToken); err != nil {
		return nil, err
	}
	return xml.Marshal(addVideoSourceConfigurationResponse{XMLNS: MediaNamespace})
}

func (h *Handler) handleRemoveVideoSourceConfiguration(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string `xml:"ProfileToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("mediasvc: decode RemoveVideoSourceConfiguration: %w", err))
	}
	if err := h.provider.RemoveVideoSourceConfiguration(ctx, req.ProfileToken); err != nil {
		return nil, err
	}
	return xml.Marshal(removeVideoSourceConfigurationResponse{XMLNS: MediaNamespace})
}

//nolint:dupl // dispatch handlers follow the same ONVIF request/response shape by design
func (h *Handler) handleGetCompatibleVideoSourceConfigurations(
	ctx context.Context, payload []byte,
) ([]byte, error) {
	var req struct {
		ProfileToken string `xml:"ProfileToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload,
			fmt.Errorf("mediasvc: decode GetCompatibleVideoSourceConfigurations: %w", err))
	}
	cfgs, err := h.provider.CompatibleVideoSourceConfigurations(ctx, req.ProfileToken)
	if err != nil {
		return nil, err
	}
	env := make([]videoSourceConfigurationEnvelope, len(cfgs))
	for i := range cfgs {
		env[i] = vsConfigToEnvelope(&cfgs[i])
	}
	return xml.Marshal(getCompatibleVideoSourceConfigurationsResponse{
		XMLNS:          MediaNamespace,
		XMLNSTT:        SchemaNamespace,
		Configurations: env,
	})
}

func (h *Handler) handleGetVideoSourceConfigurationOptions(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ConfigurationToken string `xml:"ConfigurationToken"`
		ProfileToken       string `xml:"ProfileToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload,
			fmt.Errorf("mediasvc: decode GetVideoSourceConfigurationOptions: %w", err))
	}
	opt, err := h.provider.VideoSourceConfigurationOptions(ctx, req.ConfigurationToken, req.ProfileToken)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getVideoSourceConfigurationOptionsResponse{
		XMLNS:   MediaNamespace,
		XMLNSTT: SchemaNamespace,
		Options: videoSourceConfigurationOptionsEnvelope{
			MaximumNumberOfProfiles: opt.MaximumNumberOfProfiles,
			BoundsRange: boundsRangeEnvelope{
				XRange:      intRangeEnvelope{Min: opt.BoundsRange.XRange.Min, Max: opt.BoundsRange.XRange.Max},
				YRange:      intRangeEnvelope{Min: opt.BoundsRange.YRange.Min, Max: opt.BoundsRange.YRange.Max},
				WidthRange:  intRangeEnvelope{Min: opt.BoundsRange.WidthRange.Min, Max: opt.BoundsRange.WidthRange.Max},
				HeightRange: intRangeEnvelope{Min: opt.BoundsRange.HeightRange.Min, Max: opt.BoundsRange.HeightRange.Max},
			},
			VideoSourceTokensAvailable: append([]string(nil), opt.VideoSourceTokensAvailable...),
		},
	})
}

// ---------- VideoEncoder ----------

func (h *Handler) handleGetVideoEncoderConfigurations(ctx context.Context) ([]byte, error) {
	cfgs, err := h.provider.VideoEncoderConfigurations(ctx)
	if err != nil {
		return nil, err
	}
	env := make([]videoEncoderConfigurationEnvelope, len(cfgs))
	for i := range cfgs {
		env[i] = veConfigToEnvelope(&cfgs[i])
	}
	return xml.Marshal(getVideoEncoderConfigurationsResponse{
		XMLNS:          MediaNamespace,
		XMLNSTT:        SchemaNamespace,
		Configurations: env,
	})
}

//nolint:dupl // dispatch handlers follow the same ONVIF request/response shape by design
func (h *Handler) handleGetVideoEncoderConfiguration(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ConfigurationToken string `xml:"ConfigurationToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("mediasvc: decode GetVideoEncoderConfiguration: %w", err))
	}
	cfg, err := h.provider.VideoEncoderConfiguration(ctx, req.ConfigurationToken)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getVideoEncoderConfigurationResponse{
		XMLNS:         MediaNamespace,
		XMLNSTT:       SchemaNamespace,
		Configuration: veConfigToEnvelope(&cfg),
	})
}

func (h *Handler) handleSetVideoEncoderConfiguration(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		Configuration videoEncoderConfigurationEnvelope `xml:"Configuration"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("mediasvc: decode SetVideoEncoderConfiguration: %w", err))
	}
	cfg := envelopeToVEConfig(&req.Configuration)
	if err := h.provider.SetVideoEncoderConfiguration(ctx, cfg); err != nil {
		return nil, err
	}
	return xml.Marshal(setVideoEncoderConfigurationResponse{XMLNS: MediaNamespace})
}

func (h *Handler) handleAddVideoEncoderConfiguration(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken       string `xml:"ProfileToken"`
		ConfigurationToken string `xml:"ConfigurationToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("mediasvc: decode AddVideoEncoderConfiguration: %w", err))
	}
	if err := h.provider.AddVideoEncoderConfiguration(ctx, req.ProfileToken, req.ConfigurationToken); err != nil {
		return nil, err
	}
	return xml.Marshal(addVideoEncoderConfigurationResponse{XMLNS: MediaNamespace})
}

func (h *Handler) handleRemoveVideoEncoderConfiguration(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string `xml:"ProfileToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("mediasvc: decode RemoveVideoEncoderConfiguration: %w", err))
	}
	if err := h.provider.RemoveVideoEncoderConfiguration(ctx, req.ProfileToken); err != nil {
		return nil, err
	}
	return xml.Marshal(removeVideoEncoderConfigurationResponse{XMLNS: MediaNamespace})
}

//nolint:dupl // dispatch handlers follow the same ONVIF request/response shape by design
func (h *Handler) handleGetCompatibleVideoEncoderConfigurations(
	ctx context.Context, payload []byte,
) ([]byte, error) {
	var req struct {
		ProfileToken string `xml:"ProfileToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload,
			fmt.Errorf("mediasvc: decode GetCompatibleVideoEncoderConfigurations: %w", err))
	}
	cfgs, err := h.provider.CompatibleVideoEncoderConfigurations(ctx, req.ProfileToken)
	if err != nil {
		return nil, err
	}
	env := make([]videoEncoderConfigurationEnvelope, len(cfgs))
	for i := range cfgs {
		env[i] = veConfigToEnvelope(&cfgs[i])
	}
	return xml.Marshal(getCompatibleVideoEncoderConfigurationsResponse{
		XMLNS:          MediaNamespace,
		XMLNSTT:        SchemaNamespace,
		Configurations: env,
	})
}

func (h *Handler) handleGetVideoEncoderConfigurationOptions(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ConfigurationToken string `xml:"ConfigurationToken"`
		ProfileToken       string `xml:"ProfileToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload,
			fmt.Errorf("mediasvc: decode GetVideoEncoderConfigurationOptions: %w", err))
	}
	opt, err := h.provider.VideoEncoderConfigurationOptions(ctx, req.ConfigurationToken, req.ProfileToken)
	if err != nil {
		return nil, err
	}
	resolutions := make([]resolutionEnvelope, len(opt.H264.ResolutionsAvailable))
	for i, r := range opt.H264.ResolutionsAvailable {
		resolutions[i] = resolutionEnvelope(r)
	}
	return xml.Marshal(getVideoEncoderConfigurationOptionsResponse{
		XMLNS:   MediaNamespace,
		XMLNSTT: SchemaNamespace,
		Options: videoEncoderConfigurationOptionsEnvelope{
			QualityRange: intRangeEnvelope(opt.QualityRange),
			H264: h264OptionsEnvelope{
				ResolutionsAvailable:  resolutions,
				GovLengthRange:        intRangeEnvelope(opt.H264.GovLengthRange),
				FrameRateRange:        intRangeEnvelope(opt.H264.FrameRateRange),
				EncodingIntervalRange: intRangeEnvelope(opt.H264.EncodingIntervalRange),
				H264ProfilesSupported: append([]string(nil), opt.H264.H264ProfilesSupported...),
			},
		},
	})
}

// ---------- Stream / Snapshot URI ----------

func (h *Handler) handleGetStreamURI(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		StreamSetup struct {
			Stream    string `xml:"Stream"`
			Transport struct {
				Protocol string `xml:"Protocol"`
			} `xml:"Transport"`
		} `xml:"StreamSetup"`
		ProfileToken string `xml:"ProfileToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("mediasvc: decode GetStreamUri: %w", err))
	}
	setup := StreamSetup{
		Stream:    req.StreamSetup.Stream,
		Transport: req.StreamSetup.Transport.Protocol,
	}
	uri, err := h.provider.StreamURI(ctx, req.ProfileToken, setup)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getStreamURIResponse{
		XMLNS:    MediaNamespace,
		XMLNSTT:  SchemaNamespace,
		MediaURI: mediaURIFromDomain(uri),
	})
}

func (h *Handler) handleGetSnapshotURI(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string `xml:"ProfileToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("mediasvc: decode GetSnapshotUri: %w", err))
	}
	uri, err := h.provider.SnapshotURI(ctx, req.ProfileToken)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getSnapshotURIResponse{
		XMLNS:    MediaNamespace,
		XMLNSTT:  SchemaNamespace,
		MediaURI: mediaURIFromDomain(uri),
	})
}

// ---------- Envelope converters ----------

func profileToEnvelope(p *Profile) profileEnvelope {
	env := profileEnvelope{
		Token: p.Token,
		Fixed: boolAttr(p.Fixed),
		Name:  p.Name,
	}
	if p.VideoSource != nil {
		vs := vsConfigToEnvelope(p.VideoSource)
		env.VideoSourceConfiguration = &vs
	}
	if p.VideoEncoder != nil {
		ve := veConfigToEnvelope(p.VideoEncoder)
		env.VideoEncoderConfiguration = &ve
	}
	return env
}

func vsConfigToEnvelope(cfg *VideoSourceConfiguration) videoSourceConfigurationEnvelope {
	return videoSourceConfigurationEnvelope{
		Token:       cfg.Token,
		Name:        cfg.Name,
		UseCount:    cfg.UseCount,
		SourceToken: cfg.SourceToken,
		Bounds: boundsEnvelope{
			X:      cfg.Bounds.X,
			Y:      cfg.Bounds.Y,
			Width:  cfg.Bounds.Width,
			Height: cfg.Bounds.Height,
		},
	}
}

func envelopeToVSConfig(env *videoSourceConfigurationEnvelope) VideoSourceConfiguration {
	return VideoSourceConfiguration{
		Token:       env.Token,
		Name:        env.Name,
		UseCount:    env.UseCount,
		SourceToken: env.SourceToken,
		Bounds: Rectangle{
			X:      env.Bounds.X,
			Y:      env.Bounds.Y,
			Width:  env.Bounds.Width,
			Height: env.Bounds.Height,
		},
	}
}

func veConfigToEnvelope(cfg *VideoEncoderConfiguration) videoEncoderConfigurationEnvelope {
	env := videoEncoderConfigurationEnvelope{
		Token:      cfg.Token,
		Name:       cfg.Name,
		UseCount:   cfg.UseCount,
		Encoding:   cfg.Encoding,
		Resolution: resolutionEnvelope{Width: cfg.Resolution.Width, Height: cfg.Resolution.Height},
		Quality:    cfg.Quality,
		RateControl: &rateControlEnvelope{
			FrameRateLimit:   cfg.RateControl.FrameRateLimit,
			EncodingInterval: cfg.RateControl.EncodingInterval,
			BitrateLimit:     cfg.RateControl.BitrateLimit,
		},
		SessionTimeout: cfg.SessionTout,
	}
	if cfg.Encoding == "H264" {
		env.H264 = &h264ConfigurationEnvelope{
			GovLength:   cfg.H264.GOVLength,
			H264Profile: cfg.H264.H264Profile,
		}
	}
	return env
}

func envelopeToVEConfig(env *videoEncoderConfigurationEnvelope) VideoEncoderConfiguration {
	cfg := VideoEncoderConfiguration{
		Token:       env.Token,
		Name:        env.Name,
		UseCount:    env.UseCount,
		Encoding:    env.Encoding,
		Resolution:  Resolution{Width: env.Resolution.Width, Height: env.Resolution.Height},
		Quality:     env.Quality,
		SessionTout: env.SessionTimeout,
	}
	if env.RateControl != nil {
		cfg.RateControl = VideoRateControl{
			FrameRateLimit:   env.RateControl.FrameRateLimit,
			EncodingInterval: env.RateControl.EncodingInterval,
			BitrateLimit:     env.RateControl.BitrateLimit,
		}
	}
	if env.H264 != nil {
		cfg.H264 = H264Configuration{
			GOVLength:   env.H264.GovLength,
			H264Profile: env.H264.H264Profile,
		}
	}
	return cfg
}

func mediaURIFromDomain(m MediaURI) mediaURIEnvelope {
	return mediaURIEnvelope(m)
}

func boolAttr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
