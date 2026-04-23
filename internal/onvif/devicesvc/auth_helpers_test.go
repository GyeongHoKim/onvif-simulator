package devicesvc_test

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"hash"
	"strings"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
)

// buildDigestHeader reproduces the RFC 2617 / 7616 response computation so
// the integration tests can forge Authorization: Digest headers without
// relying on an unexported helper from the auth package.
func buildDigestHeader(t *testing.T, alg auth.DigestAlgorithm, realm, username, password, method, uri, nonce, nc, cnonce string) string {
	t.Helper()
	hasher := func(s string) string {
		var h hash.Hash
		switch alg {
		case auth.DigestSHA256:
			h = sha256.New()
		default:
			h = md5.New()
		}
		h.Write([]byte(s))
		return hex.EncodeToString(h.Sum(nil))
	}
	ha1 := hasher(username + ":" + realm + ":" + password)
	ha2 := hasher(method + ":" + uri)
	resp := hasher(strings.Join([]string{ha1, nonce, nc, cnonce, "auth", ha2}, ":"))
	return strings.Join([]string{
		`Digest username="` + username + `"`,
		`realm="` + realm + `"`,
		`nonce="` + nonce + `"`,
		`uri="` + uri + `"`,
		`qop=auth`,
		`nc=` + nc,
		`cnonce="` + cnonce + `"`,
		`response="` + resp + `"`,
		`algorithm=` + string(alg),
	}, ", ")
}
