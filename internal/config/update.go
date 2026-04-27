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

// ErrProfileNotFound is returned when no profile has the given token.
var ErrProfileNotFound = errors.New("config: profile not found")

// ErrProfileAlreadyExists is returned by AddProfile when the token is taken.
var ErrProfileAlreadyExists = errors.New("config: profile already exists")

// ErrTopicNotFound is returned by SetTopicEnabled when the topic name does not exist.
var ErrTopicNotFound = errors.New("config: topic not found")

// ErrMetadataNotFound is returned when no metadata configuration has the given token.
var ErrMetadataNotFound = errors.New("config: metadata configuration not found")

// ErrMetadataAlreadyExists is returned by AddMetadataConfig when the token is taken.
var ErrMetadataAlreadyExists = errors.New("config: metadata configuration already exists")

// Update loads the on-disk config, applies mutate in memory, validates, and
// saves atomically. Concurrent Update calls are serialized. If mutate
// returns a non-nil error, Save is skipped and the error is returned
// verbatim — this lets callers signal "no-op, abort" without paying for a
// redundant disk write.
func Update(mutate func(*Config) error) error {
	if mutate == nil {
		return ErrMutateRequired
	}
	updateMu.Lock()
	defer updateMu.Unlock()

	cfg, err := Load()
	if err != nil {
		return err
	}
	if err := mutate(&cfg); err != nil {
		return err
	}
	return Save(&cfg)
}

// SetAuthEnabled toggles the Enabled flag and persists.
func SetAuthEnabled(enabled bool) error {
	return Update(func(c *Config) error {
		c.Auth.Enabled = enabled
		return nil
	})
}

// AddUser appends a new user. Returns ErrUserAlreadyExists if the username is taken.
func AddUser(u UserConfig) error {
	return Update(func(c *Config) error {
		for _, existing := range c.Auth.Users {
			if existing.Username == u.Username {
				return fmt.Errorf("%w: %s", ErrUserAlreadyExists, u.Username)
			}
		}
		c.Auth.Users = append(c.Auth.Users, u)
		return nil
	})
}

// UpsertUser inserts or replaces the user with the matching username.
func UpsertUser(u UserConfig) error {
	return Update(func(c *Config) error {
		for i, existing := range c.Auth.Users {
			if existing.Username == u.Username {
				c.Auth.Users[i] = u
				return nil
			}
		}
		c.Auth.Users = append(c.Auth.Users, u)
		return nil
	})
}

// RemoveUser deletes the user with the given username.
// Returns ErrUserNotFound if no such user exists.
func RemoveUser(username string) error {
	return Update(func(c *Config) error {
		filtered := c.Auth.Users[:0]
		removed := false
		for _, u := range c.Auth.Users {
			if u.Username == username {
				removed = true
				continue
			}
			filtered = append(filtered, u)
		}
		if !removed {
			return fmt.Errorf("%w: %s", ErrUserNotFound, username)
		}
		c.Auth.Users = filtered
		return nil
	})
}

// SetDigestAlgorithms replaces the digest algorithm list and persists.
func SetDigestAlgorithms(algorithms []string) error {
	return Update(func(c *Config) error {
		c.Auth.Digest.Algorithms = append([]string(nil), algorithms...)
		return nil
	})
}

// SetJWTIssuer replaces the JWT issuer, audience and JWKS URL and persists.
// An empty issuer clears the field; callers wanting to disable JWT should
// set Auth.JWT.Enabled=false via Update directly.
func SetJWTIssuer(issuer, audience, jwksURL string) error {
	return Update(func(c *Config) error {
		c.Auth.JWT.Issuer = issuer
		c.Auth.JWT.Audience = audience
		c.Auth.JWT.JWKSURL = jwksURL
		return nil
	})
}

// AddProfile appends a new media profile.
// Returns ErrProfileAlreadyExists if the token is already taken.
//
//nolint:gocritic // ProfileConfig is a value-typed DTO; callers pass it by value
func AddProfile(p ProfileConfig) error {
	return Update(func(c *Config) error {
		for i := range c.Media.Profiles {
			if c.Media.Profiles[i].Token == p.Token {
				return fmt.Errorf("%w: %s", ErrProfileAlreadyExists, p.Token)
			}
		}
		c.Media.Profiles = append(c.Media.Profiles, p)
		return nil
	})
}

// RemoveProfile deletes the profile with the given token.
// Returns ErrProfileNotFound if no such profile exists.
func RemoveProfile(token string) error {
	return Update(func(c *Config) error {
		filtered := c.Media.Profiles[:0]
		removed := false
		for i := range c.Media.Profiles {
			if c.Media.Profiles[i].Token == token {
				removed = true
				continue
			}
			filtered = append(filtered, c.Media.Profiles[i])
		}
		if !removed {
			return fmt.Errorf("%w: %s", ErrProfileNotFound, token)
		}
		c.Media.Profiles = filtered
		return nil
	})
}

// SetProfileMediaFilePath points the named profile at a local mp4 file. The
// embedded RTSP server reads and loops this file when the simulator starts.
func SetProfileMediaFilePath(token, path string) error {
	return mutateProfile(token, func(p *ProfileConfig) { p.MediaFilePath = path })
}

