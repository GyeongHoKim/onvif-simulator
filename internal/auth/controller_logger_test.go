package auth_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

func TestControllerSetLoggerEmitsOnUpsert(t *testing.T) {
	seedControllerTest(t)
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	store := auth.NewMutableUserStore(nil)
	ctrl := auth.NewController(store)
	ctrl.SetLogger(logger)

	u := config.UserConfig{Username: "alice", Password: "pw", Role: config.RoleOperator}
	if err := ctrl.UpsertUser(u); err != nil {
		t.Fatalf("UpsertUser: %v", err)
	}
	if !strings.Contains(buf.String(), `"username":"alice"`) {
		t.Errorf("upsert log missing alice: %q", buf.String())
	}
	if !strings.Contains(buf.String(), `"msg":"auth: user upserted"`) {
		t.Errorf("upsert message missing: %q", buf.String())
	}

	buf.Reset()
	if err := ctrl.RemoveUser("alice"); err != nil {
		t.Fatalf("RemoveUser: %v", err)
	}
	if !strings.Contains(buf.String(), `"msg":"auth: user removed"`) {
		t.Errorf("remove log missing: %q", buf.String())
	}
}

func TestControllerSetLoggerNilFallback(t *testing.T) {
	store := auth.NewMutableUserStore(nil)
	ctrl := auth.NewController(store)
	ctrl.SetLogger(nil) // must not panic
	if ctrl.Logger() == nil {
		t.Fatal("Logger() returned nil after SetLogger(nil)")
	}
}
