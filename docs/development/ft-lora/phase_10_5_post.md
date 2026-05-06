# Phase 10.5 — UBENCH Promotion + Guardrail.evaluate Closeout

**Status:** EXECUTED (2026-05-06)
**Predecessor:** Phase 10.5a (`phase_10_5a_guardrail_golden.md`, 2026-04-24)
**Trackers closed:** #215 (UBENCH framework), #216 (guardrail.evaluate 3-way gap, sub-parts 1+2)
**Tracker remaining:** #216 sub-part 3 — Phase 5 SFT re-pass (operator-deferred)

---

## What this sprint shipped

Two follow-ups queued at Phase 10 close land here:

| # | Item | Status |
|---|---|---|
| 215 | Promote `neural.benchmarks.run_benchmark` to a UxTS-pattern framework (UBENCH) with schema + spec + canonical report runner + dataset↔holdout contract + make targets + feature doc | ✅ shipped |
| 216 | Close `guardrail.evaluate` 3-way gap: (1) reward fns, (2) golden rows, (3) Phase 5 SFT re-pass | ✅ sub-parts 1+2 (Phase 10.5a); sub-part 3 deferred to operator |

UBENCH is the architectural fix for the *class* of bug that produced #216 — a spec exists,
holdout rows do not, and the runner silently skips the unmatched task. With UBENCH's
contract mode wired into `make test-ubench-contract` (and the pytest wrapper at
`docs/tests/ubench/contracts/test_dataset_holdout_contract.py`), the same shape of gap will
fail loudly in CI.

## #215 — UBENCH framework

### Files shipped

```
docs/tests/ubench/
  schema/ubench.schema.json                            # JSON-schema for .ubench.json specs
  specs/mdemg.ubench.json                              # Canonical mdemg spec (108 rows, 17 tasks, min_rows=3)
  runners/ubench_runner.py                             # Lint / contract / run modes
  contracts/test_dataset_holdout_contract.py           # pytest entry
docs/features/ubench-framework.md                       # Why / Choices / How / How to use
Makefile                                                # +test-ubench{,-lint,-contract,-run}
CLAUDE.md                                               # Testing section reference
```

### Acceptance run (live)

```
$ make test-ubench-lint        # 25ms, 1 spec, 1 pass, hash verified
$ make test-ubench-contract    # 24ms, 2 results, 0 fail
$ pytest docs/tests/ubench/contracts/ -v
  test_at_least_one_ubench_spec_exists PASSED
  test_each_spec_passes_contract PASSED
```

The contract mode validates:

1. ULTS spec count = 17 (matches `expected_specs`)
2. Golden holdout row count = 108 (matches `expected_rows`)
3. Distinct `meta.task_name` count = 17 (matches `expected_tasks`)
4. Every ULTS task has ≥ 3 golden rows (`min_rows_per_task=3`)

Any future spec landing without seeded rows will fail step 4 with the exact diagnostic that
would have caught the guardrail.evaluate gap before Phase 10's baseline run:
`contract: ULTS tasks with zero golden rows: [...]`.

### Choices made (per `feedback_plan_options_pattern.md`)

| Decision | Pick | Rationale |
|---|---|---|
| Wrap or rewrite the Phase 10 runner | **Wrap as subprocess** | 624 LOC + 114 unit tests + 1 captured baseline already shipped — rewriting risks regressions for zero gain |
| Default mode | `lint` | Cheapest CI hook; contract is one explicit step away |
| Schema validator | optional `jsonschema` with required-key fallback | Mirrors `verify_uxts_canonical_specs.py` precedent |
| Min-rows-per-task | 3 | Matches the seed-sample target from Phase 10.5a |

## #216 — guardrail.evaluate 3-way gap

### Sub-parts 1 + 2 (closed in Phase 10.5a, 2026-04-24)

Already documented in [`phase_10_5a_guardrail_golden.md`](phase_10_5a_guardrail_golden.md):

- **Sub-part 1:** `violation_detection_accuracy` + `false_positive_penalty` reward functions
  added to `neural/training/reward_functions.py` with 13 unit tests in
  `neural/training/tests/test_reward_functions.py`. Registry count 18 → 20. Verified present
  on 2026-05-06 at `reward_functions.py:511,561`.
