package config

import (
	"errors"
	"fmt"
	"sync"
)

// updateMu serializes concurrent Update/Save callers within this process.
// On-disk atomicity is handled by Save via temp-file rename; the mutex
// prevents two in-flight Load→mutate→Save sequences from racing.
var updateMu sync.Mutex

// ErrUserNotFound is returned by RemoveUser when the username does not exist.
var ErrUserNotFound = errors.New("config: user not found")

// ErrUserAlreadyExists is returned by AddUser when the username is already taken.
var ErrUserAlreadyExists = errors.New("config: user already exists")

// ErrMutateRequired is returned by Update when the mutate callback is nil.
var ErrMutateRequired = errors.New("config: Update requires a mutate function")

// Update loads the on-disk config, applies mutate in memory, validates, and
// saves atomically. Concurrent Update calls are serialized.
func Update(mutate func(*Config)) error {
	if mutate == nil {
		return ErrMutateRequired
	}
	updateMu.Lock()
	defer updateMu.Unlock()

	cfg, err := Load()
	if err != nil {
		return err
	}
	mutate(&cfg)
	return Save(&cfg)
}

// SetAuthEnabled toggles the Enabled flag and persists.
func SetAuthEnabled(enabled bool) error {
	return Update(func(c *Config) {
		c.Auth.Enabled = enabled
	})
}

// AddUser appends a new user. Returns ErrUserAlreadyExists if the username is taken.
func AddUser(u UserConfig) error {
	var duplicate bool
	if err := Update(func(c *Config) {
		for _, existing := range c.Auth.Users {
			if existing.Username == u.Username {
				duplicate = true
				return
			}
		}
		c.Auth.Users = append(c.Auth.Users, u)
	}); err != nil {
		return err
	}
	if duplicate {
		return fmt.Errorf("%w: %s", ErrUserAlreadyExists, u.Username)
	}
	return nil
}

// UpsertUser inserts or replaces the user with the matching username.
func UpsertUser(u UserConfig) error {
	return Update(func(c *Config) {
		for i, existing := range c.Auth.Users {
			if existing.Username == u.Username {
				c.Auth.Users[i] = u
				return
			}
		}
		c.Auth.Users = append(c.Auth.Users, u)
	})
}

// RemoveUser deletes the user with the given username.
// Returns ErrUserNotFound if no such user exists.
func RemoveUser(username string) error {
	var removed bool
	if err := Update(func(c *Config) {
		filtered := c.Auth.Users[:0]
		for _, u := range c.Auth.Users {
			if u.Username == username {
				removed = true
				continue
			}
			filtered = append(filtered, u)
		}
		c.Auth.Users = filtered
	}); err != nil {
		return err
	}
	if !removed {
		return fmt.Errorf("%w: %s", ErrUserNotFound, username)
	}
	return nil
}

// SetDigestAlgorithms replaces the digest algorithm list and persists.
func SetDigestAlgorithms(algorithms []string) error {
	return Update(func(c *Config) {
		c.Auth.Digest.Algorithms = append([]string(nil), algorithms...)
	})
}

// SetJWTIssuer replaces the JWT issuer, audience and JWKS URL and persists.
// An empty issuer clears the field; callers wanting to disable JWT should
// set Auth.JWT.Enabled=false via Update directly.
func SetJWTIssuer(issuer, audience, jwksURL string) error {
	return Update(func(c *Config) {
		c.Auth.JWT.Issuer = issuer
		c.Auth.JWT.Audience = audience
		c.Auth.JWT.JWKSURL = jwksURL
	})
}
