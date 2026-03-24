# Architecture Map Optimization Report

**Date**: 2026-03-24T23:01:34.786647Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 9.23 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| flow_retrieval | FLOW | 181 | -7% | 9.4 |
| schema_neo4j | SCHEMA | 351 | -3% | 9.4 |
| uxts_frameworks | UXTS | 263 | 82% | 8.8 |
| dict_pkg_codes | DICT | 489 | -2% | 9.2 |
| dep_pkg_graph | DEP | 309 | -103% | 8.8 |
| flow_observe_learn_consolidate | FLOW | 260 | -6% | 9.6 |
| flow_jiminy_guide | FLOW | 204 | -11% | 9.2 |
| flow_rsic_cycle | FLOW | 230 | -8% | 10.0 |
| svc_external | SVC | 244 | -20% | 8.8 |
| dist_channels | DIST | 299 | -31% | 9.2 |

## Question-Level Detail (Final Iteration)


### flow_retrieval

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | What are the retrieval scoring weights? | The agent's answer perfectly matches the ground truth with all weights correctly stated and nothing missing. |
| 2 | factual | 9 PASS | What is the embedding dimension? | The agent correctly identified the embedding dimension as 3072 but did not provide any additional context or explanation. |
| 3 | relational | 9 PASS | What training data does the retrieval pipeline produce? | The answer correctly describes the format and source of the training data but slightly omits specifying that the tuples are specifically for neural sidecar cross-encoder training. |
| 4 | inferential | 9 PASS | If symbol search finds candidates, what triggered it? | The agent correctly identified that the query contained symbols triggering the search, but did not explicitly mention the "#symbol" prefix as in the ground truth. |
| 5 | factual | 10 PASS | What is the max graph expansion depth? | The agent correctly identified the max graph expansion depth as 3 and referenced the specific step where it is specified. |

### schema_neo4j

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | What edge type represents Hebbian learning? | The agent correctly identified the edge type as CO_ACTIVATED_WITH and accurately described its relation to Hebbian learning. |
| 2 | relational | 9 PASS | What prevents grounding loss in L5 concepts? | The agent correctly identifies the GROUNDED_BY edge as a skip connection from L5 to L0 preventing grounding loss, but slightly rephrases the original fact. |
| 3 | factual | 10 PASS | What are the DBSCAN eps values for L0 and L4? | The agent correctly provided all the DBSCAN eps values exactly as in the ground truth. |
| 4 | factual | 9 PASS | Which node label stores LLM-named emergent concepts? | The agent correctly identified the node label as "EmergentConcept" and added context about it storing LLM-named emergent concepts, which is accurate but slightly more detailed than the ground truth. |
| 5 | relational | 9 PASS | What does the GUIDANCE_OUTCOME edge track? | The agent correctly identifies the possible values of the GUIDANCE_OUTCOME edge related to Jiminy guidance but slightly rephrases the original answer. |

### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all four frameworks and their associated purposes without missing any critical information. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly provided the number of specs and variants but omitted the critical fact about the 318 test cases. |
| 3 | relational | 9 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner demotion due to GAP-04 and the need for inline grading before CI activation, but slightly rephrases and omits the detail about false passes. |
| 4 | factual | 9 PASS | Which framework has no runner at all? | The agent correctly states that UAMS has no runner and its status is unbuilt, closely matching the ground truth but slightly rephrased. |
| 5 | factual | 8 PASS | What is the UETS framework for? | The agent correctly identifies the evaluation focus on LLM emergence and concept-naming quality but adds the UETS framework detail not present in the ground truth. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all frameworks using soft-fail CI gating with accurate descriptions and no omissions. |

### dict_pkg_codes

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | What does the 'ret' package code stand for? | The agent correctly identified all components of the retrieval pipeline and accurately described the 'ret' package code. |
| 2 | factual | 9 PASS | How many files and lines of code are in the API package? | The agent correctly states the number of files and lines of code and specifies the package, but the ground truth does not mention the package name. |
| 3 | factual | 8 PASS | What does the 'hid' package handle? | The agent correctly identified the key components of the hidden layer but added an unnecessary detail about the 'hid' package. |
| 4 | inferential | 10 PASS | Which package has the most files? | The agent correctly identified the cli package and the number of files, fully matching the ground truth. |
| 5 | factual | 9 PASS | What embedding dimensions does the emb package use? | The agent correctly identified the 3072-dimensional vectors but added an unnecessary detail about the "emb package" not present in the ground truth. |
| 6 | factual | 9 PASS | What is the package code for the Jiminy inner voice? | The agent correctly identified the package code as "jim" but added unnecessary detail about "Jiminy inner voice" not present in the ground truth. |

### dep_pkg_graph

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which packages are leaf packages with no dependencies? | The agent correctly listed all the leaf packages with no dependencies exactly as in the ground truth. |
| 2 | factual | 9 PASS | What does the api package depend on? | The agent's answer is accurate and complete except for the missing comma after "cns," which is a minor detail. |
| 3 | relational | 8 PASS | Why does ret depend on plg? | The agent correctly identifies that retrieval calls MatchIngestionModule as a fallback for unrecognized file types, but does not explicitly state the module's primary functionality. |
| 4 | relational | 9 PASS | Which package does hid depend on for emergence naming? | The agent correctly identifies the dependency of "hid" on "llm" for emergence naming but adds unnecessary detail about the package "hid." |
| 5 | relational | 8 PASS | What does grd depend on emb for? | The agent correctly identifies embedding-based similarity for constraint retrieval but uses unclear abbreviations and lacks clarity compared to the ground truth. |

