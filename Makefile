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

desktop:
	cd web && npm ci
	$(MAKE) -C desktop web build

web:
	cd web && npm ci && npm run build

test:
	cd web && npm test
	$(MAKE) -C desktop test

# The user API contract and the Luau editor stubs are versioned in the runtime
# repo; keep these copies in step when they change. AGENTFLOW_DIR can point at
# a checkout elsewhere.
AGENTFLOW_DIR ?= ../agentflow
docs-sync:
	@test -f "$(AGENTFLOW_DIR)/api/openapi/users.yaml" || { echo "missing $(AGENTFLOW_DIR)/api/openapi/users.yaml"; exit 1; }
	@test -f "$(AGENTFLOW_DIR)/api/luau/af.d.luau" || { echo "missing $(AGENTFLOW_DIR)/api/luau/af.d.luau"; exit 1; }
	tr -d '\r' < "$(AGENTFLOW_DIR)/api/openapi/users.yaml" > docs/openapi/users.yaml
	tr -d '\r' < "$(AGENTFLOW_DIR)/api/luau/af.d.luau" > docs/luau/af.d.luau

.PHONY: desktop web test docs-sync
