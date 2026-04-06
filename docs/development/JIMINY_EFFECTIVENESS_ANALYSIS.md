# Jiminy Guidance Effectiveness Analysis

**Date:** 2026-04-06
**Branch:** reh3376_dev01
**Version:** v0.7.0
**Data Window:** All available (2026-03-26 through 2026-04-06)
**Total GUIDANCE_OUTCOME edges:** 675

## Executive Summary

82.4% of Jiminy guidance is classified as "ignored" (556/675 outcomes). The primary cause is **measurement error (H5)** — a confluence of three structural issues makes accurate classification nearly impossible:

1. **LLM classifier disabled** — classification falls back to a binary heuristic (similarity ≥ 0.5 = followed, < 0.5 = ignored) with no partial_compliance output, misclassifying the 0.3-0.5 similarity range (239 items, 35% of all outcomes)
2. **Action summaries are structurally mismatched** — terse path-based summaries ("Wrote file: internal/api/handler.go") can never achieve high cosine similarity against rich guidance content ("ensure all error handling includes structured logging")
3. **Guidance sources are polluted** — 91.9% of GUIDANCE_OUTCOME edges are attached to untyped MemoryNodes (code descriptions, progress notes), not constraint/correction nodes

A secondary cause is **guidance irrelevance (H1)** — 27% of surfaced guidance items are wrong-context (e.g., Python synchronization classes surfaced for Go tasks) due to multi-repo SymbolNodes in the graph.

**Immediate fix priority:** Enable the LLM classifier (even with a local model) and enrich action summaries with semantic content. These two changes alone could reduce the false-ignore rate by 50%+.

## Data Collection Methodology

### Sources
- **TSDB (TimescaleDB):** 1,619 LLM interactions across 7 days, 15 Jiminy metrics
- **Neo4j:** 675 GUIDANCE_OUTCOME edges, 244 MemoryNodes, 36 typed constraint/correction nodes
- **Jiminy API:** Protocol metrics, tier effectiveness, trust status, latest guidance
- **Server logs:** `~/.mdemg/logs/server.log` (2.4 MB, 2026-04-03 startup only)
- **Manual review:** 15 ignored outcomes + 30 live guidance items classified by human

### Queries Executed
- TSDB: Q1-Q3 (guidance correlation), Q10-Q11 (Jiminy metrics)
- Neo4j: Q4-Q9 (outcome distribution, constraints), Q12-Q13 (similarity), Q15-Q18 (constraint analysis), Q12a (SignalState)
- API: 5 endpoint snapshots (healthz, protocol metrics, tier effectiveness, status, latest)

## Findings

### 1. Outcome Distribution

| Outcome | Count | % | Avg Similarity |
|---------|-------|---|----------------|
| ignored | 556 | 82.4% | 0.28 |
| followed | 110 | 16.3% | 0.93 |
| contradicted | 9 | 1.3% | 0.39 |
| partial_compliance | 0 | 0% | — |

The complete absence of `partial_compliance` is a red flag. The classifier should produce a gradient, not binary output. With LLM disabled, the heuristic fallback (lines 199-202 in `outcome_classifier.go`) only outputs "followed" (≥0.5) or "ignored" (<0.5).

### 2. Trend Analysis

| Period | Ignore Rate |
|--------|------------|
| Early (Mar 26-27) | 89.2% |
| Late (Mar 30 - Apr 1) | 77.6% |

Trend: **decreasing** (improving). The improvement coincides with the v0.7.0 hardening sprint which fixed RRF scoring and signal persistence. However, the feedback loop has been **silent since March 31** — no new outcomes are being recorded.

### 3. Similarity Distribution (The Smoking Gun)

| Outcome | Count | Min | P25 | P50 | P75 | Max |
|---------|-------|-----|-----|-----|-----|-----|
| ignored | 556 | 0.06 | 0.21 | 0.28 | **0.36** | 0.49 |
| followed | 110 | 0.50 | 1.00 | 1.00 | 1.00 | 1.00 |
| contradicted | 9 | 0.36 | 0.36 | 0.37 | 0.39 | 0.46 |

