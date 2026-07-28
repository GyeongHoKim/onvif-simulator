package ptzsvc

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
	soapNamespace = "http://www.w3.org/2003/05/soap-envelope"
	maxSOAPBodySize = 10 << 20

	faultCodeSender   = "Sender"
	faultCodeReceiver = "Receiver"
)

var (
	errProviderRequired = errors.New("ptzsvc: provider is required")
	errUnsupportedOp    = errors.New("ptzsvc: unsupported operation")
	errEmptySOAPBody    = errors.New("ptzsvc: empty soap body")
	errDecodePayload    = errors.New("ptzsvc: malformed request payload")
	errInvalidNamespace = errors.New("ptzsvc: unexpected operation namespace")
	errProfileNotFound  = errors.New("ptzsvc: profile not found")
	errInvalidArgs      = errors.New("ptzsvc: invalid argument value")
	errPresetNotFound   = errors.New("ptzsvc: preset not found")
)

// Handler serves the ONVIF PTZ service endpoint.
type Handler struct {
	provider Provider
	auth     AuthHook
	logger   *slog.Logger
}

// Option customizes a PTZ service Handler.
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

// NewHandler creates a PTZ-service HTTP handler.
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

// ServeHTTP dispatches SOAP PTZ-service operations.
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
		h.loggerForRequest(r).Warn("ptz: parse soap envelope", "err", err)
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
			errors.Is(err, errProfileNotFound),
			errors.Is(err, errInvalidArgs),
			errors.Is(err, errPresetNotFound):
			status = http.StatusBadRequest
			code = faultCodeSender
			level = slog.LevelWarn
		}
		h.loggerForRequest(r).LogAttrs(r.Context(), level, "ptz: dispatch fault",
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
	case "GetNodes":
		return h.handleGetNodes(ctx)
	case "GetNode":
		return h.handleGetNode(ctx, payload)
	case "GetConfigurations":
		return h.handleGetConfigurations(ctx)
	case "GetConfiguration":
		return h.handleGetConfiguration(ctx, payload)
	case "GetConfigurationOptions":
		return h.handleGetConfigurationOptions(ctx, payload)
	case "GetStatus":
		return h.handleGetStatus(ctx, payload)
	case "ContinuousMove":
		return h.handleContinuousMove(ctx, payload)
	case "AbsoluteMove":
		return h.handleAbsoluteMove(ctx, payload)
	case "RelativeMove":
		return h.handleRelativeMove(ctx, payload)
	case "Stop":
		return h.handleStop(ctx, payload)
	case "GetPresets":
		return h.handleGetPresets(ctx, payload)
	case "SetPreset":
		return h.handleSetPreset(ctx, payload)
	case "RemovePreset":
		return h.handleRemovePreset(ctx, payload)
	case "GotoPreset":
		return h.handleGotoPreset(ctx, payload)
	case "GotoHomePosition":
		return h.handleGotoHomePosition(ctx, payload)
	case "SetHomePosition":
		return h.handleSetHomePosition(ctx, payload)
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
		XMLNS: PTZNamespace,
		Capabilities: ptzCapabilitiesEnvelope{
			EFlip:                        caps.EFlip,
			Reverse:                      caps.Reverse,
			GetCompatibleConfigurations:  caps.GetCompatibleConfigurations,
			MoveStatus:                   caps.MoveStatus,
			StatusPosition:               caps.StatusPosition,
		},
	})
}

func (h *Handler) handleGetNodes(ctx context.Context) ([]byte, error) {
	nodes, err := h.provider.GetNodes(ctx)
	if err != nil {
		return nil, err
	}
	envs := make([]ptzNodeEnvelope, len(nodes))
	for i := range nodes {
		envs[i] = ptzNodeEnvelope{Token: nodes[i].Token, Name: nodes[i].Name}
	}
	return xml.Marshal(getNodesResponse{
		XMLNS:   PTZNamespace,
		XMLNSTT: SchemaNamespace,
		Nodes:   envs,
	})
}

func (h *Handler) handleGetNode(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		NodeToken string `xml:"NodeToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GetNode: %v", errDecodePayload, err)
	}
	node, err := h.provider.GetNode(ctx, req.NodeToken)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getNodeResponse{
		XMLNS:   PTZNamespace,
		XMLNSTT: SchemaNamespace,
		Node:    ptzNodeEnvelope{Token: node.Token, Name: node.Name},
	})
}

func (h *Handler) handleGetConfigurations(ctx context.Context) ([]byte, error) {
	cfgs, err := h.provider.GetConfigurations(ctx)
	if err != nil {
		return nil, err
	}
	envs := make([]ptzConfigurationEnvelope, len(cfgs))
	for i := range cfgs {
		envs[i] = ptzConfigurationEnvelope{
			Token:     cfgs[i].Token,
			Name:      cfgs[i].Name,
			UseCount:  cfgs[i].UseCount,
			NodeToken: cfgs[i].NodeToken,
		}
	}
	return xml.Marshal(getConfigurationsResponse{
		XMLNS:          PTZNamespace,
		XMLNSTT:        SchemaNamespace,
		Configurations: envs,
	})
}

func (h *Handler) handleGetConfiguration(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		PTZConfigurationToken string `xml:"PTZConfigurationToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GetConfiguration: %v", errDecodePayload, err)
	}
	cfg, err := h.provider.GetConfiguration(ctx, req.PTZConfigurationToken)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getConfigurationResponse{
		XMLNS: PTZNamespace,
		XMLNSTT: SchemaNamespace,
		Configuration: ptzConfigurationEnvelope{
			Token:     cfg.Token,
			Name:      cfg.Name,
			UseCount:  cfg.UseCount,
			NodeToken: cfg.NodeToken,
		},
	})
}

