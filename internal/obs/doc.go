// Package obs is the simulator's observability seam: it provides the
// file-only JSON logging stack that the simulator composes and every
// long-lived component (auth, event broker, ONVIF service handlers, RTSP
// server, WS-Discovery, simulator lifecycle) accepts via a WithLogger option.
//
// The simulator owns the single root *slog.Logger and its backing file sink:
// it calls Build using LoggingConfig from onvif-simulator.json (merged with
// runtime overrides), keeps the returned State for hot-reload, and applies
// LoggingConfig edits live via State.Apply. Log records go to that
// simulator-managed JSON file only—not to stdout or stderr.
//
// Front-ends (CLI, TUI, GUI) do not construct root loggers or call Build for
// production logging; they only forward Options.LogLevel and Options.LogFile
// (and optional LogExtras for tests or bridges) into simulator.Options.
//
// Components never build loggers themselves; they receive an injected
// *slog.Logger (often a child via slog.With) or fall back to Discard when nil
// so unit tests need no wiring.
//
// State returned by Build owns the active sink stack. Apply rebuilds level and
// sinks atomically so existing *slog.Logger pointers continue to work—used when
// LoggingConfig changes on disk.
package obs
