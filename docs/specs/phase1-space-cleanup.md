# Feature Spec: Phase 1 — Space Cleanup

**Phase**: Phase 1
**Status**: Implemented
**Author**: reh3376 & Claude (gMEM-dev)
**Date**: 2026-02-04

---

## Overview

Clear all ingested codebase spaces from Neo4j to start fresh, preserving only the `mdemg-dev` protected space (CMS memory).

## Requirements

### Functional Requirements

1. FR-1: Discover all distinct `space_id` values and node counts
2. FR-2: Delete all nodes from non-protected spaces
3. FR-3: Preserve `mdemg-dev` space entirely (nodes, edges, learning data)
4. FR-4: Verify post-cleanup state

### Non-Functional Requirements

1. NFR-1: Cleanup completes without timeout (batch deletion)
2. NFR-2: No data corruption in protected space

## Pre-Cleanup State

| Space | Nodes | Action |
|-------|-------|--------|
| pytorch-benchmark-v3 | 150,575 | DELETE |
| pytorch-benchmark-v4 | 129,542 | DELETE |
| pytorch-benchmark | 110,771 | DELETE |
| pytorch-benchmark-v2 | 110,771 | DELETE |
| whk-wms-v2 | 32,096 | DELETE |
| whk-wms | 30,475 | DELETE |
| mdemg-dev | 2,789 | PRESERVE |
| mdemg-self | 2,635 | DELETE |
| pytorch-small-test | 1,749 | DELETE |
| whk-wms-test | 1,057 | DELETE |
| whk-wms-finance-test | 439 | DELETE |
| (20+ test/UATS spaces) | ~100 | DELETE |
| **Total** | **573,225** | |

## Post-Cleanup State

- **Deleted**: 570,436 nodes
- **Remaining**: 2,789 nodes (mdemg-dev only)
- **mdemg-dev stats**:
  - Memory count: 1,311
  - Observation count: 1,261
  - Embedding coverage: 100%
  - Learning edges: 1,326 (phase: learning)
  - Health score: 0.986
  - Layers: L0=1,254, L1=54, L2=3

## Command Used

```bash
go run ./cmd/reset-db --all --yes
```

## Acceptance Criteria

- [x] AC-1: All non-protected spaces cleared (0 nodes)
- [x] AC-2: mdemg-dev space intact with all nodes
- [x] AC-3: Learning edges preserved
- [x] AC-4: Health score stable
- [x] AC-5: SHA256 hash added to manifest
