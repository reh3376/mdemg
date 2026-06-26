# Embedding Attribution — the `call_site` contract (EMBED-CALLSITE-001)

## Why

Every embedding the system generates is recorded to the TSDB `embedding_events`
hypertable for cost/coverage telemetry and as a training-data source. Each row
carries a **`call_site`** — the logical origin of the embed (`retrieve`,
`consult`, `jiminy.guide`, `ingest`, …). An RSIC self-reflection check
(`internal/ape/self_reflect.go` #28) is a **zero-tolerance regression guard**:
it fires a CRITICAL `alert_embedding_regression` whenever *any* row in the
24h window has an empty `call_site`. An unattributed embed means the pipeline
silently dropped provenance — exactly the regression the guard exists to catch.

EMBED-CALLSITE-001 closed the last metaless embed paths that were tripping it.

## How it works

The recorder (`internal/embeddings/embeddings.go::recordEvent`) reads the
embedding metadata attached to the **context** via
`embeddings.WithEmbeddingMeta(ctx, EmbeddingMeta{...})`. If no meta is
attached, the recorded row has an empty `call_site` *and* empty `event_type`;
the recorder adapter still backfills `space_id` from `defaultSpaceID`, which is
why empty-call_site rows look space-attributed but origin-blind.

**The contract: every code path that calls `embedder.Embed`/`EmbedBatch` on a
recorder-wired embedder MUST attach an `EmbeddingMeta` with a non-empty
`CallSite` before embedding.** The label is a stable string constant matching
the existing convention (dotted scope, e.g. `jiminy.dedup`).

Paths that intentionally do not record (one-shot CLI commands that wire no
embedding recorder; health-check probes via `SkipRecording: true`) are exempt
because they never write a row.

## What EMBED-CALLSITE-001 fixed

Three metaless call sites (audited as the complete set):

| Call site | Path | New `call_site` |
|---|---|---|
| `internal/jiminy/service.go::deduplicateItems` | semantic dedup of every guidance item on each `Guide()` — was `context.Background()` | `jiminy.dedup` |
| `internal/api/context_fingerprint.go::derive` | query-text embed for `?context=auto` | `context_fingerprint` |
| `internal/api/context_fingerprint.go::getOrBuild` | catalog-ref `EmbedBatch` during fingerprint catalog build | `context_fingerprint` |

The two fingerprint embeds are both reached through
`deriveQueryFingerprint` (`handlers.go`), so a single meta attachment there
attributes both.

The dedup path was the dominant producer (~4k empty rows/day in `mdemg-dev`)
because each `Guide()` re-embeds the content of every retrieved guidance item;
the onset (2026-06-23) coincided with JIMINY-ACTIONABILITY-001 / Lever C
exercising `Guide()` heavily.

## How to verify

```sql
-- after the fix, no recently-recorded embed should be unattributed:
SELECT COALESCE(NULLIF(call_site,''),'<EMPTY>') AS cs, count(*)
FROM embedding_events
WHERE space_id='mdemg-dev' AND time > now() - interval '1 hour'
GROUP BY 1 ORDER BY 2 DESC;
-- expect: jiminy.dedup, context_fingerprint, jiminy.guide, … and NO <EMPTY>.
```

Live Tier-3 result at ship: 325 post-fix rows, **0 empty call_sites**;
`context_fingerprint` (242) and `jiminy.dedup` (18) now present.

## Historical data

Pre-fix empty rows in the alert window were relabeled to the honest sentinel
`legacy-unattributed` (non-destructive UPDATE — preserves the row, marks it
as a known pre-fix unattributed embed), which clears the 24h check
immediately. Older empties live in **compressed** TSDB chunks outside the
check window; they are left to age out via the 90d retention policy
(TSDB-CONSUME-001) rather than force-decompressing millions of tuples to
relabel telemetry that no longer affects the guard.

## When adding a new embed call site

Attach an `EmbeddingMeta` with a unique, descriptive `CallSite` (and `SpaceID`)
to the context **before** calling the embedder. If the call genuinely should
not be recorded, set `SkipRecording: true` instead — never leave the meta off,
or the regression guard will (correctly) fire.
