# INSTANCE-ID-PIN-001 — Sprint Plan

## 1. Header & Metadata

- **Sprint ID**: INSTANCE-ID-PIN-001
- **Roadmap slot**: Q5 §5 (post-roadmap self-disclosed follow-up)
- **Parent**: INSTANCE-ID-HOSTNAME-DRIFT-001 bug (found 2026-09-22 during dashboard-metrics review)
- **Date**: 2026-09-25
- **Effort**: 2-3h
- **Impact class**: operational (real production data-loss class closer)

## 2. Problem Statement

**INSTANCE-ID-HOSTNAME-DRIFT-001** (2026-09-22): a hostname change silently broke `export-auto` because `resolveInstanceID` derives `<hostname>-<space>` at every runtime call — TSDB rows land tagged with the hostname-of-the-day, but the exporter's WHERE clause `instance_id = $4` uses the CURRENT hostname-of-the-day. Any hostname change orphans all pre-change rows from every hostname-scoped query.

Live evidence: 2026-09-22 export-auto failure — 298 rows in the 24h window all had `instance_id="Mac.lan-mdemg-dev"` (previous hostname); current hostname is `reh3376s-MacBook-Pro.local`; filter matched **zero** rows → HIGH `no llm_interactions data found` alert.

**Mitigation A** shipped 2026-09-22 (`.env` pin `MDEMG_INSTANCE_ID=Mac.lan-mdemg-dev`) unblocked the immediate alert. **This sprint ships the real fix per Option 1 in the hostname-drift bug analysis** (`docs/development/dashboard-metrics-review-2026-09-21/hostname_drift_bug.md`): auto-pin the hostname-derived instance_id to a pin-file at first-run.

## 3. Scope & Constraints

**In-scope**:
- `internal/cli/root.go::resolveInstanceID` extended with pin-file logic:
  - Resolution priority: `explicit` arg > `MDEMG_INSTANCE_ID` env > **pin-file at `~/.mdemg/instance_id`** > `<hostname>-<space>` derivation
  - When falling back to hostname derivation, WRITE the derived value to the pin-file for future runs (first-run persistence)
  - Pin-file format: newline-terminated single string (matches other `.mdemg/` file conventions)
  - Path derivation via `os.UserHomeDir()` — no hardcoded absolute paths
