BINARY := onvif-simulator
GO     := go
OS     := $(shell uname -s 2>/dev/null || echo Windows)

ifeq ($(OS), Windows)
  RM       := del /f /q
  CLI_OUT  := bin/$(BINARY).exe
  GUI_OUT  := bin/$(BINARY)-gui.exe
else
  RM       := rm -f
  CLI_OUT  := bin/$(BINARY)
  GUI_OUT  := bin/$(BINARY)-gui
endif

.PHONY: cli gui format lint clean setup

cli:
	$(GO) build -o $(CLI_OUT) ./cmd/cli

gui:
	cd cmd/gui && wails build -o ../../$(GUI_OUT)

format:
	$(GO) fmt ./...

lint:
	golangci-lint run ./...

clean:
	$(RM) $(CLI_OUT) $(GUI_OUT)

setup:
	npm install
	git config core.hooksPath .githooks
