# Sprint Plan RSIC-VALIDATE-001 — RSIC Validates For Real (Fail-Closed Self-Improvement)

## 1. Header & Metadata
- **Sprint ID:** RSIC-VALIDATE-001 — Q3 Phase 2, ranked #6 overall
- **Line:** `docs/development/rsic-validate-001/` · **Date:** 2026-06-11 · **Branch:** `reh3376_dev01`
- **Target:** v0.10.x · **Effort:** ~4d budgeted · **Spend:** $0 · **Risk:** Medium (changes RSIC's mutation + validation behavior)

## 2. Problem Statement
RSIC mutates the graph open-loop. Five defects, all confirmed in code tonight:
1. **Vacuous validation** (`calibration.go`): task criteria reference ~15
   metric keys the cycle baseline never populates (only `volatile_count` +
   `correction_rate` intersect its 10 keys) → `missing_data` → `continue`
   → `CriteriaMet` stays true. ~16/17 actions "succeed" with zero evidence;
   criteria-driven rollback is unreachable.
2. **`tombstone_stale` erodes unrelated memory** (`task_dispatch.go:414`):
   if ANY correction exists in 7 days, archive 50 ARBITRARY older
   conversation observations — no relationship between correction and
   target. Every dispatch with a recent correction (we record them
   constantly) silently archives memory.
3. **`refresh_stale_edges` only inflates weights**: the SET clause bumps
   `last_activated` to now, then the weight CASE reads the NEW value →
   staleness=0 → the decay term vanishes → pure `+0.1·log(count+1)` boost
   on every run.
4. **Synthetic outcome injection** (`executeAdjustGuidanceConfidence`):
   feeds synthetic "followed"/"ignored" into `UpdateNodeConfidence` — the
   same counters real guidance feedback increments → circular: measured
   effectiveness drives synthetic outcomes which drive measured
   effectiveness.
5. **No fail-closed rule**: for graph-mutating actions, missing evidence
   must mean NOT validated.

## 3. Scope & Constraints
**In:** populate criteria metric keys from existing report/collector
sources where available; fail-closed `missing_data` for mutating actions
(criteria-complete actions unaffected); `tombstone_stale` scoped to nodes
RELATED to the correction (same session OR co-activated neighborhood, with
the correction linkage required); `refresh_stale_edges` computes staleness
BEFORE bumping `last_activated` (true decay restored); counter-free
calibration path (`AdjustConfidenceDirect` — no followed/ignored counter
increments). Tier 1 per fix + live Tier 3 cycle.
**Out:** new health dimensions; RSIC scheduler/orchestration changes;
guidance-outcome semantics (JIMINY-BUDGET / OUTCOME-ATTRIB scope);
`alert_embedding_regression`'s empty-call_sites collector (separate
follow-up if it survives this sprint's validation honesty).

## 4. Dependencies
DH-005 health formula (report fields), live collectors, jiminy
GetConstraintEffectiveness/UpdateNodeConfidence, /v1/metrics avg_edge_weight.

## 5. Implementation Plan
- **Epic 0** — investigation (done; all five confirmed with line-level evidence).
- **Epic 1** — metrics: extend the cycle baseline + after-capture with the
  criteria keys resolvable today (edge stats, jiminy/guidance stats, J17
  protocol stats via existing collectors); document per-key source; keys
  with NO source stay absent — handled honestly by Epic 2.
- **Epic 2** — fail-closed: mutating-action registry (dispatch case list
  annotated); for mutating tasks any `missing_data` ⇒ criterion NOT met ⇒
  `CriteriaMet=false` (detail string distinguishes `missing_data_failclosed`);
  non-mutating tasks keep advisory behavior.
- **Epic 3** — executors: tombstone_stale requires correction-linkage
  (same `session_id` as the correction OR within its CO_ACTIVATED_WITH
  1-hop neighborhood) + per-run cap retained; refresh_stale_edges computes
  `staleDays` via WITH before SET (restores real decay); both
  EXPLAIN-validated.
- **Epic 4** — calibration: `AdjustConfidenceDirect(nodeID, delta)` on the
  guidance calibrator (no counter increments); RSIC-SK1 uses it; the
  followed/ignored counters remain exclusively real-feedback-driven.
- **Epic 5** — Tier 3 live: trigger a real RSIC cycle; observe fail-closed
  CriteriaMet=false on actions lacking evidence (vs tonight's vacuous
  passes), scoped tombstone candidates (0 unrelated archives), a refresh
  run whose weights can DECREASE, and constraint counters unchanged by
  RSIC adjustments. Epic 6 — docs + close.

## 6. Testing Plan
Tier 1: criterion evaluation (fail-closed paths), executor Cypher unit
coverage where harnessed, calibrator direct-path test. Tier 2:
EXPLAIN-validation; ape suite green; lint. Tier 3: the live cycle above
(real binary, real graph, real TSDB rows). Live smoke item: *run a real
RSIC cycle, observe honest CriteriaMet outcomes + scoped tombstoning +
decay-capable refresh in Neo4j/TSDB.*

## 7. Commit Strategy
One commit per epic; surprises standalone; push → auto-PR → summary.

## 8. Verification Checklist
- [ ] Criteria keys populated from real sources (documented map)
- [ ] Mutating action with missing evidence ⇒ CriteriaMet=false (live)
- [ ] tombstone_stale archives ONLY correction-linked nodes (live preview)
- [ ] refresh_stale_edges can decrease weights (math verified live)
- [ ] RSIC confidence adjustments leave followed/ignored counters unchanged
- [ ] ape suite green; lint clean; docs updated

## 9. Documentation Update — final epic.

## 10. Risks & Mitigations
| Risk | L | I | Mitigation |
|---|---|---|---|
| Fail-closed flips RSIC to mostly-failing cycles | High | Medium | That is the HONEST state; rollback paths get exercised; Epic 1 maximizes populated keys first so legitimate passes remain reachable |
| Scoped tombstone makes the action a no-op | Medium | Low | Acceptable — a no-op beats eroding unrelated memory; HIDDEN-CHURN owns deeper hygiene |
| Calibrator change starves confidence adjustment | Low | Low | Direct path preserves the adjustment; only the counter pollution stops |

## 11. Documents Accessed
`internal/ape/{calibration,cycle,task_spec,task_dispatch}.go` (line-level
evidence); jiminy guidance calibrator; roadmap; DH-005 feature doc.

## 12. Rollback
Revert commits — RSIC returns to vacuous-pass behavior (prior state);
no data migration (archived nodes from past tombstone runs are
`is_archived=true` flags, reversible by Cypher if ever needed).
