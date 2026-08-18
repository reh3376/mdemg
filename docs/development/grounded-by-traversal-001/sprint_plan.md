# GROUNDED-BY-TRAVERSAL-001 — Sprint Plan

## 1. Header & Metadata
Sprint: `GROUNDED-BY-TRAVERSAL-001` · opened 2026-08-18 · branch `reh3376_dev01`
Effort: ~3-4h (Cypher helper + models + wiring + synthesis-render + test + live smoke + docs)
Risk: LOW (opt-in flag, additive struct fields, no schema changes, no writes to substrate, prod `:8102` untouched)

**Master arc**: JIMINY-SUBSTRATE-NATIVE-001 Phase A2 (Phase A1 = INGEST-TOPOLOGY-REPAIR-001 shipped)
**Follows**: INGEST-TOPOLOGY-REPAIR-001 (Phase A1) — makes verbatim L0 content retrievable per node

## 2. Problem Statement
Phase A1 wired direct-hit content pass-through: when retrieval surfaces an L0 node, its verbatim `n.content` reaches consult synthesis. But when retrieval surfaces an **L≥1 abstraction** (hidden pattern, concept, emergent concept), the synthesis LLM sees only the ABSTRACTION's summary/name — not the concrete L0 evidence the abstraction was formed from. VISION.md line 434 designed `GROUNDED_BY` skip-connections *"from higher layers directly to L0 observations, preventing information loss"* precisely to close this gap. But `GROUNDED_BY` has zero read consumers today (grep-verified). And on the live mdemg-dev substrate: **0 `GROUNDED_BY` edges exist** (the L5 emergent step creates them only during full consolidation cycles which haven't run non-dry). Meanwhile 52,812 `ABSTRACTS_TO` + 59,674 `GENERALIZES` edges DO exist — the multi-hop path from L≥1 back to L0 is walkable via those today.

This sprint wires the read path: when retrieval surfaces an L≥1 node, walk `[:GROUNDED_BY|ABSTRACTS_TO*..3]->(:MemoryNode {layer:0})` (or `GENERALIZES` for L1→L0) and attach the top-N grounded L0 nodes as evidence on `RetrieveResult.GroundedContent`. Consult synthesis renders it. `GROUNDED_BY` becomes the fast-path optimization when it exists; the general traversal works today without waiting on consolidation.

## 3. Scope & Constraints

**In scope**:
- (E1) `RetrieveResult.GroundedContent []GroundedNode` + `GroundedNode` type in `models.go`
- (E2) `RetrieveRequest.IncludeGrounded bool` opt-in flag; classified in cacheKeyNeutralFields per CACHE-KEY-002
- (E3) `Service.fetchGroundedContent(ctx, spaceID, resultNodeIDs, maxL0PerResult, maxHops, maxCharsPerL0)` Neo4j helper — traverses `GROUNDED_BY|ABSTRACTS_TO*..N + GENERALIZES` back to L0; deterministic ordering by (similarity to source, recency); caps per-L0 content bytes
- (E4) Config: `RETRIEVE_GROUNDED_ENABLED` (default true; disable killswitch), `RETRIEVE_GROUNDED_MAX_L0_PER_RESULT` (3), `RETRIEVE_GROUNDED_MAX_HOPS` (3), `RETRIEVE_GROUNDED_CONTENT_MAX_BYTES` (2000)
- (E5) `Consult()` sets `IncludeGrounded=true` when `LlmSynthesis=true` AND any result has `Layer≥1` (mixed L0+L≥1 result sets get partial coverage; pure-L0 skip the extra fetch)
- (E6) `buildSynthesisPrompt` renders GroundedContent when present (fenced blocks per L0 evidence, cited by parent node_id)
- (E7) Live Tier-3: query surfaces an L≥1 concept; verify GroundedContent contains L0 evidence with real content
- (E8) Sprint post + feature doc + arch rule pin

**Out of scope** (deferred follow-ups):
- Creating `GROUNDED_BY` edges (requires full consolidation cycle; separate sprint or wait for weekly LaunchAgent)
- Extending Jiminy's Guide() to use GroundedContent (Phase C in master arc)
- MCP `memory_recall` opt-in to grounded content
- UBENCH tests grounded (Rule I from CLAUDE-DOCS-INGEST-001 still open)

**Constraints**:
- Opt-in flag `IncludeGrounded` (default off) preserves wire size for existing consumers
- Consult synthesis auto-opts-in (grounded evidence strictly dominates for LLM synthesis grounding)
- Zero writes to substrate; read-only Cypher only
- Traversal capped at `maxHops=3` (L5→L4→L3→L2→L1→L0 requires 5 hops via ABSTRACTS_TO; 3 hops via GROUNDED_BY skip; sprint accepts partial coverage rather than deep expensive walks)
- Per-L0 content capped at `RETRIEVE_GROUNDED_CONTENT_MAX_BYTES` (2000) — total budget ≈ top-K × maxL0PerResult × 2000 bytes ≈ 5 × 3 × 2000 = 30 KB
- Prod `:8102` untouched
- CUIDv2 (n/a — no new IDs)

