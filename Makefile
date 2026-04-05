.PHONY: setup generate generate-scale generate-append generate-fts \
        bench-cold bench-warm bench-go bench-node bench-all \
        bench-go-fts bench-node-fts bench-fts bench-cold-fts \
        bench-go-modernc bench-go-ncruces bench-cold-modernc bench-cold-ncruces \
        bench-go-mattn-fts bench-go-modernc-fts bench-go-ncruces-fts \
        bench-driver-seq-abc bench-driver-seq-bca bench-driver-seq-cab \
        bench-driver-warm bench-driver-cold \
        results clean-data clean check-go verify

DATA_DIR     ?= ./data
RESULTS_DIR  ?= ./results
COLD_OUT     ?= ./cold-results.txt
DRIVER_OUT   ?= ./driver-compare.txt
COUNT        ?= 10000
SEED         ?= 0

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
	@echo "==> Generation complete (SQLite + index.json)."

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

generate-fts: check-go
	@echo "==> Generating $(COUNT) entries with FTS..."
	cd go && go run -tags sqlite_fts5 ./cmd/generate \
	  --count $(COUNT) \
	  --data-dir ../$(DATA_DIR) \
	  $(if $(filter-out 0,$(SEED)),--seed $(SEED),) \
	  --fts
	@touch $(GO_GENERATE)
	@echo "==> FTS generation complete (SQLite + SQLite-FTS + index.json)."

# ─── Benchmarks ──────────────────────────────────────────────────────────────

bench-cold:
	@rm -f $(COLD_OUT)
	@echo "==> Purging OS buffer cache for Go run..."
	sudo purge
	@$(MAKE) bench-go 2>&1 | tee -a $(COLD_OUT)
	@echo "==> Purging OS buffer cache for Node run..."
	sudo purge
	@$(MAKE) bench-node 2>&1 | tee -a $(COLD_OUT)
	@echo "==> Cold results written to $(COLD_OUT)"

bench-cold-fts:
	@rm -f $(COLD_OUT)
	@echo "==> Purging OS buffer cache for Go FTS run..."
	sudo purge
	@$(MAKE) bench-go-fts 2>&1 | tee -a $(COLD_OUT)
	@echo "==> Purging OS buffer cache for Node FTS run..."
	sudo purge
	@$(MAKE) bench-node-fts 2>&1 | tee -a $(COLD_OUT)
	@echo "==> Cold FTS results written to $(COLD_OUT)"

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

bench-go-fts: check-go
	@echo "==> Running Go FTS benchmark..."
	@mkdir -p $(RESULTS_DIR)/go
	cd go && go run -tags sqlite_fts5 ./cmd/bench \
	  --data-dir ../$(DATA_DIR) \
	  --results-dir ../$(RESULTS_DIR) \
	  --fts

bench-node-fts:
	@echo "==> Running Node.js FTS benchmark..."
	@mkdir -p $(RESULTS_DIR)/node
	cd node && $(NODE) bench.js \
	  --data-dir ../$(DATA_DIR) \
	  --results-dir ../$(RESULTS_DIR) \
	  --fts

bench-fts: bench-go-fts bench-node-fts

bench-go-modernc: check-go
	@echo "==> Running Go benchmark (modernc)..."
	@mkdir -p $(RESULTS_DIR)/go-modernc
	cd go && go run -tags modernc ./cmd/bench \
	  --data-dir ../$(DATA_DIR) \
	  --results-dir ../$(RESULTS_DIR)/go-modernc

bench-go-ncruces: check-go
	@echo "==> Running Go benchmark (ncruces)..."
	@mkdir -p $(RESULTS_DIR)/go-ncruces
	cd go && go run -tags ncruces ./cmd/bench \
	  --data-dir ../$(DATA_DIR) \
	  --results-dir ../$(RESULTS_DIR)/go-ncruces

bench-cold-modernc:
	sudo purge
	@$(MAKE) bench-go-modernc

bench-cold-ncruces:
	sudo purge
	@$(MAKE) bench-go-ncruces

# ─── Driver comparison (FTS, order-controlled) ───────────────────────────────
# Primitive FTS targets, one per driver, consistent results dirs.
# These are the building blocks for the Latin square and cold comparison.

bench-go-mattn-fts: check-go
	@echo "==> Running Go FTS benchmark (mattn)..."
	@mkdir -p $(RESULTS_DIR)/go-mattn
	cd go && go run -tags sqlite_fts5 ./cmd/bench \
	  --data-dir ../$(DATA_DIR) \
	  --results-dir ../$(RESULTS_DIR)/go-mattn \
	  --fts

bench-go-modernc-fts: check-go
	@echo "==> Running Go FTS benchmark (modernc)..."
	@mkdir -p $(RESULTS_DIR)/go-modernc
	cd go && go run -tags modernc ./cmd/bench \
	  --data-dir ../$(DATA_DIR) \
	  --results-dir ../$(RESULTS_DIR)/go-modernc \
	  --fts

