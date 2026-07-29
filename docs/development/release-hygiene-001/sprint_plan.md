# RELEASE-HYGIENE-001 — Sprint Plan

**Date:** 2026-07-28 | **Branch:** `reh3376_dev01`
**Parent trigger:** Q4 frontier deep-dive candidate #5 (RELEASE-PIN-001 +
GRAFANA-SHIP-001, paired ~2d).

## 1. Header & Metadata

Two paired cheap closeouts. Both are the last operational commitments
from the Q3 roadmap; both are ~1-hour edits, not 1-day sprints. Combined
in one sprint dir + one commit for CHANGELOG/CLAUDE.md footprint
economy. Risk low: both are additive.

## 2. Problem Statement

**RELEASE-PIN-001**: MDEMG_VERSION pinning is currently unsatisfiable
via GHCR. Investigation showed:
- `docker-publish.yml` triggers on `push:main / workflow_run:[Release] /
  schedule / workflow_dispatch`, NEVER directly on `push:tags`
- The semver tag metadata IS configured (`type=semver,
  pattern=v{{version}}`) but only extracts when the git ref is a tag
- The `workflow_run` chain from `release.yml` hasn't fired in the last
  20 docker-publish runs — visible events are only `push` (from main
  merges) and `schedule` (weekly Monday). Neither produces a semver tag
- Result: an operator pinning `MDEMG_VERSION=v0.10.1` gets nothing —
  GHCR carries only `main` + `latest`

**GRAFANA-SHIP-001**: fresh installs get a blank Grafana. Investigation
showed:
- `internal/cli/compose_templates/docker-compose.yml` mounts 5 Grafana
  provisioning paths as RELATIVE (`./deploy/docker/grafana/...`)
- A fresh Homebrew install runs `mdemg init` in the user's cwd, writing
  `docker-compose.yml` there
- The `deploy/docker/grafana/` tree is NOT in the operator's cwd — those
  files live in the mdemg repo, which the operator doesn't have
- Result: `docker compose up` either fails on missing mounts OR mounts
  empty dirs; Grafana boots without dashboards, datasources, or alerts

Both are silent-failure-shape: no error at install time, feature just
doesn't work when the operator tries to use it.

## 3. Scope & Constraints

**In scope (two independent epics):**

- **E1 (RELEASE-PIN)**: single YAML edit — add `push: tags: ['v*']` to
  `.github/workflows/docker-publish.yml`. Verify semver metadata already
  in place. No workflow-file duplication.
- **E2 (GRAFANA-SHIP)**: embed the 14 `deploy/docker/grafana/` files
  via `embed.FS`; materialize on `mdemg init` alongside the compose
  file; unchanged compose-template mount paths still resolve.

**Out of scope:**

- Refactoring release.yml or removing the `workflow_run` trigger from
  docker-publish.yml (add-not-replace; keeps the chain alive as a
  backup)
- Any Grafana dashboard content changes (that's DASHBOARD-TRUTH-*
  territory)
- Homebrew formula updates (goreleaser + homebrew-mdemg handle that)

## 4. Dependencies

- shipped `docker-publish.yml` (already has semver tag metadata)
- shipped `internal/cli/init.go` (compose materialization hook)
- shipped `internal/cli/compose_templates/embed.go` (embed.FS precedent)

## 5. Implementation Plan (2 independent epics + docs)

**E1 — RELEASE-PIN**
- Edit `.github/workflows/docker-publish.yml` `on:` block:
  add `push: { tags: ['v*'] }` alongside the existing `push: branches:
  [main]`. Combining both under one `push:` key is standard YAML.
- Keep the `paths:` filter on branch pushes; tag pushes should NOT
  filter on paths (we always want a docker build when releasing).
- Preserve `workflow_run: [Release]` as a backup path.
- **Gate**: YAML validates (`python3 -c 'import yaml; yaml.safe_load(open(...))'`);
  the semver metadata extraction is already tested in place.

**E2 — GRAFANA-SHIP**
- New `internal/cli/grafana_templates/embed.go` — `embed.FS` over
  `deploy/docker/grafana/**` (7 dashboards + 4 datasources + alerting +
  notifiers + dashboards.yml = 14 files).
