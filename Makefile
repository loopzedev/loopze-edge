# LOOPZE – Industrial Flow Automation
# Build & Development Makefile

BINARY_NAME := loopze
MODULE      := github.com/loopzedev/loopze-edge
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME  ?= $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

GO       := go
GOFLAGS  := -trimpath
LDFLAGS  := -s -w \
	-X '$(MODULE)/internal/config.Version=$(VERSION)' \
	-X '$(MODULE)/internal/config.Commit=$(COMMIT)' \
	-X '$(MODULE)/internal/config.BuildTime=$(BUILD_TIME)'

BIN_DIR   := bin
BUILD_DIR := build
WEB_DIR   := web

# Default target
.PHONY: all
all: build

# ─── Build ────────────────────────────────────────────────────────────────────

## build: Build the LOOPZE binary for the current platform
.PHONY: build
build:
	@echo "▸ Building $(BINARY_NAME) $(VERSION)…"
	@mkdir -p $(BIN_DIR)
	$(GO) build $(GOFLAGS) -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY_NAME) ./cmd/loopze

## build-frontend: Build the Vue 3 frontend (requires Node.js / npm)
.PHONY: build-frontend
build-frontend:
	@echo "▸ Building frontend…"
	@if [ -f frontend/package.json ]; then \
		cd frontend && npm install && npm run build; \
		echo "▸ Frontend build complete."; \
	else \
		echo "⚠ No frontend project found at frontend/package.json – skipping."; \
	fi

## build-all: Build frontend first, then the Go binary (single binary with embedded frontend)
.PHONY: build-all
build-all: build-frontend build

# ─── Development ──────────────────────────────────────────────────────────────

.PHONY: dev
dev:
	@echo "▸ Starting LOOPZE backend + frontend with hot reload…"
	@if ! command -v air >/dev/null 2>&1; then \
		echo "⚠ air not found. Install: go install github.com/air-verse/air@latest"; \
		exit 1; \
	fi
	@trap 'kill 0' SIGINT; \
		air & \
		cd frontend && npm run dev & \
		wait

# ─── Testing & Linting ───────────────────────────────────────────────────────

## test: Run all Go tests
.PHONY: test
test:
	@echo "▸ Running tests…"
	$(GO) test -race -count=1 -coverprofile=coverage.out ./...
	@echo "▸ Coverage report: coverage.out"

## lint: Run golangci-lint
.PHONY: lint
lint:
	@echo "▸ Running linter…"
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "⚠ golangci-lint not found. Install: https://golangci-lint.run/welcome/install/"; \
		exit 1; \
	fi

# ─── Demo Servers ────────────────────────────────────────────────────────────

## demo-opcua: Run the OPC UA demo server (Node.js, requires npm)
.PHONY: demo-opcua
demo-opcua:
	@echo "▸ Starting OPC UA demo server…"
	@cd demo/opcua-server && \
		if [ ! -d node_modules ]; then npm install --no-audit --no-fund; fi && \
		npm start

## demo-modbus: Run the Modbus TCP demo slave on :5502 (request-tracing)
.PHONY: demo-modbus
demo-modbus:
	@echo "▸ Starting Modbus TCP demo on :5502…"
	$(GO) run ./demo/modbus-server -listen :5502 -v

## demo-mqtt: Start the Mosquitto demo broker (plain :1883, TLS :8883, mTLS :8884)
.PHONY: demo-mqtt
demo-mqtt:
	@echo "▸ Starting Mosquitto demo broker…"
	@cd demo/mqtt-broker && docker compose up -d && docker compose logs -f

## demo-mqtt-stop: Stop the Mosquitto demo broker (keeps ./data)
.PHONY: demo-mqtt-stop
demo-mqtt-stop:
	@cd demo/mqtt-broker && docker compose down

## demo-s7: Run the SIEMENS S7 demo PLC (python-snap7) on :1102 (request-tracing)
.PHONY: demo-s7
demo-s7:
	@echo "▸ Starting S7 demo PLC on :1102…"
	@cd demo/s7-server && \
		if [ ! -d .venv ]; then \
			python3 -m venv .venv && \
			.venv/bin/pip install -q -r requirements.txt; \
		fi && \
		.venv/bin/python main.py -p 1102 -v

# ─── Documentation ───────────────────────────────────────────────────────────

DOCS_VENV := .venv-docs
DOCS_BIN  := $(DOCS_VENV)/bin/mkdocs
DOCS_PORT ?= 8001

$(DOCS_BIN): docs/requirements.txt
	@echo "▸ Setting up docs venv at $(DOCS_VENV)…"
	@if ! command -v python3 >/dev/null 2>&1; then \
		echo "⚠ python3 not found. Install Python 3.10+."; exit 1; \
	fi
	@python3 -m venv $(DOCS_VENV) 2>/dev/null || { \
		echo "⚠ Failed to create venv. On Debian/Ubuntu: sudo apt install python3-venv"; \
		exit 1; \
	}
	@$(DOCS_VENV)/bin/pip install --quiet --upgrade pip
	@$(DOCS_VENV)/bin/pip install --quiet -r docs/requirements.txt
	@touch $(DOCS_BIN)

