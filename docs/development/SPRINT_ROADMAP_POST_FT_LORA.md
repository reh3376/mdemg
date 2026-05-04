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

### Sprint 2.5: **Phase 11.6.3 — MLX Watchdog (Operational Hygiene #2)** ✅ EXECUTED (2026-04-30)

**Outcome:** Auto-restart + fast-fail + degraded-mode wired end-to-end. New `internal/mlxprobe` package with state machine + supervisor integration, llmclient gate at `client.go:471` returning `ErrMLXDown` sentinel, launchd plist `com.mdemg.mlx-server.plist` with `KeepAlive.SuccessfulExit=false` + `ThrottleInterval=60s`, `mdemg watchdog status` CLI (parses `/metrics` + `launchctl print` + alert file), 3 Prometheus metrics, 4 config knobs. Tier 1 + Tier 2 fully green; Tier 3 partial (CLI verified live; destructive `kill -9 mlx` smokes deferred to operator-led validation per safe-execution policy). The retry-storm pattern observed in Phase 12 (1642% CPU when 16 LLM call sites independently retry on a dead mlx) is eliminated at the source. **Phase 13 unblocked**: sustained live A/B testing no longer risks 30-minute storms + manual recovery on every Metal-OOM crash. Doc: [`phase_11_6_3_post.md`](ft-lora/phase_11_6_3_post.md). Plan: [`sprint_plan_phase_11_6_3.md`](ft-lora/sprint_plan_phase_11_6_3.md).

### Sprint 3: **Phase 13 — Note 04 Column-Voting Retrieval** ⚠️ EXECUTED 2026-05-02 — A/B FAILED, default stays false

**Outcome:** Infrastructure ships; A/B merge gate **failed** on quick profile (mean −0.038, 2 catastrophic per-question regressions to 0.000). 4 columns (Embedding + BM25 + Graph + Structural; Temporal + RoleScoped deferred per Epic 0 audit) + RRF aggregator + cache scorer-version namespacing + V0017 `retrieval_audit` hypertable + 3 Prometheus metrics — all live and lint-clean. `RetrievalColumnVotingEnabled` default stays `false`; the equal-weights configuration regresses retrieval quality on hard-symbol queries (q 69 + q hard_sym_4 both went 0.354/0.350 → 0.000). 13 of 16 questions produced bit-identical scores between linear and RRF — the scorers converge on most queries; divergence concentrates on the 3 questions that need column re-weighting. Doc: [`phase_13_column_voting_post.md`](post-ft-lora/phase_13_column_voting_post.md). Plan: [`sprint_plan_phase_13_column_voting.md`](post-ft-lora/sprint_plan_phase_13_column_voting.md). Verdict: [`phase_13_ab_verdict_quick.json`](post-ft-lora/phase_13_ab_verdict_quick.json).

**Mid-sprint side-quest: Hotfix 11.6.3.1 — MLX always-on policy** (commits `fc0961e`, `f3d106e`, `43e671a`). Per operator policy "MLX server should NEVER be down when mdemg is running" (memory: `feedback_mlx_required_when_mdemg_running.md`). Flipped `MLX_WATCHDOG_ENABLED` default `false → true`; made `com.mdemg.mlx-server` plist Required (not Optional); added startup precondition probe in `internal/cli/preflight_mlx.go` that refuses startup if mlx unreachable; corrected `.env` `LLM_ENDPOINT` from broken `host.docker.internal:8101` (Phase 11.6.2 finding finally addressed) to `127.0.0.1:8101`; fixed 3 metric double-prefix bugs + 1 watchdog CLI endpoint-path bug. Live-verified: SIGSTOP'd mlx, watchdog detected within 15s, fast-fail gate fired (3 caller_task counters incremented), retrieve completed in 2.7s instead of ~9 min, alerts dispatched with proper cooldown, SIGCONT recovered to up within 10s.

### Sprint 3.4: **Phase 13.5 — MLX Server Stability** ✅ EXECUTED (2026-05-03) — F1 (llama.cpp) wins

