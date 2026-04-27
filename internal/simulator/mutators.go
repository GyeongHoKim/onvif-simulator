package simulator

import (
	"fmt"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

// SetDiscoveryMode persists the WS-Discovery mode and applies it live.
func (s *Simulator) SetDiscoveryMode(mode string) error {
	if err := config.SetDiscoveryMode(mode); err != nil {
		return err
	}
	if err := s.reloadFromDisk(); err != nil {
		return err
	}
	s.recordMutation("SetDiscoveryMode", "", mode)
	return nil
}

// SetHostname persists the device hostname.
func (s *Simulator) SetHostname(name string) error {
	if err := config.SetHostname(name); err != nil {
		return err
	}
	if err := s.reloadFromDisk(); err != nil {
		return err
	}
	s.recordMutation("SetHostname", "", name)
	return nil
}

// AddProfile appends a new media profile.
//
//nolint:gocritic // ProfileConfig mirrors config.ProfileConfig (value-typed DTO).
func (s *Simulator) AddProfile(p config.ProfileConfig) error {
	if err := config.AddProfile(p); err != nil {
		return err
	}
	if err := s.reloadFromDisk(); err != nil {
		return err
	}
	s.recordMutation("AddProfile", p.Token, p.Name)
	return nil
}

// RemoveProfile deletes the profile with the given token.
func (s *Simulator) RemoveProfile(token string) error {
	if err := config.RemoveProfile(token); err != nil {
		return err
	}
	if err := s.reloadFromDisk(); err != nil {
		return err
	}
	s.recordMutation("RemoveProfile", token, "")
	return nil
}

// SetProfileMediaFilePath updates the local mp4 path that the embedded RTSP
// server loops for this profile. The change applies to the persisted config
// immediately; the embedded RTSP server picks up the new path on the next
// Stop/Start cycle.
func (s *Simulator) SetProfileMediaFilePath(token, path string) error {
	if err := config.SetProfileMediaFilePath(token, path); err != nil {
		return err
	}
	if err := s.reloadFromDisk(); err != nil {
		return err
	}
	s.recordMutation("SetProfileMediaFilePath", token, path)
	return nil
}

// SetProfileSnapshotURI replaces the snapshot pass-through URI of a profile.
func (s *Simulator) SetProfileSnapshotURI(token, uri string) error {
	if err := config.SetProfileSnapshotURI(token, uri); err != nil {
		return err
	}
	if err := s.reloadFromDisk(); err != nil {
		return err
	}
	s.recordMutation("SetProfileSnapshotURI", token, uri)
	return nil
}

// SetTopicEnabled toggles the Enabled flag of one event topic.
func (s *Simulator) SetTopicEnabled(name string, enabled bool) error {
	if err := config.SetTopicEnabled(name, enabled); err != nil {
		return err
	}
	if err := s.reloadFromDisk(); err != nil {
		return err
	}
	s.recordMutation("SetTopicEnabled", name, fmt.Sprintf("enabled=%t", enabled))
	return nil
}

// SetEventsTopics replaces the full topic list.
func (s *Simulator) SetEventsTopics(topics []config.TopicConfig) error {
	if err := config.SetEventsTopics(topics); err != nil {
		return err
	}
	if err := s.reloadFromDisk(); err != nil {
		return err
	}
	s.recordMutation("SetEventsTopics", "", fmt.Sprintf("count=%d", len(topics)))
	return nil
}

// AddUser persists a new user and mirrors it into the live user store.
func (s *Simulator) AddUser(u config.UserConfig) error {
	if err := config.AddUser(u); err != nil {
		return err
	}
	if err := s.reloadFromDisk(); err != nil {
		return err
	}
	s.recordMutation("AddUser", u.Username, u.Role)
	return nil
}

// UpsertUser persists an insert-or-replace.
func (s *Simulator) UpsertUser(u config.UserConfig) error {
	if err := s.controller.UpsertUser(u); err != nil {
		return err
	}
	if err := s.reloadFromDisk(); err != nil {
		return err
	}
	s.recordMutation("UpsertUser", u.Username, u.Role)
	return nil
}

// RemoveUser deletes the user from config and the live store.
func (s *Simulator) RemoveUser(username string) error {
	if err := s.controller.RemoveUser(username); err != nil {
		return err
	}
	if err := s.reloadFromDisk(); err != nil {
		return err
	}
	s.recordMutation("RemoveUser", username, "")
	return nil
}

// SetAuthEnabled toggles the AuthConfig.Enabled flag.
func (s *Simulator) SetAuthEnabled(enabled bool) error {
	if err := s.controller.SetAuthEnabled(enabled); err != nil {
		return err
	}
	if err := s.reloadFromDisk(); err != nil {
		return err
	}
	s.recordMutation("SetAuthEnabled", "", fmt.Sprintf("enabled=%t", enabled))
	return nil
}

// ConfigSnapshot returns a deep copy of the loaded config.
func (s *Simulator) ConfigSnapshot() config.Config {
	return s.snapshotConfig()
}

// Users returns a sorted, password-free snapshot of the auth store.
func (s *Simulator) Users() []UserView {
	records := s.store.Snapshot()
	out := make([]UserView, 0, len(records))
	for _, r := range records {
		out = append(out, UserView{
			Username: r.Username,
			Roles:    append([]string(nil), r.Roles...),
		})
	}
	return out
}
