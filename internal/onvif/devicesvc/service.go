package devicesvc

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
	// memory use during io.ReadAll.
	maxSOAPBodySize = 10 << 20

	faultCodeSender   = "Sender"
	faultCodeReceiver = "Receiver"
)

var (
	errProviderRequired = errors.New("devicesvc: provider is required")
	errUnsupportedOp    = errors.New("devicesvc: unsupported operation")
	errNoServices       = errors.New("devicesvc: no services available")
	errEmptySOAPBody    = errors.New("devicesvc: empty soap body")
	errDecodePayload    = errors.New("devicesvc: malformed request payload")
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

	r.Body = http.MaxBytesReader(w, r.Body, maxSOAPBodySize)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeFault(w, http.StatusRequestEntityTooLarge, "Sender", tooLarge.Error())
			return
		}
		writeFault(w, http.StatusBadRequest, "Sender", fmt.Errorf("read request body: %w", err).Error())
		return
	}
	if closeErr := r.Body.Close(); closeErr != nil {
		writeFault(w, http.StatusBadRequest, "Sender", closeErr.Error())
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))

	payload, operation, err := parseOperation(raw)
	if err != nil {
		writeFault(w, http.StatusBadRequest, "Sender", err.Error())
		return
	}

	if authErr := s.auth.Authorize(r.Context(), operation, r); authErr != nil {
		s.writeAuthFault(w, authErr)
		return
	}

	respPayload, err := s.dispatch(r.Context(), operation, payload)
	if err != nil {
		status := http.StatusInternalServerError
		code := faultCodeReceiver
		switch {
		case errors.Is(err, errUnsupportedOp):
			status = http.StatusNotImplemented
			code = faultCodeSender
		case errors.Is(err, errDecodePayload):
			status = http.StatusBadRequest
			code = faultCodeSender
		}
		writeFault(w, status, code, err.Error())
		return
	}
	writeSOAP(w, respPayload)
}

// writeAuthFault translates an auth.Authorize error into a SOAP fault.
// Forbidden errors map to HTTP 403; other auth errors (including challenges
// returned as *auth.ChallengeError) map to 401 and copy the challenge's
// headers (e.g. WWW-Authenticate) onto the response.
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
		// The caller authenticated but their role is insufficient.
		status = http.StatusForbidden
	}
	writeFault(w, status, faultCodeSender, authErr.Error())
}

