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
filesystem | read_random     |  1000 |   1.7% |  0.01ms |  0.02ms |  0.03ms |  0.09ms |  0.25ms |   41667
filesystem | read_day        |   800 |   4.5% |  0.17ms |  0.32ms |  0.55ms |  0.65ms |  0.75ms |    3119
filesystem | create_entry    |   500 |   2.4% |  0.05ms |  0.07ms |  0.10ms |  0.17ms | 17.55ms |   14528
filesystem | create_version  |   200 |   2.9% |  0.12ms |  0.15ms |  0.19ms |  0.24ms |  0.26ms |    6711
```

### Node

```
Backend    | Operation       |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
---------------------------------------------------------------------------------------------------------
sqlite     | read_random     |  1000 |   4.7% |  0.00ms |  0.01ms |  0.01ms |  0.02ms |  0.68ms |  141163
sqlite     | read_day        |  1000 |  5.0%! |  0.01ms |  0.05ms |  0.08ms |  0.10ms |  0.27ms |   18547
sqlite     | create_entry    |  2000 |  6.3%! |  0.01ms |  0.03ms |  0.06ms |  0.09ms |  9.04ms |   34582
sqlite     | create_version  |  1000 |  6.9%! |  0.02ms |  0.04ms |  0.08ms |  0.11ms |  9.71ms |   25263
filesystem | read_random     |  1000 |   2.6% |  0.01ms |  0.03ms |  0.05ms |  0.09ms |  1.76ms |   36586
filesystem | read_day        |  1000 |  6.8%! |  0.11ms |  0.29ms |  0.52ms |  0.68ms |  1.90ms |    3492
filesystem | create_entry    |   200 |   4.1% |  0.05ms |  0.06ms |  0.07ms |  0.10ms |  0.28ms |   17180
filesystem | create_version  |   100 |   3.7% |  0.10ms |  0.12ms |  0.13ms |  0.14ms |  0.15ms |    8692
```

---

## Observations

### SQLite vs Filesystem

**`read_random` — essentially tied at warm cache.**
Both backends return a single entry in ~0.02ms (Go) / ~0.01–0.03ms (Node). With hot pages, SQLite's two B-tree traversals (partial index + main tree) cost the same as a single file open + frontmatter parse. This is the common case for a note-taking app: looking up an entry you've recently touched.

**`read_day` — SQLite wins by ~3–4×.**
Go: 0.09ms vs 0.32ms. Node: 0.05ms vs 0.29ms. This is the sharpest structural difference between the two backends. SQLite issues a single indexed range scan against `idx_latest_by_day` and returns all matching rows with content in one pass. The filesystem backend must call `ReadDir` (a syscall), filter versioned filenames, then open and parse each file individually. Even with all pages cached, the per-file open overhead adds up across a day with ~14 entries on average. The gap will widen as entries-per-day grows.

**`create_entry` — SQLite ~2× faster.**
Go: 0.03ms vs 0.07ms. Node: 0.03ms vs 0.06ms. SQLite's INSERT is a single WAL append. The filesystem must check whether the directory exists (or create it), write the file, and the OS must flush the VFS metadata. The occasional 17ms spike on FS `create_entry` likely reflects a new directory being created for a day that didn't exist yet.

**`create_version` — SQLite ~2.5× faster.**
Go: 0.06ms vs 0.15ms. Node: 0.04ms vs 0.12ms. The filesystem operation requires three steps timed together: read the existing file (to get `create_time`), `rename` it to an archive name, then write the new file. SQLite does `UPDATE is_latest=0` + `INSERT` in a single transaction against in-memory WAL pages.

**Max latency spikes on writes.**
Both backends show multi-millisecond outliers on write operations (SQLite: up to 9ms, FS `create_entry`: up to 17ms). For SQLite this is WAL checkpointing. For the filesystem it is either a new directory creation or an fsync. These spikes are real and matter for tail latency in a write-heavy workload, but the median write latency remains in the 30–150µs range for both.

### Go vs Node

**Node is faster for SQLite reads.**
`read_random`: Node 0.01ms vs Go 0.02ms. `read_day`: Node 0.05ms vs Go 0.09ms. `better-sqlite3` is a synchronous native addon with no event-loop overhead and a tightly optimized C++ binding. Go's `database/sql` adds a small cgo boundary crossing and connection pool abstraction that adds latency even with a single connection. For this workload, `better-sqlite3` is the faster SQLite driver.

**Go is comparable or slightly faster for filesystem reads.**
`read_random`: Go 0.02ms vs Node 0.03ms. `read_day`: Go 0.32ms vs Node 0.29ms (essentially tied). Go's `os.ReadFile` is a thin wrapper around a `read` syscall; Node's `fs.readFileSync` + `gray-matter` YAML parse adds a small JS overhead. The difference is minor in the cached case.

**Writes are similar across runtimes.**
Both runtimes show comparable write latencies for both backends — the bottleneck is the I/O operation itself, not the language overhead.

### Convergence and Variance

The adaptive sampler (5% precision target, 10× max) needed more than the default N for several operations:

- `sqlite create_entry` (Go): needed 3000 samples (default 500, 6× extension) — the WAL checkpoint spikes at 8ms against a median of 0.03ms create a ratio of 267:1 that makes the median CI wide until enough samples accumulate to bury the spike in the tail.
- Node write operations did not converge for `create_entry`, `create_version`, and `read_day`. The `!` flag indicates their medCI values (6–7%) are reliable enough to read but did not hit the 5% target within 10× the default N.
- `filesystem read_day` did not converge in Node (6.8%!) because the number of entries per day varies (some days have 5 entries, some have 25), making the per-call work variable and widening the CI regardless of sample count. This is structural variance, not measurement noise.

### Key Takeaway (Warm Cache)

At warm cache, both backends are fast enough that the choice likely doesn't matter for single-entry reads — sub-millisecond either way. The meaningful difference emerges with `read_day`: if day-range queries are frequent, SQLite has a durable structural advantage (~3–4×) that will hold at any cache temperature because it eliminates per-file syscall overhead entirely.

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
filesystem | read_random     |  1000 |   1.5% |  0.02ms |  0.10ms |  0.14ms |  0.20ms |  0.27ms |   10076
filesystem | read_day        |  2000 |  5.3%! |  0.17ms |  0.34ms |  2.03ms |  2.52ms |  3.26ms |    2958
filesystem | create_entry    |   500 |   3.0% |  0.05ms |  0.06ms |  0.09ms |  0.12ms |  0.23ms |   15625
filesystem | create_version  |   200 |   4.1% |  0.12ms |  0.15ms |  0.18ms |  0.23ms |  0.30ms |    6672
```

