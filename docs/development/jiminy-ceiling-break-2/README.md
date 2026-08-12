# JIMINY-CEILING-BREAK-2 — Master Arc

**Target: raise Jiminy actionable follow rate from ~12% → ≥80%.**

## Why this arc exists

Operator directive 2026-08-11: *"We need Jiminy to provide relevant and important guidance, not junk. The follow / partial follow rate should be >80% at a minimum. If that requires a new model, more data, better training, etc... that's what we will need to plan out and execute."*

This overrides the JIMINY-FOLLOW-RATE-REMEASURE-001 (2026-08-08) verdict that framed ~11-12% as "honest steady state" and downgraded ceiling-break work to non-urgent. Corrects the JIMINY-HEURISTIC-DEFAULT-001 (2026-08-10) framing that panel-title-embedded ~12% as a target ("honest ~12% post-heuristic-fix"). Both were normalization violations of the 2026-08-01 architectural directive (`trust-signal-must-be-persisted-never-ignore-honest`) that operator explicitly reinforced.

The 12% baseline is not "by design." It's a substrate-quality failure that the whole system is downstream of. This arc owns the >80% goal.

## Ceiling analysis — where the ~88% ignore rate comes from

Ranked by expected leverage:

1. **Corpus junk** — pre-JIMINY-CORPUS-003, 64 live constraints on `mdemg-dev`; audit found ~48% were session-records, event-logs, narratives, or non-canonical duplicates. Every irrelevant surface trains the agent to ignore. JIMINY-CORPUS-003 tombstoned 31 → 33 canonical. Delta expected: +2-5pp.
2. **Retrieval over-surfacing** — Lever C's role-filtered fetch (JIMINY-ACTIONABILITY-001 E5) surfaces constraints at 2.3× the per-unique-ID rate of advisory. Constraints appear in contexts they don't apply to. LEVER-C-TIGHTEN-001 shipped a partial fix. Delta from further tightening: +5-10pp.
3. **Enforcement default** — Strict mode is per-session; not every MDEMG-consuming agent runs with it default-on. JIMINY-CORPUS-003 Stage 2 verified `JIMINY_MODE=strict` + `JIMINY_STRICT_DEFAULT_ENABLED=true` are the shipped defaults + `claude-core` boot session gets enforcement. Delta: mechanical +10-20pp on escalated rules once escalation velocity increases.
4. **Classifier semantics** — Current LLM classifier's ~1% partial_compliance rate + JIMINY-CLASSIFIER-CONTEXT-001's 3pp actual lift vs 18-25pp predicted suggests the LLM under-classifies real "context-mismatch → not_applicable" cases. Delta from prompt tightening: +5-10pp.
5. **Classifier training** — HITL-CURATION-003 shipped auto-grading but operator has 4 pending grades in 7d. Real-labeled corpus growth is the highest-leverage lever for lifting the ceiling; needs velocity work + fine-tune. Delta: +15-30pp with a real retrain cycle.
6. **Model swap** — Muse Glimmer 30B or successor. Delta unknown but bounded — no model change fixes upstream corpus + surfacing problems.

Ceiling arithmetic:
- 12% + 5pp (corpus) + 10pp (retrieval) + 15pp (enforcement) + 10pp (classifier semantics) + 25pp (retrain) = **77%**
- With favorable stacking + model swap ceiling lift: **≥80% achievable**

## Phase plan

Each phase ships measurable delta. All rollback-safe. Each phase ends with a passive re-check window (7d) before the next fires so we can attribute impact.

### ✅ Phase 1 (SHIPPED 2026-08-11) — Corpus purge + strict-mode default

**Sprint:** JIMINY-CORPUS-003
**Delta:** 64 → 33 canonical constraints (48% purge); strict-mode default verified live at boot.
**Expected: +2-5pp actionable follow rate over 7d as the 168h window rolls off signals from the purged nodes.**
**Passive re-check: 2026-08-18.**

### Phase 2 — Retrieval precision (LEVER-C-TIGHTEN-002)

Tighten constraint-partition direct-fetch surface further. Options:
- Reduce `JIMINY_GUIDANCE_CONSTRAINT_INCLUDE_TOPK` (currently 4)
- Raise `JIMINY_GUIDANCE_CONSTRAINT_SIM_FLOOR` (currently 0.55)
- Add explicit-context-tag filtering — a constraint whose scope tags don't match the current action's context is suppressed regardless of embedding similarity
- Add per-tool-type constraint routing (e.g. code-editing tools only see coding-related constraints)

**Expected: +5-10pp. Effort: ~1 day.**

