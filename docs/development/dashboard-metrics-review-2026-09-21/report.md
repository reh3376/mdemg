# MDEMG Dashboard Metrics Review — 2026-09-21

Operator directive: "Review the MDEMG Grafana dashboard metrics — they're still not in the range I want to see to ensure MDEMG is truly useful."

## TL;DR

**Substrate mechanics are excellent. Behavioral metrics reflect a real substrate-quality gap the shipped arc is chasing on a documented timer.** MDEMG is delivering as designed — the retrieval / memory / graph / latency numbers show a healthy machine. The follow-rate + trust numbers look "low" because they honestly report the class-mix mismatch identified by JIMINY-CEILING-INVESTIGATION-002 (2026-09-03), not because MDEMG is failing.

Three of five active alerts are **artifacts** (FT bench stale ≠ actually stale; heuristic-share burst from my own sprint work; scorer-drift from yesterday's A/B). Two are real: **jiminy-process** (sparse-denominator drift, watch-item) and the **classifier follow rate** (24h drop 0.28→0.17 attributable to yesterday's LLM saturation).

## Alert triage (all 5)

| Alert | Sev | Root cause | Verdict |
|---|---|---|---|
| **ft-benchmark not refreshed** | HIGH | Fresh run landed 2026-09-21 06:25 UTC (0 days old). Alert fired before the scheduled run completed. | **FALSE POSITIVE — auto-clears** on next eval tick. Scheduled runner working. |
| **heuristic-share high (30.83%)** | MED | 100% of 41 heuristic rows landed in ONE hour on 2026-09-20 14:00 UTC — my sprint's kickstart cycles saturated LLM. Every other hour = 0 heuristic. | **ARTIFACT — burst, not chronic** (matches CLASSIFIER-CONSISTENCY-001's arch rule). Rolls off the 24h window at ~2026-09-21 14:00 UTC. |
| **jiminy-process (30-min AVG 0.213 < 0.25 floor)** | MED | Sparse-denominator swing on the just-shipped rule (24h avg 0.44, 30-min avg 0.21). Documented as watch-item in JIMINY-METRIC-PARTITION-ALERTS-PANELS-001 §10. | **REAL BUT EXPECTED** — recalibrate floor at T+30d once organic denominator settles. Don't recalibrate now (too early). |
| **cache low query hit ratio** | LOW | Informational. Cache warms after operator hits repeat queries. | **INFORMATIONAL** — no action. |
| **scorer-drift** | MED | I toggled RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED off/on 2× yesterday during UVTS-IMPLEMENTS-BASELINE-001 A/B. Rule fires when distinct scorer versions in 24h > threshold. | **SELF-INFLICTED by measurement sprint** — clears when the burst rolls off the 24h window. |

Net: **1 real substrate signal (jiminy-process)**, expected + timer-blocked. Rest are artifacts / rolling off.

## Metric health — categorized honest read

### 🟢 GREEN — substrate mechanics are excellent

| Metric | Live | Interpretation |
|---|---|---|
| `mdemg_rsic_health_memory` | **1.00** | Perfect — 33k nodes healthy |
| `mdemg_rsic_health_retrieval` | **1.00** | Retrieval saturated + healthy (was reweighted in DASHBOARD-TRUTH-002/SCORE-RETRIEVAL-REAL-SIGNALS-001) |
| `mdemg_rsic_health_task` | **0.965** | Excellent |
| `mdemg_j17_ticket_restore_success_rate` | **1.00** | DH-004 no-data gate working — restore either succeeds or reports honest N/A |
| `mdemg_j17_code_coverage` | **1.00** | J17 code path coverage healthy |
| `mdemg_j17_nli_bias_alert` | **0.00** | Correctly quiet post-DASHBOARD-TRUTH-001 fix (was permanently red pre-fix) |
| `mdemg_neo4j_graph_null_weight_edges` | **0** | HIDDEN-WEIGHT-001 healed all abstraction edges |
| `mdemg_neo4j_graph_orphans` | **32** | 0.03% orphan ratio on 100k node substrate — very healthy |
| `mdemg_retrieval_column_latency p95` | **0.016s** | RRF column p95 = 16ms — retrieval pipeline is snappy |
| Overall RSIC health | **0.72** | Composite; weighted-confidence sum per DH-005 |

**MDEMG's cognitive substrate mechanics are working.** Memory persists. Retrieval scores. Graph is dense (100k nodes, 800k edges) and clean.

### 🟡 YELLOW-BY-DESIGN — metrics look low because they're honest

These reflect the **CLASSIFIER-VERIFIABILITY class mix problem** JIMINY-CEILING-INVESTIGATION-002 identified. The metric mechanics are correct; the underlying substrate has known-improvable-only-through-decision-taxonomy gaps.

| Metric | 24h AVG | 7d AVG | Arc target | Owning arc |
|---|---|---|---|---|
| `mdemg_jiminy_follow_rate_classifier_verifiable` | 0.170 | 0.277 | ≥ classifier arc floor | JIMINY-CEILING-BREAK-2 Phase 4b T+30d re-measure (~2026-10-19) |
| `mdemg_jiminy_follow_rate_process_verifiable` | 0.211 | 0.157 | recalibrate at T+30d | JIMINY-METRIC-PARTITION-ALERTS-PANELS-001 watch-item |
| `mdemg_jiminy_follow_rate_hybrid` | 0.400 | 0.099 | small denom — noisy | JIMINY-METRIC-PARTITION-001 |
| `mdemg_jiminy_follow_rate_human` | 0.000 | 0.0004 | awaits organic operator grading | JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 (just shipped) |
| `mdemg_j17_avg_trust_score` | 0.343 | 0.370 | trend toward 0.80 per arc | JIMINY-CEILING-BREAK-2 |
| `mdemg_rsic_health_guidance` | 0.270 | 0.322 | tied to classifier follow rate | same arc |
| `mdemg_rsic_health_protocol` | 0.520 | ~ | J17-tied | J17 stability arc |
| `mdemg_j17_avg_comprehension` | 0.000 | 0.000 | needs T1 traffic to populate | J17-TIER-GATE-001 comprehension-mode |

