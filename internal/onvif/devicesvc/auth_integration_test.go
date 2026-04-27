package devicesvc_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
	"github.com/GyeongHoKim/onvif-simulator/internal/onvif/devicesvc"
)

// stubProvider implements the minimum devicesvc.Provider surface needed by
// these tests. It mirrors the production provider by returning static data.
type stubProvider struct{}

func (stubProvider) DeviceInfo(context.Context) (devicesvc.DeviceInfo, error) {
	return devicesvc.DeviceInfo{Manufacturer: "test"}, nil
}
func (stubProvider) Services(context.Context, bool) ([]devicesvc.ServiceDescriptor, error) {
	return []devicesvc.ServiceDescriptor{{Namespace: devicesvc.DeviceNamespace, XAddr: "http://x"}}, nil
}
func (stubProvider) GetServiceCapabilities(context.Context) (devicesvc.DeviceServiceCapabilities, error) {
	return devicesvc.DeviceServiceCapabilities{}, nil
}
func (stubProvider) GetCapabilities(context.Context, string) (devicesvc.CapabilitySet, error) {
	return devicesvc.CapabilitySet{}, nil
}
func (stubProvider) WsdlURL(context.Context) (string, error) { return "http://wsdl", nil }

// Discovery stubs
func (stubProvider) GetDiscoveryMode(context.Context) (devicesvc.DiscoveryInfo, error) {
	return devicesvc.DiscoveryInfo{DiscoveryMode: "Discoverable"}, nil
}
func (stubProvider) SetDiscoveryMode(context.Context, string) error { return nil }
func (stubProvider) GetScopes(context.Context) ([]devicesvc.ScopeEntry, error) {
	return nil, nil
}
func (stubProvider) SetScopes(context.Context, []string) error                { return nil }
func (stubProvider) AddScopes(context.Context, []string) error                { return nil }
func (stubProvider) RemoveScopes(context.Context, []string) ([]string, error) { return nil, nil }

// Network stubs
func (stubProvider) GetHostname(context.Context) (devicesvc.HostnameInfo, error) {
	return devicesvc.HostnameInfo{}, nil
}
func (stubProvider) SetHostname(context.Context, string) error { return nil }
func (stubProvider) GetDNS(context.Context) (devicesvc.DNSInfo, error) {
	return devicesvc.DNSInfo{}, nil
}
func (stubProvider) SetDNS(context.Context, devicesvc.DNSInfo) error { return nil }
func (stubProvider) GetNetworkInterfaces(context.Context) ([]devicesvc.NetworkInterfaceInfo, error) {
	return nil, nil
}
func (stubProvider) GetNetworkProtocols(context.Context) ([]devicesvc.NetworkProtocol, error) {
	return nil, nil
}
func (stubProvider) SetNetworkProtocols(context.Context, []devicesvc.NetworkProtocol) error {
	return nil
}
func (stubProvider) GetNetworkDefaultGateway(context.Context) (devicesvc.DefaultGatewayInfo, error) {
	return devicesvc.DefaultGatewayInfo{}, nil
}
func (stubProvider) SetNetworkDefaultGateway(context.Context, devicesvc.DefaultGatewayInfo) error {
	return nil
}

// System stubs
func (stubProvider) GetSystemDateAndTime(context.Context) (devicesvc.SystemDateAndTimeInfo, error) {
	return devicesvc.SystemDateAndTimeInfo{DateTimeType: "Manual", TZ: "UTC"}, nil
}
func (stubProvider) SetSystemDateAndTime(context.Context, devicesvc.SetSystemDateAndTimeParams) error {
	return nil
}
func (stubProvider) SetSystemFactoryDefault(context.Context, string) error { return nil }
func (stubProvider) SystemReboot(context.Context) (string, error)          { return "ok", nil }

