# AutoResearch Integration Analysis

**Date:** 2026-03-18
**Status:** Complete
**Scope:** Deep research analysis of [autoresearch](https://github.com/reh3376/autoresearch) concepts applicable to MDEMG's RSIC, CMS, and Jiminy systems
**Submodule:** `packaging/autoresearch/`

---

## Executive Summary

AutoResearch (by Andrej Karpathy) is a minimal autonomous ML research agent that iteratively modifies a training script, runs time-boxed experiments, and keeps or discards changes based on a single metric. Its deliberate simplicity reveals both patterns worth adopting and gaps that MDEMG already fills — while highlighting opportunities where autoresearch concepts could enhance MDEMG's recursive self-improvement capabilities.

This analysis identifies **28 actionable integration opportunities** across RSIC (12), CMS (9), and Jiminy (7), prioritized by impact and implementation effort. The highest-impact items are: closing RSIC's feedback loop (post-cycle re-assessment), adding LLM-powered reflection, implementing guidance effectiveness tracking in Jiminy, and adopting time-boxed experimentation with keep/discard semantics for RSIC actions.

---

## 1. AutoResearch Architecture Summary

### What It Is
A single-agent, single-GPU research loop that autonomously tunes neural network hyperparameters overnight. The agent modifies `train.py`, runs a 5-minute training experiment, measures `val_bpb` (validation bits-per-byte), and either keeps (git commit) or discards (git reset) the change.

### Key Files
| File | Role | Mutable by Agent? |
|------|------|--------------------|
| `prepare.py` | Fixed evaluation harness, data loading, tokenizer | No |
| `train.py` | GPT model, optimizer, hyperparameters | Yes (only file) |
| `program.md` | Agent instructions, experiment protocol, constraints | No (human edits) |
| `results.tsv` | Flat experiment log (commit, val_bpb, memory, status) | Yes (append-only) |

### Core Loop (`program.md` lines 94-112)
```
LOOP FOREVER:
1. Inspect git state
2. Modify train.py with experimental idea
3. git commit
4. Run: uv run train.py > run.log 2>&1
5. Extract val_bpb from log
6. If crash → read traceback, attempt fix
7. Log to results.tsv
8. If val_bpb improved → KEEP (advance branch)
9. If val_bpb equal/worse → DISCARD (git reset)
```

### Key Design Properties

| Property | AutoResearch | MDEMG Equivalent |
|----------|-------------|------------------|
| Experiment loop | Single metric keep/discard | RSIC 5-phase cycle |
| Knowledge store | Flat TSV + git history + context window | Neo4j graph + CMS |
| Evaluation | Immutable `evaluate_bpb()` function | UATS contract tests |
| Safety | Git reset on failure | SafetyValidator + SnapshotStore |
| Meta-learning | None (human edits program.md) | Phase 105 Global Meta-Learning |
| Time-boxing | 5-min wall clock per experiment | No per-action time budgets |
| Quality gate | val_bpb strictly lower = keep | No post-cycle metric delta check |
| Simplicity criterion | "Don't add ugly complexity for small gains" | None |

---

## 2. RSIC Gap Analysis

### Current State
23 files in `internal/ape/`, implementing a 5-phase cycle: Assess → Reflect → Plan → Execute → Validate/Calibrate. Orchestration policy handles trigger dedup, cooldown, overlap prevention. SafetyValidator gates destructive actions. SnapshotStore enables partial rollback.

### Gap R1: Post-Cycle Re-Assessment Not Performed
**Severity:** Critical
**Location:** `internal/ape/calibration.go:77`
**Current:** `MetricsAfter` is always `make(map[string]float64)` — empty. No re-assessment runs after task completion.
**AutoResearch parallel:** Every experiment ends with `grep val_bpb run.log` — the result is always measured.
**Recommendation:** Run `Assessor.Assess()` after task completion to populate `MetricsAfter`. Compute deltas for each metric. This closes the feedback loop — the single most impactful improvement.
**Effort:** Small (wire existing Assess into Validate flow)

### Gap R2: Success Criteria Never Evaluated
**Severity:** High
**Location:** `internal/ape/calibration.go:67-97`, `task_spec.go`
**Current:** `RSICTaskSpec.SuccessCriteria` and `Deliverables` are defined but validation checks only task status (completed vs failed), not criteria.
**AutoResearch parallel:** `val_bpb < previous` is the single success criterion, and it's always checked.
**Recommendation:** Evaluate `SuccessCriteria` against `MetricsAfter` deltas. A task that completed but didn't improve the target metric should be classified as "completed_no_improvement" rather than "success."
**Effort:** Small (add criteria check in Validate)

### Gap R3: Hardcoded Reflector Rules
**Severity:** High
**Location:** `internal/ape/self_reflect.go:24`
**Current:** 8 hardcoded threshold-based rules produce `ReflectionInsight` entries. Cannot detect novel patterns.
**AutoResearch parallel:** The agent uses LLM reasoning over results.tsv to generate hypotheses — no hardcoded rules.
**Recommendation:** Add an LLM-powered reflection path alongside the rule-based one. The LLM would receive the `SelfAssessmentReport` and recent `CycleOutcome` history, and could identify patterns the 8 rules miss (e.g., "pruning always fails on Tuesdays" or "consolidation improves retrieval quality but degrades edge entropy").
**Effort:** Medium (follows EmergenceNamer pattern: OpenAI/Ollama, circuit breaker, JSON grammar)

### Gap R4: Signal Learner Has No Effect
**Severity:** Medium
**Location:** `internal/ape/signal_learner.go:18`
**Current:** Signal strengths are tracked via Hebbian-style emission/response but `GetStrength()` is never called from any decision path. In-memory only — lost on restart.
**AutoResearch parallel:** N/A — autoresearch has no signal tracking.
**Recommendation:** Wire signal strength into the Planner. Actions with low historical signal strength should be deprioritized or require higher severity to trigger. Persist signal state via RSICStore.
**Effort:** Small (add GetStrength check in Plan, add RSICStore serialization)

### Gap R5: No Time-Boxing of Individual Actions
**Severity:** Medium
**Location:** `internal/ape/task_dispatch.go`
**Current:** Tasks have `TimeoutSec` in their spec but execution timeout is tier-level (micro=30s, meso=10min, macro=30min), not per-action.
**AutoResearch parallel:** Strict 5-minute wall clock per experiment. 10-minute hard kill. This prevents any single experiment from monopolizing resources.
**Recommendation:** Honor `RSICTaskSpec.TimeoutSec` per-action with individual context timeouts. Log timeout-killed actions distinctly from failures.
**Effort:** Small (wrap executor goroutines with individual timeouts)

### Gap R6: No Keep/Discard Semantics Based on Metrics
**Severity:** High
**Location:** `internal/ape/calibration.go`
**Current:** All completed tasks are treated as successes. There is no "did this actually help?" check.
**AutoResearch parallel:** The core loop is keep/discard based on metric improvement. Equal or worse = discard.
**Recommendation:** Combine with R1 (post-cycle re-assessment). If `MetricsAfter` shows no improvement or regression, trigger rollback for reversible actions. For irreversible actions (edge pruning), log as "completed_no_benefit" to inform future calibration.
**Effort:** Medium (requires R1 first, then rollback logic)

### Gap R7: Flat Calibration Confidence (No Time Weighting)
**Severity:** Medium
**Location:** `internal/ape/calibration.go:157`
**Current:** `GetCalibration()` returns `successes/total` — old outcomes weighted equally with recent ones.
**AutoResearch parallel:** N/A — autoresearch has no calibration, but the agent's context window naturally emphasizes recent results.
**Recommendation:** Apply exponential time decay to calibration outcomes: `weight = exp(-lambda * age_hours)`. Recent outcomes should dominate confidence estimates.
**Effort:** Small (modify GetCalibration to weight by timestamp)

### Gap R8: No Simplicity Gate for Proposed Actions
**Severity:** Low
**Location:** `internal/ape/improvement_plan.go`
**Current:** The Planner maps insights to actions 1:1 without considering whether the action's complexity cost is justified.
**AutoResearch parallel:** `program.md` line 37: "A small improvement that adds ugly complexity is not worth it."
**Recommendation:** Add a lightweight cost/benefit heuristic in the Planner. Actions with low expected benefit (based on calibration confidence) and high blast radius should be deprioritized.
**Effort:** Small (add filter after Plan, before Dispatch)

### Gap R9: No Exploration/Exploitation Balance
**Severity:** Medium
**Location:** `internal/ape/improvement_plan.go`
**Current:** Actions are always the same for the same symptoms. No experimentation with novel actions.
**AutoResearch parallel:** The agent naturally explores through LLM creativity. When stuck: "think harder, try combining previous near-misses, try more radical changes."
**Recommendation:** Implement epsilon-greedy or Thompson sampling in the Planner. With probability epsilon (e.g., 0.1), select a random action instead of the highest-priority one. Track outcomes to learn which novel actions work.
**Effort:** Medium (requires calibration history analysis)

### Gap R10: Watchdog Decay Is Linear
**Severity:** Low
**Location:** `internal/ape/watchdog.go:130`
**Current:** Decay score = `hoursSinceCycle * decayRate`. Linear urgency ramp.
**AutoResearch parallel:** N/A.
**Recommendation:** Use exponential decay for urgency escalation. Early hours should have low urgency growth; urgency should accelerate as time without maintenance increases.
**Effort:** Small (change multiplication to exponential)

### Gap R11: AllowedEndpoints Never Enforced
**Severity:** Low
**Location:** `internal/ape/task_spec.go`
**Current:** The endpoint whitelist is declarative documentation only. Executors call service methods directly.
**Recommendation:** While not directly inspired by autoresearch, this is a safety gap. Either enforce the whitelist via an interceptor pattern or remove the field to avoid false confidence.
**Effort:** Medium (requires refactoring executors)

### Gap R12: Snapshots Are In-Memory Only
**Severity:** Medium
**Location:** `internal/ape/action_snapshot.go`
**Current:** Max 50 snapshots, FIFO eviction, lost on server restart.
**AutoResearch parallel:** Git history serves as permanent snapshots — every state is recoverable.
**Recommendation:** Persist snapshots to Neo4j (RSICState nodes) for crash recovery. Critical for irreversible actions where rollback matters most.
**Effort:** Medium (extend RSICStore with snapshot serialization)

---

## 3. CMS Gap Analysis

### Current State
Full conversation lifecycle (observe, recall, resume, correct, consolidate, graduate). 10-stage retrieval pipeline (query analysis → intent translation → vector+BM25 → fusion → temporal → graph expansion → activation spreading → scoring → reasoning modules → LLM reranking). 5-layer hidden hierarchy (L0→L5) with KMeans/DBSCAN clustering, GraphSAGE-style message passing, and Hebbian learning edges.

### Gap C1: Query Classification Is Keyword-Based
**Severity:** High
**Location:** `internal/retrieval/scoring.go:374-448` (`ComputeRetrievalHints`)
**Current:** 5 query types detected by regex/keyword matching (`isCodeQuery`, `isArchitectureQuery`, etc.). Edge cases like "how does the auth module refresh tokens" may miss both code and architecture signals.
**AutoResearch parallel:** N/A — autoresearch has no retrieval.
**Recommendation:** Replace keyword matchers with a lightweight LLM classifier (few-shot prompt) or a small trained model. The classifier should support multi-label output (a query can be both "code" and "architecture").
**Effort:** Medium (follows intent translator pattern)

### Gap C2: Scoring Weights Are Hand-Tuned
**Severity:** High
**Location:** `internal/retrieval/scoring.go:572-861`
**Current:** 16-component linear scoring formula with ~12 hand-tuned hyperparameters (alpha=0.60, beta=0.20, gamma=0.15, etc.).
**AutoResearch parallel:** Autoresearch optimizes hyperparameters iteratively based on a single metric. MDEMG has benchmark data (whk-wms 120q, 0.783 mean score) that could serve as the optimization target.
**Recommendation:** Use the existing benchmark infrastructure to run automated hyperparameter sweeps on scoring weights. This is the most direct autoresearch-style application: modify weights → run benchmark → keep or discard. Could be a new RSIC action type: `optimize_scoring_weights`.
**Effort:** Large (requires benchmark automation pipeline, but infrastructure exists)

### Gap C3: Intent Translation Only for BM25
**Severity:** Medium
**Location:** `internal/retrieval/intent_translator.go`
**Current:** LLM rewrites conversational queries into keyword-dense strings, but only for BM25 search. Vector search uses the original query embedding.
**Recommendation:** Generate a separate embedding from the translated query for vector search. The translated query removes conversational noise and adds domain terms, which should improve vector recall.
**Effort:** Small (embed translated query alongside original, use higher-scoring result)

### Gap C4: Full Re-Clustering Every Consolidation Run
**Severity:** Medium
**Location:** `internal/hidden/service.go:315-438`
**Current:** `CreateHiddenNodes` detaches ALL L0→L1 edges and re-clusters from scratch. For 8,360+ nodes with 3,072-dim embeddings, this takes 10+ minutes.
**AutoResearch parallel:** Autoresearch is inherently incremental — each experiment changes only `train.py`, not the entire codebase.
**Recommendation:** Implement incremental consolidation: only re-cluster neighborhoods affected by new/changed nodes since the last consolidation. Track a "last_consolidated_at" cursor.
**Effort:** Large (requires change tracking and partial clustering)

### Gap C5: Reranker Sends Metadata Only
**Severity:** Medium
**Location:** `internal/retrieval/rerank.go:158-183`
**Current:** LLM rerank prompt includes name, path, and summary — not full content.
**Recommendation:** Include truncated content (first 500 chars) in the rerank prompt. For code nodes, the actual code is far more informative than the summary for relevance judgments.
**Effort:** Small (modify prompt template, fetch content field)

### Gap C6: No Benchmark-Driven Retrieval Optimization
**Severity:** High
**Location:** N/A (missing capability)
**AutoResearch parallel:** The core autoresearch concept — iterative optimization against a fixed evaluation. MDEMG has `docs/benchmarks/whk-wms/` with 120 questions and a grading pipeline.
**Recommendation:** Build an RSIC action type that: (1) runs the benchmark suite, (2) compares against baseline (0.783), (3) if a config change improved the score, keeps it; otherwise reverts. This is the most direct application of the autoresearch loop to MDEMG.
**Effort:** Large (new RSIC action type + benchmark runner integration)

### Gap C7: No Feedback From Retrieval Quality to Consolidation
**Severity:** Medium
**Location:** N/A (missing feedback loop)
**Current:** Consolidation (hidden layer creation) and retrieval (scoring + ranking) are independent. Poor retrieval results don't influence how consolidation clusters nodes.
**AutoResearch parallel:** Each experiment's result directly informs the next experiment's strategy.
**Recommendation:** Track retrieval quality metrics per-consolidation-epoch (e.g., average score of top-5 results on a test query set). Feed this back to consolidation parameters (cluster count, minimum cluster size, eps threshold).
**Effort:** Medium (requires quality tracking + parameter feedback)

### Gap C8: Temporal Reasoning Is Regex-Based
**Severity:** Low
**Location:** `internal/retrieval/temporal.go:178-222`
**Current:** Regex patterns for "last N days", "since YYYY-MM-DD", "this week", and soft keywords ("recent", "latest").
**Recommendation:** Use the intent translator's LLM to extract temporal constraints alongside keyword expansion. The LLM can handle nuanced expressions like "around the time we refactored auth."
**Effort:** Small (extend intent translator prompt)

### Gap C9: Activation Spreading Is Sequential
**Severity:** Low
**Location:** `internal/retrieval/activation.go`
**Current:** Map-based propagation in Go. For large subgraphs, this limits performance.
**Recommendation:** For graphs > 10K candidates, use sparse matrix multiplication (via gonum or a Cypher-side implementation). Not urgent — current performance is adequate for existing graph sizes.
**Effort:** Large (matrix library integration or Cypher rewrite)

---

## 4. Jiminy Gap Analysis

### Current State
4-source parallel fan-out (consulting/constraints, corrections, contradictions, frontiers) with 6-second timeout. Results are merged, deduplicated (exact string), filtered by confidence, sorted by priority+confidence, and rendered as structured prompt augmentation. Integrated via Claude Code hook on every user prompt.

### Gap J1: No Guidance Effectiveness Tracking
**Severity:** Critical
**Location:** N/A (missing capability)
**Current:** Jiminy injects guidance but never learns whether it was followed. No feedback loop.
**AutoResearch parallel:** Every experiment result feeds back into the agent's decision-making via results.tsv. Jiminy has no equivalent.
**Recommendation:** After guidance injection, observe whether the agent's subsequent action aligns with or contradicts the guidance. Reinforcement signal:
- Guidance followed → boost that guidance source's confidence
- Guidance ignored → neutral (may have been irrelevant)
- Guidance contradicted → reduce confidence, flag for review
This mirrors the signal_learner pattern (R4) but applied to Jiminy outputs.
**Effort:** Medium (observe post-action behavior, match against prior guidance)

### Gap J2: Deduplication Is Exact-String Only
**Severity:** Medium
**Location:** `internal/jiminy/service.go:382-392`
**Current:** `deduplicateItems()` uses `seen[item.Content]`. Semantically identical items with different wording pass through.
**Recommendation:** Compute embeddings for guidance items and cluster by cosine similarity > 0.85. Keep highest-confidence representative per cluster.
**Effort:** Small (embeddings already available from the retrieval pipeline)

### Gap J3: Constraint Detection Is Keyword-Based
**Severity:** High
**Location:** `internal/consulting/service.go:914-975`
**Current:** Scans node summaries for "must", "required", "forbidden", etc. Misses naturally-expressed constraints.
**AutoResearch parallel:** Autoresearch's `program.md` expresses constraints naturally ("Some increase is acceptable... but it should not blow up dramatically"). Keyword matching would miss this.
**Recommendation:** Use an LLM classifier (few-shot) to identify constraints in retrieved nodes. The prompt: "Does this text express a requirement, prohibition, or recommendation? If so, classify as must/must_not/should/should_not." Follows the EmergenceNamer pattern.
**Effort:** Medium (new LLM call with circuit breaker, cached per node)

### Gap J4: No Temporal Weighting in Correction Retrieval
**Severity:** Medium
**Location:** `internal/jiminy/corrections.go`
**Current:** Corrections ranked by cosine similarity only. A 6-month-old correction weighs the same as yesterday's.
**Recommendation:** Apply time-decay multiplier: `score * exp(-lambda * age_days)`. Recent corrections are more likely to reflect the current state of the codebase.
**Effort:** Small (add timestamp to vector query results, apply decay)

### Gap J5: Single-Space Operation
**Severity:** Medium
**Location:** `internal/jiminy/service.go:44`
**Current:** Jiminy queries only within the request's `space_id`. Global best practices from `mdemg-global` (Phase 105) are invisible.
**Recommendation:** Query both the user's space and `mdemg-global` in parallel. The retrieval pipeline already supports `spaceIDs []string`. Global constraints should be surfaced at lower priority than space-specific ones.
**Effort:** Small (pass `[]string{spaceID, "mdemg-global"}` to retrieval calls)

### Gap J6: Conflict Detection Uses Hardcoded Paradigm Pairs
**Severity:** Medium
**Location:** `internal/consulting/service.go:886-908`
**Current:** Only 4 pairs checked: sync/async, class/function, SQL/NoSQL, REST/GraphQL.
**Recommendation:** Mine implicit contradictions from the Hebbian learning graph. Node pairs with high co-activation but opposing content (detected by embedding distance or explicit CONTRADICTS edges) represent dynamic, data-driven conflicts.
**Effort:** Medium (Cypher query for co-activated but semantically distant pairs)

### Gap J7: AgentOutput Review Mode Unimplemented
**Severity:** Low
**Location:** `internal/jiminy/types.go:24-25`
**Current:** `GuidanceRequest.AgentOutput` field exists but `Guide()` never uses it.
**Recommendation:** Implement a "review mode" where Jiminy checks the agent's proposed output against constraints before delivery. This shifts from pre-action guidance to pre-output validation. Lower priority because Phase 104 guardrails partially cover this.
**Effort:** Medium (new code path in Guide, constraint matching against output)

---

## 5. Cross-Cutting Integration Opportunities

### XC1: AutoResearch-Style Optimization Loop for MDEMG
The most transformative integration would be adapting autoresearch's core loop for MDEMG's own optimization:

```
LOOP (as new RSIC macro action):
1. Snapshot current scoring/retrieval config
2. Mutate one parameter (within safe bounds)
3. Run benchmark suite (whk-wms 120q)
4. If benchmark score improved → KEEP config change
5. If equal/worse → DISCARD (restore snapshot)
6. Log to optimization_results.tsv equivalent
```

This would make MDEMG self-optimizing — the retrieval pipeline tunes itself against ground-truth benchmarks without human intervention. **This is the highest-impact cross-cutting recommendation.**

### XC2: Immutable Evaluation Harness Pattern
AutoResearch's `prepare.py` (read-only, agent cannot modify) prevents the optimizer from gaming the metric. MDEMG should formalize this: benchmark questions, expected answers, and the grading pipeline should be treated as immutable fixtures that RSIC cannot modify during self-optimization.

### XC3: Results Log as Persistent Memory
AutoResearch's `results.tsv` is the agent's memory across experiments. MDEMG should store RSIC cycle outcomes, scoring weight experiments, and benchmark results as CMS observations (obs_type: `self_improvement`), making them retrievable by the retrieval pipeline and visible to Jiminy for constraint enforcement.

---

## 6. Priority Matrix

| ID | Gap | Impact | Effort | Priority |
|----|-----|--------|--------|----------|
| **R1** | Post-cycle re-assessment | Critical | Small | **P0** |
| **R2** | Success criteria evaluation | High | Small | **P0** |
| **R6** | Keep/discard metrics semantics | High | Medium | **P0** |
| **J1** | Guidance effectiveness tracking | Critical | Medium | **P0** |
| **R3** | LLM-powered reflection | High | Medium | **P1** |
| **C2** | Learned scoring weights | High | Large | **P1** |
| **C6** | Benchmark-driven optimization (XC1) | High | Large | **P1** |
| **J3** | LLM constraint classification | High | Medium | **P1** |
| **C1** | LLM query classification | High | Medium | **P1** |
| **R4** | Wire signal learner | Medium | Small | **P2** |
| **R5** | Per-action time-boxing | Medium | Small | **P2** |
| **R7** | Time-weighted calibration | Medium | Small | **P2** |
| **J2** | Semantic deduplication | Medium | Small | **P2** |
| **J4** | Temporal weighting for corrections | Medium | Small | **P2** |
| **J5** | Multi-space guidance | Medium | Small | **P2** |
| **C3** | Intent translation for vector search | Medium | Small | **P2** |
| **C5** | Reranker with content | Medium | Small | **P2** |
| **R8** | Simplicity gate | Low | Small | **P3** |
| **R9** | Exploration/exploitation balance | Medium | Medium | **P3** |
| **R12** | Persistent snapshots | Medium | Medium | **P3** |
| **C4** | Incremental consolidation | Medium | Large | **P3** |
| **C7** | Retrieval→consolidation feedback | Medium | Medium | **P3** |
| **J6** | Graph-based conflict detection | Medium | Medium | **P3** |
| **J7** | AgentOutput review mode | Low | Medium | **P3** |
| **R10** | Exponential watchdog decay | Low | Small | **P4** |
| **R11** | AllowedEndpoints enforcement | Low | Medium | **P4** |
| **C8** | LLM temporal reasoning | Low | Small | **P4** |
| **C9** | Matrix-based activation spreading | Low | Large | **P4** |

---

## 7. Recommended Implementation Sequence

### Phase AR-1: Close the RSIC Feedback Loop (P0)
- R1: Post-cycle re-assessment
- R2: Success criteria evaluation
- R6: Keep/discard semantics
- **Outcome:** RSIC becomes self-aware of whether its actions actually helped

### Phase AR-2: Jiminy Learns From Experience (P0)
- J1: Guidance effectiveness tracking
- **Outcome:** Jiminy can strengthen useful guidance and deprioritize noise

### Phase AR-3: LLM-Powered Intelligence (P1)
- R3: LLM reflection
- J3: LLM constraint classification
- C1: LLM query classification
- **Outcome:** Replace hardcoded rules with LLM reasoning across all three systems

### Phase AR-4: Self-Optimizing Retrieval (P1)
- C2: Learned scoring weights
- C6: Benchmark-driven optimization loop (XC1)
- **Outcome:** MDEMG tunes its own retrieval pipeline against ground-truth benchmarks

### Phase AR-5: Quick Wins (P2)
- R4, R5, R7: Signal learner, time-boxing, calibration decay
- J2, J4, J5: Semantic dedup, temporal corrections, multi-space
- C3, C5: Intent translation for vectors, reranker content
- **Outcome:** 10 small improvements that compound

### Phase AR-6: Structural Improvements (P3-P4)
- R8, R9, R12: Simplicity gate, exploration/exploitation, persistent snapshots
- C4, C7: Incremental consolidation, retrieval→consolidation feedback
- J6, J7: Graph-based conflicts, output review mode
- **Outcome:** Architectural improvements for long-term scalability

---

## Documents Accessed

- `packaging/autoresearch/program.md` — Agent instructions and experiment protocol
- `packaging/autoresearch/prepare.py` — Fixed evaluation harness
- `packaging/autoresearch/train.py` — Mutable training script
- `packaging/autoresearch/README.md` — Project overview
- `internal/ape/` (23 files) — Full RSIC implementation
- `internal/api/handlers_self_improve.go` — RSIC API endpoints
- `internal/api/rsic_adapters.go` — Service adapter interfaces
- `internal/api/handlers_conversation.go` — CMS API endpoints
- `internal/retrieval/` (all files) — Retrieval pipeline
- `internal/hidden/` (key files) — Consolidation pipeline
- `internal/learning/service.go` — Hebbian learning
- `internal/jiminy/` (all files) — Jiminy inner voice
- `internal/api/handlers_jiminy.go` — Jiminy API handler
- `internal/consulting/service.go` — Constraint/conflict detection
- `.claude/hooks/prompt-context.sh` — Jiminy hook integration
- `internal/config/config.go` — Configuration parameters
- `docs/development/RSIC_GAP_ANALYSIS.md` — Prior RSIC gap analysis
- `docs/development/COGNITIVE_INTELLIGENCE_GAP_ANALYSIS.md` — Cognitive gaps 101-105
