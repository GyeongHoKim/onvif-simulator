package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // SHA-1 is required by WS-Security UsernameToken digest computation.
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	errUTMissingDigestFields = errors.New("auth: UsernameToken PasswordDigest requires nonce and created")
	errUTUnknownPasswordType = errors.New("auth: unknown password type")
	errUTPasswordTextDenied  = errors.New("auth: PasswordText disabled")
	errUTInvalidCreated      = errors.New("auth: invalid Created timestamp")
	errUTInvalidTimestamp    = errors.New("auth: unrecognized timestamp")
)

// UsernameTokenOptions configures WS-Security UsernameToken validation.
type UsernameTokenOptions struct {
	MaxClockSkew      time.Duration
	NonceCache        *NonceCache
	AllowPasswordText bool
	Clock             func() time.Time
}

// WS-Security UsernameToken password type URIs. These are public OASIS
// identifiers, not credentials.
//
//nolint:gosec // OASIS URI, not a credential.
const wssUsernameTokenProfile = "http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0"

var (
	passwordDigest = wssUsernameTokenProfile + "#PasswordDigest"
	passwordText   = wssUsernameTokenProfile + "#PasswordText"
)

type usernameTokenAuth struct {
	store       UserStore
	skew        time.Duration
	cache       *NonceCache
	allowText   bool
	clock       func() time.Time
	readBodyCap int64
}

// NewUsernameTokenAuthenticator parses the SOAP envelope body of r and
// validates a WS-Security UsernameToken header if present.
func NewUsernameTokenAuthenticator(store UserStore, opts UsernameTokenOptions) Authenticator {
	skew := opts.MaxClockSkew
	if skew <= 0 {
		skew = 5 * time.Minute
	}
	cache := opts.NonceCache
	if cache == nil {
		cache = NewNonceCache(2 * skew)
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &usernameTokenAuth{
		store:       store,
		skew:        skew,
		cache:       cache,
		allowText:   opts.AllowPasswordText,
		clock:       clock,
		readBodyCap: 1 << 20, // 1 MiB is plenty for a SOAP header scan
	}
}

func (u *usernameTokenAuth) Authenticate(ctx context.Context, r *http.Request) (*Principal, error) {
	p, err := u.authenticate(ctx, r)
	if err == nil || errors.Is(err, ErrNoCredentials) {
		return p, err
	}
	// Hard validation failure: surface as a ChallengeError carrying the ONVIF
	// NotAuthorized subcode so the SOAP fault tells WS-Security clients
	// (gSOAP-based VMS, etc.) that authentication is the problem.
	var alreadyWrapped *ChallengeError
	if errors.As(err, &alreadyWrapped) {
		return nil, alreadyWrapped
	}
	return nil, NewChallengeError(err, http.StatusUnauthorized, nil, OnvifFaultNotAuthorized)
}

func (u *usernameTokenAuth) authenticate(ctx context.Context, r *http.Request) (*Principal, error) {
	tok, err := u.readToken(r)
	if err != nil {
		return nil, err
	}
	if terr := u.verifyTimestamp(tok); terr != nil {
		return nil, terr
	}
	rec, err := u.store.Lookup(ctx, tok.Username)
	if err != nil {
		if errors.Is(err, ErrUnknownUser) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}
	if verr := u.verifyPassword(tok, rec); verr != nil {
		return nil, verr
	}
	return &Principal{
		Username: rec.Username,
		Method:   MethodUsernameToken,
		Roles:    append([]string(nil), rec.Roles...),
	}, nil
}

// readToken pulls the SOAP body from r, extracts the first UsernameToken, and
// restores r.Body so downstream handlers can re-read it. It returns
// ErrNoCredentials if the envelope lacks a UsernameToken.
func (u *usernameTokenAuth) readToken(r *http.Request) (*usernameToken, error) {
	if r.Body == nil {
		return nil, ErrNoCredentials
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, u.readBodyCap))
	if err != nil {
		return nil, fmt.Errorf("auth: read soap body: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	if !bytes.Contains(body, []byte("UsernameToken")) {
		return nil, ErrNoCredentials
	}
	tok, err := extractUsernameToken(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidCredentials, err)
	}
	if tok == nil {
		return nil, ErrNoCredentials
	}
	return tok, nil
}