**Outcome:** Production LLM substrate migrated from `mlx_lm.server` to `llama-server` (llama.cpp b9000+ with GGUF Q5_K_M). Decision was data-cited per operator constraint "follow the data, no opinion required." Research-first sprint with 4 parallel research streams (crash forensics, mlx-lm + Apple Metal community evidence, alternatives matrix, internal LLM call profile), synthesis-driven disqualification of mlx_lm.server (maintainer says "not for production") + Ollama (broken on M5/26.3.x) + LM Studio (closed-source operability risk), then empirical bake-off between F1 (llama.cpp) and F2 (MLC-LLM). F1 won every measured dimension: stability tied (both 0 crashes vs mlx's ~17/4h); F1 latency 1.6× faster (3.10s avg vs 5.02s); F1 UVTS quality at perfect parity (0.396 = 0.396) vs F2 −0.006; F1 4× larger community + MIT licensed; F1 GGUF format runs in many other tools. Production cutover landed: new `com.mdemg.llama-server.plist`, GGUF model at `.local-models/mdemg-llm-v1-gguf/`, `.env` LLM_ENDPOINT flipped to 8102, watchdog re-enabled (now probes llama-server endpoint — same code, generic by design), old mlx-server plist renamed `.disabled-phase13_5` for emergency rollback. Crash rate observed went from ~17/4h to **0/160 min × 301 calls** (clean isolated stress). Doc: [`phase_13_5_bakeoff_results.md`](post-ft-lora/phase_13_5_bakeoff_results.md). Synthesis: [`phase_13_5_mlx_research_synthesis.md`](post-ft-lora/phase_13_5_mlx_research_synthesis.md). Plan: [`sprint_plan_phase_13_5_mlx_stability.md`](post-ft-lora/sprint_plan_phase_13_5_mlx_stability.md).

### Sprint 3.5: **Phase 13.1 — Column-weight ablation** ✅ EXECUTED (2026-05-03) — embedding-heavy preset wins, default flipped

**Outcome:** Default `RetrievalColumnVotingEnabled` flipped `false → true`. Embedding-heavy preset (0.50/0.20/0.15/0.15, hops=2) wins both 16q quick A/B (mean +0.025, 0 regressions, 4 improvements) and full 120q A/B (mean +0.023, 30 improvements, 2 boundary regressions in `business_logic_constraints`). Phase 13's catastrophic regressions (q 69 + q hard_sym_4 → 0.000) are eliminated — q 69 now returns `secretsManager.module` direct hit. Forensic diagnosis confirmed H1 (equal-weights pathology with Graph+Structural over-voting on structurally-connected code, crowding out Embedding+BM25 on precise-symbol queries). 4 weight env vars added: `RETRIEVAL_COLUMN_WEIGHT_{EMBEDDING,BM25,GRAPH,STRUCTURAL}`. Cache namespace now includes weights so ablation sweeps automatically invalidate cache. Tier 3 LIVE verified on real `whk-wms` space, real OpenAI grader, real mdemg. Doc: [`phase_13_1_post.md`](post-ft-lora/phase_13_1_post.md). Plan: [`sprint_plan_phase_13_1_column_weight_ablation.md`](post-ft-lora/sprint_plan_phase_13_1_column_weight_ablation.md). Forensic diagnosis: [`phase_13_1_forensic_diagnosis.md`](post-ft-lora/phase_13_1_forensic_diagnosis.md). A/B verdict: `/tmp/phase13_1_full/ab-verdict.json` (sprint-local artifact).

### Sprint 3.5b: **Phase 13.2 — Per-category weight investigation** (queued, ~3 dev-days)

Phase 13.1 left `business_logic_constraints` as the only category with negative Δ (-0.005). The 2 boundary-case regressions are both in this category. Phase 13.2 should: (1) forensic-diagnose q `206` + q `283` to identify what makes business_logic_constraints different; (2) decide whether per-category weights are needed (would require ConsensusOpts API extension) or if a different global preset works better for this category specifically. Cost: ~$5 OpenAI. Refinement, not a blocker — Phase 13.1's improvement on every other category is shippable as-is.

### Sprint 3.6: **Phase 13.6 — Backend-agnostic naming cleanup** (~1–2 dev-days, follow-up)

Code-cleanup follow-up to Phase 13.5. Rename: `internal/cli/preflight_mlx.go` → `preflight_llm.go`; `MDEMG_ALLOW_NO_MLX` env var → `MDEMG_ALLOW_NO_LLM` (with backward-compat alias); `MLX_WATCHDOG_ENABLED` → `LLM_WATCHDOG_ENABLED`; `internal/mlxprobe` package → `internal/llmprobe`; `mdemg_mlx_*` Prometheus metrics → `mdemg_llm_endpoint_*`. The framework is functionally backend-agnostic post-Phase 13.5 but identifiers still reference mlx; renames bring identifiers in line with reality. Backward-compat aliases for env vars per `feedback_no_short_term_mlx_patches.md` discipline (no breaking changes without migration path).

### Sprint 4: **Phase 14 — Notes 05+06** (NARROW-CLOSED 2026-05-04 — split into 14, 14.1, 14.2)

**Phase 14 (this sprint, EXECUTED narrow)**: Note 06 sparse activation gate code shipped flag-off + Phase 13 Epic 6 V0017 audit-writer fix shipped + Epic 0 forensic doc + V0019 sparse_gate_metrics hypertable + Phase 11+ feature-doc backfill (5 new + 1 update). 16q quick PASSED at MIN=10/p95 (mean +0.019, 0 regressions); 120q full FAILED per-question (mean parity, 7 boundary regressions concentrated in `architecture_structure`). Note 05 deferred — Epic 0 found the spec's static 64/64/64/64 catalog bit allocation is wrong for `whk-wms` (0 symbols, 0 roles) → adaptive Builder redesign needed. See `phase_14_post.md`.

**Phase 14.1 (EXECUTED 2026-05-04, fail)**: Adaptive per-category sparse gate. Shipped per-category override config + dispatch + tests + comparator eps fix flag-off. 16q quick passed, 120q full failed (mean -0.009; q119 -0.45 + q333 -0.35 catastrophic). Diagnosis: per-category is the wrong abstraction; both regressing questions had 3 required_files in categories Phase 14.1's design didn't override. `SPARSE_GATE_CATEGORY_OVERRIDES` lands as opt-in infrastructure; eps fix in `uvts_ab_compare.py` ships globally. See `phase_14_1_post.md`.

**Phase 14.1.1 (queued)**: Complexity-based sparse gate override. Replaces per-category with required-files-count or complexity-tag dispatch. ~3 dev-days, ~$15-25 OpenAI. See `sprint_plan_phase_14_1_1_complexity_based_override.md`.

**Phase 14.2 (queued)**: Note 05 with adaptive catalog Builder. ~7 dev-days, ~$25 OpenAI. V0028 + V0029 + V0020 schema; fingerprint computation; context column; backfill CLI; combined A/B with Phase 14.1 gate. See `sprint_plan_phase_14_2_note_05_sparse_fingerprints.md`.

After Phase 14.x sequence completes, consider either (a) Note 01 narrative formalization (early Tier 3 prep) or (b) Phase 13.2 per-category weight tuning depending on operational signal.

---

## Open Decisions

1. **Workstream A scope for Sprint 1** — bundle all 4 hygiene items (11.6.2-5) or split? Recommendation: bundle. They're small, related, and the user's been investing in the cutover for 3 days now — closing the loop fully has psychological value.

2. **Workstream B branch strategy** — separate `reh3376_dev02` for research extensions per the doc's parallel-safety claim, or stay on `reh3376_dev01`? Recommendation: stay on `dev01` until UVTS-Activation lands. After that, retrieval cluster work goes to `dev02` (matches the doc's recommendation).

3. **Note 09 capstone status** — file as "deferred until collaborator recruited" or write a sprint plan now anticipating future execution? Recommendation: defer. Action 6 (recruit FEP co-implementer) is the right input. Writing a 12-section sprint plan now prematurely freezes design choices.

4. **Conflicting-guidance tracking start** — is Action 1 worth doing during Sprint 1 (Workstream A) given it's just instrumentation, or hold for a focused tracking-instrumentation sprint? Recommendation: start during Sprint 1. It's 1 day of code; the value comes from the 3-month data window, so earlier start = earlier data.

5. **Collaboration brief outreach** — is the brief in a state to publish / share with researchers, or does it need another pass? (Out of my visibility.) Recommendation: separate decision; doesn't gate any of the above.
