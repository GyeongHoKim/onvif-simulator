package auth_test

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
)

const envelopeTemplate = `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Header>
    <wsse:Security xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd">
      <wsse:UsernameToken>
        <wsse:Username>%s</wsse:Username>
        <wsse:Password Type="%s">%s</wsse:Password>
        <wsse:Nonce>%s</wsse:Nonce>
        <wsu:Created xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">%s</wsu:Created>
      </wsse:UsernameToken>
    </wsse:Security>
  </s:Header>
  <s:Body/>
</s:Envelope>`

func wsseDigest(nonce, created, password string) string {
	h := sha1.New()
	h.Write([]byte(nonce))
	h.Write([]byte(created))
	h.Write([]byte(password))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func TestUsernameTokenSuccess(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore([]auth.UserRecord{{Username: "admin", Password: "pw", Roles: []string{"onvif:Administrator"}}})
	now := time.Unix(1_700_000_000, 0).UTC()
	a := auth.NewUsernameTokenAuthenticator(store, auth.UsernameTokenOptions{
		MaxClockSkew: time.Minute,
		Clock:        func() time.Time { return now },
	})
	nonceRaw := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
	nonceB64 := base64.StdEncoding.EncodeToString(nonceRaw)
	created := now.Format(time.RFC3339)
	digest := wsseDigest(string(nonceRaw), created, "pw")
	body := fmt.Sprintf(envelopeTemplate, "admin",
		"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest",
		digest, nonceB64, created)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(body))
	p, err := a.Authenticate(context.Background(), req)
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if p.Username != "admin" || p.Method != auth.MethodUsernameToken {
		t.Fatalf("principal: %+v", p)
	}
	// Body must still be readable downstream.
	rest, rerr := io.ReadAll(req.Body)
	if rerr != nil {
		t.Fatalf("read body: %v", rerr)
	}
	if len(rest) == 0 {
		t.Fatal("request body should be restored after authentication")
	}
}

func TestUsernameTokenMissingHeader(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore([]auth.UserRecord{{Username: "u", Password: "p"}})
	a := auth.NewUsernameTokenAuthenticator(store, auth.UsernameTokenOptions{})
	envelope := `<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body/></s:Envelope>`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(envelope))
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrNoCredentials) {
		t.Fatalf("expected ErrNoCredentials, got %v", err)
	}
}

func TestUsernameTokenBadPassword(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore([]auth.UserRecord{{Username: "u", Password: "right"}})
	now := time.Unix(1_700_000_000, 0).UTC()
	a := auth.NewUsernameTokenAuthenticator(store, auth.UsernameTokenOptions{Clock: func() time.Time { return now }})
	nonceB64 := base64.StdEncoding.EncodeToString([]byte("nonce"))
	created := now.Format(time.RFC3339)
	digest := wsseDigest("nonce", created, "wrong")
	body := fmt.Sprintf(envelopeTemplate, "u",
		"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest",
		digest, nonceB64, created)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(body))
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	// Hard validation failures must surface as a *ChallengeError carrying
	// the ONVIF NotAuthorized subcode so the SOAP fault tells gSOAP-style
	// clients that authentication is the issue.
	var ce *auth.ChallengeError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ChallengeError, got %T", err)
	}
	if ce.Subcode != auth.OnvifFaultNotAuthorized {
		t.Fatalf("Subcode = %q, want %q", ce.Subcode, auth.OnvifFaultNotAuthorized)
	}
	if ce.Status != http.StatusUnauthorized {
		t.Fatalf("Status = %d, want 401", ce.Status)
	}
}

// TestUsernameTokenNoCredentialsRemainsBare verifies the soft "no credentials"
// path still surfaces as a plain ErrNoCredentials so the auth chain can fall
// through to other authenticators.
func TestUsernameTokenNoCredentialsRemainsBare(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore(nil)
	a := auth.NewUsernameTokenAuthenticator(store, auth.UsernameTokenOptions{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(
		`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body/></s:Envelope>`,
	))
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrNoCredentials) {
		t.Fatalf("expected ErrNoCredentials, got %v", err)
	}
	// Soft errors must NOT be wrapped — the chain depends on
	// errors.Is(err, ErrNoCredentials) to fall through.
	var ce *auth.ChallengeError
	if errors.As(err, &ce) {
		t.Fatalf("ErrNoCredentials should not be wrapped in *ChallengeError, got %+v", ce)
	}
}

func TestUsernameTokenReplayRejected(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore([]auth.UserRecord{{Username: "u", Password: "p"}})
	now := time.Unix(1_700_000_000, 0).UTC()
	a := auth.NewUsernameTokenAuthenticator(store, auth.UsernameTokenOptions{Clock: func() time.Time { return now }})
	nonceB64 := base64.StdEncoding.EncodeToString([]byte("replay-nonce"))
	created := now.Format(time.RFC3339)
	digest := wsseDigest("replay-nonce", created, "p")
	body := fmt.Sprintf(envelopeTemplate, "u",
		"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest",
		digest, nonceB64, created)

	first := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(body))
	if _, err := a.Authenticate(context.Background(), first); err != nil {
		t.Fatalf("first: %v", err)
	}
	second := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(body))
	_, err := a.Authenticate(context.Background(), second)
	if !errors.Is(err, auth.ErrReplayedNonce) {
		t.Fatalf("expected ErrReplayedNonce, got %v", err)
	}
}

func TestUsernameTokenClockSkew(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore([]auth.UserRecord{{Username: "u", Password: "p"}})
	now := time.Unix(1_700_000_000, 0).UTC()
	a := auth.NewUsernameTokenAuthenticator(store, auth.UsernameTokenOptions{
		MaxClockSkew: 10 * time.Second,
		Clock:        func() time.Time { return now },
	})
	nonceB64 := base64.StdEncoding.EncodeToString([]byte("n"))
	created := now.Add(-time.Hour).Format(time.RFC3339)
	digest := wsseDigest("n", created, "p")
	body := fmt.Sprintf(envelopeTemplate, "u",
		"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest",
		digest, nonceB64, created)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(body))
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrClockSkew) {
		t.Fatalf("expected ErrClockSkew, got %v", err)
	}
}

func TestUsernameTokenPasswordTextRejectedByDefault(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore([]auth.UserRecord{{Username: "u", Password: "p"}})
	now := time.Unix(1_700_000_000, 0).UTC()
	a := auth.NewUsernameTokenAuthenticator(store, auth.UsernameTokenOptions{Clock: func() time.Time { return now }})
	body := fmt.Sprintf(envelopeTemplate, "u",
		"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordText",
		"p", "ignored", now.Format(time.RFC3339))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(body))
	_, err := a.Authenticate(context.Background(), req)
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials for PasswordText, got %v", err)
	}
}

func TestUsernameTokenPasswordTextWhenAllowed(t *testing.T) {
	t.Parallel()
	store := auth.NewMutableUserStore([]auth.UserRecord{{Username: "u", Password: "p"}})
	now := time.Unix(1_700_000_000, 0).UTC()
	a := auth.NewUsernameTokenAuthenticator(store, auth.UsernameTokenOptions{
		AllowPasswordText: true,
		Clock:             func() time.Time { return now },
	})
	body := fmt.Sprintf(envelopeTemplate, "u",
		"http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordText",
		"p", "ignored", now.Format(time.RFC3339))
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", strings.NewReader(body))
	if _, err := a.Authenticate(context.Background(), req); err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
}
