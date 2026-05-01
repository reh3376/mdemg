# Sprint Roadmap — Post-FT-LORA (Phase 11.6 onwards)

**Date:** 2026-05-01
**Source documents:** `~/Downloads/mdemg-future-sprint-assessments/{mdemg-collaboration-brief.md,mdemg-research-evaluation.md}`
**Internal sprint-idea drafts:** `docs/research/mdemg_sprint_ideas/{01–09}*.md` (9 architectural extensions, gitignored)
**Status:** v1 — proposed prioritization, ready for user decision on next-sprint sequence

---

## Three Workstreams

| Workstream | Cadence | Branch | Purpose |
|---|---|---|---|
| **A. Operational hygiene** | Now (post-11.6) | `reh3376_dev01` | Close the open follow-ups from the production cutover before more work piles on |
| **B. Research extensions** | After workstream A clears + UVTS gate | `reh3376_dev02` (parallel-safe per research-eval) | The 8 architectural extensions from the brief |
| **C. Cross-cutting collaboration** | Always-on, no calendar | n/a | RD-7/Q-DM-1 collaboration asks (object-centric slots, FEP co-implementer); conflicting-guidance tracking; UAITS Paradigm governance |

---

## Workstream A — Operational Hygiene (Phase 11.6.x)

These four items came out of Phase 11.6 cutover smoke-testing and remain open. None is large; all should land before Workstream B starts.

### 11.6.2 — RSIC Scheduler Rate-Limit (Metal OOM mitigation)

**Problem.** RSIC fires 5+ concurrent `ape.reflect` calls within 200ms; even at `mlx_lm.server --prompt-concurrency 4` the queue depth × 130s/call exceeds the 180s timeout. Earlier crashes (Metal `Insufficient Memory (kIOGPUCommandBufferCallbackErrorOutOfMemory)`) are the more dangerous failure mode.

**Fix.** Semaphore in the RSIC scheduler — at most N concurrent LLM-driven actions per micro-cycle. Default N=2 (matches the moderate concurrency level we verified stable). Configurable via `RSIC_LLM_CONCURRENCY_LIMIT` env var.

**Effort:** Small (1-2 days). Single-file change in `internal/ape/` or `internal/rsic/`.
**Risk:** Low. Semaphore is local; rollback = unset env var.
**Verification:** mdemg log shows no concurrent `ape.reflect` >2; mlx_lm.server runs >24hr without crash.

### 11.6.3 — Grafana panels for `model_name` distribution + LLM latency by task

**Problem.** `model_name` column in `llm_interactions` now reflects the cutover (`/Users/reh3376/.../mdemg-llm-v1`). No dashboard panel surfaces this. Operators can't see at a glance whether traffic shifted to the local model or whether per-task latency regressed.

**Add panels:**
- `LLM call distribution by model_name` (stacked time series)
- `LLM latency p50/p95/p99 by task_name × model_name` (heatmap or per-task lines)
- `LLM error rate by task_name`
- `Open circuit-breaker count` (would have caught the earlier breaker-trip cascade pre-emptively)

**Effort:** Small (1 day). Existing `mdemg-overview.json` dashboard JSON gets new panels.
**Risk:** None.

### 11.6.4 — Jiminy production task_name swap fix

