# JIMINY-CORPUS-003 — Sprint Plan

## 1. Header & Metadata

- **Sprint:** JIMINY-CORPUS-003
- **Date:** 2026-08-11
- **Branch:** `reh3376_dev01`
- **Provenance:** operator challenge 2026-08-11 ("If the main LLM is only following 10-13% of Jiminy's guidance this entire project is a complete failure"); recanted framing from JIMINY-HEURISTIC-DEFAULT-001 verdict which normalized ~12% as "honest steady state." The 2026-08-01 directive (`trust-signal-must-be-persisted-never-ignore-honest`) already established that low compliance is a substrate-quality signal, not by-design.
- **Precedents:** JIMINY-CORPUS-001 (79 tombstoned), JIMINY-CORPUS-002 (5 tombstoned)

## 2. Problem Statement

Current `mdemg-dev` corpus: 64 live constraint nodes. Audit shows ~half are not durable rules: some are event logs (`auto-aa4903b877dd` "FT-OAI-001 Epic 5 gate: ... cost_estimate_usd=$1,429"), some are narrative session records (`prune-guardrail-compliance-check` "DATAPRUNE-AUDIT-001 Category A prune EXECUTED: deleted 1898 rows"), some are stale (`must-pin-mlx-8101` "Local Qwen3" — Phase 13.5 migrated to llama-server), and many are duplicates of the same underlying rule (6 variants of "use CUIDv2", 5 variants of "never commit to main", 4 variants of "e2e testing before commit", etc). Every irrelevant surface trains agents to ignore constraints. Compounded across 60+ nodes, this is the corpus-quality driver of the ~12% actionable follow rate.

Two-stage fix:
- **Stage 1 (corpus purge):** tombstone junk + stale + duplicate-non-canonical rules. Target: 64 → ~30 canonical constraints.
- **Stage 2 (strict-mode default):** flip `JIMINY_STRICT_DEFAULT_ENABLED` to `true` in `.env` (already true; verify) + `JIMINY_MODE=strict` default. Confirms Jiminy operates as ENFORCER not advisor per the operator's 2026-08-01 architectural directive.

## 3. Scope & Constraints

**In-scope:**
- Audit all 64 live constraint nodes; produce KEEP / TOMBSTONE_JUNK / TOMBSTONE_DUPLICATE / TOMBSTONE_STALE categorization
- Tombstone-only (is_archived=true + archive_reason='jiminy_corpus_003_purge') — reversible
- Verify strict mode is default-enabled + operates as intended
- Sprint docs, CLAUDE.md pin, CHANGELOG

**Out-of-scope:**
- Modifying node content or renaming codes (tombstone-only)
- Historical constraint_outcomes cleanup (rows honestly reflect past classifier state)
- Corpus AUDIT for role_type='correction' (35 correction nodes; separate sprint if needed)
- Retrieval or classifier changes
- Model swap (evaluated separately in queued MODEL-SWAP-MUSE-GLIMMER-EVAL-001)

## 4. Dependencies

- JIMINY-CORPUS-001 + JIMINY-CORPUS-002 (established tombstone pattern + backups)
- JIMINY-ARCHIVED-CODE-FILTER-001 (archived nodes correctly filtered from all read paths)
- JIMINY-ENFORCE-001 (strict mode mechanism)
- JIMINY-MODE-001 (mode enum + UI toggle)
- JIMINY-CLASSIFY-ESCALATION-INSPECT-001 (operator override + code visibility)

## 5. Implementation Plan (sequential)

**E1 — Backup + audit:** Full JSONL export of current 64 constraint nodes to `docs/development/jiminy-corpus-003/pre_purge_backup.jsonl`. Categorize each into KEEP / TOMBSTONE_{JUNK,DUPLICATE,STALE}. Present the tombstone list in `tombstone_list.md`.

**E2 — Small-batch tombstone (junk first):** Tombstone the JUNK category (~15 nodes). Test: retrieve + jiminy classify against a benign action — confirm no surfacing regression.

**E3 — Stale tombstone:** Tombstone the STALE category (2 nodes).

