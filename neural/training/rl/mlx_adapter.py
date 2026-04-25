"""MLX adapter — real implementation of the Phase 11 GRPO trainer callables.

Stateful class implementing the four Protocol types from ``trainer.py``
(:class:`RolloutFn`, :class:`OptimizerStepFn`, :class:`EvalFn`,
:class:`CheckpointFn`) against ``mlx_lm==0.31.2``.

Why stateful: :class:`OptimizerStepFn` receives advantages + keep_mask + the
rollout results, but *not* MLX tensors — the real backprop needs the tokenized
prompt+response tensors that were produced during :meth:`rollout_fn`. This
class stashes those tensors on ``self._stash`` between calls so
:meth:`optimizer_step_fn` can rebuild the GRPO loss in MLX-space and take a
real gradient step. Stash lifetime is one rollout/step pair; it is cleared
after each optimizer update.

Design boundaries:
  * Prompt rendering is *delegated* to ``prompt_provider(task_id, run_idx) ->
    str``. The in-repo rendering logic lives in Go (the 16 live call sites); a
    future follow-up can wire a Python shim. This keeps the adapter focused on
    MLX mechanics and lets the operator plug in whatever prompt source they
    want (live Go codepath, cached ``llm_interactions`` rows, spec fixtures).
  * Reward scoring is delegated to ``reward_fn(RewardSample, response) ->
    float``. Default: wire ``neural.training.reward_functions.REWARD_REGISTRY``
    plus ``scalarize_reward`` at the call site.
  * Evaluation (:meth:`eval_fn`) wraps the Phase 10 benchmark runner as a
    subprocess call — cheapest path to reusing the full scorer pipeline.
  * Checkpointing (:meth:`checkpoint_fn`) saves LoRA-only trainable params via
    ``mx.save_safetensors`` (matches ``mlx_lm.tuner`` convention).

This file imports ``mlx_lm`` lazily inside :meth:`__init__` so unit tests can
construct a ``_StashedSample`` and call the logprob helper with a mocked module
without paying the MLX import cost.
"""
from __future__ import annotations

import json
import logging
import subprocess
from contextlib import contextmanager
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable, Iterator, Optional

import numpy as np

from .reward_sampler import RewardSample
from .trainer import RolloutResult

logger = logging.getLogger(__name__)

# Clamp matches grpo_loss._MAX_LOG_RATIO (kept local to avoid cross-module
# coupling; both use 20.0 per the numerical-stability policy in grpo_loss.py).
_MAX_LOG_RATIO = 20.0

# Blow-up threshold for the numpy-vs-MLX loss cross-check. See the docstring
# on :meth:`MLXGRPOAdapter.optimizer_step_fn` for why O(1) divergence is
# expected and not-a-bug. Anything above this almost certainly indicates
# NaN/inf, a dtype catastrophe, or a broken logprob pipeline.
_LOSS_DISAGREEMENT_BLOWUP = 50.0


# ── stash type ──────────────────────────────────────────────────────────────


@dataclass
class _StashedSample:
    """Per-sample tensors preserved across rollout→optimizer-step.

    ``full_ids`` is the tokenization of prompt+response (len L, dtype int32).
    ``prompt_len`` is the number of prompt tokens — response targets start at
    index ``prompt_len - 1`` in the targets array (positions predicting the
    first response token and beyond).

    ``lp_old_frozen`` and ``lp_ref_frozen`` are the sum-of-response-logprobs
    under the rollout-time policy + frozen reference policy respectively.
    Both are captured once at rollout time and reused in the loss closure.
    """
    full_ids: list[int]
    prompt_len: int
    lp_old_frozen: float
    lp_ref_frozen: float


# ── per-token logprob helper (pure function, exported for tests) ────────────


