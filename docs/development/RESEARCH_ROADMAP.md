# MDEMG Research Roadmap

**Document Version:** 1.2
**Last Updated:** 2026-01-21
**Status:** Active Development - Phases 1 & 2 Implementation Complete

---

## Vision Statement

Transform MDEMG from an inference-time memory augmentation system into a **native long-term persistent memory harness** for frontier AI models. The end goal is a memory system that:

1. Models are **trained on** (via extensive interaction examples in training data)
2. Provides **internal dialog** capabilities during inference
3. Stores **context-specific concepts** (not general knowledge LLMs already possess)
4. **Intercepts and corrects** agent outputs against organizational standards
5. Uses **hierarchical graph convolution** for generalized concept representations

---

## Architecture Overview

### Current State (Inference-Time Augmentation)

```
[LLM] ←── context injection ←── [MDEMG] ←── [Neo4j Graph]
  │                                              ↑
  └──────── save insights ─────────────────────→┘
```

### Target State (Native Memory Harness)

```
┌─────────────────────────────────────────────────────────────────────┐
│                     Training Phase                                  │
│  [Base Model] + [Extensive MDEMG interaction examples]              │
│         ↓                                                           │
│  [Fine-tuned Model with native MDEMG understanding]                 │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│                     Inference Phase                                 │
│                                                                     │
│  [Coding Agent] ──→ [MDEMG Interceptor] ──→ [Validated Output]      │
│        │                    │                                       │
│        ↓                    ↓                                       │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                    MDEMG Graph                               │   │
│  │  Layer 2: Concepts    ←── AGGREGATES ←── Hidden Layer        │   │
│  │  Layer 1: Hidden      ←── GENERALIZES ←── Base Data          │   │
│  │  Layer 0: Base Data   (raw observations, stylesheets, etc.)  │   │
│  └─────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Progress Summary

| Workstream | Status | Completion |
|------------|--------|------------|
| 1. Hidden Layer Architecture | **COMPLETE** | 100% |
| 2. Interceptor Agent | **IMPLEMENTED** | 80% |
| 3. Training Data & Fine-Tuning | Not Started | 0% |
| 4. Research & Funding | In Progress | 10% |

---

## Workstream 1: Hidden Layer Architecture (MDEMG Core) ✅ COMPLETE

### Implementation Summary

All phases of the hidden layer architecture have been implemented:

| Phase | Description | Status |
|-------|-------------|--------|
| 1.1 | Layer Infrastructure | ✅ Complete |
| 1.2 | Hidden Node Creation (DBSCAN Clustering) | ✅ Complete |
| 1.3 | Forward Pass (Base → Hidden → Concept) | ✅ Complete |
| 1.4 | Backward Pass (Concept → Hidden → Base) | ✅ Complete |
| 1.5 | Context-Dependent Coexistence | ✅ Complete |
| 1.6 | Consolidation CLI Integration | ✅ Complete |

### Files Created/Modified

**Schema & Types:**

- `mdemg_build/migrations/V0005__hidden_layer_support.cypher` - Schema migration
- `mdemg_build/service/internal/domain/types.go` - Updated MemoryNode struct

**Configuration:**

- `mdemg_build/service/internal/config/config.go` - Hidden layer config options

**Hidden Layer Service:**

- `mdemg_build/service/internal/hidden/types.go` - Type definitions
- `mdemg_build/service/internal/hidden/clustering.go` - DBSCAN implementation
- `mdemg_build/service/internal/hidden/service.go` - Message passing service

**CLI Integration:**

- `mdemg_build/service/cmd/consolidate/main.go` - Updated with hidden layer flags

### New Configuration Options

```bash
HIDDEN_LAYER_ENABLED=true           # Feature toggle
HIDDEN_LAYER_CLUSTER_EPS=0.3        # DBSCAN epsilon
HIDDEN_LAYER_MIN_SAMPLES=3          # DBSCAN min samples
HIDDEN_LAYER_MAX_HIDDEN=100         # Max hidden nodes per run
HIDDEN_LAYER_FORWARD_ALPHA=0.6      # Forward pass current weight
HIDDEN_LAYER_FORWARD_BETA=0.4       # Forward pass aggregated weight
HIDDEN_LAYER_BACKWARD_SELF=0.2      # Backward pass self weight
HIDDEN_LAYER_BACKWARD_BASE=0.5      # Backward pass base weight
HIDDEN_LAYER_BACKWARD_CONC=0.3      # Backward pass concept weight
```

### CLI Usage

```bash
# Full hidden layer pipeline
go run ./cmd/consolidate --space-id my-project --hidden-layer --dry-run=false

# Individual operations
go run ./cmd/consolidate --space-id my-project --hidden-layer --cluster-only
go run ./cmd/consolidate --space-id my-project --hidden-layer --forward-only
go run ./cmd/consolidate --space-id my-project --hidden-layer --backward-only

