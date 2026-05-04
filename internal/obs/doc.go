// Package obs is the simulator's observability seam: it owns the structured
// logger that every long-lived component (auth, event broker, ONVIF service
// handlers, RTSP server, WS-Discovery, simulator lifecycle) accepts via a
// WithLogger option.
//
// Components never construct a logger themselves and never write to
// stdout/stderr directly. Front-ends (CLI, TUI, GUI) build a single root
// *slog.Logger with Build, derive child loggers via the stdlib
// (root.With("component", "media")), and inject them through the simulator's
// composition root. Components that receive nil fall back to Discard so unit
// tests can construct them without any wiring.
//
// The State returned by Build owns the active sink stack. Apply mutates level
// and sinks atomically so the *slog.Logger references already injected into
// components observe the new behavior — used by the simulator to honor live
// edits to LoggingConfig in onvif-simulator.json.
//
// stdout is reserved for user-facing CLI program output; the logger only
// writes to stderr (toggleable) and to the optional log file.
package obs
