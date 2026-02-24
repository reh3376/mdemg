# MDEMG X Post Metrics - Data Collection

**Date:** 2026-01-21
**Codebase:** whk-wms (Whiskey House Warehouse Management System)

---

## Codebase Statistics

| Metric | Value | Source |
|--------|-------|--------|
| **REPO_NAME** | whk-wms | N/A |
| **FILES_COUNT** | 3,314 | find (ts, tsx, json, md, prisma) |
| **LOC_COUNT** | 871,746 | wc -l all files |
| **TOKENS_OR_CHUNKS** | 8,904 | ingest-codebase elements |

### Lines of Code Breakdown

| Type | Lines | Files |
|------|-------|-------|
| .ts | 506,914 | 2,199 |
| .tsx | 165,021 | 558 |
| .json | 77,263 | 62 |
| .md | 119,649 | 494 |
| .prisma | 2,899 | 1 |

## Hardware Specifications

| Metric | Value |
|--------|-------|
| **HW_CPU_RAM_GPU** | MacBook Pro M4 Max (16-core), 64GB RAM |

---

## Ingestion Metrics

| Metric | Value | Notes |
|--------|-------|-------|
| **NODES_COUNT** | 8,906 | MemoryNode count in Neo4j |
| **EDGES_COUNT** | 94,654 | CO_ACTIVATED_WITH + ASSOCIATED_WITH |
| **INGEST_WALL_TIME** | 23m 1s | 6.4 elements/sec |
| **Hidden Nodes Created** | 1 | From consolidation |
| **Concept Nodes Updated** | 4 | DBSCAN clustering |
| **Embedding Provider** | OpenAI text-embedding-ada-002 | 1536 dimensions |
| **Vector Index** | ONLINE (100%) | memNodeEmbedding |

---

## Comparison Test Metrics (v3 - 2026-01-22)

### Accuracy Results

| Category | Baseline | MDEMG | Delta |
|----------|----------|-------|-------|
| architecture_structure | 0.5/20 | 10.5/20 | **+10.0** |
| service_relationships | 1.0/20 | 13.5/20 | **+12.5** |
| business_logic_constraints | 0.5/20 | 12.0/20 | **+11.5** |
| data_flow_integration | 1.5/20 | 13.5/20 | **+12.0** |
| cross_cutting_concerns | 1.5/20 | 12.5/20 | **+11.0** |
| **TOTAL** | **5.0/100** | **62.0/100** | **+57.0** |

### Score Distribution

| Score Type | Baseline | MDEMG |
|------------|----------|-------|
| Completely Correct (1.0) | 0 | 28 |
| Partially Correct (0.5) | 10 | 62 |
| Unable to Answer (0.0) | 90 | 10 |
| Confidently Wrong (-1.0) | 0 | 0 |

### Key Finding

**MDEMG achieved 12.4x better accuracy than baseline** despite:

- Baseline having direct file access (but context-limited)
- MDEMG only having file paths from API (no content summaries)

---

## X Post Variables (to be filled)

### Post 3/5 - Concrete Capability

```
{{REPO_NAME}} = whk-wms
{{FILES_COUNT}} = 3,314
{{LOC_COUNT}} = 871,746
{{TOKENS_OR_CHUNKS}} = 8,904 chunks
{{NODES_COUNT}} = 8,906
{{EDGES_COUNT}} = 94,654
{{INGEST_WALL_TIME}} = 23 min
{{HW_CPU_RAM_GPU}} = MacBook Pro M4 Max, 64GB RAM
{{LINK_LANDING_PAGE}} = [TBD]
```

### Post 4/5 - Benchmarks

```
{{REPEAT_Q_DELTA}} = _TBD_ (need multi-turn test)
{{N_TURNS}} = _TBD_
{{DECISION_PERSISTENCE_DELTA}} = _TBD_
{{REGRESSION_DELTA}} = _TBD_
```

### Post 5/5 - Compute Ask

```
{{GPU_HOURS_RANGE}} = _TBD_
{{SCALE_TARGET_1_EG_10M_nodes}} = 10M
{{SCALE_TARGET_2_EG_100M_nodes}} = 100M
```

---

## Notes

1. Current test compares single-turn question answering
2. Additional tests needed for:
   - Repeat-question rate (multi-turn conversation)
   - Decision persistence (long task chains)
   - Regression rate (code changes over time)
3. Token counting requires API usage tracking
4. Embedding costs not included in current metrics

---

## Raw Data Sources

- File list: `/Users/reh3376/mdemg/docs/tests/whk-wms-file-list.txt`
- Questions: `/Users/reh3376/mdemg/docs/tests/test_questions_100.json`
- **v3 Comparison**: `/Users/reh3376/mdemg/docs/tests/comparison-2026-01-22-v3.md`
- Baseline output: `/private/tmp/claude/-Users-reh3376-mdemg/tasks/a75cf5d.output`
- MDEMG output: `/private/tmp/claude/-Users-reh3376-mdemg/tasks/a791d82.output`

### Previous Tests (v2)

- Baseline v2: `/Users/reh3376/mdemg/docs/tests/baseline-test-2026-01-21-v2.md`
- MDEMG v2: `/Users/reh3376/mdemg/docs/tests/mdemg-test-2026-01-21-v2.md`
- Comparison v2: `/Users/reh3376/mdemg/docs/tests/comparison-2026-01-21-v2.md`
