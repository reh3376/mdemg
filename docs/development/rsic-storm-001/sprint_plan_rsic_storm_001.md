# Sprint Plan — RSIC-STORM-001: Stop the Cycle Storm, Make Archival Honest

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | RSIC-STORM-001 |
| Sprint line | `docs/development/rsic-storm-001/` |
| Date opened | 2026-06-11 |
| Branch | `reh3376_dev01` |
| Roadmap slot | Off-roadmap, earned by live evidence (3× confirmed during SUPERVISOR-002 + BACKUP-RESTORE-VERIFY-001 drills) |
| Estimated effort | 2–3 dev-days |
| OpenAI spend | $0 |
| Risk level | Medium (touches RSIC admission + archival paths; recovery op on live data — backups now restore-tested, the prerequisite this sprint waited for) |

## 2. Problem Statement (with corrected attribution)

**A. The micro-cycle storm (code-confirmed).** RSIC runs ~20–30k micro
cycles/day (one per ~3–5 s; 4 spawn within 50 ms of each tool-use burst).
Root cause is a check/act race: every `/v1/conversation/observe` fires a
trigger goroutine; `EvaluateTrigger` checks `activeCycles` + `lastTrigger`,
but both are written only by `RecordTrigger` — which the callers invoke
**after `RunCycle` completes**. For a cycle's entire multi-second duration,
every concurrent trigger passes every gate; the 300 s cooldown effectively
does not exist. Effects: llama-server saturation (the recurring
`jiminy.synthesize`/`evaluate_llm`/`intent_translate` timeout cascades),
`trigger_training_pipeline` alert spam every session, constant Neo4j load,
and destructive actions dispatched at storm frequency.

**B. Archival is unattributable and inconsistent (triage correction).**
This morning's 5,397-node CRITICAL drop was the **Context Cooler**, not
RSIC: `session-start.sh` fires `POST /v1/conversation/graduate`, whose
`ProcessGraduations` tombstoned the entire backlog of never-graduated
volatile observations (victims of the pre-DH-004 graduation bug) in one
uncapped sweep. The initial mis-attribution to RSIC happened because (a)
RSIC's `tombstone_stale` sets ONLY `is_archived` — no `archived_at`, no
reason, invisible to time-based forensics (54 nodes carry that bare
signature); (b) two property-name conventions coexist (`archive_reason`
at 3 sites vs `archived_reason` in `concepts.go`), so reason queries
silently miss half the writers.

**C. tombstone_stale rollback restores nothing (predicate drift).**
RSIC-VALIDATE-001 added the correction-linkage condition to the
**executor's** Cypher but not the **snapshot's**
(`action_snapshot.go:218` still captures the old unlinked predicate) —
the snapshot records a different node set than the executor archives, so
rollback "restores" untouched nodes: `restored_count=0` while real
archival goes un-undone. (`graduate_volatile` rollbacks work — 12/12/2
restored live — proving the snapshot infra itself is sound.)

**D. The 5,397 cooler-tombstoned observations** are recoverable
(`is_archived`, `archive_reason='context_cooler_tombstone'`): 4,451
error-debris + ~950 of real value (decisions, corrections, learnings,
progress). They were tombstoned for being volatile — but they were
volatile only because graduation was broken until DH-004. Plain
un-archiving would re-expose them to the same tombstone; recovery must
graduate them.

## 3. Scope & Constraints

**In scope**
- Atomic trigger admission (reserve-on-allow) in `OrchestrationPolicy`.
- Archival metadata: `tombstone_stale` stamps `archived_at` +
  `archive_reason` + cycle id; canonical property name `archive_reason`
  (readers coalesce both; `concepts.go` writers migrate, no data
  migration).
- Cooler tombstone per-run cap (`COOLER_TOMBSTONE_MAX_PER_RUN`) + loud
  volume logging on the graduate path.
- Fix the tombstone_stale snapshot predicate to match the executor
  (single Cypher source so they cannot drift again).
- Recovery: un-archive + graduate the non-error cooler-tombstoned
  observations (operator-recommended option (b), approved with the
  sprint); error-debris stays archived.
- Tier 3: storm-rate before/after, rollback drill, recovery verification.

**Out of scope (disclosed)**
- `retrieval.intent_translate` "context canceled" alerts — the recall
  hook legitimately needs its response within its timeout; a server-side
  ctx-detach (FEEDBACK-CTX pattern) only helps cache warming. Follow-up
  candidate RETRIEVE-CTX-001 with its own tradeoff analysis.
