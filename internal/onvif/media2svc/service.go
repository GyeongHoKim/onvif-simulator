package media2svc

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
)

const (
	soapNamespace   = "http://www.w3.org/2003/05/soap-envelope"
	maxSOAPBodySize = 10 << 20

	faultCodeSender   = "Sender"
	faultCodeReceiver = "Receiver"
)

var (
	errUnsupportedOp    = errors.New("media2svc: unsupported operation")
	errEmptySOAPBody    = errors.New("media2svc: empty soap body")
	errDecodePayload    = errors.New("media2svc: malformed request payload")
	errInvalidNamespace = errors.New("media2svc: unexpected operation namespace")
)

// Handler serves the ONVIF Media2 service endpoint.
type Handler struct {
	provider Provider
	auth     AuthHook
	logger   *slog.Logger
}

// Option customizes a Media2 service Handler.
type Option func(*Handler)

// WithAuthHook installs a request authorization hook.
func WithAuthHook(hook AuthHook) Option {
	return func(h *Handler) {
		if hook != nil {
			h.auth = hook
		}
	}
}

// WithLogger installs a structured logger.
func WithLogger(logger *slog.Logger) Option {
	return func(h *Handler) {
		if logger == nil {
			logger = obs.Discard()
		}
		h.logger = logger
	}
}

// NewHandler creates a Media2-service HTTP handler.
func NewHandler(provider Provider, opts ...Option) *Handler {
	if provider == nil {
		panic(ErrProviderRequired)
	}
	h := &Handler{
		provider: provider,
		auth:     AuthFunc(func(context.Context, string, *http.Request) error { return nil }),
		logger:   obs.Discard(),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

func (h *Handler) loggerForRequest(r *http.Request) *slog.Logger {
	return obs.LoggerFromContextOr(r.Context(), h.logger)
}

// ServeHTTP dispatches SOAP Media2-service operations.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxSOAPBodySize)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeFault(w, http.StatusRequestEntityTooLarge, faultCodeSender, "", tooLarge.Error())
			return
		}
		writeFault(w, http.StatusBadRequest, faultCodeSender, "",
			fmt.Errorf("read request body: %w", err).Error())
		return
	}
	if closeErr := r.Body.Close(); closeErr != nil {
		writeFault(w, http.StatusBadRequest, faultCodeSender, "", closeErr.Error())
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))

	payload, operation, err := parseOperation(raw)
	if err != nil {
		if r.Header.Get("Authorization") == "" {
			if challengeErr := h.auth.Authorize(r.Context(), "", r); challengeErr != nil {
				writeAuthFault(w, challengeErr)
				return
			}
		}
		h.loggerForRequest(r).Warn("media2: parse soap envelope", "err", err)
		writeFault(w, http.StatusBadRequest, faultCodeSender, "", err.Error())
		return
	}

	if authErr := h.auth.Authorize(r.Context(), operation, r); authErr != nil {
		writeAuthFault(w, authErr)
		return
	}

	respPayload, err := h.dispatch(r.Context(), operation, payload)
	if err != nil {
		status := http.StatusInternalServerError
		code := faultCodeReceiver
		level := slog.LevelError
		switch {
		case errors.Is(err, errUnsupportedOp):
			status = http.StatusNotImplemented
			code = faultCodeSender
			level = slog.LevelWarn
		case errors.Is(err, errDecodePayload),
			errors.Is(err, ErrInvalidArgs):
			status = http.StatusBadRequest
			code = faultCodeSender
			level = slog.LevelWarn
		}
		h.loggerForRequest(r).LogAttrs(r.Context(), level, "media2: dispatch fault",
			slog.String("operation", operation),
			slog.Int("status", status),
			slog.String("err", err.Error()),
		)
		writeFault(w, status, code, "", err.Error())
		return
	}
	writeSOAP(w, respPayload)
}

func writeAuthFault(w http.ResponseWriter, authErr error) {
	status := http.StatusUnauthorized
	subcode := ""
	var challenge *auth.ChallengeError
	if errors.As(authErr, &challenge) {
		if challenge.Status != 0 {
			status = challenge.Status
		}
		subcode = challenge.Subcode
		for k, vs := range challenge.Headers {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
	}
	writeFault(w, status, faultCodeSender, subcode, authErr.Error())
}

func (h *Handler) dispatch(ctx context.Context, operation string, payload []byte) ([]byte, error) {
	switch operation {
	case "GetServiceCapabilities":
		return h.handleGetServiceCapabilities(ctx)
	case "GetProfiles":
		return h.handleGetProfiles(ctx)
	case "GetVideoEncoderConfigurations":
		return h.handleGetVideoEncoderConfigurations(ctx)
	case "GetVideoEncoderConfiguration":
		return h.handleGetVideoEncoderConfiguration(ctx, payload)
	case "GetVideoEncoderConfigurationOptions":
		return h.handleGetVideoEncoderConfigurationOptions(ctx, payload)
	case "SetVideoEncoderConfiguration":
		return h.handleSetVideoEncoderConfiguration(ctx, payload)
	case "GetStreamUri":
		return h.handleGetStreamURI(ctx, payload)
	case "GetSnapshotUri":
		return h.handleGetSnapshotURI(ctx, payload)
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedOp, operation)
	}
}