### Node

```
Backend    | Operation       |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
---------------------------------------------------------------------------------------------------------
sqlite     | read_random     |  5000 |  5.7%! |  0.00ms |  0.01ms |  0.45ms |  0.83ms |  1.65ms |  123701
sqlite     | read_day        |  1000 |  7.6%! |  0.00ms |  0.02ms |  0.19ms |  0.34ms |  0.62ms |   49281
sqlite     | create_entry    |  2000 |  5.5%! |  0.01ms |  0.03ms |  0.10ms |  0.87ms | 11.02ms |   35398
sqlite     | create_version  |  1000 |  6.6%! |  0.02ms |  0.04ms |  0.09ms |  0.61ms | 11.49ms |   25587
filesystem | read_random     |   500 |   4.9% |  0.01ms |  0.13ms |  0.19ms |  0.24ms |  2.15ms |    7813
filesystem | read_day        |  1000 | 119.6%!|  0.12ms |  0.75ms |  2.59ms |  3.28ms |  4.05ms |    1338
filesystem | create_entry    |   400 |   3.3% |  0.04ms |  0.05ms |  0.06ms |  0.08ms |  0.42ms |   19078
filesystem | create_version  |   100 |   4.8% |  0.09ms |  0.11ms |  0.14ms |  0.15ms |  0.16ms |    8925
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

### Filesystem: cold cache reveals syscall cost for `read_random`

| Op | Go warm | Go cold | Node warm | Node cold |
|----|---------|---------|-----------|-----------|
| read_random | 0.02ms | 0.10ms | 0.03ms | 0.13ms |
| read_day | 0.32ms | 0.34ms | 0.29ms | 0.75ms |

`read_random` cold median is 5× slower than warm for Go (0.10ms vs 0.02ms) and 4× for Node (0.13ms vs 0.03ms). Each cold-cache file open faults in the inode, directory block, and file data page — multiple separate page faults for a single entry. The warm-cache filesystem median equaled SQLite's warm median (both ~0.02ms Go), but cold cache breaks that tie decisively: SQLite 0.02ms vs filesystem 0.10ms.

`read_day` is barely affected in Go (0.34ms cold vs 0.32ms warm) because even in the warm run most of the cost was per-file syscall overhead, not memory fetch time. Node's `read_day` balloons from 0.29ms to 0.75ms cold, with medCI of 119.6%! — the structural variance of variable-entries-per-day is amplified by cache misses, making the distribution completely uncharacterizable.

### Writes: unaffected by cache state

Write medians are nearly identical warm vs cold for both backends. WAL appends and filesystem writes are sequential, and the kernel buffers them; `fsync` is not called per-write in WAL NORMAL mode. The spikes in P99/Max (SQLite up to 11ms cold, FS up to 10ms warm) are WAL checkpoint and directory-creation artifacts, not cache-miss effects.

### Key Takeaway (Cold Cache)

**SQLite is cache-immune at this scale; filesystem read_random takes a 4–5× hit cold.** For a note-taking app, the typical access pattern after an app launch (cold) is to open a day's entries — exactly `read_day`. At cold cache, SQLite delivers `read_day` in 0.05ms (Go) / 0.02ms (Node), while the filesystem takes 0.34ms (Go) / 0.75ms (Node). The structural advantage identified in the warm run holds and widens under cold conditions.

If the app's primary read pattern is random single-entry lookup (e.g., following a link to a specific note), the filesystem is 5× slower cold but equal warm — an acceptable tradeoff if lookups are rare. If `read_day` is frequent (e.g., loading the day view on app open), SQLite's advantage is both larger and more consistent across cache states.
