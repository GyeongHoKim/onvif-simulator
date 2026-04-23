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
	ce := auth.NewChallengeError(errTestBoom, 0, headers)
	if ce.Status != http.StatusUnauthorized {
		t.Fatalf("default status want 401, got %d", ce.Status)
	}
	if !errors.Is(ce, errTestBoom) {
		t.Fatalf("errors.Is should unwrap to inner")
	}
	if ce.Error() != errTestBoom.Error() {
		t.Fatalf("unexpected Error(): %q", ce.Error())
	}
}
