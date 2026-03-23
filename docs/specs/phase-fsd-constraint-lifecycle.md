<!-- markdownlint-disable MD013 MD022 MD032 MD040 -->
# Phase FSD-2026-001: Constraint Lifecycle Closure & Behavioral Determinism

**Phase**: FSD-2026-001
**Status**: COMPLETE
**Author**: Agent
**Date**: 2026-03-20
**Branch**: reh3376_dev01

---

## Overview

FSD-2026-001 is a focused-scope delivery (FSD) addressing 21 behavioral gaps across 4 priority tiers. It transforms MDEMG from an advisory constraint system into a full constraint enforcement lifecycle with measurable behavioral determinism.

Prior to this delivery, MDEMG could detect, store, and surface constraints via Jiminy, but the loop was open: there was no enforcement at the tool boundary, no feedback about whether guidance was followed, no detection of conflicting constraints, and no way to measure whether the system was actually changing agent behavior. The guardrail pipeline existed but could only validate diffs on demand — it did not block tool use in real time.

FSD-2026-001 closes all four of these gaps simultaneously:

1. **Enforcement** (F1): PreToolUse hook enforcement + guardrail event log
2. **Feedback** (F2a/F2b/F3/F5/F6): Contradiction detection, effectiveness persistence, confidence scoring, NLI classification
3. **Conflict detection** (F4): Pairwise CONFLICTS_WITH edge creation and resolution workflow
4. **Determinism** (F9): Measurable D-score = (informed_rate × compliance_rate × coverage_ratio)

The delivery also addresses the neural re-ranker stack (NR-1/NR-2/NR-3/NR-4) as a performance and quality improvement, and closes remaining operational gaps (F7 scope filtering, F10 Jiminy caching, F11 activation weights, F13 constraint decay, F14 prompt injection sanitization, F15 hop depth, F18 edge limits, F20 authority levels, F21 LLM client unification).

---

## Gap Summary Table

All 21 gaps implemented in this delivery, grouped by priority tier.

### Tier 1 — Enforcement & Feedback Loop (Blocking)

| Gap ID | Phase Code | Description | Priority | Status |
|--------|------------|-------------|----------|--------|
| GAP-01 | F1 | PreToolUse hook enforcement — blocks tool use on `must_not` violations in real time | P0 | COMPLETE |
| GAP-02 | F2a | Contradiction detection — embedding similarity + heuristic negation in surprise scoring | P0 | COMPLETE |
| GAP-03 | F2b | NLI-enhanced contradiction detection — sidecar NLI model for higher-precision classification | P0 | COMPLETE |
| GAP-04 | F3 | Effectiveness feedback persistence — GUIDANCE_OUTCOME edges + Bayesian confidence updates | P0 | COMPLETE |
| GAP-05 | F4 | Cross-constraint conflict detection — pairwise CONFLICTS_WITH edge creation and resolution API | P0 | COMPLETE |
| GAP-06 | F5 | Dynamic confidence signal during constraint promotion — detection_confidence on new constraints | P1 | COMPLETE |
| GAP-07 | F6 | LLM/NLI constraint classification gate — replaces keyword-only detection with LLM classifier | P1 | COMPLETE |

### Tier 2 — Operational Quality (High)

| Gap ID | Phase Code | Description | Priority | Status |
|--------|------------|-------------|----------|--------|
| GAP-08 | F7 | Constraint scope filtering — file path glob scoping of guardrail enforcement | P1 | COMPLETE |
| GAP-09 | F9 | Determinism score — measurable D-score endpoint for behavioral compliance | P1 | COMPLETE |
| GAP-10 | F10 | Jiminy latency optimization — LRU + TTL cache for guidance responses | P1 | COMPLETE |
| GAP-11 | F11 | Configurable activation dimension weights — semantic/temporal/coactivation tuning | P2 | COMPLETE |
| GAP-12 | F13 | Constraint decay and expiry — time-based confidence decay for unsurfaced constraints | P2 | COMPLETE |
| GAP-13 | F14 | Prompt injection sanitization — `internal/sanitize/` package wired into synthesis | P1 | COMPLETE |
| GAP-14 | F15 | Configurable hop depth — `MAX_HOP_DEPTH` for architecture queries | P2 | COMPLETE |
| GAP-15 | F18 | Edge limit enforcement — auto-prune excess edges per node during consolidation | P2 | COMPLETE |

