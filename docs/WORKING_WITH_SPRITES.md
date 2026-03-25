# Working with sprites.dev

Practical notes from running long-form benchmarks on sprites.dev. This covers what differs from normal SSH-to-a-VM workflows and what trips you up.

---

## Platform Overview

Sprites are Firecracker microVMs — persistent Linux (Ubuntu x86-64) environments that suspend when idle and resume on demand. Key differences from a regular VM:

- **Suspend/resume purges the OS page cache.** Any benchmark or workload that depends on warm cache must account for this. After an overnight pause, first-access latency can be 3× higher. Always run twice and use the second result.
- **The sprite runs as user `sprite`** (home: `/home/sprite`), not root. Paths like `/root/...` will fail with permission denied.
- **Passwordless sudo is available** for privileged ops (e.g. `sudo sh -c 'echo 3 > /proc/sys/vm/drop_caches'`).
- **Storage is a virtual ext4 block device** (no NVMe). Write tail latency is much worse than local SSD — P99 can be 40–60× higher for workloads that trigger multiple B-tree flushes simultaneously (e.g. SQLite FTS5 WAL checkpoints).
- **Up to 8 vCPUs and 16 GB RAM.** Shared cloud compute; expect ~3× slower than Apple Silicon for CPU-bound and memory-bound work.
- **99 GB disk space** is typically available.

---

## CLI Setup

Install the sprite CLI:
```bash
curl -fsSL https://sprites.dev/install.sh | sh
# installs to ~/.local/bin/sprite
export PATH="$HOME/.local/bin:$PATH"
```

Authenticate once:
```bash
sprite auth   # prompts for API token; enter without echoing via read -rs TOKEN
```

---

## Exec: The `--` Separator

**Always use `--` before the remote command:**
```bash
sprite exec -s mysprite -- bash -c "echo hello"
```

Without `--`, sprite's flag parser interprets `-c` in `bash -c` as a sprite flag and the command fails with `unknown shorthand flag: 'c'`.

**Do not use `--http-post`.** It causes the connection to hang indefinitely after the remote command completes — no output is received, the process never exits. Use the default WebSocket mode for all exec calls.

---

## PATH on the Sprite

The sprite ships with its own node at `/.sprite/bin/node` which appears early in PATH and shadows any other installation. If you install a different runtime to `/usr/local/bin/`, force it via `--env`:

```bash
sprite exec -s mysprite \
  --env PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
  -- bash -c "node --version"
```

This pattern is required any time you install a custom runtime (Go, Node, etc.) to `/usr/local/`.

---

## Long-Running Jobs

WebSocket connections drop if a command runs too long. For anything that takes more than a few minutes:

```bash
# Launch via nohup, return immediately with the PID
sprite exec -s mysprite -- bash -c \
  "nohup bash -c 'cd /home/sprite/myproject && make build > /home/sprite/build.log 2>&1' &"

# Check progress from a separate connection anytime
sprite exec -s mysprite -- bash -c "tail -20 /home/sprite/build.log"

# Check if the process is still running
sprite exec -s mysprite -- bash -c "ps aux | grep myprocess | grep -v grep"
```

The nohup process runs autonomously on the sprite — the connection dropping has no effect on it. Add periodic progress logging to the job itself (e.g. `log.Printf("Progress: %d/%d", n, total)`) so you can poll the log to track completion without holding a connection.

---

## Checkpoints

Checkpoints are point-in-time snapshots of the writable filesystem overlay. They are **fast** (copy-on-write) and **persist independently of the sprite** — checkpoints survive sprite deletion.

**From outside (sprite CLI):**
```bash
sprite checkpoint create v1 -s mysprite
sprite checkpoint list -s mysprite     # always check here first
sprite checkpoint restore v1 -s mysprite
```

**From inside the sprite (during a session):**
```bash
sprite-env checkpoints create --comment "clean setup, deps installed"
sprite-env checkpoints list
```

