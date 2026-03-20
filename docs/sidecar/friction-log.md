# MDEMG Sidecar v0.1.0 Friction Log

Status: v0.1.0
Date: 2026-02-28
Owner: MDEMG Core
Audience: Beta testers and early adopters

---

## Purpose

Documents known limitations, workarounds, and rough edges in v0.1.0. Items here are acknowledged, not bugs — they represent scope boundaries for the initial release.

---

## F1: ~~`upgrade` and `uninstall` Are Stubs~~ (RESOLVED in S12)

**Resolved:** Both commands are now fully implemented. `mdemg sidecar upgrade` detects version drift and performs a controlled upgrade cycle (down → install → up). `mdemg sidecar uninstall` cleanly removes all sidecar artifacts with safety backup.

- `upgrade` supports `--dry-run`, `--skip-restart`, and `--format json`
- `uninstall` supports `--dry-run`, `--force`, `--keep-data`, and `--format json`
- `.mdemg/` is always backed up before removal (to `.mdemg-backup-<timestamp>/`)
- Full lifecycle coverage: `init → install → up → doctor → restart → upgrade → down → uninstall`

---

## F2: macOS arm64 Only

**What happens:** Pre-built binaries are only available for macOS arm64 (Apple Silicon). No Linux or Windows binaries are distributed.

**Workaround:** Build from source on other platforms:

```bash
git clone https://github.com/reh3376/mdemg.git
cd mdemg
go build -o bin/mdemg ./cmd/mdemg
```

Requires Go 1.24+ and CGO-compatible toolchain.

---

## F3: Remote Profile Requires Manual SSH Key Setup

**What happens:** The `studio-remote` profile assumes non-interactive SSH access is already configured. Sidecar does not set up SSH keys or manage SSH config entries.

**Workaround:**

1. Set up SSH key: `ssh-keygen -t ed25519`.
2. Copy to remote host: `ssh-copy-id macstudio-tb`.
3. Verify: `ssh macstudio-tb "echo ok"` (must not prompt).

---

## F4: Ollama Must Be Installed Separately

**What happens:** Ollama is the default provider for both embeddings (`qwen3-embedding:8b`) and text generation (`llama3.2:3b-instruct-fp16`). Sidecar does not install or manage Ollama itself. If Ollama is unavailable, `doctor` reports the `ollama.reachable` check as `warn`, not `fail`. If Ollama is running but required models are missing, `doctor` reports `ollama.models` as `warn` with `ollama pull` remediation commands.

**Workaround:** Install Ollama and pull required models manually:

```bash
curl -fsSL https://ollama.com/install.sh | sh
ollama serve &
ollama pull qwen3-embedding:8b
ollama pull llama3.2:3b-instruct-fp16
```

---

## F5: `attach-agent` Uses Positional Argument

**What happens:** The adapter name is a positional argument, not a flag.

```bash
# Correct
mdemg sidecar attach-agent claude-code

# Wrong (will error)
mdemg sidecar attach-agent --agent claude-code
```

Same applies to `detach-agent`.

---

## F6: No Automatic Service Recovery

**What happens:** If Docker containers crash or the MDEMG API stops unexpectedly, sidecar does not auto-restart them. Services remain down until manually restarted.

**Workaround:** Run `mdemg sidecar doctor` to diagnose, then `mdemg sidecar restart` to recover.

---

## F7: ~~No Dynamic Port Allocation~~ (RESOLVED in S10)

**Resolved:** Dynamic port allocation is now implemented. All services (MDEMG API, Neo4j bolt, Neo4j HTTP) use dynamic port allocation with configurable preferred ports. Container names and volumes are project-scoped. Multiple MDEMG projects can run simultaneously on the same machine.

- `sidecar up` automatically finds free ports if preferred ports are busy
- Container names are derived from the project directory (e.g., `mdemg-neo4j-myproject`)
- The lock file (`.mdemg/sidecar.lock`) is the single source of truth for runtime ports
- All downstream commands (`doctor`, `status`, `down`, `attach-agent`) read ports from the lock file
- `sidecar install` port check is advisory (warn), not blocking