### Tier 3 — Authority & Trust (Medium)

| Gap ID | Phase Code | Description | Priority | Status |
|--------|------------|-------------|----------|--------|
| GAP-16 | F20 | Authority level filtering — agent trust level gates constraint visibility | P2 | COMPLETE |

### Tier 4 — Neural Re-Ranker Stack (Performance)

| Gap ID | Phase Code | Description | Priority | Status |
|--------|------------|-------------|----------|--------|
| GAP-17 | NR-1 | Training data collection — passive JSONL logging of retrieve/rerank events | P2 | COMPLETE |
| GAP-18 | NR-2 | Python neural sidecar — FastAPI service with cross-encoder rerank + NLI endpoints | P2 | COMPLETE |
| GAP-19 | NR-3 | Neural reranker Go integration — `rerank_neural.go` + sidecar client in retrieval pipeline | P2 | COMPLETE |
| GAP-20 | NR-4 | Sidecar training and evaluation — `train.py`, `evaluate.py`, Docker packaging | P3 | COMPLETE |
| GAP-21 | F21 | LLM client unification — `internal/llmclient/` package eliminates duplicated HTTP call patterns | P2 | COMPLETE |

---

## Architecture

### Constraint Enforcement Pipeline (F1)

```
Agent tool call
     │
     ▼
PreToolUse hook (pre-tool-enforce.py)
     │  calls POST /v1/memory/guardrail/validate
     │  with {space_id, diff, files_changed}
     ▼
Guardrail pipeline (internal/guardrail/)
  1. Diff parser  — regex symbol extraction (Go/Python/JS)
  2. Constraint retrieval — vector similarity + keyword match
     ├── F7: scope filter (glob match on file paths)
     └── F20: authority level filter (agent_trust_level)
  3. LLM evaluator — OpenAI/Ollama, Temperature 0.0
  4. Response builder — Block / Warning / Pass
     │
     ├── Block  → hook exits 1 → tool call aborted
     ├── Warning → hook exits 0 + injects warning text
     └── Pass   → hook exits 0 → tool call proceeds
     │
     ▼
Enforcement event logged to in-memory ring buffer (1000 events)
GET /v1/guardrail/events returns recent events
```

### Effectiveness Feedback Pipeline (F3)

```
Jiminy guide returns guidance_id (CUID2) with each response
     │
     ▼ (F10: LRU+TTL cache checked first)
Agent performs action or ignores guidance
     │
     ▼
POST /v1/jiminy/feedback {guidance_id, action_taken, session_id}
     │
     ▼
Outcome classifier: text overlap scoring + negation detection
  → "followed" | "ignored" | "contradicted" | "unknown"
     │
     ├── F3: Write GUIDANCE_OUTCOME edge to Neo4j (persistence.go)
     └── F3: Confidence updater (Bayesian):
           followed   → +CONSTRAINT_CONFIDENCE_BOOST_PER_POSITIVE (default: 0.02)
           ignored    → -CONSTRAINT_CONFIDENCE_DECAY_PER_NEGATIVE  (default: 0.03)
           contradicted → -CONSTRAINT_CONFIDENCE_DECAY_PER_NEGATIVE
           score < CONSTRAINT_ARCHIVE_THRESHOLD (0.3) → auto-archive
     │
     ▼
GET /v1/constraints/effectiveness?space_id=X
  returns per-constraint effectiveness scores
```

### Contradiction Detection (F2a/F2b)

