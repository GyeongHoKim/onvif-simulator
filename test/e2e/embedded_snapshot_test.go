//go:build e2e

// embedded_snapshot_test.go verifies that GetSnapshotUri on a simulator
// configured with a local mp4 (MediaFilePath) returns a URI pointing at the
// simulator's own HTTP server, and that the URI actually serves a JPEG.
//
// The test skips when:
//   - GetSnapshotUri returns a URL whose host is not the simulator (the
//     profile is using an explicit SnapshotURI override, i.e. external
//     pass-through — not what this test exercises).
//   - GetSnapshotUri itself errors (snapshot capability disabled).
package e2e

import (
	"bytes"
	"context"
	"crypto/md5" //nolint:gosec // RFC 2617 Digest
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	media "github.com/use-go/onvif/media"
	sdkmedia "github.com/use-go/onvif/sdk/media"
)

func TestEmbeddedSnapshotEndpoint(t *testing.T) {
	dev := newDevice(t)
	c, cancel := context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	prof := mediaFirstProfile(t, c, dev)
	resp, err := sdkmedia.Call_GetSnapshotUri(c, dev, media.GetSnapshotUri{
		ProfileToken: prof.Token,
	})
	if err != nil {
		t.Skipf("GetSnapshotUri unsupported on this simulator: %v", err)
	}
	snapURI := strings.TrimSpace(string(resp.MediaUri.Uri))
	if snapURI == "" {
		t.Fatal("GetSnapshotUri returned empty URI")
	}

	parsed, err := url.Parse(snapURI)
	if err != nil {
		t.Fatalf("parse %q: %v", snapURI, err)
	}
	host := envOrDefault("ONVIF_HOST", "localhost:8080")
	deviceHost := strings.SplitN(host, ":", 2)[0]
	if !strings.Contains(parsed.Host, deviceHost) && !strings.HasPrefix(parsed.Host, "127.") {
		t.Skipf(
			"GetSnapshotUri %q does not point at the simulator (%s) — "+
				"likely a pass-through override; embedded snapshot test skipped",
			snapURI, deviceHost,
		)
	}
	if !strings.HasSuffix(parsed.Path, ".jpg") {
		t.Fatalf("expected .jpg URI, got %s", snapURI)
	}

	body := fetchSnapshotJPEG(t, snapURI)
	if !bytes.HasPrefix(body, []byte{0xFF, 0xD8, 0xFF}) {
		t.Fatalf("body does not start with JPEG SOI: % x", body[:4])
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("jpeg.DecodeConfig: %v", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		t.Fatalf("decoded JPEG has zero dimensions: %dx%d", cfg.Width, cfg.Height)
	}
}

// fetchSnapshotJPEG GETs uri and returns the body. When auth is enabled it
// performs a Digest challenge/response round-trip.
func fetchSnapshotJPEG(t *testing.T, uri string) []byte {
	t.Helper()

	username := envOrDefault("ONVIF_USERNAME", "admin")
	password := envOrDefault("ONVIF_PASSWORD", "")
	client := &http.Client{Timeout: defaultTimeout}

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", uri, err)
	}
	if resp.StatusCode == http.StatusUnauthorized {
		challenge := resp.Header.Get("WWW-Authenticate")
		_ = resp.Body.Close() //nolint:errcheck // closing for retry
		req.Header.Set("Authorization",
			digestAuth(t, http.MethodGet, uri, username, password, challenge))
		resp, err = client.Do(req)
		if err != nil {
			t.Fatalf("GET %s (after auth): %v", uri, err)
		}
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // best-effort
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d", uri, resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want image/jpeg", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return body
}

// digestAuth constructs an RFC 2617 Digest Authorization header value for
// the given challenge. Supports algorithm=MD5 with qop=auth — what the
// simulator emits by default.
func digestAuth(t *testing.T, method, uri, user, pass, challenge string) string {
	t.Helper()

	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatalf("parse uri: %v", err)
	}
	digestPath := parsed.RequestURI()

	fields := parseDigestChallenge(challenge)
	realm, nonce, qop := fields["realm"], fields["nonce"], fields["qop"]
	if realm == "" || nonce == "" {
		t.Fatalf("malformed challenge %q", challenge)
	}

	cnonceBytes := make([]byte, 8)
	if _, err := rand.Read(cnonceBytes); err != nil {
		t.Fatalf("rand: %v", err)
	}
	cnonce := hex.EncodeToString(cnonceBytes)
	nc := "00000001"

	ha1 := md5sum(user + ":" + realm + ":" + pass)
	ha2 := md5sum(method + ":" + digestPath)
	var response string
	if qop == "" {
		response = md5sum(ha1 + ":" + nonce + ":" + ha2)
	} else {
		response = md5sum(strings.Join([]string{ha1, nonce, nc, cnonce, "auth", ha2}, ":"))
	}

	parts := []string{
		fmt.Sprintf("username=%q", user),
		fmt.Sprintf("realm=%q", realm),
		fmt.Sprintf("nonce=%q", nonce),
		fmt.Sprintf("uri=%q", digestPath),
		fmt.Sprintf("response=%q", response),
		"algorithm=MD5",
	}
	if qop != "" {
		parts = append(parts,
			"qop=auth",
			fmt.Sprintf("nc=%s", nc),
			fmt.Sprintf("cnonce=%q", cnonce),
		)
	}
	return "Digest " + strings.Join(parts, ", ")
}

func parseDigestChallenge(header string) map[string]string {
	out := map[string]string{}
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(strings.ToLower(header), "digest ") {
		return out
	}
	body := header[len("Digest "):]
	for body != "" {
		eq := strings.IndexByte(body, '=')
		if eq < 0 {
			break
		}
		key := strings.TrimSpace(body[:eq])
		body = body[eq+1:]
		var val string
		if strings.HasPrefix(body, `"`) {
			end := strings.IndexByte(body[1:], '"')
			if end < 0 {
				break
			}
			val = body[1 : 1+end]
			body = body[1+end+1:]
		} else if comma := strings.IndexByte(body, ','); comma < 0 {
			val = body
			body = ""
		} else {
			val = body[:comma]
			body = body[comma:]
		}
		out[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(val)
		body = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(body), ","))
	}
	return out
}

func md5sum(s string) string {
	h := md5.Sum([]byte(s)) //nolint:gosec // RFC 2617 Digest mandates MD5
	return hex.EncodeToString(h[:])
}
