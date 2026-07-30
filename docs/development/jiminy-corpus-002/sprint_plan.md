# JIMINY-CORPUS-002 — Sprint Plan

**Date:** 2026-07-30 | **Branch:** `reh3376_dev01`
**Parent trigger:** JIMINY-CEILING-INVESTIGATION-001 defect **A**
(corpus contamination — 5 of top-12 surface volume goes to non-rules).
Q4 follow-up #2.

## 1. Header & Metadata

Second pass on the ConstraintPromotionGate with stricter admission
patterns targeting the narrative-shaped junk classes the JIMINY-
CORPUS-001 patterns missed (session-halt logs, workflow-violation
logs, testing-blind-spot analyses, phase-description narratives,
foundation-doc dumps). Plus retroactive tombstone of 5 confirmed
non-rules already in the substrate (backup + operator-authorized).
~1-1.5h effort. Follows the JIMINY-CORPUS-001 shape (E1 forward-fix
FIRST, E2 tombstone-only + backup + sign-off).

## 2. Problem Statement

The ceiling investigation categorized 16 high-similarity ignored
outcomes and found **44% (7/16) were surface mismatch** — the rule
being surfaced is itself a session log, phase description, or
analytical observation that shouldn't be a durable rule. Top-12
surface volume analysis:

| # | constraint_code | count | rule quality |
|---|---|---|---|
| 3 | `auto-015a122bcbb8` | 39 | session log — not a rule |
| 6 | `audit-before-prune` | 36 | session log with rule prefix |
| 7 | `auto-fcb814b48e33` | 34 | session log — not a rule |
| 8 | `full-system-gap-analysis` | 25 | phase description — not a rule |
| 9 | `auto-9f5134a1a0c3` | 22 | analytical observation |

The JIMINY-CORPUS-001 patterns catch build/test observations, bash
errors, PR-status notes, sprint-completion notes, doc-heading dumps,
sprint specs, and skill dumps. They miss:
- Session-halt narratives ("Session halt 2026-03-01. Commit 8683ac8: …")
- Workflow-violation narratives ("CRITICAL WORKFLOW VIOLATION (Phase 101): …")
- Testing/analysis narratives ("TESTING BLIND SPOT: …")
- Foundation-doc narratives ("COGNITIVE INTELLIGENCE GAP ANALYSIS — Foundation document for Phases 101-105")
- Phase-description narratives ("Phase 92: Full system gap analysis for deployable package")

## 3. Scope & Constraints

**Phase 1 (forward-fix E1 — SHIP FIRST):**
- Add 5 targeted regex patterns to
  `DefaultConstraintPromotionRejectPatterns` in `config.go`:
  1. `(?i)^Session\s+halt\b` — session-halt narratives
  2. `(?i)^CRITICAL\s+WORKFLOW\s+VIOLATION\b` — workflow-violation logs
  3. `(?i)^TESTING\s+BLIND\s+SPOT\b` — analytical observations
  4. `(?i)\bFoundation\s+document\s+for\s+Phase[s]?\b` — foundation-doc dumps
  5. `(?i)^Phase\s+\d+:\s+.*\b(?:analysis|gap analysis|synthesis)\b`
     — phase-narrative dumps
- Unit tests: each pattern matches the exact JIMINY-CEILING sample
  content; genuine constraints ("You must never …") remain unblocked
- No `.env` changes needed — patterns activate on server restart

**Phase 2 (retroactive tombstone E2 — OPERATOR-AUTHORIZED,
BACKUP-FIRST):**
- Backup `mdemg-dev` via `mdemg data export` (produces UTDS archive
  under `~/.mdemg/backups`)
- Tombstone 5 confirmed non-rules:
  - `auto-fcb814b48e33` (Session halt narrative)
  - `auto-015a122bcbb8` (CRITICAL WORKFLOW VIOLATION narrative)
  - `auto-9f5134a1a0c3` (TESTING BLIND SPOT narrative)
  - `full-system-gap-analysis` (Phase 92 phase-narrative)
  - `llm-multi-hop-synthesis` (COGNITIVE INTELLIGENCE GAP ANALYSIS
    foundation doc)
