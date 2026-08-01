# GO-IMPLEMENTS-001 — Sprint Plan

**Date:** 2026-07-31 | **Branch:** `reh3376_dev01`
**Parent trigger:** Q3 deferral (`ROADMAP_2026Q3.md`) + Q4 deep-dive
candidate #9.

## 1. Header & Metadata

Wire the Go IMPLEMENTS edge producer. Turn `internal/symbols/go_types.go`'s
nil-returning stub into a real `go/types`-backed analyzer that discovers
Go's implicit interface satisfaction; ship a CLI to invoke it against
a Go project root; live-run against mdemg's own tree and confirm the
IMPLEMENTS edges land in Neo4j where the structural RRF column +
edge-attention (IMPLEMENTS weight 0.70) can consume them. ~3-4h.

## 2. Problem Statement

Two verified live gaps:

1. `internal/symbols/go_types.go::AnalyzeImplements` returns `nil, nil`
   with the comment "golang.org/x/tools is not yet in go.mod".
2. **`grep` for `AnalyzeImplements` / `GoTypesAnalyzer` in `internal/`
   returns only the declaration site** — no caller anywhere. Even if
   the analyzer were implemented, nothing would invoke it.

Consequence per the Q3 audit: **zero Go IMPLEMENTS edges exist in a
Go-dominant codebase**. The structural RRF column that walks
IMPLEMENTS edges + the edge-attention config that weights IMPLEMENTS
at 0.70 are consumers with no data.

Why tree-sitter can't do this alone: Go's interface satisfaction is
**implicit** — a struct implements an interface just by having the
right methods, with no `implements` keyword to grep for. Only Go's
own type checker (`go/types`) can compute method-set matches. This
is why the sibling `.scm` query files exist for Rust/Java/Python/TS
but Go has the separate deep-analyzer path.

## 3. Scope & Constraints

**In scope (single commit):**

- Add `golang.org/x/tools` to `go.mod` (needs `go/packages`)
- Implement `GoTypesAnalyzer.AnalyzeImplements`:
  - `packages.Load(cfg, "./...")` with `NeedName | NeedTypes |
    NeedTypesInfo | NeedSyntax | NeedFiles` from `projectRoot`
  - Collect all `*types.Interface`s (both declared and embedded)
    and all `*types.Named` concrete types across all loaded packages
  - For each `(concrete, interface)` pair where
    `types.Implements(concrete, interface)` OR pointer-form implements,
    emit a `RelationshipRecord{Relation: "IMPLEMENTS", …}`
  - Also cover interface-embeds-interface: emit IMPLEMENTS when a
    narrower interface satisfies a wider one
  - Use `GenerateSymbolID(spaceID, filePath, name, 0)` for both
    source + target symbol_ids so the resulting Cypher `MATCH
    (source:SymbolNode {symbol_id: …})` in `SaveRelationships`
    finds the existing SymbolNodes ingested by tree-sitter
  - `ResolutionMethod: "go_types"`, `Tier: 2`, `Confidence: 1.0`
    (go/types is authoritative — not a heuristic)
  - Skip pairs from the standard library on both sides (noise
    reduction — an operator shipping app code doesn't want
    `MyString implements fmt.Stringer` edges for every string type)
- New CLI command `mdemg symbols analyze-go-implements --space-id
  <id> --root <path>` — invokes the analyzer + calls
  `symbolStore.SaveRelationships`
- Unit tests: small fixture Go module with 2 interfaces and 3
  concrete types; assert the right IMPLEMENTS pairs are produced;
  assert stdlib pairs are excluded
- Live run: `mdemg symbols analyze-go-implements --space-id
  mdemg-dev --root /Users/reh3376/mdemg`; verify IMPLEMENTS edges
  land in Neo4j (`MATCH ()-[r:IMPLEMENTS]->() RETURN count(r)`
  goes from 0 → some positive number); sample-check 2-3 edges
  against the source

**Out of scope:**

- Automatic invocation during batch-ingest (go/types wants a whole
  project; batch-ingest is per-file). Operator runs the CLI when
  they want fresh IMPLEMENTS data. A future sprint could wire a
  post-ingest hook if needed.
- Non-Go IMPLEMENTS edges (already covered by tree-sitter for
  TypeScript/Rust).
- IMPLEMENTS edge weight/attention tuning (already configured at
  0.70 by prior work; live-verify only, no changes).

## 4. Method

1. `go get golang.org/x/tools` (adds to go.mod)
2. Rewrite `internal/symbols/go_types.go`:
   - Import `go/types`, `golang.org/x/tools/go/packages`
   - Implement `AnalyzeImplements` per spec
   - Handle vendored deps + skip stdlib
3. New CLI command in `internal/cli/`
4. Unit tests in `internal/symbols/go_types_test.go` with fixture
5. `go build ./...` + `golangci-lint`
6. Live smoke against mdemg-dev
7. Docs (post, CHANGELOG, CLAUDE.md pin)

## 5. Testing Plan

- **Tier 1 (unit)**: fixture Go module with 3 interfaces + 4 concrete
  types (some implement, some don't). Assert count + specific pairs.
  Fixture inline via `packages.Config.Overlay` or a temp dir.
- **Tier 2 (integration)**: CLI command wires correctly; a `--dry-run`
  variant lists edges without writing (safer first live invocation).
- **Tier 3 (live)**:
  - Pre-run: `MATCH ()-[r:IMPLEMENTS]->() RETURN count(r)` = 0
  - Run CLI against `/Users/reh3376/mdemg` on `mdemg-dev`
  - Post-run: count > 0; spot-check 2 edges against source
  - Guard: no bogus edges (e.g. `foo implements error` all over the
    place — that's real but voluminous; the stdlib skip should
    filter it)

## 6. Commit Strategy

Single commit under `GO-IMPLEMENTS-001`.

## 7. Verification Checklist

- [ ] `golang.org/x/tools` in go.mod
- [ ] `AnalyzeImplements` implemented + stdlib-skip
- [ ] CLI command wired with `--dry-run` option
- [ ] Unit tests green
- [ ] Live: pre-count 0 → post-count > 0; sample-check 2 edges
- [ ] CHANGELOG + CLAUDE.md pin + post

## 8. Rollback

Revert commit; the analyzer + CLI + go.mod dep all disappear.
Neo4j IMPLEMENTS edges the operator's live run wrote can be dropped
with `MATCH ()-[r:IMPLEMENTS {resolution_method: "go_types"}]->()
DELETE r` if desired (fully targeted — the tree-sitter-sourced
IMPLEMENTS edges for TypeScript/Rust have a different
`resolution_method` value).

## 9. Risks

- **Risk**: `packages.Load` on the mdemg tree pulls in
  `golang.org/x/*` and other deps → memory blow-up.
  - **Mitigation**: `NeedFiles` + `Tests: false`; only include
    packages under `projectRoot`; skip anything from `vendor/` or
    `~/go/pkg/mod/` in the emit filter.
- **Risk**: stdlib-implements-stdlib pairs explode edge count.
  - **Mitigation**: skip either-side-in-stdlib in the emit filter
    (`strings.HasPrefix(pkg.PkgPath, "golang.org/x/") ||
    !strings.Contains(pkg.PkgPath, "/")`).
- **Risk**: go/types compilation errors in the target project
  short-circuit the analysis.
  - **Mitigation**: `packages.Load` returns per-package errors as
    diagnostics; log them at Warn but continue with successfully-
    loaded packages.

## 10. Documents Accessed

Filled in `post.md`.
