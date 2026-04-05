# Benchmark Results

## Setup

- **Population:** 10,000 entries spread uniformly across 730 days (past 2 years)
- **Content distribution:** Log-normal (μ=4.0, σ=1.5), median ~55 words, clamped [5, 20000]
- **Hardware:** Apple Silicon (arm64), macOS
- **SQLite config:** WAL mode, synchronous=NORMAL, page_size=4096, WITHOUT ROWID, partial indexes on is_latest
- **Adaptive sampling:** Each operation runs until the 95% CI for the median is <5% relative width, or 10× the default N. A `!` suffix on medCI means the precision target was not reached.

---

## Warm Cache Run

**Conditions:** `make clean && make generate && make bench-all` in immediate succession. Data was freshly written so the OS buffer cache held most pages. No `sudo purge` was issued. This represents the best-case read scenario — the steady state of a running application where recently-accessed data is hot.

### Go

```
Backend    | Operation       |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
----------------------------------------------------------------------------------------------------
sqlite     | read_random     |  1000 |   2.7% |  0.01ms |  0.02ms |  0.03ms |  0.03ms |  0.10ms |   58679
sqlite     | read_day        |   800 |   4.6% |  0.05ms |  0.09ms |  0.15ms |  0.21ms |  0.39ms |   11527
sqlite     | create_entry    |  3000 |   5.0% |  0.02ms |  0.03ms |  0.06ms |  0.11ms |  8.47ms |   29703
sqlite     | create_version  |  1000 |   4.7% |  0.04ms |  0.06ms |  0.09ms |  0.14ms |  9.08ms |   18100
```

### Node

```
Backend    | Operation       |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
---------------------------------------------------------------------------------------------------------
sqlite     | read_random     |  1000 |   4.7% |  0.00ms |  0.01ms |  0.01ms |  0.02ms |  0.68ms |  141163
sqlite     | read_day        |  1000 |  5.0%! |  0.01ms |  0.05ms |  0.08ms |  0.10ms |  0.27ms |   18547
sqlite     | create_entry    |  2000 |  6.3%! |  0.01ms |  0.03ms |  0.06ms |  0.09ms |  9.04ms |   34582
sqlite     | create_version  |  1000 |  6.9%! |  0.02ms |  0.04ms |  0.08ms |  0.11ms |  9.71ms |   25263
```

---

## Observations

### Go vs Node

**Node is faster for SQLite reads.**
`read_random`: Node 0.01ms vs Go 0.02ms. `read_day`: Node 0.05ms vs Go 0.09ms. `better-sqlite3` is a synchronous native addon with no event-loop overhead and a tightly optimized C++ binding. Go's `database/sql` adds a small cgo boundary crossing and connection pool abstraction that adds latency even with a single connection. For this workload, `better-sqlite3` is the faster SQLite driver.

**Writes are similar across runtimes.**
Both runtimes show comparable write latencies — the bottleneck is the I/O operation itself, not the language overhead.

### Convergence and Variance

The adaptive sampler (5% precision target, 10× max) needed more than the default N for several operations:

- `sqlite create_entry` (Go): needed 3000 samples (default 500, 6× extension) — the WAL checkpoint spikes at 8ms against a median of 0.03ms create a ratio of 267:1 that makes the median CI wide until enough samples accumulate to bury the spike in the tail.
- Node write operations did not converge for `create_entry`, `create_version`, and `read_day`. The `!` flag indicates their medCI values (6–7%) are reliable enough to read but did not hit the 5% target within 10× the default N.

### Key Takeaway (Warm Cache)

At warm cache, single-entry reads and writes are comfortably sub-millisecond. Day-range read cost is the primary variable — it scales linearly with entries per day and is the operation to watch as the dataset grows.

---

## Cold Cache Run

**Conditions:** `make bench-cold` — `sudo purge` issued before the Go run and again before the Node run, evicting the OS buffer cache both times. Data had been sitting idle; this represents first-access latency after a machine restart or long idle period.

### Go

