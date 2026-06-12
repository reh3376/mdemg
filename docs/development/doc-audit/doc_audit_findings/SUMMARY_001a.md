# DOC-AUDIT-001a — Summary (complete, audited @ 831eb0a)

**41/41 in-scope files audited** (8 T0 + 23 operator-facing T3 + 10 T1
spot-checks). Verdict histogram:

| Verdict | Count |
|---|---|
| CURRENT | 31 |
| HISTORICAL_OK (dated records, correctly framed) | 5 |
| DRIFT_MINOR | 4 |
| DRIFT_MAJOR | 1 |

**Headline results**
- The canon is healthy: CLAUDE.md fully verified CURRENT (one lane
  verdict reversed by orchestrator re-verification — the discipline
  caught a misattribution). T1's OLDEST cohort (untouched since
  2026-04-04) spot-checked at **0% drift** — the per-feature-doc rule
  demonstrably works.
- The one DRIFT_MAJOR: `00_README_v2.md`'s version ledger frozen at
  v5.12/2026-04-30 — Phase 13.5, MODEL-DIST-001/002, FT-RECURSIVE-000,
  FT-CLASSIFY-002 missing from the FT line's canonical-history doc
  (STATUS block is maintained).
- **Triple-confirmed CODE finding** (3 independent lanes):
  compose-template + docker-compose.yml `LLM_ENDPOINT` defaults to dead
  port 8101 with a stale Phase-11.6 comment — Docker deployments without
  an explicit override point at nothing. This is the stale-8101 class
  FT-CLASSIFY-002 fixed in the benchmark config.

**Proposed fix batch (operator review; per charter, executed as a
follow-up batch — CHANGELOG/CLAUDE.md never batch-edited; the compose
fix is a CODE change belonging to a dev01 sprint commit with its CI
parity pair):**
1. compose template + docker-compose.yml: LLM_ENDPOINT default → 8102,
   comment updated (CODE, + UBENCH-style parity: both copies, same
   commit).
2. 00_README_v2.md: append v5.13+ version-ledger entries covering
   13.5 → FT-CLASSIFY-002 (append-only, R-LT-4-clean).
3. README.md: tab count 8→10; caption port examples as illustrative.
4. pre-campaign-checklist.md: schema v8+ → v26 (cite config.go).
5. mdemg_beta_testing.md: version-under-test marker refresh.
6. live-validation-findings.md: reclassify F11 presentation (P3 doc-gap).

**001b/001c**: 786 rows remain PENDING by design — architecture/specs
subtrees await DORMANT-CENSUS-001's oracle (001b); T4-T6 framing sweep
folds into HYGIENE-SWEEP (001c). Calibration from this phase: T1 needs
no per-claim pass in 001b (0% drift); concentrate entirely on T2 +
stale T3 subtrees.

CHANGELOG entry for this sprint rides the next dev01 commit (charter:
this PR stays purely additive).
