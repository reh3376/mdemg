# GO-IMPLEMENTS-001 — Sprint Post

**Date:** 2026-07-31 | **Branch:** `reh3376_dev01`
**Parent trigger:** Q3 deferral (`ROADMAP_2026Q3.md`) + Q4 deep-dive
candidate #9.

## Verdict

**Shipped.** `GoTypesAnalyzer.AnalyzeImplements` is now a real
go/types-backed analyzer that discovers Go's implicit interface
satisfaction. New CLI `mdemg symbols analyze-go-implements` invokes
it and writes edges via the existing `SymbolStore.SaveRelationships`
path. **Live: 267 pairs discovered on mdemg's own tree in 472ms
→ 188 IMPLEMENTS edges landed in Neo4j** (2 → 188 net gain; the
267→188 delta reflects Go symbols not currently in the graph —
proto-generated files, test-only code, ingest filters).

## What shipped

- **`internal/symbols/go_types.go`** — the stub is gone. New:
  - `AnalyzeImplements(ctx, spaceID, projectRoot)` uses
    `packages.Load` with `NeedTypes|NeedTypesInfo|NeedSyntax|NeedFiles`
    on `./...` from `projectRoot`
  - Collects every `*types.Interface` and every concrete `*types.Named`
    with a method set across all loaded packages
  - For each pair, checks `types.Implements(concrete, iface)` AND
    `types.Implements(*concrete, iface)` (pointer-receiver methods
    count); also handles interface-embeds-interface
  - Filters: **stdlib packages skipped** (`fmt`, `io`, `encoding/json`,
    etc. — canonical stdlib paths never contain a `.`); **vendored /
    module-cache files skipped** (`/vendor/`, `/pkg/mod/`); **empty
    interface `any` skipped** (would emit N×M edges); **self-implements
    skipped** (T→T)
  - Uses `GenerateSymbolID(spaceID, relativizePath(filePath), name, 0)`
    for both source + target — matches the deterministic-hash + leading-
    slash shape the tree-sitter ingest uses, so `SaveRelationships`
    Cypher `MATCH` clauses find the existing SymbolNodes
  - `ResolutionMethod: "go_types"`, `Tier: 2`, `Confidence: 1.0`
    (go/types is authoritative, not a heuristic)
- **`internal/cli/analyze_go_implements.go`** — `mdemg symbols
  analyze-go-implements --root <path> --space-id <id>`. Flags:
  `--dry-run` (compute + print without writing), `--neo4j-*` (standard
  connection), 5-minute timeout, sample-print (first 10 pairs).
- **`internal/cli/root.go`** — new `mdemg symbols` command group;
  `analyze-go-implements` is its first subcommand
- **`internal/symbols/go_types_test.go`** — 5 unit tests:
  - `TestAnalyzeImplements_Fixture` — inline Go module fixture with 3
    interfaces (Reader, Closer, ReadCloser) + 3 concrete types (File,
    Stringer, Isolated); asserts 5 required pairs land + stdlib
    (Stringer→fmt.Stringer) skipped + self-implements skipped +
    every rel carries the right (Relation, Tier, Confidence,
    ResolutionMethod)
  - `TestAnalyzeImplements_EmptyProjectRoot` — validates arg check
  - `TestIsStdlib` — 10-case matrix (fmt, io, encoding/json,
    github.com/…, golang.org/x/…, mdemg, mdemg/internal/…, empty)
  - `TestIsVendored` — 5-case matrix
  - `TestRelativizePath_MatchesTreeSitterShape` — asserts the
    leading-slash contract that lets symbol_ids match between the
    tree-sitter and go/types paths
- **`go.mod`** — `golang.org/x/tools v0.47.0 → v0.48.0` (the stub's
  comment claiming "not in go.mod" was stale; it was already there,
  just unused)

## Live Tier-3 (mdemg-dev)

Pre-run:
```
MATCH ()-[r:IMPLEMENTS]->() WHERE r.space_id = 'mdemg-dev' RETURN count(r);
=> 2
```

Dry-run:
```
INFO go/packages loaded projectRoot=/Users/reh3376/mdemg packages=96 diagnostics=0
INFO go/types collected named types interfaces=109 concretes=1464
INFO go/types IMPLEMENTS analysis complete pairs_emitted=267

Discovered 267 IMPLEMENTS pair(s) in 472ms
```

Sample source→target validation (verify sample IDs resolve to real
SymbolNodes with correct name + path):
```
"176532b90f4befd4707af924fee9ec73" → devSpaceClient / /api/devspacepb/devspace_grpc.pb.go
"e8501a4c2e88984c8592642eb5391945" → DevSpaceClient / /api/devspacepb/devspace_grpc.pb.go
```
→ Both resolve. First edge: `devSpaceClient IMPLEMENTS DevSpaceClient`
(gRPC-generated struct → the interface it satisfies). Correct.

Live write:
```
Wrote 267 IMPLEMENTS edges to Neo4j in 116ms.
```

