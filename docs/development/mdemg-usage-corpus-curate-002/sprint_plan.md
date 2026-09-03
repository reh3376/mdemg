# MDEMG-USAGE-CORPUS-CURATE-002 — Sprint Plan

**Task**: #148
**Predecessor**: MDEMG-USAGE-CORPUS-CURATE-001 (task #144) shipped the initial exporter + curator pair
**Paired sibling**: MDEMG-DOCS-INGEST-002 (task #147) — this sprint closes the retroactive-tombstone follow-up disclosed there
**Type**: bug fix (data-hygiene envelope on shipped exporter + curator) + retroactive substrate cleanup
**Wall-clock estimate**: ~1h

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint | MDEMG-USAGE-CORPUS-CURATE-002 |
| Task # | #148 |
| Branch | `reh3376_dev01` (auto-PR flow) |
| Substrate touch? | ONE retroactive tombstone on the leaked idna LICENSE.md node (fully reversible) |
| Reversible? | ✅ trivially — code revert restores pre-fix behavior; tombstone reverse is `SET n.is_archived=false REMOVE n.archive_reason, n.archived_at` |
| Related tasks | #147 MDEMG-DOCS-INGEST-002 (paired follow-up); #144 MDEMG-USAGE-CORPUS-CURATE-001 (predecessor); #145 MDEMG-USAGE-LORA-001 (surfaced 3 claude-docs leak rows) |

## 2. Problem Statement

The shipped `mdemg_usage_export_docs.py` uses a WHERE clause of 5× `n.path CONTAINS '<sub>'` predicates:

```cypher
WHERE n.path IS NOT NULL AND (
  n.path CONTAINS 'docs/features' OR
  n.path CONTAINS 'docs/user' OR
  n.path CONTAINS 'docs/api' OR
  n.path CONTAINS 'cli-reference' OR
  n.path CONTAINS 'CLAUDE.md'
)
```

`CONTAINS 'docs/api'` matches any path containing `docs/api` ANYWHERE — including symbol nodes like `/docs/api/api-spec/uats/.venv/lib/python3.12/site-packages/urllib3/exceptions.py#EmptyPoolError`. On the 2026-08-24 snapshot the raw JSONL contained **1,583 rows**, of which the path-prefix breakdown was:

| Prefix | Count | Class |
|---|---|---|
| `mdemg-docs/*` | 1,065 | ✅ legitimate (produced by `mdemg-docs-ingest`) |
| (empty prefix from `/docs/…`) | 504 | ❌ Python symbol leaks from vendored `.venv/lib/python3.12/site-packages/**/*.py#Symbol` extraction |
| `claude-docs/*` | 13 | ❌ different corpus (`mdemg claude-docs-ingest`) |
| `CLAUDE.md` | 1 | ⚠️ code symbol (not the mdemg-docs-ingest'd CLAUDE.md) |

**Downstream impact**: MDEMG-USAGE-LORA-001 (#145) trained a LoRA adapter on a corpus that included some fraction of these leaked rows. The verdict — PARITY-WITH-TRADEOFFS but NO-PROMOTE — was NOT caused by this pollution (the delta was dominated by CCK reduced-scope training), but it added noise to the corpus.

**Also unresolved**: #147 disclosed a retroactive tombstone of the leaked idna LICENSE.md MemoryNode (`n_fbd44e97d4604cb1067d` at path `mdemg-docs/api/license/000__whole-file`) — that leak came THROUGH mdemg-docs-ingest itself (a `.md` file under `docs/api/api-spec/uats/.venv/…/idna-3.11.dist-info/licenses/LICENSE.md`, now-excluded by #147's fix but the pre-fix node still lives in the substrate). Belongs here.

## 3. Scope & Constraints

**In scope (Epic 1 — exporter root cause)**:
- New `MDEMG_DOCS_PATH_PREFIX = "mdemg-docs/"` constant
- WHERE clause tightened from 5× `CONTAINS 'sub'` to single `STARTS WITH 'mdemg-docs/'`
- `classify_surface(path)` re-anchored to require the prefix + sub-surface segment (`features/`, `user/`, `api/`, `cli-help/`, `claude/`)

**In scope (Epic 2 — curator defense-in-depth)**:
- `is_valid_mdemg_docs_path(path)` predicate + gate in `build_qa_row`
- New `wrong_prefix` skip reason tracked in report

**In scope (Epic 3 — retroactive tombstone)**:
- One-shot Cypher: `SET is_archived=true, archive_reason='mdemg_docs_ingest_002_venv_leak_purge', archived_at=datetime()` on `n_fbd44e97d4604cb1067d`
- Broader-sweep safety check: verify no other `mdemg-docs/*` node has a `.venv`/`__pycache__`/`node_modules`/`.dist-info`/`.egg-info` path substring before tombstoning

**In scope (Epic 4 — tests + live smoke)**:
- Update `test_classify_surface` for tightened prefix + add new negative regressions
- Add `test_is_valid_mdemg_docs_path` (11 subcases)
- Add `test_build_query_starts_with_prefix` (regression-lock the WHERE clause)
- Tier-3 live smoke: run curator against real raw JSONL, verify `wrong_prefix` count matches expected 518

**Out of scope (deferred to disclosed follow-ups)**:
- Retrain the LoRA adapter on the cleaned corpus — #145 is closed; a future retrain is its own sprint decision
- Audit `mdemg extract-symbols` for `.venv/` exclusion — DIFFERENT class (SymbolNode, not MemoryNode); the 504 vendored-symbol rows are from `mdemg extract-symbols`, not from `mdemg-docs-ingest`; separate sprint
- Automated cleanup of pre-fix leaked rows on future re-runs — the exporter fix alone prevents new leaks; the tombstone here is one-shot

**Constraints preserved**:
- `never-hardcode-config` — the prefix is a module-level const with a comment explaining WHY hardcoding is correct (it's the exact contract with `mdemg-docs-ingest`)
- `plan-mode-before-change` — this doc IS the plan; small, well-scoped, single substrate mutation is well-understood
- `unit-integration-e2e-docs` — Tier 1 (67 unit tests), Tier 2 (regression re-runs of full existing suite), Tier 3 (live curator run against real raw JSONL + live cypher-shell tombstone)
- `live-testing-tier-required` — real substrate mutation via `cypher-shell` on the real Neo4j
- `iterate-break-fix-verify` — verified end-to-end: exporter WHERE clause change → curator gate → live smoke count matches predicted 518 → tombstone leaves the leaked node correctly archived

## 4. Dependencies

- Neo4j reachable via docker (`docker exec mdemg-neo4j-1 cypher-shell`) — required for Epic 3 substrate mutation
- Python stdlib only (no new pip installs)
- No CI dependencies

## 5. Implementation Plan (sequential epics + gates)

**Epic 1 — Exporter WHERE clause + classify_surface tightening**:
1. `MDEMG_DOCS_PATH_PREFIX` const
2. Rewrite `SurfaceRule` from `.contains` to `.subprefix` (post-prefix segment)
3. Rewrite `classify_surface` to require prefix + subprefix match
4. Rewrite `build_query` WHERE clause to `n.path STARTS WITH '{MDEMG_DOCS_PATH_PREFIX}'`
5. Update module docstring

**Gate**: `python3 -c "from mdemg_usage_export_docs import build_query; print(build_query())"` shows `STARTS WITH` + no `CONTAINS 'docs/*'`.

**Epic 2 — Curator defense-in-depth**:
1. `MDEMG_DOCS_PATH_PREFIX` const (curator side; comment cross-refs exporter)
2. `is_valid_mdemg_docs_path(path)` predicate
3. Gate in `build_qa_row` (rejects → return None)
4. Add `wrong_prefix` skip reason in the diagnosis block

**Gate**: pin tests in Epic 4 catch the class.

**Epic 3 — Retroactive tombstone**:
1. Broader-sweep query: find any `mdemg-docs/*` MemoryNode with venv/pycache/node_modules/dist-info/egg-info in path
2. If sweep returns > 1 node, expand tombstone to cover all matching nodes with the same `archive_reason`
3. Otherwise: single-node tombstone on `n_fbd44e97d4604cb1067d`

**Gate**: post-tombstone re-query verifies `is_archived=true` + `archive_reason` set + `archived_at` recent.

**Epic 4 — Tests + docs + commit**:
1. Update `test_classify_surface` — remove pre-#148 loose asserts, add positive/negative for new prefix
2. Add `test_is_valid_mdemg_docs_path` (11 subcases including edge `mdemg-docs-fake/` exact-prefix guard)
3. Add `test_build_query_starts_with_prefix` (regression-lock the WHERE clause)
4. Add prefix-rejection cases to `test_build_qa_row`
5. Tier-3 live smoke: run curator against `training_data/mdemg_usage/raw/nodes.jsonl`, verify `wrong_prefix=518`, `0 leaks in output`
6. Write sprint plan + post
7. Update CLAUDE.md architecture note
8. Update CHANGELOG.md Unreleased entry
9. Commit + push + auto-PR

**Gate**: 67 pin tests pass; live curator output shows 0 leaks + wrong_prefix count matches sweep; PR comment posted.

## 6. Testing Plan (3 tiers)

**Tier 1 — Unit** (all in `scripts/test_mdemg_usage_curate.py`):
- 8 subcases in `test_classify_surface` (5 positive + 6 negative + 2 misc)
- 11 subcases in `test_is_valid_mdemg_docs_path`
- 6 subcases in `test_build_query_starts_with_prefix` (regression-locks the WHERE clause + preserved invariants)
- 4 new negative cases in `test_build_qa_row`
- Total: 29 new/updated subcases + full pre-existing suite still passes

**Tier 2 — Integration**:
- Full curator run against real raw JSONL: 1,583 in → 806 out with `wrong_prefix: 518` skip reason accounted-for

**Tier 3 — E2E live**:
- Docker `cypher-shell` broader-sweep verifies only 1 leaked node in scope
- Docker `cypher-shell` mutation writes `is_archived=true` + `archive_reason` + `archived_at`
- Post-mutation re-query confirms the tombstone
- Reversal command pre-verified (documented in Rollback §11)

## 7. Commit Strategy

Single commit — code + tests + docs + sprint dir + one-shot cypher (documented in sprint post, not stored as a `.sql` file since it's a one-time cleanup). Small, cohesive, atomic.

## 8. Verification Checklist

- [x] Exporter WHERE clause uses `STARTS WITH 'mdemg-docs/'`
- [x] Curator has `is_valid_mdemg_docs_path` gate wired
- [x] `wrong_prefix` skip reason tracked in report
- [x] 67 unit tests pass (all pre-existing + new)
- [x] Tier-3 live smoke: real curator on real raw JSONL → 806 out, `wrong_prefix: 518`, 0 leaks
- [x] Retroactive tombstone applied to `n_fbd44e97d4604cb1067d` — verified `is_archived=true`
- [x] Broader sweep — no other in-scope leaks
- [x] Feature doc surface unchanged (no feature doc for a bug fix per `mandatory-feature-docs` scope)
- [x] Sprint dir populated
- [x] CLAUDE.md architecture note pinned
- [x] CHANGELOG.md Unreleased entry
- [x] PR sprint summary comment posted

## 9. Documentation Update

- `scripts/mdemg_usage_export_docs.py` — docstring updated to name the tightening
- `scripts/mdemg_usage_curate.py` — docstring notes the defense-in-depth prefix gate
- `docs/development/mdemg-usage-corpus-curate-002/{sprint_plan,sprint_post}.md` — this sprint
- `CLAUDE.md` — 1 arch rule pinned (see sprint post § arch rule)
- `CHANGELOG.md` — Unreleased entry

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Prefix tightening rejects a legitimate node I don't know about | Broader-sweep + spot-check confirm ONLY the 1 known leak node exists at the exclusion pattern; per-surface count of 1,065 mdemg-docs/* rows matches the exporter's expected output |
| Tombstone on the wrong node | Node identity verified 3× (path lookup by full path returns 1 node; node_id `n_fbd44e97d4604cb1067d`; content is the idna LICENSE.md text) |
| Retrain corpus now smaller (806 vs 949 pre-fix) drops below quality gate | The pre-fix 949 CONTAINED the polluted 143 rows; the 806 clean rows are strictly better signal density. Any future retrain uses the cleaner corpus; #145's shipped adapter is unaffected (it's the corpus that was polluted, not the model in production — v1 stays production regardless) |

## 11. Rollback

Full rollback for code + substrate:

**Code**: `git revert <commit-sha>` → exporter reverts to loose CONTAINS predicates, curator loses prefix gate. Next raw JSONL re-export produces the pre-#148 polluted result.

**Substrate tombstone** (if operator needs the leaked idna LICENSE node restored for any reason):

```cypher
MATCH (n:MemoryNode {space_id:'mdemg-dev', node_id:'n_fbd44e97d4604cb1067d'})
WHERE n.archive_reason = 'mdemg_docs_ingest_002_venv_leak_purge'
SET n.is_archived = false
REMOVE n.archive_reason, n.archived_at
RETURN n;
```

## 12. Documents Accessed

- `scripts/mdemg_usage_export_docs.py` — target of Epic 1 code change
- `scripts/mdemg_usage_curate.py` — target of Epic 2 code change
- `scripts/test_mdemg_usage_curate.py` — extended with 29 new/updated subcases
- `scripts/mdemg_usage_leak_audit.py`, `mdemg_usage_lora_assemble.py`, `mdemg_usage_manifest.py`, `mdemg_usage_eval.py`, `split_mdemg_usage_v1.py` — audited for path-predicate usage (none had loose `CONTAINS` on path — safe)
- `docs/development/mdemg-usage-corpus-curate-001/` — predecessor sprint dir (established the exporter/curator surface)
- `docs/development/mdemg-docs-ingest-002/` — paired sibling sprint (leaked node source-of-truth)
- `docs/development/mdemg-usage-lora-001/` — established that 3 claude-docs rows had slipped through
- `internal/cli/mdemg_docs_ingest.go` — established `mdemg-docs/{surface}/…` path shape contract
- `training_data/mdemg_usage/raw/nodes.jsonl` — real 1,583-row raw export from 2026-08-24 used for Tier-3 smoke
- Live Neo4j (`docker exec mdemg-neo4j-1 cypher-shell`) — real substrate mutation
- CLAUDE.md pins: `plan-mode-before-change`, `never-hardcode-config`, `unit-integration-e2e-docs`, `live-testing-tier-required`, `iterate-break-fix-verify`, `must-master-data-pipelines`, `end-with-docs-accessed`, `must-comment-sprint-summary-on-pr`, `lint-before-commit`, `must-follow-12-section-format`, `never-direct-alter-schema` (verified inapplicable — no schema change, only a per-row property update on shipped fields)
- Operator directive: "proceed with #148" (2026-09-03)
