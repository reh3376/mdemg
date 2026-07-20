# Sprint FT-BENCH-REFRESH-001 — Re-run benchmark against current GGUF endpoint + wire staleness detection

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | FT-BENCH-REFRESH-001 |
| Sprint Name | Re-run benchmark against current GGUF endpoint + wire staleness detection |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Base | `main` |
| Format Version | Sprint plan v1.0 (12-section) |
| Estimated Effort | 0.5 dev-day (benchmark run wall-time may add 30-60 min, but that's automated) |
| Sprint Line | ft-bench-refresh-001 |
| Skill anchor | `skill:sprint-planning` |
| Parent scope | Stale-data finding from DASHBOARD-TRUTH-002 triage (2026-07-20) |

## 2. Problem Statement

The MDEMG Fine-Tuning Pipeline dashboard's "Latest Run — Task Scores" panel shows `consulting.classify.mean_score = 0.7667` — a value that has been static for **87 days**. Triage confirmed:

- Sole `benchmark_runs` row is `q283a23bz59mrg6faxo32ydx2` from **2026-04-24**.
- Model referenced: `.local-models/qwen3-14b-mdemg-v1` — the MLX form, **PRE-Phase-13.5 cutover** (Phase 13.5 shipped 2026-05-03, cut over to GGUF Q5_K_M on llama-server:8102).
- The panel's `ORDER BY started_at DESC LIMIT 1` will always return this same 87-day-old row until a new benchmark runs.
- Nothing schedules `run_benchmark` today. `ftloop.controller_stages.stageGate` is the only invocation site, and FT-RECURSIVE-002 is default-off.

**Impact**: operators believe they're looking at current model quality; they're looking at April numbers on a model that hasn't served a query since May. This is the same class as CONFIG-LOCAL-DEFAULTS-001's "stale binary reads current config" — reading old data as if it's current is worse than reading no data.

## 3. Scope & Constraints

**In scope**:
- One fresh `python -m neural.benchmarks.run_benchmark` run against the current GGUF endpoint (`http://127.0.0.1:8102/v1`, model `mdemg-llm-v1`) on `training_data/eval/valid_clean.jsonl` (180 rows, 9 tasks, 0% leakage — the Phase 11.5c leak-audited eval).
- Persist to TSDB `benchmark_runs` via `--persist-tsdb`.
- Cross-check dashboard now reads live current values.
- **Wire staleness detection**: gauge `mdemg_ft_bench_last_run_age_seconds{model_ref, task}` + evaluator rule `ft_benchmark_stale` (fires HIGH when age > `FT_BENCH_STALENESS_DAYS` default 7).
- **Panel enhancement**: add a `started_at` display to the "Latest Run" panel so operators immediately see the freshness.
- Document how to re-run the benchmark + where the results land.

**Out of scope**:
- Wiring an automatic scheduler for the benchmark — that's FT-RECURSIVE-002/003 scope (already spec'd, default-off by design pending operator opt-in).
- Fixing the `hidden.reclassify` vestigial-key artifact — that's DASHBOARD-TRUTH-002 E8; this sprint may run AFTER E8 lands so the refreshed benchmark reflects the fix.
- Re-training the model itself.

**Constraints**:
- **No hardcoded values.** Staleness threshold is a config knob.
- **Benchmark run must be against production settings** — same temperature, same max_tokens, same prompt template as `llama-server` production config.
- **Coordinate with DASHBOARD-TRUTH-002 E8** — if E8 lands before this sprint's benchmark run, the refreshed row will already include the vestigial-key fix (best); otherwise the refreshed run still shows 0.5 for hidden.reclassify (documented as "wait for E8 + re-run once").
- **NEURAL-RERANK-QUALITY-AB-001** ship note: A/B verdict was parity within noise; the benchmark run here does NOT need to also cover neural-rerank — separate concern.

## 4. Dependencies & Pre-Conditions

- ✅ `llama-server` UP on `127.0.0.1:8102` serving `mdemg-llm-v1` (Q5_K_M).
- ✅ `training_data/eval/valid_clean.jsonl` present with SHAs.
- ✅ `benchmark_runs` + `benchmark_results` tables exist (TSDB V0012).
- ✅ `neural.benchmarks.run_benchmark` runs cleanly (Phase 11.5d Epic 4 fix — `--rows-per-spec 0` iterates all matched rows).
- 🔗 **Coordination**: DASHBOARD-TRUTH-002 E8 (hidden.reclassify vestigial reward key filter) — decide whether this sprint runs before or after; recommend AFTER so the refreshed row includes the fix.

