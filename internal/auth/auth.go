// Package auth implements the authentication and authorization layer shared
// by all ONVIF service handlers.
//
// Three authentication schemes are supported:
//
//   - HTTP Digest (RFC 2617 / RFC 7616): Authorization: Digest ... header.
//   - WS-UsernameToken: <wsse:Security><wsse:UsernameToken> in the SOAP header.
//   - JWT Bearer (RFC 6750 + RFC 7519): Authorization: Bearer ... header.
//
// After successful authentication an AccessClass-based policy (ONVIF Core
// §5.9.4) decides whether the caller may invoke the requested operation.
package auth

import (
	"context"
	"errors"
	"net/http"
)

// Method indicates which scheme authenticated the caller.
type Method int

const (
	// MethodNone means the request had no credentials (or none were required).
	MethodNone Method = iota
	// MethodDigest means HTTP Digest authenticated the caller.
	MethodDigest
	// MethodUsernameToken means WS-Security UsernameToken authenticated the caller.
	MethodUsernameToken
	// MethodJWT means a JWT Bearer token authenticated the caller.
	MethodJWT
)

// String returns a human-readable method name.
func (m Method) String() string {
	switch m {
	case MethodDigest:
		return "Digest"
	case MethodUsernameToken:
		return "UsernameToken"
	case MethodJWT:
		return "JWT"
	default:
		return "None"
	}
}

// Principal is the outcome of a successful Authenticate call.
type Principal struct {
	Username string
	Method   Method
	Roles    []string
	// Claims carries the verified JWT claim set. Nil for Digest / UsernameToken.
	Claims map[string]any
}

// Authenticator verifies credentials on an incoming request.
// Implementations MUST return ErrNoCredentials when the request lacks any
// credential that this scheme recognizes, so that Chain can fall through to
// the next scheme.
type Authenticator interface {
	Authenticate(ctx context.Context, r *http.Request) (*Principal, error)
}

// AuthenticatorFunc adapts a bare function into an Authenticator.
type AuthenticatorFunc func(ctx context.Context, r *http.Request) (*Principal, error)

// Authenticate calls the underlying function.
func (f AuthenticatorFunc) Authenticate(ctx context.Context, r *http.Request) (*Principal, error) {
	return f(ctx, r)
}

// Public error values. Callers compare with errors.Is.
var (
	// ErrNoCredentials means the request had no credential for this scheme.
	ErrNoCredentials = errors.New("auth: no credentials supplied")
	// ErrInvalidCredentials means credentials were supplied but did not validate.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	// ErrStaleNonce means a Digest nonce expired; clients should retry with stale=true handling.
	ErrStaleNonce = errors.New("auth: stale nonce")
	// ErrReplayedNonce means a UsernameToken nonce+created pair was seen before.
	ErrReplayedNonce = errors.New("auth: replayed nonce")
	// ErrClockSkew means a UsernameToken Created timestamp was outside the allowed skew.
	ErrClockSkew = errors.New("auth: timestamp outside allowed skew")
	// ErrTokenMalformed means a JWT could not be parsed.
	ErrTokenMalformed = errors.New("auth: token malformed")
	// ErrTokenExpired means a JWT exp claim is in the past.
	ErrTokenExpired = errors.New("auth: token expired")
	// ErrTokenSignature means a JWT signature failed verification.
	ErrTokenSignature = errors.New("auth: token signature invalid")
	// ErrAudienceMismatch means the aud claim does not match the expected audience.
	ErrAudienceMismatch = errors.New("auth: audience mismatch")
	// ErrIssuerMismatch means the iss claim does not match the expected issuer.
	ErrIssuerMismatch = errors.New("auth: issuer mismatch")
	// ErrInsecureTransport means JWT was presented over plain HTTP with RequireTLS=true.
	ErrInsecureTransport = errors.New("auth: jwt requires tls")
	// ErrForbidden means the authenticated principal lacks the required access class.
	ErrForbidden = errors.New("auth: access forbidden for role")
)

// ChallengeError wraps an authentication error with HTTP response metadata
// (status code and headers such as WWW-Authenticate) that the service handler
// should copy into the 401 response.
type ChallengeError struct {
	Err     error
	Status  int
	Headers http.Header
}

// Error returns the wrapped error's message.
func (e *ChallengeError) Error() string {
	if e == nil || e.Err == nil {
		return "auth: challenge"
	}
	return e.Err.Error()
}

// Unwrap returns the wrapped error.
func (e *ChallengeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NewChallengeError returns a *ChallengeError with the given status and headers.
// Status defaults to 401 if zero.
func NewChallengeError(err error, status int, headers http.Header) *ChallengeError {
	if status == 0 {
		status = http.StatusUnauthorized
	}
	return &ChallengeError{Err: err, Status: status, Headers: headers}
}

// NewChain runs the provided Authenticators in order and implements the
// HTTP-level-first-then-WS-level rule from ONVIF Core §5.9.1.
//
// Ordering guideline:
//  1. Put HTTP-level authenticators (Digest, JWT) first.
//  2. Put WS-level authenticators (UsernameToken) last.
//
// If any authenticator succeeds, Chain returns its Principal. Subsequent
// authenticators are still consulted to honor the "HTTP first, then WS"
// validation rule — if an HTTP-level scheme succeeded and a WS-level scheme
// also presents credentials, the WS-level scheme is verified too and must
// also succeed. If an authenticator returns an error other than
// ErrNoCredentials, Chain returns that error immediately.
func NewChain(authenticators ...Authenticator) Authenticator {
	return &chain{authenticators: authenticators}
}

type chain struct {
	authenticators []Authenticator
}

func (c *chain) Authenticate(ctx context.Context, r *http.Request) (*Principal, error) {
	var winner *Principal
	combined := http.Header{}
	for _, a := range c.authenticators {
		p, err := a.Authenticate(ctx, r)
		if err == nil {
			if winner == nil {
				winner = p
			}
			continue
		}
		var ce *ChallengeError
		isChallenge := errors.As(err, &ce)
		if isChallenge {
			for k, vv := range ce.Headers {
				for _, v := range vv {
					combined.Add(k, v)
				}
			}
		}
		// No credentials for this scheme — keep trying the others, but
		// remember the challenge headers so the final 401 is informative.
		if errors.Is(err, ErrNoCredentials) {
			continue
		}
		// A hard validation error (bad password, stale nonce, forged token…).
		return nil, err
	}
	if winner != nil {
		return winner, nil
	}
	if len(combined) > 0 {
		return nil, NewChallengeError(ErrNoCredentials, http.StatusUnauthorized, combined)
	}
	return nil, ErrNoCredentials
}
