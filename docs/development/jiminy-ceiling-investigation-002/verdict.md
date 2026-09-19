# JIMINY-CEILING-INVESTIGATION-002 — Verdict

**Investigated**: 2026-09-03
**Verdict**: **Root cause identified — the arc's ceiling arithmetic assumed a healthy mix of surfaced rules; the substrate reality is that 97% of graded rows are structurally unverifiable by any LLM classifier.** Phase 4b retrain won't help this class. Two honest paths forward; both non-trivial.

## TL;DR

- **Baseline** (pre-arc 30d, 2026-07-11 → 2026-08-11): **13.97%** (the arc doc's "~12%" was slightly low)
- **First week post-Phase-2+3** (2026-08-12 → 2026-08-19): 17.62% (+3.65pp — the arc DID move the needle)
- **Post-recheck stable window** (2026-08-19 → 2026-09-03, 15d, n=653): **15.62% (+1.65pp cumulative)** — decayed from post-week-1 peak
- **Predicted through Phase 3**: +15-25pp → actual +1.65pp → **~92-93% shortfall**
- **97% of graded rows in 168h are from the top-12 meta/process rule codes** — rules whose "did the agent follow this?" cannot be verified from the action-text the classifier sees
- **Excluding those 12 codes: 7 rows / 0 followed / 0% follow rate** — the action-content rules generate almost NO graded traffic

**Retrain (Phase 4b as planned) cannot fix this.** Two honest options: (a) mark meta/process rules as informational (JIMINY-INFORMATIONAL-CATEGORY-001 escape hatch), or (b) add a NEW verification signal for process-observation events (process observers → process-outcome events → new grader).

## D1 — Per-phase attribution

| Window | Range | Total | Followed | Partial | Ignored | Follow% |
|---|---|---|---|---|---|---|
| A pre-arc baseline 30d | 2026-07-11 → 2026-08-11 | 3,646 | 404 | 211 | 3,016 | **13.97%** |
| B Phase-1 immediate | 2026-08-11 → 2026-08-12 12:00 | 228 | 25 | 0 | 203 | 10.96% (transient) |
| C Phases 2+3 first week | 2026-08-12 12:00 → 2026-08-19 | 789 | 137 | 4 | 645 | **17.62%** (+3.65pp) |
| D Post-recheck stable | 2026-08-19 → 2026-09-03 | 653 | 101 | 2 | 546 | **15.62%** (+1.65pp) |

⚠️ **Phase 2+3 lift decayed** — the immediate post-shipping window (~7 days) showed a +3.65pp bump; the following 15 days settled at +1.65pp cumulative. The bump was transient, not additive.

Predicted arithmetic (from arc kickoff): P1 +5 + P2 +5-10 + P3 +5-10 = **+15-25pp**.
Actual delivered: **+1.65pp cumulative**. Every phase underdelivered its predicted contribution.

## D2 — Surfacing volume + composition

Volume-per-day (pre-arc peaks vs post-arc):
- Pre-arc: 167/590/298/116/302 (peaks 590/day)
- Post-arc: 151/62/100/41/14/27/11/16/76/126/29 (peaks 126/day)
- Volume dropped ~50% and became noisier. Sample-size gate on daily estimates.

## D3 — Classifier verdict + source distribution shift

| Verdict | Pre-arc (14d) | Post-arc (14d) | Δ |
|---|---|---|---|
| ignored | 81.77% | 84.34% | +2.57pp (WORSE) |
| followed | 11.69% | 14.66% | +2.97pp (BETTER, +25% relative) |
| **partial_compliance** | **6.18%** | **0.20%** | **−5.98pp (30× DROP)** |
| contradicted | 0.16% | 0.80% | +0.64pp |

**⚠️ partial_compliance collapse is a metric artifact.** Phase 3's mechanism-scope gate + META-SCOPE flip (see D7) route what USED to be partial_compliance verdicts to `ignored`. Since the follow_pct formula weights partial as 0.5, losing 6% partial mass while gaining 3% followed + 3% ignored is roughly net-neutral in raw rate. The arc's classifier tightening is IMPROVING quality by being stricter, but the metric measures this shift as NEUTRAL because partial_compliance was previously carrying half-credit weight.

Classifier source (pre → post):
- llm: 87.79% → 84.74% (~stable)
- heuristic: 5.90% → 15.26% (+9.36pp — the LLM classifier is timing out or being bypassed more often)
- tier1: 5.70% → 0% (JIMINY-TIER1-BYPASS-001 shipped 2026-07-30 removed tier1 routing — expected)

⚠️ **heuristic share TRIPLED** — since JIMINY-HEURISTIC-DEFAULT-001 (2026-08-10) shifted the heuristic default from `partial_compliance` (0.5 credit) to `ignored` (0 credit), heuristic share tripling directly deflates the follow rate. This adds another **~4-5pp of measurement drag** on top of the partial_compliance collapse.

## D4 — Top-15 ignored constraint codes (168h)

| Code | total | ignored | followed | Follow% | Class |
|---|---|---|---|---|---|
| agent-handoff-requirement-guardrail | 31 | 28 | 3 | 9.7% | **META (UI-completion policy)** |
| auto-build-restart-after-feature | 33 | 27 | 6 | 18.2% | **META (workflow)** |
| memory-preservation-backup-integrity | 32 | 24 | 7 | 23.4% | **META (observability)** |
| iterate-break-fix-verify | 25 | 22 | 3 | 12.0% | **META (process)** |
| plan-mode-before-change | 23 | 20 | 3 | 13.0% | **META (process)** |
| must-validate-all-claims-before-commit | 23 | 19 | 4 | 17.4% | **META (process)** |
| project-planning-docs-in-repo-only | 21 | 17 | 4 | 19.0% | **META (documentation)** |
| end-with-docs-accessed | 15 | 13 | 2 | 13.3% | **META (documentation)** |
| openai-max-completion-tokens | 13 | 11 | 2 | 15.4% | action-content |
| no-direct-main-commits | 12 | 8 | 4 | 33.3% | action-content |
| live-testing-tier-required | 7 | 5 | 2 | 28.6% | **META (process)** |
| never-skip-discovered-issues | 5 | 4 | 1 | 20.0% | **META (process)** |

**12 of 15 top-ignored codes are meta/process rules.** These govern the agent's PROCESS (did we plan? did we test? did we validate claims? did we back up? did we hand off correctly?), not the ACTION-CONTENT the classifier sees.

The classifier is NOT WRONG when it marks these ignored — it has no observable to grade them against. The action-text (a diff, a bash command, a file write) shows the RESULT but not the process leading to it. No LLM classifier improvement (retraining, better prompts, meta-scope gates, mechanism-scope gates) can extract "did the agent think first?" from the observable evidence.

## D5 — Informational-flag adoption (JIMINY-INFORMATIONAL-CATEGORY-001)

Live counts on `mdemg-dev`:
- Total live actionable nodes (constraint+correction, non-archived): **33**
- Marked `is_informational=true`: **9**
- Still-graded actionable: 24

Of the top-12 process-class codes above: **ZERO are marked informational.** The shipped escape hatch mechanism from #99 exactly covers this class of rule — 9 nodes have been marked but none from the top-12 driving the ignored volume.

## D6 — Guidance-type mix

| guidance_type | pre-arc (14d) | post-arc (14d) |
|---|---|---|
| constraint | 1,816 | 495 |
| pattern | 1,840 | 148 |
| correction | 691 | 3 |
| learning | 590 | 114 |
| concept | 212 | 31 |
| risk | 34 | 11 |
| decision | 24 | 2 |

- **Total volume ↓84%** (5,210 → 804)
- **correction: 691 → 3 — dropped 99.6%** — the correction sink went nearly dark. Either the L1 correction promoter is not firing recently, or the tag detector isn't matching. Separate follow-up.

## D7 — Classifier prompt-flag current state (`.env`)

```
JIMINY_NONVIOLATION_CREDIT_ENABLED=true       ← CONTEXT-001
JIMINY_CONTEXT_MISMATCH_CREDIT_ENABLED=true   ← CONTEXT-001 sibling
JIMINY_MECHANISM_SCOPE_CREDIT_ENABLED=true    ← CONTEXT-002
```

⚠️ **META-SCOPE flag not present in `.env`** — the code default `false` (per CLAUDE.md pin) is authoritative. The "already-on" hypothesis in Option B of my earlier recommendation was wrong — META-SCOPE remains SHIP-DORMANT. If we DO want to test it, the recommendation from JIMINY-CLASSIFIER-META-SCOPE-001 was "flip after JIMINY-CEILING-BREAK-2 T+168h shows CONTEXT-002 underdelivers" — which is TRUE, so the flag flip is now warranted as a cheap probe.

## Sanity check

Recompute 168h follow rate EXCLUDING the top-12 process-class codes:

| Window | Total | Followed | Follow% |
|---|---|---|---|
| 168h with top-12 process codes INCLUDED | 248 | 41 | 16.73% |
| **168h EXCLUDING top-12 process codes** | **7** | **0** | **0%** |

**97% of graded rows in the last 7 days are from these 12 meta/process rules.** The action-content rule pool (`must-use-cuid2`, `never-hardcode-config`, `never-direct-alter-schema`, `no-stash-for-release`, etc.) generates almost NO surfaced+graded traffic — presumably because either (a) the agent naturally follows them and they don't surface, or (b) Lever C is surfacing them but no action-text triggers a classifiable outcome.

## Root cause

**The arc's ceiling arithmetic assumed a healthy mix of surfaced rules where the classifier could reliably grade both classes.** The measured substrate reality is:

1. **97% of graded outcomes are meta/process rules** the classifier CANNOT verify from action-text alone
2. **Classifier improvements (Phases 2+3) improved quality on the ~3% action-content minority but the metric is dominated by the 97% majority** where no classifier improvement can help
3. **partial_compliance collapse + heuristic-share tripling** adds ~10pp of measurement drag on top

**Retraining the LLM classifier (Phase 4b as planned) will NOT help this class.** No amount of training makes an LLM better at inferring unobservable process from the observable diff.

## Forward paths (ranked by evidence-strength × cost-effectiveness)

### Path 1 — Mark top-12 process rules as informational (JIMINY-INFORMATIONAL-CATEGORY-001 escape hatch)

- **Cost**: ~30 minutes (12 CLI invocations, fully reversible)
- **Mechanism**: `mdemg jiminy constraint mark --code X --space-id mdemg-dev --informational=true` per code
- **Effect**: these rules stop being graded → they route to `not_applicable` → the metric denominator collapses to the 7-row action-content pool + any newly-surfaced rules
- **Expected metric move**: 16.73% → likely 25-50% (small denominator = high variance; the underlying quality doesn't change)
- **Honest framing**: this is a MEASUREMENT REFRAME, not a quality improvement. It says "we've decided these rules can't be classifier-verified so we won't grade them."
- **Risk**: silently removes ~50% of guidance corpus from the metric surface. Operators can't tell "is this rule being followed?" from the dashboard anymore for the 12 codes. Still surfaced (agents still see them in guidance); just not graded.

### Path 2 — Add a NEW verification signal for process-observation events

- **Cost**: substantial (2-4 sprints of design + implementation; new observation types + graders + persistence)
- **Mechanism**: process observers (did plan-mode fire? did rebuild+restart run? did lint pass?) → new `process_outcome` verdict types → a new grader that reads process events, not action-text
- **Effect**: these rules become classifier-verifiable. Real quality signal.
- **Expected metric move**: cannot predict without knowing the actual compliance distribution — the honest scoreboard for the first time
- **Risk**: engineering effort. Some rules (like `agent-handoff-requirement-guardrail` — "operator user-side hand-inspection required") CANNOT be automated even with process observers. Requires human-in-the-loop for some subset.

### Path 3 — Redefine the follow-rate metric to weight only action-content rules

- **Cost**: 1 sprint (metric taxonomy + panel updates)
- **Mechanism**: partition rules into `classifier_verifiable` / `process_verifiable` / `human_verifiable` taxonomy; grade each class separately; keep the top-line "overall follow rate" as the classifier_verifiable subset
- **Effect**: honest scoreboard for the classifier-verifiable class. Process/human classes get their own gauges.
- **Expected metric move**: same as Path 1 for classifier-verifiable, plus honest zero-baseline for the other two
- **Risk**: taxonomy is a subjective call; some rules straddle classes

### Path 4 — Flip JIMINY_MENTION_VS_PERFORM_CREDIT_ENABLED (Phase 3.5 dormant probe)

- **Cost**: ~5 minutes (env flip + restart)
- **Mechanism**: enable META-SCOPE classifier prompt extension
- **Effect**: routes classifier verdicts on mention-only content to not_applicable; expected small nudge on the ~3% action-content class
- **Expected metric move**: +1-3pp at most; won't move the 97% majority
- **Risk**: T+168h passive re-measure; per JIMINY-CLASSIFIER-META-SCOPE-001 verdict was "flip counts 1/0/0" on live smoke — small effect
- **When**: worth doing regardless, as a cheap probe. Not a substitute for Path 1 or 2.

### Path 5 — Retrain (Phase 4b as originally planned)

- **Cost**: 30-100h + compute (~$X)
- **Mechanism**: LoRA retrain on cleaner corpus (post-#148 + HITL golds from #98's velocity work)
- **Effect on this metric**: **near-zero** — the metric ceiling is bounded by the 97% unverifiable class, not by classifier quality on the 3% verifiable class
- **NOT recommended as the next step** — reserve for AFTER the metric-denominator issue is fixed via Path 1 or 3

## Recommendation

**Ship in this order** for the arc to hit 80%:

1. **Path 4** (cheap probe, ship regardless): flip META-SCOPE now, 168h passive re-measure. Confirms whether the 3.5 flag has any measurable effect.
2. **Path 1** (measurement reframe, ship next): mark the top-12 process rules as informational. This is the shipped mechanism from JIMINY-INFORMATIONAL-CATEGORY-001, exactly the class it was designed for. 168h passive re-measure post-flip.
3. **Path 2 OR Path 3** (design decision): decide whether to build process-verification OR redefine the metric. Path 2 is more honest but expensive; Path 3 is honest AND cheap but changes the arc's north-star.
4. **Path 5** (retrain): only after 1-3 stabilize the metric denominator. Any retrain evaluation on the current metric would grade the model on the wrong scoreboard.

## New architectural rule pinned

**When an outcome metric aggregates rules of different verifiability classes, the metric ceiling is bounded by the mix of the classes — not by the classifier's quality on any one class.** MDEMG's Jiminy actionable follow rate lumps together (a) action-content rules the classifier CAN verify from action-text (`must-use-cuid2`, `never-hardcode-config`) and (b) process/meta rules the classifier CANNOT (`plan-mode-before-change`, `iterate-break-fix-verify`, `agent-handoff-requirement-guardrail`). When (b) dominates the surfaced+graded pool (97% here), no amount of classifier improvement moves the metric — the ceiling is fundamentally set by (b). The fix is either (i) route (b) out of the metric via `is_informational=true` (JIMINY-INFORMATIONAL-CATEGORY-001), (ii) add a NEW signal that CAN verify (b) (process-observation events), or (iii) redefine the metric to grade classes separately. Prompt engineering, mechanism-scope gates, retraining — all wasted compute on this class of gap. Corollary: BEFORE proposing any classifier-side lift for a follow-rate arc, run a top-N-ignored-codes audit to confirm the metric denominator is dominated by classifier-verifiable rules. If it isn't, redefine the metric or route the unverifiable class out first.

## Follow-ups filed

1. **Path 1 execution** — a small sprint to mark the top-12 process rules as informational. Estimated ~30min. Substantial metric denominator collapse; needs operator sign-off on which specific codes to mark (candidate list attached).
2. **META-SCOPE flag flip** (Path 4) — cheap probe; can be done any time; passive re-measure.
3. **Correction sink investigation** — D6 showed `correction: 691 → 3 pre/post arc` (99.6% drop). Either L1 correction producer is silently broken or the L0 correction detector isn't matching. Belongs to its own sprint (`JIMINY-CORRECTION-SINK-INVESTIGATE-001` candidate).
4. **Path 2 or Path 3 design sprint** — pre-Phase 4b decision on how to fix the metric denominator honestly. Should be scoped before any retrain compute is spent.

## Documents Accessed

- Live TSDB `constraint_outcomes` — D1 (4 windowed slices), D2 (per-day), D3 (verdict + source distributions pre/post), D4 (top-15 ignored codes), sanity check
- Live Neo4j `MemoryNode {space_id: 'mdemg-dev'}` — D5 (informational-flag audit)
- `.env` — D7 flag state
- `docs/development/jiminy-ceiling-break-2/README.md` — arc plan
- `docs/development/jiminy-ceiling-investigation-001/` — reference pattern
- Predecessor sprint plans (JIMINY-CORPUS-003, LEVER-C-TIGHTEN-002, JIMINY-CLASSIFIER-CONTEXT-002, JIMINY-CLASSIFIER-META-SCOPE-001, JIMINY-INFORMATIONAL-CATEGORY-001, JIMINY-HITL-VELOCITY-001, JIMINY-TIER1-BYPASS-001, JIMINY-HEURISTIC-DEFAULT-001)
- CLAUDE.md architecture pins
- Operator directive: "run option a" (2026-09-03)