**E4 — Duplicate tombstone:** Tombstone the DUPLICATE_NON_CANONICAL category (~15 nodes). Keep canonical instances only.

**E5 — Verify strict-mode default:** Confirm `JIMINY_STRICT_DEFAULT_ENABLED=true` and `JIMINY_MODE=strict` in `.env`. If already true (per JIMINY-MODE-001 shipping state), verify default session `claude-core` gets strict at boot.

**E6 — Live smoke:** Post-purge retrieve + classify against the same action, verify the guidance surface is smaller + more relevant. Query `constraint_outcomes` to confirm live constraint_code IDs shrink to the canonical set.

**E7 — Docs:** sprint_plan + post.md + CLAUDE.md pin + CHANGELOG.

## 6. Testing Plan

**Live (T3) only:**
- E1 backup file exists + is complete
- E2/E3/E4 each: cypher COUNT before + after confirms delta = expected tombstone count
- E6 smoke: `/v1/memory/retrieve` for a common query returns fewer + more-relevant constraints
- E6 smoke: `/v1/jiminy/latest` warm surfacing shows fewer duplicate rules
- Passive re-check disclosed: gauge trend over 7d — expected LIFT (fewer irrelevant surfaces → higher follow-rate)

**Unit (T1):** N/A — no code changes in Stage 1

## 7. Commit Strategy

Single commit: `feat(jiminy): corpus purge — 64 → ~30 canonical constraints (JIMINY-CORPUS-003)`. Docs bundle in the same commit.

## 8. Verification Checklist

- [x] Backup JSONL captured
- [ ] KEEP list ratified
- [ ] Tombstone list ratified
- [ ] Junk tombstoned; smoke confirms no regression
- [ ] Stale tombstoned
- [ ] Duplicates tombstoned (canonicals preserved)
- [ ] Live constraint count matches target (~30)
- [ ] Strict-mode default verified (`JIMINY_MODE=strict` in .env; boot log confirms)
- [ ] Retrieve+classify smoke on a common query shows tighter surface
- [ ] CHANGELOG entry
- [ ] CLAUDE.md pin
- [ ] Post.md with tombstoned node_ids + rollback cypher

## 9. Risks & Mitigations

**R1: Tombstoning a rule the operator later wants back.** Reversible via `is_archived=false` + remove archive_reason. Full backup in `pre_purge_backup.jsonl` + tombstoned_ids in post.md.

**R2: Downstream reader breaks on missing constraint.** JIMINY-ARCHIVED-CODE-FILTER-001 already ensured all read paths filter `is_archived`. No downstream break.

**R3: Follow-rate signal doesn't lift after 7d.** Then the classifier semantics or retrieval matching are the true binding constraint, not corpus quality — informs next sprint's target (JIMINY-CLASSIFIER-CONTEXT-002 or Lever C tightening further).

**R4: Purging a rule that the operator has escalated multiple times, breaking the escalation state.** Escalation is per-(session, node_id) and gracefully skips archived nodes. No break.

## 10. Rollback Procedures

```cypher
MATCH (c:MemoryNode {space_id:'mdemg-dev', role_type:'constraint'})
WHERE c.archive_reason = 'jiminy_corpus_003_purge'
SET c.is_archived = false, c.archive_reason = null, c.archived_at = null
```

Rollback restores every tombstoned node atomically. Backup file `pre_purge_backup.jsonl` provides byte-level content restore if needed.

## 11. Documents Accessed

- Live Cypher query against `mdemg-dev` role_type='constraint'
- Operator directive 2026-08-01 (`trust-signal-must-be-persisted-never-ignore-honest` in CMS)
- Operator challenge 2026-08-11 ("this entire project is a complete failure")
- `docs/development/jiminy-corpus-001/`, `jiminy-corpus-002/` (precedent tombstone shape)
- `docs/development/jiminy-heuristic-default-001/post.md` (recanted framing)
- CLAUDE.md pins: JIMINY-CORPUS-001, JIMINY-CORPUS-002, JIMINY-ARCHIVED-CODE-FILTER-001, JIMINY-ENFORCE-001, JIMINY-MODE-001, JIMINY-CLASSIFY-ESCALATION-INSPECT-001
