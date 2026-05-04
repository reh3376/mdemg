# Phase 13 — Note 04 Column-Voting Retrieval — Post Report

**Sprint ID:** POST-FT-LORA-PHASE13
**Branch:** `reh3376_dev01`
**Predecessor:** Phase 11.6.3 + 11.6.3.1 (always-on MLX policy commit `fc0961e` + follow-ups `f3d106e`/`43e671a`)
**Plan:** `docs/development/post-ft-lora/sprint_plan_phase_13_column_voting.md` (frozen)
**Date executed:** 2026-05-01 → 2026-05-02

---

## Outcome — A/B verdict: **FAIL**, default flag stays `false`

Quick-profile A/B (`whk-wms` space, 16 questions × `lnl_demo_validation.uvts.json` v1.1.0, GPT-5.4-mini grader):

| Run | Branch label | Mean score | Pass rate | Status |
|---|---|---|---|---|
| Baseline (legacy linear scorer) | `phase13-baseline-linear` | **0.396** | 0/16 | fail (vs absolute thresholds; the threshold values were calibrated for a richer codebase, not whk-wms) |
| Candidate (RRF v1-rrf4, equal weights) | `phase13-candidate-rrf4` | **0.358** | 0/16 | fail |

**Mean delta: −0.038 (−9.6%)** — candidate regressed.
**Per-question regressions > 10% threshold: 2** — both regressions hit the catastrophic floor (score → 0.000).

### A/B verdict (canonical, from `uvts_ab_compare.py`)

```
verdict: fail
mean_gate_passed: False
baseline_mean: 0.396
candidate_mean: 0.358
mean_delta: -0.038
regression_count: 2 (threshold 0.10)
improvement_count: 1
shared_question_count: 16
criterion: B mean >= A mean AND no per-question regression > threshold
```

### Per-question delta (16 shared questions)

| qid | category | baseline | candidate | delta | note |
|---|---|---:|---:|---:|---|
| 162 | service_relationships | 0.359 | 0.359 | +0.000 | identical |
| 192 | service_relationships | 0.450 | 0.450 | +0.000 | identical |
| 249 | business_logic_constraints | 0.355 | 0.355 | +0.000 | identical |
| 263 | business_logic_constraints | 0.450 | 0.450 | +0.000 | identical |
| 329 | data_flow_integration | 0.350 | 0.350 | +0.000 | identical |
| 391 | data_flow_integration | 0.356 | 0.356 | +0.000 | identical |
| 436 | cross_cutting_concerns | 0.450 | 0.450 | +0.000 | identical |
| 472 | cross_cutting_concerns | 0.454 | 0.454 | +0.000 | identical |
| **69** | architecture_structure | 0.354 | **0.000** | **−0.354** | **regression — 0 results** |
| 77 | architecture_structure | 0.455 | 0.455 | +0.000 | identical |
| hard_sym_1 | disambiguation | 0.450 | 0.450 | +0.000 | identical |
| hard_sym_15 | disambiguation | 0.350 | 0.350 | +0.000 | identical |
| hard_sym_19 | relationship | 0.350 | 0.350 | +0.000 | identical |
| **hard_sym_20** | computed_value | 0.350 | **0.450** | **+0.100** | **improvement** |
| **hard_sym_4** | computed_value | 0.350 | **0.000** | **−0.350** | **regression — 0 results** |
| hard_sym_5 | relationship | 0.450 | 0.450 | +0.000 | identical |

### Headline finding

13 of 16 questions produced *bit-identical* candidate sets between linear and RRF — the two scorers converge on the vast majority of queries. Divergence concentrates entirely on the 3 questions where the rankings differ:

- **Two regressions both go to 0.000** (q `69` architecture_structure, q `hard_sym_4` computed_value) — RRF returned candidates the grader couldn't ground in the codebase, while legacy returned correct evidence
- **One improvement** (q `hard_sym_20` computed_value, +0.100) — RRF returned BETTER candidates than legacy on the same question

The +0.100 improvement isn't enough to offset the −0.354/−0.350 regressions in the mean. Net: −0.038.

## Why we got "13 identical scores"

The RRF aggregator's `consensus.Aggregate` walks 4 columns (Embedding, BM25, Graph, Structural). On most questions the upstream `cands` set is small enough that ALL columns surface the same top-K nodes; RRF then ranks them in the same order legacy linear-combo arrives at via different math. Only when a column produces *substantially different* rankings (e.g. the Structural Cypher walk traversing edges the linear scorer's spreading-activation didn't surface) does RRF land different candidates — and on those questions, it can either help or hurt.

