# CLAUDE-DOCS-INGEST-001 — Sprint Plan

## 1. Header & Metadata

Sprint: `CLAUDE-DOCS-INGEST-001` · opened 2026-08-17 · branch `reh3376_dev01`
Effort: ~30 min pipeline script + ~20-40 min live ingest + ~30 min consolidation + ~15 min eval + ~1h write-up = ~3h wall
Target span: ~4-6h with waits
Risk: LOW-MEDIUM (writes to protected `mdemg-dev` space, but purely additive — new L0 nodes with unique CUIDv2 ids; existing substrate untouched; RSIC will observe + potentially consolidate; reversible via `is_archived=true` on the ingested batch)

**Follows**: `CLAUDE-DOCS-TRAINING-004` (2026-08-17) which established DO NOT PROMOTE for the LoRA path and pinned Rule F: fact-recall tasks are substrate-ingest problems in MDEMG's architecture, not model-weight-fine-tune problems.

**Prerequisites cleared**:
- `training_data/claude-docs/curated/qa.jsonl` exists (2191 rows, from `neural/training/curate_claude_docs.py`)
- `/v1/conversation/observe` endpoint live on production `:9999`
- Neo4j substrate healthy (checked via `/healthz` before Epic 2)
- RSIC + consolidation + retrieval pipelines all live

## 2. Problem Statement

MDEMG has no Claude Code CLI / Agent SDK documentation in its substrate. When queries touch Claude Code concepts (slash commands, settings keys, hook events, SDK classes, EffortLevel enum, McpServerStatusConfig type, etc.), retrieval returns nothing relevant + `mdemg-llm-v1` (14B Phase-5 SFT) hallucinates because it has no ground-truth to cite.

Sprint 004 established that LoRA fine-tuning is architecturally the wrong tool for large-corpus factual recall in MDEMG's design. MDEMG's architecture is:
- **Model weights**: reasoning, style, general knowledge (Phase-5 SFT)
- **Substrate**: specific facts, retrievable at inference via 4-5-column RRF + rerank
- **Consulting/Jiminy**: synthesizes retrieved context into task-appropriate guidance
- **RSIC**: continuously assesses substrate health + drives self-improvement across 7 dimensions

The correct tool for adding Claude Code facts is **substrate ingestion + let retrieval + consulting surface them at inference**. Zero LoRA. Zero model-weight touch. Once ingested, RSIC observes retrieval quality on Claude Code queries and can trigger consolidation, drift detection, or repair as it does for any other subject area.

## 3. Scope & Constraints

**In scope:**

- (E1) `internal/cli/claude_docs_ingest.go` — `mdemg claude-docs ingest` CLI reading `training_data/claude-docs/curated/qa.jsonl`, calling `POST /v1/conversation/observe` per row with structured metadata + `obs_type=technical_note`. Idempotent via source_sha256 dedup (skip if section already observed).
- (E2) Live ingest of all 2191 rows into space `mdemg-dev`; audit landing rate + retrieval-index integration.
- (E3) Trigger consolidation cycle — verify L1 emergent concepts form over Claude-Code cluster (e.g., "SDK classes", "hook events", "settings keys").
- (E4) Retrieval validation: 10 hand-picked Claude Code queries via `POST /v1/memory/retrieve` — verify correct L0 docs surface with reasonable rank.
- (E5) Re-run 50-row UBENCH `claude.code_knowledge` eval against ingested-substrate-backed `mdemg-llm-v1` — compare vs Sprint 004's baseline 0.379. Passes gate iff overall_mean lifts by ≥ +0.05.
- (E6) RSIC observability check: `/v1/self-improve/assess` returns without alarming; no CRITICAL/HIGH alerts fire from the new nodes.
- (E7) Sprint post + feature doc + arch rule updates.

**Out of scope:**

- Retraining any LoRA (Sprint 004 verdict stands: LoRA is wrong tool here)
- Adding new `obs_type=documentation` enum value (`technical_note` fits; enum change deferred as follow-up if it proves confusing)
- Expanding the corpus beyond the current 2191-row scrape (fresh scrape is its own sprint)
- Modifying retrieval pipeline (existing 4-5-column RRF should surface docs without new columns)
- Adding a `claude.code_knowledge` LLM-judge reward (deferred as separate sprint per Sprint 004 §5 option 4)

**Constraints:**

