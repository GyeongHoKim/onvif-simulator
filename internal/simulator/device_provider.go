package simulator

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/devicesvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/mediasvc"
)

// deviceProvider implements devicesvc.Provider against the simulator config.
type deviceProvider struct {
	sim *Simulator
}

func newDeviceProvider(s *Simulator) *deviceProvider {
	return &deviceProvider{sim: s}
}

func (p *deviceProvider) DeviceInfo(context.Context) (devicesvc.DeviceInfo, error) {
	cfg := p.sim.snapshotConfig()
	return devicesvc.DeviceInfo{
		Manufacturer: cfg.Device.Manufacturer,
		Model:        cfg.Device.Model,
		Firmware:     cfg.Device.Firmware,
		Serial:       cfg.Device.Serial,
		HardwareID:   cfg.Device.Model,
	}, nil
}

func (p *deviceProvider) Services(_ context.Context, _ bool) ([]devicesvc.ServiceDescriptor, error) {
	cfg := p.sim.snapshotConfig()
	base := p.sim.baseURL(cfg.Network.HTTPPort)
	return []devicesvc.ServiceDescriptor{
		{
			Namespace: devicesvc.DeviceNamespace,
			XAddr:     base + devicesvc.DeviceServicePath,
			Version:   devicesvc.Version{Major: 2, Minor: 40},
		},
		{
			Namespace: mediasvc.MediaNamespace,
			XAddr:     base + mediasvc.MediaServicePath,
			Version:   devicesvc.Version{Major: 2, Minor: 40},
		},
		{
			Namespace: eventsvc.EventsNamespace,
			XAddr:     base + eventsvc.EventServicePath,
			Version:   devicesvc.Version{Major: 2, Minor: 40},
		},
	}, nil
}

func (*deviceProvider) GetServiceCapabilities(context.Context) (devicesvc.DeviceServiceCapabilities, error) {
	return devicesvc.DeviceServiceCapabilities{
		Network: devicesvc.NetworkCapabilities{
			IPVersion6:       false,
			HostnameFromDHCP: false,
		},
		Security: devicesvc.SecurityCapabilities{
			UsernameToken: true,
			HTTPDigest:    true,
		},
		System: devicesvc.SystemCapabilities{
			DiscoveryResolve: true,
			DiscoveryBye:     true,
		},
	}, nil
}

func (p *deviceProvider) GetCapabilities(_ context.Context, _ string) (devicesvc.CapabilitySet, error) {
	cfg := p.sim.snapshotConfig()
	base := p.sim.baseURL(cfg.Network.HTTPPort)
	return devicesvc.CapabilitySet{
		Device: devicesvc.DeviceCapability{
			XAddr: base + devicesvc.DeviceServicePath,
			System: devicesvc.CoreSystemCapabilities{
				DiscoveryResolve:  true,
				DiscoveryBye:      true,
				SupportedVersions: []devicesvc.Version{{Major: 2, Minor: 40}},
			},
			Security: devicesvc.CoreSecurityCapabilities{
				UsernameToken: true,
				HTTPDigest:    true,
			},
		},
		Media:  devicesvc.ServiceCapability{XAddr: base + mediasvc.MediaServicePath},
		Events: devicesvc.ServiceCapability{XAddr: base + eventsvc.EventServicePath},
	}, nil
}

func (*deviceProvider) WsdlURL(context.Context) (string, error) {
	return "http://www.onvif.org/onvif/ver10/device/wsdl/devicemgmt.wsdl", nil
}

func (p *deviceProvider) GetDiscoveryMode(context.Context) (devicesvc.DiscoveryInfo, error) {
	cfg := p.sim.snapshotConfig()
	mode := cfg.Runtime.DiscoveryMode
	if mode == "" {
		mode = "Discoverable"
	}
	return devicesvc.DiscoveryInfo{DiscoveryMode: mode}, nil
}

func (p *deviceProvider) SetDiscoveryMode(_ context.Context, mode string) error {
	return p.sim.SetDiscoveryMode(mode)
}

func (p *deviceProvider) GetScopes(context.Context) ([]devicesvc.ScopeEntry, error) {
	cfg := p.sim.snapshotConfig()
	out := make([]devicesvc.ScopeEntry, 0, len(cfg.Device.Scopes))
	for _, s := range cfg.Device.Scopes {
		out = append(out, devicesvc.ScopeEntry{ScopeDef: "Configurable", ScopeItem: s})
	}
	return out, nil
}

func (*deviceProvider) SetScopes(_ context.Context, scopes []string) error {
	return config.Update(func(c *config.Config) error {
		c.Device.Scopes = append([]string(nil), scopes...)
		return nil
	})
}

