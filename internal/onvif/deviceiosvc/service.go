package deviceiosvc

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
)

const (
	soapNamespace   = "http://www.w3.org/2003/05/soap-envelope"
	maxSOAPBodySize = 10 << 20

	faultCodeSender   = "Sender"
	faultCodeReceiver = "Receiver"
)

var (
	errUnsupportedOp    = errors.New("deviceiosvc: unsupported operation")
	errEmptySOAPBody    = errors.New("deviceiosvc: empty soap body")
	errInvalidNamespace = errors.New("deviceiosvc: unexpected operation namespace")
)

// Handler serves the ONVIF DeviceIO service endpoint.
type Handler struct {
	provider Provider
	auth     AuthHook
	logger   *slog.Logger
}

// Option customizes a DeviceIO service Handler.
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

// NewHandler creates a DeviceIO-service HTTP handler.
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

// ServeHTTP dispatches SOAP DeviceIO-service operations.
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
		writeFault(w, http.StatusBadRequest, faultCodeSender,
			fmt.Errorf("read request body: %w", err).Error())
		return
	}
	if closeErr := r.Body.Close(); closeErr != nil {
		writeFault(w, http.StatusBadRequest, faultCodeSender, closeErr.Error())
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))

	payload, operation, err := parseOperation(raw)
	if err != nil {
		h.loggerForRequest(r).Warn("deviceio: parse soap envelope", "err", err)
		writeFault(w, http.StatusBadRequest, faultCodeSender, err.Error())
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
		if errors.Is(err, errUnsupportedOp) {
			status = http.StatusNotImplemented
			code = faultCodeSender
			level = slog.LevelWarn
		}
		h.loggerForRequest(r).LogAttrs(r.Context(), level, "deviceio: dispatch fault",
			slog.String("operation", operation),
			slog.Int("status", status),
			slog.String("err", err.Error()),
		)
		writeFault(w, status, code, err.Error())
		return
	}
	writeSOAP(w, respPayload)
}

// writeAuthFault writes a SOAP fault for authentication errors.
func writeAuthFault(w http.ResponseWriter, authErr error) {
	status := http.StatusUnauthorized
	writeFault(w, status, faultCodeSender, authErr.Error())
}

func (h *Handler) dispatch(ctx context.Context, operation string, _ []byte) ([]byte, error) {
	if operation == "GetServiceCapabilities" {
		return h.handleGetServiceCapabilities(ctx)
	}
	return nil, fmt.Errorf("%w: %s", errUnsupportedOp, operation)
}

func (h *Handler) handleGetServiceCapabilities(ctx context.Context) ([]byte, error) {
	caps, err := h.provider.ServiceCapabilities(ctx)
	if err != nil {
		return nil, err
	}
	env := deviceIOCapabilitiesEnvelope(caps)
	return xml.Marshal(getServiceCapabilitiesResponse{
		XMLNS:        DeviceIONamespace,
		Capabilities: env,
	})
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
			if elem.Name.Space != DeviceIONamespace {
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
		writeFault(w, http.StatusInternalServerError, faultCodeReceiver, err.Error())
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
	Reason  string   `xml:"s:Reason>s:Text"`
}

type soapFaultEnvelope struct {
	XMLNS string    `xml:"xmlns:s,attr"`
	Fault soapFault `xml:"Body>s:Fault"`
}

// writeFault writes a SOAP fault response.
func writeFault(w http.ResponseWriter, status int, code, reason string) {
	fault := soapFaultEnvelope{
		XMLNS: soapNamespace,
		Fault: soapFault{
			Code:   code,
			Reason: reason,
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
