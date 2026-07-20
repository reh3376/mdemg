# Sprint JIMINY-CONTRADICTED-BRIDGE-001 — contradicted-outcome → correction draft bridge

## 1. Header & Metadata

- **Sprint ID:** JIMINY-CONTRADICTED-BRIDGE-001
- **Sprint line:** `docs/development/jiminy-contradicted-bridge-001/`
- **Date opened:** 2026-07-20
- **Target version:** v0.11.3 (patch — additive TSDB hypertable + hot-path hook + HITL dataset)
- **Estimated effort:** ~1 dev-day, 6 sequential epics
- **OpenAI spend:** $0 (template-only content; no LLM calls added)
- **Risk level:** Low-medium — default-off feature flag; HITL-gated so no substrate mutation without operator approval; flip only after E5 live verification

## 2. Problem Statement

The `contradicted` outcome carries the highest-signal lesson signal in the guidance loop — the classifier has explicitly said *"the agent's action directly opposes the guidance intent"* — yet nothing captures the lesson.

**Live-verified state (2026-07-20):**
- 23 lifetime `constraint_outcomes.outcome_type='contradicted'` rows across all spaces; 3 in the last 30 days on `mdemg-dev`.
- Recent real examples: `never-start-mdemg-dbs` (2026-06-25), `must-document-before-implementation`, `lint-before-commit`, `rebase-dev-after-admin-merge`.
- Downstream: only `ApplyNegativeFeedback` weakens the guidance's source-node co-activations. The signal itself dead-ends after that.
- 0 corrections auto-produced.

This is the pipeline's last unblocked autonomous-learning slot. JIMINY-CORRECTION-PRODUCER-001 (shipped) picks up L0 `obs_type='correction'` observations and promotes them to L1 role_type='correction'. A bridge just needs to mint the L0 obs — safely.

## 3. Scope & Constraints

### In scope
- **V0030 TSDB migration** — hypertable `contradicted_correction_drafts`:
  - `id text` (CUIDv2), `time timestamptz DEFAULT now()`, `space_id text`, `guidance_id text`, `guidance_type text`, `source_node_id text`, `guidance_content text`, `action_summary text`, `similarity double precision`, `draft_incorrect text`, `draft_correct text`, `status text` (enum-checked: `pending`, `approved`, `dismissed`), `session_id text`, `action_hash text`.
  - Indexes: `(space_id, status, time DESC)` for HITL FetchCandidates, `(guidance_id, time DESC)` for dedup lookup.
  - Retention: `add_retention_policy('contradicted_correction_drafts', INTERVAL '365 days')`.
- **Buffered async writer** in `internal/tsdb/contradicted_drafts_writer.go` mirroring the constraint-outcomes writer pattern. Bounded FIFO + drop counter + `registerWriterStats`.
- **Bridge hook** in `internal/jiminy/service.go::RecordOutcome`: when `outcome==OutcomeContradicted` AND `s.cfg.JiminyContradictedBridgeEnabled==true`, emit a draft row. Dedup on `(guidance_id, action_hash)` — repeat contradictions of the same guidance don't spawn duplicates. Template-based `Incorrect`/`Correct`:
  - `draft_incorrect` = truncated `action_summary`.
  - `draft_correct` = truncated `item.Content` (the guidance body that was violated).
- **HITL dataset** `contradicted_correction_drafts` implementing `ReviewableDataset`:
  - `FetchCandidates` returns `status='pending'` rows.
  - Rubric mirrors the guidance rubric: "does this represent a durable rule worth remembering?".
  - `Sink.Apply` on **approve**: call `conversation.Service.Correct(ctx, CorrectRequest{Incorrect: draft.Incorrect, Correct: draft.Correct, Context: ...})` — creates the L0 obs. Mark draft `approved`. The next consolidation cycle promotes to L1 via `CreateCorrectionNodes` (shipped).
  - `Sink.Apply` on **reject** / dismissed verdict: mark draft `dismissed`; no substrate mutation.
  - `Sink.Reverse`: mark draft back to `pending`. The L0 obs (if created) stays — reversing a substrate mutation is out of scope; documented.