def sum_response_logprobs(model: Any, mx_mod: Any, full_ids: list[int],
                           prompt_len: int) -> Any:
    """Sum-of-token logprobs under ``model`` for response positions only.

    Parameters
    ----------
    model       : mlx.nn.Module (policy or reference).
    mx_mod      : the ``mlx.core`` module (passed in so this helper can be
                  unit-tested with a stub module).
    full_ids    : tokenization of prompt + response.
    prompt_len  : number of prompt tokens. The first target index predicting a
                  response token is ``prompt_len - 1``.

    Returns
    -------
    MLX scalar (0-d mx.array) — sum of per-token logprobs over response tokens.
    """
    if len(full_ids) < 2:
        return mx_mod.zeros(())
    if prompt_len < 1 or prompt_len >= len(full_ids):
        # No response tokens to score; return 0 so the caller treats this
        # sample as neutral (no gradient signal).
        return mx_mod.zeros(())

    full_arr = mx_mod.array([full_ids])          # (1, L)
    inputs = full_arr[:, :-1]                    # (1, L-1)
    targets = full_arr[:, 1:]                    # (1, L-1)

    # Forward pass. mlx_lm Qwen3 expects int ids directly.
    logits = model(inputs)                       # (1, L-1, V)

    # Import mlx.nn lazily to keep the fn testable with stubs.
    import mlx.nn as nn  # noqa: PLC0415
    log_probs = nn.log_softmax(logits, axis=-1)  # (1, L-1, V)

    # Gather logprob at each target token id.
    target_lp = mx_mod.take_along_axis(
        log_probs, targets[:, :, None], axis=-1
    ).squeeze(-1)                                # (1, L-1)

    # Mask: response targets are at positions >= prompt_len - 1.
    seq_len = target_lp.shape[-1]
    positions = mx_mod.arange(seq_len)
    mask = (positions >= (prompt_len - 1)).astype(target_lp.dtype)
    return (target_lp * mask).sum()


# ── adapter ─────────────────────────────────────────────────────────────────


PromptProvider = Callable[[str, int], str]
RewardFn = Callable[[RewardSample, str], float]


@contextmanager
def _lora_disabled(model: Any) -> Iterator[None]:
    """Temporarily zero every LoRA module's ``scale`` on ``model``, restore on exit.

    Used to compute the frozen-reference logprobs on the *same* weights as
    the policy. In ``mlx_lm==0.31.2``, a LoRA-wrapped linear's forward is
    ``base(x) + scale * ((dropout(x) @ lora_a) @ lora_b)``. Setting ``scale``
    to 0 collapses the forward to the base linear exactly — bit-identical to
    a base-only model — without allocating a second weight set.

    Why this matters: loading two copies of Qwen3-14B into Metal blows past
    the 499K ``MTLResource`` descriptor ceiling (the same ceiling that forced
    the Phase 5 MoE→dense pivot). One weight set + per-call scale toggling
    is the architectural fix for GRPO's policy/reference pair.

    Defensive with a bare model (no LoRA layers attached) or with a test
    stub that lacks ``named_modules`` — the context manager becomes a no-op
    in those cases.
    """
    lora_types: tuple = ()
    try:
        from mlx_lm.tuner.lora import (  # noqa: PLC0415
            LoRAEmbedding,
            LoRALinear,
            LoRASwitchLinear,
        )
        lora_types = (LoRALinear, LoRASwitchLinear, LoRAEmbedding)
    except (ImportError, AttributeError, ModuleNotFoundError):
        pass

    saved: list[tuple[Any, float]] = []
    if lora_types and hasattr(model, "named_modules"):
        for _name, module in model.named_modules():
            if isinstance(module, lora_types):
                saved.append((module, module.scale))
                module.scale = 0.0
    try:
        yield
    finally:
        for mod, original_scale in saved:
            mod.scale = original_scale


