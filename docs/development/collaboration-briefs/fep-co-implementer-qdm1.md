---
type: collaboration-brief
status: draft
created: 2026-05-06
author: reh3376 (drafted by Claude as Workstream C Action 6)
addresses: Q-DM-1 (FEP co-implementer), Note 09 (Active Inference Unification capstone)
companion: docs/research/mdemg_sprint_ideas/09-active-inference-unification.md
---

# Collaboration Brief — FEP Co-Implementer Recruitment (Q-DM-1)

## One-paragraph framing for outreach

MDEMG ships three currently-separate decision systems — **Jiminy** (constraint guidance), **RSIC** (recursive self-improvement cycle), and the **Consulting Service** (constraint classification + suggestion). Each has its own tunables, heuristics, and exploration-exploitation tradeoffs. Note 09 of our research roadmap proposes unifying all three under a single **variational free energy** objective per Buckley et al. 2017, with epistemic value (information gain) and pragmatic value (outcome quality) emerging as the two natural components. The catch: this is the **single most leveraged action** in our 8-note research collection per internal evaluation, but **without an FEP-specialist co-implementer the Note stays in planning-document state**. Until recruitment succeeds or fails, Note 09 cannot start. We are recruiting an FEP/active-inference specialist (academic or industry) to co-author the FEP formalism translation step and pair-implement the shadow-mode observer.

## What MDEMG is — short version

5-layer hierarchical-memory cognitive substrate for AI-assisted development. Persistent emergent long-term memory via Hebbian learning over `CO_ACTIVATED_WITH` edges. 78k+ MemoryNodes in production, sparse context fingerprints (256-bit, default-on production as of 2026-05-06), column-voting retrieval (5 RRF columns + per-category weight overrides). Backed by Neo4j + TimescaleDB + a local llama.cpp LLM (Qwen3-14B-derived `mdemg-llm-v1`).

The three decision systems we'd like to unify under FEP:

| System | What it decides | Current decision logic |
|---|---|---|
| **Jiminy** | Whether to surface a constraint to the user mid-conversation; when a constraint should escalate from advisory → blocking | NLI-based + LLM-classification fallback + per-task confidence floors |
| **RSIC** | Macro/meso/micro improvement cycle: which actions to schedule (graduate, tombstone, prune, refresh, consolidate); when to alert | 5-stage Assess→Reflect→Plan→Execute→Validate, weighted-confidence health composite (DH-005), action calibration |
| **Consulting Service** | Constraint classification, suggestion generation, conflict detection between LLM and rule-based reflectors | Rule-based + circuit-breaker LLM fallback |

All three are stable and instrumented. We have ≥3 months of telemetry per system. **What's missing**: a single objective they all optimize against. The hand-tuned priors and thresholds in each one are essentially un-coordinated. Our hypothesis (per Note 09): FEP gives a principled way to coordinate — every behavior emerges from minimizing free energy, with the per-system parameters falling out of the FEP variables (precision, prior, action selection).

## What we need — Q-DM-1 specifically

The research evaluation called Q-DM-1 the *"single most leveraged action across the entire collection"*. The reason: the FEP formalism translation (Phase 1 of Note 09 sprint plan) is the gate the whole sprint hangs on. Get the formalism right and the rest is mostly engineering. Get it wrong and the unification is slow heuristic chaos with extra notation.

**Concrete asks, ordered by leverage:**

### 1. Co-author Phase 1 of Note 09 — formalism translation (highest leverage)
Translate MDEMG's three decision systems into FEP's native language. Specifically: define the **state space, action space, observation space, generative model, and prior preferences** that capture what Jiminy/RSIC/Consulting currently do. Include precision (the activation_confidence from Sprint 02 is a candidate), the generative model (the L_n hierarchy is a candidate), and the action distribution (the existing action calibrator is a candidate prior).

This is research writing, but with the right collaborator it's 1-2 weeks of work. Without one it's ill-defined indefinitely.

**Deliverable**: 15-30 pages defining the FEP variables in MDEMG terms, with worked examples for each of the 3 systems.

### 2. Pair-implement Phase 3 — shadow-mode observer (medium leverage)
Once Phase 1 is settled, the implementation work is bounded: build an FEP observer process that runs alongside live Jiminy/RSIC/Consulting, logs its recommendations daily, computes agreement rates and disagreement-resolution outcomes. Live systems continue with current logic. **Minimum 3 months of shadow observation before any migration to authoritative FEP.**

This is mostly Python + Go integration. We have the runtime infrastructure (TSDB telemetry, the conflict tracker shipped Phase 12 Epic 6 already). The collaborator's job is the FEP computation layer — the rest is plumbing.

**Deliverable**: working FEP observer + a daily `mdemg-fep-research.json` artifact comparing FEP recommendations to live decisions across the 3 systems.

