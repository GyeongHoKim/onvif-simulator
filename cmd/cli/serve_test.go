//go:build !windows

package main

import (
	"errors"
	"syscall"
	"testing"
	"time"
)

func TestRunServeSignalShutdown(t *testing.T) {
	cleanup := chdirToTempConfig(t)
	defer cleanup()

	// Use a temp HOME/USERPROFILE so writeControlPortFile lands somewhere ephemeral.
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome) // os.UserHomeDir uses USERPROFILE on Windows

	done := make(chan error, 1)
	go func() {
		done <- runServe(nil)
	}()

	// Give the server a moment to bind.
	time.Sleep(300 * time.Millisecond)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("kill: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runServe returned: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runServe did not exit on SIGINT")
	}
}

func TestRunTUIReturnsTUIError(t *testing.T) {
	cleanup := chdirToTempConfig(t)
	defer cleanup()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome) // os.UserHomeDir uses USERPROFILE on Windows

	err := runTUI(nil)
	if err == nil {
		t.Fatal("expected error from runTUI (no TTY in tests)")
	}
	// The TUI panics or errors when there is no TTY; either way, we just
	// need to know the function path executed. Some TUI failures wrap with
	// errors.New, so we use a simple non-nil check above. Sanity guard:
	if errors.Is(err, errUnknownSubcommand) {
		t.Fatalf("unexpected unknown-subcommand error: %v", err)
	}
}
