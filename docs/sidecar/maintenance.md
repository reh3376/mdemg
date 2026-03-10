# MDEMG Sidecar Maintenance Guide

Status: v0.1.0
Date: 2026-02-28
Owner: MDEMG Core  
Audience: Developers operating sidecar across active repositories

---

## 1. Routine Operations

Core lifecycle commands:

```bash
mdemg sidecar status
mdemg sidecar status --format json
mdemg sidecar doctor --format json
mdemg sidecar up
mdemg sidecar down
mdemg sidecar restart
```

Operational expectations:

1. `status` is fast and non-destructive.
2. `doctor` provides actionable remediation.
3. `up/down/restart` are idempotent.
4. JSON outputs are retained as maintenance evidence in `.mdemg/generated/`.

---

## 2. Recommended Maintenance Cadence

## Daily

1. Run `mdemg sidecar status`.
2. Run `mdemg sidecar doctor --format json` before long coding sessions.

## Weekly

1. Run `mdemg sidecar doctor --format json` and archive output.
2. Run `mdemg sidecar status --format json` and compare state transitions.
3. Validate agent attachment still works (`claude-code`, `codex`).

## Monthly

1. Apply sidecar upgrades.
2. Test rollback path.
3. Review logs for repeated degraded states.
4. Review CI gate posture (`observe` / `soft` / `block`) and promote only with stability evidence.

---

## 3. Upgrade Procedure

`mdemg sidecar upgrade` detects version drift and performs a controlled upgrade cycle (down → install → up). Supports `--dry-run`, `--skip-restart`, and `--format json`.

```bash
mdemg sidecar status
mdemg sidecar upgrade --dry-run   # preview changes
mdemg sidecar upgrade
mdemg sidecar doctor --format json
```

Upgrade checkpoints:

1. Lockfile updated.
2. Runtime healthy after restart.
3. Agent adapters still attached.
4. Upgrade report captured and reviewable.

---

## 4. Rollback Procedure

If upgrade fails:

1. Stop runtime: `mdemg sidecar down`.
2. Restore previous binaries/config from backup artifacts.
3. Start runtime: `mdemg sidecar up`.
4. Confirm with `mdemg sidecar doctor --format json`.

Keep rollback evidence in maintenance log.

---

## 5. Backup and Restore

Backup focus:

1. `.mdemg/sidecar.yaml`
2. `.mdemg/sidecar.lock`
3. `.mdemg/backups/` adapter snapshots
4. Any repo-specific sidecar overrides

Restore validation:

1. `mdemg sidecar status`
2. `mdemg sidecar doctor --format json`
3. minimal ingest smoke test

---

## 6. Uninstall and Cleanup

`mdemg sidecar uninstall` cleanly removes all sidecar artifacts. Supports `--force` (stop running services first), `--keep-data` (preserve Neo4j volume), `--dry-run`, and `--format json`. The `.mdemg/` directory is backed up to `.mdemg-backup-<timestamp>/` before removal.

```bash
mdemg sidecar uninstall --dry-run   # preview what would be removed
mdemg sidecar uninstall
```

Post-uninstall checks:

1. No sidecar-managed services are running.
2. Agent attachments are removed or reverted from backup.
3. Hook state is consistent with user choice.

---

## 7. Maintenance Log Template

Use this template per repo:

```text
Date:
Operator:
Repo:
Action:
Before Status:
After Status:
Doctor Summary:
Report Paths:
Issues Encountered:
Rollback Needed (Y/N):
Notes:
```
