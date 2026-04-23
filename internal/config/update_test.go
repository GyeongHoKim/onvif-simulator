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

	err := config.Update(func(c *config.Config) {
		c.Device.Firmware = "9.9.9"
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

	err := config.Update(func(c *config.Config) {
		c.Network.HTTPPort = 0
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