// ---------- Operation handlers ----------

func (h *Handler) handleGetServiceCapabilities(ctx context.Context) ([]byte, error) {
	caps, err := h.provider.ServiceCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getServiceCapabilitiesResponse{
		XMLNS: Media2Namespace,
		Capabilities: media2CapabilitiesEnvelope{
			SnapshotURI:      caps.SnapshotURI,
			Rotation:         caps.Rotation,
			VideoSourceMode:  caps.VideoSourceMode,
			OSD:              caps.OSD,
			TemporaryOSDText: caps.TemporaryOSDText,
			ProfileCapabilities: profileCapsEnv{
				MaximumNumberOfProfiles: caps.MaximumNumberOfProfiles,
			},
			StreamingCapabilities: streamingCapsEnv{
				RTPMulticast:        caps.RTPMulticast,
				RTPTCP:              caps.RTPTCP,
				RTPRTSPTCP:          caps.RTPRTSPTCP,
				NonAggregateControl: caps.NonAggregateControl,
				NoRTSPStreaming:     caps.NoRTSPStreaming,
			},
			H264: caps.H264,
			H265: caps.H265,
			JPEG: caps.JPEG,
		},
	})
}

func (h *Handler) handleGetProfiles(ctx context.Context) ([]byte, error) {
	profiles, err := h.provider.Profiles(ctx)
	if err != nil {
		return nil, err
	}
	envs := make([]media2ProfileEnv, len(profiles))
	for i := range profiles {
		encs := make([]media2VideoEncoderEnv, len(profiles[i].VideoEncoderConfigurations))
		for j := range profiles[i].VideoEncoderConfigurations {
			encs[j] = media2VideoEncoderEnv{
				Token:    profiles[i].VideoEncoderConfigurations[j].Token,
				Encoding: profiles[i].VideoEncoderConfigurations[j].Encoding,
			}
		}
		envs[i] = media2ProfileEnv{
			Token:               profiles[i].Token,
			Name:                profiles[i].Name,
			VideoEncoderConfigs: encs,
		}
	}
	return xml.Marshal(getProfilesResponse{
		XMLNS:    Media2Namespace,
		XMLNSTT:  SchemaNamespace,
		Profiles: envs,
	})
}

func (h *Handler) handleGetVideoEncoderConfigurations(ctx context.Context) ([]byte, error) {
	configs, err := h.provider.VideoEncoderConfigurations(ctx)
	if err != nil {
		return nil, err
	}
	envs := make([]media2VideoEncoderEnv, len(configs))
	for i := range configs {
		envs[i] = media2VideoEncoderEnv{
			Token:    configs[i].Token,
			Encoding: configs[i].Encoding,
		}
	}
	return xml.Marshal(getVideoEncoderConfigurationsResponse{
		XMLNS:          Media2Namespace,
		Configurations: envs,
	})
}

func (h *Handler) handleGetVideoEncoderConfiguration(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		Token string `xml:"ConfigurationToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GetVideoEncoderConfiguration: %w", errDecodePayload, err)
	}
	config, err := h.provider.VideoEncoderConfiguration(ctx, req.Token)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getVideoEncoderConfigurationResponse{
		XMLNS: Media2Namespace,
		Configuration: media2VideoEncoderEnv{
			Token:    config.Token,
			Encoding: config.Encoding,
		},
	})
}

func (h *Handler) handleGetVideoEncoderConfigurationOptions(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		Token string `xml:"ConfigurationToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GetVideoEncoderConfigurationOptions: %w",
			errDecodePayload, err)
	}
	opts, err := h.provider.VideoEncoderConfigurationOptions(ctx, req.Token)
	if err != nil {
		return nil, err
	}
	env := media2EncoderOptionsEnv{}
	if opts.H264 != nil {
		env.H264 = &h264OptionsEnv{
			ResolutionsAvailable: resolutionsToEnv(opts.H264.ResolutionsAvailable),
			FrameRateRange:       intRangeToEnv(opts.H264.FrameRateRange),
			BitrateRange:         intRangeToEnv(opts.H264.BitrateRange),
		}
	}
	if opts.H265 != nil {
		env.H265 = &h265OptionsEnv{
			ResolutionsAvailable: resolutionsToEnv(opts.H265.ResolutionsAvailable),
			FrameRateRange:       intRangeToEnv(opts.H265.FrameRateRange),
			BitrateRange:         intRangeToEnv(opts.H265.BitrateRange),
		}
	}
	if opts.JPEG != nil {
		env.JPEG = &jpegOptionsEnv{
			ResolutionsAvailable: resolutionsToEnv(opts.JPEG.ResolutionsAvailable),
			FrameRateRange:       intRangeToEnv(opts.JPEG.FrameRateRange),
		}
	}
	return xml.Marshal(getVideoEncoderConfigurationOptionsResponse{
		XMLNS:   Media2Namespace,
		Options: env,
	})
}

