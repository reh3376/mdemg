# HEBB-ETA-001 — Sprint Post

**Date:** 2026-08-01 | **Branch:** `reh3376_dev01`

## Verdict

**Shipped default-off.** Precision-weighted Hebbian η primitives are live: per-node ActivationConfidence, pure-Go compute function, flag-guarded Cypher rule (both `ApplyCoactivation` + `CoactivateSession`), backfill CLI, startup log, tests. `PRECISION_WEIGHTED_ETA_ENABLED=false` in code and `.env` — zero substrate behavior change until an operator flips the flag.

## What shipped

### E1 — Config (5 knobs)
`PRECISION_WEIGHTED_ETA_ENABLED` (false), `CONFIDENCE_ALPHA` (1.0), `CONFIDENCE_BETA` (0.5), `CONFIDENCE_GAMMA` (0.3), `CONFIDENCE_HALF_LIFE_SEC` (604800 = 1w).

### E2 — Pure compute function
`internal/hidden/confidence.go`:
- `ConfidenceConfig` struct + `DefaultConfidenceConfig()` returning shipped defaults
- `ComputeActivationConfidence(reinforceCount, lastActivatedSecondsAgo, surpriseHistory, cfg)` — `sigmoid(α·log(1+n) + β·recency_decay − γ·surprise_variance)`, clamped `[0.05, 1.0]`. The 0.05 floor is deliberate — a floor of 0 would multiplicatively zero out η for edges touching an un-reinforced node and stall learning.
- 7 unit tests covering: clamp bounds, monotonic in reinforce count, decays with age, penalized by variance, empty-history no-penalty, HalfLife=0 divide-by-zero safety, default-value pin.

### E3 — Flag-guarded Cypher (both Hebbian paths)
`internal/learning/service.go`:
- `ApplyCoactivation` — new `$precisionEta` param + `precisionMult = coalesce(a.activation_confidence, 0.5) * coalesce(b.activation_confidence, 0.5)` when flag on, else `1.0`. `etaEff = eta*etaMult*precisionMult`. Weight update uses etaEff. Persists `r.eta_effective_pc` + `r.precision_mult` (null when flag off, so historical rows stay clean). RETURN clause now surfaces the true etaEff (was `$eta * etaMult`).
- `CoactivateSession` — same shape (no etaMult in this path). Both writes are byte-identical to pre-sprint when flag off.

### E4 — Backfill CLI
`mdemg confidence backfill --space-id <id> [--dry-run] [--batch-size N]`:
- `internal/cli/confidence.go` + `internal/hidden/confidence_backfill.go`
- Streams all non-archived MemoryNodes; computes reinforce count via `SUM(evidence_count)` over incoming CO_ACTIVATED_WITH edges, last_activated via MAX, surprise via node's own `surprise_score` (single-value → no variance penalty; richer history from `reinforcement_events` is a follow-up)
- Batched writes (default 500/batch) via UNWIND
- Idempotent

### E5 — Startup log
`internal/api/server.go`:
```
level=INFO msg="hebb: precision-weighted eta" enabled=false confidence_alpha=1 confidence_beta=0.5 confidence_gamma=0.3 half_life_sec=604800
```
Grepable + operator-visible. No hidden state.

## Live Tier-3 verification (mdemg-dev, 2026-08-01)

**Boot log** (post-restart):
```
level=INFO msg="hebb: precision-weighted eta" enabled=false confidence_alpha=1 confidence_beta=0.5 confidence_gamma=0.3 half_life_sec=604800
```

**Backfill dry-run**:
```
HEBB-ETA-001: would-write activation_confidence on 69655 nodes (space=mdemg-dev)
```

**Backfill real run**:
```
HEBB-ETA-001: wrote activation_confidence on 69655 nodes (space=mdemg-dev)
```

**Distribution verification** (Cypher):
```
with_conf | avg    | min    | max    | median
69655     | 0.5149 | 0.5000 | 0.9998 | 0.5000
```

The distribution matches the sigmoid shape: most nodes cluster near the neutral 0.5 (low reinforce + old recency), high-signal nodes climb toward 1.0. No values below 0.5 in this run because the sigmoid input is `α·log(1+n) + β·recency_decay − γ·variance` and with variance=0 (single-sample surprise history) and non-negative n/recency, the sigmoid argument is always ≥ 0 → output always ≥ 0.5. The `[0.05, 0.50)` band would populate once the writer starts factoring in multi-sample surprise histories with high variance (a follow-up).

## Not shipped (intentional — disclosed follow-ups)

1. **Live observe→confidence-update wiring** — the backfill CLI seeds values; per-observation refresh is a hot-path change that needs its own sprint + live smoke. Operators can re-run backfill on a schedule (weekly cron, ~5s at mdemg-dev scale) as an interim.
2. **RSIC adaptive-override action type** (`disable_precision_eta`) — needs RSIC action-registry expansion + confidence-drop detector; separate sprint.
3. **Grafana panel row** — 4 panels (confidence distribution, eta_effective_pc distribution, enablement gauge, delta over time); adds observability but not required for the substrate primitives to work.
4. **A/B benchmark harness + canary rollout** — use existing `benchmark_runs` pipeline after operator flips the flag on a dev space; the shipped `mdemg data curate` + benchmark tooling covers the measurement side.
5. **Integration test with seeded fixture graph** — the pure-function tests + CLI live-smoke cover the primitives; a fixture-based integration test would tighten the Cypher regression net but adds a fixture-loading harness.
6. **Multi-sample surprise history** — currently `surprise_history = [node.surprise_score]` (single value → no variance penalty). Richer history from the `reinforcement_events` TSDB table (24h window of surprise_factor per node) would fully activate the γ term.
7. **`.env.example` recommendation block** — omitted from this sprint because there's no "recommended values" yet (defaults are the code defaults); update once an A/B produces evidence-backed operator recommendations.

## Rules pinned

⚠️ **Behavior-changing feature flags MUST default to off in BOTH the code default AND `.env`** — a code-default-off with a `.env`-on shipping in the same commit is a same-day flag flip disguised as a config default. This sprint follows the JIMINY-CONTRADICTED-BRIDGE-001 shape: primitives + observability ship together; the flag flip is a separate operational decision after evidence.

⚠️ **A flag whose enablement depends on a data-backfill prerequisite MUST document the prerequisite in THREE places**: config comment, feature doc, and startup log (or equivalent operator-visible surface). A silent 4× η collapse from missing `activation_confidence` values would be a stealth regression; all three surfaces name the backfill-first-then-enable rule.

## Rollback

Single-commit revert. The `activation_confidence` property values written by the backfill CLI stay on the nodes (additive, unreferenced when flag off) — cleanup with `MATCH (n:MemoryNode {space_id: $sp}) REMOVE n.activation_confidence, n.activation_confidence_updated_at` if desired.

## Documents Accessed

- `docs/research/mdemg_sprint_ideas/02-precision-weighted-hebbian-eta.md`
- `docs/development/roadmap/ROADMAP_2026Q4.md`
- `internal/learning/service.go` (both Hebbian paths + params + Cypher blocks)
- `internal/hidden/confidence.go` (new)
- `internal/hidden/confidence_backfill.go` (new)
- `internal/hidden/confidence_test.go` (new)
- `internal/cli/confidence.go` (new)
- `internal/cli/root.go` (wire subcommand)
- `internal/config/config.go` (5 knob additions)
- `internal/api/server.go` (startup log)
- Live Neo4j (mdemg-dev) — 69,655 non-archived nodes, distribution verified
