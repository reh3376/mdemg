# Architecture Map Optimization Report

**Date**: 2026-03-24T22:56:57.490373Z  
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
| svc_external | SVC | 244 | -20% | 8.8 |

## Question-Level Detail (Final Iteration)


### svc_external

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 9 PASS | What port does the MDEMG server listen on? | The agent correctly identified the port number and its association with the MDEMG server but added extra context not present in the ground truth. |
| 2 | factual | 9 PASS | What port does the neural sidecar use? | The agent correctly identified the port number 8100 and its use for HTTP, but did not explicitly state the port number alone as the answer. |
| 3 | factual | 9 PASS | How does the MCP server communicate? | The agent correctly identifies communication via stdio as a subprocess for IDE integration, adding the MCP protocol detail which is accurate but not in the ground truth. |
| 4 | relational | 9 PASS | What gRPC services are defined for content ingestion? | The agent correctly identified the IngestionModule and its operations but added an extra detail about scope not present in the ground truth. |
| 5 | factual | 8 PASS | What does the ReasoningModule gRPC service do? | The answer correctly identifies the purpose of "Rerank" as re-scoring retrieval candidates and adds context about the ReasoningModule gRPC service, but slightly exceeds the concise scope of the ground truth. |
