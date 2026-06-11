# Sprint Plan — BACKUP-RESTORE-VERIFY-001: Backups That Provably Restore

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | BACKUP-RESTORE-VERIFY-001 |
| Sprint line | `docs/development/backup-restore-verify-001/` |
| Date opened | 2026-06-11 |
| Branch | `reh3376_dev01` |
| Roadmap slot | Q3 Phase 2, second "next in line" member (promoted from stretch) |
| Estimated effort | 4 dev-days |
| OpenAI spend | $0 |
| Risk level | Medium-high (touches the restore path; Tier 3 includes a real restore round-trip) |

## 2. Problem Statement

A memory substrate whose backups have never been restore-tested is one
corruption away from permanent memory loss — with a silent-failure
mechanism at every step:

1. **No checksum gate before import** (`full.go` restore paths): the
   manifest carries a SHA-256 written at backup time, and nothing ever
   reads it. A corrupted/truncated `.mdemg` restores silently.
2. **The pre-restore safety snapshot is a race** (`full.go:66`):
   `Trigger()` is async; the code sleeps 2 s and hopes. On a slow disk the
   restore proceeds before its own safety net exists.
3. **No post-restore validation**: the manifest carries node/edge counts;
   the importer logs its own counts; nobody compares them.
4. **The legacy `.dump` restore shells out to bare `"docker"`** —
   exactly the launchd-minimal-PATH failure NOSILENT-001 fixed for TSDB
   (`internal/dockerbin` exists; this path ignores it).
5. **The default-ON Neo4j backup scheduler is the unmonitored one**
   (inverted NOSILENT coverage): zero `jobhealth` references in
   `internal/backup/`; the staleness evaluator rule covers only
   `job_name='tsdb-backup'`. A Neo4j backup that fails or never runs is
   invisible.

Today's incident sharpens the point: RSIC archived 5,397 observations in
one burst this morning and its rollback restored 0 — the backup chain is
currently the only durable undo, and it has never been proven to restore.

## 3. Scope & Constraints

**In scope**
- Checksum gate before `.mdemg` import (fail closed when the manifest has
  a checksum; warn-and-proceed only for legacy manifests without one).
- Replace the 2 s sleep with completion-polling of the snapshot job
  (fail closed on snapshot failure/timeout — it is the safety net).
- Post-restore count validation against the manifest (hard-fail on
  file-truncation class; warn on conflict-skip divergence, which is
  legitimate under `CONFLICT_SKIP`).
- `.dump` restore routed via `internal/dockerbin` (config override
  preserved).
- jobhealth wiring for the Neo4j backup scheduler (`SetResultHook` →
  `jobhealth.Report`, `job_name='neo4j-backup'`) + a generalized per-job
  staleness-rule factory, registered for neo4j-backup (gated on
  `BACKUP_ENABLED`) alongside the existing tsdb-backup rule.
- Tier 3: live backup→restore round-trip on a scratch space.

**Out of scope (disclosed)**
- A `mdemg db backup restore` CLI command (restore stays API-driven;
  candidate for a UX follow-up — the Tier 3 harness uses the API).
- Staleness rules for `maintenance` (covered by MAINT-LIVE-001's
  `maintenance_no_live_run`) and `export-auto` (operator-scheduled,
  cadence unknown server-side — a wrong default would born-fire).
- The RSIC micro-cycle storm + rollback restored_count=0 findings from
  today's triage (follow-up sprint candidate RSIC-STORM-001) — this sprint
  only hardens the undo chain those findings make urgent.
- TSDB backup path changes (already NOSILENT-wired and dockerbin-routed).

**Constraints**: sequential epics; no hardcoded values; live Tier 3
mandatory; destructive test ops on a scratch space only, with LIMIT-5-first
where applicable; `mdemg-dev` graph is never the restore target.

## 4. Dependencies

- `internal/backup/` (recon report `recon.md`), `internal/transfer/`
  exporter/importer, `internal/jobhealth` (NOSILENT-001),
  `internal/dockerbin`, `internal/alert/rules.go` job rules,
  `internal/jobs` queue (snapshot polling), SUPERVISOR-002's supervised
  scheduler (just shipped — both backup schedulers now start via
  `StartSupervisedBackground`).