func (*deviceProvider) AddScopes(_ context.Context, scopes []string) error {
	return config.Update(func(c *config.Config) error {
		seen := make(map[string]bool, len(c.Device.Scopes))
		for _, s := range c.Device.Scopes {
			seen[s] = true
		}
		for _, s := range scopes {
			if !seen[s] {
				c.Device.Scopes = append(c.Device.Scopes, s)
				seen[s] = true
			}
		}
		return nil
	})
}

func (*deviceProvider) RemoveScopes(_ context.Context, scopes []string) ([]string, error) {
	removed := make([]string, 0, len(scopes))
	targets := make(map[string]bool, len(scopes))
	for _, s := range scopes {
		targets[s] = true
	}
	err := config.Update(func(c *config.Config) error {
		kept := c.Device.Scopes[:0]
		for _, s := range c.Device.Scopes {
			if targets[s] {
				removed = append(removed, s)
				continue
			}
			kept = append(kept, s)
		}
		c.Device.Scopes = kept
		return nil
	})
	if err != nil {
		return nil, err
	}
	return removed, nil
}

func (p *deviceProvider) GetHostname(context.Context) (devicesvc.HostnameInfo, error) {
	cfg := p.sim.snapshotConfig()
	return devicesvc.HostnameInfo{Name: cfg.Runtime.Hostname}, nil
}

func (p *deviceProvider) SetHostname(_ context.Context, name string) error {
	return p.sim.SetHostname(name)
}

func (p *deviceProvider) GetDNS(context.Context) (devicesvc.DNSInfo, error) {
	cfg := p.sim.snapshotConfig()
	return devicesvc.DNSInfo{
		FromDHCP:     cfg.Runtime.DNS.FromDHCP,
		SearchDomain: append([]string(nil), cfg.Runtime.DNS.SearchDomain...),
		DNSManual:    append([]string(nil), cfg.Runtime.DNS.DNSManual...),
	}, nil
}

func (*deviceProvider) SetDNS(_ context.Context, info devicesvc.DNSInfo) error {
	return config.SetDNS(config.DNSConfig{
		FromDHCP:     info.FromDHCP,
		SearchDomain: append([]string(nil), info.SearchDomain...),
		DNSManual:    append([]string(nil), info.DNSManual...),
	})
}

func (p *deviceProvider) GetNetworkInterfaces(context.Context) ([]devicesvc.NetworkInterfaceInfo, error) {
	cfg := p.sim.snapshotConfig()
	out := make([]devicesvc.NetworkInterfaceInfo, 0, len(cfg.Runtime.NetworkInterfaces))
	for _, n := range cfg.Runtime.NetworkInterfaces {
		iface := devicesvc.NetworkInterfaceInfo{
			Token:     n.Token,
			Enabled:   n.Enabled,
			HwAddress: n.HwAddress,
			MTU:       n.MTU,
		}
		if n.IPv4 != nil {
			iface.IPv4 = &devicesvc.IPv4Config{
				Enabled: n.IPv4.Enabled,
				DHCP:    n.IPv4.DHCP,
				Manual:  append([]string(nil), n.IPv4.Manual...),
			}
		}
		out = append(out, iface)
	}
	return out, nil
}

func (*deviceProvider) SetNetworkInterfaces(_ context.Context, ifaces []devicesvc.NetworkInterfaceInfo) error {
	cfgIfaces := make([]config.NetworkInterfaceConfig, 0, len(ifaces))
	for _, n := range ifaces {
		c := config.NetworkInterfaceConfig{
			Token:     n.Token,
			Enabled:   n.Enabled,
			HwAddress: n.HwAddress,
			MTU:       n.MTU,
		}
		if n.IPv4 != nil {
			c.IPv4 = &config.NetworkInterfaceIPv4{
				Enabled: n.IPv4.Enabled,
				DHCP:    n.IPv4.DHCP,
				Manual:  append([]string(nil), n.IPv4.Manual...),
			}
		}
		cfgIfaces = append(cfgIfaces, c)
	}
	return config.SetNetworkInterfaces(cfgIfaces)
}

func (p *deviceProvider) GetNetworkProtocols(context.Context) ([]devicesvc.NetworkProtocol, error) {
	cfg := p.sim.snapshotConfig()
	out := make([]devicesvc.NetworkProtocol, 0, len(cfg.Runtime.NetworkProtocols))
	for _, p := range cfg.Runtime.NetworkProtocols {
		out = append(out, devicesvc.NetworkProtocol{
			Name:    p.Name,
			Enabled: p.Enabled,
			Port:    append([]int(nil), p.Port...),
		})
	}
	return out, nil
}

func (*deviceProvider) SetNetworkProtocols(_ context.Context, protocols []devicesvc.NetworkProtocol) error {
	out := make([]config.NetworkProtocol, 0, len(protocols))
	for _, p := range protocols {
		out = append(out, config.NetworkProtocol{
			Name:    p.Name,
			Enabled: p.Enabled,
			Port:    append([]int(nil), p.Port...),
		})
	}
	return config.SetNetworkProtocols(out)
}

