package imgsvc

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
	errProviderRequired = errors.New("imgsvc: provider is required")
	errUnsupportedOp    = errors.New("imgsvc: unsupported operation")
	errEmptySOAPBody    = errors.New("imgsvc: empty soap body")
	errDecodePayload    = errors.New("imgsvc: malformed request payload")
	errInvalidNamespace = errors.New("imgsvc: unexpected operation namespace")
)

// Handler serves the ONVIF Imaging service endpoint.
type Handler struct {
	provider Provider
	auth     AuthHook
	logger   *slog.Logger
}

// Option customizes an Imaging service Handler.
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

// NewHandler creates an Imaging-service HTTP handler.
func NewHandler(provider Provider, opts ...Option) *Handler {
	if provider == nil {
		panic(errProviderRequired)
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

// ServeHTTP dispatches SOAP Imaging-service operations.
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
		writeFault(w, http.StatusBadRequest, faultCodeSender, "", fmt.Errorf("read request body: %w", err).Error())
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
				h.writeAuthFault(w, challengeErr)
				return
			}
		}
		h.loggerForRequest(r).Warn("imaging: parse soap envelope", "err", err)
		writeFault(w, http.StatusBadRequest, faultCodeSender, "", err.Error())
		return
	}

	if authErr := h.auth.Authorize(r.Context(), operation, r); authErr != nil {
		h.writeAuthFault(w, authErr)
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
			errors.Is(err, ErrInvalidArgs),
			errors.Is(err, ErrSourceNotFound),
			errors.Is(err, ErrPresetNotFound):
			status = http.StatusBadRequest
			code = faultCodeSender
			level = slog.LevelWarn
		}
		h.loggerForRequest(r).LogAttrs(r.Context(), level, "imaging: dispatch fault",
			slog.String("operation", operation),
			slog.Int("status", status),
			slog.String("err", err.Error()),
		)
		writeFault(w, status, code, "", err.Error())
		return
	}
	writeSOAP(w, respPayload)
}

func (h *Handler) writeAuthFault(w http.ResponseWriter, authErr error) {
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

//nolint:cyclop // dispatch is a straightforward operation router
func (h *Handler) dispatch(ctx context.Context, operation string, payload []byte) ([]byte, error) {
	switch operation {
	case "GetServiceCapabilities":
		return h.handleGetServiceCapabilities(ctx)
	case "GetImagingSettings":
		return h.handleGetImagingSettings(ctx, payload)
	case "SetImagingSettings":
		return h.handleSetImagingSettings(ctx, payload)
	case "GetOptions":
		return h.handleGetOptions(ctx, payload)
	case "GetStatus":
		return h.handleGetStatus(ctx, payload)
	case "GetPresets":
		return h.handleGetPresets(ctx, payload)
	case "GetCurrentPreset":
		return h.handleGetCurrentPreset(ctx, payload)
	case "SetCurrentPreset":
		return h.handleSetCurrentPreset(ctx, payload)
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
		XMLNS: ImagingNamespace,
		Capabilities: imgCapabilitiesEnvelope{
			ImageStabilization: caps.ImageStabilization,
		},
	})
}

func (h *Handler) handleGetImagingSettings(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		VideoSourceToken string `xml:"VideoSourceToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GetImagingSettings: %v", errDecodePayload, err)
	}
	settings, err := h.provider.GetImagingSettings(ctx, req.VideoSourceToken)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getImagingSettingsResponse{
		XMLNS: ImagingNamespace,
		XMLNSTT: SchemaNamespace,
		ImagingSettings: settingsToEnvelope(&settings),
	})
}

func (h *Handler) handleSetImagingSettings(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		VideoSourceToken  string                 `xml:"VideoSourceToken"`
		ImagingSettings   imagingSettingsEnvelope `xml:"ImagingSettings"`
		ForcePersistence  *bool                  `xml:"ForcePersistence"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode SetImagingSettings: %v", errDecodePayload, err)
	}
	settings := envelopeToSettings(&req.ImagingSettings)
	if err := h.provider.SetImagingSettings(ctx, req.VideoSourceToken, settings, req.ForcePersistence); err != nil {
		return nil, err
	}
	return xml.Marshal(setImagingSettingsResponse{XMLNS: ImagingNamespace})
}

func (h *Handler) handleGetOptions(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		VideoSourceToken string `xml:"VideoSourceToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GetOptions: %v", errDecodePayload, err)
	}
	opts, err := h.provider.GetOptions(ctx, req.VideoSourceToken)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getOptionsResponse{
		XMLNS: ImagingNamespace,
		XMLNSTT: SchemaNamespace,
		ImagingOptions: imagingOptionsEnvelope{
			Brightness: floatRangeEnv{Min: opts.Brightness.Min, Max: opts.Brightness.Max},
			Contrast:   floatRangeEnv{Min: opts.Contrast.Min, Max: opts.Contrast.Max},
			Sharpness:  floatRangeEnv{Min: opts.Sharpness.Min, Max: opts.Sharpness.Max},
			ExposureModes: stringListEnv{Items: opts.ExposureModes},
			ExposurePriorities: stringListEnv{Items: opts.ExposurePriorities},
			WhiteBalanceModes: stringListEnv{Items: opts.WhiteBalanceModes},
			IrCutFilterModes: stringListEnv{Items: opts.IrCutFilterModes},
			BacklightCompModes: stringListEnv{Items: opts.BacklightCompModes},
			WideDynamicRangeModes: stringListEnv{Items: opts.WideDynamicRangeModes},
			FocusModes: stringListEnv{Items: opts.FocusModes},
		},
	})
}

func (h *Handler) handleGetStatus(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		VideoSourceToken string `xml:"VideoSourceToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GetStatus: %v", errDecodePayload, err)
	}
	status, err := h.provider.GetStatus(ctx, req.VideoSourceToken)
	if err != nil {
		return nil, err
	}
	resp := getStatusResponse{
		XMLNS: ImagingNamespace,
		XMLNSTT: SchemaNamespace,
	}
	if status.FocusStatus != nil {
		resp.Status.FocusStatus = &focusStatusEnv{
			Position: status.FocusStatus.Position,
			Error:    status.FocusStatus.Error,
		}
	}
	return xml.Marshal(resp)
}