bench-go-ncruces-fts: check-go
	@echo "==> Running Go FTS benchmark (ncruces)..."
	@mkdir -p $(RESULTS_DIR)/go-ncruces
	cd go && go run -tags ncruces ./cmd/bench \
	  --data-dir ../$(DATA_DIR) \
	  --results-dir ../$(RESULTS_DIR)/go-ncruces \
	  --fts

# Latin square sequences — no purge between drivers so cache accumulates
# naturally across the sequence.  Each sequence covers all three drivers in a
# different order so position effects cancel when results are averaged.
#
#   Position:   1st (cold-ish)   2nd (partly warm)   3rd (fully warm)
#   Seq ABC:    mattn            modernc              ncruces
#   Seq BCA:    modernc          ncruces              mattn
#   Seq CAB:    ncruces          mattn                modernc
#
# Each driver appears exactly once in each position across the three sequences.

bench-driver-seq-abc:
	@echo "=== Sequence ABC: mattn → modernc → ncruces ===" | tee -a $(DRIVER_OUT)
	@$(MAKE) bench-go-mattn-fts 2>&1 | tee -a $(DRIVER_OUT)
	@$(MAKE) bench-go-modernc-fts 2>&1 | tee -a $(DRIVER_OUT)
	@$(MAKE) bench-go-ncruces-fts 2>&1 | tee -a $(DRIVER_OUT)

bench-driver-seq-bca:
	@echo "=== Sequence BCA: modernc → ncruces → mattn ===" | tee -a $(DRIVER_OUT)
	@$(MAKE) bench-go-modernc-fts 2>&1 | tee -a $(DRIVER_OUT)
	@$(MAKE) bench-go-ncruces-fts 2>&1 | tee -a $(DRIVER_OUT)
	@$(MAKE) bench-go-mattn-fts 2>&1 | tee -a $(DRIVER_OUT)

bench-driver-seq-cab:
	@echo "=== Sequence CAB: ncruces → mattn → modernc ===" | tee -a $(DRIVER_OUT)
	@$(MAKE) bench-go-ncruces-fts 2>&1 | tee -a $(DRIVER_OUT)
	@$(MAKE) bench-go-mattn-fts 2>&1 | tee -a $(DRIVER_OUT)
	@$(MAKE) bench-go-modernc-fts 2>&1 | tee -a $(DRIVER_OUT)

# Full Latin square warm comparison: runs all three sequences back-to-back.
# Each driver gets three data points, one from each cache position.
# Tees all output to $(DRIVER_OUT).

bench-driver-warm:
	@rm -f $(DRIVER_OUT)
	@echo "Driver warm comparison — Latin square (3 sequences × 3 drivers)" | tee $(DRIVER_OUT)
	@$(MAKE) bench-driver-seq-abc
	@$(MAKE) bench-driver-seq-bca
	@$(MAKE) bench-driver-seq-cab
	@echo "==> Warm results written to $(DRIVER_OUT)"

# Cold comparison: purge before each driver so every run starts from a fully
# evicted buffer cache.  Order does not matter here, but we keep it consistent.

bench-driver-cold:
	@rm -f $(DRIVER_OUT)
	@echo "Driver cold comparison — purge before each driver" | tee $(DRIVER_OUT)
	@echo "=== [cold] mattn ===" | tee -a $(DRIVER_OUT)
	sudo purge
	@$(MAKE) bench-go-mattn-fts 2>&1 | tee -a $(DRIVER_OUT)
	@echo "=== [cold] modernc ===" | tee -a $(DRIVER_OUT)
	sudo purge
	@$(MAKE) bench-go-modernc-fts 2>&1 | tee -a $(DRIVER_OUT)
	@echo "=== [cold] ncruces ===" | tee -a $(DRIVER_OUT)
	sudo purge
	@$(MAKE) bench-go-ncruces-fts 2>&1 | tee -a $(DRIVER_OUT)
	@echo "==> Cold results written to $(DRIVER_OUT)"

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
	@echo "==> Index entry count:"
	@python3 -c "import json; print(len(json.load(open('$(DATA_DIR)/index.json'))))"
	@if [ -f $(DATA_DIR)/sqlite-fts/notes.db ]; then \
	  echo "==> SQLite-FTS indexed rows:"; \
	  sqlite3 $(DATA_DIR)/sqlite-fts/notes.db "SELECT COUNT(*) FROM entries_fts;"; \
	fi

# ─── Cleanup ─────────────────────────────────────────────────────────────────

clean-data:
	rm -rf $(DATA_DIR)/sqlite $(DATA_DIR)/sqlite-fts $(DATA_DIR)/index.json $(GO_GENERATE)

clean: clean-data
	rm -rf $(RESULTS_DIR) $(COLD_OUT)
