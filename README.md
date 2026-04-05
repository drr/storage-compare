# storage-compare

Benchmarks SQLite as a storage backend for a time-stream note-taking app, with an optional SQLite FTS5 variant for full-text search. Measures four core operations (single-entry read, day-range read, create entry, create version) plus FTS search in Go and Node.js across 10k and 1M entry datasets under warm and cold cache conditions.

See [`docs/RESULTS.md`](docs/RESULTS.md) for full tables and analysis.

## Quick Start

```sh
make setup       # install Go and Node dependencies
make generate    # generate 10k entries (SQLite)
make bench-warm  # benchmark (no cache purge)
make bench-cold  # sudo purge + benchmark, results written to cold-results.txt
```

### FTS Mode

```sh
make generate-fts          # generate 10k entries (SQLite + SQLite-FTS)
make bench-fts             # benchmark SQLite vs SQLite-FTS (warm)
make bench-cold-fts        # sudo purge + FTS benchmark

make generate-fts COUNT=1000000  # 1M scale
```

## What Was Benchmarked

**SQLite** stores all entries in a single WAL-mode database with `WITHOUT ROWID` tables and partial indexes on `is_latest`. A single range scan returns a full day's entries.

**SQLite-FTS** (FTS mode only) adds an FTS5 virtual table (`porter unicode61` tokenizer) alongside the regular `entries` table. An `AFTER INSERT` trigger keeps the full-text index in sync automatically. 10% of generated entries have a searchable phrase embedded; `fts_search` queries pick a random phrase and return the top 100 ranked results.

The benchmark uses adaptive sampling — each operation runs until the 95% confidence interval for the median is within 5% relative width, making sample counts self-tuning rather than fixed. Multi-entry operations (`read_day`, `fts_search`) track both total latency and per-entry normalized latency in the output table.

## Key Findings

### SQLite vs SQLite-FTS5

**Adding FTS has zero cost on reads.** `read_random` and `read_day` are statistically indistinguishable between `sqlite` and `sqlite-fts` at both 10k and 1M. The FTS B-tree is a separate structure; reads against `entries` never touch it.

**The write penalty is constant and does not grow with dataset size.** The AFTER INSERT trigger adds a second B-tree write to every insert. This costs 2–3× the median write latency at 10k (0.03ms → 0.09ms for `create_entry` in Go) and the same 2–3× at 1M (0.04ms → 0.09ms). The overhead scales logarithmically — same as the main B-tree — so the ratio between them stays flat.

**FTS search latency grows with dataset size; per-result cost does not stay flat.** At 10k (≈40 matches/phrase), median search is 0.16ms Go / 0.09ms Node with sub-microsecond per-result cost. At 1M (≈4000 candidates/phrase, 100 returned), median is 5.5ms Go / 3.5ms Node, with 0.05ms/result. The per-result cost grew ~50× because FTS5's BM25 ranker must score all candidates to find the top 100 — the number of candidates grows linearly while the number of returned results (100) stays fixed.

**The right application pattern is FTS search → selective content fetch.** `fts_search` at 1M returns 100 ranked IDs in 5.5ms. Following up with 10 `read_random` calls for the entries you want to display adds ~0.2ms. Total: ~5.7ms — faster than a full `read_day` at 15ms and far cheaper than fetching all matching content. Never follow an FTS query with a full content fetch of all results.

**Go's `database/sql` boundary hurts FTS tail latency.** `fts_search` P95 at 1M: Go 60ms, Node 8ms. Go's `database/sql` crosses a language boundary once per row during result iteration; `better-sqlite3` returns all rows in a single native call. For a search-heavy workload at 1M scale, Node is the better runtime.

### Go SQLite Drivers

Three Go SQLite drivers were compared in a controlled experiment (Latin square warm + purge-before-each cold, 1M FTS dataset, 12 total runs):

| Driver | Mechanism | Warm `read_random` | `fts_search` |
|--------|-----------|-------------------|--------------|
| `mattn/go-sqlite3` | CGO | **0.02ms / 51k/s** | **5.3ms** |
| `modernc.org/sqlite` | Pure Go (transpiled C) | 0.03ms / 32k/s | 9.6ms |
| `ncruces/go-sqlite3` | WASM (wazero) | 0.03ms / 32k/s | 11.7ms |
| Node `better-sqlite3` | C++ V8 extension | 0.01ms / 73k/s | 3.5ms |

**At cold cache all three Go drivers converge** — point lookup cost is dominated by page faults (0.56–0.67ms each) and driver dispatch is negligible. **At warm cache, mattn leads**: CGO overhead at this query scale is smaller than the per-call overhead of ccgo transpilation (modernc) or wazero WASM dispatch (ncruces). Switching away from CGO does not improve performance; it slightly degrades it.

The Go vs Node gap is not a CGO problem. `better-sqlite3` has no language boundary during query execution — JS calls directly into C++ which calls SQLite and returns results without crossing a managed runtime boundary. This is an architectural property of the interface, not the driver, and cannot be closed by switching Go drivers.

See [`docs/GO_SQLITE_DRIVERS.md`](docs/GO_SQLITE_DRIVERS.md) for the full experimental design and analysis.

## Recommendation

**SQLite with FTS5 is the right choice.** The read path is unaffected by adding FTS5, and the write overhead is constant and sub-millisecond at any tested scale.

For search-heavy deployments at 1M+ entries, use Node's `better-sqlite3` to avoid the Go `database/sql` tail-latency problem on large FTS result sets. Among Go drivers, stick with `mattn/go-sqlite3`.

## Structure

```
go/          Go benchmark and data generator
node/        Node.js benchmark
data/        generated data (gitignored)
results/     CSV timing output (gitignored)
docs/        plan and results documentation
Makefile     all targets (setup, generate, bench-*)
```

## Dependencies

- **Go** — `github.com/mattn/go-sqlite3` (default; `-tags sqlite_fts5` for FTS), `modernc.org/sqlite` (`-tags modernc`), `github.com/ncruces/go-sqlite3` (`-tags ncruces`), `github.com/google/uuid`
- **Node** (v24.14.0 via nvm) — `better-sqlite3`, `uuid`
