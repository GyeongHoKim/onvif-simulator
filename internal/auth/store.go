package auth

import (
	"context"
	"errors"
	"sort"
	"sync"
)

// ErrUnknownUser is returned by a UserStore when no record matches the username.
var ErrUnknownUser = errors.New("auth: unknown user")

// UserRecord carries the credentials and roles needed to validate one user.
// Password is stored in plaintext because HTTP Digest (HA1 = MD5(user:realm:pwd))
// and WS-UsernameToken (SHA1(nonce+created+password)) both require it.
type UserRecord struct {
	Username string
	Password string
	Roles    []string
}

// UserStore resolves credentials by username. Implementations MUST be
// safe for concurrent use.
type UserStore interface {
	Lookup(ctx context.Context, username string) (*UserRecord, error)
}

// MutableUserStore is an in-memory, thread-safe UserStore that GUI and TUI
// code can mutate at runtime.
type MutableUserStore struct {
	mu      sync.RWMutex
	records map[string]UserRecord
}

// NewMutableUserStore returns a store pre-populated with initial records.
// Duplicate usernames in initial silently overwrite earlier entries.
func NewMutableUserStore(initial []UserRecord) *MutableUserStore {
	s := &MutableUserStore{records: make(map[string]UserRecord, len(initial))}
	for _, r := range initial {
		s.records[r.Username] = cloneRecord(r)
	}
	return s
}

// Lookup returns a copy of the record for username or ErrUnknownUser.
func (s *MutableUserStore) Lookup(_ context.Context, username string) (*UserRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.records[username]
	if !ok {
		return nil, ErrUnknownUser
	}
	clone := cloneRecord(r)
	return &clone, nil
}

// Set upserts a record.
func (s *MutableUserStore) Set(r UserRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[r.Username] = cloneRecord(r)
}

// Remove deletes a record. No error if the username is absent.
func (s *MutableUserStore) Remove(username string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.records, username)
}

// Replace atomically swaps the entire record set.
func (s *MutableUserStore) Replace(records []UserRecord) {
	next := make(map[string]UserRecord, len(records))
	for _, r := range records {
		next[r.Username] = cloneRecord(r)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = next
}

// Snapshot returns a sorted, deep-copied view of the current records.
func (s *MutableUserStore) Snapshot() []UserRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]UserRecord, 0, len(s.records))
	for _, r := range s.records {
		out = append(out, cloneRecord(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Username < out[j].Username })
	return out
}

func cloneRecord(r UserRecord) UserRecord {
	if r.Roles == nil {
		return UserRecord{Username: r.Username, Password: r.Password}
	}
	roles := make([]string, len(r.Roles))
	copy(roles, r.Roles)
	return UserRecord{Username: r.Username, Password: r.Password, Roles: roles}
}
