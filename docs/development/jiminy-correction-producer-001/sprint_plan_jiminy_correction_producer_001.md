# Sprint JIMINY-CORRECTION-PRODUCER-001 — L1 correction-role node producer

## 1. Header & Metadata

- **Sprint ID:** JIMINY-CORRECTION-PRODUCER-001
- **Sprint line:** `docs/development/jiminy-correction-producer-001/`
- **Date opened:** 2026-07-17
- **Target version:** v0.11.2 (patch — additive Neo4j edge type + new consolidation phase; no schema/migration changes)
- **Estimated effort:** ~0.75 dev-day, 6 sequential epics
- **OpenAI spend:** $0 (no LLM calls added)
- **Risk level:** Low-medium — mirrors the shipped JIMINY-CORPUS-001 constraint-promotion pattern; first live run mutates the protected `mdemg-dev` space (~30 L1 nodes) and requires operator sign-off

## 2. Problem Statement

Zero `role_type='correction'` MemoryNodes exist in `mdemg-dev` (or any space). Meanwhile, **32 L0 `obs_type='correction'` observations already sit in `mdemg-dev`** — real durable rules (samples from live query):

- "CORRECTION REINFORCEMENT: never trust an unordered sample for novelty — the earlier assumption was wrong…"
- "RECURRING BUG (3rd occurrence): OpenAI chat/completions API — newer models (gpt-5.x, gpt-4.1+) REQUIRE the `max_completion_tokens`…"
- "CORRECTION: Sprint plans (and all project planning docs) belong in `docs/development/<sprint-line>/`…"
- "CORRECTION: Do not hardcode connection pool sizes…"
- "CORRECTION: Do NOT parallelize epics…"

`CreateConstraintNodes` (`internal/hidden/constraint_nodes.go`) only promotes L0 obs carrying `constraint:*` tags to L1 `role_type='constraint'`. **There is no sibling producer for corrections.** JIMINY-ROLETYPE-ADAPTER-001 wired the pipeline to carry `role_type='correction'` end-to-end and to classify surfaced items as `GuidanceCorrection` — but there's no data upstream, so the type-`correction` slot in `constraint_outcomes` has zero rows ever.

This sprint mints the missing L1 layer.

## 3. Scope & Constraints

### In scope

- **`internal/hidden/correction_nodes.go`** — `CreateCorrectionNodes(ctx, spaceID) (*CorrectionNodeResult, error)`. Mirrors `CreateConstraintNodes`:
  - Predicate: L0 `role_type='conversation_observation'` AND `obs_type='correction'` AND not already linked via `IMPLEMENTS_CORRECTION`.
  - Promote to L1 `role_type='correction'` MemoryNode with CUIDv2 (per project rule; `github.com/nrednav/cuid2`).
  - Mint `IMPLEMENTS_CORRECTION` edge from the L0 obs to the new L1 node (mirror of `IMPLEMENTS_CONSTRAINT`).
  - Propagate embedding, surprise_score, structured_data from the L0 obs where available.
  - Idempotent: re-runs do not duplicate; existing links skip.
- **`internal/hidden/correction_gate.go`** — `CorrectionPromotionGate` mirror of the shipped `ConstraintPromotionGate`:
  - Content-pattern deny-set (config-driven): `CORRECTION_PROMOTION_REJECT_PATTERNS`.
  - No `obs_type` deny-set — the promotion predicate is already gated on `obs_type='correction'`, so transient types can't reach it (unlike constraints which came in via tag).
  - Default-on: `CORRECTION_PROMOTION_ENABLED=true`.
- **Consolidation wiring** — `RunConsolidation` calls `CreateCorrectionNodes` immediately after `CreateConstraintNodes` (same phase; parallel semantic).
- **Unit + integration tests** — table-driven gate tests; producer idempotency; dual-promotion behavior; full `go test ./...` green.
- **Live Tier-3 smoke on `mdemg-dev`** — pre-run sample of the 32 L0 candidates for sanity; **operator sign-off before mutation**; consolidate; verify L1 correction nodes emerge; retrieve → observe `role_type='correction'`; Jiminy `latest` → `type='correction'`; feedback → `constraint_outcomes.guidance_type='correction'` finally lands.
- **Canonical docs** — CLAUDE.md architecture note; CHANGELOG `[Unreleased] > Added`; `docs/features/jiminy-actionability.md` correction section; `post.md`.

### Out of scope

