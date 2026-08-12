# Sprint: SEC-TRANCHE-2 — Path-Injection Tranche

## 1. Header & Metadata

- **Sprint ID:** SEC-TRANCHE-2
- **Type:** Security hardening
- **Date:** 2026-08-11
- **Branch:** `reh3376_dev01`
- **Prerequisites:** PLUGIN-PATH-INJECTION-FIX (2026-08-10)
- **Estimated scope:** ~1 day

## 2. Problem Statement

CodeQL flagged 28 alerts in the `go/path-injection` and `go/command-injection` rule families across 15 files (1 CRITICAL + 27 HIGH). The prior PLUGIN-PATH-INJECTION-FIX shipped `validatePluginID` (regex allowlist) and `verifyPluginBinaryInside` (containment check) but CodeQL does NOT recognize regex validation as a path-injection sanitizer. Its data-flow analysis requires `filepath.Clean` + `strings.HasPrefix` containment against a trusted base — the pattern shipped by GitHub's own CodeQL standard libraries.

This tranche categorizes each alert by trust boundary, applies a structural fix at the HTTP-facing sites, and dismisses alerts that reach a genuine operator-input surface (CLI, or an intentional operator-directed API like `POST /v1/backup/restore`) with a rationale.

## 3. Scope & Constraints

- **In scope:** every alert in the batch listed by `gh api repos/reh3376/mdemg/code-scanning/alerts?state=open&per_page=100`.
- **Out of scope:** other rule families (SQL injection, XSS, etc.). Not weakening any real security check.
- **Constraints:** NEVER commit to `main`; push to `reh3376_dev01`; auto-PR fires on push.

## 4. Dependencies

None — this sprint is additive plus in-place fixes at 4 files.

## 5. Implementation Plan

Sequential epics with gates.

### E1 — new `internal/pathsafe` package with `SafeJoinUnderDir` helper

- rejects `..` before Join, rejects absolute untrusted, canonicalizes both sides via EvalSymlinks with ancestor fallback (macOS `/var` → `/private/var`), applies strict-prefix containment check.
- 6 pin tests: happy-path, `..` rejection (4 variants), absolute-path rejection, symlink-escape rejection, empty-base error, containment boundary (sibling prefix check).
- **Gate:** `go test ./internal/pathsafe/...` green.

### E2 — Category A structural fixes (7 sites, 4 files)

Wire `pathsafe.SafeJoinUnderDir` at:

1. `internal/api/plugin_handlers.go:353` (handlePluginDetail) — replaces `filepath.Join(s.cfg.PluginsDir, pluginID)`
2. `internal/api/plugin_handlers.go:440` (handlePluginValidate manifestPath) — same
3. `internal/api/plugin_handlers.go:478` (handlePluginValidate binaryPath) — routes manifest.Binary through SafeJoinUnderDir with pluginDir as base; also fixes the error-message-only binaryPath at line 481
4. `internal/plugins/scaffold/scaffold.go:104` (pluginDir join) — HTTP-reachable via handlePluginCreate; `ToModuleID` does NOT strip `.` or `/`
5. `internal/plugins/scaffold/scaffold.go:131` (per-file join in scaffold write loop)
6. `internal/backup/service.go` — new `safeBackupPath` helper wrapping SafeJoinUnderDir; used at DeleteBackup (3 sinks), writeManifest, loadManifest
7. `internal/backup/full.go:82-83` (runRestore mdemgPath + dumpPath) — routes req.BackupID through safeBackupPath
8. `internal/backup/partial.go:50` (runPartialBackup outPath) — for uniformity (server-generated ID is safe by construction, but pattern must be uniform)

Preserves existing `validatePluginID` regex check as defense-in-depth (clearer error message on bad IDs); `verifyPluginBinaryInside` retained for callers that reference it.

- **Gate:** `go build ./...`, `golangci-lint run ./internal/...`, `go test ./internal/pathsafe/... ./internal/backup/... ./internal/plugins/... ./internal/api/...` all green.

### E3 — Category B + C dismissals

For each site NOT in E2:

