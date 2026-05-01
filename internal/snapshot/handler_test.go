package snapshot_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
	"github.com/GyeongHoKim/onvif-simulator/internal/snapshot"
)

var errFakeNoCreds = errors.New("no creds")

func TestHandler_ServesCachedJPEG(t *testing.T) {
	t.Parallel()

	body := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	h := snapshot.NewHandler(
		func(token string) ([]byte, bool) {
			if token != "main" {
				return nil, false
			}
			return body, true
		},
		func(context.Context, string, *http.Request) error { return nil },
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, snapshot.PathFor("main"), http.NoBody)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want image/jpeg", got)
	}
	got, err := io.ReadAll(rec.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !bytes.Equal(got, body) {
		t.Fatalf("body mismatch: got %x, want %x", got, body)
	}
}

func TestHandler_HEADSuppressesBody(t *testing.T) {
	t.Parallel()

	body := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	h := snapshot.NewHandler(
		func(string) ([]byte, bool) { return body, true },
		func(context.Context, string, *http.Request) error { return nil },
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodHead, snapshot.PathFor("main"), http.NoBody)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD wrote body: %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Length"); got != "4" {
		t.Fatalf("Content-Length = %q, want 4", got)
	}
}

func TestHandler_MethodNotAllowed(t *testing.T) {
	t.Parallel()

	h := snapshot.NewHandler(
		func(string) ([]byte, bool) { return nil, false },
		func(context.Context, string, *http.Request) error { return nil },
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, snapshot.PathFor("main"), http.NoBody)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("Allow = %q, want GET, HEAD", got)
	}
}

func TestHandler_NotFound(t *testing.T) {
	t.Parallel()

	h := snapshot.NewHandler(
		func(string) ([]byte, bool) { return nil, false },
		func(context.Context, string, *http.Request) error { return nil },
	)

	cases := []string{
		"/onvif/snapshot/missing.jpg", // valid shape but lookup miss
		"/onvif/snapshot/",            // empty token
		"/onvif/snapshot/main.png",    // wrong extension
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			t.Parallel()
			rec := httptest.NewRecorder()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, p, http.NoBody)
			h.ServeHTTP(rec, req)
			if rec.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want 404", rec.Code)
			}
		})
	}
}

func TestHandler_AuthChallengeEmits401WithHeaders(t *testing.T) {
	t.Parallel()

	hdr := http.Header{}
	hdr.Set("WWW-Authenticate", `Digest realm="onvif-simulator", nonce="abc"`)
	challenge := auth.NewChallengeError(
		errFakeNoCreds, http.StatusUnauthorized, hdr, auth.OnvifFaultNotAuthorized,
	)
	h := snapshot.NewHandler(
		func(string) ([]byte, bool) { return []byte{0xFF, 0xD8, 0xFF, 0xE0}, true },
		func(context.Context, string, *http.Request) error { return challenge },
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, snapshot.PathFor("main"), http.NoBody)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got == "" {
		t.Fatalf("missing WWW-Authenticate header")
	}
}

func TestHandler_AuthForbiddenEmits403(t *testing.T) {
	t.Parallel()

	h := snapshot.NewHandler(
		func(string) ([]byte, bool) { return []byte{0xFF, 0xD8, 0xFF, 0xE0}, true },
		func(context.Context, string, *http.Request) error { return auth.ErrForbidden },
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, snapshot.PathFor("main"), http.NoBody)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
}

func TestHandler_AuthGenericErrorEmits500(t *testing.T) {
	t.Parallel()

	h := snapshot.NewHandler(
		func(string) ([]byte, bool) { return []byte{0xFF, 0xD8, 0xFF, 0xE0}, true },
		func(context.Context, string, *http.Request) error { return errFakeNoCreds },
	)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, snapshot.PathFor("main"), http.NoBody)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestNewHandler_PanicsOnNilLookup(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on nil lookup")
		}
	}()
	snapshot.NewHandler(nil, func(context.Context, string, *http.Request) error { return nil })
}

func TestNewHandler_PanicsOnNilHook(t *testing.T) {
	t.Parallel()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on nil hook")
		}
	}()
	snapshot.NewHandler(func(string) ([]byte, bool) { return nil, false }, nil)
}
