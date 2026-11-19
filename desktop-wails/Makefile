# AgentFlow Studio desktop, Wails v2 shell (spike)
#
#   make web     # build ../web and copy it into frontend/dist
#   make build   # build the binary (runs `make web` first if needed)
#   make run     # build and run
#   make test    # go test
#
# No Wails CLI is needed: a Wails v2 app builds with plain `go build` and the
# desktop + production tags.
#
# Linux needs the WebKitGTK 4.1 development files (libwebkit2gtk-4.1-dev on
# Debian/Ubuntu) to build, and only libwebkit2gtk-4.1-0 to run. Windows needs
# the WebView2 runtime to run (part of Windows 11 and current Windows 10);
# macOS needs nothing extra.

HOSTOS  := $(shell go env GOHOSTOS)
VERSION ?= dev
TAGS    := desktop,production
LDFLAGS := -s -w -X main.version=$(VERSION)
BIN     := agentflow-studio-wails

ifeq ($(HOSTOS),linux)
  TAGS := $(TAGS),webkit2_41
endif
ifeq ($(HOSTOS),windows)
  LDFLAGS += -H windowsgui
  BIN     := agentflow-studio-wails.exe
endif

export CGO_ENABLED := 1

.PHONY: web build run test clean

web:
	cd ../web && npm run build
	rm -rf frontend/dist
	cp -r ../web/dist frontend/dist

frontend/dist/index.html:
	$(MAKE) web

build: frontend/dist/index.html
	go build -tags $(TAGS) -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .

run: build
	./$(BIN)

test:
	go test -tags $(TAGS) ./...

clean:
	rm -rf $(BIN) frontend/dist
	mkdir -p frontend/dist && touch frontend/dist/.gitkeep