- Writes ONLY to `mdemg-dev` space (per `RSIC_PROTECTED_SPACES`)
- Idempotent: re-running the ingest MUST NOT create duplicate nodes (dedup on `source_sha256 + section_index` composite key)
- Ingest CLI takes `--dry-run` (per operator safety pattern)
- Ingest CLI takes `--limit N` (for staged rollout: `--limit 10` first, then 100, then full)
- All CUIDs via `cuid2.Generate()` (per shipped rule)
- All values config/env-driven (per `never-hardcode-config` rule)
- Zero touch to production `:8102` llama-server throughout
- Sprint post pins arch rule if verdict is PROMOTE

## 4. Dependencies

- **Sprint 004 outcome**: adapter_002 DO NOT PROMOTE — confirms the LoRA arc is dead; ingest is the successor
- **Corpus**: `training_data/claude-docs/curated/qa.jsonl` (2191 rows, valid_clean has 50 rows tagged `task_name=claude.code_knowledge` for eval)
- **CLI infra**: `internal/cli/root.go` command registration; parent command pattern (`mdemg claude-docs {ingest,status,rehydrate}`) mirrors `mdemg jiminy override {apply,list,revoke}` shape
- **Observe endpoint**: `/v1/conversation/observe` (shipped) with structured_data + metadata support
- **Retrieval endpoint**: `/v1/memory/retrieve` (shipped)
- **Consolidation**: automated via watchdog OR manual `POST /v1/consolidation/run`
- **UBENCH runner**: existing infra + 3 rewards implemented in Sprint 004
- **RSIC**: `/v1/self-improve/assess` for observability

## 5. Implementation Plan (sequential)

**Epic 1 — Ingest CLI** (~30 min)
New file: `internal/cli/claude_docs_ingest.go`
```
mdemg claude-docs ingest --corpus training_data/claude-docs/curated/qa.jsonl \
    [--space-id mdemg-dev] [--dry-run] [--limit N] [--force-reingest]
```
Reads JSONL row by row; for each row:
1. Compute dedup key = `source_sha256 + ":" + section_index`
2. If `--force-reingest` NOT set: check if any existing L0 node in space has `metadata.claude_docs_dedup_key == <key>` — skip if present (idempotent)
3. Build observe request:
   - `content`: `<prompt>\n\n<completion>` (Q+A together so retrieval finds by both keyword + embedding)
   - `space_id`: from flag / env
   - `session_id`: `"claude-docs-ingest"` (fixed audit trail across runs)
   - `obs_type`: `"technical_note"` (documentation-shaped facts; `constraint`/`correction` reserved)
   - `structured_data.claude_docs`: `{row_id, source_url, source_slug, doc_title, section_header, concept_type, section_index, source_sha256, curated_at_utc, word_count}` (round-trippable back to source)
   - `structured_data.claude_docs_dedup_key`: `<key>` (surfaces at metadata scan time)
   - `tags`: `["docs:claude-code", "docs:" + source_slug]` (query filtering shorthand)
4. POST to `/v1/conversation/observe`; log row_id + returned obs_id
5. Report totals: `ingested / skipped-duplicate / errored`

Config: `CLAUDE_DOCS_INGEST_ENDPOINT` (default `http://127.0.0.1:9999`), `CLAUDE_DOCS_INGEST_SESSION_ID` (default `claude-docs-ingest`), `CLAUDE_DOCS_INGEST_BATCH_DELAY_MS` (default 50).

**Epic 2 — Live ingest** (~20-40 min wall)
- Staged rollout: `--limit 10 --dry-run` → `--limit 10` real → verify 10 nodes present in `mdemg-dev` via `curl :9999/v1/memory/retrieve?query=...` → `--limit 100` → verify sample retrieval → full run
- Per-row observe should take ~100-200ms (embed + Neo4j write); 2191 rows ≈ 4-7 min wall
- Rolling log to stderr, one line per 100 rows

**Epic 3 — Consolidation** (~5-15 min wall)
- `POST /v1/consolidation/run` to trigger a synchronous cycle on the newly-ingested batch
- Expected: L0 → L1 emergent concept clusters form (e.g., "hook_events", "sdk_classes", "settings_keys" clusters)
- Verify via `/v1/self-improve/assess` — `Memory` dimension should show layer-1 node count increase

**Epic 4 — Retrieval validation** (~10 min)
Hand-picked 10 queries against `/v1/memory/retrieve`:
1. "What is EffortLevel?" — expect the SDK type-definitions doc
2. "How do I use the ClaudeSDKClient?" — expect the Python SDK reference
3. "What is the query() function?" — expect the Python SDK query function doc
4. "How do I configure screen reader mode?" — expect the accessibility doc
5. "What is McpServerStatusConfig?" — expect the MCP type doc
6. "How do hooks work in Claude Code?" — expect hooks reference
7. "What are the CLI flags?" — expect cli-reference
8. "How do I install Claude Code?" — expect setup doc
9. "What settings can I configure?" — expect settings reference
10. "How do I use custom tools with Claude?" — expect Agent SDK tools doc