**Critical:** The ignored P75 is 0.36 — above the `lowThreshold` (0.3). This means 25% of "ignored" items have similarity in the 0.3-0.5 range and are classified as ignored by the heuristic fallback, not by the embedding threshold.

**Threshold bucket analysis:**

| Bucket | Outcome | Count | What Should Happen |
|--------|---------|-------|--------------------|
| high (≥0.7) | followed | 91 | Correct |
| mid-high (0.5-0.7) | followed | 19 | Correct (heuristic) |
| **mid-low (0.3-0.5)** | **ignored** | **239** | **Should be partial_compliance** |
| mid-low (0.3-0.5) | contradicted | 9 | Correct (negation detected) |
| low (<0.3) | ignored | 317 | Correct |

**239 items (35.4% of all outcomes) are in the 0.3-0.5 range and classified as "ignored" when they should be "partial_compliance."** This is the largest single source of classification error.

### 4. Manual Review: Zero Classifier Agreement

15 "ignored" outcomes with content were manually reviewed:

| Human Classification | Count | Percentage |
|---------------------|-------|-----------|
| H5: Measurement error (agent likely followed) | 7 | 47% |
| H1: Irrelevant guidance (wrong source node) | 8 | 53% |
| Agree: Truly ignored | 0 | 0% |

**0% agreement between classifier and human judgment.** Every single "ignored" item was either a measurement error (valid guidance that was followed but similarity too low) or irrelevant guidance (progress notes, decisions, wrong-context corrections surfaced as guidance).

### 5. Guidance Relevance Assessment

30 live guidance items across 3 scenarios were categorized:

| Category | Count | % |
|----------|-------|---|
| Relevant | 15 | 50% |
| Wrong Context | 8 | 27% |
| Generic | 7 | 23% |
| Obvious | 0 | 0% |
| Stale | 0 | 0% |

Key issues:
- **Multi-repo noise:** SymbolNodes from Python codebases (IGN_scripts, control_loop01) surface for Go tasks
- **Patterns dominate:** 27/30 items are "pattern" type; actual constraints are underrepresented
- **Context sensitivity:** Jiminy-specific queries get 80% relevant results; generic queries get 40%

### 6. Feedback Loop Breakdown

| Metric | Value | Assessment |
|--------|-------|-----------|
| Last feedback received | 2026-03-31 | **6 days ago — loop is dead** |
| Trust score | 0.22 | Very low (scale: 0-1) |
| Current tier | T3 | Most verbose (maximum token cost) |
| Feedback count (session) | 12 | Very low for ~88 guidance cycles |
| Cooldown suppression | 48% | Nearly half of tool uses never send feedback |
| Typed nodes with feedback | 6/36 | 83% of constraints/corrections have no feedback |
| LLM classifier | **Disabled** | Falls back to 0.5 heuristic threshold |
| LLM synthesis | **Failing** | OpenAI timeout (context deadline exceeded) |

### 7. Signal Learner State

**No SignalState nodes exist in the graph.** Despite V0024 migration creating the `SignalState` uniqueness constraint, no signal data has been flushed. This means the signal learner — which should learn from follow/ignore patterns to adjust signal strengths — is operating in a permanent cold-start state.

Possible causes:
- The signal learner flush timer may not be triggering
- The server may not have been running long enough since V0024 was applied
- The flush may be failing silently

### 8. GUIDANCE_OUTCOME Edge Topology

91.9% of GUIDANCE_OUTCOME edges are on MemoryNodes with `obs_type = null` — these are general MemoryNodes (code descriptions, module summaries, build results), NOT constraint/correction nodes. This means:

1. The Guide() function retrieves from all MemoryNode types
2. Outcome edges are attached to whichever node was retrieved
3. The outcome classifier compares action summaries against code documentation strings
4. Low similarity is almost guaranteed for non-constraint source nodes

Only 33/675 edges (4.9%) are on typed constraint/correction nodes, where the classifier has any chance of semantic alignment.

## Hypothesis Assessment

