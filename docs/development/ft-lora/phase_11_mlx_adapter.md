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

---

# Follow-up — Option A single-model + Tier 1 memory hygiene

**Status:** EXECUTED (2026-04-24); full compute pass **operator-gated**.

## What happened

The initial MLX adapter (above) loaded both the policy and a frozen reference
copy of Qwen3-14B via two `mlx_lm.load(...)` calls. On Apple Silicon that
silently trips the Metal **MTLResource descriptor ceiling** (~499K per
process — the same ceiling that forced the Phase 5 MoE→dense pivot); the
process is jetsam-killed at the second load with no Python traceback.

Two follow-up tracks:

### 1. Option A — single model, LoRA scale toggling for the reference forward

`_lora_disabled(model)` context manager in `mlx_adapter.py`:

- On enter, walks `model.named_modules()` and saves+zeroes `.scale` on every
  `LoRALinear` / `LoRASwitchLinear` / `LoRAEmbedding` instance.
- `LoRALinear.__call__` is `base(x) + scale * ((dropout(x) @ lora_a) @ lora_b)`
  in `mlx_lm==0.31.2` — `scale=0` collapses the forward to the base linear
  exactly, bit-identical to a base-only model.
- On exit, restores every saved scale. Defensive with bare models or test
  stubs lacking `named_modules` → no-op in those cases.

Result: a single loaded weight set serves as both policy (LoRA on) and
frozen reference (LoRA off, Python-scalar toggle, no graph recompile).
The second `mlx_lm.load` is removed; init log reads `reference policy =
same weights with LoRA scales zeroed in-forward (descriptor-budget fix)`.

### 2. Tier 1 memory hygiene — descriptor cache + per-step telemetry

`mlx_adapter.py` additions:

- `_clear_cache()` helper calling `mx.clear_cache()` after each
  `mlx_lm.generate()`, after each policy+reference forward pair, at
  end-of-batch in `rollout_fn`, and after the optimizer barrier in
  `optimizer_step_fn`. Guards with `getattr` so test stubs stay wireable.
- Combined `mx.eval(model.state, optimizer.state, mlx_loss)` barrier —
  one graph materialization per step instead of three separate closures.
- `mx.reset_peak_memory()` at step entry so the per-step log line shows
  the *current* step's high-watermark, not a cumulative figure.
- Per-step telemetry:
  `step=N loss=%.6f peak_mem=%.2fGB cache_mem=%.2fGB active_mem=%.2fGB`.

`_mem_snapshot()` reads `mx.get_peak_memory`, `mx.get_cache_memory`,
`mx.get_active_memory`; missing APIs return 0.0 (test stubs).

**Deliberately NOT done: `mx.set_wired_limit(...)`.** On a Mac where
`max_recommended_working_set_size` equals (or very nearly equals) total RAM
— as on the 128 GB validation machine — wiring that much memory starves
the kernel of pageable headroom and can trigger a `watchdogd` panic under
sustained training load. Observed during Phase 11 Tier 1 validation: 128 GB
RAM, `max_recommended≈128.85 GB` → kernel panic after ~11 min of 25-step
smoke. Apple's recommended working set is an approximation of what Metal
*can use*, not what is *safe to wire*. Comment in `__init__` documents this
for future readers. The canonical `mlx_lm_lora/trainer/grpo_trainer.py` we
mirror does not set a wired limit either.

## Validation

- 19/19 `test_mlx_adapter.py` tests green (3 new `_lora_disabled` tests +
  `test_init_loads_policy_only`).
- 106/106 full Phase 11 suite green (`pytest neural/training/rl/tests/
  neural/training/dpo/tests/`).
- 5-step smoke against the real Phase 5 dense adapter: **PASS**. Losses
  `2.249 / 1.015 / 3.271 / 3.188 / 3.423`; `cache_mem=0.00 GB` on every
  step post-barrier; `active_mem=8.31 GB` flat; checkpoint + sidecar
  written cleanly. Result reproduced across successive 5-step runs with
  bit-identical losses (same seed).

