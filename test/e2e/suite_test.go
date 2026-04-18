//go:build e2e

// Package e2e contains end-to-end tests that verify ONVIF Profile S conformance
// against a running onvif-simulator instance using use-go/onvif as the client.
//
// Test cases are derived from the official ONVIF test specifications:
//   - ONVIF Base Device Test Specification v21.12
//   - ONVIF Media Configuration Device Test Specification
//
// Run with:
//
//	go test ./test/e2e/... -tags e2e -v
//
// Environment variables:
//
//	ONVIF_HOST     - simulator host:port (default: localhost:8080)
//	ONVIF_USERNAME - username (default: admin)
//	ONVIF_PASSWORD - password (default: "")
package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	onviflib "github.com/use-go/onvif"
)

const defaultTimeout = 10 * time.Second

func newDevice(t *testing.T) *onviflib.Device {
	t.Helper()

	host := envOrDefault("ONVIF_HOST", "localhost:8080")
	username := envOrDefault("ONVIF_USERNAME", "admin")
	password := envOrDefault("ONVIF_PASSWORD", "")

	dev, err := onviflib.NewDevice(onviflib.DeviceParams{
		Xaddr:    host,
		Username: username,
		Password: password,
	})
	if err != nil {
		t.Fatalf("failed to connect to simulator at %s: %v", host, err)
	}

	return dev
}

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	t.Cleanup(cancel)
	return c
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
