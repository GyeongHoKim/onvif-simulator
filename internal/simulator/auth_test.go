package simulator

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
	"github.com/GyeongHoKim/onvif-simulator/internal/config"
)

func generateTestECPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}
	var buf bytes.Buffer
	if encErr := pem.Encode(&buf, &pem.Block{Type: "PUBLIC KEY", Bytes: der}); encErr != nil {
		t.Fatalf("pem encode: %v", encErr)
	}
	return buf.String()
}

func TestRebuildAuthChainDigestAlgorithmsAndTTL(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	cfg := sim.ConfigSnapshot()
	cfg.Auth.Digest.Realm = "testrealm"
	cfg.Auth.Digest.Algorithms = []string{"MD5", "SHA-256"}
	cfg.Auth.Digest.NonceTTL = "5m"

	if err := sim.rebuildAuthChain(&cfg); err != nil {
		t.Fatalf("rebuildAuthChain with digest options: %v", err)
	}
}

func TestRebuildAuthChainJWTEnabled(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	pemKey := generateTestECPEM(t)
	cfg := sim.ConfigSnapshot()
	cfg.Auth.JWT.Enabled = true
	cfg.Auth.JWT.Issuer = "test-issuer"
	cfg.Auth.JWT.Algorithms = []string{"ES256"}
	cfg.Auth.JWT.PublicKeyPEM = []string{pemKey}

	if err := sim.rebuildAuthChain(&cfg); err != nil {
		t.Fatalf("rebuildAuthChain with JWT: %v", err)
	}
}

func TestRebuildAuthChainJWTNoKeyMaterial(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	cfg := sim.ConfigSnapshot()
	cfg.Auth.JWT.Enabled = true
	cfg.Auth.JWT.Issuer = "test-issuer"
	cfg.Auth.JWT.Algorithms = []string{"RS256"}

	if err := sim.rebuildAuthChain(&cfg); err == nil {
		t.Fatal("expected error: JWT enabled with no key material")
	}
}

func TestBuildJWTAuthenticatorNoKeyMaterial(t *testing.T) {
	j := &config.JWTConfig{
		Issuer:     "test",
		Algorithms: []string{"RS256"},
	}
	if _, err := buildJWTAuthenticator(j); err == nil {
		t.Fatal("expected ErrAuthJWTKeyMaterial")
	}
}

func TestBuildJWTAuthenticatorPEMKeyWithOptions(t *testing.T) {
	pemKey := generateTestECPEM(t)
	requireTLS := false
	j := &config.JWTConfig{
		Issuer:       "test-issuer",
		Algorithms:   []string{"ES256"},
		PublicKeyPEM: []string{pemKey},
		ClockSkew:    "30s",
		RequireTLS:   &requireTLS,
	}
	if _, err := buildJWTAuthenticator(j); err != nil {
		t.Fatalf("buildJWTAuthenticator with PEM key: %v", err)
	}
}

func TestBuildJWTAuthenticatorJWKSURL(t *testing.T) {
	j := &config.JWTConfig{
		Issuer:     "test",
		Algorithms: []string{"RS256"},
		JWKSURL:    "https://example.com/.well-known/jwks.json",
	}
	// Exercises the JWKS URL branch; network fetch is deferred to token validation.
	_, _ = buildJWTAuthenticator(j)
}

func TestCurrentAuthChain(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	chain, _ := sim.currentAuthChain()
	if chain == nil {
		t.Fatal("expected non-nil auth chain after New")
	}
}

func TestAuthorizeClassPreAuth(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	sim.authMu.Lock()
	sim.authEnabled = true
	sim.authMu.Unlock()

	r, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	if err := sim.authorize(context.Background(), "GetDeviceInformation", r, auth.ClassPreAuth); err != nil {
		t.Fatalf("ClassPreAuth must bypass auth: %v", err)
	}
}

func TestAuthorizeUnauthenticatedRequest(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	sim.authMu.Lock()
	sim.authEnabled = true
	sim.authMu.Unlock()

	r, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	if err := sim.authorize(context.Background(), "GetProfiles", r, auth.ClassReadSystem); err == nil {
		t.Fatal("expected auth error for unauthenticated request with auth enabled")
	}
}

func TestAuthorizeDisabledSkipsChain(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	// Auth is disabled by default; every class passes.
	r, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/", nil)
	if err := sim.authorize(context.Background(), "op", r, auth.ClassWriteSystem); err != nil {
		t.Fatalf("disabled auth should pass: %v", err)
	}
}

func TestAuthHooksWhenDisabled(t *testing.T) {
	sim, cleanup := newTestSimulator(t)
	defer cleanup()

	ctx := context.Background()
	r, _ := http.NewRequestWithContext(ctx, http.MethodGet, "/", nil)

	if err := sim.deviceAuthHook(ctx, "GetDeviceInformation", r); err != nil {
		t.Fatalf("deviceAuthHook: %v", err)
	}
	if err := sim.mediaAuthHook(ctx, "GetProfiles", r); err != nil {
		t.Fatalf("mediaAuthHook: %v", err)
	}
	if err := sim.eventAuthHook(ctx, "Subscribe", r); err != nil {
		t.Fatalf("eventAuthHook: %v", err)
	}
}