### flow_observe_learn_consolidate

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | What does the observe endpoint return? | The agent correctly identified all required fields (obs_id, node_id, surprise_score) exactly as in the ground truth. |
| 2 | relational | 9 PASS | What triggers Hebbian learning? | The agent correctly identifies co-retrieval activating learning and the creation of CO_ACTIVATED_WITH edges, adding relevant implementation details without omitting main facts. |
| 3 | factual | 10 PASS | How many phases does the consolidation pipeline have? | The agent correctly identified the number of phases and specified the context as the consolidation pipeline, matching the ground truth. |
| 4 | factual | 10 PASS | What is the DBSCAN eps scaling range across layers? | The agent's answer correctly states the DBSCAN eps scaling range as 0.10 to 0.26, matching the ground truth exactly. |
| 5 | factual | 9 PASS | What is Phase 103 in the consolidation pipeline? | The agent correctly identifies "DynamicEmergence" and its association with LLM-named concepts but adds an unnecessary detail about "Phase 103 in the consolidation pipeline." |

### flow_jiminy_guide

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 9 PASS | What triggers the Jiminy guide flow? | The agent correctly identified the script and its trigger but added extra context about the Jiminy guide flow not present in the ground truth. |
| 2 | factual | 9 PASS | What is the timeout for the guide orchestration? | The agent correctly states the timeout is 6 seconds and specifies it applies to guide orchestration, but the answer includes extra context not present in the ground truth. |
| 3 | factual | 10 PASS | What are the three J17 encoding tiers and their approximate token sizes? | The agent's answer accurately captures all main facts including token sizes, traffic percentages, and descriptions for each tier. |
| 4 | relational | 10 PASS | What parallel sources does the guide query? | The agent's answer accurately includes all key elements from the ground truth with correct terminology and no missing facts. |
| 5 | relational | 8 PASS | What does jim.Effectiveness track? | The agent correctly identifies the use of guidance_id for tracking whether guidance was followed but adds extra detail about edges and outcomes not present in the ground truth. |

### flow_rsic_cycle

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | What are the 5 stages of the RSIC cycle? | The agent correctly listed all five stages of the RSIC cycle in the proper order with no errors or omissions. |
| 2 | factual | 10 PASS | How many action types can Dispatch execute? | The agent correctly states that Dispatch can execute 13 action types, fully matching the ground truth. |
| 3 | factual | 10 PASS | What are the three RSIC tiers and their intervals? | The agent correctly identified all three RSIC tiers and their corresponding intervals exactly as in the ground truth. |
| 4 | factual | 10 PASS | What safety mechanisms does RSIC use? | The agent correctly listed all the safety mechanisms exactly as in the ground truth without any errors or omissions. |
| 5 | factual | 10 PASS | What are the 7 health dimensions assessed in Stage 1? | The agent correctly listed all seven health dimensions exactly as in the ground truth. |

### svc_external

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 9 PASS | What port does the MDEMG server listen on? | The agent correctly identified the port number and its association with the MDEMG server but added extra context not present in the ground truth. |
| 2 | factual | 9 PASS | What port does the neural sidecar use? | The agent correctly identified the port number 8100 and its use with HTTP, but did not explicitly state the port number alone as the answer. |
| 3 | factual | 9 PASS | How does the MCP server communicate? | The agent correctly identifies communication via stdio as a subprocess for IDE integration, adding the MCP protocol detail which is accurate but slightly beyond the original answer. |
| 4 | relational | 9 PASS | What gRPC services are defined for content ingestion? | The agent correctly identified the IngestionModule and its operations but added extra detail about scope not mentioned in the ground truth. |
| 5 | factual | 8 PASS | What does the ReasoningModule gRPC service do? | The answer correctly identifies the purpose of "Rerank" as re-scoring retrieval candidates but adds unnecessary detail about the ReasoningModule gRPC service. |

### dist_channels

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | How do you install MDEMG on macOS? | The agent provided the exact correct command and relevant additional context without errors. |
| 2 | factual | 10 PASS | What technology is the Linux companion app built with? | The agent's answer correctly identifies Tauri (Rust + JS) and the Catppuccin theme, matching the ground truth perfectly. |
| 3 | factual | 8 PASS | Which platform companion app is not implemented? | The agent correctly identifies the windows companion app and its not-implemented status with the GAP-13 reference, but the phrasing is less concise than the ground truth. |
| 4 | relational | 9 PASS | What CI workflow handles cross-compilation releases? | The agent correctly identified the workflow file, trigger, and use of goreleaser, but added extra details about cross-compilation and GitHub Releases not mentioned in the ground truth. |
| 5 | factual | 9 PASS | What does auto-pr.yml do? | The agent correctly describes the auto-creation of PRs to main on pushes to *_dev* branches, but adds unnecessary detail about the workflow name and trigger event. |