- `Materialize(dir string) error` walker: extracts every embedded file
  under `dir/deploy/docker/grafana/` preserving the tree — so the
  compose template's `./deploy/docker/grafana/...` mount paths
  resolve without changes.
- Hook in `internal/cli/init.go`: after the compose file is materialized,
  call `grafana_templates.Materialize(cwd)`.
- Idempotent: skip if the files already exist AND their checksums match
  (so `mdemg init` on an existing install doesn't clobber operator
  edits to dashboard JSONs).
- **Gate**: build clean; live drill — `mdemg init --quick` in a scratch
  dir creates `deploy/docker/grafana/**` populated with 14 files.

**E3 — Docs**
- CHANGELOG entry under `### Fixed` (both are shipped-feature-gaps, not
  new capability)
- CLAUDE.md architectural note: pin the "cwd-relative mounts require
  material files in cwd" invariant for future compose extensions

## 6. Testing Plan (3 tiers)

- **Tier 1 (unit):**
  - `grafana_templates_test.go` — `Materialize` writes the 14 files
    correctly under a t.TempDir(); idempotency test (re-run doesn't
    clobber edited files); manifest test (embed contains exactly the
    expected file count so a missing file fails-fast at CI).
- **Tier 2 (contract):** `go build`, `golangci-lint 0 issues`,
  `go test ./... -count=1` full green. YAML lint on docker-publish.yml.
- **Tier 3 (live):**
  - **E1**: NOT tested by drill — would require pushing a real tag,
    which cuts a release. Contract pin (YAML validity + trigger config
    presence) is sufficient. Real test will happen on the next release.
  - **E2**: `mdemg init --quick` in a scratch dir on mdemg-dev host;
    verify 14 files land at the expected paths; verify a subsequent
    `docker compose config` resolves the mount paths (dry-run — don't
    actually up the extra Grafana instance).

## 7. Commit Strategy

Sequential single-epic commits: E1 → E2 → E3 (or one combined commit
if the epics are small enough; per operator's "no parallelization"
rule sequential is safer).

## 8. Verification Checklist

- [ ] E1: `push: tags: ['v*']` trigger added to docker-publish.yml
- [ ] E1: YAML validates via `yaml.safe_load`
- [ ] E1: semver metadata already in place (unchanged)
- [ ] E2: `grafana_templates/embed.go` embeds all 14 files
- [ ] E2: `Materialize` writes to `dir/deploy/docker/grafana/**`
- [ ] E2: idempotent — re-run doesn't clobber operator-edited files
- [ ] E2: `mdemg init` hook calls Materialize
- [ ] E2: live drill on mdemg-dev host — 14 files materialize
- [ ] `go build`, `golangci-lint 0 issues`, `go test ./...` full green
- [ ] CHANGELOG entry + CLAUDE.md architectural note

## 9. Rollback Procedures

- E1: revert the YAML edit; tag-triggered docker publish stops firing
  directly (workflow_run chain and manual `workflow_dispatch` remain)
- E2: revert the go file + init.go hook; init reverts to compose-only
  materialization; a fresh install regains the blank-Grafana behavior
- No schema change, no substrate mutation.

## 10. Risks & Mitigations

- **Risk (E1)**: dual-trigger (`push:tags` AND `workflow_run:[Release]`)
  produces two docker builds per release.
  - **Mitigation**: `workflow_run` hasn't fired in visible history —
    the "duplicate" concern is theoretical. If it becomes a real
    duplicate, drop the `workflow_run` in a follow-up.
- **Risk (E2)**: operator edits a dashboard JSON in place; `mdemg init`
  clobbers the edit on re-run.
  - **Mitigation**: idempotency check via checksum — only overwrite when
    the file matches the last-shipped embed OR is missing. Operator
    edits are preserved.
- **Risk (E2)**: the embed doubles binary size.
  - **Mitigation**: the 14 files total ~50-200KB (dashboards are JSON,
    provisioning is YAML). Immaterial.

## 11. Documents Accessed

Filled in commit message.

## 12. Documentation Update

Final epic — never cut (Sprint Plan Format v1.0). Covered by E3.
