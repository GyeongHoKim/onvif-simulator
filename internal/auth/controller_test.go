package auth_test

import (
	"context"
	"errors"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

var baseConfig = config.Config{
	Version: config.CurrentVersion,
	Device: config.DeviceConfig{
		UUID:         "urn:uuid:11111111-2222-4333-8444-555555555555",
		Manufacturer: "Acme",
		Model:        "SimCam-100",
		Serial:       "SN-001",
	},
	Network: config.NetworkConfig{HTTPPort: 8080},
	Media: config.MediaConfig{
		Profiles: []config.ProfileConfig{{
			Name: "main", Token: "profile_main",
			RTSP: "rtsp://127.0.0.1:8554/main", Encoding: "H264",
			Width: 1920, Height: 1080, FPS: 30,
		}},
	},
}

func seedControllerTest(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	if err := config.Save(&baseConfig); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestControllerUpsertMirrorsStore(t *testing.T) {
	seedControllerTest(t)
	store := auth.NewMutableUserStore(nil)
	ctrl := auth.NewController(store)

	u := config.UserConfig{Username: "admin", Password: "pw", Role: config.RoleAdministrator}
	if err := ctrl.UpsertUser(u); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if err := ctrl.SetAuthEnabled(true); err != nil {
		t.Fatalf("SetAuthEnabled: %v", err)
	}
	got, err := store.Lookup(context.Background(), "admin")
	if err != nil {
		t.Fatalf("store lookup: %v", err)
	}
	if got.Password != "pw" || len(got.Roles) != 1 || got.Roles[0] != auth.OnvifRoleAdministrator {
		t.Fatalf("store record: %+v", got)
	}
	// Disk persistence.
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Auth.Users) != 1 || cfg.Auth.Users[0].Username != "admin" {
		t.Fatalf("disk users: %+v", cfg.Auth.Users)
	}
}

func TestControllerRemoveClearsBoth(t *testing.T) {
	seedControllerTest(t)
	store := auth.NewMutableUserStore(nil)
	ctrl := auth.NewController(store)

	if err := ctrl.UpsertUser(config.UserConfig{Username: "a", Password: "p", Role: config.RoleUser}); err != nil {
		t.Fatalf("seed upsert: %v", err)
	}

	if err := ctrl.RemoveUser("a"); err != nil {
		t.Fatalf("RemoveUser: %v", err)
	}
	if _, err := store.Lookup(context.Background(), "a"); !errors.Is(err, auth.ErrUnknownUser) {
		t.Fatalf("expected removed from store, got %v", err)
	}
}

func TestControllerSyncFromConfig(t *testing.T) {
	seedControllerTest(t)
	store := auth.NewMutableUserStore(nil)
	ctrl := auth.NewController(store)

	cfg := baseConfig
	cfg.Auth = config.AuthConfig{
		Enabled: true,
		Users: []config.UserConfig{
			{Username: "admin", Password: "x", Role: config.RoleAdministrator},
			{Username: "viewer", Password: "y", Role: config.RoleUser},
		},
	}
	ctrl.SyncFromConfig(&cfg)

	snap := store.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("store snapshot len: %d", len(snap))
	}
	if snap[0].Roles[0] != auth.OnvifRoleAdministrator {
		t.Fatalf("admin role: %+v", snap[0].Roles)
	}
	if snap[1].Roles[0] != auth.OnvifRoleUser {
		t.Fatalf("user role: %+v", snap[1].Roles)
	}
}
