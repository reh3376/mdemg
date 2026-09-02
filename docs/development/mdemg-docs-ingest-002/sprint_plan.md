# MDEMG-DOCS-INGEST-002 — Sprint Plan

**Task**: #147
**Predecessor**: MDEMG-DOCS-INGEST-001 (task #142) shipped the initial ingester
**Type**: bug fix (data-hygiene envelope on shipped ingester)
**Wall-clock estimate**: ~1h

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint | MDEMG-DOCS-INGEST-002 |
| Task # | #147 |
| Branch | `reh3376_dev01` (auto-PR flow) |
| Substrate touch? | code only; no schema, no substrate mutation |
| Reversible? | ✅ trivially — revert the exclusion + affected chunks re-ingest on next run |
| Related follow-up | #148 MDEMG-USAGE-CORPUS-CURATE-002 (path-predicate tightening — separate sprint) |

## 2. Problem Statement

The `mdemg mdemg-docs-ingest` CLI (`internal/cli/mdemg_docs_ingest.go`) walks the doc filesystem via `filepath.WalkDir` and produces one MemoryNode per markdown H2 section (or whole file). It walks:

1. `docs/features/**/*.md`
2. `docs/user/**/*.md`
3. `docs/api/**/*.md`
4. `CLAUDE.md` (single file)
5. `internal/cli/*.go` (cobra command Long strings via AST)

**Problem**: nothing skips accidentally-nested operational trees. Live-verified during this sprint:

- `docs/api/api-spec/uats/.venv/lib/python3.12/site-packages/idna-3.11.dist-info/licenses/LICENSE.md` — a Python packaging LICENSE file — WAS being picked up by MDEMG-DOCS-INGEST-001 as a legitimate MemoryNode in `mdemg-dev`. Path `mdemg-docs/api/license/000__whole-file` in the substrate. Non-durable data polluting the retrieval surface.

Same class hazard for `__pycache__/`, `node_modules/`, `*.dist-info/`, `*.egg-info/` — none rejected. Also blocks the operator's ability to point `--root` higher (would ingest EVERY nested project's venv/cache trees).

## 3. Scope & Constraints

**In scope**:
- Skip built-in deny-set of directory names: `.venv`, `__pycache__`, `node_modules`, `dist-info` (with `*.dist-info` suffix match), `egg-info` (with `*.egg-info` suffix match)
- Apply exclusion to BOTH walkers (`collectMarkdownChunks` + `collectCliLongChunks`) for symmetry — mirrors the never-hardcode-config rule (extend via env)
- Env override `MDEMG_DOCS_INGEST_EXCLUDE_DIRS=name1,name2,...` (comma-separated) — extends the deny set
- Escape hatch `MDEMG_DOCS_INGEST_EXCLUDE_DIRS=-` — fully disables built-ins (operator-authorized advanced use)
- Update CLI Long help to describe the exclusion + env override
- 6 pin tests (helpers + env override + subtree-skip + escape-hatch smoke)
- Tier-3 live smoke with real binary + synthetic tree

**Out of scope** (deferred to disclosed follow-ups):
- Retroactive tombstone of the 1 already-ingested idna LICENSE.md MemoryNode — belongs to #148 (path-predicate tightening) OR its own tiny cleanup pass
- Broader exclusion (e.g. `.git`, `build`, `dist`) — YAGNI; add if operator flags more classes
- Extending to `claude-docs-ingest` — that CLI reads a JSONL corpus, not the filesystem; no walker to guard

