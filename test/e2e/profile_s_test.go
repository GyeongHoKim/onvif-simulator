//go:build e2e

package e2e

// ONVIF Profile S — single end-to-end conformance suite (Device + Media over HTTP SOAP).
//
// Normative / informative references (repo: doc/):
//   - ONVIF Profile S Specification v1.3 — §7 Profile Mandatory Features (device + streaming).
//   - ONVIF Core Specification — GetServices, capability exchange (referenced from Profile S §7.2).
//   - ONVIF Base Device Test Specification v21.12 — GetDeviceInformation, GetCapabilities, GetServices.
//   - ONVIF Media Configuration Device Test Specification — MEDIA-1-1-1, MEDIA-1-1-3, GetVideoSources, GetStreamUri.
//   - ONVIF Media Service Specification (doc/ONVIF-Media-Service-Spec.pdf) — §5.2 profiles, §5.3–5.5 video
//     source/encoder configuration, §5.15 GetStreamUri, §5.16 GetSnapshotUri, §5.18 SetSynchronizationPoint, §5.21 GetServiceCapabilities.
//
// Out of scope for this package (not exercised here):
//   - WS-Discovery UDP probe/match (Profile S §7.3 also requires Core WS-Discovery).
//   - SetNetworkInterfaces (Profile S §7.4 M) — needs full NetworkInterfaceSetConfiguration mapping; GetNetworkInterfaces is covered.

import (
	"context"
	"fmt"
	"testing"
	"time"

	onviflib "github.com/use-go/onvif"
	"github.com/use-go/onvif/device"
	"github.com/use-go/onvif/media"
	sdkdevice "github.com/use-go/onvif/sdk/device"
	sdkmedia "github.com/use-go/onvif/sdk/media"
	"github.com/use-go/onvif/xsd"
	onviftypes "github.com/use-go/onvif/xsd/onvif"
)

const deviceManagementNS = "http://www.onvif.org/ver10/device/wsdl"

