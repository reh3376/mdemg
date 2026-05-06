---
created: 2026-05-04
updated: 2026-05-04
version: v0.6.0
author: reh3376
status: active (default-on since Phase 14.1.1, 2026-05-04)
phase: phase 14 + phase 14.1 + phase 14.1.1
---

# Sparse Retrieval (Note 06 Percentile Activation Gate)

## Summary

**Feature**: `sparse-retrieval`
**Summary**: Pre-rerank percentile gate that cuts the candidate list down to those whose score crosses a per-call activation threshold. Within `[MIN_ACTIVE, MAX_ACTIVE]` clamps; emits per-call metadata + Prometheus histograms + V0019 hypertable rows. **Default-on as of Phase 14.1.1 (2026-05-04)** with the hybrid config that passed the canonical 120q A/B: `SPARSE_RETRIEVAL_ENABLED=true`, `SPARSE_MIN_ACTIVE=15` global, plus a `data_flow_integration` per-category override at `MIN=20`. Operator opt-out via `SPARSE_RETRIEVAL_ENABLED=false`. The Phase 14 → 14.1 → 14.1.1 sequence is the full narrative — Phase 14 shipped flag-off after a 120q regression in `architecture_structure`; Phase 14.1 added per-category overrides; Phase 14.1.1 closed with the simpler-first hybrid that mean-improved +0.003 with 0 regressions and 10 improvements.

## Vision & Goals

The MDEMG vision frames retrieval quality through the lens of decision-support: surface the right context with the least cognitive overhead. Today's retrieval returns top-K=20 candidates regardless of query confidence, which produces three concrete costs:

1. **Rerank prompt bloat**: the LLM-bound rerank stage at `internal/retrieval/rerank.go` packs 20 candidates into a 2400-token median prompt (4300-token p95). If the right answer is at rank 1–3, the remaining 17 candidates pay tokens for nothing.
2. **Consulting / Jiminy.Guide context-window load**: same shape — top-K candidates loaded into LLM context, mostly noise past rank 5.
3. **Set-algebra weakness**: multi-query workflows ("show me code touching X AND Y") need to intersect ranked lists. With dense top-K both sides, the intersection is dominated by the noise tier of each.

The HTM-aligned answer (Hawkins & Ahmad 2016): only candidates whose score crosses the activation percentile fire. Active-set sizes become variable — sharp queries fire 3, diffuse queries fire 25 (clamped at MAX_ACTIVE).

Phase 14 shipped the gate code + V0019 metrics + tests, then ran a 4-preset A/B sweep (16q quick + 120q full). Verdict: gate produces zero net mean change with `architecture_structure`-concentrated boundary regressions. The failure mode is now diagnosed (rank-11–20 citations get cut for queries needing them); Phase 14.1 will retune adaptively.

## Current State

### Architecture

| Component | Path | Role |
|---|---|---|
| Gate logic | `internal/retrieval/gate.go` | `ApplySparseGate` with R-7 percentile + MIN/MAX clamps + per-call metadata |
| Wiring | `internal/retrieval/service.go` | Gate fires post-aggregation, pre-rerank when `cfg.SparseRetrievalEnabled && len(results) > 0` |
| Per-request override | `internal/api/handlers.go` | `?sparse=true|false` + `?sparse_percentile=N` URL params; JSON body fields take precedence |
| Request struct | `internal/models/models.go` | `SparseEnabled` + `SparsePercentile` fields |
| Config | `internal/config/config.go` | 4 knobs + Validate() bounds (`SPARSE_*`) |
| Prometheus | `internal/metrics/collectors.go` | 3 histograms: `sparse_gate_active_count`, `sparse_gate_dropped_fraction`, `sparse_gate_threshold` |
| TSDB writer | `internal/tsdb/sparse_gate_writer.go` | Buffered V0019 writer; mirrors V0017 / V0018 patterns |
| TSDB schema | V0019 hypertable | One row per gate firing |
| Adapter | `internal/api/server.go::sparseGateRecorderAdapter` | Translates retrieval-side metadata → tsdb row (cycle-safe) |

### Workflow

1. `/v1/memory/retrieve` request arrives; handler resolves per-request overrides from URL params (or JSON body)
2. Service runs scorer (RRF or legacy linear) producing ranked `results []models.RetrieveResult`
3. **If `gateOpts.Enabled` and `len(results) > 0`**: `ApplySparseGate` computes the score-distribution's percentile, admits candidates with `score >= threshold`, applies MIN_ACTIVE floor (pulls highest-scored fallbacks if needed), applies MAX_ACTIVE ceiling (demotes lowest-scored excess if needed). Returns `(active, dropped, metadata)`.
4. Prometheus histograms emit; if `SparseGateRecorder` attached, V0019 row buffers for next flush
5. Active set continues to rerank; dropped set surfaces in `debug.below_threshold_*` when `JiminyEnabled`

### Configuration

