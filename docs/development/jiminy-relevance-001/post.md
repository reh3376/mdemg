# JIMINY-RELEVANCE-001 — Sprint Post

**Date:** 2026-06-24 · branch `reh3376_dev01` · v0.11.0 (coordinated pair with
HITL-REVIEW-001). The collection + measurement infrastructure for the
operator-decided 3–6 month corpus effort. Scoped by the Step-1 diagnostic
(`diagnostic_ignored_population.md`).

## What shipped (Epics 1, 2, 4, 5 — Epic 3 deferred to HITL-REVIEW-001)

| Epic | Deliverable | Live Tier-3 result |
|---|---|---|
| **1 — capture** | `guidance_training_rows` (V0027) + buffered async writer; persists guidance text + action text + source role/layer + verdict per feedback item (the evidence previously discarded) | real guide→feedback cycle landed **6 fully-populated rows**; counters enqueued=6/dropped=0; writer joined the `mdemg_tsdb_writer_*` plane |
| **2 — label quality** | supervised auto-relabel job (`guidance-audit`) re-labels heuristic/blank rows in place with the LLM classifier; `mdemg_guidance_corpus_heuristic_label_fraction` gauge | upgraded **12/12** heuristic rows → real verdicts; jobhealth success `{sampled:12,upgraded:12}`; gauge → **0** |
| **4 — honest measurement** | "should-follow follow rate" alert rule + Grafana panel over actionable types only (excludes correctly-ignored advisory) | SQL returns one non-NULL row (0.222 live; **1.0 on idle** = no false fire); pin-tested SQL contract |
| **5 — curation** | `mdemg data curate-guidance` → versioned, leak-audited, distribution-summarized corpus + manifest | **103-row** corpus; label_source {llm:74, tier1:23, explicit:6}; leak audit exit 0 |

## The throughline
Production already emits the perfect training signal on every feedback — we now
**keep it** (Epic 1), **clean its labels** (Epic 2), **measure it honestly**
(Epic 4), and **curate it** (Epic 5). What was a one-line "the action_summary is
classified then discarded" in the diagnostic is now a growing, trustworthy,
distribution-known corpus with a `gold_fraction` the retrain trigger can watch.

## Decisions disclosed (propose-in-plan / decide-at-execution)
- **Source role/layer**: bounded best-effort inline Neo4j lookup (default 300ms,
  `source_node_id` stored for offline re-resolution) rather than carrying it on
  the item or a separate backfill.
- **Relabel model**: reuse the existing jiminy `OutcomeClassifier` (same
  production LLM) instead of a separate `GUIDANCE_AUDIT_MODEL`.
- **Auto-relabel initial run**: added `GUIDANCE_AUDIT_INITIAL_DELAY_SEC` so the
  first cleanup doesn't wait a full day (and is testable) — a genuine operability
  improvement surfaced by the live-test need.
- **Should-follow denominator**: actionable `constraint`/`correction` types;
  certified-gold grounding via `review_grades` is a documented follow-up.
- **Curation path**: TSDB-sourced `mdemg data curate-guidance`, separate from the
  file-based UAITS `paradigm_router`.

## Carried forward
- **Epic 3 (human review)** — register the guidance corpus as HITL-REVIEW-001's
  first reviewable dataset; gated on that platform merging.
- **Certified-gold everywhere** — once `review_grades` exists, the should-follow
  metric and curation prefer human verdicts over auto-labels (the LEFT JOINs are
  already written to tolerate the table's absence).
- **The retrain FUTURE-TRIGGER** — `ft-guidance-001` (synthesis) +
  FT-CLASSIFY-002 (classify), when `gold_fraction` × volume crosses threshold.
- **`jiminy-actionability-001`** — the parallel near-term surfacing lever.

## Process / environment notes
- All four shipped epics carry a binding live Tier-3 result observed in TSDB /
  Grafana / logs (not mocked).
- `psycopg2` is required for the curation script (documented); installed into
  `neural/.venv` for the live test.
- A residual, unrelated **"Jiminy Service Unavailable" CRITICAL** kept re-firing
  on the post-JIMINY-SIGNAL-001 binary while `healthz` reported `jiminy:ok` —
  flagged for separate investigation; not a real outage and out of this sprint's
  scope.

## Documents Accessed
The diagnostic + both sibling plans (HITL-REVIEW-001, jiminy-actionability-001);
`internal/jiminy/{service,types,outcome_classifier}.go`; `internal/api/{server,
guidance_audit}.go`; `internal/tsdb/{reinforcement_writer,constraint_outcomes_
writer}.go` + migrations; `internal/alert/rules.go`; `internal/config/config.go`;
`internal/cli/{serve,data,data_curate}.go`; `scripts/audit_eval_leakage.py`;
`deploy/docker/grafana/dashboards/mdemg-jiminy.json`; live TSDB
`guidance_training_rows` + `scheduled_job_events`.
