# RSIC-LLM-ALERT-GUARD-001 — Sprint Post

**Date:** 2026-06-24 · branch `reh3376_dev01` · a contained correctness fix from
a root-cause investigation (the residual flagged during JIMINY-RELEVANCE-001).

## Problem
A CRITICAL `jiminy / Jiminy Service Unavailable` alert kept re-firing (~3–6/day;
1,428 total in the log) **while `/healthz` reported `jiminy: ok`** — a false
positive. Sprint JIMINY-SIGNAL-001 had already "fixed" this, but it was still
firing.

## Root cause (evidence-backed)
**Genuine re-fire, not stale surfacing** — distinct fresh timestamps, each
matched 1:1 by a fresh `alert: dispatching service=jiminy severity=critical` log
line; the alerts clear correctly but a new one is minted on the next RSIC cycle.

There are **two independent producers** of `alert_jiminy_critical`:
1. **Rule-based reflector** (`self_reflect.go`) — guarded by JIMINY-SIGNAL-001
   (`SynergyAssessed && !JiminyHealthy`). **Not firing** (its diagnostic warn has
   zero occurrences in the log). The guard works.
2. **LLM reflector** (`llm_reflector.go`) — `alert_jiminy_critical` was in its
   `AllowedLLMActions` whitelist with **no `JiminyHealthy` cross-check**. The LLM
   is handed the whole assessment JSON (which also carried unrelated noise:
   follow-rate-drop, LLM-error-spike, embedding-regression) and **hallucinated**
   a Jiminy CRITICAL even though `jiminy_healthy=true`. The `deduplicateInsights`
   merge kept it (the rule-based path correctly raised nothing to dedup against),
   and the planner explicitly allows critical actions even at low confidence.

JIMINY-SIGNAL-001 only guarded the rule-based path; the LLM whitelist is a fully
independent second entry point it never accounted for. `/healthz` uses a real
liveness signal; the LLM reflector used **no liveness signal at all**.

## Fix (the durable, class-closing one)
Two layers:
1. **Whitelist trim** (`llm_reflector.go`): removed the three deterministic
   threshold-gated alerts (`alert_jiminy_critical`, `alert_memory_bloat`,
   `alert_synergy_overlap`) from `AllowedLLMActions`. They are produced correctly
   by the rule-based reflector from real metrics, so this loses **zero coverage**
   — the LLM is no longer even *offered* them in its system-prompt enum, so
   `parseResponse` rejects them. The LLM reflector's job is to surface *novel*
   patterns, not duplicate the deterministic alert set.
2. **Structural merge guard** (`self_reflect.go`): `deduplicateInsights` now
   drops any LLM insight recommending a `deterministicAlertActions` member —
   defense-in-depth that holds even if the whitelist is changed later. Rule-based
   copies are preserved (only LLM-originated ones are dropped).

**The general lesson** (recorded in CLAUDE.md): any deterministic CRITICAL the
LLM reflector can emit must be cross-checked against the same ground-truth signal
the rule-based path uses — the LLM whitelist is an unguarded second producer.

## Testing
- **Tier 1:** 3 new unit tests — whitelist excludes the 3 alerts; the exact
  production hallucination (rule-based raised nothing, LLM recommends
  `alert_jiminy_critical`) is dropped while a novel LLM action still merges;
  rule-based deterministic alerts are preserved. Updated 2 existing tests for the
  intentional 20→17 count change. Full `ape` suite green; lint 0; CI ULTS
  `--verify-hashes` passes (the hashed system-prompt segment is the static
  literal, unchanged by the whitelist edit).
- **Tier 3 (live):** rebuilt + restarted at 11:24:50 (`healthz` ok, `jiminy:
  ok`). An RSIC reflect cycle ran on the fixed binary at **11:29:11**
  (`insight_count=14`) — the LLM reflector is active — and produced **no**
  `alert_jiminy_critical`; **zero** new jiminy-critical dispatches since the
  restart (the last in the entire log is `10:31:39`, *before* the restart). The
  intermittent ~3–6/day false CRITICAL no longer mints — the LLM path can't
  produce it (removed from the whitelist; the merge guard backs that up).

## Documents Accessed
The background root-cause investigation; `internal/ape/{llm_reflector,self_reflect,
task_dispatch,improvement_plan}.go`; `internal/ape/llm_reflector_test.go`;
`docs/tests/ults/specs/ape_reflect.ults.json` + the ULTS runner; live
`~/.mdemg/alerts/current.json` + `~/.mdemg/logs/server.log`.
