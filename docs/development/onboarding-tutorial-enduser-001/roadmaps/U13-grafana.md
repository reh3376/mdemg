# U13 — Grafana (7 slides)

**Intent**: The 9 dashboards + opinionated "watch these" panel list + how to read p95/mean + alert-link pattern + dashboard governance.
**Target slides**: 7
**Mermaid**: 0 (dashboard screenshots belong at E5 if operator wants them)
**Natural Q&A anchor**: after U13

---

## U13.1 — Section Landing

**Title**: `U13 · Grafana — The Historical View`

**Body**:
- Intent: "Grafana is the time-series companion to /ui/. /ui/ answers 'right now'; Grafana answers 'the last 24 hours / 7 days / 30 days.' Nine dashboards covering ~70 panels — but 5 panels carry most operator attention."
- Preview: 9 dashboards → the daily 5 → honest reading patterns → alert panel links → dashboard provenance → gotchas
- Points back to U1.5 "How does it continuously improve?" — Grafana is the observability substrate for every improvement loop

**Provenance**:
- `deploy/docker/grafana/dashboards/` — 9 JSON dashboards
- CLAUDE.md "DASHBOARD-TRUTH-001/002/003" (metric honesty sprints)

---

## U13.2 — The 9 Dashboards

**Title**: `The 9 Dashboards`

**Body**:
- Full inventory (all datasource `timescaledb` literal UID per GRAFANA-SHIP):
  1. **mdemg-overview** — the home dashboard; request rate, p95 latency, error rate, breakers; carries the arc-trajectory UVTS panel
  2. **mdemg-jiminy** — follow-rate (classifier + per-class), outcome split, effectiveness, actionable compliance; the JIMINY-CEILING-BREAK-2 trajectory panel lives here
  3. **mdemg-rsic** — RSIC health dimensions (DH-005), cycle admission, insight stream, confidence gauges
  4. **mdemg-neo4j** — Neo4j CPU/memory, null-weight edges, orphan ratio, graph node count
  5. **mdemg-ft-training** — Phase 10 benchmark aggregate, per-task pass rate, FT loop state, FT benchmark staleness, latest benchmark age
  6. **mdemg-graph-topology** — Hebbian weight entropy, dynamic-edge creation, reinforcement event rate (EVENTGRAPH-001)
  7. **mdemg-hitl** — grade cadence (auto vs operator), pending drafts, reinforcement applied 7d (HITL-ANALYTICS-TILE-001)
  8. **mdemg-j17** — J17 protocol compression, trust session count, NLI bias, auto-compact AVG (DASHBOARD-TRUTH-002 fixes)
  9. **mdemg-llm-routing** — LLM request rate per task, error rate per task (filtered for caller_canceled), latency per task

**Provenance**:
- `deploy/docker/grafana/dashboards/*.json`

**note**:
- Operator cue: "You don't need to memorize all 9. Pin the Overview + Jiminy to your Grafana sidebar; the others you'll open when a specific symptom demands it."

---

## U13.3 — The Daily 5 Panels

**Title**: `What To Actually Watch`

**Body**:
- The 5 panels that catch most operational issues:
  1. **mdemg-overview** → **Error Rate** (should be ≤0.001; honest, post RETRIEVE-CALLER-CANCEL-001)
  2. **mdemg-overview** → **P95 Latency** (SLO: ≤120s per DH-004 extension; retrieval goal <2s)
  3. **mdemg-jiminy** → **Follow Rate** panel (current ~13%; title embeds JIMINY-CEILING-BREAK-2 target ≥80%)
  4. **mdemg-ft-training** → **Latest Benchmark Age** (green <3d, yellow 3-7d, red >7d; FT-BENCH-REFRESH-001)
  5. **mdemg-neo4j** → **Neo4j Memory + CPU** (CPU AVG >500% over 5min is actionable per ALERT-TRUTH-001; CPU is per-SINGLE-core)
- Alternative: watch the terminal alerts channel (U16) — the panels and alerts share thresholds, so operator attention stays coherent

**Provenance**:
- CLAUDE.md "DASHBOARD-METRICS-DEEP-DIVE-001" (operator-flagged low metrics triage)

**note**:
- Operator cue: "If you only watch 5 things, these 5. Everything else is noise until these 5 go red."

---

## U13.4 — How To Read A Panel (Honest Metrics)

**Title**: `Reading Panels Honestly`

**Body**:
- Rules per DASHBOARD-TRUTH-001/002/003:
  - Panel titles EMBED design-intent numbers (e.g. "Follow Rate (honest ~13%; target ≥80% via JIMINY-CEILING-BREAK-2)")
  - `noValue` on gated stats names the gate ("N/A (n<2)" not blank — DASHBOARD-TRUTH-002 Min/Max Trust fix)
  - Multi-row gauges use bargauge or stat-split; NEVER `lastNotNull` reduce (DASHBOARD-TRUTH-002 Retrieval Pipeline Health fix)
  - Latency buckets cover LLM path up to 120s (DASHBOARD-TRUTH-001 extended histograms)
  - Stat panels showing a RATE use a single windowed aggregate (NEVER per-bucket ratio reduced to lastNotNull — DASHBOARD-TRUTH-001 RSIC Cycle Success Rate fix)