**Constraints preserved**:
- `never-hardcode-config` — deny-set defaults live in code but every value is env-overridable
- `plan-mode-before-change` — this doc IS the plan; small, well-scoped, no substrate mutation
- `mandatory-feature-docs` — the shipped `docs/features/*.md` for MDEMG-DOCS-INGEST-001 covers the surface; this sprint is a bug-fix delta noted in CLI Long help + CLAUDE.md pin
- `unit-integration-e2e-docs` — 6 unit tests + live Tier-3 dry-run against real repo + synthetic tree; no live substrate mutation needed (the shipped ingester's dry-run is the tier-3 harness)

## 4. Dependencies

Zero external — pure `os` / `io/fs` / stdlib change. Reuses the shipped `filepath.WalkDir` + `fs.SkipDir` shape (`fs.SkipDir` is the standard signal to prune a subtree from a walk).

## 5. Implementation Plan

**Epic 1 — Deny-set helpers (`internal/cli/mdemg_docs_ingest.go`)**:

1. `mdemgDocsDefaultExcludeDirs []string` — built-in list
2. `mdemgDocsExcludeDirSet() map[string]struct{}` — resolves built-in + env; `MDEMG_DOCS_INGEST_EXCLUDE_DIRS=-` returns empty map
3. `mdemgDocsShouldExcludeDir(name string, set map[string]struct{}) bool` — exact-match + `.dist-info` / `.egg-info` suffix rules (suffix rules ONLY fire if the base pattern is in the active set — so `=-` truly disables)

**Epic 2 — Wire into both walkers**:

4. `collectMarkdownChunks` — call `excludes := mdemgDocsExcludeDirSet()` at start; inside walker, when `d.IsDir()` and `path != dir` (the walker's root) and `mdemgDocsShouldExcludeDir(d.Name(), excludes)`, return `fs.SkipDir`
5. `collectCliLongChunks` — same shape, symmetric

**Epic 3 — Update CLI Long help** to document the exclusion + env override.

**Epic 4 — 6 pin tests**:

- `TestMdemgDocsShouldExcludeDir_BuiltinSet` — exact + suffix matches + negatives (`node_modules_backup` must not match)
- `TestMdemgDocsShouldExcludeDir_EmptySetDisables` — empty set → no exclusions
- `TestMdemgDocsExcludeDirSet_EnvOverride` — env extends built-ins
- `TestMdemgDocsExcludeDirSet_EnvDashDisablesBuiltin` — `=-` returns empty
- `TestCollectMarkdownChunks_SkipsExcludedSubtree` — real filesystem test with synthetic tree containing legit + 5 junk files (venv/pycache/node_modules/dist-info/egg-info); asserts only legit surfaces
- `TestCollectMarkdownChunks_EnvOverrideDisablesBuiltin` — with `=-`, junk under `.venv` IS ingested (proves escape hatch)

**Epic 5 — Live Tier-3 smoke** (real binary, synthetic tree in scratchpad):

- 3 real docs + 5 junk (one per excluded class)
- Smoke 1 (default): expect 3 chunks
- Smoke 2 (`=-`): expect 8 chunks
- Smoke 3 (`=app`): additional `app/` subtree also excluded → still 3 chunks

**Epic 6 — Regression check against real repo docs**:

- Default vs `=-` chunk-count diff against real `docs/` tree — expect delta ≥ 0 (any delta > 0 identifies a real leaked file)
- **Live-caught 1-chunk leak**: `docs/api/api-spec/uats/.venv/lib/python3.12/site-packages/idna-3.11.dist-info/licenses/LICENSE.md` — validated the sprint's premise (proves the exclusion is not just defensive future-proofing but fixes a real live leak)

## 6. Testing Plan (3 tiers)

**Tier 1 — Unit** (Epic 4, 6 tests via `go test -run 'TestMdemgDocs|TestCollectMarkdown' ./internal/cli/`)

**Tier 2 — Integration** (full internal/cli test suite regression via `go test ./internal/cli/`)

**Tier 3 — E2E live smoke** (Epic 5 + Epic 6): real `bin/mdemg` binary against synthetic tree AND real repo docs; chunk count deltas are the observable outcome

## 7. Commit Strategy

Single commit — code + tests + docs + sprint dir all in one atomic change; small enough that granular commits add no value.

## 8. Verification Checklist

- [x] Build clean (`go build ./...`)
- [x] Lint clean (`golangci-lint run ./internal/cli/`)
- [x] All 6 new unit tests pass
- [x] Full `internal/cli` suite passes (regression check)
- [x] Tier-3 live smoke: default excludes junk, `=-` includes it, extended env additive
- [x] Tier-3 regression check against real docs: identifies the idna LICENSE leak
- [x] CLI Long help documents the exclusion + env override
- [x] Sprint dir populated
- [x] CLAUDE.md architecture note pinned (arch rule for future walkers)
- [x] CHANGELOG Unreleased entry

## 9. Documentation Update

- `internal/cli/mdemg_docs_ingest.go` — Long help updated
- `docs/development/mdemg-docs-ingest-002/{sprint_plan,sprint_post}.md` — this sprint
- `CLAUDE.md` — 1 arch rule pinned (see §Architecture Notes append at PR time)
- `CHANGELOG.md` — Unreleased entry

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Over-broad exclusion silently drops legitimate docs whose dir happens to be named `.venv` etc | Exact-name match (not substring); `node_modules_backup` NOT excluded (regression-pinned) |
| Suffix match on `.dist-info`/`.egg-info` catches legit dirs someone created with those names | Only fires when the base pattern is in the active set; `=-` disables everything; extremely unlikely a legit doc dir uses these suffixes |
| Escape hatch `=-` gets set in prod by mistake | Documented as advanced-use-only in Long help; boot-time WARN would be belt-and-suspenders (deferred as YAGNI) |

## 11. Rollback

Revert commit → walker regains no-exclusion behavior → next `mdemg-docs-ingest` re-ingests the previously-skipped subtrees. No substrate mutation; no data loss.

## 12. Documents Accessed

- `internal/cli/mdemg_docs_ingest.go` — target of code change
- `internal/cli/mdemg_docs_ingest_test.go` — extended with 6 pin tests
- `internal/cli/claude_docs_ingest.go` (checked for similar walker — reads JSONL, not filesystem; no walker to guard)
- `docs/development/mdemg-docs-ingest-001/` (predecessor sprint dir — established the ingester surface)
- CLAUDE.md pins: `plan-mode-before-change`, `never-hardcode-config`, `unit-integration-e2e-docs`, `live-testing-tier-required`, `iterate-break-fix-verify`, `must-master-data-pipelines`, `end-with-docs-accessed`, `mandatory-feature-docs`, `must-comment-sprint-summary-on-pr`, `lint-before-commit`
- Go stdlib: `io/fs` (`SkipDir`), `path/filepath` (`WalkDir`)
- Operator directive: "resume #147" (2026-09-02)
- Task #148 (MDEMG-USAGE-CORPUS-CURATE-002) — related but disjoint follow-up (curator-side path predicate; separate sprint)
