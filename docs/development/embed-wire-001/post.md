# EMBED-WIRE-001 + INGEST-EXEC-001 — Sprint Close

**Date:** 2026-06-11 · **Branch:** `reh3376_dev01` · **Roadmap:** Q3 Phase 1, rank #5

## What shipped

| Epic | Deliverable | Commit |
|---|---|---|
| 0 | Investigation + plan | `4574258` |
| 1 | Breaker + recorder reach the real embedder via the Unwrap() chain (`embeddings.Base`/`FindCached`); loud warns on no-match | `c84ecde` |
| 2 | `resolveMdemgBin()` (MDEMG_BIN → os.Executable() → PATH → ./bin/mdemg) at both exec sites; scheduled-sync → jobhealth (`codebase-sync`) | `1d5d59b` |
| 3–4 | Live verification + docs | (this) |

## Live verification (Tier 3)

- **`circuit breaker wired to OpenAI embedder` appeared in the server log
  for the FIRST TIME in the project's history** (the default config's
  CachedEmbedder wrapper had silently defeated the type assertion in every
  prior deployment). `openai-embeddings` now visible in
  `/v1/admin/breakers`.
- Real API-triggered ingest job (`/v1/memory/ingest/trigger`) ran to
  completion through the resolved binary (1/1, 0 errors).
- Tier 1 pins the production wrapper shape (ratelimit(cache(provider))),
  cache-off, and bare chains; resolution order pinned (env wins, bad env
  falls through).

## The bug-class note

Both defects were the same disease as CoactivateSession's nil-injection
and the maintenance dry-run: **wiring that looks correct, compiles, and
silently does nothing under the real configuration.** The structural
fixes (interface-driven chain walking, loud else-warns, env-first
resolution) end the instances; DORMANT-CENSUS-001 (Phase 3) ends the
class.

## Follow-ups

- Docker-side e2e of the resolved exec (no container in tonight's loop;
  resolution-order unit tests + native live run cover the logic).
- The `alert_embedding_regression` RSIC complaint (`empty call_sites`) is
  RSIC-VALIDATE-001 territory (Phase 2), not an embedder-wiring issue.

## Documents Accessed

`internal/embeddings/*` (wrappers, New), `internal/api/server.go`
(both wiring sites), `handlers.go` (runIngestJob, scheduled sync),
`handlers_ingest_codebase.go`, `internal/models/models.go`
(IngestTriggerRequest), `internal/jobs/jobs.go`, roadmap.
