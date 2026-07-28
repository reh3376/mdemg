# RELEASE-HYGIENE-001 — Sprint Post

**Date:** 2026-07-28 | **Branch:** `reh3376_dev01`
**Parent trigger:** Q4 frontier deep-dive candidate #5.

## Verdict

**Shipped.** Two paired closeouts in one commit: RELEASE-PIN (tag-triggered
docker publish) + GRAFANA-SHIP (embed + materialize the Grafana
provisioning tree). Together they close the last operational
commitments from the Q3 roadmap.

## What shipped (two epics, one commit)

**E1 — RELEASE-PIN**
- Added `push: tags: ['v*']` trigger to `.github/workflows/docker-publish.yml`
- Investigation found the file ALREADY had semver metadata configured
  (`type=semver,pattern=v{{version}}`), but the trigger set (`push:main /
  workflow_run:[Release] / schedule / workflow_dispatch`) never reached
  the workflow with a tag ref
- Preserved the `workflow_run:[Release]` chain as a backup path
- Semver metadata unchanged — will extract `v0.10.1` → tags `v0.10.1` + `v0.10`

**E2 — GRAFANA-SHIP**
- New package `internal/cli/grafana_templates/` with `//go:embed all:staged`
  over a 15-file mirror of `deploy/docker/grafana/**`
- `Materialize(destDir)` writes files under `<destDir>/deploy/docker/grafana/**`
  matching the compose template's cwd-relative mount base
- Idempotent + operator-edit-safe: uses sha256 compare to distinguish
  "already-materialized" from "operator-edited" — never overwrites
  operator changes
- Wired into `mdemg init` right after the docker-compose.yml
  materialization step; reports written vs preserved counts
- Because `//go:embed` can't cross package boundaries or use `..`, the
  canonical `deploy/docker/grafana/**` files are mirrored into
  `internal/cli/grafana_templates/staged/**` via a new
  `make sync-grafana-embed` target
- CI check `make verify-grafana-embed` fails on drift (added to
  `.github/workflows/ci.yml` alongside the DOC-CURRENCY-002 verifiers)

## Testing

- **Tier 1 (unit)**: 6 tests in `grafana_templates/embed_test.go`
  - `TestManifest_ExpectedFileCount` — file count pin (15)
  - `TestMaterialize_WritesToExpectedTree` — files land at
    `<dst>/deploy/docker/grafana/**`
  - `TestMaterialize_Idempotent_SameContent` — re-run preserves all 15
    files without writing
  - `TestMaterialize_PreservesOperatorEdits` — sha256-different files
    are preserved (invariant)
  - `TestMaterialize_EmptyDestErrors` — API contract
  - `TestMaterialize_ComposeMountPathsResolve` — `DeployRelPath` constant
    pin (changing it silently breaks every fresh install's Grafana)
- **Tier 2 (contract)**: `go build ./...`, `golangci-lint 0 issues`,
  `go test ./... -count=1` full green. YAML validates for
  docker-publish.yml. `make verify-grafana-embed` green locally.
- **Tier 3 (live)**:
  - E1: NOT tested by drill — would require pushing a real `v*` tag,
    which cuts a release. Contract pin (YAML validity + trigger config
    presence) is sufficient. Real test happens on the next release.
  - E2: `Materialize` verified against t.TempDir() in unit tests;
    equivalent to a fresh-cwd drill. Not run against the operator's
    live install since the shipped `deploy/docker/grafana/**` tree
    already exists there.

## Rules pinned

1. **When a compose service mounts cwd-relative paths, the operator's
   cwd MUST contain those paths after `mdemg init`.** Add an embed +
   materialize package following the `grafana_templates` shape rather
   than expecting the operator to clone the repo.
2. **`//go:embed` can't cross package boundaries or use `..`.** When
   embedding files that live outside the embedding package's directory,
   mirror them into `<package>/staged/**` via a Makefile target + CI
   drift check. Editing the canonical files without running the sync
   ships a stale embed.
3. **Idempotent materialization uses sha256 compare, not just
   existence.** An "already materialized" file has the SAME sha as the
   embed; an "operator-edited" file has a DIFFERENT sha; both are
   preserved (never overwritten). Only genuinely-missing files are
   written.

## Known limitations

- **E1 real test happens on next release.** The trigger config change
  is validated by YAML + contract-pin; the first `v*` tag push after
  merge will confirm docker-publish fires and semver tags land on GHCR.
- **The staged/ mirror doubles the tracked file count** for Grafana
  provisioning (14 files in `deploy/docker/grafana/` + 15 in
  `internal/cli/grafana_templates/staged/`). Trade-off vs symlinks or
  code-generation; the drift check keeps them in sync.

## Follow-ups disclosed

- **Verify E1 on next release**: the first `v*` tag push (e.g. a
  future `v0.11.0`) should populate GHCR with `v0.11.0` + `v0.11` tags.
  If it doesn't, the trigger config needs additional debugging (paths
  filter interaction, permissions, etc.).
- **Consider retiring the `workflow_run:[Release]` trigger** if it
  remains dormant post-merge. Would remove a "did it fire?" ambiguity
  and rely on the direct `push:tags` path.

## Documents Accessed

- `docs/development/q4-frontier-scoping/DEEP_DIVE_2026-07-27.md` (candidate #5)
- `docs/development/release-hygiene-001/sprint_plan.md` (this dir)
- `.github/workflows/{docker-publish,release,ci}.yml`
- `internal/cli/compose_templates/embed.go` (pattern precedent)
- `internal/cli/init.go` (compose materialization hook)
- `deploy/docker/grafana/**` (15 files mirrored)
- `Makefile` (sync + verify targets)
- Live GHCR API check via `gh api /users/reh3376/packages/container/mdemg/versions`
- `gh run list --workflow docker-publish.yml` (trigger-event audit)
