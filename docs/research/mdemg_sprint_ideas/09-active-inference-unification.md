# 09 — Active-Inference Unification

**Sprint ID**: FEP-UNIFY
**Date**: 2026-04-21 (plan authored — RESEARCH-STAGE / CAPSTONE; intentionally sketched)
**Branch**: TBD
**Scope**: Unify three currently-separate decision systems — Jiminy Guide, RSIC, and the Consulting Service — under a single **variational free energy** objective. Each currently has its own tunable parameters, heuristics, and exploration-exploitation tradeoffs. The FEP framework provides a principled unification: all three behaviors fall out of minimizing free energy, with epistemic value (information gain) and pragmatic value (outcome quality) emerging as the two natural components.

**Upstream**: [White Paper Review](mdemg-white-paper-review.md) Paper 7 (Buckley et al. 2017 — mathematical FEP review); all prior sprints in this series.

---

## Sprint Framing — Why This Sprint Is Last

This is **the capstone** of the PC/FEP thread, and it should not be started until all of the following are true:

1. Sprint 01 (PC reframe) has shipped and the vocabulary is established.
2. Sprint 02 (precision-weighted η) is live and stable — its activation_confidence is the precision term FEP needs.
3. Sprint 03 (top-down predictions) has shipped in at least shadow mode — predictions and prediction errors are the raw material FEP operates on.
4. RSIC has ≥3 months of stable outcome data — the pragmatic-value component needs calibrated outcome signals.
5. There is a concrete reason to unify — e.g., parameter tuning has become unwieldy because the three systems have independent knobs, or there's a demonstrable case where they give conflicting guidance.

This is a **very high-risk** architectural sprint. It touches the three most user-visible parts of MDEMG (guide, improvement cycle, consulting). The plan here is deliberately sketched — full specification should happen after the prerequisite sprints ship and real operational data informs it.

**Alternative framing**: treat this sprint as a *research prototype* rather than a production deliverable. Build the FEP implementation in a separate process, have it observe live MDEMG decisions, and compare its recommendations to the live systems' decisions for ≥3 months. Only unify if the FEP prototype is demonstrably better on the metrics that matter.

---

## Gap Summary

| Category | Effort |
|----------|--------|
| FEP formalism definition (what exactly are the state, action, and observation spaces?) | MEDIUM (research) |
| Generative model specification (what does MDEMG believe about its world?) | HIGH (research) |
| Free-energy functional implementation | MEDIUM (engineering) |
| Jiminy refactor to FEP action selection | HIGH (risk) |
| RSIC refactor to FEP update rule | HIGH (risk) |
| Consulting refactor to FEP query response | MEDIUM (risk) |
| Observability & A/B infrastructure | MEDIUM |
| Testing & Verification | HIGH |
| Mandatory Documentation Phase | MEDIUM |

---

## Phase 1: Formalism — Define the Three Spaces

**Goal**: Translate MDEMG's concerns into FEP's native language. This is mostly research writing, but it's the gate — the rest of the sprint hangs on getting this right.

### 1.1 State space (s)

**Question to answer**: what is the hidden state MDEMG is trying to infer?

**Current thinking**: a vector representing (a) which concepts are salient for the current developer task, (b) which are poorly-modeled (high prediction error), (c) what the developer's goal is (inferred from recent events), and (d) the graph's current confidence profile. The state is not directly observed; it's inferred from observations.

**Deliverable**: `docs/research/fep-unify-formalism.md` — full state-space definition with dimensionality and inference method.

### 1.2 Action space (a)

**Question**: what actions can the agent (the collective of Jiminy + RSIC + Consulting) take?

**Current thinking**: actions are triples `(surface_to_user, what_to_surface, when_to_surface)`. This unifies "Jiminy suggests X" and "Consulting surfaces pattern Y" and "RSIC flags problem Z" as three kinds of actions in the same space.

**Deliverable**: same doc, action-space section.

### 1.3 Observation space (o)

**Question**: what outcomes does the agent observe and use to update beliefs?

**Current thinking**: observations are `(action_taken, time_until_next_event, event_type, developer_feedback)`. RSIC already collects most of this.

**Deliverable**: same doc, observation-space section.

### 1.4 Generative model p(o, s, a)