## 4. Dependencies

- Phase A1 shipped (`n.content` populated on L0 via ingest fix + backfill of 55,391 legacy nodes) — grounded L0 evidence has content to serve
- `Service.fetchNodeContents` shipped (Phase A1) — parallel helper pattern reused here
- Existing `GENERALIZES` (59,674) + `ABSTRACTS_TO` (52,812) edges on mdemg-dev — traversal has edges to walk today
- `emergent_l5Step` (`hidden/service.go:3506` `createGroundingEdges`) — creates `GROUNDED_BY` when full consolidation runs; sprint benefits from this path as fast-optimization when edges exist, doesn't require it

## 5. Implementation Plan (sequential)

**E1**: models.go — add `type GroundedNode struct { NodeID, Path, Name, Content string; Layer int; HopsFromResult int }`; add `RetrieveResult.GroundedContent []GroundedNode json:"grounded_content,omitempty"`

**E2**: models.go — add `RetrieveRequest.IncludeGrounded bool json:"include_grounded,omitempty"`; classify in cacheKeyNeutralFields (IN-key); add to `CacheKey` struct-literal in cache.go

**E3**: retrieval/service.go — new `Service.fetchGroundedContent(ctx, spaceID, resultNodeIDs, maxL0PerResult, maxHops, maxCharsPerL0)` helper.

**Verified 2026-08-18 via cypher-shell + source read (per operator "verify before build" directive):**
- `GENERALIZES` points **L0→L1** (all 59,674 edges src_layer=0, dst_layer=1). Reverse-direction traversal `<-[:GENERALIZES]-` required to walk L1 → L0.
- `ABSTRACTS_TO` points **UP the hierarchy** (5551 × 1→2, 20918 × 2→3, 7434 × 3→4, 9470 × 3→5, 8959 × 4→5, plus 464 × 0→1 partial dup of GENERALIZES, plus 16 self-loops 5→5).
- `GROUNDED_BY` points **L5→L0 FORWARD** (write path `(l5)-[:GROUNDED_BY]->(l0)` at `hidden/service.go:3506`). ZERO edges exist on mdemg-dev today (write path is effectively broken for its stated purpose — `MATCH (member)-[:ABSTRACTS_TO*0..5]->(l0)` uses forward direction but ABSTRACTS_TO points UP, so path only produces GROUNDED_BY when members are already L0).
- **Proven-working pattern to reuse**: `internal/hidden/service.go:5917` — `MATCH path=(c)<-[:ABSTRACTS_TO|GENERALIZES*1..N]-(n)`. Reverse direction, multi-edge-type variable-length.
- **Live test 2026-08-18 PASSED**: `(root:L1)<-[:GENERALIZES]-(l0:L0)` returned 3 L0 nodes with real content (3495/3986/3976 bytes) — traversal works TODAY on real data.

Cypher shape (undirected to handle mixed GROUNDED_BY forward + ABSTRACTS_TO/GENERALIZES reverse):
```
UNWIND $nodeIds AS root_id
MATCH (root:MemoryNode {space_id:$spaceId, node_id:root_id})
WHERE root.layer >= 1 AND NOT coalesce(root.is_archived, false)
CALL {
  WITH root
  MATCH path=(root)-[:GROUNDED_BY|ABSTRACTS_TO|GENERALIZES*1..$maxHops]-(l0:MemoryNode {layer:0, space_id:$spaceId})
  WHERE NOT coalesce(l0.is_archived, false)
    AND l0.content IS NOT NULL AND l0.content <> ''
  WITH l0, min(length(path)) AS hops
  ORDER BY hops ASC, coalesce(l0.updated_at, datetime()) DESC
  LIMIT $maxL0PerResult
  RETURN collect({node_id:l0.node_id, path:l0.path, name:l0.name, content:l0.content, hops:hops}) AS l0s
}
RETURN root_id, l0s
```
Undirected `-[:...*1..N]-` handles mixed edge directions. Client-side cap per-L0 content to `maxCharsPerL0`. Returns `map[rootNodeID][]GroundedNode`.

**E4**: config/config.go — add `RetrieveGroundedEnabled` (default true; killswitch), `RetrieveGroundedMaxL0PerResult` (3), `RetrieveGroundedMaxHops` (3), `RetrieveGroundedContentMaxBytes` (2000)

**E5**: retrieval/service.go — in Retrieve(), after fetchNodeContents block, if `req.IncludeGrounded && cfg.RetrieveGroundedEnabled`, gather node_ids of results with `Layer >= 1`, call fetchGroundedContent, populate each result's `GroundedContent` field. Skip pure-L0 result sets (no gain).