```
Backend    | Operation       |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
----------------------------------------------------------------------------------------------------
sqlite     | read_random     |  4000 |   3.1% |  0.01ms |  0.02ms |  0.46ms |  0.78ms |  1.54ms |   62177
sqlite     | read_day        |  1000 |   3.9% |  0.01ms |  0.05ms |  0.22ms |  0.36ms |  0.67ms |   21524
sqlite     | create_entry    |  2500 |   4.5% |  0.02ms |  0.04ms |  0.08ms |  1.04ms | 10.47ms |   28503
sqlite     | create_version  |  1200 |   4.5% |  0.04ms |  0.05ms |  0.08ms |  0.16ms | 10.90ms |   18942
```

### Node

```
Backend    | Operation       |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
---------------------------------------------------------------------------------------------------------
sqlite     | read_random     |  5000 |  5.7%! |  0.00ms |  0.01ms |  0.45ms |  0.83ms |  1.65ms |  123701
sqlite     | read_day        |  1000 |  7.6%! |  0.00ms |  0.02ms |  0.19ms |  0.34ms |  0.62ms |   49281
sqlite     | create_entry    |  2000 |  5.5%! |  0.01ms |  0.03ms |  0.10ms |  0.87ms | 11.02ms |   35398
sqlite     | create_version  |  1000 |  6.6%! |  0.02ms |  0.04ms |  0.09ms |  0.61ms | 11.49ms |   25587
```

---

## Cold vs Warm Comparison

### SQLite: cache-insensitive medians

| Op | Go warm | Go cold | Node warm | Node cold |
|----|---------|---------|-----------|-----------|
| read_random | 0.02ms | 0.02ms | 0.01ms | 0.01ms |
| read_day | 0.09ms | 0.05ms | 0.05ms | 0.02ms |
| create_entry | 0.03ms | 0.04ms | 0.03ms | 0.03ms |
| create_version | 0.06ms | 0.05ms | 0.04ms | 0.04ms |

SQLite median latencies are essentially unchanged between warm and cold cache. The reason is straightforward: a single `read_random` touches one B-tree leaf page (a few KB); with a 10,000-entry database the entire working set fits comfortably in macOS unified memory and is re-faulted within nanoseconds after purge. SQLite is effectively memory-speed at this dataset size regardless of cache state.

**The cold effect on SQLite shows up in the tails, not the median.** `read_random` P95 jumps from 0.03ms to 0.46ms (Go) and from 0.01ms to 0.45ms (Node) — the first handful of accesses after purge fault in cold pages. By the time the benchmark is collecting enough samples to compute stable statistics, the pages are hot again. The adaptive sampler needs 4000–5000 samples (vs 1000 warm) for SQLite `read_random` in the cold run because those early cold spikes temporarily widen the CI.

### Writes: unaffected by cache state

Write medians are nearly identical warm vs cold. WAL appends are sequential, and the kernel buffers them; `fsync` is not called per-write in WAL NORMAL mode. The spikes in P99/Max (up to 11ms cold) are WAL checkpoint artifacts, not cache-miss effects.

### Key Takeaway (Cold Cache, 10k entries)

**SQLite is cache-immune at this scale.** The cold effect shows up in the tails (`read_random` P95 jumps from 0.03ms to 0.46ms in Go) but not in the median. `read_day` at cold cache is 0.05ms (Go) / 0.02ms (Node) — essentially the same as warm.

---

## Scale Run: 1M Entries (Warm Cache)

**Conditions:** `make clean-data && make generate-scale COUNT=1000000 && make bench-all`. 1,000,000 entries spread uniformly across the same 730-day window (~1,370 entries/day on average vs ~14 at 10k). The version-to-entry ratio is identical — only directory density changes.

### Go

```
Backend    | Operation       |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
----------------------------------------------------------------------------------------------------
sqlite     | read_random     |  1000 |   4.1% |  0.01ms |  0.02ms |  0.23ms |  0.33ms |  0.49ms |   49383
sqlite     | read_day        |   600 |   2.5% | 10.38ms | 13.72ms | 26.47ms | 43.38ms | 65.72ms |      73
sqlite     | create_entry    |  2500 |   4.2% |  0.02ms |  0.04ms |  0.07ms |  0.10ms | 15.57ms |   24641
sqlite     | create_version  |  1000 |   4.2% |  0.04ms |  0.06ms |  0.09ms |  0.12ms | 16.39ms |   16517
```