## 5. Implementation Plan

Sequential — never parallelize.

### E0 — Sprint plan
Commit this plan.

### E1 — Preflight
- Verify `llama-server` UP + serving `mdemg-llm-v1`: `curl -s http://127.0.0.1:8102/v1/models`.
- Verify `training_data/eval/valid_clean.jsonl` present + SHA matches manifest.
- Verify `benchmark_runs` writable: run a trivial `--dry-run` if the flag exists, or query the table.
- Verify `mdemg-timescaledb-1` UP.

**Gate**: preflight report in `docs/development/ft-bench-refresh-001/preflight.md`.

### E2 — Run the benchmark
```bash
cd /Users/reh3376/mdemg
python -m neural.benchmarks.run_benchmark \
  --config configs/benchmark_phase10.yaml \
  --eval valid_clean.jsonl \
  --rows-per-spec 0 \
  --mlx-timeout-s 300 \
  --persist-tsdb \
  --out training_data/eval/benchmark_ft_bench_refresh_001_<timestamp>.json
```
Expected wall-time: ~30-60 min on the current GGUF endpoint.
Capture the run_id.

**Gate**: `benchmark_runs` has a new row with fresh `started_at`.

### E3 — Cross-check via dashboard
Reload Grafana. Open MDEMG Fine-Tuning Pipeline. "Latest Run — Task Scores" panel should now display the new run's numbers. Capture before/after in `live_verification.md`.

**Gate**: dashboard shows fresh values.

### E4 — Staleness detection wiring
- Emit gauge `mdemg_ft_bench_last_run_age_seconds{model_ref, task}` via a new lightweight probe (called from `internal/api/server.go` supervised goroutine at intervals of `FT_BENCH_STALENESS_PROBE_INTERVAL_SEC` default 3600).
- Register evaluator rule `ft_benchmark_stale` in `internal/tsdb/alert_rules_ft.go` (or nearest existing file); fires HIGH severity when `age_seconds > FT_BENCH_STALENESS_DAYS * 86400` (default 7 days).
- Distinct `Service: "ft-benchmark"` label (NOSILENT-001 cooldown-key contract).
- Idle-safe SQL: `MAX(started_at) + COALESCE(...)`; NEVER `ORDER BY started_at DESC LIMIT 1` (TSDB-CONSUME-001 contract).
- Config: `FT_BENCH_STALENESS_DAYS` (7), `FT_BENCH_STALENESS_PROBE_INTERVAL_SEC` (3600).

**Gate**: unit test + pin test coverage (rules-no-LIMIT-1 + distinct-Service pins already scan the whole rule set, so this rule is auto-covered).

### E5 — Panel enhancement
Edit `deploy/docker/grafana/dashboards/mdemg-ft-training.json` — add a `started_at` timestamp display to the "Latest Run — Task Scores" panel, OR a companion stat panel showing "Age of latest benchmark run" reading the new gauge.

**Gate**: operator can see freshness at a glance.

### E6 — Live Tier-3
- Verify gauge emitted: `docker exec mdemg-timescaledb-1 psql -U mdemg -d mdemg_metrics -c "SELECT * FROM metric_samples WHERE metric_name='mdemg_ft_bench_last_run_age_seconds' ORDER BY time DESC LIMIT 5;"`
- Simulate staleness by overriding `FT_BENCH_STALENESS_DAYS=0` temporarily → alert rule fires → then restore + verify alert clears on next probe.
- Grafana panel visible refresh.

**Gate**: full pipeline (run → persist → gauge → rule → alert) live-verified.

### E7 — Canonical docs
- CHANGELOG [Unreleased] > Fixed + Added entries.
- CLAUDE.md architecture note.
- `docs/features/ft-benchmark-freshness.md` (new): why it matters, how to re-run, thresholds.
- Sprint post.

## 6. Testing Plan (3 tiers)

**Tier 1 — Unit**: staleness rule generator returns correct SQL + threshold formula given config; alert-rule pin tests (already scan whole set) automatically cover.

