# Architecture Map Optimization Report

**Date**: 2026-03-24T22:35:03.853816Z  
**Model**: gpt-4.1-mini (judge/agent), gpt-4.1 (compression)  
**Iterations**: 1  
**Threshold**: 9.0/10  

## Learning Progress

| Iteration | Avg Score | Maps Changed |
|-----------|-----------|-------------|
| 1 | 8.81 | 0 |

## Per-Map Results

| Map | Type | Tokens | Compaction | Final Score |
|-----|------|--------|------------|-------------|
| flow_retrieval | FLOW | 169 | 0% | 9.2 |
| schema_neo4j | SCHEMA | 339 | 0% | 9.2 |
| uxts_frameworks | UXTS | 200 | 87% | 8.2 |
| dict_pkg_codes | DICT | 479 | 0% | 9.2 |
| dep_pkg_graph | DEP | 146 | 0% | 7.6 |
| flow_observe_learn_consolidate | FLOW | 245 | 0% | 9.6 |
| flow_jiminy_guide | FLOW | 184 | 0% | 8.4 |
| flow_rsic_cycle | FLOW | 214 | 0% | 9.6 |
| svc_external | SVC | 204 | 0% | 8.8 |
| dist_channels | DIST | 229 | 0% | 8.4 |

## Question-Level Detail (Final Iteration)


### flow_retrieval

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | What are the retrieval scoring weights? | The agent's answer perfectly matches the ground truth with all weights correctly listed and nothing missing. |
| 2 | factual | 9 PASS | What is the embedding dimension? | The agent correctly identified the embedding dimension as 3072 but did not provide additional context or details. |
| 3 | relational | 8 PASS | What training data does the retrieval pipeline produce? | The answer correctly identifies the JSONL format and the ret.CollectTraining component but slightly misses specifying that the tuples are specifically for neural sidecar cross-encoder training. |
| 4 | inferential | 9 PASS | If symbol search finds candidates, what triggered it? | The agent correctly identifies that the query containing symbols triggers the symbol search, but slightly misrepresents the exact condition by implying multiple symbols rather than the presence of the #symbol prefix. |
| 5 | factual | 10 PASS | What is the max graph expansion depth? | The agent correctly identified the max graph expansion depth as 3 and referenced the specific step where it is specified. |

### schema_neo4j

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | What edge type represents Hebbian learning? | The agent correctly identified the edge type as CO_ACTIVATED_WITH and accurately described its relation to Hebbian learning. |
| 2 | relational | 8 PASS | What prevents grounding loss in L5 concepts? | The agent correctly identifies the GROUNDED_BY edge as a skip connection from L5 to L0 preventing grounding loss but adds an unnecessary phrase and slightly misattributes the effect to L5 concepts rather than grounding overall. |
| 3 | factual | 10 PASS | What are the DBSCAN eps values for L0 and L4? | The agent correctly provided all the DBSCAN eps values exactly as in the ground truth. |
| 4 | factual | 9 PASS | Which node label stores LLM-named emergent concepts? | The agent correctly identified the node label as "EmergentConcept" and provided context, but the answer included extra explanation beyond the exact label. |
| 5 | relational | 9 PASS | What does the GUIDANCE_OUTCOME edge track? | The agent correctly identifies the possible values of the GUIDANCE_OUTCOME edge related to Jiminy guidance but slightly rephrases the original answer. |

### uxts_frameworks

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which frameworks are merge-blocking in CI? | The agent correctly listed all the merge-blocking frameworks with accurate associated details. |
| 2 | factual | 7 PASS | How many UATS specs and variants exist? | The agent correctly provided the number of specs and variants but omitted the critical fact about the 318 test cases. |
| 3 | relational | 8 PASS | Why is UVTS not CI-gated? | The agent correctly identifies the runner as demoted (GAP-04) and its CI status as NONE, but misses the critical detail about inline grading missing and the consequence of false passes. |
| 4 | factual | 7 PASS | Which framework has no runner at all? | The agent correctly identifies UAMS and the absence of a runner, but adds unclear or extraneous details and misses the specific "GAP-21" designation. |
| 5 | factual | 7 PASS | What is the UETS framework for? | The agent correctly identifies the UETS framework's purpose related to emergence evaluation of LLM concept-naming quality but does not explicitly state the evaluation of the emergence concept itself. |
| 6 | factual | 10 PASS | Which frameworks use soft-fail CI gating? | The agent correctly listed all four frameworks using soft-fail CI gating with accurate descriptions. |

### dict_pkg_codes

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | What does the 'ret' package code stand for? | The agent correctly identified all components of the retrieval pipeline and accurately described the 'ret' package code. |
| 2 | factual | 9 PASS | How many files and lines of code are in the API package? | The agent correctly states the number of files and lines of code and specifies the package, but the original answer does not mention the package name. |
| 3 | factual | 8 PASS | What does the 'hid' package handle? | The agent correctly identifies the key components of the hidden layer but adds an unnecessary detail about the 'hid' package. |
| 4 | inferential | 10 PASS | Which package has the most files? | The agent correctly identified the package and the number of files, matching the ground truth exactly. |
| 5 | factual | 9 PASS | What embedding dimensions does the emb package use? | The agent correctly identified the 3072-dimensional vectors but added an unnecessary detail about the "emb package" which was not in the ground truth. |
| 6 | factual | 9 PASS | What is the package code for the Jiminy inner voice? | The agent correctly identified the package code as "jim" and provided context, but the answer included extra information beyond the single expected word. |

