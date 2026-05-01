package auth_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
)

var errStubShouldNotRun = errors.New("test: stub authenticator should not run")

type fixedAuth struct {
	p   *auth.Principal
	err error
}

func (f fixedAuth) Authenticate(context.Context, *http.Request) (*auth.Principal, error) {
	return f.p, f.err
}

type fixedPolicy struct{ allow bool }

func (f fixedPolicy) Allow(*auth.Principal, auth.AccessClass) bool { return f.allow }

func newEmptyReq() *http.Request {
	return httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
}

func TestOperationAuthorizerPreAuthBypassesAuth(t *testing.T) {
	t.Parallel()
	hook := auth.NewOperationAuthorizer(
		fixedAuth{err: errStubShouldNotRun},
		fixedPolicy{allow: false},
		auth.MapOperationClass(map[string]auth.AccessClass{"Hello": auth.ClassPreAuth}),
	)
	if err := hook(context.Background(), "Hello", newEmptyReq()); err != nil {
		t.Fatalf("PreAuth must bypass auth: %v", err)
	}
}

func TestOperationAuthorizerAuthFailure(t *testing.T) {
	t.Parallel()
	hook := auth.NewOperationAuthorizer(
		fixedAuth{err: auth.ErrInvalidCredentials},
		fixedPolicy{allow: true},
		auth.MapOperationClass(map[string]auth.AccessClass{"X": auth.ClassReadSystem}),
	)
	err := hook(context.Background(), "X", newEmptyReq())
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected auth error, got %v", err)
	}
}

func TestOperationAuthorizerForbidden(t *testing.T) {
	t.Parallel()
	hook := auth.NewOperationAuthorizer(
		fixedAuth{p: &auth.Principal{Username: "bob"}},
		fixedPolicy{allow: false},
		auth.MapOperationClass(map[string]auth.AccessClass{"X": auth.ClassWriteSystem}),
	)
	err := hook(context.Background(), "X", newEmptyReq())
	if !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("expected ErrForbidden, got %v", err)
	}
	// The forbidden error must be wrapped in a ChallengeError carrying
	// HTTP 403 and the ONVIF OperationProhibited subcode so the service
	// handler emits an ONVIF-compliant SOAP fault.
	var ce *auth.ChallengeError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ChallengeError, got %T", err)
	}
	if ce.Status != http.StatusForbidden {
		t.Fatalf("Status = %d, want 403", ce.Status)
	}
	if ce.Subcode != auth.OnvifFaultOperationProhibited {
		t.Fatalf("Subcode = %q, want %q", ce.Subcode, auth.OnvifFaultOperationProhibited)
	}
}

func TestOperationAuthorizerUnknownOpDefaultsToWriteSystem(t *testing.T) {
	t.Parallel()
	hook := auth.NewOperationAuthorizer(
		fixedAuth{p: &auth.Principal{Username: "u", Roles: []string{auth.OnvifRoleUser}}},
		auth.DefaultPolicy(),
		auth.MapOperationClass(map[string]auth.AccessClass{}),
	)
	err := hook(context.Background(), "Unknown", newEmptyReq())
	if !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("expected ErrForbidden for unknown op, got %v", err)
	}
}