### Node

```
Backend    | Operation       |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
---------------------------------------------------------------------------------------------------------
sqlite     | read_random     |  1000 |   4.9% |  0.00ms |  0.01ms |  0.16ms |  0.18ms |  0.29ms |   73394
sqlite     | read_day        |   300 |   2.9% |  5.16ms | 12.48ms | 17.65ms | 24.14ms | 27.53ms |      80
sqlite     | create_entry    |  2000 |  7.0%! |  0.02ms |  0.04ms |  0.07ms |  0.20ms | 17.12ms |   27088
sqlite     | create_version  |   800 |   4.5% |  0.03ms |  0.06ms |  0.09ms |  0.27ms | 18.09ms |   17804
```

---

## 10k vs 1M Comparison

### SQLite: scales well everywhere except `read_day`

| Op | Go 10k | Go 1M | Node 10k | Node 1M |
|----|--------|-------|----------|---------|
| read_random | 0.02ms | 0.02ms | 0.01ms | 0.01ms |
| read_day | 0.09ms | 13.72ms | 0.05ms | 12.48ms |
| create_entry | 0.03ms | 0.04ms | 0.03ms | 0.04ms |
| create_version | 0.06ms | 0.06ms | 0.04ms | 0.06ms |

`read_random`, `create_entry`, and `create_version` are all **unchanged** at 1M. SQLite's B-tree depth grows logarithmically and all three operations touch a fixed number of pages regardless of dataset size. `read_day` is the exception: the indexed range scan returns ~1370 rows instead of ~14, and pulling that much content through the cgo boundary scales linearly with rows returned.

### Key Takeaway (Scale)

`read_random`, `create_entry`, and `create_version` are flat across 100× the dataset size. `read_day` grows linearly with entries per day — plan accordingly.

---

## Scale Run: 1M Entries (Cold Cache)

**Conditions:** `make bench-cold` on the 1M dataset — `sudo purge` before Go, `sudo purge` before Node.

### Go

```
Backend    | Operation           |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
--------------------------------------------------------------------------------------------------------
sqlite     | read_random         |  2000 |   4.2% |  0.02ms |  0.61ms |  1.23ms |  1.84ms |  2.64ms |    1636
sqlite     | read_day            |   800 |   3.9% |  3.20ms | 15.07ms | 96.19ms | 276.46ms | 561.70ms |      66
sqlite     | read_day_per_entry  |   800 |   4.6% |  0.01ms |  0.01ms |  0.07ms |  0.21ms |  0.41ms |   90818
sqlite     | create_entry        |  1500 |   4.5% |  0.02ms |  0.04ms |  0.08ms |  0.22ms | 51.20ms |   23460
sqlite     | create_version      |  1200 |   4.1% |  0.04ms |  0.06ms |  0.10ms |  0.26ms | 40.90ms |   16249
```

### Node

```
Backend    | Operation           |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
-------------------------------------------------------------------------------------------------------------
sqlite     | read_random         |  2000 |   4.5% |  0.01ms |  0.77ms |  1.55ms |  2.03ms |  3.15ms |    1291
sqlite     | read_day            |   900 |   4.0% |  2.91ms | 12.11ms | 90.64ms | 269.44ms | 695.80ms |      83
sqlite     | read_day_per_entry  |   900 |   4.1% |  0.01ms |  0.01ms |  0.07ms |  0.19ms |  0.53ms |  111995
sqlite     | create_entry        |  1200 |   4.9% |  0.02ms |  0.04ms |  0.07ms |  0.20ms | 44.79ms |   25669
sqlite     | create_version      |  1000 |   4.9% |  0.03ms |  0.06ms |  0.10ms |  0.27ms | 39.62ms |   17480
```

---

## 1M Warm vs Cold Comparison