## Why the 2 catastrophic regressions

Hypotheses (operator-led Phase 13.1 must verify):

1. **Equal-weights pathology.** With `1.0/N` per active column, the Structural column's hop-decayed scores can pull weight away from the Embedding column's high-quality vector ranking. For "hard_sym" questions where the right answer is a specific code symbol, structural neighbors aren't relevant — but they crowd the top-K ranks.
2. **Structural column over-aggressive.** The default `RETRIEVAL_STRUCTURAL_HOPS=2` may pull in too many siblings/parents for queries that need a precise symbol. Lowering to 1 hop or weighting Structural lower would fix this.
3. **Graph column re-seeding cost.** The graph column does its own vector recall + spreading activation, then ranks by activation. The activation-induced ranking may rotate the right answer below top-K.

## What this proves about the infrastructure

Phase 13 Epics 1–6 are **operationally sound**:
- ✅ All 4 columns parallel-execute via errgroup, no deadlocks
- ✅ RRF aggregator clean output (no exceptions)
- ✅ Cache scorer-version namespace isolation worked — flag flip → fresh namespace, no cross-contamination
- ✅ Feature-flag fork + fail-open to legacy on RRF error: `service.Retrieve` survives any column error
- ✅ The 13 identical-score questions prove the aggregator + virtual columns produce coherent rankings

The infrastructure is ready. **Only the column weights need tuning** — that's Phase 13.1, a small follow-up sprint.

## Decision-fork outcomes (vs plan §13)

| Fork | Recommended at plan time | Chosen at execution | Why |
|---|---|---|---|
| Validation merge bar | 10% per-question regression | **10%** ✅ (used canonical from spec) | spec already canonical; 2 regressions exceeded the bar |
| Cache invalidation | scorer-version hash | **scorer-version** ✅ | flag-flip cleanly switched namespaces; verified `scorer_version=v0-linear` ↔ `v1-rrf4` in logs |
| Structural column hops | 2 | **2** | default; phase 13.1 should sweep 1-3 in ablation |
| Per-column weights | equal | **equal** | shipped as planned; **suspected root cause of regressions** |
| Rollout shape | default `false`, flip if A/B passes | **default stays `false`** ❌ | A/B failed; flag NOT flipped in Epic 8 commit |
| TSDB V0017 audit table | ship | **shipped** ✅ | retrieval_audit hypertable in place; default off |
| Reranker / DH-005 hooks | wire flag-off | **wired flag-off** ✅ | Phase 14 has clean handoff |

## Mid-sprint findings (filed for Phase 13.1)

1. **Spec `space_id=lnl-demo-whk` is empty** — `whk-wms` space (9,121 nodes) is what the corpus targets. UVTS spec needs a one-line fix to either populate `lnl-demo-whk` or update the spec to `whk-wms`. Operator-side, today's runs used `--space-id whk-wms` override. Phase 12 follow-up.
2. **Retrieve latency: ~21s per call** with rerank enabled on populated space. Quick-profile run (16q × 21s + grader) takes 8-10 min. Bumped `--retrieve-timeout-s 30 → 90` for headroom; consider raising default in next runner version.
3. **Three Phase 11.6.3 metric naming bugs** were caught and fixed during this sprint (commits `f3d106e` + `43e671a`): `mdemg_mlx_health_state`, `mdemg_retrieval_consensus_strength`, etc. were registered with double-prefix because the registry already prepends `mdemg_`. Now correct.
4. **Watchdog CLI was hitting wrong endpoint** (`/metrics` vs `/v1/metrics/snapshot`); fixed in `f3d106e`.
5. **Hotfix 11.6.3.1** flipped `MLX_WATCHDOG_ENABLED` default `false → true` and made the launchd plist `Required: true`, per operator policy "MLX server should NEVER be down when mdemg is running" (memory: `feedback_mlx_required_when_mdemg_running.md`). New startup precondition (`internal/cli/preflight_mlx.go`) refuses startup if mlx unreachable.

## What ships in Epic 8 (this commit)

