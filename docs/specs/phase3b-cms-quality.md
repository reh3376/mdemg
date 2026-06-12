# Feature Spec: CMS Quality & Retrieval Improvements

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Phase**: Phase 3B
**Status**: Implemented
**Author**: reh3376 & Claude (gMEM-dev)
**Date**: 2026-02-04

---

## Overview

Improve CMS observation quality through multi-factor scoring, enhance resume context selection with relevance-weighted ranking, and prevent low-value duplicate observations via near-duplicate detection. These improvements increase the signal-to-noise ratio of the conversation memory system.

## Requirements

### Functional Requirements

1. FR-1: Score observations on specificity, actionability, and context-richness (0.0-1.0)
2. FR-2: Rank resume observations by composite relevance (recency + surprise + type priority + co-activation) instead of pure recency
3. FR-3: Detect near-duplicate observations (cosine similarity > 0.95) and skip/merge them
4. FR-4: Comprehensive test coverage for all CMS flows with benchmark tests

### Non-Functional Requirements

1. NFR-1: Quality scoring completes in < 5μs per observation (benchmarked: ~2.6μs)
2. NFR-2: Cosine similarity for 1536-dim vectors completes in < 2μs (benchmarked: ~1.4μs)
3. NFR-3: Dedup check adds minimal latency (single Neo4j read of 50 recent observations)
4. NFR-4: Resume ranking computed entirely in Cypher (no Go-side re-sorting needed)

## API Contract

### Quality Score (Internal — not exposed via API)

```go
type QualityScore struct {
    Overall       float64 // Weighted combination (0.0-1.0)
    Specificity   float64 // How specific vs. vague (0.0-1.0)
    Actionability float64 // How actionable the content is (0.0-1.0)
    ContextRich   float64 // How much context is provided (0.0-1.0)
}
```

### Resume Response (Enhanced)

The `score` field in `ObservationResult` is now populated with the composite relevance score from the ranking algorithm, not just a placeholder.

### Dedup Response (Internal — transparent to callers)

When a duplicate is detected, `POST /v1/conversation/observe` returns the existing node's ID with summary "duplicate observation (merged with existing)". The caller sees a normal response.

## Data Model

### Resume Ranking Formula (Cypher-computed)

```
relevanceScore = 0.40 * recencyScore
               + 0.25 * surpriseScore
               + 0.20 * typePriority
               + 0.15 * coactivationScore
```

**Components:**

- **recencyScore**: `exp(-0.029 * hours_since_creation)` — half-life ~24 hours
- **surpriseScore**: stored `surprise_score` property (0.0-1.0)
- **typePriority**: correction=1.0, decision=0.9, error/blocker=0.8, preference=0.7, learning/insight=0.6, task=0.5, technical_note=0.4, progress=0.3, context=0.2
- **coactivationScore**: `log(coact_count + 1) / log(11)` — diminishing returns via logarithm

### Quality Scoring Weights

```
overall = 0.40 * specificity + 0.35 * actionability + 0.25 * contextRichness
```

### Dedup Threshold

- Cosine similarity >= 0.95 → skip (merge with existing)
- On merge: increment `duplicate_count` property on existing node

## Test Plan

### Unit Tests

- [x] TestScoreObservationQuality_HighQuality: high-quality correction scores above threshold
- [x] TestScoreObservationQuality_LowQuality: vague content scores below threshold
- [x] TestScoreObservationQuality_TableDriven: 7 scenarios across types
- [x] TestScoreSpecificity: 5 scenarios (empty, short, vague, code identifiers, paths)
- [x] TestScoreActionability: 5 scenarios across observation types
- [x] TestScoreContextRichness: 4 scenarios (no context, tags, metadata, structured)
- [x] TestIsCodeIdentifier: 10 cases (PascalCase, camelCase, snake_case, etc.)
- [x] TestQualityScore_IsLowQuality: threshold boundary test
- [x] TestDedupAction: 5 cases (below/at/above threshold)
- [x] TestDedupResult_IsDuplicate: struct field tests
- [x] TestDedupThreshold: constant range validation
- [x] TestResumeObsTypePriority: all 11 types + unknown type
- [x] TestResumeObsTypePriority_Ordering: priority hierarchy verification
- [x] TestQualityIntegration_ObservationTypes: cross-cutting quality + type tests
- [x] TestJiminyRationale_Fields: Jiminy explanation structure
- [x] TestObserveRequestValidation: request structure tests
- [x] TestRecallRequest_TemporalFiltering: temporal fields exist

### Benchmark Tests

- [x] BenchmarkScoreObservationQuality: ~2.6μs/op
- [x] BenchmarkScoreSpecificity: 139ns-2.4μs by content length
- [x] BenchmarkGenerateSummary: ~617ns/op
- [x] BenchmarkResumeObsTypePriority: ~1ns/op
- [x] BenchmarkBuildObservationTags: ~49ns/op
- [x] BenchmarkIsCodeIdentifier: ~31ns/op
- [x] BenchmarkDedupAction: <1ns/op
- [x] BenchmarkCosineSimilarity: 296ns-1.4μs by dimension

## Acceptance Criteria

- [x] AC-1: Quality scoring produces differentiated scores across observation types
- [x] AC-2: Resume returns observations ranked by composite relevance, not just recency
- [x] AC-3: Near-duplicate observations are detected and merged transparently
- [x] AC-4: Comprehensive test coverage with table-driven tests
- [x] AC-5: Benchmark tests demonstrate sub-5μs quality scoring
- [x] AC-6: All existing tests pass (`go test ./...`)
- [x] AC-7: `go vet ./...` reports no issues
- [x] AC-8: SHA256 hash added to `docs/specs/manifest.sha256`

## Dependencies

- Depends on: Phase 3A (session tracking)
- Blocks: Phase 3C (multi-agent support)

## Files Changed

### New Files

- `internal/conversation/quality.go` — Multi-factor quality scoring (specificity, actionability, context-richness)
- `internal/conversation/quality_test.go` — 8 test functions with table-driven tests
- `internal/conversation/dedup.go` — Near-duplicate detection via cosine similarity
- `internal/conversation/dedup_test.go` — Dedup threshold and action tests
- `internal/conversation/bench_test.go` — 8 benchmark functions

### Modified Files

- `internal/conversation/service.go` — Relevance-weighted resume query, dedup integration in Observe, resumeObsTypePriority helper
- `internal/conversation/service_test.go` — Added Phase 3B tests (type priority, quality integration, Jiminy, temporal)
