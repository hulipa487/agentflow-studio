# AgentFlow Studio: the web app (web/), the desktop app that shows it in the
# system webview (desktop/), and the documentation both ship (docs/).
#
#   make desktop     # build the desktop app          (desktop/agentflow-studio)
#   make web         # install and build the web app  (web/dist)
#   make test        # test both
#   make docs-sync   # refresh the contract files in docs/ from the runtime repo
#
# Each also builds on its own: see desktop/Makefile and web/package.json. The
# desktop build embeds the web build, so it needs the web app's dependencies.

.DEFAULT_GOAL := desktop

install:
	cd web && npm ci

web/node_modules:
	cd web && npm ci

desktop: web/node_modules
	$(MAKE) -C desktop web build

web: web/node_modules
	cd web && npm run build

test: web/node_modules
	cd web && npm run check
	$(MAKE) -C desktop test

# The user API contract and the loop editor stubs are versioned in the runtime
# repo; keep these copies in step when they change. AGENTFLOW_DIR can point at
# a checkout elsewhere.
AGENTFLOW_DIR ?= ../agentflow
docs-sync:
	node web/scripts/docs-sync.mjs --runtime "$(AGENTFLOW_DIR)"

.PHONY: install desktop web test docs-sync