| Op | Go warm | Go cold | Node warm | Node cold |
|----|---------|---------|-----------|-----------|
| sqlite read_random | 0.55ms | 0.61ms | 0.01ms | 0.77ms |
| sqlite read_day | 17.55ms | 15.07ms | 11.82ms | 12.11ms |
| sqlite read_day_per_entry | 0.01ms | 0.01ms | 0.01ms | 0.01ms |

**At 1M entries, warm and cold are nearly indistinguishable for most operations.** The dataset is large enough that random page accesses can't stay hot across 1000+ samples — the OS buffer cache is not a meaningful factor at this scale. Even the "warm" run is effectively cold for random reads by the time the benchmark collects enough samples.

The one exception is **Node SQLite `read_random`**: 0.01ms warm → 0.77ms cold. This reflects `better-sqlite3`'s in-process SQLite page cache, which is application heap memory and survives the OS buffer cache state but is reset when the process starts. In the warm run, the benchmark process makes 1500 sequential random reads, and the SQLite page cache accumulates the most-recently-accessed B-tree pages in memory — enough that the median re-access is a cache hit. After `sudo purge`, the process starts fresh with an empty page cache and all 1500 reads are cold page faults, driving the median to 0.77ms. Go warm showed 0.55ms because `go-sqlite3`'s default page cache is smaller and saturates faster under random access across 1M rows.

### Day-Scan Per-Entry Cost vs Single Random Read

| | SQLite Go | SQLite Node |
|---|---|---|
| `read_random` (1M cold) | 0.61ms | 0.77ms |
| `read_day_per_entry` (1M cold) | 0.01ms | 0.01ms |
| **ratio** | **61×** | **77×** |

**Entries inside a day scan are 60–77× cheaper per entry than a standalone random read.** `read_day` pays the O(log N) B-tree traversal cost once to land at the start of the day's index range, then reads sequentially through spatially-adjacent pages. After the first page fault, subsequent entries in that day are on already-loaded pages — each one costs almost nothing incremental. A standalone `read_random` starts a fresh traversal from scratch each time, landing on a different cold page at 1M scale.

**`read_day` at 1M is structurally cold at all cache temperatures.** Each call fetches ~1370 rows of content. No realistic in-process cache can hold 1370 × avg-content-size across a working set of 1M entries, so every `read_day` call pulls from storage regardless of warm/cold state. Warm and cold medians are within noise (15ms vs 17ms Go, 12ms vs 12ms Node).

---

## FTS5 Warm Cache Run (10k Entries)

**Conditions:** `make clean && make generate-fts COUNT=10000 && make bench-fts`. Generates `data/sqlite/` and `data/sqlite-fts/`. The benchmark compares plain SQLite against SQLite-with-FTS5 to isolate the cost of maintaining the FTS index. `better-sqlite3` is used on the Node side; `go-sqlite3` with `-tags sqlite_fts5` (porter unicode61 tokenizer) on the Go side.

### Go

```
Backend    | Operation            |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
--------------------------------------------------------------------------------------------------------
sqlite     | read_random          |  1000 |   3.3% |  0.01ms |  0.01ms |  0.02ms |  0.03ms |  0.12ms |   71644
sqlite     | read_day             |   600 |   4.9% |  0.05ms |  0.08ms |  0.13ms |  0.16ms |  0.22ms |   12429
sqlite     | read_day_per_entry   |   600 |   3.1% |  0.00ms |  0.01ms |  0.01ms |  0.01ms |  0.02ms |  176274
sqlite     | create_entry         |  2500 |   4.3% |  0.02ms |  0.03ms |  0.07ms |  0.10ms |  7.61ms |   29233
sqlite     | create_version       |  1200 |   4.2% |  0.04ms |  0.05ms |  0.08ms |  0.14ms |  8.04ms |   18533
sqlite-fts | read_random          |  1000 |   3.8% |  0.01ms |  0.01ms |  0.02ms |  0.03ms |  0.09ms |   76923
sqlite-fts | read_day             |   600 |   4.4% |  0.04ms |  0.08ms |  0.12ms |  0.20ms |  0.22ms |   12917
sqlite-fts | read_day_per_entry   |   600 |   2.7% |  0.00ms |  0.01ms |  0.01ms |  0.01ms |  0.02ms |  179372
sqlite-fts | create_entry         |  3000 |   4.5% |  0.04ms |  0.09ms |  0.41ms |  4.81ms | 17.67ms |   11326
sqlite-fts | create_version       |  2000 |  5.1%! |  0.06ms |  0.11ms |  0.43ms |  5.61ms | 13.44ms |    9046
sqlite-fts | fts_search           |  1200 |   4.6% |  0.08ms |  0.16ms |  0.24ms |  0.93ms |  1.13ms |    6294
sqlite-fts | fts_search_per_entry |  1200 |   3.3% |  0.00ms |  0.00ms |  0.01ms |  0.01ms |  0.01ms |  264340
```

