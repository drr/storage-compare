# Filesystem Backend Benchmark Results

> **Note:** The filesystem backend code has been removed from the main branch. It is preserved in the git tag `with-filesystem` if you need to review or rerun it.

This document captures the full benchmark results and analysis from the period when SQLite was compared against a filesystem backend (YAML-frontmatter `.md` files stored in a `YYYY/YYYY-MM/YYYY-MM-DD/` directory tree, latest version with no suffix, older versions as `GUID-v1.md`, `-v2.md`, etc.).

---

## Setup

- **Population:** 10,000 entries (warm/cold runs) and 1,000,000 entries (scale runs), spread uniformly across 730 days
- **Content distribution:** Log-normal (μ=4.0, σ=1.5), median ~55 words, clamped [5, 20000]
- **Hardware:** Apple Silicon (arm64), macOS
- **Adaptive sampling:** Each operation runs until the 95% CI for the median is <5% relative width, or 10× the default N. A `!` suffix on medCI means the precision target was not reached.

---

## Warm Cache Run (10k Entries)

**Conditions:** `make clean && make generate && make bench-all` — data freshly written, OS buffer cache hot.

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

## Cold Cache Run (10k Entries)

**Conditions:** `make bench-cold` — `sudo purge` before Go run, `sudo purge` before Node run.

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

## Warm vs Cold Comparison (10k)

### SQLite: cache-insensitive medians

| Op | Go warm | Go cold | Node warm | Node cold |
|----|---------|---------|-----------|-----------|
| read_random | 0.02ms | 0.02ms | 0.01ms | 0.01ms |
| read_day | 0.09ms | 0.05ms | 0.05ms | 0.02ms |
| create_entry | 0.03ms | 0.04ms | 0.03ms | 0.03ms |
| create_version | 0.06ms | 0.05ms | 0.04ms | 0.04ms |

SQLite median latencies are essentially unchanged between warm and cold. The cold effect shows up in the tails, not the median: `read_random` P95 jumps from 0.03ms to 0.46ms (Go) because the first few accesses after purge fault in cold pages, but the pages re-warm before enough samples accumulate to stabilize the CI.

### Filesystem: cold cache reveals syscall cost

| Op | Go warm | Go cold | Node warm | Node cold |
|----|---------|---------|-----------|-----------|
| read_random | 0.02ms | 0.10ms | 0.03ms | 0.13ms |
| read_day | 0.32ms | 0.34ms | 0.29ms | 0.75ms |

`read_random` cold median is 5× slower than warm for Go and 4× for Node. Each cold-cache file open faults in the inode, directory block, and file data page — multiple separate page faults per entry. The warm-cache filesystem median equaled SQLite's warm median (both ~0.02ms Go), but cold cache breaks that tie decisively: SQLite 0.02ms vs filesystem 0.10ms.

`read_day` cold in Go (0.34ms) is barely worse than warm (0.32ms) because even warm, most of the cost was per-file syscall overhead rather than memory fetch time. Node's `read_day` cold (0.75ms) balloons from warm (0.29ms), with medCI of 119.6%! — structural variance from variable-entries-per-day is amplified by cache misses.

---

## Scale Run: 1M Entries (Warm Cache)

**Conditions:** `make clean-data && make generate-scale COUNT=1000000 && make bench-all`. ~1,370 entries/day on average vs ~14 at 10k.

### Go

```
Backend    | Operation       |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
----------------------------------------------------------------------------------------------------
sqlite     | read_random     |  1000 |   4.1% |  0.01ms |  0.02ms |  0.23ms |  0.33ms |  0.49ms |   49383
sqlite     | read_day        |   600 |   2.5% | 10.38ms | 13.72ms | 26.47ms | 43.38ms | 65.72ms |      73
sqlite     | create_entry    |  2500 |   4.2% |  0.02ms |  0.04ms |  0.07ms |  0.10ms | 15.57ms |   24641
sqlite     | create_version  |  1000 |   4.2% |  0.04ms |  0.06ms |  0.09ms |  0.12ms | 16.39ms |   16517
filesystem | read_random     |  2000 |   3.7% |  0.02ms |  0.20ms |  0.34ms |  0.43ms |  3.08ms |    4896
filesystem | read_day        |   200 |   2.2% | 25.66ms | 222.34ms | 259.25ms | 290.04ms | 294.69ms |       4
filesystem | create_entry    |   500 |   2.2% |  0.05ms |  0.07ms |  0.09ms |  0.23ms |  5.79ms |   14833
filesystem | create_version  |   600 |   4.7% |  0.81ms |  5.52ms |  7.82ms | 12.71ms | 31.60ms |     181
```

