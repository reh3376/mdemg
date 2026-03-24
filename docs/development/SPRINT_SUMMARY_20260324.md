# Sprint Summary — 2026-03-24

## Overview

Gap analysis remediation (Phase 1-2), T1 architecture map optimization, and UITS framework creation. Branch: `reh3376_dev01`.

---

## 1. Gap Analysis — Phase 1: Quick Wins

| GAP | Title | Action | Status |
|-----|-------|--------|--------|
| GAP-06 | Docker volume name mismatch | Added `tryMigrateVolume()` to `internal/cli/docker.go` — detects `mdemg-neo4j-data` and offers migration to `mdemg_neo4j_data` | Done |
| GAP-07 | Stale public-readiness checklist | Updated `docs/development/repo-to-public-roadmap.md` to reflect actual state | Done |
| GAP-12 | VS Code extension spec | Annotated `docs/specs/phase-vscode-extension.md` as community-contribution-only | Done |
| GAP-15 | Missing export governance doc | Removed dangling reference | Done |
| GAP-17 | No TLS/HTTPS support | Already implemented via `MDEMG_TLS_*` config vars — confirmed, no action needed | Stale |
| GAP-25 | Embedding model doc inconsistency | Fixed Windows README to reference correct model | Done |

## 2. Gap Analysis — Phase 2: UxTS Framework Hardening

| GAP | Title | Action | Status |
|-----|-------|--------|--------|
| GAP-28 | UATS canonical grammar migration | Migrated 163 legacy-dialect specs to canonical `op:` grammar via `scripts/migrate_uats_canonical.py` | Done |
| GAP-29 | UATS schema dead surface area | Documented unimplemented schema features in UATS README | Done |
| GAP-30 | Status-code-only UATS specs | Added body assertions to 23 specs that had zero assertions | Done |
| GAP-31 | UxTS frameworks not CI-gated | Added CI steps: USTS (merge-blocking), UOBS/UETS/UOTS (soft-fail), UNTS (merge-blocking) | Done |

### UATS Suite State

- **195 specs** / **224 variants** / **318 test cases**
- All specs migrated to canonical `op:` grammar
- All specs have SHA256 integrity hashes
- Zero specs with zero body assertions

## 3. T1 Architecture Map Optimization

Iterative LLM-judged optimization of 10 compact T1-encoded architecture maps for Jiminy agent context injection. 9 optimization rounds.

| Map | Type | Final Score | Status |
|-----|------|------------|--------|
| flow_retrieval | FLOW | 9.2 | Converged |
| flow_observe_learn_consolidate | FLOW | 9.0 | Converged |
| flow_jiminy_guide | FLOW | 9.2 | Converged |
| flow_rsic_cycle | FLOW | 9.0 | Converged |
| schema_neo4j | SCHEMA | 9.4 | Converged |
| dict_pkg_codes | DICT | 9.6 | Converged |
| dist_channels | DIST | 9.4 | Converged |
| svc_external | SVC | 9.0 | Converged |
| dep_pkg_graph | DEP | 8.6 | Accepted plateau |
| uxts_frameworks | UXTS | 8.8 | Accepted plateau |

**Suite mean: 9.2/10 | WEAK questions: 0 | 8 converged + 2 accepted plateaus**

### Key Optimization Techniques

1. **Structural grouping** — COMPANION-APPS subsection for dist_channels (+1.8 points)
2. **WHY-ANNOTATIONS footer** — explains non-obvious dependencies (dep_pkg_graph Q3: 3→8)
3. **STATUS-EXCEPTIONS footer** — highlights edge cases (uxts_frameworks Q4: 2→10)
4. **Column headers** — `#name|scope|specs|runner|CI` improved structural parsing
5. **List-final positioning** — exception items at end reduce mid-list confusion
6. **Parenthetical→key:value** — `(none—leaf)` → `leaf:no internal deps`
7. **Ground truth calibration** — terse GT caused false negatives; expanded to match correct answer patterns

## 4. UITS Framework (UxTS #12)

**Universal Iterative-Improvement Test Specification** — formalizes T1-encoded content comprehension validation as declarative specs with a reusable runner.

### Artifacts Created

| Artifact | Path | Lines |
|----------|------|-------|
| Schema | `docs/tests/uits/schema/uits.schema.json` | 410 |
| Runner | `docs/tests/uits/runners/uits_runner.py` | 1294 |
| Specs (11) | `docs/tests/uits/specs/*.uits.json` | ~110 each |
| README | `docs/tests/uits/README.md` | 67 |

### Scoring Profile: `arch_map_default` v1

| Dimension | Weight | Description |
|-----------|--------|-------------|
| comprehension | 0.40 | Mean LLM judge score (0-10) |
| compaction | 0.25 | Byte reduction vs human-readable baseline |
| token_efficiency | 0.20 | Token count vs budget |
| fidelity | 0.15 | Round-trip field recovery (placeholder) |

**Thresholds**: composite ≥ 0.80, comprehension ≥ 9.0, floor ≥ 7.0

### Runner Capabilities

- Subcommands: `validate`, `validate-all`, `optimize`, `add-hashes`, `verify-hashes`, `profiles`
- Follows UETS pattern: `uxts_runner_core.py` + `uxts_report.py` (Section 8A canonical report)
- LLM client via OpenAI API
- Convergence detection: 3 consecutive runs ≥ 9.0 mean, 0 WEAK questions
- SHA256 integrity hashes on all 11 specs

## 5. Documentation Updates

| File | Change |
|------|--------|
| `UXTS_FRAMEWORK_MATRIX.md` | Added UITS to inventory, source of truth, and parity tables |
| `UXTS_DEVELOPER_GUIDE.md` | Updated "11 frameworks" → "12", added UITS to naming and inventory tables |
| `UXTS_PORTABLE_AGENT_SPEC.md` | Updated "11 frameworks" → "12" |
| `AGENT_HANDOFF.md` | Updated "11 frameworks" → "12", added UITS to testing framework table |
| `FRAMEWORK_GOVERNANCE.md` | Added UITS to summary table and per-framework governance section |
| `CONTRIBUTING.md` | Updated "11 frameworks" → "12", added UITS and UETS to summary table |
| `CHANGELOG.md` | Added UITS framework and T1 map optimization to Unreleased section |
| `uxts_frameworks.md` (arch map) | Added UITS line with STATUS-EXCEPTIONS footer |

---

## Files Changed Summary

| Category | Files | Description |
|----------|-------|-------------|
| UATS specs | ~180 | Canonical grammar migration + body assertions + hashes |
| Architecture maps | 10 | T1-optimized maps in `docs/architecture/maps/` |
| UITS framework | ~15 | Schema + runner + 11 specs + README |
| CI workflow | 1 | `.github/workflows/ci.yml` — 5 new UxTS gates |
| Go source | 1 | `internal/cli/docker.go` — volume migration |
| Documentation | 10+ | Governance, guides, CHANGELOG, sprint summary |
| Scripts | 2 | `map_optimization_test.py`, `migrate_uats_canonical.py` |
| Benchmark data | ~100 | Optimization iteration results in `.stash/` |

---

## Next: Phase 3 — RSIC Integration

1. Reflection pattern #21 (`uits_encoding_regression`) — triggers when UITS comprehension scores drop below threshold
2. Action type #14 (`optimize_encoding`) — automated re-optimization of degraded maps
3. Assessment dimension 8 — UITS encoding quality as RSIC health metric
