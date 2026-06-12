# Architecture Map Optimization Report

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Date**: 2026-03-24T22:52:55.110481Z  
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
| flow_retrieval | FLOW | 181 | -7% | 9.2 |
| schema_neo4j | SCHEMA | 351 | -3% | 9.4 |
| uxts_frameworks | UXTS | 253 | 83% | 8.0 |
| dict_pkg_codes | DICT | 489 | -2% | 9.2 |
| dep_pkg_graph | DEP | 292 | -93% | 8.2 |
| flow_observe_learn_consolidate | FLOW | 260 | -6% | 9.4 |
| flow_jiminy_guide | FLOW | 204 | -11% | 9.0 |
| flow_rsic_cycle | FLOW | 230 | -8% | 9.8 |
| svc_external | SVC | 244 | -19% | 8.6 |
| dist_channels | DIST | 299 | -31% | 9.4 |

## Question-Level Detail (Final Iteration)


### flow_retrieval

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | What are the retrieval scoring weights? | The agent's answer perfectly matches the ground truth with all weights correctly listed and nothing missing. |
| 2 | factual | 9 PASS | What is the embedding dimension? | The agent correctly identified the embedding dimension as 3072 but did not provide additional context or details. |
| 3 | relational | 9 PASS | What training data does the retrieval pipeline produce? | The answer correctly describes the JSONL format and the component ret.CollectTraining but slightly omits explicitly stating the tuples are for neural sidecar cross-encoder training. |
| 4 | inferential | 8 PASS | If symbol search finds candidates, what triggered it? | The agent correctly identifies that the presence of symbols in the query triggers symbol search, but does not explicitly state the key fact that the query contains the #symbol prefix. |
| 5 | factual | 10 PASS | What is the max graph expansion depth? | The agent correctly identified the max graph expansion depth as 3 and referenced the specific step where it is specified. |

### schema_neo4j

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | What edge type represents Hebbian learning? | The agent correctly identified the edge type as CO_ACTIVATED_WITH and provided an accurate description related to Hebbian learning. |
| 2 | relational | 9 PASS | What prevents grounding loss in L5 concepts? | The agent correctly identifies the GROUNDED_BY edge as a skip connection from L5 to L0 preventing grounding loss, but slightly rephrases the original fact. |
| 3 | factual | 10 PASS | What are the DBSCAN eps values for L0 and L4? | The agent's answer matches the ground truth exactly with all key facts correctly stated. |
| 4 | factual | 9 PASS | Which node label stores LLM-named emergent concepts? | The agent correctly identified the node label as "EmergentConcept" but added unnecessary detail about LLM-naming that was not in the ground truth. |
| 5 | relational | 9 PASS | What does the GUIDANCE_OUTCOME edge track? | The agent correctly identifies the possible outcomes of Jiminy guidance but adds the specific term "GUIDANCE_OUTCOME edge," which is not in the ground truth. |

### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all four frameworks and their associated purposes, matching the ground truth completely. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly stated the number of specs and variants but omitted the critical fact about the 318 test cases. |
| 3 | relational | 9 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner is demoted due to missing inline grading (GAP-04) and its impact on CI gating, but slightly rephrases the reason without explicitly mentioning "would produce false passes." |
| 4 | factual | 4 **WEAK** | Which framework has no runner at all? | The agent mentions UAMS but adds incorrect details about "runner:UNBUILT" and misses the key fact "GAP-21." |
| 5 | factual | 8 PASS | What is the UETS framework for? | The agent correctly identifies the UETS framework's purpose related to LLM emergence and concept-naming quality but adds an unclear abbreviation and slightly alters phrasing. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all frameworks using soft-fail CI gating with accurate descriptions and no omissions. |

### dict_pkg_codes

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | What does the 'ret' package code stand for? | The agent correctly identified all components of the retrieval pipeline and their association with the 'ret' package code. |
| 2 | factual | 9 PASS | How many files and lines of code are in the API package? | The agent correctly states the number of files and lines of code and specifies the package, but the ground truth does not mention the package name. |
| 3 | factual | 8 PASS | What does the 'hid' package handle? | The agent correctly identifies DBSCAN clustering, message passing, and emergence as hidden layer functions but adds an unnecessary detail about the 'hid' package. |
| 4 | inferential | 10 PASS | Which package has the most files? | The agent correctly identified the cli package and the exact number of files, fully matching the ground truth. |
| 5 | factual | 9 PASS | What embedding dimensions does the emb package use? | The agent correctly identified the 3072-dimensional vectors but added an unnecessary detail about the "emb package" not present in the ground truth. |
| 6 | factual | 9 PASS | What is the package code for the Jiminy inner voice? | The agent correctly identified the package code as "jim" but added unnecessary context not present in the ground truth. |

### dep_pkg_graph

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which packages are leaf packages with no dependencies? | The agent correctly listed all the leaf packages with no dependencies exactly as in the ground truth. |
| 2 | factual | 9 PASS | What does the api package depend on? | The agent's answer is accurate and complete except for the omission of the package "lrn" in the list. |
| 3 | relational | 5 **WEAK** | Why does ret depend on plg? | The agent mentions the "MatchIngestionModule-fallback" functionality related to routing unrecognized file types, which is relevant, but does not clearly state the core functionality of the match-module itself as in the ground truth. |
| 4 | relational | 9 PASS | Which package does hid depend on for emergence naming? | The agent correctly identifies the dependency of the package **hid** on **llm** for emergence naming but adds unnecessary detail about the package name. |
| 5 | relational | 8 PASS | What does grd depend on emb for? | The agent correctly identifies embedding-based similarity for constraint retrieval but uses unclear abbreviations and lacks clarity. |

