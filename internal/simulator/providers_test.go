package simulator

import (
	"context"
	"errors"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/devicesvc"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/mediasvc"
)

//nolint:gocyclo,cyclop,govet // sweep test exercises many provider methods sequentially.
func TestDeviceProviderReadsConfig(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()
	dp := sim.deviceProv
	ctx := context.Background()

	info, err := dp.DeviceInfo(ctx)
	if err != nil {
		t.Fatalf("DeviceInfo: %v", err)
	}
	if info.Manufacturer != "Test" || info.Serial != "SN-1" {
		t.Fatalf("unexpected device info: %+v", info)
	}

	svcs, err := dp.Services(ctx, false)
	if err != nil || len(svcs) != 3 {
		t.Fatalf("Services: %v %d", err, len(svcs))
	}

	caps, err := dp.GetServiceCapabilities(ctx)
	if err != nil || !caps.Security.HTTPDigest {
		t.Fatalf("GetServiceCapabilities: %v %+v", err, caps)
	}

	if _, err := dp.GetCapabilities(ctx, "All"); err != nil {
		t.Fatalf("GetCapabilities: %v", err)
	}

	if _, err := dp.WsdlURL(ctx); err != nil {
		t.Fatalf("WsdlURL: %v", err)
	}

	mode, err := dp.GetDiscoveryMode(ctx)
	if err != nil || mode.DiscoveryMode == "" {
		t.Fatalf("GetDiscoveryMode: %v %+v", err, mode)
	}

	scopes, err := dp.GetScopes(ctx)
	if err != nil || len(scopes) == 0 {
		t.Fatalf("GetScopes: %v %v", err, scopes)
	}

	if err := dp.SetScopes(ctx, []string{"onvif://www.onvif.org/name/test2"}); err != nil {
		t.Fatalf("SetScopes: %v", err)
	}
	if err := dp.AddScopes(ctx, []string{"onvif://www.onvif.org/name/extra"}); err != nil {
		t.Fatalf("AddScopes: %v", err)
	}
	removed, err := dp.RemoveScopes(ctx, []string{"onvif://www.onvif.org/name/extra"})
	if err != nil || len(removed) != 1 {
		t.Fatalf("RemoveScopes: %v %v", err, removed)
	}

	if _, err := dp.GetHostname(ctx); err != nil {
		t.Fatalf("GetHostname: %v", err)
	}
	if _, err := dp.GetDNS(ctx); err != nil {
		t.Fatalf("GetDNS: %v", err)
	}
	if err := dp.SetDNS(ctx, devicesvc.DNSInfo{FromDHCP: true}); err != nil {
		t.Fatalf("SetDNS: %v", err)
	}
	if _, err := dp.GetNetworkInterfaces(ctx); err != nil {
		t.Fatalf("GetNetworkInterfaces: %v", err)
	}
	if err := dp.SetNetworkInterfaces(ctx, nil); err != nil {
		t.Fatalf("SetNetworkInterfaces: %v", err)
	}
	if _, err := dp.GetNetworkProtocols(ctx); err != nil {
		t.Fatalf("GetNetworkProtocols: %v", err)
	}
	if err := dp.SetNetworkProtocols(ctx, nil); err != nil {
		t.Fatalf("SetNetworkProtocols: %v", err)
	}
	if _, err := dp.GetNetworkDefaultGateway(ctx); err != nil {
		t.Fatalf("GetNetworkDefaultGateway: %v", err)
	}
	if err := dp.SetNetworkDefaultGateway(ctx, devicesvc.DefaultGatewayInfo{}); err != nil {
		t.Fatalf("SetNetworkDefaultGateway: %v", err)
	}
	if _, err := dp.GetSystemDateAndTime(ctx); err != nil {
		t.Fatalf("GetSystemDateAndTime: %v", err)
	}
	if err := dp.SetSystemDateAndTime(ctx, devicesvc.SetSystemDateAndTimeParams{
		DateTimeType: "Manual", TZ: "UTC",
		UTCDateTime: devicesvc.SystemDateTime{Year: 2026, Month: 1, Day: 1, Hour: 0, Minute: 0, Second: 0},
	}); err != nil {
		t.Fatalf("SetSystemDateAndTime: %v", err)
	}
	if err := dp.SetSystemFactoryDefault(ctx, "Soft"); err != nil {
		t.Fatalf("SetSystemFactoryDefault: %v", err)
	}
	if _, err := dp.SystemReboot(ctx); err != nil {
		t.Fatalf("SystemReboot: %v", err)
	}
}

