# Sprint CONSOLIDATE-PERF-002 (Sprint B of the CONSOLIDATE-PERF line) — incremental forward/backward passes + bounded dynamic-edge seeds

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | CONSOLIDATE-PERF-002 |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Format | Sprint plan v1.0 (12-section) |
| Effort | ~1 dev-day |
| Parent | CONSOLIDATE-PERF-001 ("Sprint B: the incremental-ForwardPass rewrite — `last_forward_pass` infra exists") + the RETRIEVAL-TYPED-EDGES-002 capacity note (retrieval/consolidation Neo4j contention) |

## 2. Problem Statement

Live 7d phase metrics (mdemg-dev, ~100 cycles ≈ 14/day):
`forward_initial` avg **17.0s** + `backward` **15.8s** are full-graph sweeps —
every cycle recomputes message-pass embeddings for ALL 5,198 L1 nodes and ALL
19,570 L2+ concepts (SKIP/LIMIT-50 ⇒ ~500 batch queries) even when nothing
under them changed. `step:dynamic_edges` avg **23.7s** re-probes the vector
index for EVERY L≥1 node each cycle. Combined ≈ 56s × 14/day ≈ **13 min/day
of heavy Neo4j load** — the exact contention that degrades retrieval
(TYPED-EDGES-002: identical queries succeed at CPU 1, fail at CPU 127).
Steady-state change volume is tiny (tens of L0 observations/hour), so ≥95%
of that work recomputes values that cannot have changed: both passes are
pure functions of (own embedding, member embeddings+weights, concept
embeddings) — skip-if-inputs-unchanged is exactly correct.

## 3. Scope & Constraints

**In scope (all behind `HIDDEN_INCREMENTAL_PASSES_ENABLED`, code default
false, `.env` true after live smoke — the standing contract):**
- **E1 forward L1 gate**: process only h where `h.last_forward_pass IS NULL`
  OR a member is newer (`b.updated_at > h.last_forward_pass` OR
  `r:GENERALIZES.created_at > h.last_forward_pass` — edge recency catches
  CHURN-003 re-assignments of old nodes; verified: 56,404/56,404 edges carry
  `created_at`).
- **E1 forward L2+ gate**: `c.last_forward_pass IS NULL` OR a member h
  advanced (`h.last_forward_pass > c.last_forward_pass`) OR
  `r:ABSTRACTS_TO.created_at > c.last_forward_pass` (5,239/5,239 edges carry
  `created_at`) — the cascade falls out of the timestamps we already write.
- **E2 backward gate**: `h.last_backward_pass IS NULL` OR member-newer OR
  concept-advanced (`c.last_forward_pass > h.last_backward_pass`) OR
  either edge type newer.
- **E3 dynamic-edges seed bound**: seed only nodes with
  `a.created_at > $since` OR `a.updated_at > $since`, `$since` = now −
  `DYNAMIC_EDGE_INCREMENTAL_LOOKBACK_HOURS` (default 6 ≈ 3–4 cycle
  intervals; 0 = full sweep, byte-equivalent legacy). MERGE-idempotent, so
  an over-wide window only wastes work, never corrupts.
- Unit tests (gated Cypher shape; flag-off legacy-identical); live Tier-3
  before/after phase timings; docs.
**Out of scope:** `node_creation`/`post_clustering` internals (separate
phases), summaries_llm (manual-path only, automated path reads 0.1s),
changing pass math, full-recompute scheduling (the explicit
`full_recluster` / `concepts recluster` path remains the escape hatch).
**Constraints:** correctness caveat documented — weight-only drift (decay
touching GENERALIZES/ABSTRACTS_TO weights without node/edge timestamps)
is invisible to the gates; it self-corrects whenever any member changes and
fully corrects on the explicit full path. Flag-off must remain
behavior-identical.

## 4. Dependencies

✅ `last_forward_pass`/`last_backward_pass` written by the existing passes;
✅ node `updated_at` universal (59,409/59,409 L0); ✅ edge `created_at`
universal on both edge types; ✅ phase-duration metrics
(CONSOLIDATE-PERF-001) for before/after.

## 5. Implementation Plan (sequential)

- **E0** this plan. **E1** forward gates + flag + unit tests.
- **E2** backward gate + tests. **E3** dynamic-edges `$since` seed bound +
  config + tests. **E4** live Tier-3: baseline timings (7d table above) →
  flag on, trigger 2–3 consolidations (one right after fresh observations,
  one idle) → compare `mdemg_consolidation_phase_duration_seconds`; verify
  updated-counts drop to the changed subset while a fresh observation still
  propagates L0→L1→L2 (embedding-value spot check). **E5** docs: feature doc
  `docs/features/consolidation-performance.md` §Sprint-B, CHANGELOG,
  CLAUDE.md note, post.

## 6. Testing Plan

Tier 1: gate-Cypher unit tests both flag states. Tier 2: `go test ./...`.
Tier 3: E4 live before/after with propagation spot-check.

## 7. Commit Strategy

`docs(E0)` → `feat(E1+E2)` → `feat(E3)` → `docs(E4+E5)`. Surprise defects
get own fix-commits.

## 8. Verification Checklist

unit green · build+lint · live: idle-cycle forward+backward each <2s
(vs 17/15.8) · live: fresh-observation cycle still propagates to L1/L2 ·
dynamic_edges idle-cycle <2s (vs 23.7) · flag-off legacy-identical ·
docs · pushed.

## 9. Documentation Update

Feature doc §; CHANGELOG; CLAUDE.md CONSOLIDATE-PERF note amendment
(Sprint B shipped); post.md with the timing table.

## 10. Risks & Mitigations

| Risk | Sev | Mitigation |
|---|---|---|
| Gate misses a change class → stale message-pass embeddings | Med | Gates keyed on ALL input paths (node ts + both edge-type ts + cascade fields); decay-drift caveat documented; explicit full path remains; flag rollback |
| Gate query itself costs more than it saves | Low | EXISTS-subquery per candidate over indexed properties; measured live in E4 — if not a clear win, don't flip the flag (measure-first rule) |
| Concept cascade ordering (L2 gate reads h.last_forward_pass set THIS cycle) | Low | Forward runs L1 phase before L2 phase in the same call — ordering already guaranteed by the existing sequence |

## 11. Rollback

`HIDDEN_INCREMENTAL_PASSES_ENABLED=false` (legacy full sweeps); revert
commits for full removal.

## 12. Documents Accessed

`internal/hidden/service.go` (ForwardPass :1453, BackwardPass :1608,
CreateDynamicEdges :3084); live `metric_samples` 7d phase table; live Neo4j
timestamp-coverage counts; CONSOLIDATE-PERF-001 + HIDDEN-CHURN-003 +
RETRIEVAL-TYPED-EDGES-002 notes (CLAUDE.md).
