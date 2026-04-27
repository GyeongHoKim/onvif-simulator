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

FRONTEND_DIST := frontend/dist

.PHONY: cli gui format lint test test-go test-frontend coverage e2e clean setup manual

cli:
	$(GO) build -o $(CLI_OUT) ./cmd/cli

gui:
	mkdir -p build/bin
	wails build -o $(GUI_OUT)

format:
	$(GO) fmt ./...

$(FRONTEND_DIST):
	cd frontend && npm install && npm run build

lint: $(FRONTEND_DIST)
	golangci-lint run ./...

test: test-go test-frontend

test-go:
	$(GO) test -race ./internal/... ./cmd/...

test-frontend:
	cd frontend && npm run test:coverage

coverage:
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