- Verify categorization by reading the caller chain.
- Dismiss via `gh api -X PATCH repos/reh3376/mdemg/code-scanning/alerts/<N>` with `dismissed_reason: false positive` and a comment explaining the invariant (allowlist gate, tracked-files map, server-constructed ID, operator-trust boundary, etc.).

### E4 — CRITICAL #22 (validator.go:307) disposition

CodeQL taint flow originates from HTTP callers now sanitized in E2. Leave the alert open until CodeQL re-scans after this PR merges. If it persists (cross-file taint tracking is imperfect), dismiss with a rationale referencing the E2 sanitization at every HTTP call site.

### E5 — CLAUDE.md + CHANGELOG + docs

Two architectural rules pinned:

1. CodeQL does NOT recognize regex validation as a path-injection sanitizer. Use `pathsafe.SafeJoinUnderDir` (filepath.Clean + strings.HasPrefix containment) at every HTTP handler that constructs a filesystem path from URL segments or JSON fields.
2. `#nosec` is gosec-only; CodeQL ignores it. Use the pathsafe helper OR dismiss via the GitHub code-scanning API with an explicit rationale (never `// nolint` for CodeQL).

CHANGELOG entry under `[Unreleased]` Security.

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit):** pathsafe pin tests (6) — see §E1.
- **Tier 2 (integration):** package tests for backup + plugins + api MUST still pass (they exercise the new join paths transitively via the shipped test suites; the fixes are byte-preserving for happy-path inputs).
- **Tier 3 (live e2e):** build the binary and drive `POST /v1/plugins/<safe-id>/validate` against the real server; drive `DELETE /v1/backup/<safe-id>`. Verify no regression on legitimate inputs and 400 on `..` inputs.

## 7. Commit Strategy

Single commit at the end of the sprint per operator preference for this tranche:
`security: path-injection tranche (SEC-TRANCHE-2) — safeJoinUnderDir helper + CodeQL dismissals`

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `golangci-lint run ./internal/...` clean
- [ ] `go test ./internal/pathsafe/...` green (6 tests)
- [ ] `go test ./internal/backup/... ./internal/plugins/... ./internal/api/...` green (no regressions)
- [ ] All Category B + C alerts dismissed via `gh api`
- [ ] CHANGELOG entry under `[Unreleased]` Security
- [ ] CLAUDE.md pin: 2 architectural rules added
- [ ] Sprint post written

## 9. Documentation Update (final epic)

- Sprint plan (this file) + `post.md`
- CHANGELOG entry
- CLAUDE.md architecture-notes pin

## 10. Risks & Mitigations

- **R1: over-sanitization breaks legitimate paths** — mitigated by 6 pin tests including the containment-boundary (sibling-prefix) case; the EvalSymlinks fallback handles macOS `/var` → `/private/var`.
- **R2: CodeQL still flags after fix (cross-file taint imperfect)** — mitigated by parallel dismissal for CRITICAL #22 with rationale, awaiting re-scan.
- **R3: pathsafe package introduces import cycle** — mitigated by placing it in a leaf-most package (`internal/pathsafe`) with no MDEMG imports.

## 11. Rollback Procedures

Not applicable — this sprint is additive at leaf packages and byte-preserving for happy-path inputs. Revert the commit if a legitimate call is rejected; the failing case is reported at 400 with a specific error naming `ErrTraversal` / `ErrEscape` / `ErrEmptyBase`.

## 12. Documents Accessed

- `internal/api/plugin_handlers.go`
- `internal/api/handlers.go` (readFileContent + handleIngestFiles)
- `internal/api/handlers_filewatcher.go`
- `internal/api/handlers_backup.go`
- `internal/api/handlers_training_data.go`
- `internal/plugins/validator.go`
- `internal/plugins/scaffold/scaffold.go`
- `internal/backup/service.go`, `full.go`, `partial.go`
- `internal/transfer/format.go`
- `internal/tsdb/exporter.go` (lines 195-215, 300-400)
- `internal/tsdb/backup.go` (lines 490-570)
- `internal/unts/registry.go` (lines 240-370)
- Prior sprint: PLUGIN-PATH-INJECTION-FIX (2026-08-10)