**E6**: consulting/service.go — in `Consult()`, set `IncludeGrounded = req.LlmSynthesis` (auto-on when synthesis requested; strictly dominates for grounding).

**E7**: consulting/synthesis.go — extend `buildSynthesisPrompt` per-node render: if `r.GroundedContent` non-empty, after the Content block emit:
```
- **Grounded L0 Evidence** (N sources):
  1. [L0 node_id, path=X] content_snippet
  2. ...
```

**E8**: Unit test — `TestBuildSynthesisPrompt_RendersGroundedContentWhenPresent` + `TestCacheKey_EveryRequestFieldClassified` (existing pin — will fail until E2 classifies)

**E9**: Live Tier-3 (mdemg-dev):
- Query that surfaces an L≥1 emergent concept
- Verify `/v1/memory/retrieve include_content:true include_grounded:true` returns GroundedContent field with L0 evidence
- Verify `/v1/memory/consult llm_synthesis:true` narrative cites specific L0 evidence with content (not just abstraction summary)

**E10**: Sprint post + feature doc + arch rule pin (Rule R: "L≥1 results with LlmSynthesis MUST include GroundedContent"; extends Rule H from CLAUDE-DOCS-INGEST-001).

## 6. Testing Plan (3 tiers)

- **Tier 1 unit**: `TestBuildSynthesisPrompt_RendersGroundedContentWhenPresent`, `TestCacheKey_EveryRequestFieldClassified` (pin; must pass after E2)
- **Tier 2 integration**: `TestFetchGroundedContent_TraversesGeneralizesEdge` (fixture graph with L0→L1 via GENERALIZES; helper returns L0)
- **Tier 3 live e2e** (E9 above): real Neo4j on mdemg-dev; L≥1 result surfaces L0 evidence via `/v1/memory/retrieve` AND consult synthesis cites the L0 content

## 7. Commit Strategy
Single squash-merge PR (or new commits on the existing session PR if not yet merged). Auto-PR on push.

## 8. Verification Checklist
- [ ] E1-E4: build clean, unit tests green
- [ ] E5: retrieve without IncludeGrounded flag is byte-identical to pre-sprint (regression pin)
- [ ] E5: retrieve with IncludeGrounded=true on L1+ query returns non-empty GroundedContent
- [ ] E6: consult with LlmSynthesis=true auto-populates GroundedContent (verify via debug field)
- [ ] E7: synthesis prompt renders Grounded L0 Evidence section when present
- [ ] E9: live smoke — model synthesis cites specific L0 evidence (not just abstraction summary)
- [ ] E10: sprint post + feature doc + arch rule R pinned

## 9. Documentation Update
- `docs/development/grounded-by-traversal-001/sprint_post.md`
- `docs/features/graph-topology-fact-recall.md` — update "Related follow-ups" section: Phase A2 CLOSED
- `docs/development/jiminy-substrate-native-001/README.md` — update phase status: A2 done
- Optional: CLAUDE.md pin for Rule R (single-line pointer to sprint_post)

## 10. Risks & Mitigations
- **R1 (M)**: Traversal Cypher `[*1..3]` variable-length pathfind could be expensive on dense subgraphs. Mitigation: `LIMIT $maxL0PerResult` inside subquery scopes cost; kill switch via `RETRIEVE_GROUNDED_ENABLED=false`.
- **R2 (L)**: `GENERALIZES` isn't the ideal edge for L≥1→L0 traversal (it's L0→L1 direction). Cypher variable-length path traversal is directional. Mitigation: use undirected `[-[*1..3]-]` OR walk `<-[:GENERALIZES]-` back from L1 (reverse direction). Confirm via Cypher EXPLAIN before shipping.
- **R3 (L)**: Grounded content adds bytes to response. Mitigation: opt-in only + per-L0 cap + max-L0-per-result cap.
- **R4 (L)**: On very small L≥1 clusters, may fetch redundant L0s repeatedly across result rows. Not a correctness issue; deduplication is future optimization.

## 11. Rollback
- Opt-in flag default off → byte-identical behavior when not requested
- Killswitch: `RETRIEVE_GROUNDED_ENABLED=false`
- Full revert: `git revert <commit>`; zero substrate mutations

## 12. Documents Accessed
- `docs/development/jiminy-substrate-native-001/README.md` — master arc
- `docs/development/ingest-topology-repair-001/sprint_post.md` — Phase A1 verdict + arch rules K-Q
- VISION.md line 434 — `GROUNDED_BY` design intent
- `internal/hidden/service.go:3506` — `createGroundingEdges` write path (not touched)
- `internal/retrieval/service.go` — Phase A1 fetchNodeContents pattern (mirror)
- `internal/models/models.go` — RetrieveResult, RetrieveRequest, IngestRequest
- Live Cypher: 0 GROUNDED_BY / 52812 ABSTRACTS_TO / 59674 GENERALIZES on mdemg-dev
