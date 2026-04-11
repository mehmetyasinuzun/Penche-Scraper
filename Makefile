.PHONY: all dev build test clean router-run router-test ext-dev ext-build lint help

# ── Variables ────────────────────────────────────────────────────────────────

ROUTER_DIR := router
EXT_DIR    := extension
BINARY     := $(ROUTER_DIR)/bin/penche-router

# ── Top-level targets ─────────────────────────────────────────────────────────

all: build

help:
	@echo ""
	@echo "  make dev           Start router + extension watcher (Chrome)"
	@echo "  make build         Build everything (router binary + both ext targets)"
	@echo "  make test          Run all tests"
	@echo "  make router-run    Start the router server"
	@echo "  make router-test   Run Go tests"
	@echo "  make ext-build     Build extension for Chrome and Firefox"
	@echo "  make ext-dev       Watch-build extension for Chrome"
	@echo "  make lint          Run linters"
	@echo "  make clean         Remove build artifacts"
	@echo ""

dev: router-run ext-dev

build: router-build ext-build

test: router-test

clean:
	rm -rf $(ROUTER_DIR)/bin
	rm -rf $(EXT_DIR)/dist

# ── Router ────────────────────────────────────────────────────────────────────

router-build:
	@echo "→ Building router..."
	@mkdir -p $(ROUTER_DIR)/bin
	cd $(ROUTER_DIR) && go build -o bin/penche-router ./cmd/server

router-run: router-build
	@echo "→ Starting router (config: $(ROUTER_DIR)/config.yaml)"
	$(BINARY) -config $(ROUTER_DIR)/config.yaml

router-test:
	@echo "→ Running Go tests..."
	cd $(ROUTER_DIR) && go test ./... -v -count=1

router-mod-tidy:
	cd $(ROUTER_DIR) && go mod tidy

router-lint:
	cd $(ROUTER_DIR) && go vet ./...

# ── Extension ─────────────────────────────────────────────────────────────────

ext-install:
	@echo "→ Installing extension dependencies..."
	cd $(EXT_DIR) && npm install

ext-build: ext-install
	@echo "→ Building extension (Chrome + Firefox)..."
	cd $(EXT_DIR) && npm run build

ext-build-chrome: ext-install
	cd $(EXT_DIR) && npm run build:chrome

ext-build-firefox: ext-install
	cd $(EXT_DIR) && npm run build:firefox

ext-dev:
	@echo "→ Watching extension (Chrome)..."
	cd $(EXT_DIR) && npm run dev:chrome

ext-typecheck:
	cd $(EXT_DIR) && npm run typecheck

# ── Lint ──────────────────────────────────────────────────────────────────────

lint: router-lint ext-typecheck
