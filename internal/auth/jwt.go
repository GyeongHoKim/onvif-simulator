package auth

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

var (
	errJWTKeyFuncRequired = errors.New("auth: JWTOptions.KeyFunc is required")
	errJWTStaticNoPEM     = errors.New("auth: NewStaticKeyFunc requires at least one PEM block")
	errJWTNoMatchingKey   = errors.New("auth: no matching key for token kid")
	errJWTNoPEMBlock      = errors.New("auth: no PEM block")
	errJWKSURLRequired    = errors.New("auth: NewJWKSKeyFunc requires a JWKS URL")
)

// JWTOptions configures JWT Bearer token validation.
type JWTOptions struct {
	Issuer        string
	Audience      string
	Algorithms    []string
	KeyFunc       jwt.Keyfunc
	ClockSkew     time.Duration
	UsernameClaim string
	RolesClaim    string
	RequireTLS    bool
	Clock         func() time.Time
}

type jwtAuth struct {
	iss           string
	aud           string
	algs          []string
	keyFn         jwt.Keyfunc
	clockSkew     time.Duration
	usernameClaim string
	rolesClaim    string
	requireTLS    bool
	clock         func() time.Time
}

// NewJWTAuthenticator returns an Authenticator that validates tokens
// presented via Authorization: Bearer, per RFC 6750 + RFC 7519.
// UserStore is intentionally not required — JWT trust is based on the
// issuer's signing keys, not on a local credential database.
//
//nolint:gocritic // JWTOptions is passed by value for API ergonomics; callers build it as a literal at call sites.
func NewJWTAuthenticator(opts JWTOptions) (Authenticator, error) {
	if opts.KeyFunc == nil {
		return nil, errJWTKeyFuncRequired
	}
	algs := opts.Algorithms
	if len(algs) == 0 {
		algs = []string{"RS256"}
	}
	usernameClaim := opts.UsernameClaim
	if usernameClaim == "" {
		usernameClaim = "sub"
	}
	rolesClaim := opts.RolesClaim
	if rolesClaim == "" {
		rolesClaim = "roles"
	}
	skew := opts.ClockSkew
	if skew <= 0 {
		skew = 30 * time.Second
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &jwtAuth{
		iss:           opts.Issuer,
		aud:           opts.Audience,
		algs:          algs,
		keyFn:         opts.KeyFunc,
		clockSkew:     skew,
		usernameClaim: usernameClaim,
		rolesClaim:    rolesClaim,
		requireTLS:    opts.RequireTLS,
		clock:         clock,
	}, nil
}

func (j *jwtAuth) Authenticate(_ context.Context, r *http.Request) (*Principal, error) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(header)), "bearer ") {
		return nil, ErrNoCredentials
	}
	if j.requireTLS && r.TLS == nil {
		return nil, bearerChallenge(ErrInsecureTransport, "invalid_request", "JWT requires TLS")
	}
	raw := strings.TrimSpace(header[len("Bearer "):])
	parser := jwt.NewParser(
		jwt.WithValidMethods(j.algs),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(j.clock),
		jwt.WithLeeway(j.clockSkew),
	)
	tok, err := parser.Parse(raw, j.keyFn)
	if err != nil {
		return nil, mapJWTParseError(err)
	}
	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return nil, bearerChallenge(ErrTokenMalformed, "invalid_token", "unsupported claim shape")
	}
	if j.iss != "" {
		//nolint:errcheck // type-assertion _: missing or non-string claim falls through to explicit mismatch check.
		iss, _ := claims["iss"].(string)
		if iss != j.iss {
			return nil, bearerChallenge(ErrIssuerMismatch, "invalid_token", fmt.Sprintf("unexpected issuer %q", iss))
		}
	}
	if j.aud != "" && !audienceMatches(claims["aud"], j.aud) {
		return nil, bearerChallenge(ErrAudienceMismatch, "invalid_token", "audience does not include expected value")
	}

	//nolint:errcheck // type-assertion _: missing or non-string claim falls through to explicit emptiness check.
	username, _ := claims[j.usernameClaim].(string)
	if username == "" {
		return nil, bearerChallenge(ErrTokenMalformed, "invalid_token", fmt.Sprintf("missing %q claim", j.usernameClaim))
	}
	roles := rolesFromClaim(claims[j.rolesClaim])
	return &Principal{
		Username: username,
		Method:   MethodJWT,
		Roles:    roles,
		Claims:   map[string]any(claims),
	}, nil
}