Pass criterion: at least 8/10 queries return relevant Claude Code docs in top-5 results.

**Epic 5 — UBENCH re-eval** (~15-25 min wall)
- Rerun 50-row `claude.code_knowledge` eval against production `:8102` (which now has substrate access to the ingested docs via retrieval pipeline)
- Compare vs Sprint 004 baseline mean 0.3787
- **Gate: overall_mean lift ≥ +0.05** (evidence substrate ingest measurably helps)
- If lift ≥ +0.15: strong signal, promote as canonical Claude Code knowledge acquisition path
- If lift 0.05-0.15: modest but real; ship, disclose limitation
- If lift < 0.05: investigate — probably means retrieval isn't consulting Claude docs, or `mdemg-llm-v1` isn't grounding on retrieved context

⚠️ **Caveat**: UBENCH doesn't currently plumb retrieved-context into the prompt sent to the model under test. If UBENCH just fires the bare `claude.code_knowledge` prompt at `:8102` without retrieval, the model won't see the ingested docs even though they exist in the substrate. Two paths: (a) verify UBENCH goes through a MDEMG-mediated call path that includes retrieval, OR (b) if not, hand-execute 10 queries end-to-end via the actual MDEMG chat surface and grade manually. Live-verify this BEFORE running the 50-row eval.

**Epic 6 — RSIC observability** (~5 min)
- `curl :9999/v1/self-improve/assess` — expected: no CRITICAL/HIGH alerts on the ingested batch, `Memory` dimension score reflects new L0 count
- `curl :9999/healthz` clean
- `mdemg alerts list` — expect no `graph_node_drop` / `orphan_ratio_*` / `low_conversation_coverage` firings

**Epic 7 — Sprint post + feature doc + arch rule updates**
- Write `docs/development/claude-docs-ingest-001/sprint_post.md`
- Write `docs/features/claude-docs-substrate-ingest.md`
- Update CLAUDE.md pin if PROMOTE verdict (RSIC-observable fact-recall path)
- Update task list

## 6. Testing Plan (3 tiers)

**Tier 1 — Unit**: `internal/cli/claude_docs_ingest_test.go` — dedup key generation, JSONL parsing, empty-corpus safe, `--dry-run` produces zero HTTP calls, `--limit` respected.

**Tier 2 — Integration**: `TestClaudeDocsIngest_Idempotent` — capturing HTTP mock; first ingest writes N obs; second ingest with same corpus writes 0 (dedup gate works).

**Tier 3 — Live e2e** (Epic 2-6 combined): real ingest, real Neo4j writes, real retrieval query, real UBENCH, real RSIC assess. Rollback plan available (`is_archived=true` UPDATE on the batch via `structured_data.claude_docs_dedup_key IS NOT NULL`).

## 7. Commit Strategy

Single squash-merge commit at sprint close; auto-PR fires on push. Working commits on `reh3376_dev01`:
- (a) sprint dir + plan
- (b) CLI + test
- (c) sprint post + feature doc + arch rule pin

## 8. Verification Checklist

