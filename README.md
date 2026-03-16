# storage-compare

Benchmarks SQLite vs a filesystem tree of markdown files as storage backends for a time-stream note-taking app, with an optional SQLite FTS5 variant for full-text search. Measures four core operations (single-entry read, day-range read, create entry, create version) plus FTS search in Go and Node.js across 10k and 1M entry datasets under warm and cold cache conditions.

See [`docs/RESULTS.md`](docs/RESULTS.md) for full tables and analysis.

## Quick Start

```sh
make setup       # install Go and Node dependencies
make generate    # generate 10k entries (SQLite + filesystem)
make bench-warm  # benchmark (no cache purge)
make bench-cold  # sudo purge + benchmark, results written to cold-results.txt
```

### FTS Mode

```sh
make generate-fts          # generate 10k entries (SQLite + SQLite-FTS, no filesystem)
make bench-fts             # benchmark SQLite vs SQLite-FTS (warm)
make bench-cold-fts        # sudo purge + FTS benchmark

make generate-fts COUNT=1000000  # 1M scale
```

FTS mode skips the filesystem backend entirely — no markdown file tree is written. This matters at 1M entries where creating ~1M `.md` files across 730 directories would be expensive and unnecessary for the FTS comparison.

## What Was Compared

**SQLite** stores all entries in a single WAL-mode database with `WITHOUT ROWID` tables and partial indexes on `is_latest`. A single range scan returns a full day's entries.

**Filesystem** uses a `YYYY/YYYY-MM/YYYY-MM-DD/` directory tree. The latest version has no suffix; older versions are archived as `GUID-v1.md`, `GUID-v2.md`, etc. Every read requires individual file opens.

**SQLite-FTS** (FTS mode only) adds an FTS5 virtual table (`porter unicode61` tokenizer) alongside the regular `entries` table. An `AFTER INSERT` trigger keeps the full-text index in sync automatically. 10% of generated entries have a searchable phrase embedded; `fts_search` queries pick a random phrase and return the top 100 ranked results.

The benchmark uses adaptive sampling — each operation runs until the 95% confidence interval for the median is within 5% relative width, making sample counts self-tuning rather than fixed. Multi-entry operations (`read_day`, `fts_search`) track both total latency and per-entry normalized latency in the output table.

## Key Findings

### SQLite vs Filesystem

**Single-entry reads are tied warm, SQLite wins cold.** At 10k warm, both return a single entry in ~0.02ms. Cold cache breaks the tie: SQLite's small working set re-faults quickly while each filesystem file open requires separate inode and data page faults — 4–5× slower cold.

**Day-range reads favor SQLite at every scale and temperature.** At 10k warm: SQLite 3–4× faster (0.09ms vs 0.32ms Go). At 1M warm: 16× faster (17ms vs 251ms). SQLite issues one indexed range scan and reads rows off sequential B-tree pages. The filesystem must `readdir`, filter filenames, then open and parse every file independently — N separate I/O operations with no amortization.

**Per-entry normalization exposes the deepest structural difference.** Day-scan cost per entry in SQLite at 1M cold: 0.01ms. Standalone random read: 0.61ms. That's **60–77× cheaper per entry** inside a range scan — SQLite pays the B-tree traversal once then reads sequentially. Filesystem day-scan per entry (0.16ms) is only 1.4–1.7× cheaper than a random read (0.27ms) because every file open is an independent I/O regardless of access pattern.

**At 1M entries, warm and cold become indistinguishable.** The dataset is too large for the OS buffer cache to stay meaningful across hundreds of benchmark samples. Cache temperature only matters at small dataset sizes.

**Writes are comparable, SQLite ~2× faster.** Both backends write new entries in 30–70µs median. Write tails (P99) are dominated by WAL checkpoints on the SQLite side and occasional directory creation on the filesystem side.

**Directory density degrades the filesystem at scale.** At 1M entries, each day-directory holds ~1370 files vs ~14 at 10k. `readdir`, inode lookup, and directory scans all get proportionally more expensive — per-entry filesystem costs increase ~8× from 10k to 1M even in the warm case.

### SQLite vs SQLite-FTS5

**Adding FTS has zero cost on reads.** `read_random` and `read_day` are statistically indistinguishable between `sqlite` and `sqlite-fts` at both 10k and 1M. The FTS B-tree is a separate structure; reads against `entries` never touch it.

**The write penalty is constant and does not grow with dataset size.** The AFTER INSERT trigger adds a second B-tree write to every insert. This costs 2–3× the median write latency at 10k (0.03ms → 0.09ms for `create_entry` in Go) and the same 2–3× at 1M (0.04ms → 0.09ms). The overhead scales logarithmically — same as the main B-tree — so the ratio between them stays flat.

**FTS search latency grows with dataset size; per-result cost does not stay flat.** At 10k (≈40 matches/phrase), median search is 0.16ms Go / 0.09ms Node with sub-microsecond per-result cost. At 1M (≈4000 candidates/phrase, 100 returned), median is 5.5ms Go / 3.5ms Node, with 0.05ms/result. The per-result cost grew ~50× because FTS5's BM25 ranker must score all candidates to find the top 100 — the number of candidates grows linearly while the number of returned results (100) stays fixed.

**The right application pattern is FTS search → selective content fetch.** `fts_search` at 1M returns 100 ranked IDs in 5.5ms. Following up with 10 `read_random` calls for the entries you want to display adds ~0.2ms. Total: ~5.7ms — faster than a full `read_day` at 15ms and far cheaper than fetching all matching content. Never follow an FTS query with a full content fetch of all results.

**Go's cgo boundary hurts FTS tail latency.** `fts_search` P95 at 1M: Go 60ms, Node 8ms. Go's `database/sql` crosses the cgo boundary once per row during result iteration; `better-sqlite3` returns all rows in a single native call. For a search-heavy workload at 1M scale, Node is the better runtime.

## Recommendation

**SQLite is the clear choice for any write-to-read ratio.** The day-range read advantage is structural and grows with dataset size. If full-text search is needed, add FTS5 — the read path is unaffected and the write overhead is constant and sub-millisecond at any tested scale. The filesystem offers plain `.md` portability and no SQLite dependency, but at the cost of meaningful and worsening performance as the dataset grows.

For search-heavy deployments at 1M+ entries, use Node's `better-sqlite3` to avoid the Go cgo tail-latency problem on large FTS result sets.

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

- **Go** — `github.com/mattn/go-sqlite3` (built with `-tags sqlite_fts5` for FTS targets), `github.com/google/uuid`
- **Node** (v24.14.0 via nvm) — `better-sqlite3`, `gray-matter`, `uuid`