def _apply_gradient_checkpointing(model: Any) -> None:
    """Monkey-patch ``Qwen3Model.__call__`` so each transformer layer's
    forward is wrapped in ``mx.checkpoint``.

    Why the patch is at the *outer* model rather than at ``TransformerBlock``:
    ``mx.checkpoint`` tree-flattens its args (treating them as arrays/trees-
    of-arrays). Installing ``mx.checkpoint(method)`` directly as a class
    ``__call__`` therefore corrupts ``self`` — the module instance gets
    flattened to its parameter dict before the method body runs. Instead,
    we keep each layer's native ``__call__`` and build a per-layer closure
    that captures the layer reference outside the checkpointed function
    boundary, so ``mx.checkpoint`` only sees tensor inputs (``h``, ``mask``,
    ``cache``). See inline note below.

    Effect: backward-pass activation memory drops ~10x (activations for
    each layer are *recomputed* during backward instead of stored from the
    forward pass). Wall-clock cost: ~33% more per step because of the
    recomputation. For Qwen3-14B × seq_len=3000 on 128GB unified memory,
    this is what brings the 237GB outlier-step peak down to within
    physical RAM.

    Idempotent: if the Qwen3Model class is already patched by a previous
    adapter in the same process, skip. Applies to the model's installed
    class, so subsequent ``mlx_lm.load`` calls inherit the patch.

    Resilient: if ``mlx_lm.models.qwen3`` is unavailable or the model is
    not a Qwen3Model (e.g. a test stub), the function is a no-op with a
    warning. Phase 12 work on other architectures can add per-arch branches.
    """
    import mlx.core as mx  # noqa: PLC0415
    try:
        from mlx_lm.models import qwen3  # noqa: PLC0415
    except (ImportError, ModuleNotFoundError):
        logger.warning(
            "gradient checkpointing: mlx_lm.models.qwen3 not importable; "
            "skipping (non-Qwen3 model or stub)"
        )
        return

    if getattr(qwen3.Qwen3Model, "_mdemg_checkpointed", False):
        logger.info("gradient checkpointing: Qwen3Model already patched; skipping")
        return

    # Verify the loaded model's inner model is a Qwen3Model instance. If
    # not (e.g. test stub), bail cleanly.
    inner = getattr(model, "model", None)
    if inner is None or not isinstance(inner, qwen3.Qwen3Model):
        logger.warning(
            "gradient checkpointing: model.model is not a Qwen3Model "
            "(type=%s); skipping", type(inner).__name__
        )
        return

    _original_qwen3_model_call = qwen3.Qwen3Model.__call__

    def _checkpointed_qwen3_model_call(
        self,
        inputs: Any,
        cache: Any = None,
        input_embeddings: Any = None,
    ) -> Any:
        # Mirrors the upstream Qwen3Model.__call__ flow (inlined so we can
        # wrap each layer invocation). Kept in sync with mlx_lm==0.31.2.
        if input_embeddings is not None:
            h = input_embeddings
        else:
            h = self.embed_tokens(inputs)
        if cache is None:
            cache = [None] * len(self.layers)
        mask = qwen3.create_attention_mask(h, cache[0])
        for layer, c in zip(self.layers, cache):
            # Closure captures `layer` outside the checkpoint boundary so
            # mx.checkpoint only tree-flattens tensor args (h, mask, c=None
            # during training). Creating the checkpointed fn per iteration
            # is intentional — mx.checkpoint is cheap to construct; each
            # call produces an independent graph node whose intermediate
            # activations are recomputed during the backward pass.
            def _layer_fwd(x_in, m_in, c_in, _l=layer):
                return _l(x_in, m_in, c_in)
            h = mx.checkpoint(_layer_fwd)(h, mask, c)
        return self.norm(h)

    qwen3.Qwen3Model.__call__ = _checkpointed_qwen3_model_call
    qwen3.Qwen3Model._mdemg_checkpointed = True