func (u *usernameTokenAuth) verifyTimestamp(tok *usernameToken) error {
	if tok.Created == "" {
		return nil
	}
	created, err := parseWSTimestamp(tok.Created)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidCredentials, errUTInvalidCreated)
	}
	if absDuration(u.clock().Sub(created)) > u.skew {
		return ErrClockSkew
	}
	return nil
}

func (u *usernameTokenAuth) verifyPassword(tok *usernameToken, rec *UserRecord) error {
	switch strings.TrimSpace(tok.PasswordType) {
	case "", passwordDigest:
		if tok.Nonce == "" || tok.Created == "" {
			return fmt.Errorf("%w: %w", ErrInvalidCredentials, errUTMissingDigestFields)
		}
		if !u.cache.Add(tok.Nonce+"|"+tok.Created, u.clock()) {
			return ErrReplayedNonce
		}
		if !verifyPasswordDigest(tok.Password, tok.Nonce, tok.Created, rec.Password) {
			return ErrInvalidCredentials
		}
		return nil
	case passwordText:
		if !u.allowText {
			return fmt.Errorf("%w: %w", ErrInvalidCredentials, errUTPasswordTextDenied)
		}
		if !hmac.Equal([]byte(tok.Password), []byte(rec.Password)) {
			return ErrInvalidCredentials
		}
		return nil
	default:
		return fmt.Errorf("%w: %w %q", ErrInvalidCredentials, errUTUnknownPasswordType, tok.PasswordType)
	}
}

// usernameToken mirrors the fields we need from wsse:UsernameToken.
type usernameToken struct {
	Username     string
	Password     string
	PasswordType string
	Nonce        string
	Created      string
}

// extractUsernameToken pulls the first <wsse:UsernameToken> out of a SOAP
// envelope. Returns nil (no error) if the envelope has no UsernameToken.
func extractUsernameToken(body []byte) (*usernameToken, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	var tok *usernameToken
	for {
		t, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return tok, nil
		}
		if err != nil {
			return nil, err
		}
		start, ok := t.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "UsernameToken" {
			continue
		}
		// Unknown namespace is tolerated: some clients omit the xmlns decl
		// on intermediate elements. The enclosing wsse:Security element still
		// authoritatively identifies the schema.
		tok = &usernameToken{}
		if err := decodeUsernameTokenBody(decoder, start, tok); err != nil {
			return nil, err
		}
		return tok, nil
	}
}

func decodeUsernameTokenBody(decoder *xml.Decoder, start xml.StartElement, tok *usernameToken) error {
	for {
		t, err := decoder.Token()
		if err != nil {
			return err
		}
		switch el := t.(type) {
		case xml.StartElement:
			var text string
			if err := decoder.DecodeElement(&text, &el); err != nil {
				return err
			}
			text = strings.TrimSpace(text)
			switch el.Name.Local {
			case "Username":
				tok.Username = text
			case "Password":
				tok.Password = text
				for _, a := range el.Attr {
					if a.Name.Local == "Type" {
						tok.PasswordType = a.Value
					}
				}
			case "Nonce":
				tok.Nonce = text
			case "Created":
				tok.Created = text
			}
		case xml.EndElement:
			if el.Name.Local == start.Name.Local {
				return nil
			}
		}
	}
}

// verifyPasswordDigest computes Base64(SHA1(nonce + created + password)).
// The nonce in the token is already Base64; the spec says it must be decoded
// before hashing.
func verifyPasswordDigest(givenDigest, nonceB64, created, password string) bool {
	nonce, err := base64.StdEncoding.DecodeString(nonceB64)
	if err != nil {
		return false
	}
	h := sha1.New() //nolint:gosec // SHA-1 required by WS-Security UsernameToken spec.
	h.Write(nonce)
	h.Write([]byte(created))
	h.Write([]byte(password))
	computed := base64.StdEncoding.EncodeToString(h.Sum(nil))
	return hmac.Equal([]byte(givenDigest), []byte(computed))
}

func parseWSTimestamp(s string) (time.Time, error) {
	// WS-Security uses RFC 3339 / ISO 8601; Go's RFC 3339 supports the common forms.
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("%w: %q", errUTInvalidTimestamp, s)
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
