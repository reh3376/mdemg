# INGEST-TOPOLOGY-REPAIR-001 — Sprint Plan

## 1. Header & Metadata
Sprint: `INGEST-TOPOLOGY-REPAIR-001` · opened 2026-08-18 · branch `reh3376_dev01`
Effort: ~4-6h (ingest write + 5 recall Cypher edits + struct field + backfill script + tests + live smoke)
Risk: MEDIUM (touches production ingest path + retrieval Cypher; every guard: opt-in flag for content projection, non-destructive backfill, prod `:8102` untouched, no schema migration)

**Follows**: `CLAUDE-DOCS-INGEST-001` Rule H (initial framing) + operator architectural correction 2026-08-18: *"Fact recall is built into the topology of the mdemg substrate ... we have lost track of that fact and it needs to be reintroduced as a core set of functionality, ingest information parse data embed it in lowest layer. Review the graph topology deeply."*

**Reframes**: `CLAUDE-DOCS-CONSULT-PASSTHROUGH-001` (closed as symptom-patch of a topology-level problem).

## 2. Problem Statement

Two-agent deep review revealed MDEMG's graph topology WAS designed for fact recall (VISION.md:74 "TapRoot stores the concrete", VISION.md:434 "L0 skip connections | GROUNDED_BY edges from higher layers directly to L0 observations, preventing information loss"), but three coupled misses make verbatim content structurally invisible to retrieval:

**Miss #1 — Ingest paths diverged**:
| Path | `content` lands on |
|---|---|
| `/v1/conversation/observe` (modern; `service.go:614`) | `MemoryNode.content` directly |
| `/v1/memory/ingest` (legacy; `retrieval/service.go:1971-1988`) | Separate `Observation` node via `HAS_OBSERVATION` — `n.content` NEVER set |

**Miss #2 — Retrieval only reads `n.summary`**. All 5 recall columns (vectorRecall, BM25, graph, structural, concrete_recall) hardcode `RETURN n.summary` in their Cypher. `RetrieveResult` has NO `Content` field. `HAS_OBSERVATION` and `GROUNDED_BY` have zero read consumers.

**Miss #3 — Live-verified impact**: All 2191 CLAUDE-DOCS-INGEST-001 nodes have `n.content IS NULL` and `n.summary` = 65-char signposts ("Use Claude Code with a screen reader — Turn on screen reader mode"). The actual 2244-byte doc content lives one edge away on `Observation.content` — structurally unreachable. Retrieval surfaces the node by embedding match but returns only the signpost. Synthesis LLM then correctly says "not explicitly listed in the provided knowledge graph."

Consequences: `/v1/memory/consult`, `/v1/memory/retrieve`, Jiminy guidance, MCP `memory_recall`, browser UI — ALL content-blind for legacy-ingest nodes. This is why the operator flagged "we have lost track of the topology's designed function".

## 3. Scope & Constraints

**In scope** (3-layer architectural fix):

- **(E1) Ingest fix** — `IngestObservation` writes `n.content = $content` on the MemoryNode in both path + node_id branches (parity with `/v1/conversation/observe`). Auto-derive `n.summary = generateSummary(content)` when caller omits summary (symmetric with Observe line 356).
- **(E2) Retrieval fix** — Add `Content string` field to `retrieval.Candidate` + `models.RetrieveResult`. Extend 5 recall Cypher queries to project `coalesce(n.content, head([(n)-[:HAS_OBSERVATION]->(o) | o.content])[…])` (defense-in-depth for legacy nodes never re-ingested). Opt-in gate via `RetrieveRequest.IncludeContent bool` + `RETRIEVE_CONTENT_MAX_BYTES` env cap (default 8000). Default-off preserves current wire-size for existing consumers.
- **(E3) Backfill** — `mdemg data backfill-node-content --space-id <id> [--dry-run]` CLI. Cypher: `MATCH (n:MemoryNode)-[:HAS_OBSERVATION]->(o) WHERE (n.content IS NULL OR n.content='') WITH n, o ORDER BY o.created_at DESC WITH n, collect(o)[0] AS latest SET n.content = latest.content`. Reversible (`SET n.content = NULL`).
- **(E4) Synthesis prompt** — `buildSynthesisPrompt` renders full content when `r.Content` is present (falls back to summary otherwise). No new opt-in flag; content projection at retrieval layer means synthesis "just works" when caller enables IncludeContent.
- **(E5) Unit tests** — payload tests for IngestObservation writing n.content + summary auto-derivation + retrieval Cypher projecting content + synthesis prompt content-render path.
- **(E6) Live Tier-3** — reproduce EffortLevel query via consult with `IncludeContent` at retrieval → assert LLM synthesis returns exact `low/medium/high/xhigh/max`.
- **(E7) Sprint post + feature doc updates + arch rules pinned + PR.**

