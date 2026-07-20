# GRAFANA-PANEL-FILTER-001 — Sprint Post

**Shipped:** 2026-07-20 | **Branch:** `reh3376_dev01` | **PR:** (pending push)

## What shipped

Closes the disclosed follow-up from LLM-HEALTH-INVESTIGATION-001 E3. That sprint added the `NOT LIKE 'caller_canceled:%'` filter to the ALERT-rule signal so `rsic-alert_llm_health` stopped false-firing — but the Grafana panel showing operators the LLM error rate was NOT updated, so the number they saw stayed at 10.84% on `retrieval.rerank_cross` even after the alert stopped firing.

This sprint applies the same filter to the panel AND adds a pin test that enforces the contract for any future `llm_interactions.error` panel.

## Epics

- **E0** — Sprint plan (skill:sprint-planning v1.0 12-section format). Commit `40d55fa` (bundled with 4 sibling sprints from the DASHBOARD-TRUTH-002 sweep).
- **E1** — Dashboard audit. Grep swept `deploy/docker/grafana/dashboards/*.json` for `error IS NOT NULL`, `length(error)`, `error != ''`, `error <> ''`. Found: exactly 1 must-fix (`mdemg-llm-routing.json:80`); 7 safe panels doing non-error math on `llm_interactions`; 0 ambiguous; 0 pre-existing `caller_canceled` references. Commit `c51da75`.
- **E2** — Panel fix. Applied `AND error NOT LIKE 'caller_canceled:%'` to both `CASE` predicates in `mdemg-llm-routing.json:80`. Added inline SQL comment referencing `dataset_builder.go::LLMPerformance` as source-of-truth. Updated panel description to explain the exclusion. JSON validated. Commit `d77cf0a`.
- **E3** — Pin test. New `internal/grafanapin/dashboards_test.go` walks every dashboard JSON (scanned 164 panel targets across the shipped dashboards; 1 matched the `llm_interactions.error` aggregate pattern). Contract-liveness check fails if ZERO panels match (regex regression guard). Whitelist mechanism via inline comment. Negative pin test verifies the walker's regex catches unfiltered SQL. Sanity-verified: FAILS on pre-fix, PASSES on fix. Commit `3c358b8`.
- **E4** — Live Tier-3. SQL cross-check on `mdemg-dev` 24h: `retrieval.rerank_cross` raw 10.84% (27 caller_canceled rows) → honest 0.00% (0 real errors filtered/lost). `mdemg-grafana-1` restarted; loaded dashboard file contains the filter + contract markers; `/api/health` database:ok. Commit `44776a3`.
- **E5** — Canonical docs. CHANGELOG [Unreleased] > Fixed entry; CLAUDE.md LLM-HEALTH-INVESTIGATION-001 note extended in place with the dashboard-filter closure + the "pin test enforces this contract" rule; `docs/features/observability-dashboards.md` gains a "Filter contract: LLM error-rate panels" section.

## Surprise fix — separated into its own commit

Commit `911680a` — `fix(config): resolve stale merge-conflict markers in defaultValues map`.

During E3's `go build ./...` gate, discovered two unresolved `<<<<<<< / =======  / >>>>>>>` blocks in `internal/config/yaml_config.go` (lines 1075-1093) — leftover from an earlier stash-pop, not from any active sprint. They broke the whole build (syntax error → cascading `undefined: config.EffectiveConfig` for every consumer). Resolved by keeping the "Updated upstream" side (all three model defaults = `mdemg-llm-v1`), which matches the shipped CONFIG-LOCAL-DEFAULTS-001 architectural note.

Per the Phase 11.6.2 precedent ("surprise bugs during live smoke get their own fix-commit"), this was committed separately from the sprint work. Also captured 11 pre-existing cosmetic gofmt/whitespace changes that were sitting in the working tree (verified: no semantic diffs — comment reformatting, alignment padding, whitespace only).

## Live evidence

```
task_name              | calls | real_errors | raw_errors | honest_error_pct | raw_error_pct
-----------------------|-------|-------------|------------|------------------|--------------
retrieval.rerank_cross |   249 |           0 |         27 |             0.00 |         10.84
```

100% of the 24h errors are `caller_canceled:` (recorder-tagged by LLM-HEALTH-INVESTIGATION-001 E1). Zero real errors hidden by the filter.

## Pin-test enforcement

```
$ go test ./internal/grafanapin/ -v
=== RUN   TestGrafanaPanel_LLMInteractionsErrorFilter
    dashboards_test.go:143: scanned 164 panel targets across dashboards; 1 matched llm_interactions.error aggregate pattern
--- PASS: TestGrafanaPanel_LLMInteractionsErrorFilter (0.00s)
=== RUN   TestGrafanaPanel_LLMInteractionsErrorFilter_DetectsMissingFilter
--- PASS: TestGrafanaPanel_LLMInteractionsErrorFilter_DetectsMissingFilter (0.00s)
PASS
ok      mdemg/internal/grafanapin       0.307s
```

Runs under `go test ./...` → CI-guarded going forward.

## Deviations

None. Plan executed as written. One surprise fix (pre-existing merge conflicts unrelated to the sprint) surfaced by the gate check and shipped in its own commit per precedent.

## Rollback

- **Data**: N/A (no data changes).
- **Code**: revert commits `d77cf0a` (panel), `3c358b8` (pin test), `44776a3` (docs), `c51da75` (audit) individually. Reverting the pin test alone is safe; reverting the panel edit alone will make the pin test fail (correct — that's the contract).
- **Config**: none touched.

## Next up

Per the DASHBOARD-TRUTH-002 sweep queue: **DASHBOARD-TRUTH-002** — batched 9 dashboard/measurement artifact fixes across RSIC / J17 / Jiminy / FT-Training. Larger sprint (~1.5-2 dev-days).
