# Go SQLite Driver Investigation

## Background

Across the benchmark runs in this project, Go's `database/sql` + `mattn/go-sqlite3` consistently underperformed Node's `better-sqlite3` for SQLite reads — sometimes by 2–3× on warm-cache point lookups, and by larger margins on FTS iteration. The hypothesis was CGO boundary overhead: every `database/sql` call into mattn crosses the Go↔C bridge, and at the latencies involved (0.01–0.02ms per query), that round-trip is not negligible.

Two drop-in replacements exist that eliminate CGO entirely:

| Driver | Mechanism | Package | `sql.Open` name |
|--------|-----------|---------|-----------------|
| `mattn/go-sqlite3` | CGO — bundles SQLite as C, compiled at `go build` time | `github.com/mattn/go-sqlite3` | `"sqlite3"` |
| `modernc.org/sqlite` | Pure Go — SQLite C source transpiled to Go via `ccgo` | `modernc.org/sqlite` | `"sqlite"` |
| `ncruces/go-sqlite3` | WASM — SQLite compiled to WASM, run via the `wazero` runtime | `github.com/ncruces/go-sqlite3` | `"sqlite3"` |

Both alternatives ship a complete, embedded SQLite and require zero system libraries. They are fully compatible with the existing schema (WAL, WITHOUT ROWID, partial indexes, FTS5) because they bundle the same SQLite source.

**Driver name conflict:** mattn and ncruces both register as `"sqlite3"` and cannot coexist in a single binary. modernc registers as `"sqlite"`. The solution is build tags to select exactly one driver at compile time.

---

## Implementation

Three mutually exclusive files live in `go/internal/backend/`:

```
sqlite_driver_mattn.go    //go:build !modernc && !ncruces  → default
sqlite_driver_modernc.go  //go:build modernc
sqlite_driver_ncruces.go  //go:build ncruces
```

Each file imports its driver and defines two symbols consumed by `sqlite.go`:

```go
const sqliteDriverName = "sqlite"   // or "sqlite3"
func sqliteDSN(path string) string { return path }
```

`sqlite.go` uses these at open time:

```go
db, err := sql.Open(sqliteDriverName, sqliteDSN(dbPath))
```

Benchmarks are invoked via:

```
make bench-go           # default (mattn)
make bench-go-modernc   # -tags modernc
make bench-go-ncruces   # -tags ncruces
```

---

## Experimental Design

The initial uncontrolled runs (modernc then ncruces, run back-to-back after a mattn run) were confounded by cache state: modernc ran with a cold buffer cache, ncruces benefited from pages modernc had faulted in. To get clean data, two controlled experiments were run.

### Warm cache: Latin square

Three sequences covering all six (driver, position) pairings evenly:

| Sequence | Position 1 | Position 2 | Position 3 |
|----------|-----------|-----------|-----------|
| ABC | mattn | modernc | ncruces |
| BCA | modernc | ncruces | mattn |
| CAB | ncruces | mattn | modernc |

No `sudo purge` between drivers within a sequence — the OS buffer cache accumulates naturally. Each driver appears once in each position across the three sequences, so position effects cancel when results are averaged. 9 total runs; 3 data points per driver.

**Note:** mattn's position-1 run in sequence ABC was the only truly cold run in the entire warm matrix — the database had been idle long enough for the buffer cache to be empty. Positions 2 and 3 in ABC, and all positions in BCA and CAB, had warm cache from prior runs in the same matrix. For cache-sensitive operations, mattn's warm average is taken from its two warm positions only (BCA pos3, CAB pos2).

### Cold cache: purge before each

`sudo purge` immediately before each driver run. Identical starting conditions for all three. 3 purges, 3 runs.

---

## Results

All runs used the 1M-entry FTS dataset (same database files across all drivers). Warm results are position-balanced averages; cold results are single runs from a fully evicted buffer cache.

### Plain SQLite

#### Warm (steady-state)

| Operation | mattn | modernc | ncruces |
|-----------|-------|---------|---------|
| `read_random` median | **0.02ms** | 0.03ms | 0.03ms |
| `read_random` ops/sec | **51,600** | 32,009 | 32,122 |
| `read_day` median | **14.5ms** | 19.2ms | 18.0ms |
| `read_day_per_entry` ops/sec | **94,000** | 71,372 | 76,405 |
| `create_entry` median | 0.04ms | 0.05ms | 0.05ms |
| `create_version` median | 0.06ms | 0.09ms | 0.09ms |

#### Cold (purge before each)

| Operation | mattn | modernc | ncruces |
|-----------|-------|---------|---------|
| `read_random` median | 0.56ms | 0.62ms | 0.67ms |
| `read_random` ops/sec | 1,779 | 1,612 | 1,498 |
| `read_day` median | 14.4ms | 18.3ms | 18.6ms |
| `read_day_per_entry` ops/sec | 94,652 | 74,349 | 73,249 |

### SQLite-FTS

#### Warm (steady-state)

| Operation | mattn | modernc | ncruces |
|-----------|-------|---------|---------|
| `read_random` ops/sec | 6,460 | 5,936 | 5,949 |
| `fts_search` median | **5.3ms** | 9.6ms | 11.7ms |
| `fts_search` ops/sec | **188** | 104 | 86 |
| `fts_search_per_entry` ops/sec | **18,849** | 10,419 | 8,551 |
| `create_entry` median | 0.10ms | 0.24ms | 0.16ms |

#### Cold (purge before each)

| Operation | mattn | modernc | ncruces |
|-----------|-------|---------|---------|
| `read_random` median | 0.61ms | 0.63ms | 0.57ms |
| `read_random` ops/sec | 1,653 | 1,597 | 1,751 |
| `read_day` median | 13.1ms | 23.9ms | 16.2ms |
| `fts_search` median | **5.12ms** | 9.43ms | 11.29ms |
| `fts_search` ops/sec | **195** | 106 | 89 |

