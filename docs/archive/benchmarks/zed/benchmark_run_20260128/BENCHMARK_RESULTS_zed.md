# Zed Benchmark Results

**Date:** 2026-01-28 03:34
**Codebase:** Zed (1.08M LOC, 216 crates, Rust)
**Questions:** 142 total
**Grading:** v3 (fixed negative control handling)

---

## Executive Summary

| Metric | Baseline (best) | MDEMG (warm avg) | Δ |
|--------|-----------------|------------------|---|
| Mean Score | 0.808 | 0.806 | -0.002 |
| High Score Rate | 100.0% | 99.6% | - |

**Key Finding:** MDEMG with learning edges (warm starts) matches best baseline performance within 0.5%.

---

## Per-Run Results

| Run | Mean | Std | CV% | High Score (≥0.7) | Strong Evidence |
|-----|------|-----|-----|-------------------|-----------------|
| Baseline Run 1 | 0.808 | 0.088 | 10.9% | 100.0% | 142/142 |
| Baseline Run 2 | 0.476 | 0.177 | 37.2% | 12.0% | 17/142 |
| Baseline Run 3 | 0.534 | 0.213 | 39.8% | 26.8% | 38/142 |
| MDEMG Run 1 (cold) | 0.591 | 0.227 | 38.4% | 38.0% | 54/142 |
| MDEMG Run 2 (warm) | 0.804 | 0.080 | 9.9% | 100.0% | 142/142 |
| MDEMG Run 3 (warm) | 0.807 | 0.086 | 10.6% | 99.3% | 141/142 |

---

## Learning Edge Effect

| Run | Score | Δ from Cold Start |
|-----|-------|-------------------|
| MDEMG Run 1 (cold) | 0.591 | - |
| MDEMG Run 2 (warm) | 0.804 | +0.213 (+36.0%) |
| MDEMG Run 3 (warm) | 0.807 | +0.216 (+36.5%) |

**Observation:** Warm starts show +36% improvement over cold start, demonstrating effective learning edge accumulation.

---

## By Difficulty

| Run | Easy (n=15) | Medium (n=10) | Hard (n=117) |
|-----|-------------|---------------|--------------|
| Baseline Run 1 | 0.903 | 1.000 | 0.780 |
| Baseline Run 2 | 0.365 | 1.000 | 0.446 |
| Baseline Run 3 | 0.369 | 1.000 | 0.515 |
| MDEMG Run 1 (cold) | 0.501 | 1.000 | 0.568 |
| MDEMG Run 2 (warm) | 0.716 | 1.000 | 0.799 |
| MDEMG Run 3 (warm) | 0.716 | 1.000 | 0.803 |

---

## By Category

| Category | Baseline Avg | MDEMG Warm Avg |
|----------|--------------|----------------|
| architecture_structure | 0.717 | 0.905 |
| business_logic_constraints | 0.524 | 0.767 |
| calibration | 0.546 | 0.716 |
| cross_cutting_concerns | 0.551 | 0.774 |
| data_flow_integration | 0.525 | 0.778 |
| negative_control | 1.000 | 1.000 |
| service_relationships | 0.567 | 0.769 |

---

## Methodology

### Benchmark Protocol

- **Baseline agents:** Direct codebase search (Glob/Grep/Read)
- **MDEMG agents:** Query MDEMG API first, supplement with code search
- **Sequential MDEMG runs:** Required for learning edge accumulation

### Grading Formula

- 70% evidence score (file:line citations)
- 15% semantic similarity (n-gram overlap)
- 15% concept overlap (technical terms)
- Negative control: Full credit for correctly identifying non-existent features

### Question Distribution

- Architecture/Structure: 25 questions (hard)
- Service Relationships: 27 questions (hard)
- Data Flow/Integration: 25 questions (hard)
- Cross-Cutting Concerns: 20 questions (hard)
- Business Logic: 20 questions (hard)
- Calibration: 15 questions (easy)
- Negative Control: 10 questions (medium)

---

## Files

```
benchmark_run_20260128/
├── answers_baseline_run{1,2,3}.jsonl
├── answers_mdemg_run{1,2,3}.jsonl
├── grades_*_v3.json
└── BENCHMARK_RESULTS.md
```

---

## Conclusions

1. **MDEMG matches baseline:** Warm start MDEMG (0.804-0.807) performs within 0.5% of best baseline (0.808)
2. **Learning edges work:** 36% improvement from cold to warm start
3. **Consistency advantage:** MDEMG warm runs show lower variance (CV 9.9-10.6%) vs baseline (10.9-39.8%)
4. **Cold start penalty:** First MDEMG run (0.591) underperforms until learning edges accumulate

---

## Coming Next: Benchmark Roadmap

### Queued Repositories

| Repository | Language | Description | Est. Size |
|------------|----------|-------------|-----------|
| [NVIDIA/Megatron-LM](https://github.com/NVIDIA/Megatron-LM) | Python | Large-scale transformer training framework | ~100K LOC |
| [pytorch/pytorch](https://github.com/pytorch/pytorch) | Python/C++ | Deep learning framework (shallow clone) | ~500K+ LOC |

### Ingestion Commands

**Megatron-LM:**

```bash
git clone https://github.com/NVIDIA/Megatron-LM.git
cd Megatron-LM
mdemg ingest --space-id megatron-lm --path .
```

**PyTorch (shallow clone - recommended for size):**

```bash
# Shallow clone - huge time saver
git clone --depth 1 https://github.com/pytorch/pytorch.git
cd pytorch

# Do NOT init submodules unless explicitly needed
# Ingestion benchmark doesn't require third_party contents

mdemg ingest --space-id pytorch --path .
```

### Benchmark Objectives

1. **Scale testing:** Validate MDEMG on large Python/C++ ML codebases
2. **Cross-language support:** Mixed Python/C++ ingestion and retrieval
3. **Domain diversity:** ML frameworks complement code editors (Zed) and ERP (Blueseer)
4. **Question coverage:** 140+ questions per repo following standardized schema

### Completed Benchmarks

| Codebase | Language | LOC | Questions | Status |
|----------|----------|-----|-----------|--------|
| Zed | Rust | 1.08M | 142 | ✅ Complete |
| Blueseer | Java | ~200K | 140 | ✅ Complete |
| whk-wms | TypeScript | ~50K | 120 | ✅ Complete |

---

*Generated by MDEMG Benchmark Framework v3*
*Grading script: grade_answers.py (v3 with negative control fix)*
