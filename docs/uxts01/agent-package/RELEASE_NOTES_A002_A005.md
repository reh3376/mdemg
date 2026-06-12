# Release Notes: A-002 to A-005

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


Date: 2026-02-28  
Audience: Development teams consuming the UxXTS package

## Scope

This release closes remediations `A-002` through `A-005` from the UxXTS framework gap analysis.

## What Changed

1. `A-002` Assertion Dialect Alignment
   - Canonical assertion grammar (`path` + `op` + `expected`) is now the normative contract.
   - Legacy matcher-style assertions remain supported for compatibility during migration.
   - Conformance suite updated to use canonical grammar, with explicit legacy compatibility coverage.

2. `A-003` Hash Method Contract Clarification
   - Canonical method is now `sha256-c14n-v1`.
   - `sha256-jcs` is retained as a legacy alias.
   - Lint tooling now validates declared hash method values and warns on legacy alias usage.

3. `A-004` Runner-Core Deduplication
   - Shared runner-core module added for common primitives (hash and canonical status normalization).
   - UATS and UPTS runners now consume shared helpers to reduce duplication and drift risk.

4. `A-005` Portable Packaging Script
   - Package build script now supports:
     - `--repo-root <path>`
     - `--output <path>`
   - Auto-discovery fallback and source validation checks were added to improve portability.

## Impact for Teams

1. Prefer canonical assertion syntax in all new specs.
2. Existing specs should continue to run, but migration to canonical syntax is recommended.
3. Prefer `sha256-c14n-v1` for new/updated integrity declarations.
4. Delivery packaging can now run from non-default repo/output layouts.

## No Intentional Breaking Changes

The remediation set was implemented with compatibility-first behavior to avoid disruption to active workflows.