**Problem.** `WithContext("jiminy.evaluate", ...)` and `WithContext("jiminy.evaluate_llm", ...)` in `internal/jiminy/` are crossed. TSDB rows tagged `jiminy.evaluate` actually contain `outcome_classifier` content (which is `evaluate_llm`'s prompt). Affects all production rows logged through 2026-04-29 (discovered Phase 11.5e Epic 1).

**Fix.** Identify the swap site (likely in `internal/jiminy/eval_prompt.go` and `internal/jiminy/outcome_classifier.go`); swap them back. Then run a one-off TSDB migration to re-label historical rows by content (using the same content-routing logic from `scripts/x11_jiminy_evaluate_rescue.py`).

**Effort:** Small-medium (1-2 days). Code fix is small; TSDB historical relabel is the larger piece.
**Risk:** Medium. Touching production logging — back up `llm_interactions` before relabel.
**Verification:** Post-fix, new TSDB rows have `task_name` matching `system_prompt_hash` per spec.

### 11.6.5 — Prompt-cache investigation

**Problem.** `mlx_lm.server --prompt-cache-size N` could amortize repeat-prefix calls. `ape.reflect` prompts share the 20-action enum prefix (~200 tokens); with caching, second-and-onward `ape.reflect` calls in a session might skip prefix prefill, cutting latency by 20-30%.

**Effort:** Small (1 day). Set `--prompt-cache-size 2GB`, run RSIC for a couple cycles, measure latency delta.
**Risk:** Low. Cache is RAM-resident; if it OOMs, drop the size.

**Workstream A total: ~4-5 days. Single dev branch sprint.**

---

## Workstream B — Research Extensions (priority-ordered)

The research-evaluation document explicitly ranks these. I've preserved the document's tiering and added our internal context (Phase 11.6 prerequisites, draft-spec status, dependencies).

### Tier 1 — Prerequisite Gate (must precede all extensions)

#### Phase 12 / RD-9 — UVTS Activation

**What.** Promote UVTS (Unified Validation Test Suite) from spec-only to active runner. UVTS is the gating framework that hosts A/B comparisons, soak tests, and merge gates for every architectural extension that follows.

**Why first.** The next 8 extensions all carry "merge after passing UVTS gate" requirements. Without an active UVTS runner, those gates can't fire.

**Effort:** Medium (10-15 dev-days, ~3 weeks).
**Effort gate:** Read `docs/development/ft-lora/sprint_plan_phase_*.md` UVTS scope; we need a 12-section sprint plan for this before execution.
**Status:** Spec exists (somewhere in `docs/tests/uxts/`); no active runner yet.

### Tier 2 — Retrieval Cluster (HTM/Hawkins thread, parallel-safe)

These three compose into a coherent HTM-flavored retrieval architecture per the brief's §3.3. Total ~28 dev-days. Document explicitly notes they have "zero hard cross-dependencies" and can run on a separate dev branch.

#### Phase 13 / Note 04 — Column-Voting Retrieval

**What.** Replace MDEMG's weighted-linear-combination ranker with Reciprocal Rank Fusion ensemble. 6 retrieval columns: existing 3 (Embedding, BM25, GraphProximity) + 3 new (Structural, Temporal, RoleScoped). Output: ranked candidates + per-query `consensus_strength` uncertainty signal.

**Effort:** Medium (12 dev-days, ~3 weeks).
**Pre-existing draft:** `docs/research/mdemg_sprint_ideas/04-column-voting-retrieval.md` (412 lines).

#### Phase 14 / Notes 05 + 06 — Context-Specific Activations + Sparse Gate

**What.**
- Note 05: 256-bit per-observation sparse fingerprints with dynamically-assigned bit positions per space; weekly catalog refresh.
- Note 06: Percentile-based activation gate at end of retrieval pipeline (default p98 → top 2% candidates fire).

**Effort:** Medium (10.5 + 5.5 = 16 dev-days, ~4 weeks combined).
**Pre-existing drafts:** `05-context-specific-node-activations.md` (379 lines), `06-sparse-retrieval-activation.md` (286 lines).

**Workstream B / Tier 2 total: ~28 dev-days for all 3 retrieval extensions.**

### Tier 3 — Predictive Hierarchy

#### Phase 15 / Note 01 — PC Reframe + Surprise Routing (PRECEDENT for Note 02)

**What.** Mostly narrative — formalize that `internal/ape/health_formula.go`'s precision-weighted formula = Bayesian posterior mean = predictive-coding hierarchical error integration. Plus add the one piece of surprise infrastructure that's genuinely missing: **routing** (fast-track / normal / quarantine).

**Effort:** Small-medium (5-7 dev-days). Most of the math is already in `surprise.go` / `learning/service.go`; this is documentation + adding the routing decision tree.
**Pre-existing draft:** `01-pc-reframe-and-surprise-routing.md` (810 lines, the longest draft — heavy on narrative).
**Why now (after retrieval cluster):** Note 02 needs Note 01 to soak ≥1 release cycle. Sequencing Note 01 right after the retrieval cluster ships gives Note 02 a clean launch pad.

#### Phase 16 / Note 02 — Precision-Weighted Hebbian η

**What.** Per-node `activation_confidence` ∈ [0.05, 1.0]; modulates edge-weight learning rate `η_eff = η · c_a · c_b`. Feature-flagged, A/B gated, RSIC autonomous override on health regression.

**Effort:** Medium (7.5 dev-days + 1 release-cycle soak before Note 03).
**Hard prereq:** Note 01 shipped + soaked.
**Pre-existing draft:** `02-precision-weighted-hebbian-eta.md` (423 lines).

### Tier 4 — Layer-Local + Predictive Extensions (parallel-safe after Note 02 soaks)

#### Phase 17a / Note 03 — Top-Down Predictions + Promotion-Error Rule

**What.** New `:PREDICTS` graph relationship; each parent node maintains Bayesian-Laplace-smoothed prediction of children. Promotion gated on prediction-error rather than evidence-counting.

**Effort:** Large (12 dev-days + 2-week shadow soak).
**Risk:** Highest in collection — new schema, ~10× edge inflation possible; mandatory shadow mode + manual-review gate.
**Hard prereq:** Note 02 shipped + soaked.

#### Phase 17b / Note 07 — Forward-Forward Shallow Heads

**What.** 3 small classification heads (promotion, source-trust, context-appropriateness) trained with Hinton's FF algorithm over frozen LLM embeddings. Each head is layer-local; updates online from RSIC outcomes.

**Effort:** Large (11.5 dev-days + soak; partly gated on RSIC having ≥3 months of mature labeled outcomes).
**Hard prereq:** Note 01 + RSIC outcome maturity. Governance: UAITS Paradigm `online_ff` enum decision.
**Pre-existing draft:** `07-ff-shallow-heads.md` (383 lines).
**Note:** 17a and 17b are independent; can run in parallel on separate branches.

### Tier 5 — Action-Conditioned Prediction

#### Phase 18 / Note 08 — HTM Sequence Memory (Predictive Jiminy)

**What.** Variable-order Markov model over closed-vocab developer event stream (commits, test runs, reviews, etc); predicts next likely action. Stored as Neo4j subgraph. One-shot learning + 30-day decay.
**Sub-decoupling (RD-6):** Phase 5b — use sequence-memory predictions as Phase 11 RL reward shaping, decoupled from Jiminy UX gates. Operational quick-win.

**Effort:** Medium-large (10.5 dev-days + 6-8 weeks calendar including soak).
**Hard prereq:** Note 05 (fingerprints for state representation).
**Risk:** Empirical — unknown whether developer event streams have enough sequence structure. Hard merge gate at top-3 accuracy ≥0.4.

### Tier 6 — Capstone (CONDITIONAL on collaboration)

#### Phase 19 / Note 09 — Active-Inference Unification

**What.** Mathematical unification of MDEMG's three decision systems (Jiminy, RSIC, Consulting) under a single variational free-energy objective. Discrete-state FEP synthesis (Da Costa et al.). Bayesian posterior, precision-weighted prediction errors, action selection via expected free energy.

**Effort:** Program-scope (9-12 months minimum). Not a sprint.
**Critical prereq:** **FEP co-implementer recruited** (Q-DM-1 from research-eval; "single most leveraged action across the entire collection"). Doc explicitly states: "Until recruitment succeeds or fails, Note 09 should remain in planning-document state."
**Conditional second prereq:** Conflicting-guidance frequency tracking (Action 1 / RD-8) running for ≥3 months — determines whether unification is empirically justified.

**Recommendation:** Don't plan a sprint. Treat as research collaboration target. Begin Action 1 (free) immediately so we have data when/if Note 09 starts.

---

## Workstream C — Cross-Cutting (Always-On)

Five items the research-eval flags as immediate-value, often free or nearly free. None requires a sprint slot — these run alongside whatever's executing.

### Action 1 (free) — Conflicting-Guidance Tracking
Add a detector to Jiminy/RSIC/Consulting that logs divergent recommendations to TSDB. 3-month observation window. Determines whether Note 09 (capstone, 9-12 months) is justified empirically. **Cost: <1 day. Start immediately.**

### Action 2 — UAITS Paradigm Governance Discussion
The `online_ff` paradigm (used by Note 07) requires governance settling before merge. UAITS is a 4-paradigm framework (SFT/DPO/RAFT/curriculum); adding a 5th needs RFC + spec extension. **Cost: <1 day to schedule. Start during Workstream A.**

### Action 3 — Pre-Note-03 Schema Compatibility Check
Before Note 03's `:PREDICTS` schema mutation lands (V0025+ migration), 1-day review against discrete-FEP formalism (RD-2). Catches design mismatches before they cement. **Cost: 1 day. During Phase 17a planning.**

### Action 5 — Collaboration Brief: Object-Centric Slot Gap (RD-7)
Per research-eval: "the most actionable input for the collaboration brief." MDEMG's MemoryNodes are entity-shaped but lack explicit slot structure. Numenta and Meta both work in this area. Foreground in collaboration outreach. **Cost: framing in brief; outreach is asynchronous.**

### Action 6 — Collaboration Brief: FEP Co-Implementer (Q-DM-1)
Per research-eval: "single most leveraged action across the entire collection." Without an FEP-specialist co-implementer, Note 09 stays in planning-document state. Begin recruitment now (months ahead of when Note 09 would start).

---

## Recommendation — Next 3 Sprints

### Sprint 1 (Now): **Phase 11.6.x Operational Hygiene**

Bundle 11.6.2 + 11.6.3 + 11.6.4 + 11.6.5 into a single ~1-week sprint. Closes the production cutover follow-ups before any larger work starts. Sets up Workstream A for `done` so we're not carrying tech debt into the research extensions.

Plus (cheap, parallel):
- Action 1: Conflicting-guidance tracker (1 day, runs in background for months)
- Action 2: UAITS governance discussion scheduled

### Sprint 2: **Phase 12 — UVTS Activation** ✅ EXECUTED (2026-05-01)

3-week sprint, foundational. Unblocks all 8 research extensions. Per the doc this is "the highest-leverage single sprint the project could run right now."

**Outcome:** Shipped across 5 incremental commits (`0a99f29`, `4b27717`, `d6601b8`, `d10c1a5`, sprint-close). 5 latent runner defects fixed. V0016 TSDB migration + runner persistence + A/B harness + lnl_demo `ab_mode` extension + polysemy spec (partial-authoring) + ConflictTracker production wiring (Workstream C #1, deferred from 11.6.x). Live-testing formalized as required Tier-3 (CMS observation `p5iv8effstxk5ujd1fa2qfy8`). Phase 13 (Column-Voting) and Workstream C ConflictTracker observation window now unblocked. Mid-sprint discovery: MLX server fragility under sustained load — Phase 11.6.3 (MLX watchdog) is now the immediate next sprint, ahead of Phase 13. Doc: [`phase_12_uvts_post.md`](post-ft-lora/phase_12_uvts_post.md). Plan: [`sprint_plan_phase_12_uvts.md`](post-ft-lora/sprint_plan_phase_12_uvts.md).

### Sprint 3: **Phase 13 — Note 04 Column-Voting Retrieval** (gated on Phase 11.6.3 MLX Watchdog)

3-week sprint. Lowest-risk + highest-coverage research extension. Yields per-query `consensus_strength` signal that downstream extensions consume. Parallel-safe with any FT-LORA follow-up work.

After Sprint 3, sequence either becomes (a) Notes 05+06 in parallel (continue Tier 2), or (b) Note 01 narrative formalization (early Tier 3 prep) depending on operational signal.

---

## Open Decisions

1. **Workstream A scope for Sprint 1** — bundle all 4 hygiene items (11.6.2-5) or split? Recommendation: bundle. They're small, related, and the user's been investing in the cutover for 3 days now — closing the loop fully has psychological value.

2. **Workstream B branch strategy** — separate `reh3376_dev02` for research extensions per the doc's parallel-safety claim, or stay on `reh3376_dev01`? Recommendation: stay on `dev01` until UVTS-Activation lands. After that, retrieval cluster work goes to `dev02` (matches the doc's recommendation).

3. **Note 09 capstone status** — file as "deferred until collaborator recruited" or write a sprint plan now anticipating future execution? Recommendation: defer. Action 6 (recruit FEP co-implementer) is the right input. Writing a 12-section sprint plan now prematurely freezes design choices.

4. **Conflicting-guidance tracking start** — is Action 1 worth doing during Sprint 1 (Workstream A) given it's just instrumentation, or hold for a focused tracking-instrumentation sprint? Recommendation: start during Sprint 1. It's 1 day of code; the value comes from the 3-month data window, so earlier start = earlier data.

5. **Collaboration brief outreach** — is the brief in a state to publish / share with researchers, or does it need another pass? (Out of my visibility.) Recommendation: separate decision; doesn't gate any of the above.