// dispatch routes ONVIF operations to their handlers.
//
//nolint:gocyclo,cyclop,funlen // flat switch by design; splitting adds indirection
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
		return s.handleGetServices(ctx, payload)
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
					JSONWebToken:  caps.Security.JSONWebToken,
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
			return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode GetCapabilities: %w", err))
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
						DiscoveryResolve:  caps.Device.System.DiscoveryResolve,
						DiscoveryBye:      caps.Device.System.DiscoveryBye,
						SupportedVersions: supportedVersionEnvelopes(caps.Device.System.SupportedVersions),
					},
					Security: coreSecurityCapabilitiesEnvelope{
						UsernameToken: caps.Device.Security.UsernameToken,
						HTTPDigest:    caps.Device.Security.HTTPDigest,
						JSONWebToken:  caps.Device.Security.JSONWebToken,
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

	// --- Discovery (§7.3) ---

	case "GetDiscoveryMode":
		info, err := s.provider.GetDiscoveryMode(ctx)
		if err != nil {
			return nil, err
		}
		return xml.Marshal(getDiscoveryModeResponse{XMLNS: DeviceNamespace, DiscoveryMode: info.DiscoveryMode})

	case "SetDiscoveryMode":
		var req struct {
			DiscoveryMode string `xml:"DiscoveryMode"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode SetDiscoveryMode: %w", err))
		}
		if err := s.provider.SetDiscoveryMode(ctx, req.DiscoveryMode); err != nil {
			return nil, err
		}
		return xml.Marshal(emptyResponse{XMLName: xml.Name{Local: "SetDiscoveryModeResponse"}, XMLNS: DeviceNamespace})

	case "GetScopes":
		scopes, err := s.provider.GetScopes(ctx)
		if err != nil {
			return nil, err
		}
		entries := make([]scopeEntryEnvelope, len(scopes))
		for i, scope := range scopes {
			entries[i] = scopeEntryEnvelope(scope)
		}
		return xml.Marshal(getScopesResponse{XMLNS: DeviceNamespace, Scopes: entries})

	case "SetScopes":
		var req struct {
			Scopes []string `xml:"Scopes"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode SetScopes: %w", err))
		}
		if err := s.provider.SetScopes(ctx, req.Scopes); err != nil {
			return nil, err
		}
		return xml.Marshal(emptyResponse{XMLName: xml.Name{Local: "SetScopesResponse"}, XMLNS: DeviceNamespace})

	case "AddScopes":
		var req struct {
			ScopeItem []string `xml:"ScopeItem"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode AddScopes: %w", err))
		}
		if err := s.provider.AddScopes(ctx, req.ScopeItem); err != nil {
			return nil, err
		}
		return xml.Marshal(emptyResponse{XMLName: xml.Name{Local: "AddScopesResponse"}, XMLNS: DeviceNamespace})

	case "RemoveScopes":
		var req struct {
			ScopeItem []string `xml:"ScopeItem"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode RemoveScopes: %w", err))
		}
		removed, err := s.provider.RemoveScopes(ctx, req.ScopeItem)
		if err != nil {
			return nil, err
		}
		return xml.Marshal(removeScopesResponse{XMLNS: DeviceNamespace, RemovedScopes: removed})

	// --- Network configuration (§7.4) ---

	case "GetHostname":
		info, err := s.provider.GetHostname(ctx)
		if err != nil {
			return nil, err
		}
		return xml.Marshal(getHostnameResponse{
			XMLNS:               DeviceNamespace,
			HostnameInformation: hostnameInfoEnvelope(info),
		})

	case "SetHostname":
		var req struct {
			Name string `xml:"Name"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode SetHostname: %w", err))
		}
		if err := s.provider.SetHostname(ctx, req.Name); err != nil {
			return nil, err
		}
		return xml.Marshal(emptyResponse{XMLName: xml.Name{Local: "SetHostnameResponse"}, XMLNS: DeviceNamespace})

	case "GetDNS":
		info, err := s.provider.GetDNS(ctx)
		if err != nil {
			return nil, err
		}
		manualAddrs := make([]ipAddressEnvelope, len(info.DNSManual))
		for i, a := range info.DNSManual {
			if isIPv6Addr(a) {
				manualAddrs[i] = ipAddressEnvelope{IPv6Address: a}
			} else {
				manualAddrs[i] = ipAddressEnvelope{IPv4Address: a}
			}
		}
		return xml.Marshal(getDNSResponse{
			XMLNS: DeviceNamespace,
			DNSInformation: dnsInfoEnvelope{
				FromDHCP:     info.FromDHCP,
				SearchDomain: info.SearchDomain,
				DNSManual:    manualAddrs,
			},
		})

	case "SetDNS":
		var req struct {
			FromDHCP     bool     `xml:"FromDHCP"`
			SearchDomain []string `xml:"SearchDomain"`
			DNSManual    []struct {
				IPv4Address string `xml:"IPv4Address"`
				IPv6Address string `xml:"IPv6Address"`
			} `xml:"DNSManual"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode SetDNS: %w", err))
		}
		manual := make([]string, 0, len(req.DNSManual))
		for _, m := range req.DNSManual {
			if m.IPv6Address != "" {
				manual = append(manual, m.IPv6Address)
			} else if m.IPv4Address != "" {
				manual = append(manual, m.IPv4Address)
			}
		}
		if err := s.provider.SetDNS(ctx, DNSInfo{
			FromDHCP:     req.FromDHCP,
			SearchDomain: req.SearchDomain,
			DNSManual:    manual,
		}); err != nil {
			return nil, err
		}
		return xml.Marshal(emptyResponse{XMLName: xml.Name{Local: "SetDNSResponse"}, XMLNS: DeviceNamespace})

	case "GetNetworkInterfaces":
		ifaces, err := s.provider.GetNetworkInterfaces(ctx)
		if err != nil {
			return nil, err
		}
		envs := make([]networkInterfaceEnvelope, len(ifaces))
		for i, iface := range ifaces {
			env := networkInterfaceEnvelope{
				Token:     iface.Token,
				Enabled:   iface.Enabled,
				HwAddress: iface.HwAddress,
				MTU:       iface.MTU,
			}
			if iface.IPv4 != nil {
				manual := make([]prefixedAddressEnvelope, len(iface.IPv4.Manual))
				for j, m := range iface.IPv4.Manual {
					manual[j] = prefixedAddressEnvelope{Address: m}
				}
				env.IPv4 = &ipv4ConfigEnvelope{
					Enabled: iface.IPv4.Enabled,
					Config: ipv4NetworkConfigEnvelope{
						Manual: manual,
						DHCP:   iface.IPv4.DHCP,
					},
				}
			}
			envs[i] = env
		}
		return xml.Marshal(getNetworkInterfacesResponse{XMLNS: DeviceNamespace, NetworkInterfaces: envs})

	case "GetNetworkProtocols":
		protocols, err := s.provider.GetNetworkProtocols(ctx)
		if err != nil {
			return nil, err
		}
		envs := make([]networkProtocolEnvelope, len(protocols))
		for i, p := range protocols {
			envs[i] = networkProtocolEnvelope(p)
		}
		return xml.Marshal(getNetworkProtocolsResponse{XMLNS: DeviceNamespace, NetworkProtocols: envs})

	case "SetNetworkProtocols":
		var req struct {
			NetworkProtocols []networkProtocolEnvelope `xml:"NetworkProtocols"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode SetNetworkProtocols: %w", err))
		}
		protocols := make([]NetworkProtocol, len(req.NetworkProtocols))
		for i, p := range req.NetworkProtocols {
			protocols[i] = NetworkProtocol(p)
		}
		if err := s.provider.SetNetworkProtocols(ctx, protocols); err != nil {
			return nil, err
		}
		return xml.Marshal(emptyResponse{XMLName: xml.Name{Local: "SetNetworkProtocolsResponse"}, XMLNS: DeviceNamespace})

	case "GetNetworkDefaultGateway":
		gw, err := s.provider.GetNetworkDefaultGateway(ctx)
		if err != nil {
			return nil, err
		}
		return xml.Marshal(getNetworkDefaultGatewayResponse{
			XMLNS:          DeviceNamespace,
			NetworkGateway: networkGatewayEnvelope(gw),
		})

	case "SetNetworkDefaultGateway":
		var req struct {
			IPv4Address []string `xml:"IPv4Address"`
			IPv6Address []string `xml:"IPv6Address"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode SetNetworkDefaultGateway: %w", err))
		}
		if err := s.provider.SetNetworkDefaultGateway(ctx, DefaultGatewayInfo{
			IPv4Address: req.IPv4Address,
			IPv6Address: req.IPv6Address,
		}); err != nil {
			return nil, err
		}
		return xml.Marshal(emptyResponse{
			XMLName: xml.Name{Local: "SetNetworkDefaultGatewayResponse"},
			XMLNS:   DeviceNamespace,
		})

	// --- System (§7.5) ---

	case "GetSystemDateAndTime":
		info, err := s.provider.GetSystemDateAndTime(ctx)
		if err != nil {
			return nil, err
		}
		var tz *timeZoneEnvelope
		if info.TZ != "" {
			tz = &timeZoneEnvelope{TZ: info.TZ}
		}
		var utcDT *dateTimeEnvelope
		dt := info.UTCDateTime
		if dt.Year != 0 || dt.Month != 0 || dt.Day != 0 || dt.Hour != 0 || dt.Minute != 0 || dt.Second != 0 {
			utcDT = &dateTimeEnvelope{
				Date: dateEnvelope{Year: dt.Year, Month: dt.Month, Day: dt.Day},
				Time: timeEnvelope{Hour: dt.Hour, Minute: dt.Minute, Second: dt.Second},
			}
		}
		return xml.Marshal(getSystemDateAndTimeResponse{
			XMLNS: DeviceNamespace,
			SystemDateAndTime: systemDateAndTimeEnvelope{
				DateTimeType:    info.DateTimeType,
				DaylightSavings: info.DaylightSavings,
				TimeZone:        tz,
				UTCDateTime:     utcDT,
			},
		})

	case "SetSystemDateAndTime":
		var req struct {
			DateTimeType    string `xml:"DateTimeType"`
			DaylightSavings bool   `xml:"DaylightSavings"`
			TimeZone        struct {
				TZ string `xml:"TZ"`
			} `xml:"TimeZone"`
			UTCDateTime struct {
				Date struct {
					Year  int `xml:"Year"`
					Month int `xml:"Month"`
					Day   int `xml:"Day"`
				} `xml:"Date"`
				Time struct {
					Hour   int `xml:"Hour"`
					Minute int `xml:"Minute"`
					Second int `xml:"Second"`
				} `xml:"Time"`
			} `xml:"UTCDateTime"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode SetSystemDateAndTime: %w", err))
		}
		if err := s.provider.SetSystemDateAndTime(ctx, SetSystemDateAndTimeParams{
			DateTimeType:    req.DateTimeType,
			DaylightSavings: req.DaylightSavings,
			TZ:              req.TimeZone.TZ,
			UTCDateTime: SystemDateTime{
				Year: req.UTCDateTime.Date.Year, Month: req.UTCDateTime.Date.Month, Day: req.UTCDateTime.Date.Day,
				Hour: req.UTCDateTime.Time.Hour, Minute: req.UTCDateTime.Time.Minute, Second: req.UTCDateTime.Time.Second,
			},
		}); err != nil {
			return nil, err
		}
		return xml.Marshal(emptyResponse{XMLName: xml.Name{Local: "SetSystemDateAndTimeResponse"}, XMLNS: DeviceNamespace})

	case "SetSystemFactoryDefault":
		var req struct {
			FactoryDefault string `xml:"FactoryDefault"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode SetSystemFactoryDefault: %w", err))
		}
		if err := s.provider.SetSystemFactoryDefault(ctx, req.FactoryDefault); err != nil {
			return nil, err
		}
		return xml.Marshal(emptyResponse{XMLName: xml.Name{Local: "SetSystemFactoryDefaultResponse"}, XMLNS: DeviceNamespace})

	case "SystemReboot":
		message, err := s.provider.SystemReboot(ctx)
		if err != nil {
			return nil, err
		}
		return xml.Marshal(systemRebootResponse{XMLNS: DeviceNamespace, Message: message})

	// --- User handling (§7.6) ---

	case "GetUsers":
		users, err := s.provider.GetUsers(ctx)
		if err != nil {
			return nil, err
		}
		envs := make([]userEnvelope, len(users))
		for i, u := range users {
			envs[i] = userEnvelope{Username: u.Username, UserLevel: u.UserLevel}
		}
		return xml.Marshal(getUsersResponse{XMLNS: DeviceNamespace, Users: envs})

	case "CreateUsers":
		var req struct {
			Users []userEnvelope `xml:"User"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode CreateUsers: %w", err))
		}
		users := envelopesToUsers(req.Users)
		if err := s.provider.CreateUsers(ctx, users); err != nil {
			return nil, err
		}
		return xml.Marshal(emptyResponse{XMLName: xml.Name{Local: "CreateUsersResponse"}, XMLNS: DeviceNamespace})

	case "SetUser":
		var req struct {
			Users []userEnvelope `xml:"User"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode SetUser: %w", err))
		}
		users := envelopesToUsers(req.Users)
		if err := s.provider.SetUser(ctx, users); err != nil {
			return nil, err
		}
		return xml.Marshal(emptyResponse{XMLName: xml.Name{Local: "SetUserResponse"}, XMLNS: DeviceNamespace})

	case "DeleteUsers":
		var req struct {
			Usernames []string `xml:"Username"`
		}
		if err := xml.Unmarshal(payload, &req); err != nil {
			return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode DeleteUsers: %w", err))
		}
		if err := s.provider.DeleteUsers(ctx, req.Usernames); err != nil {
			return nil, err
		}
		return xml.Marshal(emptyResponse{XMLName: xml.Name{Local: "DeleteUsersResponse"}, XMLNS: DeviceNamespace})

	default:
		return nil, fmt.Errorf("%w: %s", errUnsupportedOp, operation)
	}
}