```
POST /v1/memory/observe {content, obs_type}
     │
     ▼
Surprise scoring pipeline (conversation/surprise.go)
     │
     ├── F2a: Embedding similarity check
     │       candidates = vector recall (sim ≥ CONTRADICTION_SIM_THRESHOLD, default: 0.75)
     │       heuristic negation: "must not" / "never" / "do not" / "avoid" / "forbidden"
     │       creates CONTRADICTS edge between conflicting nodes
     │
     └── F2b: NLI path (CONTRADICTION_NLI_ENABLED=true)
             calls sidecar POST /nli {premise, hypothesis}
             returns {entailment, neutral, contradiction} probabilities
             triggers contradiction if contradiction score > 0.6
```

### Conflict Detection (F4)

```
POST /v1/constraints/detect-conflicts {space_id}
     │
     ▼
ConflictDetector (internal/hidden/constraint_conflicts.go)
  1. Fetch all constraint nodes in space (role_type='constraint')
  2. Pairwise comparison (up to CONSTRAINT_CONFLICT_MAX_PAIRS, default: 500)
  3. Embedding cosine similarity ≥ CONSTRAINT_CONFLICT_SIM_THRESHOLD (default: 0.6)
  4. Type-aware conflict heuristic:
       must vs must_not on similar topics → CONFLICTS_WITH edge
  5. CONFLICTS_WITH edge properties: similarity_score, detection_method, resolution_status="unresolved"
     │
     ▼
GET /v1/constraints/conflicts?space_id=X   — list all conflicts
PATCH /v1/constraints/conflicts/{src_id}:{tgt_id}/resolve
  {resolution_text} → resolution_status="resolved"
```

### Neural Re-Ranker Stack (NR-1 through NR-4)

```
NR-1: Training Data Collection
  Passive JSONL logging in retrieval pipeline (rerank_collector.go)
  Each retrieve event logs: query, candidates, final_ranking
  Written to NEURAL_DATA_DIR (default: .mdemg/neural/training-data/)
  Enabled via NEURAL_DATA_COLLECTION=true

NR-2: Python Neural Sidecar (neural/)
  FastAPI service on port 8100 (default)
  POST /rerank  {query, documents[]}  → {rankings: [{index, score}]}
  POST /nli     {premise, hypothesis} → {entailment, neutral, contradiction}
  GET  /health  → {status, models: {reranker, nli}}
  Docker: neural/Dockerfile (cross-encoder + NLI models via sentence-transformers)
  Tests: 33 pytest tests across test_app.py (9), test_evaluate.py (16), test_train.py (8)

NR-3: Go Integration (internal/retrieval/rerank_neural.go)
  NeuralReranker struct with HTTP client, timeout, fallback
  Replaces or supplements BM25/LLM reranking in retrieval pipeline
  Enabled via NEURAL_RERANK_ENABLED=true
  NEURAL_RERANK_URL=http://localhost:8100
  NEURAL_RERANK_TIMEOUT_MS=1000
  Falls back to NEURAL_RERANK_FALLBACK provider if sidecar unavailable

NR-4: Training & Evaluation
  neural/neural_sidecar/train.py: fine-tunes cross-encoder from collected JSONL
  neural/neural_sidecar/evaluate.py: NDCG@10, MRR, precision metrics
  packaging/autoresearch/train.py and prepare.py: AutoResearch training pipeline
```

### LLM Client Unification (F21)

`internal/llmclient/` provides a single unified HTTP client for OpenAI and Ollama APIs, eliminating the duplicated request/response types and HTTP call patterns that existed across `summarize`, `hidden`, `consulting`, `metalearn`, `ape`, and `retrieval` packages.

---

## New API Endpoints

Eight new endpoints added in FSD-2026-001:

| Endpoint | Method | Phase | Purpose |
|----------|--------|-------|---------|
| `/v1/guardrail/events` | GET | F1 | Recent enforcement events from in-memory ring buffer |
| `/v1/constraints/effectiveness` | GET | F3 | Per-constraint effectiveness scores (followed/ignored/contradicted) |
| `/v1/constraints/detect-conflicts` | POST | F4 | Run pairwise conflict detection, create CONFLICTS_WITH edges |
| `/v1/constraints/conflicts` | GET | F4 | List detected conflicts for a space |
| `/v1/constraints/conflicts/{id}/resolve` | PATCH | F4 | Mark a conflict as resolved with resolution text |
| `/v1/constraints/scope/{node_id}` | PATCH | F7 | Override a constraint's scope (file path glob pattern) |
| `/v1/metrics/determinism` | GET | F9 | Compute D-score for a space |
| `/v1/neural/status` | GET | NR-3 | Neural sidecar configuration and enablement status |

