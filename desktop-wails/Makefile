# AgentFlow Studio desktop, Wails v2 shell (spike)
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
BIN     := agentflow-studio-wails

ifeq ($(HOSTOS),linux)
  TAGS := $(TAGS),webkit2_41
endif
ifeq ($(HOSTOS),windows)
  LDFLAGS += -H windowsgui
  BIN     := agentflow-studio-wails.exe
endif

export CGO_ENABLED := 1

.PHONY: web build windows winres run test clean

web:
	cd ../web && npm run build
	rm -rf frontend/dist
	cp -r ../web/dist frontend/dist

frontend/dist/index.html:
	$(MAKE) web

build: frontend/dist/index.html
	go build -tags $(TAGS) -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) .

# Windows binaries cross-compile from any OS: Wails needs no cgo there.
windows: frontend/dist/index.html
	for arch in amd64 arm64; do \
	  CGO_ENABLED=0 GOOS=windows GOARCH=$$arch go build -tags desktop,production -trimpath \
	    -ldflags "-s -w -H windowsgui -X main.version=$(VERSION)" -o agentflow-studio-wails-$$arch.exe . ; \
	done

# The .syso files are committed, so a plain go build on Windows picks up the
# icon (winres/icon*.png), the manifest (per-monitor DPI awareness, Common
# Controls v6 for native dialogs) and the version info. Rerun after changing
# winres/.
winres:
	go run github.com/tc-hib/go-winres@v0.3.3 make --in winres/winres.json --arch amd64,arm64

run: build
	./$(BIN)

test:
	go test -tags $(TAGS) ./...

clean:
	rm -rf $(BIN) frontend/dist
	mkdir -p frontend/dist && touch frontend/dist/.gitkeep
