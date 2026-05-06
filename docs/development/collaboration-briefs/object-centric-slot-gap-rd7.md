---
type: collaboration-brief
status: draft
created: 2026-05-06
author: reh3376 (drafted by Claude as Workstream C Action 5)
addresses: RD-7 (Object-Centric Slot Gap), R-LT-3, O-LT-5
companion: docs/research/mdemg_sprint_ideas/mdemg-specification.md §R-LT-3
---

# Collaboration Brief — Object-Centric Slot Gap (RD-7)

## One-paragraph framing for outreach

MDEMG is a 5-layer hierarchical-memory cognitive substrate for AI-assisted development that ships persistent emergent long-term memory via Hebbian learning, sparse fingerprints, and column-voting retrieval. We have entity-shaped MemoryNodes — one node per symbol/file/concept — but **no object-centric slot structure**: no slot-attention, no DETR-style query slots, no equivalent persistent-identity binding that survives context changes. Per our internal manifesto and research evaluation, this is the **single most actionable gap** for collaborative input — Numenta and Meta both have active lines of work in this space (slot attention, DETR query slots, neural object representation, Numenta's reference frames). We are recruiting collaborators / advisors who can specify what minimum-viable slot structure looks like at substrate level (persistent entity identity across context changes, at minimum) and target form (full compositional binding) before we fork into a successor framework.

## What MDEMG already has (state of the art *today*)

- **MemoryNodes per symbol** — one node per code symbol / file / concept; not pooled. Persistent IDs via CUIDv2.
- **Sparse context fingerprints** — 256-bit per-observation vectors discriminate the same MemoryNode in different contexts (default-on production as of 2026-05-06, Phase 14.2.3). Catalog refs are path-segment tokens; column votes by Jaccard similarity.
- **5-layer concept hierarchy** — L0 base nodes → L1 hidden patterns → L2-L5 concepts via DBSCAN + KMeans. Each concept is a cluster centroid with named identity.
- **Hebbian learning over CO_ACTIVATED_WITH edges** — strengthens connections between co-activating nodes; Phase 12 conflicting-guidance tracker observes when learned patterns disagree across components.
- **Column-voting retrieval** — RRF over 5 columns (Embedding, BM25, Graph, Structural, Context); per-category weight overrides; cache namespace `v1-rrf5`.

What this gives us behaviorally: stable identity for atomic symbols, semantic clustering, and context-specific retrieval. What it does **not** give us: explicit binding, per-slot attention over a structured representation, or the ability to refer to "the same entity in a different role" without leaning on heuristic name-matching + spatial co-location.

## The gap (verbatim from `mdemg-specification.md` §R-LT-3)

> No object-centric slot structure. No slot-attention, no DETR-style query slots, no equivalent. Concepts are clusters with names; they are not bound entities with persistent identity that survive context changes.

**Manifesto identifies this as one of five inductive biases for the substrate**, and the only one currently absent from MDEMG:

1. ✅ Sparse distributed representations — Phase 14.2.x
2. ✅ Structure-content factorization — partial via 5-layer hierarchy
3. ❌ **Object-centric slots — gap (this brief)**
4. ◐ Action-conditioned prediction — partial via RSIC's plan→execute loop
5. ✅ Layer-local objectives — Hebbian + Note 07 FF heads

## What we need from collaborators

Three concrete asks, ordered by leverage:

### 1. Specify "minimum-viable slot structure" at substrate level (highest leverage)
Before MDEMG forks into a successor framework, we need a foundational document that names the chosen approach (slot attention vs DETR-style query slots vs neural object representation vs reference frames vs hybrid). At minimum the substrate provides **persistent entity identity that survives context changes**; at maximum **full compositional binding**. We are open to either end of that spectrum but cannot pre-fork specify without input from people who've built one of these systems.

**Concrete deliverable from this collaboration**: 5-15 pages including (a) the pick + rationale relative to MDEMG's existing entity-shaped MemoryNode + path-segment fingerprint substrate, (b) a substrate-level commitment we can write into the foundational document, (c) reservation of "elaborate implementation" to post-fork sprints.

### 2. Implementation specification (medium leverage)
Once minimum-viable form is settled, the candidate approaches map to wildly different implementation paths:

| Approach | What changes for MDEMG | Compute footprint |
|---|---|---|
| Slot attention | ~Add a slot-attention module over MemoryNode embeddings; new SlotNode label + binding edge | Modest, runs alongside retrieval |
| DETR-style query slots | Query is a slot, retrieval populates slots iteratively | Adds N×M attention per query (N=slots, M=candidates) |
| Numenta reference frames | Closer to the existing graph; reference frames as L_n abstraction | Compatible with current substrate; biggest spec lift |
| Neural object representation | Per-entity learned representation that updates over context | Substantial; needs persistent representation store |
| Hybrid | Identity at substrate level, attention at retrieval | Most flexible; hardest to spec |

**Deliverable**: a 1-2 page comparison sheet recommending one of these or a hybrid for MDEMG's specific data shape (entity-keyed graph + sparse fingerprints + RRF retrieval).

### 3. Pre-implementation gut-check on whether MDEMG should prototype pre-fork (lowest leverage but cheapest)
Open question Q-S-1 from the specification: should MDEMG prototype object-centric structure (perhaps via a new node type) pre-fork to surface what the implementation looks like, or is the substrate dependency too strong for prototype scope? An hour of conversation with someone who's built one of these would resolve this question quickly.

**Deliverable**: 30-60 min call + 1-page summary.

## What we offer in return

- **Real production data** — 78k MemoryNodes across multiple spaces with real co-activation patterns, retrieval audit trails, and 9-domain test corpus (UVTS framework, Phase 12).
- **Empirical validation harness** — UVTS A/B comparison framework (`uvts_ab_compare.py`) gives quantitative read on whether a slot-structure prototype improves retrieval. Production-grade Phase 14 sparse-gate + Phase 14.2 fingerprint-column infrastructure provides a high baseline to compare against.
- **Joint authorship** of foundational-document section. We're committed to citing collaborators in the spec and the academic write-up of the successor framework.
- **Compute** — local Mac M5 Max with MLX + llama.cpp pipeline already operational; OpenAI API access for embeddings; TimescaleDB for telemetry.

## Why now

R-LT-3 (the deferral risk) activates if MDEMG forks before substrate-level slot structure is specified. We are within 6-12 months of fork-point per the roadmap. The cost of specifying pre-fork is small. The cost of deferring indefinitely is high — a successor that ships without object-centric structure inherits the same gap MDEMG has, and there's no second chance to introduce it as a substrate-level commitment without breaking everything built on top.

## Logistics

- **Outreach targets** (from the specification's research-evaluation): Numenta (reference frames + HTM lineage matches MDEMG's design assumptions), Meta AI (slot attention + DETR), independent researchers in object-centric learning.
- **Format**: async first (this brief + a public Loom walkthrough of MDEMG's current substrate, when ready), 30-60 min call to triage, then targeted ask matching their availability.
- **Compensation / authorship**: we offer co-authorship on the foundational-document section; honoraria available if budget allows; for academic collaborators, citation in any subsequent paper.
- **Timeline**: outreach starts immediately. No hard deadline — fork is ≥6 months out. Ideal: 1-2 collaborators committed by ≥3 months pre-fork.

## Open issues / what we explicitly *don't* know

- Whether the path-segment fingerprint substrate (Phase 14.2.x) interferes with or accelerates an object-centric structure (it's our most compositional substrate piece to date).
- Whether RSIC's existing reflect-plan-execute loop is enough action-conditioning to validate object-centric predictions, or whether we need separate slot-conditioned predictors.
- Whether the existing 5-layer hierarchy maps cleanly onto reference-frame-style "frames at multiple scales" or whether the hierarchy itself is a layer-confusion that needs untangling alongside slot specification.

These are exactly the kinds of questions where collaboration leverage is highest.

## How to respond

- Email: [redacted in operator-edited copy]
- Conversation: GitHub Discussion at `WhiskeyHouse/mdemg` (repo public)
- Or: schedule via Calendly link (operator inserts)
- We will follow up within 1 business day.

## References

- `docs/research/mdemg_sprint_ideas/mdemg-specification.md` §R-LT-3, §O-LT-5, §Q-S-1 — internal source for this brief
- `docs/development/SPRINT_ROADMAP_POST_FT_LORA.md` §Workstream C — strategic context
- `docs/features/context-fingerprinting.md` — Phase 14.2.x substrate state, the closest existing piece to "object-centric"
- Numenta — Hawkins reference frames; HTM cortical column model
- Meta AI — slot attention (Locatello et al. 2020); DETR (Carion et al. 2020)
- Manifesto — `novel-ANN-_topo-needed.md` (operator-private)