### Phase 3 — Classifier semantics (JIMINY-CLASSIFIER-CONTEXT-002)

JIMINY-CLASSIFIER-CONTEXT-001 (2026-07-21) predicted +18-25pp; delivered +3pp because 88% of NA routing landed on advisory (outside actionable denominator). Round 2 targets imperative-constraint NA routing specifically:
- Add per-constraint-type prompt variants (`must` vs `must_not` vs `should`)
- Extend context-mismatch clause with per-tool-type applicability rules
- Add a "would applying this constraint have changed the action?" question to the classifier chain

**Expected: +5-10pp on the actionable denominator directly. Effort: ~1 day.**

### Phase 4 — HITL curation velocity + retrain

**4a — HITL velocity sprint (JIMINY-HITL-VELOCITY-001):**
- Bulk-review keyboard-only UI (grade 40+ items per operator session)
- Autograde-first-then-confirm workflow (operator reviews only autograde disagreements)
- Corpus-lift dashboard: per-week gold-label growth + retrain-readiness signal

**Expected: get corpus to N=500+ operator-confirmed golds. Effort: ~2 days.**

**4b — Guidance-classifier retrain (JIMINY-CLASSIFIER-RETRAIN-001):**
- Follow the FT recursive-loop shipped path (`docs/features/ft-recursive-loop.md`)
- Target: LoRA on `mdemg-llm-v1` (or successor) trained specifically on the guidance-classification task
- Gate benchmark: aggregate ≥ current baseline + per-constraint follow-rate lift

**Expected: +15-25pp actionable rate. Effort: ~3-5 days including gate + promotion.**

### Phase 5 — Model evaluation & swap

**Sprint:** MODEL-SWAP-MUSE-GLIMMER-EVAL-001 (already queued)
Ship the eval; if positive, DEPLOY sprint follows.

**Expected: +?pp — depends entirely on what the eval shows. Delta may be marginal if upstream problems dominate.**

## Cross-cutting: measurement discipline

Every phase ends with:
- 7d passive re-check window
- TSDB dashboard delta capture
- Attribution: was the delta from this phase, or from natural traffic shifts?

If any phase's realized delta is <50% of predicted, PAUSE the arc and re-diagnose. Prediction vs actual is a first-class signal — don't stack phases blindly on top of misdiagnoses.

## What this arc EXPLICITLY rejects

- **The ~12% "honest steady state" framing** — it's a failure state, not a design feature. Any panel description, alert floor, or docs framing that normalizes it must be updated.
- **Corpus purge alone as the fix** — Phase 1 alone will not reach the target. Corpus purge is necessary but insufficient.
- **Model swap as a magic bullet** — no LLM fixes an unfiltered noisy corpus that surfaces constraints in wrong contexts. Model work only pays off after upstream stack is aligned.
- **"Neither urgent" framing** — this arc IS the urgent frontier. Everything else queues behind it until we're materially closer to 80%.

## Doc + framing hygiene cleanup

To be done as part of Phase 2 kickoff:
- Panel titles that embed "~12% by design" → change to trajectory language ("current 12%, target 80%")
- CLAUDE.md pin for JIMINY-HEURISTIC-DEFAULT-001 → add annotation that the "~12% honest" framing is under active revision by this arc
- CHANGELOG entries for JIMINY-FOLLOW-RATE-REMEASURE-001 + JIMINY-HEURISTIC-DEFAULT-001 → add annotations

## Current state (2026-08-11)

- **Phase 1: SHIPPED** — JIMINY-CORPUS-003 tombstoned 31 nodes + verified strict-mode default. Passive re-check 2026-08-18.
- Phase 2-5: queued, this arc doc owns the plan.

## Baseline snapshot (2026-08-12 18:17 UTC — arc Phases 1-4a + arc-adjacents SHIPPED)

Captured immediately after the framing hygiene sweep landed (all shipped Phase-1-through-4a work is live on mdemg-dev). This snapshot is the AUTHORITATIVE "before" for the 2026-08-19 passive re-check window rollover.

### Corpus (live, non-archived)

| role_type | actionable | informational | total |
|---|---:|---:|---:|
| constraint | 26 | 7 | 33 |
| correction | 2 | 1 | 3 |
| **total** | **28** | **8** | **36** |

Pre-arc (before JIMINY-CORPUS-003 + JIMINY-CORRECTION-CORPUS-001): 64 constraints + 39 corrections = 103 nodes. Post-arc: **36 nodes → 65% cut**. Of the surviving 36, 8 (22%) are marked informational and correctly excluded from actionable follow-rate accounting via the JIMINY-INFORMATIONAL-CATEGORY-001 override path.