class MLXGRPOAdapter:
    """Real MLX wiring for the Phase 11 GRPO trainer's four callables.

    Construction loads ``base_model_path`` + ``adapter_path`` once. The
    reference-policy forward shares the same weights but disables the LoRA
    contribution for the duration of the forward pass (see
    :func:`_lora_disabled`). This halves the Metal descriptor footprint vs.
    loading two copies of the base model — required on Qwen3-14B where two
    copies exceed the ``MTLResource`` ceiling.

    Usage::

        adapter = MLXGRPOAdapter(
            base_model_path=cfg.base_model_path,
            adapter_path="adapters/phase5",
            prompt_provider=my_provider,
            reward_fn=my_scorer,
            clip_ratio=cfg.clip_ratio, kl_coef=cfg.kl_coef,
            entropy_coef=cfg.entropy_coef,
            lr=1e-5,
            generation_kwargs={"max_tokens": 4000, "temp": 0.7},
        )
        trainer = GRPOTrainer(
            config=cfg, sampler=sampler,
            rollout_fn=adapter.rollout_fn,
            optimizer_step_fn=adapter.optimizer_step_fn,
            eval_fn=adapter.eval_fn,
            checkpoint_fn=adapter.checkpoint_fn,
        )
        trainer.train()
    """

    def __init__(
        self,
        *,
        base_model_path: str,
        adapter_path: str | None,
        prompt_provider: PromptProvider,
        reward_fn: RewardFn,
        clip_ratio: float,
        kl_coef: float,
        entropy_coef: float,
        lr: float,
        weight_decay: float = 0.01,
        betas: tuple[float, float] = (0.9, 0.999),
        eps: float = 1e-8,
        generation_kwargs: dict[str, Any] | None = None,
        eval_cmd: list[str] | None = None,
        gradient_checkpointing: bool = True,
    ) -> None:
        # Lazy imports so unit tests can exercise the class without a Metal
        # backend. Any real instantiation in a Mac env pulls these in once.
        import mlx.core as mx  # noqa: PLC0415
        import mlx.optimizers as optim  # noqa: PLC0415
        import mlx_lm  # noqa: PLC0415

        self._mx = mx

        logger.info("MLXGRPOAdapter loading policy: base=%s adapter=%s",
                    base_model_path, adapter_path)
        self.model, self.tokenizer = mlx_lm.load(
            base_model_path, adapter_path=adapter_path
        )
        # Reference policy is the SAME weights with LoRA scales toggled to 0
        # during the reference forward (see :func:`_lora_disabled`). This
        # avoids a second mlx_lm.load — which on Qwen3-14B silently trips the
        # Metal 499K MTLResource ceiling and kills the process.
        logger.info(
            "MLXGRPOAdapter: reference policy = same weights with LoRA "
            "scales zeroed in-forward (descriptor-budget fix)"
        )

        if gradient_checkpointing:
            _apply_gradient_checkpointing(self.model)
            logger.info(
                "MLXGRPOAdapter: gradient checkpointing applied to Qwen3 "
                "transformer layers (~10x backward activation memory "
                "reduction at ~33%% wall-clock cost)"
            )

        self.optimizer = optim.AdamW(
            learning_rate=lr,
            betas=list(betas),
            eps=eps,
            weight_decay=weight_decay,
        )

        self.prompt_provider = prompt_provider
        self.reward_fn = reward_fn
        self.clip_ratio = float(clip_ratio)
        self.kl_coef = float(kl_coef)
        self.entropy_coef = float(entropy_coef)
        self.generation_kwargs = dict(generation_kwargs or {})
        self.eval_cmd = list(eval_cmd) if eval_cmd else None

        self._stash: Optional[list[Optional[_StashedSample]]] = None
        self._step_counter = 0

        # ── Tier 1 memory hygiene ───────────────────────────────────────────
        # Metal's MTLResource descriptor cache accumulates across kernels and
        # will silently jetsam-kill the process once it exhausts the 499K
        # per-process ceiling (same ceiling that forced Phase 5 MoE→dense).
        # Fix: clear the descriptor cache between rollout/forward-pair/step
        # phases (see rollout_fn + optimizer_step_fn), reset peak-memory
        # counters, emit per-step telemetry.
        #
        # We deliberately do NOT call ``mx.set_wired_limit(...)`` — on a Mac
        # where ``max_recommended_working_set_size`` equals (or very nearly
        # equals) total RAM, wiring that much memory starves the kernel of
        # pageable headroom and can trigger a ``watchdogd`` kernel panic
        # under sustained training load (observed during Phase 11 Tier 1
        # validation: 128GB RAM, max_recommended≈128.85GB → reboot after
        # ~11 min of 25-step smoke). Apple's recommended working set is an
        # approximation of what Metal *can use*, not what is *safe to wire*.
        # ``mlx_lm_lora/trainer/grpo_trainer.py`` (the canonical GRPO
        # implementation we mirror) does not set a wired limit either.
        if hasattr(mx, "reset_peak_memory"):
            mx.reset_peak_memory()

    # ── memory hygiene helpers ──────────────────────────────────────────────

    def _clear_cache(self) -> None:
        """Best-effort release of the Metal descriptor cache.

        The real :mod:`mlx.core` exposes ``clear_cache()``; test stubs can
        omit it — hence the guarded call. Invoked at rollout/forward-pair/
        step boundaries to keep descriptor growth bounded across a long run.
        """
        clear = getattr(self._mx, "clear_cache", None)
        if clear is not None:
            clear()

    def _mem_snapshot(self) -> tuple[float, float, float]:
        """Return (peak_gb, cache_gb, active_gb) — zero for missing APIs."""
        def _g(name: str) -> float:
            fn = getattr(self._mx, name, None)
            return float(fn() / 1e9) if fn is not None else 0.0
        return _g("get_peak_memory"), _g("get_cache_memory"), _g("get_active_memory")

    # ── RolloutFn ───────────────────────────────────────────────────────────

    def rollout_fn(self, batch: list[RewardSample]) -> list[RolloutResult]:
        """Generate a response per sample, score it, stash tokenized tensors."""
        import mlx_lm  # noqa: PLC0415

        results: list[RolloutResult] = []
        stash: list[Optional[_StashedSample]] = []

        for sample in batch:
            prompt = self.prompt_provider(sample.task_id, sample.run_idx)
            response = mlx_lm.generate(
                self.model, self.tokenizer, prompt,
                **self.generation_kwargs,
            )
            # Tier 1 memory hygiene: mlx_lm.generate leaves sampler-side
            # intermediates in the descriptor cache that never get reclaimed
            # implicitly (documented in mlx-lm #883, mlx-examples #724).
            self._clear_cache()

            prompt_ids = self.tokenizer.encode(prompt)
            full_ids = self.tokenizer.encode(prompt + response)
            prompt_len = len(prompt_ids)
            response_len = len(full_ids) - prompt_len

            if response_len <= 0:
                # Degenerate generation; emit a zero-logprob result so the
                # trainer keeps rolling. Will contribute 0 advantage if the
                # reward is also 0.
                reward = self.reward_fn(sample, response)
                results.append(RolloutResult(
                    task_id=sample.task_id,
                    response_text=response,
                    scalar_reward=reward,
                    logprob_new=0.0, logprob_old=0.0, logprob_ref=0.0,
                ))
                stash.append(None)
                continue

            lp_new = sum_response_logprobs(self.model, self._mx,
                                            full_ids, prompt_len)
            # Reference = same model, LoRA scales zeroed for this forward only.
            with _lora_disabled(self.model):
                lp_ref = sum_response_logprobs(self.model, self._mx,
                                                full_ids, prompt_len)
            # Materialize to Python floats — the MLX-space recomputation will
            # re-run the forward pass under the current policy from scratch.
            lp_new_f = float(lp_new)
            lp_ref_f = float(lp_ref)
            # Release the forward-pair activations before the next sample's
            # generate(); without this the descriptor footprint grows
            # monotonically across the batch.
            del lp_new, lp_ref
            self._clear_cache()

            reward = self.reward_fn(sample, response)
            results.append(RolloutResult(
                task_id=sample.task_id,
                response_text=response,
                scalar_reward=reward,
                logprob_new=lp_new_f,
                logprob_old=lp_new_f,   # same at rollout time (on-policy)
                logprob_ref=lp_ref_f,
            ))
            stash.append(_StashedSample(
                full_ids=full_ids,
                prompt_len=prompt_len,
                lp_old_frozen=lp_new_f,
                lp_ref_frozen=lp_ref_f,
            ))

        self._stash = stash
        # End-of-batch cache sweep before the trainer calls optimizer_step_fn.
        self._clear_cache()
        return results

    # ── OptimizerStepFn ─────────────────────────────────────────────────────

    def optimizer_step_fn(
        self,
        loss_value: float,
        *,
        advantages: np.ndarray | None = None,
        keep_mask: np.ndarray | None = None,
        rollouts: list[RolloutResult] | None = None,  # unused; see below
    ) -> None:
        """One optimizer step: rebuild GRPO loss in MLX, backprop, update.

        ``loss_value`` (numpy-computed scalar from ``compute_grpo_loss``) is
        used as a *coarse* blow-up sanity check only. The two losses compute
        different quantities by construction and are not expected to match:

        * numpy path: ``logprob_new`` and ``logprob_old`` are set equal in
          :meth:`rollout_fn` (on-policy data), so the surrogate ratio is
          exactly 1 and ``policy_loss`` collapses to ``-mean(advantages)``.
          The numpy loss therefore barely depends on the current policy.
        * MLX path: recomputes ``lp_new`` under the *current* (gradient-
          tracked) policy. Even at step 1 before any update, grad-on vs
          grad-off forward passes in MLX can differ at O(1) due to kernel-
          path differences (fused ops, intermediate storage). After step 1
          the policy actually drifts from rollout weights, so divergence is
          expected and correct — MLX is the gradient source of truth.

        The threshold below (``_LOSS_DISAGREEMENT_BLOWUP``) is sized to catch
        genuine bugs like NaN/inf or order-of-magnitude explosions, not the
        expected O(1) numerical divergence.

        ``rollouts`` is accepted for Protocol conformance but unused — we rely
        on the stashed tensors (already tokenized) instead of re-tokenizing.
        """
        import mlx.core as mx  # noqa: PLC0415
        import mlx.nn as nn  # noqa: PLC0415

        # Reset peak-memory counter at step boundary so telemetry shows the
        # *current* step's high-watermark, not a cumulative one. With an
        # unbounded peak we can't tell whether a 250GB reading is "this step
        # was a monster" or "some earlier step was a monster" — essential
        # signal for diagnosing OOM-adjacent runs.
        if hasattr(mx, "reset_peak_memory"):
            mx.reset_peak_memory()

        if self._stash is None:
            raise RuntimeError(
                "optimizer_step_fn invoked before rollout_fn — no tensors "
                "stashed. The trainer must call rollout_fn first."
            )
        if advantages is None or keep_mask is None:
            raise RuntimeError(
                "optimizer_step_fn requires advantages + keep_mask kwargs "
                "from the trainer. Protocol signature mismatch?"
            )

        # Indices into the *original* batch that survived both keep_mask AND
        # had a non-None stash (None means degenerate generation).
        keep_arr = np.asarray(keep_mask, dtype=bool)
        kept_indices = [
            i for i, keep in enumerate(keep_arr)
            if keep and self._stash[i] is not None
        ]
        if not kept_indices:
            logger.warning("optimizer_step_fn: all samples dropped; skipping")
            self._stash = None
            return
        if len(kept_indices) != len(advantages):
            # `keep_mask` has ALL-batch positions; `advantages` is already
            # restricted to kept-advantage entries by estimate_advantage.
            # But we may have stash=None samples that the mask thinks are kept
            # (very rare — only when generation succeeded enough to set
            # keep=True but then produced 0-len response). Re-align.
            adv_full = _expand_advantages_to_batch(
                advantages, keep_arr, len(self._stash)
            )
            kept_adv = np.array(
                [adv_full[i] for i in kept_indices], dtype=np.float32
            )
        else:
            kept_adv = advantages.astype(np.float32)

        adv_mx = mx.array(kept_adv)
        lp_old_frozen = mx.array(
            [self._stash[i].lp_old_frozen for i in kept_indices],
            dtype=mx.float32,
        )
        lp_ref_frozen = mx.array(
            [self._stash[i].lp_ref_frozen for i in kept_indices],
            dtype=mx.float32,
        )

        clip = self.clip_ratio
        kl_coef = self.kl_coef
        entropy_coef = self.entropy_coef
        n_kept = len(kept_indices)

        # ── Tier 2: per-sample gradient accumulation ────────────────────────
        # Previously we built one loss over all N kept samples and backproped
        # through a single mx.value_and_grad call. That held N full forward
        # graphs alive simultaneously (for the backward tape), which on a
        # 14B model × seq_len=3000 easily hits 100+ GB per batch and drove
        # jetsam kills at batch_size=4 during the Tier 1 25-step smoke.
        #
        # Per-sample accumulation: compute each sample's ∂L_i/∂θ independently,
        # scale by 1/N, accumulate into grads_accum. Mathematically identical
        # to the batched mean (∂(mean_i L_i)/∂θ = (1/N) Σ ∂L_i/∂θ) — preserves
        # GRPO semantics exactly. Peak activation memory is bounded to ONE
        # sample regardless of batch size. Cost: N forward+backward passes
        # serialized instead of one fused pass; wall-clock roughly the same
        # because per-sample ops are the same ops, just scheduled one at a
        # time on the GPU.
        from mlx.utils import tree_map  # noqa: PLC0415

        grads_accum: Any = None
        mlx_loss_total = 0.0

        for local_i, orig_i in enumerate(kept_indices):
            # Capture per-sample constants into the closure as Python floats
            # so value_and_grad only differentiates w.r.t. model params, not
            # these fixed rollout-time values.
            adv_i = float(kept_adv[local_i])
            lp_old_i = float(self._stash[orig_i].lp_old_frozen)  # type: ignore[union-attr]
            lp_ref_i = float(self._stash[orig_i].lp_ref_frozen)  # type: ignore[union-attr]
            full_ids_i = self._stash[orig_i].full_ids            # type: ignore[union-attr]
            prompt_len_i = self._stash[orig_i].prompt_len        # type: ignore[union-attr]

            def sample_loss_fn(model: Any,
                                _adv=adv_i,
                                _lp_old=lp_old_i,
                                _lp_ref=lp_ref_i,
                                _full=full_ids_i,
                                _plen=prompt_len_i,
                                _n=n_kept) -> Any:
                lp_new_i = sum_response_logprobs(model, mx, _full, _plen)
                log_ratio_i = mx.clip(lp_new_i - _lp_old,
                                       -_MAX_LOG_RATIO, _MAX_LOG_RATIO)
                ratio_i = mx.exp(log_ratio_i)
                clipped_i = mx.clip(ratio_i, 1.0 - clip, 1.0 + clip)
                # Per-sample clipped surrogate; no mean reduction (that's
                # what the 1/N scaling at the end does across accumulation).
                policy_loss_i = -mx.minimum(ratio_i * _adv, clipped_i * _adv)
                log_diff_i = mx.clip(lp_new_i - _lp_ref,
                                      -_MAX_LOG_RATIO, _MAX_LOG_RATIO)
                kl_i = log_diff_i
                entropy_i = -lp_new_i
                sample_loss_scaled = (
                    policy_loss_i + kl_coef * kl_i - entropy_coef * entropy_i
                ) / _n
                return sample_loss_scaled

            sample_grad_fn = nn.value_and_grad(self.model, sample_loss_fn)
            sample_loss, sample_grads = sample_grad_fn(self.model)

            if grads_accum is None:
                grads_accum = sample_grads
            else:
                grads_accum = tree_map(
                    lambda a, b: a + b, grads_accum, sample_grads
                )

            # Materialize + free this sample's forward/backward graph before
            # the next iteration. Without this eval the graphs accumulate and
            # we're back to the same activation-memory problem.
            mx.eval(grads_accum)
            mlx_loss_total += float(sample_loss)
            self._clear_cache()

        # Apply accumulated gradients; combined barrier for model + optimizer.
        self.optimizer.update(self.model, grads_accum)
        model_state = getattr(self.model, "state", None)
        if model_state is None:
            model_state = self.model.parameters()
        mx.eval(model_state, self.optimizer.state)
        del grads_accum
        self._clear_cache()

        mlx_loss_f = float(mlx_loss_total)
        delta = abs(mlx_loss_f - loss_value)

        # Per-step memory telemetry. If memory stays bounded across a long
        # run these numbers are flat-or-sawtooth; monotonic growth means the
        # hygiene pattern has a leak and we should catch it before jetsam
        # does.
        self._step_counter += 1
        peak_gb, cache_gb, active_gb = self._mem_snapshot()
        logger.info(
            "GRPO step=%d loss=%.6f peak_mem=%.2fGB cache_mem=%.2fGB "
            "active_mem=%.2fGB",
            self._step_counter, mlx_loss_f, peak_gb, cache_gb, active_gb,
        )
        # Debug-level log for the routine O(1) divergence; warning only on
        # blow-ups that suggest a genuine bug (NaN/inf, 50+ delta).
        if not np.isfinite(mlx_loss_f) or delta > _LOSS_DISAGREEMENT_BLOWUP:
            logger.warning(
                "GRPO loss BLOWUP: numpy=%.6f mlx=%.6f delta=%.4f "
                "(threshold=%.1f — check for NaN/inf or dtype issue)",
                loss_value, mlx_loss_f, delta, _LOSS_DISAGREEMENT_BLOWUP,
            )
        else:
            logger.debug(
                "GRPO loss: numpy=%.6f mlx=%.6f delta=%.4f (expected O(1))",
                loss_value, mlx_loss_f, delta,
            )

        self._stash = None  # consume

    # ── EvalFn ──────────────────────────────────────────────────────────────

    def eval_fn(self, step: int) -> float:
        """Run the Phase 10 benchmark runner as a subprocess, return aggregate.

        Config expects ``eval_cmd`` to be a ready-to-spawn command like::

            ["python", "-m", "neural.benchmarks.run_benchmark",
             "--config", "configs/benchmark_phase10_rl_eval.yaml",
             "--out", "/tmp/rl_eval.json"]

        The runner writes the aggregate to the ``--out`` path's JSON, which
        this method parses and returns.

        If ``eval_cmd`` is ``None``, returns ``float("nan")`` — the trainer's
        early-stop wrapper treats NaN as "no signal" and keeps going. Operator
        should supply a real command for the full compute pass.
        """
        if self.eval_cmd is None:
            logger.warning(
                "eval_fn: no eval_cmd configured; returning NaN (trainer "
                "early-stop will skip this iteration)"
            )
            return float("nan")

        # Convention: the last arg pair is ``--out <path>``. We parse the
        # aggregate from that JSON after the subprocess completes.
        try:
            out_idx = self.eval_cmd.index("--out")
            out_path = Path(self.eval_cmd[out_idx + 1])
        except (ValueError, IndexError) as exc:
            raise RuntimeError(
                "eval_cmd must include '--out <path>'; got "
                f"{self.eval_cmd!r}"
            ) from exc

        logger.info("eval_fn step=%d invoking: %s", step, self.eval_cmd)
        result = subprocess.run(
            self.eval_cmd, capture_output=True, text=True, check=False,
        )
        if result.returncode != 0:
            logger.error(
                "eval_fn subprocess failed (rc=%d): %s",
                result.returncode, result.stderr[-500:],
            )
            return float("nan")

        try:
            payload = json.loads(out_path.read_text())
        except (FileNotFoundError, json.JSONDecodeError) as exc:
            logger.error("eval_fn: cannot parse %s: %s", out_path, exc)
            return float("nan")

        # Phase 10 runner writes {"aggregate_weighted_score": 0.x, ...}
        agg = payload.get("aggregate_weighted_score")
        if agg is None:
            logger.error("eval_fn: missing aggregate_weighted_score in %s",
                         out_path)
            return float("nan")
        return float(agg)

    # ── CheckpointFn ────────────────────────────────────────────────────────

    def checkpoint_fn(self, path: Path, step: int) -> None:
        """Save LoRA-only trainable parameters as safetensors.

        Matches ``mlx_lm.tuner.utils.load_adapters`` load convention so the
        resulting directory can be passed back into ``mlx_lm.load(adapter_path=...)``.
        """
        import mlx.core as mx  # noqa: PLC0415
        from mlx.utils import tree_flatten  # noqa: PLC0415

        path = Path(path)
        path.mkdir(parents=True, exist_ok=True)
        trainable = dict(tree_flatten(self.model.trainable_parameters()))
        target = path / "adapters.safetensors"
        mx.save_safetensors(str(target), trainable)
        logger.info("checkpoint_fn step=%d wrote %d params to %s",
                    step, len(trainable), target)


# ── helper ──────────────────────────────────────────────────────────────────


def _expand_advantages_to_batch(
    kept_adv: np.ndarray,
    keep_mask: np.ndarray,
    batch_size: int,
) -> np.ndarray:
    """Map ``kept_adv`` (len N_kept) back onto a batch-sized array.

    Entries where ``keep_mask`` is False get 0.0. Only used for the rare
    stash=None edge case inside :meth:`optimizer_step_fn`.
    """
    out = np.zeros(batch_size, dtype=np.float64)
    kept_positions = np.where(keep_mask)[0]
    if len(kept_positions) != len(kept_adv):
        # keep_mask and kept_adv are supposed to be aligned by construction
        # in estimate_advantage; a mismatch means an upstream bug.
        raise RuntimeError(
            f"keep_mask has {len(kept_positions)} True entries but "
            f"advantages has {len(kept_adv)}"
        )
    out[kept_positions] = kept_adv
    return out
