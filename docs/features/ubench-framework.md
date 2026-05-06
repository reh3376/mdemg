# UBENCH Framework

**Status:** Active (Phase 10.5 follow-up, 2026-05-06)
**Type:** Reward-signal benchmark framework (UxTS pattern)
**Wraps:** `neural/benchmarks/run_benchmark.py`
**Companion:** `docs/development/ft-lora/phase_10_benchmark_post.md`

---

## Why this exists

Phase 10 shipped the automated benchmark runner (`neural.benchmarks.run_benchmark`), but it
sat outside the UxTS family of validation frameworks (UVTS / ULTS / UBTS / UATS / …). Two
classes of bug fell through the cracks as a result:

1. **Spec ↔ data drift.** ULTS spec `guardrail.evaluate` was authored mid-Sprint-B (after the
   Phase 5 dataset closed), so `valid_golden.jsonl` had zero matching rows. The benchmark
   runner silently emitted 16 specs × 5 runs = 80 rows and skipped the 17th — caller had no
   way to know one task was unscored. The 17th-task gap was caught only because the operator
   eyeballed the per-task table.
2. **No registry/heuristic parity gate enforced in CI.** Phase 10 Epic 4 added the
   `--scorer={heuristic,registry,dual}` flag with a 1% delta promise, but the only enforcement
   was a manual shadow-run.

UBENCH promotes the benchmark to UxTS so both classes fail loudly:

- A schema validates the .ubench.json spec, including config + holdout SHA pins.
- A `--mode contract` step asserts every ULTS spec resolves to ≥ N golden rows, refusing the
  shape that produced the guardrail.evaluate gap.
- `--mode run` invokes the wrapped runner and threshold-gates the canonical UxTS report on
  aggregate score + truncation count.
- A pytest wrapper at `docs/tests/ubench/contracts/test_dataset_holdout_contract.py` runs the
  contract in standard CI without requiring a live LLM endpoint.

## Choices

| Decision | Pick | Alternative considered | Why |
|---|---|---|---|
| Wrap or rewrite | **Wrap** `neural.benchmarks.run_benchmark` as a subprocess | Rewrite the runner inside `docs/tests/ubench/runners/` | The existing runner is 624 LOC + 114 unit/integration tests + 1 baseline already captured. Rewriting risks regressions for zero gain. |
| Where the spec lives | `docs/tests/ubench/specs/<name>.ubench.json` | Inline in the YAML config | Mirrors UVTS / ULTS / UBTS layout; one line in `verify_uxts_canonical_specs.py` covers it for free. |
| Default mode | `lint` (schema + SHA) | `contract` | Lint is the cheapest CI hook and the most common pre-merge check. Contract is a 5x cost (loads JSONL) and is the explicit `make test-ubench-contract` target. |
| Run mode invocation | subprocess of `python -m neural.benchmarks.run_benchmark` | Library import | Subprocess insulates UBENCH from runner-internal exceptions and matches how operators run the benchmark today. |
| Schema validation library | optional `jsonschema` import with required-key fallback | hard `jsonschema` dependency | The repo already uses optional jsonschema in `verify_uxts_canonical_specs.py`. UBENCH stays consistent. |
| Contract assertion shape | per-spec, per-task `≥ min_rows_per_task` | total holdout row floor | The spec language for "every spec has data" is per-spec. Total-row floors miss the silent-skip case. |

## How it works

### Files

```
docs/tests/ubench/
  schema/ubench.schema.json          # JSON-schema for .ubench.json specs
  specs/mdemg.ubench.json            # The canonical mdemg spec (Phase 10.5)
  runners/ubench_runner.py           # Spec validator + run-mode subprocess wrapper
  contracts/test_dataset_holdout_contract.py   # pytest entry for CI
```

### Modes

| Mode | What it does | LLM required? | Latency |
|---|---|---|---|
| `lint` | Schema validation + config + holdout SHA verification | No | <100ms |
| `contract` | Lint + dataset↔holdout contract: every ULTS spec has ≥ `min_rows_per_task` matching golden rows; spec count + total-row count + task count match expected | No | <100ms |
| `run` | Lint + contract + invoke `python -m neural.benchmarks.run_benchmark`; gate on `min_aggregate_weighted_score` + `max_truncated_rows` | **Yes** (`http://127.0.0.1:8102/v1` per Phase 13.5 cutover) | ~14 min |