- **Sub-part 2:** 3 golden rows appended to `training_data/eval/valid_golden.jsonl` from
  TSDB rows post-2026-04-21 (`task_name='guardrail.evaluate'`). Holdout SHA bumped to
  `b2004783…`. Verified present on 2026-05-06 at file lines 106–108 (3 rows, all
  `meta.task_name='guardrail.evaluate'`).

### Sub-part 3 — deferred to operator

**Operator-deferred, not framework-deferrable.** Sub-part 3 is "add `guardrail.evaluate`
to a Phase 5 SFT training manifest and re-run the SFT pass so `qwen3-14b-mdemg-v1` (and
the production GGUF derived from it) carries trained behavior on this task." This requires:

- ~4–8 hours of MLX training time (single dense-Qwen3-14B-4bit pass)
- An operator-scheduled maintenance window (the local LLM endpoint at port 8102 cannot
  serve while a training pass is running)
- A re-baseline of the 17th task afterward via `make test-ubench-run`

Because this is a compute window the operator picks (not work an automated agent can
schedule), this sub-part stays open in the tracker until the operator runs it.

**Mitigation while sub-part 3 stays open.** UBENCH's contract mode now flags the
`guardrail.evaluate` task as `n=3` for the holdout — it has the minimum row count to be
*scored*. The benchmark runs over the cold-start (untrained) base behavior on this task.
Phase 11 GRPO advantage normalization within the J group will skew slightly because three
of the four J tasks are SFT-trained and one is not, but the bias is stable across
candidate models (any post-GRPO model will see the same skew on the same task), so the
relative-comparison signal is preserved. The absolute score on `guardrail.evaluate` is
the only number to discount until sub-part 3 lands.

## Sequence of work (for reproducibility)

1. Confirm Phase 10.5a sub-parts 1+2 already on disk (reward fns at line 511,561; rows at 106-108).
2. Survey existing UxTS frameworks (`docs/tests/uvts/`, `docs/tests/ults/`) for layout pattern.
3. Author `ubench.schema.json` (top-level spec contract).
4. Author `mdemg.ubench.json` with current SHAs:
   - `configs/benchmark_phase10.yaml` → `76c97eb8…`
   - `training_data/eval/valid_golden.jsonl` → `b2004783…`
5. Author `ubench_runner.py` with lint / contract / run modes; reuse
   `docs/tests/uxts_report.py` for the canonical report envelope.
6. Smoke-test runner against the live spec; verify lint passes and contract reports
   `17 specs / 108 rows / 17 tasks`.
7. Add pytest wrapper at `docs/tests/ubench/contracts/`.
8. Wire `make test-ubench{,-lint,-contract,-run}`.
9. Author `docs/features/ubench-framework.md`.
10. Update CLAUDE.md Testing section to point at UBENCH.

## What remains open (by intent)

- Sub-part 3 of #216 — operator-only Phase 5 SFT re-pass (see above).
- UBENCH `--mode run` smoke against live llama-server. Not blocking (the wrapped runner is
  proven via Phase 10's captured baseline); will fire next time the operator runs a full
  benchmark cycle.
- Registry-vs-heuristic parity gate is schema-supported (`registry_parity` block) but not
  yet wired in `ubench_runner.run_full`. The shadow-run mechanism still lives in
  `evaluate_ft.py --scorer=dual` per Phase 10 Epic 4. Wiring is a one-day Phase 10.6 if
  needed; flagged here for tracker hygiene.

## Documents Accessed

- `docs/development/ft-lora/phase_10_benchmark_post.md`
- `docs/development/ft-lora/phase_10_5a_guardrail_golden.md`
- `docs/tests/uxts_report.py`, `docs/tests/uxts_runner_core.py`
- `docs/tests/uvts/schema/uvts.schema.json`, `docs/tests/uvts/specs/lnl_demo_validation.uvts.json`,
  `docs/tests/uvts/runners/uvts_runner.py`
- `docs/tests/ults/schema/ults.schema.json`, `docs/tests/ults/specs/*.ults.json`,
  `docs/tests/ults/runners/ults_runner.py`
- `configs/benchmark_phase10.yaml`
- `training_data/eval/valid_golden.jsonl`
- `neural/benchmarks/run_benchmark.py`
- `neural/training/reward_functions.py` (lines 491-516, 561)
- `Makefile`, `CLAUDE.md`
