# AGENTS.md

This file provides guidance to coding agents when working with code in this repository.

It describes **patterns, boundaries, and conventions** — not concrete implementations. Use the symbol/grep tools to discover file names, type names, and constants; they are not documented here on purpose, because they rot.

## Project

A software simulator of an ONVIF device. Three interchangeable front-ends are offered so the same simulator core can be driven by different users:

- **GUI** — for human operators; built with Wails (Go backend + web frontend).
- **TUI** — for human operators in a terminal; built with Bubble Tea.
- **CLI** — for scripting and AI-agent-driven usage; plain flags/subcommands, non-interactive.

All three share the same `internal/` core; none of them owns simulator logic.

## Common commands

Use `just` recipes rather than calling `go`, `golangci-lint`, or `wails` directly — they encode the correct output paths, build tags, and defaults. The legacy `Makefile` is a deprecation wrapper that forwards to the same `just` recipes; prefer `just <recipe>` for new work.

| Task | Command |
| --- | --- |
| Build CLI/TUI binary | `just cli` |
| Build GUI binary (requires Wails CLI) | `just gui` |
| Build CLI for Raspberry Pi (arm + arm64) | `just cli-rpi` |
| Cross-compile rpicam-tagged code (no fetch) | `just rpicam-build-check` |
| Format | `just format` |
| Lint | `just lint` |
| Unit tests (race detector) | `just test` |
| Coverage | `just coverage` |
| E2E suite (speaks SOAP against a running simulator) | `just e2e` |
| Browse Go doc in browser | `just manual` / `DOCS_PORT=3000 just manual` |
| One-time repo setup (hooks + commitlint) | `just setup` |

Run a single Go test: `go test -race -run TestName ./internal/<pkg>/...`.

**Code quality gate (required after every code change):** run `just format` followed by `just lint` before handing the task back. Do not skip either step.

`just e2e` honors `ONVIF_HOST`, `ONVIF_USERNAME`, `ONVIF_PASSWORD` — point them at a running simulator.

Toolchain versions are pinned in `mise.toml` (including `just` itself). Run `mise install` after cloning.

GUI frontend lives in `internal/gui/frontend/` (React + Vite + Tailwind, shadcn registry), is built by npm into `internal/gui/frontend/dist`, and is hosted by Wails from `cmd/gui`. Run `wails dev` inside `cmd/gui` for the dev harness; Wails invokes the frontend build for production. The TUI is a Bubble Tea program in `internal/tui` with no web assets; launch it with `onvif-simulator tui` (same binary as the CLI).

## Build channels

The repo ships two Go binary build channels:

- **default** — built with `just cli` and goreleaser build id `cli`. Runs on linux/darwin/windows × amd64/arm64. No platform-specific embedded assets. Produces `onvif-simulator`.
- **rpi** — built with `just cli-rpi` and goreleaser build id `cli-rpi`. Runs on linux/arm (Pi Zero/2/3 32-bit) and linux/arm64 (Pi 3/4/5 64-bit). The build pipeline fetches `mtxrpicam_32.tar.gz` and `mtxrpicam_64.tar.gz` from the pinned mediamtx-rpicamera release (a separate repo from mediamtx itself) into `internal/rpicamera/mtxrpicam_{32,64}/` and `go build -tags rpicam` embeds them via `//go:embed`. Produces `onvif-simulator-rpi-arm` / `onvif-simulator-rpi-arm64`. The default channel never carries this asset and `kind=rpicam` profiles fail fast with `rpicamera.ErrUnsupported` on non-rpi builds.

The `rpicam` build tag controls every rpicam runtime path. PR CI does not download the binary — it cross-compiles `-tags rpicam` against a 1-byte placeholder file checked into each `mtxrpicam_*` directory (`just rpicam-build-check`). The pinned mediamtx-rpicamera version lives in `Justfile`'s `mtxrpicam_version`; tarball checksums in `scripts/mtxrpicam.sha256` are bumped in lockstep so the fetch step (`scripts/fetch-mtxrpicam.sh`) refuses unverified blobs. Those hashes are cross-checkable against mediamtx's own `internal/staticsources/rpicamera/mtxrpicamdownloader/HASH_MTXRPICAM_*_TAR_GZ`, giving an out-of-band verification source independent of the release page itself. GUI is intentionally CLI/TUI-only on the rpi channel — there is no `just gui-rpi` and there will not be one.

## Architecture

