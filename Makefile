# DEPRECATED: This Makefile is preserved for backward compatibility only.
# The Justfile at the repo root is the canonical source of truth and these
# targets simply delegate to `just <recipe>`. This file will be removed in a
# follow-up issue once tooling, hooks, and external scripts have all migrated.
#
# Install just:
#   mise install                           # picks up the version pinned in mise.toml
#   brew install just                      # macOS / Linuxbrew
#   scoop install just                     # Windows (scoop)
#   cargo install just                     # any platform with Rust
#
# Pin / variable definitions live in Justfile; keep them there so there is
# exactly one source to bump.

JUST ?= just

.PHONY: build cli gui gui-windows gui-darwin gui-linux \
        cli-rpi cli-rpi-arm cli-rpi-arm64 rpicam-fetch rpicam-build-check \
        format lint test test-go test-frontend coverage e2e clean setup manual

build:              ; $(JUST) build
cli:                ; $(JUST) cli
gui:                ; $(JUST) gui
gui-windows:        ; $(JUST) gui-windows
gui-darwin:         ; $(JUST) gui-darwin
gui-linux:          ; $(JUST) gui-linux
cli-rpi:            ; $(JUST) cli-rpi
cli-rpi-arm:        ; $(JUST) cli-rpi-arm
cli-rpi-arm64:      ; $(JUST) cli-rpi-arm64
rpicam-fetch:       ; $(JUST) rpicam-fetch
rpicam-build-check: ; $(JUST) rpicam-build-check
format:             ; $(JUST) format
lint:               ; $(JUST) lint
test:               ; $(JUST) test
test-go:            ; $(JUST) test-go
test-frontend:      ; $(JUST) test-frontend
coverage:           ; $(JUST) coverage
e2e:                ; $(JUST) e2e
clean:              ; $(JUST) clean
setup:              ; $(JUST) setup

# DOCS_PORT is exported so the Justfile picks it up via env_var_or_default.
DOCS_PORT ?= 8080
manual:
	DOCS_PORT=$(DOCS_PORT) $(JUST) manual
