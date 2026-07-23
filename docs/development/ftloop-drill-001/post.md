# FTLOOP-DRILL-001 — Sprint Post

**Dates:** 2026-07-22 → 23 | **Branch:** `reh3376_dev01`
**Operator authorization:** "run the ftloop drill overnight"; started early
at the operator's prompt.

## Verdict

**The recursive-retrain machinery is proven end-to-end.** The controller ran
a full cycle autonomously — ledger consume (1 poll), compute lease, RSIC
quiesce, five supervised subprocess stages, correct failure alerting, honest
terminal state — and the drill did exactly what drills are for: it caught
two environment defects no unit or per-stage test could see, both fixed and
verified the same night against the real artifacts.

Full evidence: `drill_record.md` (timeline, defects, verification,
teardown). Summary:

- export 794ms · curate 960ms (814 rows) · train 612 iters ~4.5h
  (SHA-pinned base) · fuse 28GB · **convert_failed** →
  fix `7c6ee1cb` (`resolveTool` launchd-PATH chain, also covering the gate's
  binaries) + fix `9e3d53f8` (`gguf` pinned in neural [training]) →
  manual re-run of the repaired stages: 29.5GB f16 → **11GB Q5_K_M
  candidate** → **gate 0.8652 ≥ 0.80 PASS** (100 rows, 0 truncated).
- No promotion (hard rule; operator-only). Production llama-server pid
  unchanged throughout. Teardown verified; env reverted same night.

## What this unblocks

FT-RECURSIVE-003 (RSIC integration + canary + auto-rollback) now builds on
a drilled loop, with two pre-work items recorded: a lease-aware
`training_readiness_stale` rule (it false-alarmed during the legitimate
quiesce) and a `converting` ledger stage (fuse currently hides inside
`train`'s window).

## Verification checklist

- [x] Controller autonomy: consume/lease/quiesce/stages/alerts/terminal
- [x] Both drill-caught defects fixed with pins; build+lint green
- [x] Repaired convert+quantize+gate verified on real artifacts
- [x] Gate PASS 0.8652, 0 truncated
- [x] NO `ft-loop promote`
- [x] Teardown: env reverted, lease released, prod llama pid 1265 unchanged
- [x] Docs: drill_record, CHANGELOG, CLAUDE.md note, this post
- [x] Env-var drift checker clean

## Documents Accessed

`internal/ftloop/{controller,controller_stages,gate}.go`;
`internal/tsdb/ft_cycle_ledger.go`; the runbook; live ledger /
`scheduled_job_events` / server.log / workdir artifacts; gate-report.json.
