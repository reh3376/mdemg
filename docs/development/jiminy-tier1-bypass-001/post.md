# JIMINY-TIER1-BYPASS-001 — Sprint Post

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** JIMINY-CEILING-INVESTIGATION-001 defect **B**
(tier1 systematically mislabels follows as ignored). Q4 follow-up #4.

## Verdict

**Shipped.** Tier1 (embedding-similarity) is now bypassed for the
follow/ignore decision when `JIMINY_TIER1_BYPASS_ENABLED=true`. The
sub-`naThreshold` → `NotApplicable` pre-gate stays tier1 (a real
optimization — gates truly-unrelated cases at zero LLM cost). All
other cases route to the LLM tier2 which has full prompt context
(including the shipped context-mismatch and non-violation-credit
clauses from prior sprints).

## What shipped

- **Config**: `JiminyTier1BypassEnabled` env `JIMINY_TIER1_BYPASS_ENABLED`
  (default false; flipped ON in `.env` after live smoke)
- **`internal/jiminy/outcome_classifier.go::Classify`** — three-branch
  gate change:
  - Sub-`naThreshold` (sim < 0.10): `NotApplicable` tier1 (UNCHANGED)
  - `[naThreshold, lowThreshold)` band (0.10 ≤ sim < 0.20): was
    tier1 `Ignored`, now falls through to LLM tier2 when bypass=true
  - `≥ highThreshold && !hasNegation` (sim ≥ 0.55): was tier1
    `Followed`, now falls through to LLM tier2 when bypass=true
  - Middle band + high-with-negation: LLM tier2 (UNCHANGED both ways)
- **`internal/jiminy/service.go`** — constructor wire from
  `cfg.JiminyTier1BypassEnabled` + startup log
- **`internal/jiminy/tier1_bypass_test.go`** — 6 unit tests covering
  every branch × flag state combination, using an httptest LLM stub
  server to verify routing happens

## Why this matters — the ceiling investigation's defect B

**Live 7d pre-flip data on mdemg-dev:**

| classifier_source | outcome | n | avg_sim |
|---|---|---|---|
| tier1 | ignored | 414 | 0.166 |
| tier1 | followed | 6 | 0.570 |
| llm | ignored | 569 | 0.837 |
| llm | followed | 135 | 0.866 |
| llm | partial_compliance | 8 | 0.750 |

The tier1 `ignored` rows (414/7d) all live in the [0.10, 0.20)
similarity band — topically related but low embedding-sim. The
JIMINY-CEILING-INVESTIGATION-001 found this cohort has a **1% follow
rate on real durable rules** — because embedding-sim between rule
text and action text is DEFINITIONALLY blind to follows: "committed
to reh3376_dev01 branch" *follows* "never commit to main" but the
embeddings aren't similar.

Bypassing this tier1 verdict and routing to LLM lets the LLM classifier
apply the full shipped prompt (with context-mismatch, non-violation-
credit, and its own understanding of what "following" means) to the
decision. Expected effect: correct labeling of the ~414/7d cohort as
either `followed` (true positives previously mislabeled), `not_applicable`
(unrelated context — filtered out of `constraint_outcomes` by the
shipped writer gate), or genuine `ignored`.

## Cost analysis

- Pre-flip: ~102 LLM calls/day, ~35% tier1 fraction (420/1187 in 7d)
- Post-flip: expect +60 LLM calls/day (the ~414/7d ignored + ~6/7d
  followed rows shift to LLM = ~60/day)
- Total: ~162 LLM calls/day — well within current substrate capacity
- Guard: alert-evaluator's `alert_llm_health` rule watches for
  saturation; no new saturation alerts fired during the smoke window

## Live Tier-3 smoke on mdemg-dev

Server restarted 2026-07-30 10:21:22 UTC with `tier1_bypass=true`.
Startup log confirmed the wire:
```
level=INFO msg="jiminy: semantic outcome classifier enabled"
  ... tier1_bypass=true
```

**Baseline 24h before flip:**
- tier1 43.3% (282/652)
- llm 49.5% (322/652)
- heuristic 7.2% (47/652)

**Post-flip observation window** (post 10:21:22 UTC):
```
 classifier_source | outcome_type | n  | avg_sim
-------------------+--------------+----+---------
 llm               | ignored      | 10 |   0.830
```
**100% llm, 0% tier1** — the bypass is working exactly as designed.
Because `not_applicable` is filtered from `constraint_outcomes` by
the shipped writer gate at `service.go:1730,1762`, the only tier1
rows that should APPEAR in the table are the (rare) legacy behaviors
that no longer fire under bypass. Steady state: LLM handles all
follow/ignore decisions; the sub-`naThreshold` `NotApplicable`
pre-gate stays tier1 (invisible in this table).

## Rules pinned

⚠️ **Embedding-similarity between rule text and action text is
definitionally blind to follows.** A rule like "never commit to main"
and an action like "committed to reh3376_dev01 branch" have low
cosine similarity, but the action IS a follow of the rule. Any
classifier that reduces the follow/ignore decision to a similarity
threshold will systematically mislabel follows. The correct
architectural role for tier1 embedding-similarity is as a fast
pre-gate for `not_applicable` (unrelated) only — where semantic
distance IS the right signal.

⚠️ **When bypassing a fast-path classifier, verify the downstream
path's cost + latency lift is small.** Live volume analysis
(35% tier1 baseline, 5-10 LLM calls / minute organic traffic on
mdemg-dev) let this ship confidently: +60 LLM calls/day is well
within saturation headroom.

## Follow-ups disclosed

1. **JIMINY-CORPUS-002** (JIMINY-CEILING-INVESTIGATION-001 defect A)
   remains open — corpus contamination (~55% of surface volume goes to
   non-rules like auto-* narratives). Independent of this sprint; the
   next lever for follow-rate improvement after the bypass A/B
   settles.

2. **Follow-rate lift measurement**: pre-flip 7d baseline follow rate
   on real durable rules was ~18.6% (llm-only slice). Predicted lift
   after this sprint + JIMINY-CLASSIFIER-CONTEXT-001 (shipped) +
   JIMINY-ACTIONABILITY-COMPLIANCE-CREDIT-001 (shipped) compounding:
   ~35-50% honest ceiling per the investigation. Requires 3-7d of
   post-flip data to measure honestly.

3. **The 90% target may itself be miscalibrated** (investigation
   pinned). Some rules genuinely won't apply to every relevant-looking
   action, and legitimate operator behavior sometimes needs to violate
   rules with reason. 50-70% may be the honest ceiling. Not a task —
   an expectation-setting note.

## Documents Accessed

- `docs/development/jiminy-ceiling-investigation-001/post.md` (parent —
  defect B analysis + 7d cohort data)
- `docs/development/jiminy-tier1-bypass-001/sprint_plan.md` (this dir)
- `internal/jiminy/outcome_classifier.go::Classify` (target function)
- `internal/jiminy/service.go` (constructor wire site)
- `internal/config/config.go` (env-var wire pattern)
- `internal/jiminy/relevance_gate_test.go` (test pattern reference)
- Live TSDB queries against `constraint_outcomes` (7d cohort +
  post-flip observation window)
- Live server log (`~/.mdemg/logs/server.log`) for wire confirmation
