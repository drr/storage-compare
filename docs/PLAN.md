# Storage Benchmark: SQLite vs Filesystem

## Context

Evaluating read and search performance characteristics between a single SQLite database file and a directory tree of individual markdown files, for a time-stream oriented daily note-taking app. The benchmark will help decide the storage backend. Write performance is not a concern; the focus is cold-cache random reads, day-range reads, and insertion latency (new entries and new versions).

---

## Project Structure

```
/Users/drr/ws/storage-compare/
├── Makefile
├── .gitignore               # data/, results/, *.db, node_modules/
├── data/
│   ├── index.json           # generated; [{id, day, version_count}] for all entries
│   ├── sqlite/notes.db      # generated; gitignored
│   └── fs/YYYY/YYYY-MM/YYYY-MM-DD/
│       ├── YYYY-MM-DD-{GUID}.md          # latest version (no suffix)
│       ├── YYYY-MM-DD-{GUID}-v1.md       # archived version (only exists if updated)
│       └── YYYY-MM-DD-{GUID}-v2.md       # ...
├── go/
│   ├── go.mod
│   ├── cmd/
│   │   ├── generate/main.go # data generator CLI
│   │   └── bench/main.go    # Go benchmark CLI
│   └── internal/
│       ├── model/entry.go
│       ├── wordgen/wordgen.go
│       ├── backend/sqlite.go
│       ├── backend/filesystem.go
│       ├── bench/runner.go
│       ├── bench/ops_sqlite.go
│       ├── bench/ops_fs.go
│       └── bench/cache_darwin.go + cache_other.go
├── node/
│   ├── package.json
│   ├── bench.js
│   ├── ops/sqlite.js
│   ├── ops/fs.js
│   └── runner.js
├── results/                 # gitignored
└── docs/PLAN.md
```

---

## Why No Bash Benchmark

A bash benchmark was considered and removed. The fundamental problem: bash cannot query SQLite or parse YAML frontmatter without spawning a subprocess (`sqlite3`, `python3`, etc.), so each timed operation would be dominated by process startup overhead (~5–25ms), not storage latency. The numbers would measure "how fast is sqlite3 CLI startup" rather than "how fast is SQLite". Go and Node both run in-process and give meaningful storage comparisons.

---

## Data Model

### Entry struct (shared across all backends)

```go
type Entry struct {
    ID         string    // UUIDv4
    VersionID  int       // monotonically increasing per entry (1, 2, 3...)
    EntryType  string    // always "markdown-text"
    CreateTime time.Time // set at entry creation; constant across versions
    ModifyTime time.Time // updated per version
    IsLatest   bool
    Content    string    // markdown body
}
```

### Content distribution

Log-normal with mu=4.0, sigma=1.5; clamped to [5, 20000] words.
- Median ≈ 55 words, 75th pct ≈ 200 words, 99th pct ≈ 3000 words
- Vocabulary: ~5000 common English words embedded as a Go constant
- Paragraphs every 40–80 words; occasional headings for realistic markdown

### Timestamp distribution

Entries spread uniformly across past 2 years (730 days). Timestamps within a day span 08:00–18:00 with jitter.

---

## SQLite Schema

```sql
PRAGMA journal_mode = WAL;
PRAGMA synchronous  = NORMAL;
PRAGMA page_size    = 4096;

CREATE TABLE entries (
    id           TEXT    NOT NULL,
    version_id   INTEGER NOT NULL,
    entry_type   TEXT    NOT NULL DEFAULT 'markdown-text',
    create_time  INTEGER NOT NULL,   -- Unix epoch ms
    modify_time  INTEGER NOT NULL,   -- Unix epoch ms
    is_latest    INTEGER NOT NULL DEFAULT 1 CHECK (is_latest IN (0,1)),
    content      TEXT,
    PRIMARY KEY (id, version_id)
) WITHOUT ROWID;

CREATE INDEX idx_latest_by_id  ON entries(id)          WHERE is_latest = 1;
CREATE INDEX idx_latest_by_day ON entries(modify_time) WHERE is_latest = 1;
```

---

## Filesystem Format

Directory path uses **creation date** (constant across versions):
`./data/fs/YYYY/YYYY-MM/YYYY-MM-DD/`

**Latest version filename** (no version suffix):
`YYYY-MM-DD-{GUID}.md`

**Older versions** (renamed when a new version is written):
`YYYY-MM-DD-{GUID}-v1.md`, `YYYY-MM-DD-{GUID}-v2.md`, ...

File content: YAML frontmatter + markdown body. No `is_latest` field — implicit from absence of version suffix.

---

## Benchmark Run Design

Each run starts with a single `sudo purge` (cold-cache) or no purge (warm). Per-operation latency is recorded for every operation; stats computed at the end.

