# 01 — Predictive Coding Reframe & Surprise Routing

**Sprint ID**: PC-REFRAME-A (target: PC-001 on branch assignment)
**Date**: 2026-04-21
**Branch**: TBD (recommend `reh3376_dev01` or `reh3376_dev02`)
**Scope**: Formalize MDEMG's implicit predictive-coding / free-energy-principle structure and ship the first concrete extension (surprise × source-trust routing). Precedes Sprint B (precision-weighted Hebbian η + top-down predictions).

**Upstream**: [White Paper Review](mdemg-white-paper-review.md) — Papers 5, 6, 7, 9 (predictive coding, FEP, NGRAD).

---

## Sprint Framing

This is a **narrative + incremental-capability sprint**, not a ground-up rewrite. Three observations about the current codebase drive the scope:

1. **`internal/ape/health_formula.go` already implements precision-weighted evidence integration.** The formula `overall = Σ(w_i · c_i · s_i) / Σ(w_i · c_i)` is mathematically identical to a Bayesian precision-weighted posterior mean and, structurally, to predictive coding's hierarchical weighted-error integration. No math change needed — just naming and narrative.
2. **`internal/conversation/surprise.go` already implements a 4-factor variational surprise proxy** (term novelty, correction detection, NLI contradiction, embedding novelty) and persists `surprise_score` on every observation.
3. **`internal/learning/service.go` already applies surprise as a Hebbian initial-weight multiplier** (1.0× / 1.5× / 2.0× boost). Relevance scoring already weights surprise at 25%.

The formal PC framing has mostly arrived without anyone naming it. Sprint A does the naming, and adds the one piece of surprise machinery that is genuinely missing: **routing** (fast-track / normal / quarantine), as opposed to weight-boosting alone.

---

## Gap Summary

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Documentation & Formalization | 0 | 3 | 2 | 0 | **5** |
| Surprise Routing (new capability) | 1 | 3 | 1 | 0 | **5** |
| Observability | 0 | 1 | 2 | 0 | **3** |
| Testing & Verification | 0 | 2 | 1 | 0 | **3** |
| Mandatory Documentation Phase | 0 | 5 | 2 | 0 | **7** |
| **Total** | **1** | **14** | **8** | **0** | **23** |

---

## Phase 1: Documentation & Formalization

**Goal**: Make explicit what the code already does. Unlock the research-literature framing for future sprints and for contributor onboarding.

### 1.1 Add PC/FEP scaffold section to VISION.md (HIGH)

**Gap**: VISION.md describes MDEMG as "emergent long-term memory with Hebbian learning" — accurate but atheoretical. The math in `health_formula.go` and `surprise.go` corresponds to specific, named quantities from predictive coding and the free energy principle, which gives a much stronger scientific framing.

**Fix** — Add new section after "Architectural Philosophy" in `VISION.md`:

```markdown
## Theoretical Foundation

MDEMG's learning mechanisms map directly onto predictive coding (PC) and the
free energy principle (FEP), two converging frameworks from computational
neuroscience for how hierarchical systems learn from local signals without
backpropagation.

### Precision-Weighted Evidence Integration

The RSIC overall health formula (DH-005) combines per-dimension scores as
a precision-weighted average:

    overall = Σ(w_i · c_i · s_i) / Σ(w_i · c_i)

where `w_i` is the prior precision (dimension reliability × user impact) and
`c_i` is the observational precision (data-sufficiency confidence). The
product `π_i = w_i · c_i` is the **total precision** of dimension i. This is
mathematically identical to the posterior mean of a Gaussian likelihood with
independent sources (the foundational quantity in PC, Kalman filtering, and
Bayesian evidence accumulation).

### Variational Surprise as the Novelty Signal

The surprise detector (`internal/conversation/surprise.go`) computes a
4-factor proxy for variational surprise — the negative log probability of an
observation under current beliefs:

    S(obs) ≈ 0.40·correction + 0.25·term_novelty + 0.25·embedding_novelty + 0.10·contradiction

In FEP terms, S is an upper bound on -log p(obs | beliefs). Minimizing S over
time is equivalent to free-energy minimization.

### The Interceptor Loop as Active Inference

The detect → correct → iterate → learn loop implements active inference:
- **Detect** updates beliefs given new observations (perception).
- **Correct** takes actions expected to reduce predicted error (action).
- **Iterate** re-observes outcomes.
- **Learn** updates the generative model (learning).

This is not a metaphor — the three components are mathematically dual to the
three terms in the free energy functional under standard assumptions.

### Why this matters

Naming what the system already does unlocks four things: (1) a principled
derivation of update rules where we currently use heuristics; (2) a
research-literature hook for future sprints; (3) clearer reasoning about
where the architecture is weak (missing top-down predictions, no precision
weighting on η); (4) a defensible scientific narrative for the public repo.
```

