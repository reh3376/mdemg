# Sprint PROMETHEUS-SCRAPE-INVESTIGATION-001 — Investigate why /metrics HTTP scrape endpoint returns 404

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | PROMETHEUS-SCRAPE-INVESTIGATION-001 |
| Sprint Name | Investigate the missing Prometheus /metrics HTTP scrape endpoint |
| Owner | Roger Henley |
| Branch | `reh3376_dev01` |
| Base | `main` |
| Format Version | Sprint plan v1.0 (12-section) |
| Estimated Effort | 0.5 dev-day (investigation; fix scope decided from findings) |
| Sprint Line | prometheus-scrape-investigation-001 |
| Skill anchor | `skill:sprint-planning` |
| Parent scope | Bonus finding from DASHBOARD-TRUTH-002 triage (2026-07-20) |

## 2. Problem Statement

MDEMG registers Prometheus-style gauges/counters/histograms extensively (`mdemg_rsic_*`, `mdemg_j17_*`, `mdemg_jiminy_*`, `mdemg_tsdb_*`, `mdemg_mlx_*`, `mdemg_neo4j_*`, etc. — several hundred metric names). The DASHBOARD-TRUTH-002 triage discovered:

```
curl -s http://localhost:9999/metrics
404 page not found
```

There is no `promhttp` handler mounted at `:9999/metrics`. Every gauge is only reachable via the TSDB `metric_samples` sink (the buffered writer that persists Prometheus samples into TimescaleDB as rows).

