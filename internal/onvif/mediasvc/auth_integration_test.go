package mediasvc_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/mediasvc"
)

// minimal provider needed by the integration tests.
type stubProvider struct{}

func (stubProvider) ServiceCapabilities(context.Context) (mediasvc.ServiceCapabilities, error) {
	return mediasvc.ServiceCapabilities{SnapshotURI: true}, nil
}

func (stubProvider) Profiles(context.Context) ([]mediasvc.Profile, error) {
	return []mediasvc.Profile{{Token: "profile_main", Name: "main"}}, nil
}

func (stubProvider) Profile(_ context.Context, token string) (mediasvc.Profile, error) {
	if token != "profile_main" {
		return mediasvc.Profile{}, mediasvc.ErrProfileNotFound
	}
	return mediasvc.Profile{Token: token, Name: "main"}, nil
}

func (stubProvider) CreateProfile(_ context.Context, name, token string) (mediasvc.Profile, error) {
	return mediasvc.Profile{Token: token, Name: name}, nil
}

func (stubProvider) DeleteProfile(context.Context, string) error { return nil }

func (stubProvider) VideoSources(context.Context) ([]mediasvc.VideoSource, error) {
	return []mediasvc.VideoSource{{Token: "VS_MAIN"}}, nil
}

func (stubProvider) VideoSourceConfigurations(context.Context) ([]mediasvc.VideoSourceConfiguration, error) {
	return nil, nil
}

func (stubProvider) VideoSourceConfiguration(context.Context, string) (mediasvc.VideoSourceConfiguration, error) {
	return mediasvc.VideoSourceConfiguration{}, mediasvc.ErrConfigNotFound
}

func (stubProvider) SetVideoSourceConfiguration(context.Context, mediasvc.VideoSourceConfiguration) error {
	return nil
}

func (stubProvider) AddVideoSourceConfiguration(context.Context, string, string) error {
	return nil
}

func (stubProvider) RemoveVideoSourceConfiguration(context.Context, string) error { return nil }

func (stubProvider) CompatibleVideoSourceConfigurations(context.Context, string) ([]mediasvc.VideoSourceConfiguration, error) {
	return nil, nil
}

func (stubProvider) VideoSourceConfigurationOptions(context.Context, string, string) (mediasvc.VideoSourceConfigurationOptions, error) {
	return mediasvc.VideoSourceConfigurationOptions{}, nil
}

func (stubProvider) VideoEncoderConfigurations(context.Context) ([]mediasvc.VideoEncoderConfiguration, error) {
	return nil, nil
}

func (stubProvider) VideoEncoderConfiguration(context.Context, string) (mediasvc.VideoEncoderConfiguration, error) {
	return mediasvc.VideoEncoderConfiguration{}, mediasvc.ErrConfigNotFound
}

func (stubProvider) SetVideoEncoderConfiguration(context.Context, mediasvc.VideoEncoderConfiguration) error {
	return nil
}

func (stubProvider) AddVideoEncoderConfiguration(context.Context, string, string) error { return nil }

func (stubProvider) RemoveVideoEncoderConfiguration(context.Context, string) error { return nil }

func (stubProvider) CompatibleVideoEncoderConfigurations(context.Context, string) ([]mediasvc.VideoEncoderConfiguration, error) {
	return nil, nil
}

func (stubProvider) VideoEncoderConfigurationOptions(context.Context, string, string) (mediasvc.VideoEncoderConfigurationOptions, error) {
	return mediasvc.VideoEncoderConfigurationOptions{}, nil
}

func (stubProvider) StreamURI(context.Context, string, mediasvc.StreamSetup) (mediasvc.MediaURI, error) {
	return mediasvc.MediaURI{URI: "rtsp://x/y"}, nil
}

func (stubProvider) SnapshotURI(context.Context, string) (mediasvc.MediaURI, error) {
	return mediasvc.MediaURI{URI: "http://x/snap.jpg"}, nil
}