func audienceMatches(raw any, expected string) bool {
	switch v := raw.(type) {
	case string:
		return v == expected
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == expected {
				return true
			}
		}
	case []string:
		return slices.Contains(v, expected)
	}
	return false
}

func rolesFromClaim(raw any) []string {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []any:
		out := make([]string, 0, len(v))
		for _, it := range v {
			if s, ok := it.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), v...)
	}
	return nil
}

func mapJWTParseError(err error) error {
	switch {
	case errors.Is(err, jwt.ErrTokenExpired):
		return bearerChallenge(ErrTokenExpired, "invalid_token", "token expired")
	case errors.Is(err, jwt.ErrTokenNotValidYet):
		return bearerChallenge(ErrTokenMalformed, "invalid_token", "token not yet valid")
	case errors.Is(err, jwt.ErrTokenSignatureInvalid):
		return bearerChallenge(ErrTokenSignature, "invalid_token", "signature invalid")
	case errors.Is(err, jwt.ErrTokenMalformed):
		return bearerChallenge(ErrTokenMalformed, "invalid_token", "malformed token")
	default:
		return bearerChallenge(ErrTokenMalformed, "invalid_token", err.Error())
	}
}

// bearerChallenge wraps err in a ChallengeError with an RFC 6750 §3-compliant
// WWW-Authenticate: Bearer header.
func bearerChallenge(err error, code, description string) error {
	h := http.Header{}
	parts := []string{`realm="onvif"`}
	if code != "" {
		parts = append(parts, fmt.Sprintf(`error=%q`, code))
	}
	if description != "" {
		parts = append(parts, fmt.Sprintf(`error_description=%q`, description))
	}
	h.Set("WWW-Authenticate", "Bearer "+strings.Join(parts, ", "))
	return NewChallengeError(err, http.StatusUnauthorized, h, OnvifFaultNotAuthorized)
}

// NewStaticKeyFunc builds a jwt.Keyfunc that verifies tokens against the
// provided PEM-encoded public keys. Useful when the issuer does not expose
// a JWKS endpoint. When multiple keys are given, tokens must carry a `kid`
// header; it is used as the 0-based index into pemBlocks. A single-key
// configuration needs no kid.
func NewStaticKeyFunc(pemBlocks [][]byte) (jwt.Keyfunc, error) {
	if len(pemBlocks) == 0 {
		return nil, errJWTStaticNoPEM
	}
	keys := make([]any, 0, len(pemBlocks))
	for i, blk := range pemBlocks {
		pub, err := parsePublicKeyPEM(blk)
		if err != nil {
			return nil, fmt.Errorf("auth: pem[%d]: %w", i, err)
		}
		keys = append(keys, pub)
	}
	return func(tok *jwt.Token) (any, error) {
		if len(keys) == 1 {
			return keys[0], nil
		}
		//nolint:errcheck // type-assertion _: missing kid falls through to errJWTNoMatchingKey below.
		kid, _ := tok.Header["kid"].(string)
		for i, key := range keys {
			if kid == strconv.Itoa(i) {
				return key, nil
			}
		}
		return nil, errJWTNoMatchingKey
	}, nil
}

func parsePublicKeyPEM(raw []byte) (any, error) {
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errJWTNoPEMBlock
	}
	if block.Type == "RSA PUBLIC KEY" {
		return x509.ParsePKCS1PublicKey(block.Bytes)
	}
	return x509.ParsePKIXPublicKey(block.Bytes)
}

// NewJWKSKeyFunc builds a jwt.Keyfunc backed by a cached JWKS endpoint.
// The cache refreshes on a best-effort basis; see the underlying keyfunc
// library for tuning.
func NewJWKSKeyFunc(jwksURL string) (jwt.Keyfunc, error) {
	if jwksURL == "" {
		return nil, errJWKSURLRequired
	}
	kf, err := keyfunc.NewDefault([]string{jwksURL})
	if err != nil {
		return nil, fmt.Errorf("auth: jwks init: %w", err)
	}
	return kf.Keyfunc, nil
}