- RSIC LLM-reflector JSON-truncation failures (separate quality issue;
  load drop from Epic A likely improves it — measure, don't fix here).
- Re-architecting cooler TTL/graduation policy (DH-004 fixed graduation;
  this sprint only caps and attributes the tombstone sweep).
- Session-start hook behavior (firing graduate is fine once capped).

**Constraints**: sequential epics; no hardcoded values; LIMIT-5-first for
the recovery op; live Tier 3 mandatory; destructive-op state restoration;
zero test failures.

## 4. Dependencies

- `internal/ape/orchestration_policy.go` (admission), `task_dispatch.go`
  (tombstone executor), `action_snapshot.go` (snapshot predicates),
  `cycle.go` (rollback call site).
- `internal/conversation/cooler.go` (tombstone Cypher + cap),
  `internal/api/handlers_conversation.go` + `server.go` (graduate paths).
- BACKUP-RESTORE-VERIFY-001 (shipped): restore-tested backups are the
  safety net for the recovery op; a fresh pre-recovery backup is taken in
  Epic D.
- No schema migrations.

## 5. Implementation Plan (sequential epics)

**Epic 0 — Plan + corrected triage record** (this doc). Gate: committed.

**Epic A — Atomic trigger admission (the storm killer)**
- `EvaluateTrigger`: on Allowed, immediately (same mutex hold) write the
  active-cycle record and the cooldown record (`pending` cycle id;
  `RecordTrigger` later fills the real id). A failed/aborted cycle calls
  `CompleteCycle` (already in both call sites' error paths) which clears
  the active slot; the cooldown record persists either way (a failed
  cycle must still cool down).
- Idempotent with the existing `RecordTrigger` (now an update, not the
  first write).
- Tier 1: N concurrent `EvaluateTrigger` admit exactly 1; second trigger
  within cooldown rejected even while the first cycle is mid-flight;
  post-`CompleteCycle` + cooldown-expiry re-admission; macro/micro tier
  independence preserved.
- Gate: `go test ./internal/ape/` green.

**Epic B — Honest, attributable archival**
- `executeTombstoneStale` Cypher: `SET stale.is_archived = true,
  stale.archived_at = datetime(), stale.archive_reason =
  'rsic_tombstone_stale', stale.archived_cycle_id = $cycleID` (cycle id
  threaded from the dispatcher).
- Canonical name `archive_reason`: `concepts.go` writers switch; all
  in-repo readers (triage-style queries in CLI/handlers, if any) coalesce
  `archive_reason`/`archived_reason`. No retroactive data migration
  (historical `archived_reason` rows remain; documented).
- Cooler: `ProcessGraduations` tombstone step takes
  `COOLER_TOMBSTONE_MAX_PER_RUN` (default 500, 0 = unlimited) and logs
  count + remaining backlog at Warn when the cap binds.
- Tier 1: Cypher fragments pinned (reason/at/cycle-id present), cap
  plumbing, coalesce readers.

**Epic C — Rollback predicate unification**
- Extract the tombstone candidate predicate into ONE shared Cypher
  fragment (Go const) used by both `executeTombstoneStale` and
  `capturePreState("tombstone_stale")` — the drift class becomes
  impossible, not just fixed.
- Tier 1: pin test asserts both call sites reference the shared const;
  snapshot returns the same candidate set the executor would archive.
- Tier 3 (in Epic E): forced tombstone + failed validation on a scratch
  space → rollback restores count > 0 and the nodes are live again.

**Epic D — Recovery of the cooler-tombstone victims (operator option (b))**
- Pre-op: fresh partial backup (restore-tested path from #434).
- Un-archive + graduate (set `volatile=false`, clear archive fields,
  stamp `recovered_reason='cooler_backlog_recovery'`) for
  `archive_reason='context_cooler_tombstone'` AND `obs_type <> 'error'`
  (~950 nodes). LIMIT 5 first → verify → full run → re-count.
- Error-debris (4,451) stays archived (it is what the bloat alert wanted
  pruned). Disclosed as the operator decision taken.

**Epic E — Tier 3 live verification**
- Storm rate: cycles/hour before (685+/hr today) vs after restart with
  Epic A — expect ≤ 12/hr/space for micro_auto (300 s cooldown), and the
  `RSICTriggerRejected{reason="cooldown"|"overlap"}` counters actually
  incrementing.
- Observe llama-server pressure: no new `jiminy.evaluate_llm` /
  `synthesize` timeout bursts in the post-restart window.
- Rollback drill (Epic C) on a scratch space.
- Recovery verification (Epic D): counts, sample content retrievable via
  `/v1/memory/retrieve`.
- Graduate-endpoint cap: re-fire the session-start curl — tombstones ≤
  cap, loud log.

**Epic F — Documentation (final epic — never cut)**
- `docs/features/rsic-orchestration.md` (new: admission semantics,
  cooldowns, archival attribution, rollback guarantees).
- CHANGELOG, CLAUDE.md note, `verification.md`, `post.md`.

## 6. Testing Plan

- **Tier 1**: admission race (concurrent goroutines), cooldown-from-
  admission semantics, shared-predicate pin, Cypher metadata fragments,
  cooler cap, coalesce readers. Existing ape/conversation/api suites stay
  green.
- **Tier 2**: integration suites (`tests/integration`) unchanged-green;
  snapshot/rollback against live Neo4j where the env provides it.
- **Tier 3**: Epic E against the live stack — real cycle rates from the
  real log, real rollback restoring real nodes, real recovery.

## 7. Commit Strategy

One commit per epic; live-smoke surprises get their own fix commits; docs
last; single push (auto-PR).

## 8. Verification Checklist

- [ ] Concurrent triggers admit exactly one (unit + live rate check)
- [ ] Live micro_auto rate drops from ~700/hr to ≤ 12/hr/space
- [ ] `RSICTriggerRejected` cooldown/overlap counters increment live
- [ ] tombstone_stale rows carry archived_at + archive_reason + cycle id
- [ ] Snapshot and executor share one predicate (pin test)
- [ ] Live rollback drill restores count > 0
- [ ] Cooler tombstone cap binds with loud log on the graduate path
- [ ] ~950 non-error victims recovered + graduated (LIMIT-5-first), error
      debris untouched, pre-op backup taken
- [ ] No new LLM timeout bursts post-fix window
- [ ] `golangci-lint` clean; full `go test ./internal/...` green
- [ ] Feature doc + CHANGELOG + CLAUDE.md + verification + post

## 9. Documentation Update — Epic F above

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Reserve-on-allow starves legitimate cycles when a cycle hangs | Low | Medium | Existing 30-min stale-active cleanup already in EvaluateTrigger; CompleteCycle on every error path |
| Tighter admission hides RSIC regressions previously surfaced by sheer volume | Low | Low | Watchdog + macro cron remain independent trigger sources |
| Recovery re-floods retrieval with stale observations | Medium | Low | Only non-error types (~950 of 79k nodes); graduated so cooler won't re-sweep; pre-op backup + LIMIT-5 first |
| Cooler cap leaves backlog un-tombstoned forever | Low | Low | Cap logs remaining backlog; successive runs drain it at cap rate |
| Property-name unification breaks an unknown reader | Low | Medium | Writers move to canonical name; all known readers coalesce both; historical rows untouched |

## 11. Documents Accessed

- Live forensics (2026-06-11): `~/.mdemg/logs/server.log` (1,927
  cycles/hr; graduate POST at 14:22:23Z; graduate_volatile rollbacks
  12/12/2; tombstone rollbacks 0), Neo4j `mdemg-dev`
  (`archive_reason='context_cooler_tombstone'`: 5,397; bare-is_archived
  RSIC signature: 54), `.claude/hooks/session-start.sh:517`,
  `pre-compact.sh:214`
- `internal/ape/{orchestration_policy,task_dispatch,action_snapshot,cycle}.go`
- `internal/api/{handlers_conversation,server}.go`,
  `internal/conversation/cooler.go`, `internal/cli/concepts.go`
- `internal/config/config.go` (`RSIC_TRIGGER_COOLDOWN_SEC` 300)
- RSIC-VALIDATE-001 + DH-004 + EVENTGRAPH-003 sprint notes (prior wiring)

## 12. Rollback Procedures

- Epics A–C: code-only, revert commits.
- Epic D recovery: pre-op partial backup (restore-tested); recovered
  nodes carry `recovered_reason='cooler_backlog_recovery'` so the exact
  set can be re-archived with one query if the operator reverses the
  decision.