# Combined with legacy consolidation
go run ./cmd/consolidate --space-id my-project --legacy --hidden-layer
```

### Technical Details

See `docs/HIDDEN_LAYER_SPEC.md` for full technical specification including:

- Message passing algorithms
- DBSCAN clustering parameters
- Forward/backward pass formulas
- Layer traversal for retrieval

---

## Workstream 2: Interceptor Agent (aci-claude-go)

### Status: In Design

### Problem Statement

Current architecture injects memory context *before* agent execution. We need to also validate and correct agent output *after* execution against organizational standards stored in MDEMG.

### Solution: MDEMG Interceptor Agent

An agent that:

1. Intercepts raw agent output (code, thoughts)
2. Queries MDEMG for relevant concepts/standards
3. Compares output against standards using LLM
4. Generates corrections if deviations detected
5. Facilitates the "internal dialog" by recording the interaction

### Architecture

```
┌─────────────────────────────────────────────────────────────────────┐
│                        Orchestrator                                 │
│                                                                     │
│  ┌──────────┐    ┌──────────┐    ┌──────────────────┐    ┌──────┐ │
│  │ Planner  │───→│  Coder   │───→│ MDEMG Interceptor│───→│Output│ │
│  └──────────┘    └──────────┘    └──────────────────┘    └──────┘ │
│       │               │                   │                        │
│       │               │                   │                        │
│       ↓               ↓                   ↓                        │
│  ┌─────────────────────────────────────────────────────────────┐  │
│  │                         MDEMG                                │  │
│  │  ┌─────────┐  ┌─────────┐  ┌─────────┐  ┌─────────┐        │  │
│  │  │Concepts │  │ Hidden  │  │  Base   │  │ Dialog  │        │  │
│  │  │(rules)  │  │(patterns)│ │ (data)  │  │(thoughts)│        │  │
│  │  └─────────┘  └─────────┘  └─────────┘  └─────────┘        │  │
│  └─────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

### Implementation Plan

See `docs/INTERCEPTOR_DESIGN.md` for full design specification.

**Phase 2.1: Interceptor Agent Design** ✅ Complete

- [x] Define InterceptorAgent interface
- [x] Define InterceptionResult and Deviation types
- [x] Define configuration options

**Phase 2.2: Interceptor Implementation** ✅ Complete

- [x] Implement MDEMG query for relevant concepts
- [x] Implement LLM-based deviation detection
- [x] Implement correction generation
- [x] Implement internal dialog recording

**Phase 2.3: Orchestrator Integration** ✅ Complete

- [x] Add interception point after coder returns
- [x] Add configuration for interception behavior
- [ ] Implement revision loop (TODO: feedback loop with counter)

### Files Created (aci-claude-go)

- `internal/interceptor/types.go` - Type definitions for interceptor
- `internal/interceptor/interceptor.go` - Interceptor interface and implementation
- Updated `internal/agent/types.go` - Added AgentTypeInterceptor
- Updated `internal/config/config.go` - Added interceptor configuration
- Updated `internal/orchestrator/orchestrator.go` - Integrated interceptor into pipeline

---

## Workstream 3: Training Data & Model Fine-Tuning

### Status: Not Started

### Phases

**Phase 3.1: Interaction Logging** - Not Started

- Add structured logging for all MDEMG interactions
- Export in training-ready format (JSONL)
- Privacy filtering for sensitive content

**Phase 3.2: Synthetic Data Generation** - Not Started

- Design templates for MDEMG interaction scenarios
- Generate synthetic examples
- Mix with real interaction logs

**Phase 3.3: Fine-Tuning Pipeline** - Not Started (requires compute)

- Select base model
- Prepare training data
- Run fine-tuning experiments

---

## Workstream 4: Research & Funding

### Status: In Progress

### X Post Drafts

See `docs/XAI_POST_DRAFTS.md` for prepared posts targeting xAI/Elon Musk for compute funding.

### Potential Grant Programs

| Program | Focus | Relevance |
|---------|-------|-----------|
| NSF AI Institutes | Continual learning | Memory persistence |
| DARPA L2M | Lifelong Learning Machines | Directly relevant |
| ONR Science of AI | Fundamental research | Memory architectures |
| ARO AI | Cognitive architectures | Internal dialog |
| Simons Foundation | Fundamental research | Theoretical foundations |
| Allen Institute for AI | Applied AI research | Practical systems |

---

## Related Documentation

| Document | Location | Description |
|----------|----------|-------------|
| Hidden Layer Spec | `docs/HIDDEN_LAYER_SPEC.md` | Technical specification |
| Interceptor Design | `docs/INTERCEPTOR_DESIGN.md` | Interceptor architecture |
| X Post Drafts | `docs/XAI_POST_DRAFTS.md` | Funding outreach |
| Claude Mem Guide | `docs/CLAUDE_MEM_GUIDE.md` | Claude-mem integration |

---

## Changelog

| Date | Version | Changes |
|------|---------|---------|
| 2026-01-21 | 1.0 | Initial roadmap created |
| 2026-01-21 | 1.1 | Updated with Phase 1 completion, added progress summary |
| 2026-01-21 | 1.2 | Phase 2 (Interceptor) implementation complete in aci-claude-go |
