# Sprint Plan — DASHBOARD-FIXES-001: Browser Dashboard Tab Correctness

## 1. Header & Metadata
- **Sprint ID:** DASHBOARD-FIXES-001
- **Line dir:** `docs/development/dashboard-fixes-001/`
- **Date opened:** 2026-06-24
- **Branch:** `<handle>_dev<NN>` (PR to `main`)
- **Target version:** patch (UI-only correctness fixes)
- **Effort:** ~1 dev-day
- **Risk:** Low (vanilla-JS UI fixes + a few server response-field additions; no schema/data changes)
- **Lineage:** Findings from the HITL-REVIEW-001 UI audit (3-agent fan-out over all 11 `:9999/ui` tabs, 2026-06-24). The Review tab itself was correct + improved in-sprint; this sprint fixes the **pre-existing** bugs the audit surfaced in the other tabs.

## 2. Problem Statement
A full audit of the browser dashboard (`internal/api/ui/tabs/*.js`) against the live server found several tabs whose render code reads fields the backend doesn't return (silent blanks), or computes display state from the wrong signal (operator mislead). None crash the page, but several mislead operators or hide data. All endpoints return HTTP 200 with sane JSON — the bugs are client-side field mismatches + display-logic errors, plus a few server response-field gaps.

## 3. Scope & Constraints
**In scope — by severity:**

### 🔴 Critical (functional breakage)
1. **backup** (`tabs/backup.js`) — three field-name mismatches vs `GET /v1/backup/list` + a non-functional Restore section:
   - `backup.js:154` reads `b.created` → endpoint returns `created_at` (Created column always `—`).
   - `backup.js:155` reads `b.size` → endpoint returns `size_bytes` (Size column always `—`).
   - Space column reads `b.space_id || b.space_ids` → endpoint returns `b.spaces` (array).
   - `backup.js:183` Restore filters `b.status === 'completed'` but the list items have **no `status` field** → Restore dropdown always "No completed backups" (entire Restore section dead). Fix: include `status` in the list response **or** fetch per-backup status; prefer adding `status` to the list endpoint.
2. **learning** (`tabs/learning.js:45`) — `isFrozen = fs?.frozen` but `/v1/learning/freeze/status` returns `{count, frozen_spaces}` (no top-level `.frozen`). After the 30s poll a frozen space displays "active". **Fix: read `ls.freeze_state?.frozen`** (already present in `/v1/learning/stats`).
3. **rsic** (`tabs/rsic.js:35` + `utils/dom.js:31`) — the tab synthesizes `statusBadge('running')`, but `'running'` is not in `dom.js`'s green list → the State badge renders **red when RSIC is healthy**. Fix: add `'running'` to the green set in `dom.js`, or map running→`'active'`.