func TestDeviceProviderUserOps(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()
	dp := sim.deviceProv
	ctx := context.Background()

	if err := dp.CreateUsers(ctx, []devicesvc.UserInfo{{
		Username: "bob", Password: "pw", UserLevel: config.RoleOperator,
	}}); err != nil {
		t.Fatalf("CreateUsers: %v", err)
	}
	users, err := dp.GetUsers(ctx)
	if err != nil || len(users) != 1 {
		t.Fatalf("GetUsers: %v %v", err, users)
	}
	if err := dp.SetUser(ctx, []devicesvc.UserInfo{{
		Username: "bob", Password: "newpw", UserLevel: config.RoleAdministrator,
	}}); err != nil {
		t.Fatalf("SetUser: %v", err)
	}
	if err := dp.DeleteUsers(ctx, []string{"bob"}); err != nil {
		t.Fatalf("DeleteUsers: %v", err)
	}
}

//nolint:gocyclo,cyclop,govet // sweep test exercises every Media provider method.
func TestMediaProvider(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()
	mp := sim.mediaProv
	ctx := context.Background()

	caps, err := mp.ServiceCapabilities(ctx)
	if err != nil || !caps.SnapshotURI {
		t.Fatalf("ServiceCapabilities: %v %+v", err, caps)
	}

	profiles, err := mp.Profiles(ctx)
	if err != nil || len(profiles) == 0 {
		t.Fatalf("Profiles: %v %v", err, profiles)
	}
	prof, err := mp.Profile(ctx, profiles[0].Token)
	if err != nil || prof.Token != profiles[0].Token {
		t.Fatalf("Profile: %v %+v", err, prof)
	}
	if _, err := mp.Profile(ctx, "missing"); !errors.Is(err, mediasvc.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}

	if _, err := mp.CreateProfile(ctx, "x", "x"); !errors.Is(err, mediasvc.ErrInvalidArgs) {
		t.Fatalf("expected ErrInvalidArgs for CreateProfile, got %v", err)
	}
	if err := mp.DeleteProfile(ctx, "x"); !errors.Is(err, mediasvc.ErrInvalidArgs) {
		t.Fatalf("expected ErrInvalidArgs for DeleteProfile, got %v", err)
	}

	if _, err := mp.VideoSources(ctx); err != nil {
		t.Fatalf("VideoSources: %v", err)
	}
	vs, err := mp.VideoSourceConfigurations(ctx)
	if err != nil || len(vs) == 0 {
		t.Fatalf("VideoSourceConfigurations: %v", err)
	}
	if _, err := mp.VideoSourceConfiguration(ctx, vs[0].SourceToken); err != nil {
		t.Fatalf("VideoSourceConfiguration: %v", err)
	}
	if _, err := mp.VideoSourceConfiguration(ctx, "missing"); !errors.Is(err, mediasvc.ErrConfigNotFound) {
		t.Fatalf("expected ErrConfigNotFound, got %v", err)
	}
	if err := mp.SetVideoSourceConfiguration(ctx, mediasvc.VideoSourceConfiguration{}); err != nil {
		t.Fatalf("SetVideoSourceConfiguration: %v", err)
	}
	if err := mp.AddVideoSourceConfiguration(ctx, "p", "c"); err != nil {
		t.Fatalf("AddVideoSourceConfiguration: %v", err)
	}
	if err := mp.RemoveVideoSourceConfiguration(ctx, "p"); err != nil {
		t.Fatalf("RemoveVideoSourceConfiguration: %v", err)
	}
	if _, err := mp.CompatibleVideoSourceConfigurations(ctx, "p"); err != nil {
		t.Fatalf("CompatibleVideoSourceConfigurations: %v", err)
	}
	if _, err := mp.VideoSourceConfigurationOptions(ctx, "", ""); err != nil {
		t.Fatalf("VideoSourceConfigurationOptions: %v", err)
	}

	encs, err := mp.VideoEncoderConfigurations(ctx)
	if err != nil || len(encs) == 0 {
		t.Fatalf("VideoEncoderConfigurations: %v", err)
	}
	if _, err := mp.VideoEncoderConfiguration(ctx, profiles[0].Token); err != nil {
		t.Fatalf("VideoEncoderConfiguration: %v", err)
	}
	if _, err := mp.VideoEncoderConfiguration(ctx, "missing"); !errors.Is(err, mediasvc.ErrConfigNotFound) {
		t.Fatalf("expected ErrConfigNotFound, got %v", err)
	}
	if err := mp.SetVideoEncoderConfiguration(ctx, mediasvc.VideoEncoderConfiguration{}); err != nil {
		t.Fatalf("SetVideoEncoderConfiguration: %v", err)
	}
	if err := mp.AddVideoEncoderConfiguration(ctx, "p", "c"); err != nil {
		t.Fatalf("AddVideoEncoderConfiguration: %v", err)
	}
	if err := mp.RemoveVideoEncoderConfiguration(ctx, "p"); err != nil {
		t.Fatalf("RemoveVideoEncoderConfiguration: %v", err)
	}
	if _, err := mp.CompatibleVideoEncoderConfigurations(ctx, "p"); err != nil {
		t.Fatalf("CompatibleVideoEncoderConfigurations: %v", err)
	}
	if _, err := mp.VideoEncoderConfigurationOptions(ctx, "", ""); err != nil {
		t.Fatalf("VideoEncoderConfigurationOptions: %v", err)
	}

	uri, err := mp.StreamURI(ctx, profiles[0].Token, mediasvc.StreamSetup{})
	if err != nil || uri.URI == "" {
		t.Fatalf("StreamURI: %v %+v", err, uri)
	}
	if _, err := mp.StreamURI(ctx, "missing", mediasvc.StreamSetup{}); !errors.Is(err, mediasvc.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound for StreamURI, got %v", err)
	}
	if _, err := mp.SnapshotURI(ctx, profiles[0].Token); !errors.Is(err, mediasvc.ErrNoSnapshot) {
		t.Fatalf("expected ErrNoSnapshot for profile without snapshot, got %v", err)
	}
	if _, err := mp.SnapshotURI(ctx, "missing"); !errors.Is(err, mediasvc.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound for SnapshotURI, got %v", err)
	}

	if _, err := mp.GuaranteedNumberOfVideoEncoderInstances(ctx, ""); err != nil {
		t.Fatalf("GuaranteedNumberOfVideoEncoderInstances: %v", err)
	}

	if _, err := mp.MetadataConfigurations(ctx); err != nil {
		t.Fatalf("MetadataConfigurations: %v", err)
	}
	if _, err := mp.MetadataConfiguration(ctx, "missing"); err == nil {
		t.Fatal("expected ErrConfigNotFound")
	}
	if err := mp.AddMetadataConfiguration(ctx, "p", "c"); err != nil {
		t.Fatalf("AddMetadataConfiguration: %v", err)
	}
	if err := mp.RemoveMetadataConfiguration(ctx, "p"); err != nil {
		t.Fatalf("RemoveMetadataConfiguration: %v", err)
	}
	if _, err := mp.MetadataConfigurationOptions(ctx, "", ""); err != nil {
		t.Fatalf("MetadataConfigurationOptions: %v", err)
	}
	if _, err := mp.CompatibleMetadataConfigurations(ctx, "p"); err != nil {
		t.Fatalf("CompatibleMetadataConfigurations: %v", err)
	}
}