### Spec key reference

```jsonc
{
  "ubench_version": "1.0.0",
  "benchmark":          {...},  // name, description, version
  "model_under_test":   {...},  // path, base_sha, adapter_manifest, endpoint
  "config":             {...},  // path + sha256 to YAML config
  "judge":              {...},  // model + max_tokens(>=3000) + latency_budget(>=15000)
  "ults":               {...},  // specs_dir + expected_specs count
  "golden_holdout":     {...},  // path + sha256 + expected_rows + expected_tasks + min_rows_per_task
  "registry_parity":    {...},  // optional shadow-run delta gate
  "thresholds":         {...},  // min_aggregate_weighted_score, max_truncated_rows
  "output":             {...},  // report_path, baseline_path, preflight_report_path
  "metadata":           {...}   // author, created, notes
}
```

The `min_rows_per_task` field is the load-bearing one — set it to the smallest acceptable
N before any task is silently under-evaluated. The Phase 10.5 spec sets it to 3 (matching
the seed-sample target from `phase_10_5a_guardrail_golden.md`).

### Canonical report shape

Reports are produced via `docs/tests/uxts_report.py::build_report` so the framework fits
into the same downstream tooling as UVTS / ULTS / UBTS / etc. — every UxTS framework writes
the same `framework / framework_version / summary / integrity / results` envelope.

## How to use

### CI: schema + contract (fast, no LLM)

```bash
make test-ubench-lint        # schema + SHA verification (~25ms)
make test-ubench-contract    # lint + dataset↔holdout contract (~25ms)
```

The pytest wrapper:

```bash
pytest docs/tests/ubench/contracts/ -v
```

### Operator: full benchmark run

Pre-flight: ensure the LLM endpoint is reachable.

```bash
curl -fsS http://127.0.0.1:8102/v1/models > /dev/null
make test-ubench-run
```

This invokes `python -m neural.benchmarks.run_benchmark` under the spec's pinned config
(currently `configs/benchmark_phase10.yaml`) and asserts the resulting aggregate score
clears `thresholds.min_aggregate_weighted_score` (0.80) with zero truncated rows.

### When a contract fails

Three failure shapes:

1. `ults.expected_specs: spec=N discovered=M` — the spec count drifted. Either (a) a new
   ULTS spec landed without bumping `expected_specs` in `mdemg.ubench.json`, or (b) a spec
   file became invalid JSON. Resolve before merge.
2. `golden_holdout.expected_rows / expected_tasks` mismatch — `valid_golden.jsonl` was
   regenerated. Update the spec's `expected_rows`, `expected_tasks`, and `sha256` together
   in one commit; the SHA pin guarantees the carve was deliberate.
3. `contract: ULTS tasks with zero golden rows: [...]` — the canonical Phase 10 gap. A
   spec was added without seeding holdout rows. Either close the gap (carve real rows
   from production traffic, e.g. `phase_10_5a_guardrail_golden.md` recipe) or delete the
   spec. Don't suppress the failure.

### Updating the spec after a deliberate change

```bash
# After changing configs/benchmark_phase10.yaml or training_data/eval/valid_golden.jsonl:
shasum -a 256 configs/benchmark_phase10.yaml
shasum -a 256 training_data/eval/valid_golden.jsonl
# Update config.sha256, golden_holdout.sha256, and expected_rows / expected_tasks
# inside docs/tests/ubench/specs/mdemg.ubench.json, then:
make test-ubench-contract
```

## Related

- `docs/development/ft-lora/phase_10_benchmark_post.md` §6 — the original guardrail.evaluate
  3-way gap that motivated this framework.
- `docs/development/ft-lora/phase_10_5a_guardrail_golden.md` — sub-parts 1+2 of #216;
  reward functions + golden rows. Sub-part 3 (Phase 5 SFT re-pass) is operator-deferred.
- `docs/tests/uxts_report.py` — canonical report builder shared across all UxTS frameworks.
- `docs/tests/uxts_runner_core.py` — shared SHA / canonical-JSON helpers.
- `configs/benchmark_phase10.yaml` — the pinned config the spec references.
- `neural/benchmarks/run_benchmark.py` — the wrapped runner.