## Full compute pass (100-step): operator-gated

Rationale: two 25-step extended smoke attempts on the validation Mac (128 GB
unified memory) surfaced hardware-level constraints distinct from the
descriptor-cache issue:

| Config | Outcome | Cause |
|---|---|---|
| `--batch-size 4` | died step 13 | jetsam (peak 253 GB — 4 parallel backward graphs × seq_len=3000 on Qwen3-14B) |
| `--batch-size 1` | died step 6 | silent (no jetsam event, no Python traceback, no crash dump) |

Peak memory per step at batch_size=1 was fine (15-35 GB steady-state, 87 GB
first-step compile); the batch_size=4 jetsam was activation memory during
backward, not descriptor leak. The batch_size=1 silent death cause is
undiagnosed — likely Metal-level allocation failure that bypasses the usual
VM jetsam path. System-side swap was 80% full by the time of both deaths.

The sprint-plan predecessor (`87f69fc`) already flagged this ("compute pass
operator-gated"). The full 100-step run is deferred to a dedicated operator
session:

1. Fresh reboot (clears any Metal orphan allocations).
2. No other GPU/ANE consumers (close Chrome/Teams/Docker).
3. `--batch-size 1` initially; attempt batch-size 2 only after 50+ steps
   prove stable.
4. Monitor `cache_mem` in the per-step telemetry; any non-zero value is
   the signal to kill.

Phase 12 (HITL DPO) is **not blocked** by the deferred full run — the DPO
pair generator consumed the same Phase 10 benchmark rows and shipped with
its manifest in PR #349. The regression gate (Epic 5) can consume the
Phase 11 adapter once it exists.

## Architectural follow-ups worth considering before Phase 12

Neither is a blocker, but both would materially reduce the compute-pass
memory budget:

- **Gradient checkpointing** on the LoRA-targeted transformer layers.
  `mlx_lm` does not expose this natively; implementing it requires a
  thin forward-hook wrapper. ~10× reduction in backward-activation memory.
- **Per-sample gradient accumulation in the optimizer step.** Today
  `optimizer_step_fn` rebuilds the loss over all kept samples
  simultaneously; reshaping to accumulate per-sample gradients would
  bound peak memory to 1 sample regardless of batch size, at the cost
  of N forward passes serialized.

## Artifacts (delta over preceding commit)

- `neural/training/rl/mlx_adapter.py` — `_lora_disabled` CM + Tier 1 hygiene
  + `reset_peak_memory` step boundary + telemetry (~90 LOC added)
- `neural/training/rl/tests/test_mlx_adapter.py` — 3 new `_lora_disabled`
  tests; `test_init_loads_policy_and_reference` renamed and rewritten to
  assert single load
- `neural/training/rl/trainer.py` — `main()` rewritten from stub to live
  MLX wiring (loads MLXGRPOAdapter, wires prompt_provider/reward_fn,
  builds registry eval_cmd)
- `neural/training/rl/live_wiring.py` (new, ~400 LOC) —
  `ChatTemplatedPromptProvider` + `SpecDrivenRewardFn` + `InProcessEvaluator`
- `neural/training/rl/tests/test_live_wiring.py` (new, ~380 LOC)
- This doc (Follow-up section appended)

---

# Tier 2 — Per-sample gradient accumulation

**Status:** EXECUTED (2026-04-24); proven to extend stable run length but not sufficient alone for 25+ step runs on 128 GB unified memory.

## What changed

`optimizer_step_fn` rewritten from one-shot `value_and_grad` over all N kept
samples to a per-sample loop:

```python
for local_i, orig_i in enumerate(kept_indices):
    def sample_loss_fn(model, ...):
        lp_new_i = sum_response_logprobs(model, mx, full_ids_i, prompt_len_i)
        # Per-sample clipped surrogate + KL + entropy, scaled by 1/N
        return (policy_loss_i + kl_coef * kl_i - entropy_coef * entropy_i) / n_kept

    sample_loss, sample_grads = nn.value_and_grad(self.model, sample_loss_fn)(self.model)

    if grads_accum is None:
        grads_accum = sample_grads
    else:
        grads_accum = tree_map(lambda a, b: a + b, grads_accum, sample_grads)

    mx.eval(grads_accum)          # free this sample's forward/backward graph
    self._clear_cache()

self.optimizer.update(self.model, grads_accum)
```

**Mathematically identical** to the prior batched-mean loss: `∂(mean_i L_i)/∂θ = (1/N) Σ ∂L_i/∂θ`. No change to GRPO semantics.

**Memory profile**: peak activation memory is bounded to ONE sample's
forward+backward graph regardless of batch size. Between samples,
`mx.eval(grads_accum)` materializes the accumulated grads and releases the
prior sample's activation tape; `_clear_cache()` drains Metal descriptor
buffers.

## Validation

- 19/19 `test_mlx_adapter.py` green (stub gained `mlx.utils.tree_map`).
- 106/106 full Phase 11 + DPO suite green.
- 5-step smoke at batch_size=4: **PASS**. Peaks now vary 13-91 GB per step
  (was monotonic watermark before). Losses differ slightly from Tier 1
  (`2.250 / 0.940 / 3.023 / 1.776 / 3.567` vs Tier 1 `2.249 / 1.015 / 3.271 /
  3.188 / 3.423`) — expected because per-sample vs batched-mean means
  slightly different numerical paths; both are correct GRPO.
- 25-step smoke at batch_size=4: made it to **step 16** (vs Tier 1 step 13
  jetsam kill). Terminal operator-kill at step 16 because swap saturated.
  Key observations:
  - Most steps: 13-100 GB peak, ~20-50 sec wall-clock
  - ~every 10th step: 230-240 GB peak, 5-9 min wall-clock (swap thrashing)
  - The "monster" steps coincide with 4× long-sequence samples in one batch;
    the per-sample loop serializes the backward passes but one sample's
    Qwen3-14B × 3000-token forward+backward can transiently hit ~60 GB,
    and MLX's allocator keeps unfused-op intermediates pushing peak higher.

## Per-step memory spike: root cause & next lever

Per-sample accumulation cuts the N-way batch-stacking problem but does not
address the **fundamental single-sample ceiling**: Qwen3-14B × seq_len=3000
activation tape × MLX's current allocator behavior (no fused attention, no
gradient checkpointing) = occasional 230+ GB transients on outlier batches.

