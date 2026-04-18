//go:build e2e

package e2e

import (
	"testing"

	"github.com/use-go/onvif/device"
	sdkdevice "github.com/use-go/onvif/sdk/device"
)

// TestGetDeviceInformation verifies that the simulator returns valid device
// identification fields.
//
// Derived from ONVIF Base Device Test Specification v21.12, section
// "Device Management – GetDeviceInformation".
//
// PASS criteria:
//   - GetDeviceInformationResponse is received
//   - Manufacturer, Model, FirmwareVersion, SerialNumber, HardwareId are non-empty
func TestGetDeviceInformation(t *testing.T) {
	dev := newDevice(t)

	resp, err := sdkdevice.Call_GetDeviceInformation(ctx(t), dev, device.GetDeviceInformation{})
	if err != nil {
		t.Fatalf("GetDeviceInformation failed: %v", err)
	}

	fields := map[string]string{
		"Manufacturer":    string(resp.Manufacturer),
		"Model":           string(resp.Model),
		"FirmwareVersion": string(resp.FirmwareVersion),
		"SerialNumber":    string(resp.SerialNumber),
		"HardwareId":      string(resp.HardwareId),
	}

	for name, value := range fields {
		if value == "" {
			t.Errorf("GetDeviceInformationResponse.%s must not be empty", name)
		}
	}
}

// TestGetCapabilities verifies that the simulator advertises at least the
// Media service endpoint required by Profile S.
//
// Derived from ONVIF Base Device Test Specification v21.12, section
// "Device Management – GetCapabilities".
//
// PASS criteria:
//   - GetCapabilitiesResponse is received
//   - Capabilities.Media is present
//   - Capabilities.Media.XAddr is non-empty
func TestGetCapabilities(t *testing.T) {
	dev := newDevice(t)

	resp, err := sdkdevice.Call_GetCapabilities(ctx(t), dev, device.GetCapabilities{
		Category: "All",
	})
	if err != nil {
		t.Fatalf("GetCapabilities failed: %v", err)
	}

	if resp.Capabilities.Media.XAddr == "" {
		t.Error("GetCapabilitiesResponse.Capabilities.Media.XAddr must not be empty")
	}
}

// TestGetServices verifies that the simulator lists the Device service and that
// its namespace is the well-known ONVIF device management namespace.
//
// Derived from ONVIF Base Device Test Specification v21.12, section
// "Device Management – GetServices".
//
// PASS criteria:
//   - GetServicesResponse is received
//   - Service.Namespace matches the ONVIF device management namespace
//   - Service.XAddr is non-empty
func TestGetServices(t *testing.T) {
	dev := newDevice(t)

	resp, err := sdkdevice.Call_GetServices(ctx(t), dev, device.GetServices{
		IncludeCapability: false,
	})
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}

	const deviceNS = "http://www.onvif.org/ver10/device/wsdl"

	if string(resp.Service.Namespace) != deviceNS {
		t.Errorf("GetServicesResponse.Service.Namespace: want %q, got %q",
			deviceNS, resp.Service.Namespace)
	}

	if string(resp.Service.XAddr) == "" {
		t.Error("GetServicesResponse.Service.XAddr must not be empty")
	}
}