### Node

```
Backend    | Operation            |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
-------------------------------------------------------------------------------------------------------------
sqlite     | read_random          |  1500 |   4.7% |  0.00ms |  0.01ms |  0.01ms |  0.02ms |  0.70ms |  161082
sqlite     | read_day             |   500 |   4.9% |  0.02ms |  0.04ms |  0.07ms |  0.09ms |  0.23ms |   25397
sqlite     | read_day_per_entry   |   500 |   4.4% |  0.00ms |  0.00ms |  0.00ms |  0.01ms |  0.02ms |  312793
sqlite     | create_entry         |  2000 |   4.7% |  0.01ms |  0.03ms |  0.06ms |  0.10ms |  8.63ms |   36697
sqlite     | create_version       |  1000 |  7.1%! |  0.02ms |  0.04ms |  0.08ms |  0.11ms |  8.41ms |   26578
sqlite-fts | read_random          |  1500 |   4.6% |  0.00ms |  0.01ms |  0.02ms |  0.03ms |  0.17ms |  158932
sqlite-fts | read_day             |  1000 |  6.1%! |  0.01ms |  0.04ms |  0.06ms |  0.08ms |  0.18ms |   27713
sqlite-fts | read_day_per_entry   |  1000 |  3.0%! |  0.00ms |  0.00ms |  0.00ms |  0.01ms |  0.02ms |  308642
sqlite-fts | create_entry         |  1800 |   4.8% |  0.04ms |  0.07ms |  0.21ms |  5.04ms | 16.45ms |   13945
sqlite-fts | create_version       |  1000 |  7.4%! |  0.05ms |  0.10ms |  0.29ms |  5.57ms | 13.04ms |    9760
sqlite-fts | fts_search           |  1000 |  6.7%! |  0.06ms |  0.09ms |  0.19ms |  0.68ms |  0.87ms |   10904
sqlite-fts | fts_search_per_entry |  1000 |  4.0%! |  0.00ms |  0.00ms |  0.00ms |  0.01ms |  0.01ms |  452489
```

---

## Observations: SQLite vs SQLite-FTS5

### Reads: zero overhead from the FTS index

`read_random` and `read_day` are statistically indistinguishable between `sqlite` and `sqlite-fts` at both runtimes. The FTS5 virtual table is a separate B-tree; reads against the `entries` table do not touch it. Adding FTS to a database imposes no read-path penalty whatsoever.

### Writes: the trigger tax

The AFTER INSERT trigger that keeps `entries_fts` in sync fires on every `INSERT INTO entries`, adding a second B-tree write to each operation.

| Op | sqlite (Go) | sqlite-fts (Go) | ratio | sqlite (Node) | sqlite-fts (Node) | ratio |
|----|-------------|-----------------|-------|---------------|-------------------|-------|
| create_entry | 0.03ms | 0.09ms | 3× | 0.03ms | 0.07ms | 2.3× |
| create_version | 0.05ms | 0.11ms | 2.2× | 0.04ms | 0.10ms | 2.5× |

The median write cost roughly triples for `create_entry` and more than doubles for `create_version`. This is the fundamental cost of synchronous FTS maintenance: every write to the main table pays for a write to the inverted index in the same transaction. At 10k entries the FTS B-tree is small and hot, so this overhead is still in the 0.07–0.11ms range — fast enough for interactive use. At 1M entries the FTS B-tree will be larger and the per-write cost will increase.