- [ ] E1: `mdemg claude-docs ingest --help` shows all flags; `--dry-run` on limit=5 prints 5 planned observes
- [ ] E2: L0 node count in `mdemg-dev` increases by ~2191 within 10 min post-ingest
- [ ] E3: L1 emergent-concept count increases by ≥5 after consolidation cycle
- [ ] E4: 8/10 hand-picked Claude Code queries surface relevant docs in top-5
- [ ] E5: 50-row UBENCH claude.code_knowledge overall_mean lifts by ≥+0.05 (or, if UBENCH doesn't route through retrieval, hand-grade 10 queries end-to-end)
- [ ] E6: `/v1/self-improve/assess` returns without new CRITICAL/HIGH alerts; `/healthz` OK
- [ ] E7: sprint_post.md exists; feature doc exists; arch rule pinned (or explicitly not pinned with rationale)
- [ ] Live smoke: hand-inspect 3 ingested nodes via `curl :9999/v1/memory/retrieve?query=<term>` and verify structured_data round-trips the source metadata

## 9. Documentation Update (final epic — never cut)

- `docs/development/claude-docs-ingest-001/sprint_post.md` — REQUIRED
- `docs/features/claude-docs-substrate-ingest.md` — REQUIRED (per `mandatory-feature-docs` rule)
- `CLAUDE.md` — pin arch rule if PROMOTE: "Fact-recall corpora ingest into MDEMG substrate as L0 technical_note observations; retrieval + consolidation + consulting + RSIC handle acquisition, abstraction, surface, and health. LoRA is for STYLE/REASONING/CALIBRATION shifts, never large-corpus fact recall (Rule F from CLAUDE-DOCS-TRAINING-004)."
- `CHANGELOG.md` — Unreleased entry: "CLAUDE-DOCS-INGEST-001: [verdict]. Claude Code corpus (2191 rows) now retrievable via MDEMG substrate; adapter arc closed as category error."

## 10. Risks & Mitigations

- **R1 (MEDIUM)**: UBENCH doesn't route through MDEMG's retrieval; ingested docs never reach the model under test → eval shows no lift even though ingest was successful.
  - Mitigation: Epic 5's caveat — verify the UBENCH call path BEFORE running the 50-row eval. If bare-prompt, do hand-graded end-to-end validation instead.
- **R2 (LOW)**: Ingest floods RSIC with new-node alerts (`graph_node_drop` inverse, `orphan_ratio` from unclustered L0 nodes).
  - Mitigation: NODE-DROP-CALIBRATION-001 already handles the increase-direction sensibly (only alerts on DROPS). Orphan-ratio might spike momentarily until consolidation runs (E3). Run E3 immediately after E2.
- **R3 (LOW)**: Ingested docs incorrectly promote to constraints (via JIMINY-CORPUS-001 `ConstraintPromotionGate`).
  - Mitigation: `obs_type=technical_note` is NOT in the constraint-promoter's admission list; docs stay at L0 documentation nodes. Verify via `MATCH (n:MemoryNode {obs_type:'technical_note'}) WHERE n.role_type='constraint' RETURN count(n)` — expect 0.
- **R4 (LOW)**: 2191 embeds in a burst starves the embedder for other production work.
  - Mitigation: batch delay (`CLAUDE_DOCS_INGEST_BATCH_DELAY_MS` default 50ms) gives ~20 req/s; runs in ~2 min but rate-limits so live production hook classify/synthesis calls stay responsive.
- **R5 (LOW)**: Structured_data payload exceeds Neo4j property size limit.
  - Mitigation: each row ~200-500 bytes structured_data; Neo4j strings support 4GB per property. Non-risk.

## 11. Rollback Procedures

- **Full rollback** (if verdict is DO NOT PROMOTE):
  ```cypher
  MATCH (n:MemoryNode)
  WHERE n.structured_data CONTAINS '"claude_docs_dedup_key"'
    AND n.space_id = 'mdemg-dev'
  SET n.is_archived = true,
      n.archive_reason = 'claude_docs_ingest_001_rollback',
      n.archived_at = datetime()
  RETURN count(n) AS archived
  ```
- **Selective rollback** (subset by source_slug): same predicate but scope to `structured_data CONTAINS '"source_slug":"<slug>"'`
- **Cleanup emergent-concept L1** (rare — if consolidation over-abstracted): `mdemg concepts repair --space-id mdemg-dev` (shipped SF-3 tool)

Zero risk to production `:8102` — no serving-symlink flips, no model changes, no LoRA loads.

## 12. Documents Accessed

- `docs/development/claude-docs-training-004/sprint_post.md` — Rule F (fact-recall = substrate-ingest not fine-tune)
- CLAUDE.md — MDEMG architecture pillars: model weights + substrate + consulting/jiminy + RSIC
- `training_data/claude-docs/curated/qa.jsonl` — 2191-row corpus source
- `internal/models/models.go` — ObsType enum (`technical_note` in allowed list)
- `internal/conversation/service.go` — `createObservationNode` shape, `structured_data` support
- `internal/api/server.go` — `/v1/conversation/observe` handler contract
- `internal/cli/ingest.go`, `internal/cli/ingest_claude_md.go` — existing ingest CLI patterns
- `internal/cli/root.go` — command registration pattern (`mdemg jiminy` parent subcommand shape)
- `internal/hidden/constraint_gate.go` — promotion gate (proves technical_note NOT admitted → docs stay L0)
- `docs/tests/ults/specs/claude_code_knowledge.ults.json` — 50-row eval target
- `training_data/eval/claude-docs-training-004/baseline_20260817.json` — 0.379 baseline reference
