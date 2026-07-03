# Sprint DASHBOARD-TRUTH-001 — Make the RSIC / J17 / Jiminy dashboards honest

## 1. Header & Metadata

- **Sprint ID:** DASHBOARD-TRUTH-001
- **Sprint line:** `docs/development/dashboard-truth-001/`
- **Date opened:** 2026-07-03
- **Target version:** patch (metric-honesty; no user-facing runtime API changes)
- **Estimated effort:** ~1 dev-day (7 sequential epics)
- **OpenAI spend:** $0 (execution on local Fable 5 sub-agents; no teacher calls)
- **Risk level:** Low–Medium — E2 (NLI bias) and E3 (compression anchor) change health **scores**, not just panels; disclosed, default-driven, live-verified.

## 2. Problem Statement

An operator review of three Grafana dashboards surfaced eight alarming numbers. A live Fable-5 investigation (three parallel read-only agents) reproduced every value and classified each as **ARTIFACT** (the dashboard/metric is wrong) vs **REAL-LOW** (the capability is genuinely degraded). Six of the eight are artifacts — the same LIMIT-1 / COALESCE-to-0 / stale-gauge / wrong-anchor / structurally-red-alert class that ALERT-TRUTH-001 began closing. They mask a healthy RSIC (71/71 cycle completions in 6h) and mislabel the one genuinely-low capability (Jiminy guidance quality → deferred to Sprint 2, JIMINY-CORPUS cleanup).

**Findings that this sprint fixes (all ARTIFACT):**

| Dashboard | Metric | Reproduced truth | Defect class |
|---|---|---|---|
| RSIC | Cycle Success Rate = 0% | 100% (71/71) | per-bucket ratio + `COALESCE(…,0)` + `lastNotNull` latches started-only buckets |
| J17 | NLI Mean Bias 0.61 + Bias Alert | permanently red by construction | compares NLI *comprehension* vs a *compliance* heuristic; no min-sample floor; wrong sidecar-request panel |
| J17 | Protocol 68.5% | honest but mis-anchored | hardcoded "5.0× = perfect" compression target vs achievable ~1.8–3× |
| J17 | (trust-store hygiene) | ~100 stale sessions immortal | hydration rewrites `last_update` → 168h TTL never expires them |
| Jiminy | Outcome pie / trends | contradict honest follow rate ~3× | multi-credit lifetime-cumulative gauges |
| Jiminy | Should-Follow 0.094 | 0.145 excl. n/a | panel counts `not_applicable` as 0.0 |

**Explicitly REAL-LOW (NOT this sprint — Sprint 2):** guidance follow rate ~16%, guidance dimension 0.335, T1-unreachable, Min-Trust 22% (live session converging to the `ignored` EMA anchor 0.2). Root cause = guidance quality (junk constraint corpus + repetition + Lever B off), addressed by the JIMINY-CORPUS sprint.

## 3. Scope & Constraints

**In scope:** the six ARTIFACT fixes above — dashboard-panel SQL, the NLI-bias metric redefinition + a real NLI-call counter, the compression-anchor config, trust-store hydration hygiene, and the Jiminy panel/alert-rule corrections.

**Out of scope (→ Sprint 2 JIMINY-CORPUS):** junk constraint-node purge, per-node repetition control, the outcome-classifier relevance gate (irrelevant→`not_applicable`), Lever B (`JIMINY_DIRECTIVE_SYNTHESIS_ENABLED`), HITL curation, the `RetrieveForJiminy` role_type adapter gap.

**Constraints:** sequential epics (no parallelism); docs before implementation within each epic; 3 testing tiers incl. live Tier-3; no hardcoded values (new knobs are config with sane defaults); CUIDv2 for any new ids; never leave test failures; never commit to `main`; lint before commit. E2/E3 change scores — disclose the before/after in the PR, never silently.

## 4. Dependencies

- Live stack up: mdemg :9999, TSDB `mdemg-timescaledb-1`, Neo4j `mdemg-neo4j-1`, Grafana `mdemg-grafana-1` (all verified up). NLI sidecar live on :8100 (Docker `nli-MiniLM2`) and :8101 (native `deberta-v3-xsmall`).
- Prior sprints on `main`: ALERT-TRUTH-001 (rule-family pin tests `TestAllRules_NoLimitOneAntiPattern`, `TestAllRules_DistinctServicePerSeverity`), TSDB-CONSUME-001 (idle-safe SQL contract), DH-005 (health weighting), JIMINY-EFFECTIVENESS-001 (trust EMA), JIMINY-RELEVANCE-001 (should-follow rate + alert rule).
- Investigation evidence: the three Fable-5 agent reports (summarized in §2, full detail captured in `investigation_findings.md`).