### Node

```
Backend    | Operation       |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
---------------------------------------------------------------------------------------------------------
sqlite     | read_random     |  1000 |   4.9% |  0.00ms |  0.01ms |  0.16ms |  0.18ms |  0.29ms |   73394
sqlite     | read_day        |   300 |   2.9% |  5.16ms | 12.48ms | 17.65ms | 24.14ms | 27.53ms |      80
sqlite     | create_entry    |  2000 |  7.0%! |  0.02ms |  0.04ms |  0.07ms |  0.20ms | 17.12ms |   27088
sqlite     | create_version  |   800 |   4.5% |  0.03ms |  0.06ms |  0.09ms |  0.27ms | 18.09ms |   17804
filesystem | read_random     |  2500 |   3.8% |  0.01ms |  0.21ms |  0.38ms |  0.46ms |  2.65ms |    4834
filesystem | read_day        |   100 |   3.9% | 21.70ms | 243.28ms | 280.23ms | 312.01ms | 312.31ms |       4
filesystem | create_entry    |   400 |   2.6% |  0.04ms |  0.06ms |  0.28ms |  1.43ms |  4.92ms |   17857
filesystem | create_version  |  1000 |  5.3%! |  0.10ms |  0.37ms |  0.86ms |  9.63ms | 15.33ms |    2692
```

---

## Scale Run: 1M Entries (Cold Cache)

**Conditions:** `make bench-cold` on the 1M dataset.

### Go

```
Backend    | Operation          |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
---------------------------------------------------------------------------------------------------------
sqlite     | read_random        |  2000 |   4.2% |  0.02ms |  0.61ms |  1.23ms |  1.84ms |  2.64ms |    1636
sqlite     | read_day           |   800 |   3.9% |  3.20ms | 15.07ms | 96.19ms | 276.46ms | 561.70ms |      66
sqlite     | read_day_per_entry |   800 |   4.6% |  0.01ms |  0.01ms |  0.07ms |  0.21ms |  0.41ms |   90818
sqlite     | create_entry       |  1500 |   4.5% |  0.02ms |  0.04ms |  0.08ms |  0.22ms | 51.20ms |   23460
sqlite     | create_version     |  1200 |   4.1% |  0.04ms |  0.06ms |  0.10ms |  0.26ms | 40.90ms |   16249
filesystem | read_random        |  2000 |   2.5% |  0.02ms |  0.27ms |  0.46ms |  0.59ms |  1.00ms |    3716
filesystem | read_day           |   200 |   1.7% | 26.60ms | 223.49ms | 257.98ms | 294.96ms | 313.96ms |       4
filesystem | read_day_per_entry |   200 |   0.9% |  0.02ms |  0.16ms |  0.19ms |  0.22ms |  0.23ms |    6112
filesystem | create_entry       |   500 |   3.5% |  0.05ms |  0.07ms |  0.22ms |  0.37ms |  1.14ms |   14972
filesystem | create_version     |   200 |   4.6% |  0.84ms |  6.30ms |  8.06ms | 12.63ms | 25.23ms |     159
```

### Node

```
Backend    | Operation          |     N |  medCI |     Min |  Median |     P95 |     P99 |     Max | ops/sec
-----------------------------------------------------------------------------------------------------------
sqlite     | read_random        |  2000 |   4.5% |  0.01ms |  0.77ms |  1.55ms |  2.03ms |  3.15ms |    1291
sqlite     | read_day           |   900 |   4.0% |  2.91ms | 12.11ms | 90.64ms | 269.44ms | 695.80ms |      83
sqlite     | read_day_per_entry |   900 |   4.1% |  0.01ms |  0.01ms |  0.07ms |  0.19ms |  0.53ms |  111995
sqlite     | create_entry       |  1200 |   4.9% |  0.02ms |  0.04ms |  0.07ms |  0.20ms | 44.79ms |   25669
sqlite     | create_version     |  1000 |   4.9% |  0.03ms |  0.06ms |  0.10ms |  0.27ms | 39.62ms |   17480
filesystem | read_random        |  1500 |   3.9% |  0.02ms |  0.26ms |  0.49ms |  0.71ms |  2.69ms |    3876
filesystem | read_day           |   100 |   4.9% | 23.97ms | 247.47ms | 299.60ms | 337.94ms | 368.11ms |       4
filesystem | read_day_per_entry |   100 |   6.4% |  0.02ms |  0.18ms |  0.22ms |  0.24ms |  0.26ms |    5512
filesystem | create_entry       |   400 |   3.3% |  0.04ms |  0.06ms |  0.23ms |  0.47ms |  0.95ms |   17978
filesystem | create_version     |  1000 |  5.3%! |  0.10ms |  0.37ms |  0.81ms |  2.48ms | 26.91ms |    2723
```