| Hypothesis | Assessment | Key Evidence | Confidence |
|------------|-----------|--------------|------------|
| H1: Guidance irrelevant | **MEDIUM-HIGH** | 27% wrong context in live capture; 91.9% untyped source nodes; multi-repo SymbolNode noise | 60% |
| H2: Guidance obvious | **LOW** | Zero items classified as "obvious" in manual review; constraints are project-specific | 5% |
| H3: Guidance too late | **MEDIUM** | Feedback loop silent for 6 days; 48% cooldown suppression; but no direct timing evidence | 30% |
| H4: Guidance not seen | **LOW** | Prompt-context.sh hook works; guidance appears in context; agent processes it | 10% |
| H5: Measurement wrong | **HIGH** | 0% classifier-human agreement; 239 items misclassified in 0.3-0.5 range; LLM disabled; zero partial_compliance; binary heuristic; action summary structural mismatch | 95% |

**H5 is the dominant cause.** Even if guidance were 100% relevant, the classifier would still produce ~80% "ignored" due to structural similarity mismatch and the disabled LLM tier.

## Recommended Actions

| # | Action | Hypothesis | Effort | Priority | Impact |
|---|--------|-----------|--------|----------|--------|
| 1 | **Enable LLM classifier** with local Ollama model | H5 | 2h | P0 | Eliminates heuristic fallback; enables partial_compliance |
| 2 | **Enrich action summaries** with file content snippets and intent | H5 | 4h | P0 | Increases semantic similarity for valid follow actions |
| 3 | **Lower heuristic threshold** from 0.5 to 0.35 as interim fix | H5 | 15min | P0 | Immediately reclassifies ~100 items from ignored to followed |
| 4 | **Filter guidance sources** to typed nodes only (constraint, correction) | H1 | 3h | P1 | Eliminates irrelevant code descriptions from outcome tracking |
| 5 | **Reduce cooldown** from 30s to 10s or make adaptive | H3/H5 | 1h | P1 | Increases feedback coverage from 52% to ~85% |
| 6 | **Investigate SignalState flush failure** | H5 | 2h | P1 | Restores signal learning persistence |
| 7 | **Prune multi-repo SymbolNodes** or scope retrieval to Go code | H1 | 3h | P2 | Reduces wrong-context guidance from 27% to ~5% |
| 8 | **Add outcome recording for pattern sources** with type annotations | H1 | 4h | P2 | Enables proper analysis of pattern guidance effectiveness |
| 9 | **Design A/B threshold experiment** — lower 0.3 to 0.15, measure 48h | H5 | 1h | P2 | Empirical validation of threshold sensitivity |

## Monitoring

Run the diagnostic script monthly to track effectiveness trends:

```bash
python3 scripts/jiminy_effectiveness_report.py --space-id mdemg-dev --days 30
```

Key Performance Indicators to track:
- **Ignore rate** (target: < 50%, currently 82.4%)
- **Manual review agreement rate** (target: > 80%, currently 0%)
- **Partial compliance count** (target: > 0, currently 0)
- **Feedback coverage** (target: > 80%, currently 16.7%)
- **SignalState node count** (target: > 0, currently 0)
- **Trust score** (target: > 0.5, currently 0.22)

## Documents Accessed

- `internal/jiminy/outcome_classifier.go` — Classify() method, similarity thresholds (0.7/0.3), heuristic fallback (0.5)
- `internal/jiminy/effectiveness.go` — LRU tracker, 2hr TTL, 1000 capacity
- `internal/jiminy/service.go:1090-1299` — RecordOutcome flow
- `internal/jiminy/persistence.go:44-96` — PersistGuidanceOutcome Neo4j edge creation
- `internal/jiminy/types.go:113-155` — Outcome constants, Feedback structs
- `internal/cli/hook_templates/post-tool-observe.py` — Feedback hook, 30s cooldown, action summary construction
- `internal/cli/hook_templates/prompt-context.sh` — Guidance injection flow
- `internal/tsdb/migrations/005_interaction_enrichment.sql` — guidance_id column
- `scripts/tsdb_data_review.py` — Template for diagnostic script architecture
