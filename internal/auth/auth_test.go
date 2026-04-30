package auth_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
)

var (
	errTestBadCredential = errors.New("test: bad credential")
	errTestBoom          = errors.New("test: boom")
)

func TestChainFallsThroughNoCredentials(t *testing.T) {
	t.Parallel()
	a := auth.AuthenticatorFunc(func(context.Context, *http.Request) (*auth.Principal, error) {
		return nil, auth.ErrNoCredentials
	})
	b := auth.AuthenticatorFunc(func(context.Context, *http.Request) (*auth.Principal, error) {
		return &auth.Principal{Username: "u", Method: auth.MethodDigest}, nil
	})
	chain := auth.NewChain(a, b)
	p, err := chain.Authenticate(context.Background(), newEmptyReq())
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if p.Username != "u" {
		t.Fatalf("principal: %+v", p)
	}
}

func TestChainStopsOnHardError(t *testing.T) {
	t.Parallel()
	a := auth.AuthenticatorFunc(func(context.Context, *http.Request) (*auth.Principal, error) {
		return nil, errTestBadCredential
	})
	b := auth.AuthenticatorFunc(func(context.Context, *http.Request) (*auth.Principal, error) {
		t.Error("should not be called after hard error")
		return nil, errTestBoom
	})
	chain := auth.NewChain(a, b)
	_, err := chain.Authenticate(context.Background(), newEmptyReq())
	if !errors.Is(err, errTestBadCredential) {
		t.Fatalf("expected hard error, got %v", err)
	}
}

func TestChainReturnsNoCredentialsWhenAllEmpty(t *testing.T) {
	t.Parallel()
	noCreds := auth.AuthenticatorFunc(func(context.Context, *http.Request) (*auth.Principal, error) {
		return nil, auth.ErrNoCredentials
	})
	chain := auth.NewChain(noCreds, noCreds)
	_, err := chain.Authenticate(context.Background(), newEmptyReq())
	if !errors.Is(err, auth.ErrNoCredentials) {
		t.Fatalf("expected ErrNoCredentials, got %v", err)
	}
}

func TestChallengeErrorUnwrap(t *testing.T) {
	t.Parallel()
	headers := http.Header{}
	headers.Set("WWW-Authenticate", `Digest realm="test"`)
	ce := auth.NewChallengeError(errTestBoom, 0, headers, auth.OnvifFaultNotAuthorized)
	if ce.Status != http.StatusUnauthorized {
		t.Fatalf("default status want 401, got %d", ce.Status)
	}
	if !errors.Is(ce, errTestBoom) {
		t.Fatalf("errors.Is should unwrap to inner")
	}
	if ce.Error() != errTestBoom.Error() {
		t.Fatalf("unexpected Error(): %q", ce.Error())
	}
	if ce.Subcode != auth.OnvifFaultNotAuthorized {
		t.Fatalf("Subcode = %q, want %q", ce.Subcode, auth.OnvifFaultNotAuthorized)
	}
}

// TestChainAggregatesChallengeSubcode verifies that when no authenticator
// supplies credentials, the aggregated ChallengeError carries the ONVIF
// NotAuthorized subcode so the SOAP fault tells WS-Security clients (gSOAP)
// that authentication is the failure mode.
func TestChainAggregatesChallengeSubcode(t *testing.T) {
	t.Parallel()
	headers := http.Header{}
	headers.Set("WWW-Authenticate", `Digest realm="test"`)
	a := auth.AuthenticatorFunc(func(context.Context, *http.Request) (*auth.Principal, error) {
		return nil, auth.NewChallengeError(auth.ErrNoCredentials, http.StatusUnauthorized, headers, auth.OnvifFaultNotAuthorized)
	})
	chain := auth.NewChain(a)
	_, err := chain.Authenticate(context.Background(), newEmptyReq())
	var ce *auth.ChallengeError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ChallengeError, got %T: %v", err, err)
	}
	if ce.Subcode != auth.OnvifFaultNotAuthorized {
		t.Fatalf("Subcode = %q, want %q", ce.Subcode, auth.OnvifFaultNotAuthorized)
	}
	if ce.Status != http.StatusUnauthorized {
		t.Fatalf("Status = %d, want 401", ce.Status)
	}
}

