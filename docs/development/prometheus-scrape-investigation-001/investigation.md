# PROMETHEUS-SCRAPE-INVESTIGATION-001 — Investigation

**Date:** 2026-07-21
**Method:** grep + git-history + config-audit; no code or data changes.
**Verdict:** **(c) DELIBERATE REMOVAL — already fully documented.** No follow-up sprint warranted.

## E1 — Current-state audit

```
$ curl -s http://localhost:9999/metrics
404 page not found

$ grep -rn "promhttp\|prometheus\.NewRegistry\|prometheus\.DefaultRegisterer" internal/ cmd/
(no matches)

$ grep "prometheus" go.mod
(no matches)
```

- No `promhttp` handler registration anywhere in the tree.
- No `prometheus/client_golang` dependency in `go.mod`.
- No `prometheus.NewRegistry` or `prometheus.DefaultRegisterer` references.

The `internal/metrics/` package is a **home-brewed registry** (`registry.go`, `recorder.go`, `collectors.go`, `snapshot.go`) — not built on top of `prometheus/client_golang`.

**Actively-used sinks:**
1. `/v1/metrics/snapshot` — JSON snapshot endpoint (`{data: {counters, gauges, histograms}}`). Live-verified working.
2. TSDB `metric_samples` — buffered writer persists every gauge/counter tick. Live: **161 metrics / 56,778 samples in the last hour** on `mdemg-dev`.

## E2 — Git history

Two commits ever touched `prometheus/client_golang`:

- **6704256 (2026-03-26)** — `feat(grafana): add RSIC health metrics, graph exploration, dashboard polish, and dev guide`. This is when Prometheus gauges were originally REGISTERED via the Prometheus client library.
- **981d40d (2026-03-28)** — `fix(j17): fix 12 cascading breaks in J17 protocol pipeline`. **This commit deleted `docs/api/api-spec/uats/specs/prometheus_metrics.uats.json`** and (per its own commit message) also included the **"Prometheus-to-TSDB migration, MetricsRecorder/snapshot system"**.

Two days after Prometheus gauges were introduced, they were removed and replaced with the home-brew MetricsRecorder that persists directly to TSDB. This was a deliberate architectural choice, not an oversight.

## E3 — Config + docs cross-check

- **No config switches** — no `PROMETHEUS_ENABLED`, `METRICS_HTTP_ENABLED`, or `SCRAPE_*` variable exists. (The `SCRAPER_*` variables that grep found are the web-scraper feature, unrelated to Prometheus scrape.)
- **No Prometheus datasource** in Grafana — only `neo4j.yml`, `nodegraph-api.yml`, `timescaledb.yml` under `deploy/docker/grafana/provisioning/datasources/`.
- **No Prometheus container** in the shipped docker-compose files.
- **Documented explicitly**: `docs/features/prometheus-observability-monitoring.md:18` reads:

> **Note (2026-03-29):** The `/v1/prometheus` endpoint has been replaced by `/v1/metrics/snapshot` (JSON format). Prometheus has been replaced by TimescaleDB as the metrics backend. Alert rules are now managed via Grafana alerting with SQL queries against TimescaleDB.

## E4 — Verdict + recommendation

**(c) DELIBERATE REMOVAL, AND already fully documented.**

The DASHBOARD-TRUTH-002 triage report flagged `/metrics 404` as a "bonus finding" — but that was a false-alarm. The 404 is the shipped, intended, documented behavior. Every piece of the ecosystem is aligned:
- No producer (no Prometheus client library dependency)
- No consumer (no Prometheus container in compose; no Prometheus datasource in Grafana)
- No config knob (nothing to enable/disable)
- Explicit documentation of the migration decision

**No follow-up sprint is warranted.** No code should be written; no `promhttp` handler should be wired. The metrics access model is clean:
1. **Live metrics** → `GET /v1/metrics/snapshot` (JSON)
2. **Historical metrics** → SQL against `metric_samples` in TSDB
3. **Alerts** → TSDB-query alert rules via `internal/alert/`
4. **Dashboards** → Grafana with the `timescaledb` PostgreSQL datasource

## Trivial doc-hygiene follow-up

`docs/features/ui-gap-analysis.md:74` still references `/v1/prometheus` as a UI endpoint gap:
```
- `/v1/prometheus`, `/v1/metrics/trends`
```

This is stale. The `/v1/prometheus` endpoint was retired in 2026-03-29 per the migration; UI-gap analysis should either drop the reference or replace with `/v1/metrics/snapshot`. Not fixing here — noted as a minor doc-currency cleanup for the next docs pass (or for the DOC-CURRENCY-001 line).

Also `docs/features/j17-feedback-loop-closure.md:216` (from March 2026) still says: "3 integration tests: feedback updates metrics, endpoint returns OK, prometheus has J17 metrics". The J17 tests still exist but "prometheus" here refers to the historical setup; the actual gauges now land in TSDB via the recorder. Also stale-language, low-priority cleanup.

## What NOT to do

- **Do not** wire `promhttp.Handler()` at `/metrics`. It contradicts the shipped architecture, doubles emission (TSDB writer + Prometheus scrape are two sinks for the same signal), and complicates the mental model.
- **Do not** re-add `prometheus/client_golang` to go.mod. The home-brew registry has been production for 4 months.
- **Do not** treat the DASHBOARD-TRUTH-002 "bonus finding" as an open bug. The triage agent's classification of `/metrics 404` as a suspicious finding was itself a false-positive — the correct classification is "as-designed".

## Sample size caveat

None applicable — the investigation is code-and-config-audit, deterministic.
