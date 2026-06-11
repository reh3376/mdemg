# 08 — HTM Sequence Memory for Predictive Jiminy

**Sprint ID**: HTM-SEQUENCE-MEMORY
**Date**: 2026-04-21 (plan authored — RESEARCH-STAGE, intentionally less exhaustive)
**Branch**: TBD
**Scope**: Add a lightweight **HTM-style sequence memory** over the event stream that learns "what typically comes after what" and feeds predictions to Jiminy Guide. Transforms Jiminy from reactive (responds to queries) to genuinely predictive (anticipates the next action). Research-stage — specification kept deliberately shallow so that empirical findings can reshape the approach.

**Upstream**: [White Paper Review](mdemg-white-paper-review.md) Paper 2 (HTM sequence memory; one-shot pattern learning; context-specific predictions via distal activation).

---

## Sprint Framing

Three reasons this is research-stage and the plan is kept lean:

1. **Empirical unknowns dominate.** Will the event stream have enough sequence structure to learn from? How many examples before predictions stabilize? What's the acceptable false-positive rate for predictive suggestions? These cannot be answered from the literature alone.
2. **Interaction with other sprints is unclear.** Context fingerprints (Sprint 05) provide the right substrate for HTM's distal context, but HTM without context is a weaker model. Best to specify in detail *after* Sprint 05 lands.
3. **The UX design is unsolved.** A predictive Jiminy that's too eager is annoying; too conservative is invisible. Calibration is a product question, not just an engineering one.

So this plan is **a skeleton**, not a complete spec. Sprint 08 should be re-specced after the shadow-mode prototype runs for ≥4 weeks and real sequence structure is measured in live event streams.

---

## Gap Summary

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Sequence Memory Data Structure | 0 | 3 | 1 | 0 | **4** |
| Event Stream Wiring | 0 | 2 | 1 | 0 | **3** |
| Shadow-Mode Prediction Stream | 0 | 3 | 1 | 0 | **4** |
| Jiminy Integration (live, after shadow) | 0 | 2 | 2 | 0 | **4** |
| Observability & UX | 0 | 2 | 1 | 0 | **3** |
| Testing & Verification | 0 | 2 | 1 | 0 | **3** |
| Mandatory Documentation Phase | 0 | 4 | 2 | 0 | **6** |
| **Total** | **0** | **18** | **9** | **0** | **27** |

---

## Phase 1: Sequence Memory Data Structure

### 1.1 Event typing (HIGH)

**Gap**: MDEMG events are heterogeneous (hook events, API calls, CLI invocations, RSIC cycles). For sequence learning, we need a **typed event vocabulary** — e.g. `test_run`, `commit`, `review_request`, `protocol_consult`, etc.

**Fix** — `internal/events/types.go`: define a closed enum of ~30 event types covering the most frequent developer actions. Map existing event emission to this vocabulary. Events outside the vocabulary get tagged `other` and don't participate in sequence learning until the vocabulary is extended.

**Files**: `internal/events/types.go`

---

### 1.2 Sequence memory structure (HIGH)

**Decision** — implement a **variable-order Markov model** with an HTM-style representation: each (state, context) pair has its own "cell," and the cell records what state came next. This approximates HTM sequence memory without the full complexity of distal synapse segments.

Storage: a compact Neo4j subgraph per space:

```cypher
(state:SequenceState {event_type: 'test_run', space_id: $sid})
(context:SequenceContext {hash: '...', space_id: $sid})
(state)-[:IN_CONTEXT]->(context)
(context)-[:PREDICTS {weight: 0.73, evidence: 42}]->(next_state:SequenceState)
```

Context is a hash of the last N events (variable order; up to N=4). Allows "in the context of (test_run, review_request, test_run), what comes next?"

**Files**: `internal/sequence/memory.go` (new), `internal/migrations/V0030_sequence_memory.cypher`

---

### 1.3 One-shot and few-shot learning (HIGH)

**Gap**: A core HTM claim is fast learning. We need to enable a pattern to become predictive after 1-3 observations, not 20.