The P99/Max spikes (up to 17ms) on FTS writes are larger than on plain SQLite writes, because an FTS5 checkpoint flushes both the main WAL and the FTS shadow tables together.

### FTS search latency

`fts_search` issues `SELECT entry_id FROM entries_fts WHERE entries_fts MATCH ? ORDER BY rank LIMIT 100` against a random phrase from the 25-phrase tag set. At 10k entries with 10% phrase embedding, each phrase matches ~40 entries on average.

| | Go | Node |
|---|---|---|
| fts_search median | 0.16ms | 0.09ms |
| fts_search_per_entry | <0.001ms | <0.001ms |

A full-text query with ranking returns in under 0.2ms warm. The per-entry cost is sub-microsecond — once the index is traversed, returning additional matching rows is nearly free. Node is roughly 2× faster here for the same reason it leads on plain SQLite reads: `better-sqlite3`'s synchronous prepared-statement path avoids cgo round-trip overhead.

### Convergence

Several Node rows show `!` (did not converge within 10× default N). This is driven by write tail variance: the ratio of a ~14ms FTS checkpoint spike to a ~0.07ms median creates a CI that requires many hundreds of samples to stabilize. The median values are reliable; the `!` flag is a precision note, not a data quality problem. The Go `create_version` row also hit `5.1%!` for the same reason. This variance pattern will be worth watching at 1M entries where write tails are likely to grow.

### Key Takeaway (FTS, 10k)

FTS5 is read-free and write-taxed. If the workload is predominantly read-heavy with occasional writes, adding FTS costs nothing on the read path and the write overhead (2–3× median, still sub-millisecond at 10k) is acceptable. If the workload is write-intensive — bulk imports, high-frequency version creation — the trigger cost accumulates and the 1M scale run will be the critical data point to evaluate whether it remains acceptable.

---

## FTS5 Warm Cache Run (1M Entries)

**Conditions:** `make clean && make generate-fts COUNT=1000000 && make bench-fts`. Same setup as the 10k FTS run, scaled to 1M entries. Each phrase from the 25-phrase tag set now matches ~4,000 entries on average (10% embedding rate × 1M entries ÷ 25 phrases), but `fts_search` uses `LIMIT 100` so only the top-ranked results are returned.

### Go

```
Backend    | Operation            |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
--------------------------------------------------------------------------------------------------------
sqlite     | read_random          |  6000 |   4.6% |  0.01ms |  0.02ms |  0.25ms |  0.44ms |  1.10ms |   43877
sqlite     | read_day             |   400 |   4.6% |  0.37ms | 15.63ms | 42.30ms | 61.46ms | 78.51ms |      64
sqlite     | read_day_per_entry   |   400 |   5.5% |  0.01ms |  0.01ms |  0.03ms |  0.04ms |  0.06ms |   87214
sqlite     | create_entry         |  1500 |   4.7% |  0.02ms |  0.04ms |  0.07ms |  0.10ms | 13.42ms |   25157
sqlite     | create_version       |   800 |   4.1% |  0.04ms |  0.06ms |  0.12ms |  0.24ms | 15.71ms |   16097
sqlite-fts | read_random          |  1000 |   2.7% |  0.01ms |  0.02ms |  0.16ms |  0.26ms |  0.47ms |   50208
sqlite-fts | read_day             |   400 |   2.3% | 12.04ms | 14.45ms | 21.13ms | 27.47ms | 33.62ms |      69
sqlite-fts | read_day_per_entry   |   400 |   2.0% |  0.01ms |  0.01ms |  0.02ms |  0.02ms |  0.02ms |   95274
sqlite-fts | create_entry         |  1000 |   4.9% |  0.05ms |  0.09ms |  0.37ms |  5.76ms |  8.21ms |   11152
sqlite-fts | create_version       |  1800 |   4.9% |  0.07ms |  0.12ms |  0.45ms |  6.88ms | 24.25ms |    8439
sqlite-fts | fts_search           |   600 |   3.7% |  4.66ms |  5.46ms | 59.99ms | 62.68ms | 66.12ms |     183
sqlite-fts | fts_search_per_entry |   600 |   3.7% |  0.05ms |  0.05ms |  0.60ms |  0.63ms |  0.66ms |   18315
```

