package eventsvc_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/eventsvc"
)

// ---------- EventServiceHandler auth tests ---------------------------------------

func TestEventAuthPreAuthAllowed(t *testing.T) {
	h := newEventAuthenticatedHandler(t, auth.UserRecord{
		Username: "admin", Password: "secret", Roles: []string{auth.OnvifRoleAdministrator},
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, eventsvc.EventServicePath,
		bytes.NewBufferString(envelopeForEvent("GetServiceCapabilities")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PreAuth status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestEventAuthChallengeOnMissingCredentials(t *testing.T) {
	h := newEventAuthenticatedHandler(t, auth.UserRecord{
		Username: "admin", Password: "secret", Roles: []string{auth.OnvifRoleAdministrator},
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, eventsvc.EventServicePath,
		bytes.NewBufferString(envelopeForEvent("GetEventProperties")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401; body=%s", rec.Code, rec.Body.String())
	}
	if chal := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(chal, "Digest ") {
		t.Fatalf("expected Digest challenge, got %q", chal)
	}
}

func TestEventAuthDigestSuccessOnGetEventProperties(t *testing.T) {
	h := newEventAuthenticatedHandler(t, auth.UserRecord{
		Username: "admin", Password: "secret", Roles: []string{auth.OnvifRoleAdministrator},
	})
	body := envelopeForEvent("GetEventProperties")

	probe := httptest.NewRequestWithContext(context.Background(), http.MethodPost, eventsvc.EventServicePath,
		bytes.NewBufferString(body))
	probeRec := httptest.NewRecorder()
	h.ServeHTTP(probeRec, probe)
	nonce := extractNonce(probeRec.Header().Get("WWW-Authenticate"))
	if nonce == "" {
		t.Fatalf("missing nonce: %q", probeRec.Header().Get("WWW-Authenticate"))
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, eventsvc.EventServicePath,
		bytes.NewBufferString(body))
	req.Header.Set("Authorization", buildDigestHeader(t, "admin", "secret",
		eventsvc.EventServicePath, nonce))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestEventAuthUserCannotActuate(t *testing.T) {
	h := newEventAuthenticatedHandler(t, auth.UserRecord{
		Username: "viewer", Password: "p", Roles: []string{auth.OnvifRoleUser},
	})
	body := envelopeForEvent("CreatePullPointSubscription")

	probe := httptest.NewRequestWithContext(context.Background(), http.MethodPost, eventsvc.EventServicePath,
		bytes.NewBufferString(body))
	probeRec := httptest.NewRecorder()
	h.ServeHTTP(probeRec, probe)
	nonce := extractNonce(probeRec.Header().Get("WWW-Authenticate"))
	if nonce == "" {
		t.Fatalf("missing nonce")
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, eventsvc.EventServicePath,
		bytes.NewBufferString(body))
	req.Header.Set("Authorization", buildDigestHeader(t, "viewer", "p",
		eventsvc.EventServicePath, nonce))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for User invoking Actuate op, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestEventAuthOperatorCanActuate(t *testing.T) {
	h := newEventAuthenticatedHandler(t, auth.UserRecord{
		Username: "ops", Password: "p", Roles: []string{auth.OnvifRoleOperator},
	})
	body := envelopeForEvent("CreatePullPointSubscription")

	probe := httptest.NewRequestWithContext(context.Background(), http.MethodPost, eventsvc.EventServicePath,
		bytes.NewBufferString(body))
	probeRec := httptest.NewRecorder()
	h.ServeHTTP(probeRec, probe)
	nonce := extractNonce(probeRec.Header().Get("WWW-Authenticate"))
	if nonce == "" {
		t.Fatalf("missing nonce")
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, eventsvc.EventServicePath,
		bytes.NewBufferString(body))
	req.Header.Set("Authorization", buildDigestHeader(t, "ops", "p",
		eventsvc.EventServicePath, nonce))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Operator status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

// TestEventAuthEmptyBodyDigestProbe covers the 2-pass HTTP Digest probe:
// an unparseable body with no Authorization header must produce a
// 401 + WWW-Authenticate challenge so the client can negotiate.
func TestEventAuthEmptyBodyDigestProbe(t *testing.T) {
	h := newEventAuthenticatedHandler(t, auth.UserRecord{
		Username: "admin", Password: "secret", Roles: []string{auth.OnvifRoleAdministrator},
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		eventsvc.EventServicePath, bytes.NewBufferString(""))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401; body=%s", rec.Code, rec.Body.String())
	}
	if chal := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(chal, "Digest ") {
		t.Fatalf("expected Digest challenge, got %q", chal)
	}
}

// TestEventAuthEmptyBodyWithAuthHeaderReturns400 confirms the probe
// fallback only fires when the Authorization header is absent.
func TestEventAuthEmptyBodyWithAuthHeaderReturns400(t *testing.T) {
	h := newEventAuthenticatedHandler(t, auth.UserRecord{
		Username: "admin", Password: "secret", Roles: []string{auth.OnvifRoleAdministrator},
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		eventsvc.EventServicePath, bytes.NewBufferString(""))
	req.Header.Set("Authorization", `Digest username="x"`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400; body=%s", rec.Code, rec.Body.String())
	}
}

// ---------- SubscriptionManagerHandler auth tests --------------------------------

func TestSubscriptionAuthPullMessagesRequiresCredentials(t *testing.T) {
	h := newSubscriptionAuthenticatedHandler(t, auth.UserRecord{
		Username: "admin", Password: "secret", Roles: []string{auth.OnvifRoleAdministrator},
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		eventsvc.SubscriptionManagerPath+"?id=sub-001",
		bytes.NewBufferString(envelopeForEvent("PullMessages")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSubscriptionAuthDigestSuccessOnPullMessages(t *testing.T) {
	h := newSubscriptionAuthenticatedHandler(t, auth.UserRecord{
		Username: "ops", Password: "p", Roles: []string{auth.OnvifRoleOperator},
	})
	body := envelopeForEvent("PullMessages")
	path := eventsvc.SubscriptionManagerPath + "?id=sub-001"

	probe := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path,
		bytes.NewBufferString(body))
	probeRec := httptest.NewRecorder()
	h.ServeHTTP(probeRec, probe)
	nonce := extractNonce(probeRec.Header().Get("WWW-Authenticate"))
	if nonce == "" {
		t.Fatalf("missing nonce: %q", probeRec.Header().Get("WWW-Authenticate"))
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path,
		bytes.NewBufferString(body))
	req.Header.Set("Authorization", buildDigestHeader(t, "ops", "p",
		path, nonce))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSubscriptionAuthUserCannotPullMessages(t *testing.T) {
	h := newSubscriptionAuthenticatedHandler(t, auth.UserRecord{
		Username: "viewer", Password: "p", Roles: []string{auth.OnvifRoleUser},
	})
	body := envelopeForEvent("PullMessages")
	path := eventsvc.SubscriptionManagerPath + "?id=sub-001"

	probe := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path,
		bytes.NewBufferString(body))
	probeRec := httptest.NewRecorder()
	h.ServeHTTP(probeRec, probe)
	nonce := extractNonce(probeRec.Header().Get("WWW-Authenticate"))
	if nonce == "" {
		t.Fatalf("missing nonce")
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path,
		bytes.NewBufferString(body))
	req.Header.Set("Authorization", buildDigestHeader(t, "viewer", "p",
		path, nonce))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for User invoking Actuate op, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestSubscriptionAuthRenewRequiresCredentials(t *testing.T) {
	h := newSubscriptionAuthenticatedHandler(t, auth.UserRecord{
		Username: "admin", Password: "secret", Roles: []string{auth.OnvifRoleAdministrator},
	})
	// Renew uses WSNBaseNotificationNS envelope.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		eventsvc.SubscriptionManagerPath+"?id=sub-001",
		bytes.NewBufferString(envelopeForWSN("Renew")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401; body=%s", rec.Code, rec.Body.String())
	}
}

// TestSubscriptionAuthEmptyBodyDigestProbe covers the 2-pass HTTP Digest
// probe on the SubscriptionManager endpoint: an unparseable body with no
// Authorization header must surface a 401 + WWW-Authenticate challenge.
func TestSubscriptionAuthEmptyBodyDigestProbe(t *testing.T) {
	h := newSubscriptionAuthenticatedHandler(t, auth.UserRecord{
		Username: "admin", Password: "secret", Roles: []string{auth.OnvifRoleAdministrator},
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		eventsvc.SubscriptionManagerPath+"?id=sub-001", bytes.NewBufferString(""))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401; body=%s", rec.Code, rec.Body.String())
	}
	if chal := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(chal, "Digest ") {
		t.Fatalf("expected Digest challenge, got %q", chal)
	}
}

// TestSubscriptionAuthEmptyBodyWithAuthHeaderReturns400 confirms the probe
// fallback only fires when the Authorization header is absent.
func TestSubscriptionAuthEmptyBodyWithAuthHeaderReturns400(t *testing.T) {
	h := newSubscriptionAuthenticatedHandler(t, auth.UserRecord{
		Username: "admin", Password: "secret", Roles: []string{auth.OnvifRoleAdministrator},
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		eventsvc.SubscriptionManagerPath+"?id=sub-001", bytes.NewBufferString(""))
	req.Header.Set("Authorization", `Digest username="x"`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400; body=%s", rec.Code, rec.Body.String())
	}
}

// ---------- helper ---------------------------------------------------------------

func extractNonce(header string) string {
	const key = "nonce"
	i := strings.Index(header, key+"=")
	if i < 0 {
		return ""
	}
	rest := header[i+len(key)+1:]
	if strings.HasPrefix(rest, `"`) {
		j := strings.IndexByte(rest[1:], '"')
		if j < 0 {
			return ""
		}
		return rest[1 : 1+j]
	}
	if j := strings.IndexAny(rest, ", "); j >= 0 {
		return rest[:j]
	}
	return rest
}