### 3. Review the migration plan (lower leverage but cheap)
Phase 4 of Note 09 is the gradual migration: migrate one system at a time to authoritative FEP, never simultaneously. RSIC migrates last because it modifies the graph. Each migration is independently reversible; legacy logic is retained for ≥6 months after FEP becomes authoritative. We need a sanity check from someone who's seen FEP go from research → production before — does our migration cadence match what works?

**Deliverable**: 2-page review of `09-active-inference-unification.md` Phase 4 with concrete suggestions.

## Why MDEMG is interesting from an FEP-research perspective

Three traits that make MDEMG a useful FEP testbed:

1. **Three decision systems already separated and instrumented.** Most FEP papers pick one decision domain. Here you can compare across three (and their interaction effects).
2. **3+ months of historical telemetry.** Conflicting-guidance tracker already running (Action 1 of Workstream C, shipped Phase 12 Epic 6). Real outcome data exists. Hand-coded priors are reverse-engineerable.
3. **Production stakes are bounded** — MDEMG is a personal/team development assistant, not a safety-critical system. FEP can be tried and rolled back without consequence.

What we offer in return:

- **Joint authorship** on the foundational document AND any academic write-up.
- **Real production data** under operator-controlled access (privacy-scrubbed).
- **Compute** — local Mac M5 Max with MLX + llama.cpp + Neo4j + TimescaleDB. OpenAI API for grader / embedding work.
- **Operator engagement** — daily availability, fast iteration, explicit "do not let me block you" charter.
- **Honoraria** if budget allows; for academic collaborators, citation as co-implementer.

## What we explicitly are NOT asking for

- Active-inference theoretical innovation. The Buckley et al. 2017 review is the basis we're working from.
- Replacement of the existing decision systems before shadow data justifies it.
- A pure-research detour. We want a working observer that produces data we can act on.

## Why now

The roadmap puts Note 09 at "9-12 months minimum, capstone effort." Action 1 (conflict tracker) has been running ~6 months by the time Note 09 could start, giving the FEP observer concrete disagreement data to compare against. **Recruiting now (months ahead of sprint kickoff) means Phase 1 formalism work can happen in parallel with the empirical data accumulating.** Recruiting at sprint kickoff means we waste 1-2 months on formalism while the rest of the team waits.

R-LT (deferral risk) is real here too: without a co-implementer, Note 09 stays a planning document. The opportunity cost of *not* unifying — the three systems continue diverging in their tuned heuristics — is paid every quarter.

## Logistics

- **Outreach targets**: active-inference research groups (Karl Friston's lineage; Buckley group at Sussex; Magnus Koudahl's group), graduate students looking for production-data thesis projects, industry researchers with FEP background (DeepMind's active-inference work; Anthropic's process-reward modeling group).
- **Format**: this brief + a 30-min screencast of the three decision systems in action, then a triage call.
- **Compensation / authorship**: co-authorship on Note 09 → foundational doc → any subsequent paper; honorarium negotiable; open to part-time advisor arrangements.
- **Timeline**: outreach starts immediately. Note 09 sprint launch ≥6 months out. Ideal: collaborator signs on within 3 months; Phase 1 formalism complete ≥1 month before sprint kickoff.

## Risk if recruitment fails

- Note 09 stays planning-document indefinitely.
- The 3 decision systems continue with un-coordinated tuning.
- The capstone of the PC/FEP thread (Notes 01 → 09) never lands as a unified system.

This is an acceptable steady-state — the systems work fine independently — but it's not the substrate-level coordination we believe is achievable.

## How to respond

- Email: [redacted in operator-edited copy]
- GitHub Discussion at `WhiskeyHouse/mdemg`
- Calendly link (operator inserts)
- We will follow up within 1 business day.

## References

- `docs/research/mdemg_sprint_ideas/09-active-inference-unification.md` — full Note 09 sprint plan
- `docs/research/mdemg_sprint_ideas/01-pc-reframe-and-surprise-routing.md` — Note 01 (predicate of Note 09)
- `docs/research/mdemg_sprint_ideas/02-precision-weighted-hebbian-eta.md` — Sprint 02 (provides the precision term)
- `docs/research/mdemg_sprint_ideas/03-top-down-predictions-and-promotion.md` — Sprint 03 (provides predictions / errors)
- `docs/development/SPRINT_ROADMAP_POST_FT_LORA.md` §Note 09 — strategic context
- `docs/research/mdemg_sprint_ideas/mdemg-research-evaluation.md` — internal source naming Q-DM-1 as the highest-leverage action (operator-private)
- Buckley, Kim, McGregor, Seth (2017) — "The free energy principle for action and perception: A mathematical review"
