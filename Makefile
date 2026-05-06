BINARY := onvif-simulator
GO     := go

ifeq ($(OS),Windows_NT)
  UNAME_S  := $(shell uname -s 2>NUL)
  CLI_OUT  := bin/$(BINARY).exe
  GUI_OUT  := build/bin/$(BINARY)-gui.exe
  ifneq (,$(findstring MINGW,$(UNAME_S))$(findstring MSYS,$(UNAME_S))$(findstring CYGWIN,$(UNAME_S)))
    RM     := rm -rf
  else
    RM     := rmdir /s /q
  endif
else
  RM       := rm -rf
  CLI_OUT  := bin/$(BINARY)
  GUI_OUT  := build/bin/$(BINARY)-gui
endif

FRONTEND_DIST := internal/gui/frontend/dist

# Pinned mediamtx-rpicamera release tag. mediamtx-rpicamera lives in its own
# repo (bluenviron/mediamtx-rpicamera) with versioning independent from
# mediamtx itself; mediamtx v1.18.1 internally pins this same v2.5.6 (see
# its internal/staticsources/rpicamera/mtxrpicamdownloader/VERSION). Bump
# together with scripts/mtxrpicam.sha256 so the Pi build channel ships a
# reviewable, reproducible binary blob.
MTXRPICAM_VERSION ?= v2.5.6
RPICAM_DIR_32     := internal/rpicamera/mtxrpicam_32
RPICAM_DIR_64     := internal/rpicamera/mtxrpicam_64

.PHONY: build cli gui gui-windows gui-darwin gui-linux \
        cli-rpi cli-rpi-arm cli-rpi-arm64 rpicam-fetch rpicam-build-check \
        format lint test test-go test-frontend coverage e2e clean setup manual

build: cli gui

cli:
	$(GO) build -o $(CLI_OUT) ./cmd/cli

gui: $(FRONTEND_DIST)
	cd cmd/gui && wails build

gui-windows: $(FRONTEND_DIST)
	cd cmd/gui && wails build -nsis -platform windows/amd64

gui-darwin: $(FRONTEND_DIST)
	cd cmd/gui && wails build -platform darwin/amd64

gui-linux: $(FRONTEND_DIST)
	cd cmd/gui && wails build -platform linux/amd64 -tags webkit2_41

# Raspberry Pi build channel. Embeds mtxrpicam_{32,64} into the simulator
# binary so a Pi user gets a single self-contained artifact. The fetch step
# is idempotent: it skips the download when the on-disk blob already matches
# the checksum pinned in scripts/mtxrpicam.sha256.
rpicam-fetch: $(RPICAM_DIR_32)/mtxrpicam $(RPICAM_DIR_64)/mtxrpicam

$(RPICAM_DIR_32)/mtxrpicam:
	./scripts/fetch-mtxrpicam.sh 32 $(MTXRPICAM_VERSION) $(RPICAM_DIR_32)

$(RPICAM_DIR_64)/mtxrpicam:
	./scripts/fetch-mtxrpicam.sh 64 $(MTXRPICAM_VERSION) $(RPICAM_DIR_64)

cli-rpi: cli-rpi-arm cli-rpi-arm64

cli-rpi-arm: rpicam-fetch
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
	  $(GO) build -tags rpicam -o bin/$(BINARY)-rpi-arm ./cmd/cli

cli-rpi-arm64: rpicam-fetch
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
	  $(GO) build -tags rpicam -o bin/$(BINARY)-rpi-arm64 ./cmd/cli

# rpicam-build-check verifies that the rpicam-tagged code paths still compile
# without spending time downloading the upstream binary. PR CI calls this; it
# relies on the placeholder file checked into mtxrpicam_{32,64}/ to satisfy
# //go:embed and cross-compiles for both Pi targets.
rpicam-build-check:
	GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 \
	  $(GO) build -tags rpicam ./...
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
	  $(GO) build -tags rpicam ./...

format:
	golangci-lint fmt ./...

$(FRONTEND_DIST):
	cd internal/gui/frontend && npm install && npm run build

lint: $(FRONTEND_DIST)
	golangci-lint run ./...

test: test-go test-frontend

test-go: $(FRONTEND_DIST)
	$(GO) test -race ./internal/... ./cmd/...

test-frontend:
	cd internal/gui/frontend && npm run test:coverage

coverage: $(FRONTEND_DIST)
	$(GO) test ./internal/... ./cmd/... -coverprofile=coverage.out -covermode=atomic
	$(GO) tool cover -func=coverage.out

e2e:
	ONVIF_HOST=$(or $(ONVIF_HOST),localhost:8080) \
	ONVIF_USERNAME=$(or $(ONVIF_USERNAME),admin) \
	ONVIF_PASSWORD=$(or $(ONVIF_PASSWORD),) \
	$(GO) test ./test/e2e/... -tags e2e -v

clean:
	-$(RM) ./bin
	-$(RM) ./build

setup:
	npm install
	git config core.hooksPath .githooks

DOCS_PORT ?= 8080

manual:
	$(GO) run ./cmd/manual -port=$(DOCS_PORT)