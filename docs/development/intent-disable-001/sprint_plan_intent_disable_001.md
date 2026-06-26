# Sprint INTENT-DISABLE-001 — Disable intent translation (evidence-backed)

## 1. Header & Metadata
- **Sprint ID**: INTENT-DISABLE-001
- **Sprint line**: `docs/development/intent-disable-001/`
- **Date opened**: 2026-06-26
- **Target version**: v0.11.x (config-default correctness + operator default)
- **Estimated effort**: ~0.5 dev-day
- **OpenAI spend**: $0 (local model only; the A/B itself used the local llama-server)
- **Risk level**: Low (turns a net-negative feature off; shipped default was already off)

## 2. Problem Statement
`retrieval.intent_translate` — the LLM query-rewrite-before-embedding on the retrieval hot path — was the single largest source of LLM errors: **70% of all LLM errors over 7 days (123/175)**, a steady **~15% timeout rate**, driving the recurring `alert_llm_health` error-rate spike and HIGH consecutive-failure alerts. It is a **synchronous, fail-open** call: on timeout it logs a Warn and uses the raw query, but blocks retrieval up to the timeout first (avg 3.8s, up to 15s). The config **default was 2000ms** — below the avg local-model latency (~4400ms) — so any operator enabling intent on defaults gets ~100% failures; this also violates the standing "never set latency_budget < 15000ms" rule.

The open question (operator-posed): **does intent translation actually improve retrieval quality enough to justify the cost?**

## 3. Scope & Constraints
**In scope:** the UVTS A/B to answer the question; then, on the data, disable + the durable follow-through (config-default fix, the `?intent=` measurement override + its UATS contract, docs).
**Out of scope:** removing the intent translator code (kept — re-enableable + re-verifiable after model/substrate changes); retuning intent (the data says it's net-negative, nothing to tune toward).
**Constraints:** data-decides the keep/disable call; 3 testing tiers; live Tier-3; docs final.

## 4. Dependencies
- UVTS harness (`docs/tests/uvts/`), live stack, llama-server :8102.
- The `?intent=true|false` URL override (added this sprint) to A/B without server restarts.

## 5. Implementation Plan
**Epic 0 — Measurement enablement.** Add `?intent=true|false` URL-param override to the retrieve handler (mirrors `?sparse=`/`?strict_context=`), so the UVTS runner can A/B intent on vs off via `--extra-url-params`.

**Epic 1 — The A/B (the decision input).** UVTS quick (16q) then full (120q), intent OFF (baseline) vs ON (candidate via `?intent=true`), back-to-back to minimise substrate drift.
- **Quick 16q:** OFF 0.3900 / ON 0.3960 (+0.006, pass) — *small-sample, within noise*.
- **Full 120q:** OFF **0.4170** / ON **0.4070** (**−0.010, FAIL**), 0 regressions >0.1 but 4 improvements net-negative.
- **Verdict: intent translation is net-negative on retrieval quality.** Combined with the latency + alert cost → **DISABLE** (data-decided).

**Epic 2 — Disable + config correctness.**
- Local `.env`: `INTENT_ENABLED=false` (stops the chronic errors/alerts; shipped default already false).
- `INTENT_TIMEOUT_MS` default `2000 → 15000` (config.go) — honors the no-tight-timeout rule; protects any operator who re-enables intent. Floor stays 200 (intent is fail-open; operators MAY fast-fail).

**Epic 3 — Contract.** UATS `?intent=true|false` variants added to `memory_retrieve_sparse_context.uats.json` (response-shape contract; hash re-pinned).

**Epic 4 — Testing (3 tiers).** Tier 1 config/api unit; Tier 2 UATS contract (7/7 live); Tier 3 live: server restarted with intent off → 0 intent_translate calls post-restart; the A/B itself is live Tier-3 evidence.

**Epic 5 — Docs (final).** This plan + `post.md`, CLAUDE.md note, CHANGELOG. A/B verdict JSONs archived in the sprint dir.

## 6. Testing Plan (3 tiers)
- **Tier 1:** `go test ./internal/config/ ./internal/api/` green; `verify_config_consumers.py` 726/726.
- **Tier 2:** UATS `validate` of the sparse_context spec — 7/7 pass incl. both new intent variants; hashes verified.
- **Tier 3 (live):** the 120q A/B (real binary + real services); post-disable restart shows zero new `retrieval.intent_translate` interactions.

## 7. Commit Strategy
Single sprint commit on `reh3376_dev01` (handler override + config default + UATS + docs; `.env` is gitignored — local runtime change). Push → auto-PR.

## 8. Verification Checklist
- [x] `?intent=` URL override added + live-smoke confirmed it toggles translation
- [x] UVTS 120q A/B run; verdict archived (`ab_verdict_full_120q.json`)
- [x] `INTENT_TIMEOUT_MS` default 2000→15000; config guard green
- [x] `.env` INTENT_ENABLED=false; server restart shows 0 post-restart intent calls
- [x] UATS 7/7 live, hashes re-pinned
- [x] build + lint clean
- [ ] CLAUDE.md + CHANGELOG + post.md updated

## 9. Documentation Update — Epic 5
## 10. Risks & Mitigations
| Risk | L | I | Mitigation |
|---|---|---|---|
| Quick-profile (+0.006) misread as "intent helps" | — | — | Ran the full 120q; it flipped to −0.010. Never decide a costly architectural call on the 16q sample. |
| Disabling loses a future-useful rewrite | Low | Low | Code kept; `?intent=true` A/B re-runs the verdict after any model/substrate change before re-enabling. |
| Operator re-enables intent on the old 2000ms default | Low | Med | Default raised to 15000ms. |

## 11. Documents Accessed
- `internal/api/handlers.go` (retrieve URL-param block, intent call site)
- `internal/config/config.go` (IntentTimeoutMs)
- `internal/retrieval/intent_translator.go` (fail-open behavior)
- `docs/tests/uvts/` runner + `lnl_demo_validation.uvts.json`
- `docs/api/api-spec/uats/specs/memory_retrieve_sparse_context.uats.json`
- Live `llm_interactions` TSDB

## 12. Rollback Procedures
- Re-enable: `.env` `INTENT_ENABLED=true` + restart (re-introduces the chronic errors — re-run the A/B first).
- The handler override + config default + UATS are additive/correctness changes; revert via git if needed.