---

## New Config Parameters

38 new environment variable config parameters added in FSD-2026-001:

| Parameter | Type | Default | Phase |
|-----------|------|---------|-------|
| `GUARDRAIL_HOOK_ENABLED` | bool | false | F1 |
| `GUARDRAIL_HOOK_TIMEOUT_MS` | int | 3000 | F1 |
| `CONTRADICTION_ENABLED` | bool | true | F2a |
| `CONTRADICTION_SIM_THRESHOLD` | float64 | 0.75 | F2a |
| `CONTRADICTION_MAX_CANDIDATES` | int | 20 | F2a |
| `CONTRADICTION_NLI_ENABLED` | bool | false | F2b |
| `JIMINY_PERSISTENCE_ENABLED` | bool | false | F3 |
| `CONSTRAINT_CONFIDENCE_DECAY_PER_NEGATIVE` | float64 | 0.03 | F3 |
| `CONSTRAINT_CONFIDENCE_BOOST_PER_POSITIVE` | float64 | 0.02 | F3 |
| `CONSTRAINT_ARCHIVE_THRESHOLD` | float64 | 0.3 | F3 |
| `CONSTRAINT_CONFLICT_DETECTION_ENABLED` | bool | false | F4 |
| `CONSTRAINT_CONFLICT_SIM_THRESHOLD` | float64 | 0.6 | F4 |
| `CONSTRAINT_CONFLICT_MAX_PAIRS` | int | 500 | F4 |
| `CONSTRAINT_CLASSIFIER_GATE_ENABLED` | bool | false | F6 |
| `CONSTRAINT_NLI_ENABLED` | bool | false | F6 |
| `CONSTRAINT_SCOPE_FILTERING_ENABLED` | bool | false | F7 |
| `LEARNING_ASYMMETRIC_ENABLED` | bool | false | F9 |
| `DETERMINISM_SCORING_ENABLED` | bool | false | F9 |
| `JIMINY_CACHE_ENABLED` | bool | true | F10 |
| `JIMINY_CACHE_TTL_SEC` | int | 300 | F10 |
| `JIMINY_CACHE_SIZE` | int | 200 | F10 |
| `JIMINY_PARTIAL_TIMEOUT_MS` | int | 2000 | F10 |
| `ACTIVATION_DIM_SEMANTIC_WEIGHT` | float64 | 0.6 | F11 |
| `ACTIVATION_DIM_TEMPORAL_WEIGHT` | float64 | 0.2 | F11 |
| `ACTIVATION_DIM_COACTIVATION_WEIGHT` | float64 | 0.2 | F11 |
| `CONSTRAINT_DECAY_ENABLED` | bool | false | F13 |
| `CONSTRAINT_DECAY_RATE_PER_WEEK` | float64 | 0.01 | F13 |
| `MAX_HOP_DEPTH` | int | 3 | F15 |
| `LEARNING_AUTO_PRUNE_EXCESS_ENABLED` | bool | false | F18 |
| `CONSTRAINT_AUTHORITY_ENABLED` | bool | false | F20 |
| `CONSTRAINT_DEFAULT_AUTHORITY` | string | "team_standard" | F20 |
| `NEURAL_DATA_COLLECTION` | bool | false | NR-1 |
| `NEURAL_DATA_DIR` | string | ".mdemg/neural/training-data" | NR-1 |
| `SIDECAR_ENABLED` | bool | false | NR-2 |
| `NEURAL_RERANK_ENABLED` | bool | false | NR-3 |
| `NEURAL_RERANK_URL` | string | "http://localhost:8100" | NR-3 |
| `NEURAL_RERANK_TIMEOUT_MS` | int | 1000 | NR-3 |
| `NEURAL_RERANK_FALLBACK` | string | (from RERANK_PROVIDER) | NR-3 |