## docs-install: Install mkdocs-material into a local venv (.venv-docs/)
.PHONY: docs-install
docs-install: $(DOCS_BIN)
	@echo "▸ Docs toolchain ready."

## docs-serve: Serve the docs locally with live reload (override port: DOCS_PORT=9000)
.PHONY: docs-serve
docs-serve: $(DOCS_BIN)
	@echo "▸ Serving docs at http://localhost:$(DOCS_PORT) (Ctrl-C to stop)…"
	@$(DOCS_VENV)/bin/mkdocs serve --dev-addr 127.0.0.1:$(DOCS_PORT)

## docs-build: Build the static documentation site into site/
.PHONY: docs-build
docs-build: $(DOCS_BIN)
	@echo "▸ Building docs into site/…"
	@$(DOCS_VENV)/bin/mkdocs build --strict
	@echo "▸ Done. Output: site/"

## docs-clean: Remove the built docs (site/) and the docs venv (.venv-docs/)
.PHONY: docs-clean
docs-clean:
	@echo "▸ Cleaning docs build output and venv…"
	@rm -rf site $(DOCS_VENV)
	@echo "▸ Done."

# ─── Versioned docs (mike) ────────────────────────────────────────────────
# Inactive by default — see the comment in mkdocs.yml under `extra.version`
# for how to switch from single-version to multi-version mode.

## docs-deploy: Push a versioned docs build to gh-pages (e.g. VERSION=0.5 ALIAS=latest)
.PHONY: docs-deploy
docs-deploy: $(DOCS_BIN)
	@if [ -z "$(VERSION)" ]; then \
		echo "⚠ Set VERSION, e.g. make docs-deploy VERSION=0.5 ALIAS=latest"; \
		exit 1; \
	fi
	@echo "▸ Deploying docs version $(VERSION)$(if $(ALIAS), (alias: $(ALIAS)))…"
	@$(DOCS_VENV)/bin/mike deploy --push --update-aliases $(VERSION) $(ALIAS)

## docs-versions: List deployed documentation versions on gh-pages
.PHONY: docs-versions
docs-versions: $(DOCS_BIN)
	@$(DOCS_VENV)/bin/mike list

## docs-set-default: Set the default version visitors land on (e.g. VERSION=latest)
.PHONY: docs-set-default
docs-set-default: $(DOCS_BIN)
	@if [ -z "$(VERSION)" ]; then \
		echo "⚠ Set VERSION, e.g. make docs-set-default VERSION=latest"; \
		exit 1; \
	fi
	@$(DOCS_VENV)/bin/mike set-default --push $(VERSION)

# ─── Docker ───────────────────────────────────────────────────────────────────

DOCKER_IMAGE ?= loopze-edge
DOCKER_TAG   ?= local

## docker-build: Build the Docker image (uses demo/Dockerfile, repo root as context)
.PHONY: docker-build
docker-build:
	@echo "▸ Building $(DOCKER_IMAGE):$(DOCKER_TAG)…"
	@docker build \
		-f demo/Dockerfile \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_TIME=$(BUILD_TIME) \
		-t $(DOCKER_IMAGE):$(DOCKER_TAG) \
		.

## docker-up: Start the demo compose stack (build if needed, detached)
.PHONY: docker-up
docker-up:
	@echo "▸ Starting LOOPZE via docker compose…"
	@docker compose -f demo/docker-compose.yml up -d --build
	@echo "▸ Open http://localhost:1880"

## docker-down: Stop the demo compose stack (state in demo/data is kept)
.PHONY: docker-down
docker-down:
	@docker compose -f demo/docker-compose.yml down

## docker-logs: Follow logs of the running compose stack
.PHONY: docker-logs
docker-logs:
	@docker compose -f demo/docker-compose.yml logs -f loopze

# ─── Cross Compilation ───────────────────────────────────────────────────────

PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	linux/arm \
	windows/amd64 \
	darwin/amd64 \
	darwin/arm64

## cross-compile: Build for all target platforms
.PHONY: cross-compile
cross-compile:
	@echo "▸ Cross-compiling $(BINARY_NAME) $(VERSION) for all platforms…"
	@mkdir -p $(BUILD_DIR)
	@for platform in $(PLATFORMS); do \
		GOOS=$${platform%/*}; \
		GOARCH=$${platform#*/}; \
		output=$(BUILD_DIR)/$(BINARY_NAME)-$${GOOS}-$${GOARCH}; \
		if [ "$${GOOS}" = "windows" ]; then output=$${output}.exe; fi; \
		echo "  → $${GOOS}/$${GOARCH}"; \
		GOOS=$${GOOS} GOARCH=$${GOARCH} $(GO) build $(GOFLAGS) \
			-ldflags "$(LDFLAGS)" \
			-o $${output} ./cmd/loopze || exit 1; \
	done
	@echo "▸ All binaries written to $(BUILD_DIR)/"

# ─── Cleanup ─────────────────────────────────────────────────────────────────

## clean: Remove build artifacts and generated files
.PHONY: clean
clean:
	@echo "▸ Cleaning…"
	@rm -rf $(BIN_DIR) $(BUILD_DIR) coverage.out
	@echo "▸ Done."

# ─── Help ─────────────────────────────────────────────────────────────────────

## help: Show this help message
.PHONY: help
help:
	@echo "LOOPZE – Industrial Flow Automation"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
