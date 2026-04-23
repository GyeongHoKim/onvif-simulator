package auth_test

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"hash"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
)

// digestClient computes a Digest Authorization header that matches the server
// logic in digest.go. Used by the tests below as a reference client.
//
//nolint:unparam // realm is part of the Digest protocol contract; we only use one realm in these tests but keep the parameter for clarity.
func digestClient(t *testing.T, alg auth.DigestAlgorithm, realm, username, password, method, uri, nonce, nc, cnonce string) string {
	t.Helper()
	hasher := func(in string) string {
		var h hash.Hash
		switch alg {
		case auth.DigestSHA256:
			h = sha256.New()
		default:
			h = md5.New()
		}
		h.Write([]byte(in))
		return hex.EncodeToString(h.Sum(nil))
	}
	ha1 := hasher(username + ":" + realm + ":" + password)
	ha2 := hasher(method + ":" + uri)
	resp := hasher(strings.Join([]string{ha1, nonce, nc, cnonce, "auth", ha2}, ":"))
	return strings.Join([]string{
		`Digest username="` + username + `"`,
		`realm="` + realm + `"`,
		`nonce="` + nonce + `"`,
		`uri="` + uri + `"`,
		`qop=auth`,
		`nc=` + nc,
		`cnonce="` + cnonce + `"`,
		`response="` + resp + `"`,
		`algorithm=` + string(alg),
	}, ", ")
}

// fetchNonce issues a request without credentials and pulls the nonce out of
// the first WWW-Authenticate: Digest header for the requested algorithm.
func fetchNonce(t *testing.T, a auth.Authenticator, alg auth.DigestAlgorithm) string {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/onvif/device_service", http.NoBody)
	_, err := a.Authenticate(context.Background(), req)
	var ce *auth.ChallengeError
	if !errors.As(err, &ce) {
		t.Fatalf("expected ChallengeError, got %v", err)
	}
	for _, v := range ce.Headers["Www-Authenticate"] {
		if !strings.HasPrefix(v, "Digest ") {
			continue
		}
		if !strings.Contains(v, "algorithm="+string(alg)) {
			continue
		}
		const key = `nonce="`
		i := strings.Index(v, key)
		if i < 0 {
			t.Fatalf("no nonce in challenge: %q", v)
		}
		rest := v[i+len(key):]
		j := strings.IndexByte(rest, '"')
		if j < 0 {
			t.Fatalf("unterminated nonce: %q", v)
		}
		return rest[:j]
	}
	t.Fatalf("no challenge for algorithm %s: %v", alg, ce.Headers)
	return ""
}

func TestDigestNoCredentialsChallenges(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore([]auth.UserRecord{{Username: "admin", Password: "pw"}})
	a := auth.NewDigestAuthenticator(store, auth.DigestOptions{Realm: "r"})
	_, err := a.Authenticate(context.Background(), httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody))
	var ce *auth.ChallengeError
	if !errors.As(err, &ce) {
		// Plain ErrNoCredentials is also acceptable since Chain will convert it.
		if !errors.Is(err, auth.ErrNoCredentials) {
			t.Fatalf("expected ErrNoCredentials or ChallengeError, got %v", err)
		}
	}
}

func TestDigestMD5Success(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore([]auth.UserRecord{{Username: "admin", Password: "secret", Roles: []string{"onvif:Administrator"}}})
	a := auth.NewDigestAuthenticator(store, auth.DigestOptions{
		Realm:      "onvif",
		Algorithms: []auth.DigestAlgorithm{auth.DigestMD5},
	})
	nonce := fetchNonce(t, a, auth.DigestMD5)
	authz := digestClient(t, auth.DigestMD5, "onvif", "admin", "secret", http.MethodPost, "/onvif/device_service", nonce, "00000001", "cnonce-value")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/onvif/device_service", http.NoBody)
	req.Header.Set("Authorization", authz)
	p, err := a.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if p.Username != "admin" || p.Method != auth.MethodDigest {
		t.Fatalf("principal: %+v", p)
	}
}

func TestDigestSHA256Success(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore([]auth.UserRecord{{Username: "u", Password: "p"}})
	a := auth.NewDigestAuthenticator(store, auth.DigestOptions{
		Realm:      "onvif",
		Algorithms: []auth.DigestAlgorithm{auth.DigestMD5, auth.DigestSHA256},
	})
	nonce := fetchNonce(t, a, auth.DigestSHA256)
	authz := digestClient(t, auth.DigestSHA256, "onvif", "u", "p", http.MethodPost, "/x", nonce, "00000001", "c")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/x", http.NoBody)
	req.Header.Set("Authorization", authz)
	if _, err := a.Authenticate(context.Background(), req); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
}

func TestDigestWrongPasswordFails(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore([]auth.UserRecord{{Username: "admin", Password: "secret"}})
	a := auth.NewDigestAuthenticator(store, auth.DigestOptions{Realm: "onvif"})
	nonce := fetchNonce(t, a, auth.DigestMD5)
	authz := digestClient(t, auth.DigestMD5, "onvif", "admin", "WRONG", http.MethodPost, "/x", nonce, "00000001", "c")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/x", http.NoBody)
	req.Header.Set("Authorization", authz)
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestDigestStaleNonce(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore([]auth.UserRecord{{Username: "u", Password: "p"}})
	now := time.Unix(1_700_000_000, 0)
	clock := now
	a := auth.NewDigestAuthenticator(store, auth.DigestOptions{
		Realm:    "onvif",
		NonceTTL: 1 * time.Second,
		Clock:    func() time.Time { return clock },
	})
	nonce := fetchNonce(t, a, auth.DigestMD5)
	clock = clock.Add(2 * time.Second)
	authz := digestClient(t, auth.DigestMD5, "onvif", "u", "p", http.MethodPost, "/x", nonce, "00000001", "c")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/x", http.NoBody)
	req.Header.Set("Authorization", authz)
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrStaleNonce) {
		t.Fatalf("expected ErrStaleNonce, got %v", err)
	}
	// The challenge on stale must include stale=true for RFC 2617 compliance.
	var ce *auth.ChallengeError
	if !errors.As(err, &ce) {
		t.Fatalf("expected ChallengeError, got %v", err)
	}
	gotStale := false
	for _, v := range ce.Headers["Www-Authenticate"] {
		if strings.Contains(v, "stale=true") {
			gotStale = true
			break
		}
	}
	if !gotStale {
		t.Fatalf("challenge missing stale=true: %v", ce.Headers)
	}
}

func TestDigestForgedNonceRejected(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore([]auth.UserRecord{{Username: "u", Password: "p"}})
	a := auth.NewDigestAuthenticator(store, auth.DigestOptions{Realm: "onvif"})
	authz := digestClient(t, auth.DigestMD5, "onvif", "u", "p", http.MethodPost, "/x", "not-a-real-nonce", "00000001", "c")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/x", http.NoBody)
	req.Header.Set("Authorization", authz)
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestDigestUnknownUser(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore(nil)
	a := auth.NewDigestAuthenticator(store, auth.DigestOptions{Realm: "onvif"})
	nonce := fetchNonce(t, a, auth.DigestMD5)
	authz := digestClient(t, auth.DigestMD5, "onvif", "ghost", "p", http.MethodPost, "/x", nonce, "00000001", "c")
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/x", http.NoBody)
	req.Header.Set("Authorization", authz)
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}
