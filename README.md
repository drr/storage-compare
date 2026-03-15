# storage-compare

Benchmarks SQLite vs a filesystem tree of markdown files as storage backends for a time-stream note-taking app. Measures four operations (single-entry read, day-range read, create entry, create version) in Go and Node.js across 10k and 1M entry datasets under warm and cold cache conditions.

See [`docs/RESULTS.md`](docs/RESULTS.md) for full tables and analysis.

## Quick Start

```sh
make setup       # install Go and Node dependencies
make generate    # generate 10k entries in both backends
make bench-warm  # benchmark (no cache purge)
make bench-cold  # sudo purge + benchmark, results written to cold-results.txt
```

## What Was Compared

Both backends store the same data model: versioned markdown notes with YAML frontmatter, spread uniformly across 730 days. SQLite uses a single WAL-mode database with partial indexes on `is_latest`. The filesystem uses a `YYYY/YYYY-MM/YYYY-MM-DD/` directory tree where the latest version has no suffix and older versions are archived as `GUID-v1.md`, `GUID-v2.md`, etc.

The benchmark uses adaptive sampling — each operation runs until the 95% confidence interval for the median is within 5% relative width, making sample counts self-tuning rather than fixed.

## Key Findings

**Single-entry reads are tied at small scale, SQLite wins cold.** At 10k entries with a warm cache, both backends return a single entry in ~0.02ms. The difference emerges when the cache is cold: SQLite's B-tree traversal re-faults quickly (small dataset, few pages), while each filesystem file open requires separate inode and data page faults — 4–5× slower cold. SQLite is the safer choice if the app starts cold frequently.

**Day-range reads favor SQLite at every scale and temperature.** At 10k warm, SQLite is 3–4× faster (0.09ms vs 0.32ms in Go). At 1M warm, the gap is 16× (17ms vs 251ms). This is structural: SQLite issues a single indexed range scan and reads rows sequentially off already-loaded B-tree pages. The filesystem must call `readdir`, filter filenames, then open and parse every file individually — N independent I/O operations with no amortization.

**The per-entry cost inside a day scan exposes the deepest difference.** By normalizing day-scan latency by the number of entries read, we can compare the marginal cost of each entry in a range query vs a standalone random read. For SQLite at 1M cold: random read costs 0.61–0.77ms; day-scan per entry costs 0.01ms — **60–77× cheaper**. SQLite pays the B-tree traversal once and then reads sequentially. For the filesystem, day-scan per entry (0.16–0.18ms) is only 1.4–1.7× cheaper than a random read (0.26–0.27ms), because every file open is an independent I/O regardless of how you got there.

**At 1M entries, warm and cold cache become indistinguishable.** The dataset is large enough that random accesses can't stay hot across hundreds of samples — the OS buffer cache is not a meaningful factor. Both backends show nearly identical warm and cold medians at 1M scale. Cache temperature only matters at small dataset sizes.

**Writes are comparable, SQLite ~2× faster.** Both backends write new entries in 30–70µs median. SQLite's WAL append is simpler than a filesystem `write()` + VFS metadata flush, giving it a 2× edge. Write tails (P99) are dominated by WAL checkpoints on the SQLite side and occasional directory creation on the filesystem side — both show multi-millisecond spikes that are real but rare.

**Directory density degrades the filesystem at scale.** At 1M entries across 730 days, each day-directory holds ~1370 files vs ~14 at 10k. `readdir` over a large directory is more expensive, and locating a specific file's entry within the directory requires scanning more blocks. This causes per-entry filesystem costs to increase ~8× from 10k to 1M even in the warm case — the filesystem is doing harder work per unit, not just more work.

## Recommendation

SQLite is the clear choice for a note-taking app at any realistic scale. The structural advantage of range queries (day view) is large and grows with dataset size, single-entry reads are equal or better at all cache conditions, and writes are faster. The filesystem offers simpler data portability (plain `.md` files) and no dependency on SQLite, but those benefits come at a meaningful and worsening performance cost as the dataset grows.

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

- **Go** — `github.com/mattn/go-sqlite3`, `github.com/google/uuid`, `gopkg.in/yaml.v3`
- **Node** (v24.14.0 via nvm) — `better-sqlite3`, `gray-matter`, `uuid`
