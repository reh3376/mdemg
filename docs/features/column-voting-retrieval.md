---
created: 2026-05-04
updated: 2026-05-04
version: v0.6.0
author: reh3376
status: active
phase: phase 13 + phase 13.1
---

# Column-Voting Retrieval (RRF + `consensus_strength`)

## Summary

**Feature**: `column-voting-retrieval`
**Summary**: Reciprocal Rank Fusion (RRF) aggregator that runs four retrieval columns in parallel (Embedding + BM25 + Graph + Structural; a 5th Context column joined in Phase 14.2, default-on since 14.2.3 — see `context-fingerprinting.md`), fuses their per-column rankings into a single ranked list, and emits a `consensus_strength` per-call signal alongside the candidate set. Replaces the legacy linear scorer at the call site behind a feature flag (`RETRIEVAL_COLUMN_VOTING_ENABLED`, default `true` since Phase 13.1). Cache namespace isolated per weight preset via a hash-based scorer-version key. Retrieval audit rows persist to TSDB V0017.

## Vision & Goals

The MDEMG vision frames retrieval as the connection layer that surfaces relevant memories so developers make better-informed decisions. Phase 13 was triggered by two structural problems with the legacy linear scorer (`scoring.go:797` pre-Phase-13):

1. **Weight tuning was fragile.** The legacy formula combined ~12 signals (vector + BM25 + activation + recency + confidence + path-boost + comparison-boost + L1 boost + bypass + hub penalty + redundancy + stale) with empirically-tuned weights. Any change to one signal could subtly degrade another in non-obvious ways. There was no honest way to validate weight changes short of an A/B against real users.
2. **No uncertainty signal.** The scorer returned a final ranked list with per-candidate scores — no per-call signal of how much the underlying signals agreed. Downstream extensions (Notes 05–06) needed an uncertainty input.

Phase 13 adopted the HTM-aligned column-voting pattern: each column produces its own ranking based on a single signal kind, then a deterministic RRF aggregator fuses them. RRF (Cormack et al. 2009) is `score(node) = Σ_c (weight_c / (k + rank_c(node)))` with `k=60`, a result-quality property that's robust to score-scale differences across columns.

`consensus_strength = (columns_containing_node / total_columns_queried) × avg(normalized_rank)` is the per-call uncertainty output. High consensus = many columns surfaced the same nodes near the top; low consensus = columns disagreed. Note 05 and Note 06 (Phase 14) consume this signal.

## Current State

### Architecture

| Component | Path | Role |
|---|---|---|
| Column interface | `internal/retrieval/columns/types.go` | `Column.Run(ctx, query) ([]Result, error)` contract |
| Embedding column | `internal/retrieval/columns/embedding.go` | Wraps existing vector recall |
| BM25 column | `internal/retrieval/columns/bm25.go` | Wraps existing fulltext search |
| Graph column | `internal/retrieval/columns/graph.go` | Wraps spreading activation |
| Structural column | `internal/retrieval/columns/structural.go` | Cypher walk over `contains` / `defined_in` edges, configurable hop depth |
| RRF aggregator | `internal/retrieval/consensus.go` | Parallel column execution via `errgroup` with per-column timeout, RRF formula, `consensus_strength` |
| Scorer fork | `internal/retrieval/service.go:644-682` | At `service.Retrieve`: chooses linear scorer vs RRF based on `cfg.RetrievalColumnVotingEnabled` |
| Cache namespace | `internal/retrieval/service.go::scorerVersion()` | Returns `"v0-linear"` or `"v2-rrf5|e=0.500|b=0.200|g=0.150|s=0.150|c=0.100|hops=2|emb=true|bm=true|gr=true|st=true|ctx=true|strict=0.250|catmaps=<hash>|tge=on|…"` — weight/hop/enable/typed-edges changes flip cache namespace automatically |
| Retrieval audit | `internal/retrieval/retrieval_audit.go` + `internal/tsdb/retrieval_audit_writer.go` | V0017 hypertable; one row per retrieve call when `RETRIEVAL_AUDIT_ENABLED=true` |
| Ablation runner | `scripts/phase13_1_ablation_runner.py` | Sweeps weight presets; restarts mdemg per preset; runs UVTS A/B; produces verdict matrix |

The two columns originally specified for Phase 13 (`Temporal` and `RoleScoped`) were **deferred per Epic 0 data audit**: `whk-wms` MemoryNodes had >5% null `last_activated_at` and 0 distinct `role` properties. Shipping those columns without populated data would produce noise. The 4-column variant shipped in production (the Context column later made it 5 — Phase 14.2.3).

### Workflow