**Out of scope** (disclosed follow-ups):

- **(F1) L5→L0 skip-connection activation** — when retrieval surfaces L≥1 concept, follow `[:GROUNDED_BY|ABSTRACTS_TO*..3]->(:MemoryNode {layer:0})` to attach L0 evidence. VISION.md:434's original design promise. Complementary but separate sprint after IncludeContent lands.
- **(F2) `GET /v1/memory/nodes/{id}/content`** — low-level primitive for debug/ad-hoc. Cheap add-on but not sprint-blocking.
- **(F3) UBENCH retrieval-augmented mode** — route bench through `/v1/memory/consult` with `IncludeContent` so ingest sprints can be measured. Deferred.
- **(F4) Deterministic Observation selection** — the `[…][0]` in Cypher list comprehension picks nondeterministically; production code needs `ORDER BY o.created_at DESC LIMIT 1` semantics via subquery. Implement in E2 (not deferred — must be correct on ship).

**Constraints**:
- Production `:8102` untouched throughout
- Opt-in `IncludeContent` flag preserves default byte-identical wire behavior
- No schema migration (Neo4j properties are schema-less; adding `n.content` to existing MemoryNodes is additive)
- All 5 recall Cypher edits must preserve existing RETURN order (append content column at end)
- Backfill is tombstone-reversible + additive (never overwrites non-null n.content)
- CUIDv2 (n/a — no new IDs minted)
- Live-testing tier required for every ingest+retrieval combination

## 4. Dependencies

- `CLAUDE-DOCS-INGEST-001` shipped (2191 dark docs are the primary test bed for the backfill)
- Consulting service + LLM synthesizer shipped
- Existing `generateSummary(text, maxChars)` helper (`conversation/service.go:~356`)
- Neo4j driver access from `retrieval.Service` (already present)
- `RETRIEVE_CONTENT_MAX_BYTES` config addition to `internal/config/config.go`

## 5. Implementation Plan (sequential)

**E1 — Ingest writes `n.content` + auto-summary** (`internal/retrieval/service.go:1949-2030`)
- Add `SET n.content = $content` to the post-merge SET clause in BOTH path branch (line ~1988) AND node_id branch (line ~2029)
- Before Cypher execution, if `req.Summary == ""` and `contentToText(req.Content)` non-empty, derive summary via shared helper (extract `generateSummary` to a shared package if not already accessible, else inline)
- Preserve existing content_hash skip path (line 1906-1937) — no regression

**E2 — Retrieval `Content` field + Cypher projection** (5 files + models)
- `models.RetrieveResult`: add `Content string \`json:"content,omitempty"\``
- `retrieval.Candidate` (`service.go:1292-1314`): add `Content string`
- `models.RetrieveRequest`: add `IncludeContent bool \`json:"include_content,omitempty"\``
- Config: `RETRIEVE_CONTENT_MAX_BYTES` (default 8000)
- Extend 5 Cypher queries in `retrieval/service.go::vectorRecall` (~line 1355), `retrieval/column_bm25.go:59`, `retrieval/concrete_recall.go:119`, `retrieval/reverse_ref.go:381`, `retrieval/reflection.go:427`:
  - Add to RETURN: `coalesce(n.content, [(n)-[:HAS_OBSERVATION]->(o) | o.content][0..1][0], '') AS content`
  - Note: `[...][0..1][0]` returns first-element-or-null safely
  - Populate `Candidate.Content` in each column's row-scan
- Retrieval service: when `req.IncludeContent`, propagate `Candidate.Content` → `RetrieveResult.Content` capped at `RETRIEVE_CONTENT_MAX_BYTES`; when false, drop content client-side to preserve wire size

**E3 — Backfill CLI** (`internal/cli/backfill_node_content.go` + test)
- New `mdemg data backfill-node-content --space-id X [--dry-run] [--limit N]`
- Cypher (batched via LIMIT): `MATCH (n:MemoryNode {space_id:$s}) WHERE (n.content IS NULL OR n.content='') AND exists { MATCH (n)-[:HAS_OBSERVATION]->() } WITH n, [(n)-[:HAS_OBSERVATION]->(o) | o] AS obs UNWIND obs AS o WITH n, o ORDER BY o.created_at DESC WITH n, collect(o)[0] AS latest WHERE latest.content IS NOT NULL AND latest.content <> '' SET n.content = latest.content RETURN count(n) AS repaired`
- Post-repair: regenerate summary via same generateSummary helper if summary is empty or shorter than 100 chars (arbitrary "signpost only" heuristic)
- Report totals: repaired / already-had-content / no-observation
- Live Tier-3: on the 2191 dark claude-docs