- **Config**: `JIMINY_CONTRADICTED_BRIDGE_ENABLED` (default **false** — flip in E6 after E5 live smoke); `JIMINY_CONTRADICTED_BRIDGE_WRITER_FLUSH_INTERVAL_SEC` (default 30); `REVIEW_CONTRADICTED_DATASET_ENABLED` (default true — the HITL dataset registers regardless; only the write side is flag-gated).
- **Unit + integration tests**.
- **Live Tier-3** — flag on; synthesize a contradicted outcome; verify draft persistence; approve via HITL API; verify L0 obs created; consolidation → verify L1 correction node.
- **Canonical docs** — CLAUDE.md, CHANGELOG, `docs/features/hitl-review.md`, `post.md`.

### Out of scope
- **Full-auto mode** (no HITL gate). Deferred; HITL is the safer default given JIMINY-CORPUS-001 substrate-pollution lessons.
- **LLM-synthesized draft phrasing**. Templates only for the MVP.
- **Structured correction propagation** — orthogonal.
- **Retroactive processing of the 23 historical `contradicted` rows** — the bridge fires forward-only; a backfill CLI is a follow-up.

### Constraints
- **Sequential epics** (memory: `feedback_sequential_epics.md`).
- **Live Tier-3 required** (memory: `feedback_live_testing_required.md`).
- **Default-off flag** — the writer + HITL dataset ship enabled; the *bridge hook itself* is default-off. E6 flips iff E5 verifies clean.
- **No hardcoded literals** beyond the ontology values.
- **CUIDv2** for draft ids.
- **TSDB schema version bumped** to 30 in `internal/config/config.go`.
- **RRF-SCALE-001-safe** — the bridge triggers on the outcome enum, not a score threshold.

## 4. Dependencies

- **JIMINY-CORRECTION-PRODUCER-001** (merged) — the downstream L0→L1 promoter.
- **HITL-REVIEW-001** (in `main`) — `ReviewableDataset` interface + `Registry.Register` + `Sink`/`Reverse` framework.
- Existing `POST /v1/conversation/correct` → `conversation.Service.Correct` internal method — the sink target.
- Existing V0028+V0029 writer patterns to mirror.

## 5. Implementation Plan

### Epic 0 — Sprint plan
This document, on `reh3376_dev01`.

### Epic 1 — V0030 hypertable + writer
- New migration file `migrations/V0030_contradicted_correction_drafts.sql`.
- `internal/tsdb/contradicted_drafts_writer.go` — buffered async writer, drop counter, `registerWriterStats("contradicted_drafts")`.
- `internal/tsdb/schema.go` — bump `TSDBRequiredSchemaVersion` to 30.
- `internal/config/config.go` — set `TSDBRequiredSchemaVersionDefault = 30`.
- Wired at server construction same as other buffered writers.

### Epic 2 — Bridge hook
- `internal/jiminy/service.go::RecordOutcome` — inside the per-item outcome loop, after the existing outcome-writer call, insert:
  ```go
  if outcome == OutcomeContradicted && s.contradictedDraftWriter != nil &&
      s.cfg.JiminyContradictedBridgeEnabled {
      s.contradictedDraftWriter.Enqueue(contradictedDraft{...})
  }
  ```
- Template + dedup helper in a new `internal/jiminy/contradicted_bridge.go`:
  - `Incorrect`: truncated `req.ActionSummary` (default 400 chars).
  - `Correct`: truncated `item.Content` (default 400 chars).
  - Dedup: `action_hash = sha256(guidance_id + '|' + normalize(action_summary))[:16]`. Writer checks a bounded LRU (in-process, 1000 entries) before enqueue; hit → skip.
- Config: `JiminyContradictedBridgeEnabled bool`, `JiminyContradictedBridgeWriterFlushIntervalSec int` (30), `JiminyContradictedBridgeMaxContentLen int` (400).