Responsibilities are split into tightly separated layers. Folder names are intentionally omitted here — some of today's packages are placeholders and will be reorganized. Locate the layers by role with the symbol/grep tools.

- **Configuration** — owns the on-disk configuration schema (`onvif-simulator.json`). The root `Config` struct contains:
  - `DeviceConfig` — static device identity (UUID, manufacturer, model, serial, scopes).
  - `NetworkConfig` — HTTP port, **RTSP port** for the embedded RTSP listener (0 means use the default 8554 via `RTSPPortOrDefault`), optional bind `interface`, and WS-Discovery XAddr list.
  - `MediaConfig` — ONVIF media **profiles**. Each profile declares a **`kind`** (default `"file"`, also `"rpicam"`). For `kind=file`, **`media_file_path`** points at a local **H.264/H.265 MP4** the simulator loops; width, height, FPS, and encoding are **probed from the file at startup** and published in the live config snapshot (persisted JSON may still carry prior values for those fields). For `kind=rpicam`, **`rpicam`** carries capture parameters (camera_id, width, height, fps, bitrate, idr_period, hflip/vflip, sharpness/contrast/brightness/saturation) and the simulator captures live H.264 from a Raspberry Pi camera through the embedded `mtxrpicam` helper; this kind is only available on binaries built with the `rpicam` tag (the dedicated Pi build channel — see *Build channels* below). Optional **`snapshot_uri`** (HTTP(S) URL returned by `GetSnapshotUri`; the process does not render snapshots itself), optional **metadata** configuration entries, and **`MaxVideoEncoderInstances`** are profile-level fields shared by both kinds.
  - `AuthConfig` — authentication switch, user credentials, Digest and JWT tuning.
  - `RuntimeConfig` — device state that ONVIF Device Management Set* operations mutate at runtime (discovery mode, hostname, DNS, default gateway, network protocols, system date/time). Persisted so the simulator retains the last applied values across restarts.
  - `EventsConfig` — event service parameters (max pull points, max notification producers, default subscription timeout, topic list). Pull-point and push subscriptions count against separate capacity caps per ONVIF Core §9.5; setting max notification producers to zero disables WS-BaseNotification push entirely. Each `TopicConfig` entry has an `Enabled` flag; disabled topics are hidden from `GetEventProperties` but still routable by the broker.
  - Saves are atomic (write-to-temp + rename) and validation must pass before any write. Mutations are exposed as targeted, field-level helpers (e.g. `config.SetDiscoveryMode`, `config.SetTopicEnabled`) so callers never rewrite the whole file.

- **Auth** — the shared authentication and authorization primitives every ONVIF service handler consumes.
  - Authentication is a **pluggable scheme chain**. HTTP-level schemes evaluate before WS-level schemes (ONVIF Core §5.9.1). Missing credentials fall through to the next scheme; any other failure aborts the chain and produces a challenge response that handlers copy onto the HTTP reply.
  - Authorization applies the ONVIF access-class / user-level matrix (ONVIF Core §5.9.4). Unknown operations default to the most restrictive class.
  - The runtime user store is thread-safe and live-editable. A single controller keeps it in sync with the persisted config — callers never mutate either side directly.

- **ONVIF service handlers** — one per ONVIF service (Device Management, Media, Events, PTZ, Imaging, …). Each is a **pure dispatcher**: domain data comes from an injected data provider, authorization from an injected auth hook, and the handler itself only parses SOAP envelopes, enforces a request-size cap, and maps SOAP faults to HTTP status codes.

- **Event Broker** (`internal/event`) — the concrete `eventsvc.Provider`. It manages both pull-point and WS-BaseNotification push subscriptions in one keyed-by-UUID registry. Pull-point subscriptions hold an in-memory event queue; push subscriptions hold a consumer EPR and, on Publish, fan out a Notify SOAP POST to that consumer asynchronously so a slow consumer cannot block the publish path. Per-subscription consecutive-failure tracking force-expires unresponsive push consumers once a configurable threshold is reached. Subscription expiry runs lazily on access and proactively via a background goroutine. GUI/TUI code calls typed helpers on the broker to publish events without touching raw XML:
  - `broker.MotionAlarm(sourceToken, state)` — `tns1:VideoSource/MotionAlarm`
  - `broker.ImageTooBlurry / ImageTooDark / ImageTooBright` — image quality alerts
  - `broker.DigitalInput(inputToken, logicalState)` — `tns1:Device/Trigger/DigitalInput`
  - `broker.SyncProperty(...)` — re-emit "Initialized" for any topic after `SetSynchronizationPoint`
  - `broker.Publish(topic, rawXML)` — low-level escape hatch for topics without a typed helper
  - `broker.UpdateConfig(BrokerConfig)` — hot-swap max-pull-points, timeout, and topic list without restart

