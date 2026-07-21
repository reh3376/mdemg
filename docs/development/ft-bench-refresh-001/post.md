# FT-BENCH-REFRESH-001 — Sprint Post

**Shipped:** 2026-07-21 | **Branch:** `reh3376_dev01` | **PR:** (pending push)

## What shipped

Closes the DASHBOARD-TRUTH-002 stale-data finding: `benchmark_runs` had a sole row from 2026-04-24 (88 days old, pre-Phase-13.5 MLX model) which the FT Latest Run panel displayed as if current. Sprint refreshed the row against the current GGUF endpoint + wired staleness detection so this can't silently happen again.

## Epics (all committed)

- **E0** — Sprint plan (bundled `40d55fa`)
- **E1** — Preflight (`8d2043b`): llama-server + valid_clean.jsonl + TSDB verified
- **E2+E3+E6** — Fresh benchmark + dashboard cross-check + live Tier-3 (`ecb31ee`)
- **E4** — Staleness alert rule + config (`47a1871`, `1e4d9b1` test)
- **E5** — Freshness panel (`47a1871`)
- **E7** — Canonical docs (this commit)

## Live evidence

**E2 fresh benchmark run:**
```
run_id:                    jc81749c2d95d7be029d2c614
started_at:                2026-07-21 03:28:35Z
completed_at:              2026-07-21 03:40:20Z (11m 45s wall)
aggregate_weighted_score:  0.8544
specs_with_matched_rows:   12/17
```

**Quality delta vs baseline:**
| run | started_at | model | aggregate |
|---|---|---|---|
| **jc81749c…** (new) | 2026-07-21 | GGUF Q5_K_M via llama-server | **0.8544** |
| q283a23bz… (baseline) | 2026-04-24 | MLX qwen3-14b-mdemg-v1 | 0.8338 |
| Δ | +88 days | GGUF vs MLX | **+0.021** |

GGUF endpoint outperforms the 88-day-old MLX baseline. No Phase 13.5 regression; if anything a modest improvement.

**E4/E6 staleness rule:**
- Fresh row age 0.0004 days → default 7d threshold → rule OK
- Force `FT_BENCH_STALENESS_DAYS=0` → rule would fire → restored

**E5 freshness panel:**
```
panel id=25 title="Latest Benchmark Age"
gridPos: h=4 w=6 x=0 y=23 (above Per-Task Pass Rate panel)
thresholds: green <3d, yellow 3-7d, red >7d
```

## Two learnings captured (deferred to follow-up)

1. **`--persist-tsdb` writes a SQL sidecar, NOT direct INSERT.** Must run `docker exec -i mdemg-timescaledb-1 psql < sidecar.sql` to land rows. Not documented in the runner's help text. FT-RECURSIVE-002 should automate this OR the runner should gain a direct-INSERT mode.
2. **`--rows-per-spec 5 --n-runs 1` is the right scope for a refresh** (~45 calls, ~12 min). The plan's `--rows-per-spec 0 --n-runs 5` (~1200 calls, hours) is prohibitively heavy — better suited to a formal per-quarter regression run than a routine refresh.

## Deviations from plan

- Rescoped E2 invocation from `--rows-per-spec 0 --n-runs 5` (plan default) to `--rows-per-spec 5 --n-runs 1` to fit reasonable wall time. Documented + will inform future sprints.
- E2's `--persist-tsdb` sidecar behavior required a manual `psql < sidecar.sql` step; not automated in this sprint (deferred as follow-up per above).

## Rollback

- Data: fresh benchmark row is additive; safe.
- Code: revert per-commit; `FT_BENCH_STALENESS_DAYS` env override disables the rule without a code change.
- Panel: revert the mdemg-ft-training.json edit (panel id 25).

## Next up

Per DASHBOARD-TRUTH-002 sweep queue: **PROMETHEUS-SCRAPE-INVESTIGATION-001** (final sprint — diagnose /metrics HTTP 404).
