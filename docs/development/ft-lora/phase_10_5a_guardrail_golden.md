# Phase 10.5a — Guardrail Reward Fns + Golden Seed

**Status:** EXECUTED (2026-04-24)
**Predecessor:** FT-LORA-PHASE11 (PR #349 merged 2026-04-24)
**Scope:** sub-parts 1 + 2 of parent task #216. Sub-part 3 (SFT re-pass) deferred to Phase 5.5.

## Summary

Phase 10 shipped the benchmark framework with 16 of 17 ULTS tasks baselined (aggregate 0.8338). `guardrail.evaluate` (the 17th spec, added Sprint FT-LORA-B) was excluded because (a) the two declared reward functions were unimplemented, and (b) no golden holdout rows existed for it. This sprint lands (a) + (b). (c) — retraining the Phase 5 model on `guardrail.evaluate` — remains deferred to a future Phase 5.5 SFT re-pass and is not in scope here.

## Deliverables

### 1. Reward functions (`neural/training/reward_functions.py`)

Two new functions consumed by the ULTS spec's `reward_functions` array:

| Name | Semantics | Formula |
|---|---|---|
| `violation_detection_accuracy` | F1 over sets of `constraint_node_id` values under `violations`. Handles clean-vs-clean (1.0) and asymmetric-miss (0.0) edge cases explicitly. | `2·P·R / (P + R)` |
| `false_positive_penalty` | Penalty for fabricated violations. Returns 1.0 when no violations predicted; decays linearly as the share of predicted IDs outside the golden set grows. | `1.0 − (FP / |pred|)` |

Both fall back to `json_valid` when `expected` is missing, matching the `classification_accuracy` / `evaluation_accuracy` precedent. Both return 0.0 on invalid JSON. Registry count: 18 → 20.

Shared helper `_extract_violation_ids()` enforces the ULTS spec's nested shape (`violations: [{constraint_node_id, description, rationale}, ...]`) — malformed shapes yield an empty set, so downstream F1/penalty math treats them consistently as "no predictions."

### 2. Golden holdout append

Queried `llm_interactions` for `task_name = 'guardrail.evaluate'` post-2026-04-21. **Found 3 rows** (the minimum of the 3-5 seed-sample target), all timestamped 2026-04-22, all with valid JSON responses conforming to the spec schema.

| # | Row time (UTC) | Response constraints detected |
|---|---|---|
| 1 | 2026-04-22 04:32:51.184984+00 | `test-guardrail-constraint-001` (plaintext password) |
| 2 | 2026-04-22 04:33:17.657272+00 | `test-guardrail-constraint-001` (plaintext password) |
| 3 | 2026-04-22 04:35:57.929793+00 | `test-guardrail-constraint-001` (plaintext password) |

**Diversity caveat:** all 3 rows exercise the same test prompt (identical constraint and diff). Phase 5.5 SFT re-pass should carve additional rows from production traffic once `GUARDRAIL_ENABLED=true` has been running for at least one operating window. For GRPO-variance purposes this is acceptable — the rows serve as reward-signal anchors, not as SFT training data.

**Row count:** 105 → 108 golden rows across 17 (now) tasks.

**SHA delta:**
- Before: `8e44cdf9a085e71085ce615e1cdc09f1ea0a2d1eada53c857dac02d040d7fe77`
- After:  `b2004783607cd4934b2da6a3e7d95f066e82a936d677bcfb42d246864116fe3e`

Golden file remains on disk only (gitignored per `training_data/*` policy); SHA stamp lives here for reproducibility.

### 3. Unit tests

13 new tests in `neural/training/tests/test_reward_functions.py`:

- `TestViolationDetectionAccuracy` (9 tests): exact match, both-clean, missed violation, fabricated violation, partial-overlap F1, invalid response JSON, invalid expected JSON, no-expected fallback, malformed violations-array treatment.
- `TestFalsePositivePenalty` (6 tests): no FPs, no-predictions-no-penalty, all-FPs, half-FPs, invalid JSON, no-expected fallback.
- Registry: row-count bumped (18 → 20) + explicit membership assertions for the two new names.

Total suite: 72 passed (59 existing + 13 new), ≤0.05s.

## Out of Scope (Phase 5.5 follow-up)

- **Sub-part 3:** add `guardrail.evaluate` to a Phase 5 SFT training manifest. Model `qwen3-14b-mdemg-v1` has never been trained on this task; any baseline before the re-pass is cold-start base-model behavior — unsuitable as a GRPO advantage-normalization signal. Scheduling depends on operator availability for the SFT re-run (~4-8 hrs MLX).
- **Golden diversity:** seed pool capped at 3 identical-prompt rows until `GUARDRAIL_ENABLED=true` in production produces heterogeneous traffic. Re-carve when ≥ 20 rows with ≥ 5 distinct `system_prompt_hash` values exist.
- **Re-baseline of the 17th task:** blocked on sub-part 3.

## Artifacts

- `neural/training/reward_functions.py` — 2 new fns + `_extract_violation_ids()` helper (~100 LOC)
- `neural/training/tests/test_reward_functions.py` — 13 new tests
- `training_data/eval/valid_golden.jsonl` (on disk, SHA `b2004783…`) — 3 rows appended
- `training_data/eval/valid_golden.jsonl.sha256` — updated
- This doc

## Documents Accessed

- `/Users/reh3376/mdemg/docs/tests/ults/specs/guardrail_evaluate.ults.json` — reward names + output schema
- `/Users/reh3376/mdemg/neural/training/reward_functions.py` — registry + existing patterns (`classification_accuracy`, `evaluation_accuracy` fallbacks)
- `/Users/reh3376/mdemg/neural/training/tests/test_reward_functions.py` — test conventions
- `/Users/reh3376/mdemg/docs/development/ft-lora/phase_10_benchmark_post.md` — prior golden SHA + row-count pattern
- `llm_interactions` TSDB rows (3 × `guardrail.evaluate`, 2026-04-22)
