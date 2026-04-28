package auth

import (
	"context"
	"crypto/hmac"
	"crypto/md5" //nolint:gosec // MD5 is required by RFC 2617 Digest authentication (ONVIF Core §5.9.3).
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"net/http"
	"strings"
	"time"
)

// DigestAlgorithm names the hash used for HA1/HA2/response computation.
type DigestAlgorithm string

const (
	// DigestMD5 is RFC 2617 MD5 digest.
	DigestMD5 DigestAlgorithm = "MD5"
	// DigestSHA256 is RFC 7616 SHA-256 digest.
	DigestSHA256 DigestAlgorithm = "SHA-256"
)

// DigestOptions configures HTTP Digest authentication.
type DigestOptions struct {
	Realm      string
	Algorithms []DigestAlgorithm
	NonceTTL   time.Duration
	// NonceSecret is used to HMAC-sign nonces so forged nonces are rejected.
	// If empty, a random 32-byte secret is generated at construction time.
	NonceSecret []byte
	Clock       func() time.Time
}

const (
	defaultDigestRealm    = "onvif-simulator"
	defaultDigestNonceTTL = 5 * time.Minute

	digestNonceSecretLen = 32
	digestNonceTSLen     = 8
	digestNonceSigLen    = 16

	defaultDigestHeaderCap = 8
)

var (
	errDigestHeaderShape  = errors.New("auth: malformed Digest header")
	errDigestMissingField = errors.New("auth: missing required Digest field")
)

type digestAuth struct {
	store      UserStore
	realm      string
	algorithms []DigestAlgorithm
	nonceTTL   time.Duration
	nonceKey   []byte
	clock      func() time.Time
}

// NewDigestAuthenticator parses Authorization: Digest headers and validates
// the response against records from store.
//
//nolint:gocritic // DigestOptions is passed by value for API ergonomics; callers build it as a literal at call sites.
func NewDigestAuthenticator(store UserStore, opts DigestOptions) Authenticator {
	realm := opts.Realm
	if realm == "" {
		realm = defaultDigestRealm
	}
	algs := opts.Algorithms
	if len(algs) == 0 {
		algs = []DigestAlgorithm{DigestMD5}
	}
	ttl := opts.NonceTTL
	if ttl <= 0 {
		ttl = defaultDigestNonceTTL
	}
	key := opts.NonceSecret
	if len(key) == 0 {
		key = make([]byte, digestNonceSecretLen)
		if _, err := rand.Read(key); err != nil {
			panic(fmt.Errorf("auth: generate digest nonce secret: %w", err))
		}
	}
	clock := opts.Clock
	if clock == nil {
		clock = time.Now
	}
	return &digestAuth{
		store:      store,
		realm:      realm,
		algorithms: algs,
		nonceTTL:   ttl,
		nonceKey:   key,
		clock:      clock,
	}
}

func (d *digestAuth) Authenticate(ctx context.Context, r *http.Request) (*Principal, error) {
	header := r.Header.Get("Authorization")
	if strings.TrimSpace(header) == "" {
		// No Authorization at all — emit a challenge so the 401 has
		// WWW-Authenticate headers even if other authenticators in the
		// Chain report plain ErrNoCredentials.
		return nil, d.challenge(ErrNoCredentials, false)
	}
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(header)), "digest ") {
		// Some other scheme (Bearer, Basic, ...): defer to sibling authenticators.
		return nil, ErrNoCredentials
	}
	fields, err := parseDigestHeader(header)
	if err != nil {
		return nil, d.challenge(fmt.Errorf("%w: %w", ErrInvalidCredentials, err), false)
	}
	alg := DigestAlgorithm(strings.ToUpper(fields["algorithm"]))
	if alg == "" {
		alg = DigestMD5
	}
	if !d.algorithmSupported(alg) {
		return nil, d.challenge(fmt.Errorf("%w: unsupported algorithm %q", ErrInvalidCredentials, alg), false)
	}
	expired, valid := d.verifyNonce(fields["nonce"])
	if !valid {
		return nil, d.challenge(ErrInvalidCredentials, false)
	}
	if expired {
		return nil, d.challenge(ErrStaleNonce, true)
	}
	rec, err := d.store.Lookup(ctx, fields["username"])
	if err != nil {
		if errors.Is(err, ErrUnknownUser) {
			return nil, d.challenge(ErrInvalidCredentials, false)
		}
		return nil, err
	}
	ha1 := digestHash(alg, rec.Username+":"+d.realm+":"+rec.Password)
	ha2 := digestHash(alg, r.Method+":"+fields["uri"])
	var want string
	qop := fields["qop"]
	switch qop {
	case "":
		want = digestHash(alg, ha1+":"+fields["nonce"]+":"+ha2)
	case "auth":
		want = digestHash(alg, strings.Join([]string{
			ha1, fields["nonce"], fields["nc"], fields["cnonce"], qop, ha2,
		}, ":"))
	default:
		return nil, d.challenge(fmt.Errorf("%w: unsupported qop %q", ErrInvalidCredentials, qop), false)
	}
	if !hmac.Equal([]byte(want), []byte(strings.ToLower(fields["response"]))) {
		return nil, d.challenge(ErrInvalidCredentials, false)
	}
	return &Principal{
		Username: rec.Username,
		Method:   MethodDigest,
		Roles:    append([]string(nil), rec.Roles...),
	}, nil
}