- Config knob `MDEMG_INSTANCE_ID_PIN_PATH` (env var only, no config-struct field needed since resolveInstanceID is called before Config init in some paths) — default `~/.mdemg/instance_id`; empty/`=-` disables pinning (fallback to shipped hostname behavior, matches MDEMG-DOCS-INGEST-002's `=-` escape hatch pattern)
- 5 pin tests via `t.TempDir()` covering: explicit arg wins; env wins over pin-file; pin-file wins over hostname derivation; first-run derives + writes; corrupt/malformed pin-file falls back to derivation with WARN; `=-` disables pin fully
- Live Tier-3 smoke on mdemg-dev: remove the `.env` `MDEMG_INSTANCE_ID` line (mitigation A); delete `~/.mdemg/instance_id`; run `mdemg data export-auto`; verify pin-file gets written with `reh3376s-MacBook-Pro.local-mdemg-dev` value (current hostname) AND that export still works (finds `Mac.lan-mdemg-dev` rows because... wait no — this is the migration edge)

**⚠️ Migration consideration**: this fix ALONE doesn't heal the existing broken state. A machine with prior data tagged `<old-hostname>-*` needs one of:
- Keep the `MDEMG_INSTANCE_ID` env pin (mitigation A) — the pin-file will be initialized with that value on first run (env still takes precedence during resolution) but new tag-writes are already using the env-pinned value, so pin-file just backs up the setting
- OR bootstrap the pin-file with the old hostname manually: `echo "Mac.lan-mdemg-dev" > ~/.mdemg/instance_id`
- OR accept that the pin-file mechanism prevents FUTURE drift, and past rows stay accessible via the shipped `.env` mitigation

This sprint chooses **prevent future drift only** — no data migration. The .env mitigation stays as belt-and-suspenders.

**Out-of-scope**:
- Retroactive relabeling of historical rows (would be data-migration; separate sprint if ever needed)
- Extending the pin pattern to other hostname-derived identifiers (backup filenames, cache keys, etc.) — audit disclosed as follow-up per hostname_drift_bug.md's arch rule proposal, not in scope here
- Extending exporter WHERE clause to also match `instance_id IS NULL` — mentioned as Option 2 in the bug doc but rejected as "doesn't fix the hostname-drift class"

**Constraints**:
- `must-follow-12-section-format` — this doc ✓
- `never-hardcode-config` — path env-tunable; default via `os.UserHomeDir()`
- `unit-integration-e2e-docs` — 3 tier plan below
- `live-testing-tier-required` — real binary against real filesystem + real TSDB
- `no-direct-main-commits` — auto-PR path
- `end-with-docs-accessed` — §12 populated

## 4. Dependencies

- **Upstream (live)**:
  - `internal/cli/root.go::resolveInstanceID` — extension point
  - `internal/tsdb/exporter.go` — consumes the resolved instance_id
- **Downstream (unblocked)**:
  - Cross-space parity for other hostname-derived identifiers (future sprint)

## 5. Implementation Plan

Sequential; single epic.

### Epic 1 — Pin-file logic + tests

- **E1.1**: Refactor `resolveInstanceID(explicit, spaceID string) string`:
  1. If `explicit != ""` → return explicit (unchanged)
  2. If `os.Getenv("MDEMG_INSTANCE_ID") != ""` → return env (unchanged)
  3. Compute pin-path: `MDEMG_INSTANCE_ID_PIN_PATH` env override; if unset default `~/.mdemg/instance_id`; if env=`=-` disable pin entirely (skip to step 5)
  4. **NEW**: If pin-file exists AND non-empty → read + trim + return the pinned value
  5. Derive `<hostname>-<space>` (unchanged)
  6. **NEW**: If pinning enabled AND path is writable → mkdir parent + write derived value to pin-file (best-effort; log-and-continue on failure)
  7. Return derived value
- **E1.2**: Pin tests in `internal/cli/root_test.go` (or new `instance_id_test.go` if root_test doesn't exist):
  - `TestResolveInstanceID_ExplicitArgWins`
  - `TestResolveInstanceID_EnvWinsOverPinFile`
  - `TestResolveInstanceID_PinFileWinsOverHostname`
  - `TestResolveInstanceID_FirstRunWritesPinFile`
  - `TestResolveInstanceID_MalformedPinFallsThrough`
  - `TestResolveInstanceID_DashDashDisablesPin`
  - All via `t.TempDir()` for pin-path override; use `t.Setenv` for env var manipulation

- **Gate**: `go build ./...` clean; new tests + existing tests green; `golangci-lint run ./internal/cli/...` 0 issues.

### Epic 2 — Live Tier-3 smoke

- **E2.1**: On mdemg-dev, remove the mitigation A pin from `.env` (temporarily); delete `~/.mdemg/instance_id` if it exists.
- **E2.2**: Run `MDEMG_INSTANCE_ID= mdemg data export-auto --space-id mdemg-dev --keep 30` — expect FAILURE (would-be new-hostname derivation, no matching rows). This validates the pre-fix behavior is still reproducible.
- **E2.3**: Build the sprint's mdemg binary → rerun the same command with pin-file empty → verify: (a) pin-file gets created at `~/.mdemg/instance_id` with the derived value `reh3376s-MacBook-Pro.local-mdemg-dev`, (b) export STILL fails (because rows have `Mac.lan-*` tag; pin created wrong-hostname value on first run). This is EXPECTED for the fresh-machine case; a machine with prior data needs a bootstrap step (see §3 migration note).
- **E2.4**: Simulate the correct bootstrap: `echo "Mac.lan-mdemg-dev" > ~/.mdemg/instance_id` → rerun export → verify it succeeds (finds the historical rows) AND that subsequent runs continue to use the pinned value regardless of hostname changes.
- **E2.5**: Restore mitigation A (re-add `.env` MDEMG_INSTANCE_ID line) as belt-and-suspenders. Env still wins over pin-file per the resolution priority.
- **Gate**: pin-file created + read live; export-auto works with pin-file mechanism live.

### Epic 3 — Documentation

- Feature doc: N/A (existing feature; behavior extension)
- CLAUDE.md pin: NEW arch rule ("hostname as identity is fragile — pin at first-use") — proposed in hostname_drift_bug.md, formalized here.
- CHANGELOG.md entry
- Sprint post `docs/development/instance-id-pin-001/sprint_post.md` with live-smoke results

## 6. Testing Plan

**Tier 1 — Unit**:
- 6 new pin tests in `internal/cli/instance_id_test.go` (see §5 E1.2)

**Tier 2 — Integration**:
- Existing `internal/cli/` test suite green (no code invoking `resolveInstanceID` outside test contexts is affected — resolution priority is backward-compat when pin-file is empty/missing)
- Existing `internal/tsdb/exporter_test.go` green (exporter reads what resolveInstanceID gives it; unchanged interface)

**Tier 3 — Live e2e**: See §5 E2 above.

## 7. Commit Strategy

Two commits:
1. `feat(cli): pin hostname-derived instance_id at first-run (INSTANCE-ID-PIN-001)`
2. `docs: INSTANCE-ID-PIN-001 sprint post + CLAUDE.md pin + CHANGELOG`

## 8. Verification Checklist

- [ ] `go build ./...` clean
- [ ] `go test ./internal/cli/... -count=1` green (6 new pins + existing)
- [ ] `golangci-lint run ./internal/cli/...` 0 issues
- [ ] All 5 drift checks clean (docs / metrics / config / tsdb / routes)
- [ ] Live Tier-3 (§5 E2): pin-file mechanism verified end-to-end against real filesystem + real TSDB
- [ ] Sprint post reflects actual smoke results
- [ ] PR summary comment posted per `must-comment-sprint-summary-on-pr`

## 9. Documentation Update

- CLAUDE.md architecture note (new arch rule: hostname-as-identity is fragile — pin at first-use)
- CHANGELOG.md Unreleased/Added entry
- Sprint post
- ROADMAP_2026Q5.md §5 checkbox update (informational — INSTANCE-ID-HOSTNAME-DRIFT-001 follow-up closed)

## 10. Risks & Mitigations

| Risk | Class | Likelihood | Impact | Mitigation |
|---|---|---|---|---|
| Fresh-install operator gets wrong-hostname pinned on first run | migration | HIGH on first install | LOW (env pin overrides + data isn't lost) | Env var still wins over pin-file; operators with prior data set `MDEMG_INSTANCE_ID` in `.env` per shipped mitigation A. Sprint post documents the bootstrap step for machines with prior TSDB data. |
| Pin-file write fails silently | operational | LOW | LOW | Best-effort write with WARN log; derived value still returned; next run retries. |
| Pin-file corrupted → resolveInstanceID returns garbage | correctness | LOW | LOW-MED | Pin test `TestResolveInstanceID_MalformedPinFallsThrough` covers this — malformed content (empty, whitespace-only, non-string) → WARN + fall through to hostname derivation. |
| Concurrent processes write pin-file simultaneously (race) | correctness | VERY LOW | LOW | Write is atomic via tmp+rename pattern; last-writer wins (all processes derive same value on same host); no data loss. |
| `=-` escape-hatch env value collides with a real value | UX | VERY LOW | LOW | Matches shipped MDEMG-DOCS-INGEST-002 pattern; documented in CLI Long help. |

## 11. Rollback Procedures

Non-destructive:
1. `git revert <commit-shas>` — undoes the pin-file logic; resolveInstanceID reverts to hostname-only derivation
2. `rm ~/.mdemg/instance_id` if the pinned value should NOT be preserved post-revert
3. `.env` mitigation A (`MDEMG_INSTANCE_ID=...`) stays working regardless — it's the resolution winner

Zero substrate mutation. Zero migration.

## 12. Documents Accessed

- `docs/development/dashboard-metrics-review-2026-09-21/hostname_drift_bug.md` — the parent bug analysis with Option 1 spec
- `internal/cli/root.go::resolveInstanceID` — extension point
- `internal/tsdb/exporter.go:365 exportTable` — the consumer WHERE clause
- `internal/cli/mdemg_docs_ingest.go` — `=-` escape-hatch pattern to mirror
- `docs/development/mdemg-docs-ingest-002/` — pattern reference
- CLAUDE.md — arch rule slot
- CHANGELOG.md — Unreleased slot
