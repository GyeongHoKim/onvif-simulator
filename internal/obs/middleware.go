package obs

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"time"
)

// RequestIDHeader is the HTTP header used to propagate request IDs in and out
// of the simulator. Set on the response by RequestMiddleware so clients can
// correlate logs with their own request traces.
const RequestIDHeader = "X-Request-ID"

const (
	requestIDByteLen = 8
	statusServerErr  = 500
	statusClientErr  = 400
)

type contextKey int

const (
	loggerCtxKey contextKey = iota
	requestIDCtxKey
)

// LoggerFromContext returns the request-scoped logger if RequestMiddleware
// has run on this request, otherwise Discard. Service handlers should prefer
// this over their handler-scoped logger so per-request fields (request_id,
// remote_addr) are present on every emitted record.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerCtxKey).(*slog.Logger); ok && l != nil {
		return l
	}
	return Discard()
}

// LoggerFromContextOr returns the request-scoped logger if one is attached
// to ctx, otherwise fallback. Service handlers wired with WithLogger but
// also reachable without obs.RequestMiddleware (e.g. unit tests) use this to
// pick request-scoped attributes when present and the handler-scoped logger
// otherwise.
func LoggerFromContextOr(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if l, ok := ctx.Value(loggerCtxKey).(*slog.Logger); ok && l != nil {
		return l
	}
	if fallback != nil {
		return fallback
	}
	return Discard()
}

// RequestIDFromContext returns the request id assigned by RequestMiddleware,
// or "" when there is none.
func RequestIDFromContext(ctx context.Context) string {
	if s, ok := ctx.Value(requestIDCtxKey).(string); ok {
		return s
	}
	return ""
}

// WithLogger attaches logger to ctx so LoggerFromContext can retrieve it.
// Useful for tests and for callers that wire scoped loggers without going
// through RequestMiddleware.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerCtxKey, logger)
}

// RequestMiddleware wraps next so every request gets:
//   - a request ID (taken from RequestIDHeader or generated) echoed back on
//     the response
//   - a request-scoped logger reachable via LoggerFromContext, pre-populated
//     with request_id, remote_addr, method, path
//   - one summary log line emitted after the response is written, level
//     adjusted by status code (debug for 2xx/3xx, info for 4xx, warn for 5xx)
//
// The summary line uses the message "soap_request" so all front-ends can
// filter consistently.
func RequestMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	if next == nil {
		panic("obs: RequestMiddleware requires a non-nil handler")
	}
	if logger == nil {
		logger = Discard()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := r.Header.Get(RequestIDHeader)
		if rid == "" {
			rid = newRequestID()
		}
		w.Header().Set(RequestIDHeader, rid)

		scoped := logger.With(
			slog.String("request_id", rid),
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
		)
		ctx := context.WithValue(r.Context(), loggerCtxKey, scoped)
		ctx = context.WithValue(ctx, requestIDCtxKey, rid)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		defer func() {
			dur := time.Since(start)
			level := slog.LevelDebug
			switch {
			case rec.status >= statusServerErr:
				level = slog.LevelWarn
			case rec.status >= statusClientErr:
				level = slog.LevelInfo
			}
			scoped.LogAttrs(ctx, level, "soap_request",
				slog.Int("status", rec.status),
				slog.String("soap_action", r.Header.Get("SOAPAction")),
				slog.Duration("duration", dur),
			)
		}()
		next.ServeHTTP(rec, r.WithContext(ctx))
	})
}

func newRequestID() string {
	var b [requestIDByteLen]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand.Read is documented never to fail on supported
		// platforms; if it does, fall back to a deterministic placeholder
		// so the log line still carries a non-empty id rather than panicking.
		return "0000000000000000"
	}
	return hex.EncodeToString(b[:])
}

// statusRecorder captures the HTTP status code the inner handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(code int) {
	if !s.wrote {
		s.status = code
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wrote {
		s.wrote = true
	}
	return s.ResponseWriter.Write(b)
}