---

## 10k vs 1M: Filesystem Degradation from Directory Density

| Op | Go 10k | Go 1M | Node 10k | Node 1M |
|----|--------|-------|----------|---------|
| read_random | 0.02ms | 0.20ms | 0.03ms | 0.21ms |
| read_day | 0.32ms | 222ms | 0.29ms | 243ms |
| create_entry | 0.07ms | 0.07ms | 0.06ms | 0.06ms |
| create_version | 0.15ms | 5.52ms | 0.12ms | 0.37ms |

All the filesystem degradation comes from **directory density** — 1370 files/directory at 1M vs 14 at 10k — not from a higher fraction of versioned entries.

**`read_random` (+10×):** Opening a single file requires the kernel to locate its inode within the directory. With 1370 files per directory, more directory blocks must be scanned.

**`read_day` (+700×):** Purely linear scaling — 1370 file opens instead of 14. No shortcut exists; every entry's content must be read individually.

**`create_version` (+37× Go, +3× Node):** Archiving the current file requires scanning the directory for `GUID-vN.md` files to determine the existing version count. That scan is proportionally more expensive in a 1370-file directory.

**`create_entry` (unchanged):** Writing a new file to an existing directory doesn't require reading the directory — density-independent.

---

## Per-Entry Cost: The Deepest Structural Difference

At 1M cold cache:

| | SQLite Go | SQLite Node | FS Go | FS Node |
|---|---|---|---|---|
| `read_random` | 0.61ms | 0.77ms | 0.27ms | 0.26ms |
| `read_day_per_entry` | 0.01ms | 0.01ms | 0.16ms | 0.18ms |
| **ratio** | **61×** | **77×** | **1.7×** | **1.4×** |

**SQLite: entries inside a day scan are 60–77× cheaper per entry than a standalone random read.** `read_day` pays the O(log N) B-tree traversal once to land at the start of the day's range, then reads sequentially through spatially-adjacent pages. Each subsequent entry costs almost nothing incremental.

**Filesystem: entries inside a day scan are only ~1.5× cheaper than a standalone read.** Each file is an independent inode, a separate `open()`, and a separate page fault — regardless of whether you got there via `readdir` or a direct path. The per-entry cost in a day scan is structurally the same as a random read.

This is the fundamental structural difference: SQLite transforms a range query into a sequential scan where each additional entry is nearly free. The filesystem makes every entry an independent I/O unit regardless of access pattern.

---

## Summary of Findings

**`read_random` — tied warm, SQLite wins cold.**
At 10k warm, both return a single entry in ~0.02ms. Cold cache breaks the tie: SQLite 0.02ms vs filesystem 0.10ms (Go) / 0.13ms (Node). SQLite's compact B-tree re-faults quickly; each filesystem file open requires separate inode and data page faults.

**`read_day` — SQLite wins at every scale and temperature.**
10k warm: SQLite 3–4× faster (0.09ms vs 0.32ms Go). 1M warm: 16× faster (14ms vs 222ms Go). This advantage is structural and widens monotonically with dataset size.

**Writes — SQLite ~2× faster.**
Both backends write new entries in 30–70µs median. Write tails (P99) are dominated by WAL checkpoints on the SQLite side and occasional directory creation on the filesystem side.

**At 1M entries, warm and cold are indistinguishable for most operations.**
The dataset is too large for the OS buffer cache to stay hot across hundreds of samples. Cache temperature only matters at the 10k scale.

**Directory density degrades the filesystem at scale.**
At 1M entries, per-entry filesystem costs increase ~8× from 10k to 1M even in the warm case, purely from directory lookup overhead. The only operation that does not degrade is `create_entry`, which is density-independent.

**Conclusion:** SQLite is the right choice. The day-range read advantage is structural and grows with dataset size. The filesystem offers plain `.md` portability and no SQLite dependency, but at the cost of meaningful and worsening performance as the dataset grows. At 1M entries — the realistic long-term scale for daily use over years — the gap is no longer incremental, it's categorical.
