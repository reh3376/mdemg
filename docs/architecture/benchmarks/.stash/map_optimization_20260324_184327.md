# Architecture Map Optimization Report

**Date**: 2026-03-24T22:43:27.898726Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 8.80 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| svc_external | SVC | 209 | -3% | 8.8 |

## Question-Level Detail (Final Iteration)


### svc_external

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 9 PASS | What port does the MDEMG server listen on? | The agent correctly identified the port number and its association with the MDEMG server but added extra context not present in the ground truth. |
| 2 | factual | 9 PASS | What port does the neural sidecar use? | The agent correctly identified the port number 8100 but added unnecessary context about the neural sidecar. |
| 3 | factual | 9 PASS | How does the MCP server communicate? | The agent correctly states communication via stdio and subprocess, but adds "MCP server" which is not in the ground truth. |
| 4 | relational | 9 PASS | What gRPC services are defined for content ingestion? | The agent correctly identified the IngestionModule and its operations Matches, Parse, and Sync, but added an unnecessary detail about gRPC service and content→observations notation. |
| 5 | factual | 8 PASS | What does the ReasoningModule gRPC service do? | The agent correctly explains that Rerank involves re-scoring retrieval candidates and adds the detail of reordering by relevance, which is accurate though slightly more detailed than the ground truth. |
