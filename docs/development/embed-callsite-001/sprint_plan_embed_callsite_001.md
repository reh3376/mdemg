# Sprint EMBED-CALLSITE-001 — Close the metaless-embed attribution gap

## 1. Header & Metadata

- **Sprint ID**: EMBED-CALLSITE-001
- **Sprint line**: `docs/development/embed-callsite-001/`
- **Date opened**: 2026-06-26
- **Target version**: v0.11.x (patch — telemetry-attribution bug fix)
- **Estimated effort**: ~0.5 dev-day
- **OpenAI spend**: $0 (local-only)
- **Risk level**: Low (additive metadata on existing embed paths; no behavior change to dedup/fingerprint results)

## 2. Problem Statement

The RSIC self-reflection check #28 (`internal/ape/self_reflect.go:526`) is zero-tolerance: it fires a CRITICAL `alert_embedding_regression` whenever **any** `embedding_events` row in the report window has an empty `call_site`. The alert has been firing every reflection cycle (~5–10 min). Investigation traced the empty rows to **embedding call sites that attach no `EmbeddingMeta`**, so the recorder writes a blank `call_site` (and blank `event_type`), while the recorder adapter backfills `space_id` from `defaultSpaceID`.

This is a **real attribution gap**, not a false positive. The check is a valid regression guard (it caught a gap introduced this session). The fix is to close the gap — give every recorded embed a `call_site` — not to weaken the alarm.

**Evidence (`mdemg-dev`):** empty-call_site rows were **0/day** through 2026-06-19, then **1,536 (06-23) → 2,024 (06-24) → 4,125 (06-25)**. Onset coincides exactly with the JIMINY-ACTIONABILITY-001 / Lever C work, which exercises `Guide()` (and therefore the dedup embed path) heavily.

## 3. Scope & Constraints

**In scope** — the complete metaless-embed inventory (full audit, 3 call sites / 2 files):

1. `internal/jiminy/service.go` `deduplicateItems` (line ~1455) — uses `context.Background()`; embeds every guidance item's content on every `Guide()` call. **Primary producer** (~the entire ~4k/day).
2. `internal/api/context_fingerprint.go` `derive` (line ~91) — embeds the query text metaless.
3. `internal/api/context_fingerprint.go` `getOrBuild` (line ~159) — `EmbedBatch` of catalog refs metaless.

Both fingerprint embeds are reached via `deriveQueryFingerprint` (`handlers.go:4139`), which sets no meta — so a single meta attachment there fixes #2 and #3.

**Out of scope:**
- CLI embed paths (`internal/cli/space.go`, `internal/cli/embeddings.go`) — these one-shot processes wire **no embedding recorder** (`recordEvent` early-returns on `recorder==nil`), so they never write `embedding_events` rows. Verified during audit.
- The internal embedder-chain passthroughs (`ratelimit.go`, `openai.go`, `embeddings.go`, `ollama.go`) — they inherit the caller's ctx; not independent call sites.
- Changing the zero-tolerance check itself — it is a correct guard.
- Any change to dedup/fingerprint *results* — this sprint only adds attribution metadata.

**Constraints:** sequential epics; 3 testing tiers + live Tier-3; no-hardcoding (call-site labels are stable string constants matching the existing convention `jiminy.guide`/`consult`/`retrieve`); docs are the final epic.

## 4. Dependencies

- None. Self-contained against the existing `embeddings.WithEmbeddingMeta` / `EmbeddingMeta` plumbing and the live stack for Tier-3.

## 5. Implementation Plan (sequential epics + gates)

**Epic 0 — Sprint plan (this doc).** Commit the plan.

**Epic 1 — Fix the jiminy dedup path (primary producer).**
`deduplicateItems(items)` → `deduplicateItems(ctx, spaceID, items)`; build a meta'd context
`embeddings.WithEmbeddingMeta(ctx, embeddings.EmbeddingMeta{CallSite: "jiminy.dedup", SpaceID: spaceID})`
and use it for the per-item `Embed` calls instead of `context.Background()`. Update the single caller (Guide(), line ~1001) to pass `ctx, req.SpaceID`.
**Gate:** `go build ./...` clean; Tier-1 dedup test asserts `call_site="jiminy.dedup"`.

