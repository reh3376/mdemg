# HEBB-ETA-001 — Sprint Plan

**Date:** 2026-08-01 | **Branch:** `reh3376_dev01`
**Research:** `docs/research/mdemg_sprint_ideas/02-precision-weighted-hebbian-eta.md`
**Roadmap:** Q4 stretch tier (unblocked; prereqs SURPRISE-TOPK-001 + CoactivateSession semantics fix both shipped Q3).

## Section 1 — Header & Metadata

Line: HEBB-ETA. Sprint: 001. Behavior-changing (medium-high risk per research doc). **Ships DEFAULT-OFF** for safe substrate delivery; flag-flip requires A/B verdict.

## Section 2 — Problem Statement

The Hebbian rule in `internal/learning/service.go` treats every co-activation the same:
```
w' = clamp(wmin, wmax, (1-μ)·w + η·prod)
```
η is a single config value (× existing multipliers), independent of the confidence in the nodes being co-activated. Predictive coding says the update should be **precision-weighted** — a small confident update beats a large unconfident update. Practically: an edge between two well-evidenced, recently-reinforced nodes should update more aggressively than an edge between two sparsely-seen nodes.

## Section 3 — Scope & Constraints

**In scope** (this sprint — the safe substrate primitives):
- `ActivationConfidence` per-node property + compute function + pure-Go tests
- Backfill CLI to seed the property from existing graph state
- Flag-guarded Cypher update rule (both `ApplyCoactivation` + `CoactivateSession` paths) — default false → byte-identical to pre-sprint
- `eta_effective_pc` + `precision_mult` persisted on the edge for observability
- Startup log for operator visibility
- Config knobs (5)

**Out of scope** (disclosed follow-ups):
- **Live observe→confidence-update wiring** (hot-path change; needs its own sprint + live smoke — the writer here is the CLI backfill only, not per-observation refresh)
- RSIC adaptive-override action type (`disable_precision_eta`)
- Grafana panel row
- A/B benchmark harness + canary rollout (use existing `benchmark_runs` after operator enables flag)
- Integration test with seeded fixture graph
- Shadow-mode dual-write (would add complexity; the flag is already reversible without rebuild)

**Constraints**: no ULTS hash change (no LLM prompts touched); flag-guarded so live default is byte-identical to pre-sprint; V0-style Cypher property additions (no schema migration needed — `coalesce(..., 0.5)` handles missing values).

## Section 4 — Dependencies

Shipped Q3: SURPRISE-TOPK-001, CoactivateSession semantics fix (EVENTGRAPH-003 revealed + fixed the never-invoked path). None blocking now.

## Section 5 — Implementation Plan