**References**: Millidge, Seth, Buckley (2021) "Predictive Coding: a Theoretical and Experimental Review"; Buckley, Kim, McGregor, Seth (2017) "The free energy principle for action and perception: A mathematical review"; Lillicrap et al. (2020) "Backpropagation and the brain".

**Files**: `VISION.md`

---

### 1.2 Add PC/FEP terminology block to CLAUDE.md (HIGH)

**Gap**: CLAUDE.md is the primary agent-orientation doc. Planning agents and Claude Code sessions should use the same vocabulary as VISION.md so sprint plans feed back into the framework coherently.

**Fix** — Add new section to `CLAUDE.md`:

```markdown
## Theoretical Vocabulary (PC-REFRAME-A)

When reasoning about MDEMG's learning mechanisms, prefer these names:

| Code concept          | PC/FEP name                    | Where implemented |
|-----------------------|--------------------------------|-------------------|
| `w_i * c_i`           | total precision π_i            | health_formula.go |
| `c_i`                 | observational precision        | self_assess.go    |
| `w_i`                 | prior precision                | health_formula.go |
| `surprise_score`      | variational surprise (proxy)   | surprise.go       |
| `surpriseFactor`      | precision-weighted gain        | learning/service.go |
| Hebbian update        | local prediction-error descent | learning/service.go |
| Detect→Correct→Iterate→Learn | active inference cycle  | jiminy + RSIC     |
| L0 → L1 promotion     | hierarchical feature induction | hidden/service.go |

**Not yet implemented** (Sprint B):
- Top-down predictions (L_{n+1} → L_n prediction stream)
- Precision-weighted η (surprise currently boosts initial weight only)
- Hierarchical prediction errors driving promotion
```

**Files**: `CLAUDE.md`

---

### 1.3 Annotate `health_formula.go` with PC identification (MEDIUM)

**Gap**: The file's doc comment correctly describes the math but doesn't name it. Future readers (human or agent) should see the PC identification inline.

**Fix** — Extend the `ComputeOverallHealthWith` doc comment in `internal/ape/health_formula.go`:

```go
// ComputeOverallHealthWith is ComputeOverallHealth with operator-tunable
// weights. Used by the live wiring to thread RSIC_HEALTH_WEIGHT_<DIM> env
// overrides through without requiring a rebuild.
//
// Theoretical foundation (see VISION.md "Theoretical Foundation"):
// This formula is a precision-weighted Bayesian evidence integration —
// mathematically identical to the posterior mean of a Gaussian likelihood
// with independent sources, and structurally identical to predictive
// coding's hierarchical weighted-error minimization. The product
// π_i = w_i · c_i is the total precision of dimension i: base prior
// precision (w_i) times observational precision from data sufficiency (c_i).
// Dimensions with zero total precision are automatically excluded — this
// is the correct Bayesian behavior, not a special-case branch.
//
// References:
//   - Millidge, Seth, Buckley (2021) "Predictive Coding: a Theoretical
//     and Experimental Review", Neural Computation
//   - Buckley et al. (2017) "The free energy principle for action and
//     perception: A mathematical review"
func ComputeOverallHealthWith(r *SelfAssessmentReport, w HealthWeights) float64 {
```

**Files**: `internal/ape/health_formula.go`

---

### 1.4 Annotate `surprise.go` as variational surprise proxy (MEDIUM)

**Gap**: `DetectSurprise` returns a weighted combination of four heuristic factors. Its relationship to variational surprise (-log p under current beliefs) is not documented, making it look like an ad-hoc novelty score rather than a principled FEP quantity.

**Fix** — Extend the doc comment on `DetectSurprise`:

