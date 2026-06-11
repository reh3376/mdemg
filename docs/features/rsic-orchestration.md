# RSIC Orchestration — Admission, Archival Attribution, Rollback

## Why

RSIC's self-improvement cycles are powerful and partially destructive
(archival, pruning, confidence adjustment). Three guarantees make them
safe to leave always-on, and all three were broken until RSIC-STORM-001:

1. **Admission was raceable.** `EvaluateTrigger` checked the active-cycle
   and cooldown records, but they were written only after a cycle
   *completed* — so during a cycle's multi-second run, every concurrent
   trigger passed every gate. Live: ~20–30k micro cycles/day (4 spawning
   within 50 ms of each tool-use burst), llama-server permanently
   saturated, recurring LLM timeout cascades, alert spam, destructive
   actions dispatched at storm frequency.
2. **Archival was unattributable.** `tombstone_stale` set only
   `is_archived` — no timestamp, no reason, invisible to forensics. Two
   property-name conventions (`archive_reason` / `archived_reason`)
   split the writers, so reason queries silently missed half of them.
3. **Rollback restored nothing.** The rollback snapshot's candidate
   predicate had drifted from the executor's (RSIC-VALIDATE-001 updated
   one, not the other) — the snapshot captured a different node set, so
   rollback "restored" untouched nodes (`restored_count=0` live).

## How it works now

**Reserve-on-allow admission**: `EvaluateTrigger` writes the active-cycle
and cooldown (and dedupe) records under the same lock that performs the
checks. `RecordTrigger` updates the reservation with the real cycle ID;
`CompleteCycle` frees the slot; a failed cycle still cools down. Verified
live: 1 cycle admitted / 132 rejected in the first four minutes
(`mdemg_rsic_trigger_rejected_total{reason="cooldown"|"overlap"}`).
`RSIC_TRIGGER_COOLDOWN_SEC` (default 300) is now an enforced contract.

**Attributable archival**: every `tombstone_stale` write stamps
`archived_at`, `archive_reason='rsic_tombstone_stale'`, and
`archived_cycle_id`. The canonical reason property is **`archive_reason`**
(historical `archived_reason` rows exist — forensic queries should
`coalesce(n.archive_reason, n.archived_reason)`).

**One predicate, two consumers**: the tombstone candidate Cypher lives in
a single shared constant used by both the executor and the rollback
snapshot — the drift class is structurally impossible. Rollback restores
`is_archived=false` and clears the attribution fields; drilled live
(snapshot 3 → archive 3 → rollback 3).

**Context Cooler cap**: `ProcessGraduations`' tombstone step archives at
most `COOLER_TOMBSTONE_MAX_PER_RUN` (default 500; 0 = unlimited) per run
and logs a loud warning when the cap binds. (The 2026-06-11 incident was
one uncapped sweep of 5,397 never-graduated observations — the backlog of
the pre-DH-004 graduation bug — triggered by the session-start hook's
graduate call.)

## Configuration

| Env | Default | Meaning |
|---|---|---|
| `RSIC_TRIGGER_COOLDOWN_SEC` | 300 | min seconds between admitted triggers per source+space (now enforced) |
| `COOLER_TOMBSTONE_MAX_PER_RUN` | 500 | max cooler tombstones per graduation run (0 = unlimited) |

## How to observe

- Admissions vs rejections: `/v1/metrics/snapshot` →
  `mdemg_rsic_trigger_rejected_total{source,reason}`.
- Archival attribution: `MATCH (n) WHERE n.is_archived RETURN
  coalesce(n.archive_reason, n.archived_reason), n.archived_cycle_id`.
- Cap pressure: `context cooler: tombstone cap reached` warnings.
- Recovery audit trail: nodes recovered from the 2026-06-11 incident
  carry `recovered_reason='cooler_backlog_recovery'`.

Sprint: `docs/development/rsic-storm-001/` (plan with corrected incident
attribution, live verification).
