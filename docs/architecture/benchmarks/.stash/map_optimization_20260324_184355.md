# Architecture Map Optimization Report

**Date**: 2026-03-24T22:43:55.905963Z  
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
| dep_pkg_graph | DEP | 159 | -8% | 8.2 |

## Question-Level Detail (Final Iteration)


### dep_pkg_graph

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which packages are leaf packages with no dependencies? | The agent correctly listed all the leaf packages with no dependencies exactly as in the ground truth. |
| 2 | factual | 9 PASS | What does the api package depend on? | The agent's answer is accurate and complete except for the missing comma after "cns," which is a minor detail. |
| 3 | relational | 5 **WEAK** | Why does ret depend on plg? | The agent mentions fallback functionality related to MatchIngestionModule but misses the core fact that the question is about the match-module's primary functionality. |
| 4 | relational | 9 PASS | Which package does hid depend on for emergence naming? | The agent correctly identifies "llm" as the package for emergence naming but adds unnecessary wording without additional factual content. |
| 5 | relational | 8 PASS | What does grd depend on emb for? | The agent correctly identifies the dependency on embedding-based constraint retrieval but adds unnecessary detail and slightly rephrases the core concept. |
