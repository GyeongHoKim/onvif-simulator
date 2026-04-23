package mediasvc

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
)

const (
	soapNamespace = "http://www.w3.org/2003/05/soap-envelope"
	// maxSOAPBodySize caps incoming SOAP payload bytes to avoid unbounded
	// memory use during io.ReadAll. Matches devicesvc.
	maxSOAPBodySize = 10 << 20

	faultCodeSender   = "Sender"
	faultCodeReceiver = "Receiver"
)

// Sentinel errors returned by dispatch and Provider implementations. They
// map to SOAP fault codes and HTTP statuses in ServeHTTP.
var (
	errProviderRequired = errors.New("mediasvc: provider is required")
	errUnsupportedOp    = errors.New("mediasvc: unsupported operation")
	errEmptySOAPBody    = errors.New("mediasvc: empty soap body")
	errDecodePayload    = errors.New("mediasvc: malformed request payload")

	// ErrProfileNotFound indicates the requested profile token does not
	// exist. Provider implementations return this; the Handler maps it to
	// HTTP 400 + SOAP fault code Sender (ONVIF ter:NoProfile).
	ErrProfileNotFound = errors.New("mediasvc: profile not found")
	// ErrConfigNotFound indicates the requested configuration token does
	// not exist.
	ErrConfigNotFound = errors.New("mediasvc: configuration not found")
	// ErrInvalidArgs indicates client-supplied argument values are not
	// valid (e.g. non-supported encoding, negative bitrate).
	ErrInvalidArgs = errors.New("mediasvc: invalid argument value")
	// ErrNoSnapshot indicates the profile has no snapshot URI configured.
	ErrNoSnapshot = errors.New("mediasvc: snapshot uri not available")
)

// Handler serves the ONVIF media service endpoint.
type Handler struct {
	provider Provider
	auth     AuthHook
}

// Option customizes a media service Handler.
type Option func(*Handler)

// WithAuthHook installs a request authorization hook.
func WithAuthHook(hook AuthHook) Option {
	return func(h *Handler) {
		if hook != nil {
			h.auth = hook
		}
	}
}

// NewHandler creates a media-service HTTP handler.
func NewHandler(provider Provider, opts ...Option) *Handler {
	if provider == nil {
		panic(errProviderRequired)
	}
	h := &Handler{
		provider: provider,
		auth:     AuthFunc(func(context.Context, string, *http.Request) error { return nil }),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// ServeHTTP dispatches SOAP media-service operations.
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
			writeFault(w, http.StatusRequestEntityTooLarge, faultCodeSender, tooLarge.Error())
			return
		}
		writeFault(w, http.StatusBadRequest, faultCodeSender, fmt.Errorf("read request body: %w", err).Error())
		return
	}
	if closeErr := r.Body.Close(); closeErr != nil {
		writeFault(w, http.StatusBadRequest, faultCodeSender, closeErr.Error())
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))

	payload, operation, err := parseOperation(raw)
	if err != nil {
		writeFault(w, http.StatusBadRequest, faultCodeSender, err.Error())
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
		switch {
		case errors.Is(err, errUnsupportedOp):
			status = http.StatusNotImplemented
			code = faultCodeSender
		case errors.Is(err, errDecodePayload),
			errors.Is(err, ErrProfileNotFound),
			errors.Is(err, ErrConfigNotFound),
			errors.Is(err, ErrInvalidArgs),
			errors.Is(err, ErrNoSnapshot):
			status = http.StatusBadRequest
			code = faultCodeSender
		}
		writeFault(w, status, code, err.Error())
		return
	}
	writeSOAP(w, respPayload)
}