func (h *Handler) handleGetPresets(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		VideoSourceToken string `xml:"VideoSourceToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GetPresets: %v", errDecodePayload, err)
	}
	presets, err := h.provider.GetPresets(ctx, req.VideoSourceToken)
	if err != nil {
		return nil, err
	}
	envs := make([]imagingPresetEnv, len(presets))
	for i := range presets {
		envs[i] = imagingPresetEnv{
			Token: presets[i].Token,
			Name:  presets[i].Name,
			Type:  presets[i].Type,
		}
	}
	return xml.Marshal(getPresetsResponse{
		XMLNS:   ImagingNamespace,
		Presets: envs,
	})
}

func (h *Handler) handleGetCurrentPreset(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		VideoSourceToken string `xml:"VideoSourceToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GetCurrentPreset: %v", errDecodePayload, err)
	}
	preset, err := h.provider.GetCurrentPreset(ctx, req.VideoSourceToken)
	if err != nil {
		return nil, err
	}
	resp := getCurrentPresetResponse{
		XMLNS: ImagingNamespace,
	}
	if preset != nil {
		resp.Preset = &imagingPresetEnv{
			Token: preset.Token,
			Name:  preset.Name,
			Type:  preset.Type,
		}
	}
	return xml.Marshal(resp)
}

func (h *Handler) handleSetCurrentPreset(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		VideoSourceToken string `xml:"VideoSourceToken"`
		PresetToken      string `xml:"PresetToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode SetCurrentPreset: %v", errDecodePayload, err)
	}
	if err := h.provider.SetCurrentPreset(ctx, req.VideoSourceToken, req.PresetToken); err != nil {
		return nil, err
	}
	return xml.Marshal(setCurrentPresetResponse{XMLNS: ImagingNamespace})
}

// ---------- Conversion helpers ----------

func settingsToEnvelope(s *Settings) imagingSettingsEnvelope {
	env := imagingSettingsEnvelope{}
	if s.Brightness != nil {
		env.Brightness = &brightnessVal{Value: *s.Brightness}
	}
	if s.Contrast != nil {
		env.Contrast = &contrastVal{Value: *s.Contrast}
	}
	if s.Sharpness != nil {
		env.Sharpness = &sharpnessVal{Value: *s.Sharpness}
	}
	if s.ExposureMode != nil {
		env.Exposure = &exposureEnv{Mode: *s.ExposureMode}
		if s.ExposurePriority != nil {
			env.Exposure.Priority = *s.ExposurePriority
		}
	}
	if s.WhiteBalanceMode != nil {
		env.WhiteBalance = &whiteBalanceEnv{Mode: *s.WhiteBalanceMode}
	}
	if s.IrCutFilter != nil {
		env.IrCutFilter = &irCutFilterVal{Value: *s.IrCutFilter}
	}
	if s.BacklightCompMode != nil {
		env.BacklightCompensation = &backlightCompEnv{Mode: *s.BacklightCompMode}
		if s.BacklightCompLevel != nil {
			env.BacklightCompensation.Level = *s.BacklightCompLevel
		}
	}
	if s.WideDynamicRangeMode != nil {
		env.WideDynamicRange = &wideDynamicRangeEnv{Mode: *s.WideDynamicRangeMode}
		if s.WideDynamicRangeLevel != nil {
			env.WideDynamicRange.Level = *s.WideDynamicRangeLevel
		}
	}
	if s.FocusMode != nil {
		env.Focus = &focusSettingsEnv{FocusMode: *s.FocusMode}
	}
	return env
}

func envelopeToSettings(env *imagingSettingsEnvelope) Settings {
	s := Settings{}
	if env.Brightness != nil {
		s.Brightness = &env.Brightness.Value
	}
	if env.Contrast != nil {
		s.Contrast = &env.Contrast.Value
	}
	if env.Sharpness != nil {
		s.Sharpness = &env.Sharpness.Value
	}
	if env.Exposure != nil {
		s.ExposureMode = &env.Exposure.Mode
		if env.Exposure.Priority != "" {
			s.ExposurePriority = &env.Exposure.Priority
		}
	}
	if env.WhiteBalance != nil {
		s.WhiteBalanceMode = &env.WhiteBalance.Mode
	}
	if env.IrCutFilter != nil {
		s.IrCutFilter = &env.IrCutFilter.Value
	}
	if env.BacklightCompensation != nil {
		s.BacklightCompMode = &env.BacklightCompensation.Mode
		s.BacklightCompLevel = &env.BacklightCompensation.Level
	}
	if env.WideDynamicRange != nil {
		s.WideDynamicRangeMode = &env.WideDynamicRange.Mode
		s.WideDynamicRangeLevel = &env.WideDynamicRange.Level
	}
	if env.Focus != nil {
		s.FocusMode = &env.Focus.FocusMode
	}
	return s
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
			if elem.Name.Space != ImagingNamespace {
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