func (stubProvider) GuaranteedNumberOfVideoEncoderInstances(context.Context, string) (int, error) {
	return 1, nil
}

func (stubProvider) MetadataConfigurations(context.Context) ([]mediasvc.MetadataConfiguration, error) {
	return nil, nil
}

func (stubProvider) MetadataConfiguration(context.Context, string) (mediasvc.MetadataConfiguration, error) {
	return mediasvc.MetadataConfiguration{}, nil
}

func (stubProvider) AddMetadataConfiguration(context.Context, string, string) error { return nil }

func (stubProvider) RemoveMetadataConfiguration(context.Context, string) error { return nil }

func (stubProvider) SetMetadataConfiguration(context.Context, mediasvc.MetadataConfiguration) error {
	return nil
}

func (stubProvider) CompatibleMetadataConfigurations(context.Context, string) ([]mediasvc.MetadataConfiguration, error) {
	return nil, nil
}

func (stubProvider) MetadataConfigurationOptions(context.Context, string, string) (mediasvc.MetadataConfigurationOptions, error) {
	return mediasvc.MetadataConfigurationOptions{}, nil
}

// ---------- helpers ----------

func envelopeFor(op string) string {
	return `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">` +
		`<s:Body><trt:` + op + ` xmlns:trt="` + mediasvc.MediaNamespace + `"/></s:Body></s:Envelope>`
}

func newAuthenticatedHandler(t *testing.T, users ...auth.UserRecord) http.Handler {
	t.Helper()
	store := auth.NewMutableUserStore(users)
	digest := auth.NewDigestAuthenticator(store, auth.DigestOptions{Realm: "onvif"})
	ut := auth.NewUsernameTokenAuthenticator(store, auth.UsernameTokenOptions{})
	hook := auth.NewOperationAuthorizer(
		auth.NewChain(digest, ut),
		auth.DefaultPolicy(),
		auth.MapOperationClass(auth.MediaOperationClasses),
	)
	return mediasvc.NewHandler(stubProvider{}, mediasvc.WithAuthHook(
		mediasvc.AuthFunc(hook),
	))
}

// ---------- tests ----------

func TestMediaAuthPreAuthAllowed(t *testing.T) {
	h := newAuthenticatedHandler(t, auth.UserRecord{
		Username: "admin", Password: "secret", Roles: []string{auth.OnvifRoleAdministrator},
	})
	// GetServiceCapabilities is PreAuth.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, mediasvc.MediaServicePath,
		bytes.NewBufferString(envelopeFor("GetServiceCapabilities")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("PreAuth status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestMediaAuthChallengeOnMissingCredentials(t *testing.T) {
	h := newAuthenticatedHandler(t, auth.UserRecord{
		Username: "admin", Password: "secret", Roles: []string{auth.OnvifRoleAdministrator},
	})
	// GetProfiles is ReadMedia → requires credentials.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, mediasvc.MediaServicePath,
		bytes.NewBufferString(envelopeFor("GetProfiles")))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401; body=%s", rec.Code, rec.Body.String())
	}
	if chal := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(chal, "Digest ") {
		t.Fatalf("expected Digest challenge, got %q", chal)
	}
}

// TestMediaAuthEmptyBodyDigestProbe covers the 2-pass HTTP Digest probe:
// an unparseable body with no Authorization header must produce a
// 401 + WWW-Authenticate challenge.
func TestMediaAuthEmptyBodyDigestProbe(t *testing.T) {
	h := newAuthenticatedHandler(t, auth.UserRecord{
		Username: "admin", Password: "secret", Roles: []string{auth.OnvifRoleAdministrator},
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		mediasvc.MediaServicePath, bytes.NewBufferString(""))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401; body=%s", rec.Code, rec.Body.String())
	}
	if chal := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(chal, "Digest ") {
		t.Fatalf("expected Digest challenge, got %q", chal)
	}
}