**Checkpoints are per-sprite.** You cannot restore a checkpoint from sprite A onto sprite B. There is no cross-sprite fork capability. If you want to run parallel experiments from the same base, you must re-run setup on each sprite individually.

**Check before assuming a checkpoint is gone.** Even if a sprite was deleted or re-created, `sprite checkpoint list -s NAME` may show checkpoints you've forgotten about.

---

## Destroying Sprites

`sprite destroy` requires a TTY for confirmation and cannot be piped. Use the API instead:
```bash
sprite api /v1/sprites/SPRITE_NAME -- -X DELETE --max-time 10
```

---

## Services (Persistent HTTP Processes)

For long-running HTTP services that should survive reboots and auto-start on incoming requests, use `sprite-env services` from inside the sprite — not a background process:

```bash
# Inside the sprite
sprite-env services create myapp --cmd /usr/local/bin/myapp --args "--port,8080" --http-port 8080
sprite-env services list
sprite-env services stop myapp
```

Only one service can have `--http-port`. The `--cmd` flag takes only the binary path; arguments go in `--args` as a comma-separated list.

The sprite URL format is: `https://<sprite-name>-<org>.sprites.dev/`

By default the URL requires a Bearer token (`Authorization: Bearer $SPRITE_API_TOKEN`). Make it public with `sprite url update --auth public`.

---

## Installing Runtimes

**Go** — pre-installed at `/usr/local/go/bin/go`. Check version with `go version`.

**Node.js** — the sprite ships with Node v22 at `/.sprite/bin/node`. To install a different version without nvm (nvm installs can timeout over slow connections):
```bash
curl -fsSL https://nodejs.org/dist/v24.14.0/node-v24.14.0-linux-x64.tar.xz \
  | tar -C /usr/local -xJ --strip-components=1
```
Then force `/usr/local/bin` first in PATH (see PATH section above).

---

## Network Policy

Outbound network access is filtered by DNS-based egress policy. The policy is read-only inside the container; update it externally via the API:
```bash
curl -X POST https://api.sprites.dev/v1/sprites/{id}/policy/network \
  -H "Authorization: Bearer $SPRITE_API_TOKEN" \
  -d '{"rules": [{"include": "defaults"}, {"domain": "example.com", "action": "allow"}]}'
```

`{"include": "defaults"}` allows GitHub, npm, PyPI, Docker Hub, and common AI APIs. Raw IP connections are blocked unless the IP was resolved from an allowed domain.

---

## Performance Expectations

On a warm sprite vs local Apple Silicon (M-series):

| | Sprite | Mac | Ratio |
|---|---|---|---|
| SQLite read_random | 0.05ms | 0.02ms | 2.5× |
| SQLite read_day (10k) | 0.34ms | 0.09ms | 3.8× |
| SQLite create_entry | 0.10ms | 0.03ms | 3.3× |
| Filesystem create_entry | 0.10ms | 0.07ms | 1.4× |

**Structural conclusions transfer:** if backend A beats backend B on Mac, it beats it on the sprite too, by roughly the same ratio. The 3× multiplier is safe for planning purposes.

**Exception: write tail latency on virtual disk.** SQLite FTS5 WAL checkpoints at P99 are 40–60× worse on the sprite than on local NVMe — not just 3×. This is a qualitative difference, not just scaling. Plan for it in write-heavy workloads.

---

## In-VM Claude Agent

The sprite ships with a Claude-aware skill at `~/.claude/skills/sprite/SKILL.md` and supporting docs at `/.sprite/llm.txt` and `/.sprite/docs/agent-context.md`. When running Claude Code inside the sprite, it loads these automatically and knows to use `sprite-env` (the local Unix socket CLI) rather than the external `sprite` CLI.

Key files to read when inside a sprite:
- `/.sprite/llm.txt` — platform summary
- `/.sprite/docs/agent-context.md` — full reference (lifecycle, services, checkpoints, network policy)
- `/.sprite/docs/languages.md` — installed runtimes