## 5. Implementation Plan (sequential epics + gates)

**E0 — Sprint plan (this doc) + investigation record.** Commit the plan + `investigation_findings.md`.

**E1 — RSIC Cycle Success Rate panel.** `deploy/docker/grafana/dashboards/mdemg-rsic.json`: replace the per-bucket success ratio with a single windowed aggregate over `mdemg_rsic_cycle_total` (`SUM(completed+dry_run)/NULLIF(SUM(all terminal),0)*100`, no `COALESCE`-to-0, no bucketing); `noValue` → "N/A". Grep the whole RSIC dashboard for sibling bucketed-ratio-with-`lastNotNull` panels; fix any found. **Gate:** live query returns ~100%; JSON is valid; provisioning reload shows the corrected value.

**E2 — J17 NLI bias metric + NLI-call observability.** `internal/jiminy/nli_calibration.go` + `service.go`: (a) redefine `MeanBias`/`BiasAlert` to compare like-for-like — exclude `ignored`-outcome samples (where NLI-comprehension vs compliance-heuristic divergence is by-design) OR gate on the heuristic claiming comprehension; (b) `J17_NLI_CALIBRATION_MIN_SAMPLES` (default 50) floor before the alert can fire; (c) `J17_NLI_CALIBRATION_BIAS_THRESHOLD` already exists — confirm config-driven, recalibrate default with rationale. Add `RecordNLICall` counter + latency gauge (`mdemg_j17_nli_requests_total`, `_latency_ms`) at the real NLI call site; relabel the dashboard "Sidecar Requests" panel to the tier-prediction client it actually measures, add an NLI-calls panel. **Decision (disclose in PR):** canonical NLI sidecar `:8100` vs `:8101`. **Gate:** bias alert clears under the current mostly-ignored regime (or fires only above the floor on a real divergence); NLI-call counter increments live.

**E3 — J17 Protocol compression anchor.** `internal/ape/self_assess.go::scoreProtocol`: extract the hardcoded `5.0` perfect-compression anchor to `J17_COMPRESSION_TARGET_RATIO` (default calibrated to the observed achievable ratio, ~2.5–3×). **Gate:** Protocol dimension recomputes to the honest value; before/after disclosed. Update the config-default test.

**E4 — Trust-store hydration hygiene.** `internal/jiminy/trust_store.go`: preserve the original `last_update` (or `last_feed_at`) through startup hydration so the 168h TTL cleanup actually expires stale test sessions; add a significance floor (min feedback count / recency) so one-shot/stale sessions don't dominate the `mdemg_j17_min_trust_score` gauge. **Gate:** stale test sessions age out (or are excluded); min-trust gauge reflects only live-significant sessions.

**E5 — Jiminy dashboard panel corrections.** `deploy/docker/grafana/dashboards/mdemg-jiminy.json`: pie + Outcome-Trends → honest windowed `constraint_outcomes`/`guidance_training_rows` counts (drop the multi-credit cumulative gauges); Should-Follow panel → exclude `not_applicable` from the denominator; the `guidance_should_follow_rate_low` alert rule (`internal/alert/rules.go`) → same n/a-exclusion; rename "Total Guidance Issued" to what it measures; fix the x=14/x=16 gridPos collision; remove panel-15 leftover field overrides. **Gate:** pie matches the honest windowed follow rate; JSON valid; rule pin tests pass.

**E6 — Testing (3 tiers).** See §6. **Gate:** all tiers green; live Tier-3 reproduces every fixed panel.