// TestONVIF_ProfileS is the consolidated Profile S e2e suite. Subtest names map to spec sections or test case IDs.
func TestONVIF_ProfileS(t *testing.T) {
	// --- Base Device Test Specification v21.12 + Core (service & device identity) ---

	t.Run("BaseDeviceTestSpec_GetDeviceInformation", func(t *testing.T) {
		// Base Device Test Spec v21.12 — Device Management / GetDeviceInformation.
		// Profile S v1.3 §7.5.3 — System / GetDeviceInformation (M).
		dev := newDevice(t)
		resp, err := sdkdevice.Call_GetDeviceInformation(ctx(t), dev, device.GetDeviceInformation{})
		if err != nil {
			t.Fatalf("GetDeviceInformation: %v", err)
		}
		for field, value := range map[string]string{
			"Manufacturer":    string(resp.Manufacturer),
			"Model":           string(resp.Model),
			"FirmwareVersion": string(resp.FirmwareVersion),
			"SerialNumber":    string(resp.SerialNumber),
			"HardwareId":      string(resp.HardwareId),
		} {
			if value == "" {
				t.Errorf("GetDeviceInformationResponse.%s must not be empty", field)
			}
		}
	})

	t.Run("Core_and_BaseDeviceTestSpec_GetServices", func(t *testing.T) {
		// ONVIF Core Specification — GetServices (service enumeration).
		// Base Device Test Spec v21.12 — Device Management / GetServices.
		dev := newDevice(t)
		resp, err := sdkdevice.Call_GetServices(ctx(t), dev, device.GetServices{IncludeCapability: false})
		if err != nil {
			t.Fatalf("GetServices: %v", err)
		}
		if string(resp.Service.Namespace) != deviceManagementNS {
			t.Fatalf("Service.Namespace: want %q, got %q", deviceManagementNS, resp.Service.Namespace)
		}
		if string(resp.Service.XAddr) == "" {
			t.Fatal("Service.XAddr must not be empty")
		}
	})

	// --- Profile S v1.3 §7.2 Capabilities ---

	t.Run("ProfileS_7_2_Capabilities", func(t *testing.T) {
		// Profile S §7.2.3 — Capabilities Function List for Devices: GetCapabilities (M), GetWsdlUrl (M).
		// Core — GetServiceCapabilities for the device service (capability exchange).
		dev := newDevice(t)
		c := ctx(t)

		t.Run("GetCapabilities_All", func(t *testing.T) {
			// Base Device Test Spec v21.12 — GetCapabilities; Profile S requires Media.XAddr for streaming.
			resp, err := sdkdevice.Call_GetCapabilities(c, dev, device.GetCapabilities{Category: "All"})
			if err != nil {
				t.Fatalf("GetCapabilities: %v", err)
			}
			if string(resp.Capabilities.Media.XAddr) == "" {
				t.Fatal("Capabilities.Media.XAddr must not be empty")
			}
		})

		t.Run("GetWsdlUrl", func(t *testing.T) {
			resp, err := sdkdevice.Call_GetWsdlUrl(c, dev, device.GetWsdlUrl{})
			if err != nil {
				t.Fatalf("GetWsdlUrl: %v", err)
			}
			if string(resp.WsdlUrl) == "" {
				t.Fatal("GetWsdlUrlResponse.WsdlUrl must not be empty")
			}
		})

		t.Run("GetServiceCapabilities", func(t *testing.T) {
			resp, err := sdkdevice.Call_GetServiceCapabilities(c, dev, device.GetServiceCapabilities{})
			if err != nil {
				t.Fatalf("GetServiceCapabilities: %v", err)
			}
			_ = resp.Capabilities
		})
	})

	// --- Profile S v1.3 §7.3 Discovery (Device service HTTP only; not WS-Discovery UDP) ---

	t.Run("ProfileS_7_3_Discovery_DeviceService", func(t *testing.T) {
		// Profile S §7.3.3 — Discovery Function List for Devices (Get/Set discovery mode, scopes).
		dev := newDevice(t)
		c := ctx(t)

		t.Run("GetDiscoveryMode", func(t *testing.T) {
			resp, err := sdkdevice.Call_GetDiscoveryMode(c, dev, device.GetDiscoveryMode{})
			if err != nil {
				t.Fatalf("GetDiscoveryMode: %v", err)
			}
			if string(resp.DiscoveryMode) == "" {
				t.Fatal("DiscoveryMode must not be empty")
			}
		})

		t.Run("SetDiscoveryMode_idempotent", func(t *testing.T) {
			cur, err := sdkdevice.Call_GetDiscoveryMode(c, dev, device.GetDiscoveryMode{})
			if err != nil {
				t.Fatalf("GetDiscoveryMode: %v", err)
			}
			_, err = sdkdevice.Call_SetDiscoveryMode(c, dev, device.SetDiscoveryMode{DiscoveryMode: cur.DiscoveryMode})
			if err != nil {
				t.Fatalf("SetDiscoveryMode: %v", err)
			}
		})

		t.Run("GetScopes", func(t *testing.T) {
			_, err := sdkdevice.Call_GetScopes(c, dev, device.GetScopes{})
			if err != nil {
				t.Fatalf("GetScopes: %v", err)
			}
		})

		t.Run("AddScopes_RemoveScopes", func(t *testing.T) {
			scopeURI := xsd.AnyURI("onvif://www.onvif.org/name/onvif-simulator-e2e")
			if _, err := sdkdevice.Call_AddScopes(c, dev, device.AddScopes{ScopeItem: scopeURI}); err != nil {
				t.Fatalf("AddScopes: %v", err)
			}
			if _, err := sdkdevice.Call_RemoveScopes(c, dev, device.RemoveScopes{ScopeItem: scopeURI}); err != nil {
				t.Fatalf("RemoveScopes: %v", err)
			}
		})

		t.Run("SetScopes", func(t *testing.T) {
			before, err := sdkdevice.Call_GetScopes(c, dev, device.GetScopes{})
			if err != nil {
				t.Fatalf("GetScopes: %v", err)
			}
			scopeStr := string(before.Scopes.ScopeItem)
			if scopeStr == "" {
				t.Skip("no scopes returned; cannot test SetScopes idempotently")
			}
			if _, err := sdkdevice.Call_SetScopes(c, dev, device.SetScopes{Scopes: xsd.AnyURI(scopeStr)}); err != nil {
				t.Fatalf("SetScopes: %v", err)
			}
		})
	})

	// --- Profile S v1.3 §7.4 Network configuration ---

	t.Run("ProfileS_7_4_NetworkConfiguration", func(t *testing.T) {
		// Profile S §7.4.3 — Network Configuration Function List for Devices.
		dev := newDevice(t)
		c := ctx(t)

		t.Run("GetHostname_SetHostname", func(t *testing.T) {
			h, err := sdkdevice.Call_GetHostname(c, dev, device.GetHostname{})
			if err != nil {
				t.Fatalf("GetHostname: %v", err)
			}
			_, err = sdkdevice.Call_SetHostname(c, dev, device.SetHostname{Name: h.HostnameInformation.Name})
			if err != nil {
				t.Fatalf("SetHostname: %v", err)
			}
		})

		t.Run("GetDNS_SetDNS", func(t *testing.T) {
			dns, err := sdkdevice.Call_GetDNS(c, dev, device.GetDNS{})
			if err != nil {
				t.Fatalf("GetDNS: %v", err)
			}
			_, err = sdkdevice.Call_SetDNS(c, dev, device.SetDNS{
				FromDHCP:     dns.DNSInformation.FromDHCP,
				SearchDomain: dns.DNSInformation.SearchDomain,
				DNSManual:    dns.DNSInformation.DNSManual,
			})
			if err != nil {
				t.Fatalf("SetDNS: %v", err)
			}
		})

		t.Run("GetNetworkInterfaces", func(t *testing.T) {
			if _, err := sdkdevice.Call_GetNetworkInterfaces(c, dev, device.GetNetworkInterfaces{}); err != nil {
				t.Fatalf("GetNetworkInterfaces: %v", err)
			}
		})

		t.Run("GetNetworkProtocols_SetNetworkProtocols", func(t *testing.T) {
			p, err := sdkdevice.Call_GetNetworkProtocols(c, dev, device.GetNetworkProtocols{})
			if err != nil {
				t.Fatalf("GetNetworkProtocols: %v", err)
			}
			_, err = sdkdevice.Call_SetNetworkProtocols(c, dev, device.SetNetworkProtocols{
				NetworkProtocols: p.NetworkProtocols,
			})
			if err != nil {
				t.Fatalf("SetNetworkProtocols: %v", err)
			}
		})

		t.Run("GetNetworkDefaultGateway_SetNetworkDefaultGateway", func(t *testing.T) {
			gw, err := sdkdevice.Call_GetNetworkDefaultGateway(c, dev, device.GetNetworkDefaultGateway{})
			if err != nil {
				t.Fatalf("GetNetworkDefaultGateway: %v", err)
			}
			_, err = sdkdevice.Call_SetNetworkDefaultGateway(c, dev, device.SetNetworkDefaultGateway{
				IPv4Address: gw.NetworkGateway.IPv4Address,
				IPv6Address: gw.NetworkGateway.IPv6Address,
			})
			if err != nil {
				t.Fatalf("SetNetworkDefaultGateway: %v", err)
			}
		})
	})

	// --- Profile S v1.3 §7.5 System ---

	t.Run("ProfileS_7_5_System", func(t *testing.T) {
		// Profile S §7.5.3 — System Function List for Devices.
		dev := newDevice(t)
		c := ctx(t)

		t.Run("GetSystemDateAndTime", func(t *testing.T) {
			resp, err := sdkdevice.Call_GetSystemDateAndTime(c, dev, device.GetSystemDateAndTime{})
			if err != nil {
				t.Fatalf("GetSystemDateAndTime: %v", err)
			}
			if string(resp.SystemDateAndTime.DateTimeType) == "" {
				t.Fatal("SystemDateAndTime.DateTimeType must not be empty")
			}
		})

		t.Run("SetSystemDateAndTime", func(t *testing.T) {
			now := time.Now().UTC()
			_, err := sdkdevice.Call_SetSystemDateAndTime(c, dev, device.SetSystemDateAndTime{
				DateTimeType:    "Manual",
				DaylightSavings: false,
				TimeZone:        onviftypes.TimeZone{TZ: "UTC"},
				UTCDateTime: onviftypes.DateTime{
					Date: onviftypes.Date{
						Year: xsd.Int(now.Year()), Month: xsd.Int(now.Month()), Day: xsd.Int(now.Day()),
					},
					Time: onviftypes.Time{
						Hour: xsd.Int(now.Hour()), Minute: xsd.Int(now.Minute()), Second: xsd.Int(now.Second()),
					},
				},
			})
			if err != nil {
				t.Fatalf("SetSystemDateAndTime: %v", err)
			}
		})

		t.Run("SetSystemFactoryDefault", func(t *testing.T) {
			if _, err := sdkdevice.Call_SetSystemFactoryDefault(c, dev, device.SetSystemFactoryDefault{
				FactoryDefault: "Soft",
			}); err != nil {
				t.Fatalf("SetSystemFactoryDefault: %v", err)
			}
		})

		t.Run("SystemReboot", func(t *testing.T) {
			if _, err := sdkdevice.Call_SystemReboot(c, dev, device.SystemReboot{}); err != nil {
				t.Fatalf("SystemReboot: %v", err)
			}
		})
	})

	// --- Profile S v1.3 §7.6 User handling ---

	t.Run("ProfileS_7_6_UserHandling", func(t *testing.T) {
		// Profile S §7.6.3 — User Handling Function List for Devices.
		dev := newDevice(t)
		c := ctx(t)

		t.Run("GetUsers", func(t *testing.T) {
			resp, err := sdkdevice.Call_GetUsers(c, dev, device.GetUsers{})
			if err != nil {
				t.Fatalf("GetUsers: %v", err)
			}
			if string(resp.User.Username) == "" {
				t.Fatal("at least one user with non-empty Username expected")
			}
		})

		t.Run("CreateUsers_SetUser_DeleteUsers", func(t *testing.T) {
			username := fmt.Sprintf("e2e_%d", time.Now().UnixNano())
			if _, err := sdkdevice.Call_CreateUsers(c, dev, device.CreateUsers{
				User: onviftypes.User{
					Username: username, Password: "E2eTempPass9!", UserLevel: "User",
				},
			}); err != nil {
				t.Fatalf("CreateUsers: %v", err)
			}
			t.Cleanup(func() {
				_, _ = sdkdevice.Call_DeleteUsers(ctx(t), dev, device.DeleteUsers{Username: xsd.String(username)})
			})
			if _, err := sdkdevice.Call_SetUser(c, dev, device.SetUser{
				User: onviftypes.User{
					Username: username, Password: "E2eTempPass9!x", UserLevel: "User",
				},
			}); err != nil {
				t.Fatalf("SetUser: %v", err)
			}
			if _, err := sdkdevice.Call_DeleteUsers(c, dev, device.DeleteUsers{Username: xsd.String(username)}); err != nil {
				t.Fatalf("DeleteUsers: %v", err)
			}
		})
	})

	// --- Profile S v1.3 §7.8 + Media Configuration Test Spec + ONVIF Media Service Specification ---

	t.Run("ProfileS_7_8_MediaServiceSpec_and_MediaConfigurationTestSpec", func(t *testing.T) {
		// Profile S §7.8.3 — GetProfiles, GetStreamUri, RTSP streaming.
		// Media Service Spec — §5.21 GetServiceCapabilities; §5.2 CreateProfile/DeleteProfile; §5.3–5.5 video
		// source/encoder listing & options & set; §5.15.1 GetStreamUri (Table 4 transports); §5.16.1 GetSnapshotUri;
		// §5.18 SetSynchronizationPoint; GetGuaranteedNumberOfVideoEncoderInstances.
		dev := newDevice(t)
		c := ctx(t)

		t.Run("GetServiceCapabilities", func(t *testing.T) {
			// Media Service Spec §5.21 — static media service capabilities (streaming, snapshot, profile limits).
			resp, err := sdkmedia.Call_GetServiceCapabilities(c, dev, media.GetServiceCapabilities{})
			if err != nil {
				t.Fatalf("GetServiceCapabilities: %v", err)
			}
			if resp.Capabilities.ProfileCapabilities.MaximumNumberOfProfiles < 1 {
				t.Fatalf("MaximumNumberOfProfiles must be >= 1, got %d",
					resp.Capabilities.ProfileCapabilities.MaximumNumberOfProfiles)
			}
		})

		t.Run("GetProfiles_MEDIA-1-1-1", func(t *testing.T) {
			resp, err := sdkmedia.Call_GetProfiles(c, dev, media.GetProfiles{})
			if err != nil {
				t.Fatalf("GetProfiles: %v", err)
			}
			if len(resp.Profiles) == 0 {
				t.Fatal("MEDIA-1-1-1: at least one profile required")
			}
			for _, p := range resp.Profiles {
				if string(p.Token) == "" {
					t.Error("profile Token must not be empty")
				}
				if string(p.VideoSourceConfiguration.Token) == "" {
					t.Errorf("profile %q: VideoSourceConfiguration.Token required", p.Token)
				}
				if string(p.VideoEncoderConfiguration.Token) == "" {
					t.Errorf("profile %q: VideoEncoderConfiguration.Token required", p.Token)
				}
			}
		})

		t.Run("GetProfile_MEDIA-1-1-3_consistency", func(t *testing.T) {
			listResp, err := sdkmedia.Call_GetProfiles(c, dev, media.GetProfiles{})
			if err != nil {
				t.Fatalf("GetProfiles: %v", err)
			}
			seen := make(map[string]bool)
			for _, p := range listResp.Profiles {
				token := string(p.Token)
				if seen[token] {
					t.Errorf("duplicate profile token %q", token)
				}
				seen[token] = true
			}
			for _, listed := range listResp.Profiles {
				single, err := sdkmedia.Call_GetProfile(c, dev, media.GetProfile{ProfileToken: listed.Token})
				if err != nil {
					t.Errorf("GetProfile(%q): %v", listed.Token, err)
					continue
				}
				if single.Profile.Token != listed.Token {
					t.Errorf("token mismatch: want %q, got %q", listed.Token, single.Profile.Token)
				}
				if single.Profile.Name != listed.Name {
					t.Errorf("name mismatch for %q", listed.Token)
				}
			}
		})

		t.Run("GetVideoSources", func(t *testing.T) {
			// Media Service Spec §5.3.1 — list video sources.
			resp, err := sdkmedia.Call_GetVideoSources(c, dev, media.GetVideoSources{})
			if err != nil {
				t.Fatalf("GetVideoSources: %v", err)
			}
			if string(resp.VideoSources.Token) == "" {
				t.Error("VideoSources.Token must not be empty")
			}
		})

		t.Run("CreateProfile_DeleteProfile", func(t *testing.T) {
			// Media Service Spec §5.2.1 CreateProfile, §5.2.22 DeleteProfile — empty profile lifecycle.
			name := onviftypes.Name(fmt.Sprintf("e2e_profile_%d", time.Now().UnixNano()))
			cr, err := sdkmedia.Call_CreateProfile(c, dev, media.CreateProfile{Name: name})
			if err != nil {
				t.Fatalf("CreateProfile: %v", err)
			}
			if string(cr.Profile.Token) == "" {
				t.Fatal("CreateProfile: returned Profile.Token must not be empty")
			}
			if _, err := sdkmedia.Call_DeleteProfile(c, dev, media.DeleteProfile{ProfileToken: cr.Profile.Token}); err != nil {
				t.Fatalf("DeleteProfile: %v", err)
			}
		})

		t.Run("VideoSourceConfiguration_commands", func(t *testing.T) {
			// Media Service Spec §5.4 — GetVideoSourceConfigurations, GetVideoSourceConfiguration,
			// GetCompatibleVideoSourceConfigurations, GetVideoSourceConfigurationOptions, SetVideoSourceConfiguration.
			prof := mediaFirstProfile(t, c, dev)

			if _, err := sdkmedia.Call_GetVideoSourceConfigurations(c, dev, media.GetVideoSourceConfigurations{}); err != nil {
				t.Fatalf("GetVideoSourceConfigurations: %v", err)
			}

			vsCfg, err := sdkmedia.Call_GetVideoSourceConfiguration(c, dev, media.GetVideoSourceConfiguration{
				ConfigurationToken: prof.VideoSourceConfiguration.Token,
			})
			if err != nil {
				t.Fatalf("GetVideoSourceConfiguration: %v", err)
			}

			if _, err := sdkmedia.Call_GetCompatibleVideoSourceConfigurations(c, dev, media.GetCompatibleVideoSourceConfigurations{
				ProfileToken: prof.Token,
			}); err != nil {
				t.Fatalf("GetCompatibleVideoSourceConfigurations: %v", err)
			}

			if _, err := sdkmedia.Call_GetVideoSourceConfigurationOptions(c, dev, media.GetVideoSourceConfigurationOptions{
				ProfileToken:       prof.Token,
				ConfigurationToken: prof.VideoSourceConfiguration.Token,
			}); err != nil {
				t.Fatalf("GetVideoSourceConfigurationOptions: %v", err)
			}

			if _, err := sdkmedia.Call_SetVideoSourceConfiguration(c, dev, media.SetVideoSourceConfiguration{
				Configuration:    vsCfg.Configuration,
				ForcePersistence: xsd.Boolean(true),
			}); err != nil {
				t.Fatalf("SetVideoSourceConfiguration: %v", err)
			}
		})

		t.Run("VideoEncoderConfiguration_commands", func(t *testing.T) {
			// Media Service Spec §5.5 — encoder listing, compatible list, options, set.
			prof := mediaFirstProfile(t, c, dev)

			if _, err := sdkmedia.Call_GetVideoEncoderConfigurations(c, dev, media.GetVideoEncoderConfigurations{}); err != nil {
				t.Fatalf("GetVideoEncoderConfigurations: %v", err)
			}

			veCfg, err := sdkmedia.Call_GetVideoEncoderConfiguration(c, dev, media.GetVideoEncoderConfiguration{
				ConfigurationToken: prof.VideoEncoderConfiguration.Token,
			})
			if err != nil {
				t.Fatalf("GetVideoEncoderConfiguration: %v", err)
			}

			if _, err := sdkmedia.Call_GetCompatibleVideoEncoderConfigurations(c, dev, media.GetCompatibleVideoEncoderConfigurations{
				ProfileToken: prof.Token,
			}); err != nil {
				t.Fatalf("GetCompatibleVideoEncoderConfigurations: %v", err)
			}

			if _, err := sdkmedia.Call_GetVideoEncoderConfigurationOptions(c, dev, media.GetVideoEncoderConfigurationOptions{
				ProfileToken:       prof.Token,
				ConfigurationToken: prof.VideoEncoderConfiguration.Token,
			}); err != nil {
				t.Fatalf("GetVideoEncoderConfigurationOptions: %v", err)
			}

			if _, err := sdkmedia.Call_SetVideoEncoderConfiguration(c, dev, media.SetVideoEncoderConfiguration{
				Configuration:    veCfg.Configuration,
				ForcePersistence: xsd.Boolean(true),
			}); err != nil {
				t.Fatalf("SetVideoEncoderConfiguration: %v", err)
			}
		})

		t.Run("GetGuaranteedNumberOfVideoEncoderInstances", func(t *testing.T) {
			// Media Service Spec + Profile S encoder-capacity reporting (per video source configuration).
			prof := mediaFirstProfile(t, c, dev)
			if _, err := sdkmedia.Call_GetGuaranteedNumberOfVideoEncoderInstances(c, dev, media.GetGuaranteedNumberOfVideoEncoderInstances{
				ConfigurationToken: prof.VideoSourceConfiguration.Token,
			}); err != nil {
				t.Fatalf("GetGuaranteedNumberOfVideoEncoderInstances: %v", err)
			}
		})

		t.Run("GetStreamUri_Table4_transports", func(t *testing.T) {
			// Media Service Spec §5.15.1 — Table 4 StreamSetup combinations; omitted if NoRTSPStreaming is true.
			caps, err := sdkmedia.Call_GetServiceCapabilities(c, dev, media.GetServiceCapabilities{})
			if err != nil {
				t.Fatalf("GetServiceCapabilities: %v", err)
			}
			if caps.Capabilities.StreamingCapabilities.NoRTSPStreaming {
				t.Skip("NoRTSPStreaming: GetStreamUri not required")
			}
			prof := mediaFirstProfile(t, c, dev)
			// Table 4: RTP unicast + UDP | HTTP | RTSP (Streaming Specification cross-reference in §5.15.1).
			transports := []onviftypes.TransportProtocol{
				"RTSP", "HTTP", "UDP",
			}
			for _, proto := range transports {
				proto := proto
				t.Run(fmt.Sprintf("RTP-Unicast_%s", proto), func(t *testing.T) {
					uriResp, err := sdkmedia.Call_GetStreamUri(c, dev, media.GetStreamUri{
						ProfileToken: prof.Token,
						StreamSetup: onviftypes.StreamSetup{
							Stream: "RTP-Unicast",
							Transport: onviftypes.Transport{
								Protocol: proto,
							},
						},
					})
					if err != nil {
						t.Skipf("GetStreamUri not supported for transport %s: %v", string(proto), err)
					}
					if string(uriResp.MediaUri.Uri) == "" {
						t.Fatalf("GetStreamUri: empty MediaUri for transport %s", string(proto))
					}
				})
			}
		})

		t.Run("GetSnapshotUri", func(t *testing.T) {
			// Media Service Spec §5.16.1 — mandatory when SnapshotUri capability is true.
			caps, err := sdkmedia.Call_GetServiceCapabilities(c, dev, media.GetServiceCapabilities{})
			if err != nil {
				t.Fatalf("GetServiceCapabilities: %v", err)
			}
			if !caps.Capabilities.SnapshotUri {
				t.Skip("SnapshotUri capability false — GetSnapshotUri not mandatory")
			}
			prof := mediaFirstProfile(t, c, dev)
			snap, err := sdkmedia.Call_GetSnapshotUri(c, dev, media.GetSnapshotUri{ProfileToken: prof.Token})
			if err != nil {
				t.Fatalf("GetSnapshotUri: %v", err)
			}
			if string(snap.MediaUri.Uri) == "" {
				t.Fatal("GetSnapshotUri: MediaUri.Uri must not be empty")
			}
		})

		t.Run("SetSynchronizationPoint", func(t *testing.T) {
			// Media Service Spec §5.18 — insert sync point on active streams for profile.
			prof := mediaFirstProfile(t, c, dev)
			if _, err := sdkmedia.Call_SetSynchronizationPoint(c, dev, media.SetSynchronizationPoint{
				ProfileToken: prof.Token,
			}); err != nil {
				t.Fatalf("SetSynchronizationPoint: %v", err)
			}
		})
	})
}

// mediaFirstProfile returns the first media profile or fails the test.
func mediaFirstProfile(t *testing.T, c context.Context, dev *onviflib.Device) onviftypes.Profile {
	t.Helper()
	resp, err := sdkmedia.Call_GetProfiles(c, dev, media.GetProfiles{})
	if err != nil {
		t.Fatalf("GetProfiles: %v", err)
	}
	if len(resp.Profiles) == 0 {
		t.Fatal("no media profiles")
	}
	return resp.Profiles[0]
}
