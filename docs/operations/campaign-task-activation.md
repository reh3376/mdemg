# Campaign Task Activation Guide

## Overview

MDEMG has 16 LLM consumer tasks that produce training data. For the 30-day multi-instance collection campaign, each task needs 500+ records. At 30 days, that's ~17 records/day per task.

This guide categorizes tasks by activation method and provides configuration instructions for campaign participants.

## Recommended `.env` Additions

Add these to your `.env` file for maximum task coverage:

```bash
# Campaign activation flags
QUERY_CLASSIFY_ENABLED=true
# INTENT_ENABLED stays false — do NOT enable (see warning below)
JIMINY_ENABLED=true
JIMINY_EVALUATE_LLM_ENABLED=true
EMERGENCE_ENABLED=true
CONSULTING_LLM_CONSTRAINTS_ENABLED=true
METALEARN_ENABLED=true
```

> ⚠️ **Warning — do NOT set `INTENT_ENABLED=true` (INTENT-DISABLE-001, 2026-06-26):** intent translation was proven net-negative in the 120q UVTS A/B (mean −0.010) and was ~70% of all LLM errors (~15% chronic timeout rate, adding avg 3.8s to retrieval). Keep it `false`; re-enable only after a fresh `?intent=true` 120q A/B shows a real lift.

## Task Categories

### Bucket A: Config-Activatable (flip env var)

These tasks have complete code but are disabled by default.

| Task | Env Var | Default | Action |
|------|---------|---------|--------|
| `retrieval.intent_translate` | `INTENT_ENABLED` | `false` | **Keep `false`** — proven net-negative (INTENT-DISABLE-001) |
| `retrieval.query_classify` | `QUERY_CLASSIFY_ENABLED` | `false` | Set `true` |
| `hidden.name_emergence` | `EMERGENCE_ENABLED` | `false` | Set `true` |

**Verification:** After enabling, trigger retrieval queries and check:
```bash
TSDB_PORT=5433 ./bin/mdemg data status --space-id mdemg-dev
```

### Bucket B: Usage-Pattern-Dependent (need specific API calls)

These tasks require specific features to be used during development.

| Task | Required Usage | How to Generate |
|------|---------------|-----------------|
| `consulting.classify` | Consulting pipeline calls | Use `/v1/memory/consult` when making architecture decisions |
| `consulting.synthesis` | Consulting synthesis (requires `SYNTHESIS_ENABLED=true`) | Same as above, with synthesis enabled |
| `jiminy.synthesize` | Jiminy guidance events | Use Claude Code with MDEMG hooks enabled (`JIMINY_ENABLED=true`) |
| `jiminy.evaluate` | Jiminy guidance outcome tracking | Same — hooks send feedback automatically |
| `jiminy.evaluate_llm` | LLM-tier revalidation | Fires on contested outcomes (automatic when jiminy is active) |
| `jiminy.codegen` | Constraint codification | RSIC `codify_constraint` action (automatic) |
| `ape.reflect` | RSIC meso/macro reflection cycles | Keep MDEMG running — RSIC fires automatically on session triggers |
| `hidden.summarize` | Consolidation pipeline | Runs automatically as observations accumulate |
| `hidden.reclassify` | Reclassification cycles | Runs automatically during consolidation |
| `summarize.generate` | Summarization requests | Runs during consolidation |
| `metalearn.generalize` | Cross-space meta-learning | Requires `METALEARN_ENABLED=true` + multi-space operation |
| `retrieval.rerank_cross` | Cross-encoder reranking | Already active — fires on every retrieval query |
| `retrieval.rerank_nli` | NLI fallback reranking | Only fires when cross-encoder fails |

### Natural Data Accumulation

Some tasks accumulate data naturally just by using MDEMG:
- **High volume:** `retrieval.rerank_cross` (fires on every retrieval)
- **Medium volume:** `hidden.summarize`, `hidden.reclassify` (consolidation cycles)
- **Low volume:** `jiminy.*` tasks (requires active Claude Code sessions with hooks)
- **Rare:** `metalearn.generalize`, `jiminy.evaluate_llm` (specific conditions)

### Campaign Daily Targets

To reach 500 records per task in 30 days, each task needs ~17 records/day. For usage-dependent tasks, this means:

| Activity | Tasks Covered | Recommended Frequency |
|----------|--------------|----------------------|
| Retrieval queries | `rerank_cross`, `query_classify`, `intent_translate` | 20+ unique queries/day |
| Claude Code with hooks | `jiminy.*` (4 tasks) | 2+ coding sessions/day |
| Architecture decisions | `consulting.*` (2 tasks) | 3-5 consult calls/day |
| Keep MDEMG running | `ape.reflect`, `hidden.*` | Always-on background |

## Monitoring

Check accumulation rates daily:
```bash
# Quick status
TSDB_PORT=5433 ./bin/mdemg data status --space-id mdemg-dev

# Exit non-zero if under threshold
TSDB_PORT=5433 ./bin/mdemg data status --space-id mdemg-dev --warn

# Pre-campaign validation
./bin/mdemg data check --pre-campaign
```

## Documents Accessed

- `internal/config/config.go` — INTENT_ENABLED, QUERY_CLASSIFY_ENABLED, EMERGENCE_ENABLED defaults
- `internal/retrieval/query_classifier.go` — Query classifier WithContext wiring
- `internal/retrieval/intent_translator.go` — Intent translator WithContext wiring
- `docs/tests/ults/specs/` — 16 ULTS task specifications
- `docs/development/ft-lora/03_IMPLEMENTATION_PLAN_v2.md` — Consumer task labels table