This is not a broken system — the TSDB sink is intentional and load-bearing (it's how alert rules query, how DASHBOARD-TRUTH-* fixes are measured, how Grafana panels read). But the absence of a scrape endpoint is either:

**(a)** A deliberate architectural choice — TSDB is the sole persistence layer; Prometheus scrape was never wanted. **THEN** the 404 should be intentional and documented, and operator expectations set.

**(b)** An oversight — the handler was never wired but the framework's metric-registration pattern implies Prometheus. **THEN** either wire the handler OR document the intent.

**(c)** Deliberately disabled at some point — the handler may have existed and been removed. **THEN** the git history explains why, and we surface that reasoning.

Investigation is the right first step; the fix — if any — depends on the answer.

## 3. Scope & Constraints

**In scope**:
- Git-history spelunking: was there ever a `/metrics` handler? If so, when was it removed and why?
- Code review: how are metrics currently registered/emitted? Is there a Prometheus registry object at all, or only the TSDB writer sink?
- Config surface: is there a `PROMETHEUS_ENABLED`, `METRICS_HTTP_ENABLED`, or similar switch?
- Operational context: is any external system currently expected to scrape MDEMG? (Grafana Alloy? A separate Prometheus server? None?)
- Document the finding + recommended path forward.
- If the finding is unambiguous, spec a follow-up sprint to wire (or explicitly not-wire) the handler.

**Out of scope**:
- Building the handler (that's the follow-up if warranted).
- Migrating any dashboards to Prometheus datasource (unless the finding is "we should").
- Changing the TSDB sink.

**Constraints**:
- **Read-only investigation** — no code/config changes.
- **Cross-reference**: what does `docs/features/*.md` say about Prometheus scrape expectations?
- **Report writes to** `docs/development/prometheus-scrape-investigation-001/investigation.md`.

## 4. Dependencies & Pre-Conditions

- ✅ `mdemg` server running on `:9999`.
- ✅ Git history accessible.
- ✅ `internal/metrics/` package present (registry / recorder / writer).

## 5. Implementation Plan

### E0 — Sprint plan
Commit this plan.

### E1 — Current-state audit
- `grep -rn "promhttp\|Handle.*metrics\|/metrics" internal/ cmd/` — is `promhttp.Handler()` referenced anywhere?
- `grep -rn "prometheus.NewRegistry\|prometheus.DefaultRegisterer" internal/ cmd/` — is there a registry?
- Read `internal/metrics/` package layout: registry.go, recorder.go, writer.go.
- Enumerate: what does the recorder emit to (TSDB only, or TSDB + Prometheus registry)?
- Report: does the Prometheus client library get pulled in as a dependency? (`go list -m all | grep prometheus`)

**Gate**: current-state audit in `investigation.md` §1.

### E2 — Git history
- `git log --all --oneline -- '**/metrics*'` — every commit touching metrics code.
- `git log --all --oneline -S "/metrics" -- '**/*.go'` — every commit that added or removed `"/metrics"` as a string.
- `git log --all --oneline -S "promhttp" -- '**/*.go'` — same for the promhttp reference.
- Cross-check with sprint documents in `docs/development/` for any metrics-related sprint.

**Gate**: history summary in `investigation.md` §2 — was the handler ever there, when removed, what commit message said.

### E3 — Config + docs cross-check
- `grep -rn "PROMETHEUS\|METRICS_HTTP\|SCRAPE" internal/config/ .env* deploy/` — any switch?
- `grep -rn "prometheus\|metrics" docs/features/` — any feature doc reference the scrape endpoint?
- Check `deploy/docker/grafana/datasources/` — is Prometheus configured as a datasource? What URL does it point to?
- Check `deploy/docker/prometheus/` — does a Prometheus container exist in compose?

**Gate**: config/deploy audit in `investigation.md` §3.

### E4 — Diagnosis + recommendation
Given E1–E3, write §4 of `investigation.md` classifying as (a), (b), or (c):

- **(a) deliberate**: no fix needed; documentation-only sprint follow-up to make expectations explicit. Update `docs/features/observability.md` (or create).
- **(b) oversight**: spec a follow-up sprint to wire `promhttp.Handler()` at `/metrics`, with a switch (`METRICS_HTTP_ENABLED` default true), and update `docs/features/*.md`.
- **(c) deliberate removal**: document the historical reasoning + note whether current state has changed enough to reconsider.

If **(b)**, disclose whether wiring the handler would double-emit (TSDB sink + Prometheus scrape) and whether that's OK.

**Gate**: diagnosis written; follow-up sprint spec drafted (as a stub in the same file).

### E5 — Canonical docs
- CHANGELOG entry (this is an investigation — no code change; CHANGELOG entry documents the *finding* under a new `### Investigation` subsection).
- CLAUDE.md architecture note if the finding warrants it.
- If diagnosis is (a) or (c), `docs/features/observability.md` gets a "Metrics access model" section.
- Sprint post.

## 6. Testing Plan (3 tiers)

Investigation sprint — Tiers reframed:

**Tier 1 (Unit-equivalent)**: reproducibility of the 404 — anyone with the repo checked out can reproduce.
**Tier 2 (Integration-equivalent)**: cross-check TSDB sink is emitting correctly; audit that no consumer of MDEMG metrics currently expects `/metrics`.
**Tier 3 (Live E2E)**: confirm on live `mdemg-dev` — what's the actual current behavior? Is anything actively broken? (Answer expected: no — TSDB path is fine.)

## 7. Commit Strategy

1. `docs(prometheus-scrape-investigation-001): E0 — sprint plan`
2. `docs(prometheus-scrape-investigation-001): E1 — current-state audit`
3. `docs(prometheus-scrape-investigation-001): E2 — git history`
4. `docs(prometheus-scrape-investigation-001): E3 — config + docs cross-check`
5. `docs(prometheus-scrape-investigation-001): E4 — diagnosis + fix spec (if warranted)`
6. `docs(prometheus-scrape-investigation-001): E5 — CHANGELOG + CLAUDE.md + observability doc + sprint post`

May collapse to fewer commits.

## 8. Verification Checklist

- [ ] E1/E2/E3 audits all committed under `docs/development/prometheus-scrape-investigation-001/`
- [ ] Diagnosis (a)/(b)/(c) verdict documented
- [ ] Follow-up sprint spec written (or "no follow-up needed" documented)
- [ ] CHANGELOG + CLAUDE.md + sprint post committed
- [ ] Pushed; auto-PR created

## 9. Documentation Update (Epic E5 — never cut)

- **CHANGELOG.md** [Unreleased] > Investigation subsection: the sprint's finding.
- **CLAUDE.md**: if verdict is (a) or (c), add a "Metrics access model" note explaining TSDB-only. If (b), just note the follow-up sprint.
- **Feature doc**: `docs/features/observability.md` — created or extended to explicitly document the current metrics access model.
- **Sprint post**: findings + verdict.

## 10. Risks & Mitigations

| Risk | Severity | Mitigation |
|---|---|---|
| Investigation reveals no clear historical reason | Low | Ship the code-review-based recommendation ((a)-oversight-treat-as-b, or (a)-deliberate) with explicit "no history — deciding forward-only" caveat |
| Wiring `/metrics` (in follow-up sprint) would double-emit and confuse consumers | Medium | E4 explicitly diagnoses this; follow-up sprint decides on emit-strategy (e.g. Prom scrape reads current tick, TSDB sink is authoritative history) |
| An external system IS scraping (unknown to us) and adding the handler changes its behavior | Very low | E3 cross-checks operational deploy; if a scraper exists, it's already broken (404); adding the handler only fixes them |

## 11. Rollback Procedures

- N/A (no code/config changes).

## 12. Documents Accessed

- DASHBOARD-TRUTH-002 triage report (this session)
- `internal/metrics/{registry.go,recorder.go}`
- `internal/api/server.go` (mux registration)
- `cmd/mdemg/*.go` (server entrypoint)
- `docs/features/` (search for prometheus/metrics)
- `deploy/docker/` (compose files + grafana datasources)
- Git history for `/metrics`, `promhttp`, `prometheus.` references
- CLAUDE.md (search for Prometheus / scrape mentions — expected: none current)
