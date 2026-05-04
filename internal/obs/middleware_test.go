package obs

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequestMiddlewareAddsRequestID(t *testing.T) {
	t.Parallel()
	// Use a tempfile sink so we can read the encoded JSON line — With-attached
	// fields (request_id, remote_addr, …) only show up in the formatted
	// output, not in raw slog.Record.Attrs.
	dir := t.TempDir()
	path := filepath.Join(dir, "rid.log")
	logger, state, err := Build(Config{Level: "debug", File: path})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	t.Cleanup(func() { _ = state.Close() }) //nolint:errcheck // best-effort flush in tests

	var seenRID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenRID = RequestIDFromContext(r.Context())
		l := LoggerFromContext(r.Context())
		if l == nil {
			t.Error("LoggerFromContext returned nil")
		}
		w.WriteHeader(http.StatusOK)
	})

	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/test", http.NoBody)
	RequestMiddleware(inner, logger).ServeHTTP(rr, req)

	if seenRID == "" {
		t.Error("middleware did not attach request id to context")
	}
	if rr.Header().Get(RequestIDHeader) != seenRID {
		t.Errorf("response header %q != ctx id %q", rr.Header().Get(RequestIDHeader), seenRID)
	}
	if closeErr := state.Close(); closeErr != nil {
		t.Fatalf("Close: %v", closeErr)
	}
	contents, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	out := string(contents)
	if !strings.Contains(out, `"msg":"soap_request"`) {
		t.Errorf("summary log missing: %q", out)
	}
	if !strings.Contains(out, `"request_id":"`+seenRID+`"`) {
		t.Errorf("summary log missing request_id=%q: %q", seenRID, out)
	}
}

func TestRequestMiddlewareEchoesIncomingID(t *testing.T) {
	t.Parallel()
	logger := Discard()
	want := "abc-123"
	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", http.NoBody)
	req.Header.Set(RequestIDHeader, want)

	var got string
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = RequestIDFromContext(r.Context())
	})
	RequestMiddleware(inner, logger).ServeHTTP(rr, req)
	if got != want {
		t.Errorf("RequestIDFromContext=%q want %q", got, want)
	}
	if rr.Header().Get(RequestIDHeader) != want {
		t.Errorf("response header=%q want %q", rr.Header().Get(RequestIDHeader), want)
	}
}

func TestRequestMiddlewareLevelByStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		status    int
		wantLevel slog.Level
	}{
		{http.StatusOK, slog.LevelDebug},
		{http.StatusBadRequest, slog.LevelInfo},
		{http.StatusInternalServerError, slog.LevelWarn},
	}
	for _, tc := range cases {
		captured := &captureHandler{}
		logger, state, err := Build(Config{Level: "debug", File: noFileSentinel, Extras: []slog.Handler{captured}})
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		t.Cleanup(func() { _ = state.Close() }) //nolint:errcheck // best-effort flush in tests

		inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
		})
		rr := httptest.NewRecorder()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/", http.NoBody)
		RequestMiddleware(inner, logger).ServeHTTP(rr, req)

		if got := captured.Count(); got != 1 {
			t.Fatalf("status=%d: count=%d want 1", tc.status, got)
		}
		if got := captured.Records()[0].Level; got != tc.wantLevel {
			t.Errorf("status=%d: level=%v want %v", tc.status, got, tc.wantLevel)
		}
	}
}

func TestRequestMiddlewareNilLoggerSafe(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", http.NoBody)
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	// Must not panic.
	RequestMiddleware(inner, nil).ServeHTTP(rr, req)
}

func TestLoggerFromContextDefaultsToDiscard(t *testing.T) {
	t.Parallel()
	l := LoggerFromContext(context.Background())
	if l == nil {
		t.Fatal("LoggerFromContext returned nil")
	}
	if l.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("default logger should be discard (not enabled)")
	}
}

func TestLoggerFromContextOr(t *testing.T) {
	t.Parallel()
	fallback := slog.New(slog.DiscardHandler).With("role", "fallback")
	if got := LoggerFromContextOr(context.Background(), fallback); got != fallback {
		t.Fatal("without middleware ctx, should use fallback")
	}
	if got := LoggerFromContextOr(context.Background(), nil); got == nil {
		t.Fatal("nil fallback should behave like Discard(), not nil")
	}

	scoped := slog.New(slog.DiscardHandler).With("role", "scoped")
	ctx := WithLogger(context.Background(), scoped)
	if got := LoggerFromContextOr(ctx, fallback); got != scoped {
		t.Fatal("scoped logger in ctx should win over fallback")
	}
}

func TestWithLoggerRoundTrip(t *testing.T) {
	t.Parallel()
	want := slog.New(slog.DiscardHandler).With("k", "v")
	ctx := WithLogger(context.Background(), want)
	if got := LoggerFromContext(ctx); got != want {
		t.Errorf("LoggerFromContext mismatch")
	}
}

func TestRequestMiddlewareRecorderWritePath(t *testing.T) {
	t.Parallel()
	captured := &captureHandler{}
	logger, state, buildErr := Build(Config{Level: "debug", File: noFileSentinel, Extras: []slog.Handler{captured}})
	if buildErr != nil {
		t.Fatalf("Build: %v", buildErr)
	}
	t.Cleanup(func() { _ = state.Close() }) //nolint:errcheck // best-effort flush in tests

	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Write without explicit WriteHeader — should default to 200.
		_, _ = w.Write([]byte("hello")) //nolint:errcheck // body is recorded by httptest
	})
	rr := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", http.NoBody)
	RequestMiddleware(inner, logger).ServeHTTP(rr, req)
	if rr.Body.String() != "hello" {
		t.Errorf("body=%q", rr.Body.String())
	}
}
