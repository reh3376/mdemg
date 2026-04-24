# Phase 11 — MLX Adapter (follow-up to PR #349)

**Status:** EXECUTED (2026-04-24)
**Predecessor:** FT-LORA-PHASE11 (PR #349 merged 2026-04-24) — GRPO orchestrator shipped with MLX adapter deferred to a follow-up per the sprint plan's "real MLX wiring is the Epic 6 e2e smoke item" note in `trainer.main()`.
**Scope:** the MLX adapter module that makes the compute pass runnable — nothing else.

## Summary

PR #349 shipped the GRPO step-loop orchestrator, loss, advantage estimator, reward sampler, and DPO pair generator, all exercised by 73 mocked tests. The four `Protocol`-typed callables (`RolloutFn` / `OptimizerStepFn` / `EvalFn` / `CheckpointFn`) were left as mock injection points; the trainer's `main()` logged "MLX adapter not yet wired" and exited 2. This change lands the real wiring.

## Deliverables

### 1. `neural/training/rl/mlx_adapter.py` — `MLXGRPOAdapter` class (~330 LOC)

Stateful implementation of all four Protocol callables against `mlx_lm==0.31.2`:

- **`rollout_fn(batch)`** — For each `RewardSample`: renders prompt via injected `prompt_provider(task_id, run_idx)`, calls `mlx_lm.generate()`, tokenizes prompt + response, computes sum-of-token logprobs under current policy (= `logprob_new` = `logprob_old`) and frozen reference (= `logprob_ref`), scores via injected `reward_fn(sample, response)`, **stashes `_StashedSample` tensors on `self._stash`** so the next optimizer step can rebuild the MLX computation graph.
- **`optimizer_step_fn(loss_value, *, advantages, keep_mask, rollouts)`** — Rebuilds GRPO loss inside `mx.value_and_grad` using the stashed tensors + advantages (recomputing `lp_new` under the *current* policy), runs `optimizer.update()`, forces `mx.eval()` so the next rollout sees updated weights. Cross-checks the MLX-recomputed loss against the numpy-computed one passed in; logs a warning if they disagree by > 1e-2.
- **`eval_fn(step)`** — Shells out to the Phase 10 benchmark runner via configurable `eval_cmd` (`["python", "-m", "neural.benchmarks.run_benchmark", ..., "--out", "/tmp/...json"]`), parses `aggregate_weighted_score` from the `--out` JSON. Returns NaN when `eval_cmd` is None or subprocess fails — the trainer's early-stop treats NaN as "no signal."
- **`checkpoint_fn(path, step)`** — Saves LoRA-only trainable parameters via `mx.save_safetensors(path/"adapters.safetensors", tree_flatten(model.trainable_parameters()))`. Layout matches `mlx_lm.load(adapter_path=...)` reload convention.

Pure helper `sum_response_logprobs(model, mx_mod, full_ids, prompt_len)` is exported for unit testing — computes per-token logprobs, masks out prompt positions, returns a scalar MLX array. Position-mask semantics: response targets start at index `prompt_len - 1` in the `targets` array (the position that predicts the first response token).

### 2. Trainer protocol extension (`neural/training/rl/trainer.py`)

`OptimizerStepFn` signature extended from `(loss_value: float) -> None` to also accept `advantages`, `keep_mask`, and `rollouts` as keyword arguments. The trainer threads these through from `_train_one_step`. Mocks use `lambda loss_v, **_: ...` — 5 test lambdas updated in `test_trainer_integration.py`. No change in test count (8 still pass); no behavior change for existing mocks since the `**_` catches the new kwargs.

### 3. Unit tests (`neural/training/rl/tests/test_mlx_adapter.py`) — 16 new

Heavy stub infrastructure installs numpy-backed stubs for `mlx.core` / `mlx.nn` / `mlx.optimizers` / `mlx.utils` / `mlx_lm` into `sys.modules` before the adapter module is imported. Covered cases:

- Init wires 2 `mlx_lm.load` calls (policy + reference), freezes reference, constructs AdamW with config lr
- `rollout_fn` stashes tensors per sample; zero-length-response → `stash[i] = None`
- `optimizer_step_fn` raises when invoked without prior rollout or without context kwargs
- Normal step path updates optimizer state + consumes stash
- All-dropped-batch path: early return with stash cleared, no optimizer call
- `eval_fn`: NaN when no cmd; parses aggregate from subprocess output; NaN on subprocess failure; raises when `--out` missing from cmd
- `checkpoint_fn` writes safetensors
- `sum_response_logprobs` edge cases (empty seq, prompt_len >= seq_len)
- `_expand_advantages_to_batch` helper: happy path + mismatch raises

Total Phase 11 suite: 73 → **89 tests** passing in 0.12s. No Metal dependency — runs in any CI environment.

## Architectural Notes

**Why stateful (stash between rollout and step):** `OptimizerStepFn` in the trainer's Protocol passes a pre-computed *scalar* loss. Real MLX backprop needs the full computation graph from model params → response-token logprobs → loss — which the scalar throws away. Rather than re-tokenize inside the step (expensive + error-prone re-derivation) we keep the tokenized tensors alive on the adapter instance. Stash lifetime is exactly one rollout/step pair; cleared after each optimizer update. Any optimizer step without a prior rollout raises a clear `RuntimeError`.

**Why recompute `lp_new` inside the loss closure:** The `lp_new` captured during `rollout_fn` is a materialized Python float — no backward graph. To get a real gradient we must forward-pass the stashed `full_ids` through the current model *inside* `loss_fn`, so `mx.value_and_grad` sees the dependency on the trainable LoRA params. `lp_old` and `lp_ref` remain frozen floats — `lp_old` stays at its rollout-time value (that's the whole point of PPO/GRPO's importance ratio), `lp_ref` comes from the frozen reference model.

**Prompt rendering is deliberately delegated.** The 16 live MDEMG call sites render prompts from Go code; replicating that in Python would double the work. `prompt_provider(task_id, run_idx) -> str` is the seam. The operator wires it at `main()` time (options documented in the docstring: live Go subprocess, cached `llm_interactions` rows, spec fixtures).

**Eval via subprocess instead of in-process:** The Phase 10 runner is already a CLI with its own TSDB-writing side effects. Embedding it would duplicate those paths; a subprocess call reuses the well-tested runner unchanged. Cost is a one-time Python startup per eval cadence (every `eval_interval_steps = 100`) — negligible compared to the eval's own runtime.

## Out of Scope

- `main()` wiring in `trainer.py` — still shells out and logs "MLX adapter not yet wired." Full end-to-end `python -m neural.training.rl.trainer --config ...` wiring is an operator-supplied script because the `prompt_provider` is site-specific.
- Real-MLX smoke test (loading a small-ish real model + running 1 step). Deferred to Epic 6 e2e; requires a Mac with Metal + a small checkpoint (Qwen2.5-0.5B fits) + `pytest -m mlx` selector.
- Live TSDB-writing persistence (still SQL sidecar).

## Artifacts

- `neural/training/rl/mlx_adapter.py` — ~330 LOC
- `neural/training/rl/tests/test_mlx_adapter.py` — ~230 LOC, 16 tests
- `neural/training/rl/trainer.py` — Protocol + call-site edit (10 lines)
- `neural/training/rl/tests/test_trainer_integration.py` — 5 lambda signatures updated
- This doc

## Documents Accessed

- `/Users/reh3376/mdemg/neural/training/rl/trainer.py` — Protocol definitions + `_train_one_step`
- `/Users/reh3376/mdemg/neural/training/rl/grpo_loss.py` — numerical-stability patterns (`_MAX_LOG_RATIO = 20.0`)
- `/Users/reh3376/mdemg/neural/training/rl/advantage.py` — `AdvantageEstimate` shape
- `/Users/reh3376/mdemg/neural/training/rl/reward_sampler.py` — `RewardSample` dataclass
- `/Users/reh3376/mdemg/neural/training/rl/tests/test_trainer_integration.py` — lambda mock patterns
- `/Users/reh3376/mdemg/configs/rl_phase11.yaml` — knobs the adapter consumes
- `/Users/reh3376/mdemg/docs/tests/ults/specs/consulting_classify.ults.json` — ULTS prompt shape (referenced for `prompt_provider` interface)
- `mlx_lm==0.31.2` API via `inspect.signature`: `mlx_lm.load`, `mlx_lm.generate`, `mlx_lm.tuner.utils.{linear_to_lora_layers, load_adapters}`, `mlx_lm.tuner.trainer.default_loss`
