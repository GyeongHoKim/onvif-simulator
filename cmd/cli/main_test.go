package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

func TestParseOnOff(t *testing.T) {
	cases := map[string]struct {
		in     string
		want   bool
		errCmp error
	}{
		"on":      {"on", true, nil},
		"true":    {"TRUE", true, nil},
		"1":       {"1", true, nil},
		"off":     {"off", false, nil},
		"false":   {"FALSE", false, nil},
		"0":       {"0", false, nil},
		"unknown": {"maybe", false, errUnrecognisedOnOff},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := parseOnOff(tc.in)
			if tc.errCmp != nil {
				if !errors.Is(err, tc.errCmp) {
					t.Fatalf("expected %v, got %v", tc.errCmp, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestPrintUsageWritesAllCommands(t *testing.T) {
	var buf bytes.Buffer
	printUsage(&buf)
	out := buf.String()
	for _, want := range []string{"serve", "tui", "config show", "config validate", "event motion", "event sync"} {
		if !strings.Contains(out, want) {
			t.Fatalf("usage missing %q. Got:\n%s", want, out)
		}
	}
}

func TestRunHelpFlags(t *testing.T) {
	for _, flag := range []string{"-h", "--help", "help"} {
		if err := run([]string{flag}); err != nil {
			t.Fatalf("run(%s): %v", flag, err)
		}
	}
}

func TestRunUnknownSubcommand(t *testing.T) {
	err := run([]string{"bogus"})
	if !errors.Is(err, errUnknownSubcommand) {
		t.Fatalf("expected errUnknownSubcommand, got %v", err)
	}
}

func TestRunConfigSubcommandRequired(t *testing.T) {
	if err := runConfig(nil); !errors.Is(err, errConfigSubcommandReq) {
		t.Fatalf("expected errConfigSubcommandReq, got %v", err)
	}
}

func TestRunConfigUnknownSubcommand(t *testing.T) {
	if err := runConfig([]string{"bogus"}); !errors.Is(err, errConfigUnknownSubcommand) {
		t.Fatalf("expected errConfigUnknownSubcommand, got %v", err)
	}
}

// TestRunConfigUnknownSubcommandNoSideEffects guards against the regression
// where runConfig used to call config.EnsureExists (which mkdir's the user
// config directory and writes a default JSON file) before validating the
// subcommand — meaning a typo'd `onvif-simulator config bogus` would leave
// debris in the operator's real ~/.config or ~/Library/Application Support.
func TestRunConfigUnknownSubcommandNoSideEffects(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)            // macOS UserConfigDir base
	t.Setenv("XDG_CONFIG_HOME", dir) // Linux UserConfigDir base
	t.Setenv("AppData", dir)         // Windows UserConfigDir base
	t.Cleanup(func() { config.SetPath("") })

	if err := runConfig([]string{"bogus"}); !errors.Is(err, errConfigUnknownSubcommand) {
		t.Fatalf("expected errConfigUnknownSubcommand, got %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) > 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("expected no filesystem side effects, found: %v", names)
	}
}

func TestRunEventSubcommandRequired(t *testing.T) {
	if err := runEvent(nil); !errors.Is(err, errEventSubcommandReq) {
		t.Fatalf("expected errEventSubcommandReq, got %v", err)
	}
}

func TestPostSimpleEventBadArgs(t *testing.T) {
	if err := postSimpleEvent(0, "motion", []string{"only-token"}); !errors.Is(err, errEventUsageSimple) {
		t.Fatalf("expected errEventUsageSimple, got %v", err)
	}
}

func TestPostSyncEventBadArgs(t *testing.T) {
	if err := postSyncEvent(0, []string{"only", "two"}); !errors.Is(err, errEventUsageSync) {
		t.Fatalf("expected errEventUsageSync, got %v", err)
	}
}

func TestRunConfigShow(t *testing.T) {
	cfgPath, cleanup := writeTempConfig(t)
	defer cleanup()

	if err := runConfig([]string{"show", "-config", cfgPath}); err != nil {
		t.Fatalf("runConfig show: %v", err)
	}
}

func TestRunConfigValidate(t *testing.T) {
	cfgPath, cleanup := writeTempConfig(t)
	defer cleanup()

	if err := runConfig([]string{"validate", "-config", cfgPath}); err != nil {
		t.Fatalf("runConfig validate: %v", err)
	}
}

func TestRunEventNoControlPort(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp) // os.UserHomeDir uses USERPROFILE on Windows
	err := runEvent([]string{"motion", "VS0", "on"})
	if !errors.Is(err, errControlNotRunning) {
		t.Fatalf("expected errControlNotRunning, got %v", err)
	}
}

func TestRunEventUnknownSubcommand(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	t.Setenv("USERPROFILE", tmp) // os.UserHomeDir uses USERPROFILE on Windows
	// Write a port file so the lookup succeeds and we hit the unknown branch.
	cfgDir := filepath.Join(tmp, ".onvif-simulator")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "control.port"), []byte("12345\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	err := runEvent([]string{"made-up-event"})
	if !errors.Is(err, errEventUnknownSubcommand) {
		t.Fatalf("expected errEventUnknownSubcommand, got %v", err)
	}
}

func TestPostControlSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	port := mustPortFromURL(t, srv.URL)
	if err := postControl(port, "/anything", []byte(`{}`)); err != nil {
		t.Fatalf("postControl: %v", err)
	}
}

func TestPostControlReturnsServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	port := mustPortFromURL(t, srv.URL)
	err := postControl(port, "/anything", []byte(`{}`))
	if !errors.Is(err, errControlServer) {
		t.Fatalf("expected errControlServer, got %v", err)
	}
}

func TestPostSimpleEventEndToEnd(t *testing.T) {
	var got tokenState
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got) //nolint:errcheck // assertion below.
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	port := mustPortFromURL(t, srv.URL)
	if err := postSimpleEvent(port, "motion", []string{"VS9", "on"}); err != nil {
		t.Fatalf("postSimpleEvent: %v", err)
	}
	if got.Token != "VS9" || !got.State {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestPostSyncEventEndToEnd(t *testing.T) {
	var got syncRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got) //nolint:errcheck // assertion below.
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	port := mustPortFromURL(t, srv.URL)
	if err := postSyncEvent(port, []string{
		"tns1:Custom/Topic", "SrcItem", "Tok", "DataItem", "off",
	}); err != nil {
		t.Fatalf("postSyncEvent: %v", err)
	}
	if got.Topic != "tns1:Custom/Topic" || got.State {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

// mustPortFromURL extracts the port from an httptest server URL.
func mustPortFromURL(t *testing.T, url string) int {
	t.Helper()
	hostport := strings.TrimPrefix(url, "http://")
	_, portStr, err := net.SplitHostPort(hostport)
	if err != nil {
		t.Fatalf("split: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("atoi: %v", err)
	}
	return port
}

// referenced from control_test.go.
var _ = config.FileName

// silence test-time context warnings in some go versions.
var _ = context.Background