**Run order:**
1. `read_random`
2. `read_day`
3. `create_entry`
4. `create_version`

**Operation counts:**

| Op | Go | Node |
|----|----|------|
| `read_random` | 1000 | 500 |
| `read_day` | 200 | 100 |
| `create_entry` | 500 | 200 |
| `create_version` | 200 | 100 |

**Operations:**

| Op | SQLite | Filesystem |
|----|--------|------------|
| `read_random` | `SELECT * WHERE id=? AND is_latest=1` | open + parse frontmatter of `GUID.md` |
| `read_day` | `SELECT * WHERE modify_time in [day] AND is_latest=1` | `ReadDir`, filter unversioned, read each |
| `create_entry` | `INSERT` | mkdir-p if needed + write `GUID.md` |
| `create_version` | txn: `UPDATE is_latest=0`, `INSERT` new row | `rename` to `-vN.md` + write new `GUID.md` |

**Output:** ASCII summary table + `results/{lang}/{backend}_{op}_timings.csv` (one ns value per line).

---

## Adaptive Statistical Robustness

Each operation runs adaptively rather than for a fixed N. The benchmark collects samples in rounds and checks after each round whether the median has stabilized to the target precision. N is never decreased below the default minimum.

### Convergence criterion

The 95% confidence interval for the median is computed using the **order statistics method** — no bootstrap required. For n sorted samples, the CI spans indices:

```
lo = floor(n/2 - ceil(0.98 * sqrt(n)))
hi = ceil(n/2  + ceil(0.98 * sqrt(n)))
```

Derived from the binomial: 1.96/2 ≈ 0.98. Convergence is declared when:

```
(sorted[hi] - sorted[lo]) / median < precision
```

Default precision target: **5%** relative CI width. Configurable via `--precision`. Requires at least 30 samples before convergence can be declared.

### Adaptive loop

Samples are collected in rounds of `batchSize` (= the default minimum N). After each round, convergence is checked. If not converged, another round is collected. The loop stops at `maxN = minN * max-factor` (default `max-factor=10`). Pool-based ops (`create_version`) signal exhaustion by returning `ok=false`; after 20 consecutive failures the runner stops regardless.

### Why the median, not the mean

The median is the right target for latency benchmarks because the distributions are heavily right-skewed — a small number of outlier events (WAL checkpoint, fsync, OS scheduler preemption) inflate the mean arbitrarily. The median reflects the typical operation cost. The ops/sec figure in the output is `1s / median`.

### Statistical properties by operation type

| Op type | Variance source | Typical N to converge |
|---------|----------------|----------------------|
| `read_random` (cached) | Low — tight bulk distribution | At or near default N |
| `read_day` | Medium-high — variable entries per day | 1–2× default N; may not converge (structural) |
| `create_entry` | High — periodic WAL/fsync spikes | 3–6× default N |
| `create_version` | Pool-limited — stops at pool size | Whatever the pool allows |

### P95/P99 reliability

The convergence criterion targets the **median only**. P95 and P99 are reported but are inherently noisier:
- At N=1000: ~50 observations above P95 (rough ±30% CI), ~10 above P99 (rough ±60% CI)
- For reliable P99 within ±20%, approximately N=2500 is needed
- The `!` suffix on the `medCI` column flags rows where even the median did not converge — P95/P99 should be treated as directional only in those rows

### Output format

The summary table includes a `medCI` column showing the achieved relative CI width as a percentage. A `!` suffix means the precision target was not reached within `max-factor × minN` samples.

```
Backend    | Operation       |     N |  medCI |     Min |  Median | ...
sqlite     | read_random     |  1000 |   2.7% |  0.01ms |  0.02ms | ...  ← converged
filesystem | read_day        |  2000 |  5.2%! |  0.17ms |  0.35ms | ...  ← did not converge
```

---

## Makefile Targets

```
setup             # go mod download + npm ci
generate          # 10k entries, both backends
generate-scale    # make generate-scale COUNT=1000000
generate-append   # make generate-append COUNT=5000
bench-cold        # sudo purge + bench-all
bench-warm        # bench-all without purge
bench-go          # go benchmark only
bench-node        # node benchmark only
bench-all         # go + node
verify            # sanity checks on generated data
clean-data        # rm data/sqlite data/fs index.json
clean             # clean-data + results
```

---

## Dependencies

- **Go:** `github.com/google/uuid`, `github.com/mattn/go-sqlite3`, `gopkg.in/yaml.v3`
- **Node:** `better-sqlite3` (sync API, no event-loop noise), `gray-matter`, `uuid`
- **System:** `sqlite3` CLI (for `verify` target only), `python3` (for `verify` target only)
- **Homebrew:** `go` only