func TestSnapshotURIWhenSet(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	if err := sim.SetProfileSnapshotURI("profile_main", "http://127.0.0.1/snap.jpg"); err != nil {
		t.Fatalf("SetProfileSnapshotURI: %v", err)
	}
	uri, err := sim.mediaProv.SnapshotURI(context.Background(), "profile_main")
	if err != nil {
		t.Fatalf("SnapshotURI: %v", err)
	}
	if uri.URI == "" {
		t.Fatal("expected non-empty snapshot uri")
	}
}

func TestNormaliseUserLevel(t *testing.T) {
	if got := normaliseUserLevel(""); got != config.RoleUser {
		t.Fatalf("expected default role User, got %q", got)
	}
	if got := normaliseUserLevel("  Operator  "); got != "Operator" {
		t.Fatalf("expected trimmed Operator, got %q", got)
	}
}

func TestBaseURLAndHTTPURL(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	url := sim.baseURL(8080)
	if url == "" {
		t.Fatal("expected non-empty baseURL")
	}
	if got := httpURL("127.0.0.1", 8080, "/x"); got != "http://127.0.0.1:8080/x" {
		t.Fatalf("unexpected httpURL: %s", got)
	}
}

func TestGetSystemDateAndTimeManual(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	ctx := context.Background()
	if err := sim.deviceProv.SetSystemDateAndTime(ctx, devicesvc.SetSystemDateAndTimeParams{
		DateTimeType: "Manual",
		TZ:           "UTC",
		UTCDateTime:  devicesvc.SystemDateTime{Year: 2026, Month: 4, Day: 1, Hour: 12},
	}); err != nil {
		t.Fatalf("SetSystemDateAndTime: %v", err)
	}
	if err := sim.reloadFromDisk(); err != nil {
		t.Fatalf("reloadFromDisk: %v", err)
	}
	info, err := sim.deviceProv.GetSystemDateAndTime(ctx)
	if err != nil {
		t.Fatalf("GetSystemDateAndTime: %v", err)
	}
	if info.DateTimeType != "Manual" {
		t.Fatalf("expected Manual DateTimeType, got %q", info.DateTimeType)
	}
}