**E4 — Synthesis renders content** (`internal/consulting/synthesis.go:220-240`)
- In the per-node RENDER block, if `r.Content != ""`, emit `- **Content**: <content-truncated-to-CONSULT_CONTENT_PASSTHROUGH_MAX_CHARS_PER_NODE-or-similar>` after Summary
- If Content present, skip Summary (avoid redundancy) OR emit both (belt-and-suspenders — decide based on prompt token budget)
- Existing byte-identical rendering when Content is absent

**E5 — Unit tests**
- `TestIngestObservation_WritesContentToMemoryNode` (both branches)
- `TestIngestObservation_AutoDerivesSummaryWhenAbsent`
- `TestVectorRecall_ProjectsContentWhenAvailable`
- `TestVectorRecall_FallsBackToObservationContent`
- `TestBuildSynthesisPrompt_RendersContentWhenPresent`
- `TestBackfillNodeContent_LiftsFromLatestObservation`

**E6 — Live Tier-3 verification**
- Rebuild `bin/mdemg`; restart production only if serve.go changed (should not — this is Cypher + struct + one method)
- Run backfill on `mdemg-dev` → expect 2191 nodes repaired
- Cypher verify: `MATCH (n) WHERE n.path STARTS WITH 'claude-docs/' RETURN count(*), count(CASE WHEN n.content IS NOT NULL AND n.content <> '' THEN 1 END)` — expect (2191, 2191)
- Retrieve with `include_content:true`: query "EffortLevel" → assert response has non-empty `content` field
- Consult with `llm_synthesis:true` (no separate content flag needed): query "What are the exact values of EffortLevel?" → assert response contains "xhigh" and "max" (which appear ONLY in the full content, not in any node name or summary)
- Retrieve WITHOUT `include_content` → assert content field is absent/empty (backward-compat pin)
- Re-run one row through `/v1/memory/ingest` → verify n.content set + summary auto-derived

**E7 — Sprint post + feature docs + arch rule updates + PR**
- Write `docs/development/ingest-topology-repair-001/sprint_post.md`
- Update `docs/features/claude-docs-substrate-ingest.md` — close CLAUDE-DOCS-CONSULT-PASSTHROUGH-001 follow-up + link to the topology fix
- New `docs/features/graph-topology-fact-recall.md` — document the L0-content-canonical + skip-connection design intent + Rule J
- Update `CLAUDE.md` pin: Rule J (fact-recall canonical topology: n.content on L0 MemoryNode is single source of truth; ingest paths converge on this; retrieval opt-in projects it)
- CHANGELOG entry
- Commit + push, add sprint summary to auto-created PR

## 6. Testing Plan (3 tiers)

**Tier 1 unit**: 6 tests per E5 above; all in `internal/retrieval/`, `internal/consulting/`, `internal/cli/`.

**Tier 2 integration**: `TestIngestAndRetrieve_ContentRoundTrip` (E2E through service layer against real embedder + mock Neo4j via `neo4jrec`).

**Tier 3 live e2e** (E6 above): real 2191 claude-docs nodes repaired; real retrieve returns content; real consult returns synthesis containing verbatim EffortLevel values.

## 7. Commit Strategy

Single squash-merge PR. Working commits on `reh3376_dev01`:
- (a) sprint dir + plan
- (b) E1 ingest fix + tests
- (c) E2 retrieval fix + tests
- (d) E3 backfill CLI + tests
- (e) E4 synthesis prompt + test
- (f) E6 live verification results + sprint post + feature docs

## 8. Verification Checklist

- [ ] E1: Every existing `IngestObservation` test still green; new content-write test green
- [ ] E2: 5 recall Cypher queries return content coalesce; `TestVectorRecall_ProjectsContentWhenAvailable` + fallback test green; opt-in flag preserves byte-identical wire when false
- [ ] E3: backfill CLI `--dry-run` on 2191 dark docs shows 2191 planned repairs; real run repairs 2191; count invariant matches
- [ ] E4: synthesis prompt renders content section when present; falls back to summary when absent
- [ ] E5: all 6 unit tests green
- [ ] E6: Cypher verify 2191/2191 claude-docs have content; consult on EffortLevel returns "xhigh" + "max" in synthesis; retrieve without flag preserves current wire
- [ ] E7: sprint post + feature docs written; Rule J pinned in CLAUDE.md; PR opened + sprint summary comment posted
- [ ] Lint: `go build ./...`, `go vet ./...`, `neural/.venv/bin/python -m ruff check` all clean
- [ ] Production `:8102` untouched throughout (verified via lsof + kickstart pid stability)

## 9. Documentation Update (final epic — never cut)

