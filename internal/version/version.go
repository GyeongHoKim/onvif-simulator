// Package version exposes the build-time identifiers injected via -ldflags
// (see .goreleaser.yml's `cli` and `cli-rpi` build entries). Callers print
// these values from the `onvif-simulator version` subcommand and reference
// them in user-agent strings; nothing in the simulator alters them at
// runtime.
package version

// Version is the goreleaser tag (e.g. "v0.4.1"). "unknown" on local builds.
var Version = "unknown"

// Commit is the git commit SHA captured at build time. "unknown" on local builds.
var Commit = "unknown"

// Date is the build timestamp captured at build time. "unknown" on local builds.
var Date = "unknown"

// RPICam reports whether this binary was built with the rpicam tag (the Pi
// build channel) so the version subcommand can show it. The default value is
// false; the rpicam-tagged build replaces it via init in version_rpicam.go.
var RPICam = false
