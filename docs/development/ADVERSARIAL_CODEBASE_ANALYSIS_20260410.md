# Adversarial Codebase Analysis — 2026-04-10

**Branch:** `reh3376_dev01` | **Tag:** v0.7.0+ | **Method:** Deep-dive with systematic refutation

This analysis was produced by first identifying risks, bugs, and optimization opportunities
across the MDEMG codebase, then launching adversarial investigations that attempted to
**disprove** each finding. Only claims that survived refutation are presented as confirmed.
Corrected and invalidated claims are documented explicitly to prevent confirmation bias.

**Reference documents:**
- `VISION.md` — architectural philosophy and long-term goals
- `docs/development/AUTORESEARCH_INTEGRATION_ANALYSIS.md` — 28 gaps across RSIC/CMS/Jiminy
- `neural/training/` — LLM training pipeline (MLX LoRA, GRPO, Qwen3-30B-A3B target)

---

## Table of Contents

1. [Confirmed Bugs](#confirmed-bugs)
2. [Claims Corrected by Adversarial Review](#claims-corrected-by-adversarial-review)
3. [Optimization Strategies — Validated vs Invalidated](#optimization-strategies)
4. [Priority Roadmap](#priority-roadmap)
5. [AutoResearch Gap Status](#autoresearch-gap-status)
6. [Methodology Notes](#methodology-notes)

---

## Confirmed Bugs

Findings that survived all refutation attempts, organized by severity.

### Critical

#### BUG-C1: Docker Healthcheck Port Mismatch

- **Location:** `docker-compose.yml:122`
- **Description:** Healthcheck uses `${MDEMG_PORT:-9999}` inside the container, but the
  Go application listens on hardcoded `:9999`. When `mdemg init` writes a custom port to
  `.env`, the healthcheck curls the wrong port and the container is marked unhealthy.
- **Impact:** Dynamic port assignment (a supported `mdemg init` feature) breaks container
  health monitoring. Orchestrators (Docker, Kubernetes) may restart healthy containers.
- **Fix:** Either hardcode `:9999` in the healthcheck (matching the app) or make the Go
  server respect `MDEMG_PORT` at listen time. The former is simpler; the latter is more
  correct long-term.

#### BUG-C2: CI Coverage Never Generated

- **Location:** `.github/workflows/ci.yml:109`
- **Description:** The `go test` command is missing `-coverprofile=coverage.out`. The
  subsequent Codecov upload step runs but uploads nothing — it is a no-op.
- **Impact:** Zero coverage tracking. Regressions in test coverage are invisible.
- **Fix:** Add `-coverprofile=coverage.out` to the test command.

#### BUG-C3: train_ft.py --num-layers / --lora-rank Swap

- **Location:** `neural/training/train_ft.py:258`
- **Description:** The `resolved_rank` value (LoRA rank, e.g. 16) is passed to the
  `--num-layers` flag, which controls how many model layers are loaded/truncated. The
  `--lora-rank` flag is entirely absent from the constructed command.
- **Impact:** Every training run produces a model with the wrong number of layers AND
  default LoRA rank. The resulting adapter is architecturally incorrect.
- **Fix:** Pass `resolved_rank` to `--lora-rank` and use a separate layer count
  parameter (or omit `--num-layers` to use the full model).

### High

#### BUG-H1: Model Default Three-Way Mismatch

- **Location:** `internal/config/config.go:46` (struct comment), `config.go:1291`
  (`FromEnv()`), `docker-compose.yml` (env default)
- **Description:** Three different default model values exist:
  - Struct comment: `gpt-4.1`
  - `FromEnv()` fallback: `gpt-5.4`
  - Docker compose default: `gpt-4.1`
- **Impact:** Silent config divergence between Docker and native deployments. Operators
  may not realize which model they are actually using.
- **Fix:** Align all three to a single canonical default. Update the struct comment to
  match `FromEnv()`.

#### BUG-H2: TSDB Environment Variable Name Mismatch

- **Location:** `docker-compose.yml:35` vs `internal/config/config.go:3186`
- **Description:** Compose sets `TSDB_DBNAME`, but the Go config reads `TSDB_DATABASE`.
  Both default to `mdemg_metrics`, so it works out of the box — but any operator who
  customizes `TSDB_DBNAME` in their `.env` will find it silently ignored.
- **Impact:** Breaks on customized database names. Difficult to debug because the default
  masks the issue.
- **Fix:** Align the env var name. Add `TSDB_DBNAME` as an alias in `FromEnv()` or
  update compose to use `TSDB_DATABASE`.

#### BUG-H3: Activation Seeding Ignores RRF Score

- **Location:** `internal/retrieval/activation.go:369-398`
- **Description:** `SpreadingActivationWithAttention` seeds activation energy from
  `max(VectorSim, BM25Score)` but ignores `RRFScore`. The entire retrieval pipeline
  computes a fused RRF score, then the activation stage discards it.
- **Impact:** RRF-boosted results (high on both vector and BM25) get no additional
  advantage in spreading activation. The fusion signal is wasted.
- **Fix:** Seed from `max(VectorSim, BM25Score, RRFScore)` — one-line change.

#### BUG-H4: Hollow Evaluation Metrics

- **Location:** `neural/training/evaluate_ft.py:115-123`
- **Description:** The `coherence`, `coverage`, `specificity`, and `follow_rate` metric
  functions all delegate to `check_non_empty()`, which unconditionally returns `1.0` for
  any non-empty string.
- **Impact:** The regression gate (`regression_gate.py`) compares these metrics across
  versions — but since they always return 1.0, the gate never catches quality degradation.
  Training quality is unmonitored.
- **Fix:** Implement real metrics. At minimum: ROUGE/BERTScore for coherence, query-term
  coverage ratio, specificity as inverse of generic-phrase frequency, follow_rate from
  Jiminy outcome data.

### Medium

#### BUG-M1: Circuit Breaker Repeated Alert Firing

- **Location:** `internal/llmclient/client.go:293-307`
- **Description:** `trackResult` fires the failure callback on every request after the
  consecutive failure threshold is exceeded. There is no "tripped" state guard to fire
  the alert once and suppress until recovery.
- **Impact:** Alert storm during sustained LLM outages. Every failed request generates
  a new alert.
- **Fix:** Add a `tripped bool` field. Fire the callback on the transition to tripped
  state only. Reset on success.

#### BUG-M2: 502 Not in Retry Set

- **Location:** `internal/llmclient/client.go` (`shouldRetry`)
- **Description:** `shouldRetry` returns true for 429 (rate limit) and 503 (service
  unavailable) but not 502 (bad gateway). Upstream proxy errors cause hard failures.
- **Impact:** Transient proxy errors (common with cloud LLM providers behind load
  balancers) are not retried, causing unnecessary task failures.
- **Fix:** Add 502 to the retry set.

#### BUG-M3: Jiminy Exact-String Deduplication

- **Location:** `internal/jiminy/service.go:1117`
- **Description:** `deduplicateItems` uses exact string comparison. Two guidance items
  that differ by a single word or punctuation are treated as unique.
- **Impact:** Near-duplicate guidance clutters the output, wasting context window tokens.
  Relates to AutoResearch gap J2.
- **Fix:** Use cosine similarity on embeddings with a configurable threshold (e.g. 0.85).
  The embedding infrastructure already exists in the retrieval layer.

#### BUG-M4: Jiminy Corrections Have No Temporal Weighting

- **Location:** `internal/jiminy/corrections.go:31-38`
- **Description:** Correction retrieval ranks by pure cosine similarity with no temporal
  decay. A correction from 6 months ago ranks equally with one from yesterday.
- **Impact:** Stale or superseded corrections may outrank recent, more relevant ones.
  Relates to AutoResearch gap J4.
- **Fix:** Apply a time-decay multiplier to the similarity score. The decay infrastructure
  already exists in `internal/retrieval/`.

#### BUG-M5: Dead Config Field `ScoringRho`

- **Location:** `internal/config/config.go:110`
- **Description:** `ScoringRho` is defined in the config struct but never read.
  `getDecayRate` falls back to `ScoringRhoL0`, not `ScoringRho`.
- **Impact:** Confusing for operators who set `SCORING_RHO` expecting it to do something.
- **Fix:** Remove the field or wire it as intended.

#### BUG-M6: Unbounded `lastTickets` Map (Memory Leak)

- **Location:** `internal/jiminy/ticket.go:19`
- **Description:** The `lastTickets` map stores J17 session tickets keyed by session ID
  and grows without bound. No eviction or TTL.
- **Impact:** Slow memory leak. Only significant for long-running server instances with
  many unique sessions (weeks+).
- **Fix:** Add a max-size LRU eviction or periodic sweep.

### Low

#### BUG-L1: Evaluator Cache Dead Code

- **Location:** `internal/jiminy/evaluator.go:289-329`
- **Description:** `evalCacheGet` and `evalCachePut` are defined and have tests, but
  `llmEvaluate()` never calls them.
- **Impact:** Dead code. The LLM evaluator makes redundant calls for identical inputs.
- **Fix:** Either wire the cache into `llmEvaluate()` or remove the dead code.

#### BUG-L2: Trust Store Dead Goroutine

- **Location:** `internal/jiminy/trust_store.go:50-62`
- **Description:** `Start()` launches a goroutine with a 30-second ticker that executes
  an empty loop body. It consumes a goroutine and ticker resources for no purpose.
- **Impact:** Negligible resource waste; code confusion.
- **Fix:** Remove the goroutine or implement the intended periodic trust recalculation.

---

## Claims Corrected by Adversarial Review

These findings were initially reported but partially or fully refuted upon deeper investigation.

| Original Claim | Correction | Evidence |
|---|---|---|
| "LLM classification half-integrated — scoring gates ignore it" | **Partially refuted.** LLM classification DOES influence scoring indirectly through RRF fusion weights. `computeQueryGates` uses regex, but the broader pipeline uses the LLM hint for weight selection. The claim overstated the gap. | `internal/retrieval/scoring.go` RRF weight path |
| "Edge.UpdatedAt silently discarded" | **Real but zero impact.** `_ = upd` confirmed at `service.go:1187,1333`, but `Edge.UpdatedAt` is a dead field with zero downstream consumers. No data loss occurs. | Grep for `UpdatedAt` across codebase |
| "ndcg_delta reward hardcoded to 0.7" | **Partially refuted.** `reward_functions.py:345` has differentiated format-validation paths. However, both *valid* result paths do converge on 0.7, so the differentiation only affects invalid inputs. | `neural/training/reward_functions.py:345` |
| "No pipeline automation exists" | **Overstated.** CLI commands (`train_ft`, `evaluate_ft`, `regression_gate`, etc.) and RSIC task-type awareness exist. The missing piece is an end-to-end orchestrator, not all automation. | CLI command inventory, RSIC task types |
| "UxTS soft-fail in CI is a gap" | **Intentional by design.** CI step names are explicitly labeled `(soft-fail)`. This is a deliberate choice to avoid blocking merges on non-critical test frameworks during early adoption. | `.github/workflows/ci.yml` step names |

---

## Optimization Strategies

### Invalidated (Do NOT Pursue)

These strategies were proposed in the initial analysis but failed adversarial challenge.

#### ~~Strategy: Self-Optimizing Scoring Weights~~

- **Why invalidated:** 120 benchmark questions is far too small a dataset to tune 25+
  interdependent scoring parameters. Parameter dependencies (e.g., BM25 weight affects
  RRF which affects activation seeding) make independent mutation unreliable. This would
  produce overfit parameters that degrade on real queries.
- **Prerequisite to revisit:** Benchmark scale of 500+ diverse questions with held-out
  validation set. Bayesian optimization (not grid search) to handle dependencies.

#### ~~Strategy: Wire Signal Learner to Production Decisions~~

- **Why invalidated:** Zero signal data exists in Neo4j. `GetStrength` is never called
  in production code paths. The signal learner has a severe cold-start problem — there
  is no data to learn from, and no production code path generating signal observations.
- **Prerequisite to revisit:** First, instrument production code to emit signal
  observations. Accumulate at least 2 weeks of data. Then evaluate whether the learned
  weights are meaningful before wiring them to decisions.

#### ~~Strategy: Add Intent Translation for Vector Search~~

- **Why invalidated:** **Already implemented.** `internal/api/handlers.go:444-454`
  replaces `req.QueryText` with LLM-translated text before embedding when
  `INTENT_ENABLED=true`. The feature exists but is disabled by default.
- **Action instead:** Investigate why `INTENT_ENABLED` defaults to `false`. Check git
  history for benchmark comparisons. It may have been tested and found to hurt vector
  recall, or it may simply have never been evaluated.

#### ~~Strategy: Incremental Consolidation (Replace Full Re-Cluster)~~

- **Why invalidated:** The hidden layer uses KMeans, which is O(n * k * iterations) —
  not O(n^2). Daily full re-cluster with explicit comment "we re-cluster the full set"
  (`internal/hidden/service.go:337`) is a deliberate design choice for global coherence.
  At current scale, consolidation completes in under 10 minutes.
- **Prerequisite to revisit:** When observation count exceeds 100K and consolidation
  duration exceeds 30 minutes.

### Validated (Pursue)

These strategies survived adversarial challenge and represent genuine improvements.

#### Strategy V1: Pass LLM Classification Hint to Scoring Gates

- **Location:** `internal/retrieval/scoring.go:528` (`computeQueryGates`)
- **Current state:** `computeQueryGates` uses regex pattern matching to determine query
  type. Meanwhile, the LLM classification (when `QUERY_CLASSIFY_ENABLED=true`) has
  already computed a richer query type upstream.
- **Proposed change:** Thread the already-computed `queryType` from LLM classification
  into `computeQueryGates` as a parameter. Fall back to regex when classification is
  disabled or unavailable.
- **Effort:** Low. The classification result exists; the gates exist. This is plumbing.
- **Risk:** Low. Regex fallback preserves current behavior when LLM classification is off.
- **Expected impact:** More accurate gate activation for ambiguous queries where regex
  heuristics fail (e.g., questions phrased as statements).

#### Strategy V2: Fix Activation Seeding to Include RRF Score

- **Location:** `internal/retrieval/activation.go:369-398`
- **Current state:** `SpreadingActivationWithAttention` seeds energy from
  `max(VectorSim, BM25Score)`, ignoring the fused `RRFScore`.
- **Proposed change:** `max(VectorSim, BM25Score, RRFScore)` — one-line change.
- **Effort:** Minimal.
- **Risk:** Low. RRFScore is always >= max of its components (by RRF formula design),
  so this effectively seeds from RRFScore when available. Verify with benchmarks.
- **Expected impact:** Results that score well on both vector and BM25 (the strongest
  candidates) get proportionally higher activation energy for spreading.

#### Strategy V3: Training Pipeline Hardening

Three sub-tasks, all prerequisites before any valid training run:

**V3a: Fix `--num-layers` / `--lora-rank` argument swap**
- **Location:** `neural/training/train_ft.py:258`
- **Change:** Pass `resolved_rank` to `--lora-rank`, use proper layer count for
  `--num-layers` (or omit to use full model).
- **Effort:** Minimal code change, critical correctness fix.

**V3b: Implement real evaluation metrics**
- **Location:** `neural/training/evaluate_ft.py:115-123`
- **Change:** Replace `check_non_empty()` stubs with:
  - `coherence`: ROUGE-L or BERTScore against reference answers
  - `coverage`: query-term recall ratio
  - `specificity`: inverse generic-phrase frequency (TF-IDF based)
  - `follow_rate`: Jiminy outcome data from TSDB if available
- **Effort:** Medium. Requires choosing metric implementations and writing tests.

**V3c: Validate regression gate with real metrics**
- **Location:** `neural/training/regression_gate.py`
- **Change:** Once V3b is done, verify that the gate correctly blocks regressions.
  Run a synthetic test with an intentionally degraded model.
- **Effort:** Low once V3b is complete.

#### Strategy V4: Investigate INTENT_ENABLED=false History

- **Location:** `internal/api/handlers.go:444-454`, `.env` defaults
- **Action:** `git log --all -S 'INTENT_ENABLED'` to find when it was set to false and
  whether benchmark data accompanied the decision. Check if intent translation was
  tested against the 120-question benchmark.
- **Possible outcomes:**
  - If tested and found harmful: document why, consider improving translation quality
  - If never evaluated: run benchmark with/without, measure recall@10 delta
  - If disabled for cost: note LLM call cost and evaluate batch-mode translation
- **Effort:** Investigation only. No code change until findings are clear.

#### Strategy V5: Fix Critical Bugs (BUG-C1, C2, C3)

All three critical bugs are straightforward fixes:

- **C1 (Docker healthcheck):** Hardcode `:9999` in healthcheck or make app respect
  `MDEMG_PORT`. ~5 min fix.
- **C2 (CI coverage):** Add `-coverprofile=coverage.out` to test command. ~2 min fix.
- **C3 (train_ft.py):** Covered by V3a above.

---

## Priority Roadmap

### Phase 1: Immediate (Blocks Everything Else)

| Item | Type | Effort | Tracking |
|------|------|--------|----------|
| Fix `train_ft.py` `--num-layers`/`--lora-rank` swap (BUG-C3 / V3a) | Bug fix | 15 min | — |
| Add `-coverprofile` to CI (BUG-C2) | Bug fix | 5 min | — |
| Fix Docker healthcheck port (BUG-C1) | Bug fix | 15 min | — |

### Phase 2: Next Sprint

| Item | Type | Effort | Tracking |
|------|------|--------|----------|
| Pass LLM classification to scoring gates (V1) | Enhancement | 2-4 hrs | — |
| Fix activation seeding to include RRFScore (V2) | Enhancement | 30 min | — |
| Implement real evaluation metrics (V3b) | Enhancement | 4-8 hrs | — |
| Investigate `INTENT_ENABLED=false` history (V4) | Research | 1-2 hrs | — |
| Align model default 3-way mismatch (BUG-H1) | Bug fix | 30 min | — |
| Align TSDB env var names (BUG-H2) | Bug fix | 30 min | — |

### Phase 3: Backlog

| Item | Type | Effort | Tracking |
|------|------|--------|----------|
| Circuit breaker tripped-state guard (BUG-M1) | Bug fix | 1 hr | — |
| Add 502 to retry set (BUG-M2) | Bug fix | 15 min | — |
| Jiminy semantic dedup (BUG-M3 / gap J2) | Enhancement | 2-4 hrs | — |
| Jiminy temporal weighting (BUG-M4 / gap J4) | Enhancement | 2-4 hrs | — |
| Remove dead `ScoringRho` config (BUG-M5) | Cleanup | 15 min | — |
| Bound `lastTickets` map (BUG-M6) | Bug fix | 30 min | — |
| Wire or remove evaluator cache (BUG-L1) | Cleanup | 30 min | — |
| Remove trust store dead goroutine (BUG-L2) | Cleanup | 15 min | — |
| Validate regression gate end-to-end (V3c) | Testing | 2 hrs | — |

---

## AutoResearch Gap Status

Cross-reference with `docs/development/AUTORESEARCH_INTEGRATION_ANALYSIS.md` (28 gaps).

| Gap | Status After This Analysis |
|-----|---------------------------|
| R1 (calibration after) | **CLOSED** — `MetricsAfter` populated from post-assessment |
| R2 (success criteria) | **CLOSED** — evaluated in calibration |
| R3 (reflection depth) | **CLOSED** — 31 rule-based patterns + LLM reflection wired |
| R4 (signal learner) | **OPEN but low priority** — zero data exists, cold-start problem |
| R5 (task timeout) | **CLOSED** — per-task `context.WithTimeout` |
| R6 (rollback) | **CLOSED** — calibration suppression + auto-rollback |
| J2 (semantic dedup) | **OPEN** — exact string match only (BUG-M3) |
| J4 (temporal weighting) | **OPEN** — no time decay in corrections (BUG-M4) |

Remaining gaps (R7-R12, C1-C9, J1, J3, J5-J7) were not directly investigated in this
analysis. Refer to the AutoResearch document for their descriptions.

---

## Methodology Notes

### Approach

1. **Initial sweep:** Five parallel research agents scanned the codebase for risks,
   bugs, and optimization opportunities across: retrieval pipeline, training pipeline,
   Jiminy/RSIC, configuration/deployment, and architecture alignment with VISION.md.

2. **Adversarial challenge:** Five additional agents were tasked with **disproving**
   each finding. Each agent was given the specific claim and told to find evidence that
   contradicts it.

3. **Reconciliation:** Claims that survived refutation are marked "confirmed." Claims
   where refuting evidence was found are documented in the corrections table with the
   specific evidence that changed the assessment.

### Why This Matters

Standard code analysis has a strong confirmation bias: once a pattern looks like a bug,
investigators seek confirming evidence and stop. The adversarial step forces examination
of the null hypothesis — "what if this is NOT a bug?" — which caught:

- An already-implemented feature (intent translation) that would have been re-built
- An optimization (incremental consolidation) that would have added complexity for
  negligible gain at current scale
- A scoring gap that was less severe than initially assessed (LLM classification
  does influence scoring, just not through the exact path first examined)
- A dead field (Edge.UpdatedAt) that appeared to be data loss but has zero consumers

---

## Remediation Status

All 14 confirmed bugs were remediated in the ACA-BFC sprint (2026-04-10). H3 was already fixed prior to analysis.

| Bug | Severity | Status | Commit Epic |
|-----|----------|--------|-------------|
| C1 | Critical | **Fixed** | E1: Infra |
| C2 | Critical | **Fixed** | E1: Infra |
| C3 | Critical | **Fixed** | E2: Training |
| H1 | High | **Fixed** | E1: Infra |
| H2 | High | **Fixed** | E1: Infra |
| H3 | High | **Already fixed** (pre-analysis) | N/A |
| H4 | High | **Fixed** | E2: Training |
| M1 | Medium | **Fixed** | E3: LLM Client |
| M2 | Medium | **Fixed** | E3: LLM Client |
| M3 | Medium | **Fixed** | E4: Jiminy |
| M4 | Medium | **Fixed** | E4: Jiminy |
| M5 | Medium | **Fixed** | E1: Infra |
| M6 | Medium | **Fixed** | E4: Jiminy |
| L1 | Low | **Fixed** | E4: Jiminy |
| L2 | Low | **Fixed** | E4: Jiminy |

**New configuration introduced:**
- `JIMINY_DEDUP_SIMILARITY_THRESHOLD` (default: 0.85) — M3
- `JIMINY_CORRECTION_DECAY_RATE` (default: 0.01) — M4
- `J17_TICKET_CACHE_SIZE` (default: 1000) — M6

**Configuration removed:**
- `SCORING_RHO` — M5 (dead field; suffixed variants `SCORING_RHO_L0` etc. unaffected)

---

*Generated: 2026-04-10 | Branch: reh3376_dev01 | Analysis method: adversarial refutation*
*Remediation: 2026-04-10 | Sprint: ACA-BFC | All 14 bugs fixed*