- No schema migrations (V0024 `scheduled_job_events` already exists).

## 5. Implementation Plan (sequential epics)

**Epic 0 — Plan + recon committed.** (this doc + agent recon)

**Epic 1 — Checksum gate** (`internal/backup/full.go`)
- `restoreFromMdemg`: before import, `sha256File(mdemgPath)` and compare to
  `manifest.Checksum`. Mismatch → fail the restore job with an explicit
  integrity error (no import attempted). Manifest without checksum
  (legacy) → `slog.Warn` + proceed.
- Backup side already records the checksum — no change.
- Tier 1: corrupted-file detection, legacy-manifest passthrough, happy path.

**Epic 2 — Snapshot completion polling** (`full.go`)
- Replace `time.Sleep(2s)` with polling of the snapshot job in the jobs
  queue until `completed` (interval 500 ms), bounded by
  `BACKUP_SNAPSHOT_WAIT_TIMEOUT_SEC` (new config, default 300). Snapshot
  job `failed`/timeout → abort the restore (fail closed).
- Tier 1: completion, failure, timeout paths (fake clock/queue as needed).

**Epic 3 — Post-restore count validation** (`full.go` + transfer result)
- Compare what the importer READ from the file against
  `manifest.NodeCount/EdgeCount`: read < manifest ⇒ truncated/corrupt ⇒
  job fails (hard). Created/merged vs manifest divergence under
  `CONFLICT_SKIP` ⇒ warn + include both numbers in the job result
  (`validation` block) — legitimate when restoring into a non-empty graph.
- Tier 1: truncation failure, skip-divergence warning, clean equality.

**Epic 4 — dockerbin routing for `.dump` restore** (`full.go`)
- `cfg.FullCmd` set → honor it (operator override). Unset/default
  `"docker"` → `dockerbin.Path()`.
- Tier 1: routing decision table.

**Epic 5 — jobhealth + generalized staleness rules**
- `backup.Scheduler.SetResultHook(JobResultFunc)` mirroring
  `tsdb.TSDBBackupScheduler` (success/latency/error per scheduled run —
  full and partial both report).
- Wire in `server.go::SetTSDBClient` next to the existing tsdb hook:
  `job_name='neo4j-backup'`, same pool + dispatcher via
  `jobhealth.Report` (nil-safe; no import of alert into backup).
- `internal/alert/rules.go`: extract `jobStalenessRule(jobName, title,
  stalenessHours)` factory; `JobHealthRules` keeps `tsdb-backup` and adds
  `neo4j-backup` (window = `BACKUP_PARTIAL_INTERVAL_HOURS × 2` unless
  `BACKUP_JOB_STALENESS_HOURS` overrides; registered only when
  `BACKUP_ENABLED=true`). Distinct `Service` per rule (NOSILENT cooldown
  rule): `scheduled-job-staleness-neo4j`.
- Tier 1: factory output, gating, distinct services.

**Epic 6 — Tier 3 live round-trip** (the existential checkbox)
1. Create scratch space `backup-rt-test` with a known node/edge population
   (via observe API; ≥ 60 nodes so counts are meaningful).
2. `POST /v1/backup/trigger` partial for that space → manifest exists,
   checksum + counts recorded; jobhealth row lands in
   `scheduled_job_events`? (scheduler-only — manual triggers report via
   the same hook if wired; verify either way and document which).
3. Corruption drill: copy the `.mdemg`, flip bytes, restore → checksum
   gate rejects (observe job error).
4. Delete the scratch space (space prune / data clean — scratch only).
5. Restore the pristine backup → poll job → counts validate vs manifest →
   query Neo4j: node/edge counts for the space match.
6. Staleness rule: verify `neo4j_backup_no_recent_success` loads (rule
   count log) and is silent right after a successful scheduled/manual run
   recording; force-check SQL directly against `scheduled_job_events`.
7. Clean up the scratch space; verify `mdemg-dev` untouched (count before
   = after).

