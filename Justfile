# OS-agnostic build/test/release entrypoints. Mirrors the legacy Makefile
# targets 1:1; the Makefile is now a thin deprecation wrapper that defers
# to these recipes.
#
# Install just:
#   mise install                            # picks up the version pinned in mise.toml
#   brew install just                       # macOS / Linuxbrew
#   scoop install just                      # Windows (scoop)
#   cargo install just                      # any platform with Rust
#
# Run `just --list` to see all recipes.

set windows-shell := ["pwsh.exe", "-NoProfile", "-Command"]

binary := "onvif-simulator"
cli_out := if os_family() == "windows" { "bin/onvif-simulator.exe" } else { "bin/onvif-simulator" }

frontend_dist := "internal/gui/frontend/dist"

# Pinned mediamtx-rpicamera release tag. mediamtx-rpicamera lives in its own
# repo (bluenviron/mediamtx-rpicamera) with versioning independent from
# mediamtx itself; mediamtx v1.18.1 internally pins this same v2.5.6 (see
# its internal/staticsources/rpicamera/mtxrpicamdownloader/VERSION). Bump
# together with scripts/mtxrpicam.sha256 so the Pi build channel ships a
# reviewable, reproducible binary blob.
mtxrpicam_version := env_var_or_default('MTXRPICAM_VERSION', 'v2.5.6')
rpicam_dir_32 := "internal/rpicamera/mtxrpicam_32"
rpicam_dir_64 := "internal/rpicamera/mtxrpicam_64"

docs_port := env_var_or_default('DOCS_PORT', '8080')

# Default: list all recipes.
default:
    @just --list

# Build CLI and GUI binaries.
build: cli gui

# Build CLI/TUI binary.
cli:
    go build -o {{cli_out}} ./cmd/cli

# Build GUI binary (requires Wails CLI).
gui: _frontend-dist
    cd cmd/gui && wails build

# Build GUI for Windows (NSIS installer).
gui-windows: _frontend-dist
    cd cmd/gui && wails build -nsis -platform windows/amd64

# Build GUI for macOS (amd64).
gui-darwin: _frontend-dist
    cd cmd/gui && wails build -platform darwin/amd64

# Build GUI for Linux (amd64, webkit2_41 build tag).
gui-linux: _frontend-dist
    cd cmd/gui && wails build -platform linux/amd64 -tags webkit2_41

# Ensure the frontend bundle exists. Skipped when already present so repeat
# invocations don't pay the npm install cost on every test/lint run.
[unix]
_frontend-dist:
    @if [ ! -d "{{frontend_dist}}" ]; then cd internal/gui/frontend && npm install && npm run build; fi

# Ensure the frontend bundle exists. Skipped when already present so repeat
# invocations don't pay the npm install cost on every test/lint run.
[windows]
_frontend-dist:
    @if (-not (Test-Path '{{frontend_dist}}')) { cd internal/gui/frontend && npm install && npm run build }

# Build CLI for both Raspberry Pi targets (arm + arm64).
cli-rpi: cli-rpi-arm cli-rpi-arm64

# Build CLI for Raspberry Pi 32-bit (linux/arm, ARMv7).
[unix]
cli-rpi-arm: rpicam-fetch
    GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -tags rpicam -o bin/{{binary}}-rpi-arm ./cmd/cli

# Build CLI for Raspberry Pi 32-bit (linux/arm, ARMv7).
[windows]
cli-rpi-arm: rpicam-fetch
    $env:GOOS = 'linux'; $env:GOARCH = 'arm'; $env:GOARM = '7'; $env:CGO_ENABLED = '0'; go build -tags rpicam -o bin/{{binary}}-rpi-arm ./cmd/cli

# Build CLI for Raspberry Pi 64-bit (linux/arm64).
[unix]
cli-rpi-arm64: rpicam-fetch
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags rpicam -o bin/{{binary}}-rpi-arm64 ./cmd/cli

# Build CLI for Raspberry Pi 64-bit (linux/arm64).
[windows]
cli-rpi-arm64: rpicam-fetch
    $env:GOOS = 'linux'; $env:GOARCH = 'arm64'; $env:CGO_ENABLED = '0'; go build -tags rpicam -o bin/{{binary}}-rpi-arm64 ./cmd/cli

