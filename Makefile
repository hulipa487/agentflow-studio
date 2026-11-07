# AgentFlow Studio (desktop)
#
#   make build   # build the binary
#   make run     # build and run
#   make test    # go test ./...
#   make tidy    # go mod tidy
#
# Fyne requires CGO. Windows uses zig (hermetic) unless a gcc is on PATH;
# Linux uses the system gcc; macOS uses clang.

HOSTOS := $(shell go env GOHOSTOS)

# Go's Windows linker defaults to the console subsystem, so a plain go build
# opens a CMD window beside the Fyne UI. With cgo the final PE is written by
# the external linker (zig), where -H=windowsgui alone is not enough; the
# -Wl,--subsystem,windows flag is what actually sets the GUI subsystem.
# (fyne package sets this itself; go build does not.)
ifeq ($(HOSTOS),windows)
  export CGO_ENABLED := 1
  export CC          := zig cc
  export CXX         := zig c++
  LDFLAGS := -ldflags "-H=windowsgui -extldflags=-Wl,--subsystem,windows"
  BIN     := agentflow-studio.exe
else ifeq ($(HOSTOS),darwin)
  export CGO_ENABLED := 1
  export CC          := clang
  export CXX         := clang++
  LDFLAGS :=
  BIN     := agentflow-studio
else
  export CGO_ENABLED := 1
  export CC          := cc
  export CXX         := c++
  LDFLAGS :=
  BIN     := agentflow-studio
endif

.DEFAULT_GOAL := build

build:
	go build $(LDFLAGS) -o $(BIN) .

run: build
	./$(BIN)

test:
	go test ./...

tidy:
	go mod tidy

# The user API contract and the Luau editor stubs are versioned in the runtime
# repo; keep these copies in step when they change. AGENTFLOW_DIR can point at
# a checkout elsewhere.
AGENTFLOW_DIR ?= ../agentflow
docs-sync:
	@test -f "$(AGENTFLOW_DIR)/api/openapi/users.yaml" || { echo "missing $(AGENTFLOW_DIR)/api/openapi/users.yaml"; exit 1; }
	@test -f "$(AGENTFLOW_DIR)/api/luau/af.d.luau" || { echo "missing $(AGENTFLOW_DIR)/api/luau/af.d.luau"; exit 1; }
	tr -d '\r' < "$(AGENTFLOW_DIR)/api/openapi/users.yaml" > docs/openapi/users.yaml
	tr -d '\r' < "$(AGENTFLOW_DIR)/api/luau/af.d.luau" > docs/luau/af.d.luau

clean:
	rm -f agentflow-studio agentflow-studio.exe

.PHONY: build run test tidy docs-sync clean