**Question**: what does MDEMG believe about how states produce observations?

**Current thinking**: a factored model — transitions `p(s_{t+1} | s_t, a_t)` from the graph's top-down predictions (Sprint 03), emissions `p(o_t | s_t)` from the retrieval model, priors `p(s_0)` from DH-005 health state.

**Deliverable**: same doc, with an architecture diagram showing where each factor comes from.

---

## Phase 2: Free-Energy Functional Implementation

### 2.1 Variational posterior q(s | o)

**Fix** — Implement a tractable approximation to the true posterior. Start with mean-field Gaussian (simplest case, validates the infrastructure). Upgrade to structured approximation only if needed.

**Files**: `internal/fep/posterior.go` (new)

### 2.2 Free-energy computation

**Fix** — `F = E_q[log q(s|o) - log p(o, s, a)]` decomposed into:
- Accuracy term: -E_q[log p(o | s, a)]
- Complexity term: KL[q(s|o) || p(s)]

Both computable from existing MDEMG quantities once the generative model is wired.

**Files**: `internal/fep/free_energy.go`

### 2.3 Expected free energy for action selection

**Fix** — For each candidate action a, compute:
- Pragmatic value: E[p(preferred_outcomes | a)]
- Epistemic value: E[information gain | a] = H[q(s|o)] - E_q[H[q(s|o,a)]]

Action selection: argmax (pragmatic + epistemic). This is the **core FEP formula**.

**Files**: `internal/fep/action_selection.go`

---

## Phase 3: Shadow-Mode Deployment

**Goal**: FEP observer runs alongside live systems, logs its recommendations. Live systems continue to use current logic.

### 3.1 FEP observer process

**Fix** — A sidecar Go routine that subscribes to the same event stream Jiminy and RSIC see, computes its own recommendations, logs them to TSDB for later comparison.

**Files**: `internal/fep/observer.go`

### 3.2 Comparison metrics

**Fix** — Daily job compares FEP's recommendations against live Jiminy/RSIC/Consulting decisions. Track:
- Agreement rate
- Disagreement resolution (manual review: was FEP right?)
- Metric-by-metric: developer effectiveness, retrieval quality, etc.

### 3.3 Duration

**Minimum**: 3 months of shadow observation before any consideration of migration to authoritative FEP.

---

## Phase 4: Gradual Live Migration

**Goal**: If shadow-mode results justify it, migrate live systems to FEP *one at a time*. Never migrate all three simultaneously.

### 4.1 Consulting first (lowest risk)

Consulting recommendations are soft — a bad recommendation doesn't change system state. Good first candidate.

### 4.2 Jiminy second

Jiminy's guidance is user-facing; bad guidance harms UX but doesn't corrupt the graph. Medium risk.

### 4.3 RSIC last (highest risk)

RSIC actions modify the graph. FEP replacement must have demonstrated superior judgment for months before we let it drive RSIC.

### 4.4 Rollback per-system

Each system's FEP migration is reversible independently. The legacy logic is retained (not deleted) until the FEP system has been authoritative for ≥6 months.

---

## Phase 5: Observability

### 5.1 Prometheus metrics

```
mdemg_fep_free_energy{space_id} - histogram
mdemg_fep_pragmatic_value{space_id, action_kind}
mdemg_fep_epistemic_value{space_id, action_kind}
mdemg_fep_agreement_with_legacy{space_id, system} - gauge
```

### 5.2 Research dashboard

`mdemg-fep-research.json` with FEP trajectories, agreement rates, A/B outcome comparisons.

### 5.3 Explainability surface

Every FEP-driven action carries a decomposition (pragmatic vs epistemic contribution, which posterior samples dominated). Critical for operator trust.

---

## Phase 6: Testing & Verification

### 6.1 Mathematical correctness tests

FEP computation verified against hand-worked toy problems with known closed-form solutions.

### 6.2 Integration tests

FEP observer runs on a captured event trace; its decisions are deterministic given the trace.

### 6.3 A/B — very long horizon

At least one full release cycle of shadow observation before migrating any live system.

### 6.4 Reversibility test

Migrated system can be reverted to legacy logic within one deploy cycle with no data loss.

---

## Phase 7: Mandatory Documentation Phase

