# Sprint FTLOOP-DRILL-001 — the enabled tiny-subset drill (FT-RECURSIVE-002's remaining step)

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FTLOOP-DRILL-001 |
| Owner | Roger Henley (operator-authorized: "run the ftloop drill overnight", 2026-07-22) |
| Branch | `reh3376_dev01` |
| Format | Sprint plan v1.0 (12-section) |
| Effort | prep ~0.25 day + one supervised overnight window (~1–2 h) |
| Parent | FT-RECURSIVE-002 disclosed remaining step: "a single *enabled* tiny-subset drill off-peak" — every stage was live-validated individually in Epic 6, but the actuator has never run END-TO-END under its own supervision with `FT_LOOP_ENABLED=true` |

## 2. Problem Statement

The recursive-retrain actuator (gate → ledger → controller → five-stage
pipeline) ships default-off with each stage individually live-validated, but
the assembled loop — controller picking an open cycle off the ledger,
acquiring the compute lease, quiescing RSIC, walking
export→curate→train→convert→gate as supervised subprocesses, and halting at
`promote_pending` — has never executed as one autonomous run. FT-RECURSIVE-003
(RSIC integration + canary + auto-rollback) builds directly on this loop;
running it blind would be building on an untested assembly. On-box training
saturates llama-server (E6-12), hence the operator-scheduled overnight
window.

## 3. Scope & Constraints

**In scope:** one end-to-end cycle on a tiny corpus
(`FT_LOOP_EXPORT_SINCE_DAYS=1` — the config-native subset lever: curate rows
shrink, and train_ft reads iters from the manifest, so training shrinks with
the corpus); gate scoped via `FT_LOOP_GATE_TASK_FILTER=consulting.classify`;
either terminal outcome (`promote_pending` on gate PASS, `failed` on gate
FAIL) proves the machinery — the FT-CLASSIFY-002 NO-PROMOTE precedent.
**Hard rules:** NO promotion (operator-only — the drill session must NOT run
`mdemg ft-loop promote`); serving stays untouched (production llama-server
:8102 never restarted with the candidate; the gate uses side-port 18102);
`FT_LOOP_ENABLED` reverts to unset/false in teardown the same night;
evidence recorded in `drill_record.md`.
**Out of scope:** FT-RECURSIVE-003 features, gate-threshold tuning,
promotion, any retrain at production scale.

## 4. Dependencies (pre-verified 2026-07-22 ~18:40Z)

✅ 860 GB free (floor 100) · ✅ base model `.local-models/qwen3-14b-4bit-base`
present · ✅ `neural/.venv/bin/python` OK · ✅ gate side-port 18102 free ·
✅ ledger `ft_training_cycles` exists with 0 rows (clean single-flight) ·
✅ 2,127 clean llm_interactions in the last 24 h (tiny-corpus input) ·
✅ controller supervised as the conditional 16th loop (SUPERVISOR-002).

## 5. Implementation Plan

- **E0** this plan + `runbook.md` (fully self-contained for the overnight
  session) — committed before the window.
- **E1 (overnight)** execute the runbook: stage env → kickstart → verify
  controller loop → open a `triggered` cycle (CUIDv2 row, the exact
  `Gate.OpenCycle` shape) → supervise stage transitions via ledger +
  `ft-loop:<stage>` jobhealth + logs → terminal state.
- **E2 (overnight)** teardown per runbook: revert env, kickstart, verify
  lease released + RSIC quiesce lifted + production serving untouched +
  no `promote` run; record `drill_record.md`.
- **E3** docs: CHANGELOG, CLAUDE.md FT-RECURSIVE-002 note amendment
  (drill complete), post; push.

## 6. Testing Plan

The drill IS the Tier-3 test. Tier 1/2 shipped with FT-RECURSIVE-002
(controller/gate/lease unit + contract suites, all green in CI).

## 7. Commit Strategy

`docs(E0)` now; `docs(E1+E2 evidence + E3)` after the window. Surprise
defects → own fix-commits.

## 8. Verification Checklist

controller consumed the cycle from the ledger · lease acquired + released ·
RSIC quiesced during the run · all five stages reported to
`scheduled_job_events` as `ft-loop:<stage>` · terminal state reached
(promote_pending OR failed) with reason recorded · NO promotion · production
llama-server untouched (same pid/model before/after) · env reverted ·
evidence committed.

## 9. Documentation Update

CHANGELOG; CLAUDE.md (FT-RECURSIVE-002 "remaining" → drill complete);
`drill_record.md` + post.

## 10. Risks & Mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| Train hangs / crashes overnight | Med | Controller stages are ctx-cancellable; lease expiry (14 h) unwedges RSIC; the wake session supervises live and can cancel |
| Box saturation degrades production llama-server | Med | Operator-scheduled off-peak window; quiesce pauses RSIC triggers; drill session avoids other heavy work |
| Tiny corpus yields degenerate curate (0 rows for some paradigm) | Low | Any stage failure is a VALID drill outcome (machinery proof); failure reason recorded, fix disclosed |
| Cycle left open (wedged single-flight) | Low | Teardown verifies terminal status; if wedged, record a `failed` event manually + document |

## 11. Rollback

Teardown IS the rollback: env reverted, no serving mutation ever happens
below `promote` (which is forbidden this drill). A wedged cycle closes with
a manual `failed` ledger event.

## 12. Documents Accessed

`internal/ftloop/{controller,controller_stages,gate,lease}.go`;
`internal/tsdb/ft_cycle_ledger.go`; `internal/config/config.go` (FT_LOOP_*);
FT-RECURSIVE-002 sprint docs (`epic6_issues.md`, `run_record.md`); live
preflight queries (disk/ledger/corpus/ports).
