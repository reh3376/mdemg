# MDEMG Outstanding Backlog

Re-derived 2026-06-25. The forward-looking work-list of sprints that are planned-but-incomplete or planned-but-unstarted. "What shipped" lives in `CHANGELOG.md`; "what's next strategically" lives in `docs/development/roadmap/ROADMAP_2026Q3.md`. This file is the concrete near-term execution queue.

> Maintenance: when a sprint here ships, move its line to a struck-through "Done" note (or delete) and update CHANGELOG + ROADMAP. Add new follow-ups as sprints spawn them.

---

## A. jiminy-actionability-001 — finish the sprint (PARTIAL — shipped 5 of 7 epics)

The original near-term guidance-surfacing lever. Plan: `docs/development/jiminy-actionability-001/sprint_plan_jiminy_actionability_001.md` (Epics 0–6). Lever A + Lever B + live A/B + docs shipped (PR #475); the rest is outstanding:

- **Epic 5 — Lever C (retrieval-side actionable bias)** — ❌ NOT built. The Epic 4 A/B *triggered* its contingency (surfaced-actionable fraction only 6.7%→10.5% because retrieval doesn't surface `constraint`/`correction` candidates — the binding constraint is upstream). Role-scoped retrieval *boost* (not a hard floor), RRF-SCALE-001-safe (config-gated, RRF-calibrated default, re-audit every `Score`/`.Activation` comparison, score-audit doc). ~3d. (Was tentatively named "jiminy-actionability-002"; folds back as the real Epic 5.)
- **Epic 4 — complete the binding gate** — tune Lever A/B from observed data; measure follow-rate movement (multi-week or windowed); record a proper `ab_results.md`. As shipped, the verdict was "Lever A insufficient" *without* the tuning pass.
- **Epic 1 — commit the reusable A/B harness** — currently an ad-hoc `/tmp` script; the plan called for a committed reproducible harness.
- **Epic 6 — write the missing `post.md`**; reconcile feature-doc name (`jiminy-actionability.md` shipped vs `guidance-actionability.md` planned).

## B. DASHBOARD-FIXES-001 — `:9999/ui` tab correctness (READY, not executed) ← **STARTING NOW**

Plan: `docs/development/dashboard-fixes-001/sprint_plan_dashboard_fixes_001.md` (Epics 0–4, ~1 dev-day). 10 logged bugs from the HITL-REVIEW-001 UI audit:
- 🔴 **backup** (3 field-name mismatches vs `/v1/backup/list` + dead Restore dropdown), **learning** (frozen space shows "active"), **rsic** (healthy State badge renders red).
- 🟡 **plugins** (Details label stuck + refetch on hide), **memory** (dead 30s subscription + hidden `observation_count`/`learning_activity`), **rsic** (no immediate poll on mount).
- 🟢 **status** (hardcoded badge, grafanaPort never populated, checks hidden), **config** (dead double-assigned handler), **features** (`config_key` empty for 8/9 services — server gap), **training** (table render + datetime inputs).
- Server response-field additions: `status` on `/v1/backup/list`, `config_key` on `/v1/admin/features`.
- Tier 3: Playwright assertion per fix (`tests/e2e/browser-ui/test_browser_ui.py`).

## C. GRAFANA-AUDIT-002 — re-audit + improve `:3000` dashboards — ✅ SHIPPED 2026-06-25

Re-audit: 0 FAIL (dashboards report correctly); fixed the dead `_p50` panel target; added 3 new-gauge panels (jiminy surfaced-actionable, null-weight-edges, conversation-coverage). Disclosed follow-up: `llm_interactions.quality` has no writer (Entropy Health panel — metric-instrumentation task). See `docs/development/grafana-audit-002/`.

<details><summary>original scope</summary>

GRAFANA-AUDIT-001 (shipped 2026-05-21) built the harness `scripts/grafana_panel_audit.py` and left **17 EMPTY / 18 SKIP / 0 FAIL** across ~146 panels / 8 dashboards. That result is **~5 weeks + ~10 metric-changing sprints stale** (TSDB-CONSUME-001 deleted/windowed gauges + removed ft_* panels; HIDDEN-WEIGHT/CHURN, EVENTGRAPH, jiminy-actionability added new gauges with **no panels**).
- **Phase 1 (correctness):** re-run `grafana_panel_audit.py` against the current live TSDB; produce the current PASS/EMPTY/FAIL/SKIP verdict per panel.
- **Phase 2 (improve):** wire-or-delete the EMPTY/FAIL panels; add panels for the new unpanelled metrics (`mdemg_jiminy_surfaced_actionable_fraction`/`_abstraction_fraction`, incremental-clustering signals, eventgraph counters, HIDDEN-CHURN gauges, etc.). UOBS/UOTS dashboard specs gate it.
</details>

---

## Roadmap-tier follow-ups (not near-term queue; see ROADMAP §5 + the roadmap phases)
- **HIDDEN-CHURN-003 split/merge maintenance** — *only if* incremental-clustering pattern-count drift is observed (the `concepts recluster` command is the interim escape hatch). Conditional, not committed.
- Other ROADMAP_2026Q3 Phase 1–4 sprints (HOOKWIRE/HOOKSYNC, RSIC-VALIDATE, NEGFEED+COOLER, TSDB-CONSUME follow-ons, FT-CLASSIFY-002, etc.) — strategic queue, owned by the roadmap.