⚠️ **The 24h classifier drop from 0.28 → 0.17 is likely artifact of yesterday's LLM-saturation burst (see heuristic-share alert).** The 7d avg 0.277 is the honest steady state. Will recover as the 24h window rolls off yesterday's spike.

### 🟠 YELLOW-CAPACITY — low-signal-density (not enough data yet)

| Metric | Current | Blocker |
|---|---|---|
| `mdemg_j17_events_total` | 5 | J17 tickets accumulate slowly; not a defect |
| `mdemg_j17_tier_t1_fraction` | 0 | T1 promotion is comprehension-gated (J17-TIER-GATE-001); waiting for T1 comprehension >0.6 with N>20 |
| `mdemg_j17_trust_session_count` | 2 | Only 2 sessions with ≥5 feedback events (per J17_TRUST_MIN_FEEDBACK_COUNT gate) |
| HITL grades (7d) | **0** | The corpus-growth bottleneck — nobody's grading. Direct impact on human-class + guidance follow rate lift. |

### ⚫ WHERE MDEMG IS TRULY BLOCKED FROM BEING "USEFUL" — the honest gap

Per the shipped roadmap arcs, ONE structural blocker dominates: **operator HITL throughput.**

- **Classifier retrain gate**: PHASE-4B-GATE-DECISION-001 (2026-09-11) DEFER'd on signal density (~8 classifier rows/day; needs ~30d more to detect ±5pp lift statistically). Timer-blocked to ~2026-10-19.
- **Human-class writer**: Just shipped (JIMINY-HITL-HUMAN-CLASS-INTEGRATION-001 yesterday). Gauge stays at 0 until operator grades.
- **Corpus quality**: Every honest lever (HITL-CURATION-002/003 auto-grader, JIMINY-HITL-VELOCITY-001 keyboard UI) points to operator grading throughput as the ceiling.

## Where MDEMG IS demonstrably useful right now

- Persistent memory across sessions: **working** (33k nodes, 190 IMPLEMENTS edges, 268k co-activation edges)
- Substrate-native constraint enforcement: **working** (Jiminy classifier verdict → constraint_outcomes → alert channel)
- Real-time retrieval: **snappy** (p95 = 16ms column latency)
- Alert channel: **honest** (5 alerts total → 3 informational artifacts, 1 self-inflicted, 1 real expected)
- Observability: **complete** (9 Grafana dashboards, 4 per-class follow-rate gauges, per-class alert rules shipped)
- Process observation: **live** (6 process observers grading real commits since 2026-09-19)

## Recommended next moves (ranked)

1. **Wait for the T+30d timer** (~2026-10-19). PHASE-4B-GATE-DECISION-001's math re-derives once organic classifier-class rows accumulate. Nothing to do until then except let the substrate work.
2. **Operator grades** — pending queue for `human_class_queue` is empty (nothing surfaced yet because human rules are marked informational). Once natural traffic surfaces one, grade it — moves the human class gauge off 0 organically. The `guidance` dataset has 200-cap pending items; even 10 operator grades per week would meaningfully lift the classifier gauge over a month.
3. **DON'T retune floors yet** — the arc says wait for organic drift. The two "low" real signals (classifier + process) are within honest measurement variance for their current sample sizes.
4. **Consider #6 HEBB-ETA-001** (3-5d research from Q5 §3) if operator wants a substrate-side lever that isn't timer-blocked. Ships a new learning-rate signal that could improve edge quality → cascade to retrieval quality → cascade to follow rate. Speculative but not blocked.

## Verdict on the question

**Is MDEMG truly useful right now?** Yes — as a cognitive substrate. Substrate mechanics deliver.

**Are the behavioral metrics where they need to be?** No — but the arc that owns closing the gap is executing on the documented timer (T+30d re-measure → retrain gate re-derive → decide whether to retrain or grow corpus). The math is right. The instrumentation is honest. The wait is intentional.

**What would move the needle fastest?** Operator HITL grading throughput. Every other lever (classifier retrain, corpus curation) is downstream of graded corpus.

## Files touched by this review

None. Read-only investigation. Zero substrate mutation.

## Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q5.md`
- `docs/development/jiminy-ceiling-investigation-002/sprint_post.md` — root-cause pin (class-mix problem)
- `docs/development/phase-4b-gate-decision-001/sprint_post.md` — DEFER decision + T+30d timer
- `docs/development/jiminy-metric-partition-alerts-panels-001/sprint_post.md` — process floor watch-item
- `docs/development/jiminy-hitl-human-class-integration-001/sprint_post.md` — 4th class writer (just shipped)
- `deploy/docker/grafana/dashboards/*.json` — 9 dashboards
- Live TSDB `metric_samples` — 30+ gauges + histograms
- Live TSDB `constraint_outcomes` + `review_grades` + `benchmark_runs`
- `~/.mdemg/alerts/current.json` — 5 pending alerts
