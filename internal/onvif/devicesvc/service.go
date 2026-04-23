package devicesvc

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const (
	soapNamespace = "http://www.w3.org/2003/05/soap-envelope"
)

var (
	errProviderRequired = errors.New("devicesvc: provider is required")
	errUnsupportedOp    = errors.New("devicesvc: unsupported operation")
	errNoServices       = errors.New("devicesvc: no services available")
	errEmptySOAPBody    = errors.New("devicesvc: empty soap body")
)

// Handler serves the ONVIF device management endpoint.
type Handler struct {
	provider Provider
	auth     AuthHook
}

// Option customizes a Device Service.
type Option func(*Handler)

// WithAuthHook installs a request authorization hook.
func WithAuthHook(hook AuthHook) Option {
	return func(s *Handler) {
		if hook != nil {
			s.auth = hook
		}
	}
}

// NewHandler creates a Device Service HTTP handler.
func NewHandler(provider Provider, opts ...Option) *Handler {
	if provider == nil {
		panic(errProviderRequired)
	}
	svc := &Handler{
		provider: provider,
		auth:     AuthFunc(func(context.Context, string, *http.Request) error { return nil }),
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// ServeHTTP dispatches SOAP device-management operations.
func (s *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	payload, operation, err := readOperation(r)
	if err != nil {
		writeFault(w, http.StatusBadRequest, "Sender", err.Error())
		return
	}

	authErr := s.auth.Authorize(r.Context(), operation, r)
	if authErr != nil {
		writeFault(w, http.StatusUnauthorized, "Sender", authErr.Error())
		return
	}

	respPayload, err := s.dispatch(r.Context(), operation, payload)
	if err != nil {
		status := http.StatusInternalServerError
		code := "Receiver"
		if errors.Is(err, errUnsupportedOp) {
			status = http.StatusNotImplemented
			code = "Sender"
		}
		writeFault(w, status, code, err.Error())
		return
	}
	writeSOAP(w, respPayload)
}

func (s *Handler) dispatch(ctx context.Context, operation string, payload []byte) ([]byte, error) {
	switch operation {
	case "GetDeviceInformation":
		info, err := s.provider.DeviceInfo(ctx)
		if err != nil {
			return nil, err
		}
		return xml.Marshal(getDeviceInformationResponse{
			XMLNS:           DeviceNamespace,
			Manufacturer:    info.Manufacturer,
			Model:           info.Model,
			FirmwareVersion: info.Firmware,
			SerialNumber:    info.Serial,
			HardwareID:      info.HardwareID,
		})
	case "GetServices":
		var req struct {
			IncludeCapability bool `xml:"IncludeCapability"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, fmt.Errorf("devicesvc: decode GetServices: %w", err)
		}

		services, err := s.provider.Services(ctx, req.IncludeCapability)
		if err != nil {
			return nil, err
		}
		if len(services) == 0 {
			return nil, errNoServices
		}
		svc := services[0]
		return xml.Marshal(getServicesResponse{
			XMLNS: DeviceNamespace,
			Service: serviceEnvelope{
				Namespace: svc.Namespace,
				XAddr:     svc.XAddr,
				Version: versionEnvelope{
					Major: svc.Version.Major,
					Minor: svc.Version.Minor,
				},
			},
		})
	case "GetServiceCapabilities":
		caps, err := s.provider.GetServiceCapabilities(ctx)
		if err != nil {
			return nil, err
		}
		return xml.Marshal(getServiceCapabilitiesResponse{
			XMLNS: DeviceNamespace,
			Capabilities: deviceServiceCapabilitiesEnvelope{
				Network: networkCapabilitiesEnvelope{
					IPFilter:           caps.Network.IPFilter,
					ZeroConfiguration:  caps.Network.ZeroConfiguration,
					IPVersion6:         caps.Network.IPVersion6,
					DynDNS:             caps.Network.DynDNS,
					Dot11Configuration: caps.Network.Dot11Configuration,
					HostnameFromDHCP:   caps.Network.HostnameFromDHCP,
					NTP:                caps.Network.NTP,
				},
				Security: securityCapabilitiesEnvelope{
					UsernameToken: caps.Security.UsernameToken,
					HTTPDigest:    caps.Security.HTTPDigest,
				},
				System: systemCapabilitiesEnvelope{
					DiscoveryResolve:  caps.System.DiscoveryResolve,
					DiscoveryBye:      caps.System.DiscoveryBye,
					HTTPSystemLogging: caps.System.HTTPSystemLogging,
				},
			},
		})
	case "GetCapabilities":
		var req struct {
			Category string `xml:"Category"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, fmt.Errorf("devicesvc: decode GetCapabilities: %w", err)
		}
		caps, err := s.provider.GetCapabilities(ctx, req.Category)
		if err != nil {
			return nil, err
		}
		return xml.Marshal(getCapabilitiesResponse{
			XMLNS: DeviceNamespace,
			Capabilities: capabilitiesEnvelope{
				Device: deviceCapabilityEnvelope{
					XAddr: caps.Device.XAddr,
					Network: coreNetworkCapabilitiesEnvelope{
						IPFilter:          caps.Device.Network.IPFilter,
						ZeroConfiguration: caps.Device.Network.ZeroConfiguration,
						IPVersion6:        caps.Device.Network.IPVersion6,
						DynDNS:            caps.Device.Network.DynDNS,
					},
					System: coreSystemCapabilitiesEnvelope{
						DiscoveryResolve: caps.Device.System.DiscoveryResolve,
						DiscoveryBye:     caps.Device.System.DiscoveryBye,
						SupportedVersions: versionEnvelope{
							Major: firstVersion(caps.Device.System.SupportedVersions).Major,
							Minor: firstVersion(caps.Device.System.SupportedVersions).Minor,
						},
					},
					Security: coreSecurityCapabilitiesEnvelope{
						UsernameToken: caps.Device.Security.UsernameToken,
						HTTPDigest:    caps.Device.Security.HTTPDigest,
					},
				},
				Media:   serviceCapabilityEnvelope{XAddr: caps.Media.XAddr},
				Events:  serviceCapabilityEnvelope{XAddr: caps.Events.XAddr},
				PTZ:     serviceCapabilityEnvelope{XAddr: caps.PTZ.XAddr},
				Imaging: serviceCapabilityEnvelope{XAddr: caps.Imaging.XAddr},
			},
		})
	case "GetWsdlUrl":
		wsdlURL, err := s.provider.WsdlURL(ctx)
		if err != nil {
			return nil, err
		}
		return xml.Marshal(getWsdlURLResponse{
			XMLNS:   DeviceNamespace,
			WsdlURL: wsdlURL,
		})
	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedOp, operation)
	}
}

func readOperation(r *http.Request) (payload []byte, operation string, err error) {
	defer func() {
		if closeErr := r.Body.Close(); closeErr != nil {
			if err == nil {
				err = closeErr
				return
			}
			err = errors.Join(err, closeErr)
		}
	}()
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read request body: %w", err)
	}
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

	decoder := xml.NewDecoder(strings.NewReader(string(env.Body.Inner)))
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
		writeFault(w, http.StatusInternalServerError, "Receiver", err.Error())
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
		Body: soapBody{
			InnerXML: innerXML,
		},
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

func firstVersion(versions []Version) Version {
	if len(versions) == 0 {
		return Version{}
	}
	return versions[0]
}

type soapEnvelope struct {
	XMLName  xml.Name `xml:"env:Envelope"`
	XMLNSEnv string   `xml:"xmlns:env,attr"`
	Body     soapBody `xml:"env:Body"`
}

type soapBody struct {
	InnerXML string `xml:",innerxml"`
}

type getDeviceInformationResponse struct {
	XMLName         xml.Name `xml:"GetDeviceInformationResponse"`
	XMLNS           string   `xml:"xmlns,attr"`
	Manufacturer    string   `xml:"Manufacturer"`
	Model           string   `xml:"Model"`
	FirmwareVersion string   `xml:"FirmwareVersion"`
	SerialNumber    string   `xml:"SerialNumber"`
	HardwareID      string   `xml:"HardwareId"`
}

type getServicesResponse struct {
	XMLName xml.Name        `xml:"GetServicesResponse"`
	XMLNS   string          `xml:"xmlns,attr"`
	Service serviceEnvelope `xml:"Service"`
}

type serviceEnvelope struct {
	Namespace string          `xml:"Namespace"`
	XAddr     string          `xml:"XAddr"`
	Version   versionEnvelope `xml:"Version"`
}

type versionEnvelope struct {
	Major int `xml:"Major"`
	Minor int `xml:"Minor"`
}

type getServiceCapabilitiesResponse struct {
	XMLName      xml.Name                          `xml:"GetServiceCapabilitiesResponse"`
	XMLNS        string                            `xml:"xmlns,attr"`
	Capabilities deviceServiceCapabilitiesEnvelope `xml:"Capabilities"`
}

type deviceServiceCapabilitiesEnvelope struct {
	Network  networkCapabilitiesEnvelope  `xml:"Network"`
	Security securityCapabilitiesEnvelope `xml:"Security"`
	System   systemCapabilitiesEnvelope   `xml:"System"`
}

type networkCapabilitiesEnvelope struct {
	IPFilter           bool `xml:"IPFilter,attr,omitempty"`
	ZeroConfiguration  bool `xml:"ZeroConfiguration,attr,omitempty"`
	IPVersion6         bool `xml:"IPVersion6,attr,omitempty"`
	DynDNS             bool `xml:"DynDNS,attr,omitempty"`
	Dot11Configuration bool `xml:"Dot11Configuration,attr,omitempty"`
	HostnameFromDHCP   bool `xml:"HostnameFromDHCP,attr,omitempty"`
	NTP                int  `xml:"NTP,attr,omitempty"`
}

type securityCapabilitiesEnvelope struct {
	UsernameToken bool `xml:"UsernameToken,attr,omitempty"`
	HTTPDigest    bool `xml:"HttpDigest,attr,omitempty"`
}

type systemCapabilitiesEnvelope struct {
	DiscoveryResolve  bool `xml:"DiscoveryResolve,attr,omitempty"`
	DiscoveryBye      bool `xml:"DiscoveryBye,attr,omitempty"`
	HTTPSystemLogging bool `xml:"HttpSystemLogging,attr,omitempty"`
}

type getCapabilitiesResponse struct {
	XMLName      xml.Name             `xml:"GetCapabilitiesResponse"`
	XMLNS        string               `xml:"xmlns,attr"`
	Capabilities capabilitiesEnvelope `xml:"Capabilities"`
}

type capabilitiesEnvelope struct {
	Device  deviceCapabilityEnvelope  `xml:"Device,omitempty"`
	Media   serviceCapabilityEnvelope `xml:"Media,omitempty"`
	Events  serviceCapabilityEnvelope `xml:"Events,omitempty"`
	PTZ     serviceCapabilityEnvelope `xml:"PTZ,omitempty"`
	Imaging serviceCapabilityEnvelope `xml:"Imaging,omitempty"`
}

type deviceCapabilityEnvelope struct {
	XAddr    string                           `xml:"XAddr,omitempty"`
	Network  coreNetworkCapabilitiesEnvelope  `xml:"Network"`
	System   coreSystemCapabilitiesEnvelope   `xml:"System"`
	Security coreSecurityCapabilitiesEnvelope `xml:"Security"`
}

type coreNetworkCapabilitiesEnvelope struct {
	IPFilter          bool `xml:"IPFilter,omitempty"`
	ZeroConfiguration bool `xml:"ZeroConfiguration,omitempty"`
	IPVersion6        bool `xml:"IPVersion6,omitempty"`
	DynDNS            bool `xml:"DynDNS,omitempty"`
}

type coreSystemCapabilitiesEnvelope struct {
	DiscoveryResolve  bool            `xml:"DiscoveryResolve,omitempty"`
	DiscoveryBye      bool            `xml:"DiscoveryBye,omitempty"`
	SupportedVersions versionEnvelope `xml:"SupportedVersions"`
}

type coreSecurityCapabilitiesEnvelope struct {
	UsernameToken bool `xml:"UsernameToken,omitempty"`
	HTTPDigest    bool `xml:"HttpDigest,omitempty"`
}

type serviceCapabilityEnvelope struct {
	XAddr string `xml:"XAddr,omitempty"`
}

type getWsdlURLResponse struct {
	XMLName xml.Name `xml:"GetWsdlUrlResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
	WsdlURL string   `xml:"WsdlUrl"`
}