- Cypher: `MATCH (n:MemoryNode {space_id:'mdemg-dev', role_type:'constraint'})
  WHERE n.constraint_code = X SET n.is_archived = true,
  n.archive_reason = 'jiminy_corpus_002_purge', n.archived_at = datetime()`
- 5 nodes (one per constraint_code, verified live) — small batch,
  reversible via `is_archived=false` + remove archive_reason
- Post-tombstone verification: query the 5 codes are is_archived,
  count non-archived constraints (was 64, should be 59)

**Out of scope:**

- Retroactive tombstone of `audit-before-prune` (# 6, 36 surface
  count) — the ceiling investigation notes it as "session log with
  rule prefix" — the SUFFIX might be a real rule, so it needs
  operator review before tombstoning. Deferred.
- Empty-`constraint_code` investigation (data hole, # 4) — separate
  follow-up sprint
- Additional pattern discovery — this sprint's 5 patterns cover the
  top-12 diagnosed non-rules; more patterns can be added as new
  false-positives are found

## 4. Method

**Phase 1 (~10 min):**
- Edit `DefaultConstraintPromotionRejectPatterns` in config.go
- Add unit test file `constraint_gate_test_002.go` in `internal/hidden/`
  with test cases from the ceiling investigation
- `go test` verify green
- Restart server; verify no legitimate constraint promotion breaks
  in the log

**Phase 2 (~20 min, operator-authorized):**
- `mdemg data export --space-id mdemg-dev` (creates backup)
- Verify backup file exists + reports counts
- Execute the 5 tombstone Cypher statements
- Verify: 5 rows now `is_archived=true`, `constraint_code IN (…)`,
  59 live constraints remaining
- Document all tombstoned node_ids in `post.md` for reversibility

## 5. Testing Plan

- **Tier 1 (unit)**: 5 pattern-match tests + 3 negative tests (real
  constraint text NOT matching the new patterns)
- **Tier 2 (integration)**: none needed — Reject() is pure regex; the
  same pattern-check pipeline JIMINY-CORPUS-001 covered
- **Tier 3 (live)**:
  - Server restart picks up new patterns
  - Post-tombstone Cypher verification
  - Follow-up: over 24-48h, watch for the surface-volume shift on
    `constraint_outcomes` (the 5 tombstoned codes should stop
    appearing)

## 6. Commit Strategy

Single commit under `JIMINY-CORPUS-002`.

## 7. Verification Checklist

- [ ] 5 new patterns added to DefaultConstraintPromotionRejectPatterns
- [ ] Unit tests green (5 positive + 3 negative)
- [ ] Server restart clean; existing constraint promotions unchanged
- [ ] Backup file created + size sanity-checked
- [ ] 5 tombstone Cypher statements executed
- [ ] Post-tombstone verification (5 archived, 59 live)
- [ ] All 5 tombstoned node_ids recorded in post.md
- [ ] CHANGELOG + CLAUDE.md pin + post

## 8. Rollback

**Pattern rollback**: revert commit, restart server.

**Tombstone rollback**: `MATCH (n:MemoryNode {space_id:'mdemg-dev',
role_type:'constraint'}) WHERE n.archive_reason = 'jiminy_corpus_002_purge'
SET n.is_archived = false, n.archive_reason = null, n.archived_at = null`.
Fully reversible; nodes never left the graph.

## 9. Risks

- **Risk**: over-matching a legitimate constraint. The 5 patterns are
  narrow (anchored with `^` where possible, using specific phrase
  matches like `"Foundation document for Phase"`).
  - **Mitigation**: negative unit tests cover 3 real-constraint-text
    samples that would over-match if a pattern were too broad.
- **Risk**: tombstoning a node that WAS a real rule. The 5 codes are
  confirmed non-rules by the ceiling investigation's hand-classified
  categorization + content preview (verified again live before
  tombstoning).
  - **Mitigation**: tombstone (`is_archived=true`) is fully reversible;
    the pre-tombstone backup captures full node state.
- **Risk**: retrieval / graph pipelines expecting these constraint
  nodes to exist. Tombstoned nodes remain in the graph — the
  `is_archived` flag excludes them from Lever C surfacing and other
  live queries but not from forensic use.

## 10. Documents Accessed

Filled in `post.md`.
