# PHASE-E1-CORPUS-AUDIT-001 — audit report

**Source corpus**: `training_data/sft/claude_code_knowledge_v2/train.jsonl`
**Overlap threshold** for PROVEN_COVERAGE: `0.3` (asymmetric 3-gram overlap of answer→retrieved content).

## Summary

| Classification | Count | % |
|---|---|---|
| **PROVEN_COVERAGE** (safe to strip in E2) | 2203 | 81.4% |
| **SUBSTRATE_MISS** (keep in FT corpus) | 503 | 18.6% |
| **AUDIT_ERROR** (manual review before strip) | 0 | 0.0% |
| **Total** | 2706 | 100.0% |

## Overlap distribution

| Bucket | Count |
|---|---|
| [0.0, 0.1) | 453 |
| [0.1, 0.2) | 39 |
| [0.2, 0.3) | 11 |
| [0.3, 0.4) | 9 |
| [0.4, 0.5) | 2 |
| [0.5, 0.6) | 2 |
| [0.6, 0.7) | 7 |
| [0.7, 0.8) | 8 |
| [0.8, 0.9) | 12 |
| [0.9, 1.0) | 2163 |

## Top-20 SUBSTRATE_MISS rows (lowest overlap first)

| row_idx | overlap | n_results | top1_name |
|---|---|---|---|
| 39 | 0.000 | 5 | Monitor sessions with agent view |
| 49 | 0.000 | 5 | Environment variables |
| 52 | 0.000 | 5 | VS Code commands and shortcuts |
| 64 | 0.000 | 5 | CHANGELOG.md |
| 69 | 0.000 | 5 | CLINotFoundError: Claude Code not found |
| 82 | 0.000 | 5 | Monitor sessions with agent view |
| 83 | 0.000 | 5 | CHANGELOG.md |
| 89 | 0.000 | 5 | CHANGELOG.md |
| 105 | 0.000 | 5 | Environment variables |
| 113 | 0.000 | 5 | CHANGELOG.md |
| 125 | 0.000 | 5 | Monitor sessions with agent view |
| 132 | 0.000 | 5 | Environment variables |
| 144 | 0.000 | 5 | CHANGELOG.md |
| 164 | 0.000 | 5 | Claude Code settings |
| 171 | 0.000 | 5 | CHANGELOG.md |
| 197 | 0.000 | 5 | CHANGELOG.md |
| 247 | 0.000 | 5 | What you see while Claude Code retries or waits |
| 287 | 0.000 | 5 | CHANGELOG.md |
| 303 | 0.000 | 5 | CHANGELOG.md |
| 321 | 0.000 | 5 | CHANGELOG.md |

## Recommendation

**Proceed to E2**: 81.4% of the corpus has substrate coverage. Strip PROVEN_COVERAGE rows in E2, benchmark retrain in E3 to confirm no fact-recall regression.
