# Architecture Map Optimization Report

**Date**: 2026-03-24T23:08:32.889276Z  
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
| svc_external | SVC | 287 | -40% | 9.0 |

## Question-Level Detail (Final Iteration)


### svc_external

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 9 PASS | What port does the MDEMG server listen on? | The agent correctly identified the port number and its association with the MDEMG server but added extra context not present in the ground truth. |
| 2 | factual | 9 PASS | What port does the neural sidecar use? | The agent correctly identified the port number 8100 and its use with HTTP, but did not explicitly state the port number alone as the ground truth did. |
| 3 | factual | 9 PASS | How does the MCP server communicate? | The agent correctly states communication via stdio as a subprocess for IDE integration but adds the MCP protocol detail, which is not in the ground truth. |
| 4 | relational | 10 PASS | What gRPC services are defined for content ingestion? | The agent correctly identified the IngestionModule and its three operations Matches, Parse, and Sync, matching the ground truth perfectly. |
| 5 | factual | 8 PASS | What does the ReasoningModule gRPC service do? | The answer correctly identifies the rerank operation as reordering retrieval candidates by relevance, but adds unnecessary detail about the ReasoningModule gRPC service not present in the ground truth. |
