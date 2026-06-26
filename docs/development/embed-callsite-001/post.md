# EMBED-CALLSITE-001 — Post

## Outcome

Closed the persistent CRITICAL `alert_embedding_regression` ("empty call_sites").
Root cause was a **real attribution gap**, not a false positive: three embed
call sites attached no `EmbeddingMeta`, so the recorder wrote blank `call_site`
(and blank `event_type`), while the recorder adapter backfilled `space_id` from
the default — producing space-attributed but origin-blind rows that the
zero-tolerance RSIC check #28 (`self_reflect.go:526`) correctly flagged.

## Investigation chain

1. Check fires on `report.EmbeddingDataset.EmptyCallSites > 0` (threshold 0,
   24h window per `self_assess.go:221`).
2. `EmptyCallSites` = `COUNT(*) FILTER (WHERE call_site IS NULL OR call_site='')`
   (`dataset_builder.go:255`).
3. Live: ~4k empty/day in `mdemg-dev`, **0/day before 2026-06-23** then
   1,536 → 2,024 → 4,125 (06-23/24/25). New regression.
4. Empty rows had empty `event_type` too ⇒ no `EmbeddingMeta` attached
   (not the `CallSite="ingest"` path, which sets `event_type`).
5. Producer = `deduplicateItems` (`jiminy/service.go:1455`): `context.Background()`,
   embeds every guidance item's content per `Guide()`. Onset matched the
   Lever C / jiminy-actionability work that drove `Guide()` volume.
6. Full audit of every `.Embed`/`.EmbedBatch` in `internal/` ⇒ exactly 3
   metaless recorder-wired sites (dedup + 2 context-fingerprint); CLI paths
   wire no embedding recorder (excluded).

## Changes

- `internal/jiminy/service.go` — `deduplicateItems(items)` →
  `deduplicateItems(ctx, spaceID, items)`; attaches
  `EmbeddingMeta{CallSite:"jiminy.dedup", SpaceID:spaceID}`; caller in `Guide()`
  passes `ctx, req.SpaceID`.
- `internal/api/handlers.go` — `deriveQueryFingerprint` attaches
  `EmbeddingMeta{CallSite:"context_fingerprint", SpaceID:spaceID}`; both
  fingerprint embeds (`derive`, `getOrBuild`) inherit it via ctx.
- `internal/jiminy/service_test.go` — updated existing test signature; added
  `TestDeduplicateItems_AttributesCallSite` (capturing embedder asserts
  `call_site="jiminy.dedup"`, `space_id` threaded).

## Testing

- **Tier 1/2:** `go test ./internal/jiminy/ ./internal/api/` green; new
  call-site-assertion test passes; dedup results unchanged.
- **Tier 3 (live):** rebuilt binary, restarted server, drove 4 `Guide()` +
  2 `?context=auto` retrieves. Post-fix `embedding_events` (325 new rows):
  **0 empty call_sites**; `context_fingerprint` 242, `jiminy.dedup` 18,
  plus the usual attributed sites. Lint `0 issues`.

## Data hygiene (Epic 4)

Relabeled pre-fix empties in the 24h alert window to the sentinel
`legacy-unattributed` (4,168 rows; non-destructive UPDATE, reversible,
preserves provenance) → check window now 0 empties, alert clears next cycle.
Remaining ~32.8k empties live in **compressed** chunks outside the window
(UPDATE hit `tuple decompression limit exceeded` at 2.7M tuples); left to age
out via 90d retention rather than force-decompressing — they cannot trip the
24h-windowed guard.

## Follow-ups

- None required. The `call_site` contract is documented in
  `docs/features/embedding-attribution.md` for future embed call sites.
- (Optional) A CI grep guard that flags new `embedder.Embed(` calls lacking a
  nearby `WithEmbeddingMeta` could prevent regressions — not built (the live
  zero-empty query + the RSIC guard already catch it post-merge).

## Documents Accessed

- `internal/ape/self_reflect.go`, `internal/ape/self_assess.go`,
  `internal/ape/task_dispatch.go`
- `internal/tsdb/dataset_builder.go`
- `internal/embeddings/embeddings.go`, `recorder.go`
- `internal/jiminy/service.go`, `internal/api/handlers.go`,
  `internal/api/context_fingerprint.go`
- Live `embedding_events` TSDB (`mdemg-timescaledb-1`)