### Epic 3 — HITL dataset + sink
- `internal/review/contradicted_drafts_dataset.go`:
  - `type contradictedDraftsDataset struct{ pool *pgxpool.Pool; rubricVersion int; sink contradictedDraftsSink }`.
  - `ID() = "contradicted_drafts"`, `DisplayName() = "Contradicted-outcome correction drafts"`.
  - `FetchCandidates`: `SELECT ... FROM contradicted_correction_drafts WHERE space_id=$1 AND status='pending' ORDER BY time DESC LIMIT $2`.
  - `FetchItem`: single-row lookup by `id`.
- `type contradictedDraftsSink struct{ correctSvc CorrectService; pool *pgxpool.Pool }`:
  - `Apply` on approve: call `correctSvc.Correct` and mark draft `status='approved'` with `applied_at, applied_obs_id`.
  - `Apply` on dismissed: mark `status='dismissed'`.
  - `Reverse`: reset draft to `status='pending'`; L0 obs left in place (documented).
- Small `CorrectService` interface adapter for testability.
- Register in `server.go` alongside the guidance dataset registration, gated on `REVIEW_CONTRADICTED_DATASET_ENABLED` (default true).

### Epic 4 — Tier 1 + Tier 2 tests
- Writer flush pin.
- Bridge dedup (in-process LRU behavior).
- Template render (truncation, empty-content).
- Sink apply pin against a mock CorrectService.

### Epic 5 — Live Tier-3 smoke on `mdemg-dev`
1. Set `JIMINY_CONTRADICTED_BRIDGE_ENABLED=true` in `.env`.
2. Rebuild + restart mdemg.
3. Warm a guidance context; capture guidance_id; author an `action_summary` that clearly opposes a surfaced guidance item; submit via `POST /v1/jiminy/feedback`.
4. After the writer flush (~30 s), verify a row in `contradicted_correction_drafts` with `status='pending'`.
5. `GET /v1/review/candidates?dataset=contradicted_drafts&space_id=mdemg-dev` — verify the draft surfaces.
6. `POST /v1/review/grade` with `verdict=approved` — verify sink calls `Correct`; verify a new L0 `obs_type='correction'` MemoryNode; verify draft status flipped to `approved`.
7. `POST /v1/memory/consolidate` on `mdemg-dev` — verify a new L1 `role_type='correction'` node is minted.
8. Capture evidence in `live_verification.md`.

### Epic 6 — Canonical docs + default flip
- CLAUDE.md architecture note.
- CHANGELOG entry.
- `docs/features/hitl-review.md` new §Contradicted-drafts dataset.
- `post.md`.
- Flip `JIMINY_CONTRADICTED_BRIDGE_ENABLED` default false → true iff E5 verified. Document the decision explicitly in `post.md`.

## 6. Testing (3 tiers)
- **Tier 1** — 4 pins per §Epic 4.
- **Tier 2** — full write-through: bridge → writer flush → DB row → HITL FetchCandidates → sink Apply → mock CorrectService called.
- **Tier 3** — the live sequence in §Epic 5.

## 7. Commit Strategy
One commit per epic on `reh3376_dev01`:
1. `docs(jiminy-contradicted-bridge-001): E0 — sprint plan`
2. `feat(jiminy-contradicted-bridge-001): E1 — V0030 hypertable + async writer`
3. `feat(jiminy-contradicted-bridge-001): E2 — RecordOutcome bridge hook`
4. `feat(jiminy-contradicted-bridge-001): E3 — HITL dataset + Correct sink`
5. `test(jiminy-contradicted-bridge-001): E4 — unit + integration tests`
6. `docs(jiminy-contradicted-bridge-001): E5 — live Tier-3 verification`
7. `docs(jiminy-contradicted-bridge-001): E6 — CLAUDE.md/CHANGELOG/feature/post + default flip decision`

Auto-PR fires. Sprint summary comment after E6.