func (p *deviceProvider) GetNetworkDefaultGateway(context.Context) (devicesvc.DefaultGatewayInfo, error) {
	cfg := p.sim.snapshotConfig()
	return devicesvc.DefaultGatewayInfo{
		IPv4Address: append([]string(nil), cfg.Runtime.DefaultGateway.IPv4Address...),
		IPv6Address: append([]string(nil), cfg.Runtime.DefaultGateway.IPv6Address...),
	}, nil
}

func (*deviceProvider) SetNetworkDefaultGateway(_ context.Context, info devicesvc.DefaultGatewayInfo) error {
	return config.SetDefaultGateway(config.DefaultGatewayConfig{
		IPv4Address: append([]string(nil), info.IPv4Address...),
		IPv6Address: append([]string(nil), info.IPv6Address...),
	})
}

func (p *deviceProvider) GetSystemDateAndTime(context.Context) (devicesvc.SystemDateAndTimeInfo, error) {
	cfg := p.sim.snapshotConfig()
	info := devicesvc.SystemDateAndTimeInfo{
		DateTimeType:    cfg.Runtime.SystemDateAndTime.DateTimeType,
		DaylightSavings: cfg.Runtime.SystemDateAndTime.DaylightSavings,
		TZ:              cfg.Runtime.SystemDateAndTime.TZ,
	}
	if info.DateTimeType == "" {
		info.DateTimeType = "NTP"
	}
	t := time.Now().UTC()
	if cfg.Runtime.SystemDateAndTime.ManualDateTimeUTC != "" && info.DateTimeType == "Manual" {
		if parsed, err := time.Parse(time.RFC3339, cfg.Runtime.SystemDateAndTime.ManualDateTimeUTC); err == nil {
			t = parsed
		}
	}
	info.UTCDateTime = devicesvc.SystemDateTime{
		Year: t.Year(), Month: int(t.Month()), Day: t.Day(),
		Hour: t.Hour(), Minute: t.Minute(), Second: t.Second(),
	}
	return info, nil
}

//nolint:gocritic // SetSystemDateAndTimeParams is value-typed by interface contract.
func (*deviceProvider) SetSystemDateAndTime(_ context.Context, params devicesvc.SetSystemDateAndTimeParams) error {
	cfg := config.SystemDateTimeConfig{
		DateTimeType:    params.DateTimeType,
		DaylightSavings: params.DaylightSavings,
		TZ:              params.TZ,
	}
	dt := params.UTCDateTime
	if dt.Year != 0 {
		cfg.ManualDateTimeUTC = time.Date(
			dt.Year, time.Month(dt.Month), dt.Day,
			dt.Hour, dt.Minute, dt.Second, 0, time.UTC,
		).UTC().Format(time.RFC3339)
	}
	return config.SetSystemDateAndTime(cfg)
}

func (*deviceProvider) SetSystemFactoryDefault(context.Context, string) error {
	return nil
}

func (*deviceProvider) SystemReboot(context.Context) (string, error) {
	return "reboot scheduled (simulated)", nil
}

func (p *deviceProvider) GetUsers(context.Context) ([]devicesvc.UserInfo, error) {
	cfg := p.sim.snapshotConfig()
	out := make([]devicesvc.UserInfo, 0, len(cfg.Auth.Users))
	for _, u := range cfg.Auth.Users {
		out = append(out, devicesvc.UserInfo{Username: u.Username, UserLevel: u.Role})
	}
	return out, nil
}

func (p *deviceProvider) CreateUsers(_ context.Context, users []devicesvc.UserInfo) error {
	for _, u := range users {
		role := normaliseUserLevel(u.UserLevel)
		if err := p.sim.AddUser(config.UserConfig{
			Username: u.Username, Password: u.Password, Role: role,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (p *deviceProvider) SetUser(_ context.Context, users []devicesvc.UserInfo) error {
	for _, u := range users {
		role := normaliseUserLevel(u.UserLevel)
		if err := p.sim.UpsertUser(config.UserConfig{
			Username: u.Username, Password: u.Password, Role: role,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (p *deviceProvider) DeleteUsers(_ context.Context, usernames []string) error {
	for _, u := range usernames {
		if err := p.sim.RemoveUser(u); err != nil {
			return err
		}
	}
	return nil
}

func normaliseUserLevel(level string) string {
	trimmed := strings.TrimSpace(level)
	if trimmed == "" {
		return config.RoleUser
	}
	return trimmed
}

// baseURL returns a base URL for composing ONVIF XAddr values, e.g.
// "http://192.168.1.10:8080". Port comes from config; host is derived from
// the first non-loopback interface address.
func (*Simulator) baseURL(configPort int) string {
	host := localAddrForXAddr()
	return "http://" + net.JoinHostPort(host, strconv.Itoa(configPort))
}