```go
// DetectSurprise computes overall surprise score (0.0-1.0).
//
// Theoretical foundation: this is a 4-factor proxy for variational
// surprise S(obs) ≈ -log p(obs | current_beliefs), the core quantity
// in the free energy principle. The factors trade exactness for
// tractability:
//
//   - CorrectionScore (0.4 weight): hardest-evidence signal — the user
//     explicitly told us our beliefs were wrong. Highest precision.
//   - TermNovelty (0.25): lexical evidence that the observation contains
//     concepts not in our vocabulary. Low precision but near-free to
//     compute.
//   - EmbeddingNovelty (0.25): semantic distance from known observations.
//     Medium precision; captures contradiction even when lexical overlap
//     is high.
//   - ContradictionScore (0.1): NLI-based detection of entailment
//     violation. High signal when NLI is operational; gated by
//     ContradictionNLIEnabled.
//
// The output is used (a) as an initial-weight multiplier in Hebbian
// updates (learning/service.go), (b) as a 25% term in conversation
// relevance ranking, and (c) as a routing signal in the ingestion gate
// (ingestion_gate.go; added in PC-REFRAME-A Phase 2).
func (d *SurpriseDetector) DetectSurprise(ctx context.Context, obs Observation) (float64, SurpriseFactors, error) {
```

**Files**: `internal/conversation/surprise.go`

---

### 1.5 New feature doc: `docs/features/predictive-coding-scaffold.md` (HIGH)

**Gap**: No standalone feature doc explains how MDEMG's PC/FEP structure works. This is the reference doc that future contributors and planning agents will search for.

**Fix** — Create new file using the standard YAML frontmatter template:

```markdown
---
title: Predictive Coding Scaffold
status: active
since: v0.8.6
owners: [mdemg-core]
related: [rsic-feedback-loop, health-formula, surprise-detection]
---

# Predictive Coding Scaffold

## What this is

A formal identification of MDEMG's learning mechanisms as an implementation
of predictive coding (PC) and the free energy principle (FEP). Not new
runtime behavior — a reframing of existing behavior in terms that connect
to the neuroscience-ML literature.

## The identification

| MDEMG mechanism          | PC/FEP quantity                 |
|--------------------------|---------------------------------|
| DH-005 health formula    | precision-weighted posterior mean |
| Surprise detector        | variational surprise S(obs)     |
| Hebbian edge update      | local prediction-error descent  |
| Layer promotion          | hierarchical feature induction  |
| Interceptor loop         | active inference cycle          |
| RSIC feedback            | generative-model update from outcomes |

## What follows from the identification

1. **Sprint-level**: every update rule in the system can (and should) be
   precision-weighted. Currently DH-005 is the only one that is.
2. **Architectural**: MDEMG is missing the **top-down prediction stream**
   that is canonical in PC — higher layers do not predict lower-layer
   activity. This is the main addition targeted by Sprint B.
3. **Research**: the framework is a candidate for the Millidge-Tschantz-
   Buckley (2022) result that PC approximates backprop on arbitrary
   computation graphs. This means MDEMG's heterogeneous pipeline
   (Neo4j + sidecar + LLM + RSIC) can be trained end-to-end via local
   PC updates, without requiring a monolithic differentiable graph.

## What this is NOT

- Not a claim that MDEMG is biologically accurate.
- Not a commitment to eliminate backprop from the neural sidecar
  (see Ororbia & Mali — FF underperforms at scale; same caution
  applies to "PC everywhere").
- Not a runtime behavior change. Runtime changes land in Phase 2
  (surprise routing) and Sprint B (precision-weighted η, top-down
  predictions).

## See also

- VISION.md "Theoretical Foundation"
- CLAUDE.md "Theoretical Vocabulary"
- `docs/features/rsic-feedback-loop.md` — "Health Weighting & Confidence"
- White paper review: `docs/development/mdemg-white-paper-review.md`
```

**Files**: `docs/features/predictive-coding-scaffold.md` (new)

---

## Phase 2: Surprise Routing (new capability)

**Goal**: Extend the surprise signal from weight-multiplier (current) to routing-decision (new). High-surprise observations from high-trust sources take a fast path; high-surprise observations from low-trust sources enter a quarantine queue pending corroboration.

