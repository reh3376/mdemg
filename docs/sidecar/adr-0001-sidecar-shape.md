# ADR-0001: Sidecar Package Shape and Architectural Contracts

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Status:** Accepted
**Date:** 2026-02-27
**Deciders:** MDEMG Core
**Authority:** `docs/sidecar/roadmap.md` Sections 1A, 1B, 5.2, 6B, 7A, 7B, 10A

---

## Context

MDEMG needs a sidecar packaging layer that lets developers add persistent memory (CMS) to any repository with minimal friction. The sidecar must support local and remote runtime profiles, integrate with multiple AI agent environments, and provide deterministic lifecycle management.

Key constraints:

1. A unified CLI (`mdemg`) already exists with 12+ subcommands (Phase 93).
2. Two AI agent environments require integration: Claude Code and Codex.
3. The primary user runs a MacBook (control) + MacStudio (runtime offload) workflow.
4. All sidecar behavior must be idempotent, auditable, and reversible.

This ADR resolves the foundational shape decisions before any sidecar Go code is written (Phase S0 gate).

---

## Decision 1: Package Shape

**Extend the existing `mdemg` binary with a `sidecar` subcommand group.**

The sidecar is not a separate binary. All sidecar commands live under `mdemg sidecar <verb>`:

- `mdemg sidecar init`
- `mdemg sidecar install`
- `mdemg sidecar up / down / restart`
- `mdemg sidecar status`
- `mdemg sidecar doctor`
- `mdemg sidecar attach-agent`
- `mdemg sidecar upgrade`
- `mdemg sidecar uninstall`

Rationale:

1. Reuses existing CLI framework (Cobra), config loading, and lifecycle infrastructure.
2. Avoids split ownership between two top-level binaries.
3. Simplifies distribution — one binary to install, upgrade, and manage.
4. Consistent with Phase 93 design (unified CLI foundation).

Rejected alternative: standalone `mdemg-sidecar` binary. This would duplicate config loading, require separate release pipeline, and create user confusion about which binary to invoke.

---

## Decision 2: Profile Model

**Two profiles only: `local` and `studio-remote`. No aliases.**

| Profile | Control Plane | Data Plane | Transport |
|---------|--------------|------------|-----------|
| `local` | Same host | Same host | Direct (localhost) |
| `studio-remote` | MacBook | MacStudio | `docker-context` or `ssh-exec` |

Rules:

1. Profile is declared in `.mdemg/sidecar.yaml` under `profile`.
2. `studio-remote` requires `runtime.remote.host` to be non-empty.
3. Aliases (`studio`, `remote`, `r`) are disallowed in initial release per Section 1B.
4. Unknown profile values fail validation with remediation guidance.

---

## Decision 3: Adapter Contracts (DEC-001 Resolution)

**Versioned adapter modules for Claude Code and Codex with distinct config formats and paths.**

### Claude Code Adapter (`claude-code-v1`)

- **Config path:** `.claude/mcp.json` (project-local)
- **Config format:** JSON
- **Merge strategy:** Deep-merge `mcpServers.mdemg` key into existing JSON
- **Prerequisite:** `.claude/` directory must exist (fail with remediation if not)
- **Backup:** Copy original to `.mdemg/backups/mcp.json.<timestamp>` before mutation
- **Print-only mode:** `--print-only` emits the merge payload to stdout without writing

### Codex Adapter (`codex-v1`)

- **Config path:** `.codex/config.toml` (project-local)
- **Config format:** TOML
- **Merge strategy:** Insert or update `[mcp_servers.mdemg]` section
- **Prerequisite:** `.codex/` directory must exist (fail with remediation if not)
- **Backup:** Copy original to `.mdemg/backups/config.toml.<timestamp>` before mutation
- **Print-only mode:** `--print-only` emits the TOML section to stdout without writing

Common rules:

1. Adapter names are pinned: `claude-code-v1`, `codex-v1`. Version suffix enables future breaking changes.
2. Adapters must never guess config paths. If the expected path does not exist and the prerequisite directory is missing, fail with adapter-specific diagnostic.
3. `attach-agent` is idempotent: re-running produces identical config state.
4. `detach-agent` restores from backup or removes the MDEMG-managed section.

---

## Decision 4: Remote Transport (DEC-002 Resolution)

**`docker-context` is the primary transport. `ssh-exec` is the fallback.**

| Transport | Mechanism | When Used |
|-----------|-----------|-----------|
| `docker-context` | Named Docker context pointing to remote Docker daemon | Default for `studio-remote` |
| `ssh-exec` | Direct SSH command execution on remote host | Fallback when Docker context unavailable |

Configuration in `sidecar.yaml`:

```yaml
runtime:
  remote:
    host: "macstudio-tb"
    transport: "docker-context"   # docker-context | ssh-exec
```

Rules:

1. `transport` defaults to `docker-context` if unspecified.
2. `install` validates the selected transport is functional (Docker context exists, or SSH connectivity succeeds).
3. Transport can be overridden per-command with `--transport` flag (planned).
4. Switching transport requires re-running `doctor` to validate new path.
5. Both transports must support the same lifecycle operations (`up`, `down`, `restart`, health probes).

---

## Decision 5: Migration and Rollback Policy

### Config Versioning

- `sidecar.yaml` includes a `version` field (string enum, initially `"1"`).
- The running binary validates `version` against its supported set.
- Unsupported versions fail with explicit upgrade/downgrade guidance.

### Upgrade Behavior

1. `mdemg sidecar upgrade` backs up current config to `.mdemg/backups/sidecar.yaml.<timestamp>` before any mutation.
2. Minor version upgrades apply automatically (additive fields only).
3. Major version upgrades require explicit `--migrate-config` flag.
4. Failed upgrades auto-restore from the pre-upgrade snapshot and report the failure in the upgrade report.
5. Upgrade reports include `version_before`, `version_after`, `config_migrated`, and `config_migration_steps[]`.

### Uninstall Behavior

1. `mdemg sidecar uninstall` stops all services, detaches all adapters, and removes sidecar-managed artifacts.
2. Backups are retained by default under `.mdemg/backups/`.
3. `--purge-backups` removes backup directory contents (requires explicit opt-in).
4. Uninstall report lists `removed_artifacts[]`, `retained_artifacts[]`, and `detached_adapters[]`.

### Rollback Path

1. Any mutating command that fails mid-execution must restore pre-command state from backup.
2. `doctor` can detect inconsistent post-failure state and provide remediation guidance.
3. Manual rollback: restore `sidecar.yaml` from backup + re-run `install`.

---

## Consequences

### Positive

1. Single binary simplifies user mental model and distribution.
2. Explicit adapter contracts prevent config-guessing bugs.
3. Backup-first policy ensures all mutations are reversible.
4. Profile model is minimal and unambiguous.
5. Transport fallback provides resilience for remote workflows.

### Negative

1. Single binary means sidecar code must not bloat the core binary size significantly.
2. Two config formats (JSON for Claude Code, TOML for Codex) increase adapter implementation surface.
3. `docker-context` as primary transport assumes Docker is available on both hosts.

### Risks

1. Codex config format may evolve; `codex-v1` adapter version pin mitigates this.
2. Docker context setup has a learning curve for users unfamiliar with remote Docker.

---

## References

- Roadmap: `docs/sidecar/roadmap.md`
- Phase 93 (Unified CLI): `docs/specs/phase93-unified-cli-foundation.md`
- Schemas: `docs/sidecar/schemas/README.md`
- Implementation Journal: `docs/sidecar/implementation-journal.md`
- Decision Register: Roadmap Section 10A (DEC-001, DEC-002)