**E7 — Documentation (never cut).** `docs/features/` metric-semantics update, CLAUDE.md Architecture Note, CHANGELOG, `post.md`. **Gate:** canonical docs current.

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit):** NLI-calibration ignored-exclusion + min-sample floor (`nli_calibration_test.go`); compression-anchor config default (`config` test); trust-hydration `last_update` preservation (`trust_store_test.go`); NLI-call counter registration.
- **Tier 2 (integration/contract):** alert-rule pin tests re-run (`TestAllRules_NoLimitOneAntiPattern`, `_DistinctServicePerSeverity`); the should-follow rule's n/a-exclusion SQL is idle-safe (aggregate+COALESCE, reads `time`); dashboard JSON lint (valid JSON, datasource refs resolve).
- **Tier 3 (live — required):** against the running stack — (1) RSIC cycle-success panel query returns ~100%; (2) NLI `bias_alert` clears / min-sample floor holds, `mdemg_j17_nli_requests_total` increments on a live guidance call; (3) Protocol dimension reads the calibrated anchor value; (4) a stale test trust session ages out / is excluded from the min gauge; (5) Jiminy pie count == honest windowed `constraint_outcomes` follow count. Observe each in TSDB query / Grafana panel / `/metrics`.

## 7. Commit Strategy

One commit per epic on `reh3376_dev01` (E0 plan, E1…E5 fixes, E6 tests folded into their epics where natural, E7 docs). Push once at the end → auto-PR. Any surprise bug caught in live smoke gets its own fix-commit (Phase 11.6.2 precedent). Sprint summary added to the PR comment.

## 8. Verification Checklist

- [ ] RSIC Cycle Success panel reads ~100% live; no sibling bucketed-ratio panels remain
- [ ] NLI bias alert no longer permanently red; min-sample floor enforced; threshold config-driven
- [ ] `mdemg_j17_nli_requests_total` increments on a live NLI call; sidecar panels correctly labelled; canonical sidecar decided + disclosed
- [ ] Protocol compression anchor config-driven; before/after score disclosed
- [ ] Trust-store hydration preserves `last_update`; stale sessions expire/excluded; min-trust gauge significant-only
- [ ] Jiminy pie/trends read honest windowed sources; Should-Follow excludes n/a (panel + alert rule); "Total Guidance Issued" renamed; gridPos collision fixed; panel-15 overrides removed
- [ ] `go build ./...` clean; `golangci-lint run ./...` clean; `go test ./...` green
- [ ] All dashboard JSON valid; Grafana provisioning reload clean
- [ ] Tier-3 live checks all observed
- [ ] CLAUDE.md note + CHANGELOG + feature doc + post.md updated

## 9. Documentation Update — E7 above (canonical docs current, per-feature doc)

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| NLI-bias redefinition makes the alert never fire (over-corrects into blindness) | Medium | Medium | Keep it a real detector: min-sample floor + like-for-like comparison that CAN fire on a genuine calibration drift; add a unit test that a truly-divergent window still alerts |
| Compression-anchor recalibration hides a future real compression regression | Low | Medium | Config-driven default calibrated to observed; document the rationale; the gauge remains, only the perfect-anchor moves |
| Dashboard JSON edit breaks provisioning | Low | Low | JSON-lint each file; reload Grafana provisioning in Tier-3; keep a pre-edit copy |
| Trust-hydration change drops live trust history | Low | Medium | Only touch `last_update` provenance, never the trust value or EMA; unit test preserves value |
| Sibling artifact panels missed | Medium | Low | Grep every dashboard for the `ORDER BY … LIMIT 1` / per-bucket-ratio / `lastNotNull`-on-ratio patterns during E1/E5 |

## 11. Documents Accessed

- `deploy/docker/grafana/dashboards/{mdemg-rsic,mdemg-j17,mdemg-jiminy}.json`
- `internal/ape/{self_assess.go,self_reflect.go,cycle.go,live_collectors.go}`
- `internal/jiminy/{nli_calibration.go,nli_comprehension.go,service.go,trust_store.go,trust.go,protocol_metrics.go,encoder.go,stats.go}`
- `internal/alert/rules.go`, `internal/config/config.go`, `internal/tsdb/dataset_builder.go`
- CLAUDE.md Architecture Notes: ALERT-TRUTH-001, TSDB-CONSUME-001, DH-005, JIMINY-RELEVANCE-001, JIMINY-EFFECTIVENESS-001
- The three Fable-5 investigation agent reports (2026-07-03), captured in `investigation_findings.md`

## 12. Rollback Procedures

- **Dashboard JSON (E1/E5):** revert the file; Grafana re-provisions from git on restart.
- **Config knobs (E2/E3/E4):** all additive with sane defaults; revert the default or set the env var to restore prior behavior. NLI threshold/floor and compression anchor are env-overridable.
- **Code (E2/E4):** contained to `internal/jiminy` + `internal/ape`; revert the epic commit. No schema/data migration → no data rollback needed.
- No protected-space mutation in this sprint (that is Sprint 2).