// TestMediaAuthEmptyBodyWithAuthHeaderReturns400 confirms the probe
// fallback only fires when the Authorization header is absent.
func TestMediaAuthEmptyBodyWithAuthHeaderReturns400(t *testing.T) {
	h := newAuthenticatedHandler(t, auth.UserRecord{
		Username: "admin", Password: "secret", Roles: []string{auth.OnvifRoleAdministrator},
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		mediasvc.MediaServicePath, bytes.NewBufferString(""))
	req.Header.Set("Authorization", `Digest username="x"`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestMediaAuthDigestSuccessOnGetProfiles(t *testing.T) {
	h := newAuthenticatedHandler(t, auth.UserRecord{
		Username: "viewer", Password: "p", Roles: []string{auth.OnvifRoleUser},
	})
	body := envelopeFor("GetProfiles")

	probe := httptest.NewRequestWithContext(context.Background(), http.MethodPost, mediasvc.MediaServicePath, bytes.NewBufferString(body))
	probeRec := httptest.NewRecorder()
	h.ServeHTTP(probeRec, probe)
	nonce := extractQuoted(probeRec.Header().Get("WWW-Authenticate"), "nonce")
	if nonce == "" {
		t.Fatalf("missing nonce: %q", probeRec.Header().Get("WWW-Authenticate"))
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, mediasvc.MediaServicePath, bytes.NewBufferString(body))
	req.Header.Set("Authorization", buildDigestHeader(t, auth.DigestMD5, "onvif", "viewer", "p",
		http.MethodPost, mediasvc.MediaServicePath, nonce, "00000001", "c"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestMediaAuthUserCannotActuate(t *testing.T) {
	// ReadMedia allows User, but CreateProfile is Actuate (Operator+).
	h := newAuthenticatedHandler(t, auth.UserRecord{
		Username: "viewer", Password: "p", Roles: []string{auth.OnvifRoleUser},
	})
	body := envelopeFor("CreateProfile")

	probe := httptest.NewRequestWithContext(context.Background(), http.MethodPost, mediasvc.MediaServicePath, bytes.NewBufferString(body))
	probeRec := httptest.NewRecorder()
	h.ServeHTTP(probeRec, probe)
	nonce := extractQuoted(probeRec.Header().Get("WWW-Authenticate"), "nonce")
	if nonce == "" {
		t.Fatalf("missing nonce")
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, mediasvc.MediaServicePath, bytes.NewBufferString(body))
	req.Header.Set("Authorization", buildDigestHeader(t, auth.DigestMD5, "onvif", "viewer", "p",
		http.MethodPost, mediasvc.MediaServicePath, nonce, "00000001", "c"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for User invoking Actuate op, got %d; body=%s", rec.Code, rec.Body.String())
	}
}

func TestMediaAuthOperatorCanActuate(t *testing.T) {
	h := newAuthenticatedHandler(t, auth.UserRecord{
		Username: "ops", Password: "p", Roles: []string{auth.OnvifRoleOperator},
	})
	body := envelopeFor("CreateProfile")

	probe := httptest.NewRequestWithContext(context.Background(), http.MethodPost, mediasvc.MediaServicePath, bytes.NewBufferString(body))
	probeRec := httptest.NewRecorder()
	h.ServeHTTP(probeRec, probe)
	nonce := extractQuoted(probeRec.Header().Get("WWW-Authenticate"), "nonce")
	if nonce == "" {
		t.Fatalf("missing nonce")
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, mediasvc.MediaServicePath, bytes.NewBufferString(body))
	req.Header.Set("Authorization", buildDigestHeader(t, auth.DigestMD5, "onvif", "ops", "p",
		http.MethodPost, mediasvc.MediaServicePath, nonce, "00000001", "c"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Operator status = %d; body=%s", rec.Code, rec.Body.String())
	}
}

// extractQuoted pulls key="..." (or key=bare) out of a header parameter list.
func extractQuoted(header, key string) string {
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