**Tier 2 — Integration**: run-benchmark integration test would need llama-server; ship a Go-level TSDB writer test that inserts a synthetic-old row and verifies the staleness rule SQL returns the expected verdict.

**Tier 3 — Live E2E**: E6's full pipeline test. Capture live values + rule fire/clear behavior.

## 7. Commit Strategy

1. `docs(ft-bench-refresh-001): E0 — sprint plan`
2. `docs(ft-bench-refresh-001): E1 — preflight`
3. `chore(ft-bench-refresh-001): E2 — fresh benchmark run against current GGUF endpoint`
4. `docs(ft-bench-refresh-001): E3 — dashboard verification`
5. `feat(ft-bench-refresh-001): E4 — staleness gauge + alert rule`
6. `feat(ft-bench-refresh-001): E5 — freshness display in FT dashboard panel`
7. `docs(ft-bench-refresh-001): E6 — live Tier-3 verification`
8. `docs(ft-bench-refresh-001): E7 — CHANGELOG + CLAUDE.md + feature doc + sprint post`

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./...` 0 issues
- [ ] `go test ./...` clean (staleness rule pin tests green)
- [ ] Fresh benchmark row landed in `benchmark_runs`
- [ ] Grafana dashboard shows fresh values + freshness indicator
- [ ] Staleness rule fires under simulated old data + clears under fresh
- [ ] CHANGELOG + CLAUDE.md + feature doc + sprint post committed
- [ ] Pushed; auto-PR created

## 9. Documentation Update (Epic E7 — never cut)

- **CHANGELOG.md** [Unreleased] > Fixed (stale data disclosure) + Added (staleness gauge + rule).
- **CLAUDE.md**: new architectural note — "Any dashboard reading `benchmark_runs` MUST also expose freshness. Stale-data disguised as current is worse than no-data."
- **Feature doc**: `docs/features/ft-benchmark-freshness.md`.
- **Sprint post** with the actual before/after mean_score values, the run_id, and any surprises from re-benchmarking against the new endpoint.

## 10. Risks & Mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| Fresh benchmark reveals model regression vs 87-day-old numbers | Medium | This is INFORMATION not a defect; if it happens, disclose loudly in the post + open a separate investigation sprint |
| llama-server slow / unstable during benchmark run | Low | Use `--mlx-timeout-s 300` (name is legacy — applies to all backends via preflight probe); if degraded, pause the benchmark, investigate LLM health via `mdemg watchdog status` |
| Coordination with DASHBOARD-TRUTH-002 E8 mistimed | Low | Explicit sequencing: recommend DASHBOARD-TRUTH-002 land first, then this sprint's E2 |
| Staleness rule fires too aggressively (7 days may be too tight for manual-run workflows) | Low | Config-tunable via `FT_BENCH_STALENESS_DAYS`; default calibrated to weekly cadence; operator can raise |
| Panel edit conflicts with DASHBOARD-TRUTH-002 E4/E9/E10 (same file) | Medium | Merge order matters; git will surface — resolve trivially since edits target different panels |

## 11. Rollback Procedures

- **Data**: fresh benchmark row is additive; safe.
- **Code**: revert per-commit; the staleness rule is safe to revert (won't leave stale alerts).
- **Config**: `FT_BENCH_STALENESS_DAYS=0` (or very large) effectively disables the rule without code change.

## 12. Documents Accessed

- DASHBOARD-TRUTH-002 triage report (this session)
- CLAUDE.md § Local LLM Runtime (Phase 13.5), § FT-CLASSIFY-002, § FT-RECURSIVE-001/002
- `neural/benchmarks/run_benchmark.py`
- `configs/benchmark_phase10.yaml`
- `training_data/eval/valid_clean.jsonl` + manifest
- `docs/tests/ults/specs/*.ults.json` (17 tasks)
- `docs/tests/ubench/specs/mdemg.ubench.json` (referenced but not used here — UBENCH is the wrapper, this sprint runs the base)
- `internal/tsdb/dataset_builder.go` (persistence path)
- `deploy/docker/grafana/dashboards/mdemg-ft-training.json`
- `internal/ape/alert_rules_*.go` (rule registry pattern for NOSILENT-001-compliant rule)