- **Jiminy contradicted-outcome → correction bridge** (a `contradicted` outcome minting a fresh correction) — separate sprint; requires new signal semantics + operator-review gate.
- **Operator-authored correction CLI** (`mdemg concepts correction add --incorrect ... --correct ...`) — deferrable; the existing `POST /v1/conversation/correct` endpoint already creates L0 correction obs, which this sprint's producer will promote.
- **Node/edge schema changes beyond `IMPLEMENTS_CORRECTION`** — no new migrations; add the edge type in-line (Neo4j is schemaless per relationship type).
- **Retuning `JIMINY_SURFACE_ACTIONABLE_WEIGHT`, quotas, or Lever A composition** — orthogonal.

### Constraints

- **Sequential epics** (memory: `feedback_sequential_epics.md`).
- **Live Tier-3 required** (memory: `feedback_live_testing_required.md`).
- **No hardcoded literals** beyond the ontology constant `"correction"` (per JIMINY-CORPUS-001 precedent, ontology values are constants, not config).
- **CUIDv2** for new node ids (`github.com/nrednav/cuid2`).
- **Protected `mdemg-dev`**: first live run mutates ~30 nodes — operator sign-off required before executing the smoke.
- **Idempotency**: producer must be safe to re-run (`NOT (obs)-[:IMPLEMENTS_CORRECTION]->()` guard).
- **RRF-SCALE-001-safe**: no new score gate — this sprint operates at the data layer.

## 4. Dependencies

