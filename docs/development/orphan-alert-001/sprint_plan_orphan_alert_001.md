# Sprint ORPHAN-ALERT-001 — fix false orphan alerts + is_archived-blind RSIC ratio

## 1. Header & Metadata
- **Sprint ID**: ORPHAN-ALERT-001
- **Sprint line**: `docs/development/orphan-alert-001/`
- **Date opened**: 2026-06-26
- **Target version**: v0.11.x (alert/metric correctness)
- **Estimated effort**: ~0.5 dev-day
- **OpenAI spend**: $0
- **Risk level**: Low (alert-rule SQL + a read-only RSIC query; no data mutation)

## 2. Problem Statement
The graph-health **"High Orphan Ratio"** alert fires persistently — but on **1-node UATS/test spaces** (`uats-correct-test` 1/1 = 1.0, `global` 2/2 = 1.0), not mdemg-dev (693/83034 = **0.8%**, healthy). Root causes, three distinct:

1. **`high_orphan_ratio` rule** (`alert/rules.go:91`, threshold 0.10) does `ORDER BY ratio DESC LIMIT 1` with **no minimum-node floor** — any tiny space where the single node is an orphan yields ratio 1.0 and trips it. (The `ORDER BY … LIMIT 1` anti-pattern TSDB-CONSUME-001 warned about.)
2. **`high_orphan_count` rule** (`alert/rules.go:75`, threshold 50) does `ORDER BY time DESC LIMIT 1` — it reads *whichever space's gauge was written last*, non-deterministic across spaces, and 50 is below mdemg-dev's accepted ~693 historical-orphan baseline.
3. **RSIC self-assess orphan query** (`self_assess.go:297`) is **`is_archived`-blind** — it counts the 4,457 archived tombstones as orphans → `OrphanRatio` 6.2% (vs true live 1.0%), inflating the RSIC health computation. Same class as HIDDEN-CHURN-001's archived-blind fetch. (The `mdemg_neo4j_graph_orphans` gauge already excludes archived — 693 — and is correct.)

The 693 live orphans are old unclustered `conversation_observation`s (NULL `created_at_ms`; the accepted historical orphans per EVENTGRAPH-004) — **not a data-integrity problem**; this sprint is purely alert/metric correctness.

## 3. Scope & Constraints
**In scope:** make both orphan alert rules deterministic + significance-gated + config-driven; fix the RSIC orphan query to exclude archived (numerator AND denominator).
**Out of scope:** clustering/backfilling the 693 live orphans (accepted historical per EVENTGRAPH-004; no synthetic backfill); the broader RSIC health reweighting.
**Constraints:** no hardcoding (thresholds + floor become config); 3 testing tiers + live Tier-3; docs final.

## 4. Dependencies
- The `mdemg_neo4j_graph_orphans` + `mdemg_neo4j_graph_nodes` per-space gauges (correct, archived-excluding) — already emitted.

## 5. Implementation Plan
**Epic 0 — Plan** (this doc).

**Epic 1 — Parameterize the orphan rules.** Extract `high_orphan_count` + `high_orphan_ratio` out of `DefaultRules()` into `alert.OrphanRules(minNodes int, ratioThreshold float64, countThreshold int)` (the established parameterized-rule pattern, appended in `serve.go`). Both rules:
- join the orphans + nodes gauges per space, **filter `total_nodes >= minNodes`** (significance floor),
- aggregate with `COALESCE(MAX(...), 0)` so they always return one non-NULL row (idle-safe; no `ORDER BY … LIMIT 1`).
New config: `ORPHAN_RATIO_MIN_NODES` (50), `ORPHAN_RATIO_THRESHOLD` (0.10), `ORPHAN_COUNT_THRESHOLD` (1000 — above the accepted historical baseline; the ratio rule is the scale-aware primary signal).

**Epic 2 — Fix the RSIC orphan query.** `self_assess.go` orphan Cypher: add `AND NOT coalesce(n.is_archived,false)` to the orphan match, and exclude archived from `total` too, so `OrphanRatio = live_orphans / live_nodes`.

**Epic 3 — Testing (3 tiers).**
- Tier 1: rule SQL builder unit (floor present, COALESCE/MAX, threshold wired); RSIC query string asserts `is_archived` exclusion.
- Tier 2: live SQL execution of both rules against TSDB returns mdemg-dev-dominated, non-firing values; UATS unaffected.
- Tier 3: confirm "High Orphan Ratio"/"High Orphan Count" stop firing on the 1-node spaces; mdemg-dev ratio 0.8% < 0.10; RSIC OrphanRatio drops 6.2%→~1.0%.

**Epic 4 — Docs.** Feature note (orphan alert semantics), CLAUDE.md note, CHANGELOG, post.

## 6. Testing Plan (3 tiers)
- **Tier 1:** `go test ./internal/alert/ ./internal/ape/ ./internal/config/`; `verify_config_consumers.py`.
- **Tier 2:** execute the new rule SQL live against TSDB — returns the true max-among-significant-spaces ratio/count (mdemg-dev, sub-threshold); confirm tiny spaces excluded.
- **Tier 3:** live alert evaluator no longer emits the orphan false positives; RSIC self-assess OrphanRatio reflects live ratio.

## 7. Commit Strategy
Single sprint commit on `reh3376_dev01` (rules + serve wiring + config + RSIC query + tests + docs). Push → auto-PR.

## 8. Verification Checklist
- [ ] `OrphanRules(...)` with min-node floor + COALESCE/MAX (no ORDER BY … LIMIT 1)
- [ ] 3 config fields added; consumer guard green
- [ ] RSIC orphan query excludes archived (numerator + denominator)
- [ ] live: orphan false alerts cease; mdemg-dev sub-threshold; RSIC ratio ~1.0%
- [ ] build + lint clean; alert/ape/config tests pass
- [ ] CLAUDE.md + CHANGELOG + post updated

## 9. Documentation Update — Epic 4
## 10. Risks & Mitigations
| Risk | L | I | Mitigation |
|---|---|---|---|
| Floor hides a real small-space orphan problem | Low | Low | 50-node floor is config (`ORPHAN_RATIO_MIN_NODES`); small spaces are test/scratch, not production substrate |
| Count threshold 1000 bakes in mdemg-dev's current state | Low | Low | Config-driven; the ratio rule (scale-aware) is the primary signal; documented |
| RSIC ratio change shifts health score | Low | Low | The change makes it *more correct* (live ratio); health weights already normalised (DH-005) |

## 11. Documents Accessed
- `internal/alert/rules.go`, `internal/cli/serve.go`, `internal/ape/self_assess.go`, `internal/ape/self_reflect.go`
- `internal/api/server.go` (gauge query — the correct archived-excluding reference)
- `internal/config/config.go`
- Live `metric_samples` TSDB + Neo4j mdemg-dev graph

## 12. Rollback Procedures
- Revert the commit; the rules return to `DefaultRules()` hardcoded form. No data migration involved.