---

## Migrations

### V0020: Constraint Lifecycle (`migrations/V0020__constraint_lifecycle.cypher`)

Adds the full constraint lifecycle schema:

1. **GUIDANCE_OUTCOME relationship indexes**: Three indexes on `space_id`, `outcome_type`, and `guidance_id` for efficient outcome queries.

2. **Constraint node lifecycle properties**: Backfills `detection_confidence`, `scope`, `authority_level`, `last_surfaced_at`, `effectiveness_score`, `total_surfaced`, `total_followed`, `total_ignored`, `total_contradicted` on all existing `role_type='constraint'` nodes. Defaults: `detection_confidence=1.0`, `scope='space'`, `authority_level='inferred'`.

3. **CO_ACTIVATED_WITH direction property**: Adds `direction='bidirectional'` to all existing edges as default. Enables asymmetric Hebbian learning (F9) without breaking backward compatibility.

4. **Scope and authority indexes**: Composite indexes on `(space_id, scope)` and `(space_id, authority_level)` for efficient constraint filtering.

### V0021: Constraint Conflicts (`migrations/V0021__constraint_conflicts.cypher`)

Adds the conflict detection schema:

1. **CONFLICTS_WITH resolution index**: Index on `resolution_status` for filtered conflict queries.

2. **Constraint decay properties**: Backfills `expires_at=null` and `decay_rate=0.0` on existing constraint nodes (F13 preparation).

---

## File Inventory

### New Files (FSD-2026-001)

**API Handlers** (`internal/api/`):
- `handlers_enforcement.go` — `GET /v1/guardrail/events`, enforcement event ring buffer
- `handlers_guardrail.go` — updated with F7 scope and F20 authority level wiring
- `handlers_constraint_conflicts.go` — `POST /v1/constraints/detect-conflicts`, `GET /v1/constraints/conflicts`, `PATCH /v1/constraints/conflicts/{id}/resolve`
- `handlers_constraint_metrics.go` — `GET /v1/constraints/effectiveness`
- `handlers_constraint_scope.go` — `PATCH /v1/constraints/scope/{node_id}`
- `handlers_determinism.go` — `GET /v1/metrics/determinism`
- `handlers_neural.go` — `GET /v1/neural/status`

**Jiminy** (`internal/jiminy/`):
- `cache.go` — F10: LRU + TTL cache for `GuidanceCache`, `CacheKey` hash-based lookup
- `persistence.go` — F3: Neo4j write-through store for GUIDANCE_OUTCOME edges
- `confidence_updater.go` — F3: Bayesian confidence updates on constraint nodes

**Guardrail** (`internal/guardrail/`):
- `constraint_retrieval.go` — F7/F20: scope-aware and authority-filtered constraint retrieval

**Hidden Layer** (`internal/hidden/`):
- `constraint_conflicts.go` — F4: `ConflictDetector` struct, `DetectConflicts()`, `ListConflicts()`, `ResolveConflict()`

**Metrics** (`internal/metrics/`):
- `determinism.go` — F9: `DeterminismScore` struct and `ComputeDeterminismScore()` Cypher query

**Sanitize** (`internal/sanitize/`):
- `sanitize.go` — F14: `SanitizePromptInput()` for injection pattern removal, wired into synthesis
- `sanitize_test.go` — unit tests

**Conversation** (`internal/conversation/`):
- `nli_classifier.go` — F6: `NLIClassifier` interface, sidecar client integration for contradiction classification
- `nli_client.go` — HTTP client for sidecar NLI endpoint

**Retrieval** (`internal/retrieval/`):
- `rerank_neural.go` — NR-3: `NeuralReranker` struct with HTTP client, timeout, and fallback logic
- `rerank_collector.go` — NR-1: `RerankDataCollector` for passive JSONL training data logging

**LLM Client** (`internal/llmclient/`):
- `client.go` — F21: Unified LLM client for OpenAI and Ollama (chat completions + generate APIs)
- `types.go` — F21: Shared request/response types (`Message`, `OpenAIChatRequest`, `OllamaGenerateRequest`, etc.)

