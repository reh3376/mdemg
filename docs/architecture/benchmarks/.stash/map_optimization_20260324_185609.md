# Architecture Map Optimization Report

**Date**: 2026-03-24T22:56:09.341497Z  
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
| dep_pkg_graph | DEP | 309 | -103% | 8.8 |

## Question-Level Detail (Final Iteration)


### dep_pkg_graph

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which packages are leaf packages with no dependencies? | The agent correctly listed all the leaf packages with no internal dependencies exactly as in the ground truth. |
| 2 | factual | 9 PASS | What does the api package depend on? | The agent's answer is accurate and complete except for the missing "lrn" package in the list. |
| 3 | relational | 8 PASS | Why does ret depend on plg? | The agent correctly identifies that retrieval calls MatchIngestionModule as a fallback for unrecognized file types, but does not explicitly name the module or fully explain its ingestion role. |
| 4 | relational | 9 PASS | Which package does hid depend on for emergence naming? | The agent correctly identified the dependency of the package "hid" on "llm" for emergence naming, but added unnecessary detail about the package "hid." |
| 5 | relational | 8 PASS | What does grd depend on emb for? | The agent correctly identifies embedding-based similarity for constraint retrieval but uses unclear abbreviations and lacks clarity compared to the ground truth. |
