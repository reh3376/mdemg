# Cognitive Intelligence Gap Analysis

**Status**: Active (Phases 101-103b Complete, 104-105 Planned)
**Date**: 2026-02-23 (Updated: 2026-02-24)
**Related**: `VISION.md`, `AGENT_HANDOFF.md`

---

## Purpose

While recent gap analyses (Phases 81-92) have successfully hardened the test frameworks (UxTS), automated self-improvement (RSIC), and developer packaging (Deployable Package), MDEMG's core vision is to be a **Cognitive Substrate** and **Subject Matter Expert (SME) Agent**.

This gap analysis evaluates the current implementation against the long-term cognitive and reasoning goals defined in `VISION.md`, specifically focusing on the "Agent Consulting Service," "Multi-Agent Coordination," and "Emergence Principle."

---

## Gap Analysis Summary

| # | Gap | Severity | Effort | Phase |
|---|-----|----------|--------|-------|
| 1 | Shallow SME Synthesis | HIGH | L | 101 |
| 2 | Query Rigidity (Intent Translation) | HIGH | M | 102 |
| 3 | Static Hardcoded Abstractions | MEDIUM | M | 103 |
| 4 | ~~Active Guardrail Enforcement~~ **(Closed)** | HIGH | L | 104 |
| 5 | Cross-Space Collective Learning | MEDIUM | XL | 105 |

---

## Gap 1: Shallow SME Synthesis (The "Consult" Gap)

**Severity**: HIGH | **Effort**: L | **Phase**: 101

### Current State

The `POST /v1/memory/consult` and `POST /v1/memory/suggest` endpoints use static keyword matching to classify retrieved results (e.g., if a file has "error" or "auth", it's marked as `SuggestionRisk` or `SuggestionPattern`). The response concatenates summaries from `RetrieveResult` without any true understanding or synthesis. Furthermore, the **CMS Skill Registry** (`docs/features/skill-registry.md`) relies on simple tag-based queries to recall hardcoded pointers.

### Required State

The Agent Consulting Service must act as a true SME. It should use an LLM Reasoning Module to synthesize the raw retrieved nodes into a coherent, context-aware answer that explicitly explains *why* the retrieved patterns matter to the user's current task.

### Gap Details

Currently, MDEMG returns slightly formatted search results, not true "advice." To fulfill the vision of an "Internal Dialog," the consult endpoint must perform LLM-driven multi-hop synthesis over the retrieved graph. We can leverage the existing **Meta-Cognition Enforcement** (`docs/features/meta-cognition-enforcement.md`) to dynamically trigger this deeper SME synthesis when session health drops, ensuring the agent gets real help when struggling.

---

## Gap 2: Query Rigidity (Intent Translation)

**Severity**: HIGH | **Effort**: M | **Phase**: 102

### Current State

The retrieval pipeline uses the exact user query string for vector embedding. If a user asks a conversational question ("Why do we use Redis?"), it embeds poorly against the factual, declarative statements stored in the codebase nodes.

### Required State

A Query Rewriting / Intent Extraction step before vector recall. The system should use an LLM or local model to expand queries (e.g., "Why do we use Redis?" → "redis cache session store architecture decision rationale").

### Gap Details

This was identified as a "Future" optimization in Deliverable 10.1.3 but is critical for AI agents that interact conversationally. We can build upon the **Skill Registry** infrastructure by creating a dedicated "Intent Translation" reasoning module that intercepts and expands queries before they hit the vector index.

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

## Gap 4: Active Guardrail Enforcement

**Severity**: HIGH | **Effort**: L | **Phase**: 104

### Current State

The system successfully auto-detects constraint patterns ("must", "must not", "deadline") and promotes them to **Constraint Nodes** (`docs/features/constraint-nodes.md`) during consolidation (Phase 45.5). The `Suggest()` API can surface these `ConstraintNodes` if asked. However, there is no mechanism to proactively block or warn an agent when it violates these constraints.

### Required State

Integration with the MCP server to actively evaluate an agent's proposed code changes against the active `ConstraintNodes` in the graph. If an agent attempts to use a `deprecated` API or violates an architectural `must`, the MCP server should proactively return an error or warning to the agent.

### Gap Details

MDEMG is an "Active Participant" but currently waits for the agent to call `/suggest`. Proactive guardrails are necessary for robust AI coding workflows. We have the data structure (`role_type: 'constraint'`), we just lack the pre-commit/MCP enforcement layer.

---

## Gap 5: Cross-Space Collective Learning

**Severity**: MEDIUM | **Effort**: XL | **Phase**: 105

### Current State

Learning (`CO_ACTIVATED_WITH` edges) strengthens locally within a single `space_id`. Phase 5 (DevSpace CRDT) allows merging these edges during import/export. The **L5 Emergent Layer** builds powerful meta-patterns, but they remain isolated within their origin space.

### Required State

True "Meta-Learning" or "Collective Learning Aggregation" (unchecked in `VISION.md` Phase 5). If Agent A discovers a powerful architectural pattern in Space 1 and it emerges to an L5 Concept, the system should abstract that pattern and make it available as general SME knowledge to Agent B working in an entirely different Space 2 without requiring a full DevSpace merge.

### Gap Details

The graph currently remains siloed per workspace. Universal, abstract principles (e.g. L5 nodes representing cross-domain architectures) learned in one repository cannot automatically cross-pollinate to another repository in real-time. Extending the L5 emergence logic to bridge across `space_id` boundaries will fulfill the ultimate vision of a shared organizational brain.

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
