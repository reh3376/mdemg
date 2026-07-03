# JIMINY-CORPUS-001 Epic 2 — Purge Record (2026-07-03)

Operator-authorized, tombstone-only (reversible), executed with backup + small-batch-first + sign-off.

## Result
- **Live `role_type='constraint'` nodes: 140 → 61** (79 tombstoned).
- `archive_reason='jiminy_corpus_junk_purge'`: **49** (junk: build/test-status, bash-error, PR/sprint/phase completion notes, doc/SPEC/SKILL/template dumps).
- `archive_reason='jiminy_corpus_dedup'`: **30** (redundant copies of duplicated genuine rules; one canonical kept per rule — e.g. the 12-node `CONSTRAINT 1–5: Never modify production database schemas` set → kept `never-direct-alter-schema`, tombstoned 11).
- These 79 accounted for the majority of the last 7 days' constraint surfacings (junk alone ≈ 58%).

## Precision refinements during generation
- The junk-49 = 47 shipped-gate rejects + `t8j3…` (phase-completion false-negative, 169 surf) + `xinav…` (`forensic-readiness-gap-analysis`, 104 surf) — the last two caught by the E2 gate-widening.
- **`bj4w17ne…` (`memory-preservation-backup-integrity`) was SPARED** — the prior candidate list mislabeled it `skill_dump`, but the start-anchored `^skill:` regex never actually rejected it; its content is a genuine "CMS Memory Protection Constraint" rule. Excluded to avoid tombstoning a real rule.

## Gate widened (forward-fix, own code in this epic)
Two patterns added to `DefaultConstraintPromotionRejectPatterns()` so the phase/PR-completion-status class (`t8j3`/`xinav`) is rejected at promotion going forward, without over-matching genuine rules that merely mention a phase/PR (the "rebase after --admin merge" rule still passes). Unit-tested against live content.

## Reversibility
Tombstones are reversible:
```cypher
MATCH (n:MemoryNode {space_id:'mdemg-dev'})
WHERE n.archive_reason IN ['jiminy_corpus_junk_purge','jiminy_corpus_dedup']
SET n.is_archived=false REMOVE n.archive_reason, n.archived_at
```
Full node backups: `.mdemg-backup-jiminy-corpus-20260703_154420/` (`constraint_nodes_full_backup.csv` + `constraint_nodes_restore_keys.csv`, all 140). Tombstoned id+reason lists: `tombstoned_junk.csv`, `tombstoned_dedup.csv`.

## Open items (operator-disclosed, not acted on)
- `bj4w17ne…` disposition — spared as genuine; no action.
- The two paraphrased "never use `mdemg db start`" rule groups kept separate (< 0.85 similarity); paraphrase-merge is an optional operator call.