- **E1** — 5 config knobs: `PRECISION_WEIGHTED_ETA_ENABLED` (default false), `CONFIDENCE_ALPHA` (1.0), `CONFIDENCE_BETA` (0.5), `CONFIDENCE_GAMMA` (0.3), `CONFIDENCE_HALF_LIFE_SEC` (604800 = 1w).
- **E2** — `internal/hidden/confidence.go`: `ConfidenceConfig` + `ComputeActivationConfidence(reinforceCount, lastActivatedSecondsAgo, surpriseHistory, cfg)` — pure function, sigmoid over `α·log(1+n) + β·recency_decay − γ·surprise_variance`, clamped [0.05, 1.0]. Plus 7 unit tests (clamp bounds, monotonic in reinforce, decays with age, penalized by variance, empty-history no-penalty, HalfLife=0 safe, default-value pin).
- **E3** — Cypher extension (both paths):
  - `ApplyCoactivation` — add `$precisionEta` param + `precisionMult` compute + `etaEff = eta*etaMult*precisionMult` + `SET r.eta_effective_pc/precision_mult` (null when flag off) + `RETURN etaEff AS eta_effective`.
  - `CoactivateSession` — same shape, minus the etaMult (this path doesn't use it).
  - When flag off: `precisionMult=1.0` → byte-identical to pre-sprint math.
- **E4** — `internal/hidden/confidence_backfill.go` + `internal/cli/confidence.go`: `mdemg confidence backfill --space-id <id> [--dry-run] [--batch-size N]`. Reads reinforce_count via SUM(evidence_count) on incoming CO_ACTIVATED_WITH edges; last_activated via MAX(last_activated_at); surprise via node's surprise_score (single-value → no variance penalty; a richer history from reinforcement_events is a follow-up).
- **E5** — Startup log: `slog.Info("hebb: precision-weighted eta", enabled, alpha, beta, gamma, half_life_sec)` in `internal/api/server.go`.
- **E6** — Feature doc + sprint plan + post + CLAUDE.md pin + CHANGELOG.

## Section 6 — Testing Plan (3 tiers)

- **Tier 1 unit**: 7 tests in `confidence_test.go` covering the pure function's properties + default-value pin.
- **Tier 2 integration**: build + full `go test ./...` (learning package tests exercise the Cypher via mock scenarios but no live Neo4j).
- **Tier 3 live e2e**:
  1. Rebuild + restart mdemg → verify boot log emits `hebb: precision-weighted eta enabled=false alpha=1 beta=0.5 gamma=0.3 half_life_sec=604800`.
  2. `mdemg confidence backfill --space-id mdemg-dev --dry-run` → count of computable nodes.
  3. `mdemg confidence backfill --space-id mdemg-dev` (real) → count of written nodes.
  4. Neo4j query: `MATCH (n:MemoryNode {space_id:'mdemg-dev'}) WHERE n.activation_confidence IS NOT NULL RETURN count(n), avg, min, max, median` — verify distribution is sensible (mass near 0.5 for sparse nodes, tail up to ~1.0 for high-signal nodes).
  5. Confirm flag stays default-off (`grep PRECISION_WEIGHTED_ETA_ENABLED .env` = empty/absent).

## Section 7 — Commit Strategy

Single commit — 6 epics form a cohesive default-off primitives shipment. Follow-up sprints ship the deferred pieces (live observer, RSIC override, Grafana, A/B, integration test) if the flag is validated worth flipping.

## Section 8 — Verification Checklist

- [ ] `go build ./...` clean
- [ ] `go test ./...` all green
- [ ] `golangci-lint run` clean on modified packages
- [ ] Server restart shows `hebb: precision-weighted eta enabled=false ...` boot log
- [ ] Backfill CLI dry-run succeeds
- [ ] Backfill CLI real run writes confidence values on non-archived nodes
- [ ] Distribution query shows sensible [0.05, 1.0] spread with mass near 0.5

## Section 9 — Documentation Update

- `docs/features/precision-weighted-eta.md` (new)
- CLAUDE.md — pin under HEBB line
- CHANGELOG.md — user-visible entry

## Section 10 — Risks & Mitigations

- **R1**: Flag-flip without backfill collapses η by 4× (0.5 × 0.5 = 0.25) on every pair whose nodes lack `activation_confidence`. **Mitigation**: config comment + feature doc + startup log all name this constraint; operator must backfill before enabling.
- **R2**: Enabling on production without an A/B could regress retrieval quality invisibly. **Mitigation**: default-off ships in code + `.env` untouched; enabling requires operator intent (env flip + rebuild/restart).
- **R3**: The `$precisionEta` param increments Cypher plan cache miss on the first request per Neo4j session. **Mitigation**: negligible one-shot cost; Neo4j caches by param signature.

## Section 11 — Documents Accessed

- `docs/research/mdemg_sprint_ideas/02-precision-weighted-hebbian-eta.md` (source design)
- `docs/development/roadmap/ROADMAP_2026Q4.md` (Q4 stretch tier ranking)
- `internal/learning/service.go` (both Hebbian update sites)
- `internal/config/config.go` (env-parsing patterns)
- `internal/cli/analyze_go_implements.go` (CLI subcommand pattern)
- `internal/api/server.go` (startup log location)
- Live Neo4j on mdemg-dev (69,655 non-archived nodes)