### 2.1 Add `SourceTrust` to `Observation` (CRITICAL)

**Gap**: The `Observation` struct in `internal/conversation/service.go` lacks any notion of source trust. Surprise routing requires a trust signal orthogonal to surprise itself.

**Current state**: Observations have `Source` (string, e.g. "chat", "hook", "ingestion") but no trust grade.

**Fix** — Add to `Observation` in `internal/conversation/service.go`:

```go
// SourceTrust is a calibrated trust grade for the observation's source.
// Populated by the ingestion gate, not the caller. Values:
//   trusted    — authenticated user, verified code review, or validated SME ingestion
//   normal     — default for unauthenticated agents or routine ingestion (no elevated trust)
//   untrusted  — anonymous, flagged by upstream checks, or from a previously-correcting source
// Used by the ingestion gate to decide fast-track / normal / quarantine routing
// when combined with surprise_score.
SourceTrust string `json:"source_trust,omitempty"`
```

**Trust grade assignment** — implemented in `internal/conversation/source_trust.go` (new):

```go
package conversation

import (
    "mdemg/internal/config"
)

// ClassifySourceTrust returns a trust grade given the Principal
// (from middleware/auth.go) and the observation's origin.
func ClassifySourceTrust(source string, principal auth.Principal, cfg config.Config) string {
    // Authenticated user with a role scope → trusted
    if principal.AuthMethod != "none" && len(principal.Scopes) > 0 {
        return "trusted"
    }
    // Hook from verified tool (claude-code, validated CLI) → normal-to-trusted
    if source == "hook" && principal.AuthMethod != "none" {
        return "trusted"
    }
    // Anonymous API call with public endpoint → untrusted
    if principal.AuthMethod == "none" {
        return "untrusted"
    }
    return "normal"
}
```

**Files**: `internal/conversation/service.go`, `internal/conversation/source_trust.go` (new)

**Dependency**: GAP-16 `RequireScope` machinery (already landed per updated `repo-to-public-roadmap.md`).

---

### 2.2 New `IngestionGate` for surprise × source-trust routing (CRITICAL)

**Gap**: No component decides what to do with high-surprise observations beyond weight-boosting. All paths lead to the same ingestion pipeline.

**Fix** — Create `internal/conversation/ingestion_gate.go`:

```go
package conversation

import (
    "context"
    "mdemg/internal/config"
)

// IngestionLane describes the route an observation takes after surprise +
// source-trust classification.
type IngestionLane string

const (
    // LaneFastTrack: high surprise + trusted source → promote eagerly,
    // expedite emergence, skip default corroboration wait.
    LaneFastTrack IngestionLane = "fast_track"
    // LaneNormal: default ingestion. Current behavior for most observations.
    LaneNormal IngestionLane = "normal"
    // LaneQuarantine: high surprise + untrusted source → persist node but
    // do not create edges or trigger promotion until corroborated by
    // trusted-source observations or manual review.
    LaneQuarantine IngestionLane = "quarantine"
)

// IngestionGate is the surprise × source-trust routing policy. Thresholds
// are configurable via env vars; defaults are conservative.
type IngestionGate struct {
    cfg config.Config
}

// Route classifies the observation's ingestion lane. Pure function of
// surprise_score, source_trust, and configured thresholds.
func (g *IngestionGate) Route(obs Observation) IngestionLane {
    hi := g.cfg.SurpriseFastTrackThreshold   // default 0.75
    qt := g.cfg.SurpriseQuarantineThreshold  // default 0.60
    switch obs.SourceTrust {
    case "trusted":
        if obs.SurpriseScore >= hi {
            return LaneFastTrack
        }
        return LaneNormal
    case "untrusted":
        if obs.SurpriseScore >= qt {
            return LaneQuarantine
        }
        return LaneNormal
    default: // "normal"
        return LaneNormal
    }
}
```

**Wiring** — in `internal/conversation/service.go` `Observe()`:

