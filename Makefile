# LOOPZE – Industrial Flow Automation
# Build & Development Makefile

BINARY_NAME := loopze
MODULE      := github.com/niceclouds/loopze
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
