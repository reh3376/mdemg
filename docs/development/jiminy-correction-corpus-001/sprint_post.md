# JIMINY-CORRECTION-CORPUS-001 — Sprint Plan + Post

**Date:** 2026-08-12
**Branch:** `reh3376_dev01`
**Arc:** JIMINY-CEILING-BREAK-2 (arc-adjacent, complements JIMINY-CORPUS-003)

## Problem

JIMINY-CORPUS-003 (2026-08-11) audited only `role_type='constraint'` nodes (64 → 33). The parallel `role_type='correction'` corpus (39 nodes) was **never audited**. Live TSDB analysis this session revealed the top 7d ignore-drivers are almost entirely LIVE corrections with junk / session-record / duplicate shape:

| Code | Role | 7d Events | Follow Rate |
|---|---|---:|---:|
| `never-manual-pr-author-field` | correction | 73 | 12.3% |
| `auto-b48d40e79848` | correction | 82 | 11.0% |
| `auto-7b6f52c5be6a` | correction | 68 | 13.2% |
| `must-master-bash-sed-edit` | correction | 62 | 12.9% |
| `file-path-access-tracker` | correction | 57 | 7.0% |
| `project-planning-docs-in-repo-only` | correction | 46 | 6.5% |
| `mdemg-cms-memory-only` | correction | 41 | 17.1% |
| ... | ... | ... | ... |

Every one is a live correction generating dozens of ignored outcomes / week. Most are duplicates of shipped constraints (`file-path-access-tracker` = dup of `end-with-docs-accessed` constraint; `must-master-bash-sed-edit` = dup of `plan-mode-before-change`; etc.); some are pure session-records dressed as rules (`auto-99cc04d4866f` "Jiminy gap: user had to explicitly request plan mode...").

## Shipped

**Audit of 39 live corrections** categorized against the same "would a competent developer follow this rule per-action?" test used in JIMINY-CORPUS-003:

- **35 TOMBSTONE**: 24 DUPLICATES of shipped constraints (`always-commit-before-goreleaser` = `no-stash-for-release`, `never-commit-to-main-directly` = `no-direct-main-commits`, `auto-b48d40e79848` = `auto-build-restart-after-feature`, etc.) + 11 JUNK (session-records, incident reports, test fixtures like `live-test-correction-coverage`)
- **1 MARK_INFORMATIONAL**: `mdemg-cms-memory-only` — meta about the CMS substrate itself, not a per-action rule
- **2 KEEP_REAL** (actionable corrections not covered by constraint corpus):
  - `project-planning-docs-in-repo-only` — real rule about docs location
  - `query-mdemg-cms-file-paths` — real rule "use CMS query for file paths, not glob"
- **1 KEEP_ACTIONABLE**: `no-hardcode-pool-sizes` was initially in tombstone list as a DUP of `never-hardcode-config`, but on re-audit is narrower + more actionable (specifically about connection pool sizes with prod OOM context). Kept.

**Live corpus after purge:**

| Role | Actionable | Informational | Total |
|---|---:|---:|---:|
| constraint | 26 | 7 | 33 |
| correction | 2 | 1 | 3 |
| **total** | **28** | **8** | **36** |

From 33 constraints + 39 corrections = 72 pre-sprint → **36 total post-sprint (50% cut).**

Tombstones use `is_archived=true` + `archive_reason='jiminy_correction_corpus_001_purge{,_dup}'` + `archived_at=datetime()`. Fully reversible.

**Applied in 2 batches** per JIMINY-CORPUS-001 tombstone-safety pattern:
- Small-batch: 5 obvious DUPs → verified 39 → 34
- Remaining: 30 → verified 34 → 3

## Live Tier-3 (mdemg-dev, 2026-08-12)

- `docker exec ... cypher-shell`: pre-purge 39, post-purge 3 live corrections ✓
- Informational mark landed: `mdemg-cms-memory-only` shows `is_informational=TRUE`, `informational_marked_at` set ✓
- Backup captured to `docs/development/jiminy-correction-corpus-001/pre_purge_backup.txt`

## Expected mechanical lift

**Much larger than JIMINY-INFORMATIONAL-CATEGORY-001's 0.6pp.** The tombstoned 35 corrections account for the DOMINANT share of the 1155 ignored outcomes in the 7d window. Once the 168h window rolls off pre-purge outcomes:
- Ignored outcomes from tombstoned codes: **~450-600** (~40-50% of current ignored)
- Actionable follow rate lifts mechanically **from 13.32% to ~22-28%** (window rollover, ~5-7 days)
- Combined with the LEVER-C-TIGHTEN-002 scope-gate + JIMINY-CLASSIFIER-CONTEXT-002 mechanism-scope gate → composite ceiling by 2026-08-19 could hit **~30-40%**

## Rollback

```cypher
MATCH (c:MemoryNode {space_id:'mdemg-dev', role_type:'correction'})
WHERE c.archive_reason IN ['jiminy_correction_corpus_001_purge','jiminy_correction_corpus_001_purge_dup']
SET c.is_archived=false, c.archive_reason=null, c.archived_at=null
```

Full backup at `pre_purge_backup.txt` (40 lines with header).

## One arch rule pinned (CLAUDE.md)

**Corpus-purge audits MUST cover role_type='correction' at parity with role_type='constraint'.** JIMINY-CORPUS-003 was constraint-only, and the ~50% ignored-outcome tail was driven by corrections. Any future corpus-purge sprint MUST enumerate BOTH role types up front. The auditor's "would a competent developer follow this?" test applies identically to both — the split into constraint vs correction is a categorization artifact from the L0 obs_type distinction, not a rule-quality distinction. Ceiling analyses that count only "constraints" will systematically underestimate the corpus junk load.

## Follow-ups disclosed

- **Auto-detected duplicates** — the fact that 24 of 39 corrections were DUPs of existing constraints suggests the correction promoter (`CreateCorrectionNodes`) doesn't dedup against the constraint corpus. Follow-up sprint: at correction-promotion time, check for high-similarity match against live constraints and either link (not promote) or tombstone-on-arrival. Prevents this pile from re-accumulating.
- **Historical outcomes on tombstoned codes** — the 7d window still holds outcomes generated when these codes were live. The mechanical lift arrives via window rollover (~7 days), not immediately.
- **Framing hygiene sweep** — deferred from JIMINY-CEILING-BREAK-2 §"Doc + framing hygiene cleanup". Still pending.

## Documents Accessed

- `docs/development/jiminy-corpus-003/tombstone_list.md` — audit shape precedent
- `docs/development/jiminy-ceiling-break-2/README.md` — arc position
- Live TSDB (`constraint_outcomes` 7d top-25 by code)
- Live Neo4j (`MemoryNode` role_type=correction where NOT is_archived)
- CLAUDE.md pins: JIMINY-CORPUS-001, JIMINY-CORPUS-002, JIMINY-CORPUS-003, JIMINY-ARCHIVED-CODE-FILTER-001, JIMINY-CEILING-BREAK-2, JIMINY-INFORMATIONAL-CATEGORY-001