func (h *Handler) handleGetConfigurationOptions(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ConfigurationToken string `xml:"ConfigurationToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GetConfigurationOptions: %v", errDecodePayload, err)
	}
	opts, err := h.provider.GetConfigurationOptions(ctx, req.ConfigurationToken)
	if err != nil {
		return nil, err
	}

	ptRanges := make([]spaceRangeEnv, len(opts.PanTiltPositionSpaceRange))
	for i, r := range opts.PanTiltPositionSpaceRange {
		ptRanges[i] = spaceRangeEnv{
			URI:   r.URI,
			XRange: intRangeEnv{Min: r.XRange.Min, Max: r.XRange.Max},
			YRange: intRangeEnv{Min: r.YRange.Min, Max: r.YRange.Max},
		}
	}
	zRanges := make([]spaceRangeEnv, len(opts.ZoomPositionSpaceRange))
	for i, r := range opts.ZoomPositionSpaceRange {
		zRanges[i] = spaceRangeEnv{
			URI:   r.URI,
			XRange: intRangeEnv{Min: r.XRange.Min, Max: r.XRange.Max},
		}
	}

	return xml.Marshal(getConfigurationOptionsResponse{
		XMLNS: PTZNamespace,
		XMLNSTT: SchemaNamespace,
		Options: ptzConfigurationOptionsEnv{
			PanTiltPositionSpaceRange: ptRanges,
			ZoomPositionSpaceRange:    zRanges,
			PTZTimeout: intRangeEnv{
				Min: float64(opts.PTZTimeout.Min),
				Max: float64(opts.PTZTimeout.Max),
			},
		},
	})
}

func (h *Handler) handleGetStatus(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string `xml:"ProfileToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GetStatus: %v", errDecodePayload, err)
	}
	st, err := h.provider.GetStatus(ctx, req.ProfileToken)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(getStatusResponse{
		XMLNS:   PTZNamespace,
		XMLNSTT: SchemaNamespace,
		Status: statusEnvelope{
			Position:   vectorToEnvelope(st.Position),
			MoveStatus: moveStatusEnv{
				PanTilt: st.MoveStatus.PanTilt,
				Zoom:    st.MoveStatus.Zoom,
			},
		},
	})
}

func (h *Handler) handleContinuousMove(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string         `xml:"ProfileToken"`
		Velocity     speedEnvelope  `xml:"Velocity"`
		Timeout      *string        `xml:"Timeout"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode ContinuousMove: %v", errDecodePayload, err)
	}
	vel := envelopeToSpeed(&req.Velocity)
	if err := h.provider.ContinuousMove(ctx, req.ProfileToken, vel, req.Timeout); err != nil {
		return nil, err
	}
	return xml.Marshal(continuousMoveResponse{XMLNS: PTZNamespace})
}

func (h *Handler) handleAbsoluteMove(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string          `xml:"ProfileToken"`
		Position     vectorEnvelope  `xml:"Position"`
		Speed        *speedEnvelope  `xml:"Speed"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode AbsoluteMove: %v", errDecodePayload, err)
	}
	pos := envelopeToVector(&req.Position)
	var spd *Speed
	if req.Speed != nil {
		s := envelopeToSpeed(req.Speed)
		spd = &s
	}
	if err := h.provider.AbsoluteMove(ctx, req.ProfileToken, pos, spd); err != nil {
		return nil, err
	}
	return xml.Marshal(absoluteMoveResponse{XMLNS: PTZNamespace})
}

func (h *Handler) handleRelativeMove(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken  string          `xml:"ProfileToken"`
		Translation   vectorEnvelope  `xml:"Translation"`
		Speed         *speedEnvelope  `xml:"Speed"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode RelativeMove: %v", errDecodePayload, err)
	}
	trans := envelopeToVector(&req.Translation)
	var spd *Speed
	if req.Speed != nil {
		s := envelopeToSpeed(req.Speed)
		spd = &s
	}
	if err := h.provider.RelativeMove(ctx, req.ProfileToken, trans, spd); err != nil {
		return nil, err
	}
	return xml.Marshal(relativeMoveResponse{XMLNS: PTZNamespace})
}

func (h *Handler) handleStop(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string  `xml:"ProfileToken"`
		PanTilt      *bool   `xml:"PanTilt"`
		Zoom         *bool   `xml:"Zoom"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode Stop: %v", errDecodePayload, err)
	}
	if err := h.provider.Stop(ctx, req.ProfileToken, req.PanTilt, req.Zoom); err != nil {
		return nil, err
	}
	return xml.Marshal(stopResponse{XMLNS: PTZNamespace})
}