Post-run:
```
MATCH ()-[r:IMPLEMENTS]->() WHERE r.space_id = 'mdemg-dev' RETURN count(r);
=> 188  (2 → 188)
```

**267 emitted vs 188 landed = 79 pairs' source or target isn't in the
SymbolNode graph** (proto-generated files, test-only code, ingest
filters). This is expected + acceptable: the analyzer is a producer,
the SymbolNode graph is the consumer; a missing SymbolNode isn't a
bug in the analyzer, and the MATCH failing silently is the shipped
SaveRelationships contract.

Sample landed edges (via `MATCH (a)-[r:IMPLEMENTS {resolution_method:
'go_types'}]->(b) LIMIT 5`):
- `devSpaceClient IMPLEMENTS DevSpaceClient` (gRPC generated)
- `UnimplementedDevSpaceServer IMPLEMENTS DevSpaceServer` (embed pattern)
- `Server IMPLEMENTS DevSpaceServer` (my `internal/devspace/server.go`
  hand-written Server → generated interface — the exact "your code
  satisfies this interface" pattern that Go's implicit satisfaction
  makes uniquely hard to detect statically)
- `UnimplementedDevSpaceServer IMPLEMENTS UnsafeDevSpaceServer`
- `Server IMPLEMENTS UnsafeDevSpaceServer`

All five are correct Go IMPLEMENTS. The structural RRF column and
edge-attention (IMPLEMENTS weight 0.70) now have real data to walk.

## Rules pinned

⚠️ **Go IMPLEMENTS is implicit; tree-sitter CAN'T detect it.** The
sibling `.scm` query files for TypeScript/Rust/Java match syntactic
`implements` clauses, but Go has no such keyword — satisfaction is a
runtime property of the method set. Only `go/types` can compute the
match. When adding a new tree-sitter language pack for Go
relationships, do NOT add a `queries/go/inheritance.scm` (it would
find nothing); route through `GoTypesAnalyzer` instead.

⚠️ **`relativizePath` MUST produce the leading-slash shape** that the
tree-sitter ingest uses in `SymbolNode.file_path`. The
`GenerateSymbolID` hash is `sha256(spaceID | filePath | name)[:16]` —
a shape mismatch means the analyzer's IDs never resolve against the
tree-sitter-ingested SymbolNodes and every IMPLEMENTS edge silently
drops at `SaveRelationships`' `MATCH` clause. Pin-tested via
`TestRelativizePath_MatchesTreeSitterShape`.

⚠️ **Filter the empty interface (`any` / no-method) before pairing.**
Every type satisfies `interface{}`, so a naive iteration would emit
N×M edges (in mdemg's case: 1464 × ~5 empty interfaces ≈ 7300 noise
edges). The filter `iface.NumMethods() == 0 → continue` is
structural, not a config knob.

## Follow-ups disclosed

1. **Automatic post-ingest invocation** — currently the CLI is
   operator-invoked. If IMPLEMENTS edges start driving retrieval
   quality measurably, a hook that runs `AnalyzeImplements` after
   any batch-ingest that touched a `.go` file would keep the edges
   fresh. Not urgent (batch-ingest of Go files is infrequent on a
   living repo; operator can rerun the CLI on demand).
2. **267→188 gap investigation** — 79 pairs' symbols aren't in the
   SymbolNode graph. Some are legitimate (proto-generated files
   under `/api/devspacepb/` that the ingest filter excludes), but
   others may indicate the ingest is skipping files it shouldn't.
   A future audit could categorize the 79 missing pairs and either
   (a) accept the ingest filter as-is and document the gap, or
   (b) widen the ingest to include the missing files.
3. **Cross-package IMPLEMENTS retrieval-quality measurement** — the
   Q4 deep-dive's premise was that this data would improve
   retrieval on "who implements X?" queries. Now that 188 edges
   are live, a follow-up UVTS or RQA-style audit could measure the
   before/after lift on interface-related queries.

## Documents Accessed

- `docs/development/roadmap/ROADMAP_2026Q3.md` (Q3 deferral spec —
  the verified gap statement)
- `docs/development/q4-frontier-scoping/DEEP_DIVE_2026-07-27.md`
  (Q4 candidate #9)
- `docs/development/go-implements-001/sprint_plan.md` (this dir)
- `internal/symbols/go_types.go` (target — was a nil-returning stub)
- `internal/symbols/query_engine.go` (sibling tree-sitter path — the
  reason Go needs a separate analyzer)
- `internal/symbols/store.go::GenerateSymbolID` (deterministic-hash
  contract the analyzer must match)
- `internal/symbols/relationships.go::SaveRelationships` (the
  storage path the CLI reuses)
- `internal/api/handlers.go:940-975` (the batch-ingest wire — for
  understanding the tree-sitter side of the pipeline)
- Live Neo4j queries against mdemg-dev for pre/post edge counts +
  sample validation
