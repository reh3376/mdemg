# EMBED-CALLSITE-002 — Sprint Post

**Sprint line**: embed-callsite-002
**Shipped**: 2026-08-14
**Prior**: EMBED-CALLSITE-001 (2026-06-26)
**Follows-from**: JIMINY-RULES-UI-001 (Epic 3 handler introduced the regression)

## 1. Problem statement

Active MEDIUM alert `rsic-alert_embedding_regression` fired at 09:53:59 UTC with reason "empty call_sites detected". Investigation traced 6 rows in `embedding_events` with `call_site=''` and `space_id='mdemg-dev'`. Row `content` matched the 6 content rewrites executed as part of JIMINY-CORPUS-AUDIT-004 via the shipped `POST /v1/jiminy/rules` endpoint. Root cause: JIMINY-RULES-UI-001 Epic 3's `doRulesCreate` handler (`internal/api/handlers_jiminy_rules.go`) called `s.embedder.Embed(r.Context(), req.Content)` on both the dedup pre-query path (line 474) and the persistent-node embed path (line 518) without wrapping the context via `embeddings.WithEmbeddingMeta(ctx, EmbeddingMeta{CallSite:"...", SpaceID:"..."})`.

This is the exact zero-tolerance class EMBED-CALLSITE-001 closed on 2026-06-26 — the RSIC self-reflect check #28 (`self_reflect.go:526`) fires `alert_embedding_regression` CRITICAL any time `EmptyCallSites > 0` in the 24h window. The regression re-opened the class in a NEW call site added by a sprint that didn't include an EMBED-CALLSITE-001 audit step.

## 2. Fix

Two-line source change in `internal/api/handlers_jiminy_rules.go::doRulesCreate`:

```go
embedCtx := embeddings.WithEmbeddingMeta(r.Context(), embeddings.EmbeddingMeta{
    CallSite: "jiminy.rules.create",
    SpaceID:  req.SpaceID,
})
// ... both Embed sites now pass embedCtx:
emb, embErr := s.embedder.Embed(embedCtx, req.Content)  // line 474 (dedup)
// ...
if e, err := s.embedder.Embed(embedCtx, req.Content); err == nil {  // line 518 (persistent)
```

Both embed calls share the same meta (same request, same content — dedup pre-query and persistent-node embed are semantically the same operation from the attribution perspective).

## 3. Pin test

`TestRulesCreate_EmbedCallSitesAttached` in `handlers_jiminy_rules_test.go` — source-string assertion that `doRulesCreate` contains the required wiring markers (`embeddings.WithEmbeddingMeta`, `CallSite: "jiminy.rules.create"`, `SpaceID:  req.SpaceID`) AND has zero remaining bare `s.embedder.Embed(r.Context(), ...)` calls. Source-string form because the recorder integration can't unit-test without a wired-up recorder + TSDB round-trip; the shape assertion catches the regression class at build time.

Pin test green: `go test ./internal/api/ -run TestRulesCreate_EmbedCallSitesAttached` → `ok mdemg/internal/api 0.488s`.

## 4. Live Tier-3 verification (mdemg-dev)

1. Rebuilt binary + re-signed (macOS gatekeeper) + launchctl kickstart
2. Temporarily set `JIMINY_RULES_UI_WRITE_ENABLED=true` via `launchctl setenv` + kickstart (arc-safe: write flag off in `.env`; env-var override for smoke only, unset after)
3. `POST /v1/jiminy/rules` with a real rule → `constraint_code=auto-iqm6h3blcemi, node_id=xmyyywk1vdzjsqljsg3c59eh`
4. Waited ~15s for buffered writer flush
5. Query `SELECT call_site, count(*) FROM embedding_events WHERE time > <ts>` → **`jiminy.rules.create=2`**, zero `<EMPTY>` rows
6. Cleaned up: `DELETE FROM embedding_events WHERE call_site='jiminy.rules.create' AND time > <ts>` (2 rows), tombstoned smoke node (`is_archived=true, archive_reason='embed_callsite_002_smoke_cleanup'`)
7. Unset `JIMINY_RULES_UI_WRITE_ENABLED` + kickstart + verified 503 restored (arc-safe state)

## 5. What did the 6 orphan rows do?

The 6 empty-call_site rows from the JIMINY-CORPUS-AUDIT-004 rewrites (executed via the same buggy code path) were left in place as historical artifact. RSIC check #28's 24h lookback window will roll them off naturally. The alert cleared on next evaluator pass (post-fix; no new empties).

## 6. Two new arch rules pinned (JIMINY-RULES-UI-001 didn't catch this — the audit step was skipped)

**Rule A (EMBED-CALLSITE class defense)**: Every new HTTP handler that calls `embedder.Embed` or `embedder.EmbedBatch` on a recorder-wired embedder MUST attach `embeddings.WithEmbeddingMeta(ctx, EmbeddingMeta{CallSite:..., SpaceID:...})` BEFORE embedding. EMBED-CALLSITE-001 closed the class in June; this sprint proves the class re-opens whenever a new call site is added without an audit step. Any PR introducing a new handler that embeds MUST add its call site to the `embedding.call_site` audit trail — this is what prevents `alert_embedding_regression` re-fires.

**Rule B (buffered-writer flush timing)**: Live verification of `embedding_events`-writer attribution requires ~10–15s after the last write for the buffered CopyFrom flush. When smoking an EMBED-CALLSITE fix, wait at least one flush interval before querying — a "no rows" result immediately after the write is a flush-timing artifact, not evidence of failure.

## 7. Follow-ups

- **None new**. The class is regression-pinned. Future sprints introducing a new embed call site: add the wiring in the same commit, add a call site to the pin test's regex list if desired (or trust the "no bare `s.embedder.Embed(r.Context()` calls" assertion at the handler level).

## 8. Files touched

- `internal/api/handlers_jiminy_rules.go` — added `"mdemg/internal/embeddings"` import + `embedCtx` wrapper + both Embed sites use it
- `internal/api/handlers_jiminy_rules_test.go` — new `TestRulesCreate_EmbedCallSitesAttached` pin
- `CHANGELOG.md` — Unreleased entry
- `CLAUDE.md` — 2 arch rules pinned in the EMBED-CALLSITE-001 note

## 9. Documents Accessed

- `internal/api/handlers_jiminy_rules.go`, `_test.go`
- `internal/embeddings/*.go` (WithEmbeddingMeta contract)
- `internal/ape/self_reflect.go` (check #28 zero-tolerance guard)
- `CLAUDE.md` (EMBED-CALLSITE-001 pin)
- Live TSDB queries against `embedding_events` on mdemg-dev
