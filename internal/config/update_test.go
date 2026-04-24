package config_test

import (
	"errors"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

func seed(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	if err := config.Save(&validConfig); err != nil {
		t.Fatalf("seed Save: %v", err)
	}
}

func loadOrFail(t *testing.T) config.Config {
	t.Helper()
	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return c
}

func upsertOrFail(t *testing.T, u config.UserConfig) {
	t.Helper()
	if err := config.UpsertUser(u); err != nil {
		t.Fatalf("UpsertUser(%q): %v", u.Username, err)
	}
}

func TestUpdate(t *testing.T) {
	seed(t)

	err := config.Update(func(c *config.Config) error {
		c.Device.Firmware = "9.9.9"
		return nil
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	got := loadOrFail(t)
	if got.Device.Firmware != "9.9.9" {
		t.Fatalf("firmware not persisted: %q", got.Device.Firmware)
	}
}

func TestUpdateRejectsInvalid(t *testing.T) {
	seed(t)

	err := config.Update(func(c *config.Config) error {
		c.Network.HTTPPort = 0
		return nil
	})
	if !errors.Is(err, config.ErrNetworkPortInvalid) {
		t.Fatalf("expected ErrNetworkPortInvalid, got %v", err)
	}
	// Disk must still contain the original valid value.
	got := loadOrFail(t)
	if got.Network.HTTPPort != validConfig.Network.HTTPPort {
		t.Fatalf("disk mutated on invalid Update: got %d want %d", got.Network.HTTPPort, validConfig.Network.HTTPPort)
	}
}

func TestAddUser(t *testing.T) {
	seed(t)

	u := config.UserConfig{Username: "admin", Password: "pw", Role: config.RoleAdministrator}
	if err := config.SetAuthEnabled(true); err == nil {
		// enabling auth with no users should fail validation
		t.Fatal("expected error enabling auth with empty users")
	}

	if err := config.AddUser(u); err != nil {
		t.Fatalf("AddUser: %v", err)
	}
	if err := config.SetAuthEnabled(true); err != nil {
		t.Fatalf("SetAuthEnabled: %v", err)
	}

	if err := config.AddUser(u); !errors.Is(err, config.ErrUserAlreadyExists) {
		t.Fatalf("expected ErrUserAlreadyExists, got %v", err)
	}

	got := loadOrFail(t)
	if len(got.Auth.Users) != 1 || got.Auth.Users[0].Username != "admin" {
		t.Fatalf("unexpected users: %+v", got.Auth.Users)
	}
	if !got.Auth.Enabled {
		t.Fatal("auth.enabled should be true")
	}
}

func TestUpsertUser(t *testing.T) {
	seed(t)

	u1 := config.UserConfig{Username: "admin", Password: "pw1", Role: config.RoleAdministrator}
	u2 := config.UserConfig{Username: "admin", Password: "pw2", Role: config.RoleOperator}
	upsertOrFail(t, u1)
	upsertOrFail(t, u2)
	got := loadOrFail(t)
	if len(got.Auth.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(got.Auth.Users))
	}
	if got.Auth.Users[0].Password != "pw2" || got.Auth.Users[0].Role != config.RoleOperator {
		t.Fatalf("upsert did not replace: %+v", got.Auth.Users[0])
	}
}

func TestRemoveUser(t *testing.T) {
	seed(t)

	upsertOrFail(t, config.UserConfig{Username: "a", Password: "p", Role: config.RoleUser})
	upsertOrFail(t, config.UserConfig{Username: "b", Password: "p", Role: config.RoleUser})

	if err := config.RemoveUser("a"); err != nil {
		t.Fatalf("RemoveUser: %v", err)
	}
	if err := config.RemoveUser("ghost"); !errors.Is(err, config.ErrUserNotFound) {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}

	got := loadOrFail(t)
	if len(got.Auth.Users) != 1 || got.Auth.Users[0].Username != "b" {
		t.Fatalf("unexpected users after remove: %+v", got.Auth.Users)
	}
}

func TestSetJWTIssuer(t *testing.T) {
	seed(t)

	if err := config.SetJWTIssuer("https://issuer.example.com", "onvif-sim", "https://issuer.example.com/.well-known/jwks.json"); err != nil {
		t.Fatalf("SetJWTIssuer: %v", err)
	}
	got := loadOrFail(t)
	if got.Auth.JWT.Issuer != "https://issuer.example.com" ||
		got.Auth.JWT.Audience != "onvif-sim" ||
		got.Auth.JWT.JWKSURL != "https://issuer.example.com/.well-known/jwks.json" {
		t.Fatalf("jwt issuer not persisted: %+v", got.Auth.JWT)
	}
}

func TestSetDigestAlgorithms(t *testing.T) {
	seed(t)

	if err := config.SetDigestAlgorithms([]string{"MD5", "SHA-256"}); err != nil {
		t.Fatalf("SetDigestAlgorithms: %v", err)
	}
	got := loadOrFail(t)
	if len(got.Auth.Digest.Algorithms) != 2 {
		t.Fatalf("algorithms not persisted: %+v", got.Auth.Digest.Algorithms)
	}
}

func TestAddProfile(t *testing.T) {
	seed(t)

	p := config.ProfileConfig{
		Name: "sub", Token: "profile_sub",
		RTSP: "rtsp://127.0.0.1:8554/sub", Encoding: "H264",
		Width: 640, Height: 480, FPS: 15,
	}
	if err := config.AddProfile(p); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if err := config.AddProfile(p); !errors.Is(err, config.ErrProfileAlreadyExists) {
		t.Fatalf("expected ErrProfileAlreadyExists, got %v", err)
	}
	got := loadOrFail(t)
	if len(got.Media.Profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(got.Media.Profiles))
	}
}

func TestRemoveProfile(t *testing.T) {
	seed(t)

	sub := config.ProfileConfig{
		Name: "sub", Token: "profile_sub",
		RTSP: "rtsp://127.0.0.1:8554/sub", Encoding: "H264",
		Width: 640, Height: 480, FPS: 15,
	}
	if err := config.AddProfile(sub); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	if err := config.RemoveProfile("profile_main"); err != nil {
		t.Fatalf("RemoveProfile: %v", err)
	}
	if err := config.RemoveProfile("ghost"); !errors.Is(err, config.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
	got := loadOrFail(t)
	if len(got.Media.Profiles) != 1 || got.Media.Profiles[0].Token != "profile_sub" {
		t.Fatalf("unexpected profiles after remove: %+v", got.Media.Profiles)
	}
}

func TestRemoveProfileKeepsAtLeastOne(t *testing.T) {
	seed(t)

	err := config.RemoveProfile("profile_main")
	if err == nil {
		t.Fatal("removing last profile must be rejected by validation")
	}
	if !errors.Is(err, config.ErrMediaNoProfiles) {
		t.Fatalf("expected ErrMediaNoProfiles, got %v", err)
	}
}

func TestSetProfileRTSP(t *testing.T) {
	seed(t)

	if err := config.SetProfileRTSP("profile_main", "rtsp://10.0.0.1:8554/live"); err != nil {
		t.Fatalf("SetProfileRTSP: %v", err)
	}
	got := loadOrFail(t)
	if got.Media.Profiles[0].RTSP != "rtsp://10.0.0.1:8554/live" {
		t.Fatalf("rtsp not persisted: %q", got.Media.Profiles[0].RTSP)
	}
	if err := config.SetProfileRTSP("ghost", "rtsp://x/y"); !errors.Is(err, config.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestSetProfileSnapshotURI(t *testing.T) {
	seed(t)

	if err := config.SetProfileSnapshotURI("profile_main", "http://host/snap.jpg"); err != nil {
		t.Fatalf("SetProfileSnapshotURI: %v", err)
	}
	got := loadOrFail(t)
	if got.Media.Profiles[0].SnapshotURI != "http://host/snap.jpg" {
		t.Fatalf("snapshot uri not persisted: %q", got.Media.Profiles[0].SnapshotURI)
	}
	// Clearing is allowed.
	if err := config.SetProfileSnapshotURI("profile_main", ""); err != nil {
		t.Fatalf("clear SnapshotURI: %v", err)
	}
}

func TestSetProfileEncoder(t *testing.T) {
	seed(t)

	if err := config.SetProfileEncoder("profile_main", "H265", 1280, 720, 25, 2048, 50); err != nil {
		t.Fatalf("SetProfileEncoder: %v", err)
	}
	got := loadOrFail(t).Media.Profiles[0]
	if got.Encoding != "H265" || got.Width != 1280 || got.Height != 720 || got.FPS != 25 ||
		got.Bitrate != 2048 || got.GOPLength != 50 {
		t.Fatalf("encoder not persisted: %+v", got)
	}
}

func TestSetProfileVideoSourceToken(t *testing.T) {
	seed(t)

	if err := config.SetProfileVideoSourceToken("profile_main", "VS_ALT"); err != nil {
		t.Fatalf("SetProfileVideoSourceToken: %v", err)
	}
	got := loadOrFail(t)
	if got.Media.Profiles[0].VideoSourceToken != "VS_ALT" {
		t.Fatalf("video source token not persisted: %q", got.Media.Profiles[0].VideoSourceToken)
	}
	if err := config.SetProfileVideoSourceToken("ghost", "VS_X"); !errors.Is(err, config.ErrProfileNotFound) {
		t.Fatalf("expected ErrProfileNotFound, got %v", err)
	}
}

func TestAddProfileRejectsInvalid(t *testing.T) {
	seed(t)

	// Missing name/token + invalid encoding → Update rolls back, validation error surfaced.
	err := config.AddProfile(config.ProfileConfig{Token: "bad"})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	got := loadOrFail(t)
	if len(got.Media.Profiles) != 1 {
		t.Fatalf("disk mutated on invalid AddProfile: %d profiles", len(got.Media.Profiles))
	}
}

func TestUpdateNilMutate(t *testing.T) {
	seed(t)

	if err := config.Update(nil); !errors.Is(err, config.ErrMutateRequired) {
		t.Fatalf("expected ErrMutateRequired, got %v", err)
	}
}

func TestSetDiscoveryMode(t *testing.T) {
	seed(t)

	if err := config.SetDiscoveryMode("NonDiscoverable"); err != nil {
		t.Fatalf("SetDiscoveryMode: %v", err)
	}
	got := loadOrFail(t)
	if got.Runtime.DiscoveryMode != "NonDiscoverable" {
		t.Fatalf("discovery mode not persisted: %q", got.Runtime.DiscoveryMode)
	}

	if err := config.SetDiscoveryMode("Discoverable"); err != nil {
		t.Fatalf("SetDiscoveryMode Discoverable: %v", err)
	}
	if err := config.SetDiscoveryMode("InvalidMode"); !errors.Is(err, config.ErrDiscoveryModeInvalid) {
		t.Fatalf("expected ErrDiscoveryModeInvalid, got %v", err)
	}
}

func TestSetHostname(t *testing.T) {
	seed(t)

	if err := config.SetHostname("mycamera"); err != nil {
		t.Fatalf("SetHostname: %v", err)
	}
	got := loadOrFail(t)
	if got.Runtime.Hostname != "mycamera" {
		t.Fatalf("hostname not persisted: %q", got.Runtime.Hostname)
	}
}

func TestSetDNS(t *testing.T) {
	seed(t)

	dns := config.DNSConfig{
		FromDHCP:     false,
		SearchDomain: []string{"example.com"},
		DNSManual:    []string{"8.8.8.8", "8.8.4.4"},
	}
	if err := config.SetDNS(dns); err != nil {
		t.Fatalf("SetDNS: %v", err)
	}
	got := loadOrFail(t)
	if got.Runtime.DNS.DNSManual[0] != "8.8.8.8" {
		t.Fatalf("dns not persisted: %+v", got.Runtime.DNS)
	}
}

func TestSetDefaultGateway(t *testing.T) {
	seed(t)

	gw := config.DefaultGatewayConfig{
		IPv4Address: []string{"192.168.1.1"},
	}
	if err := config.SetDefaultGateway(gw); err != nil {
		t.Fatalf("SetDefaultGateway: %v", err)
	}
	got := loadOrFail(t)
	if len(got.Runtime.DefaultGateway.IPv4Address) != 1 || got.Runtime.DefaultGateway.IPv4Address[0] != "192.168.1.1" {
		t.Fatalf("default gateway not persisted: %+v", got.Runtime.DefaultGateway)
	}
}

func TestSetNetworkProtocols(t *testing.T) {
	seed(t)

	protocols := []config.NetworkProtocol{
		{Name: "HTTP", Enabled: true, Port: []int{80}},
		{Name: "RTSP", Enabled: true, Port: []int{554}},
	}
	if err := config.SetNetworkProtocols(protocols); err != nil {
		t.Fatalf("SetNetworkProtocols: %v", err)
	}
	got := loadOrFail(t)
	if len(got.Runtime.NetworkProtocols) != 2 {
		t.Fatalf("protocols not persisted: %+v", got.Runtime.NetworkProtocols)
	}

	invalid := []config.NetworkProtocol{{Name: "", Enabled: true}}
	if err := config.SetNetworkProtocols(invalid); !errors.Is(err, config.ErrNetworkProtocolNameEmpty) {
		t.Fatalf("expected ErrNetworkProtocolNameEmpty, got %v", err)
	}
}

func TestSetSystemDateAndTime(t *testing.T) {
	seed(t)

	cfg := config.SystemDateTimeConfig{
		DateTimeType:    "Manual",
		DaylightSavings: true,
		TZ:              "KST-9",
	}
	if err := config.SetSystemDateAndTime(cfg); err != nil {
		t.Fatalf("SetSystemDateAndTime: %v", err)
	}
	got := loadOrFail(t)
	if got.Runtime.SystemDateAndTime.TZ != "KST-9" || !got.Runtime.SystemDateAndTime.DaylightSavings {
		t.Fatalf("system date/time not persisted: %+v", got.Runtime.SystemDateAndTime)
	}
}

func TestSetEventsTopics(t *testing.T) {
	seed(t)

	topics := []config.TopicConfig{
		{Name: "tns1:VideoSource/MotionAlarm", Enabled: true},
		{Name: "tns1:Device/Trigger/DigitalInput", Enabled: false},
	}
	if err := config.SetEventsTopics(topics); err != nil {
		t.Fatalf("SetEventsTopics: %v", err)
	}
	got := loadOrFail(t)
	if len(got.Events.Topics) != 2 {
		t.Fatalf("topics not persisted: %+v", got.Events.Topics)
	}
	if !got.Events.Topics[0].Enabled {
		t.Fatalf("first topic should be enabled: %+v", got.Events.Topics[0])
	}
}

func TestSetEventsTopicsRejectsInvalid(t *testing.T) {
	seed(t)

	invalid := []config.TopicConfig{
		{Name: "", Enabled: true},
	}
	if err := config.SetEventsTopics(invalid); !errors.Is(err, config.ErrEventsTopicNameEmpty) {
		t.Fatalf("expected ErrEventsTopicNameEmpty, got %v", err)
	}
}

func TestSetTopicEnabled(t *testing.T) {
	seed(t)

	topics := []config.TopicConfig{
		{Name: "tns1:VideoSource/MotionAlarm", Enabled: false},
	}
	if err := config.SetEventsTopics(topics); err != nil {
		t.Fatalf("SetEventsTopics: %v", err)
	}

	if err := config.SetTopicEnabled("tns1:VideoSource/MotionAlarm", true); err != nil {
		t.Fatalf("SetTopicEnabled: %v", err)
	}
	got := loadOrFail(t)
	if !got.Events.Topics[0].Enabled {
		t.Fatalf("topic not enabled after SetTopicEnabled: %+v", got.Events.Topics[0])
	}

	if err := config.SetTopicEnabled("tns1:NoSuchTopic", true); !errors.Is(err, config.ErrTopicNotFound) {
		t.Fatalf("expected ErrTopicNotFound, got %v", err)
	}
}

func TestSetNetworkInterfaces(t *testing.T) {
	seed(t)

	ifaces := []config.NetworkInterfaceConfig{
		{
			Token:   "eth0",
			Enabled: true,
			IPv4: &config.NetworkInterfaceIPv4{
				Enabled: true,
				DHCP:    false,
				Manual:  []string{"192.168.1.100/24"},
			},
		},
	}
	if err := config.SetNetworkInterfaces(ifaces); err != nil {
		t.Fatalf("SetNetworkInterfaces: %v", err)
	}
	got := loadOrFail(t)
	if len(got.Runtime.NetworkInterfaces) != 1 {
		t.Fatalf("network interfaces not persisted: %+v", got.Runtime.NetworkInterfaces)
	}
	if got.Runtime.NetworkInterfaces[0].Token != "eth0" {
		t.Fatalf("wrong token: %q", got.Runtime.NetworkInterfaces[0].Token)
	}
	if len(got.Runtime.NetworkInterfaces[0].IPv4.Manual) == 0 ||
		got.Runtime.NetworkInterfaces[0].IPv4.Manual[0] != "192.168.1.100/24" {
		t.Fatalf("wrong address: %+v", got.Runtime.NetworkInterfaces[0].IPv4)
	}
}

func TestAddRemoveUpsertMetadataConfig(t *testing.T) {
	seed(t)

	mc := config.MetadataConfig{
		Token:     "meta0",
		PTZStatus: true,
		Analytics: false,
	}

	// AddMetadataConfig happy path
	if err := config.AddMetadataConfig(mc); err != nil {
		t.Fatalf("AddMetadataConfig: %v", err)
	}
	got := loadOrFail(t)
	if len(got.Media.MetadataConfigurations) != 1 {
		t.Fatalf("metadata config not persisted: %+v", got.Media.MetadataConfigurations)
	}

	// AddMetadataConfig duplicate
	if err := config.AddMetadataConfig(mc); !errors.Is(err, config.ErrMetadataAlreadyExists) {
		t.Fatalf("expected ErrMetadataAlreadyExists, got %v", err)
	}

	// UpsertMetadataConfig updates existing
	mc.PTZStatus = false
	if err := config.UpsertMetadataConfig(mc); err != nil {
		t.Fatalf("UpsertMetadataConfig update: %v", err)
	}
	got = loadOrFail(t)
	if got.Media.MetadataConfigurations[0].PTZStatus {
		t.Fatalf("PTZStatus not updated by upsert")
	}

	// UpsertMetadataConfig inserts new
	mc2 := config.MetadataConfig{Token: "meta1", Name: "Meta 1"}
	if err := config.UpsertMetadataConfig(mc2); err != nil {
		t.Fatalf("UpsertMetadataConfig insert: %v", err)
	}
	got = loadOrFail(t)
	if len(got.Media.MetadataConfigurations) != 2 {
		t.Fatalf("upsert did not insert: %+v", got.Media.MetadataConfigurations)
	}

	// RemoveMetadataConfig happy path
	if err := config.RemoveMetadataConfig("meta0"); err != nil {
		t.Fatalf("RemoveMetadataConfig: %v", err)
	}
	got = loadOrFail(t)
	if len(got.Media.MetadataConfigurations) != 1 {
		t.Fatalf("remove did not delete: %+v", got.Media.MetadataConfigurations)
	}

	// RemoveMetadataConfig not found
	if err := config.RemoveMetadataConfig("noSuch"); !errors.Is(err, config.ErrMetadataNotFound) {
		t.Fatalf("expected ErrMetadataNotFound, got %v", err)
	}
}
