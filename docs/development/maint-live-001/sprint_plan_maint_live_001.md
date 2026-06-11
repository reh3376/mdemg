# Sprint Plan MAINT-LIVE-001 — Scheduled Maintenance Actually Runs

## 1. Header & Metadata

- **Sprint ID:** MAINT-LIVE-001 (Roadmap Q3 Phase 1, rank #4)
- **Sprint line:** `docs/development/maint-live-001/`
- **Date opened:** 2026-06-11
- **Branch:** `reh3376_dev01`
- **Target version:** v0.10.x
- **Estimated effort:** ~2 dev-days
- **OpenAI spend:** $0
- **Risk level:** Medium (first-ever live prune on the production graph; mitigated by dry-run preview, tombstone-not-delete node semantics, and batch caps)

## 2. Problem Statement

Weekly decay+prune has **never executed**: `mdemg maintenance` defaults
`--dry-run=true` and the LaunchAgent plist passes no override — every
scheduled run previews and reports success (inside NOSILENT-001's blind
spot: the job "ran", so no failure/staleness alert fires). Consequences
visible tonight: Memory Bloat alerts (79k+ nodes in mdemg-dev), ~36k
prunable near-zero edges, ~20k orphan SymbolNodes, and decay never applied.
HIDDEN-WEIGHT-001 makes this sprint properly sequenced: the weights that
decay/prune compute over are finally real.

**Beta-readiness framing (operator, 2026-06-11):** the application must run
properly unattended — a hygiene loop that silently no-ops is
disqualifying for beta.

## 3. Scope & Constraints

**In scope:**
- LaunchAgent runs LIVE: `--dry-run=false` in both plist copies
  (packaging + embedded templates, CI-checked) + refresh of the installed
  plist on this machine.
- **Dry-run visibility:** maintenance records `dry_run` into the job-event
  `metadata` JSONB (design decision: NO new column/migration —
  `scheduled_job_events.metadata` exists for exactly this; the evaluator
  filters `metadata->>'dry_run'`. Disclosed deviation from the roadmap's
  "dry_run field" wording, same queryability, zero schema risk).
- **Evaluator rule `maintenance_no_live_run`** (distinct service
  `maintenance-liveness`): maintenance rows exist in the lookback window
  but NONE ran live — the only-ever-dry-runs pattern self-reports. Config:
  `MAINT_LIVE_ALERT_ENABLED` (default true), `MAINT_LIVE_LOOKBACK_DAYS`
  (default 8 — weekly cadence + buffer).
- **`mdemg upgrade` (darwin) refreshes LaunchAgent plists + Claude hooks**
  from the embedded templates — so fixes like this one actually reach
  installed machines instead of rotting beside a new binary.
- Tier 3: the **first-ever live maintenance run** on mdemg-dev, preview-
  first, with full before/after evidence.

**Out of scope:** retuning decay/prune parameters (defaults stand;
HIDDEN-CHURN-001 owns hierarchy quality); maintenance for non-darwin
schedulers (cron/systemd docs exist); the Neo4j-backup jobhealth gap
(BACKUP-RESTORE-VERIFY-001).

**Safety constraints (verified in code before planning):**
- Node handling is **tombstone-only** (`shouldTombstoneNode`), never
  delete, with protections: abstraction-chain membership, degree >
  max-degree, observations within 90d retention. Compatible with the
  operator's keep-the-orphans decision (which barred synthetic backfill,
  not normal lifecycle).
- Edge pruning DELETEs only `weight < 0.01 AND evidence < 3 AND age > 30d`
  edges — the designed lifecycle, meaningful now that weights are real.
- Live run protocol: dry-run preview counts → operator-visible summary →
  live run → post-verification (small-batch-first rule satisfied by the
  preview + batched execution at 1000/txn).

## 4. Dependencies

NOSILENT-001 (`reportScheduledJob`, V0024, evaluator rules pattern);
HIDDEN-WEIGHT-001 (real weights); HOOKSYNC-001 (alert delivery to surface
the new rule); launchd template CI parity check (existing).

## 5. Implementation Plan

- **Epic 0 — Investigation + plan (done):** maintenance flow read;
  prune semantics verified tombstone-not-delete; plist confirmed
  override-free; metadata-vs-column decided.