func TestSetNetworkInterfacesWithIPv4(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	ctx := context.Background()
	ifaces := []devicesvc.NetworkInterfaceInfo{
		{
			Token:   "eth0",
			Enabled: true,
			IPv4: &devicesvc.IPv4Config{
				Enabled: true,
				DHCP:    false,
				Manual:  []string{"192.168.1.100/24"},
			},
		},
	}
	if err := sim.deviceProv.SetNetworkInterfaces(ctx, ifaces); err != nil {
		t.Fatalf("SetNetworkInterfaces: %v", err)
	}
	if err := sim.reloadFromDisk(); err != nil {
		t.Fatalf("reloadFromDisk: %v", err)
	}
	got, err := sim.deviceProv.GetNetworkInterfaces(ctx)
	if err != nil {
		t.Fatalf("GetNetworkInterfaces: %v", err)
	}
	if len(got) != 1 || got[0].IPv4 == nil {
		t.Fatalf("expected 1 interface with IPv4 config, got %+v", got)
	}
}

func TestGetNetworkProtocolsRoundTrip(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	ctx := context.Background()
	want := []devicesvc.NetworkProtocol{
		{Name: "HTTP", Enabled: true, Port: []int{80}},
		{Name: "HTTPS", Enabled: false, Port: []int{443}},
	}
	if err := sim.deviceProv.SetNetworkProtocols(ctx, want); err != nil {
		t.Fatalf("SetNetworkProtocols: %v", err)
	}
	if err := sim.reloadFromDisk(); err != nil {
		t.Fatalf("reloadFromDisk: %v", err)
	}
	got, err := sim.deviceProv.GetNetworkProtocols(ctx)
	if err != nil {
		t.Fatalf("GetNetworkProtocols: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d protocols, got %d", len(want), len(got))
	}
}