- Expect to see arc-trajectory numbers (the follow-rate panel's title literally says "arc target ≥80%; see JIMINY-CEILING-BREAK-2") — this is BY DESIGN, not a bug

**Provenance**:
- CLAUDE.md "Dashboard metric honesty" sprint family (001 + 002 + 003)
- CLAUDE.md "Operator-flagged low-reading metrics triage (DASHBOARD-METRICS-DEEP-DIVE-001)"

**note**:
- Operator cue: "If a panel feels dishonest, read the TITLE first. 90% of 'this metric looks low' is 'this metric embeds its own honest target in the title.'"

---

## U13.5 — Dashboard Governance

**Title**: `How Dashboards Stay Honest`

**Body**:
- Dashboard changes land in `deploy/docker/grafana/dashboards/*.json`
- GRAFANA-SHIP (RELEASE-HYGIENE-001 E2): `make sync-grafana-embed` mirrors to `internal/cli/grafana_templates/staged/`; `make verify-grafana-embed` CI gate fails merge on drift
- Datasource UID in every panel = literal `timescaledb` (NOT `${DS_TIMESCALEDB}`); operator Grafana install uses literal via provisioning (`datasources.yml`)
- When adding a Grafana panel reading `llm_interactions.error`:
  - Must apply `NOT LIKE 'caller_canceled:%'` filter (GRAFANA-PANEL-FILTER-001)
  - Pin test (`internal/grafanapin/dashboards_test.go`) walks every JSON + fails if any panel in aggregate lacks the required exclusion
- Title governance (DASHBOARD-TRUTH-002 E1/E2): embed design-intent number in title; description-only is second-class

**Provenance**:
- CLAUDE.md "Release hygiene closeouts (RELEASE-HYGIENE-001)" + "GRAFANA-PANEL-FILTER-001"
- `internal/grafanapin/` + `Makefile` sync/verify targets

**note**:
- Operator cue: "If you want to add a panel, edit the JSON + `make sync-grafana-embed` + commit both in the same change. Drift gate catches you otherwise."

---

## U13.6 — Alert ↔ Panel Linking

**Title**: `When Alerts Fire — Where To Look`

**Body**:
- The alerts channel (U16) names the rule; each rule has a corresponding Grafana panel
- Rule-to-panel map (abbreviated — full in `internal/alert/rules.go`):
  - `jiminy_follow_rate_classifier_verifiable` → `mdemg-jiminy` Follow Rate panel
  - `ft_benchmark_stale` → `mdemg-ft-training` Latest Benchmark Age panel
  - `scheduled_job_recent_failure` → `mdemg-overview` Scheduled Jobs History
  - `low_cache_hit_ratio` → `mdemg-overview` Cache Hit Ratio panel
  - `neo4j_high_cpu` → `mdemg-neo4j` Neo4j CPU panel
- Workflow: alert fires → open the named panel → read the trajectory + title + current value → decide (triage / override / wait / fix)

**Provenance**:
- `internal/alert/rules.go` + each dashboard's panels

**note**:
- Operator cue: "Alerts tell you WHAT fired; panels tell you WHEN it started + the trajectory. Both together are the triage toolkit."

---

## U13.7 — Gotchas + Where To Go Deeper

**Title**: `Grafana — Gotchas + More`

**Body**:
- Gotchas:
  - Grafana admin credential is printed by `mdemg init` ONCE; store it somewhere (operator Keychain / password manager); default URL `http://localhost:3030`
  - Panels showing "No data" with a known-good query are usually a time-window issue — expand to 24h or 7d
  - Alert-rule changes land in `internal/alert/rules.go` + require server restart; Grafana `mdemg-*` dashboards are read-only (no inline alerting config here)
  - Dashboard UIDs are STABLE: `mdemg-overview`, `mdemg-jiminy`, etc. — DO NOT RENAME (external bookmarks + alert-link URLs would break)
- Feature docs:
  - CLAUDE.md "Dashboard metric honesty" sprint family (001/002/003)
  - `docs/features/*.md` — many feature docs include their Grafana panel references
- Where this feeds into: U14 CLI (`mdemg healthz` + `mdemg synergy status` + `mdemg watchdog status` are the three READ CLIs that correspond to Grafana + /ui/ Status), U15 APIs (Grafana reads TSDB directly via `timescaledb` datasource; no API intermediary)

**note**:
- Operator cue: "Grafana + /ui/ + CLI are the three operator surfaces. Pick the one that matches your TIMESCALE — now (/ui/), minutes (CLI), hours+ (Grafana)."
- Transition: "U14 next — the CLI command surface, now surveyed in full."
