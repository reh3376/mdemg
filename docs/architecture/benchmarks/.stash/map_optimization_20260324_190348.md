# Architecture Map Optimization Report

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date**: 2026-03-24T23:03:48.318857Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 9.00 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| svc_external | SVC | 289 | -41% | 9.0 |

## Question-Level Detail (Final Iteration)


### svc_external

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 9 PASS | What port does the MDEMG server listen on? | The agent correctly identified the port number and its purpose but added extra context not present in the ground truth. |
| 2 | factual | 10 PASS | What port does the neural sidecar use? | The agent correctly identified the port number 8100 as used by the neural sidecar, matching the ground truth exactly. |
| 3 | factual | 9 PASS | How does the MCP server communicate? | The agent correctly identifies communication via stdio as a subprocess for IDE integration, adding the MCP protocol detail which is accurate but not in the ground truth. |
| 4 | relational | 9 PASS | What gRPC services are defined for content ingestion? | The agent correctly identified the IngestionModule and its operations but added a minor detail about its role without missing any main facts. |
| 5 | factual | 8 PASS | What does the ReasoningModule gRPC service do? | The answer correctly identifies reranking as re-scoring retrieval candidates and adds the detail of reordering by semantic relevance, but it introduces specifics about the ReasoningModule gRPC service not present in the ground truth. |