**Fix** — Any observed (context, next) transition creates an edge with evidence=1 immediately. Edge is queryable for prediction right away; confidence in the prediction is low but non-zero. Classic evidence-counting layers on top for stable patterns.

**Files**: `internal/sequence/memory.go`

---

### 1.4 Forgetting (MEDIUM)

**Fix** — Edges with no reinforcement for >30 days decay to evidence=0 and are dropped. Prevents the sequence graph from accumulating stale patterns indefinitely.

**Files**: `internal/ape/cycle.go` (macro cycle)

---

## Phase 2: Event Stream Wiring

### 2.1 Tap the event stream (HIGH)

**Fix** — In the main event-handling path (wherever events are emitted to metrics/logs), also feed them to a new `SequenceLearner`:

```go
func (l *SequenceLearner) Observe(ctx context.Context, event Event) {
    // Append event to per-session context ring buffer.
    // On each new event, record the (context, event) transition
    // in the sequence memory.
}
```

Sessions are per-developer or per-agent — context is session-local, not global.

**Files**: `internal/sequence/learner.go` (new)

---

### 2.2 Session context management (HIGH)

**Fix** — New session tables in Neo4j or Redis for per-session recent-event rings. Cheap lookups for the next-event prediction.

**Files**: `internal/sequence/session.go` (new)

---

### 2.3 Back-pressure / rate limiting (MEDIUM)

**Gap**: Event streams can burst. Sequence learning must not block event handling.

**Fix** — Async queue. Drop-oldest on overflow. Metric on drop rate.

**Files**: `internal/sequence/learner.go`

---

## Phase 3: Shadow-Mode Prediction Stream

**Goal**: Compute predictions but do not surface them yet. Validate quality on real data for ≥4 weeks.

### 3.1 Prediction API (HIGH)

**Fix** — `Predict(context) → []NextEventPrediction` returns ranked probable next events with confidences.

**Files**: `internal/sequence/predict.go`

### 3.2 Shadow-mode emission to TSDB (HIGH)

**Fix** — Every event triggers a prediction *before* it is itself recorded. The prediction is logged to TSDB; after the event actually occurs, log whether the prediction was correct. Builds the evaluation dataset.

**Files**: `internal/sequence/learner.go`, TSDB schema addition

### 3.3 Prediction quality metrics (HIGH)

**Metrics**: top-1 accuracy, top-3 accuracy, mean reciprocal rank, coverage (what fraction of events had a prediction at all), calibration (how well does confidence match accuracy?).

**Files**: `internal/metrics/registry.go`

### 3.4 Weekly review report (MEDIUM)

**Fix** — Generated report summarizing prediction quality week-over-week. Feeds the go/no-go decision for Phase 4.

**Files**: `scripts/reports/sequence-quality.py`

---

## Phase 4: Jiminy Integration (LIVE — only after shadow-mode validation)

### 4.1 Gate criteria (HIGH)

Before enabling any predictive suggestions in Jiminy:
- Top-3 accuracy ≥ 0.4 over the last 2 weeks of shadow data
- Coverage ≥ 0.3 (at least 30% of events had a prediction)
- Calibration: mean |confidence - accuracy| < 0.15

If these aren't met, Phase 4 does not ship. Replan Sprint 08.

### 4.2 Jiminy predictive hint API (HIGH)

**Fix** — New Jiminy method `GetPredictiveHints(session) → []Hint` that returns top-K predictions from the sequence memory, surfaced in the Jiminy Guide UI.

**Files**: `internal/jiminy/service.go`, `internal/jiminy/predictive.go` (new)

### 4.3 UX: calibration-aware suggestions (MEDIUM)

**Fix** — Low-confidence predictions are not shown. Medium-confidence predictions shown as "suggestions" (soft). High-confidence predictions shown as "next steps" (bold). Respects user's expressed preference for prediction aggressiveness.

**Files**: Jiminy client UI, `internal/jiminy/predictive.go`

### 4.4 Feedback loop (MEDIUM)

