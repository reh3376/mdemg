# JIMINY-CORPUS-AUDIT-004 Batch Record

**Date:** 2026-08-14
**Operator disposition:** Option B (safe subset) — 6 tombstones + 1 metadata fix
**UNCERTAIN (2):** operator ruled "keep both informational" — no action; both already `is_informational=true`
**Content rewrites (8):** HELD for operator hand-review

## Applied mutations

### Tombstones (6)

All use `archive_reason` prefix `jiminy_corpus_audit_004_operator_option_B_2026-08-14__<class>`. Reversible via un-tombstone.

| node_id | code | class |
|---|---|---|
| qi43sv83g136ds43vsrfjxr0 | auto-250af3293675 (CUIDv2 must_not twin) | dedup_of_must_use_cuid2_dual_mint_class |
| g4tzgs1ffjkykh4v0sf3q553 | must-e2e-live-data-verify | subset_of_po8w22fqiv42_live_testing |
| iy0t8cbin69620sn30po8k5m | trace-all-breaks | merged_into_wyl4j8zswjxt_iterate_break_fix_verify |
| if6icfdlwvvxac68bvddsplh | own-test-failures-immediately | merged_into_a5ax6i2owzuw_never_skip_discovered_issues |
| bc4np1kal8ag4tgaolatrkka | rebase-dev-after-admin-merge | obsolete_superseded_by_sync_dev_after_merge_workflow |
| w17xt3ozfs1baqghrpnqaic0 | never-trust-unordered-samples | junk_contextless_session_artifact |

### Metadata fix (1)

- `necnzrfavaag8uzexngv59xs` `must-follow-12-section-format` — constraint_type `must_not` → `must` (content is a MUST; fix stamped in `metadata_fix_note` property)

## Reversibility

Any single tombstone can be un-applied via:
```cypher
MATCH (n:MemoryNode {node_id: '<nid>'})
SET n.is_archived = false, n.archive_reason = NULL, n.archived_at = NULL
RETURN n.node_id;
```

The metadata fix can be reverted via:
```cypher
MATCH (n:MemoryNode {node_id: 'necnzrfavaag8uzexngv59xs'})
SET n.constraint_type = 'must_not', n.metadata_fix_note = NULL
RETURN n.node_id;
```

## Pre-batch state

Full snapshot of the 37 live rules at pre-batch state saved in `pre_batch_snapshot.json` (this dir).

## Post-batch state (Option B verified)

- Live count: **31** (was 37)
- Corpus health: 20 actionable constraints + 8 informational constraints + 2 actionable corrections + 1 informational correction = 31

## Follow-up: 7 content rewrites (per-item operator-approved, tombstone-and-recreate per round-1 immutable lock)

Executed after Option B, via the shipped `POST /v1/jiminy/rules/{code}/tombstone` + `POST /v1/jiminy/rules?override_dedup=true` flow (2 API calls per rewrite). `JIMINY_RULES_UI_WRITE_ENABLED` temporarily flipped ON via `launchctl setenv` + kickstart; reverted at batch end.

Every rewrite: new node with fresh CUIDv2 node_id; same or renamed `constraint_code`; freshly-embedded content. Old node stays `is_archived=true` with `archive_reason='ui_edit_supersede_jiminy_corpus_audit_004_rewrite_<N>_of_7_2026-08-14[_<variant>]'`.

| # | code | new severity | rename? | Δ chars | rationale |
|---|---|---|---|---|---|
| 1 | `query-mdemg-cms-file-paths` | should | no | 114 → 133 | absolute glob/grep ban unfollowable → 53 ignored/7d; rewrite: "CMS FIRST; glob/grep only for exact-token or CMS-miss fallback" |
| 2 | `project-planning-docs-in-repo-only` | should | no | 753 → 423 | dropped stale FT-OAI clause (dropped 2026-04-22) |
| 3 | `live-testing-tier-required` | must | ✓ from `auto-01288edd49b1` | 1162 → 613 | trim Phase 11.6.x evidence narrative (belongs in CLAUDE.md pin); rename to readable code |
| 4 | `no-direct-main-commits` | must | no | 322 → 399 | generalize hardcoded `mdemg-dev01` → `<handle>_dev<01-09>` pattern |
| 5 | `memory-preservation-backup-integrity` | should | no | 420 → 421 | drop dead item (1) (backups now automated + staleness-alerted per BACKUP-RESTORE-VERIFY-001); renumber 2→1, 3→2, 4→3 |
| 6 | `sequential-epics` | must | ✓ from `auto-c0a62b1da979` | 264 → 274 | generalize away stale sprint-specific "Causal Insertion" reference; rename to readable code |
| 7 | `iterate-break-fix-verify` | must | no | 569 → 471 | trim J17 case narrative; ABSORB `trace-all-breaks` (tombstoned in Option B) — "trace the ENTIRE pipeline, never stop at the first break found" |

### Live-doc cross-ref updates

`docs/development/jiminy-rules-ui-001/sprint_plan.md` — 2 refs updated:
- `auto-01288edd49b1` → `live-testing-tier-required`
- `auto-c0a62b1da979 no-parallel-epics` → `sequential-epics`

Historical docs (`docs/development/jiminy-corpus-003/tombstone_list.md`, `jiminy-roletype-adapter-001/live_verification.md`, `pre_batch_snapshot.json`) left verbatim per the framing-hygiene rule (historical records reference codes as-they-were-at-that-time).

## Rewrite reversibility

Each rewrite is reversible by un-tombstoning the archived twin + tombstoning the current live version:
```cypher
// Undo rewrite N (example: #1)
MATCH (old:MemoryNode {constraint_code: 'query-mdemg-cms-file-paths', is_archived: true})
WHERE old.archive_reason CONTAINS 'rewrite_1_of_7'
SET old.is_archived = false, old.archive_reason = NULL, old.archived_at = NULL;

MATCH (new:MemoryNode {constraint_code: 'query-mdemg-cms-file-paths', is_archived: false})
WHERE NOT new.archive_reason CONTAINS 'rewrite_1_of_7'
SET new.is_archived = true, new.archive_reason = 'reverted_rewrite_1', new.archived_at = datetime();
```

Or bulk-revert every rewrite by scanning `archive_reason CONTAINS 'jiminy_corpus_audit_004_rewrite_'`.

## Final post-batch state (verified, 2026-08-14 after Option B + 7 rewrites)

- Live count: **31** (unchanged from Option B post-state; rewrites preserve count via tombstone+recreate)
- Corpus health: 20 actionable constraints + 8 informational constraints + 2 actionable corrections + 1 informational correction
- WRITE flag reverted (503 confirmed on probe create)
