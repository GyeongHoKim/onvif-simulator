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

Use `make` targets rather than calling `go`, `golangci-lint`, or `wails` directly — they encode the correct output paths, build tags, and defaults.

| Task | Command |
| --- | --- |
| Build CLI/TUI binary | `make cli` |
| Build GUI binary (requires Wails CLI) | `make gui` |
| Format | `make format` |
| Lint | `make lint` |
| Unit tests (race detector) | `make test` |
| Coverage | `make coverage` |
| E2E suite (speaks SOAP against a running simulator) | `make e2e` |
| One-time repo setup (hooks + commitlint) | `make setup` |

Run a single Go test: `go test -race -run TestName ./internal/<pkg>/...`.

**Code quality gate (required after every code change):** run `make format` followed by `make lint` before handing the task back. Do not skip either step.

`make e2e` honors `ONVIF_HOST`, `ONVIF_USERNAME`, `ONVIF_PASSWORD` — point them at a running simulator.

Toolchain versions are pinned in `mise.toml`. Run `mise install` after cloning.

GUI frontend lives in `frontend/` (React + Vite + Tailwind, shadcn registry) and is hosted by Wails. `wails dev` starts the dev harness; Wails invokes the frontend build for production. The TUI is a Bubble Tea program and has no web assets.

## Architecture

Responsibilities are split into tightly separated layers. Folder names are intentionally omitted here — some of today's packages are placeholders and will be reorganized. Locate the layers by role with the symbol/grep tools.

- **Configuration** — owns the on-disk configuration schema (device identity, network, media profiles, auth settings). Saves are atomic and validation must pass before any write. Mutations are exposed as targeted, field-level operations so callers never rewrite the whole file.

- **Auth** — the shared authentication and authorization primitives every ONVIF service handler consumes.
  - Authentication is a **pluggable scheme chain**. HTTP-level schemes evaluate before WS-level schemes (ONVIF Core §5.9.1). Missing credentials fall through to the next scheme; any other failure aborts the chain and produces a challenge response that handlers copy onto the HTTP reply.
  - Authorization applies the ONVIF access-class / user-level matrix (ONVIF Core §5.9.4). Unknown operations default to the most restrictive class.
  - The runtime user store is thread-safe and live-editable. A single controller keeps it in sync with the persisted config — callers never mutate either side directly.

- **ONVIF service handlers** — one per ONVIF service (Device Management, Media, Events, PTZ, Imaging, …). Each is a **pure dispatcher**: domain data comes from an injected data provider, authorization from an injected auth hook, and the handler itself only parses SOAP envelopes, enforces a request-size cap, and maps SOAP faults to HTTP status codes.

- **WS-Discovery** — message encoding/decoding (Hello, Bye, Probe/ProbeMatch, Resolve/ResolveMatch), scope matching, and UDP multicast transport. Discovery Proxy is out of scope.

- **Simulator lifecycle + front-ends** — the composition root that assembles the layers above into a runnable simulator, plus the GUI, TUI, and CLI surfaces. These exist as stubs today and will be wired up later.

Adding a new ONVIF service means creating a sibling handler alongside the existing ones with its own data provider and auth hook, and reusing the shared authorization primitive via a one-line adapter.

Authoritative ONVIF specifications and WSDLs are vendored in-tree — find them by searching for `*.wsdl` / the ONVIF spec PDFs and treat them as the primary reference when adding or changing service behavior.

## Git

The repo uses GitButler; the working branch is usually `gitbutler/workspace`. The `pre-commit` hook blocks direct `git commit` on that branch — use `but commit` (or the GitButler app) instead. The `but` skill wraps all common git operations.