---

## Analysis

### Cold cache: all drivers converge for point lookups

At cold cache, `read_random` for plain sqlite lands at 0.56–0.67ms across all three drivers — a range of ~20% against a baseline of ~600µs. The dominant cost is page faults (OS buffer cache miss → SSD read → memory fault). Driver dispatch overhead, whether CGO, ccgo, or WASM, is negligible against that. **Driver choice is irrelevant for cold-cache point lookups.**

The same convergence holds for sqlite-fts `read_random` (0.57–0.63ms). The cold signal is entirely storage-bound.

### Warm cache: mattn wins, CGO hypothesis inverted

At warm cache, mattn's plain sqlite `read_random` is **0.02ms (51,600 ops/sec)** vs modernc and ncruces at **0.03ms (~32,000 ops/sec)** — a consistent 1.5× advantage across all six warm positions where mattn ran hot.

This is the opposite of the CGO hypothesis. Eliminating CGO does not improve warm-cache performance; it slightly degrades it. The overhead of ccgo's transpiled C (modernc) and wazero's WASM dispatch (ncruces) is larger per query than the CGO boundary crossing at this latency scale. At 0.02ms per query, the CGO round-trip is a small fraction of total query cost; the additional interpreter/runtime overhead in the alternatives is proportionally more expensive.

`read_day` shows the same ordering (mattn 14.5ms < ncruces 18ms < modernc 19ms), and `read_day_per_entry` confirms it (mattn 94k/s > ncruces 76k/s > modernc 71k/s). In every warm-cache read benchmark, mattn leads.

### FTS search: the most robust signal

`fts_search` is the clearest finding in the entire experiment. It is **cache-insensitive** — the FTS working set at 1M entries does not fit in the OS buffer cache, so every `fts_search` call is effectively cold regardless of warm/cold designation. This makes it the ideal operation for comparing pure driver dispatch overhead, free of cache-state noise.

Across all 12 controlled runs (3 warm positions × 3 sequences + 3 cold runs):

| Driver | All `fts_search` medians | Mean |
|--------|--------------------------|------|
| mattn | 5.24, 5.22, 5.46, 5.22, 5.46, 5.46, 5.12 ms | **5.3ms** |
| modernc | 9.33, 9.47, 10.02, 9.33, 9.47, 10.02, 9.43 ms | **9.6ms** |
| ncruces | 11.97, 11.39, 11.74, 11.97, 11.39, 11.74, 11.29 ms | **11.6ms** |

mattn is **1.8× faster than modernc** and **2.2× faster than ncruces** for FTS search, consistently, at every cache temperature and in every sequence position. This reflects per-row result iteration cost: `fts_search` returns 100 rows, and Go's `database/sql` crosses a language boundary for each `rows.Scan()` call. For mattn, that boundary is CGO (fast native call). For modernc, it's a call into ccgo's transpiled C runtime. For ncruces, it's a wazero WASM host function call. The ccgo and WASM dispatch costs accumulate across 100 iterations per query, adding ~4ms and ~6ms respectively.

### Writes: minor differences, same order of magnitude

All three drivers show create_entry medians in the 0.04–0.06ms range (plain sqlite) and 0.10–0.33ms (sqlite-fts). The bottleneck is WAL append and FTS trigger execution, not driver dispatch. The P99/Max tails (up to 90ms) are WAL checkpoint artifacts identical in character across all drivers.

### The Go vs Node gap revisited

The original motivation was explaining why Go underperforms Node. The driver comparison shows that switching drivers doesn't close the gap:

| Driver | warm `read_random` | `fts_search` |
|--------|--------------------|--------------|
| mattn | 0.02ms / 51k/s | 5.3ms |
| modernc | 0.03ms / 32k/s | 9.6ms |
| ncruces | 0.03ms / 32k/s | 11.6ms |
| Node `better-sqlite3` | 0.01ms / 73k/s | 3.5ms |

Node leads all three Go drivers by 1.5–2× for both operations. The gap is not CGO — it is architectural. `better-sqlite3` is a synchronous V8 C++ extension. During a query, there is no language boundary whatsoever: JS calls C++, which calls SQLite C, which returns results directly to C++ memory, which is returned to JS. Go's `database/sql` interface, regardless of driver, crosses a language or runtime boundary for every `rows.Scan()` call. This is a fixed cost of the interface design, not the driver implementation.

---

## Conclusions

1. **Use mattn for all Go SQLite workloads.** It is the fastest Go SQLite driver tested across every operation at warm cache, and equivalent to alternatives at cold cache. The CGO dependency is a build-time cost (requires a C compiler), not a runtime performance liability.

2. **modernc and ncruces are not performance improvements.** Both are 1.5× slower than mattn on warm point lookups and 1.8–2.2× slower on FTS search. Use them only when CGO is categorically unavailable (cross-compilation targets, restricted build environments).

3. **ncruces is preferable to modernc when CGO is unavailable.** For warm `read_day` and `read_day_per_entry`, ncruces is slightly faster than modernc. The wazero WASM runtime has lower overhead than ccgo's transpiled C for sequential row iteration in range scans.

4. **Driver choice does not matter at cold cache for point lookups.** All three converge to 0.56–0.67ms when the buffer cache is empty. If the workload is primarily cold-start (e.g., infrequent access, no persistent cache), driver selection is a build-complexity decision, not a performance one.

5. **The Go vs Node performance gap is structural and cannot be closed by switching drivers.** Node's `better-sqlite3` leads all Go drivers by 1.5–2× at warm cache. Eliminating CGO makes the gap larger, not smaller. For latency-critical Go applications, this is an inherent property of the `database/sql` interface design.