```go
// After surprise detection, before node creation:
lane := s.ingestionGate.Route(obs)
obs.IngestionLane = string(lane)

switch lane {
case LaneFastTrack:
    // Eager promotion eligibility, skip default 5-obs corroboration window
    obs.Metadata["promotion_eligible_at"] = "immediate"
case LaneQuarantine:
    // Persist node, but do NOT create CO_ACTIVATED_WITH edges
    // Do NOT enqueue for promotion evaluation
    // Store obs.Metadata["quarantine_reason"] = "low_trust_high_surprise"
    return s.observeQuarantined(ctx, obs)
}
// LaneNormal: existing path unchanged
```

**Files**: `internal/conversation/ingestion_gate.go` (new), `internal/conversation/service.go` (modify `Observe` and add `observeQuarantined`)

---

### 2.3 Quarantine re-evaluation on corroboration (HIGH)

**Gap**: A quarantined observation must be re-evaluated when a trusted source corroborates it. Without this the quarantine lane is a one-way trip.

**Fix** — Add corroboration check in `internal/conversation/service.go`:

```go
// observeCorroborationCheck is called after a trusted-source observation
// lands. It finds quarantined nodes with high embedding similarity to the
// new observation and re-routes them through the normal lane.
func (s *Service) observeCorroborationCheck(
    ctx context.Context, newObs Observation,
) error {
    if newObs.SourceTrust != "trusted" {
        return nil
    }
    candidates, err := s.findQuarantinedSimilar(ctx, newObs.SpaceID, newObs.Embedding, 0.82)
    if err != nil {
        return err
    }
    for _, qobs := range candidates {
        // Re-route: create edges, enable promotion
        if err := s.releaseFromQuarantine(ctx, qobs.NodeID); err != nil {
            // Log but continue — partial release is safe
            continue
        }
    }
    return nil
}
```

**Cypher for release** (inside `releaseFromQuarantine`):

```cypher
MATCH (n:MemoryNode {node_id: $nodeId})
REMOVE n.quarantined
SET n.released_at = datetime(),
    n.released_by = $releasedByNodeId
RETURN n
```

**Files**: `internal/conversation/service.go`

---

### 2.4 Env vars + config (HIGH)

**Fix** — Add to `internal/config/config.go`:

```go
// Surprise routing thresholds (PC-REFRAME-A, Phase 2)
SurpriseFastTrackThreshold  float64 // SURPRISE_FAST_TRACK_THRESHOLD (default 0.75)
SurpriseQuarantineThreshold float64 // SURPRISE_QUARANTINE_THRESHOLD (default 0.60)
SurpriseCorroborationSim    float64 // SURPRISE_CORROBORATION_SIM (default 0.82)
SurpriseRoutingEnabled      bool    // SURPRISE_ROUTING_ENABLED (default true)
```

**Plus** — expose in all 3 compose templates (`docker-compose.yml`, `docker-compose.prod.yml`, `docker-compose.dev.yml`), `.env.example`, and `config.yaml` template.

**Files**: `internal/config/config.go`, `.env.example`, `deploy/docker/docker-compose*.yml`, `internal/config/yaml_config.go`

---

### 2.5 Quarantine admin endpoints (MEDIUM)

**Fix** — Add to `internal/api/server.go`:

```go
// GET /v1/admin/quarantine?space_id=X — list quarantined observations
// POST /v1/admin/quarantine/{node_id}/release — manually release
// DELETE /v1/admin/quarantine/{node_id} — purge (for clearly-bad data)
```

Guarded by `RequireScope("admin:quarantine")`.

**Files**: `internal/api/server.go`, `internal/api/handlers_quarantine.go` (new)

---

## Phase 3: Observability

**Goal**: Make the new routing visible in Grafana. Operators must be able to see (a) routing rate by lane, (b) quarantine depth over time, (c) corroboration release rate.

### 3.1 New Prometheus gauges (HIGH)

**Fix** — Add to `internal/metrics/registry.go`:

```go
mdemg_ingestion_lane_total{space_id, lane}           // counter
mdemg_quarantine_depth{space_id}                     // gauge
mdemg_quarantine_released_total{space_id, reason}    // counter (reason: corroboration|manual)
mdemg_quarantine_age_seconds{space_id}               // histogram
```

**Files**: `internal/metrics/registry.go`, `internal/conversation/service.go` (emission sites)

---

### 3.2 Grafana panels (MEDIUM)

**Fix** — Extend `deploy/grafana/dashboards/mdemg-rsic.json` with a new row "Ingestion Routing (PC-REFRAME-A)":

