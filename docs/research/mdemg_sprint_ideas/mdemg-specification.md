# MDEMG Specification — R&D Phase Stewardship and Successor Framework Preparation

**Document ID:** MDEMG-SPEC-RD-001
**Version:** 1.0
**Date:** 2026-04-27
**Author:** Technical reviewer working with reh3376
**Repository under analysis:** https://github.com/reh3376/mdemg.git (HEAD: PR #358 merged 2026-04-27 04:53 UTC)
**Intended consumers:** Planning agents executing concurrent sprints on `reh3376_dev01` and `reh3376_dev02`
**Strategic frame:** MDEMG as R&D vehicle, not production target; successor framework on the horizon
**Calibration:** Loyal opposition; honest critique; placation explicitly disallowed

---

## Table of Contents

- [§0 — Document Purpose and Reading Notes](#0--document-purpose-and-reading-notes)
- [§1 — Codebase Reconnaissance Findings](#1--codebase-reconnaissance-findings)
- [§2 — Alignment Assessment Against the Long-Term Goal](#2--alignment-assessment-against-the-long-term-goal)
- [§3 — Fork-Timing Specification](#3--fork-timing-specification)
- [§4 — Risks Specification](#4--risks-specification)
- [§5 — Opportunities Specification](#5--opportunities-specification)
- [§6 — Sprint-Generation Directives](#6--sprint-generation-directives)
- [§7 — Open Questions Surfaced by Reconnaissance](#7--open-questions-surfaced-by-reconnaissance)
- [§8 — Specification for the Successor Framework's Foundational Document](#8--specification-for-the-successor-frameworks-foundational-document)
- [Appendix A — Risk and Opportunity Entry Template](#appendix-a--risk-and-opportunity-entry-template)
- [Appendix B — Cross-Reference Index](#appendix-b--cross-reference-index)
- [Appendix C — Reconciliation with Existing Project Documents](#appendix-c--reconciliation-with-existing-project-documents)
- [Appendix D — Glossary](#appendix-d--glossary)

---

## §0 — Document Purpose and Reading Notes

### §0.1 Document purpose

This document is a specification, not a recommendation. It exists so that planning agents can decompose the work it describes into sprint-ready backlogs without having to re-derive scope from source material. Every entry in §4 (Risks) and §5 (Opportunities) is self-contained: identifier, statement, observable indicators, severity or value rating, prerequisites, recommended action, success criteria, trigger conditions for re-evaluation. The document is meant to be parsed mechanically by planning agents, then sequenced into sprints with the dependency-respect rules laid out in §6.

This is *not* a vision document, a research roadmap, or a marketing artifact. The vision lives in `VISION.md`. The research roadmap lives in `mdemg-research-evaluation.md` and the eight architectural-extension notes. This document specifies the operational work required to steward MDEMG through its remaining R&D phase, identify what to extract from MDEMG before fork, and prepare the successor framework's foundational document.

### §0.2 Strategic frame and long-term goal

**MDEMG is the R&D vehicle, not the production target.** The user has been explicit about this: MDEMG has served as a year-long R&D substrate, has produced architectural intuitions that are now coherent, and is the *template* from which a more abstracted, more user-facing, more robust successor framework will be built. The successor framework is a separate artifact, not a future version of MDEMG.

**The long-term goal is not "improve MDEMG."** The user's stated long-term goal is the construction of continuous-learning artificial neural networks with biologically-inspired topologies — systems that establish reference frames, build world models, continuously predict current state, project forward and backward in time, and improve themselves recursively such that they become smarter and wiser the way a biological neural network does. The user has explicitly noted that the BNN analogy is a vocabulary stand-in: the actual artifact may not resemble a BNN, but no better vocabulary currently exists.

**Fork-timing is constrained, not arbitrary.** The user has stated that the right moment to fork is not before the FT-LORA workstream is fully complete — all adapters trained, multi-LoRA serving validated end-to-end at the 16 MDEMG call sites currently routed to gpt-5.4-mini. The reasoning: forking around an unresolved infrastructure question produces a worse template and forces re-work in both branches. Counter-pressure: the longer the fork is delayed, the more MDEMG-specific design choices calcify into the eventual successor's starting point. The fork-timing question is therefore not a single threshold but a balance — that's why §3 of this document specifies it operationally.

**Calibration is loyal opposition.** Honest critique. The user has stated explicitly that placation does no good. Where the document says uncomfortable things in §2 (Alignment Assessment) or §4 (Risks), those things are calibrated to be true and useful, not to be diplomatic. The user is the decision-maker; the document's role is to surface observations sharply so the decisions are well-informed.

### §0.3 Confidence calibration and limitation acknowledgments

This document is written by a technical reviewer who has walked the codebase, read the project artifacts, and synthesized observations against the long-term goal. It is not written by a researcher in continuous learning, neuromorphic computing, or BNN-inspired ANN architectures. Some recommendations are calibrated guesses about what the research community considers tractable; the document flags which conclusions are confident and which are speculative.

Three explicit confidence levels are used throughout:

- **Confident** — grounded in directly inspected code, in canonical literature with a clear consensus, or in unambiguous artifacts.
- **Moderate** — grounded in one but not both of the above, or where the inference required to reach the claim has only one or two steps.
- **Speculative** — grounded in plausible inference where the underlying evidence does not directly support the claim. Speculative conclusions are explicitly labeled and should be triaged by the user, not consumed by planning agents as direction.

The document also carries one structural limitation worth surfacing: the most recent codebase state was reconstructed from a shallow clone (HEAD: PR #358 merged 2026-04-27) plus the publicly visible PR description for PR #358. PR #357 (the first REAL Phase 11 LoRA training, commit `ab32f6f`) and PR #358 (the kl=0.10 retry + benchmark documentation) merged within ~24 hours of this document's authorship. AGENT_HANDOFF.md and CHANGELOG.md as of HEAD do not yet reflect PR #357 or #358. That documentation debt is itself a finding that lands in §4.

### §0.4 Reading notes for planning agents

**Read the sections in order.** §0 sets the lens. §1 establishes the facts. §2 establishes the alignment assessment that determines whether items in §4 and §5 advance the long-term goal or not. §3 establishes the gate. §4 and §5 are the operational backlog. §6 specifies how to consume them. §7 captures unresolved questions for the user. §8 looks ahead to what the successor framework's foundational document needs to be.

**§4 and §5 entries use a uniform template** (Appendix A). Every risk has the same fields. Every opportunity has the same fields. Planning agents should treat the entries as records, not prose.

**Cross-references use identifiers, not section numbers.** Every risk has an ID like `R-FT-1` or `R-INT-2`. Every opportunity has an ID like `O-MDEMG-1` or `O-LT-3`. Cross-references in the document point to identifiers, not to sections, so reorganization is robust.

**Fork-relationship is a primary axis.** Every opportunity in §5 is tagged `pre-fork`, `fork-gating`, `post-fork`, or `substrate-agnostic`. Planning agents should not sprint on `post-fork` items during the remaining MDEMG R&D phase. This is a hard rule, not a guideline.

**Planning agents should not invent scope.** If an entry is incomplete or ambiguous, surface it to the user via §7 (Open Questions) rather than guessing. The user has been explicit about wanting honest signals; a sprint that closes an invented scope wastes the cycle that should have been spent surfacing the ambiguity.

---

## §1 — Codebase Reconnaissance Findings

This section is descriptive. It tells the planning agent what is actually in the codebase as of PR #358 merged 2026-04-27. File-level pointers are included so the planning agent can verify any claim against the source. Without this section, every later recommendation operates on assumption; with it, every recommendation is grounded.

### §1.1 Repository structure and scale

The repository is large for a single-author project. Direct measurements from HEAD:

- **270,432 lines of Go** across 774 files
- **86,955 lines of Python** across 237 files
- **279 Go test files**
- **46 internal/ subsystems** (under `internal/`)
- **15 CLI entrypoints** (under `cmd/`)
- **27 Cypher migrations** (V0001 through V0024 plus support files, under `migrations/`)
- **150+ API endpoints** (estimated from `HandleFunc`/`Handle(` matches in `api/` and `internal/api/`)
- **15 active UxTS frameworks** (per `docs/specs/FRAMEWORK_GOVERNANCE.md`)

The Go side is the load-bearing memory and retrieval substrate. The Python side is the neural sidecar (re-ranking, NLI, tier prediction) plus the FT-LORA training pipeline. The split is architectural: Go owns the memory graph, the constraint enforcement, the API surface, and the coordination machinery. Python owns ML training and inference work that doesn't fit Go's runtime.

The 46 internal subsystems by Go LOC, ranked: `cli` (38,714), `api` (26,053), `jiminy` (17,316), `retrieval` (14,915), `ape` (15,944), `languages` (12,087), `hidden` (12,157), `conversation` (10,913), `tsdb` (5,310), `symbols` (5,982), `sidecar` (3,478), and the rest below 3,500 each. The five highest-LOC subsystems together account for ~110,000 of the 270,000 Go LOC — roughly 40% — so reading those five gives most of the architectural picture.

### §1.2 The graph layer and Hebbian dynamics

The memory graph runs on Neo4j with native vector indexes. The schema has evolved through 24 migrations: V0001 set up `schema_meta`; V0002–V0009 established the basic node/edge taxonomy with vector indexes; V0010–V0017 added agent-identity, observation templates, backup metadata, relationship edges, secondary labels, theme-of edges, dynamic-edge indexes; V0018 bumped vector dimension to 3072; V0019–V0024 added performance indexes, constraint lifecycle, J17 protocol support, symbol natural keys, and signal state tracking.

The Hebbian update rule is in `internal/learning/service.go:957`. The pure function is:

```go
func HebbianWeightUpdate(w, ai, aj, eta, mu, wmin, wmax float64) float64 {
    prod := ai * aj
    newW := (1-mu)*w + eta*prod
    if newW < wmin {
        return wmin
    }
    return wmax * math.Tanh(newW/wmax)
}
```

This is **classical Hebbian-with-decay, tanh soft-capped at `wmax`**. The update rule is `Δw = η·a_i·a_j − μ·w_ij`, which simplifies to `new_w = (1−μ)·w + η·a_i·a_j` as the comment notes. The activation product `a_i·a_j` is the simplest possible co-occurrence signal — both nodes activated, edge strengthens; one or neither, edge decays.

This is not the precision-weighted Hebbian rule from research note 02. It is not BCM (Bienenstock-Cooper-Munro), which would multiply by `(a_j − θ)`. It is not Oja's rule, which would subtract a normalization term. It is not predictive-coded, which would use prediction error rather than activation product. The codebase implements vanilla Hebbian-with-decay and operates within that constraint.

The decay is two-source: a global decay term (`μ`) applied at every update, and a separate evidence-weighted prune cycle in `PruneDecayedEdges()` that removes edges below a threshold weighted by `evidence_count`. The unification of these two decay sources is a known architectural concern (mentioned in user's project files as "two parallel decay systems") and remains outstanding.

The eta scheduling is in `effectiveEta()` at line 187 and `phaseMultiplier()` at line 222. Eta scales by edge count — small graphs see higher eta (faster learning), large graphs see lower eta (slower drift). This is a maturity-based learning rate decay, which is a reasonable engineering choice but not a research commitment.

### §1.3 The representation layer

Embeddings come from external providers. `internal/embeddings/embeddings.go` defines an `Embedder` interface; the two implementations are `internal/embeddings/ollama.go` (Ollama backend, default model `qwen3-embedding:8b`, 4096 native dimensions, 3072 target via MRL truncation) and `internal/embeddings/openai.go` (OpenAI backend, default model `text-embedding-3-large`, 3072 dimensions). There is no learned embedding adapter, no fine-tunable embedding head, no representation-modification machinery in the Go side of the codebase.

This is a load-bearing fact for §2. **MDEMG does not learn or modify representations.** It receives them as a service from external models. The graph layer adapts retrieval over a frozen representation space — Hebbian updates change which nodes get retrieved together, but the underlying embedding manifold is fixed by upstream models the system does not control.

The Python neural sidecar at `neural/neural_sidecar/` contains a reranker (`reranker.py`), an NLI classifier (`nli.py`), and a tier predictor (`tier_model.py`). All three use frozen pretrained models from HuggingFace. The training pipelines under `neural/training/` (including the LoRA work) train *adapter* layers on top of frozen base models — they do not train the representation layer that MDEMG's graph keys on. Fine-tuning the LLM at the 16 call sites is a separate concern from the embeddings the graph uses for retrieval.

### §1.4 The clustering and concept-emergence mechanism

Concept emergence happens in `internal/hidden/`. The mechanism is DBSCAN clustering over cosine distance on embeddings, followed by LLM-driven naming via Phase 103 (Dynamic Emergence). The actual implementation is in `internal/hidden/clustering.go` — `PrecomputeDistanceMatrix()` builds a pairwise cosine distance matrix in parallel, `DBSCANWithMatrix()` runs DBSCAN with given epsilon and minSamples, and the layer-specific parameters are computed adaptively based on layer index (epsilon scales as `base_eps · (1 + 0.4·layer)`, minSamples decreases with layer).

The five-layer hierarchy from VISION.md (L0 observations → L5 emergent concepts) is implemented as a sequence of pipeline steps in `internal/hidden/pipeline.go`. Each step is a `NodeCreator` that takes the previous layer's output and produces the next layer's nodes via clustering, summarization, and edge creation. The clustering operates on embeddings; the summarization is LLM-driven (`step_cluster_summary.go`); the naming is LLM-driven (`emergence_namer.go`).

This is also a load-bearing fact for §2. **MDEMG's "concept emergence" is clustering plus naming.** New concepts emerge as cluster labels over the frozen embedding space, not as new representational units. This is genuinely useful for the agent-memory use case — clusters of related observations get a name, the name becomes a queryable handle, the agent can ask about the named concept rather than enumerating its members. It is not what the long-term goal frames as concept formation in a continuous-learning system, where new concepts would correspond to new representational dimensions or new manifold regions the system carved out itself.

The DBSCAN implementation is O(n²) in distance matrix computation. This becomes a scale ceiling at large graph sizes — a known concern in `risk-opp-04232026-01.md` (R3, O6). At present graph sizes the cost is bounded, but the architectural point is that clustering-based emergence has a built-in scale wall that learned-representation emergence does not.

### §1.5 The Jiminy interceptor and its components

`internal/jiminy/` contains 69 Go files implementing the Jiminy inner-voice service. The architectural pattern is the interceptor pattern: detect → correct → iterate → learn. The five composing components (per the user's project memory) are:

- **Jiminy Guide (pre-action)** — injects guidance into every user prompt via Claude Code hooks. Files: `service.go`, `guidance_prompt.go`, `formatter.go`, `encoder.go`. Orchestrates five knowledge sources in parallel with a 6-second timeout: consulting constraints, correction vector search, contradiction edges, frontier detection, trust-scored history.

- **Guardrail Validate + pre-bash-check (during-action)** — `internal/guardrail/` runs constraint enforcement at MCP integration points; `.claude/hooks/pre-bash-check.py` runs at bash-execution time. Both check the agent's intended action against accumulated constraints.

- **Jiminy Evaluate (post-action)** — `evaluator.go`, `eval_prompt.go`, `outcome_classifier.go`. After the agent acts, Evaluate scores whether the guidance was followed, contradicted, or ignored. The score feeds the next component.

- **EffectivenessTracker (feedback)** — `effectiveness.go`, `nli_calibration.go`, `nli_comprehension.go`. Tracks per-guidance-item effectiveness over time. Per-tier comprehension grading via NLI scoring.

- **SignalLearner + RSIC SK1 (Hebbian learning loop)** — `internal/ape/signal_learner.go` plus `internal/ape/cycle.go` (RSIC SK1 actions). The signal learner converts effectiveness signals into RSIC adjustments: tier threshold updates, code retirement, constraint archival.

The interceptor pattern is architecturally important and worth carrying forward. The pattern itself — five composing components forming a closed feedback loop around agent action — is a generalizable architectural commitment, not an MDEMG idiom. The pattern should appear in the successor framework's foundational document as a first-class commitment.

The J17 AI-to-AI protocol (`internal/jiminy/protocol*.go` files) extends Jiminy with three-tier encoding (T1 coded codes, T2 telegraphic abbreviations, T3 full natural-language explanations), HMAC-signed session tickets, and an ML tier predictor in the neural sidecar. The protocol's compression ratio (up to 5.2× per VISION.md) is the headline result; the underlying mechanism is per-agent comprehension tracking that promotes/demotes encoding density based on demonstrated comprehension.

### §1.6 The RSIC self-improvement cycle

`internal/ape/` contains 41 Go files implementing the Recursive Self-Improvement Cycle. The structure is: 5-stage cycle (Assess → Reflect → Plan → Execute → Validate) operating at 3 temporal scales (Micro: minutes; Meso: hours; Macro: days).

The components:

- **Assess** — `self_assess.go`, `health_formula.go`. Computes a 7-dimension health score: retrieval, memory, edge, task, guidance, protocol, synergy.
- **Reflect** — `self_reflect.go`, `llm_reflector.go`. Pattern detection across 19 reflection patterns. Uses an LLM reflector for high-level pattern naming.
- **Plan** — `improvement_plan.go`. Produces prioritized improvement tasks with estimated impact.
- **Execute** — `task_dispatch.go`, `cycle.go`. Runs automated actions: tier threshold adjustment, code retirement, constraint archival. Safety gates: dry-run mode, rollback snapshots, confidence thresholds, cooldown policies (`safety_validator.go`).
- **Validate** — Before/after comparison ensuring changes improved rather than degraded quality. Calibration logic in `calibration.go`.

The signal learner (`signal_learner.go`) is the link between the Jiminy interceptor and RSIC. SignalLearner converts Jiminy effectiveness data into RSIC actions; RSIC SK1 actions feed back into Jiminy's tier thresholds and constraint priorities.

This is the third load-bearing fact for §2. **RSIC modifies parameters, not architecture.** The 19 reflection patterns are pre-defined; the actions RSIC can take are pre-defined; the safety gates are pre-defined. RSIC tunes thresholds within a fixed mechanism. It does not modify the mechanism itself, does not add new patterns or new action types, does not change its own learning rules. This is operationally important — it makes RSIC safe to run autonomously — and architecturally limited. The long-term goal's "recursive self-improvement" framing is more ambitious than what RSIC currently does.

### §1.7 The neural sidecar

`neural/neural_sidecar/` is the runtime ML service. FastAPI app (`app.py`) exposes `/rerank`, `/nli`, `/protocol/predict-tier`, `/health`. The three loaded models are:

- **Reranker** — `reranker.py`. Cross-encoder for retrieval re-ranking. Trained via `train.py` on rerank collection data from the Go side (`internal/retrieval/rerank_collector.go` writes JSONL training examples).
- **NLI Classifier** — `nli.py`. Natural language inference for guidance comprehension scoring. Used by Jiminy's NLI calibration system.
- **Tier Model** — `tier_model.py`. Predicts optimal J17 encoding tier (T1/T2/T3) based on agent comprehension history. Trained via `train_protocol.py` on protocol data from `internal/jiminy/protocol_data_collector.go`.

The sidecar is structurally significant: it represents MDEMG's only learned-representation surface. The three models are the only components in the system whose weights are updated by MDEMG itself rather than by an external service. The ML scope is narrow — task-specific cross-encoders, classifiers, and tier predictors — but it is real machine learning happening inside the system, distinct from the Go side's frozen-embedding retrieval.

### §1.8 The FT-LORA workstream

This is the most active surface as of HEAD. The full chain through PR #358:

- **FT-LORA-A through FT-LORA-E** (PRs #335, #336, #338-340, #343, commit `14cd2b3`) — preparatory sprints establishing the training infrastructure, expert selection profiles, asymmetric quantization, early-stop logic, tier-aware CLI.
- **FT-LORA-DATA** (PR #346, commit `234baec`) — dataset curation. 4 training-ready datasets: `tier1` (3,500 rows, 16 tasks balanced), three family adapters (T-family 1,700, C-family 1,200, J-family 600). Mixed-teacher synthesis (gpt-5.4-mini + local Qwen3.6 MLX).
- **FT-LORA-PHASE5** (PR #347, commit `c0be250`) — first real MDEMG fine-tuning. Mid-sprint MoE → dense pivot when the Metal MTLResource cap (499K) blocked MoE LoRA backward passes on M5 Max. Single-tier LoRA on `mlx-community/Qwen3-14B-4bit`, 7 dense target modules, rank=32, alpha=64. Output: `.local-models/qwen3-14b-mdemg-v1/`. Dual regression PASS, 16/16 ULTS tasks.
- **FT-LORA-PHASE10** (PR #348, commit `b81c5fb`) — automated benchmark framework. First authoritative baseline at aggregate weighted 0.8338 across 16-of-17 ULTS specs × 5 runs. Two silent scorer bugs caught and fixed (each worth ~2-4% aggregate). Phase 11 GRPO unblocked.
- **FT-LORA-PHASE11** (PR #349) — GRPO trainer + DPO pair generator + dual regression harness shipped. Code-complete, compute pass operator-gated. 73 tests across 3 tiers, all green.
- **FT-LORA-PHASE10.5a** (after PR #349) — added two reward functions for `guardrail.evaluate` (F1 violation detection accuracy + false positive penalty). Registry 18 → 20.
- **FT-LORA-PHASE11 follow-up: MLX adapter** — `neural/training/rl/mlx_adapter.py` (~330 LOC) lands. `MLXGRPOAdapter` provides real MLX wiring for the four trainer Protocol callables. Suite 73 → 89 tests. Compute pass now runnable.
- **PR #352** (Option A + Tier 1 + live wiring), **PR #354** (LoRA install bugfix wave + Tier 3 checkpoint, broken-then-fixed), **PR #356** (kl=0.05 retry + stratified_by_task sampler) — iterative refinement, each catching specific bugs.
- **PR #357** (commit `ab32f6f`) — **first REAL Phase 11 LoRA training**. Aggregate target met for the first time: 0.8514 vs 0.8505 target (+1.76pp over baseline 0.8338). Per-task cap failed on three C-group classifiers (`consulting.classify` -4.33pp, `consulting.synthesis` -3.04pp, `retrieval.query_classify` -10.00pp). The implementation chain proven empirically correct end-to-end.
- **PR #358** (HEAD, merged 2026-04-27) — kl_coef config bump 0.05 → 0.10 retry currently running (~13 hr wall-clock, ETA 2026-04-27 ~17:00 UTC), plus per-task and per-metric benchmark documentation pinning the actual data behind the +1.76pp aggregate claim.

The 16 MDEMG LLM call sites currently route to gpt-5.4-mini. The cost-replacement target is to migrate them to the local Qwen3-14B-RL adapter once both Phase 11 gates pass (5a aggregate +2pp over baseline; 5b no per-task regression worse than -2pp).

The PR #358 documentation explicitly notes which call sites are migration-ready *today* (T-group reflective/synthesis: `ape.reflect` +4.00pp, `jiminy.synthesize` +2.00pp, `metalearn.generalize` +0.76pp, `summarize.generate` +0.40pp, `hidden.summarize` +0.44pp), which must stay on gpt-5.4-mini until C-group regressors close, and which are at ceiling or zero-stddev (8/16) where the choice is dominated by cost/latency/privacy rather than quality.

The kl=0.10 retry is the next signal. If it passes both gates, the adapter promotes from `.local-models/qwen3-14b-mdemg-v1-rl-sandbox/` to `.local-models/qwen3-14b-mdemg-v1-rl/` and unblocks Phase 12 HITL DPO. If it fails per-task again with the same three regressors at similar magnitudes, the evidence suggests kl=0.05 was already at the right regularization level and a different mitigation strategy is required (per-task LoRA freezing on the regressors, or shipping run 5 with explicit per-task gate exemptions documented).

### §1.9 The UxTS framework family

Per `docs/specs/FRAMEWORK_GOVERNANCE.md`, fifteen UxTS frameworks govern test and verification across MDEMG:

| Framework | Name | State |
| --- | --- | --- |
| UNTS | Universal Hash Test Specification | active |
| UDTS | Universal DevSpace Test Specification | active |
| UATS | Universal API Test Specification | active (124 specs) |
| UPTS | Universal Parser Test Specification | active (28 specs) |
| UBTS | Universal Benchmark Test Specification | active (CI smoke, soft-fail) |
| USTS | Universal Security Test Specification | pilot |
| UAMS | Universal Auth Method Specification | spec-only (no runner) |
| UOBS | Universal Observability Specification | active |
| UOTS | Universal Observability Test Specification | active |
| UVTS | Universal Validation Test Specification | pilot (setup-only runner) |
| UETS | Universal Emergence Test Specification | active (E1-E5 enforced) |
| UITS | Universal Iterative-Improvement Test Specification | active (11 specs) |
| ULTS | Universal LLM Task Specification | active (16 specs) |
| UTDS | Universal Training Data Specification | active (3 specs) |
| UAITS | Universal AI Training Specification | active (1 spec, 41 checks) |

The framework governance file specifies: every active framework must define schema, specs, runnable harness/runner, and execution path in CI. **Schema-runner parity is mandatory for promotion to active.** Every field in a framework's schema must be either enforced by the runner or detected as unimplemented with a hard fail.

This is structurally important. The UxTS family is one of MDEMG's strongest architectural commitments — a uniform pattern for governing testable surfaces across the codebase. The pattern (schema + spec + runner + CI) is generalizable. It should carry forward to the successor framework. The specific frameworks (UAITS for training, ULTS for LLM tasks, etc.) are MDEMG-specific instantiations of the pattern; the pattern itself is substrate-agnostic.

The `unts-registry.json` tracks SHA-pinned canonical specs across the framework family. The verification workflow at `.github/workflows/uxts-canonical-specs.yml` enforces hash integrity at CI time. This is governance infrastructure — it doesn't do new things, but it makes existing things reliable.

### §1.10 What is *not* in the code

This subsection is deliberately included because a planning agent reading §2 needs to understand the absence of certain mechanisms is a fact, not an oversight. Direct code search confirms:

- **No predictive coding mechanism.** No `predictive_coding`, `prediction_error`, or `PredictiveCoding` symbols exist in any Go or Python file. Note 03 (Top-Down Predictions and Prediction-Error Promotion) proposes adding `:PREDICTS` schema to the graph; this proposal is not implemented.
- **No world model.** No `world_model` or `WorldModel` symbols. The graph stores observations and concepts; it does not maintain a generative model of state evolution.
- **No reference frame learning.** No `reference_frame` or `ReferenceFrame` symbols. The reference frames the system uses for organization are inherited from external structure (file paths, package hierarchies, symbol trees from tree-sitter parsing of the host codebase).
- **No active inference.** No `active_inference` or `ActiveInference` symbols. RSIC is reactive (detect divergence, correct it); it does not select actions to minimize expected free energy in any explicit sense.
- **No free energy minimization.** No `free_energy` or related symbols.
- **No object-centric slot structure.** No slot-attention, no DETR-style query slots, no equivalent. Concepts are clusters with names; they are not bound entities with persistent identity that survive context changes.

The "predict" usages that *do* exist in the Python sidecar (`tier_model.py`, `app.py`) are application-level prediction (NLI tier classification, response generation), not architectural prediction. They predict what an agent will comprehend, not what the world will do next.

These absences are not flaws. They are facts about MDEMG's scope. The R&D-vehicle framing fits because MDEMG has produced empirical familiarity with what these mechanisms *would have to look like* if they were present, by virtue of operating without them and observing the limits.

### §1.11 Documentation state and known staleness

The `AGENT_HANDOFF.md` is dated 2026-04-21 and covers work through ~2026-04-24. The CHANGELOG `[Unreleased]` section ends at the FT-LORA Phase 11 MLX adapter follow-up (2026-04-24). Neither yet documents PR #357 (first REAL Phase 11 training, 2026-04-27 03:51 UTC) or PR #358 (kl=0.10 retry + benchmark documentation, 2026-04-27 04:53 UTC).

This is a 1-2 PR documentation lag, consistent with the user's project memory note that documentation debt accumulates at sprint velocity (4-5 PRs/day). The mandatory documentation phase rule the user has instituted exists precisely to address this — every sprint is supposed to end with a documentation update task. The lag at HEAD suggests that rule was relaxed or skipped for the most recent two PRs, possibly because they are tightly coupled (PR #357's training run produced data; PR #358's documentation pins the data; both serve the same kl=0.10 retry hypothesis).

This is worth flagging as a process risk in §4 because *if the spec produced from this document directs sprints based on what AGENT_HANDOFF says*, those sprints will be miscalibrated by 1-2 PRs.

---

## §2 — Alignment Assessment Against the Long-Term Goal

This section is the document's most consequential. It evaluates how the current codebase aligns with the long-term goal — continuous-learning ANNs with BNN-inspired topologies, reference frames, world models, recursive self-improvement. Where MDEMG is well-positioned for the bridge to the long-term goal. Where MDEMG's design choices are local optimizations that don't generalize. Where MDEMG approximates the *appearance* of the long-term-goal capabilities without the substance.

The section is calibrated to loyal opposition. It says uncomfortable things directly. It is not a critique of MDEMG's execution — execution has been excellent. It is an honest assessment of what MDEMG is and is not, against a goal more ambitious than MDEMG itself.

### §2.1 What MDEMG actually learns

MDEMG learns edge weights over a graph of nodes whose embeddings come from external models. The Hebbian update at `internal/learning/service.go:957` adjusts edge weights based on co-activation; the decay term causes unused edges to fade. That is real learning in a strict sense — the system's behavior changes over time based on accumulated experience, and the changes persist in the graph state.

The boundaries of this learning are:

- **No representation learning.** The embedding manifold is fixed by the external embedding model. MDEMG's learning operates over coordinates the system did not choose.
- **No structural learning.** The graph schema (node types, edge types, layer structure) is fixed by migrations. MDEMG's learning does not modify what kinds of relationships are representable.
- **No mechanism learning.** The Hebbian rule is fixed. RSIC tunes the rule's parameters (eta, decay, thresholds); it does not modify the rule itself.

This is a sophisticated retrieval-and-organization system that learns *which patterns to surface* over a frozen substrate of *what patterns are representable*. The distinction matters because the long-term goal demands learning at all three levels: representation, structure, and mechanism. MDEMG learns at one of three.

The +58.4% retrieval improvement claim from VISION.md is real and earned. It represents what the available level of learning can produce when applied with discipline. It is not evidence that the substrate can support continuous-learning ANNs; it is evidence that retrieval-over-frozen-representations can be improved substantially through edge-weight-only adjustments.

### §2.2 What MDEMG calls "emergence"

Concept emergence in MDEMG is DBSCAN clustering plus LLM-driven naming. Clusters of similar embeddings get a label; the label becomes a queryable concept. This works well for the agent-memory use case — the agent can ask "what do we know about authentication patterns?" and the system can return a cluster of observations that share the trait, named at clustering time.

The boundaries of this emergence:

- **No new dimensions.** The embedding space has 3072 dimensions (or 4096 native truncated to 3072). Clusters live in that space. New clusters do not add new representational axes.
- **No compositional generalization.** A cluster is a region of the existing space, not a new compound concept that combines existing concepts in a structurally meaningful way.
- **No new types of relations.** Edges are typed by the migration schema. New concepts inherit the existing edge types; they do not introduce new ways for concepts to relate.

This is "emergence" in the sense that cluster labels were not pre-specified. It is not "emergence" in the sense the manifesto frames it — where new representational units form from accumulated experience and the system carves new conceptual axes that did not exist before.

The architectural difference matters for the long-term goal. A system that establishes reference frames must carve representational space according to the structure of the world; it cannot simply cluster within a pre-given space. MDEMG's emergence is downstream of an upstream representation system that does the actual carving. Carrying MDEMG's emergence mechanism forward to the successor would inherit that limitation.

### §2.3 What MDEMG calls "reference frames"

The reference frames MDEMG uses for organization are inherited from the host codebase's external structure: file paths, package hierarchies, symbol trees parsed by tree-sitter. The `internal/symbols/` subsystem extracts these as data; `internal/hidden/` organizes observations against them; `internal/retrieval/` queries within them.

This is genuinely useful for the agent-memory use case. The reason MDEMG's retrieval works for code is precisely because it organizes around the structures that are meaningful in code — files, packages, classes, functions. These are reference frames in a meaningful sense; they are not arbitrary.

The boundaries:

- **The frames are given, not learned.** When the codebase changes, the frames change. MDEMG adapts; it does not propose them.
- **The frames are external.** A successor framework that targets domains without natural file-path structure (general reasoning, world modeling, sensorimotor control) cannot inherit MDEMG's reference frames. It must learn them.
- **The frames are not modifiable by the system.** The agent cannot decide that two files are "really" the same concept and merge their frames; the agent cannot carve a sub-frame within a function it discovers has multiple distinct roles.

The manifesto's notion of reference frames is closer to Hawkins' *Thousand Brains* framing — distributed reference frames that the system constructs to support compositional reasoning, learned from experience, modifiable as the system's understanding evolves. MDEMG's reference frames are static external scaffolding. Calling them "reference frames" risks vocabulary drift: planning agents should use the term carefully and the successor framework's foundational document should distinguish "MDEMG-style inherited frames" from "learned reference frames" explicitly.

### §2.4 What MDEMG calls "self-improvement"

RSIC modifies parameters within fixed mechanisms. The 19 reflection patterns are pre-defined. The action types RSIC can execute (tier threshold adjustment, code retirement, constraint archival) are pre-defined. The safety gates (dry-run, rollback, confidence thresholds, cooldown) are pre-defined. RSIC's "self-improvement" is calibrated parameter search inside a designed envelope.

This is operationally important. RSIC's safety properties depend on the fixity of the envelope. A system that could modify its own learning rules autonomously would have a much harder safety story — every safety mechanism becomes self-modifiable, and the verification problem becomes intractable. Fixed envelope, modifiable parameters is a defensible engineering choice.

The boundaries:

- **No mechanism modification.** RSIC does not propose new reflection patterns, new action types, new health dimensions.
- **No architectural modification.** RSIC does not restructure the graph schema, the layer hierarchy, or the pipeline ordering.
- **No goal modification.** RSIC's optimization target (the 7-dimension health score) is fixed. RSIC tunes inputs to this target; it does not reconsider what counts as health.

The long-term goal's "recursive self-improvement" is more ambitious than RSIC's parameter-search. It implies a system that can modify its own learning rules, restructure its own architecture, and refine its own optimization targets, while remaining safe to operate. That is a genuinely hard research problem. MDEMG's RSIC does not attempt it. The fact that RSIC is called "Recursive Self-Improvement Cycle" risks the same vocabulary drift as "reference frames" — useful conceptual framing for the work that *is* done, misleading if read as a claim about the work that is *not* done.

PR #358 is a perfect operational example of what RSIC-flavored work looks like in practice. The kl_coef bump from 0.05 to 0.10 is exactly the kind of parameter tightening RSIC's machinery is designed to do — observe per-task instability, hypothesize tighter regularization, run, measure, ship if it works. That is not architectural self-improvement. It is hyperparameter search with discipline. The discipline is real and valuable; the framing as "recursive self-improvement" overshoots.

### §2.5 What MDEMG cannot do without a representation-learning substrate

Three capabilities the long-term goal demands that MDEMG's architecture cannot supply, by design:

**Continuous learning of new representations.** The long-term goal frames continuous learning as a system that updates its own representations over time without catastrophic forgetting. MDEMG cannot update representations; the embeddings are external and frozen. Even the LoRA training in PR #357/#358 doesn't address this — LoRA fine-tunes the LLM at the 16 call sites, not the embedding model the graph keys on. A successor framework that wants continuous representation learning needs a different substrate at the foundational level.

**World-model construction.** The long-term goal frames a system that "continuously predicts current state and projects to time t or -t." MDEMG has no prediction machinery. The graph stores observations; it does not maintain a generative model that produces predictions. Note 03's proposal (`:PREDICTS` schema, prediction-error promotion) is a step in this direction, but it is a graph-level addition over the existing substrate, not a substrate-level commitment to generative modeling. A successor framework that wants world models needs prediction as a first-class architectural commitment, not a graph schema extension.

**Recursive self-improvement that modifies the learning rules.** The long-term goal frames recursive self-improvement at multiple levels — the system improves its parameters, its architecture, and the rules by which it improves itself. RSIC modifies parameters. RSIC does not modify architecture or modify itself. Genuine recursive self-improvement requires a meta-learning substrate where the learning rules are themselves represented and modifiable. MDEMG's substrate represents knowledge (graph nodes), not learning rules.

These are not complaints. They are descriptions of what MDEMG is and is not. The R&D-vehicle framing fits exactly because MDEMG's operating in their absence has produced clear empirical familiarity with what their absence costs and what their presence would have to provide.

### §2.6 The architectural ceiling

MDEMG is a frozen-representation retrieval-and-constraint substrate. That is the architectural shape. Every subsystem fits in that shape: `embeddings` provides frozen representations, `learning` updates edge weights over them, `hidden` clusters them, `retrieval` queries over them, `jiminy` enforces constraints derived from them, `ape` (RSIC) tunes parameters of the system that does this.

The ceiling is not a failure of effort. It is a property of the architectural shape. Reaching the long-term goal from this shape would require either:

1. Replacing the frozen-representation foundation with a learned-representation foundation (the embedding layer becomes trainable, the manifold becomes continuously updated), and rebuilding everything that keys on it; or
2. Adding new substrate alongside the existing one (a generative-model layer, a representation-learning layer, a meta-learning layer) and re-architecting how the substrates compose; or
3. Building the successor framework on a different foundation entirely, with MDEMG as a template for the patterns that work (interceptor, multi-temporal RSIC, UxTS framework family, multi-LoRA serving) but not as the structural starting point.

The user's stated framing — MDEMG as template, successor as separate artifact — is option 3. This document operates within that framing. Options 1 and 2 are not zero-cost; in particular, option 1 ("rebuild everything that keys on the embedding layer") would require touching every subsystem in the codebase. Option 3 is the lowest-cost path that doesn't entrench MDEMG's ceiling in the successor.

### §2.7 What the long-term goal demands that MDEMG's architecture cannot supply

Restating §2.5 more sharply, the long-term goal demands four substrate-level commitments MDEMG does not have:

- **Trainable representations.** The substrate must support representation learning. MDEMG's embedding layer is external and frozen.
- **Generative modeling / prediction.** The substrate must produce predictions about current state and future state. MDEMG's graph stores observations; nothing produces predictions over them.
- **Reference-frame construction.** The substrate must let the system carve representational space into frames that support compositional reasoning. MDEMG inherits frames from external structure.
- **Meta-learning / mechanism modification.** The substrate must let the system modify its own learning rules over time. MDEMG has fixed mechanisms with parameter-only modification.

The successor framework's foundational document (§8) must commit to all four at the substrate level. The MDEMG R&D phase should produce empirical input to *how* each commitment should be implemented, drawn from the architectural intuitions generated by working without them. That is what makes MDEMG genuinely valuable as an R&D vehicle even after the architectural ceiling is recognized.

### §2.8 Where MDEMG is structurally well-positioned for the successor

The honest assessment is not that everything in MDEMG is dead-ended. Several patterns generalize cleanly to the successor framework and should be carried forward. Naming them explicitly helps planning agents distinguish carry-forward work (high leverage) from MDEMG-specific work that won't transfer (lower leverage in a fork-aware view).

**The interceptor pattern** (Guide → Validate → Evaluate → Track → Learn) is a generalizable architectural commitment. The closed feedback loop around agent action, with each component owning a distinct phase, is substrate-agnostic. The successor framework should commit to the pattern as a first-class element. The specific implementations (Jiminy as Guide, NLI as Track) are MDEMG-specific; the pattern is not.

**The multi-temporal RSIC scaffolding** (Micro/Meso/Macro temporal scales for self-improvement work) is a generalizable architectural commitment. The successor framework will need self-improvement at multiple temporal scales for similar reasons MDEMG does. The specific 19 reflection patterns are MDEMG-specific; the multi-temporal scaffolding is not.

**The UxTS framework family** (schema + spec + runner + CI as a uniform pattern for governing testable surfaces) is a generalizable architectural commitment, possibly MDEMG's strongest. The pattern produces empirically reliable governance across heterogeneous testable surfaces. The specific frameworks (UAITS, ULTS, UPTS) are MDEMG-specific instantiations; the pattern itself is substrate-agnostic and should be a first-class commitment in the successor.

**The constraint-and-evidence schema** in the graph (constraints with confidence, contradictions, frontiers, evidence counts) is a generalizable knowledge-representation pattern. The specific Cypher schema is MDEMG-specific; the conceptual layout — typed constraints, evidence-weighted decay, surfaced contradictions, identified frontiers — is not.

**The multi-LoRA serving infrastructure** being built right now via the FT-LORA workstream is generalizable. The pattern (one base model, many adapters, programmatic per-request adapter selection) is the production-ready realization of multi-task specialization. The specific adapter format and base model are MDEMG-specific; the pattern is substrate-agnostic and aligns with what the successor framework will need for per-domain specialization.

**The fail-open default** for LLM-backed capabilities (graceful degradation when the LLM provider is unavailable) is a generalizable resilience commitment. Every LLM-dependent capability in MDEMG degrades gracefully. The successor framework should inherit this commitment.

**The dual-gate regression discipline** (5a aggregate gate + 5b per-task gate) is a generalizable testing pattern for ML system updates. It catches the specific failure mode where aggregate metrics mask catastrophic per-task regressions. The successor framework should inherit this pattern wherever ML updates are gated.

These are the carry-forward template candidates. Each represents a real architectural commitment MDEMG has made, validated through use, and earned the right to forward to the successor. The §5 opportunities specification will name specific work to crystallize each as a substrate-agnostic pattern before fork.

### §2.9 The honest summary

**MDEMG is the right R&D vehicle. MDEMG is not the right production substrate.**

It is the right R&D vehicle because operating it for a year has produced architectural intuitions that are now coherent. The patterns in §2.8 are real. The architectural ceiling in §2.6 is also real. The user's framing — MDEMG as template, successor as separate artifact — fits the actual situation rather than imposing a frame on it.

It is not the right production substrate for the long-term goal because the long-term goal demands substrate-level commitments MDEMG does not have. Trainable representations, generative modeling, reference-frame construction, meta-learning — these are foundational, not bolt-on. A frozen-representation retrieval-and-constraint substrate cannot host them by graceful extension; it can only inform what their successor implementations should look like.

The remaining MDEMG R&D phase should be operated as the user has specified: continue until FT-LORA fully complete, extract the architectural intuitions formally, identify carry-forward templates and leave-behind decisions, then fork. The successor framework's foundational document is drafted in parallel with the late MDEMG R&D phase, not after.

The risks and opportunities specifications in §4 and §5 operate within this framing. Some risks endanger MDEMG's R&D outputs (R-INT-class). Some endanger the successor's tractability (R-SUC-class). Some endanger the long-term goal independent of MDEMG (R-LT-class). Some opportunities ship in MDEMG (O-MDEMG-class). Some prototype in MDEMG to inform the successor (O-PROTO-class). Some advance the long-term goal (O-LT-class). Some are substrate-agnostic (O-AGN-class). The classification matters because it determines sprint scoping.

---

## §3 — Fork-Timing Specification

This section operationalizes the fork gate. It specifies what "FT-LORA fully complete" means concretely, what additional state-of-the-codebase criteria should hold at fork time, what gets harder if the fork happens earlier, and what gets harder if it happens later. The output is a binary checklist planning agents can monitor; when all indicators are green, the fork is unblocked.

The user has stated that the right moment to fork is not before FT-LORA fully complete. This document accepts that as a hard constraint and specifies the gate around it. The user has also acknowledged a counter-pressure — design choices calcify the longer fork is delayed — which the document handles by adding additional fork-readiness criteria beyond just FT-LORA.

### §3.1 What "FT-LORA fully complete" means operationally

Three sub-conditions must hold:

**FT-1: Phase 11 GRPO either passes both gates or definitively does not.** Currently, PR #358's kl=0.10 retry is in flight (ETA 2026-04-27 ~17:00 UTC). The retry produces one of three outcomes:

- **Both gates pass (5a aggregate ≥ 0.8505 AND 5b no per-task regression > 2pp).** Adapter promotes from `-rl-sandbox/` to `-rl/`. Phase 11 closed. FT-1 satisfied with green signal.
- **Closer-but-not-passing (aggregate ≥ 0.85, fewer than 3 cap violations).** One more iteration warranted with per-task `samples_per_task_per_step` reweighting on remaining regressors. FT-1 deferred until that iteration lands.
- **No closer (3 regressors at similar magnitudes).** Evidence kl=0.05 was already the right regularization level. Pivot to per-task LoRA freezing (treat regressors as do-not-train), or ship run 5 with explicit per-task gate exemptions documented. FT-1 satisfied with red-acknowledged signal — the work is done, the answer is "not all 16 tasks are migrate-ready," but Phase 11 is closed in the operational sense.

The third outcome is operationally acceptable. FT-1 does not require all 16 tasks to be migrate-ready; it requires that Phase 11 has produced a definitive signal that allows the cost-replacement decision to be made for each call site individually.

**FT-2: Phase 12 HITL DPO either runs successfully or is consciously skipped.** Phase 12 is currently unblocked (the DPO pair generator from PR #349 stands on its own). Phase 12 either:

- **Runs and produces a DPO-tuned adapter that improves over the Phase 11 result on the C-group regressors and other persistent quality issues.** FT-2 satisfied.
- **Is consciously skipped because Phase 11's output is sufficient for the cost-replacement decision and HITL data collection effort exceeds expected benefit.** FT-2 satisfied with documented decision-and-rationale.

The skip option is genuine. If Phase 11 produces a result where 12-14 of 16 call sites are migrate-ready and the remaining 2-4 stay on gpt-5.4-mini in a hybrid routing strategy, Phase 12 may not be cost-effective. This is a user decision, not an automatic one.

**FT-3: Multi-adapter serving operational at all 16 call sites.** The packaging spec MDEMG-FT-PKG-001 must be fully implemented: `mdemg model fuse` working, adapter distribution channel established (HF Hub or alternative), `llmclient` capable of programmatic per-request adapter selection. The 16 call sites must each be configured to route to the correct provider (local Qwen3-14B-RL, gpt-5.4-mini, or hybrid based on call-site identity).

The empirical validation criterion: a sustained operational period (1+ week) at production volume where the multi-adapter routing produces no regressions on call-site behavior. PR #358's per-task table is the input that determines call-site routing; the validation is that the routing actually works in production.

### §3.2 Beyond FT-LORA: additional fork-gate criteria

Even with FT-LORA complete, two additional criteria should hold at fork time. These exist because the user noted MDEMG as template requires the template to be in good shape — not in mid-refactor — when the fork happens.

**FG-1: Documentation freshness. AGENT_HANDOFF.md, CHANGELOG.md, and the gap analysis document are within ~2 days of HEAD.** The current state (1-2 PR documentation lag at HEAD) is incompatible with using MDEMG as a template for a successor — the successor's planning would operate against stale documentation. Fork-readiness includes a documentation freshness gate.

The mandatory documentation phase rule the user has instituted is the operational mechanism. Fork-readiness measurement: every PR within the last 2 weeks before fork has triggered the documentation phase. Spot-check by sampling.

**FG-2: Architectural intuitions formally documented.** The successor framework's foundational document (§8) requires inputs from MDEMG R&D. By fork time, those inputs must be drafted, even if the foundational document itself is still in development. Specifically: each carry-forward template candidate from §2.8 is documented as a substrate-agnostic pattern with a precise specification, separated from its MDEMG-specific implementation.

This is sprint-able work; it should be triggered partway through FT-LORA's late phase rather than waiting until FT-LORA is complete. Better to enter the fork with the architectural intuitions already captured than to capture them retrospectively when memory of operational details has decayed.

### §3.3 The fork-readiness checklist

Five binary indicators planning agents can monitor:

- [ ] **FT-1:** Phase 11 GRPO retry produces a definitive signal (both gates pass, or closer-but-not-passing iteration completes, or no-closer pivot decision made and documented)
- [ ] **FT-2:** Phase 12 HITL DPO either runs or is consciously skipped with documented rationale
- [ ] **FT-3:** Multi-adapter serving operational at all 16 call sites with 1+ week of production validation
- [ ] **FG-1:** AGENT_HANDOFF.md, CHANGELOG.md, gap analysis within 2 days of HEAD; mandatory doc phase compliance verified for last 2 weeks of PRs
- [ ] **FG-2:** Architectural intuitions documented (each §2.8 carry-forward template specified as substrate-agnostic pattern)

When all five indicators are green, the fork is unblocked. Planning agents should track these as a top-level dashboard signal and report fork-readiness percentage in every sprint retrospective during the late MDEMG R&D phase.

### §3.4 What gets harder if the fork happens earlier

Three failure modes activate if the fork happens before the gate is satisfied:

**Forking around unresolved infrastructure.** If FT-LORA is mid-flight at fork time, the successor inherits an unresolved infrastructure question. Multi-LoRA serving is the single largest production-readiness commitment MDEMG is making right now; not knowing whether it works in production at the 16 call sites means the successor designs its own adapter-serving without empirical signal. The successor is then forced to either (a) wait for MDEMG to finish FT-LORA before its own adapter-serving design can be validated, or (b) commit to a different design without the empirical signal MDEMG would have produced. Both options are worse than waiting.

**Less empirical signal on which abstractions are load-bearing.** Many of the abstractions in MDEMG that look generic (the interceptor pattern, the multi-temporal RSIC, the UxTS family) became generic *through use*. Forking before they have been used in production for longer means forking with hypotheses about which abstractions are real and which are accidental. The hypothesis quality is bounded by operational time.

**Re-work in both branches.** If MDEMG continues independently after fork (because production users still depend on it), and the successor proceeds in parallel, design changes that would have been single-branch updates become dual-branch updates. The branch overhead compounds. Fork timing should minimize branch divergence in the period when both branches are operationally important.

### §3.5 What gets harder if the fork happens later

Two failure modes activate if the fork is delayed past the gate:

**Design choices calcify.** Each new feature added to MDEMG after the fork-readiness gate is achieved without a "would I build this differently in the successor?" review locks in more of MDEMG's specific choices. The longer this continues, the more MDEMG-specific the eventual template becomes, and the harder it is to extract the substrate-agnostic patterns from the substrate-specific implementations. The gate is partially what triggers the discipline of asking the question on every new feature.

**Architectural-intuitions output decays.** The user's stated long-term goal requires architectural intuitions that are most accessible immediately after the operational experience that produced them. Delaying the fork delays the moment when those intuitions are formally captured, increasing the risk that they decay or get rationalized away.

### §3.6 Recommendation

Track the fork-readiness checklist starting now. Do not fork before all five indicators green. Do not delay past the point where new MDEMG features are landing without the "would I build this differently?" review.

Specifically: when 3-of-5 indicators are green, begin drafting the successor framework's foundational document (§8). This produces parallel progress and surfaces design tensions while MDEMG can still be used to validate or refute them empirically. When 5-of-5 indicators are green, the fork is unblocked and can be executed when convenient.

The user's stated reasoning ("forking before LoRA fully complete makes a worse template") is correct. The counter-pressure ("design choices calcify the longer fork is delayed") is also correct. The fork-readiness checklist balances both by (a) requiring FT-LORA completion, (b) requiring documentation freshness so the template is usable, and (c) requiring architectural intuitions documented so the carry-forward work is captured before fork rather than retrospectively.

---
## §4 — Risks Specification

This section catalogs risks organized by what they threaten, not by component. Each entry uses the uniform template specified in Appendix A. Planning agents should treat entries as records to decompose into sprint backlogs, not as prose to interpret.

The four classes of risk by what's threatened:

- **R-FT** — Risks to the immediate FT-LORA completion gate
- **R-INT** — Risks to the architectural intuitions the work is supposed to produce
- **R-SUC** — Risks to the successor framework's tractability
- **R-LT** — Risks to the long-term goal independent of MDEMG

Severity ratings: **Critical** (compromises the strategic frame if unaddressed), **High** (compromises a sprint outcome if unaddressed), **Medium** (degrades a sprint outcome if unaddressed), **Low** (creates work that planning agents should track but not prioritize).

### §4.1 Risks to the immediate FT-LORA completion gate

#### R-FT-1: Phase 11 kl=0.10 retry fails per-task gate, blocking adapter promotion

- **What is threatened:** FT-1 fork-gate indicator (§3.1). The implementation chain is proven correct end-to-end through PR #357 and #358, but adapter promotion to `-rl/` blocks if the per-task gate continues to fail.
- **Severity:** High
- **Confidence:** Confident — the kl=0.05 → kl=0.10 hypothesis is reasonable but not certain to hold. The three persistent C-group regressors share a profile (small row counts, single-token-flip sensitivity at temp=0) that is structurally hard to fix with regularization alone.
- **Observable indicators:** PR #358 retry result lands; if 5a aggregate ≥ 0.8505 AND 5b no per-task regression > 2pp, R-FT-1 deactivates. If 5b still trips on `consulting.classify`, `consulting.synthesis`, or `retrieval.query_classify`, R-FT-1 is active.
- **Prerequisites for mitigation:** Retry result available. Per-task breakdown documented (the pattern PR #358 established).
- **Recommended action:** If retry produces "closer but not passing" outcome (aggregate ≥ 0.85, fewer than 3 cap violations), execute one more iteration with `samples_per_task_per_step` reweighting on the remaining regressors. If retry produces "no closer" outcome (3 regressors at similar magnitudes), pivot to per-task LoRA freezing strategy (treat the 3 regressors as do-not-train) OR ship run 5 with explicit per-task gate exemptions documented and acknowledge that 13/16 call sites are migrate-ready while 3 stay on gpt-5.4-mini.
- **Success criteria:** FT-1 fork-gate indicator green. Either both gates pass on a single training run, or the per-task exemption decision is documented and the cost-replacement strategy is updated to reflect the hybrid routing.
- **Trigger conditions for re-evaluation:** PR #358 retry result lands. Re-evaluate R-FT-1 status within 24 hours of result.
- **Cross-references:** R-FT-2 (HITL DPO becomes more important if Phase 11 doesn't fully close per-task), O-MDEMG-2 (gpt-5.4-mini benchmark needed for cost-quality decision regardless of outcome).

#### R-FT-2: Phase 12 HITL DPO data collection effort exceeds expected benefit

- **What is threatened:** FT-2 fork-gate indicator (§3.1). HITL DPO is unblocked but the operational cost of collecting human-judged preference pairs may not justify the marginal quality improvement over Phase 11's RL-tuned base.
- **Severity:** Medium
- **Confidence:** Speculative — the cost-benefit balance depends on Phase 11's final outcome (R-FT-1 dependency) and on the user's willingness to spend HITL annotation effort. No empirical signal yet on actual HITL effort required.
- **Observable indicators:** Time spent on HITL annotation vs. quality delta produced. If first 100 preference pairs cost > 4 hours of annotation and produce < 1pp quality improvement, R-FT-2 is active.
- **Prerequisites for mitigation:** R-FT-1 resolved. Phase 12 begins with measurable annotation effort tracking.
- **Recommended action:** Time-box Phase 12 to a fixed annotation budget (e.g., 200 preference pairs or 8 hours of annotation, whichever comes first). Measure quality delta. Decide explicitly: continue, ship Phase 11 result as final, or pivot.
- **Success criteria:** FT-2 fork-gate indicator green. Either Phase 12 produces a measurable improvement that justifies the effort, or Phase 12 is consciously skipped with documented rationale.
- **Trigger conditions for re-evaluation:** Phase 12 begins; first annotation budget consumed.
- **Cross-references:** R-FT-1, O-PROTO-2 (DPO pair generator could be repurposed for prediction-error-based learning experiments).

#### R-FT-3: Multi-adapter serving has unsurfaced quality issues at production volume

- **What is threatened:** FT-3 fork-gate indicator (§3.1). Even if Phase 11 and Phase 12 produce passing adapters, the actual serving infrastructure at the 16 call sites may exhibit issues only visible at production volume — adapter loading latency under load, quality drift on call sites with idiosyncratic prompts not represented in the benchmark, race conditions in adapter-switching at high request rates.
- **Severity:** High
- **Confidence:** Moderate — multi-LoRA serving is well-understood in vLLM/SGLang/TGI, but MDEMG's specific routing logic at the `llmclient` layer is custom and hasn't been operated at scale.
- **Observable indicators:** Production telemetry from the 16 call sites after multi-adapter routing is enabled. Specifically: adapter-load p99 latency, per-call-site response quality vs. baseline, error rate on adapter-switching boundaries.
- **Prerequisites for mitigation:** R-FT-1 resolved with a passing adapter. `llmclient` updated for multi-adapter routing. Production telemetry instrumented.
- **Recommended action:** Run multi-adapter serving in shadow mode (production traffic, but responses logged-not-served) for 1 week before flipping to active routing. Compare shadow responses against gpt-5.4-mini baselines on the same call sites. If quality holds, flip active. If quality degrades on specific call sites, rollback those call sites to gpt-5.4-mini and document.
- **Success criteria:** FT-3 fork-gate indicator green. 1+ week of production validation at active routing with no rollbacks. Per-call-site routing decisions documented.
- **Trigger conditions for re-evaluation:** Multi-adapter routing enabled in shadow or active mode.
- **Cross-references:** R-FT-1, O-MDEMG-2 (gpt-5.4-mini benchmark provides comparison baseline).

#### R-FT-4: Single-developer velocity bottleneck blocks fork-readiness completion

- **What is threatened:** All FT-class fork-gate indicators. The user is the sole developer; concurrent sprints across two dev branches mitigate but do not eliminate the velocity ceiling.
- **Severity:** Medium
- **Confidence:** Confident — already documented as R1 in `risk-opp-04232026-01.md`.
- **Observable indicators:** Sprint cycle time. Documentation phase compliance (FG-1 indicator). Backlog depth at PRs in flight.
- **Prerequisites for mitigation:** Existing structural mitigations in place (concurrent sprint planning, mandatory documentation phase rule, planning agents executing in parallel).
- **Recommended action:** This is structural and largely outside the planning agents' control. Surface to user when sprint cycle time degrades (>3 days for previously 1-day cycles) or when documentation phase compliance drops below 90%. Recommend prioritization adjustments or sprint scope reductions.
- **Success criteria:** Sprint cycle time stable; documentation phase compliance ≥ 90% for last 2 weeks of PRs.
- **Trigger conditions for re-evaluation:** Continuous monitoring through sprint planning.
- **Cross-references:** FG-1 fork-gate indicator, O-AGN-1 (publishing operational lessons reduces future single-developer load by attracting collaborators).

### §4.2 Risks to the architectural intuitions the work is supposed to produce

#### R-INT-1: Frozen-representation ceiling mistaken for property of memory-graph systems generally

- **What is threatened:** The successor framework's substrate-level commitment to trainable representations (§2.7). If the architectural intuition that emerges from MDEMG is "memory-graph systems are limited to retrieval over external embeddings," the successor will be designed within that constraint rather than against it.
- **Severity:** Critical
- **Confidence:** Moderate — depends on how the carry-forward documentation work is framed. The risk activates if the framing implicitly accepts MDEMG's substrate boundaries as natural.
- **Observable indicators:** The architectural intuitions documentation (FG-2 fork-gate). If "frozen embeddings" appears as a default assumption in the substrate-agnostic pattern descriptions, R-INT-1 is active.
- **Prerequisites for mitigation:** The §2.5–§2.7 framing of this document. Distinguishing what MDEMG learns vs. what the long-term goal demands.
- **Recommended action:** Every carry-forward template documentation explicitly lists the substrate dependency it inherits from MDEMG and whether that dependency carries forward. The interceptor pattern carries forward (substrate-agnostic). The DBSCAN-based emergence does not (frozen-embedding-dependent). The UxTS framework family carries forward (substrate-agnostic). The Hebbian edge-weight learning rule does not (representation-frozen-dependent). Make these distinctions explicit in every pattern documented.
- **Success criteria:** Architectural intuitions documentation distinguishes substrate-agnostic patterns from substrate-dependent ones; no carry-forward template implicitly accepts frozen-representation as a generic constraint.
- **Trigger conditions for re-evaluation:** Each carry-forward template documentation reviewed by user before fork.
- **Cross-references:** R-INT-2, R-INT-3, FG-2 fork-gate indicator, §8.

#### R-INT-2: Interceptor pattern treated as MDEMG idiom rather than generalizable architectural commitment

- **What is threatened:** §2.8 carry-forward asset. The interceptor pattern (Guide → Validate → Evaluate → Track → Learn) is one of MDEMG's strongest patterns and a high-leverage template for the successor. If it carries forward documented as "Jiminy" rather than as a substrate-agnostic pattern, the successor's interceptor design will inherit MDEMG-specific assumptions.
- **Severity:** High
- **Confidence:** Confident — the pattern is real and generalizable; the risk is purely about documentation framing.
- **Observable indicators:** Whether the interceptor pattern carry-forward documentation specifies the abstract pattern (5 components, distinct phases, closed feedback loop) before the MDEMG instantiation, or after. Order matters for clarity.
- **Prerequisites for mitigation:** Recognition that the pattern is substrate-agnostic.
- **Recommended action:** Carry-forward template documentation for the interceptor pattern: lead with the abstract specification (each phase's responsibilities, contract between phases, what kind of state crosses each phase boundary), then provide MDEMG's Jiminy as a reference implementation. The successor framework's foundational document (§8) treats the abstract pattern as a first-class commitment, not as "Jiminy ported forward."
- **Success criteria:** Interceptor pattern documented as substrate-agnostic; planning agents and user can describe the pattern without reference to MDEMG.
- **Trigger conditions for re-evaluation:** Architectural intuitions documentation work begins.
- **Cross-references:** R-INT-1, O-AGN-4, §2.8.

#### R-INT-3: RSIC's parameter-tuning surface conflated with architectural self-improvement

- **What is threatened:** The successor framework's recursive-self-improvement substrate commitment (§2.7). RSIC is parameter tuning over fixed mechanisms (§2.4). If RSIC carries forward as the model for "recursive self-improvement," the successor will inherit RSIC's scope rather than expand it. The long-term goal demands more.
- **Severity:** Critical
- **Confidence:** Confident — the conflation risk is real because "Recursive Self-Improvement Cycle" is RSIC's name. Vocabulary drift is the failure mode.
- **Observable indicators:** Whether the architectural intuitions documentation describes RSIC accurately (calibrated parameter search inside a designed envelope) or aspirationally (recursive self-improvement). The user's project memory note already captures this distinction; the documentation work needs to preserve it.
- **Prerequisites for mitigation:** §2.4 framing in this document. The user's existing accurate framing in project memory.
- **Recommended action:** RSIC carry-forward documentation explicitly distinguishes "RSIC-as-implemented" (parameter tuning, fixed envelope, multi-temporal scaffolding) from "recursive self-improvement as the long-term goal demands" (mechanism-modifying, architecture-modifying, rule-modifying). The carry-forward asset is the multi-temporal scaffolding pattern, not the parameter-tuning scope. The successor framework's foundational document specifies what mechanism-level recursive self-improvement requires beyond what RSIC provides.
- **Success criteria:** RSIC's actual scope is documented accurately; the multi-temporal scaffolding is identified as the carry-forward asset; the successor's recursive-self-improvement substrate commitment is named separately as more ambitious.
- **Trigger conditions for re-evaluation:** Architectural intuitions documentation work begins.
- **Cross-references:** R-INT-1, R-INT-2, O-LT-4, §2.4, §2.8.

#### R-INT-4: Documentation debt at sprint velocity erodes architectural-intuitions output

- **What is threatened:** FG-1 and FG-2 fork-gate indicators. AGENT_HANDOFF.md and CHANGELOG.md are 1-2 PRs stale at HEAD (per §1.11). The mandatory documentation phase rule was instituted to prevent exactly this; documentation lag at the most recent PRs suggests the rule is not always applied.
- **Severity:** High
- **Confidence:** Confident — the lag is directly observable. The user's project memory note already names this pattern.
- **Observable indicators:** Per-PR documentation phase compliance. Time elapsed between merge and documentation update. AGENT_HANDOFF.md last-updated date vs. HEAD merge date.
- **Prerequisites for mitigation:** Mandatory documentation phase rule enforced as a sprint-completion gate. Planning agents reject sprint-complete signals if documentation phase not executed.
- **Recommended action:** Planning agents enforce mandatory documentation phase as a hard gate on sprint completion, not a soft suggestion. Every sprint plan must include the documentation phase as the final task. No sprint is complete without CHANGELOG, AGENT_HANDOFF, CLAUDE.md, cli-reference.md (where relevant), feature docs, homebrew-mdemg CHANGELOG + beta testing guide (where relevant), and submodule pointer update. Spot-check every 5th PR; if compliance < 90%, escalate.
- **Success criteria:** AGENT_HANDOFF.md within 1 day of HEAD; per-PR documentation phase compliance ≥ 90% over rolling 2-week window.
- **Trigger conditions for re-evaluation:** Continuous monitoring through sprint planning.
- **Cross-references:** FG-1, R-FT-4 (single-developer velocity is the underlying pressure).

### §4.3 Risks to the successor framework's tractability

#### R-SUC-1: Fork happens before infrastructure abstractions stabilize

- **What is threatened:** The successor's first-six-months tractability. If the fork happens during active FT-LORA work, the successor inherits hypotheses about adapter-serving rather than empirical signal. Re-work in both branches becomes likely.
- **Severity:** Critical
- **Confidence:** Confident — this is what the user's stated fork-timing constraint exists to prevent.
- **Observable indicators:** Fork-readiness checklist state. If the fork is executed with FT-1, FT-2, or FT-3 still red, R-SUC-1 has activated.
- **Prerequisites for mitigation:** §3 fork-readiness checklist enforced. No fork before all 5 indicators green.
- **Recommended action:** Planning agents do not execute fork-related sprint work while any fork-readiness indicator is red. Substrate-agnostic carry-forward documentation can proceed in parallel; substrate decisions for the successor wait.
- **Success criteria:** Fork executes with all 5 fork-readiness indicators green.
- **Trigger conditions for re-evaluation:** Continuous monitoring of fork-readiness checklist.
- **Cross-references:** R-FT-1, R-FT-2, R-FT-3, §3.

#### R-SUC-2: Fork happens after MDEMG-specific design choices calcify

- **What is threatened:** The successor's substrate-agnostic foundation. Each new feature added to MDEMG after fork-readiness is achieved without "would I build this differently in the successor?" review locks in MDEMG-specific assumptions.
- **Severity:** High
- **Confidence:** Moderate — the calcification risk is real but bounded if the discipline of asking the question is maintained.
- **Observable indicators:** Time elapsed between fork-readiness gate green and actual fork execution. Number of new MDEMG features added in that interval without successor-impact review.
- **Prerequisites for mitigation:** Successor framework foundational document (§8) drafted at 3-of-5 fork-readiness green, finalized at 5-of-5.
- **Recommended action:** Planning agents track time elapsed since fork-readiness gate green. If > 4 weeks at 5-of-5 without fork execution, surface to user for re-evaluation. New features added during this interval must include "successor-impact" assessment in the sprint plan.
- **Success criteria:** Fork executes within 4 weeks of 5-of-5 fork-readiness; or fork delay is consciously decided with documented rationale.
- **Trigger conditions for re-evaluation:** Monthly review of fork-readiness duration.
- **Cross-references:** R-SUC-1, FG-2.

#### R-SUC-3: Successor inherits MDEMG's vocabulary uncritically

- **What is threatened:** The successor's substrate-level architectural commitments. If the successor uses "reference frame" to mean what MDEMG means by reference frame (file paths) when the long-term goal demands learned reference frames, the successor's design is corrupted by vocabulary drift before it begins.
- **Severity:** Critical
- **Confidence:** Confident — the vocabulary drift is named in §2.3 and §2.4 and is a real failure mode.
- **Observable indicators:** Whether the successor framework's foundational document distinguishes MDEMG-vocabulary terms from long-term-goal-vocabulary terms. The §8 specification of this document is the input.
- **Prerequisites for mitigation:** §2 alignment assessment internalized; §8 successor-doc specification.
- **Recommended action:** The successor framework's foundational document includes an explicit terminology section that distinguishes inherited-from-MDEMG terms (with their MDEMG-specific meaning) from successor-introduced terms (with their long-term-goal-aligned meaning). "Reference frame" in particular gets distinguished — "MDEMG-style inherited frames" vs. "learned reference frames." Same for "emergence," "self-improvement," "memory."
- **Success criteria:** Successor's foundational document has explicit terminology section that prevents vocabulary drift.
- **Trigger conditions for re-evaluation:** Successor's foundational document drafting begins.
- **Cross-references:** R-INT-1, R-INT-3, §2.3, §2.4, §8.

#### R-SUC-4: No formal mechanism to extract architectural lessons from MDEMG before fork

- **What is threatened:** The architectural-intuitions output of MDEMG R&D. Without a formal extraction mechanism, lessons are tacit knowledge that lives in the user's head and risks being lost or rationalized away during fork transition.
- **Severity:** High
- **Confidence:** Moderate — depends on whether FG-2 fork-gate criterion is taken seriously as a sprint-able output rather than a vague aspiration.
- **Observable indicators:** Whether each §2.8 carry-forward template has a written substrate-agnostic specification before fork. Whether each MDEMG-specific design decision the successor would inherit has a documented rationale.
- **Prerequisites for mitigation:** FG-2 fork-gate criterion enforced. Architectural-intuitions documentation work scoped as deliverable sprints.
- **Recommended action:** Plan a series of "architectural intuition extraction" sprints starting at 3-of-5 fork-readiness green. One sprint per carry-forward template (interceptor pattern, multi-temporal RSIC, UxTS framework family, constraint-and-evidence schema, multi-LoRA serving, fail-open default, dual-gate regression). Each sprint produces a written specification of the substrate-agnostic pattern, separated from its MDEMG-specific implementation.
- **Success criteria:** All §2.8 carry-forward templates have written substrate-agnostic specifications. Specifications are usable inputs to the successor framework's foundational document.
- **Trigger conditions for re-evaluation:** Fork-readiness reaches 3-of-5; extraction sprints scheduled.
- **Cross-references:** FG-2, R-INT-1, R-INT-2, R-INT-3, O-LT-1 through O-LT-5, §8.

### §4.4 Risks to the long-term goal independent of MDEMG

#### R-LT-1: Long-term goal pursued as sequence of incremental MDEMG features rather than substrate-level research program

- **What is threatened:** Substrate-level commitments needed for the long-term goal (§2.7). If trainable representations, generative modeling, reference-frame construction, and meta-learning are pursued as MDEMG bolt-ons, each one becomes constrained by MDEMG's existing architecture and inherits its ceiling.
- **Severity:** Critical
- **Confidence:** Confident — this is the central risk the R&D-vehicle framing is meant to address. The risk activates if the framing slips and MDEMG features are added as if they were the long-term goal's implementation.
- **Observable indicators:** Whether new sprints during the late MDEMG R&D phase target the long-term goal directly (substrate-level work) or indirectly (MDEMG features that approximate long-term-goal capabilities). Sprints that add `:PREDICTS` schema to MDEMG, for instance, are MDEMG-bolt-on; sprints that specify how prediction-as-substrate works in the successor are substrate-level.
- **Prerequisites for mitigation:** §2 alignment assessment internalized. Sprint scoping that distinguishes MDEMG features from successor substrate work.
- **Recommended action:** Every sprint that proposes adding capabilities to MDEMG that resemble long-term-goal commitments goes through a triage: is this an MDEMG operational improvement, an MDEMG prototype to inform the successor (O-PROTO class), or genuine substrate-level work for the successor (O-LT class)? The first two are appropriate for the late MDEMG R&D phase. The third should be deferred to post-fork — but specified during pre-fork in the successor's foundational document.
- **Success criteria:** No sprint during late MDEMG R&D phase adds long-term-goal-shaped capabilities to MDEMG without explicit triage as MDEMG-feature, prototype-to-inform-successor, or substrate-level-deferred. Triage is documented in the sprint plan.
- **Trigger conditions for re-evaluation:** Continuous monitoring of sprint plans during late MDEMG R&D phase.
- **Cross-references:** R-INT-1, O-LT-1 through O-LT-5, O-PROTO-1 through O-PROTO-5, §2.7.

#### R-LT-2: Eight architectural extensions in research-evaluation document treated as exhaustive

- **What is threatened:** The successor framework's substrate completeness. The eight notes (02–09) cover precision-weighted Hebbian, top-down predictions, column-voting retrieval, context-specific activations, sparse retrieval activation, forward-forward shallow heads, HTM sequence memory, active-inference unification. They are valuable extensions to MDEMG. They are not a complete specification of what the successor framework needs.
- **Severity:** High
- **Confidence:** Confident — `mdemg-research-evaluation.md` itself flagged the manifesto-coverage gap (object-centric slot structure not addressed by any of the 8 notes). The risk is the eight notes being treated as the substrate-design backlog.
- **Observable indicators:** Whether the successor framework's foundational document discusses substrate-level commitments beyond what the eight notes propose. If §8 of this spec drives a foundational document that limits itself to "implement notes 02–09 properly," R-LT-2 has activated.
- **Prerequisites for mitigation:** §2.5 and §2.7 framing of substrate-level commitments. The manifesto's identified hard gap (object-centric slots) explicitly named.
- **Recommended action:** The successor framework's foundational document treats the eight notes as one input among several, not as the complete backlog. Specifically, the foundational document includes substrate-level work the eight notes do not address: object-centric slot structure (manifesto's hard gap), trainable representation layer (§2.5), generative-model substrate, meta-learning substrate. The eight notes inform implementation choices once the substrate is specified.
- **Success criteria:** Successor framework's foundational document has explicit substrate-level commitments separate from research-note implementation choices.
- **Trigger conditions for re-evaluation:** Successor's foundational document drafting begins.
- **Cross-references:** R-LT-1, R-LT-3, §2.5, §2.7, §8.

#### R-LT-3: Object-centric representation gap deferred indefinitely

- **What is threatened:** The manifesto's identified hard gap. Object-centric representation is the manifesto's flagged unaddressed gap among the five inductive biases (sparse distributed reps, structure-content factorization, **object-centric slots**, action-conditioned prediction, layer-local objectives). If the gap is deferred indefinitely because it requires substrate changes MDEMG cannot accommodate, it never gets addressed.
- **Severity:** High
- **Confidence:** Moderate — the gap is real and acknowledged; whether it gets deferred depends on the successor framework's scoping.
- **Observable indicators:** Whether the successor framework's foundational document includes object-centric slot structure as a substrate-level commitment. If §8 leads to a foundational document that defers object-centric representation to a later iteration, R-LT-3 is active.
- **Prerequisites for mitigation:** §2.5 framing. The manifesto's identified gap. The user's awareness that this gap is hard and substrate-level.
- **Recommended action:** Object-centric slot structure is named as a first-class substrate-level commitment in the successor framework's foundational document. Implementation specification (slot attention vs. DETR-style queries vs. neural object representation vs. other) is a research question for the successor's first sprints, not for MDEMG R&D. The substrate-level commitment must exist in the foundational document even if implementation is deferred.
- **Success criteria:** Successor framework's foundational document names object-centric slot structure as a substrate-level commitment with implementation deferred to first successor sprints, not to indefinite future.
- **Trigger conditions for re-evaluation:** Successor's foundational document drafting begins.
- **Cross-references:** R-LT-2, O-LT-5, §2.5.

#### R-LT-4: Continuous learning approached additively rather than as substrate redesign

- **What is threatened:** The substrate-level commitment to trainable representations (§2.7). If continuous learning is added to MDEMG via incremental features (precision-weighted Hebbian, prediction-error promotion, sparse activation), each addition further entrenches the frozen-representation architecture by working around it rather than replacing it.
- **Severity:** Critical
- **Confidence:** Moderate — the risk activates if the eight architectural extensions are implemented in MDEMG as the path to continuous learning rather than as prototypes informing the successor's substrate redesign.
- **Observable indicators:** Whether sprints implementing the eight notes are scoped as MDEMG features (additive) or as MDEMG prototypes (informing successor substrate). The O-PROTO opportunity class in §5 makes this distinction explicit.
- **Prerequisites for mitigation:** O-PROTO opportunity class enforced. Sprints implementing notes 02–09 explicitly scoped as prototypes-to-inform-successor with clear measurement criteria for what the prototype teaches.
- **Recommended action:** Sprints implementing any of the eight architectural extensions in MDEMG must declare prototype scope: what hypothesis is being tested, what the result will inform in the successor's substrate design. The prototype produces a result and a recommendation, not a permanent MDEMG feature. The scope discipline prevents additive entrenchment.
- **Success criteria:** Every sprint implementing notes 02–09 has documented prototype scope, hypothesis, and successor-substrate informational target. No sprint implements an architectural extension as a permanent MDEMG feature.
- **Trigger conditions for re-evaluation:** Each sprint plan implementing one of the eight notes.
- **Cross-references:** R-LT-1, R-LT-2, O-PROTO-1 through O-PROTO-5.

---

## §5 — Opportunities Specification

This section catalogs opportunities organized by what they advance. Each entry uses the uniform template specified in Appendix A. Opportunities are tagged by fork-relationship: `pre-fork`, `fork-gating`, `post-fork`, or `substrate-agnostic`. Planning agents do not sprint on `post-fork` items during the remaining MDEMG R&D phase (per §6.3).

The four classes of opportunity by what's advanced:

- **O-MDEMG** — Opportunities to ship in MDEMG (pre-fork)
- **O-PROTO** — Opportunities to prototype in MDEMG to inform the successor (pre-fork experiments)
- **O-LT** — Opportunities the long-term goal imposes that current planning hasn't surfaced
- **O-AGN** — Substrate-agnostic opportunities (do regardless of timing)

Value ratings: **High** (advances multiple goals, time-sensitive, or required for fork-readiness), **Medium** (advances at least one goal materially), **Low** (advances a goal but with smaller leverage).

### §5.1 Opportunities to ship in MDEMG (pre-fork)

#### O-MDEMG-1: Complete the FT-LORA workstream end-to-end

- **What it advances:** FT-1, FT-2, FT-3 fork-gate indicators (§3.1). Cost-replacement of the 16 gpt-5.4-mini call sites with local Qwen3-14B-RL adapter at the migrate-ready ones.
- **Fork-relationship:** fork-gating
- **Value rating:** High
- **Confidence:** Confident — this is the user's stated path to fork-readiness.
- **Prerequisites:** PR #358 kl=0.10 retry result. Phase 12 HITL DPO decision (run or skip with rationale). Multi-adapter serving infrastructure operational.
- **Recommended action:** Continue the FT-LORA sprint chain through completion. Per R-FT-1, handle the kl=0.10 retry result with the three-outcome decision tree. Per R-FT-2, time-box Phase 12 if it runs. Per R-FT-3, run multi-adapter serving in shadow mode for 1 week before active routing.
- **Success criteria:** All three FT fork-gate indicators green. Cost-replacement operational at 12-16 of 16 call sites (the exact count depends on Phase 11/12 outcomes for C-group regressors).
- **Resource estimate:** 2-4 sprints depending on retry outcomes. Currently in flight.
- **Cross-references:** R-FT-1, R-FT-2, R-FT-3, FT-1, FT-2, FT-3, §1.8.

#### O-MDEMG-2: Run gpt-5.4-mini on the Phase 10 benchmark to close the cost-quality tradeoff curve

- **What it advances:** The cost-replacement decision specifically. PR #358 explicitly named this as "the missing piece for the cost-replacement decision." Without a gpt-5.4-mini number on the same 16-task Phase 10 benchmark, "RL-merged adapter is cost-effective" is structurally unverifiable — base vs. RL-merged is measured, but base vs. gpt-5.4-mini and RL-merged vs. gpt-5.4-mini are not.
- **Fork-relationship:** pre-fork
- **Value rating:** High
- **Confidence:** Confident — PR #358 documents this gap and estimates ~$10–30 OpenAI spend, ~30 min wall-clock.
- **Prerequisites:** Phase 10 benchmark runner operational (already true). OpenAI API access (already true). User authorization for ~$10-30 spend.
- **Recommended action:** Run a one-shot Phase 10 benchmark with `--model gpt-5.4-mini` (or equivalent harness configuration). Capture per-task and per-metric breakdown matching the existing baseline / RL-merged tables. Append to `docs/development/ft-lora/phase_11_mlx_adapter.md`. Single sprint, well-bounded.
- **Success criteria:** gpt-5.4-mini number on the same 16-task Phase 10 benchmark, with per-task breakdown comparable to existing tables. Cost-quality tradeoff curve fully populated.
- **Resource estimate:** 1 sprint, 1 day.
- **Cross-references:** O-MDEMG-1, R-FT-3, §1.8.

#### O-MDEMG-3: Migrate T-group call sites to RL-merged adapter pre-Phase-12

- **What it advances:** Cost reduction at the 5 T-group call sites (`ape.reflect`, `jiminy.synthesize`, `metalearn.generalize`, `summarize.generate`, `hidden.summarize`) where PR #358's data shows RL-merged is empirically better than baseline. C-group call sites stay on gpt-5.4-mini until R-FT-1 closes. This is hybrid routing operationalized today.
- **Fork-relationship:** pre-fork
- **Value rating:** Medium
- **Confidence:** Moderate — the per-task data is from one training run; some additional validation may be wanted before active routing flip on production traffic.
- **Prerequisites:** O-MDEMG-1 in progress (RL-merged adapter exists). Multi-adapter serving infrastructure operational. `llmclient` capable of per-call-site routing.
- **Recommended action:** Wire `llmclient` for per-call-site routing with the 5 T-group sites mapped to RL-merged adapter and the remaining 11 to gpt-5.4-mini. Run in shadow mode for 1 week (per R-FT-3). Compare shadow responses to current production. If quality holds, flip the 5 T-group sites to active routing. Document the 11 that stay on gpt-5.4-mini and why.
- **Success criteria:** 5 T-group call sites operational on RL-merged adapter without quality regression. Cost reduction measurable. Documented routing decision per call site.
- **Resource estimate:** 1-2 sprints, 1-2 weeks.
- **Cross-references:** O-MDEMG-1, R-FT-3, §1.8.

#### O-MDEMG-4: Address DBSCAN O(n²) scale ceiling (only if pre-fork; otherwise defer to successor)

- **What it advances:** Operational scale of MDEMG itself. The DBSCAN scale wall (§1.4) bites at large graph sizes. R3 in `risk-opp-04232026-01.md` flags this; O6 proposes alternatives (LSH, HDBSCAN, Leiden, GAT-based clustering).
- **Fork-relationship:** pre-fork (only sprint this if before fork; otherwise defer to successor)
- **Value rating:** Medium
- **Confidence:** Moderate — the scale wall is real but bounded at present graph sizes. Pre-fork value depends on whether MDEMG's operational scale is bumping into the wall during the late R&D phase.
- **Prerequisites:** Empirical signal that DBSCAN is operationally limiting (large graph operations slowing, memory pressure during clustering). If no operational signal, defer.
- **Recommended action:** Decision sprint first: research spike on LSH vs. HDBSCAN vs. Leiden vs. GAT-based clustering for the specific MDEMG use case. Output is a decision document with a chosen approach and an effort estimate. Implementation sprint follows only if (a) MDEMG is hitting the wall operationally during late R&D phase, and (b) the chosen approach is substrate-agnostic enough to carry forward to the successor or to be left behind cleanly.
- **Success criteria:** Decision document produced. Implementation only if conditions (a) and (b) both hold.
- **Resource estimate:** Decision sprint: 1 sprint, 3 days. Implementation: 2-4 sprints if pursued.
- **Cross-references:** R3 in `risk-opp-04232026-01.md`, O6 in same. §1.4.

#### O-MDEMG-5: Wire authorization enforcement (close GAP-16)

- **What it advances:** Operational completeness of MDEMG. The user's project memory notes that authentication is complete (4 methods: none, API key, JWT, SAML), but authorization is NOT implemented — `Principal.Metadata["scopes"]` is parsed from JWT but never enforced by any handler. GAP-16 effort is Medium per the gap analysis.
- **Fork-relationship:** pre-fork
- **Value rating:** Medium
- **Confidence:** Confident — the gap is named and effort-estimated.
- **Prerequisites:** Identity plumbing already in place. Decision on which handlers should enforce which scopes.
- **Recommended action:** Sprint to add scope enforcement at handler level. Specifically: middleware that checks `Principal.Metadata["scopes"]` against handler-declared required scopes. Per-handler required-scope declarations as a first pass cover the API surface. Tests added to UATS specs to ensure enforcement is checked at CI.
- **Success criteria:** Scope enforcement operational at the API surface. Per-handler required-scope declarations documented. UATS specs check enforcement.
- **Resource estimate:** 1-2 sprints.
- **Cross-references:** GAP-16 in `mdemg-gap-analysis.md`. The user's project memory note on this gap.

### §5.2 Opportunities to prototype in MDEMG to inform the successor (pre-fork experiments)

These opportunities implement architectural extensions in MDEMG specifically to learn from them, with results feeding the successor framework's substrate design. Each prototype must declare scope: what hypothesis is being tested, what the result informs in the successor (per R-LT-4 mitigation).

#### O-PROTO-1: Precision-weighted Hebbian (Note 02) — small experiment in existing graph

- **What it advances:** Empirical signal on whether precision-weighted Hebbian updates change retrieval quality enough to justify the schema change in the successor. Note 02's proposal: scale eta by `c_a · c_b` where `c_a, c_b` are confidence values per node.
- **Fork-relationship:** pre-fork (prototype)
- **Value rating:** Medium
- **Confidence:** Moderate — note 02's proposal is concrete and small in scope. The hypothesis (precision-weighting improves retrieval quality) is testable. Whether the empirical answer transfers to the successor's substrate-level Hebbian-like rule depends on the successor's substrate.
- **Prerequisites:** Note 02 reviewed; experimental scope agreed with user. Confidence values per node already exist in the schema (constraints, evidence_count fields); precision-weighting can be derived from existing fields.
- **Recommended action:** Implement precision-weighted variant of `HebbianWeightUpdate` behind a feature flag. Run A/B on retrieval quality benchmarks (the same benchmarks used for the +58.4% trajectory). Report quality delta and effort estimate for production deployment. Result feeds the successor's substrate-level decision on whether confidence-weighted updates are first-class.
- **Success criteria:** A/B result published. Recommendation to successor substrate design (precision-weighting first-class, optional, or not used).
- **Resource estimate:** 2 sprints.
- **Cross-references:** Note 02 in `mdemg-research-evaluation.md`. R-LT-4.

#### O-PROTO-2: Top-down prediction edges (Note 03) — schema-level experiment

- **What it advances:** Empirical signal on whether prediction-error-driven promotion produces better concept hierarchies than frequency-driven promotion. Note 03's proposal: add `:PREDICTS` schema, accumulate prediction errors per parent-child pair, promote on prediction-error magnitude.
- **Fork-relationship:** pre-fork (prototype)
- **Value rating:** High — this is the closest existing prototype to the successor's substrate-level prediction commitment. The empirical signal directly informs §2.7's "generative modeling / prediction" commitment.
- **Confidence:** Moderate — note 03's proposal is detailed but the schema change is large. Prototype scope must be tightly bounded.
- **Prerequisites:** Note 03 reviewed. Sub-graph or specific subsystem (e.g., one layer of the hierarchy) chosen as prototype scope.
- **Recommended action:** Implement `:PREDICTS` schema for one layer transition (e.g., L2 → L1). Compare prediction-error-driven promotion against existing frequency-driven promotion on the same data. Measure: emergent-concept stability, retrieval quality on the named concepts, false-positive rate of promotions. Result feeds the successor's substrate-level prediction commitment.
- **Success criteria:** Prototype produces measurable comparison. Recommendation to successor on whether prediction-error promotion is substrate-level worth or not.
- **Resource estimate:** 3-4 sprints.
- **Cross-references:** Note 03 in `mdemg-research-evaluation.md`. R-LT-4. §2.7.

#### O-PROTO-3: Column-voting retrieval with RRF (Note 04) — config-flag experiment

- **What it advances:** Empirical signal on whether ensemble retrieval with reciprocal rank fusion outperforms the current weighted-linear-combination ranker. Note 04's proposal: replace weighted-linear ranker with 6-column RRF ensemble.
- **Fork-relationship:** pre-fork (prototype)
- **Value rating:** Medium
- **Confidence:** Confident — RRF is well-understood; the prototype is implementation, not research.
- **Prerequisites:** Note 04 reviewed. Existing retrieval benchmarks operational.
- **Recommended action:** Implement RRF ensemble alongside existing ranker behind a config flag. A/B on retrieval quality benchmarks. Document quality delta and operational cost (latency, complexity). Result feeds the successor's retrieval architecture.
- **Success criteria:** A/B result published. Recommendation on which retrieval architecture (weighted-linear vs. RRF) the successor adopts.
- **Resource estimate:** 2 sprints.
- **Cross-references:** Note 04 in `mdemg-research-evaluation.md`. R-LT-4.

#### O-PROTO-4: Forward-Forward shallow heads (Note 07) — narrowest scope prototype

- **What it advances:** Empirical signal on whether layer-local objectives (one of the manifesto's five inductive biases) are tractable in MDEMG's architecture. Note 07's proposal: forward-forward learning on shallow heads attached to specific subsystems.
- **Fork-relationship:** pre-fork (prototype)
- **Value rating:** Medium
- **Confidence:** Speculative — forward-forward in this context is research-grade. The prototype is exploratory.
- **Prerequisites:** Note 07 reviewed. Specific subsystem chosen as prototype host (probably the neural sidecar reranker, where layer-local training is least disruptive).
- **Recommended action:** Implement forward-forward shallow head as an alternative training path for one specific subsystem (suggest the reranker). Compare against backprop training on the same task. Measure: training stability, learned representation quality, convergence behavior. Result feeds the successor's substrate-level decision on whether layer-local objectives are first-class.
- **Success criteria:** Prototype produces measurable comparison or definitive negative result. Recommendation to successor.
- **Resource estimate:** 3-4 sprints.
- **Cross-references:** Note 07 in `mdemg-research-evaluation.md`. R-LT-4. Manifesto's five inductive biases.

#### O-PROTO-5: A representation-learning experiment MDEMG cannot host

- **What it advances:** Concrete demonstration of what MDEMG's substrate cannot do. The most valuable prototype may be the one that *fails* — a deliberate attempt to fine-tune the embedding layer or add a learnable embedding adapter, surfacing exactly which subsystems break and how. The empirical signal of "this experiment cannot work in MDEMG" is the input that justifies the substrate-level commitment in the successor.
- **Fork-relationship:** pre-fork (prototype, deliberately exploratory)
- **Value rating:** High — though counterintuitive. Negative results are the most informative.
- **Confidence:** Speculative — the experiment design is itself part of the work.
- **Prerequisites:** §2.5 framing internalized. Willingness to invest in an experiment expected to fail by design.
- **Recommended action:** Design a prototype that attempts to fine-tune the embedding layer (or add a learnable embedding adapter) for one subsystem. Document every architectural assumption that breaks: graph schema, edge weight semantics, retrieval ranking, RSIC's health metrics. The "everything that breaks" list is the input to the successor's substrate-level commitment. Time-box to 1-2 sprints; the goal is demonstration, not production.
- **Success criteria:** Documented list of architectural assumptions that break. List feeds the successor's foundational document as evidence for why representation-learning must be substrate-level rather than additive.
- **Resource estimate:** 1-2 sprints.
- **Cross-references:** §2.5, R-INT-1, R-LT-4, O-LT-1.

### §5.3 Opportunities the long-term goal imposes that current planning hasn't surfaced

These opportunities specify substrate-level work for the successor framework. Planning agents do not execute these as MDEMG sprints. They are sprint-able only after fork. Pre-fork, planning agents can sprint on the *specification* of these opportunities (§8 deliverables) but not on the implementation.

#### O-LT-1: Define the successor framework's representation-learning substrate

- **What it advances:** §2.7's "trainable representations" commitment. The successor framework needs a substrate where representations are learned and updated, not received as a service.
- **Fork-relationship:** post-fork (specification work pre-fork)
- **Value rating:** High
- **Confidence:** Speculative on implementation; confident on the commitment requirement.
- **Prerequisites:** Successor framework's foundational document (§8) drafted to the point of substrate commitments.
- **Recommended action:** Pre-fork: specify what "trainable representations" means in the successor's foundational document. Compare candidate approaches (learned embedding adapters, end-to-end trainable representation layer, replay-buffer-based continual learning, etc.). Post-fork: implementation sprints based on the specified approach.
- **Success criteria:** Substrate-level commitment specified in the foundational document. Implementation tracked separately.
- **Resource estimate:** Specification: 1-2 sprints. Implementation: post-fork, multi-month research program.
- **Cross-references:** §2.5, §2.7, O-PROTO-5, §8.

#### O-LT-2: Define the successor framework's reference-frame mechanism

- **What it advances:** Genuinely learned reference frames as a substrate-level commitment, distinguished from MDEMG-style inherited frames (§2.3).
- **Fork-relationship:** post-fork (specification work pre-fork)
- **Value rating:** High
- **Confidence:** Speculative — what "learned reference frames" means concretely is itself part of the specification work.
- **Prerequisites:** Successor framework's foundational document. R-SUC-3 mitigation (terminology section distinguishing inherited from learned frames).
- **Recommended action:** Pre-fork: specify what learned reference frames mean in the successor's substrate. Compare candidate approaches (HTM-style cortical columns with grid-cell-like coding, learned hierarchical position encodings, slot-based reference frames). Decide what minimum viable form the substrate provides; reserve more elaborate forms for post-fork research.
- **Success criteria:** Reference-frame mechanism specified in the foundational document with terminology distinguished from MDEMG-style inheritance.
- **Resource estimate:** Specification: 2-3 sprints. Implementation: post-fork.
- **Cross-references:** §2.3, §2.7, R-SUC-3, §8.

#### O-LT-3: Define the successor framework's world-model component

- **What it advances:** Generative modeling as a substrate-level commitment. The long-term goal's "continuously predicts current state and projects to time t or -t" requires a generative model that can roll out future states and explain past states.
- **Fork-relationship:** post-fork (specification work pre-fork)
- **Value rating:** High
- **Confidence:** Speculative — generative-model substrate design is research-grade.
- **Prerequisites:** Successor framework's foundational document. O-PROTO-2 result (informs whether prediction-edge prototyping in MDEMG produced useful signal).
- **Recommended action:** Pre-fork: specify what generative modeling means in the successor substrate. Candidate approaches include world-model latent-state prediction (Dreamer-style), embedding-space prediction (JEPA-style), graph-structured prediction (MDEMG's `:PREDICTS` schema scaled to substrate level). Decide minimum viable form.
- **Success criteria:** World-model component specified in the foundational document.
- **Resource estimate:** Specification: 2-3 sprints. Implementation: post-fork, multi-month research program.
- **Cross-references:** §2.5, §2.7, O-PROTO-2, §8.

#### O-LT-4: Define the successor framework's recursive-self-improvement mechanism

- **What it advances:** Recursive self-improvement that modifies architecture and learning rules, distinguished from RSIC's parameter-tuning scope (§2.4, R-INT-3).
- **Fork-relationship:** post-fork (specification work pre-fork)
- **Value rating:** High
- **Confidence:** Speculative — mechanism-modifying self-improvement is open research; safety properties are non-trivial.
- **Prerequisites:** Successor framework's foundational document. Carry-forward documentation of MDEMG's RSIC including explicit scoping (R-INT-3 mitigation).
- **Recommended action:** Pre-fork: specify what mechanism-modifying recursive self-improvement means. Compare candidate approaches (meta-learning over learning rules, neural architecture search, learned-update-rule networks). Specify safety envelope: what kinds of modification are permitted, what verification is required. Reserve full implementation for post-fork research.
- **Success criteria:** Recursive-self-improvement mechanism specified in the foundational document with safety envelope explicit. Distinguished from RSIC.
- **Resource estimate:** Specification: 2-3 sprints. Implementation: post-fork, multi-month research program.
- **Cross-references:** §2.4, R-INT-3, §8.

#### O-LT-5: Define the successor framework's object-centric slot structure

- **What it advances:** The manifesto's identified hard gap (R-LT-3). Object-centric representation as a substrate-level commitment.
- **Fork-relationship:** post-fork (specification work pre-fork)
- **Value rating:** High
- **Confidence:** Speculative — the manifesto's gap is named, but the implementation specification is research-grade.
- **Prerequisites:** Successor framework's foundational document. Acknowledgment of the manifesto's gap as substrate-level not deferrable (R-LT-3 mitigation).
- **Recommended action:** Pre-fork: specify what object-centric slot structure means in the successor's substrate. Compare candidate approaches (slot attention, DETR-style query slots, neural object representation). Specify what minimum viable form the substrate provides — at minimum, persistent entity identity that survives context changes; at maximum, full compositional binding. Reserve elaborate implementations for post-fork.
- **Success criteria:** Object-centric slot structure specified in the foundational document with minimum and target forms distinguished.
- **Resource estimate:** Specification: 2-3 sprints. Implementation: post-fork research program.
- **Cross-references:** §2.5, R-LT-3, §8. Manifesto's identified hard gap.

### §5.4 Substrate-agnostic opportunities (do regardless of timing)

These opportunities advance goals independent of fork timing. They can sprint at any point during MDEMG R&D phase or after fork without scope conflict.

#### O-AGN-1: Publish operational lessons from FT-LORA workstream

- **What it advances:** Field knowledge. The FT-LORA workstream has produced multiple generalizable operational lessons: the silent gradient-flow bug from runs 1-4 (closure-captured `Module` references in `mx.checkpoint` produce zero gradients with no error), the dual-gate (5a aggregate + 5b per-task) regression discipline, the MoE→Dense pivot under the Metal MTLResource cap, the kl-coefficient parameter-tuning discipline. Each is broadly useful to the field.
- **Fork-relationship:** substrate-agnostic
- **Value rating:** Medium-High — though the value is in field contribution rather than direct MDEMG/successor work.
- **Confidence:** Confident — the lessons are real and well-documented internally.
- **Prerequisites:** FT-LORA workstream complete (per O-MDEMG-1) so the lessons can be presented as completed cases rather than in-flight observations.
- **Recommended action:** After FT-LORA fork-readiness, draft a public-facing post-mortem covering the key operational lessons. Distinct from the collaboration brief (which is research-direction-focused); this is operational-reproducibility-focused. Forum: technical blog, or a public companion-document in the repository.
- **Success criteria:** Published post-mortem accessible to the field. Lessons documented in a way that lets others avoid the same bugs.
- **Resource estimate:** 1 sprint.
- **Cross-references:** O-MDEMG-1, R-FT-4 (publishing reduces single-developer load by attracting collaborators).

#### O-AGN-2: Externalize J17 protocol as standalone spec

- **What it advances:** Protocol adoption beyond MDEMG. The J17 AI-to-AI Communication Protocol (three-tier encoding, HMAC-signed session tickets, ML tier prediction) is generalizable beyond MDEMG. Existing O4 in `risk-opp-04232026-01.md` proposed this; remains valid.
- **Fork-relationship:** substrate-agnostic
- **Value rating:** Medium
- **Confidence:** Confident — the protocol is mature and well-documented internally.
- **Prerequisites:** Stable J17 protocol surface (already true).
- **Recommended action:** Extract J17 protocol specification from MDEMG's internal documentation into a standalone specification document. Include the encoding scheme, the session ticket format, the tier prediction interface. Publish as a standalone artifact (separate repository or RFC-style document).
- **Success criteria:** Standalone J17 specification published. Specification is consumable without MDEMG context.
- **Resource estimate:** 2 sprints.
- **Cross-references:** O4 in `risk-opp-04232026-01.md`. §1.5.

#### O-AGN-3: Build MCP marketplace presence

- **What it advances:** MDEMG adoption (and through it, signal on which patterns matter to other users). Existing O7 in `risk-opp-04232026-01.md` proposed this; remains valid.
- **Fork-relationship:** substrate-agnostic
- **Value rating:** Medium
- **Confidence:** Confident — the MCP ecosystem is established; submission process is well-defined.
- **Prerequisites:** MDEMG operational at v0.6.x (already true). MCP server integration mature (already true).
- **Recommended action:** Submit to MCP marketplace. Create "MDEMG for Claude Code" landing document highlighting the 60% token reduction and Phase 104 MCP guardrails. Submit to Cursor integration directory. Track install analytics.
- **Success criteria:** Marketplace listing live. Install count baseline established.
- **Resource estimate:** 1-2 sprints.
- **Cross-references:** O7 in `risk-opp-04232026-01.md`.

#### O-AGN-4: Document the multi-temporal RSIC pattern as generalizable architectural contribution

- **What it advances:** Carry-forward template work (R-INT-3 mitigation). The multi-temporal RSIC scaffolding (Micro/Meso/Macro temporal scales for self-improvement work) is generalizable beyond MDEMG. Documenting it as a substrate-agnostic pattern is valuable both for the successor and for the field.
- **Fork-relationship:** substrate-agnostic
- **Value rating:** Medium
- **Confidence:** Confident — the pattern is real and the documentation work is well-scoped.
- **Prerequisites:** R-INT-3 mitigation (RSIC carry-forward documentation distinguishes the parameter-tuning scope from the multi-temporal pattern).
- **Recommended action:** Document the multi-temporal RSIC pattern as a substrate-agnostic architectural contribution. Pattern: 5-stage cycle (Assess → Reflect → Plan → Execute → Validate) at 3 temporal scales (Micro: minutes; Meso: hours; Macro: days). Substrate-agnostic specification: each stage's responsibilities, contracts between stages, calibration requirements. MDEMG's RSIC as reference implementation.
- **Success criteria:** Substrate-agnostic specification of multi-temporal RSIC pattern. Specification usable as input to the successor framework's foundational document and as a standalone field contribution.
- **Resource estimate:** 1-2 sprints.
- **Cross-references:** R-INT-3, §2.8, §8.

---
## §6 — Sprint-Generation Directives

This section gives planning agents direct instructions on how to consume the document. The directives are operational, not interpretive. Follow them mechanically.

### §6.1 How to read this specification

Read sections in order. §0 establishes the lens — strategic frame, calibration, confidence levels. §1 establishes the facts — what is in the codebase. §2 establishes the alignment assessment — what the codebase can and cannot support relative to the long-term goal. §3 establishes the gate — when fork is unblocked. §4 and §5 are the operational backlog. §7 captures unresolved questions for the user. §8 looks ahead.

A planning agent that skips §0–§3 and goes directly to §4–§5 will produce sprint plans that miss the strategic frame. Don't do that. Read in order.

### §6.2 Sprint scoping rules

Every sprint plan generated from this specification must declare four things explicitly:

1. **Which risk(s) and/or opportunity ID(s) the sprint addresses.** No sprint is generated without an entry in §4 or §5 to back it. A sprint that doesn't trace to a §4 or §5 entry is invented scope and gets rejected.

2. **Fork-relationship.** Stated as one of: `pre-fork`, `fork-gating`, `post-fork`, `substrate-agnostic`. Inherits from the §5 entry's fork-relationship if the sprint is opportunity-driven. Inherits from the risk's mitigation timing if risk-driven.

3. **Prerequisites met.** The §4 or §5 entry's prerequisites must be verified true before the sprint is scheduled. If a prerequisite is not met, the sprint is deferred or chained behind a prerequisite-completion sprint.

4. **Success criteria committed.** The §4 or §5 entry's success criteria must be the sprint's definition-of-done at sprint level. Per-epic and per-story success criteria are derived from the entry's success criteria, not invented.

The discipline is mechanical. Planning agents that skip any of these four declarations have generated invalid sprint plans.

### §6.3 What is out of scope for sprint planning

**Hard exclusions during the late MDEMG R&D phase:**

- **All `post-fork` opportunities (O-LT-1 through O-LT-5) for implementation.** Specification work on these opportunities is in scope as `pre-fork` (specification produces inputs to the successor's foundational document). Implementation is not.
- **Anything that would require the successor's substrate.** Examples: trainable representation layers in MDEMG's graph, world-model latent-state prediction in MDEMG's pipeline, learned reference-frame mechanisms replacing MDEMG's tree-sitter-derived frames. These are substrate-level changes the successor will make, not changes MDEMG should make additively.
- **Anything that would entrench MDEMG's architectural ceiling.** Per R-LT-4: sprints that approach long-term-goal capabilities by adding features to MDEMG's frozen-representation substrate. The right scope for these is O-PROTO (prototype to inform successor), not permanent feature.

**Soft exclusions (in scope but require triage):**

- **Any sprint that proposes adding capabilities resembling long-term-goal commitments to MDEMG.** Per R-LT-1, these go through triage: MDEMG operational improvement (proceed), MDEMG prototype to inform successor (O-PROTO scope), or substrate-level work for successor (O-LT scope; specification only pre-fork).
- **Any sprint exceeding 4 weeks of scope.** Long sprints lose calibration. Decompose into smaller sprints with explicit prerequisite chaining.
- **Any sprint without documentation phase task.** Per R-INT-4 and FG-1 mitigation, mandatory documentation phase is non-negotiable. Sprints without a documentation phase task get rejected and re-planned.

### §6.4 Dependency-respect rules

Sequencing rules planning agents must respect:

**Risk-mitigation dependencies:**

- R-INT-1, R-INT-2, R-INT-3 mitigations all require the architectural-intuitions documentation work (FG-2). Sequence after FG-2 begins.
- R-SUC-1 mitigation requires fork-readiness checklist enforcement. No fork-related sprints execute while any FT or FG indicator is red.
- R-SUC-3 mitigation requires the §8 successor-doc specification. Successor-vocabulary work is sequenced after §8 work begins.
- R-LT-1, R-LT-4 mitigation requires the O-PROTO opportunity scoping discipline. Sprints that touch architectural extensions must be O-PROTO scoped, not O-MDEMG scoped.

**Opportunity prerequisites:**

- O-MDEMG-1 (FT-LORA completion) is the precondition for FT fork-gate indicators. Scheduled first; other O-MDEMG opportunities serialize behind it.
- O-MDEMG-2 (gpt-5.4-mini benchmark) can run in parallel with O-MDEMG-1; it does not block FT-LORA completion.
- O-MDEMG-3 (T-group migration) requires O-MDEMG-1 partial result (RL-merged adapter exists) but does not require O-MDEMG-1 fully complete.
- O-PROTO opportunities (1-5) can run in parallel with each other and with O-MDEMG opportunities. Each prototype is independent of the others.
- O-LT specification work (1-5) can begin once fork-readiness is 3-of-5 green (per §3.6). Specification produces inputs to §8; §8 produces the successor's foundational document.
- O-AGN opportunities (1-4) are substrate-agnostic; sequence at convenience.

**Cross-class dependencies:**

- O-PROTO-2 (top-down prediction edges prototype) and O-LT-3 (world-model component specification) are linked. The prototype's empirical signal informs the specification. Sequence O-PROTO-2 before O-LT-3 specification when timing allows.
- O-PROTO-5 (representation-learning experiment) and O-LT-1 (trainable representations specification) are linked. Same reasoning — prototype before specification when timing allows.
- R-LT-3 (object-centric gap deferral) and O-LT-5 (object-centric specification) are explicitly linked. The risk activates if the specification doesn't happen pre-fork.

### §6.5 Reporting format for sprint outputs

Every sprint completion report must include:

1. **Sprint summary** — what risks/opportunities were addressed (by ID), what was shipped.
2. **Outcome against success criteria** — for each addressed entry, whether success criteria were met, partially met, or not met.
3. **Fork-readiness checklist update** — current state of the 5 indicators (FT-1, FT-2, FT-3, FG-1, FG-2). Marked as green / red / partial. Reasoning for partial states.
4. **New observations** — anything surfaced during the sprint that doesn't fit existing risks or opportunities. These are inputs to future spec revisions; do not add new risks/opportunities mid-sprint.
5. **Open questions surfaced for user** — items belonging in §7 of a future spec revision. The planning agent does not make decisions on these; they're surfaced for the user.

The report is consumable by a future planning agent (or a future sprint cycle of the same agent) without re-deriving context.

### §6.6 Re-planning triggers

This specification is not static. Trigger conditions for spec revision:

- **Fork-readiness state changes substantively.** When any indicator flips red ↔ green, spec is reviewed for re-calibration.
- **A risk activates with observable indicators.** When R-FT-1, R-FT-2, R-FT-3, R-INT-4 (and so on) show observable indicators of activation, the spec is reviewed for whether the recommended action is still appropriate.
- **A user decision changes scope.** If the user decides to fork early, defer fork, change the long-term goal scoping, or revise the strategic frame, the spec is revised.
- **PR #358 retry result lands.** Per R-FT-1's trigger condition, the spec is reviewed within 24 hours of the retry result.
- **Quarterly review even if no triggers fire.** A spec that doesn't get revised quarterly during a fast-moving R&D phase has decayed.

Revisions update §1 reconnaissance findings, §3 fork-readiness checklist state, §4 risks active/inactive flags, §5 opportunities completion state, §7 open-questions list. The strategic frame in §0 and the alignment assessment in §2 do not change without explicit user decision.

---

## §7 — Open Questions Surfaced by Reconnaissance

This section catalogs questions surfaced during reconnaissance that the document cannot resolve. Planning agents bring these to the user; planning agents do not invent answers.

Questions are split into three categories. Architectural questions need engineering investigation and may be resolvable by deeper code inspection. Strategic questions need the user's judgment on direction or priority. Calibration questions are about this document's own calibration and may revise the document.

### §7.1 Architectural questions

**Q-A-1: Is the GAP-01 walkCodebase fallback bridge intended to land before fork or after?** The user's project memory notes GAP-01 as adding a fallback bridge in `walkCodebase` for community parsers via the existing gRPC `IngestionModule.Parse()` interface. The 27 core parsers are Go regex/AST compiled into the binary by design. The fallback bridge would let community parsers extend the system. The question is timing: is this work that completes MDEMG's parser story before fork, or is it deferred to the successor where parser extensibility might be designed differently?

**Q-A-2: Is the V0023 self-healing migration's batched dedup pattern a one-off or generalizable?** The user's project memory notes V0023 introduced SymbolNode natural-key MERGE on `(space_id, name, file_path, symbol_type)` replacing hash-based `symbol_id`, with batched dedup before constraint creation. The pattern (heal data with batched dedup, then add constraint) may or may not be the canonical resolution for migration safety in MDEMG. If canonical, future migrations should follow it. If one-off, the pattern is a fix for a specific situation. Clarification helps planning agents apply or not apply the pattern in future migration work.

**Q-A-3: Is the SymbolNode natural-key MERGE the canonical resolution or interim?** Related to Q-A-2. Natural-key MERGE replaces hash-based identity. Some downstream code may still reference `symbol_id`; some may have been updated. The question: is this transition complete, or is there interim cruft that planning agents should treat as scheduled cleanup?

**Q-A-4: What does the planning agent's role look like when the document directs it to "specify what X means" (e.g., O-LT-1 through O-LT-5)?** Specification work for substrate-level commitments is genuinely novel work. The question: does the planning agent draft specifications and surface to user for approval, or does the planning agent surface the question and wait for the user to draft specifications? Different process implications.

**Q-A-5: Is there an existing place in the codebase or docs where carry-forward template work should live?** The architectural-intuitions documentation (FG-2) needs a home. Options: a dedicated `docs/successor-framework/` directory, a series of separate documents in `docs/development/`, a single consolidated document. The user's preference shapes how planning agents structure the FG-2 work.

**Q-A-6: Is the kl=0.10 retry result available at the time of reading this document?** The retry was in flight at PR #358 merge with ~13 hr wall-clock. ETA was 2026-04-27 ~17:00 UTC. By the time planning agents read this spec, the retry result is either available or imminent. Confirmation of result state determines whether R-FT-1 is currently active.

**Q-A-7: How many of the 16 LLM call sites does the user expect to migrate to the local adapter under the realistic Phase 11 + Phase 12 outcome?** PR #358's data suggests 5 T-group sites are migration-ready today. C-group sites are TBD. PR #358 itself frames "12-14 of 16" as a realistic outcome. The user's expectation calibration helps planning agents scope the multi-adapter routing work and the success criteria for FT-3.

### §7.2 Strategic questions

**Q-S-1: Is the manifesto's object-centric slot structure something MDEMG should attempt before fork (as O-PROTO) or is it explicitly successor-only?** R-LT-3 mitigation includes object-centric slot structure as substrate-level commitment in the successor. The question: should MDEMG also prototype object-centric structure (perhaps via a new node type) to surface what the implementation looks like, or is the substrate dependency too strong for prototype scope?

**Q-S-2: Is the long-term goal's "project to time t or -t" something MDEMG should prototype on its existing graph?** The graph has a temporal dimension (decay, evidence_count). A prototype that uses the graph to project past or future state would surface what's possible and what's not. The question: useful prototype, or scope creep into territory the successor's substrate is better suited for?

**Q-S-3: What does "recursive self-improvement" mean operationally for the successor — modify the substrate's connectivity? Modify the learning rules? Modify the learning rules' modification rules?** R-INT-3 and O-LT-4 both flag this. The question is genuine and the answer shapes substrate design. The user's framing of the long-term goal suggests all three levels, but the implementation tractability varies dramatically.

**Q-S-4: What is the user's appetite for the negative-result prototype O-PROTO-5?** The most informative prototype may be one that fails. The question is whether the user wants planning agents to scope O-PROTO-5 as a 1-2 sprint commitment or to treat it as too speculative for sprint cycles.

**Q-S-5: How should the successor framework relate operationally to MDEMG after fork?** Three plausible models: (a) MDEMG continues independently as production tooling, successor proceeds in parallel as research; (b) MDEMG is frozen as a reference, no further development; (c) MDEMG's late-phase users migrate to the successor over time. The choice affects how aggressively to invest in MDEMG operational work pre-fork.

**Q-S-6: Is the FT-LORA completion gate strict, or is "FT-LORA mostly complete with hybrid routing operational" sufficient?** PR #358's framing acknowledges that 12-14 of 16 call sites migrate-ready is a realistic outcome. The question: does fork-readiness require all 16 migrated, or is the hybrid routing strategy sufficient evidence of completion? §3.1 specified the latter; user confirmation prevents miscalibration.

**Q-S-7: How time-sensitive is the fork-timing window?** The user's stated reasoning is that fork-readiness must be achieved before fork. The unstated question is whether there's an upper bound — a point at which deferring fork costs more than forking. The §3.5 framing names this; the user's calibration on the upper bound shapes how aggressively planning agents push on FG-1 and FG-2.

### §7.3 Calibration questions

**Q-C-1: Does the §2 alignment assessment land at the right calibration?** The section is the sharpest in the document. It says MDEMG has an architectural ceiling, that "concept emergence" is clustering plus naming (not what the manifesto frames), that "reference frames" are inherited not learned, that "self-improvement" is parameter tuning not architectural. These are accurate but they may land too sharply. The user's calibration determines whether to soften, sharpen, or leave as-is.

**Q-C-2: Is the entry template (uniform fields per risk and per opportunity) the right structure for planning-agent consumption, or should it be more compact?** The current template has 8 fields per risk and 9 per opportunity. Verbose but mechanical to consume. The user's experience with the planning agents' actual workflow determines whether this is the right verbosity.

**Q-C-3: Is the fork-readiness checklist's 5 indicators the right granularity?** Fewer indicators (e.g., 3) might be easier to track. More indicators (e.g., 8) might surface more failure modes. The current 5 (FT-1, FT-2, FT-3, FG-1, FG-2) reflects a balance; the user's preference shapes future revisions.

---

## §8 — Specification for the Successor Framework's Foundational Document

The successor framework is a separate artifact, not a future version of MDEMG. It will need a foundational document before its first sprint. This section specifies what that document needs to contain, what work in MDEMG R&D is supposed to produce inputs to it, and the relationship between this specification (governing MDEMG R&D) and that document (governing post-fork work).

### §8.1 The successor framework's foundational document — purpose

The successor framework's foundational document specifies the architectural commitments the successor makes at the substrate level. It is the analog of MDEMG's `VISION.md` but with two key differences:

- **It is written before implementation begins**, not after 105 phases of work. The successor benefits from the architectural intuitions MDEMG R&D produced. Those intuitions go into the foundational document as commitments rather than emerging through retrospective documentation.
- **It commits at the substrate level**, not at the application level. MDEMG's `VISION.md` describes an application (cognitive substrate for AI coding agents). The successor's foundational document describes a substrate (a continuous-learning ANN with BNN-inspired topologies, reference frames, world models, recursive self-improvement). Applications are downstream of the substrate.

The foundational document is the operational contract between the user's long-term goal (which is research-grade and high-level) and the successor's first implementation sprint (which is concrete and bounded). Without the foundational document, the gap is too wide; the first sprint either over-scopes (attempts the entire long-term goal at once) or under-scopes (incrementally implements something that doesn't reach the substrate).

### §8.2 What the foundational document must contain

Five substrate-level commitments and five governance commitments:

**Substrate commitments (the long-term goal made architecturally concrete):**

1. **Trainable representations.** The substrate supports continuous representation learning. The foundational document specifies the chosen approach (per O-LT-1) and the contract: how the rest of the system interacts with a representation layer that updates over time.

2. **Reference-frame mechanism.** The substrate supports learned reference frames distinguished from MDEMG-style inherited frames. The foundational document specifies the chosen approach (per O-LT-2) — at minimum, reference frames are learned; at target, reference frames support compositional reasoning across distributed columns.

3. **World-model component.** The substrate supports generative modeling — current-state prediction and projection forward and backward in time. The foundational document specifies the chosen approach (per O-LT-3) and the contract with the rest of the system (how predictions are produced, consumed, and validated).

4. **Recursive self-improvement mechanism.** The substrate supports self-improvement at architecture and rule levels, distinguished from RSIC's parameter-tuning scope. The foundational document specifies the chosen approach (per O-LT-4) and the safety envelope (what kinds of modification are permitted, what verification is required).

5. **Object-centric slot structure.** The substrate supports persistent entity identity that survives context changes — the manifesto's identified hard gap, addressed at substrate level. The foundational document specifies the chosen approach (per O-LT-5) at minimum (persistent identity) and target (full compositional binding) levels.

**Governance commitments (the carry-forward templates from MDEMG):**

6. **The interceptor pattern.** Five-component closed feedback loop around action (Guide → Validate → Evaluate → Track → Learn). Substrate-agnostic specification per R-INT-2 mitigation. MDEMG's Jiminy is a reference implementation.

7. **Multi-temporal RSIC scaffolding.** Five-stage cycle (Assess → Reflect → Plan → Execute → Validate) at three temporal scales (Micro / Meso / Macro). Substrate-agnostic per R-INT-3 mitigation. Distinguished from RSIC's scope (parameter tuning); the successor's recursive self-improvement extends beyond RSIC's scope.

8. **The UxTS framework family pattern.** Schema + spec + runner + CI as a uniform pattern for governing testable surfaces. Substrate-agnostic. The successor framework adopts the pattern; specific frameworks (UAITS, ULTS, UPTS) may carry forward, transform, or be replaced based on substrate fit.

9. **Constraint-and-evidence schema.** Typed constraints with confidence, contradictions, frontiers, evidence counts. Substrate-agnostic. The successor's knowledge representation may differ from MDEMG's graph but inherits the conceptual layout.

10. **Multi-LoRA serving infrastructure.** One base model, many adapters, programmatic per-request adapter selection. Substrate-agnostic — the successor framework will need per-domain specialization with similar infrastructure regardless of what the base model architecture is.

### §8.3 What MDEMG R&D should produce as inputs

The foundational document is not written from scratch. The MDEMG R&D phase produces specific inputs:

**Architectural intuitions (FG-2 fork-gate work):** Each of the five governance commitments has a substrate-agnostic specification produced during MDEMG R&D. These specifications are the foundational document's governance sections, not new work.

**Prototype empirical signal (O-PROTO opportunities):** The five prototypes (O-PROTO-1 through O-PROTO-5) produce empirical signal feeding the substrate commitments. O-PROTO-2 informs O-LT-3 (world-model). O-PROTO-5 informs O-LT-1 (trainable representations). The other prototypes inform implementation choices within the substrate commitments.

**Specification work (O-LT specifications):** O-LT-1 through O-LT-5 specifications, produced pre-fork as the foundational document's substrate sections. Each specification compares candidate approaches, picks one, and documents the contract.

**Operational lessons (O-AGN-1 publication):** The post-mortem from O-AGN-1 documents operational hazards the successor should avoid — silent gradient-flow bugs, vocabulary drift, documentation debt at sprint velocity. These become "lessons learned" appendices in the foundational document.

**MDEMG-specific carry-forward decisions:** Which MDEMG components carry forward (the §2.8 list) and which don't (DBSCAN-based emergence, Hebbian-with-decay learning rule, frozen-embedding retrieval). These decisions inform the foundational document's "what we are deliberately not carrying forward" section.

### §8.4 Relationship between this specification and the foundational document

This specification governs MDEMG's remaining R&D phase. Its scope is operational: what to ship in MDEMG, what to prototype in MDEMG, what to specify before fork.

The successor's foundational document governs everything after fork. Its scope is architectural: what the successor framework's substrate commits to, what its first implementation sprint targets.

The boundary is the fork-gate from §3. When fork-readiness is achieved, this specification's MDEMG-shipping scope (§5.1) is closed; the successor's foundational document takes over for all forward-looking work. The substrate-agnostic carry-forward work specified here continues to be relevant to the successor as inputs.

The two documents do not overlap. This specification does not specify the successor's substrate. The successor's foundational document does not direct MDEMG sprints. The handoff at fork-time is clean.

### §8.5 Recommended cadence

Begin drafting the foundational document at fork-readiness 3-of-5 green. Iterate as items go green. Finalize at 5-of-5 green. The foundational document's draft state during MDEMG R&D phase is itself an input to MDEMG sprint planning — when the foundational document raises questions that MDEMG R&D can answer empirically (a prototype that reveals an architectural assumption), planning agents can sprint accordingly.

The cadence matters because writing the foundational document under time pressure after fork is a known failure mode. The user's stated long-term goal is ambitious; the foundational document needs time to mature. Starting at 3-of-5 fork-readiness gives roughly 4-8 weeks of parallel drafting time.

---

## Appendix A — Risk and Opportunity Entry Template

Risk entries use the following uniform structure:

```
### R-{class}-{N}: {short statement}

- **What is threatened:** {goal/asset/intuition this risk endangers}
- **Severity:** {Critical | High | Medium | Low}
- **Confidence:** {Confident | Moderate | Speculative} — {why this calibration}
- **Observable indicators:** {what code, metrics, or operational evidence shows the risk is active}
- **Prerequisites for mitigation:** {what must be true before mitigation is possible}
- **Recommended action:** {what planning agents should sprint on}
- **Success criteria:** {how planning agents know mitigation worked}
- **Trigger conditions for re-evaluation:** {what signals require re-assessing this risk}
- **Cross-references:** {related risks, opportunities, fork-gate criteria}
```

Opportunity entries use the following uniform structure:

```
### O-{class}-{N}: {short statement}

- **What it advances:** {goal this opportunity moves forward}
- **Fork-relationship:** {pre-fork | fork-gating | post-fork | substrate-agnostic}
- **Value rating:** {High | Medium | Low}
- **Confidence:** {Confident | Moderate | Speculative} — {why this calibration}
- **Prerequisites:** {what must be true before this opportunity can be pursued}
- **Recommended action:** {what planning agents should sprint on}
- **Success criteria:** {how planning agents know the opportunity was captured}
- **Resource estimate:** {sprint-cycle ballpark}
- **Cross-references:** {related risks, opportunities, fork-gate criteria}
```

Class identifiers for risks: `FT` (FT-LORA gate), `INT` (architectural intuitions), `SUC` (successor tractability), `LT` (long-term goal independent of MDEMG).

Class identifiers for opportunities: `MDEMG` (ship in MDEMG), `PROTO` (prototype to inform successor), `LT` (long-term goal substrate work), `AGN` (substrate-agnostic).

---

## Appendix B — Cross-Reference Index

### Risks by class

| ID | Statement | Severity | Threatens |
| --- | --- | --- | --- |
| R-FT-1 | Phase 11 kl=0.10 retry fails per-task gate | High | FT-1 fork-gate |
| R-FT-2 | Phase 12 HITL DPO effort exceeds expected benefit | Medium | FT-2 fork-gate |
| R-FT-3 | Multi-adapter serving has unsurfaced quality issues | High | FT-3 fork-gate |
| R-FT-4 | Single-developer velocity bottleneck | Medium | All FT fork-gate |
| R-INT-1 | Frozen-representation ceiling mistaken for general property | Critical | Successor substrate commitments |
| R-INT-2 | Interceptor pattern treated as MDEMG idiom | High | §2.8 carry-forward |
| R-INT-3 | RSIC parameter-tuning conflated with arch self-improvement | Critical | Successor substrate (recursive SI) |
| R-INT-4 | Documentation debt erodes architectural-intuitions output | High | FG-1, FG-2 fork-gates |
| R-SUC-1 | Fork before infrastructure abstractions stabilize | Critical | Successor first-six-months tractability |
| R-SUC-2 | Fork after MDEMG-specific design choices calcify | High | Successor substrate-agnostic foundation |
| R-SUC-3 | Successor inherits MDEMG vocabulary uncritically | Critical | Successor substrate commitments |
| R-SUC-4 | No formal mechanism to extract architectural lessons | High | Architectural-intuitions output |
| R-LT-1 | Long-term goal pursued as MDEMG features | Critical | Successor substrate-level commitments |
| R-LT-2 | Eight architectural extensions treated as exhaustive | High | Successor substrate completeness |
| R-LT-3 | Object-centric representation gap deferred indefinitely | High | Manifesto's identified hard gap |
| R-LT-4 | Continuous learning approached additively | Critical | Trainable representations commitment |

### Opportunities by class

| ID | Statement | Fork-relationship | Value |
| --- | --- | --- | --- |
| O-MDEMG-1 | Complete FT-LORA workstream end-to-end | fork-gating | High |
| O-MDEMG-2 | gpt-5.4-mini benchmark for cost-quality curve | pre-fork | High |
| O-MDEMG-3 | Migrate T-group call sites to RL-merged | pre-fork | Medium |
| O-MDEMG-4 | DBSCAN scale ceiling (only if pre-fork) | pre-fork | Medium |
| O-MDEMG-5 | Wire authorization enforcement (GAP-16) | pre-fork | Medium |
| O-PROTO-1 | Precision-weighted Hebbian experiment | pre-fork (prototype) | Medium |
| O-PROTO-2 | Top-down prediction edges experiment | pre-fork (prototype) | High |
| O-PROTO-3 | Column-voting retrieval RRF experiment | pre-fork (prototype) | Medium |
| O-PROTO-4 | Forward-Forward shallow heads experiment | pre-fork (prototype) | Medium |
| O-PROTO-5 | Representation-learning experiment (negative result) | pre-fork (prototype) | High |
| O-LT-1 | Trainable representations substrate (post-fork) | post-fork (spec pre-fork) | High |
| O-LT-2 | Reference-frame mechanism (post-fork) | post-fork (spec pre-fork) | High |
| O-LT-3 | World-model component (post-fork) | post-fork (spec pre-fork) | High |
| O-LT-4 | Recursive self-improvement mechanism (post-fork) | post-fork (spec pre-fork) | High |
| O-LT-5 | Object-centric slot structure (post-fork) | post-fork (spec pre-fork) | High |
| O-AGN-1 | Publish FT-LORA operational lessons | substrate-agnostic | Medium-High |
| O-AGN-2 | Externalize J17 protocol as standalone spec | substrate-agnostic | Medium |
| O-AGN-3 | Build MCP marketplace presence | substrate-agnostic | Medium |
| O-AGN-4 | Document multi-temporal RSIC pattern | substrate-agnostic | Medium |

### Fork-gate indicators

| ID | What it indicates | Section |
| --- | --- | --- |
| FT-1 | Phase 11 GRPO produces definitive signal | §3.1 |
| FT-2 | Phase 12 HITL DPO runs or consciously skipped | §3.1 |
| FT-3 | Multi-adapter serving operational at 16 call sites | §3.1 |
| FG-1 | Documentation freshness within 2 days of HEAD | §3.2 |
| FG-2 | Architectural intuitions formally documented | §3.2 |

---

## Appendix C — Reconciliation with Existing Project Documents

This specification supersedes some prior documents in scope and complements others. Explicit reconciliation:

**`risk-opp-04232026-01.md` (MDEMG-SPR-PLAN-001 v1.0):** This document supersedes the prior risk-opp register on the strategic frame. The prior register treated MDEMG as production target with 9 risks and 9 opportunities organized for incremental improvement. This document treats MDEMG as R&D vehicle and reorganizes by what's threatened/advanced relative to the long-term goal. Specific overlaps:

- Prior R1 (single-contributor velocity) → R-FT-4 here
- Prior R2 (model dependency post-pivot) → addressed by FT-LORA Packaging Spec; lives in FT fork-gate work
- Prior R3 (DBSCAN scale ceiling) → O-MDEMG-4 here (only sprint pre-fork if hitting wall operationally)
- Prior R4 (FT-OAI-003 north-star blocked) → retired per `mdemg-research-evaluation.md` §2.2 freshness check (FT-OAI direction dropped 2026-04-22)
- Prior O1 (Phase 10 unblocked) → COMPLETE per PR #348
- Prior O3 (UAITS closed-loop) → in flight via Phase 11/12; lives in FT-LORA work
- Prior O4 (J17 standalone) → O-AGN-2 here
- Prior O6 (DBSCAN→GAT/LSH) → O-MDEMG-4 here, with explicit "only if pre-fork" caveat
- Prior O7 (MCP marketplace) → O-AGN-3 here
- Prior O8 (self-reinforcing training data) → in flight via FT-LORA workstream

The prior register is not retired — its entries that haven't been addressed remain valid backlog items. This document layers strategic frame on top of the prior register's operational items.

**`MDEMG_FT_LORA_PACKAGING_SPEC.md` (MDEMG-FT-PKG-001 v1.0):** This document defers to the packaging spec for FT-3 implementation details. The packaging spec specifies how multi-adapter serving works at the implementation level; this document specifies that multi-adapter serving must be operational at the 16 call sites for FT-3 to close. No conflict; complementary.

**`mdemg-research-evaluation.md`:** This document treats the eight architectural extension notes (02–09) as inputs to O-PROTO opportunities and O-LT specifications, not as direct backlog items for MDEMG. The research evaluation's per-note analysis is preserved as the rubric for which prototypes to scope first; this document adds the "prototype to inform successor" framing.

**`novel-ANN-_topo-needed.md` (manifesto):** This document treats the manifesto's five inductive biases as substrate-level commitments for the successor framework. The manifesto's identified hard gap (object-centric slot structure) is R-LT-3 and O-LT-5 here. The manifesto is not superseded; it remains the strategic source for what the long-term goal requires architecturally.

**`AGENT_HANDOFF.md`:** This document is consistent with AGENT_HANDOFF through ~PR #356 (the last clearly-documented state). PRs #357 and #358 are not yet in AGENT_HANDOFF; this document treats the AGENT_HANDOFF lag as R-INT-4 (documentation debt risk).

**`VISION.md`:** This document does not supersede VISION.md. VISION.md describes MDEMG as it exists; this document describes how to steward MDEMG through its remaining R&D phase. The two are aligned but operate at different levels — VISION.md is application-level, this document is operational/strategic.

**`mdemg-gap-analysis.md`:** This document treats the gap analysis as authoritative on specific gap items where it conflicts with AGENT_HANDOFF (per the user's project memory note). GAP-01, GAP-03, GAP-10, GAP-11, GAP-16 — the gap analysis supersedes AGENT_HANDOFF on these. This document reflects the gap analysis state.

---

## Appendix D — Glossary

Project-specific terms used in this document, alphabetized:

- **AGENT_HANDOFF.md** — Top-level project document tracking MDEMG state for new agent sessions. Currently 1-2 PRs stale at HEAD per §1.11.
- **BCM** — Bienenstock-Cooper-Munro learning rule. Hebbian variant with sliding threshold. Not implemented in MDEMG; mentioned in §1.2 as a contrast to MDEMG's classical Hebbian-with-decay.
- **C-group** — Calibration / classification ULTS task group in the FT-LORA benchmark. Deterministic temperature=0 evaluation. Six tasks. The three persistent regressors in PR #357/#358 are C-group.
- **CHANGELOG.md** — Project changelog. Currently 1-2 PRs stale at HEAD per §1.11.
- **DBSCAN** — Density-based spatial clustering algorithm used in `internal/hidden/clustering.go` for concept emergence. O(n²) distance-matrix complexity is the scale ceiling per §1.4.
- **DPO** — Direct Preference Optimization. Preference-tuning method bypassing reward model. Phase 12 HITL DPO uses preference pairs from Phase 11 GRPO output.
- **FG-1, FG-2** — Fork-gate criteria beyond FT-LORA: documentation freshness, architectural intuitions documented. §3.2.
- **FT-1, FT-2, FT-3** — FT-LORA fork-gate sub-conditions: Phase 11 definitive signal, Phase 12 decision, multi-adapter serving operational. §3.1.
- **GRPO** — Group Relative Policy Optimization. RL fine-tuning method used in Phase 11. Variant of PPO that estimates advantages by comparing rollouts within a group rather than against a learned value function.
- **HITL** — Human-in-the-Loop. Phase 12 HITL DPO uses human-judged preference pairs.
- **J-group** — Judge-evaluated ULTS task group in the FT-LORA benchmark. Three tasks. Uses gpt-5.4-mini as judge.
- **J17** — AI-to-AI Communication Protocol. Three-tier encoding (T1 coded, T2 telegraphic, T3 full natural-language) with HMAC-signed session tickets. §1.5.
- **Jiminy** — MDEMG's inner-voice service. Five-component interceptor (Guide → Validate → Evaluate → Track → Learn). §1.5.
- **kl_coef** — KL coefficient in GRPO loss. Controls how strongly the policy is pulled toward the SFT reference. Phase 11 retry tightens from 0.05 to 0.10.
- **LoRA** — Low-Rank Adaptation. PEFT method used for FT-LORA training of Qwen3-14B-4bit base.
- **MoE** — Mixture of Experts. Original FT-LORA target was Qwen3.6-35B-A3B MoE; pivoted to Qwen3-14B dense due to Metal MTLResource cap (499K) on M5 Max.
- **MTLResource** — Apple Metal resource descriptor. M5 Max has a 499K cap that blocked MoE LoRA backward passes per §1.8.
- **Note 02 through Note 09** — The eight architectural extension notes in `mdemg-research-evaluation.md`. Treated as inputs to O-PROTO opportunities here.
- **NLI** — Natural Language Inference. Used in `internal/jiminy/nli_*` for guidance comprehension scoring and in the neural sidecar for tier prediction.
- **Oja's rule** — Hebbian variant with normalization term. Not implemented in MDEMG; mentioned in §1.2 as contrast.
- **PEFT** — Parameter-Efficient Fine-Tuning. The FT-LORA workstream uses LoRA, the dominant PEFT method.
- **Phase 5** — First MDEMG SFT fine-tuning, completed PR #347. Output: `qwen3-14b-mdemg-v1`. Aggregate baseline 0.8338 on Phase 10 benchmark.
- **Phase 10** — Automated benchmark framework, completed PR #348. 16 ULTS specs × 5 runs.
- **Phase 11** — Automated RL post-training, GRPO. Code complete PR #349; first REAL training PR #357; kl=0.10 retry PR #358.
- **Phase 12** — HITL DPO. Unblocked by Phase 11; pair generator from PR #349.
- **PR #357** — First REAL Phase 11 LoRA training, commit `ab32f6f`, merged 2026-04-27 03:51 UTC. Aggregate target met +1.76pp; per-task gate failed on 3 C-group regressors.
- **PR #358** — kl=0.10 retry config + benchmark documentation, merged 2026-04-27 04:53 UTC. Retry in flight at merge time. ETA 2026-04-27 ~17:00 UTC.
- **PRIME** — Process Reinforcement through Implicit Rewards. Recent RL fine-tuning method, mentioned in glossary context.
- **Qwen3-14B-Dense-4bit** — Base model for FT-LORA workstream. `mlx-community/Qwen3-14B-4bit`. Selected after MoE→Dense pivot.
- **RAFT** — Retrieval-Augmented Fine-Tuning. One of UAITS's four paradigms.
- **RSIC** — Recursive Self-Improvement Cycle. `internal/ape/`. Five-stage cycle at three temporal scales. §1.6.
- **SFT** — Supervised Fine-Tuning. Phase 5 was MDEMG's first SFT pass.
- **T-group** — Temperature / generation ULTS task group in the FT-LORA benchmark. Seven tasks. Five are migration-ready to RL-merged adapter today per PR #358.
- **UAITS** — Universal AI Training Specification. UxTS framework for training data curation. §1.9.
- **ULTS** — Universal LLM Task Specification. UxTS framework for LLM task contracts. 16 specs in the FT-LORA benchmark. §1.9.
- **UxTS** — UxTS framework family. Schema + spec + runner + CI uniform pattern. §1.9.

---

*End of document.*