### 7.1 CHANGELOG.md
### 7.2 AGENT_HANDOFF.md
### 7.3 VISION.md — major revision; FEP becomes the unifying framework described at the top, not just in theoretical-foundation section
### 7.4 `docs/features/active-inference-unification.md` (new)
### 7.5 `docs/research/fep-unify-formalism.md` (the research doc from Phase 1)
### 7.6 CLAUDE.md — FEP as the agent's operating principle
### 7.7 Homebrew beta testing guide + submodule bump
### 7.8 Migration runbook for each of the three systems

---

## Risk Analysis & Rollback

### R1: FEP performs worse than the heuristic systems it replaces

**Likelihood**: Medium. The heuristics have been tuned for months; FEP is principled but inexperienced.

**Mitigation**: 3-month shadow-mode minimum. Per-system migration gates. Always-available rollback to legacy logic.

**Rollback**: Legacy logic retained indefinitely. Disable FEP with a single flag per system.

### R2: Mathematical infrastructure is genuinely difficult

**Likelihood**: High. Variational inference over a graph is non-trivial. Sparse-Gaussian posterior approximations have known failure modes.

**Mitigation**: Start with the simplest posterior (mean-field Gaussian). Only upgrade if shadow mode shows the simple version is insufficient. Consider hiring or consulting with a researcher experienced with FEP implementations.

**Rollback**: Abandon FEP implementation if math infrastructure is unreliable. Return to heuristics.

### R3: The generative model is wrong

**Likelihood**: High. Our beliefs about MDEMG's state are themselves hypotheses.

**Mitigation**: Phase 1 is explicitly research-stage. Phase 3 shadow-mode is the empirical test of whether the generative model matches reality.

**Rollback**: Refactor or abandon the model. Return to heuristics.

### R4: FEP becomes a one-person project

**Likelihood**: Medium-High. The math is specialized. If only one person understands it, the project is brittle.

**Mitigation**: Documentation. Consider recruiting a co-implementer before Phase 2 starts. If unable, deprioritize this sprint and accept that MDEMG stays at the heuristic level.

---

## Sprint Size Estimate — Very Rough

This sprint is **not** the usual 1-2 week slot. More realistic phasing:

| Phase | Estimate |
|-------|----------|
| 1. Formalism | 2 weeks (research writing) |
| 2. Implementation | 3 weeks |
| 3. Shadow Mode | 3 months calendar |
| 4. Gradual Migration | 6 months calendar |
| 5-7. Ongoing | throughout |
| **Total calendar** | **~9-12 months** |

This is **program scope**, not sprint scope. It should be managed as an ongoing initiative, not a single sprint.

---

## Alternative: Don't Do This

A legitimate option: decide that the PC/FEP thread ends at Sprint 03 (top-down predictions) and the independent tracks end at Sprints 06/07. The remaining value of FEP unification is primarily theoretical elegance; if the heuristic systems are working well after sprints 01-07 ship, unifying them may not be worth the risk.

**Decision criterion**: after Sprints 01-07 have soaked for 3 months, measure the frequency of "conflicting-guidance" incidents (Jiminy says one thing, RSIC flags another, Consulting suggests a third). If frequency is low (<1% of sessions), heuristics are composing fine and FEP unification is not worth the risk. If frequency is high (>5%), unification becomes the natural next step.

---

## Dependencies

**Blocks**: None.

**Blocked by**:
- Sprint 01 (vocabulary)
- Sprint 02 (precision / activation_confidence)
- Sprint 03 (top-down predictions — raw material for generative model)
- ≥3 months stable operation of Sprints 01-03
- A concrete operational motivation (not just theoretical appeal)

---

## Documents Accessed

- White paper review Paper 7 (Buckley et al., FEP mathematical review)
- `internal/jiminy/service.go`, `internal/ape/cycle.go`, `internal/consulting/service.go` (the three systems to be unified)

---

## Final Note

This plan is a **placeholder**. Its real purpose is to hold a spot in the sprint sequence so that (a) we remember FEP unification is the theoretical endpoint of this thread, and (b) we do not accidentally rebuild pieces of it in sprints 01-08 in inconsistent ways. When this sprint actually runs, its plan will be rewritten from scratch based on what has been learned in the preceding 12-18 months.
