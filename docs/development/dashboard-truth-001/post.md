# DASHBOARD-TRUTH-001 — Post

**Status: SHIPPED — the RSIC / J17 / Jiminy dashboards now report honest numbers.** · 2026-07-03 · branch `reh3376_dev01`

An operator flagged eight alarming metrics across three dashboards. Three parallel Fable-5 read-only agents reproduced every value live and classified each ARTIFACT vs REAL-LOW. **6/8 were measurement artifacts** (the LIMIT-1 / COALESCE-to-0 / stale-gauge / wrong-anchor / structurally-red-alert class ALERT-TRUTH-001 began closing). This sprint fixed those. The one genuinely-low capability — Jiminy guidance quality (follow rate ~16%, which cascades into T1-unreachable and min-trust 22%) — is deferred to Sprint 2 (JIMINY-CORPUS).

## Shipped (6 fix-commits + plan)

1. **E1 — RSIC Cycle Success Rate = 0% → 100%.** Panel computed the success ratio *per `time_bucket`* + `COALESCE(…,0)` + `lastNotNull`; a started-only bucket latched 0. Replaced with a single windowed aggregate. Sibling fixed: Retrieval Pipeline Health gauge (empty-window masked as 0%). Live: 100%.
2. **E2 — J17 NLI bias alert was permanently red by construction.** It compared NLI *comprehension* (contradiction→1.0 "understood but violated") against a *compliance* heuristic (ignored→0.0); with ~80% ignored, bias ≫ 0.15 forever. Fix: exclude `ignored` samples (divergence by-design), min-sample floor `J17_NLI_CALIBRATION_MIN_SAMPLES` (50), config threshold, and real NLI-call observability (`mdemg_j17_nli_requests_total` / `_latency_ms` — the old `mdemg_j17_sidecar_requests` counts only the dormant tier-prediction client). A follow-up hardened it **at the source** (`Report()`) so both gauge emitters + `rsic_adapters` + the drift insight are covered by one chokepoint (ALERT-TRUTH-001 precedent). Live: 0/0, holding 65+ min.
3. **E3 — J17 Protocol compression anchor config-driven.** `scoreProtocol` hardcoded "5.0× = perfect" while real achievable compression is ~1.8–3× (30d p50 1.56, p95 2.0). Extracted to `J17_COMPRESSION_TARGET_RATIO` (default 3.0, calibrated *above* the p95 so it's not trivially met). Disclosed: compression sub-score 0.170→0.341, Protocol dimension 64.5%→68.7% on the populated window.
4. **E4 — J17 trust-store hydration rot.** Hydration stamped `LastUpdate=now()`, so the 168h TTL never expired stale sessions AND the next flush rewrote the persisted timestamp — all 116 sessions looked freshly active (115 last-fed >168h ago). Bonus: `CleanupExpired()` had zero callers. Fix: `RestoreEntry` preserves provenance; cleanup wired at load + the 30s ticker; significance floor `J17_TRUST_MIN_FEEDBACK_COUNT` (5) on the min/avg/max/count gauges. **Live: session count 116→1.**
5. **E5 — Jiminy dashboard honest sources.** The Outcome pie + Trends stacked lifetime-cumulative multi-credit gauges (0.73 vs honest 0.16 — a 3× contradiction on the same dashboard) → replaced with windowed `constraint_outcomes` counts. Should-Follow excluded `not_applicable` from the denominator (0.084→0.142; same in the `guidance_should_follow_rate_low` alert rule). Renamed "Total Guidance Issued", fixed a gridPos collision, removed stale panel overrides. Live: pie 0.168 direction-agrees; should-follow 0.142.

## Tier-3 (live, real binary + real services)

| Epic | Live observation | Verdict |
|---|---|---|
| E1 | RSIC cycle-success query → 100% | ✅ |
| E2 | `nli_bias_alert`/`mean_bias` → 0/0, held 65+ min | ✅ |
| E3 | anchor formula live + unit-tested; steady-state 0.645→0.687 (populated window); post-restart transient 0.40 (empty J17 window, expected) | ✅ |
| E4 | `trust_session_count` 116→1; min/avg/max = 0.32 (single significant session); 116 Neo4j nodes retained (in-memory cleanup, non-destructive) | ✅ |
| E5 | should-follow 0.142; pie windowed counts 203/131/1262/1 → 0.168 | ✅ |

## New config knobs (all default-sane, no-hardcoding rule)

- `J17_NLI_CALIBRATION_MIN_SAMPLES` (50; 0 disables the floor)
- `J17_COMPRESSION_TARGET_RATIO` (3.0; must be >1.0 else warn+fallback)
- `J17_TRUST_MIN_FEEDBACK_COUNT` (5; ≤0 disables the floor)

## Notes / follow-ups

- **Stale-binary lesson (recorded):** the "0.559 post-restart" that looked like an E2 residual was the *dying pre-E2 process's final recorder flushes*. When validating via `metric_samples` right after a restart, exclude rows older than the new pid's start time. (CONFIG-LOCAL-DEFAULTS-001 class.)
- **NLI sidecar drift (operator action, not code):** `.env` targets the Docker sidecar `:8100` (`nli-MiniLM2`, ~2.5× faster, compose-managed) while a native `:8101` (`deberta-v3-xsmall`) LaunchAgent runs unused. Recommendation: make `:8100` canonical, bootout/redesignate `com.mdemg.neural-sidecar`.
- **Cold-start protocol transient:** a freshly-restarted server reads low Protocol until its in-memory J17 window warms. The DH-005 data-sufficiency confidence gauge already down-weights this in overall health; a dimension-level confidence gate for J17 protocol is a possible refinement.
- **Sprint 2 (JIMINY-CORPUS):** the REAL-LOW guidance-quality work — junk constraint-node purge [authorized: backup + small-batch + sign-off], repetition control, ignored↔not_applicable classifier relevance gate, enable Lever B, HITL curation.

## Documents Accessed

- `deploy/docker/grafana/dashboards/{mdemg-rsic,mdemg-j17,mdemg-jiminy}.json`
- `internal/ape/{self_assess.go,self_reflect.go,live_collectors.go}`, `internal/jiminy/{nli_calibration.go,service.go,trust.go,trust_store.go}`, `internal/api/rsic_adapters.go`, `internal/alert/rules.go`, `internal/config/config.go`, `internal/metrics/{collectors.go,registry.go}`
- `investigation_findings.md`, `sprint_plan_dashboard_truth_001.md`