# Fetch mtxrpicam helper binaries for the Pi channel (idempotent).
[unix]
rpicam-fetch:
    ./scripts/fetch-mtxrpicam.sh 32 {{mtxrpicam_version}} {{rpicam_dir_32}}
    ./scripts/fetch-mtxrpicam.sh 64 {{mtxrpicam_version}} {{rpicam_dir_64}}

# Fetch mtxrpicam helper binaries for the Pi channel (PowerShell on Windows).
[windows]
rpicam-fetch:
    pwsh -NoProfile -File ./scripts/fetch-mtxrpicam.ps1 -WordSize 32 -Version '{{mtxrpicam_version}}' -DestDir '{{rpicam_dir_32}}'
    pwsh -NoProfile -File ./scripts/fetch-mtxrpicam.ps1 -WordSize 64 -Version '{{mtxrpicam_version}}' -DestDir '{{rpicam_dir_64}}'

# Cross-compile rpicam-tagged code for both Pi targets (no upstream fetch).
[unix]
rpicam-build-check:
    GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -tags rpicam ./...
    GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags rpicam ./...

# Cross-compile rpicam-tagged code for both Pi targets (no upstream fetch).
[windows]
rpicam-build-check:
    $env:GOOS = 'linux'; $env:GOARCH = 'arm'; $env:GOARM = '7'; $env:CGO_ENABLED = '0'; go build -tags rpicam ./...
    $env:GOOS = 'linux'; $env:GOARCH = 'arm64'; $env:CGO_ENABLED = '0'; go build -tags rpicam ./...

# Run go formatters across all packages.
format:
    golangci-lint fmt ./...

# Run linter across all packages.
lint: _frontend-dist
    golangci-lint run ./...

# Run unit tests (Go + frontend).
test: test-go test-frontend

# Run Go unit tests with the race detector.
test-go: _frontend-dist
    go test -race ./internal/... ./cmd/...

# Run frontend unit tests with coverage.
test-frontend:
    cd internal/gui/frontend && npm run test:coverage

# Run Go tests and print coverage summary.
coverage: _frontend-dist
    go test ./internal/... ./cmd/... -coverprofile=coverage.out -covermode=atomic
    go tool cover -func=coverage.out

# Run E2E SOAP suite against a running simulator (honors ONVIF_HOST, ONVIF_USERNAME, ONVIF_PASSWORD).
[unix]
e2e:
    ONVIF_HOST="${ONVIF_HOST:-localhost:8080}" ONVIF_USERNAME="${ONVIF_USERNAME:-admin}" ONVIF_PASSWORD="${ONVIF_PASSWORD:-}" go test ./test/e2e/... -tags e2e -v

# Run E2E SOAP suite against a running simulator (honors ONVIF_HOST, ONVIF_USERNAME, ONVIF_PASSWORD).
[windows]
e2e:
    if (-not $env:ONVIF_HOST) { $env:ONVIF_HOST = 'localhost:8080' }; \
    if (-not $env:ONVIF_USERNAME) { $env:ONVIF_USERNAME = 'admin' }; \
    if (-not $env:ONVIF_PASSWORD) { $env:ONVIF_PASSWORD = '' }; \
    go test ./test/e2e/... -tags e2e -v

# Remove build artifacts. Scoped to ./bin and ./build/bin — the rest of
# ./build (Wails appicon, NSIS installer template, darwin Info.plist, …)
# is tracked source and must survive `clean`.
[unix]
clean:
    rm -rf ./bin ./build/bin

# Remove build artifacts. Scoped to ./bin and ./build/bin — the rest of
# ./build (Wails appicon, NSIS installer template, darwin Info.plist, …)
# is tracked source and must survive `clean`.
[windows]
clean:
    if (Test-Path ./bin) { Remove-Item -Recurse -Force ./bin }
    if (Test-Path ./build/bin) { Remove-Item -Recurse -Force ./build/bin }

# One-time repo setup: install npm deps and wire up git hooks.
setup:
    npm install
    git config core.hooksPath .githooks

# Browse Go docs in a local browser. Override DOCS_PORT to use a different port.
manual:
    go run ./cmd/manual -port={{docs_port}}
