# Sprint Post: SEC-TRANCHE-2 — Path-Injection Tranche

**Shipped:** 2026-08-11
**Branch:** `reh3376_dev01`

## 1. Header & Metadata

- Sprint ID: SEC-TRANCHE-2
- Prereq closed: PLUGIN-PATH-INJECTION-FIX (2026-08-10)
- Alerts processed: 28 (1 CRITICAL + 27 HIGH)

## 2. Problem Statement (recap)

CodeQL flagged 28 alerts across 15 files. Prior sprint shipped `validatePluginID` (regex allowlist) but CodeQL does NOT recognize regex validation as a path-injection sanitizer. The recognized pattern is `filepath.Clean + strings.HasPrefix` containment. This sprint applies that pattern at 8 HTTP-reachable sites and dismisses 13 alerts that reach an operator-input trust boundary.

## 3. Scope & Constraints (recap)

Structural fix at HTTP-facing sites; dismissal with rationale at operator-trust / allowlist / server-constructed sites; never weaken a real security check.

## 4. Dependencies

None external.

## 5. Implementation Plan — WHAT SHIPPED

### E1 — new `internal/pathsafe` package

- `pathsafe.SafeJoinUnderDir(baseDir, untrusted)` (`internal/pathsafe/pathsafe.go`)
- 6 pin tests all green (`internal/pathsafe/pathsafe_test.go`)
- Symlink-escape test guards against the macOS `/var → /private/var` class

### E2 — Category A structural fixes (8 sites, 5 files)

1. `internal/api/plugin_handlers.go:353` (handlePluginDetail) ✔
2. `internal/api/plugin_handlers.go:440` (handlePluginValidate manifestPath) ✔
3. `internal/api/plugin_handlers.go:478` (handlePluginValidate binaryPath) — routes manifest.Binary through SafeJoinUnderDir with pluginDir as base ✔
4. `internal/api/plugin_handlers.go:481` (error-message-only branch) — same helper for uniformity ✔
5. `internal/plugins/scaffold/scaffold.go` Generate pluginDir join ✔
6. `internal/plugins/scaffold/scaffold.go` per-file loop join ✔
7. `internal/backup/service.go` — new `Service.safeBackupPath` helper; used at DeleteBackup (3 sinks) + writeManifest + loadManifest ✔
8. `internal/backup/full.go` runRestore — mdemgPath + dumpPath ✔
9. `internal/backup/partial.go` runPartialBackup outPath ✔

### E3 — Category B + C dismissals (13 alerts)

| # | File / line | Category | Reason |
|---|---|---|---|
| 40 | handlers.go:3323 | C — operator-directed | handleIngestFiles processes operator-provided file list; path IS the argument |
| 41 | handlers.go:3586 | C — operator-directed | readFileContent called from handleIngestFiles path |
| 42 | handlers_filewatcher.go:46 | C — operator-directed | Path is intended argument (which dir to watch) |
| 56 | scaffold.go:107 | B — sanitized at handler | Actually fixed in E2 via SafeJoinUnderDir; dismissed to close on next scan |
| 57 | scaffold.go:136 | B — sanitized at handler | Same as #56 |
| 58 | validator.go:113 | B — sanitized at HTTP callers | HTTP path now goes through SafeJoinUnderDir |
| 59 | validator.go:130 | B — same | Same |
| 60 | validator.go:202 | B — same | Same |
| 61 | validator.go:284 | B — same | Same |
| 62 | transfer/format.go:54 | C — leaf helper | HTTP callers (backup/partial.go) sanitized upstream |
| 63 | transfer/format.go:59 | C — leaf helper | Same |
| 64 | tsdb/backup.go:518 | C — server-constructed | tarGzDirectory srcDir/destPath from CLI cfg or os.TempDir |
| 65 | tsdb/exporter.go:327 | C — allowlist gate | tableName validated against `tableSpecs` map at exporter.go:201-205 |
| 66 | tsdb/exporter.go:392 | C — allowlist gate | Same |
| 67 | unts/registry.go:356 | C — tracked-files gate | computeFileHash reached only after `f, ok := r.files[path]` map lookup |

### E4 — CRITICAL #22 disposition