- Stat panel: current quarantine depth
- Time-series: ingestion rate by lane (stacked)
- Stat panel: quarantine release rate (24h)
- Histogram: quarantine age distribution

**Files**: `deploy/grafana/dashboards/mdemg-rsic.json`

---

### 3.3 CLI inspection command (MEDIUM)

**Fix** — Add `mdemg quarantine list` / `mdemg quarantine release <node_id>` in `internal/cli/quarantine.go` (new).

**Files**: `internal/cli/quarantine.go` (new), `internal/cli/root.go` (register)

---

## Phase 4: Testing & Verification

### 4.1 Unit tests (HIGH)

**Fix** — Create:

- `internal/conversation/ingestion_gate_test.go`: table-driven test covering all 9 (trust × surprise-tier) combinations
- `internal/conversation/source_trust_test.go`: classification for each Principal auth method
- `internal/conversation/service_quarantine_test.go`: quarantine round-trip (observe → quarantine → corroborate → release)

**Required coverage**: all exported functions, all routing branches, quarantine Cypher error paths.

**Files**: all three test files above

---

### 4.2 Integration test (HIGH)

**Fix** — Create `tests/integration/pc_reframe_test.go` (build tag `integration`):

Scenario: untrusted source posts high-surprise observation X → verify X is quarantined with no edges. Trusted source posts corroborating observation Y with similar embedding → verify X is released and edges are created.

**Files**: `tests/integration/pc_reframe_test.go` (new)

---

### 4.3 A/B evaluation against whk-wms benchmark (MEDIUM)

**Fix** — Run the 120-question whk-wms benchmark with `SURPRISE_ROUTING_ENABLED=true` and `=false`; compare mean retrieval score, high-score rate, strong-evidence rate. Expected result: no regression (routing only changes untrusted+high-surprise behavior, which should be rare in whk-wms which is all trusted-source data). If a regression appears, investigate before merging.

**Files**: `docs/tests/benchmarks/pc-reframe-a-ab.md` (new, results file)

---

## Phase 5: Mandatory Documentation Phase

Per project standard (memory: "every sprint development map must include a documentation update as the final task"). No sprint is complete without this phase.

### 5.1 CHANGELOG.md (HIGH)

**Fix** — Under `[Unreleased]`:

```markdown
### Added
- PC-REFRAME-A: Predictive Coding scaffold (VISION.md, CLAUDE.md, feature doc)
- Surprise × source-trust ingestion routing (fast-track / normal / quarantine)
- 4 new env vars: SURPRISE_FAST_TRACK_THRESHOLD, SURPRISE_QUARANTINE_THRESHOLD,
  SURPRISE_CORROBORATION_SIM, SURPRISE_ROUTING_ENABLED
- 4 new Prometheus metrics for ingestion lane observability
- 2 new admin endpoints for quarantine management
- New CLI: `mdemg quarantine list|release`

### Changed
- `internal/ape/health_formula.go` now documents itself as precision-weighted
  Bayesian evidence integration (no behavior change)
- `internal/conversation/surprise.go` now documents itself as a variational
  surprise proxy (no behavior change)
```

**Files**: `CHANGELOG.md`

---

### 5.2 AGENT_HANDOFF.md (HIGH)

**Fix** — Add to "COMPLETED SINCE LAST HANDOFF" block:

```markdown
- ✅ PC-REFRAME-A: Predictive Coding reframe & surprise routing (2026-MM-DD):
  - Formalized VISION.md and CLAUDE.md with PC/FEP vocabulary
  - Annotated health_formula.go and surprise.go with theoretical identifications
  - New feature doc: docs/features/predictive-coding-scaffold.md
  - New surprise × source-trust routing with 3 lanes (fast-track/normal/quarantine)
  - Quarantine auto-release on trusted-source corroboration (cosine ≥ 0.82)
  - 4 env vars, 4 metrics, 2 admin endpoints, 1 CLI command
  - 23 tasks across 5 phases, A/B benchmark result: no regression
```

**Files**: `AGENT_HANDOFF.md`

---

### 5.3 CLAUDE.md (HIGH)

Already covered by Phase 1.2 (the Theoretical Vocabulary block). Verify it ships with the sprint.