**Neural Sidecar** (`neural/`):
- `neural_sidecar/app.py` — NR-2: FastAPI application with `/rerank`, `/nli`, `/health` routes
- `neural_sidecar/reranker.py` — NR-2: Cross-encoder reranker using sentence-transformers
- `neural_sidecar/nli.py` — NR-2: NLI model wrapper
- `neural_sidecar/schemas.py` — NR-2: Pydantic request/response schemas
- `neural_sidecar/config.py` — NR-2: Configuration (model names, ports)
- `neural_sidecar/train.py` — NR-4: Fine-tuning from collected JSONL data
- `neural_sidecar/evaluate.py` — NR-4: NDCG@10, MRR, precision metrics
- `neural/Dockerfile` — NR-2: Container packaging
- `neural/pyproject.toml` — NR-2: Python project metadata
- `tests/test_app.py` (9 tests) — NR-2: FastAPI endpoint tests
- `tests/test_train.py` (8 tests) — NR-4: Training pipeline tests
- `tests/test_evaluate.py` (16 tests) — NR-4: Evaluation metrics tests

**Migrations**:
- `migrations/V0020__constraint_lifecycle.cypher`
- `migrations/V0021__constraint_conflicts.cypher`

**Scripts**:
- `scripts/fsd-acceptance.sh` — end-to-end acceptance test for FSD-2026-001

### Modified Files

| File | Changes |
|------|---------|
| `internal/config/config.go` | 38 new config parameters (F1–F21, NR-1 through NR-4) |
| `internal/api/server.go` | FSD wiring: enforcement log, conflict detector, F6/F7/F20 middleware |
| `internal/guardrail/guardrail.go` | F7 scope filtering, F20 authority level filtering in validate pipeline |
| `internal/jiminy/service.go` | F3 persistence + confidence updater wiring, F10 cache integration |
| `internal/jiminy/types.go` | F3 `GuidanceOutcomeResult`, `ConstraintEffectivenessResult` types |
| `internal/hidden/service.go` | F18: `EdgePruner` wired as Step 6 in consolidation pipeline |
| `internal/hidden/constraint_nodes.go` | F7: scope inference, F20: authority_level on new constraints |
| `internal/conversation/contradiction.go` | F2a: embedding similarity + heuristic negation detection |
| `internal/conversation/surprise.go` | F2a: contradiction scoring integrated into surprise pipeline |
| `internal/consulting/synthesis.go` | F14: `SanitizePromptInput()` applied before LLM synthesis |
| `internal/retrieval/activation.go` | F11: configurable dimension weights (semantic/temporal/coactivation) |
| `internal/retrieval/service.go` | F9: direction-aware weight scaling for asymmetric edges; NR-3 integration |
| `internal/retrieval/rerank.go` | NR-3: NeuralReranker option in reranking pipeline |
| `internal/models/models.go` | F20: `AgentTrustLevel` field in `GuardrailRequest` |
| `.claude/hooks/pre-tool-enforce.py` | F1: PreToolUse hook calling guardrail validate endpoint |

---

## Test Coverage

### Go Unit Tests (42 packages)

All 42 Go test packages pass with 0 failures. Key packages with FSD-2026-001 additions:

| Package | New Tests | Phase |
|---------|-----------|-------|
| `internal/jiminy` | cache, persistence, confidence updater | F3, F10 |
| `internal/hidden` | constraint_conflicts, constraint_nodes | F4, F5 |
| `internal/metrics` | determinism scoring | F9 |
| `internal/sanitize` | injection pattern tests | F14 |
| `internal/conversation` | contradiction detection, NLI classifier | F2a, F2b |
| `internal/retrieval` | rerank_neural, rerank_collector, asymmetric scoring | NR-1, NR-3, F9 |
| `internal/llmclient` | unified client (OpenAI + Ollama paths) | F21 |
| `internal/guardrail` | scope filter, authority level filter | F7, F20 |

### Python Sidecar Tests (33 tests via pytest)

