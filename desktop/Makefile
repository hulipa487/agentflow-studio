# AgentFlow Studio desktop: the web app (../web) in the system webview, via Wails v2
#
#   make web     # build ../web and copy it into frontend/dist
#   make build   # build the binary (runs `make web` first if needed)
#   make windows # cross-compile Windows amd64 and arm64 .exe files
#   make winres  # regenerate the Windows icon, manifest and version resources
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
BIN     := agentflow-studio

ifeq ($(HOSTOS),linux)
  TAGS := $(TAGS),webkit2_41
endif
ifeq ($(HOSTOS),windows)
  export CGO_ENABLED := 0
  LDFLAGS += -H windowsgui
  BIN     := agentflow-studio.exe
  RUN_BIN := $(BIN)
else
  export CGO_ENABLED := 1
  RUN_BIN := ./$(BIN)
endif

.PHONY: web build windows winres run test clean

web:
	cd ../web && npm run build
	node ../web/scripts/desktop-assets.mjs sync

frontend/dist/index.html:
	$(MAKE) web

build: frontend/dist/index.html
	go build -tags $(TAGS) -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .

# Windows binaries cross-compile from any OS: Wails needs no cgo there.
windows: frontend/dist/index.html
	node ../web/scripts/desktop-assets.mjs build-windows "$(VERSION)"

# The .syso files are committed, so a plain go build on Windows picks up the
# icon (winres/icon*.png), the manifest (per-monitor DPI awareness, Common
# Controls v6 for native dialogs) and the version info. Rerun after changing
# winres/.
winres:
	go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --arch amd64,arm64

run: build
	$(RUN_BIN)

test:
	go test -tags $(TAGS) ./...

clean:
	node ../web/scripts/desktop-assets.mjs clean
