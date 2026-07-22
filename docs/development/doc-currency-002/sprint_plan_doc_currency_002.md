# Sprint DOC-CURRENCY-002 — Repo-wide documentation currency fix (181 findings)

## 1. Header & Metadata
| Field | Value |
|---|---|
| Sprint ID | DOC-CURRENCY-002 |
| Owner | Roger Henley | Branch | `reh3376_dev01` |
| Format | v1.0 (12-section) | Effort | ~1 dev-day (agent-parallelized) |
| Parent | Operator directive 2026-07-21: "review all existing repo documentation to ensure it reflects current state" — 6-agent read-only audit found 181 findings (~85 WRONG, ~69 STALE, ~20 MISSING) across ~55 living docs |

## 2. Problem Statement
Six parallel audit agents verified operator-actionable claims (env vars, defaults, commands, ports, endpoints, counts, feature-state) across the living doc surface against the code. Findings cluster into six root-cause classes: (1) Prometheus-scrape ghosts (stack deleted 2026-03-28; ~10 docs); (2) recalibrated-defaults lag (~25); (3) LLM local-first lag (~10); (4) phantom/renamed env vars + pruned endpoints (~15 — silent no-ops and 404ing curls); (5) count drift (~20); (6) superseded-plan language (~10). The one CI-gated section (UXTS matrix §1) was 100% correct — proving gates work exactly as far as they read.

## 3. Scope & Constraints
**In**: apply every audit finding (tables pinned in `findings_*.md` in this dir); judgment rewrites for the 6 heavy items (CMS.md config block, prometheus-observability-monitoring.md, INGEST_CODEBASE_API.md, UXTS matrix §2/§3, CLAUDE.md 12 fixes, config.go:507 comment drift); prevention tooling (`scripts/verify_doc_env_vars.py` — extracts env tokens from living docs, fails on vars absent from config.go; soft-fail CI step).
**Out**: frozen/archive/sprint-record docs; AGENT_HANDOFF.md; full unified-cli.md sections (index table now, prose later — disclosed); NEW neural-sidecar operator doc (disclosed follow-up); CHANGELOG rewrites (append-only record).
**Constraints**: minimal diffs (fix the claim, don't reflow); every fix verified against code BEFORE writing (findings include evidence but code may have moved); fixer agents work DISJOINT file sets (no merge conflicts); frozen files untouched.

## 4. Dependencies
✅ Six audit reports with file:line + evidence + 1-line fixes (findings_*.md); ✅ config.go as ground truth for defaults; ✅ all 6 agents' "clean" lists (skip those files).

## 5. Implementation Plan
- **E0** plan + findings files (this commit).
- **E1** 6 parallel fixer agents (disjoint sets): root-docs / features a-j / features k-z / user+api+guides+ops / tests+sidecar / (CLAUDE.md + heavies = ME, not an agent).
- **E2** (me, parallel with E1) judgment rewrites: CMS.md config block (real vars from config.go), prometheus-observability-monitoring.md (TSDB-native rewrite), INGEST_CODEBASE_API.md (successor endpoints), UXTS matrix §2/§3, CLAUDE.md ×12, config.go:507 comment.
- **E3** prevention: `scripts/verify_doc_env_vars.py` + soft-fail CI step (kills the phantom-var class).
- **E4** verification sweep: re-grep the six class patterns across living docs (must return 0 unexplained hits); `go build ./...` + config tests (comment-only code change); JSON untouched.
- **E5** CHANGELOG + CLAUDE.md doc-governance note + post + push (single PR).

## 6. Testing Plan
T1: verify_doc_env_vars.py self-test on a synthetic phantom. T2: full go test (config comment change) + the script run over living docs = clean. T3: manual spot-check of 10 random applied fixes against code + the class-pattern grep sweep.

## 7. Commit Strategy
docs(E0 plan+findings) → per-area fixer commits (6) → docs(E2 heavies) → feat(E3 prevention) → docs(E5).

## 8. Verification Checklist
All findings applied or explicitly deferred-with-reason · class-pattern sweep clean · script green in CI mode · build/tests green · PR pushed.

## 9. Documentation Update
CHANGELOG Fixed (consolidated); CLAUDE.md new doc-governance note (the §1-gated-vs-ungated lesson + the env-var linter contract); post.md with per-class counts.

## 10. Risks & Mitigations
| Risk | Sev | Mitigation |
|---|---|---|
| Fixer agent misapplies a nuanced fix | Med | verify-before-write rule; minimal-diff rule; my E4 spot-check; findings include evidence line |
| Audit finding itself wrong (code moved again) | Low | fixers re-verify each claim against code before editing; skip+report if mismatch |
| Two agents touch one file | Low | disjoint area assignment; heavies reserved to me |
| Prevention script false-positives (example/placeholder vars) | Med | allowlist file + soft-fail CI |

## 11. Rollback
Pure docs + one comment + one soft-fail CI step: revert per-commit.

## 12. Documents Accessed
Six audit reports (findings_*.md, this dir); internal/config/config.go; internal/api/server.go; internal/cli/; neural/benchmarks/run_benchmark.py; UXTS_FRAMEWORK_MATRIX.md; deploy compose/grafana files.