func (h *Handler) handleSetVideoEncoderConfiguration(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		Token string `xml:"Configuration>token"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode SetVideoEncoderConfiguration: %w",
			errDecodePayload, err)
	}
	if err := h.provider.SetVideoEncoderConfiguration(ctx, req.Token,
		&VideoEncoderConfiguration{}); err != nil {
		return nil, err
	}
	return xml.Marshal(setVideoEncoderConfigurationResponse{XMLNS: Media2Namespace})
}

func (h *Handler) handleGetStreamURI(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string `xml:"ProfileToken"`
		Protocol     string `xml:"Protocol"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GetStreamUri: %w", errDecodePayload, err)
	}
	uri, err := h.provider.StreamURI(ctx, req.ProfileToken, req.Protocol)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getStreamURIResponse{
		XMLNS: Media2Namespace,
		URI:   uri,
	})
}

func (h *Handler) handleGetSnapshotURI(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string `xml:"ProfileToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GetSnapshotUri: %w", errDecodePayload, err)
	}
	uri, err := h.provider.SnapshotURI(ctx, req.ProfileToken)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getSnapshotURIResponse{
		XMLNS: Media2Namespace,
		URI:   uri,
	})
}

// ---------- Conversion helpers ----------

func resolutionsToEnv(resolutions []Resolution) []resolutionEnv {
	out := make([]resolutionEnv, len(resolutions))
	for i := range resolutions {
		out[i] = resolutionEnv{Width: resolutions[i].Width, Height: resolutions[i].Height}
	}
	return out
}

func intRangeToEnv(r IntRange) intRangeEnv {
	return intRangeEnv(r)
}

// ---------- SOAP helpers ----------

func parseOperation(data []byte) (payload []byte, operation string, err error) {
	var env struct {
		Body struct {
			Inner []byte `xml:",innerxml"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(data, &env); err != nil {
		return nil, "", fmt.Errorf("parse soap envelope: %w", err)
	}
	if len(env.Body.Inner) == 0 {
		return nil, "", errEmptySOAPBody
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	inBody := false
	for {
		tok, err := decoder.Token()
		if err != nil {
			return nil, "", fmt.Errorf("parse soap body: %w", err)
		}
		switch elem := tok.(type) {
		case xml.StartElement:
			if elem.Name.Local == "Body" && elem.Name.Space == soapNamespace {
				inBody = true
				continue
			}
			if !inBody {
				continue
			}
			if elem.Name.Space != Media2Namespace {
				return nil, "", fmt.Errorf("%w: %s", errInvalidNamespace, elem.Name.Space)
			}
			return env.Body.Inner, elem.Name.Local, nil
		case xml.EndElement:
			if inBody && elem.Name.Local == "Body" && elem.Name.Space == soapNamespace {
				return nil, "", errEmptySOAPBody
			}
		}
	}
}

type soapEnvelope struct {
	XMLNS string   `xml:"xmlns,attr"`
	Body  soapBody `xml:"Body"`
}

type soapBody struct {
	InnerXML string `xml:",innerxml"`
}

func writeSOAP(w http.ResponseWriter, payload []byte) {
	envelope := soapEnvelope{
		XMLNS: soapNamespace,
		Body:  soapBody{InnerXML: string(payload)},
	}
	body, err := xml.Marshal(envelope)
	if err != nil {
		writeFault(w, http.StatusInternalServerError, faultCodeReceiver, "", err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		_ = err
	}
	if _, err := w.Write(body); err != nil {
		_ = err
	}
}

type soapFault struct {
	XMLName xml.Name `xml:"s:Fault"`
	Code    string   `xml:"s:Code>s:Value"`
	Subcode string   `xml:"s:Code>s:Subcode>s:Value,omitempty"`
	Reason  string   `xml:"s:Reason>s:Text"`
}

type soapFaultEnvelope struct {
	XMLNS string    `xml:"xmlns:s,attr"`
	Fault soapFault `xml:"Body>s:Fault"`
}

func writeFault(w http.ResponseWriter, status int, code, subcode, reason string) {
	fault := soapFaultEnvelope{
		XMLNS: soapNamespace,
		Fault: soapFault{
			Code:    code,
			Subcode: subcode,
			Reason:  reason,
		},
	}
	body, err := xml.Marshal(fault)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		_ = err
	}
	if _, err := w.Write(body); err != nil {
		_ = err
	}
}

// Ensure Handler implements http.Handler.
var _ http.Handler = (*Handler)(nil)