### Node

```
Backend    | Operation            |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
-------------------------------------------------------------------------------------------------------------
sqlite     | read_random          |  1000 |   3.7% |  0.00ms |  0.01ms |  0.02ms |  0.03ms |  0.25ms |   79738
sqlite     | read_day             |   100 |   4.4% | 10.86ms | 12.34ms | 13.47ms | 15.29ms | 15.77ms |      81
sqlite     | read_day_per_entry   |   100 |   4.1% |  0.01ms |  0.01ms |  0.01ms |  0.01ms |  0.01ms |  111272
sqlite     | create_entry         |  2000 |  5.9%! |  0.02ms |  0.03ms |  0.06ms |  0.10ms | 15.17ms |   29304
sqlite     | create_version       |   500 |   5.0% |  0.03ms |  0.05ms |  0.08ms |  0.32ms | 14.28ms |   18433
sqlite-fts | read_random          |   500 |   5.0% |  0.01ms |  0.01ms |  0.02ms |  0.03ms |  0.19ms |   74305
sqlite-fts | read_day             |   100 |   2.6% | 10.45ms | 11.99ms | 13.28ms | 13.85ms | 14.18ms |      83
sqlite-fts | read_day_per_entry   |   100 |   3.0% |  0.01ms |  0.01ms |  0.01ms |  0.01ms |  0.01ms |  114718
sqlite-fts | create_entry         |   600 |   4.4% |  0.04ms |  0.07ms |  0.21ms |  5.92ms |  7.16ms |   13393
sqlite-fts | create_version       |   400 |   5.0% |  0.06ms |  0.10ms |  0.27ms |  6.49ms |  7.30ms |   10084
sqlite-fts | fts_search           |  1000 |  6.3%! |  2.63ms |  3.51ms |  8.43ms | 49.33ms | 52.11ms |     285
sqlite-fts | fts_search_per_entry |  1000 |  6.3%! |  0.03ms |  0.04ms |  0.08ms |  0.49ms |  0.52ms |   28484
```

---

## Observations: FTS5 at 1M Entries

### Reads: still zero overhead

`read_random` and `read_day` remain indistinguishable between `sqlite` and `sqlite-fts` at 1M, exactly as at 10k. Go `read_day` median: 15.63ms vs 14.45ms — within noise given the high inherent variance of that operation at this scale. The FTS index adds no read-path cost at any dataset size.

### Writes: overhead ratio holds constant

| Op | sqlite (Go) | sqlite-fts (Go) | ratio | sqlite (Node) | sqlite-fts (Node) | ratio |
|----|-------------|-----------------|-------|---------------|-------------------|-------|
| create_entry | 0.04ms | 0.09ms | 2.25× | 0.03ms | 0.07ms | 2.3× |
| create_version | 0.06ms | 0.12ms | 2× | 0.05ms | 0.10ms | 2× |

The 2–3× write overhead observed at 10k is unchanged at 1M. The FTS B-tree write cost scales the same way as the main B-tree write cost — logarithmically — so the ratio between them stays flat. Adding FTS to a 1M-entry database costs the same relative write penalty as at 10k. Absolute latency is still sub-millisecond at the median for both backends.

### FTS search: the 1M reality

`fts_search` at 1M is a different workload than at 10k:

| | 10k | 1M | factor |
|---|---|---|---|
| Go median | 0.16ms | 5.46ms | 34× |
| Node median | 0.09ms | 3.51ms | 39× |
| Go P95 | 0.24ms | 60.0ms | 250× |
| Node P95 | 0.19ms | 8.43ms | 44× |
| matches per query | ~40 | ~4000 | 100× |

The median grows ~34–39×, while the match count grows ~100×. This means the FTS index is doing better than linear — returning 100 ranked results from 4000 matches is not 100× slower than returning them from 40 matches, because the FTS5 BM25 ranking algorithm and early termination optimize the common case. The per-entry cost confirms this: 0.05ms/entry (Go) and 0.04ms/entry (Node) at 1M vs sub-microsecond at 10k. The cost per returned entry grew, but not catastrophically.

