# GO-IMPLEMENTS-002 — Sprint Post

**Date:** 2026-08-05 | **Branch:** `reh3376_dev01`
**Trigger:** GO-IMPLEMENTS-001 disclosed follow-up (Q4 backlog): "audit the 267→188 emitted-vs-landed gap (79 pairs' symbols aren't in the SymbolNode graph); disposition: document + accept, or widen ingest."

## Verdict

Shipped. Gap resolved 96% at source via a small analyzer-side filter for generated protobuf code; residual 3 pairs are a stale-hash-formula class remediable by operational re-ingest (no code change). Diagnostic tooling shipped so future audits are one CLI invocation.

## Investigation

Pre-sprint: **83 discovered pairs failed to land** as `IMPLEMENTS` edges (269 discovered, 194 landed). Root-caused via `--dump-pairs` diagnostic + Cypher gap-diff:

| Class | Count | Character |
|---|---|---|
| **dst-missing** (target interface not in SymbolNode) | 54 | 10 unique interfaces, ALL from `/api/modulepb/mdemg-module_grpc.pb.go` — gRPC-generated `*Server` and `Unsafe*Server` wire types |
| **both-missing** (src + dst both absent) | 25 | 44/50 endpoints in `/api/**` — same generated-protobuf class |
| **src-missing** (concrete not in graph) | 2 | `LocalExecutor` + `RemoteExecutor` — real production code |
| **both-present-no-edge** (should have landed) | 2 | `contradictedDraftsSink` + `Dispatcher` — added TODAY in HITL-AUTO-DISMISS-001; symbol-ingest ran, analyzer hadn't been re-run |

**Root cause of the 79 generated-code pairs**: `internal/symbols/go_types.go::AnalyzeImplements` walked ALL `types.Named` in ALL packages, including protobuf-generated files that tree-sitter ingest correctly excludes from `SymbolNode`. The analyzer emitted pairs whose targets/sources didn't exist in the graph → `SaveRelationships` MATCH silently dropped them.

**Root cause of the 2 `LocalExecutor`/`RemoteExecutor` pairs (deeper finding)**: Neo4j's `SymbolNode.symbol_id` for these pre-refactor symbols was hashed with the OLD formula `sha256("space|path|name|line")`. Current `GenerateSymbolID` deliberately excludes line (per the "same symbol at different lines is the same symbol" comment) — verified by reverse-engineering the hash:
```
sha256("mdemg-dev|/internal/cli/executor_local.go|LocalExecutor|12")[:32] = 15bc062b591c78aa7ae65ee12dd7990c  ← what Neo4j has
sha256("mdemg-dev|/internal/cli/executor_local.go|LocalExecutor")[:32]    = 6e7c3ee4a3ccdad14d80c8664cbe3a99  ← what the analyzer computes
```
The `extract_symbols.go:208` caller still passes `sym.Line`, but the current `GenerateSymbolID` signature (`_ int`) IGNORES it — so post-refactor writes use the new formula while pre-refactor SymbolNodes still carry old-formula IDs. Match fails for legacy nodes.

## What shipped

### `internal/symbols/go_types.go` — source-side filter (Fix 1)
```go
if isGeneratedProtobuf(filePath) {
    continue
}

func isGeneratedProtobuf(filePath string) bool {
    return strings.HasSuffix(filePath, ".pb.go") ||
        strings.HasSuffix(filePath, "_grpc.pb.go")
}
```
Placed alongside the existing `isVendored` filter in the type-collection loop. Attacks the 79 generated-code pairs at source — the analyzer never even emits them, so no phantom "MATCH found 0" drops in `SaveRelationships`.

### `internal/symbols/relationships.go` — diagnostic fields on `RelationshipRecord`
Added optional non-persisted fields: `SourceName`, `SourcePath`, `TargetName`, `TargetPath`. The writer (`SaveRelationships`) only reads the two symbol_ids; the diagnostic breadcrumbs are carried in-memory so the CLI can name what dropped without a second Neo4j round-trip.

### `internal/cli/analyze_go_implements.go` — `--dump-pairs` flag
```
mdemg symbols analyze-go-implements --root . --space-id mdemg-dev --dry-run \
  --dump-pairs /tmp/discovered.tsv
```
Writes ALL discovered pairs as TSV with header + 6 columns (`source_id`, `source_name`, `source_path`, `target_id`, `target_name`, `target_path`). Enables one-liner gap audit: `comm` or `join` the TSV against a Neo4j `MATCH ()-[:IMPLEMENTS]-()` dump.

## Tests

2 new pin tests in `internal/symbols/go_types_pb_filter_test.go`:
- `TestIsGeneratedProtobuf_MatchesExpectedShapes` — 8 cases: `.pb.go` / `_grpc.pb.go` matches; regular `.go` doesn't; edge case `api/pb/helper.go` (not the suffix) doesn't match
- `TestIsGeneratedProtobuf_EmptyPath` — defensive: no crash on empty input

`go test ./internal/symbols/...` clean.

## Live Tier-3 (mdemg-dev)

**Pre-sprint** (baseline): discovered 269, landed 194, gap **83**.

