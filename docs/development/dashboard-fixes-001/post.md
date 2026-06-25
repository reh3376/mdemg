# DASHBOARD-FIXES-001 — Sprint Post

## Summary

Fixed the 10 pre-existing `:9999/ui` tab bugs the HITL-REVIEW-001 UI audit surfaced. All were client-side field mismatches / display-logic errors (endpoints returned 200 with sane JSON), plus two server response-field gaps. Live Playwright-verified.

## What shipped (by epic)

**Epic 1 — Critical**
- `backup.js` read `created`/`size`/`space_id`; `/v1/backup/list` returns `created_at`/`size_bytes`/`spaces` → columns were blank. Fixed field names. The Restore dropdown filtered a non-existent `status` field → always empty; the handler now emits a response-time `status="completed"` (a listed manifest is always a completed artifact). **UATS `backup_list.uats.json`** extended to assert `status` + `created_at`/`size_bytes`/`spaces`, hash re-pinned (UxTS contract, per the operator directive to use the framework for repeatable schema).
- `learning.js` sourced `frozen` from `/v1/learning/freeze/status` (no top-level `.frozen`) → frozen showed "active"; now reads `/v1/learning/stats` `freeze_state.frozen`.
- `dom.js` `statusBadge` — `running`/`completed` are now healthy (green) states (RSIC State badge rendered red when healthy).

**Epic 2 — Medium**
- `plugins.js` Details label toggles in-handler (render-time ternary left it stuck); detail fetched on SHOW only.
- `memory.js` surfaces `observation_count`; renders the `memoryDistribution` subscription that was fetched-and-discarded (graph phase + edge count + phase alerts).
- `main.js` immediate `pollRsicTab()` on `switchTab('rsic')`.

**Epic 3 — Low**
- `status.js` State badge reflects `degraded`; per-subsystem `healthz.checks` surfaced.
- `config.js` removed dead double-assigned `cb.onchange`.
- `handlers_features.go` populated `config_key` for features with a real enable flag (embeddings→EMBEDDING_PROVIDER, anomaly_detection→ANOMALY_DETECTION_ENABLED, hidden_layer→HIDDEN_LAYER_ENABLED, scraper→SCRAPER_ENABLED); always-on cores (learning/retrieval/conversation) honestly stay empty.
- `training_data.js` renders `result.tables` explicitly (array/object/scalar).

**Epic 4 — Playwright + docs**
- 7 `TestDashboardFixes001` Playwright assertions, all green live; full browser-ui suite regression-checked; CHANGELOG + this post.

## UxTS note

Mid-sprint the operator flagged that repeatable JSON contracts must use the UxTS framework. The backup/list `status` addition was therefore captured in the **UATS** spec (not ad-hoc), and the UI assertions use the existing Playwright/browser-ui harness. A check confirmed Jiminy did **not** proactively surface the UxTS-relevance guidance (guide returned 0 items; the knowledge existed only as low-score abstraction concepts) — a live instance of the jiminy-actionability gap. The actionable constraint was seeded into CMS.

## Testing
- **Tier 3 (live + Playwright):** `TestDashboardFixes001` (7 assertions) + full `test_browser_ui.py` regression.
- **Contract:** `backup_list.uats.json` asserts the new `status` + UI-depended fields; hash verified.

## Documents Accessed
- `internal/api/ui/tabs/{backup,learning,plugins,memory,status,config,training_data}.js`, `utils/dom.js`, `main.js`
- `internal/api/handlers_backup.go`, `internal/api/handlers_features.go`
- `docs/api/api-spec/uats/specs/backup_list.uats.json`, `tests/e2e/browser-ui/test_browser_ui.py`
