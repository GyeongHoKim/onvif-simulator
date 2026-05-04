package auth

import (
	"log/slog"
	"sync/atomic"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
	"github.com/GyeongHoKim/onvif-simulator/internal/obs"
)

// Controller bridges the persistent config.AuthConfig and the live
// MutableUserStore that the auth layer consults on every request.
//
// GUI and TUI code should call these methods so disk persistence and the
// in-memory store stay in sync:
//
//	ctrl.UpsertUser(config.UserConfig{Username: ..., Password: ..., Role: ...})
type Controller struct {
	store  *MutableUserStore
	logger atomic.Pointer[slog.Logger]
}

// NewController returns a Controller backed by store.
func NewController(store *MutableUserStore) *Controller {
	if store == nil {
		panic("auth: NewController requires a non-nil store")
	}
	c := &Controller{store: store}
	discard := obs.Discard()
	c.logger.Store(discard)
	return c
}

// SetLogger installs a structured logger. Nil falls back to discard. Safe
// for concurrent use; the simulator may swap loggers on config reload.
func (c *Controller) SetLogger(logger *slog.Logger) {
	if logger == nil {
		logger = obs.Discard()
	}
	c.logger.Store(logger)
}

// Logger returns the currently installed logger.
func (c *Controller) Logger() *slog.Logger {
	return c.logger.Load()
}

// Store exposes the underlying MutableUserStore (for wiring into an Authenticator).
func (c *Controller) Store() *MutableUserStore { return c.store }

// SyncFromConfig replaces the live user set with the users currently in cfg.
// Call this after loading config at startup, and again after any
// config.Update/Save sequence that did not go through the controller.
func (c *Controller) SyncFromConfig(cfg *config.Config) {
	if cfg == nil {
		c.store.Replace(nil)
		c.Logger().Debug("auth: sync from nil config, store cleared")
		return
	}
	c.store.Replace(userRecordsFromConfig(cfg.Auth.Users))
	c.Logger().Debug("auth: sync from config",
		"users", len(cfg.Auth.Users),
		"enabled", cfg.Auth.Enabled,
	)
}

// UpsertUser persists u to disk and then mirrors it into the live store.
// On disk validation failure the live store is left untouched.
func (c *Controller) UpsertUser(u config.UserConfig) error {
	if err := config.UpsertUser(u); err != nil {
		c.Logger().Warn("auth: upsert user failed", "username", u.Username, "err", err)
		return err
	}
	c.store.Set(recordFromConfig(u))
	c.Logger().Info("auth: user upserted", "username", u.Username, "role", u.Role)
	return nil
}

// RemoveUser deletes the user from disk and from the live store.
func (c *Controller) RemoveUser(username string) error {
	if err := config.RemoveUser(username); err != nil {
		c.Logger().Warn("auth: remove user failed", "username", username, "err", err)
		return err
	}
	c.store.Remove(username)
	c.Logger().Info("auth: user removed", "username", username)
	return nil
}

// SetAuthEnabled toggles the AuthConfig.Enabled flag and persists.
// The live store is untouched; disable is enforced upstream by wiring
// (or by leaving the Authenticator out of the handler).
func (c *Controller) SetAuthEnabled(enabled bool) error {
	if err := config.SetAuthEnabled(enabled); err != nil {
		c.Logger().Warn("auth: toggle enabled failed", "enabled", enabled, "err", err)
		return err
	}
	c.Logger().Info("auth: enabled toggled", "enabled", enabled)
	return nil
}

func userRecordsFromConfig(users []config.UserConfig) []UserRecord {
	out := make([]UserRecord, 0, len(users))
	for _, u := range users {
		out = append(out, recordFromConfig(u))
	}
	return out
}

func recordFromConfig(u config.UserConfig) UserRecord {
	return UserRecord{
		Username: u.Username,
		Password: u.Password,
		Roles:    []string{roleToOnvif(u.Role)},
	}
}

// roleToOnvif normalises a config role name (Administrator / Operator / User
// / Extended / custom) into the onvif: prefixed identifier used by the
// default policy.
func roleToOnvif(role string) string {
	switch role {
	case config.RoleAdministrator:
		return OnvifRoleAdministrator
	case config.RoleOperator:
		return OnvifRoleOperator
	case config.RoleUser:
		return OnvifRoleUser
	default:
		return role
	}
}
