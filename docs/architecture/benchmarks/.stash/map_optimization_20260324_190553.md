# Architecture Map Optimization Report

**Date**: 2026-03-24T23:05:53.605081Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 8.40 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| dep_pkg_graph | DEP | 302 | -99% | 8.4 |

## Question-Level Detail (Final Iteration)


### dep_pkg_graph

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which packages are leaf packages with no dependencies? | The agent correctly listed all the leaf packages with no dependencies, matching the ground truth exactly. |
| 2 | factual | 7 PASS | What does the api package depend on? | The agent correctly listed almost all dependencies but omitted "lrn" from the answer. |
| 3 | relational | 8 PASS | Why does ret depend on plg? | The agent correctly identifies that retrieval calls MatchIngestionModule as a fallback for unrecognized file types, but does not explicitly state the module's name or full functionality. |
| 4 | relational | 9 PASS | Which package does hid depend on for emergence naming? | The agent correctly identifies the dependency of "hid" on "llm" for emergence naming but adds unnecessary detail about the package "hid." |
| 5 | relational | 8 PASS | What does grd depend on emb for? | The agent correctly identifies embedding-based similarity for constraint retrieval but uses unclear abbreviations and lacks clarity compared to the ground truth. |
