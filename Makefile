.PHONY: setup generate generate-scale generate-append \
        bench-cold bench-warm bench-go bench-node bench-all \
        results clean-data clean check-go verify

DATA_DIR    ?= ./data
RESULTS_DIR ?= ./results
COUNT       ?= 10000
SEED        ?= 0

GO_BIN      := $(shell command -v go 2>/dev/null)
GO_GENERATE := $(DATA_DIR)/.generate.stamp

# Resolve node/npm from the pinned nvm version (.nvmrc = 22.15.0).
# Falls back to whatever is on PATH if nvm isn't present.
NODE_VERSION := 24.14.0
NVM_NODE     := $(HOME)/.nvm/versions/node/v$(NODE_VERSION)/bin/node
NVM_NPM      := $(HOME)/.nvm/versions/node/v$(NODE_VERSION)/bin/npm
NODE         := $(if $(wildcard $(NVM_NODE)),$(NVM_NODE),node)
NPM          := $(if $(wildcard $(NVM_NPM)),$(NVM_NPM),npm)

# ─── Setup ───────────────────────────────────────────────────────────────────

setup: check-go
	@echo "==> Installing Go dependencies..."
	cd go && go mod download
	@echo "==> Installing Node.js dependencies (node $(NODE_VERSION))..."
	cd node && $(NPM) ci
	@echo "==> Setup complete."

check-go:
	@if [ -z "$(GO_BIN)" ]; then \
	  echo "ERROR: 'go' not found in PATH."; \
	  echo "Install Go: brew install go   (or visit https://go.dev/dl/)"; \
	  exit 1; \
	fi

# ─── Data Generation ─────────────────────────────────────────────────────────

generate: check-go
	@echo "==> Generating $(COUNT) entries..."
	cd go && go run ./cmd/generate \
	  --count $(COUNT) \
	  --data-dir ../$(DATA_DIR) \
	  $(if $(filter-out 0,$(SEED)),--seed $(SEED),)
	@touch $(GO_GENERATE)
	@echo "==> Generation complete."

generate-scale: check-go
	@echo "==> Generating $(COUNT) entries (scale test)..."
	cd go && go run ./cmd/generate \
	  --count $(COUNT) \
	  --data-dir ../$(DATA_DIR) \
	  --batch-size 1000
	@echo "==> Scale generation complete."

generate-append: check-go
	@echo "==> Appending $(COUNT) entries..."
	cd go && go run ./cmd/generate \
	  --count $(COUNT) \
	  --data-dir ../$(DATA_DIR) \
	  --append

# ─── Benchmarks ──────────────────────────────────────────────────────────────

bench-cold:
	@echo "==> Purging OS buffer cache for Go run..."
	sudo purge
	@$(MAKE) bench-go
	@echo "==> Purging OS buffer cache for Node run..."
	sudo purge
	@$(MAKE) bench-node

bench-warm: bench-all

bench-all: bench-go bench-node

bench-go: check-go
	@echo "==> Running Go benchmark..."
	@mkdir -p $(RESULTS_DIR)/go
	cd go && go run ./cmd/bench \
	  --data-dir ../$(DATA_DIR) \
	  --results-dir ../$(RESULTS_DIR)

bench-node:
	@echo "==> Running Node.js benchmark..."
	@mkdir -p $(RESULTS_DIR)/node
	cd node && $(NODE) bench.js \
	  --data-dir ../$(DATA_DIR) \
	  --results-dir ../$(RESULTS_DIR)

# ─── Results ─────────────────────────────────────────────────────────────────

results:
	@echo "=== Go Results ==="
	@ls $(RESULTS_DIR)/go/*.csv 2>/dev/null && cat $(RESULTS_DIR)/go/*.csv || echo "(none)"
	@echo ""
	@echo "=== Node Results ==="
	@ls $(RESULTS_DIR)/node/*.csv 2>/dev/null && cat $(RESULTS_DIR)/node/*.csv || echo "(none)"

# ─── Verification ────────────────────────────────────────────────────────────

verify:
	@echo "==> Verifying SQLite latest count..."
	@sqlite3 $(DATA_DIR)/sqlite/notes.db "SELECT COUNT(*) FROM entries WHERE is_latest=1;"
	@echo "==> Checking for versioned FS files..."
	@find $(DATA_DIR)/fs -name '*-v[0-9]*.md' | wc -l | xargs echo "Versioned files:"
	@echo "==> Index entry count:"
	@python3 -c "import json; print(len(json.load(open('$(DATA_DIR)/index.json'))))"

# ─── Cleanup ─────────────────────────────────────────────────────────────────

clean-data:
	rm -rf $(DATA_DIR)/sqlite $(DATA_DIR)/fs $(DATA_DIR)/index.json $(GO_GENERATE)

clean: clean-data
	rm -rf $(RESULTS_DIR)