// SetProfileSnapshotURI replaces the snapshot pass-through URI of one profile.
// Pass "" to clear.
func SetProfileSnapshotURI(token, uri string) error {
	return mutateProfile(token, func(p *ProfileConfig) { p.SnapshotURI = uri })
}

// SetProfileVideoSourceToken changes the video source a profile references.
// Pass "" to reset to the default.
func SetProfileVideoSourceToken(token, sourceToken string) error {
	return mutateProfile(token, func(p *ProfileConfig) { p.VideoSourceToken = sourceToken })
}

func mutateProfile(token string, mutate func(*ProfileConfig)) error {
	return Update(func(c *Config) error {
		for i := range c.Media.Profiles {
			if c.Media.Profiles[i].Token == token {
				mutate(&c.Media.Profiles[i])
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrProfileNotFound, token)
	})
}

// SetDiscoveryMode persists the discovery mode ("Discoverable" or
// "NonDiscoverable").
func SetDiscoveryMode(mode string) error {
	return Update(func(c *Config) error {
		c.Runtime.DiscoveryMode = mode
		return nil
	})
}

// SetHostname persists the device hostname.
func SetHostname(hostname string) error {
	return Update(func(c *Config) error {
		c.Runtime.Hostname = hostname
		return nil
	})
}

// SetDNS persists the DNS configuration.
func SetDNS(dns DNSConfig) error {
	return Update(func(c *Config) error {
		c.Runtime.DNS = dns
		return nil
	})
}

// SetDefaultGateway persists the default gateway configuration.
func SetDefaultGateway(gw DefaultGatewayConfig) error {
	return Update(func(c *Config) error {
		c.Runtime.DefaultGateway = gw
		return nil
	})
}

// SetNetworkProtocols replaces the full network protocol list and persists.
func SetNetworkProtocols(protocols []NetworkProtocol) error {
	return Update(func(c *Config) error {
		c.Runtime.NetworkProtocols = append([]NetworkProtocol(nil), protocols...)
		return nil
	})
}

// SetNetworkInterfaces replaces the full network interface list and persists.
// This is called by the devicesvc SetNetworkInterfaces handler.
func SetNetworkInterfaces(ifaces []NetworkInterfaceConfig) error {
	return Update(func(c *Config) error {
		c.Runtime.NetworkInterfaces = append([]NetworkInterfaceConfig(nil), ifaces...)
		return nil
	})
}

// SetSystemDateAndTime persists the system date/time configuration.
func SetSystemDateAndTime(cfg SystemDateTimeConfig) error {
	return Update(func(c *Config) error {
		c.Runtime.SystemDateAndTime = cfg
		return nil
	})
}

// AddMetadataConfig appends a metadata configuration entry.
// Returns ErrMetadataAlreadyExists if the token is already taken.
func AddMetadataConfig(m MetadataConfig) error {
	return Update(func(c *Config) error {
		for i := range c.Media.MetadataConfigurations {
			if c.Media.MetadataConfigurations[i].Token == m.Token {
				return fmt.Errorf("%w: %s", ErrMetadataAlreadyExists, m.Token)
			}
		}
		c.Media.MetadataConfigurations = append(c.Media.MetadataConfigurations, m)
		return nil
	})
}

// RemoveMetadataConfig deletes the metadata configuration with the given token.
// Returns ErrMetadataNotFound if no such entry exists.
func RemoveMetadataConfig(token string) error {
	return Update(func(c *Config) error {
		filtered := c.Media.MetadataConfigurations[:0]
		removed := false
		for i := range c.Media.MetadataConfigurations {
			if c.Media.MetadataConfigurations[i].Token == token {
				removed = true
				continue
			}
			filtered = append(filtered, c.Media.MetadataConfigurations[i])
		}
		if !removed {
			return fmt.Errorf("%w: %s", ErrMetadataNotFound, token)
		}
		c.Media.MetadataConfigurations = filtered
		return nil
	})
}

// UpsertMetadataConfig inserts or replaces the metadata configuration with the matching token.
func UpsertMetadataConfig(m MetadataConfig) error {
	return Update(func(c *Config) error {
		for i := range c.Media.MetadataConfigurations {
			if c.Media.MetadataConfigurations[i].Token == m.Token {
				c.Media.MetadataConfigurations[i] = m
				return nil
			}
		}
		c.Media.MetadataConfigurations = append(c.Media.MetadataConfigurations, m)
		return nil
	})
}

// SetEventsTopics replaces the events topic list and persists. Callers pass
// the full desired list; existing entries not present in the new list are
// dropped.
func SetEventsTopics(topics []TopicConfig) error {
	return Update(func(c *Config) error {
		c.Events.Topics = append([]TopicConfig(nil), topics...)
		return nil
	})
}

// SetTopicEnabled toggles the Enabled flag of one event topic by name.
// Returns ErrTopicNotFound if no topic with that name exists.
func SetTopicEnabled(name string, enabled bool) error {
	return Update(func(c *Config) error {
		for i := range c.Events.Topics {
			if c.Events.Topics[i].Name == name {
				c.Events.Topics[i].Enabled = enabled
				return nil
			}
		}
		return fmt.Errorf("%w: %s", ErrTopicNotFound, name)
	})
}
