package snapshot

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/GyeongHoKim/onvif-simulator/internal/auth"
)

// AuthOperation is the operation name passed to the auth hook for snapshot
// requests. Registered against ClassReadMedia in auth.MediaOperationClasses.
const AuthOperation = "GetSnapshotHTTP"

// AuthHook authorizes one snapshot request before the handler serves bytes.
// Implementations should return *auth.ChallengeError when they want the
// handler to emit a 401 with WWW-Authenticate headers; any other non-nil
// error becomes a plain 500.
type AuthHook func(ctx context.Context, operation string, r *http.Request) error

// CacheLookup returns the JPEG bytes for a profile token, or false when no
// snapshot is available for that token. The simulator populates the cache at
// startup; tokens that did not have a MediaFilePath produce a miss and the
// handler returns 404.
type CacheLookup func(token string) ([]byte, bool)

// NewHandler builds an http.Handler that serves cached JPEG bytes from the
// simulator's snapshot cache. Both hooks are required; pass an always-allow
// AuthHook when running unauthenticated.
func NewHandler(lookup CacheLookup, hook AuthHook) http.Handler {
	if lookup == nil {
		panic("snapshot: lookup is nil")
	}
	if hook == nil {
		panic("snapshot: auth hook is nil")
	}
	return &handler{lookup: lookup, hook: hook}
}

type handler struct {
	lookup CacheLookup
	hook   AuthHook
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	token := TokenFromPath(r.URL.Path)
	if token == "" {
		http.NotFound(w, r)
		return
	}

	if err := h.hook(r.Context(), AuthOperation, r); err != nil {
		writeAuthError(w, err)
		return
	}

	bytes, ok := h.lookup(token)
	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.Itoa(len(bytes)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(bytes) //nolint:errcheck // client disconnect carries no info we can act on
}

// writeAuthError mirrors the mediasvc handler's 401 emission: copy
// WWW-Authenticate headers from the ChallengeError onto the response, fall
// back to 403 for ErrForbidden, and 500 for anything else (e.g. user-store
// I/O failure).
func writeAuthError(w http.ResponseWriter, err error) {
	status := http.StatusUnauthorized
	var challenge *auth.ChallengeError
	if errors.As(err, &challenge) {
		if challenge.Status != 0 {
			status = challenge.Status
		}
		for k, vs := range challenge.Headers {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
	} else if errors.Is(err, auth.ErrForbidden) {
		status = http.StatusForbidden
	} else {
		status = http.StatusInternalServerError
	}
	http.Error(w, err.Error(), status)
}