// User stubs
func (stubProvider) GetUsers(context.Context) ([]devicesvc.UserInfo, error) {
	return []devicesvc.UserInfo{{Username: "admin", UserLevel: "Administrator"}}, nil
}
func (stubProvider) CreateUsers(context.Context, []devicesvc.UserInfo) error { return nil }
func (stubProvider) SetUser(context.Context, []devicesvc.UserInfo) error     { return nil }
func (stubProvider) DeleteUsers(context.Context, []string) error             { return nil }
func (stubProvider) SetNetworkInterfaces(context.Context, []devicesvc.NetworkInterfaceInfo) error {
	return nil
}

const envelopeGet = `<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <tds:%s xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>
  </s:Body>
</s:Envelope>`

func newAuthenticatedHandler(t *testing.T) http.Handler {
	t.Helper()
	store := auth.NewMutableUserStore([]auth.UserRecord{{
		Username: "admin", Password: "secret",
		Roles: []string{auth.OnvifRoleAdministrator},
	}})
	digest := auth.NewDigestAuthenticator(store, auth.DigestOptions{Realm: "onvif"})
	ut := auth.NewUsernameTokenAuthenticator(store, auth.UsernameTokenOptions{})
	hook := auth.NewOperationAuthorizer(
		auth.NewChain(digest, ut),
		auth.DefaultPolicy(),
		auth.MapOperationClass(auth.DeviceOperationClasses),
	)
	return devicesvc.NewHandler(stubProvider{}, devicesvc.WithAuthHook(
		devicesvc.AuthFunc(hook),
	))
}