**Post-sprint** (with filter, live re-run):
```
INFO go/types collected named types interfaces=85 concretes=1356    (was 111 / 1475)
INFO go/types IMPLEMENTS analysis complete pairs_emitted=169         (was 269)
INFO saved relationships count=169 types=1 space_id=mdemg-dev
```

**Gap re-computation**: discovered 169, landed 196 (MERGE preserved existing), gap **3**:
```
RemoteExecutor  (/internal/cli/executor_remote.go) -> Executor (/internal/sidecar/executor.go)
LocalExecutor   (/internal/cli/executor_local.go)  -> Executor (/internal/sidecar/executor.go)
DatasetBuilder  (/internal/tsdb/dataset_builder.go) -> DatasetProvider (same file)
```

**All 3 remaining are the stale-hash-formula class** — remediation is operational (`mdemg extract-symbols` full re-ingest), not code.

## Rules pinned

⚠️ **Symbol analyzers must apply the SAME file-set filters the symbol ingest applies** — emitting pairs against symbols the ingest excludes silently drops them at `SaveRelationships` MATCH and creates operator confusion (the analyzer says "wrote 269"; the graph shows 194). New filters added to the ingest side MUST have a mirror on the analyzer side. For the tree-sitter Go ingest today: `.pb.go`, `_grpc.pb.go`, vendored, module-cache, stdlib — all four now filtered on both sides.

⚠️ **`GenerateSymbolID` is a versioned contract; the current formula excludes `line`** (per the "same symbol at different lines is the same symbol" comment). Any pre-refactor SymbolNode written under the old (line-including) formula produces a different hash under the current formula → cross-tool references break silently. When making hash-formula changes to identifier functions like this, a full re-ingest is REQUIRED to migrate the substrate; there is no in-place UPDATE (the old + new IDs collide with different data). Document the re-ingest as the migration op in the same commit that changes the formula. (The change that dropped `line` from the hash predates GO-IMPLEMENTS-002; this sprint just diagnoses the resulting staleness.)

⚠️ **Diagnostic dumps for symbol-graph audits MUST carry name + path alongside symbol_id** — a bare-ID dump like `source_id|target_id` is useless for gap-audit; the operator (or agent) can't name what dropped. The `RelationshipRecord.Source*` / `.Target*` fields are non-persisted breadcrumbs that make audit tractable without a Neo4j reverse-lookup per row.

## Not shipped (intentional)

- **Full `mdemg extract-symbols` re-ingest** to fix the 3 stale-hash pairs — that's the operational remediation, not a code change. Recipe: `mdemg extract-symbols --root . --space-id mdemg-dev` (bulk overwrite; SymbolNodes MERGE on `(space_id, symbol_id)` — but since the OLD SymbolNodes have old-formula IDs, they'll persist as duplicates rather than update in place). Real cleanup requires a two-step: (a) full re-ingest with new formula; (b) `graph repair --space-id mdemg-dev` to sweep the orphaned old-formula nodes. Deferred — operator should schedule the re-ingest during a maintenance window; 3 pairs isn't worth an immediate service disruption.
- **Purge stale post-filter edges** (`IMPLEMENTS` edges previously created against generated-code symbols) — the filter drops NEW emissions, but historical edges remain in Neo4j (MERGE is one-way). Could be cleaned via `MATCH (s)-[r:IMPLEMENTS]->(t) WHERE s.file_path ENDS WITH '.pb.go' OR t.file_path ENDS WITH '.pb.go' DELETE r`. Deferred — those edges are semantically correct (a `*Server` genuinely does implement its wire interface), just noisy in the graph. Operator can run the cleanup if the noise bothers them.
- **`grpc.pb.go` extension** — the current filter matches both `.pb.go` and `_grpc.pb.go`. If protobuf-plugins ship files with new suffixes (`.pb.gw.go` for grpc-gateway, `.pb.validate.go` for protoc-gen-validate), extend the predicate.

## Follow-ups disclosed

- **`mdemg extract-symbols` full re-ingest** for the 3 stale-hash pairs — operator schedule
- **Cross-language symbol filter alignment** — the analyzer's file-set filters should be a shared/tested set with the tree-sitter ingest, not two parallel lists. If either drifts, the class of "phantom missing pairs" returns. Small refactor.
- **`grpc.pb.go` extension** conditional on operator seeing new protobuf-plugin file shapes

## Rollback

Single-commit revert. Post-revert, the analyzer will resume emitting the 79 phantom-missing pairs; `SaveRelationships` will resume silently dropping them; the gap returns to 83. The `--dump-pairs` diagnostic + `RelationshipRecord` breadcrumbs are additive (harmless if unused).

## Documents Accessed

- `internal/symbols/go_types.go` (analyzer, edited: filter + diagnostic breadcrumbs)
- `internal/symbols/relationships.go` (added `Source*` / `Target*` breadcrumb fields)
- `internal/symbols/store.go:64` (`GenerateSymbolID` — the current line-excluding formula)
- `internal/cli/extract_symbols.go:208` (ingest-side symbol_id write — passes `sym.Line` that the current formula ignores)
- `internal/cli/analyze_go_implements.go` (CLI, edited: `--dump-pairs` flag + enriched output)
- GO-IMPLEMENTS-001 post (parent sprint, gap disclosure)
- Live Neo4j `SymbolNode` + `IMPLEMENTS` on mdemg-dev (gap-audit ground truth)
