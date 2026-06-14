# Space Hygiene — Neo4j Space Cleanup (clean slate for live testing)

**Date:** 2026-06-14 · operator-requested, in the DATAPRUNE line. Remove junk/
test/demo Neo4j spaces so live testing of new functionality runs on a clean
graph. Export-backup first (all to `.mdemg-backup-20260613_195431/spaces-backup/`).

## Deleted (24 spaces, ~143k nodes — exported then deleted)
Demo/dormant: **lnl-demo-whk** (51,483), **whk-wms** (49,470), **linear**
(10,032 — dormant one-time sync 2026-02-24), **ubts-load** (6,019).
Symbol-only test spaces: **e2e-test** (10,918), **whk-wms-test** (130),
**uats-snapshot-test** (10). Template-artifact names (invalid space ids):
**{{SPACE_ID}}** (101), **${test_space}** (4), **uats-batch-{{$timestamp}}**
(3), **${test_space}:templates** (1), **uats-ingest-{{$timestamp}}** (1).
Misc test: uats-templates-test:templates (247), ubts-benchmark (79),
hw-timeout-test (52), backup-rt-test (51), embed-test (11), negfeed-tier3 (6),
3× uats-ingest-178128…, uats-observe-test (2), uats-observe-pinned-test (2),
uats-skills-register-test (2), uats-correct-test (1).

## Retained (essential)
mdemg-dev (protected), demo, mdemg, mdemg-codebase, **global** (2 — the MDEMG
cross-space `IncludeGlobalSpace` concept, a valid name).

## Blank / null space_id — resolved per "infra stays null, name real data, delete junk"
Investigation found the blank "space" is NOT a space — it is two things:
- **Global infrastructure (correctly null, retained):** `SchemaMeta`/
  `SchemaMigration`/`Migration` (28, schema versioning, queried by `{key}`),
  `SignalState` (100, signal learner, queried globally), `InterviewPrompt`
  (395, gap interviews, by `prompt_id`), `ScrapedContent`/`ScrapeJob` (27,
  scraper, by `content_id`/`job_id`), `CapabilityGap` (18). All queried WITHOUT
  space_id by design — assigning one would pollute space-scoped queries. There
  is no genuine orphaned *space data* to name.
- **Junk (delete):** 155 `MemoryNode` with no space_id and empty content —
  J17/J12 test artifacts (`node_id` like `j17-trust-test…`). Backed up to
  `spaces-backup/blank_empty_memorynodes_backup.txt`; deletion Cypher staged
  (`/tmp/delete_blank_junk.cypher`, operator-run — guard-blocked DETACH DELETE).

## Two tool bugs fixed (commit alongside)
1. **`mdemg space delete`** gated its pre-check on `count(MemoryNode)` but
   deletes all labels → SymbolNode-only spaces (e2e-test 10,918) reported "no
   nodes" and silently survived. Pre-check now counts all labels.
2. **`mdemg space list`** panicked (`sid.(string)` on a nil space_id) whenever a
   MemoryNode had null space_id. Query now excludes null/empty space_id; the
   assertion is nil-guarded.

## Follow-ups (disclosed, not done)
- The template-artifact spaces ({{SPACE_ID}}, ${test_space},
  uats-batch-{{$timestamp}}) came from UATS/UBTS test runs with **unsubstituted
  template variables** — the test harnesses should substitute or reject
  placeholder space_ids (else they regenerate). Worth a guard in the runners.
- The global-feature writers (signal learner, gap interviews, scraper) create
  null-space_id nodes by design; per the operator's "infra stays null" choice
  they are left as-is (now correctly excluded from `space list`).
