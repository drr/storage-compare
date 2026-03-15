# storage-compare

Benchmarks SQLite vs a filesystem tree of markdown files as storage backends for a time-stream note-taking app. Runs adaptive benchmarks in Go and Node.js measuring four operations across warm and cold cache conditions.

## Quick Start

```sh
make setup       # install Go and Node dependencies
make generate    # generate 10k entries in both backends
make bench-warm  # benchmark (no cache purge)
make bench-cold  # sudo purge + benchmark (cold cache)
```

## Operations Benchmarked

| Operation | SQLite | Filesystem |
|-----------|--------|------------|
| `read_random` | `SELECT` by id with partial index | open + parse YAML frontmatter |
| `read_day` | indexed range scan on modify_time | `ReadDir` + open each file |
| `create_entry` | `INSERT` | mkdir-p + write file |
| `create_version` | `UPDATE` + `INSERT` in txn | `rename` to `-vN.md` + write new file |

## Results

See [`docs/RESULTS.md`](docs/RESULTS.md) for full tables and analysis. Summary:

- **SQLite `read_random`**: ~0.01–0.02ms, cache-immune at 10k entries
- **Filesystem `read_random`**: equal warm (~0.02ms), 4–5× slower cold (~0.10ms)
- **`read_day`**: SQLite wins 3–7× in both warm and cold conditions due to a single indexed range scan vs per-file syscalls
- **Writes**: both backends 30–150µs median; SQLite ~2× faster; both show multi-ms tail spikes from WAL checkpoints / directory creation

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