**Epic 2 — Fix the context-fingerprint path.**
In `deriveQueryFingerprint` (`handlers.go:4139`), attach
`embeddings.WithEmbeddingMeta(ctx, embeddings.EmbeddingMeta{CallSite: "context_fingerprint", SpaceID: spaceID})`
before calling `derive`. The ctx flows through `derive`→`getOrBuild`, so both embeds (#2, #3) inherit it.
**Gate:** `go build ./...` clean.

**Epic 3 — Testing (3 tiers).** Unit (call-site assertion in dedup), integration (Guide→dedup→recorder records `jiminy.dedup`), live Tier-3 (run real Guide()/retrieve `?context=auto` against the live stack; query `embedding_events`; confirm **0 new empty call_sites** and `jiminy.dedup`/`context_fingerprint` rows appear; confirm the RSIC alert stops firing).

**Epic 4 — Data hygiene.** The ~7.7k historical empty-call_site rows (06-23..26) are pre-fix telemetry noise. Per `feedback_prune_nonconforming_data`, backfill-label them to a sentinel (`<legacy-unattributed>`) or leave as dated historical record — operator decision in the post. The check windows on recent data, so it self-clears once new rows are attributed; no destructive prune required.

**Epic 5 — Documentation (final, never cut).** Feature doc `docs/features/embedding-attribution.md` (the call_site contract: every recorded embed MUST carry an `EmbeddingMeta` with a `CallSite`); CLAUDE.md Architecture Note; CHANGELOG; `post.md`.

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit):** `internal/jiminy` — a fake recorder captures events; assert dedup embeds record `call_site="jiminy.dedup"`. Keep existing dedup result-equivalence tests green (results unchanged).
- **Tier 2 (integration):** drive `Guide()` with a stub embedder + capturing recorder; assert every recorded embed has a non-empty `call_site`.
- **Tier 3 (live):** real binary + real services. Pre/post `embedding_events` query on `mdemg-dev`: (a) new empty-call_site count = 0 after the fix; (b) `jiminy.dedup` rows present; (c) `context_fingerprint` rows present after a `?context=auto` retrieve; (d) RSIC `alert_embedding_regression` no longer fires on the next cycle.

## 7. Commit Strategy

Sequential commits on `reh3376_dev01`: Epic 0 (plan) → Epic 1 (jiminy dedup + test) → Epic 2 (fingerprint) → Epic 3 (live verification doc) → Epic 5 (docs/CHANGELOG/CLAUDE/post). Push → auto-PR.

## 8. Verification Checklist

- [ ] `deduplicateItems` records `call_site="jiminy.dedup"`
- [ ] context-fingerprint embeds record `call_site="context_fingerprint"`
- [ ] `go build ./...` + `golangci-lint run ./...` clean
- [ ] Tier-1/Tier-2 tests pass; dedup results unchanged
- [ ] Live: 0 new empty call_sites; new attributed rows visible in TSDB
- [ ] Live: RSIC `alert_embedding_regression` stops firing
- [ ] No remaining metaless `.Embed`/`.EmbedBatch` in recorder-wired code paths
- [ ] Feature doc + CLAUDE.md + CHANGELOG + post.md updated

## 9. Documentation Update — Epic 5 above

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Threading ctx into `deduplicateItems` changes dedup behavior | Low | Med | Only the embed ctx changes; cosine/threshold logic untouched; result-equivalence test |
| A metaless embed path missed in the audit | Low | Med | Full `grep` of all `.Embed`/`.EmbedBatch` across `internal/`; live post-fix query confirms 0 new empties (catches any missed path) |
| Historical empties keep the windowed check firing briefly | Low | Low | Check windows on recent rows; clears within one window once new rows attributed |

## 11. Documents Accessed

- `internal/ape/self_reflect.go` (check #28), `internal/ape/task_dispatch.go` (alert dispatch)
- `internal/tsdb/dataset_builder.go` (EmptyCallSites query)
- `internal/embeddings/embeddings.go`, `internal/embeddings/recorder.go` (recorder + meta plumbing)
- `internal/jiminy/service.go` (`deduplicateItems`, `Guide`)
- `internal/api/context_fingerprint.go`, `internal/api/handlers.go` (`deriveQueryFingerprint`)
- Live `embedding_events` TSDB (`mdemg-timescaledb-1`)
