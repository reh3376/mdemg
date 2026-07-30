# DORMANT-CENSUS-003 — Sprint Plan

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** DORMANT-CENSUS-002 disclosed follow-up #4.

## 1. Header & Metadata

Extend the DORMANT-CENSUS-001/002 pattern to metrics-registry gauges,
counters, and histograms. Ship `scripts/verify_metrics_consumers.py`
+ `docs/api/metrics_consumer_inventory.json` + CI drift check. Key
false-positive triage baked in: histogram base → derivative name
expansion (`H` → `H_p95`, `H_p99`, `H_bucket`, `H_sum`, `H_count`)
+ snapshot-reader recognition (any histogram is IN_USE if
`/v1/metrics/snapshot` has any consumer). ~2h effort.

## 2. Problem Statement

DORMANT-CENSUS-002 established the writer↔reader inventory pattern
for TSDB tables. **Metrics-registry gauges were deferred** because
"grep-diff has high false-positive rate on histogram base vs `_p95`
percentile derivatives" (per that sprint's post).

The metrics-registry surface today has **117 unique declared metric
names** across `internal/**/*.go` via `r.NewCounter/NewGauge/NewHistogram`.
Consumers live in:
- `deploy/docker/grafana/**/*.json` (dashboard panels)
- `internal/cli/grafana_templates/staged/**/*.json` (mirror)
- `internal/alert/rules.go` (evaluator rules)
- `internal/**/*.go` (in-process readers, e.g. RSIC self-assess)
- Snapshot API (`/v1/metrics/snapshot`) — exposes all metrics as JSON

Without a verifier, silent-drop of a metric (writer removed but
consumers still reference it, OR consumer changes but the metric is
still declared) recurs at each sprint. Same class DORMANT-CENSUS-001
and -002 solved for their surfaces.

## 3. Scope & Constraints

**In scope (single commit):**

- **`scripts/verify_metrics_consumers.py`** — enumerates declared
  metrics from Go via regex on `\w+\.New(Counter|Gauge|Histogram)\("<name>"`
  (excluding `_test.go`), tags each by TYPE, and adjudicates against
  the inventory:
  - Counter/Gauge: consumer must reference the exact name (with or
    without the `mdemg_` namespace prefix).
  - Histogram: consumer must reference ANY variant among
    {base, `_p95`, `_p99`, `_bucket`, `_sum`, `_count`}. Direct base
    consumers are rare; percentile-derivative consumers are typical.
  - If ANY consumer file grep-hits `/v1/metrics/snapshot`, all
    histograms + gauges are considered "possibly consumed via
    snapshot API" and can be marked `IN_USE_SNAPSHOT_ONLY` (an
    operator adjudication).
- **`docs/api/metrics_consumer_inventory.json`** — inventory shape:
  ```
  {
    "metrics": {
      "<metric_name>": {
        "type": "counter" | "gauge" | "histogram",
        "declaration": "<file>:<line>",
        "consumers": ["<consumer path>", ...],
        "disposition": "IN_USE" | "DORMANT_INTENTIONAL" | "DORMANT_TO_REMOVE" | "IN_USE_SNAPSHOT_ONLY" | "REMOVED",
        "notes": "..."
      }
    }
  }
  ```
- Initial `--generate` run: creates the inventory with declared
  metrics + auto-discovered consumers (histograms expanded).
  Anything with 0 consumers → `UNREVIEWED` (forces adjudication).
- **CI integration**: same shape as `verify_tsdb_consumers.py` —
  fails merge on drift (added/removed/unreviewed).
- Sprint POST records the first-pass census: how many gauges are
  clearly IN_USE (dashboards or alerts hit them), how many are
  UNREVIEWED, and highlights any real DORMANT_TO_REMOVE candidates
  found during first-pass adjudication.

**Out of scope:**

- Removing any dormant metrics found during first-pass — that's a
  future cleanup sprint (mirrors DORMANT-CENSUS-002 → FT-DORMANT-
  CLEANUP-001 sequencing)
- Adjudicating every UNREVIEWED metric — the sprint SHIPS the
  verifier + does a quick first-pass. Operators can iterate on
  adjudication in follow-up work.
- Percentile-derivative writer verification (the mechanism that
  produces `_p95` etc. is in `recorder.go`; verifying THAT wiring
  is a separate concern)

## 4. Method

1. Write the enumerator (regex over `internal/**/*.go`) → declared metrics list
2. Write the consumer walker (Grafana JSON + alert rules + Go readers + snapshot detection)
3. Write the `check()` + `generate()` + `--generate` CLI mirroring `verify_tsdb_consumers.py`
4. Run `--generate` → produces initial inventory
5. Manual first-pass adjudication: mark obvious IN_USE (dashboards + alert rules) + a few DORMANT candidates
6. Wire CI check into `.github/workflows/ci.yml`
7. Docs (post, CHANGELOG, CLAUDE.md pin)
8. Commit

## 5. Testing Plan

- **Tier 1 (unit)**: — the verifier IS the test (its `check()` mode)
- **Tier 2 (integration)**: run against the live tree — must produce
  a well-formed inventory + finish in <5s (117 metrics × few consumer
  files is small)
- **Tier 3 (live)**: N/A — this is a build-time forcing function

## 6. Commit Strategy

Single commit under `DORMANT-CENSUS-003`.

## 7. Verification Checklist

- [ ] `verify_metrics_consumers.py` enumerates declared metrics
- [ ] Histogram derivative naming (base ↔ `_p95` etc.) handled
- [ ] Snapshot-reader recognition path
- [ ] Inventory JSON generated + committed
- [ ] CI wired
- [ ] First-pass adjudication (best-effort; UNREVIEWED remainder acceptable
      if the verifier fails CI cleanly on the rest)
- [ ] CHANGELOG + CLAUDE.md pin + post

## 8. Rollback

Revert commit. No substrate mutation.

## 9. Risks

- **Risk**: false-positive dormant on a metric consumed via a code
  path the walker misses (e.g. templated variable name).
  - **Mitigation**: snapshot-reader recognition catches all
    histograms/gauges consumed via the JSON API; operators can
    manually mark `DORMANT_INTENTIONAL` with a note on genuine cases
- **Risk**: verifier runtime scales poorly on a giant codebase.
  - **Mitigation**: 117 declared × a few dozen consumer files is
    trivial; grep-scanning finishes in <1s

## 10. Documents Accessed

Filled in `post.md`.
