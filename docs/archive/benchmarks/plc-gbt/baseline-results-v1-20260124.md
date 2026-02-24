# PLC-GBT Baseline Benchmark Results v1

**Date:** 2026-01-24
**Codebase:** plc-gbt
**Question Set:** test_questions_v2_selected.json (100 questions)
**Model:** Claude Haiku

---

## Summary

| Metric | Value |
|--------|-------|
| **Questions Attempted** | 26/100 |
| **Completion Rate** | 26% |
| **Self-Assessed Avg Score** | 0.942 |
| **Time Elapsed** | ~18 minutes |
| **Disqualified** | No |

---

## Key Findings

The baseline agent using traditional file search (Read, Glob, Grep) could only complete 26% of questions within the 20-minute time limit. This demonstrates the inherent limitation of context-window-only approaches for large codebases (815K LOC).

---

## Scores by Category (Attempted Only)

| Category | Avg Score | Count |
|----------|-----------|-------|
| api_services | 1.00 | 5 |
| configuration_infrastructure | 1.00 | 3 |
| control_loop_architecture | 0.95 | 6 |
| data_models_schema | 0.90 | 4 |
| database_persistence | 1.00 | 2 |
| ui_ux | 0.83 | 4 |
| ai_ml_integration | 1.00 | 2 |

---

## Individual Answers

### Q1 (api_006): CLI_VERSION

- Files: plc_control_loop_cli.py:58
- Answer: "1.0.0"
- Score: 1.0

### Q2 (cfg_002): OPENAI_SEED

- Files: openai_service.py:20, .env:43
- Answer: 1
- Score: 1.0

### Q3 (api_009): Temperature for truth-seeking

- Files: openai_service.py:147
- Answer: 0.2
- Score: 1.0

### Q4 (ai_003): Temperature for deterministic

- Files: openai_service.py:147
- Answer: 0.2
- Score: 1.0

### Q5 (ai_001): Default OpenAI model

- Files: openai_service.py:18
- Answer: ft:gpt-4o-mini-2024-07-18:whiskey-house:plc-gbt:CMx89NDT
- Score: 1.0

### Q6 (api_008): ExecutionStatus enum

- Files: cli_api_bridge.py:101-105
- Answer: SUCCESS, ERROR, TIMEOUT, UNAUTHORIZED
- Score: 1.0

### Q7 (cla_001): PID controller types

- Files: control-loop.types.ts:34
- Answer: Manual, Automatic, Cascade, Override, Program, Ratio
- Score: 0.5 (partial)

### Q8 (dms_001): ProcessVariable quality

- Files: control-loop.types.ts:49
- Answer: 'good', 'bad', 'uncertain'
- Score: 1.0

### Q9 (cfg_006): Default cache TTL

- Files: plc_control_loop_cli.py:96
- Answer: 300 seconds
- Score: 1.0

### Q10 (dms_008): Schema directories

- Files: schemas/control-loops/subtypes/
- Answer: cascade, combined_ff_cascade, feedforward, multi_formula_weighted_ff
- Score: 1.0

### Q11 (dms_004): ValidationLevel values

- Files: validation_manager.py:32-37
- Answer: BASIC, STANDARD, COMPREHENSIVE, PRODUCTION
- Score: 1.0

### Q12 (api_001): CommandRequest timeout

- Files: cli_api_bridge.py:131
- Answer: 30.0 seconds
- Score: 1.0

### Q13 (db_002): Chunk size

- Files: file_storage_service.py:42
- Answer: 8192 bytes
- Score: 1.0

### Q14 (api_005): Checksum algorithm

- Files: file_storage_service.py:75
- Answer: SHA-256
- Score: 1.0

### Q15 (dms_006): JSON Schema version

- Files: master-framework.json:2
- Answer: 2020-12 draft
- Score: 1.0

### Q16 (uix_004): React Query cache times

- Files: Providers.tsx:29-30
- Answer: staleTime=5min, gcTime=10min
- Score: 1.0

### Q17 (uix_001): Layout store name

- Files: layout-store.ts:179, 412
- Answer: useLayoutStore with key 'plc-gbt-layout-storage'
- Score: 1.0

### Q18 (uix_002): ToolType values

- Files: layout-store.ts:7-14
- Answer: explorer, search, analytics, workflows, control-loops, settings, plc-git
- Score: 1.0

### Q19 (uix_009): Layout presets

- Files: layout-store.ts:375
- Answer: minimal, development, debugging
- Score: 1.0

### Q20 (uix_007): Widget types

- Files: analytics.types.ts:246-257
- Answer: 11 types found (expected 4)
- Score: 0.5 (partial)

### Q21 (cla_004): Overshoot limit

- Files: cascade/ladder-logic-standard-pid-cascade.json:183-188
- Answer: default=10.0, range=0.0-50.0%
- Score: 1.0

### Q22 (cla_005): Response ratio

- Files: cascade/ladder-logic-standard-pid-cascade.json:246-251
- Answer: default=0.2, min=0.1, max=1.0
- Score: 1.0

### Q23 (cla_009): Stability margin

- Files: cascade/ladder-logic-standard-pid-cascade.json:253-259
- Answer: default=2.0, min=1.0, max=5.0
- Score: 1.0

### Q24 (cla_008): Response time target range

- Files: cascade/ladder-logic-standard-pid-cascade.json:177-181
- Answer: 1.0-3600.0 seconds
- Score: 1.0

### Q25 (cla_003): Communication methods

- Files: cascade/ladder-logic-standard-pid-cascade.json:122-127
- Answer: direct_write, message_instruction, produced_tag, ethernet_ip
- Score: 1.0

### Q26 (cla_006): Scaling types

- Files: cascade/ladder-logic-standard-pid-cascade.json:148-151
- Answer: linear, square_root, custom
- Score: 1.0

---

## Comparison Notes

This baseline result will be compared against MDEMG benchmark results.

Key comparison metrics:

- **Completion rate**: Baseline 26%, MDEMG TBD
- **Time efficiency**: Baseline ~0.7 questions/min
- **Accuracy on attempted**: Baseline 0.942

---

*Generated: 2026-01-24*