func (h *Handler) handleGetPresets(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string `xml:"ProfileToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GetPresets: %v", errDecodePayload, err)
	}
	presets, err := h.provider.GetPresets(ctx, req.ProfileToken)
	if err != nil {
		return nil, err
	}
	envs := make([]presetEnvelope, len(presets))
	for i := range presets {
		envs[i] = presetEnvelope{
			Token:    presets[i].Token,
			Name:     presets[i].Name,
			Position: vectorToEnvelope(presets[i].Position),
		}
	}
	return xml.Marshal(getPresetsResponse{
		XMLNS:   PTZNamespace,
		XMLNSTT: SchemaNamespace,
		Presets: envs,
	})
}

func (h *Handler) handleSetPreset(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string  `xml:"ProfileToken"`
		PresetName   *string `xml:"PresetName"`
		PresetToken  *string `xml:"PresetToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode SetPreset: %v", errDecodePayload, err)
	}
	token, err := h.provider.SetPreset(ctx, req.ProfileToken, req.PresetName, req.PresetToken)
	if err != nil {
		return nil, err
	}
	return xml.Marshal(setPresetResponse{
		XMLNS:       PTZNamespace,
		PresetToken: token,
	})
}

func (h *Handler) handleRemovePreset(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string `xml:"ProfileToken"`
		PresetToken  string `xml:"PresetToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode RemovePreset: %v", errDecodePayload, err)
	}
	if err := h.provider.RemovePreset(ctx, req.ProfileToken, req.PresetToken); err != nil {
		return nil, err
	}
	return xml.Marshal(removePresetResponse{XMLNS: PTZNamespace})
}

func (h *Handler) handleGotoPreset(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string         `xml:"ProfileToken"`
		PresetToken  string         `xml:"PresetToken"`
		Speed        *speedEnvelope `xml:"Speed"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GotoPreset: %v", errDecodePayload, err)
	}
	var spd *Speed
	if req.Speed != nil {
		s := envelopeToSpeed(req.Speed)
		spd = &s
	}
	if err := h.provider.GotoPreset(ctx, req.ProfileToken, req.PresetToken, spd); err != nil {
		return nil, err
	}
	return xml.Marshal(gotoPresetResponse{XMLNS: PTZNamespace})
}

func (h *Handler) handleGotoHomePosition(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string         `xml:"ProfileToken"`
		Speed        *speedEnvelope `xml:"Speed"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode GotoHomePosition: %v", errDecodePayload, err)
	}
	var spd *Speed
	if req.Speed != nil {
		s := envelopeToSpeed(req.Speed)
		spd = &s
	}
	if err := h.provider.GotoHomePosition(ctx, req.ProfileToken, spd); err != nil {
		return nil, err
	}
	return xml.Marshal(gotoHomePositionResponse{XMLNS: PTZNamespace})
}

func (h *Handler) handleSetHomePosition(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		ProfileToken string `xml:"ProfileToken"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("%w: decode SetHomePosition: %v", errDecodePayload, err)
	}
	if err := h.provider.SetHomePosition(ctx, req.ProfileToken); err != nil {
		return nil, err
	}
	return xml.Marshal(setHomePositionResponse{XMLNS: PTZNamespace})
}

// ---------- Conversion helpers ----------

func vectorToEnvelope(v *Vector) *vectorEnvelope {
	if v == nil {
		return nil
	}
	env := &vectorEnvelope{}
	if v.PanTilt != nil {
		env.PanTilt = &panTiltEnv{X: v.PanTilt.X, Y: v.PanTilt.Y}
	}
	if v.Zoom != nil {
		env.Zoom = &zoomEnv{X: v.Zoom.X}
	}
	return env
}

func envelopeToVector(env *vectorEnvelope) Vector {
	if env == nil {
		return Vector{}
	}
	v := Vector{}
	if env.PanTilt != nil {
		v.PanTilt = &PanTilt{X: env.PanTilt.X, Y: env.PanTilt.Y}
	}
	if env.Zoom != nil {
		v.Zoom = &Zoom{X: env.Zoom.X}
	}
	return v
}

// speedEnvelope is used for both request and response speed elements.
type speedEnvelope struct {
	PanTilt *panTiltEnv `xml:"tt:PanTilt,omitempty"`
	Zoom    *zoomEnv    `xml:"tt:Zoom,omitempty"`
}

func envelopeToSpeed(env *speedEnvelope) Speed {
	if env == nil {
		return Speed{}
	}
	s := Speed{}
	if env.PanTilt != nil {
		s.PanTilt = &PanTilt{X: env.PanTilt.X, Y: env.PanTilt.Y}
	}
	if env.Zoom != nil {
		s.Zoom = &Zoom{X: env.Zoom.X}
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
			if elem.Name.Space != PTZNamespace {
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
	XMLNS   string   `xml:"xmlns,attr"`
	Body    soapBody `xml:"Body"`
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