- **Epic 1 — Live by default on the schedule (~0.25d):**
  `--dry-run=false` in both `com.mdemg.maintenance.plist` copies;
  `reportScheduledJob` gains metadata support; `maintenance` records
  `dry_run`; refresh the installed plist (substitutions per the
  service-install pattern learned in HOOKSYNC Epic 6).
- **Epic 2 — Liveness rule (~0.25d):** `MaintenanceLivenessRules` +
  config + serve wiring + Tier 1 tests (mirrors HookHealthRules).
- **Epic 3 — `mdemg upgrade` refresh (~0.5d):** darwin path re-renders
  embedded plist templates (with substitutions) into
  `~/Library/LaunchAgents` for mdemg-managed agents + re-runs the hooks
  install merge; gated + logged; `--no-refresh` opt-out.
- **Epic 4 — Tier 3: first live maintenance (~0.5d):** dry-run preview on
  mdemg-dev (counts recorded) → live run → verify: edges deleted vs
  preview, tombstone counts + protection reasons, node/edge totals +
  Memory Bloat trend, retrieval control probe healthy, null-weight gauge
  still 0, job event row carries `dry_run=false`, liveness rule loaded
  and silent. Surprise findings get standalone fix-commits.
- **Epic 5 — Documentation (final, never cut):** feature doc
  `docs/features/scheduled-maintenance.md`; CHANGELOG; CLAUDE.md;
  roadmap tick; post.md.

## 6. Testing Plan (3 tiers)

- **Tier 1:** rule construction tests; metadata plumbing test;
  upgrade-refresh template rendering test (substitution correctness).
- **Tier 2:** plist copies identical (existing CI step covers); `go test`
  suites; UATS untouched (no API surface change beyond rule).
- **Tier 3 (live):** Epic 4 — the real first run with before/after
  evidence from Neo4j + TSDB + the alert file. Live smoke item: *run
  maintenance live against mdemg-dev, observe deleted-edge/tombstone
  counts in Neo4j, the dry_run=false job event in TSDB, and bloat-trend
  + retrieval health after.*

## 7. Commit Strategy

One commit per epic; surprises get standalone fix-commits; push →
auto-PR → summary.

## 8. Verification Checklist

- [ ] Plist copies pass `--dry-run=false`; installed plist refreshed
- [ ] Job events carry `dry_run` in metadata (live row verified)
- [ ] `maintenance_no_live_run` loaded (evaluator count +1), silent after
      the live run, fires in a forced dry-run-only simulation
- [ ] `mdemg upgrade` refresh renders correct plists/hooks (darwin)
- [ ] Live run: preview counts ≈ live counts; tombstones have protection
      audit; retrieval probe healthy post-run; null gauge 0
- [ ] Suites green; lint clean; docs complete

## 9. Documentation Update — Epic 5 above.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| First live prune removes load-bearing edges | Low | High | Criteria are weight<0.01+evidence<3+age>30d (designed lifecycle); preview-first; retrieval control probe after; edges re-learnable via Hebbian paths |
| Tombstoning sweeps nodes the operator values | Low | Medium | Tombstone ≠ delete (status flag, recoverable); protection rules verified in code; post-run audit of tombstone reasons |
| Upgrade refresh clobbers operator-customized plists | Medium | Medium | Refresh only mdemg-managed agents (marker check), `--no-refresh` opt-out, log every file touched |
| Weekly live runs surprise operators upgrading | Low | Low | CHANGELOG + feature doc call it out; the dry-run default on the CLI itself is unchanged (manual runs still preview) |

## 11. Documents Accessed

- `internal/cli/maintenance.go`, `prune.go` (tombstone + edge-prune
  semantics, protections), `job_report.go`
- `packaging/launchd/com.mdemg.maintenance.plist` + embedded copy
- `internal/alert/rules.go` (rule patterns), `internal/config/config.go`
- `docs/development/roadmap/ROADMAP_2026Q3.md`; operator beta-readiness
  directive (2026-06-11)

## 12. Rollback Procedures

- Plist: remove `--dry-run=false` (reverts to preview-only).
- Tombstoned nodes: status flag reversible by Cypher SET.
- Deleted edges: not restorable individually — but re-derivable
  (Hebbian re-learning, consolidation re-creation); preview counts
  bound the blast radius before execution.
- Rule/config/upgrade-refresh: revert commits.