```
neural/tests/test_app.py     — 9 tests  (endpoints, validation, health)
neural/tests/test_evaluate.py — 16 tests (NDCG, MRR, precision metrics)
neural/tests/test_train.py    — 8 tests  (training pipeline, data loading)
```

### UATS Contract Tests (12 new specs, 171 total)

New UATS specs added for FSD-2026-001:

| Spec File | Endpoint | Phase |
|-----------|----------|-------|
| `guardrail_events.uats.json` | GET /v1/guardrail/events | F1 |
| `constraint_effectiveness.uats.json` | GET /v1/constraints/effectiveness | F3 |
| `constraint_conflicts_detect.uats.json` | POST /v1/constraints/detect-conflicts | F4 |
| `constraint_conflicts_list.uats.json` | GET /v1/constraints/conflicts | F4 |
| `constraint_scope_filter.uats.json` | PATCH /v1/constraints/scope/{id} | F7 |
| `determinism_score.uats.json` | GET /v1/metrics/determinism | F9 |
| `neural_status.uats.json` | GET /v1/neural/status | NR-3 |
| `neural_sidecar_health.uats.json` | GET /health (sidecar) | NR-2 |
| `neural_sidecar_rerank.uats.json` | POST /rerank (sidecar) | NR-2 |
| `neural_sidecar_nli.uats.json` | POST /nli (sidecar) | NR-2 |
| `jiminy_feedback.uats.json` | POST /v1/jiminy/feedback | F3 |
| `jiminy_feedback_persist.uats.json` | POST /v1/jiminy/feedback (persistence path) | F3 |

Total UATS: 171 specs, 297 variants, 100% pass rate (11 skipped as `llm_required` in CI — expected).

### Integration Tests

- `tests/integration/autoresearch_test.go` — AR-1/AR-2/AR-3 coverage including feedback roundtrip

### Acceptance Test

```bash
bash scripts/fsd-acceptance.sh [--binary <path>] [--base-url <url>]
```

Phases covered:
1. Prerequisites — binary and jq availability check
2. Server health — `GET /healthz`
3. Guardrail validation — `POST /v1/memory/guardrail/validate` (requires GUARDRAIL_ENABLED=true)
4. Guardrail events — `GET /v1/guardrail/events`
5. Constraint scope update — `PATCH /v1/constraints/scope/{id}`
6. Constraint effectiveness — `GET /v1/constraints/effectiveness`
7. Conflict detection — `POST /v1/constraints/detect-conflicts`
8. Conflict listing — `GET /v1/constraints/conflicts`
9. Determinism score — `GET /v1/metrics/determinism` (requires DETERMINISM_SCORING_ENABLED=true)
10. Neural status — `GET /v1/neural/status`

---

## Acceptance Criteria

- **AC-1**: `POST /v1/memory/guardrail/validate` returns Block/Warning/Pass with file-scoped constraint filtering when `CONSTRAINT_SCOPE_FILTERING_ENABLED=true`.
- **AC-2**: `GET /v1/guardrail/events` returns the last N enforcement events from the in-memory log.
- **AC-3**: `POST /v1/jiminy/feedback` with a `guidance_id` persists a `GUIDANCE_OUTCOME` edge to Neo4j when `JIMINY_PERSISTENCE_ENABLED=true`.
- **AC-4**: Constraint `effectiveness_score` and counter fields (`total_followed`, `total_ignored`, `total_contradicted`) update after feedback submission.
- **AC-5**: `GET /v1/constraints/effectiveness` returns per-constraint scores ordered by surfacing frequency.
- **AC-6**: `POST /v1/constraints/detect-conflicts` creates `CONFLICTS_WITH` edges for pairwise similar constraints and returns a `new_conflicts` count.
- **AC-7**: `PATCH /v1/constraints/conflicts/{id}/resolve` marks a conflict as `resolution_status="resolved"`.
- **AC-8**: `GET /v1/metrics/determinism` returns a D-score with `informed_actions`, `compliance_rate`, and `coverage_ratio` components when `DETERMINISM_SCORING_ENABLED=true`.
- **AC-9**: Neural sidecar responds to `POST /rerank` and `POST /nli` when `SIDECAR_ENABLED=true`.
- **AC-10**: `GET /v1/neural/status` reflects `NEURAL_RERANK_ENABLED`, `NEURAL_DATA_COLLECTION`, and sidecar URL config.
- **AC-11**: V0020 and V0021 migrations apply cleanly via `mdemg db migrate` with no errors.
- **AC-12**: 171 UATS specs pass at 100% (excluding `llm_required` skips).
- **AC-13**: `golangci-lint run ./...` reports 0 issues.
- **AC-14**: `pytest neural/tests/` reports 33 passed, 0 failed.

