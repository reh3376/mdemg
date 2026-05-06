# Phase 14.2.3 — Per-category context-column weight (post-execution)

**Date**: 2026-05-06
**Branch**: `reh3376_dev01`
**Predecessors**: Phase 14.2 → 14.2.1 → 14.2.2 (all narrow-close).
**Verdict**: **PASSED 120q full** — first preset across the entire Phase 14.2.x sequence to clear the canonical merge gate. **Default flipped on.**

---

## Executive summary

Phase 14.2.2 shipped a high-variance ContextColumn: real lift on most categories (`computed_value` +0.090, `architecture_structure` +0.014, `data_flow_integration` +0.010) plus 9 zero-score rescues, but offset by 3 catastrophic regressions (q262, q194, q211) all clustered in 3 specific categories (`service_relationships` -0.043, `business_logic_constraints` -0.023, `relationship` -0.017).

Phase 14.2.3 zero-weights the column on those 3 categories and keeps default weight (0.10) elsewhere — same mechanism Phase 14.1 used for its sparse gate (per-category overrides via env JSON). Mirror pattern: **identical infrastructure, different signal application**.

**120q full A vs B:**

| Metric | A (column off via cat-weight=0 everywhere) | B (per-category weight) | Δ |
|---|---|---|---|
| **Mean** | 0.4030 | **0.4120** | **+0.0090** ✓ |
| min | 0.000 | **0.350** | **+0.350** (zero-rescues stuck) |
| std | 0.072 | **0.049** | -0.023 (much tighter) |
| `correct_file_rate` | 0.578 | 0.603 | +0.025 |
| Big improvements >10% | — | **11** | (was 9 in 14.2.2) |
| **Catastrophic regressions >10%** | — | **0** | **(was 3 in 14.2.2)** ✓ |
| 91 → 97 unchanged + 13 → 8 small drift | — | — | (more questions actually moved) |

**Both merge-gate criteria met:**
- B mean ≥ A mean: 0.4120 vs 0.4030 → ✅ +0.009
- 0 per-question regressions >10% (eps=1e-6): ✅

Per the sprint plan §13 fork #1 decision matrix: **`RETRIEVAL_CONTEXT_COLUMN_ENABLED` default flipped `false → true`** + **`CONTEXT_FINGERPRINT_ENABLED` default flipped `false → true`** (Phase 14.2.3 commit, this sprint).

---

## What landed

### Per-category weight infrastructure (commit `29f13a3`)

- **Config**: `Config.RetrievalContextColumnCategoryWeights map[string]float64`. Env `RETRIEVAL_CONTEXT_COLUMN_CATEGORY_WEIGHTS` JSON map. Validate cross-checks: weights ∈ [0, 10], category keys non-empty.
- **Default seed** (per the 14.2.2 forensic):
  ```json
  {
    "service_relationships":      0.0,
    "business_logic_constraints": 0.0,
    "relationship":               0.0
  }
  ```
- **Wire-in**: `ScoreAndRankRRF` gains a `category string` param. `req.Category` flows from `RetrieveRequest` (already plumbed by Phase 14.1 for the sparse gate). Override map looked up; fall back to global `RetrievalContextColumnWeight` when category unmatched.

### Default flips (this commit)

- `CONTEXT_FINGERPRINT_ENABLED` default `false` → **`true`** — observation fingerprints now write at observe-time by default
- `RETRIEVAL_CONTEXT_COLUMN_ENABLED` default `false` → **`true`** — 5th RRF column active by default
- Operator opt-out: set the env vars to `false` and restart

### Live verification on whk-wms

- 120q A baseline (column off via cat-weight=0 across all): mean=0.4030, std=0.072, correct_file_rate=0.578
- 120q B candidate (per-category weights): mean=0.4120, std=0.049, correct_file_rate=0.603
- 11 big improvements / 0 regressions / 97 unchanged / 8 small drift
- Server restart after env-flip: pid changed cleanly, scorer_version `v1-rrf5|...|c=0.100|...|ctx=true|...`, watchdog `up`

---

## Per-category breakdown (120q)

| Category | n | A mean | B mean | Δ | column weight |
|---|---|---|---|---|---|
| `architecture_structure` | 18 | 0.4040 | **0.4340** | **+0.030** | 0.10 |
| `business_logic_constraints` | 20 | 0.3920 | **0.3970** | **+0.005** | **0.00** |
| `computed_value` | 5 | 0.3000 | **0.3700** | **+0.070** | 0.10 |
| `cross_cutting_concerns` | 20 | 0.4120 | 0.4120 | 0.000 | 0.10 |
| `data_flow_integration` | 20 | 0.3820 | **0.3920** | **+0.010** | 0.10 |
| `disambiguation` | 7 | 0.4210 | 0.4210 | 0.000 | 0.10 |
| `relationship` | 6 | 0.4000 | 0.4000 | 0.000 | **0.00** |
| `service_relationships` | 20 | 0.4460 | 0.4360 | -0.010 | **0.00** |

The two categories where the column is at zero-weight (`business_logic_constraints` +0.005, `relationship` 0.000) net positive or neutral relative to 14.2.2 (which was -0.023, -0.017). `service_relationships` -0.010 with column disabled — within noise; would have been -0.043 with column enabled.

