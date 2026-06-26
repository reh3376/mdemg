# ORPHAN-ALERT-001 — Post

## Outcome
The persistent graph-health "High Orphan Ratio" alert was a **false positive** — and the investigation surfaced three distinct defects, all alert/metric correctness (no data-integrity problem; the 693 live orphans are accepted historical unclustered `conversation_observation`s per EVENTGRAPH-004).

## The three findings
1. **`high_orphan_ratio` fired on 1-node test spaces.** The rule used `ORDER BY ratio DESC LIMIT 1` with no node floor, so `uats-correct-test` (1 orphan / 1 node = 1.0) and `global` (2/2) tripped the 0.10 threshold while mdemg-dev was healthy at **0.8%**.
2. **`high_orphan_count` was non-deterministic.** `ORDER BY time DESC LIMIT 1` read whichever space's gauge was written last; threshold 50 was below mdemg-dev's accepted ~693 baseline.
3. **RSIC self-assess orphan query was `is_archived`-blind** (`self_assess.go:297`) — counted the 4,457 archived tombstones, inflating `OrphanRatio` to **6.2%** vs the true live **1.0%** (HIDDEN-CHURN-001 class). The `mdemg_neo4j_graph_orphans` gauge already excluded archived (the "gauge=70" I first reported was my own averaging artifact across ~10 spaces; the mdemg-dev-labelled gauge is correctly 693).

## Changes
- `internal/alert/rules.go` — extracted the two orphan rules to **`OrphanRules(minNodes, ratioThreshold, countThreshold)`**: per-space orphan+node gauge join, **`total_nodes >= minNodes` significance floor**, `COALESCE(MAX(...),0)` deterministic idle-safe aggregation (no `ORDER BY … LIMIT 1`, per the TSDB-CONSUME-001 alert-SQL contract). Distinct Service labels (`graph-health-count` / `graph-health-ratio`) so they don't share a cooldown key with `low_graph_health` (NOSILENT-001 contract).
- `internal/cli/serve.go` — append `OrphanRules(cfg...)`.
- `internal/config/config.go` — `ORPHAN_RATIO_MIN_NODES` (50), `ORPHAN_RATIO_THRESHOLD` (0.10), `ORPHAN_COUNT_THRESHOLD` (1000).
- `internal/ape/self_assess.go` — orphan query now excludes `is_archived` from BOTH the total and the orphan count → live ratio.

## Testing
- **Tier 1:** `TestOrphanRules_FloorAndAggregation` (floor present, COALESCE/MAX, no LIMIT 1, distinct services, threshold + default fallbacks); `DefaultRules` count 10→8; `go test ./internal/alert/ ./internal/ape/ ./internal/config/` green; `verify_config_consumers` 729/729; lint 0.
- **Tier 2 (live SQL):** new ratio rule returns **0.0083** (< 0.10), count returns **693** (< 1000) — both sub-threshold, mdemg-dev-dominated, tiny spaces excluded.
- **Tier 3 (live):** restarted; evaluator running new rules; **no orphan alerts fire** post-restart; fixed RSIC query computes **693/68,129 = 1.02%** vs the old 6.2%.

## Notes
- The 693 live orphans remain (accepted historical per EVENTGRAPH-004 — no synthetic backfill). The count rule's 1000 default sits above this baseline; the ratio rule (scale-aware) is the primary signal.
- A genuine future orphan spike (>1000 in a ≥50-node space, or >10% live ratio) still fires correctly.

## Documents Accessed
- `internal/alert/rules.go`, `internal/cli/serve.go`, `internal/ape/self_assess.go`, `internal/api/server.go`, `internal/config/config.go`
- Live `metric_samples` TSDB + Neo4j mdemg-dev graph
