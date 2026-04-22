package discovery

// SOAP 1.2 and WS-Discovery XML namespaces (WS-Discovery Table 3).
const (
	soapNS = "http://www.w3.org/2003/05/soap-envelope"
	wsaNS  = "http://schemas.xmlsoap.org/ws/2004/08/addressing"
	discNS = "http://schemas.xmlsoap.org/ws/2005/04/discovery"
)

// NSDeviceService is the ONVIF Device service WSDL namespace (QName prefix tds).
const NSDeviceService = "http://www.onvif.org/ver10/device/wsdl"

// WS-Discovery SOAP 1.2 Action URIs (excluding Discovery Proxy-only flows).
const (
	ActionHello = "http://schemas.xmlsoap.org/ws/2005/04/discovery/Hello"
	// ToDiscovery is the distinguished multicast destination for Hello/Bye/Probe/Resolve.
	ToDiscovery = "urn:schemas-xmlsoap-org:ws:2005:04:discovery"

	ActionProbe          = "http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe"
	ActionProbeMatches   = "http://schemas.xmlsoap.org/ws/2005/04/discovery/ProbeMatches"
	ActionResolve        = "http://schemas.xmlsoap.org/ws/2005/04/discovery/Resolve"
	ActionResolveMatches = "http://schemas.xmlsoap.org/ws/2005/04/discovery/ResolveMatches"
	ActionBye            = "http://schemas.xmlsoap.org/ws/2005/04/discovery/Bye"
)

// WSAAnonymous is used as a:To in Probe Match / Resolve Match when the corresponding
// request had no explicit ReplyTo (reply goes to UDP source address).
const WSAAnonymous = "http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous"

// DefaultUDPPort is the IANA-assigned WS-Discovery UDP port (WS-Discovery §2.4).
const DefaultUDPPort = 3702

// MulticastIPv4 is the IPv4 multicast group for WS-Discovery (WS-Discovery §2.4).
const MulticastIPv4 = "239.255.255.250"

// Matching rule URIs (WS-Discovery §5.1). ONVIF Core §7.3.3 requires rfc3986.
const (
	MatchByRFC2396 = "http://schemas.xmlsoap.org/ws/2005/04/discovery/rfc2396"
	MatchByRFC3986 = "http://schemas.xmlsoap.org/ws/2005/04/discovery/rfc3986"
	MatchByUUID    = "http://schemas.xmlsoap.org/ws/2005/04/discovery/uuid"
	MatchByLDAP    = "http://schemas.xmlsoap.org/ws/2005/04/discovery/ldap"
	MatchByStrcmp0 = "http://schemas.xmlsoap.org/ws/2005/04/discovery/strcmp0"
)
