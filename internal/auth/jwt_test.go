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
	return signRS256WithKid(t, key, "", claims)
}

func signRS256WithKid(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	if kid != "" {
		token.Header["kid"] = kid
	}
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

func TestNewJWTAuthenticatorRequiresKeyFunc(t *testing.T) {
	t.Parallel()
	_, err := auth.NewJWTAuthenticator(auth.JWTOptions{})
	if err == nil {
		t.Fatal("expected error when KeyFunc is nil")
	}
}

func TestJWTIssuerMismatch(t *testing.T) {
	t.Parallel()
	priv, pub := newRSAPair(t)
	now := time.Unix(1_700_000_000, 0)
	a := newJWTAuth(t, pub, &auth.JWTOptions{
		Issuer:   "expected-iss",
		Audience: "a",
		Clock:    func() time.Time { return now },
	})
	tok := signRS256(t, priv, jwt.MapClaims{
		"iss": "wrong-iss", "aud": "a", "sub": "x",
		"exp": now.Add(time.Hour).Unix(),
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrIssuerMismatch) {
		t.Fatalf("expected ErrIssuerMismatch, got %v", err)
	}
}

func TestJWTMissingUsernameClaim(t *testing.T) {
	t.Parallel()
	priv, pub := newRSAPair(t)
	now := time.Unix(1_700_000_000, 0)
	a := newJWTAuth(t, pub, &auth.JWTOptions{
		Issuer:   "i",
		Audience: "a",
		Clock:    func() time.Time { return now },
	})
	tok := signRS256(t, priv, jwt.MapClaims{
		"iss": "i", "aud": "a",
		"exp": now.Add(time.Hour).Unix(),
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrTokenMalformed) {
		t.Fatalf("expected ErrTokenMalformed, got %v", err)
	}
}

func TestJWTNotValidYet(t *testing.T) {
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
		"iat": now.Unix(),
		"nbf": now.Add(time.Hour).Unix(),
		"exp": now.Add(2 * time.Hour).Unix(),
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrTokenMalformed) {
		t.Fatalf("expected ErrTokenMalformed (not yet valid), got %v", err)
	}
}

func TestJWTMalformedTokenString(t *testing.T) {
	t.Parallel()
	_, pub := newRSAPair(t)
	now := time.Unix(1_700_000_000, 0)
	a := newJWTAuth(t, pub, &auth.JWTOptions{
		Clock: func() time.Time { return now },
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer not-a-valid-jwt")
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrTokenMalformed) {
		t.Fatalf("expected ErrTokenMalformed, got %v", err)
	}
}

func TestJWTAudienceAsJSONArray(t *testing.T) {
	t.Parallel()
	priv, pub := newRSAPair(t)
	now := time.Unix(1_700_000_000, 0)
	a := newJWTAuth(t, pub, &auth.JWTOptions{
		Issuer:   "https://issuer.example",
		Audience: "onvif-sim",
		Clock:    func() time.Time { return now },
	})
	tok := signRS256(t, priv, jwt.MapClaims{
		"iss": "https://issuer.example",
		"aud": []any{"onvif-sim"},
		"sub": "alice",
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	p, err := a.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if p.Username != "alice" {
		t.Fatalf("username: %q", p.Username)
	}
}

func TestJWTRolesFromAnySlice(t *testing.T) {
	t.Parallel()
	priv, pub := newRSAPair(t)
	now := time.Unix(1_700_000_000, 0)
	a := newJWTAuth(t, pub, &auth.JWTOptions{
		Issuer:   "i",
		Audience: "a",
		Clock:    func() time.Time { return now },
	})
	tok := signRS256(t, priv, jwt.MapClaims{
		"iss": "i", "aud": "a", "sub": "bob",
		"exp":   now.Add(time.Hour).Unix(),
		"iat":   now.Unix(),
		"roles": []any{"onvif:User", 42, nil, ""},
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	p, err := a.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if len(p.Roles) != 1 || p.Roles[0] != "onvif:User" {
		t.Fatalf("roles: %+v", p.Roles)
	}
}

func TestNewStaticKeyFuncMultiKeyWithKID(t *testing.T) {
	t.Parallel()
	_, pub0 := newRSAPair(t)
	priv1, pub1 := newRSAPair(t)
	kf, err := auth.NewStaticKeyFunc([][]byte{pub0, pub1})
	if err != nil {
		t.Fatalf("NewStaticKeyFunc: %v", err)
	}
	now := time.Unix(1_700_000_000, 0)
	a, err := auth.NewJWTAuthenticator(auth.JWTOptions{
		KeyFunc:   kf,
		Issuer:    "i",
		Audience:  "a",
		Clock:     func() time.Time { return now },
		ClockSkew: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewJWTAuthenticator: %v", err)
	}
	tok := signRS256WithKid(t, priv1, "1", jwt.MapClaims{
		"iss": "i", "aud": "a", "sub": "kid-user",
		"exp": now.Add(time.Hour).Unix(),
		"iat": now.Unix(),
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
	req.Header.Set("Authorization", "Bearer "+tok)
	p, err := a.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if p.Username != "kid-user" {
		t.Fatalf("username: %q", p.Username)
	}
}

func TestNewJWKSKeyFuncEmptyURL(t *testing.T) {
	t.Parallel()
	_, err := auth.NewJWKSKeyFunc("")
	if err == nil {
		t.Fatal("expected error for empty JWKS URL")
	}
}
