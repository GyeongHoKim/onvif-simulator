package simulator

import (
	"context"

	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/deviceiosvc"
)

// deviceIOProvider implements deviceiosvc.Provider for the simulator.
type deviceIOProvider struct {
	sim *Simulator
}

func newDeviceIOProvider(s *Simulator) *deviceIOProvider {
	return &deviceIOProvider{sim: s}
}

func (*deviceIOProvider) ServiceCapabilities(_ context.Context) (deviceiosvc.ServiceCapabilities, error) {
	return deviceiosvc.ServiceCapabilities{
		RelayOutputs:  2,
		DigitalInputs: 4,
	}, nil
}
