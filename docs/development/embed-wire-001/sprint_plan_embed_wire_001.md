# Sprint Plan EMBED-WIRE-001 + INGEST-EXEC-001 — Embedder Wiring + Ingest Exec Resolution

## 1. Header & Metadata
- **Sprint ID:** EMBED-WIRE-001 (+ paired INGEST-EXEC-001, per roadmap) — Q3 Phase 1, rank #5
- **Line:** `docs/development/embed-wire-001/` · **Date:** 2026-06-11 · **Branch:** `reh3376_dev01`
- **Target:** v0.10.x · **Effort:** ~4 dev-days budgeted (expected less) · **Spend:** $0 · **Risk:** Low-Medium

## 2. Problem Statement
Two verified-live silent failures in core data paths (beta blockers):
1. **Embedder breaker + recorder wiring is type-assertion-fragile.**
   `embeddings.New` returns `*CachedEmbedder` under the DEFAULT config
   (`EmbeddingCacheEnabled=true`), so `server.go:356/359`'s
   `emb.(*embeddings.OpenAI)` / `(*embeddings.Ollama)` assertions both
   fail SILENTLY (no else branch) → the embedding circuit breaker has
   never been wired in any default deployment. The recorder assertion at
   `server.go:1252` (`emb.(*embeddings.CachedEmbedder)`) has the inverse
   fragility: disable the cache and training-data recording silently dies.
   Same disease, both directions: wiring that depends on the wrapper
   stack's exact shape.
2. **Server-triggered ingest execs a hardcoded relative `./bin/mdemg`**
   (`handlers_ingest_codebase.go:286`, `handlers.go:2824`) — broken in
   Docker (the documented-primary deployment) and in any CWD ≠ repo root.
   The scheduled sync (`StartScheduledSync` → `runScheduledSyncCheck`)
   triggers these jobs with no jobhealth reporting.

## 3. Scope & Constraints
**In:** `Unwrap()` on all three wrappers (Cached, RateLimited, NilSafe) +
chain-walking helpers (`embeddings.Base`, `embeddings.FindCached`); wiring
rewritten to walk the chain with LOUD warn when nothing matches; binary
resolution helper (`MDEMG_BIN` env → `os.Executable()` → PATH → ./bin/mdemg)
shared by both exec sites; scheduled-sync outcomes into jobhealth
(`job_name='codebase-sync'`, nil-safe); 3-tier tests incl. live breaker
visibility via `/v1/admin/breakers`.
**Out:** embedding-pipeline quality (the `alert_embedding_regression`
RSIC rule's empty-call_sites complaint — RSIC-VALIDATE-001 territory);
Docker e2e (validated via resolution-order unit tests + native live run;
no Docker instance in tonight's loop).
**Constraints:** sequential epics; live Tier 3; no hardcoding (env-first
resolution); fail LOUD not silent.

## 4. Dependencies
HOOKSYNC alert delivery (to see new warns), DH-004 `/v1/admin/breakers`
(live breaker visibility), NOSILENT jobhealth, existing job-queue ingest.

## 5. Implementation Plan
- **Epic 0** — investigation + plan (done; all sites read).
- **Epic 1 (EMBED-WIRE)** — wrapper `Unwrap()`s + helpers; server wiring
  via chain-walk for breaker AND recorder; loud else-warns; Tier 1 chain
  tests (cache-on, cache-off, ratelimit+cache, nilsafe-wrapped).
- **Epic 2 (INGEST-EXEC)** — `resolveMdemgBin()`; both exec sites use it;
  scheduled-sync reports to jobhealth; Tier 1 resolution-order tests.
- **Epic 3** — Tier 3 live: restart server → breaker `openai-embeddings`
  visible in `/v1/admin/breakers` + wiring log line present; trigger a
  real ingest job via the API → resolved binary works natively; job event
  row for the sync path.
- **Epic 4** — docs (CHANGELOG, CLAUDE.md, roadmap tick, post.md).

## 6. Testing Plan
Tier 1: chain-walk unit tests + resolution-order tests. Tier 2: suites +
lint; UATS untouched. Tier 3: live breaker visibility, real API-triggered
ingest, jobhealth row. Live smoke item: *restart the real server, observe
"circuit breaker wired" log + the embeddings breaker in /v1/admin/breakers,
run a real ingest job through the API, observe the codebase-sync job event.*

## 7. Commit Strategy
One commit per epic; surprises standalone; push → auto-PR → summary.

## 8. Verification Checklist
- [ ] Breaker wired under DEFAULT config (cache on) — log + /v1/admin/breakers
- [ ] Recorder reaches CachedEmbedder regardless of outer wrappers; loud warn when absent
- [ ] Both exec sites resolve via env→executable→PATH→fallback (tests pin order)
- [ ] Real API ingest job runs with resolved binary (native live)
- [ ] Scheduled-sync outcome lands in scheduled_job_events
- [ ] Suites green; lint clean; docs updated

## 9. Documentation Update — Epic 4.

## 10. Risks & Mitigations
| Risk | L | I | Mitigation |
|---|---|---|---|
| Unwrap chain misses a future wrapper | M | M | `Base()` is interface-driven (`interface{ Unwrap() Embedder }`) — any wrapper implementing it joins the chain; loud warn otherwise |
| os.Executable() differs from repo bin in dev | L | L | MDEMG_BIN env wins; fallback preserves ./bin/mdemg |
| jobhealth rows add noise per sync tick | L | L | report only when a sync actually triggers work (skip idle ticks) |

## 11. Documents Accessed
`internal/embeddings/{embeddings,openai,ollama,nilsafe,ratelimit,cache}.go`;
`internal/api/server.go` (356/961/1252, StartScheduledSync),
`handlers.go:2824`, `handlers_ingest_codebase.go:286`;
`internal/config/config.go` (cache default); roadmap.

## 12. Rollback
Revert commits — wiring returns to silently-dead (prior state); exec
returns to ./bin/mdemg; no data migration.