### 🟡 Medium (confusing UX / wasted work)
4. **plugins** (`tabs/plugins.js:87`) — "Details" button label ternary evaluated at render time → label stuck at "Details" after expand (never "Hide"). And `loadDetail` (`:119`) re-fetches `GET /v1/plugins/:id` on **hide**. Fix: update `button.textContent` inside the click handler; guard the fetch on show-only.
5. **memory** (`tabs/memory.js:19`) — `const dist = state.get('memoryDistribution')` is fetched every 30s and **never used** (dead subscription). Either render the distribution (`phase`, `edge_count`, `alerts` — there's a live `phase_saturated` alert) or drop the subscription. Also surface `learning_activity` + `observation_count` (present in `/v1/memory/stats`, never shown).
6. **rsic** — no immediate `poll()` on tab switch → "unknown" badges for up to 10s on mount (`main.js` doesn't call `pollRsicTab()` on `switchTab('rsic')`).

### 🟢 Low (polish / server data gaps)
7. **status** (`tabs/status.js:48,88`) — State row hardcoded `statusBadge('running')` (doesn't reflect `degraded`); `grafanaPort` never populated (always defaults 3000); `healthz.checks` hidden.
8. **config** (`tabs/config.js:131-136`) — dead double-assigned `cb.onchange` (line 131 overwritten by 136); harden the search filter with `String(e.value)`.
9. **features** (`tabs/features.js:83`) — `config_key` empty for 8/9 config-only services (server-side data gap — populate it in `GET /v1/admin/features`).
10. **training** (`tabs/training_data.js:155`) — `result.tables` rendered via raw array coercion; suggest `<input type="datetime-local">` for the From/To fields.

**Out of scope:** the Review tab (correct + improved in HITL-REVIEW-001); any backend behavior beyond the response-field additions named above; visual redesign.

**Constraints:** vanilla-JS only (no framework/build step — matches the existing UI); UI is embedded (`go:embed`), so a rebuild is needed to see changes live; every UI change needs a Playwright assertion (house rule).

## 4. Dependencies
- `internal/api/ui/tabs/*.js`, `internal/api/ui/utils/dom.js`, `internal/api/ui/main.js`.
- Server endpoints for the response-field additions: `GET /v1/backup/list` (add `status`), `GET /v1/admin/features` (populate `config_key`) — `internal/api/handlers_*.go`.
- Existing Playwright suite: `tests/e2e/browser-ui/test_browser_ui.py`.

## 5. Implementation Plan (sequential)
- **Epic 0** — Plan (this doc).
- **Epic 1 — Critical (backup, learning, rsic):** fix the field mismatches + Restore filter (backup), freeze-state source (learning), the green badge (rsic). Add `status` to `/v1/backup/list` if needed. Gate: each fix verified against the live response shape.
- **Epic 2 — Medium (plugins, memory, rsic-poll):** Details-button label + show-only fetch (plugins), dead-subscription cleanup + surface useful fields (memory), immediate poll on rsic mount.
- **Epic 3 — Low (status, config, features, training):** the polish items + the `config_key` server population.
- **Epic 4 — Playwright + docs:** extend `test_browser_ui.py` with assertions that catch each class (e.g. backup Created/Size columns non-empty when a backup exists; learning freeze reflects state; rsic badge green when healthy). Feature/CHANGELOG note.

## 6. Testing Plan (3 tiers)
- **Tier 1/2:** N/A for vanilla JS (no unit harness) — rely on the response-shape assertions + lint of any Go handler changes.
- **Tier 3 (live + Playwright):** for each fixed tab, a Playwright assertion against the running server: backup shows real Created/Size + a populated Restore dropdown (when a backup exists); learning freeze badge tracks `freeze_state`; rsic State badge is green when `/self-improve/health` is `ok`; plugins Details toggles label; no JS console errors on any tab. Run `pytest test_browser_ui.py` (the suite already covers all tabs incl. `review`).

## 7. Commit Strategy
One commit per epic (`fix(dashboard): …`). Push once; auto-PR.

## 8. Verification Checklist
- [ ] backup Created/Size/Space columns populated; Restore dropdown lists completed backups
- [ ] learning freeze badge reflects real `freeze_state` after a poll cycle
- [ ] rsic State badge green when healthy; immediate poll on mount
- [ ] plugins Details label toggles; no re-fetch on hide
- [ ] memory dead subscription removed or distribution rendered; `observation_count`/`learning_activity` shown
- [ ] status State row reflects health; grafanaPort discovered; config dead handler removed
- [ ] features `config_key` populated server-side
- [ ] Playwright assertions added per fix; `pytest test_browser_ui.py` green; no console errors
- [ ] CHANGELOG + (if applicable) feature-doc note

## 9. Documentation Update
CHANGELOG `Fixed` entry enumerating the corrected tabs; a short note in any dashboard feature doc if one exists.

## 10. Risks & Mitigations
| Risk | Sev | Mitigation |
|---|---|---|
| A "fix" assumes a response shape that varies by state | Med | Verify each field against the live response in multiple states (empty vs populated) before changing the read |
| Server response-field additions (backup `status`, features `config_key`) touch handlers | Low | Keep additive; pin with a UATS/contract assertion |
| Regressions in other tabs from `dom.js` badge change | Low | `'running'`→green is additive; Playwright covers all tabs |

## 11. Documents Accessed
The HITL-REVIEW-001 UI audit (3-agent fan-out, 2026-06-24); `internal/api/ui/tabs/*.js`; `internal/api/ui/utils/dom.js`; `tests/e2e/browser-ui/test_browser_ui.py`; the live `:9999` endpoints per tab.

## 12. Rollback
UI-only; revert the commits. No schema/data changes (the server response-field additions are additive and harmless if unused).
