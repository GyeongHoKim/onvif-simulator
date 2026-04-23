package auth_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
)

func TestMutableUserStoreLookupUnknown(t *testing.T) {
	t.Parallel()
	s := auth.NewMutableUserStore(nil)
	_, err := s.Lookup(context.Background(), "ghost")
	if !errors.Is(err, auth.ErrUnknownUser) {
		t.Fatalf("expected ErrUnknownUser, got %v", err)
	}
}

func TestMutableUserStoreSetLookup(t *testing.T) {
	t.Parallel()
	s := auth.NewMutableUserStore([]auth.UserRecord{
		{Username: "admin", Password: "pw", Roles: []string{"onvif:Administrator"}},
	})
	got, err := s.Lookup(context.Background(), "admin")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if got.Password != "pw" {
		t.Fatalf("password mismatch: %q", got.Password)
	}
	// Mutating the returned record must not affect the store.
	got.Password = "mutated"
	again, err := s.Lookup(context.Background(), "admin")
	if err != nil {
		t.Fatalf("second Lookup: %v", err)
	}
	if again.Password != "pw" {
		t.Fatalf("returned record should be a copy; got %q", again.Password)
	}
}

func TestMutableUserStoreReplace(t *testing.T) {
	t.Parallel()
	s := auth.NewMutableUserStore([]auth.UserRecord{{Username: "a", Password: "1"}})
	s.Replace([]auth.UserRecord{{Username: "b", Password: "2"}})
	if _, err := s.Lookup(context.Background(), "a"); !errors.Is(err, auth.ErrUnknownUser) {
		t.Fatalf("expected a to be gone, got %v", err)
	}
	if _, err := s.Lookup(context.Background(), "b"); err != nil {
		t.Fatalf("expected b to be present, got %v", err)
	}
}

func TestMutableUserStoreConcurrent(t *testing.T) {
	t.Parallel()
	s := auth.NewMutableUserStore(nil)
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			s.Set(auth.UserRecord{Username: "u", Password: "p"})
		}()
		go func() {
			defer wg.Done()
			if _, err := s.Lookup(context.Background(), "u"); err != nil && !errors.Is(err, auth.ErrUnknownUser) {
				t.Errorf("Lookup: %v", err)
			}
		}()
	}
	wg.Wait()
}

func TestMutableUserStoreSnapshotSorted(t *testing.T) {
	t.Parallel()
	s := auth.NewMutableUserStore(nil)
	s.Set(auth.UserRecord{Username: "c"})
	s.Set(auth.UserRecord{Username: "a"})
	s.Set(auth.UserRecord{Username: "b"})
	snap := s.Snapshot()
	if len(snap) != 3 || snap[0].Username != "a" || snap[1].Username != "b" || snap[2].Username != "c" {
		t.Fatalf("unexpected snapshot order: %+v", snap)
	}
}
