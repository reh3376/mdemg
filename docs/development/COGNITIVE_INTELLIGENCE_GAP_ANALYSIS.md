# Cognitive Intelligence Gap Analysis

**Status**: COMPLETE — All 5 gaps closed (Phases 101-105)
**Date**: 2026-02-23 (Updated: 2026-03-09)
**Related**: `VISION.md`, `AGENT_HANDOFF.md`

---

## Purpose

While recent gap analyses (Phases 81-92) have successfully hardened the test frameworks (UxTS), automated self-improvement (RSIC), and developer packaging (Deployable Package), MDEMG's core vision is to be a **Cognitive Substrate** and **Subject Matter Expert (SME) Agent**.

This gap analysis evaluates the current implementation against the long-term cognitive and reasoning goals defined in `VISION.md`, specifically focusing on the "Agent Consulting Service," "Multi-Agent Coordination," and "Emergence Principle."

---

## Gap Analysis Summary

| # | Gap | Severity | Effort | Phase | Status |
|---|-----|----------|--------|-------|--------|
| 1 | ~~Shallow SME Synthesis~~ | HIGH | L | 101 | **CLOSED** |
| 2 | ~~Query Rigidity (Intent Translation)~~ | HIGH | M | 102 | **CLOSED** |
| 3 | ~~Static Hardcoded Abstractions~~ | MEDIUM | M | 103 | **CLOSED** |
| 4 | ~~Active Guardrail Enforcement~~ | HIGH | L | 104 | **CLOSED** |
| 5 | ~~Cross-Space Collective Learning~~ | MEDIUM | XL | 105 | **CLOSED** |

---

## Gap 1: Shallow SME Synthesis (The "Consult" Gap) — CLOSED

**Severity**: HIGH | **Effort**: L | **Phase**: 101 | **Status**: Complete

### Resolution (Phase 101)

**RESOLVED.** Phase 101 upgraded the consulting service with an LLM-driven SME Synthesis Engine. The `POST /v1/memory/consult` endpoint now performs multi-hop graph traversal and LLM reasoning to synthesize retrieved nodes into coherent, context-aware answers. Implementation: `internal/consulting/synthesizer.go`, spec: `docs/specs/phase101-sme-synthesis.md`.

---

## Gap 2: Query Rigidity (Intent Translation) — CLOSED

**Severity**: HIGH | **Effort**: M | **Phase**: 102 | **Status**: Complete

### Resolution (Phase 102)

**RESOLVED.** Phase 102 added LLM query rewriting before vector embedding. Conversational queries (e.g., "Why do we use Redis?") are expanded to match declarative node text (e.g., "redis cache session store architecture decision rationale") before hitting the vector index. Implementation: `internal/retrieval/intent.go`, spec: `docs/specs/phase102-intent-translation.md`.

---

## Gap 3: Static Hardcoded Abstractions (The "Consolidation" Gap) — CLOSED

**Severity**: MEDIUM | **Effort**: M | **Phase**: 103 | **Status**: Complete

### Current State (Post-Phase 103)

**RESOLVED.** Phase 103 introduced LLM-driven dynamic concept naming. Dense `CO_ACTIVATED_WITH` clusters that don't match any hardcoded pattern (concern, config, temporal, UI, comparison, constraint) are sent to an LLM for automatic naming and classification. The pipeline step runs at phase 22 (after hardcoded patterns at phase 20, before dynamic edges at phase 25). Creates `:MemoryNode:EmergentConcept` nodes with `role_type: 'dynamic_emergent'` and LLM-proposed labels from a constrained set (pattern, principle, bridge, concern, workflow). Fail-open per cluster, idempotent via `NOT EXISTS` subquery.

### Implementation

- Pipeline step: `internal/hidden/step_dynamic_emergence.go` (phase 22, optional)
- LLM namer: `internal/hidden/emergence_namer.go` (OpenAI/Ollama, circuit breaker protected)
- Core logic: `Service.CreateDynamicEmergentNodes()` in `internal/hidden/service.go`
- Config: 8 `EMERGENCE_*` env vars, default disabled
- API: `enable_dynamic_emergence: true` in consolidate request body
- Spec: `docs/specs/phase103-dynamic-emergence.md`

### Phase 103b: Emergence Model Evaluation

- `LLM_ENDPOINT` env var decouples LLM text-generation from embeddings (`EffectiveLLMEndpoint()`)
- Ollama `format` JSON schema for grammar-constrained output
- UETS framework: 8 model specs, 7/7 passing, 5 evaluation patterns (E1-E5)
- `num_ctx` config support, `--endpoint` CLI override for remote execution
- Validated model: `llama3.2:3b` Q4_K_M (fastest latency, top name quality)
- UETS: `docs/tests/uets/`

---

## Gap 4: Active Guardrail Enforcement — CLOSED

**Severity**: HIGH | **Effort**: L | **Phase**: 104 | **Status**: Complete

### Resolution (Phase 104)

**RESOLVED.** Phase 104 added `POST /v1/memory/guardrail/validate` and an MCP `validate_changes` tool. 4-step pipeline: diff parse → constraint retrieval → LLM evaluation → response build. Fail-open design, dual provider (OpenAI/Ollama). Proactively evaluates code changes against stored constraint nodes. Implementation: `internal/guardrail/`, spec: `docs/specs/phase104-active-mcp-guardrails.md`.

---

## Gap 5: Cross-Space Collective Learning — CLOSED

**Severity**: MEDIUM | **Effort**: XL | **Phase**: 105 | **Status**: Complete

### Resolution (Phase 105)

**RESOLVED.** Phase 105 added `POST /v1/memory/meta-learn` which promotes L4/L5 concepts to the `mdemg-global` protected space via LLM generalization (strips local specifics, preserves architectural insights). `ORIGINATED_FROM` edges track provenance. Retrieval pipeline supports `include_global_space: true` for cross-space vector+BM25 search. Multi-space support added to `vectorRecall`, `BM25Search`, `fetchOutgoingEdges`. Implementation: `internal/metalearn/`, spec: `docs/specs/phase105-global-meta-learning.md`.

---

## Development Plan: Phases 101-105

| Phase | Title | Description |
|-------|-------|-------------|
| **101** | SME Synthesis Engine (**Complete**) | Upgrade `/v1/memory/consult` to use LLM-based multi-hop synthesis instead of keyword string matching. |
| **102** | Intent Translation (**Complete**) | Implement query rewriting before vector embedding to align conversational queries with declarative node text. |
| **103** | Dynamic Emergence (**Complete**) | LLM-driven concept naming for unclassified CO_ACTIVATED_WITH clusters. Pipeline step at phase 22, fail-open per cluster. |
| **103b** | Emergence Model Evaluation (**Complete**) | `LLM_ENDPOINT` config separation, UETS framework (8 model specs, 7/7 passing), `llama3.2:3b` Q4_K_M validated as default. |
| **104** | Active MCP Guardrails (**Complete**) | `POST /v1/memory/guardrail/validate` endpoint, MCP `validate_changes` tool, 4-step pipeline (diff parse → constraint retrieval → LLM eval → response build), fail-open, dual provider (OpenAI/Ollama). |
| **105** | Global Meta-Learning (**Complete**) | `POST /v1/memory/meta-learn` promotes L4/L5 concepts to `mdemg-global` space via LLM generalization. Retrieval supports `include_global_space: true` for cross-space vector+BM25 search. `ORIGINATED_FROM` edges track provenance. |