1. Request arrives at `/v1/memory/retrieve`
2. `service.Retrieve` runs candidate gathering (vector + BM25 + graph), feeds into the scorer
3. If `cfg.RetrievalColumnVotingEnabled=true` (current production default):
   - `Service.ScoreAndRankRRF` runs 3 virtual columns over the candidate set (Embedding/BM25/Graph as presorted views) plus a true-parallel Cypher walk for Structural
   - `consensus.Aggregate` fuses with RRF formula using per-column weights from config
   - Returns ranked list + `ConsensusResult` with per-column latency + `AggregateConsensus`
4. Cache key incorporates `scorerVersion()` → weight/hop/enable changes don't serve stale entries from a different config
5. If `RETRIEVAL_AUDIT_ENABLED=true`, one row lands in V0017 with `consensus_strength`, per-column latency, top-K node IDs
6. Prometheus metrics emit per call: `mdemg_retrieval_consensus_strength`, `mdemg_retrieval_column_latency_seconds{column}`, `mdemg_retrieval_column_failed_total{column,reason}`

### Phase 13.1 — Embedding-Heavy Default

Phase 13's initial commit shipped equal weights `0.25 / 0.25 / 0.25 / 0.25` and **failed full 120q A/B** (mean parity but 2 catastrophic regressions on q 69 — `secretsManager.module + Azure Key Vault` — and `hard_sym_4`). Phase 13.1 forensic diagnosis (`docs/development/post-ft-lora/phase_13_1_forensic_diagnosis.md`) traced the root cause: at equal weights, Graph + Structural columns over-voted on precise-symbol queries, displacing the correct file the Embedding column had ranked correctly.

The 13.1 ablation runner swept five weight presets through 16q quick + 120q full A/B. **Embedding-heavy `0.50 / 0.20 / 0.15 / 0.15`** won:
- 120q full: mean +0.023 (+5.9%), 30 improvements, 2 boundary regressions in `business_logic_constraints` (per-question fails by exactly -0.10 boundary)
- q 69 + hard_sym_4 catastrophic regressions eliminated

