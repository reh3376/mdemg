# Architecture Map Optimization Report

**Date**: 2026-03-24T22:49:05.266641Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 8.20 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| dep_pkg_graph | DEP | 292 | -93% | 8.2 |

## Question-Level Detail (Final Iteration)


### dep_pkg_graph

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which packages are leaf packages with no dependencies? | The agent correctly listed all the leaf packages with no internal dependencies exactly as in the ground truth. |
| 2 | factual | 9 PASS | What does the api package depend on? | The agent's answer is accurate and complete except for missing the package "lrn" from the list. |
| 3 | relational | 5 **WEAK** | Why does ret depend on plg? | The agent mentions the fallback routing to plugins but misses explicitly stating the core functionality of the MatchIngestionModule itself. |
| 4 | relational | 9 PASS | Which package does hid depend on for emergence naming? | The agent correctly identifies the dependency of "hid" on "llm" for emergence naming but adds unnecessary detail about the package "hid." |
| 5 | relational | 8 PASS | What does grd depend on emb for? | The agent correctly identifies embedding-based similarity for constraint retrieval but uses unclear abbreviations and lacks clarity compared to the ground truth. |