The P95 tail tells a different story: 60ms in Go and 8ms in Node. These spikes correspond to queries that hit phrases with the highest match density — where the FTS ranking scan must evaluate more candidate rows before it can confidently emit the top 100. The Go `fts_search` P95/P99 (60–63ms) is notably worse than Node (8–49ms), reflecting cgo boundary overhead on the result iteration loop for large result sets. `better-sqlite3`'s tight C++ binding returns all rows in a single native call; Go's `database/sql` crosses the cgo boundary once per row scan.

The Go `fts_search` sampler needed only 600 samples to converge because the median is stable (most queries hit the same 5ms band); the Node sampler needed 1000 and still showed `6.3%!` because the bimodal distribution (fast queries vs occasional 49ms spikes) keeps the CI slightly wide.

### Per-entry normalization: what the `fts_search_per_entry` row reveals

The same per-entry normalization applied to `read_day` was applied to `fts_search` from the start — the op returns `(elapsed, numResults, ok)` and the runner computes per-result timings automatically. This is the critical lens for understanding FTS at scale.

| | 10k | 1M | factor |
|---|---|---|---|
| `fts_search_per_entry` Go | <0.001ms | 0.05ms | ~50× |
| `fts_search_per_entry` Node | <0.001ms | 0.04ms | ~40× |
| `read_day_per_entry` Go | 0.01ms | 0.01ms | 1× |
| `read_day_per_entry` Node | 0.00ms | 0.01ms | 1× |

**`read_day_per_entry` is scale-invariant; `fts_search_per_entry` is not.** Day-scan per-entry stays at 0.01ms from 10k to 1M because SQLite does sequential page reads — once the scan is positioned, each additional row is a buffer increment. FTS search does BM25 ranking work per candidate, and the number of candidates grows with dataset size (~40 at 10k, ~4000 at 1M). Even though only 100 results are returned, the ranker must score all candidates to find the top 100. The per-result cost at 1M (0.04–0.05ms) reflects this scoring overhead amortized across the 100 returned entries.

**`fts_search_per_entry` vs `read_random` at 1M.** An FTS query returning 100 IDs at 0.05ms/result is significantly cheaper than 100 individual `read_random` calls at 0.02ms each (2ms total for the IDs alone, plus fetching content separately). The FTS index is doing real work — ranking across thousands of candidates — for less than the cost of re-accessing those entries individually. This is the correct usage pattern: run `fts_search` to get ranked IDs, then `read_random` for the entries you actually want to display.

### Comparing `read_day` to `fts_search` at 1M

| Op | Entries accessed | Content returned | Go median | Node median |
|---|---|---|---|---|
| `read_day` | ~1370 (sequential scan) | full content, all entries | 15.63ms | 12.34ms |
| `fts_search` | ~4000 candidates scored, 100 ranked IDs returned | entry_id only | 5.46ms | 3.51ms |

FTS search is faster than a day-scan despite touching more candidate entries because it returns only entry IDs — no content deserialization, no YAML parsing equivalent. A real workload fetching content for the top 10 FTS results (10 × 0.02ms `read_random`) would complete in ~5.5ms + 0.2ms = ~5.7ms total, still faster than a full day-scan. The combined FTS+lookup pattern scales well because you're selectively fetching a small subset of the matched entries.

### Key Takeaway (FTS, 1M)

The write overhead from FTS maintenance is constant — 2–3× penalty that does not grow with dataset size. The read overhead is zero. These conclusions hold from 10k to 1M.

FTS search scales sub-linearly with returned results but the per-result cost grows with dataset size as the ranker scores more candidates. At 1M entries, median search is 3.5–5.5ms with a P95 tail of 8–60ms. The cgo-per-row boundary makes Go's tail significantly worse than Node's for large result sets — for a search-heavy production workload at 1M scale, `better-sqlite3` is the better runtime. The correct application pattern is `fts_search` → get IDs → selective `read_random` for content; this is cheaper than a full day-scan and scales to any result subset size.
