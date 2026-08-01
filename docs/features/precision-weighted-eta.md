# Precision-Weighted Hebbian η

**Shipped in:** HEBB-ETA-001 (2026-08-01), **default-off**
**Research:** `docs/research/mdemg_sprint_ideas/02-precision-weighted-hebbian-eta.md`

## Why

The default Hebbian rule treats every co-activation equally: `w' = clamp((1-μ)·w + η·prod)`. Every reinforcement moves the edge weight by the same η regardless of how confident we are in the nodes being co-activated. That over-weights signal from sparsely-seen or unstable nodes and under-weights signal from well-evidenced, recently-active ones.

Predictive coding (see `docs/research/mdemg_sprint_ideas/01-pc-reframe-and-surprise-routing.md`) says the learning-rate should be **precision-weighted**: small confident updates beat large unconfident ones. HEBB-ETA-001 introduces per-node `ActivationConfidence` and modulates η by the product of the two nodes' confidences.

## Choices

Three approaches considered:

1. **Hard-code confidence into η via edge properties** — Would require rewriting the whole update path and complicates rollback. Rejected.
2. **Shadow-mode dual-write** — Compute etaEff without applying, for offline analysis. Extra Cypher writes on every reinforcement. Deferred as unnecessary complexity — the shipped flag is already reversible without rebuild.
3. **Flag-guarded live update with backfill CLI** — Ships the primitives, keeps behavior byte-identical when flag off, gives operators a safe way to seed confidence values before enabling. **Chosen.**

## How it works

**Per-node confidence** (`internal/hidden/confidence.go`):
```
c = sigmoid(α·log(1+n_reinforce) + β·recency_decay(halflife) − γ·surprise_variance)
```
Clamped to `[0.05, 1.0]`. The 0.05 floor prevents multiplicative zeroing of η on edges touching un-reinforced nodes.

**Cypher update rule** (both `ApplyCoactivation` + `CoactivateSession`):
```cypher
// When flag off: precisionMult = 1.0 → byte-identical to pre-sprint math.
// When flag on: precisionMult = c_a * c_b (missing values default to 0.5 via COALESCE)
WITH ..., CASE WHEN $precisionEta
  THEN coalesce(a.activation_confidence, 0.5) * coalesce(b.activation_confidence, 0.5)
  ELSE 1.0
END AS precisionMult,
    ($eta * etaMult * precisionMult) AS etaEff
SET r.weight = ...(1-μ)·w + etaEff·prod...,
    r.eta_effective_pc = CASE WHEN $precisionEta THEN etaEff ELSE null END,
    r.precision_mult = CASE WHEN $precisionEta THEN precisionMult ELSE null END
```

`r.eta_effective_pc` + `r.precision_mult` persist on the edge for post-hoc analysis (null when flag off — historical rows stay clean).

**Backfill CLI**: seeds `n.activation_confidence` from the current graph state (reinforce count via SUM(evidence_count) on incoming CO_ACTIVATED_WITH, last_activated via MAX, surprise via node.surprise_score). Idempotent; safe to schedule.

## How to use

### 1. Backfill first (mandatory before enabling)

```bash
# Dry-run to see the count
mdemg confidence backfill --space-id mdemg-dev --dry-run

# Real run (writes activation_confidence on every non-archived MemoryNode)
mdemg confidence backfill --space-id mdemg-dev
```

⚠️ **Without backfill, enabling `PRECISION_WEIGHTED_ETA_ENABLED=true` shrinks η by 4× on every pair** (both nodes fall back to the 0.5 COALESCE default → precisionMult = 0.25). This is a silent performance-relevant substrate regression. The startup log names this constraint.

### 2. Enable in a dev space

```bash
# .env
PRECISION_WEIGHTED_ETA_ENABLED=true
```
Restart mdemg. Boot log confirms:
```
level=INFO msg="hebb: precision-weighted eta" enabled=true confidence_alpha=1 ...
```

### 3. Observe

Per-edge post-flip: `r.eta_effective_pc` (float, populated only on updates during flag-on windows) + `r.precision_mult` (float, [0.0025, 1.0]). Query examples:

```cypher
// Distribution of precision multipliers on recent reinforcements
MATCH ()-[r:CO_ACTIVATED_WITH {space_id: 'mdemg-dev'}]->()
WHERE r.precision_mult IS NOT NULL
  AND r.updated_at > datetime() - duration('PT1H')
RETURN
  percentileCont(r.precision_mult, 0.10) AS p10,
  percentileCont(r.precision_mult, 0.50) AS p50,
  percentileCont(r.precision_mult, 0.90) AS p90
```

### 4. A/B validation

Use the existing `benchmark_runs` + `mdemg data curate` pipeline to compare retrieval quality before/after flag flip in a dev space. Success criterion (from the research doc §3.3): B ≥ A with no individual question regressing more than 10%.

### 5. Rollback

Set `PRECISION_WEIGHTED_ETA_ENABLED=false`, restart. The `activation_confidence` values on nodes and `eta_effective_pc` / `precision_mult` on edges are additive — safe to leave in place. Cleanup optional:

```cypher
MATCH (n:MemoryNode {space_id: 'mdemg-dev'}) REMOVE n.activation_confidence, n.activation_confidence_updated_at
MATCH ()-[r:CO_ACTIVATED_WITH {space_id: 'mdemg-dev'}]->() REMOVE r.eta_effective_pc, r.precision_mult
```

## Related sprints + follow-ups

- **HEBB-ETA-001 sprint dir**: `docs/development/hebb-eta-001/` (plan + post)
- **Follow-up 1 — Live observe→confidence-update wiring**: current writer is CLI backfill only; per-observation refresh needs its own sprint + live smoke.
- **Follow-up 2 — RSIC adaptive-override**: `disable_precision_eta` action that clears the flag at runtime on sustained health drop.
- **Follow-up 3 — Grafana panel**: 4-panel row on `mdemg-rsic.json`.
- **Follow-up 4 — Multi-sample surprise history**: current writer uses single-value history (variance=0 → γ term inactive). Fuller history from `reinforcement_events` TSDB would activate the variance penalty.
- **Upstream — Sprint 01 PC-REFRAME**: `docs/research/mdemg_sprint_ideas/01-pc-reframe-and-surprise-routing.md` (surprise-routing side of predictive-coding reframe; independent from this sprint).