The default flipped to embedding-heavy in the same Phase 13.1 commit (`6ed411e`, merged in PR #365). `RETRIEVAL_COLUMN_VOTING_ENABLED` flipped from `false` to `true`.

### Configuration

| Env Var | Default | Description |
|---|---|---|
| `RETRIEVAL_COLUMN_VOTING_ENABLED` | `true` | Master toggle. `false` reverts to legacy linear scorer; cache namespace flips automatically |
| `RETRIEVAL_RRF_K` | `60` | RRF constant in `score = w / (k + rank)` (Cormack et al. default) |
| `RETRIEVAL_STRUCTURAL_HOPS` | `2` | Cypher walk depth for Structural column |
| `RETRIEVAL_COLUMN_TIMEOUT_FRACTION` | `0.8` | Per-column ctx-timeout as fraction of total retrieve budget |
| `RETRIEVAL_COLUMN_WEIGHT_EMBEDDING` | `0.50` | RRF weight on Embedding column (Phase 13.1 default) |
| `RETRIEVAL_COLUMN_WEIGHT_BM25` | `0.20` | RRF weight on BM25 column |
| `RETRIEVAL_COLUMN_WEIGHT_GRAPH` | `0.15` | RRF weight on Graph column |
| `RETRIEVAL_COLUMN_WEIGHT_STRUCTURAL` | `0.15` | RRF weight on Structural column |
| `RETRIEVAL_COLUMN_EMBEDDING_ENABLED` | `true` | Per-column suppression knob |
| `RETRIEVAL_COLUMN_BM25_ENABLED` | `true` | Per-column suppression knob |
| `RETRIEVAL_COLUMN_GRAPH_ENABLED` | `true` | Per-column suppression knob |
| `RETRIEVAL_COLUMN_STRUCTURAL_ENABLED` | `true` | Per-column suppression knob |
| `RETRIEVAL_AUDIT_ENABLED` | `true` | Write V0017 row per retrieve call (default `true` since TSDB-CONSUME-001 — feeds the scorer-drift tripwires) |

## Choices that were made

### Why RRF (not weighted score-sum)

RRF is rank-based; it doesn't depend on the absolute score scales of the input columns. Embedding scores live in `[0, 1]`, BM25 in `[0, ∞]`, graph activation in `[0, ~5]`. Combining them as scores would require per-column normalization that's hand-tuned and breaks when corpus statistics shift. RRF treats only ranks, so it's robust to score-scale drift.

### Why 4 columns (not the spec's 6)

Epic 0 data audit found `Temporal` and `RoleScoped` columns would feed off properties (`last_activated_at`, `role`) that were >5% null on `whk-wms`. Shipping them with sparse data would produce noise that the operator would have to tune through later. The 4-column variant uses only properties that are densely populated. The deferred columns are scoped for a future sprint (when role/timestamp population improves).

### Why default-on after Phase 13.1 (not flag-off as Phase 13 originally shipped)

Phase 13.1's 120q A/B showed +5.9% mean lift with 2 boundary regressions vs 30 improvements. Per the standing precedent (`feedback_data_decides_not_operator.md`), the data justified the default flip in the same commit. Operator opt-out remains via `RETRIEVAL_COLUMN_VOTING_ENABLED=false` for any operator who needs the legacy scorer.

### Why scorer-version cache namespace (not flag-flip cache flush)

A cache flush on every config change loses warm cache for hours after a deploy. The scorer-version hash is surgical: changing weights flips the namespace key automatically, so old (stale) entries become invisible to new (correct) lookups but the cache still serves warm entries that match the current config. Same fix protects future ranker work that happens after Phase 13.

### Why RRF k=60 (Cormack default, not tuned)

The spec's k=60 is the reference value from the original paper. Tuning it would add another A/B dimension without a clear hypothesis. Phase 13 deferred k-tuning to a follow-up sprint that hasn't been scoped — the embedding-heavy weights captured the lift available without needing to touch k.

### Why `consensus_strength` formula

The chosen formula `(columns_containing_node / total_columns_queried) × avg(normalized_rank)` produces a coarse step (1/N to N/N) that's stable to grader noise and meaningful as an uncertainty input to downstream consumers. A continuous formula (variance of per-column ranks) was considered and rejected — too sensitive to single-column outliers.

## Notes

### Known limitations

- **Jiminy compatibility**: when JiminyEnabled is true, the linear scorer's breakdown-enabled path is used because Jiminy's explanation surfaces the per-component breakdown that RRF doesn't produce. RRF + Jiminy integration is deferred to a follow-up sprint.
- **Per-category weight bias**: Phase 13.1 120q showed `business_logic_constraints` had 2 boundary regressions (per-question fail by exactly -0.10). Phase 13.2 (queued) is scoped to investigate per-category weight tuning.
- **Cache-hit retrieves bypass audit logging**: 5 retrieves → 3 audit rows in Phase 14 Epic 0 smoke. Cache hits don't change ranking so it's mostly fine, but volume metrics on V0017 should be interpreted with this in mind.

### Risks & gaps

- **Ablation runner fragility**: the runner edits `.env` + restarts mdemg per preset. A failed restart between presets leaves `.env` in a non-baseline state — the runner has a `defer-style` restore but pathological aborts (kill -9) can skip it. Always check `.env` after a runner crash.

### Future improvements

- Phase 13.2 — per-category weight tuning (queued)
- Phase 13.6 — backend-agnostic naming cleanup (queued)
- RRF + Jiminy integration

## API Endpoints

The feature does not add HTTP endpoints — it transparently replaces the scorer at `/v1/memory/retrieve`. Per-call observability lands in V0017 + Prometheus.

## CLI Commands

| Command | Description |
|---|---|
| `python3 scripts/phase13_1_ablation_runner.py --presets diagnostic-set --profile quick --baseline /tmp/uvts-baseline/grades.json` | Sweep weight presets and produce A/B verdict matrix |

## Configuration Reference

See "Configuration" table above. All knobs honor `Validate()` cross-field checks at startup (e.g. weights ≥ 0, structural hops ≥ 1).

## Dependencies

| Feature | Relationship |
|---|---|
| `uvts-validation` | Phase 13/13.1 used UVTS A/B as merge gate |
| `local-llm-runtime` | Reranker (post-aggregator) hits llama-server |
| TSDB V0017 (`retrieval_audit`) | Persistence for `consensus_strength` per-call |
| `sparse-retrieval` (Phase 14) | Note 06 gate fires post-aggregator on this feature's output |

## Related Files

- `internal/retrieval/columns/` (4 files) — column implementations
- `internal/retrieval/consensus.go` — RRF aggregator
- `internal/retrieval/scoring_rrf.go` — `Service.ScoreAndRankRRF` entry
- `internal/retrieval/service.go` — scorer fork + `scorerVersion()` cache key
- `internal/retrieval/retrieval_audit.go` — interface
- `internal/tsdb/retrieval_audit_writer.go` — V0017 writer (Phase 14 Epic 0 wired this; Phase 13 had only the schema)
- `internal/tsdb/migrations/017_retrieval_audit.sql` — V0017 schema
- `scripts/phase13_1_ablation_runner.py` — preset sweep + A/B verdict
- `docs/development/post-ft-lora/sprint_plan_phase_13_column_voting.md` — Phase 13 plan
- `docs/development/post-ft-lora/phase_13_1_post.md` — Phase 13.1 ablation outcome
- `docs/development/post-ft-lora/phase_13_1_forensic_diagnosis.md` — q 69 + q hard_sym_4 root-cause analysis