---

### 5.4 `docs/user/cli-reference.md` (HIGH)

**Fix** — Add new subsection "Surprise Routing (PC-REFRAME-A)" under environment variables. Document all 4 new env vars with defaults, ranges, and behavior.

**Fix** — Add new subsection documenting `mdemg quarantine list` and `mdemg quarantine release`.

**Files**: `docs/user/cli-reference.md`

---

### 5.5 Feature docs index (HIGH)

**Fix** — Register `docs/features/predictive-coding-scaffold.md` in:
- `docs/features/README.md` (index)
- Any feature-doc discovery tooling (`mdemg features list` if applicable)

**Files**: `docs/features/README.md`

---

### 5.6 Homebrew-mdemg (submodule) — beta testing guide (MEDIUM)

**Fix** — In `packaging/homebrew-mdemg/mdemg_beta_testing.md`:
- Update "Version under test" to new version (v0.8.6 or whatever tags)
- Add new section "Surprise Routing Beta" with env var override examples and how to inspect quarantine via CLI
- Update `packaging/homebrew-mdemg/CHANGELOG.md` with new version block

**Files**: `packaging/homebrew-mdemg/mdemg_beta_testing.md`, `packaging/homebrew-mdemg/CHANGELOG.md`

---

### 5.7 Submodule pointer update (MEDIUM)

**Fix** — Commit submodule pointer bump in parent repo after homebrew-mdemg changes land, same sequence as RELEASE-v0.8.5:
1. Push homebrew-mdemg changes first
2. Bump parent `packaging/homebrew-mdemg` pointer
3. Merge PR to `main`
4. Tag, let GoReleaser regenerate formula on top

**Files**: `packaging/homebrew-mdemg` (submodule pointer in parent)

---

## Risk Analysis & Rollback

### R1: Surprise routing blocks legitimate observations

**Likelihood**: Medium. If source-trust classification is too conservative, legitimate untrusted-but-useful observations get quarantined.

