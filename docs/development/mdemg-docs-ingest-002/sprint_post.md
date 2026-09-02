# MDEMG-DOCS-INGEST-002 — Sprint Post

**Task**: #147
**Completed**: 2026-09-02 (~1h wall-clock)
**Verdict**: ✅ SHIPPED — deny-set exclusion wired into both filesystem walkers; live-caught 1 pre-existing junk leak that MDEMG-DOCS-INGEST-001 had silently ingested.

Full plan at `sprint_plan.md`. Ship state + verification + one arch rule pinned below.

## What shipped

| Artifact | Notes |
|---|---|
| `internal/cli/mdemg_docs_ingest.go` | New helpers `mdemgDocsDefaultExcludeDirs` / `mdemgDocsExcludeDirSet` / `mdemgDocsShouldExcludeDir`; wired into `collectMarkdownChunks` + `collectCliLongChunks` via `fs.SkipDir` at directory entries; CLI Long help updated |
| `internal/cli/mdemg_docs_ingest_test.go` | 6 new pin tests (helpers × 4, live walker with synthetic tree × 2) |
| `docs/development/mdemg-docs-ingest-002/{sprint_plan,sprint_post}.md` | This sprint |
| `CLAUDE.md` — arch note (pin at PR time) | Filesystem-walking ingesters MUST guard operational-tree deny-set |
| `CHANGELOG.md` | Unreleased entry |

Built-in deny-set: `.venv`, `__pycache__`, `node_modules`, `*.dist-info` (suffix), `*.egg-info` (suffix). Env override `MDEMG_DOCS_INGEST_EXCLUDE_DIRS=name1,name2,...` extends; `=-` fully disables (advanced escape hatch).

## Verification

| Check | Result |
|---|---|
| `go build ./...` | ✅ clean |
| `golangci-lint run ./internal/cli/` | ✅ 0 issues |
| 6 new unit tests | ✅ 6/6 pass |
| Full `internal/cli` test suite | ✅ passes (no regressions) |
| Tier-3 live smoke — synthetic tree (3 legit + 5 junk) | ✅ default excludes 5 junk / `=-` includes all 8 / `=app` also excludes app/ subtree |
| **Tier-3 live smoke — real repo docs** | ✅ **default 1259 chunks / `=-` 1260 chunks — DELTA identified 1 real leak** |
| Production llama-server on port 8102 untouched | ✅ (this sprint touches no runtime) |

**Live-caught leak** (validates the sprint's premise, not just defensive-future-proofing):
```
docs/api/api-spec/uats/.venv/lib/python3.12/site-packages/idna-3.11.dist-info/licenses/LICENSE.md
```
MDEMG-DOCS-INGEST-001 had ingested this as a MemoryNode in `mdemg-dev` — a Python packaging LICENSE file surfaced by the walker because it's a `.md` file under `docs/api/...`. Path in substrate: `mdemg-docs/api/license/000__whole-file`. Post-fix, this file is skipped WHOLESALE via `fs.SkipDir` on the parent `.venv` subtree.

## Sprint execution — live-caught confirmation

The task description named the exclusion classes on the presumption that a leak was possible. The Tier-3 regression check confirmed it: **the +1 chunk delta between default and `=-` proves at least one real junk file was landing in the substrate.** Every future MDEMG-DOCS-INGEST-001 re-run that would have re-ingested this file now silently skips it.

## Arch rule pinned (proposed for CLAUDE.md next PR)

**Filesystem-walking ingesters MUST guard an operational-tree deny-set at directory entries.** Any CLI/service that walks `filepath.WalkDir` over user-writable paths (docs trees, workspace roots, project subtrees) MUST skip subtrees named `.venv`, `__pycache__`, `node_modules`, `*.dist-info`, `*.egg-info` via `fs.SkipDir` at directory entries. These trees contain hundreds/thousands of files not authored by the operator — Python virtualenvs alone can carry 10K+ `LICENSE.md` files across bundled dependencies. Any leak into a durable substrate (MemoryNodes, TSDB rows, training corpora) pollutes downstream retrieval + inference for as long as the ingested rows survive retention.

**Shape** (byte-for-byte reusable):
1. `defaultExcludeDirs []string` — built-in deny-set as a package-level const
2. `excludeDirSet() map[string]struct{}` helper — resolves built-in + env override; `=-` returns empty (escape hatch)
3. `shouldExcludeDir(name, set)` helper — exact-match + optional `.dist-info` / `.egg-info` suffix rules (suffix rules ONLY fire if the base pattern is in the active set — so `=-` truly disables)
4. At the walker's directory entry: `if path != rootDir && shouldExcludeDir(d.Name(), excludes) { return fs.SkipDir }`
5. Env override MUST be named `<CLI_NAME>_EXCLUDE_DIRS` (comma-separated) — follows `never-hardcode-config`
6. CLI Long help MUST document the exclusion + env override + `=-` escape hatch
7. Pin test the escape hatch with a real filesystem synthetic tree, not just a mock

Applies retroactively to every existing filesystem-walking ingester (audit list at next arc): `mdemg-docs-ingest` ✅ (this sprint), `mdemg claude-docs-ingest` N/A (reads JSONL, not filesystem), `mdemg ingest` (repo-wide code ingest — has its own vendor/gitignore rules; audit for parity).

## Follow-ups disclosed

1. **Retroactive tombstone of the ingested idna LICENSE.md MemoryNode** — the leaked node still lives in `mdemg-dev` at path `mdemg-docs/api/license/000__whole-file`. Cleanup approach: `is_archived=true` + `archive_reason='mdemg_docs_ingest_002_venv_leak_purge'`. Belongs to #148 (MDEMG-USAGE-CORPUS-CURATE-002) OR its own tiny cleanup pass — not blocking, low signal (1 node), can wait for the paired curator sprint.
2. **Audit `mdemg ingest`** (the repo-wide code ingester) for parity with this deny-set. Different surface (code, not docs) so its own vendor/gitignore rules may already cover the class; verify then no-op or extend as needed.
3. **Boot-time WARN when `MDEMG_DOCS_INGEST_EXCLUDE_DIRS=-` is set** — belt-and-suspenders reminder that the escape hatch is active. Deferred as YAGNI unless operators actually flip it in prod by mistake.

## Documents Accessed

- `docs/development/mdemg-docs-ingest-002/sprint_plan.md` (this sprint)
- `docs/development/mdemg-docs-ingest-001/` — predecessor (established the walker surface)
- `internal/cli/mdemg_docs_ingest.go` — target of code change (both walkers + Long help)
- `internal/cli/mdemg_docs_ingest_test.go` — extended with 6 pin tests
- `internal/cli/claude_docs_ingest.go` — audited (reads JSONL, no walker; N/A for this fix)
- Go stdlib: `io/fs` (`SkipDir`), `path/filepath` (`WalkDir`, `Rel`), `os` (`ReadFile`, `Stat`, `MkdirAll`)
- Real repo `docs/api/api-spec/uats/.venv/**/*` — live-verified leak source
- CLAUDE.md pins (§3 sprint plan)
- Operator directive: "resume #147" (2026-09-02)
- Task #148 (paired follow-up — mentioned as scope-adjacent, not required by this sprint)