func TestChainMergesCombinedHeadersIntoHardChallengeError(t *testing.T) {
	t.Parallel()
	digestHeaders := http.Header{}
	digestHeaders.Add("WWW-Authenticate", `Digest realm="test"`)
	bearerHeaders := http.Header{}
	bearerHeaders.Add("WWW-Authenticate", `Bearer realm="test"`)

	noCreds := auth.AuthenticatorFunc(func(context.Context, *http.Request) (*auth.Principal, error) {
		return nil, auth.NewChallengeError(auth.ErrNoCredentials, http.StatusUnauthorized, digestHeaders, auth.OnvifFaultNotAuthorized)
	})
	hardFail := auth.AuthenticatorFunc(func(context.Context, *http.Request) (*auth.Principal, error) {
		return nil, auth.NewChallengeError(errTestBadCredential, http.StatusUnauthorized, bearerHeaders, auth.OnvifFaultNotAuthorized)
	})

	chain := auth.NewChain(noCreds, hardFail)
	_, err := chain.Authenticate(context.Background(), newEmptyReq())
	var ce *auth.ChallengeError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ChallengeError, got %T: %v", err, err)
	}
	got := ce.Headers.Values("WWW-Authenticate")
	if len(got) != 2 {
		t.Fatalf("WWW-Authenticate count = %d, want 2 (%v)", len(got), got)
	}
	if got[0] != `Digest realm="test"` {
		t.Fatalf("first challenge = %q, want digest", got[0])
	}
	if got[1] != `Bearer realm="test"` {
		t.Fatalf("second challenge = %q, want bearer", got[1])
	}
	if !errors.Is(err, errTestBadCredential) {
		t.Fatalf("expected wrapped hard error, got %v", err)
	}
}

// TestChainDoesNotWrapOperationalErrorWhenChallengeHeadersAccumulated verifies
// that an internal error from a later authenticator is not coerced into 401
// NotAuthorized just because an earlier scheme left WWW-Authenticate challenges
// on the accumulated header map.
func TestChainDoesNotWrapOperationalErrorWhenChallengeHeadersAccumulated(t *testing.T) {
	t.Parallel()
	digestHeaders := http.Header{}
	digestHeaders.Add("WWW-Authenticate", `Digest realm="test"`)
	noCreds := auth.AuthenticatorFunc(func(context.Context, *http.Request) (*auth.Principal, error) {
		return nil, auth.NewChallengeError(auth.ErrNoCredentials, http.StatusUnauthorized, digestHeaders, auth.OnvifFaultNotAuthorized)
	})
	boom := auth.AuthenticatorFunc(func(context.Context, *http.Request) (*auth.Principal, error) {
		return nil, errTestBoom
	})
	chain := auth.NewChain(noCreds, boom)
	_, err := chain.Authenticate(context.Background(), newEmptyReq())
	if !errors.Is(err, errTestBoom) {
		t.Fatalf("expected operational error, got %v", err)
	}
	var ce *auth.ChallengeError
	if errors.As(err, &ce) {
		t.Fatalf("operational error must not become *ChallengeError, got %+v", ce)
	}
}

// TestChainMergesCombinedHeadersIntoPlainInvalidCredentials checks that a plain
// ErrInvalidCredentials (not already *ChallengeError) still merges earlier
// challenge headers when building the final NotAuthorized response.
func TestChainMergesCombinedHeadersIntoPlainInvalidCredentials(t *testing.T) {
	t.Parallel()
	digestHeaders := http.Header{}
	digestHeaders.Add("WWW-Authenticate", `Digest realm="test"`)
	noCreds := auth.AuthenticatorFunc(func(context.Context, *http.Request) (*auth.Principal, error) {
		return nil, auth.NewChallengeError(auth.ErrNoCredentials, http.StatusUnauthorized, digestHeaders, auth.OnvifFaultNotAuthorized)
	})
	bad := auth.AuthenticatorFunc(func(context.Context, *http.Request) (*auth.Principal, error) {
		return nil, auth.ErrInvalidCredentials
	})
	chain := auth.NewChain(noCreds, bad)
	_, err := chain.Authenticate(context.Background(), newEmptyReq())
	var ce *auth.ChallengeError
	if !errors.As(err, &ce) {
		t.Fatalf("expected *ChallengeError, got %T: %v", err, err)
	}
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
	got := ce.Headers.Values("WWW-Authenticate")
	if len(got) != 1 || got[0] != `Digest realm="test"` {
		t.Fatalf("WWW-Authenticate = %v, want digest challenge merged", got)
	}
}