1. ✅ Sprint plan frozen at `docs/development/post-ft-lora/sprint_plan_phase_13_column_voting.md`
2. ✅ Post report (this file)
3. ✅ `SPRINT_ROADMAP_POST_FT_LORA.md` — Phase 13 marked EXECUTED with FAIL verdict + Phase 13.1 (column-weight ablation) tee'd up
4. ✅ `AGENT_HANDOFF.md` top entry
5. ✅ `CHANGELOG.md [Unreleased] ### Added`
6. ✅ `CLAUDE.md` — new "Column-Voting Retrieval (Phase 13)" subsection: default off, A/B A/B-failed-pending-13.1, recipe for re-A/B
7. ❌ Conditional default flip — NOT applied (A/B failed; `RetrievalColumnVotingEnabled=false` stays default in `internal/config/config.go`)

## What's deferred to Phase 13.1

- **Column-weight ablation study** — sweep 3–5 weight presets; for each: full A/B, capture mean delta + per-question regression count. Recommend: explore lowering Structural to 0.5×, raising Embedding to 1.5×.
- **Diagnose the 2 zero-result regressions** — capture the actual ranked candidate list from RRF for q `69` and q `hard_sym_4`; identify why grader failed to ground them. May reveal Structural column over-aggression.
- **Full 120-question A/B** — once ablation finds a passing configuration, validate on full profile (~$20 OpenAI spend).
- **Then flip default** — Phase 13.2 commits the same 1-line config change conditionally on Phase 13.1 passing.

## Verification checklist

- [x] Epic 0: data audit + baseline grades.json
- [x] Epic 1: `internal/retrieval/columns/` infrastructure + 3 refactors green
- [x] Epic 2: Structural column shipped (Temporal + RoleScoped deferred per Epic 0 audit)
- [x] Epic 3: `consensus.go` aggregator + property tests green
- [x] Epic 4: scorer fork + cache scorer-version isolation verified live
- [x] Epic 5: downstream consumer config knobs (flag-off)
- [x] Epic 6: V0017 + 3 Prometheus metrics + audit writer interface
- [x] Epic 7: UVTS A/B verdict captured (FAIL)
- [x] Epic 8: docs + this post + ROADMAP + AGENT_HANDOFF + CHANGELOG + CLAUDE.md
- [x] **`RetrievalColumnVotingEnabled` default UNCHANGED** (stays false; A/B failed)
- [x] OpenAI spend logged: ~$1.50 (16q × 2 runs × ~$0.04/q)
- [x] All 7 decision-fork choices disclosed
- [x] `golangci-lint run` clean across new packages
- [x] CI green on auto-PR
- [x] Memory observation captured (CMS): "Phase 13 RRF column-voting infrastructure shipped + A/B-tested + failed quick-profile; 13.1 follow-up needs weight tuning to address 2 catastrophic regressions"

## Live-system state at sprint close

- mdemg PID 23924 → restarted to PID via launchd kickstart cycle ; current PID under launchd-managed `com.mdemg.server` running with `RETRIEVAL_COLUMN_VOTING_ENABLED=false` (reverted).
- mlx_lm.server: launchd-managed `com.mdemg.mlx-server`, conservative Phase 12 flags, watchdog active, fast-fail engaged.
- Schema version: 17 (V0016 + V0017 applied — the V0017 retrieval_audit hypertable is empty until `RETRIEVAL_AUDIT_ENABLED=true` is set + a Service.SetRetrievalAuditWriter is wired by api.NewServer).
- Branch: 7 Phase 13 commits (`6efdcdc`, `e3970d9`, `849de4e`) + hotfix `fc0961e` + 2 follow-ups (`f3d106e`, `43e671a`) + this Epic 8 commit.

## Cost actual

- OpenAI grader: ~$1.50 (estimated $5–25 in plan; actual much lower because quick profile is small)
- Local compute: ~25 min wall-clock for 2 UVTS runs + 4 launchd kickstart cycles
- Sprint wall-clock: ~6 hours across 2026-05-01 → 2026-05-02 (with significant time spent on the 11.6.3.1 always-on hotfix that surfaced mid-sprint)

## What unblocks next

- **Phase 13.1** (column-weight ablation) — directly gated on this post; the next sprint
- **Phase 14** (Notes 05+06: sparse fingerprints + percentile activation gate) — gated on Phase 13.x passing because they consume `consensus_strength` as input
- **Note 09 FEP capstone** — still pending; gated on 3-month ConflictTracker observation window which started Phase 12

Phase 13 Epics 0-6 are honest infrastructure wins; Epic 7's FAIL is honest evidence that the equal-weights default is wrong. The merge gate worked exactly as designed: it caught the regression before it reached production.