**Fix** — When user accepts or rejects a predictive hint, emit a reinforcement signal to the sequence memory. Extends learning beyond passive observation.

**Files**: `internal/jiminy/predictive.go`

---

## Phase 5: Observability & UX

### 5.1 Prometheus metrics (HIGH)

Listed in 3.3 plus:

```
mdemg_sequence_states_total{space_id}
mdemg_sequence_prediction_confidence{space_id} - histogram
mdemg_sequence_prediction_accepted_total{space_id, confidence_tier}
```

**Files**: `internal/metrics/registry.go`

### 5.2 CLI debug (HIGH)

`mdemg sequence inspect --session <id>` — prints recent context, top predictions, accuracy history.

**Files**: `internal/cli/sequence.go` (new)

### 5.3 Grafana (MEDIUM)

New dashboard `mdemg-sequence.json` with accuracy trends, coverage, calibration chart.

**Files**: `deploy/grafana/dashboards/mdemg-sequence.json`

---

## Phase 6: Testing & Verification

### 6.1 Unit tests (HIGH)
- Sequence memory correctness on synthetic sequences
- Variable-order context hashing stability
- Decay and forgetting correctness

### 6.2 Integration test (HIGH)
Inject a known sequence pattern 5 times, assert that prediction confidence for the next element exceeds threshold.

### 6.3 Synthetic benchmark (MEDIUM)
Generate synthetic event streams with known Markov structure. Assert sequence memory recovers the true transition probabilities within ε.

---

## Phase 7: Mandatory Documentation Phase

### 7.1 CHANGELOG.md (HIGH)
### 7.2 AGENT_HANDOFF.md (HIGH)
### 7.3 VISION.md — add HTM sequence memory section (HIGH)
### 7.4 `docs/features/predictive-jiminy.md` (new feature doc) (HIGH)
### 7.5 CLAUDE.md — add sequence-memory vocabulary (MEDIUM)
### 7.6 Homebrew beta testing guide + submodule bump (MEDIUM)

---

## Risk Analysis & Rollback

### R1: Insufficient sequence structure in real event streams

**Likelihood**: Medium-High. Developer workflows may be too heterogeneous for strong sequence prediction.

**Mitigation**: Shadow-mode Phase 3 measures this before committing to Phase 4. If accuracy is too low, Sprint 08 does not ship predictive suggestions; the sequence memory is retained as a passive observability tool only.

**Rollback**: Disable `SEQUENCE_LEARNING_ENABLED`. Graph subgraph remains; no hints surfaced.

### R2: Predictive hints harm UX

**Likelihood**: Medium. Wrong predictions are annoying; right ones can feel invasive.

**Mitigation**: Calibration-aware tiers (4.3). Per-user opt-in. User feedback closes the loop.

**Rollback**: Per-tier config disables each surface level independently.

### R3: Sequence memory grows unboundedly

**Likelihood**: Low. Forgetting rule (1.4) caps it.

**Mitigation**: Metric on state count with alert at > 100k per space.

---

## Sprint Size Estimate

| Phase | Estimate |
|-------|----------|
| 1. Sequence Memory Structure | 2 days |
| 2. Event Stream Wiring | 1.5 days |
| 3. Shadow-Mode Prediction Stream | 2 days |
| 4. Jiminy Integration (if gate passes) | 2 days |
| 5. Observability & UX | 1 day |
| 6. Testing & Verification | 1.5 days |
| 7. Mandatory Documentation | 0.5 day |
| Shadow-mode soak | 4 weeks calendar |
| **Total dev time** | **~10.5 days** |
| **Total calendar** | **~6-8 weeks incl. shadow & gate review** |

---

## Dependencies

**Blocks**: None.

**Blocked by**: Strongly recommended — 05 (context-specific activations) should land first. Sequence memory benefits materially from context fingerprints for distal-synapse-style context matching.

**Touches but does not block**: 03 (top-down predictions use a similar mechanism conceptually — both are predictions, but over different timescales).

---

## Documents Accessed

- `internal/jiminy/service.go`
- White paper review Paper 2