func (s *Handler) handleGetServices(ctx context.Context, payload []byte) ([]byte, error) {
	var req struct {
		IncludeCapability bool `xml:"IncludeCapability"`
	}
	if err := xml.Unmarshal(payload, &req); err != nil {
		return nil, errors.Join(errDecodePayload, fmt.Errorf("devicesvc: decode GetServices: %w", err))
	}

	services, err := s.provider.Services(ctx, req.IncludeCapability)
	if err != nil {
		return nil, err
	}
	if len(services) == 0 {
		return nil, errNoServices
	}
	envelopes := make([]serviceEnvelope, len(services))
	for i, svc := range services {
		envelopes[i] = serviceEnvelope{
			Namespace:    svc.Namespace,
			XAddr:        svc.XAddr,
			Version:      versionEnvelope(svc.Version),
			Capabilities: svc.Capability,
		}
	}
	return xml.Marshal(getServicesResponse{
		XMLNS:    DeviceNamespace,
		Services: envelopes,
	})
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

func supportedVersionEnvelopes(versions []Version) []versionEnvelope {
	envelopes := make([]versionEnvelope, len(versions))
	for i, v := range versions {
		envelopes[i] = versionEnvelope(v)
	}
	return envelopes
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
	XMLName  xml.Name          `xml:"GetServicesResponse"`
	XMLNS    string            `xml:"xmlns,attr"`
	Services []serviceEnvelope `xml:"Service"`
}

type serviceEnvelope struct {
	Namespace    string          `xml:"Namespace"`
	XAddr        string          `xml:"XAddr"`
	Version      versionEnvelope `xml:"Version"`
	Capabilities string          `xml:"Capabilities,omitempty"`
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
	JSONWebToken  bool `xml:"JsonWebToken,attr,omitempty"`
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
	DiscoveryResolve  bool              `xml:"DiscoveryResolve,omitempty"`
	DiscoveryBye      bool              `xml:"DiscoveryBye,omitempty"`
	SupportedVersions []versionEnvelope `xml:"SupportedVersions"`
}

type coreSecurityCapabilitiesEnvelope struct {
	UsernameToken bool `xml:"UsernameToken,omitempty"`
	HTTPDigest    bool `xml:"HttpDigest,omitempty"`
	JSONWebToken  bool `xml:"JsonWebToken,omitempty"`
}

type serviceCapabilityEnvelope struct {
	XAddr string `xml:"XAddr,omitempty"`
}

type getWsdlURLResponse struct {
	XMLName xml.Name `xml:"GetWsdlUrlResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
	WsdlURL string   `xml:"WsdlUrl"`
}

// emptyResponse is a reusable SOAP response with no payload fields.
type emptyResponse struct {
	XMLName xml.Name `xml:""`
	XMLNS   string   `xml:"xmlns,attr"`
}

// ---------- Discovery envelopes -------------------------------------------------

type getDiscoveryModeResponse struct {
	XMLName       xml.Name `xml:"GetDiscoveryModeResponse"`
	XMLNS         string   `xml:"xmlns,attr"`
	DiscoveryMode string   `xml:"DiscoveryMode"`
}

type scopeEntryEnvelope struct {
	ScopeDef  string `xml:"ScopeDef"`
	ScopeItem string `xml:"ScopeItem"`
}

type getScopesResponse struct {
	XMLName xml.Name             `xml:"GetScopesResponse"`
	XMLNS   string               `xml:"xmlns,attr"`
	Scopes  []scopeEntryEnvelope `xml:"Scopes"`
}

type removeScopesResponse struct {
	XMLName       xml.Name `xml:"RemoveScopesResponse"`
	XMLNS         string   `xml:"xmlns,attr"`
	RemovedScopes []string `xml:"RemovedScopes,omitempty"`
}

// ---------- Network envelopes ---------------------------------------------------

type hostnameInfoEnvelope struct {
	FromDHCP bool   `xml:"FromDHCP"`
	Name     string `xml:"Name,omitempty"`
}

type getHostnameResponse struct {
	XMLName             xml.Name             `xml:"GetHostnameResponse"`
	XMLNS               string               `xml:"xmlns,attr"`
	HostnameInformation hostnameInfoEnvelope `xml:"HostnameInformation"`
}

type ipAddressEnvelope struct {
	IPv4Address string `xml:"IPv4Address,omitempty"`
	IPv6Address string `xml:"IPv6Address,omitempty"`
}

type dnsInfoEnvelope struct {
	FromDHCP     bool                `xml:"FromDHCP"`
	SearchDomain []string            `xml:"SearchDomain,omitempty"`
	DNSManual    []ipAddressEnvelope `xml:"DNSManual,omitempty"`
}

type getDNSResponse struct {
	XMLName        xml.Name        `xml:"GetDNSResponse"`
	XMLNS          string          `xml:"xmlns,attr"`
	DNSInformation dnsInfoEnvelope `xml:"DNSInformation"`
}

type prefixedAddressEnvelope struct {
	Address string `xml:"Address,omitempty"`
}

type ipv4NetworkConfigEnvelope struct {
	Manual []prefixedAddressEnvelope `xml:"Manual,omitempty"`
	DHCP   bool                      `xml:"DHCP"`
}

type ipv4ConfigEnvelope struct {
	Enabled bool                      `xml:"Enabled"`
	Config  ipv4NetworkConfigEnvelope `xml:"Config"`
}

type networkInterfaceEnvelope struct {
	Token     string              `xml:"token,attr"`
	Enabled   bool                `xml:"Enabled"`
	HwAddress string              `xml:"Info>HwAddress,omitempty"`
	MTU       int                 `xml:"Info>MTU,omitempty"`
	IPv4      *ipv4ConfigEnvelope `xml:"IPv4,omitempty"`
}

type getNetworkInterfacesResponse struct {
	XMLName           xml.Name                   `xml:"GetNetworkInterfacesResponse"`
	XMLNS             string                     `xml:"xmlns,attr"`
	NetworkInterfaces []networkInterfaceEnvelope `xml:"NetworkInterfaces"`
}

type networkProtocolEnvelope struct {
	Name    string `xml:"Name"`
	Enabled bool   `xml:"Enabled"`
	Port    []int  `xml:"Port,omitempty"`
}

type getNetworkProtocolsResponse struct {
	XMLName          xml.Name                  `xml:"GetNetworkProtocolsResponse"`
	XMLNS            string                    `xml:"xmlns,attr"`
	NetworkProtocols []networkProtocolEnvelope `xml:"NetworkProtocols"`
}

type networkGatewayEnvelope struct {
	IPv4Address []string `xml:"IPv4Address,omitempty"`
	IPv6Address []string `xml:"IPv6Address,omitempty"`
}

type getNetworkDefaultGatewayResponse struct {
	XMLName        xml.Name               `xml:"GetNetworkDefaultGatewayResponse"`
	XMLNS          string                 `xml:"xmlns,attr"`
	NetworkGateway networkGatewayEnvelope `xml:"NetworkGateway"`
}

// ---------- System envelopes ----------------------------------------------------

type timeZoneEnvelope struct {
	TZ string `xml:"TZ"`
}

type dateEnvelope struct {
	Year  int `xml:"Year"`
	Month int `xml:"Month"`
	Day   int `xml:"Day"`
}

type timeEnvelope struct {
	Hour   int `xml:"Hour"`
	Minute int `xml:"Minute"`
	Second int `xml:"Second"`
}

type dateTimeEnvelope struct {
	Date dateEnvelope `xml:"Date"`
	Time timeEnvelope `xml:"Time"`
}

type systemDateAndTimeEnvelope struct {
	DateTimeType    string            `xml:"DateTimeType"`
	DaylightSavings bool              `xml:"DaylightSavings"`
	TimeZone        *timeZoneEnvelope `xml:"TimeZone,omitempty"`
	UTCDateTime     *dateTimeEnvelope `xml:"UTCDateTime,omitempty"`
}

type getSystemDateAndTimeResponse struct {
	XMLName           xml.Name                  `xml:"GetSystemDateAndTimeResponse"`
	XMLNS             string                    `xml:"xmlns,attr"`
	SystemDateAndTime systemDateAndTimeEnvelope `xml:"SystemDateAndTime"`
}

type systemRebootResponse struct {
	XMLName xml.Name `xml:"SystemRebootResponse"`
	XMLNS   string   `xml:"xmlns,attr"`
	Message string   `xml:"Message"`
}

// ---------- User envelopes ------------------------------------------------------

type userEnvelope struct {
	Username  string `xml:"Username"`
	Password  string `xml:"Password,omitempty"`
	UserLevel string `xml:"UserLevel"`
}

type getUsersResponse struct {
	XMLName xml.Name       `xml:"GetUsersResponse"`
	XMLNS   string         `xml:"xmlns,attr"`
	Users   []userEnvelope `xml:"User"`
}

func envelopesToUsers(envs []userEnvelope) []UserInfo {
	users := make([]UserInfo, len(envs))
	for i, e := range envs {
		users[i] = UserInfo(e)
	}
	return users
}

// isIPv6Addr reports whether addr is an IPv6 address (contains a colon).
func isIPv6Addr(addr string) bool {
	return strings.Contains(addr, ":")
}