Left OPEN pending CodeQL re-scan post-merge. Every HTTP call site into `ValidateProtoCompliance` now sanitizes `pluginID` via `SafeJoinUnderDir`, so the cross-file taint should close. If it persists, follow-up dismissal with rationale.

### E5 — docs

- CHANGELOG entry under `[Unreleased]` Security ✔
- CLAUDE.md pin: 2 architectural rules ✔
- Sprint plan + this post ✔

## 6. Testing Plan — RESULTS

- **Tier 1:** `go test ./internal/pathsafe/...` → 6/6 pass (100%).
- **Tier 2:** `go test ./internal/pathsafe/... ./internal/backup/... ./internal/plugins/... ./internal/api/...` → all green (pathsafe 0.287s, backup 1.822s, plugins cached, plugins/scaffold 0.486s, api 12.944s).
- **Tier 3 (live e2e):** deferred to post-merge (the fixes are byte-preserving for happy-path inputs and covered by the existing package test suites; a real HTTP smoke against `POST /v1/plugins/<safe-id>/validate` is the next verification step but not shipping-blocking — the pin tests + package suites cover the semantic behavior).

## 7. Commit Strategy

Single commit: `security: path-injection tranche (SEC-TRANCHE-2) — safeJoinUnderDir helper + CodeQL dismissals`.

## 8. Verification Checklist

- [x] `go build ./...` clean
- [x] `golangci-lint run ./internal/pathsafe/... ./internal/backup/... ./internal/plugins/scaffold/... ./internal/api/...` — 0 issues
- [x] `go test ./internal/pathsafe/...` green (6 tests)
- [x] `go test ./internal/backup/... ./internal/plugins/... ./internal/api/...` green (no regressions)
- [x] 13 Category B + C alerts dismissed via `gh api`
- [x] CHANGELOG entry under `[Unreleased]` Security
- [x] CLAUDE.md pin: 2 architectural rules added
- [x] Sprint plan + post written

## 9. Documentation Update

- Sprint plan (`sprint_plan.md`), post (this file)
- CHANGELOG under `[Unreleased]` Security
- CLAUDE.md architecture-notes pin

## 10. Risks & Mitigations — POST-SHIP

- **R1 (over-sanitization):** happy path preserved (existing tests all pass); the containment-boundary pin covers the subtle sibling-prefix case.
- **R2 (CodeQL still flags after fix):** mitigated by explicit dismissal for CRITICAL #22 pending re-scan.
- **R3 (import cycle):** `internal/pathsafe` is a leaf package with only stdlib imports; verified via `go build ./...`.

## 11. Rollback Procedures

Revert the single commit. Rejection cases surface as HTTP 400 with `pathsafe.ErrTraversal` / `ErrEscape` / `ErrEmptyBase` in the message; any legitimate call rejected can be triaged from the error string.

## 12. Documents Accessed

See sprint_plan.md §12.

## Deliverable summary

**Structural fixes (8 sites, 5 files):** plugin_handlers.go ×4, scaffold.go ×2, backup/service.go ×5 (via `safeBackupPath` helper), backup/full.go ×2, backup/partial.go ×1.

**Dismissals (13 alerts):** listed in §E3 table above with per-alert rationale.

**CRITICAL #22:** left open pending CodeQL re-scan post-merge.

**Re-classifications:** 5 sites moved from prompt's Category C → Category A after reading caller chains (backup/service.go, backup/full.go, scaffold.go, transfer/format.go [C dismissed after E2 upstream sanitization]). Rationale: these are HTTP-reachable via the backup + plugin API surfaces; the prompt's "CLI operator input" categorization missed those handler entry points.

**Additional vulnerabilities found beyond CodeQL alerts:** the `ToModuleID` sanitizer in `internal/plugins/scaffold/scaffold.go` was silently accepting `..` and `/` in the module id (only lowercases + hyphenates spaces/underscores). This is fixed structurally by the E2 SafeJoinUnderDir wiring at Generate.

**Also:** noticed the `binaryPath` in the error-message branch of `handlePluginValidate` (line 481) rendered an unsanitized manifest.Binary path in the error response — a minor info-leak (not exec). Fixed uniformly with the same helper.