---

## Design Decisions

1. **All FSD features are opt-in via config flags**: Every new behavior defaults to `false` or its safest value. Operators enable individual features incrementally. This preserves full backward compatibility — a deployment without any FSD env vars behaves identically to pre-FSD.

2. **Enforcement event log is in-memory only**: The ring buffer (1000 events) is not persisted to Neo4j. This is intentional — enforcement events are high-frequency operational data, not long-term memory. They are observable for debugging without polluting the knowledge graph.

3. **GUIDANCE_OUTCOME edges are space-scoped**: The `space_id` property on edges enables multi-space effectiveness queries without cross-space contamination.

4. **Contradiction detection is non-blocking**: The contradiction pipeline runs asynchronously within surprise scoring. It adds CONTRADICTS edges but does not reject the observation being ingested. This is by design — the system records the contradiction as a signal rather than enforcing immediate resolution.

5. **Neural sidecar is a separate process**: The Python sidecar runs on port 8100 and is not embedded in the Go binary. This allows independent scaling, model updates, and zero Go build dependencies on Python/PyTorch.

6. **LLM client unification (F21) is additive**: The new `internal/llmclient/` package provides shared types. Existing packages were not forcibly migrated — only new code in this FSD uses it. Gradual migration is deferred.

---

## Dependencies

- Requires: Phase 104 (Active MCP Guardrails) — guardrail pipeline foundation
- Requires: Phase Jiminy — guidance service foundation and `guidance_id` tracking
- Requires: Phase AR-2 — Jiminy effectiveness tracking base (feedback endpoint)
- Requires: V0019 migration — performance indexes for constraint query efficiency
- Neural sidecar: Python 3.11+, `sentence-transformers`, `fastapi`, `uvicorn` (see `neural/pyproject.toml`)

---

## Documents Accessed

- `AGENT_HANDOFF.md` — Phase FSD-2026-001 session summary (lines 44-200)
- `CHANGELOG.md` — FSD-2026-001 entry format and prior phase patterns
- `docs/specs/phase105-global-meta-learning.md` — spec format reference
- `docs/specs/phase97-process-lifecycle-security.md` — spec format reference
- `migrations/V0020__constraint_lifecycle.cypher` — migration details
- `migrations/V0021__constraint_conflicts.cypher` — migration details
- `internal/config/config.go` — all 38 FSD config parameters (lines 529–603)
- `internal/api/handlers_enforcement.go` — enforcement event log and ring buffer
- `internal/api/handlers_constraint_conflicts.go` — conflict CRUD endpoints
- `internal/api/handlers_constraint_scope.go` — scope update endpoint
- `internal/api/handlers_determinism.go` — determinism score endpoint
- `internal/api/handlers_constraint_metrics.go` — effectiveness endpoint
- `internal/api/handlers_neural.go` — neural status endpoint
- `internal/guardrail/guardrail.go` — F7/F20 filtering in validation pipeline
- `internal/hidden/constraint_conflicts.go` — ConflictDetector types and methods
- `internal/metrics/determinism.go` — DeterminismScore and D-score formula
- `internal/jiminy/cache.go` — GuidanceCache LRU+TTL
- `internal/llmclient/types.go` — unified LLM client type definitions
- `neural/neural_sidecar/app.py` — FastAPI sidecar application
- `neural/tests/test_app.py` — sidecar API tests
- `scripts/fsd-acceptance.sh` — acceptance test phases
- `docs/api/api-spec/uats/specs/` — FSD UATS spec files (12 new specs)