func TestVideoSourcesDeduplicated(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	// Add a second profile sharing the same video source token.
	if err := sim.AddProfile(config.ProfileConfig{
		Name:             "sub",
		Token:            "profile_sub",
		RTSP:             "rtsp://127.0.0.1:8554/sub",
		Encoding:         "H264",
		Width:            640,
		Height:           480,
		FPS:              15,
		VideoSourceToken: config.DefaultVideoSourceToken,
	}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	ctx := context.Background()
	srcs, err := sim.mediaProv.VideoSources(ctx)
	if err != nil {
		t.Fatalf("VideoSources: %v", err)
	}
	if len(srcs) != 1 {
		t.Fatalf("expected 1 deduplicated VideoSource, got %d", len(srcs))
	}

	vsCfgs, err := sim.mediaProv.VideoSourceConfigurations(ctx)
	if err != nil {
		t.Fatalf("VideoSourceConfigurations: %v", err)
	}
	if len(vsCfgs) != 1 {
		t.Fatalf("expected 1 deduplicated VideoSourceConfiguration, got %d", len(vsCfgs))
	}
}

func TestMetadataConfigurationFoundAndSet(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	ctx := context.Background()
	if err := sim.mediaProv.SetMetadataConfiguration(ctx, mediasvc.MetadataConfiguration{
		Token:     "md_tok",
		Name:      "md_name",
		Analytics: true,
	}); err != nil {
		t.Fatalf("SetMetadataConfiguration: %v", err)
	}
	if err := sim.reloadFromDisk(); err != nil {
		t.Fatalf("reloadFromDisk: %v", err)
	}
	got, err := sim.mediaProv.MetadataConfiguration(ctx, "md_tok")
	if err != nil {
		t.Fatalf("MetadataConfiguration: %v", err)
	}
	if got.Token != "md_tok" || !got.Analytics {
		t.Fatalf("unexpected metadata config: %+v", got)
	}
}

func TestGuaranteedEncoderInstancesCustomMax(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	if err := config.Update(func(c *config.Config) error {
		c.Media.MaxVideoEncoderInstances = 4
		return nil
	}); err != nil {
		t.Fatalf("config.Update: %v", err)
	}
	if err := sim.reloadFromDisk(); err != nil {
		t.Fatalf("reloadFromDisk: %v", err)
	}
	n, err := sim.mediaProv.GuaranteedNumberOfVideoEncoderInstances(context.Background(), "")
	if err != nil {
		t.Fatalf("GuaranteedNumberOfVideoEncoderInstances: %v", err)
	}
	if n != 4 {
		t.Fatalf("expected 4 encoder instances, got %d", n)
	}
}

func TestVideoEncoderConfigurationH264Fields(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	enc, err := sim.mediaProv.VideoEncoderConfiguration(context.Background(), "profile_main")
	if err != nil {
		t.Fatalf("VideoEncoderConfiguration: %v", err)
	}
	if enc.H264.H264Profile == "" {
		t.Fatal("expected H264Profile to be set for H264 encoding")
	}
}

func TestSetHostnameRoundTrip(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	ctx := context.Background()
	if err := sim.deviceProv.SetHostname(ctx, "mydevice"); err != nil {
		t.Fatalf("SetHostname: %v", err)
	}
	info, err := sim.deviceProv.GetHostname(ctx)
	if err != nil {
		t.Fatalf("GetHostname: %v", err)
	}
	if info.Name != "mydevice" {
		t.Fatalf("expected hostname 'mydevice', got %q", info.Name)
	}
}

func TestSetDiscoveryModeRoundTrip(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	ctx := context.Background()
	if err := sim.deviceProv.SetDiscoveryMode(ctx, discoveryModeNonDiscoverable); err != nil {
		t.Fatalf("SetDiscoveryMode: %v", err)
	}
	info, err := sim.deviceProv.GetDiscoveryMode(ctx)
	if err != nil {
		t.Fatalf("GetDiscoveryMode: %v", err)
	}
	if info.DiscoveryMode != discoveryModeNonDiscoverable {
		t.Fatalf("expected NonDiscoverable, got %q", info.DiscoveryMode)
	}
}

func TestGetDefaultGatewayRoundTrip(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	ctx := context.Background()
	if err := sim.deviceProv.SetNetworkDefaultGateway(ctx, devicesvc.DefaultGatewayInfo{
		IPv4Address: []string{"192.168.1.1"},
	}); err != nil {
		t.Fatalf("SetNetworkDefaultGateway: %v", err)
	}
	if err := sim.reloadFromDisk(); err != nil {
		t.Fatalf("reloadFromDisk: %v", err)
	}
	info, err := sim.deviceProv.GetNetworkDefaultGateway(ctx)
	if err != nil {
		t.Fatalf("GetNetworkDefaultGateway: %v", err)
	}
	if len(info.IPv4Address) == 0 {
		t.Fatal("expected IPv4Address in gateway info")
	}
}
