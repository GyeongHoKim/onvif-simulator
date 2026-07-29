package deviceiosvc

import (
	"context"
	"errors"
	"net/http"
)

const (
	// DeviceIOServicePath is the ONVIF DeviceIO service endpoint path.
	DeviceIOServicePath = "/onvif/deviceio_service"

	// DeviceIONamespace is the ONVIF DeviceIO WSDL namespace.
	DeviceIONamespace = "http://www.onvif.org/ver10/deviceio/wsdl"
)

// Sentinel errors.
var (
	ErrProviderRequired = errors.New("deviceiosvc: provider is required")
)

// ServiceCapabilities is the GetServiceCapabilities response payload.
type ServiceCapabilities struct {
	RelayOutputs  int
	DigitalInputs int
}

// Provider supplies DeviceIO operation data to the Handler.
type Provider interface {
	ServiceCapabilities(ctx context.Context) (ServiceCapabilities, error)
}

// AuthHook authorizes a DeviceIO operation.
type AuthHook interface {
	Authorize(ctx context.Context, operation string, r *http.Request) error
}

// AuthFunc is a function adapter for AuthHook.
type AuthFunc func(ctx context.Context, operation string, r *http.Request) error

// Authorize implements AuthHook.
func (f AuthFunc) Authorize(ctx context.Context, operation string, r *http.Request) error {
	return f(ctx, operation, r)
}