**Epic 7 — Documentation (final epic — never cut)**
- `docs/features/backup-restore.md` (new feature doc: Why / Choices /
  How it works / How to use, incl. restore API, checksum gate, validation
  semantics, jobhealth coverage).
- CHANGELOG, CLAUDE.md note, `verification.md`, `post.md`.

## 6. Testing Plan

- **Tier 1**: checksum gate (corrupt/legacy/clean), snapshot polling
  (complete/fail/timeout), count validation (truncate/skip/equal),
  dockerbin routing table, staleness-rule factory + gating. Existing
  backup_test.go suite stays green.
- **Tier 2**: round-trip against the importer with an in-repo fixture
  export (no Docker): export → corrupt → reject; export → import →
  validation block populated.
- **Tier 3**: Epic 6 against the live stack — real server, real Neo4j,
  real files on disk, real alert rows. Scratch space only.

## 7. Commit Strategy

One commit per epic; live-smoke surprises get their own fix commits;
docs last; single push at sprint end (auto-PR).

## 8. Verification Checklist

- [ ] Corrupted `.mdemg` is rejected before import (live drill, Epic 6.3)
- [ ] Restore waits for the safety snapshot; snapshot failure aborts restore
- [ ] Post-restore validation block in job result; truncation hard-fails
- [ ] `.dump` restore resolves docker via dockerbin (unit decision table)
- [ ] `scheduled_job_events` rows with `job_name='neo4j-backup'` land on
      scheduled runs; failure fires a high alert (jobhealth.Report)
- [ ] `neo4j_backup_no_recent_success` rule loads, gated on BACKUP_ENABLED,
      distinct Service label
- [ ] live smoke: full round-trip — backup scratch space, corrupt-reject,
      delete, restore, counts match manifest and Neo4j; `mdemg-dev`
      node count unchanged before/after
- [ ] `golangci-lint run ./...` clean; full `go test ./internal/...` green
- [ ] Feature doc + CHANGELOG + CLAUDE.md + verification + post

## 9. Documentation Update — Epic 7 above

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Restore drill touches live data | Low | Critical | Scratch space only; CONFLICT_SKIP import; mdemg-dev count check before/after; pre-restore snapshot ON in the drill |
| Checksum gate breaks legacy backups | Medium | Medium | Gate applies only when the manifest HAS a checksum; legacy warns |
| Snapshot polling deadlocks restore when queue is wedged | Low | Medium | Hard timeout (`BACKUP_SNAPSHOT_WAIT_TIMEOUT_SEC` 300) fails closed with explicit error |
| Count validation false-fails under CONFLICT_SKIP | Medium | Low | Skip-divergence is warn-only; only read-truncation hard-fails |
| RSIC churn shifts mdemg-dev counts during the drill window | Medium | Low | Validation compares manifest↔scratch-space only; mdemg-dev check is scoped to the same query before/after |
| Staleness rule born-fires on fresh installs (no rows yet) | Medium | Medium | Same gating pattern as tsdb rule (enabled-flag + interval-derived window); verified live in Epic 6.6 |

## 11. Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q3.md` (sprint entry, line 51)
- Recon report (agent, 2026-06-11): `internal/backup/{types,service,full,partial,scheduler,retention}.go`,
  `internal/transfer/{exporter,importer}.go`, `internal/tsdb/backup.go`,
  `internal/jobhealth/jobhealth.go`, `internal/dockerbin/dockerbin.go`,
  `internal/alert/rules.go`, `internal/api/{server,handlers_backup}.go`,
  `internal/cli/{db_backup,tsdb}.go`
- NOSILENT-001 + MAINT-LIVE-001 + SUPERVISOR-002 sprint docs (patterns reused)

## 12. Rollback Procedures

- All changes are additive/fail-closed on the restore path; revert commits
  to restore legacy behavior.
- Epic 6 drill: scratch space deleted at the end; pristine backup file +
  pre-restore snapshot retained until verification passes, then subject to
  normal retention.
- New alert rule disabled by `BACKUP_ENABLED=false` or rule `Enabled=false`.