**Next architectural lever — gradient checkpointing on transformer layers.**
`mx.checkpoint` (or a thin module wrap equivalent) recomputes layer forward
activations during backward instead of storing them. Expected impact:
~10× reduction in backward activation memory, at the cost of ~33% more
wall-clock per step. Implementation target: wrap each `Qwen3DecoderLayer`
call site in `mlx_lm`'s model with a checkpoint shim.

Neither Tier 2 alone nor Tier 2 + checkpointing changes GRPO mathematics;
both are pure memory-management changes.

## 100-step compute pass: still operator-gated

Tier 2 extends the stable run length substantially (beat both prior kill
points) but does not yet close the hardware gap on 128 GB unified memory
with outlier long-sequence batches. Until gradient checkpointing lands,
the operator preconditions in the Follow-up section above (fresh reboot,
batch_size 1 first, monitor telemetry) remain in force.

## Artifacts (delta over Tier 1 merge at c816e71)

- `neural/training/rl/mlx_adapter.py` — `optimizer_step_fn` refactor (~70 LOC
  changed). Per-sample loop + `tree_map`-based accumulation.
- `neural/training/rl/tests/test_mlx_adapter.py` — stub gained `_tree_map`
  helper; existing tests still pass without modification (the accumulation
  pattern lands with a single sample batch in test fixtures).
- This doc (Tier 2 section appended)
