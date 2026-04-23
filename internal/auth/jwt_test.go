package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
)

func newRSAPair(t *testing.T) (priv *rsa.PrivateKey, pubPEM []byte) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa keygen: %v", err)
	}
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("marshal public: %v", err)
	}
	pubPEM = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	return priv, pubPEM
}

func signRS256(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	s, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func newJWTAuth(t *testing.T, pub []byte, opts *auth.JWTOptions) auth.Authenticator {
	t.Helper()
	kf, err := auth.NewStaticKeyFunc([][]byte{pub})
	if err != nil {
		t.Fatalf("keyfunc: %v", err)
	}
	opts.KeyFunc = kf
	a, err := auth.NewJWTAuthenticator(*opts)
	if err != nil {
		t.Fatalf("NewJWTAuthenticator: %v", err)
	}
	return a
}

func TestJWTSuccess(t *testing.T) {
	t.Parallel()
	priv, pub := newRSAPair(t)
	now := time.Unix(1_700_000_000, 0)
	a := newJWTAuth(t, pub, &auth.JWTOptions{
		Issuer:   "https://issuer.example",
		Audience: "onvif-sim",
		Clock:    func() time.Time { return now },
	})
	tok := signRS256(t, priv, jwt.MapClaims{
		"iss":   "https://issuer.example",
		"aud":   "onvif-sim",
		"sub":   "alice",
		"exp":   now.Add(time.Hour).Unix(),
		"nbf":   now.Add(-time.Minute).Unix(),
		"iat":   now.Unix(),
		"roles": []string{"onvif:Administrator"},
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	p, err := a.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if p.Username != "alice" || p.Method != auth.MethodJWT {
		t.Fatalf("principal: %+v", p)
	}
	if len(p.Roles) != 1 || p.Roles[0] != "onvif:Administrator" {
		t.Fatalf("roles: %+v", p.Roles)
	}
}

func TestJWTExpired(t *testing.T) {
	t.Parallel()
	priv, pub := newRSAPair(t)
	now := time.Unix(1_700_000_000, 0)
	a := newJWTAuth(t, pub, &auth.JWTOptions{
		Issuer:   "i",
		Audience: "a",
		Clock:    func() time.Time { return now },
	})
	tok := signRS256(t, priv, jwt.MapClaims{
		"iss": "i", "aud": "a", "sub": "x",
		"exp": now.Add(-time.Hour).Unix(),
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrTokenExpired) {
		t.Fatalf("expected ErrTokenExpired, got %v", err)
	}
}

func TestJWTBadSignature(t *testing.T) {
	t.Parallel()
	goodPriv, _ := newRSAPair(t)
	_, badPub := newRSAPair(t)
	now := time.Unix(1_700_000_000, 0)
	a := newJWTAuth(t, badPub, &auth.JWTOptions{
		Issuer:   "i",
		Audience: "a",
		Clock:    func() time.Time { return now },
	})
	tok := signRS256(t, goodPriv, jwt.MapClaims{
		"iss": "i", "aud": "a", "sub": "x",
		"exp": now.Add(time.Hour).Unix(),
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrTokenSignature) {
		t.Fatalf("expected ErrTokenSignature, got %v", err)
	}
}

func TestJWTAudienceMismatch(t *testing.T) {
	t.Parallel()
	priv, pub := newRSAPair(t)
	now := time.Unix(1_700_000_000, 0)
	a := newJWTAuth(t, pub, &auth.JWTOptions{
		Issuer:   "i",
		Audience: "expected",
		Clock:    func() time.Time { return now },
	})
	tok := signRS256(t, priv, jwt.MapClaims{
		"iss": "i", "aud": "other", "sub": "x",
		"exp": now.Add(time.Hour).Unix(),
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrAudienceMismatch) {
		t.Fatalf("expected ErrAudienceMismatch, got %v", err)
	}
}

func TestJWTMissingBearer(t *testing.T) {
	t.Parallel()
	_, pub := newRSAPair(t)
	a := newJWTAuth(t, pub, &auth.JWTOptions{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrNoCredentials) {
		t.Fatalf("expected ErrNoCredentials, got %v", err)
	}
}

func TestJWTRequiresTLS(t *testing.T) {
	t.Parallel()
	priv, pub := newRSAPair(t)
	now := time.Unix(1_700_000_000, 0)
	a := newJWTAuth(t, pub, &auth.JWTOptions{
		RequireTLS: true,
		Clock:      func() time.Time { return now },
	})
	tok := signRS256(t, priv, jwt.MapClaims{
		"sub": "x",
		"exp": now.Add(time.Hour).Unix(),
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrInsecureTransport) {
		t.Fatalf("expected ErrInsecureTransport, got %v", err)
	}
}
