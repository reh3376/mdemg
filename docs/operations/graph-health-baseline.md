# Graph Health Baseline Measurements

**Date:** 2026-04-04
**Space:** `mdemg-dev`
**Server version:** v0.6.0
**Branch:** `reh3376_dev01` (commit 1a0b149)

---

## Total Nodes by Label

| Label | Count |
|-------|-------|
| SymbolNode | 242,627 |
| Observation | 85,482 |
| MemoryNode | 63,003 |
| TapRoot | 1 |
| **Total** | **391,113** |

## SymbolNode Categories (Vendor vs Project)

| Category | Count | % of SymbolNodes |
|----------|-------|-----------------|
| neural/.venv | 162,044 | 66.8% |
| project | 71,327 | 29.4% |
| src-tauri/target | 6,332 | 2.6% |
| packaging | 2,924 | 1.2% |
| **Vendor total** | **171,300** | **70.6%** |
| **Project total** | **71,327** | **29.4%** |

## CO_ACTIVATED_WITH Edge Stats

| Metric | Value |
|--------|-------|
| Total edges | 622,632 |
| Avg weight | 0.1307 |
| Min weight | 0.0996 |
| Max weight | 1.0000 |
| Avg evidence_count | 1.6138 |

### Edge Categories (Vendor vs Project)

| Category | Edges | Avg Weight |
|----------|-------|-----------|
| project↔project | 403,857 | 0.1345 |
| vendor↔project | 113,955 | 0.1328 |
| vendor↔vendor | 104,524 | 0.1137 |

**CORRECTION:** Plan stated cross-vendor edges = 0. Actual: 218,479 vendor-touching
edges exist. These are noise (packaging doc stubs co-retrieved with project docs).
DETACH DELETE of vendor nodes removes all 218,479, leaving 403,857 project↔project.

## SymbolNode Duplication

| Metric | Value |
|--------|-------|
| Duplicate groups | 14,542 |
| Excess nodes | 45,248 |

### Top 10 Most-Duplicated

| Name | File | Type | Copies |
|------|------|------|--------|
| _mm512_mask_blend_ps | /neural/.venv/.../vec512_complex_float.h | method | 255 |
| B | /packaging/.../src-tauri/target/.../tests.rs | type | 203 |
| A | /packaging/.../src-tauri/target/.../tests.rs | type | 203 |
| json | /docs/development/API_REFERENCE.md | code_block | 167 |
| CanBeReused | /neural/.venv/.../LazyIr.h | method | 165 |
| ClassOpKind | /neural/.venv/.../LazyIr.h | method | 165 |
| ToString | /neural/.venv/.../LazyIr.h | method | 165 |
| json | /docs/user/api-reference.md | code_block | 147 |
| json | /packaging/homebrew-mdemg/docs/api-reference.md | code_block | 124 |
| json | /packaging/mdemg-windows/docs/api-reference.md | code_block | 124 |

## MemoryNode Embedding Coverage

| Metric | Value |
|--------|-------|
| Total MemoryNodes | 63,003 |
| With embedding | 62,900 |
| Missing embedding | 103 |
| Coverage | 99.84% |

## MemoryNodes by Layer

| Layer | Count |
|-------|-------|
| L0 (base) | 57,126 |
| L1 (hidden patterns) | 4,717 |
| L2 (concepts) | 1,080 |
| L3 | 49 |
| L4 | 10 |
| L5 (emergent) | 21 |

## Observation Count

| Metric | Value |
|--------|-------|
| Total observations | 85,482 |

## Health Score

| Metric | Value |
|--------|-------|
| Health score | 0.7408 |
| Embedding coverage | 0.9984 |
| Avg degree | 15.83 |
| Max degree | 1,133 |
| Orphan count | 2 |
| Last 24h memories | 4,530 |
| Last 7d memories | 8,715 |
| Last 30d memories | 62,968 |

## .mdemgignore Patterns (at time of baseline)

```
packaging/
neural/.venv/
__pycache__/
*.pyc
docs/architecture/benchmarks/.stash/
.mdemg/
bin/
dist/
man/
private/
.git/
node_modules/
vendor/
.env
.env.*
*.so *.dylib *.exe *.dll *.zip *.tar.gz *.whl *.min.js *.bundle.js *.map
.DS_Store Thumbs.db
.idea/ .vscode/ .cursor/ *.swp *.swo
j17-comprehension-test/
.claude/worktrees/
```

## Backup Locations

| Backup | Path | Size |
|--------|------|------|
| Neo4j | `~/.mdemg/backups/neo4j-pre-graph-health-20260404.tar.gz` | ~3.7 GB (copying) |
| TSDB | `~/.mdemg/backups/tsdb-pre-graph-health-20260404.dump` | 46 MB |

---

## Measurement Tracking Table

| Metric | Phase 0 (Baseline) | Phase 1 (Vendor) | Phase 2 (MERGE Fix) | Phase 3 (Dedup) | Phase 7 (Final) |
|--------|-------------------|-------------------|---------------------|-----------------|-----------------|
| Total nodes | 391,113 | 218,272 | 218,272 | ___ | ___ |
| SymbolNodes | 242,627 | 78,191 | 78,438 | ___ | ___ |
| MemoryNodes | 63,003 | 54,011 | 54,011 | ___ | ___ |
| Observations | 85,482 | 86,070 | 86,070 | ___ | ___ |
| CO_ACTIVATED edges | 622,632 | 404,153 | 404,153 | ___ | ___ |
| Avg edge weight | 0.1307 | 0.1344 | 0.1344 | ___ | ___ |
| Duplicate groups | 14,542 | ___ | 6,369 (12,665 excess) | ___ | ___ |
| Embedding coverage % | 99.84% | 99.81% | 99.81% | ___ | ___ |
| Health score | 0.7408 | 0.7446 | 0.7446 | ___ | ___ |
| Vendor nodes | 171,300 | 0 | 0 | ___ | ___ |
| Orphan SymbolNodes | — | — | 7,649 | ___ | ___ |
| Idempotent re-ingest | — | — | PASS (78,438=78,438) | ___ | ___ |