## 8. Verification Checklist
- [ ] E0 committed
- [ ] V0030 migration + hypertable + retention + indexes
- [ ] TSDB schema version bumped to 30 + CI validator passes
- [ ] Buffered writer with `registerWriterStats("contradicted_drafts")`
- [ ] Bridge hook gated on `JIMINY_CONTRADICTED_BRIDGE_ENABLED`
- [ ] Template + dedup (in-process LRU)
- [ ] HITL dataset registered; FetchCandidates + Sink both implemented
- [ ] Sink.Apply on approve calls `conversation.Service.Correct`
- [ ] `go build ./...` clean
- [ ] `go test ./...` full-suite green
- [ ] `golangci-lint run` clean
- [ ] Live smoke: draft persisted, approved via HITL, L0 obs created, L1 correction node emerges
- [ ] CLAUDE.md architecture note
- [ ] CHANGELOG entry
- [ ] `docs/features/hitl-review.md` §Contradicted-drafts section
- [ ] `post.md` written
- [ ] Default-flip decision documented in `post.md`

## 9. Documentation Update — Epic 6.

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| False-positive `contradicted` verdicts inject bogus drafts | Medium | Low (HITL gate) | HITL gate is the design point; operator reviews before substrate mutation. |
| Duplicate drafts on repeat violations | Medium | Low | In-process LRU dedup on `(guidance_id, action_hash)`. DB-side unique index deferred. |
| Bridge floods HITL | Very Low | Low | Live volume is ~3/mo; even a 10× spike is manageable. |
| Sink dependency on `conversation.Service.Correct` — nil-safe when `conversationSvc` unavailable | Low | Medium | Sink returns clear error; HITL surfaces it; draft stays `pending`. |
| Ships default-off; behavior change is the flip | Low | Low | E5 gates the flip on verified live evidence; E6 documents the decision. |
| Reverse path leaves the L0 obs in place | Low | Low | Documented in Sink.Reverse comment + feature doc. |

## 11. Documents Accessed
- `internal/jiminy/service.go::RecordOutcome` + `OutcomeContradicted` + existing negFeedback branch
- `internal/jiminy/types.go`
- `internal/jiminy/confidence_updater.go` (contradicted → 1.5× decay case)
- `internal/review/dataset.go` (`ReviewableDataset` interface)
- `internal/review/registry.go` (`Registry.Register`)
- `internal/review/sink_guidance.go` (existing sink pattern)
- `internal/api/server.go` (dataset registration + writer wiring pattern)
- `internal/conversation/service.go::Correct` (sink target)
- `internal/api/handlers_conversation.go`
- Live TSDB: `SELECT count(*), min(time), max(time) FROM constraint_outcomes WHERE outcome_type='contradicted'` → 23 total, 3 last-30d.

## 12. Rollback Procedures

- **Feature flag** — `JIMINY_CONTRADICTED_BRIDGE_ENABLED=false` stops all new drafts.
- **HITL dataset unregistration** — `REVIEW_CONTRADICTED_DATASET_ENABLED=false` hides the dataset from the reviewer surface.
- **Drafts DB** — `contradicted_correction_drafts` is additive; drop the retention policy or drop the table on a dev DB. Production drafts are non-destructive by design.
- **L0 obs already created by the sink** — reversible per the existing `mdemg concepts tombstone` surface.
- **Schema migration** — V0030 is additive.

## Acceptance Criteria
1. `contradicted_correction_drafts` gains a row within seconds of a live `contradicted` verdict on `mdemg-dev`.
2. HITL `/v1/review/candidates?dataset=contradicted_drafts` surfaces the pending draft.
3. Approving the draft creates a real L0 `obs_type='correction'` MemoryNode via `conversation.Service.Correct`.
4. Next consolidation cycle promotes the new L0 obs to a fresh L1 `role_type='correction'` node.
5. Full test suite green; lint clean.
6. Canonical docs updated per §5 Epic 6.