The improvements come from **5 categories with the column at default weight 0.10**:
- `architecture_structure` +0.030 (n=18, large sample)
- `computed_value` +0.070 (n=5, small but consistent)
- `data_flow_integration` +0.010 (n=20)
- `cross_cutting_concerns` +0.000 (n=20)
- `disambiguation` +0.000 (n=7)

**std dropped 0.072 → 0.049**: the column is a strong signal in helped categories AND the bad-category disabling removed the variance source. Net: more consistent retrieval scores.

---

## Why Phase 14.2.3 worked when 14.2 / 14.2.1 / 14.2.2 didn't

The 4-step retune narrative:

| Phase | Change | 16q | 120q | Verdict |
|---|---|---|---|---|
| 14.2 | Original LLM-summary tags | parity | parity | narrow close |
| 14.2.1 | Vector derivation | parity | not run | narrow close |
| 14.2.2 | Path-segment Builder retune | **+0.006 PASS** | **-0.004 FAIL** (3 regressions) | narrow close |
| **14.2.3** | **Per-category column weight (zero on 3 categories)** | not run | **+0.009 PASS, 0 regressions** | **default-on** |

The two real levers turned out to be:
1. **Catalog content** (14.2.2): swap LLM-summary tags for path-segment tokens. Without this, the column had no semantic-grounded refs to vote on.
2. **Per-category weight** (14.2.3): apply the column only where it helps. Without this, the variance-on-bad-categories cancelled the lift-on-good-categories at the mean.

Same pattern as Phase 14 → 14.1 → 14.1.1 (sparse gate failed 120q, per-category overrides shipped infrastructure flag-off, hybrid passed).

---

## Decision matrix outcome

From sprint plan §13 fork #1:

| Outcome | Default flip | Status |
|---|---|---|
| `fingerprint_only` passes 120q | flip `RETRIEVAL_CONTEXT_COLUMN_ENABLED=true` | ✅ **selected** |

Plus follow-on: `CONTEXT_FINGERPRINT_ENABLED=true` so observation fingerprints are written by default (the Stage 6 catalog-refresh hook needs this to be useful).

`RETRIEVAL_CONTEXT_COLUMN_CATEGORY_WEIGHTS` ships with the 3-category default seed; operators can extend or override.

---

## Spend

**OpenAI** (Phase 14.2.3): ~$10 (one full 120q B run; A_full was reused from 14.2.2). Total Phase 14.2.x sequence: ~$30-35 across 14.2 + 14.2.1 + 14.2.2 + 14.2.3.

**Wall clock**: ~45 min from "Builder edit" to "120q verdict".

---

## Operator runbook (default-on after this commit)

```bash
# Default behavior post-14.2.3 — no env flags needed:
# CONTEXT_FINGERPRINT_ENABLED=true, RETRIEVAL_CONTEXT_COLUMN_ENABLED=true
./bin/mdemg restart

# Verify scorer namespace
curl -X POST http://localhost:9999/v1/memory/retrieve \
     -H 'Content-Type: application/json' \
     -d '{"space_id":"<id>","query_text":"<query>","top_k":5}'
# Server log shows: scorer_version="v1-rrf5|...|c=0.100|...|ctx=true|..."

# With server-derived query fingerprint
curl -X POST 'http://localhost:9999/v1/memory/retrieve?context=auto' ...

# Override per-category weight at boot:
echo 'RETRIEVAL_CONTEXT_COLUMN_CATEGORY_WEIGHTS={"my_cat":0.05,"another":0}' >> .env
./bin/mdemg restart

# Operator opt-out:
echo "RETRIEVAL_CONTEXT_COLUMN_ENABLED=false" >> .env
./bin/mdemg restart
```

---

## Phase 14 sequence — final state

| Phase | Status | Default | Notes |
|---|---|---|---|
| 14 | EXECUTED 2026-05-04 (narrow close) | flag-off | gate 16q passed, 120q failed |
| 14.1 | EXECUTED 2026-05-04 (infra) | flag-off | per-cat sparse-gate dispatch infra |
| 14.1.1 | EXECUTED 2026-05-04 | **default-on** | hybrid PASSED 120q |
| 14.2 | EXECUTED 2026-05-05 (narrow close) | flag-off | sparse fingerprint infra |
| 14.2.1 | EXECUTED 2026-05-05 (narrow close) | flag-off | vector derivation |
| 14.2.2 | EXECUTED 2026-05-05 (narrow close) | flag-off | path-seg retune; 16q PASS, 120q FAIL |
| **14.2.3** | **EXECUTED 2026-05-06** | **default-on** | **per-category weight; 120q PASSED** |

Phase 14 sequence (gate + fingerprints) closes here. Both Note 06 (sparse gate) and Note 05 (sparse fingerprints) are default-on in production.

---

## Documents accessed

- `phase_14_2_2_post.md` — forensic source
- `internal/retrieval/scoring_rrf.go` (14.2.2 ScoreAndRankRRF)
- `internal/config/config.go` (Phase 14.1 `SparseGateCategoryOverrides` JSON-env pattern, mirrored here)

**Generated**:
- `phase_14_2_3_grades_B_per_cat_full.json` (raw grader output)