func TestDeviceAuthPreAuthAllowed(t *testing.T) {
	h := newAuthenticatedHandler(t)
	// GetServices is PRE_AUTH per §5.9.4.3 — must succeed without credentials.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, devicesvc.DeviceServicePath,
		bytes.NewBufferString(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><tds:GetServices xmlns:tds="http://www.onvif.org/ver10/device/wsdl"><tds:IncludeCapability>false</tds:IncludeCapability></tds:GetServices></s:Body></s:Envelope>`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetServices (PreAuth) status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeviceAuthChallengeOnMissingCredentials(t *testing.T) {
	h := newAuthenticatedHandler(t)
	// GetDeviceInformation is READ_SYSTEM; without credentials must return 401 + Digest challenge.
	body := strings.Replace(envelopeGet, "%s", "GetDeviceInformation", 1)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, devicesvc.DeviceServicePath, bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401; body=%s", rec.Code, rec.Body.String())
	}
	challenge := rec.Header().Get("WWW-Authenticate")
	if !strings.HasPrefix(challenge, "Digest ") {
		t.Fatalf("expected Digest challenge header, got %q", challenge)
	}
}

func TestDeviceAuthDigestSuccess(t *testing.T) {
	h := newAuthenticatedHandler(t)
	body := strings.Replace(envelopeGet, "%s", "GetDeviceInformation", 1)

	// 1st request — capture the challenge.
	probe := httptest.NewRequestWithContext(context.Background(), http.MethodPost, devicesvc.DeviceServicePath, bytes.NewBufferString(body))
	probeRec := httptest.NewRecorder()
	h.ServeHTTP(probeRec, probe)
	chal := probeRec.Header().Get("WWW-Authenticate")
	if chal == "" {
		t.Fatal("missing challenge")
	}
	nonce := extractQuoted(chal, "nonce")
	if nonce == "" {
		t.Fatalf("no nonce in challenge: %q", chal)
	}

	// 2nd request — respond with a valid Digest header.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, devicesvc.DeviceServicePath, bytes.NewBufferString(body))
	authz := digestClient(t, auth.DigestMD5, "onvif", "admin", "secret", http.MethodPost, devicesvc.DeviceServicePath, nonce, "00000001", "c")
	req.Header.Set("Authorization", authz)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated status = %d want 200; body=%s", rec.Code, rec.Body.String())
	}
}

// TestDeviceAuthEmptyBodyDigestProbe covers the 2-pass HTTP Digest probe:
// a request with an empty body and no Authorization header must surface a
// 401 + WWW-Authenticate challenge instead of the parse-error 400, so the
// client can harvest the nonce and re-issue the request.
func TestDeviceAuthEmptyBodyDigestProbe(t *testing.T) {
	h := newAuthenticatedHandler(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		devicesvc.DeviceServicePath, bytes.NewBufferString(""))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d want 401; body=%s", rec.Code, rec.Body.String())
	}
	if chal := rec.Header().Get("WWW-Authenticate"); !strings.HasPrefix(chal, "Digest ") {
		t.Fatalf("expected Digest challenge, got %q", chal)
	}
}

// TestDeviceAuthEmptyBodyWithAuthHeaderReturns400 confirms the probe
// fallback is gated by an absent Authorization header. A malformed body
// that already carries credentials still reports the parse error.
func TestDeviceAuthEmptyBodyWithAuthHeaderReturns400(t *testing.T) {
	h := newAuthenticatedHandler(t)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		devicesvc.DeviceServicePath, bytes.NewBufferString(""))
	req.Header.Set("Authorization", `Digest username="x"`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400; body=%s", rec.Code, rec.Body.String())
	}
}

// TestDeviceUnauthenticatedHandlerEmptyBodyReturns400 confirms the probe
// fallback is also gated by an installed auth hook: with the default no-op
// auth, an unparseable body still maps to 400.
func TestDeviceUnauthenticatedHandlerEmptyBodyReturns400(t *testing.T) {
	h := devicesvc.NewHandler(stubProvider{})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
		devicesvc.DeviceServicePath, bytes.NewBufferString(""))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d want 400; body=%s", rec.Code, rec.Body.String())
	}
}

func TestDeviceAuthForbiddenReturns403(t *testing.T) {
	// Non-admin user cannot hit an unrecoverable op.
	store := auth.NewMutableUserStore([]auth.UserRecord{{
		Username: "viewer", Password: "p",
		Roles: []string{auth.OnvifRoleUser},
	}})
	digest := auth.NewDigestAuthenticator(store, auth.DigestOptions{Realm: "onvif"})
	hook := auth.NewOperationAuthorizer(digest, auth.DefaultPolicy(), auth.MapOperationClass(auth.DeviceOperationClasses))
	h := devicesvc.NewHandler(stubProvider{}, devicesvc.WithAuthHook(devicesvc.AuthFunc(hook)))

	body := strings.Replace(envelopeGet, "%s", "SetSystemFactoryDefault", 1)

	// Probe.
	probe := httptest.NewRequestWithContext(context.Background(), http.MethodPost, devicesvc.DeviceServicePath, bytes.NewBufferString(body))
	probeRec := httptest.NewRecorder()
	h.ServeHTTP(probeRec, probe)
	nonce := extractQuoted(probeRec.Header().Get("WWW-Authenticate"), "nonce")
	if nonce == "" {
		t.Fatal("missing nonce")
	}

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, devicesvc.DeviceServicePath, bytes.NewBufferString(body))
	req.Header.Set("Authorization", digestClient(t, auth.DigestMD5, "onvif", "viewer", "p", http.MethodPost, devicesvc.DeviceServicePath, nonce, "00000001", "c"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for forbidden op, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "env:Sender") {
		t.Fatalf("expected env:Sender fault code, got %s", rec.Body.String())
	}
}

// extractQuoted returns the value of key=... from a header parameter list,
// handling both quoted and bare values.
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

// digestClient is a small helper that mirrors the server-side response
// computation. Re-exported here so the integration test can build a valid
// Authorization: Digest header without importing auth internals.
func digestClient(t *testing.T, alg auth.DigestAlgorithm, realm, username, password, method, uri, nonce, nc, cnonce string) string {
	t.Helper()
	// Delegate to the exported helper in the auth package's tests by copying
	// the algorithm here. The actual computation lives in auth.digestHash,
	// which is unexported, so reproduce it inline using hashing primitives.
	return buildDigestHeader(t, alg, realm, username, password, method, uri, nonce, nc, cnonce)
}