**Mitigation**: 
- Feature flag `SURPRISE_ROUTING_ENABLED` defaults true but is easily toggled.
- Quarantine lane persists nodes (doesn't drop data); corroboration auto-releases.
- Admin endpoint allows manual release.
- A/B benchmark (Phase 4.3) validates no regression on representative data.

**Rollback**: Set `SURPRISE_ROUTING_ENABLED=false`. All observations take the Normal lane. Zero data loss — existing quarantined nodes remain persisted and can be released later.

### R2: Source-trust classification is ambiguous in multi-agent contexts

**Likelihood**: Medium-High. When two agents collaborate (dev01 and dev02 branches), trust classification may be unclear.

**Mitigation**:
- Default multi-agent source to `normal` (neither fast-track nor quarantine).
- Document the classification rules clearly in CLAUDE.md Section 1.2.
- `source_trust` is persisted on the observation, so reclassification is possible if policy changes.

**Rollback**: Same as R1.

### R3: Quarantine queue grows unboundedly

**Likelihood**: Low. Quarantine release happens automatically on corroboration.

**Mitigation**:
- `mdemg_quarantine_depth` gauge + Grafana alert at depth > 1000 per space
- TTL: quarantined observations older than 30 days are auto-purged (add to RSIC macro cycle)

**Rollback**: Admin endpoint `DELETE /v1/admin/quarantine/{node_id}` for manual purge.

### R4: Documentation-only changes create merge conflicts with concurrent sprints

**Likelihood**: Medium. VISION.md and CLAUDE.md are touched by many sprints.

**Mitigation**:
- Land Phase 1 (docs) before Phase 2 (code). Fast to merge.
- Coordinate with planning agents on dev01/dev02 to pause VISION.md edits during this sprint.

---

## Files Changed (summary)

**New files** (7):
- `docs/features/predictive-coding-scaffold.md`
- `internal/conversation/ingestion_gate.go`
- `internal/conversation/source_trust.go`
- `internal/conversation/ingestion_gate_test.go`
- `internal/conversation/source_trust_test.go`
- `internal/conversation/service_quarantine_test.go`
- `internal/api/handlers_quarantine.go`
- `internal/cli/quarantine.go`
- `tests/integration/pc_reframe_test.go`
- `docs/tests/benchmarks/pc-reframe-a-ab.md`

**Modified files** (approximately 15):
- `VISION.md`
- `CLAUDE.md`
- `CHANGELOG.md`
- `AGENT_HANDOFF.md`
- `docs/user/cli-reference.md`
- `docs/features/README.md`
- `internal/ape/health_formula.go` (comment only)
- `internal/conversation/surprise.go` (comment only)
- `internal/conversation/service.go`
- `internal/config/config.go`
- `internal/config/yaml_config.go`
- `internal/metrics/registry.go`
- `internal/api/server.go`
- `internal/cli/root.go`
- `.env.example`
- `deploy/docker/docker-compose.yml`
- `deploy/docker/docker-compose.prod.yml`
- `deploy/docker/docker-compose.dev.yml`
- `deploy/grafana/dashboards/mdemg-rsic.json`
- `packaging/homebrew-mdemg/mdemg_beta_testing.md` (submodule)
- `packaging/homebrew-mdemg/CHANGELOG.md` (submodule)

---

## Sprint Size Estimate

| Phase | Estimate |
|-------|----------|
| 1. Documentation & Formalization | 0.5 day |
| 2. Surprise Routing | 2.5 days |
| 3. Observability | 1 day |
| 4. Testing & Verification | 1.5 days |
| 5. Mandatory Documentation Phase | 0.5 day |
| Buffer / review / A/B | 1 day |
| **Total** | **~7 days (1 week)** |

This is a small-to-medium sprint. It intentionally stops short of the higher-risk changes (precision-weighted η in Hebbian updates, top-down predictions) which are deferred to Sprint B.

---

## Dependencies

**Blocks**:
- Sprint B (PC-REFRAME-B): precision-weighted η and top-down predictions both build on the vocabulary and observability added here.

**Blocked by**:
- None. All required infrastructure (auth/Principal for source-trust, surprise detector, RSIC health) already exists.

**Touches but does not block**:
- UAITS framework — the new routing signals (lane, quarantine state) may eventually become UAITS training-data features. Not required for Sprint A.

---

# Follow-on Sprints

This sprint is the foundation for a broader PC/FEP-aligned work stream. Subsequent sprints are specified in separate documents:

- **02 — Precision-weighted Hebbian η** (`02-precision-weighted-hebbian-eta.md`): extends DH-005 precision to the learning rate, not just initial weights. Medium-high risk.
- **03 — Top-down predictions & prediction-error promotion** (`03-top-down-predictions-and-promotion.md`): adds the reciprocal L_{n+1} → L_n prediction stream. Highest-risk architectural addition.
- **04 — Column-voting retrieval** (`04-column-voting-retrieval.md`): ensemble retrieval with consensus ranking. Independent of the PC track.
- **05 — Context-specific node activations** (`05-context-specific-node-activations.md`): HTM-inspired per-observation context fingerprints.
- **06 — Sparse retrieval activation** (`06-sparse-retrieval-activation.md`): top-N firing threshold at retrieval.
- **07 — FF shallow heads** (`07-ff-shallow-heads.md`): Forward-Forward-trained classifiers for small heads over frozen embeddings.
- **08 — HTM sequence memory** (`08-htm-sequence-memory.md`): predictive Jiminy Guide via sequence memory.
- **09 — Active-inference unification** (`09-active-inference-unification.md`): unify Jiminy + RSIC + consulting under a single free-energy objective.

Sprints 02 and 03 are the direct continuation of this PC/FEP thread; 04–07 are parallel, independent work; 08–09 are research-stage.

---

## Documents Accessed (during plan authorship)

- `VISION.md`, `CLAUDE.md`, `AGENT_HANDOFF.md` (project + repo versions)
- `internal/ape/health_formula.go`
- `internal/ape/self_assess.go`
- `internal/conversation/surprise.go`
- `internal/conversation/service.go`
- `internal/learning/service.go`
- `internal/config/config.go`
- `docs/development/INSTALLER_SYNC_PLAN.md` (format reference)
- `docs/development/ft-lora/03_IMPLEMENTATION_PLAN_v2.md` (format reference)
- `docs/development/SPRINT_SUMMARY_20260328.md` (format reference)
- White paper review: `mdemg-white-paper-review.md`