- `docs/development/ingest-topology-repair-001/sprint_post.md` — REQUIRED
- `docs/features/graph-topology-fact-recall.md` — NEW REQUIRED (per `mandatory-feature-docs` rule)
- `docs/features/claude-docs-substrate-ingest.md` — update follow-up section
- `CLAUDE.md` — pin Rule J: "Fact-recall canonical topology: `MemoryNode.content` is the single source of truth for verbatim facts. Ingest paths must converge on writing there (both `/v1/memory/ingest` and `/v1/conversation/observe`). Retrieval opt-in projects it via `IncludeContent` for verbatim workflows; defaults off to preserve wire size for the 15 shipped guidance-first LLM call sites."
- `CHANGELOG.md` — Unreleased entry

## 10. Risks & Mitigations

- **R1 (M)**: Adding `content` to recall Cypher RETURN adds bytes to every retrieval response even when `IncludeContent` is false.
  - Mitigation: strip client-side in retrieval service layer if flag is false. Cypher gains ~one column per row but the response drops it before serialization. Alternate: conditionally build the Cypher with/without the content column (two variants, one per flag state). Choose based on cleanliness of code.
- **R2 (M)**: Auto-summary derivation from long docs could produce misleading summaries.
  - Mitigation: reuse the shipped `generateSummary` helper (proven for Observe path); only fires when caller omits summary; caller can always override.
- **R3 (L)**: Backfill on 2191 nodes could stress Neo4j.
  - Mitigation: batched via `LIMIT`; CLI reports progress; --dry-run first; production untouched (mdemg-dev only for now).
- **R4 (L)**: Nondeterministic Observation pick in Cypher `[...][0]`.
  - Mitigation: use `head([(n)-[:HAS_OBSERVATION]->(o) | o.content][0..1])` which is deterministic-empty OR wrap in a subquery `CALL { WITH n MATCH (n)-[:HAS_OBSERVATION]->(o) RETURN o.content AS oc ORDER BY o.created_at DESC LIMIT 1 }`. Choose based on Cypher perf.
- **R5 (L)**: 5 recall Cypher edits + struct field bumps risk breaking column pass-through.
  - Mitigation: unit test per column; integration test end-to-end; live smoke pins the wire.

## 11. Rollback Procedures

- **Ingest fix (E1)**: `git revert <commit>` — next ingest reverts to non-writing behavior; existing rows keep content that was written during the fix window (harmless).
- **Retrieval fix (E2)**: default-off `IncludeContent` flag makes the wire byte-identical when off; setting `IncludeContent:false` reproduces pre-fix behavior. Full revert = git revert.
- **Backfill fix (E3)**: additive-only (never overwrites existing n.content). To undo: `MATCH (n:MemoryNode) WHERE n.content IS NOT NULL AND exists { MATCH (n)-[:HAS_OBSERVATION]->() } AND n.updated_at >= <backfill-run-timestamp> SET n.content = NULL` (scoped to nodes updated during the backfill window).

Zero risk to production `:8102` — no llama-server changes, no serving symlink flips, no schema migrations.

## 12. Documents Accessed

- `docs/development/ingest-topology-repair-001/sprint_plan.md` — this file
- Two-agent deep-review reports (topology-agent + ingest-agent, session-scoped)
- `VISION.md:74, 130-137, 147, 434, 574-577` — design intent for L0/L1/L5 + GROUNDED_BY skip
- `docs/features/hidden-pattern-identity.md`, `docs/features/l5-emergent-layer.md`
- `internal/retrieval/service.go:1355 (vectorRecall), 1843-2077 (IngestObservation), 1971-1988 + 2012-2029 (ingest Cypher)`
- `internal/retrieval/column_bm25.go:59`, `concrete_recall.go:119`, `reverse_ref.go:381`, `reflection.go:427` — 4 other recall column Cypher
- `internal/models/models.go:95-117 (RetrieveResult), ConsultRequest, IngestRequest`
- `internal/consulting/service.go` — `Consult()` flow at line 321 (LlmSynthesis fire); `fetchRelatedConcepts` pattern at line 356 (Neo4j read helper shape)
- `internal/consulting/synthesis.go:22-70 (interface), 85-170 (Synthesize), 220-240 (buildSynthesisPrompt render loop)`
- `internal/conversation/service.go:232-535 (Observe), 601-719 (createObservationNode)`, line 614 for direct-on-MemoryNode content write
- `internal/api/handlers.go:705-792 (handleIngest), 2393-2500 (handleConsult)`
- `internal/api/handlers_conversation.go:24-153 (handleObserve)`
- `internal/cli/space.go:384-407` — Neo4j `MATCH ... RETURN n.content` pattern reference
- `docs/development/claude-docs-ingest-001/sprint_post.md` — Rule H (initial framing; superseded by this sprint's topology-level fix)
- `docs/development/claude-docs-training-004/sprint_post.md` — Rule F (fact-recall = substrate not fine-tune)
- Live cypher-shell verifies: 2191 claude-docs nodes with `n.content IS NULL / has_obs TRUE / 2244-byte o.content`