- **JIMINY-ROLETYPE-ADAPTER-001** (merged as part of PR #499) — retrieval + classifier already know how to surface + label correction items.
- **JIMINY-CORPUS-001** — the promotion-gate pattern is fresh precedent to mirror.
- Existing `POST /v1/conversation/correct` endpoint (creates L0 correction obs — the upstream data source).
- No new env vars beyond gate config; no new MCP tools; no new migrations.

## 5. Implementation Plan

### Epic 0 — Sprint plan committed
This document, on `reh3376_dev01`, after operator "go".

### Epic 1 — `CreateCorrectionNodes` producer

**File:** `internal/hidden/correction_nodes.go` (new).

Structure mirrors `constraint_nodes.go`:

- `type CorrectionNodeResult struct { Created, Updated, Linked, Rejected int }`.
- `(*Service).CreateCorrectionNodes(ctx, spaceID) (*CorrectionNodeResult, error)`.
- **Find query** (`session.ExecuteWrite` transaction):
  ```cypher
  MATCH (obs:MemoryNode {space_id: $spaceId, role_type: 'conversation_observation'})
  WHERE obs.obs_type = 'correction'
    AND NOT coalesce(obs.is_archived, false)
    AND NOT (obs)-[:IMPLEMENTS_CORRECTION]->(:MemoryNode {role_type: 'correction'})
  RETURN obs.node_id AS nodeId, obs.name AS name, obs.content AS content,
         obs.embedding AS embedding, obs.tags AS tags,
         obs.structured_data AS structuredData, obs.surprise_score AS surpriseScore
  ```
- **Gate**: for each obs, run through `CorrectionPromotionGate.Reject` (E2). Rejected → `Rejected++` and skip.
- **Promote**: mint CUIDv2 `newID`; `MERGE (c:MemoryNode {node_id:$newID, space_id:$spaceId, role_type:'correction', ...})` with created_at/updated_at, embedding, layer=1, propagated content/name.
- **Link**: `MATCH (obs) MATCH (c) MERGE (obs)-[:IMPLEMENTS_CORRECTION {created_at: datetime()}]->(c)`.
- **Return**: populated counters.

Behavior-neutral until wired in E3 (no consolidation caller yet).

### Epic 2 — `CorrectionPromotionGate`

**File:** `internal/hidden/correction_gate.go` (new).

Mirrors `ConstraintPromotionGate` API:

- `type CorrectionPromotionGate struct { enabled bool; rejectPatterns []*regexp.Regexp; minContentLen int }`.
- `NewCorrectionPromotionGate(cfg config.Config) *CorrectionPromotionGate` — reads:
  - `CORRECTION_PROMOTION_ENABLED` (bool, default `true`).
  - `CORRECTION_PROMOTION_REJECT_PATTERNS` (comma-separated regex list; default from the constraint gate's shipped defaults for consistency).
  - `CORRECTION_PROMOTION_MIN_CONTENT_LEN` (int, default 20 — a genuine correction is at least a sentence).
- `Reject(content string) (bool, string)` — returns (rejected, reason). Rejected iff enabled AND (content < min OR any pattern matches).
- Symbols added to `internal/config/config.go`: struct fields, `FromEnv` parse, struct literal in defaults.

### Epic 3 — Consolidation wiring + tests

**File:** `internal/hidden/service.go` (or wherever `RunConsolidation` lives). Add a `CreateCorrectionNodes(ctx, spaceID)` call **immediately after** `CreateConstraintNodes` in the consolidation cycle; record its result under a new phase metric label `correction_nodes`.

**Tests:**
- Unit — gate accept/reject truth table; empty-content skip; min-len rejection; a real correction sample (e.g. `max_completion_tokens gpt-5.x`) accepted.
- Unit — `CreateCorrectionNodes` idempotency: seed a Neo4j fixture with one obs; call twice; assert `Created=1` first pass, `Linked=1` (or `Rejected=0/Created=0`) second pass.
- Unit — dual-promotion: a fixture obs with `obs_type='correction'` AND `constraint:must` tag → after `CreateConstraintNodes + CreateCorrectionNodes`, TWO L1 nodes exist (one per role), both linked back to the L0 seed.

Full `go test ./...` green; `golangci-lint run ./...` clean.

### Epic 4 — Live Tier-3 smoke

**Sequence:**

1. Sample the 32 L0 correction candidates — inspect for junk vs durable-rule shape.
2. **Operator sign-off** before executing the mutation.
3. Rebuild binary; restart.
4. Trigger consolidation on `mdemg-dev` (via `/v1/consolidation/run` or a `mdemg` CLI).
5. Verify L1 correction node count:
   ```cypher
   MATCH (n:MemoryNode {space_id:'mdemg-dev', role_type:'correction'})
   WHERE NOT coalesce(n.is_archived, false)
   RETURN count(*) AS n
   ```
   Expect > 0.
6. Verify `IMPLEMENTS_CORRECTION` edges:
   ```cypher
   MATCH (:MemoryNode {obs_type:'correction'})-[r:IMPLEMENTS_CORRECTION]->(:MemoryNode {role_type:'correction'})
   RETURN count(r)
   ```
7. Retrieve with query aligned to a real correction (e.g. `max_completion_tokens gpt-5 API`) → expect `role_type='correction'` on at least one L1 hit.
8. `/v1/jiminy/warm` + `/v1/jiminy/latest` → expect `type='correction'` in surfaced items.
9. `/v1/jiminy/feedback` → `SELECT ... FROM constraint_outcomes WHERE guidance_type='correction'` → expect ≥1 row.
10. Capture before/after distribution in `docs/development/jiminy-correction-producer-001/live_verification.md`.

### Epic 5 — Canonical docs (never cut)

- **CLAUDE.md** — new architecture note "**L1 correction-role producer (Sprint JIMINY-CORRECTION-PRODUCER-001)**" under the Jiminy cluster.
- **CHANGELOG.md** — `[Unreleased] > Added` entry.
- **`docs/features/jiminy-actionability.md`** — new §"Correction producer" describing the surface; feature doc gets a "how to use" (via `POST /v1/conversation/correct`).
- **`docs/development/jiminy-correction-producer-001/post.md`** — sprint close.

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit)** — gate table; producer idempotency; dual-promotion.
- **Tier 2 (integration)** — full consolidation on a fixture graph; assert L1 counts + edges + property propagation.
- **Tier 3 (live e2e)** — the sequence above on `mdemg-dev`; before/after evidence in `live_verification.md`.

## 7. Commit Strategy

Sequential commits on `reh3376_dev01`:
1. `docs(jiminy-correction-producer-001): E0 — sprint plan`
2. `feat(jiminy-correction-producer-001): E1 — CreateCorrectionNodes producer`
3. `feat(jiminy-correction-producer-001): E2 — CorrectionPromotionGate`
4. `feat(jiminy-correction-producer-001): E3 — wire into RunConsolidation + tests`
5. `docs(jiminy-correction-producer-001): E4 — live Tier-3 verification`
6. `docs(jiminy-correction-producer-001): E5 — CLAUDE.md/CHANGELOG/feature/post`

Auto-PR fires on push. Sprint summary comment after E5.

## 8. Verification Checklist

- [ ] E0 committed
- [ ] `CreateCorrectionNodes` mirrors `CreateConstraintNodes` structure; idempotent
- [ ] `IMPLEMENTS_CORRECTION` edge type used
- [ ] `CorrectionPromotionGate` config-driven, default-on
- [ ] `RunConsolidation` invokes producer after constraint promotion
- [ ] Gate truth table + producer idempotency tests pass
- [ ] `go build ./...` clean
- [ ] `go test ./...` full-suite green
- [ ] `golangci-lint run ./...` clean
- [ ] **Operator sign-off before Tier-3 live mutation on `mdemg-dev`**
- [ ] Live: L1 correction nodes emerge (count > 0)
- [ ] Live: `POST /v1/memory/retrieve` returns `role_type='correction'` on a correction-relevant query
- [ ] Live: `constraint_outcomes.guidance_type='correction'` gains ≥1 row
- [ ] CLAUDE.md architecture note appended
- [ ] CHANGELOG entry added
- [ ] `docs/features/jiminy-actionability.md` correction section written
- [ ] `post.md` written

## 9. Documentation Update — Epic 5 above.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Dual-promotion surfaces "same" idea twice via constraint AND correction | Medium | Low | Surfacing at Lever C is role-filtered; the two nodes have distinct NodeIDs so dedup at surface passes both — that IS the intended UX (constraint before-the-fact, correction after-the-fact). If empirically noisy, tighten predicate to `obs_type='correction' AND NOT ANY tag STARTS WITH 'constraint:'` (documented as fallback). |
| Junk promotion parallel to the JIMINY-CORPUS-001 issue | Low | Medium | Gate default-on with the same content-pattern deny-set from `ConstraintPromotionGate` as a starting point. E4 samples the 32 candidates for sanity before mutation. |
| Live mutation on protected `mdemg-dev` | Medium | Medium | Operator sign-off required at E4; tombstone-only rollback path via `is_archived=true` (mirrors JIMINY-CORPUS-001 pattern). |
| `IMPLEMENTS_CORRECTION` edge type collides with an existing edge | Very Low | Low | New name; grep confirms zero occurrences. |
| Consolidation phase timing (E3 adds a phase to the ~47s cycle) | Very Low | Low | 32 candidates → small write batch; measured before/after. |
| A CorrectRequest with structured metadata (Incorrect/Correct/Context) needs richer L1 mapping | Low | Medium | Sprint scope keeps L1 content = obs content (which already renders "CORRECTION: Incorrect: X | Correct: Y | Context: Z"). Structured propagation to L1 `structured_data` is a follow-up if the phrasing needs a first-class parse for synthesis. |

## 11. Documents Accessed

- `internal/hidden/constraint_nodes.go` (mirror source)
- `internal/hidden/service.go` (`RunConsolidation` wiring site)
- `internal/hidden/constraint_gate.go` (gate mirror source)
- `internal/conversation/service.go` (`Correct` endpoint L0 producer; `ObsTypeCorrection`)
- `internal/conversation/types.go` (`ObsTypeCorrection` const)
- `internal/api/handlers_conversation.go` (`POST /v1/conversation/correct` handler)
- `internal/jiminy/retrieval_source.go` (classifier `case "correction": return GuidanceCorrection`)
- `internal/jiminy/service.go` (Lever C role-filtered query `WHERE c.role_type IN ['constraint','correction']`)
- Live Neo4j: `MATCH (n:MemoryNode {space_id:'mdemg-dev'}) WHERE n.obs_type='correction' AND NOT coalesce(n.is_archived,false) RETURN count(*)` → 32; sample of 8 rows for shape verification.
- CLAUDE.md JIMINY-ROLETYPE-ADAPTER-001 + JIMINY-CORPUS-001 architecture notes.

## 12. Rollback Procedures

Tombstone-only rollback (JIMINY-CORPUS-001 pattern):

```cypher
MATCH (c:MemoryNode {space_id:'mdemg-dev', role_type:'correction'})
WHERE c.created_at > datetime('2026-07-17T00:00:00Z')
SET c.is_archived = true, c.archive_reason = 'jiminy_correction_producer_001_rollback', c.archived_at = datetime()
```

Reversible via `is_archived=false` + remove `archive_reason`. No hard-delete. `IMPLEMENTS_CORRECTION` edges left in place — they simply target archived nodes.

## Acceptance Criteria

1. L1 `role_type='correction'` MemoryNodes exist in `mdemg-dev` (count > 0).
2. `POST /v1/memory/retrieve` returns non-empty `role_type='correction'` on a correction-relevant query.
3. `/v1/jiminy/latest` surfaces `type='correction'` items.
4. `constraint_outcomes` gains ≥1 `guidance_type='correction'` row.
5. Full test suite green; lint clean.
6. Canonical docs updated per §5.
