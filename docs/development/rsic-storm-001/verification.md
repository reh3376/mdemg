# RSIC-STORM-001 — Tier 3 Live Verification

Date: 2026-06-11 · Branch: `reh3376_dev01` · Live stack: native `bin/mdemg`
(LaunchAgent) + Docker Neo4j/TimescaleDB · Space: `mdemg-dev` + drill spaces

## 1. The storm is dead (Epic A)

Restart with reserve-on-allow at 17:46:24Z. Four-minute measurement window:

| Metric | Pre-fix | Post-fix |
|---|---|---|
| Micro cycles started | ~47 per 4 min (685–1,927/hr live) | **1** |
| `rsic_trigger_rejected_total{reason="cooldown"}` | never incremented (gate raceable) | **97** |
| `rsic_trigger_rejected_total{reason="overlap"}` | never incremented | **35** |

132 storm triggers rejected, 1 admitted — the 300 s cooldown is real for
the first time. Unit pin: 50 concurrent triggers admit exactly one.

**LLM health**: zero errors across all tasks since the restart (9 calls) —
including `retrieval.intent_translate`, which had been failing with
timeouts/cancellations all day under storm-saturated llama-server.

## 2. Rollback drill (Epic C) — live Neo4j

`tests/integration/rsic_tombstone_rollback_test.go` (integration tag, run
against the live container): seeded a correction + 3 linked observations +
1 unlinked observation in a drill space, then snapshot → execute →
rollback:

- Snapshot captured exactly the **3 linked** candidates
  (`AffectedCount=3`, reversible) — the shared-predicate fix; pre-fix the
  drifted snapshot captured a different set.
- Executor archived the same 3 with full attribution
  (`archive_reason='rsic_tombstone_stale'`, `archived_at`,
  `archived_cycle_id='drill-cycle'`); the unlinked node untouched.
- `Rollback` → **`RestoredCount=3`** (pre-fix live value: 0); all archive
  metadata cleared. PASS (0.78 s).

## 3. Recovery (Epic D) — operator option (b) executed

- Pre-op partial backup triggered (`bk-20260611-174626`); the
  restore-verified `bk-20260611-162337` (1 h old) also held the
  pre-recovery state.
- LIMIT 5 first: 5 nodes un-archived + graduated, verified
  (`still_archived=0, still_volatile=0`), then full run.
- **952 recovered** (`recovered_reason='cooler_backlog_recovery'`,
  `volatile=false` so the cooler cannot re-sweep them): progress 681,
  context 140, decisions 34, context_signal 31, technical_note 31,
  corrections 10, learnings 8, others 17.
- Error-debris stays archived (operator decision; one-query reversal via
  the `recovered_reason` stamp if ever wanted).

## 4. Cooler cap (Epic B) — live re-fire

`POST /v1/conversation/graduate` on mdemg-dev:

```
graduated=29 tombstoned=500 remaining_volatile=58
WARN context cooler: tombstone cap reached — backlog remains and will
     drain on subsequent runs  tombstoned=500 cap=500
```

- Cap bound at exactly `COOLER_TOMBSTONE_MAX_PER_RUN=500` with the loud
  warning (the incident run swept 5,397 uncapped).
- Composition check: all 500 were `obs_type='error'` debris (this
  session's drills generated hundreds of Bash-error observations) — the
  recovered/graduated nodes were untouched.
- `graduated=29` — the DH-004 graduation path is working on live data.

## 5. Attribution (Epic B)

New tombstones carry `archived_at` + `archive_reason` +
`archived_cycle_id` (drill §2 evidence). Canonical property:
`archive_reason`; `concepts.go` writer migrated; historical
`archived_reason` rows remain (readers coalesce — this exact naming split
caused the incident's hours-long mis-attribution).

## Documents Accessed

- `docs/development/rsic-storm-001/sprint_plan_rsic_storm_001.md`
- `internal/ape/{orchestration_policy,task_dispatch,action_snapshot}.go`
- `internal/conversation/cooler.go`, `internal/cli/concepts.go`
- Live: `~/.mdemg/logs/server.log`, `/v1/metrics/snapshot`, Neo4j
  `mdemg-dev` + `rsic-rollback-drill`, TSDB `llm_interactions`