### flow_observe_learn_consolidate

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | What does the observe endpoint return? | The agent correctly identified all required fields (obs_id, node_id, surprise_score) exactly as in the ground truth. |
| 2 | relational | 9 PASS | What triggers Hebbian learning? | The agent correctly identifies co-retrieval activating learning and CO_ACTIVATED_WITH edge creation, adding relevant details about Hebbian learning and implementation specifics. |
| 3 | factual | 10 PASS | How many phases does the consolidation pipeline have? | The agent correctly states the exact number of phases and identifies the relevant pipeline, fully matching the ground truth. |
| 4 | factual | 10 PASS | What is the DBSCAN eps scaling range across layers? | The agent's answer correctly states the DBSCAN eps scaling range as 0.10 to 0.26, matching the ground truth exactly. |
| 5 | factual | 8 PASS | What is Phase 103 in the consolidation pipeline? | The agent correctly identifies the DynamicEmergence phase and its role in LLM-named concepts emerging but adds an unnecessary detail about "Phase 103 in the consolidation pipeline" not present in the ground truth. |

### flow_jiminy_guide

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 9 PASS | What triggers the Jiminy guide flow? | The agent correctly identifies the hook and script path and that it runs on every prompt, but adds an unnecessary detail about the "Jiminy guide flow" not present in the ground truth. |
| 2 | factual | 9 PASS | What is the timeout for the guide orchestration? | The agent correctly states the timeout duration and context but adds unnecessary detail beyond the exact answer. |
| 3 | factual | 10 PASS | What are the three J17 encoding tiers and their approximate token sizes? | The agent correctly identified all three tiers with accurate token sizes, traffic percentages, and descriptions matching the ground truth. |
| 4 | relational | 10 PASS | What parallel sources does the guide query? | The agent's answer accurately includes all key elements from the ground truth with correct terminology and no missing facts. |
| 5 | relational | 7 PASS | What does jim.Effectiveness track? | The agent correctly identifies the role of guidance_id in tracking outcomes but adds unnecessary detail and misses the concise focus on feedback correlation. |

### flow_rsic_cycle

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | What are the 5 stages of the RSIC cycle? | The agent correctly listed all five stages of the RSIC cycle in the proper order with no omissions. |
| 2 | factual | 9 PASS | How many action types can Dispatch execute? | The agent correctly identifies the number of action types but adds an unnecessary detail about "Dispatch" that is not in the ground truth. |
| 3 | factual | 10 PASS | What are the three RSIC tiers and their intervals? | The agent correctly stated all three RSIC tiers and their corresponding time intervals exactly as in the ground truth. |
| 4 | factual | 10 PASS | What safety mechanisms does RSIC use? | The agent correctly listed all the safety mechanisms exactly as in the ground truth. |
| 5 | factual | 10 PASS | What are the 7 health dimensions assessed in Stage 1? | The agent correctly listed all seven health dimensions exactly as in the ground truth. |

### svc_external

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 9 PASS | What port does the MDEMG server listen on? | The agent correctly identified the port number and its purpose but added extra context not present in the ground truth. |
| 2 | factual | 9 PASS | What port does the neural sidecar use? | The agent correctly identified the port number 8100 and its use with HTTP, but did not explicitly state the port number alone as the answer. |
| 3 | factual | 8 PASS | How does the MCP server communicate? | The agent correctly states communication via stdio as a subprocess for IDE integration but adds the MCP protocol detail, which is not in the ground truth. |
| 4 | relational | 9 PASS | What gRPC services are defined for content ingestion? | The agent correctly identified the IngestionModule and its operations but added an extra detail about scope not present in the ground truth. |
| 5 | factual | 8 PASS | What does the ReasoningModule gRPC service do? | The answer correctly identifies the rerank operation as re-scoring retrieval candidates and adds the detail of reordering by relevance, but it introduces extra specifics about the ReasoningModule gRPC service not present in the ground truth. |

### dist_channels

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | How do you install MDEMG on macOS? | The agent provided the exact correct command along with accurate additional context about the installation method. |
| 2 | factual | 10 PASS | What technology is the Linux companion app built with? | The agent's answer correctly identifies Tauri (Rust + JS) and the Catppuccin theme, matching the ground truth perfectly. |
| 3 | factual | 9 PASS | Which platform companion app is not implemented? | The agent correctly identifies the windows companion app and its not-implemented status with the GAP-13 reference, but the phrasing is slightly less direct than the ground truth. |
| 4 | relational | 9 PASS | What CI workflow handles cross-compilation releases? | The answer correctly identifies release.yml, tag push trigger, and goreleaser usage but adds extra detail about GitHub Releases not mentioned in the ground truth. |
| 5 | factual | 9 PASS | What does auto-pr.yml do? | The agent correctly describes the auto-creation of PRs to main on pushes to *_dev* branches but adds unnecessary detail about the workflow name. |