- **WS-Discovery** — message encoding/decoding (Hello, Bye, Probe/ProbeMatch, Resolve/ResolveMatch), scope matching, and UDP multicast transport. Discovery Proxy is out of scope.

- **Embedded RTSP** (`internal/rtsp`) — in-process RTSP server (gortsplib) on **`NetworkConfig`'s RTSP port**. It starts when **at least one** profile has a usable source — `kind=file` with non-empty `media_file_path`, **or** `kind=rpicam` with `rpicam` configured. Each such profile is served at **`rtsp://<advertised-host>:<port>/<profile-token>`**. **`GetStreamUri`** always returns that form of URI for this device even when no source is registered (clients then see no media on that path). Sources implement the small `rtsp.Source` interface (`Describe`/`AttachStream`/`Ready`/`Run`); file-backed sources loop the MP4 (`looper`), live sources push access units through `LiveSource`. `Ready` lets the live path hold `DESCRIBE` until the first IDR so clients never see a pre-keyframe SDP. Video tracks are limited to **H.264 and H.265** packetization today.

- **Raspberry Pi camera** (`internal/rpicamera`) — vendored and adapted from mediamtx's [rpicamera static source](https://github.com/bluenviron/mediamtx/tree/main/internal/staticsources/rpicamera). On a binary built with the `rpicam` tag (linux/arm or linux/arm64), `rpicamera.Open` extracts the embedded `mtxrpicam` helper into `/dev/shm`, fork+execs it, and surfaces H.264 access units through an `OnData` callback. On every other build the package compiles to a disabled stub whose `Open` returns `rpicamera.ErrUnsupported` so the simulator can fail with a clear "this build does not support the Raspberry Pi camera" message. Capture parameters live in `rpicamera.Params`; the upstream wire-format struct stays internal so the public surface tracks only what the simulator's `MediaConfig.RPICam` exposes.

- **Simulator lifecycle + front-ends** — `internal/simulator` is the composition root: it starts the HTTP server (ONVIF SOAP), optional embedded RTSP, WS-Discovery, and the event broker. **CLI** entrypoint is **`cmd/cli`** (`serve` by default, plus `tui`, `config`, `event`). **GUI** entrypoint is **`cmd/gui`** (Wails app in **`internal/gui`**). **TUI** is **`internal/tui`**, driven from the CLI **`tui`** subcommand.

- **Observability** — every long-lived component (auth, event broker, ONVIF service handlers, RTSP server, WS-Discovery listener, simulator lifecycle) accepts a `*slog.Logger` via a `WithLogger` functional option. Nil falls back to a discard logger so unit tests can construct components without any wiring. The composition root creates a single root logger and derives child loggers via `.With("component", "<name>")`. SOAP HTTP endpoints are wrapped in a request middleware that attaches a request id, a request-scoped logger, and emits one summary log line per request (level scaled by HTTP status). The logger is **owned by the simulator**, not the front-ends: when constructed without an explicit logger, the simulator reads `LoggingConfig` from the on-disk config and builds a single **file-only JSON sink** (defaulting to a path under `os.UserCacheDir`) plus an internal hot-reload State that the reload path mutates whenever `LoggingConfig` changes. Front-ends only forward optional flag/env overrides through `Options.LogLevel` / `Options.LogFile` and stay otherwise oblivious. There is no stderr/stdout sink: stdout is reserved for user-facing CLI program output, and stderr is only ever written by pre-logger fatal paths. Tests that need to capture records inject an `slog.Handler` via `Options.LogExtras`.

Adding a new ONVIF service means creating a sibling handler alongside the existing ones with its own data provider, auth hook, and `WithLogger` option, and reusing the shared authorization primitive via a one-line adapter.

Authoritative ONVIF specifications and WSDLs are vendored in-tree — find them by searching for `*.wsdl` / the ONVIF spec PDFs and treat them as the primary reference when adding or changing service behavior.

## Git

The repo uses GitButler; the working branch is usually `gitbutler/workspace`. The `pre-commit` hook blocks direct `git commit` on that branch — use `but commit` (or the GitButler app) instead. The `but` skill wraps all common git operations.
