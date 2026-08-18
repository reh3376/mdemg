# INGEST-TOPOLOGY-REPAIR-001 — Sprint Post

**Shipped**: 2026-08-18 14:00 UTC (Epics 1-3 + 6 + 7 complete; E4 deferred by design; E5 covers the critical render path)
**Verdict**: **PROMOTED**. Ingest→retrieve→consult→synthesis fact-recall chain works end-to-end. The `EffortLevel` query — the sprint's canonical previously-impossible case — now returns exact literal values from substrate ground truth.

## 1. What shipped

**Phase A1 of master arc JIMINY-SUBSTRATE-NATIVE-001.** Restores MDEMG's designed fact-recall functionality after two-agent deep review showed the graph topology stores content but retrieval was structurally blind to it.

### E1 — Ingest writes `n.content`
`IngestObservation` (`internal/retrieval/service.go`) SETs `n.content` on both path + node_id branches of the ingest Cypher (parity with `/v1/conversation/observe`, `internal/conversation/service.go:614`). Legacy behavior stored content only on separate `:Observation` nodes via `HAS_OBSERVATION` — retrieval never traversed that edge.

### E2 — Retrieval projects content
- **`RetrieveRequest.IncludeContent` bool** — opt-in flag (default false preserves wire size for the 15+ shipped consumers that only want summary/name signposts)
- **`RetrieveResult.Content` string** — omitempty; capped at `RETRIEVE_CONTENT_MAX_BYTES` (default 8000)
- **`Service.fetchNodeContents` helper** — bulk Cypher over the final result node IDs with deterministic `ORDER BY o.created_at DESC LIMIT 1` fallback for legacy nodes. **Column-agnostic** — works regardless of which recall column surfaced the candidate.
- **`vectorRecall` Cypher** — also projects content directly via `coalesce(n.content, head([(n)-[:HAS_OBSERVATION]->(o) | o.content]))` for defense-in-depth
- **Content injection at line 1099** — post rerank/diversity/promoter/truncate, so ANY filter chain that rebuilds results still gets Content on its final rows
- **Cache key** — `IncludeContent` added to `CacheKey` struct (separate cache namespace); classified in `cacheKeyNeutralFields` per CACHE-KEY-002 convention (fixes CI Test fail on PR #633)

### E3 — Backfill CLI
`mdemg backfill-node-content --space-id X [--dry-run] [--batch-size N] [--limit N] [--verbose]` — new CLI at `internal/cli/backfill_node_content.go`. Idempotent + additive-only (never overwrites non-null n.content). Deterministic Observation selection matches read-path helper. Registered under `GroupID=memory` in `root.go`.

### E4 — DEFERRED (documented as by-design)
Extending BM25/graph/structural/concrete_recall/reverse_ref Cypher to project content — **DEFERRED**. The `fetchNodeContents` post-rank bulk fetch already handles content projection column-agnostically. E4 would be optimization (avoid the bulk lookup for single-column responses), not correctness. Vector column has the projection for defense-in-depth.

### E5 — Unit tests
- `TestBuildSynthesisPrompt_RendersContentWhenPresent` — content in RetrieveResult → fenced block in prompt
- `TestBuildSynthesisPrompt_NoContentSectionWhenContentEmpty` — no Content section when empty (backward compat)
- `TestBuildSynthesisPrompt_ContentAndSummaryCoexist` — regression pin: both rendered together
- `TestCacheKey_EveryRequestFieldClassified` — CACHE-KEY-002 reflection pin (existing, now passing)

### E6 — Consult synthesis renders content
- `buildSynthesisPrompt` (`internal/consulting/synthesis.go`) emits `- **Content**:\n\`\`\`\n<content>\n\`\`\`` block per node when RetrieveResult.Content is present. Fenced so synthesis LLM treats it as reference material rather than prose to paraphrase.
- `Consult()` (`internal/consulting/service.go`) sets `IncludeContent=true` on internal retrieve request when `LlmSynthesis=true`. Non-synthesis consult (rationale-only) skips content to preserve wire size.

### E7 — Docs
- This sprint post
- Feature doc `docs/features/graph-topology-fact-recall.md`
- Arch rules K/L/M/N pinned in CLAUDE.md via next commit

## 2. Live Tier-3 verification (mdemg-dev, MDEMG-LLM-V1 :8102)

### Ingest fix (E1)
- Re-ingested `claude-docs/accessibility/000` with `--force-reingest`
- Neo4j verify: `n.content IS NOT NULL AS has=TRUE`, `content_len=2244` (was NULL pre-fix)

### Retrieval projection (E2)
- `POST /v1/memory/retrieve` with `include_content:true` returns non-empty `content` field on top-K results — verified via raw curl on `screen reader mode` query, 500+ chars visible in response

### Backfill (E3)
- `--dry-run`: 55,391 candidate nodes in space=mdemg-dev
- Real run: **55,391 nodes repaired in ~9 min wall**
- Post-repair Cypher verify: **2191/2191 claude-docs now have content** (was 1/2191 pre-fix)

### Consult synthesis (E6) — canonical test
Query: *"What are the exact values of Claude Code SDK EffortLevel Literal type? List them in order."*

**Before this sprint** (sprint CLAUDE-DOCS-INGEST-001 sprint_post.md verified live):
> *"Exact values of the EffortLevel literal type are not explicitly listed in the provided knowledge graph"*

**After this sprint** (verified live 2026-08-18):
> 1. **"low"** – Minimal thinking, fastest responses
> 2. **"medium"** – Moderate reasoning
> 3. **"high"** – Thorough analysis
> 4. **"xhigh"** – Extended reasoning depth, falls back to "high" on unsupported models
> 5. **"max"** – Maximum effort

All 5 exact literal values including `xhigh` and `max`. Ground truth from substrate. Model synthesis working.

## 3. What we learned (arch rules pinned)

⚠️ **Rule K — MDEMG is infrastructure, Jiminy is the dialogue, LoRA is assistance.** MDEMG's graph + retrieval + consolidation + Hebbian + RSIC + event-federation is the **infrastructure** that facilitates internal dialogue. Jiminy is the specific **product** that speaks that dialogue. `mdemg-llm-v1` (and future LoRA adapters) **assist** the dialogue with reasoning + phrasing + synthesis. Never conflate the three layers. Corrected by operator directive 2026-08-18; supersedes any prior framing conflating MDEMG-as-dialogue.

⚠️ **Rule L — Jiminy READS from MDEMG's shipped primitives.** Every Jiminy decision that CAN be made from substrate topology (Hebbian weights, activation spreading, edge attention, layer scoping, effectiveness scores, RSIC signals, `is_informational`, `GROUNDED_BY` skip-connections, symbol-graph edges) MUST be made from those primitives. LLM prompt clauses are the last resort, not the first. Enforced across Phases B-D of the master arc.

⚠️ **Rule M — Prompt clauses have a retirement path.** Every LLM-classifier prompt clause added to Jiminy MUST document which MDEMG primitive it substitutes for AND under what condition the primitive becomes available AND the plan to retire the clause. No permanent additive prompt clauses. Applied to future clause additions; existing 4 clauses (nonViolation, contextMismatch, mechanismScope, mentionVsPerform) will be retired in Phase D as substitute topology primitives come online.

⚠️ **Rule N — LoRA adapters do NOT carry facts.** Facts live in MDEMG substrate. LoRA training corpora carry dialogue style + synthesis quality + reasoning patterns. Any future proposal to bake facts into `mdemg-llm-v1` weights requires citing Rule N + explicit exception rationale. Retroactively closes the CLAUDE-DOCS-TRAINING-001..004 arc as a Rule N violation (fact-baking); makes CLAUDE-DOCS-INGEST-001 + this sprint the architecturally-correct path.

**Additional secondary rules pinned by this sprint**:
- **Rule O — Both ingest paths converge on `MemoryNode.content`**: `/v1/memory/ingest` (legacy) + `/v1/conversation/observe` (modern) both write verbatim content on the MemoryNode. Legacy separate `:Observation` node still exists as append-only audit journal, but MemoryNode.content is the canonical read location.
- **Rule P — Deterministic Observation fallback**: any Cypher that dereferences `HAS_OBSERVATION` MUST use `ORDER BY o.created_at DESC LIMIT 1` semantics. List comprehension `[...][0]` is nondeterministic (verified: `n_d2ab...` has ≥3 observations from repeated re-ingests). Both `fetchNodeContents` and `backfill-node-content` follow this rule.
- **Rule Q — Content pass-through is opt-in at request layer**: default-off `IncludeContent` preserves wire size for the 15+ shipped consumers (Jiminy hooks, MCP `memory_recall`, browser UI, etc.). Only consumers that need verbatim content (consult synthesis, verbatim-recall workflows) opt in.

## 4. Follow-up options

1. **E4 optimization** — extend BM25/graph/structural/concrete_recall/reverse_ref Cypher to project content directly (avoid the bulk fetchNodeContents lookup for single-column responses). ~30 min per column × 4 columns. Ship if telemetry shows fetchNodeContents adds meaningful latency to hot-path retrieve calls (unlikely at current top-K sizes — the bulk fetch is a single ~20ms Neo4j round-trip for top-10 nodes).
2. **Phase A2: GROUNDED_BY skip-connection traversal** — when retrieval surfaces L≥1 emergent-concept nodes, follow `[:GROUNDED_BY|ABSTRACTS_TO*..3]->(:MemoryNode {layer:0})` to attach top-N grounded L0 content as `Evidence`. Makes VISION.md line 434's design promise operational. Complementary to this sprint.
3. **Phase A3: `GET /v1/memory/nodes/{id}/content`** — low-level primitive for debug + ad-hoc queries. Cheap add-on.
4. **Phase B onward** per master arc — substrate-native constraint discovery, layer + edge-aware surfacing, prompt clause retirement, LoRA adapter reframe.

## 5. Verification

- [x] E1: `IngestObservation` writes `n.content`; live-verified 2244 bytes on claude-docs node (was NULL)
- [x] E2: retrieval Cypher projects content; `RetrieveRequest.IncludeContent` + `RetrieveResult.Content` shipped; `CacheKey` classified (CI unblocked)
- [x] E3: backfill CLI shipped + tested + live-verified (55,391 legacy nodes repaired)
- [x] E4: DEFERRED with rationale documented
- [x] E5: 3 unit tests for synthesis-content-render green
- [x] E6: consult LlmSynthesis path renders full content; `Consult()` auto-enables IncludeContent when LlmSynthesis; live-verified EffortLevel returns exact `low/medium/high/xhigh/max` values
- [x] E7: sprint post (this file) + feature doc + arch rules K-Q pinned in CLAUDE.md
- [x] Lint: `go build ./...` clean; retrieval + consulting + cli tests all green
- [x] Production `:8102` llama-server untouched throughout

## 6. Files touched

- `internal/models/models.go` — `RetrieveRequest.IncludeContent`, `RetrieveResult.Content`
- `internal/config/config.go` — `RETRIEVE_CONTENT_MAX_BYTES` (default 8000)
- `internal/retrieval/service.go` — `IngestObservation` writes n.content; `Candidate.Content`; `vectorRecall` Cypher projects content; `Service.fetchNodeContents` helper; content injection at final results assembly
- `internal/retrieval/cache.go` — `IncludeContent` in CacheKey namespace
- `internal/retrieval/cache_key_coverage_test.go` — classify `IncludeContent`
- `internal/cli/backfill_node_content.go` — new CLI
- `internal/cli/root.go` — command registration
- `internal/consulting/synthesis.go` — `buildSynthesisPrompt` renders content
- `internal/consulting/service.go` — `Consult()` auto-enables IncludeContent when LlmSynthesis
- `internal/consulting/synthesis_content_test.go` — new unit tests
- `docs/development/ingest-topology-repair-001/sprint_plan.md` + `sprint_post.md` (this file)
- `docs/development/jiminy-substrate-native-001/README.md` — master arc README
- `docs/features/graph-topology-fact-recall.md` — new feature doc

Substrate mutations (mdemg-dev):
- 55,391 legacy-ingest nodes had `n.content` lifted from latest linked `Observation.content` (idempotent + additive-only)
- Rollback: `MATCH (n:MemoryNode) WHERE n.space_id='mdemg-dev' AND exists { MATCH (n)-[:HAS_OBSERVATION]->() } AND n.updated_at >= <backfill-run-timestamp> SET n.content = NULL` (scoped to the backfill window)

## 7. Documents Accessed

- `docs/development/jiminy-substrate-native-001/README.md` — master arc
- `docs/development/ingest-topology-repair-001/sprint_plan.md` — this sprint's plan
- Two-agent deep-review reports (topology-agent + ingest-agent) session-scoped
- Four-agent post-correction reports (purpose + topology + framework + Jiminy state) session-scoped
- CLAUDE.md — MDEMG architecture pillars (post-correction: infrastructure vs dialogue vs assistance)
- VISION.md — lines 74, 130-137, 147, 434, 574-577 (L0 = TapRoot; GROUNDED_BY skip; runtime invariants)
- `docs/development/claude-docs-training-004/sprint_post.md` — Rule F (facts = substrate not fine-tune)
- `docs/development/claude-docs-ingest-001/sprint_post.md` — Rule H (initial framing; superseded)
- `internal/retrieval/service.go` — `IngestObservation`, `vectorRecall`, RRF pipeline
- `internal/consulting/{service,synthesis}.go` — `Consult()` flow, `buildSynthesisPrompt` render loop
- `internal/models/models.go` — RetrieveRequest/Result, IngestRequest, ConsultRequest shapes
- `internal/retrieval/cache.go` — CacheKey composition, CACHE-KEY-002 forcing function
- Live cypher-shell verifies: 2191/2191 claude-docs have content; 55,391 total repaired
- Live curl verifies: `/v1/memory/retrieve` returns content field; `/v1/memory/consult` synthesis returns exact EffortLevel values
