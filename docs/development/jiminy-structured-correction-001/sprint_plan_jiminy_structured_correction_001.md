# Sprint JIMINY-STRUCTURED-CORRECTION-001 — propagate Incorrect/Correct/Context to L1 correction nodes

## 1. Header & Metadata
- **Sprint ID:** JIMINY-STRUCTURED-CORRECTION-001
- **Sprint line:** `docs/development/jiminy-structured-correction-001/`
- **Date opened:** 2026-07-20
- **Target version:** v0.11.4 (patch — additive properties on L0 + L1 correction nodes)
- **Estimated effort:** ~0.5 dev-day, 6 sequential epics
- **OpenAI spend:** $0 (no new LLM call added)
- **Risk level:** Low — additive JSON-in-JSON persistence + additive L1 property propagation + one synthesis-prompt tweak that reads the new fields only when present

## 2. Problem Statement
`POST /v1/conversation/correct` receives `Incorrect`/`Correct`/`Context` as first-class fields — then discards the structure, joining them into a free-text content string `"CORRECTION: Incorrect: X | Correct: Y | Context: Z"` before persisting. The struct fields land in a `Metadata` map that (verified live) never reaches the graph. Downstream synthesis (Lever B directive rendering) and HITL preview must regex-parse the joined string to recover the semantic pair — brittle, and impossible for corrections whose content deviates from the template. Live-verified state on `mdemg-dev`:
- Fresh L0 correction `po2zahas8mh10ahwe0iimmoz` (E5 of JIMINY-CONTRADICTED-BRIDGE-001) has `structured_data` populated with only constraint-detector fields (`constraint_code`, `detected_constraints`) — no `correction` key.
- `metadata_*` prefix properties: `meta_keys = []` on that same node. The flattening code in `service.go:671-675` is currently dead — the CREATE cypher doesn't include those params.
- 60 L0 corrections in `mdemg-dev` (32 producer-promoted per JIMINY-CORRECTION-PRODUCER-001 E4 + fresh ones since) all missing structured `correction`.

## 3. Scope & Constraints

### In scope
- `conversation.Service.Correct` merges `{incorrect, correct, context}` into obs's `StructuredData` under a `correction` key (additive; preserves the existing constraint-detector `constraint_code`/`detected_constraints`). Investigate the `metadata_*` dead code path — either repair (add matching cypher params) or remove (with a comment explaining prior intent). Rationale disclosed either way.
- `CreateCorrectionNodes` parses `structured_data.correction` on the L0 obs and sets `correction_incorrect`, `correction_correct`, `correction_context` as top-level properties on the new L1 correction node. Absent-safe: an old-format L0 (no structured `correction`) still promotes with the three L1 fields empty.
- `mdemg corrections rehydrate-structured --space-id <id>` — one-time backfill CLI. Walks L0 correction obs whose `structured_data.correction IS NULL`, parses the joined content string via the well-known template regex `^CORRECTION: Incorrect: (.+?) \| Correct: (.+?)(?: \| Context: (.+))?$`, populates the structured fields, and (via linked `IMPLEMENTS_CORRECTION`) updates the L1 node too. Idempotent + `--dry-run` preview + batched.
- Lever B directive synthesis prompt (`internal/jiminy/guidance_prompt.go` — Sprint JIMINY-ACTIONABILITY-001): when the item carries structured `correction_correct`, prefer it as the imperative core ("Do Y") and reference `correction_incorrect` as anti-pattern context ("not X"). Gated on presence — falls back to prior behavior for pattern/constraint/learning items unchanged.
- HITL contradicted-drafts sink `Preview` text (`internal/api/contradicted_drafts_dataset.go`) surfaces the drafted pair rather than the joined blob.
- Unit + Tier-2 tests for round-trip + propagation + backfill regex.
- Live Tier-3 across all four modified surfaces.
- Canonical docs.

