# MDEMG Sidecar Installation Guide

Status: v0.1.0
Date: 2026-02-28
Owner: MDEMG Core  
Audience: Developers installing sidecar into an existing repository

Note: This guide targets the planned `mdemg sidecar` command surface in `docs/sidecar/roadmap.md`. The roadmap is the normative source for sidecar behavior; external UxTS docs are informative only in this context.

---

## 1. Installation Profiles

Use one profile per repository:

1. `local`: MDEMG runtime and data services run on the same machine as your IDE/agent.
2. `studio-remote`: IDE/agent runs on MacBook, heavy runtime/data services run on MacStudio.

Operating mode for this repository is treated as `brownfield`: install must preserve existing repo state unless explicit force/replace flags are used.

---

## 2. Getting the Binary

### Homebrew (macOS)

```bash
brew install reh3376/mdemg/mdemg
```

### Curl installer (macOS/Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/reh3376/mdemg/main/scripts/install.sh | bash
```

Verifies SHA256 checksum automatically. Override install directory with `INSTALL_DIR`:

```bash
INSTALL_DIR=/usr/local/bin curl -fsSL https://raw.githubusercontent.com/reh3376/mdemg/main/scripts/install.sh | bash
```

### Build from source

```bash
git clone https://github.com/reh3376/mdemg.git
cd mdemg
make build-cli
# Binary at ./bin/mdemg
```

Requires Go 1.24+ and CGO-compatible toolchain.

---

## 3. Prerequisites

## 3.1 Required (All Profiles)

1. Git repository with write access.
2. MDEMG CLI available in `PATH` or as `./bin/mdemg`.
3. Docker engine available and healthy.
4. Ollama installed and running (`ollama serve`).
5. Required Ollama models pulled:
   ```bash
   ollama pull qwen3-embedding:4b        # embeddings (1536 dimensions)
   ollama pull llama3.2:3b-instruct-fp16  # text generation (cognitive features)
   ```

## 3.2 Additional for `studio-remote`

1. SSH access from MacBook to MacStudio.
2. Non-interactive SSH works (`ssh macstudio-tb` succeeds without prompt loops).
3. Docker available on MacStudio.

---

## 4. Preflight Checklist

Run before installation:

```bash
mdemg version
docker ps
```

Expected:

1. CLI returns version/build details.
2. Docker command succeeds without daemon errors.

---

## 5. Local Profile Installation (Step by Step)

From the target repository root:

```bash
mdemg sidecar init --profile local --agents claude-code,codex
mdemg sidecar install --dry-run
mdemg sidecar install
mdemg sidecar up
mdemg sidecar doctor --format json
```

Checkpoints:

1. `.mdemg/sidecar.yaml` exists.
2. `.mdemg/sidecar.lock` exists.
3. `.mdemg/generated/install-report.json` exists.
4. `.mdemg/generated/doctor-report.json` exists.
5. `status` shows runtime healthy.
6. `doctor` reports no blocking failures.

---

## 6. Studio-Remote Profile Installation (MacBook Control, MacStudio Runtime)

From the target repository root on MacBook:

```bash
mdemg sidecar init --profile studio-remote --agents claude-code,codex
mdemg sidecar install --dry-run
mdemg sidecar install
mdemg sidecar up
mdemg sidecar doctor --format json
```

Expected behavior:

1. Sidecar validates remote host connectivity.
2. Runtime services launch on MacStudio.
3. A stable local endpoint is exposed for agent integrations on MacBook.

Validation commands:

```bash
mdemg sidecar status
mdemg sidecar doctor --format json
```

---

## 7. Agent Attachment

Attach adapters after install/up:

```bash
mdemg sidecar attach-agent claude-code
mdemg sidecar attach-agent codex
```

Note: The adapter name is a positional argument, not a flag.

Checkpoints:

1. Agent config backups exist in `.mdemg/backups/`.
2. Agent can list/call MDEMG tools.

---

## 8. First-Run Verification

Run a minimal workflow:

```bash
mdemg sidecar doctor --format json
mdemg ingest --space-id sidecar-smoke --path .
```

Verify:

1. Ingest command succeeds.
2. CMS endpoints are reachable (covered by doctor checks).
3. JSON reports are present under `.mdemg/generated/` for audit/review.

---

## 9. Rollback and Cleanup

To stop services:

```bash
mdemg sidecar down
```

To fully remove sidecar integration:

```bash
mdemg sidecar uninstall
```

Note: `uninstall` is currently a stub (v0.1.0). See `docs/sidecar/friction-log.md` (F1) for the manual workaround.

Before uninstall, ensure any required backups are captured.

---

## 10. Multi-Project Support

MDEMG supports running multiple sidecar instances simultaneously on the same machine. Each project gets its own:

- Neo4j container (named `mdemg-neo4j-{project}`, e.g., `mdemg-neo4j-myapp`)
- Neo4j volume (named `mdemg-neo4j-data-{project}`)
- API server (dynamically allocated port, recorded in `.mdemg/sidecar.lock`)

Port allocation is automatic — if the preferred port (9999 for API, 7687 for Neo4j bolt, 7474 for Neo4j HTTP) is occupied, sidecar scans a range to find a free port. The lock file becomes the single source of truth for runtime ports; all commands (`status`, `doctor`, `down`, `attach-agent`) read from it.

---

## 11. Common Installation Issues

1. Docker unavailable: see `docs/sidecar/troubleshooting.md` (`TRBL-INSTALL-DOCKER`).
2. Remote host unreachable: see `TRBL-REMOTE-SSH`.
3. Agent config merge conflict: see `TRBL-AGENT-CONFIG`.
4. Port conflict: now handled automatically via dynamic allocation. The `install` command shows an advisory warning if the preferred port is busy; `up` will find a free port.