// writeAuthFault mirrors devicesvc: 401 with copied challenge headers for
// ChallengeError, 403 for ErrForbidden, 401 otherwise.
func (*Handler) writeAuthFault(w http.ResponseWriter, authErr error) {
	status := http.StatusUnauthorized
	var challenge *auth.ChallengeError
	if errors.As(authErr, &challenge) {
		if challenge.Status != 0 {
			status = challenge.Status
		}
		for k, vs := range challenge.Headers {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
	}
	if errors.Is(authErr, auth.ErrForbidden) && status == http.StatusUnauthorized {
		status = http.StatusForbidden
	}
	writeFault(w, status, faultCodeSender, authErr.Error())
}

func (h *Handler) dispatch(ctx context.Context, operation string, payload []byte) ([]byte, error) {
	if resp, handled, err := h.dispatchProfile(ctx, operation, payload); handled {
		return resp, err
	}
	if resp, handled, err := h.dispatchVideoSource(ctx, operation, payload); handled {
		return resp, err
	}
	if resp, handled, err := h.dispatchVideoEncoder(ctx, operation, payload); handled {
		return resp, err
	}
	if resp, handled, err := h.dispatchURI(ctx, operation, payload); handled {
		return resp, err
	}
	return nil, fmt.Errorf("%w: %s", errUnsupportedOp, operation)
}

func (h *Handler) dispatchProfile(ctx context.Context, op string, payload []byte) (
	resp []byte, handled bool, err error,
) {
	switch op {
	case "GetServiceCapabilities":
		resp, err := h.handleGetServiceCapabilities(ctx)
		return resp, true, err
	case "GetProfiles":
		resp, err := h.handleGetProfiles(ctx)
		return resp, true, err
	case "GetProfile":
		resp, err := h.handleGetProfile(ctx, payload)
		return resp, true, err
	case "CreateProfile":
		resp, err := h.handleCreateProfile(ctx, payload)
		return resp, true, err
	case "DeleteProfile":
		resp, err := h.handleDeleteProfile(ctx, payload)
		return resp, true, err
	}
	return nil, false, nil
}

func (h *Handler) dispatchVideoSource(ctx context.Context, op string, payload []byte) (
	resp []byte, handled bool, err error,
) {
	switch op {
	case "GetVideoSources":
		resp, err := h.handleGetVideoSources(ctx)
		return resp, true, err
	case "GetVideoSourceConfigurations":
		resp, err := h.handleGetVideoSourceConfigurations(ctx)
		return resp, true, err
	case "GetVideoSourceConfiguration":
		resp, err := h.handleGetVideoSourceConfiguration(ctx, payload)
		return resp, true, err
	case "SetVideoSourceConfiguration":
		resp, err := h.handleSetVideoSourceConfiguration(ctx, payload)
		return resp, true, err
	case "AddVideoSourceConfiguration":
		resp, err := h.handleAddVideoSourceConfiguration(ctx, payload)
		return resp, true, err
	case "RemoveVideoSourceConfiguration":
		resp, err := h.handleRemoveVideoSourceConfiguration(ctx, payload)
		return resp, true, err
	case "GetCompatibleVideoSourceConfigurations":
		resp, err := h.handleGetCompatibleVideoSourceConfigurations(ctx, payload)
		return resp, true, err
	case "GetVideoSourceConfigurationOptions":
		resp, err := h.handleGetVideoSourceConfigurationOptions(ctx, payload)
		return resp, true, err
	}
	return nil, false, nil
}

func (h *Handler) dispatchVideoEncoder(ctx context.Context, op string, payload []byte) (
	resp []byte, handled bool, err error,
) {
	switch op {
	case "GetVideoEncoderConfigurations":
		resp, err := h.handleGetVideoEncoderConfigurations(ctx)
		return resp, true, err
	case "GetVideoEncoderConfiguration":
		resp, err := h.handleGetVideoEncoderConfiguration(ctx, payload)
		return resp, true, err
	case "SetVideoEncoderConfiguration":
		resp, err := h.handleSetVideoEncoderConfiguration(ctx, payload)
		return resp, true, err
	case "AddVideoEncoderConfiguration":
		resp, err := h.handleAddVideoEncoderConfiguration(ctx, payload)
		return resp, true, err
	case "RemoveVideoEncoderConfiguration":
		resp, err := h.handleRemoveVideoEncoderConfiguration(ctx, payload)
		return resp, true, err
	case "GetCompatibleVideoEncoderConfigurations":
		resp, err := h.handleGetCompatibleVideoEncoderConfigurations(ctx, payload)
		return resp, true, err
	case "GetVideoEncoderConfigurationOptions":
		resp, err := h.handleGetVideoEncoderConfigurationOptions(ctx, payload)
		return resp, true, err
	}
	return nil, false, nil
}

func (h *Handler) dispatchURI(ctx context.Context, op string, payload []byte) (
	resp []byte, handled bool, err error,
) {
	switch op {
	case "GetStreamUri":
		resp, err := h.handleGetStreamURI(ctx, payload)
		return resp, true, err
	case "GetSnapshotUri":
		resp, err := h.handleGetSnapshotURI(ctx, payload)
		return resp, true, err
	}
	return nil, false, nil
}

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
	decoder := xml.NewDecoder(bytes.NewReader(env.Body.Inner))
	for {
		tok, err := decoder.Token()
		if err != nil {
			return nil, "", fmt.Errorf("parse soap body: %w", err)
		}
		if start, ok := tok.(xml.StartElement); ok {
			return env.Body.Inner, start.Name.Local, nil
		}
	}
}

func writeSOAP(w http.ResponseWriter, payload []byte) {
	envelope := soapEnvelope{
		XMLNSEnv: soapNamespace,
		Body: soapBody{
			InnerXML: string(payload),
		},
	}
	body, err := xml.Marshal(envelope)
	if err != nil {
		writeFault(w, http.StatusInternalServerError, faultCodeReceiver, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return
	}
	if _, err := w.Write(body); err != nil {
		return
	}
}

func writeFault(w http.ResponseWriter, status int, code, reason string) {
	innerXML := fmt.Sprintf(
		"<env:Fault xmlns:env=%q><env:Code><env:Value>env:%s</env:Value></env:Code>"+
			"<env:Reason><env:Text xml:lang=%q>%s</env:Text></env:Reason></env:Fault>",
		soapNamespace,
		xmlEscape(code),
		"en",
		xmlEscape(reason),
	)
	fault := soapEnvelope{
		XMLNSEnv: soapNamespace,
		Body:     soapBody{InnerXML: innerXML},
	}
	body, err := xml.Marshal(fault)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return
	}
	if _, err := w.Write(body); err != nil {
		return
	}
}

func xmlEscape(value string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
		"'", "&apos;",
	)
	return replacer.Replace(value)
}

type soapEnvelope struct {
	XMLName  xml.Name `xml:"env:Envelope"`
	XMLNSEnv string   `xml:"xmlns:env,attr"`
	Body     soapBody `xml:"env:Body"`
}

type soapBody struct {
	InnerXML string `xml:",innerxml"`
}