func (d *digestAuth) algorithmSupported(alg DigestAlgorithm) bool {
	for _, a := range d.algorithms {
		if strings.EqualFold(string(a), string(alg)) {
			return true
		}
	}
	return false
}

// challenge wraps err in a ChallengeError carrying WWW-Authenticate headers
// for each configured algorithm. When stale=true, the challenge marks the
// current nonce as stale so compliant clients reuse their credentials.
func (d *digestAuth) challenge(err error, stale bool) error {
	h := http.Header{}
	for _, alg := range d.algorithms {
		nonce := d.mintNonce(d.clock())
		parts := []string{
			fmt.Sprintf("realm=%q", d.realm),
			fmt.Sprintf("qop=%q", "auth"),
			fmt.Sprintf("nonce=%q", nonce),
			fmt.Sprintf("algorithm=%s", alg),
		}
		if stale {
			parts = append(parts, `stale=true`)
		}
		h.Add("WWW-Authenticate", "Digest "+strings.Join(parts, ", "))
	}
	return NewChallengeError(err, http.StatusUnauthorized, h, OnvifFaultNotAuthorized)
}

// mintNonce produces a nonce of the form base64(ts||sig) where ts is a
// unix-nano big-endian int64 and sig = HMAC-SHA256(secret, ts)[:16].
func (d *digestAuth) mintNonce(now time.Time) string {
	ts := make([]byte, digestNonceTSLen)
	binary.BigEndian.PutUint64(ts, uint64(now.UnixNano()))
	sig := d.signNonce(ts)
	raw := make([]byte, 0, len(ts)+len(sig))
	raw = append(raw, ts...)
	raw = append(raw, sig...)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// verifyNonce returns (expired, valid).
func (d *digestAuth) verifyNonce(nonce string) (expired, valid bool) {
	raw, err := base64.RawURLEncoding.DecodeString(nonce)
	if err != nil || len(raw) != digestNonceTSLen+digestNonceSigLen {
		return false, false
	}
	ts, sig := raw[:digestNonceTSLen], raw[digestNonceTSLen:]
	want := d.signNonce(ts)
	if !hmac.Equal(sig, want) {
		return false, false
	}
	// nonce timestamps are encoded from time.UnixNano() which returns int64;
	// the round-trip through uint64 is lossless for any clock value we care about.
	issued := time.Unix(0, int64(binary.BigEndian.Uint64(ts))) //nolint:gosec // lossless round-trip; see above.
	if d.clock().Sub(issued) > d.nonceTTL {
		return true, true
	}
	return false, true
}

func (d *digestAuth) signNonce(ts []byte) []byte {
	mac := hmac.New(sha256.New, d.nonceKey)
	mac.Write(ts)
	return mac.Sum(nil)[:digestNonceSigLen]
}

func digestHash(alg DigestAlgorithm, s string) string {
	var h hash.Hash
	switch alg {
	case DigestSHA256:
		h = sha256.New()
	default:
		h = md5.New() //nolint:gosec // MD5 required by RFC 2617 Digest.
	}
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

// parseDigestHeader extracts key=value pairs from an Authorization: Digest header.
// It handles quoted values and allows unquoted tokens for numeric fields (nc, qop, algorithm).
func parseDigestHeader(value string) (map[string]string, error) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(strings.ToLower(trimmed), "digest ") {
		return nil, errDigestHeaderShape
	}
	body := trimmed[len("Digest "):]
	out := make(map[string]string, defaultDigestHeaderCap)
	for body != "" {
		eq := strings.IndexByte(body, '=')
		if eq < 0 {
			return nil, fmt.Errorf("%w: missing = near %q", errDigestHeaderShape, body)
		}
		key := strings.TrimSpace(body[:eq])
		body = body[eq+1:]
		var val string
		if strings.HasPrefix(body, `"`) {
			end := strings.IndexByte(body[1:], '"')
			if end < 0 {
				return nil, fmt.Errorf("%w: unterminated quoted value for %q", errDigestHeaderShape, key)
			}
			val = body[1 : 1+end]
			body = body[1+end+1:]
		} else {
			comma := strings.IndexByte(body, ',')
			if comma < 0 {
				val = body
				body = ""
			} else {
				val = body[:comma]
				body = body[comma:]
			}
			val = strings.TrimSpace(val)
		}
		out[strings.ToLower(key)] = val
		body = strings.TrimSpace(body)
		body = strings.TrimPrefix(body, ",")
		body = strings.TrimSpace(body)
	}
	if out["username"] == "" || out["response"] == "" || out["nonce"] == "" || out["uri"] == "" {
		return nil, errDigestMissingField
	}
	return out, nil
}