### Follow-rate signal (7d window, `constraint_outcomes`, filtered to `guidance_type IN ('constraint','correction')`)

| Metric | Value |
|---|---:|
| Actionable outcomes (7d) | 1412 |
| Followed | 166 |
| Partial compliance | 41 |
| Ignored | 1199 |
| Contradicted | 2 |
| Not applicable | 0 (already filtered at writer gate) |
| **Actionable follow rate** | **13.25%** |
| `mdemg_jiminy_follow_rate` gauge | 14.00% |
| Total (all guidance types, 7d) | 2358 |

### What the 2026-08-19 re-check will measure

The 168h rolling window will have fully rolled off pre-arc outcomes (JIMINY-CORPUS-003 tombstones from 2026-08-11 ~20:00 UTC; JIMINY-CORRECTION-CORPUS-001 tombstones from 2026-08-12 ~14:00 UTC). Attribution of the delta:

- **Corpus purges** (JIMINY-CORPUS-003 + JIMINY-CORRECTION-CORPUS-001): projected +8-15pp (tombstoned codes stop generating ignored outcomes as historical rows age out)
- **Scope-gate** (LEVER-C-TIGHTEN-002): projected +5-10pp (constraints stop surfacing on unrelated-context queries; ignored count drops proportionally)
- **Mechanism-scope classifier gate** (JIMINY-CLASSIFIER-CONTEXT-002): projected +5-10pp (LLM verdicts route more items to `not_applicable`, shrinking actionable denominator)
- **Informational category** (JIMINY-INFORMATIONAL-CATEGORY-001): projected +0.6pp (7 meta-rule outcomes routed to NA; small because meta rules are low-volume)
- **Correction dedup** (CREATE-CORRECTION-DEDUP-001): 0pp direct — prevents re-accumulation of the 24-DUP class; measurable via `SkippedDup` counter, not follow rate
- **Composite projected**: **13.25% → 22-35% by 2026-08-19**

If actual delta is <50% of projected on any lever, arc pauses per the JIMINY-CEILING-BREAK-2 measurement-discipline rule and re-diagnoses. If actual matches or exceeds, Phase 4b (classifier retrain, gated on corpus growth via keyboard-driven HITL Velocity UI) becomes the next fire.

### Composite arc shipped 2026-08-11 → 2026-08-12 (13 sprints in ~14 hours)

| # | Sprint | Commit | Nature |
|---|---|---|---|
| 1 | JIMINY-CORPUS-003 | `ebef90a2` | 31 constraints tombstoned |
| 2 | LEVER-C-TIGHTEN-002 | `ee10d28a` | scope-gate at surfacing |
| 3 | JIMINY-CLASSIFIER-CONTEXT-002 | `c3ba063a` | mechanism-scope gate at classify |
| 4a | JIMINY-HITL-VELOCITY-001 | `1ae6e358` | keyboard-driven bulk review |
| adj | JIMINY-INFORMATIONAL-CATEGORY-001 | `dab5628a` | is_informational property |
| adj | JIMINY-CORRECTION-CORPUS-001 | `6fda2ec3` | 35 corrections tombstoned |
| adj | CREATE-CORRECTION-DEDUP-001 | `f796fd27` | promoter dedup |
| adj | FRAMING-HYGIENE-SWEEP-001 | `9b494f0f` | trajectory language everywhere |

Total: 8 shipped sprints (12 including the security + beta + investigation + queue + measurement sprints from earlier session-day).

## Documents Accessed

- Operator directive 2026-08-11 (both mid-turn messages: "complete failure" + ">80% target")
- Operator directive 2026-08-01 (`trust-signal-must-be-persisted-never-ignore-honest`)
- `docs/development/jiminy-corpus-003/{sprint_plan,tombstone_list,pre_purge_backup.jsonl}`
- `docs/development/jiminy-follow-rate-decline-2026-08-10/INVESTIGATION.md`
- `docs/development/jiminy-heuristic-default-001/post.md`
- `docs/development/jiminy-follow-rate-remeasure-001/verdict.md`
- `docs/development/jiminy-classifier-context-001/ab_verdict.md`
- `docs/development/jiminy-actionability-inversion-001/fix_spec.md`
- `docs/development/lever-c-tighten-001/`
- `docs/features/hitl-auto-curation.md`
- `docs/features/ft-recursive-loop.md`
- `docs/development/muse-glimmer-30b-investigation/RESEARCH_MEMO.md`
- `docs/development/model-swap-muse-glimmer-eval-001/sprint_plan.md`
- Live Cypher against mdemg-dev pre + post JIMINY-CORPUS-003 purge