### dep_pkg_graph

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | Which packages are leaf packages with no dependencies? | The agent correctly listed all the leaf packages with no dependencies exactly as in the ground truth. |
| 2 | factual | 7 PASS | What does the api package depend on? | The agent's answer is mostly correct but misses the package "lrn" which is present in the ground truth. |
| 3 | relational | 4 **WEAK** | Why does ret depend on plg? | The agent mentions a dependency on "plg" for "match-module" but fails to specify the correct module name (MatchIngestionModule) and lacks clarity on the functionality. |
| 4 | relational | 9 PASS | Which package does hid depend on for emergence naming? | The agent correctly identifies "llm" as the package for emergence naming but adds unnecessary wording without additional critical details. |
| 5 | relational | 8 PASS | What does grd depend on emb for? | The agent correctly identifies the dependency on embedding-based constraint retrieval but adds unnecessary detail and slightly rephrases the concept. |

### flow_observe_learn_consolidate

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | What does the observe endpoint return? | The agent correctly identified all required fields (obs_id, node_id, surprise_score) exactly as in the ground truth. |
| 2 | relational | 9 PASS | What triggers Hebbian learning? | The agent correctly identifies co-retrieval activating learning and the creation of CO_ACTIVATED_WITH edges, adding relevant details about Hebbian learning and soft-cap, with only minor elaboration beyond the ground truth. |
| 3 | factual | 10 PASS | How many phases does the consolidation pipeline have? | The agent correctly identified the number of phases and specified the context as the consolidation pipeline. |
| 4 | factual | 10 PASS | What is the DBSCAN eps scaling range across layers? | The agent's answer correctly states the DBSCAN eps scaling range as 0.10 to 0.26, fully matching the ground truth. |
| 5 | factual | 9 PASS | What is Phase 103 in the consolidation pipeline? | The agent correctly identifies "Dynamic Emergence" and its association with LLM-named concepts, adding a minor detail about the consolidation pipeline phase. |

### flow_jiminy_guide

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 8 PASS | What triggers the Jiminy guide flow? | The agent correctly identified the script and its execution on every prompt but added an unnecessary detail about the "Jiminy guide flow" not present in the ground truth. |
| 2 | factual | 9 PASS | What is the timeout for the guide orchestration? | The agent correctly states the timeout duration and context but adds unnecessary detail beyond the simple time value. |
| 3 | factual | 10 PASS | What are the three J17 encoding tiers and their approximate token sizes? | The agent correctly identified all three tiers with accurate token counts, traffic percentages, and descriptions matching the ground truth. |
| 4 | relational | 10 PASS | What parallel sources does the guide query? | The agent's answer accurately includes all key elements from the ground truth with correct terminology and no missing facts. |
| 5 | relational | 5 **WEAK** | What does jim.Effectiveness track? | The agent mentions tracking guidance_id for feedback correlation but incorrectly includes "jim.Effectiveness," which is not in the ground truth and may cause confusion. |

### flow_rsic_cycle

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 9 PASS | What are the 5 stages of the RSIC cycle? | The agent correctly identified all five stages with detailed descriptions, only missing the explicit naming format matching the ground truth. |
| 2 | factual | 9 PASS | How many action types can Dispatch execute? | The agent correctly states the number of action types but adds an unnecessary detail about "Dispatch" that is not in the ground truth. |
| 3 | factual | 10 PASS | What are the three RSIC tiers and their intervals? | The agent correctly stated all three RSIC tiers and their corresponding intervals exactly as in the ground truth. |
| 4 | factual | 10 PASS | What safety mechanisms does RSIC use? | The agent correctly listed all the safety mechanisms exactly as in the ground truth. |
| 5 | factual | 10 PASS | What are the 7 health dimensions assessed in Stage 1? | The agent correctly listed all seven health dimensions exactly as in the ground truth. |

### svc_external

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 9 PASS | What port does the MDEMG server listen on? | The agent correctly identified the port number and its association with the MDEMG server but added extra context not present in the ground truth. |
| 2 | factual | 9 PASS | What port does the neural sidecar use? | The agent correctly identified the port number 8100 and its use with HTTP, but did not explicitly state the port number alone as in the ground truth. |
| 3 | factual | 9 PASS | How does the MCP server communicate? | The agent correctly states communication via stdio as a subprocess for IDE integration but adds "MCP server" which is not in the ground truth, a minor detail not confirmed. |
| 4 | relational | 9 PASS | What gRPC services are defined for content ingestion? | The agent correctly identified the IngestionModule and its operations Matches, Parse, and Sync, but added the detail about gRPC service which is not in the ground truth. |
| 5 | factual | 8 PASS | What does the ReasoningModule gRPC service do? | The agent correctly identifies reranking as re-scoring retrieval candidates and specifies the ReasoningModule gRPC service, but does not explicitly state "rerank" as the function name. |

### dist_channels

| # | Difficulty | Score | Question | Reason |
|---|-----------|-------|----------|--------|
| 1 | factual | 10 PASS | How do you install MDEMG on macOS? | The agent provided the exact correct command with appropriate context and formatting. |
| 2 | factual | 9 PASS | What technology is the Linux companion app built with? | The agent correctly identified Tauri (Rust + JS) and Catppuccin but slightly altered the phrasing and omitted explicitly stating "theme." |
| 3 | factual | 7 PASS | Which platform companion app is not implemented? | The agent correctly identifies the Windows companion and GAP-13 but adds an unnecessary detail about implementation status, slightly deviating from the concise ground truth. |
| 4 | relational | 7 PASS | What CI workflow handles cross-compilation releases? | The agent correctly identified the workflow file but missed the trigger condition and the use of goreleaser. |
| 5 | factual | 9 PASS | What does auto-pr.yml do? | The agent correctly describes the auto-creation of PRs to main from *_dev* branches but adds the filename auto-pr.yml, which is not in the ground truth, and slightly expands beyond the original concise fact. |
