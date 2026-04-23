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

.PHONY: cli gui format lint test coverage e2e clean setup

cli:
	$(GO) build -o $(CLI_OUT) ./cmd/cli

gui:
	mkdir -p build/bin
	cd cmd/gui && wails build -o ../../$(GUI_OUT)

format:
	$(GO) fmt ./...

lint:
	golangci-lint run ./...

test:
	$(GO) test -race ./internal/...

coverage:
	$(GO) test ./internal/... -coverprofile=coverage.out -covermode=atomic
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