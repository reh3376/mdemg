# MDEMG Documentation Archive Index

**Created:** 2026-02-02
**Purpose:** Historical documentation preserved for reference

---

## Archive Structure

```
docs/archive/
├── ARCHIVE_INDEX.md           # This file
├── benchmarks/                # Historical benchmark runs
│   ├── whk-wms/              # Pre-v22 whk-wms runs
│   ├── megatron-lm/          # Megatron-LM benchmark
│   ├── zed/                  # Zed editor benchmark
│   ├── blueseer/             # Blueseer benchmark
│   ├── clawdbot/             # Clawdbot benchmark
│   ├── plc-gbt/              # PLC-GBT benchmark
│   ├── vscode-scale/         # VS Code scale tests
│   ├── llm-summary/          # LLM summarization tests
│   └── pytorch/              # PyTorch benchmark
├── uats-duplicates-20260203/  # Duplicate UATS spec/runner/schema files
│   ├── specs-v1.0.0/         # 45 specs (no SHA256 hashes, compact format)
│   ├── runners-v1.0.0/       # Runner v1.0.0 (no hash verification)
│   ├── schema/                # Identical schema copy
│   ├── uats-spec-v1/         # Distribution archive (tarball + example)
│   ├── Makefile.uats          # Identical Makefile copy
│   └── README-top-level.md    # Older README variant
└── investigations/           # Completed investigations
    ├── BENCHMARK_RUN3_INVESTIGATION.md
    └── CONFIDENCE_SCORE_INVESTIGATION.md
```

---

## Why These Files Were Archived

### Benchmarks

**Reason:** Superseded by canonical benchmark results in `docs/benchmarks/whk-wms/benchmark_run_20260130/`

Historical benchmark runs used different:

- Question sets and versions
- Grading formulas and weights
- Agent models and configurations
- Path handling (some had bugs)

**Current canonical results:**

- Location: `docs/benchmarks/whk-wms/benchmark_run_20260130/`
- MDEMG + Edge Attention: 0.898 mean score
- Baseline: 0.854 mean score
- See `docs/benchmarks/UP_TO_DATE_BENCHMARK_SUMMARY.md`

### Investigations

**BENCHMARK_RUN3_INVESTIGATION.md**

- Status: Resolved
- Issue: Benchmark variance between runs
- Resolution: Path handling bug fixed, edge-type attention implemented

**CONFIDENCE_SCORE_INVESTIGATION.md**

- Status: Resolved
- Issue: Low confidence scores in early runs
- Resolution: Cold start behavior documented, edge attention improved consistency

---

## Active Documentation (Not Archived)

### Benchmarks

- `docs/benchmarks/whk-wms/benchmark_run_20260130/` - Canonical results
- `docs/benchmarks/UP_TO_DATE_BENCHMARK_SUMMARY.md` - Current summary
- `docs/benchmarks/BENCHMARK_V4_README.md` - V4 framework guide
- `docs/benchmarks/framework/` - Benchmark utilities

### Parser

- `docs/lang-parser/PARSER_SPEC.md` - Consolidated parser specification
- `docs/lang-parser/lang-parse-spec/upts/` - UPTS test framework

### Investigations (Active)

- `docs/investigations/BENCHMARK_IMPROVEMENTS.md`
- `docs/investigations/LEARNING_EDGES_ANALYSIS.md`
- `docs/investigations/leakage-tests/`

---

## Restoring Archived Content

If you need to reference archived content:

```bash
# View archived benchmark results
cat docs/archive/benchmarks/whk-wms/archive/benchmark_v22_test/BENCHMARK_RESULTS_V22.md

# Compare with current
diff docs/archive/benchmarks/whk-wms/archive/benchmark_v22_test/grades_mdemg_run1.json \
     docs/benchmarks/whk-wms/benchmark_run_20260130/grades_mdemg_run1.json
```

---

### UATS Duplicates (2026-02-03)

**Reason:** Duplicate UATS files existed at `docs/api/api-spec/` top level alongside the canonical `docs/api/api-spec/uats/` directory.

- **Canonical (kept):** `docs/api/api-spec/uats/` — v1.1.0 runner, SHA256 hashes in specs, referenced by Makefile and CONTRIBUTING.md
- **Archived:** Top-level `specs/`, `runners/`, `schema/`, `README.md`, `Makefile.uats` — v1.0.0 runner, no SHA256 hashes, not referenced by any tooling
- **Archived:** `uats-spec-v1/` — distribution directory with tarball (v1.0.1)

---

## Archive Date

- **2026-02-02:** Benchmark and investigation archives
- **2026-02-03:** UATS duplicate consolidation