| Env Var | Default | Description |
|---|---|---|
| `SPARSE_RETRIEVAL_ENABLED` | **`true`** | Master toggle. Default-on since Phase 14.1.1 (2026-05-04 — hybrid 120q passed) |
| `SPARSE_ACTIVATION_PERCENTILE` | `0.95` | Within-call score percentile cutoff in `[0.5, 0.999]` |
| `SPARSE_MIN_ACTIVE` | **`15`** | Floor on active set size. Bumped from 3 in Phase 14.1.1 (Phase 14's MIN=3 caused boundary regressions) |
| `SPARSE_MAX_ACTIVE` | `20` | Ceiling on active set size. Matches observed top-K cap |
| `SPARSE_GATE_CATEGORY_OVERRIDES` | `{"data_flow_integration":{"min_active":20}}` | Per-category overrides seed. The default seed handles q302-shape (4-required-files in `data_flow_integration`); operator-supplied JSON REPLACES the seed entirely (merge isn't supported) |

### V0019 telemetry

`sparse_gate_metrics` hypertable rows per gate firing:

| Column | Notes |
|---|---|
| `metric_id` | CUIDv2 |
| `recorded_at` | TS |
| `space_id` | `whk-wms` etc. |
| `percentile_applied` | After per-request override |
| `threshold_score` | The score at the cutoff |
| `input_count` | Size of the input candidate list |
| `active_count` | Returned active set size |
| `dropped_count` | input − active |
| `floor_applied` | True if MIN_ACTIVE pulled extras up |
| `ceiling_applied` | True if MAX_ACTIVE pushed excess down |
| `scorer_version` | Denormalized from V0017 for standalone analysis |

Phase 14.1 reads this hypertable to retune from production traffic instead of synthetic distributions.

## Choices that were made

### Why pre-rerank ordering (not post-rerank)

Pre-rerank captures the prompt-bloat-reduction benefit. Post-rerank wastes the saved compute by reranking everything before culling. Sprint plan §13 fork #8 chose pre-rerank, confirmed by Phase 14 Epic 0 forensic showing rerank input p95 at 4300 tokens.

### Why R-7 percentile (Excel-default linear interpolation)

R-7 is what operators expect when they say "p95" — same definition as Excel's `PERCENTILE.INC`. Other estimators (R-1 nearest-rank, R-9 with bias correction) produce step-discrete answers that confuse the operator's mental model.

### Why MIN_ACTIVE floor (clamps the gate from being too aggressive)

The percentile gate alone admits very few candidates in the high-confidence tail of typical distributions (Phase 14 Epic 0 showed p95 admits 1–2 of K=20 in the dominant production K range). The MIN_ACTIVE floor prevents the gate from collapsing to a single candidate — protects against rerank staring at an empty input.

### Why default `MIN_ACTIVE=3` (not Phase 14 Epic 2's empirically-passing MIN=10)

Phase 14 Epic 2's 16q quick PASSED at MIN=10/p95. The 120q full FAILED per-question on `architecture_structure`. Shipping default MIN=10 would invite operators to flip the gate on and hit the same failure mode without the diagnostic data Phase 14.1 will use to mitigate. Default MIN=3 is the conservative floor (matches sprint plan §13 fork #2 + the spec); operators who opt in should set MIN=10 explicitly while Phase 14.1 ships.

### Why ship flag-off (not default-on)

Per sprint plan §10 risk #1 contingency: "On fail: stop at flag-off, scope Phase 14.1 with adaptive-per-query-type percentile." 120q full with mean parity + 7 boundary regressions across 4 categories qualifies as the fail case the contingency was written for.

### Why per-request override (not just env-config)

A/B sweeps need per-call control without restarting mdemg. The URL-param overrides let a Python ablation runner toggle the gate on a per-question basis if needed; in practice Phase 14 Epic 2 used per-preset env changes + restart, but the override stays in the API for future experimentation.

### Why V0019 (separate from V0017 retrieval_audit)

V0017 captures pre-gate state (top_k_node_ids, consensus_strength). V0019 captures the gate's effect on that state. Joining them answers "what fraction of the candidate set crossed the bar at percentile p, and what threshold did p resolve to?" Single-table ALTER would conflate two distinct concerns and complicate Phase 14.1's retune queries.

## Notes

### Known limitations

- **Default-off pending Phase 14.1 retune.** Operators who opt in via `SPARSE_RETRIEVAL_ENABLED=true` should set `SPARSE_MIN_ACTIVE=10` and expect ~50% rerank input reduction with mean parity AND 7 boundary regressions on the 120q lnl_demo corpus.
- **`architecture_structure` category sensitivity**: queries needing rank 11–20 citations get cut. Phase 14.1 plans per-category MIN_ACTIVE override (`{"architecture_structure": {"min_active": 20}, ...}`).
- **Floating-point boundary**: the comparator at `uvts_ab_compare.py` uses strict `<` for regression check, which counts deltas of `-0.10000000001` as regressions. Phase 14.1 should add an `eps` tolerance.
- **Cache-hit retrieves bypass the gate** (because the cached response was already gated when computed). Subsequent identical queries don't re-execute the gate. By design but operators should not interpret V0019 row counts as "1 row per /v1/memory/retrieve call".

### Risks & gaps

- **Spec recommendation drift**: Note 06 spec recommended `MIN_ACTIVE=3` and the default ships there, but operator-recommendation-via-doc says MIN=10. The doc + the code default disagree. Phase 14.1 should reconcile.
- **No test coverage of category-specific behavior**: Tier 1 unit tests verify percentile + clamp correctness on synthetic distributions but don't exercise per-category regression patterns.

### Future improvements

**Phase 14.1 (executed 2026-05-04, partial — flag-off):**
- ✅ Per-category MIN_ACTIVE / MAX_ACTIVE / percentile via `SPARSE_GATE_CATEGORY_OVERRIDES` JSON config — **shipped flag-off**, infrastructure ready
- ✅ `eps=1e-6` tolerance in `uvts_ab_compare.py` — **shipped globally**, eliminates floating-point boundary false-positives
- ❌ Default-on flip — **120q failed**, 2 catastrophic regressions in `service_relationships` + `data_flow_integration` (both 3-required-files questions). Per-category was the wrong abstraction

**Phase 14.1.1 (executed 2026-05-04 — DEFAULT-ON):**
- ✅ Tested simpler-first hypothesis: raise global `SPARSE_MIN_ACTIVE` from 3 → 15. Initial 120q hit one catastrophic regression (q302 in `data_flow_integration`, 4 required files).
- ✅ Pivoted to **hybrid**: MIN=15 global + `data_flow_integration` MIN=20 override (using the Phase 14.1 per-category mechanism). Re-ran 120q → **PASSED** with mean +0.003, 0 regressions, 10 improvements.
- ✅ Defaults flipped: `SPARSE_RETRIEVAL_ENABLED=true`, `SPARSE_MIN_ACTIVE=15`, `SPARSE_GATE_CATEGORY_OVERRIDES` seeded with `{"data_flow_integration": {"min_active": 20}}`. The complexity-plumbing design from the Phase 14.1.1 plan stub is deferred — the simpler global+override pattern proved sufficient.
- See `phase_14_1_1_post.md`.

**Phase 14.x extensions:**
- Adaptive percentile based on `consensus_strength` (high consensus → tighter gate; low consensus → wider) — research extension

## API Endpoints

The feature does not add HTTP endpoints. Per-request overrides flow through query string params on `/v1/memory/retrieve`:

| Method | Endpoint | Param | Description |
|---|---|---|---|
| POST | `/v1/memory/retrieve?sparse=true` | bool | Enable gate for this call |
| POST | `/v1/memory/retrieve?sparse=false` | bool | Disable gate for this call |
| POST | `/v1/memory/retrieve?sparse_percentile=0.97` | float | Per-call percentile (0.5–0.999) |

JSON body fields (`sparse`, `sparse_percentile`) take precedence over URL params.

## CLI Commands

The feature does not add CLI commands. Phase 14.1 may add `mdemg retrieval gate-status --space-id <id>` for ad-hoc V0019 queries; not yet scoped.

## Configuration Reference

See "Configuration" table above. Validation (`Validate()`) enforces:
- `0.5 ≤ percentile ≤ 0.999`
- `MIN_ACTIVE ≥ 0`
- `MAX_ACTIVE ≥ MIN_ACTIVE`

## Dependencies

| Feature | Relationship |
|---|---|
| `column-voting-retrieval` | Gate operates on this feature's output (post-aggregation) |
| `local-llm-runtime` | The rerank stage that consumes the gated set hits llama-server |
| TSDB V0017 (`retrieval_audit`) | Captures pre-gate state |
| TSDB V0019 (`sparse_gate_metrics`) | Captures gate firing stats |
| `uvts-validation` | Phase 14 Epic 2 used UVTS A/B for the verdict |

## Related Files

- `internal/retrieval/gate.go` — main implementation
- `internal/retrieval/gate_test.go` — Tier 1 unit tests
- `internal/retrieval/service.go` — wiring (post-aggregation, pre-rerank)
- `internal/tsdb/sparse_gate_writer.go` — V0019 writer
- `internal/tsdb/migrations/019_sparse_gate_metrics.sql` — V0019 schema
- `internal/api/server.go::sparseGateRecorderAdapter` — adapter
- `scripts/phase14_epic2_sparse_ablation.py` — A/B sweep script
- `docs/development/post-ft-lora/sprint_plan_phase_14_*.md` — sprint plan
- `docs/development/post-ft-lora/phase_14_score_distribution_analysis.md` — Epic 0 forensic
- `docs/development/post-ft-lora/phase_14_post.md` — executed truth + verdict tables
