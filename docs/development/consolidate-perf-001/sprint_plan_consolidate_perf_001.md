# Sprint Plan — CONSOLIDATE-PERF-001 (Sprint A: instrument + cheap wins)

## 1. Header & Metadata
- **Sprint ID**: CONSOLIDATE-PERF-001 (Sprint A of the consolidation-performance track)
- **Sprint line**: `docs/development/consolidate-perf-001/`
- **Date opened**: 2026-06-30
- **Target version**: v0.11.2 (patch)
- **Estimated effort**: ~1 dev-day
- **OpenAI spend**: $0
- **Risk level**: Low–Medium (the cheap wins must be behavior-preserving;
  instrumentation is zero-risk; the risky algorithmic change — incremental
  ForwardPass — is deferred to Sprint B with this sprint's measurements in hand)

## 2. Problem Statement
Memory consolidation (Flow A — the full hierarchy pipeline behind
`POST /v1/memory/consolidate` / the RSIC watchdog trigger) runs **~38 min,
2–3×/day** on the 83k-node `mdemg-dev` graph (live: 2312–2339 s per cycle, 4
completions in 36h), saturating Neo4j CPU (windowed AVG to 471, bursts to
1003%). This is the real signal the now-truthful `neo4j-cpu` alert
(ALERT-TRUTH-001) points at. Before the algorithmic fix (Sprint B), we need the
**actual per-phase breakdown** — the dominant phase is currently *inferred*
(ForwardPass/BackwardPass), not measured — and the **low-risk reductions** that
don't touch the delicate identity/churn logic (HIDDEN-CHURN-001/002/003 lineage).

Live-corrected facts (two sub-agent claims were wrong on live data):
- The `(space_id, layer)` composite index **already exists** (`memorynode_layer_idx`),
  as do `last_forward_pass`/`last_backward_pass` properties + indexes — so the
  label scans are already indexed and incremental-ForwardPass infra is partly in
  place (Sprint B).
- Consolidation **completes** ~2–3×/day (not "continuous aborting") — the
  watchdog path has no 30-min deadline; the mid-cycle abort only bites the manual
  `/v1/memory/consolidate` path (Epic 4 addresses that).

Inferred cost drivers (to be confirmed by Epic 1's measurement):
1. **ForwardPass + BackwardPass** (`hidden/service.go:1467–1674`) — batched
   full-scan of all L1 patterns (50/batch), **re-run up to 5×** (per concept
   layer). → Sprint B.
2. **`findSimilarConcept`** (`service.go:1347`) — a full vector scan per cluster.
   → Epic 2.
3. **`listHiddenPatternRefs`** (`hidden_identity.go:213`) — all L1 patterns +
   members every cycle. → Sprint B.
4. **`dynamic_edges`** (`service.go:3063`) — Cartesian cross-join on L3+ nodes.
   → Epic 3.

## 3. Scope & Constraints
**In scope:**
1. **Per-phase instrumentation** — time each consolidation phase, emit
   `mdemg_consolidation_phase_duration_seconds{phase,space_id}` + a structured
   per-cycle summary log. The measurement that targets Sprint B.
2. **Batch `findSimilarConcept`** — replace k per-cluster full vector scans with
   one `UNWIND`-over-centroids query. Behavior-preserving (same nearest concept).
3. **`dynamic_edges` off the hot path** — config-gated to run every Nth cycle
   instead of every consolidation. No edges lost, just deferred.
4. **Manual-path abort guard** — `CONSOLIDATE_TIMEOUT_MS` default 30→60 min so the
   manual path completes instead of aborting mid-cycle at 30.

**Out of scope (→ Sprint B, data-driven):** incremental ForwardPass/BackwardPass
dirty-tracking; the run-ForwardPass-once-not-5× change; `listHiddenPatternRefs`
bounding. The algorithmic core — needs this sprint's measurements + careful
identity/churn validation.

**Constraints:** no hardcoded values (every new knob → config); sequential epics;
live Tier-3 required; cheap wins behavior-preserving (proven by identity-survival
+ churn parity vs the HIDDEN-CHURN-003 baseline); dev branch → auto-PR.

## 4. Dependencies
`internal/hidden/` (the consolidation service: ForwardPass/BackwardPass,
`findSimilarConcept`, `dynamic_edges`, the phase sequence), the consolidate
handler + timeout (`internal/api/handlers.go:1606`), the metrics registry, the
macro-cron scheduler (`server.go:617`), and the HIDDEN-CHURN-003 incremental path
(must stay intact).

## 5. Implementation Plan (sequential epics + gates)
- **Epic 0** — this plan committed.
- **Epic 1 — instrumentation.** Wrap each phase in the consolidation pipeline with
  timing; emit the histogram + a per-cycle summary log. *Gate*: a real
  consolidation shows non-zero per-phase rows in TSDB.
- **Epic 2 — batch `findSimilarConcept`.** One `UNWIND $centroids` query → nearest
  existing concept per centroid. *Gate*: unit test proves identical matches vs the
  per-cluster loop on a fixture.
- **Epic 3 — `dynamic_edges` cadence.** `DYNAMIC_EDGES_EVERY_N_CYCLES` (default 6);
  skipped cycles log it. *Gate*: edges still created on the scheduled cycle; none
  lost vs a baseline.
- **Epic 4 — timeout default 30→60 min** (`CONSOLIDATE_TIMEOUT_MS`). *Gate*: manual
  consolidate completes.
- **Epic 5 — Documentation** (final): feature doc / CLAUDE.md note / CHANGELOG;
  record the measured per-phase breakdown as the Sprint-B input.

## 6. Testing Plan (3 tiers)
- **Tier 1:** `findSimilarConcept` batch-vs-loop equivalence; phase-timer unit;
  dynamic-edges cadence gating.
- **Tier 2:** a consolidation against a seeded graph — phase metrics emitted; edge
  counts unchanged by the batching/cadence changes.
- **Tier 3 (live):** run real consolidations on `mdemg-dev`; **observe the
  per-phase breakdown in TSDB** (the deliverable that scopes Sprint B); confirm
  **100% identity survival + churn parity** (HIDDEN-CHURN-003 invariant) and a
  measurable CPU/time reduction from Epics 2–3; manual consolidate completes under
  the 60-min guard.

## 7. Commit Strategy
One commit per epic on `reh3376_dev01`; final promotes CHANGELOG → v0.11.2.
Push → auto-PR.

## 8. Verification Checklist
- [ ] per-phase metric live in TSDB
- [ ] `findSimilarConcept` batched + equivalence-tested
- [ ] `dynamic_edges` cadence-gated, no edges lost
- [ ] `CONSOLIDATE_TIMEOUT_MS` default 60 min
- [ ] 100% identity survival + churn parity (HIDDEN-CHURN-003 invariant)
- [ ] measured CPU/time reduction
- [ ] `golangci-lint run ./...` + build clean
- [ ] CLAUDE.md + CHANGELOG + feature doc + the per-phase breakdown for Sprint B

## 9. Documentation Update — Epic 5 above.

## 10. Risks & Mitigations
| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| A cheap win subtly changes the hierarchy | Low | High | Behavior-preserving by construction; Tier-3 asserts identity survival + churn parity vs the HIDDEN-CHURN-003 baseline |
| Deferring `dynamic_edges` drops edges | Low | Medium | Cadence only *delays*; edges still created on the scheduled cycle; verified by count |
| We optimize the wrong phase | Medium | Low | Epic 1 measures first; Epics 2–3 are low-risk regardless; the big bet (ForwardPass) waits for the data |
| Batched vector query returns different matches | Low | Medium | Tier-1 equivalence test vs the per-cluster loop; identical threshold/limit semantics |

## 11. Documents Accessed
- `internal/hidden/service.go` (ForwardPass/BackwardPass/`findSimilarConcept`/`dynamic_edges`/phase sequence)
- `internal/hidden/hidden_identity.go`
- `internal/api/handlers.go` (consolidate handler + timeout)
- `internal/metrics/` (registry, collectors)
- `internal/cli/serve.go`, `internal/api/server.go` (triggers / macro-cron)
- CLAUDE.md HIDDEN-CHURN-001/002/003 + consolidation notes

## 12. Rollback Procedures
All changes are config-defaulted code; revert the commit(s). No schema/migration.
Instrumentation is additive. `dynamic_edges` cadence + timeout are config knobs.
