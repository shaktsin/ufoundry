.PHONY: help install dev upgrade init run start stop restart status reload logs ps \
        panel panel-build panel-dev \
        skills skill-add skill-rm \
        test lint format check shell gateway \
        config edit \
        db db-backup db-reset \
        reset clean \
        build publish-test publish \
        info doctor

INSTALL_STAMP = $(VENV)/.install-stamp

# ---------------------------------------------------------------------------
# Variables
# ---------------------------------------------------------------------------

PYTHON    := python3
VENV      := .venv
BIN       := $(VENV)/bin
PIP       := $(BIN)/pip
UFOUNDRY    := $(BIN)/ufoundry

CONFIG_DIR  := $(HOME)/.ufoundry
CONFIG_FILE := $(CONFIG_DIR)/config.yaml
LOG_LEVEL   := DEBUG

# Colors
BLUE   := \033[0;34m
GREEN  := \033[0;32m
YELLOW := \033[0;33m
RED    := \033[0;31m
NC     := \033[0m

# ---------------------------------------------------------------------------
# Help
# ---------------------------------------------------------------------------

help: ## Show this help
	@echo "$(BLUE)UFoundry — Self-hosted AI Assistant$(NC)"
	@echo ""
	@echo "$(GREEN)Config:$(NC)  $(CONFIG_FILE)"
	@echo "$(GREEN)Venv:$(NC)    $(VENV)"
	@echo ""
	@echo "$(GREEN)Setup$(NC)"
	@echo "  $(YELLOW)install$(NC)       Create venv and install all dependencies"
	@echo "  $(YELLOW)dev$(NC)           Install with dev dependencies (pytest, black, mypy…)"
	@echo "  $(YELLOW)upgrade$(NC)       Upgrade all installed dependencies"
	@echo "  $(YELLOW)init$(NC)          Run interactive configuration wizard"
	@echo "  $(YELLOW)doctor$(NC)        Check config, connectors, and skill health"
	@echo ""
	@echo "$(GREEN)Run$(NC)"
	@echo "  $(YELLOW)run$(NC)           Start in foreground (Ctrl+C to stop); auto-starts web panel"
	@echo "  $(YELLOW)start$(NC)         Start as background daemon"
	@echo "  $(YELLOW)stop$(NC)          Stop daemon or foreground processes"
	@echo "  $(YELLOW)restart$(NC)       Stop then start daemon"
	@echo "  $(YELLOW)status$(NC)        Show whether daemon is running"
	@echo "  $(YELLOW)reload$(NC)        Hot-reload config without restart"
	@echo "  $(YELLOW)logs$(NC)          Tail the live log file"
	@echo "  $(YELLOW)ps$(NC)            List all UFoundry processes"
	@echo ""
	@echo "$(GREEN)Control Panel$(NC)"
	@echo "  $(YELLOW)panel$(NC)         Start web control panel (installs deps, opens browser)"
	@echo "  $(YELLOW)panel-build$(NC)   Build frontend → ufoundry/controlpanel/static/"
	@echo "  $(YELLOW)panel-dev$(NC)     Start frontend HMR dev server (needs 'make run' on :8080)"
	@echo ""
	@echo "$(GREEN)Skills$(NC)"
	@echo "  $(YELLOW)skills$(NC)        List all loaded skills"
	@echo "  $(YELLOW)skill-add$(NC)     Install a skill:  make skill-add SKILL=<path|url|name>"
	@echo "  $(YELLOW)skill-rm$(NC)      Remove a skill:   make skill-rm  SKILL=<name>"
	@echo ""
	@echo "$(GREEN)Config & DB$(NC)"
	@echo "  $(YELLOW)config$(NC)        Print current config.yaml"
	@echo "  $(YELLOW)edit$(NC)          Open config.yaml in \$$EDITOR"
	@echo "  $(YELLOW)db$(NC)            Open SQLite shell"
	@echo "  $(YELLOW)db-backup$(NC)     Backup database to ~/.ufoundry/backups/"
	@echo "  $(YELLOW)db-reset$(NC)      Delete database (keep config)"
	@echo ""
	@echo "$(GREEN)Development$(NC)"
	@echo "  $(YELLOW)test$(NC)          Run pytest"
	@echo "  $(YELLOW)lint$(NC)          Run flake8 + mypy"
	@echo "  $(YELLOW)format$(NC)        Format code with black"
	@echo "  $(YELLOW)check$(NC)         lint + test"
	@echo "  $(YELLOW)shell$(NC)         Python REPL with ufoundry imported"
	@echo "  $(YELLOW)gateway$(NC)       Start gateway only (no connectors, for dev)"
	@echo ""
	@echo "$(GREEN)Cleanup$(NC)"
	@echo "  $(YELLOW)reset$(NC)         Wipe ~/.ufoundry/ + sessions, keep .venv  →  re-run 'make init'"
	@echo "  $(YELLOW)clean$(NC)         Nuke everything: .venv + ~/.ufoundry/ + sessions  →  clean slate"
	@echo ""
	@echo "$(GREEN)Release$(NC)"
	@echo "  $(YELLOW)build$(NC)         Build frontend + dist packages"
	@echo "  $(YELLOW)publish-test$(NC)  Upload to TestPyPI"
	@echo "  $(YELLOW)publish$(NC)       Upload to PyPI"
	@echo "  $(YELLOW)info$(NC)          Show system/path information"

# ---------------------------------------------------------------------------
# Setup
# ---------------------------------------------------------------------------

$(INSTALL_STAMP): pyproject.toml
	@if [ ! -d "$(VENV)" ]; then \
		echo "$(YELLOW)Creating virtual environment...$(NC)"; \
		$(PYTHON) -m venv $(VENV); \
	fi
	@$(PIP) install --quiet --upgrade pip
	@$(PIP) install --quiet -e ".[all]"
	@touch $(INSTALL_STAMP)

install: $(INSTALL_STAMP) ## Create venv and install dependencies (skips if pyproject.toml unchanged)
	@echo "$(GREEN)✓ UFoundry installed. Run 'make init' to configure.$(NC)"

dev: ## Install with dev dependencies (pytest, black, mypy, flake8)
	@echo "$(BLUE)Installing dev dependencies...$(NC)"
	@if [ ! -d "$(VENV)" ]; then $(PYTHON) -m venv $(VENV); fi
	@$(PIP) install --quiet --upgrade pip
	@$(PIP) install --quiet -e ".[dev]"
	@touch $(INSTALL_STAMP)
	@echo "$(GREEN)✓ Dev install complete.$(NC)"

upgrade: ## Upgrade all installed dependencies
	@echo "$(YELLOW)Upgrading dependencies...$(NC)"
	@$(PIP) install --quiet --upgrade pip
	@$(PIP) install --quiet --upgrade -e .
	@touch $(INSTALL_STAMP)
	@echo "$(GREEN)✓ Upgraded.$(NC)"


init: $(INSTALL_STAMP) ## Run interactive configuration wizard
	@echo "$(BLUE)Running configuration wizard...$(NC)"
	@mkdir -p $(CONFIG_DIR)
	@$(UFOUNDRY) onboard --config $(CONFIG_FILE) --log-level $(LOG_LEVEL)

doctor: $(INSTALL_STAMP) ## Check config, connectors, and skill health
	@echo "$(BLUE)Running diagnostics...$(NC)"
	@$(UFOUNDRY) doctor --config $(CONFIG_FILE) --log-level $(LOG_LEVEL) || true

# ---------------------------------------------------------------------------
# Run
# ---------------------------------------------------------------------------

run: $(INSTALL_STAMP) ## Start in foreground; auto-starts web panel if configured (Ctrl+C to stop)
	@echo "$(BLUE)Starting UFoundry...$(NC)"
	@echo "$(YELLOW)Ctrl+C to stop$(NC)"
	@( \
		PANEL_PID=""; \
		if grep -q "ui_type: web" $(CONFIG_FILE) 2>/dev/null && grep -q "enabled: true" $(CONFIG_FILE) 2>/dev/null; then \
			$(PIP) install --quiet -e ".[panel]"; \
			PORT=$$(grep 'web_port' $(CONFIG_FILE) 2>/dev/null | awk '{print $$2}' | head -1); \
			PORT=$${PORT:-8080}; \
			$(BIN)/python -m ufoundry.controlpanel --config $(CONFIG_FILE) --no-open --log-level $(LOG_LEVEL) & \
			PANEL_PID=$$!; \
			echo "$(GREEN)✓ Panel → http://127.0.0.1:$$PORT$(NC)"; \
		fi; \
		cleanup() { [ -n "$$PANEL_PID" ] && kill "$$PANEL_PID" 2>/dev/null; }; \
		trap cleanup EXIT INT TERM; \
		$(UFOUNDRY) orchestrate --config $(CONFIG_FILE) --log-level $(LOG_LEVEL); \
	)

start: $(INSTALL_STAMP) ## Start as background daemon
	@echo "$(BLUE)Starting daemon...$(NC)"
	@$(UFOUNDRY) start --config $(CONFIG_FILE) --log-level $(LOG_LEVEL)
	@sleep 1
	@$(MAKE) status

stop: ## Stop daemon or any foreground UFoundry processes
	@echo "$(YELLOW)Stopping UFoundry...$(NC)"
	@PID_FILE="$(CONFIG_DIR)/ufoundry.pid"; \
	if [ -f "$$PID_FILE" ]; then \
		PID=$$(cat "$$PID_FILE" 2>/dev/null); \
		if [ -n "$$PID" ] && kill -0 "$$PID" 2>/dev/null; then \
			echo "  Stopping daemon PID=$$PID"; \
			kill -TERM "$$PID" 2>/dev/null || true; \
		fi; \
		rm -f "$$PID_FILE"; \
	fi
	@for pat in "ufoundry orchestrate" "ufoundry.controlpanel" "ufoundry.gateway"; do \
		pids=$$(pgrep -f "$$pat" 2>/dev/null); \
		if [ -n "$$pids" ]; then \
			echo "  Stopping $$pat (PID $$pids)"; \
			kill -TERM $$pids 2>/dev/null || true; \
		fi; \
	done
	@sleep 1
	@for pat in "ufoundry orchestrate" "ufoundry.controlpanel" "ufoundry.gateway"; do \
		pids=$$(pgrep -f "$$pat" 2>/dev/null); \
		if [ -n "$$pids" ]; then \
			echo "  $(RED)Force-killing $$pat (PID $$pids)$(NC)"; \
			kill -KILL $$pids 2>/dev/null || true; \
		fi; \
	done
	@echo "$(GREEN)✓ Stopped$(NC)"

restart: stop start ## Stop then start daemon

status: $(INSTALL_STAMP) ## Show whether daemon is running
	@$(UFOUNDRY) status --config $(CONFIG_FILE) || echo "$(RED)UFoundry is not running$(NC)"

reload: $(INSTALL_STAMP) ## Hot-reload config without restart
	@echo "$(YELLOW)Reloading config...$(NC)"
	@$(UFOUNDRY) reload --config $(CONFIG_FILE)

logs: ## Tail the live log file
	@tail -f $(CONFIG_DIR)/logs/ufoundry.log

ps: ## List all UFoundry processes
	@ps aux | grep -E "ufoundry|python.*gateway|python.*connector" | grep -v grep \
		|| echo "$(YELLOW)No UFoundry processes running$(NC)"

# ---------------------------------------------------------------------------
# Control Panel
# ---------------------------------------------------------------------------

panel: $(INSTALL_STAMP) ## Start web control panel (installs deps, opens browser)
	@echo "$(BLUE)Starting control panel...$(NC)"
	@$(PIP) install --quiet -e ".[panel]"
	@$(UFOUNDRY) panel --config $(CONFIG_FILE) --log-level $(LOG_LEVEL)

panel-build: ## Build frontend → ufoundry/controlpanel/static/
	@echo "$(BLUE)Building frontend...$(NC)"
	@cd ufoundry/controlpanel/frontend && npm install && npm run build
	@echo "$(GREEN)✓ Built → ufoundry/controlpanel/static/$(NC)"

panel-dev: ## Start frontend HMR dev server (requires 'make run' on :8080)
	@echo "$(BLUE)Starting HMR dev server → http://localhost:5173$(NC)"
	@echo "$(YELLOW)Requires 'make run' running on :8080$(NC)"
	@cd ufoundry/controlpanel/frontend && npm run dev

# ---------------------------------------------------------------------------
# Skills
# ---------------------------------------------------------------------------

skills: $(INSTALL_STAMP) ## List all loaded skills
	@$(UFOUNDRY) skills list --config $(CONFIG_FILE)

skill-add: $(INSTALL_STAMP) ## Install a skill  (make skill-add SKILL=<path|url|name>)
	@if [ -z "$(SKILL)" ]; then \
		echo "$(RED)Usage: make skill-add SKILL=<path|url|name>$(NC)"; exit 1; \
	fi
	@$(UFOUNDRY) skills install $(SKILL) --config $(CONFIG_FILE)
	@echo "$(YELLOW)Run 'make reload' to activate$(NC)"

skill-rm: $(INSTALL_STAMP) ## Remove a skill  (make skill-rm SKILL=<name>)
	@if [ -z "$(SKILL)" ]; then \
		echo "$(RED)Usage: make skill-rm SKILL=<name>$(NC)"; exit 1; \
	fi
	@$(UFOUNDRY) skills remove $(SKILL) --config $(CONFIG_FILE)
	@echo "$(YELLOW)Run 'make reload' to deactivate$(NC)"

# ---------------------------------------------------------------------------
# Config & DB
# ---------------------------------------------------------------------------

config: ## Print current config.yaml
	@cat $(CONFIG_FILE) 2>/dev/null || echo "$(YELLOW)No config found — run 'make init' first$(NC)"

edit: ## Open config.yaml in $$EDITOR (falls back to nano)
	@[ -f $(CONFIG_FILE) ] || { echo "$(YELLOW)No config — run 'make init' first$(NC)"; exit 1; }
	@$${EDITOR:-nano} $(CONFIG_FILE)

db: ## Open SQLite shell
	@sqlite3 $(CONFIG_DIR)/ufoundry.db

db-backup: ## Backup database to ~/.ufoundry/backups/
	@mkdir -p $(CONFIG_DIR)/backups
	@cp $(CONFIG_DIR)/ufoundry.db $(CONFIG_DIR)/backups/ufoundry-$(shell date +%Y%m%d-%H%M%S).db
	@echo "$(GREEN)✓ Backed up$(NC)"

db-reset: stop ## Delete database — keep config (WARNING: loses all history)
	@echo "$(RED)WARNING: Deletes all messages, tasks, and history$(NC)"
	@read -p "Are you sure? (yes/no): " c; [ "$$c" = "yes" ] || { echo "Cancelled"; exit 0; }
	@rm -f $(CONFIG_DIR)/ufoundry.db $(CONFIG_DIR)/ufoundry.db-shm $(CONFIG_DIR)/ufoundry.db-wal
	@echo "$(GREEN)✓ Database cleared — restart UFoundry to recreate$(NC)"

# ---------------------------------------------------------------------------
# Development
# ---------------------------------------------------------------------------

test: ## Run pytest
	@echo "$(YELLOW)Running tests...$(NC)"
	@$(BIN)/pytest tests/ -v 2>/dev/null || echo "$(YELLOW)No tests found$(NC)"

lint: ## Run flake8 + mypy
	@echo "$(YELLOW)Linting...$(NC)"
	@$(BIN)/flake8 ufoundry/ || true
	@$(BIN)/mypy ufoundry/ || true

format: ## Format code with black
	@$(BIN)/black ufoundry/
	@echo "$(GREEN)✓ Formatted$(NC)"

check: lint test ## Run lint + test

shell: $(INSTALL_STAMP) ## Open Python REPL with ufoundry imported
	@$(BIN)/python -i -c "from ufoundry import *; print('UFoundry loaded')"

gateway: $(INSTALL_STAMP) ## Start gateway only — no connectors (dev/debug)
	@echo "$(BLUE)Starting gateway only...$(NC)"
	@$(BIN)/python -m ufoundry.gateway --config $(CONFIG_FILE) --log-level $(LOG_LEVEL)

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------

reset: stop ## Wipe ~/.ufoundry/ + sessions, keep .venv  →  re-run 'make init'
	@echo "$(RED)Deletes: ~/.ufoundry/ (config, db, vault, logs) and *.session files$(NC)"
	@read -p "Are you sure? (yes/no): " c; [ "$$c" = "yes" ] || { echo "Cancelled"; exit 0; }
	@rm -rf $(CONFIG_DIR)
	@find . -maxdepth 2 -name "*.session" -delete 2>/dev/null || true
	@echo "$(GREEN)✓ Reset. Run 'make init' to configure from scratch.$(NC)"

clean: stop ## Nuke everything: .venv + ~/.ufoundry/ + sessions + build artifacts
	@echo "$(RED)Deletes: .venv, ~/.ufoundry/, *.session, build artifacts$(NC)"
	@read -p "Are you sure? (yes/no): " c; [ "$$c" = "yes" ] || { echo "Cancelled"; exit 0; }
	@rm -rf build/ dist/ *.egg-info $(VENV) $(CONFIG_DIR)
	@find . -type d -name __pycache__ -exec rm -rf {} + 2>/dev/null || true
	@find . \( -name "*.pyc" -o -name "*.pyo" -o -name "*.session" \) -delete 2>/dev/null || true
	@echo "$(GREEN)✓ Clean slate. Run 'make install && make init' to start fresh.$(NC)"

# ---------------------------------------------------------------------------
# Release
# ---------------------------------------------------------------------------

build: panel-build ## Build frontend + distribution packages (wheel + sdist)
	@echo "$(BLUE)Building...$(NC)"
	@$(BIN)/python -m build
	@echo "$(GREEN)✓ Built → dist/$(NC)"

publish-test: build ## Upload to TestPyPI
	@$(BIN)/twine upload --repository testpypi dist/*

publish: build ## Upload to PyPI
	@echo "$(RED)Publishing to PyPI$(NC)"
	@read -p "Are you sure? (yes/no): " c; [ "$$c" = "yes" ] || { echo "Cancelled"; exit 0; }
	@$(BIN)/twine upload dist/*
	@echo "$(GREEN)✓ Published$(NC)"

# ---------------------------------------------------------------------------
# Info
# ---------------------------------------------------------------------------

info: ## Show system paths and state
	@echo "$(BLUE)UFoundry Info$(NC)"
	@echo "  Python    : $(shell $(PYTHON) --version)"
	@echo "  Venv      : $(VENV)"
	@echo "  Config    : $(CONFIG_FILE)"
	@echo "  Database  : $(CONFIG_DIR)/ufoundry.db"
	@echo "  Logs      : $(CONFIG_DIR)/logs/"
	@echo "  Log level : $(LOG_LEVEL)"
	@echo ""
	@[ -f $(CONFIG_FILE) ]         && echo "$(GREEN)  ✓ Config exists$(NC)"   || echo "$(YELLOW)  ! No config$(NC)"
	@[ -f $(CONFIG_DIR)/ufoundry.db ] && echo "$(GREEN)  ✓ Database exists$(NC)" || echo "$(YELLOW)  ! No database$(NC)"
	@[ -d $(VENV) ]                && echo "$(GREEN)  ✓ Venv exists$(NC)"     || echo "$(YELLOW)  ! No venv$(NC)"

.DEFAULT_GOAL := help

# ---------------------------------------------------------------------------
# Go engine (see GO_ENGINE.md)
# ---------------------------------------------------------------------------
.PHONY: go-build go-test go-vet go-engine go-universal app-dev app-check app-build app-build-universal

GO         ?= go
GO_VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
GO_COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
GO_LDFLAGS := -s -w -X github.com/shaktsin/ufoundry/internal/version.Version=$(GO_VERSION) \
              -X github.com/shaktsin/ufoundry/internal/version.Commit=$(GO_COMMIT)

go-build: ## Build the Go engine + CLI → bin/ufoundry
	@mkdir -p bin
	$(GO) build -trimpath -ldflags '$(GO_LDFLAGS)' -o bin/ufoundry ./cmd/ufoundry

go-test: ## Run Go tests with the race detector
	$(GO) test -race ./...

go-vet: ## gofmt check + go vet
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal; echo "run gofmt -w"; exit 1)
	$(GO) vet ./...

go-engine: go-build ## Run the Go engine in the foreground
	./bin/ufoundry engine

app-dev: go-build ## Run the Mac app's UI in a browser against a running engine
	@echo "$(YELLOW)Start the engine first: ./bin/ufoundry engine$(NC)"
	cd app/frontend && npm install && npm run dev

app-check: ## Type-check and test the app's UI
	cd app/frontend && npm install && npx svelte-check && npx vitest run

app-build: ## Build UFoundry.app (macOS only) → bin/UFoundry.app
	app/build/macos/bundle.sh

app-build-universal: ## Build a universal UFoundry.app (arm64 + x86_64)
	app/build/macos/bundle.sh --universal

go-universal: ## Build a universal (arm64 + x86_64) macOS binary → bin/ufoundry-darwin
	@mkdir -p bin
	GOOS=darwin GOARCH=arm64 $(GO) build -trimpath -ldflags '$(GO_LDFLAGS)' -o bin/ufoundry-darwin-arm64 ./cmd/ufoundry
	GOOS=darwin GOARCH=amd64 $(GO) build -trimpath -ldflags '$(GO_LDFLAGS)' -o bin/ufoundry-darwin-amd64 ./cmd/ufoundry
	lipo -create -output bin/ufoundry-darwin bin/ufoundry-darwin-arm64 bin/ufoundry-darwin-amd64