### Out of scope
- Neo4j schema migration (schemaless graph; properties are additive on the `MemoryNode:Correction` label).
- Rewriting historical L0 obs whose `structured_data` was populated by the constraint detector (compose additively — don't drop).
- New LLM calls (Lever B already invokes `jiminy.synthesize` — this sprint tightens the prompt, not the surface).
- Bridge draft-shape changes.

### Constraints
- Sequential epics.
- **Live Tier-3 required for every modified surface** (per the reinforced rule).
- No hardcoded literals beyond the ontology values (`"correction"`, `structured_data` key `"correction"`).
- Additive-only graph writes (no `REMOVE` / no `DELETE` / no `SET is_archived=true`).
- Idempotent backfill: re-running skips obs that already have the structured key.
- CUIDv2 unaffected.

## 4. Dependencies
- **JIMINY-CORRECTION-PRODUCER-001** (merged) — the L0→L1 producer this sprint extends with property propagation.
- **JIMINY-CONTRADICTED-BRIDGE-001** (merged) — bridge sink already provides structured drafts; this sprint makes them visible downstream.
- No new env vars.

## 5. Implementation Plan

### Epic 0 — Sprint plan committed
This document.

### Epic 1 — Correct persists structured fields
`internal/conversation/service.go::Correct` — before the `Observe(obsReq)` call, merge the correction fields into `StructuredData`:
```go
obsReq.StructuredData = map[string]any{
    "correction": map[string]any{
        "incorrect": req.Incorrect,
        "correct":   req.Correct,
        "context":   req.Context,
    },
}
```
Existing `Metadata` map is preserved (no removal — the fields are useful even if the graph doesn't persist them, for audit/logging). The dead `metadata_*` param flattening in `Observe`'s cypher path gets a short investigation note in the commit; if truly dead, remove with a comment; if latent-broken, fix by adding the params to the CREATE cypher (may reveal a separate hidden bug; disclosed).

### Epic 2 — CreateCorrectionNodes propagates to L1
`internal/hidden/correction_nodes.go`:
- Extend the find query `RETURN` list with `obs.structured_data AS structuredData`.
- Parse `structured_data` (JSON string) → look for `correction: {incorrect, correct, context}` sub-object.
- Store on `correctionObs` intermediary struct as three fields.
- Include in the CREATE cypher params + property SET (both the with-embedding and without-embedding variants).

Absent-safe: if `structured_data` doesn't have `correction`, the three fields stay empty strings; L1 node is created with empty properties (backward-compatible with old-format obs).

### Epic 3 — Backfill CLI
`internal/cli/corrections.go` (new) — `mdemg corrections rehydrate-structured`:
- Flags: `--space-id <required>`, `--dry-run` (default true), `--batch-size` (default 100).
- Query: L0 correction obs where `structured_data IS NULL OR NOT (structured_data CONTAINS '"correction"')`.
- For each: parse content via the template regex; if match, build the structured `correction` sub-object; if `structured_data` already has other keys (constraint detector), merge; if no match, log WARN and skip.
- On non-dry-run: `SET obs.structured_data = $newSd` on L0 + `SET l1.correction_incorrect/_correct/_context = ...` on any linked L1.
- Prints: {scanned, parsed, skipped_unparseable, would_write / wrote, l1_updated}.

### Epic 4 — Downstream consumers
- `internal/jiminy/guidance_prompt.go` — Lever B directive-synthesis instruction gains a conditional clause: when the item is `type=correction` and carries structured `correction_correct`, render as "Do <correction_correct> — not <correction_incorrect>". Fallback: existing behavior. Guarded on presence.
- `internal/api/contradicted_drafts_dataset.go` — sink `Preview` prefers the structured pair when present in the item Meta.

### Epic 5 — Live Tier-3
1. New: `POST /v1/conversation/correct` with structured Incorrect/Correct/Context → verify L0 obs has `structured_data.correction`.
2. Trigger consolidation → verify L1 correction node has `correction_incorrect`/`correction_correct`/`correction_context` set.
3. Backfill CLI dry-run on `mdemg-dev` → verify N parsed / N unparseable; then live run → verify structured populates on old obs; verify L1 propagation.
4. Warm + `/v1/jiminy/latest` → verify Lever B narrative uses "Do Y — not X" form when a correction is surfaced.
5. HITL Preview against the still-pending or a fresh contradicted draft → verify structured pair renders.
6. `live_verification.md`.

### Epic 6 — Canonical docs
CLAUDE.md architecture note + CHANGELOG + `docs/features/jiminy-actionability.md` §Structured-correction propagation + `post.md`.

## 6. Testing (3 tiers)
- **Tier 1** — Correct→StructuredData round-trip; L1 propagation truth table (structured present / absent / partial); regex parses the joined content in edge cases (no Context, embedded pipes in Context, trailing whitespace).
- **Tier 2** — full flow: `Correct` (real; nil Neo4j path via fixture) → intermediate assertion structured lands; producer propagates.
- **Tier 3** — live sequence in §Epic 5. All four modified surfaces exercised on real services with observable outputs.

## 7. Commit Strategy
One commit per epic on `reh3376_dev01`:
1. `docs(jiminy-structured-correction-001): E0 — sprint plan`
2. `feat(jiminy-structured-correction-001): E1 — Correct persists structured correction fields`
3. `feat(jiminy-structured-correction-001): E2 — CreateCorrectionNodes propagates structured to L1`
4. `feat(jiminy-structured-correction-001): E3 — rehydrate-structured backfill CLI`
5. `feat(jiminy-structured-correction-001): E4 — Lever B synthesis + HITL preview use structured`
6. `docs(jiminy-structured-correction-001): E5 — live Tier-3 verification`
7. `docs(jiminy-structured-correction-001): E6 — CLAUDE.md/CHANGELOG/feature/post`

Auto-PR fires; sprint summary comment after E6.

## 8. Verification Checklist
- [ ] E0 committed
- [ ] `Correct` writes `structured_data.correction = {incorrect, correct, context}`
- [ ] Existing `structured_data` fields (constraint-detector) preserved additively
- [ ] `metadata_*` dead-code status disclosed (repaired OR removed with rationale)
- [ ] `CreateCorrectionNodes` sets three L1 properties from `structured_data.correction`
- [ ] Absent-safe: old-format L0 still promotes (empty L1 fields)
- [ ] Backfill CLI dry-runs with counts; live-runs idempotently
- [ ] Backfill parses template + skips unparseable (WARN)
- [ ] Backfill updates linked L1 nodes too
- [ ] Lever B synthesis prompt reads structured fields when present
- [ ] HITL Preview surfaces the pair when present
- [ ] `go build ./...` clean
- [ ] `go test ./...` full-suite green
- [ ] `golangci-lint run` clean
- [ ] Live: new Correct → L0 + L1 have structured fields
- [ ] Live: backfill CLI populates old-format L0 corrections
- [ ] Live: Lever B narrative uses imperative "Do Y — not X" form
- [ ] Live: HITL Preview shows structured pair
- [ ] CLAUDE.md note
- [ ] CHANGELOG entry
- [ ] Feature doc section
- [ ] `post.md` written

## 9. Documentation Update — Epic 6.

## 10. Risks & Mitigations
| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Dead `metadata_*` code hints at a hidden broader gap | Medium | Low-Medium | E1 investigation surfaces the finding; if a broader Metadata-feature is broken, disclose + defer to a follow-up sprint (don't expand scope) |
| Backfill regex mis-parses hand-authored corrections | Low | Low | Dry-run preview + WARN + skip; report unparseable count; operator can `--exclude-node-ids` follow-up if needed |
| Lever B prompt tweak regresses non-correction phrasing | Very Low | Low | Guarded on `type=='correction'` + presence of `correction_correct`; unchanged behavior otherwise; live A/B is out of scope but E5 spot-check surfaces regressions |
| Existing `structured_data` constraint-detector fields silently overwritten | Low | Medium | E1 merges (doesn't replace) — preserves existing keys; unit test pins that |
| L1 nodes minted before this sprint remain without structured fields | Medium | Low | Backfill CLI updates linked L1s (via IMPLEMENTS_CORRECTION); operator can re-run at will |

## 11. Documents Accessed
- `internal/conversation/service.go::Correct` (L523+) + Observe insert path (L590+ CREATE cypher, L670+ metadata_ flatten)
- `internal/conversation/types.go` (`ObservationType`, `ObserveRequest.StructuredData`)
- `internal/hidden/correction_nodes.go` (CreateCorrectionNodes findCypher + property CREATE)
- `internal/hidden/constraint_nodes.go::structuredData` extraction pattern (mirror source)
- `internal/jiminy/guidance_prompt.go` (Lever B `directiveSynthesisInstruction`)
- `internal/api/contradicted_drafts_dataset.go` (Sink Preview text)
- Live Neo4j: sample L0 correction `po2zahas8mh10ahwe0iimmoz` shows structured_data has constraint-detector keys only; meta_keys=[] confirms dead metadata_ code
- Live count: 60 L0 correction obs in mdemg-dev, all missing structured `correction`

## 12. Rollback Procedures
- **Backfill CLI** — additive only; dry-run before write; per-space scope. Rollback: no automatic — the structured `correction` sub-object stays. If mis-populated by a regex bug, re-run backfill with a fixed regex to overwrite, OR `mdemg concepts tombstone` the corrupted obs (unlikely to be needed).
- **Producer L1 property write** — additive on the graph label; no rollback needed. A rolled-back binary that doesn't read the properties still promotes correctly.
- **Correct structured persist** — additive JSON key; a rolled-back reader ignores it.
- **Lever B synthesis prompt tweak** — code-only; revert the commit.

## Acceptance Criteria
1. `POST /v1/conversation/correct` with structured fields → L0 obs `structured_data.correction = {incorrect, correct, context}` (verified live).
2. Consolidation → L1 correction node carries `correction_incorrect`/`correction_correct`/`correction_context` as top-level properties.
3. Backfill CLI populates structured on old-format L0 corrections (60 on `mdemg-dev`) + updates linked L1s.
4. Lever B narrative surfaces "Do Y — not X" imperative form when a correction is surfaced with structured fields.
5. HITL Preview shows the structured pair.
6. Full test suite green; lint clean.
7. Canonical docs updated.
