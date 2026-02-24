# Cognitive Intelligence Gap Analysis

**Status**: Draft
**Date**: 2026-02-23
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
| 4 | Active Guardrail Enforcement | HIGH | L | 104 |
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

## Gap 3: Static Hardcoded Abstractions (The "Consolidation" Gap)

**Severity**: MEDIUM | **Effort**: M | **Phase**: 103

### Current State

While the **L5 Emergent Layer** (`docs/features/l5-emergent-layer.md`) successfully clusters L3+ nodes using **BRIDGES** (`docs/features/bridges-edge-type.md`), `ANALOGOUS_TO`, and `COMPOSES_WITH` edges, lower-layer nodes (ConcernNodes, ComparisonNodes, TemporalNodes) are still created during consolidation using hardcoded regex/string matching in Go (e.g., `*auth*`, `*config*`).

### Required State

LLM-driven dynamic emergence across all layers. The system should detect structurally dense clusters (via `CO_ACTIVATED_WITH` edges) that *don't* match known patterns, pass the cluster's contents to the LLM Semantic Summary Service (Phase 11.2), and ask it to invent a name and description for the newly emerged abstraction.

### Gap Details

The "Emergence Principle" in `VISION.md` states: "Let structure arise from data, don't impose it." Relying on hardcoded Go string matching limits emergence to only the concepts we pre-programmed. By injecting LLM-driven naming into the existing L5 union-find clustering logic, we can achieve true, unstructured dynamic emergence.

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
| **101** | SME Synthesis Engine | Upgrade `/v1/memory/consult` to use LLM-based multi-hop synthesis instead of keyword string matching. |
| **102** | Intent Translation | Implement query rewriting before vector embedding to align conversational queries with declarative node text. |
| **103** | Dynamic Emergence | Replace hardcoded consolidation patterns with density-based cluster detection and LLM-generated abstraction naming. |
| **104** | Active MCP Guardrails | Add pre-commit/MCP-level validation against `ConstraintNodes` to proactively block architectural violations. |
| **105** | Global Meta-Learning | Implement cross-space promotion of Layer 4/5 concepts to a global "Org-Level" graph for true cross-pollination. |
