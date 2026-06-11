"""Unit tests for MLXGRPOAdapter (Phase 11 MLX wiring).

Heavy mocking: the real adapter pulls mlx_lm + mlx.core + mlx.optimizers
+ mlx.nn on ``__init__``, which requires a working Metal backend and a real
Qwen3-14B checkpoint. CI machines have neither, so these tests patch every
MLX touchpoint with numpy / pure-Python stubs and exercise the pure control
flow (stash lifecycle, kept-sample masking, prompt rendering, subprocess
wrapper, checkpoint write).

A real-MLX smoke test lives in ``neural/training/rl/tests/test_mlx_adapter_smoke.py``
(to be added at Epic 6 e2e time, ``-m mlx`` marker).
"""
from __future__ import annotations

import json
import sys
import types
from unittest.mock import MagicMock, patch

import numpy as np
import pytest


@pytest.fixture
def stub_mlx(monkeypatch):
    """Install numpy-backed stubs for mlx.core + mlx.nn + mlx.optimizers +
    mlx.utils + mlx_lm.

    Returns a dict of the installed stubs so tests can assert call counts.
    """
    # ── mlx.core stub (numpy-backed array / reductions) ─────────────────────
    mx_stub = types.SimpleNamespace()

    def _mx_array(x, dtype=None):
        # Preserve int dtype for token-id arrays (so take_along_axis works),
        # float32 otherwise.
        if dtype is None:
            try:
                arr = np.asarray(x)
                if arr.dtype.kind in "iu":
                    return arr.astype(np.int64)
                return arr.astype(np.float32)
            except (TypeError, ValueError):
                return np.asarray(x, dtype=np.float32)
        return np.asarray(x, dtype=np.float32)
    mx_stub.array = _mx_array
    mx_stub.zeros = lambda shape: np.zeros(shape, dtype=np.float32)
    mx_stub.arange = lambda n: np.arange(n).astype(np.int64)
    mx_stub.exp = np.exp
    mx_stub.clip = np.clip
    mx_stub.mean = np.mean
    mx_stub.minimum = np.minimum
    mx_stub.stack = lambda xs: np.stack([np.asarray(x) for x in xs])
    mx_stub.take_along_axis = np.take_along_axis
    mx_stub.eval = MagicMock()
    mx_stub.save_safetensors = MagicMock()
    mx_stub.float32 = np.float32

    # ── Tier 1 memory-hygiene API stubs ─────────────────────────────────────
    # MLXGRPOAdapter calls these at init + per-step. Real mlx.core exposes
    # them on Metal builds; stubs just need to be callable and return plausible
    # values so the adapter logic keeps flowing.
    mx_stub.clear_cache = MagicMock()
    mx_stub.set_wired_limit = MagicMock()
    mx_stub.reset_peak_memory = MagicMock()
    mx_stub.device_info = MagicMock(
        return_value={"max_recommended_working_set_size": 64 * (1024 ** 3)}
    )
    mx_stub.get_peak_memory = MagicMock(return_value=1.0 * (1024 ** 3))
    mx_stub.get_cache_memory = MagicMock(return_value=0.5 * (1024 ** 3))
    mx_stub.get_active_memory = MagicMock(return_value=0.25 * (1024 ** 3))

    # ── mlx.nn stub ─────────────────────────────────────────────────────────
    nn_stub = types.SimpleNamespace()

    def log_softmax(x, axis=-1):
        x_max = np.max(x, axis=axis, keepdims=True)
        shifted = x - x_max
        return shifted - np.log(np.sum(np.exp(shifted), axis=axis, keepdims=True))
    nn_stub.log_softmax = log_softmax

    # value_and_grad returns a callable that returns (loss_value, grads_dict)
    def value_and_grad(model, loss_fn):
        def wrapped(m):
            loss = loss_fn(m)
            return loss, {"fake_grad": np.zeros(1)}
        return wrapped
    nn_stub.value_and_grad = value_and_grad

    # ── mlx.optimizers stub ─────────────────────────────────────────────────
    optim_stub = types.SimpleNamespace()
    adamw_calls = []

    class _StubAdamW:
        def __init__(self, **kwargs):
            adamw_calls.append(kwargs)
            self.state = {}

        def update(self, model, grads):
            self.state["last_grads"] = grads
    optim_stub.AdamW = _StubAdamW

    # ── mlx.utils stub (tree_flatten + tree_map) ────────────────────────────
    utils_stub = types.SimpleNamespace()
    utils_stub.tree_flatten = lambda params: list(
        (k, np.zeros(1)) for k in (params.keys() if isinstance(params, dict) else ["p0"])
    )

    # Tier 2 per-sample accumulation uses tree_map(lambda a, b: a + b, ...) to
    # accumulate per-sample grads. For stubs the grad tree is a flat dict
    # {"fake_grad": np.zeros(1)}, so the single-level walk below suffices.
    def _tree_map(fn, *trees):
        if not trees:
            return None
        first = trees[0]
        if isinstance(first, dict):
            return {k: fn(*(tr[k] for tr in trees)) for k in first}
        return fn(*trees)
    utils_stub.tree_map = _tree_map

    # ── mlx_lm stub (load + generate) ──────────────────────────────────────
    mlx_lm_stub = types.SimpleNamespace()
    load_calls = []

    def _stub_load(path, adapter_path=None, **_):
        load_calls.append({"path": path, "adapter_path": adapter_path})

        class _StubModel:
            """Callable acting as an mlx.nn.Module. Returns uniform logits
            so log_softmax produces finite values."""

            def __call__(self, inputs):
                # inputs shape: (1, L). Return (1, L, V=8).
                seq_len = inputs.shape[1]
                # Uniform small logits → log_softmax finite.
                return np.ones((1, seq_len, 8), dtype=np.float32)

            def freeze(self): pass

            def parameters(self): return {}

            def trainable_parameters(self): return {"lora.A": np.zeros(1)}

        model = _StubModel()
        model.freeze = MagicMock(wraps=model.freeze)
        model.parameters = MagicMock(wraps=model.parameters)
        model.trainable_parameters = MagicMock(wraps=model.trainable_parameters)

        tokenizer = MagicMock()
        # Simple deterministic tokenizer: split on spaces, int-hash each word.
        def _encode(s):
            return [abs(hash(w)) % 7 + 1 for w in s.split()][:50] or [1, 2]
        tokenizer.encode = _encode
        return model, tokenizer

    mlx_lm_stub.load = _stub_load
    # IMPORTANT: response has leading space so the split-tokenizer grows the
    # full sequence length past the prompt length (otherwise response_len=0
    # and the sample is stashed as None).
    mlx_lm_stub.generate = lambda model, tok, prompt, **_: " resp tok alpha beta"

    # Install stubs in sys.modules BEFORE import.
    monkeypatch.setitem(sys.modules, "mlx", types.ModuleType("mlx"))
    monkeypatch.setitem(sys.modules, "mlx.core", mx_stub)
    monkeypatch.setitem(sys.modules, "mlx.nn", nn_stub)
    monkeypatch.setitem(sys.modules, "mlx.optimizers", optim_stub)
    monkeypatch.setitem(sys.modules, "mlx.utils", utils_stub)
    monkeypatch.setitem(sys.modules, "mlx_lm", mlx_lm_stub)

    return {
        "mx": mx_stub,
        "nn": nn_stub,
        "optim": optim_stub,
        "mlx_lm": mlx_lm_stub,
        "adamw_calls": adamw_calls,
        "load_calls": load_calls,
    }


@pytest.fixture
def sample_rewardsample():
    from neural.training.rl.reward_sampler import RewardSample
    return RewardSample(
        task_id="task_T",
        run_idx=0,
        sampling_group="T",
        response_text="old_response",
        reward_vector={"f1": 0.8},
        scalar_reward=0.8,
    )


def _build_adapter(stub_mlx):
    """Helper — constructs an MLXGRPOAdapter against the stubs."""
    from neural.training.rl.mlx_adapter import MLXGRPOAdapter
    return MLXGRPOAdapter(
        base_model_path="/fake/base",
        adapter_path="/fake/adapter",
        prompt_provider=lambda task_id, run_idx: f"prompt_for:{task_id}",
        reward_fn=lambda sample, response: 0.75,
        clip_ratio=0.2, kl_coef=0.01, entropy_coef=0.0,
        lr=1e-5,
    )


# ── init / wiring ───────────────────────────────────────────────────────────


def test_init_loads_policy_only(stub_mlx):
    """One mlx_lm.load call — reference uses the same weights with LoRA scales
    zeroed in-forward (see :func:`_lora_disabled`). Second load was dropped
    to stay under Metal's MTLResource descriptor ceiling on Qwen3-14B."""
    adapter = _build_adapter(stub_mlx)
    # Exactly 1 load: policy; reference is the same model w/ LoRA disabled.
    assert len(stub_mlx["load_calls"]) == 1
    assert stub_mlx["load_calls"][0]["adapter_path"] == "/fake/adapter"
    # No ref_model attribute — removed with the single-load fix.
    assert not hasattr(adapter, "ref_model")
    # AdamW constructed with lr=1e-5
    assert stub_mlx["adamw_calls"][0]["learning_rate"] == 1e-5


# ── rollout_fn ──────────────────────────────────────────────────────────────


def test_rollout_fn_stashes_tensors(stub_mlx, sample_rewardsample):
    adapter = _build_adapter(stub_mlx)
    results = adapter.rollout_fn([sample_rewardsample, sample_rewardsample])
    assert len(results) == 2
    for r in results:
        assert r.task_id == "task_T"
        assert r.scalar_reward == 0.75
        assert "resp tok" in r.response_text
        # logprob_new should equal logprob_old at rollout time (on-policy)
        assert r.logprob_new == r.logprob_old
    # Two stashed entries aligned with the batch.
    assert adapter._stash is not None
    assert len(adapter._stash) == 2


def test_rollout_fn_handles_zero_length_response(stub_mlx, sample_rewardsample):
    """If prompt == full tokenization (empty response), stash is None."""
    adapter = _build_adapter(stub_mlx)
    # Monkey the tokenizer to return identical ids for prompt and prompt+resp.
    adapter.tokenizer.encode = lambda s: [1, 2, 3]
    results = adapter.rollout_fn([sample_rewardsample])
    assert len(results) == 1
    assert results[0].logprob_new == 0.0
    assert adapter._stash == [None]


# ── optimizer_step_fn ───────────────────────────────────────────────────────


def test_optimizer_step_fn_requires_stash(stub_mlx):
    adapter = _build_adapter(stub_mlx)
    # No rollout_fn called first → error
    with pytest.raises(RuntimeError, match="before rollout_fn"):
        adapter.optimizer_step_fn(
            0.1,
            advantages=np.array([1.0]),
            keep_mask=np.array([True]),
            rollouts=[],
        )


def test_optimizer_step_fn_requires_context_kwargs(stub_mlx, sample_rewardsample):
    adapter = _build_adapter(stub_mlx)
    adapter.rollout_fn([sample_rewardsample])
    with pytest.raises(RuntimeError, match="requires advantages"):
        adapter.optimizer_step_fn(0.1)  # no kwargs


def test_optimizer_step_fn_updates_optimizer(stub_mlx, sample_rewardsample):
    adapter = _build_adapter(stub_mlx)
    adapter.rollout_fn([sample_rewardsample, sample_rewardsample])

    adapter.optimizer_step_fn(
        0.1,
        advantages=np.array([1.0, -0.5]),
        keep_mask=np.array([True, True]),
        rollouts=[],
    )
    # Stash consumed after step.
    assert adapter._stash is None
    # Optimizer.update recorded last grads.
    assert "last_grads" in adapter.optimizer.state


def test_optimizer_step_fn_skips_all_dropped(stub_mlx, sample_rewardsample, caplog):
    adapter = _build_adapter(stub_mlx)
    adapter.rollout_fn([sample_rewardsample])
    adapter.optimizer_step_fn(
        0.0,
        advantages=np.array([]),
        keep_mask=np.array([False]),
        rollouts=[],
    )
    assert adapter._stash is None  # still consumed


# ── eval_fn ─────────────────────────────────────────────────────────────────


def test_eval_fn_none_returns_nan(stub_mlx):
    adapter = _build_adapter(stub_mlx)
    result = adapter.eval_fn(step=10)
    assert np.isnan(result)


def test_eval_fn_with_cmd_parses_aggregate(stub_mlx, tmp_path):
    from neural.training.rl.mlx_adapter import MLXGRPOAdapter
    out_path = tmp_path / "out.json"
    out_path.write_text(json.dumps({"aggregate_weighted_score": 0.8505}))

    adapter = MLXGRPOAdapter(
        base_model_path="/fake", adapter_path=None,
        prompt_provider=lambda t, r: "p", reward_fn=lambda s, r: 0.0,
        clip_ratio=0.2, kl_coef=0.01, entropy_coef=0.0, lr=1e-5,
        eval_cmd=["echo", "--out", str(out_path)],
    )
    with patch("subprocess.run") as mock_run:
        mock_run.return_value = MagicMock(returncode=0, stdout="", stderr="")
        result = adapter.eval_fn(step=1)
    assert result == 0.8505


def test_eval_fn_subprocess_failure_returns_nan(stub_mlx, tmp_path):
    from neural.training.rl.mlx_adapter import MLXGRPOAdapter
    adapter = MLXGRPOAdapter(
        base_model_path="/fake", adapter_path=None,
        prompt_provider=lambda t, r: "p", reward_fn=lambda s, r: 0.0,
        clip_ratio=0.2, kl_coef=0.01, entropy_coef=0.0, lr=1e-5,
        eval_cmd=["false", "--out", str(tmp_path / "missing.json")],
    )
    with patch("subprocess.run") as mock_run:
        mock_run.return_value = MagicMock(returncode=1, stdout="", stderr="boom")
        result = adapter.eval_fn(step=1)
    assert np.isnan(result)


def test_eval_cmd_without_out_raises(stub_mlx):
    from neural.training.rl.mlx_adapter import MLXGRPOAdapter
    adapter = MLXGRPOAdapter(
        base_model_path="/fake", adapter_path=None,
        prompt_provider=lambda t, r: "p", reward_fn=lambda s, r: 0.0,
        clip_ratio=0.2, kl_coef=0.01, entropy_coef=0.0, lr=1e-5,
        eval_cmd=["python", "-m", "something"],
    )
    with pytest.raises(RuntimeError, match="--out"):
        adapter.eval_fn(step=1)


# ── checkpoint_fn ───────────────────────────────────────────────────────────


def test_checkpoint_fn_writes_safetensors(stub_mlx, tmp_path):
    adapter = _build_adapter(stub_mlx)
    target_dir = tmp_path / "ckpt"
    adapter.checkpoint_fn(target_dir, step=100)
    assert target_dir.exists()
    # save_safetensors was invoked with the expected path.
    stub_mlx["mx"].save_safetensors.assert_called_once()
    args, _ = stub_mlx["mx"].save_safetensors.call_args
    assert args[0].endswith("adapters.safetensors")


# ── sum_response_logprobs (pure function) ──────────────────────────────────


def test_sum_response_logprobs_returns_zero_for_empty(stub_mlx):
    from neural.training.rl.mlx_adapter import sum_response_logprobs
    model = MagicMock()
    result = sum_response_logprobs(model, stub_mlx["mx"], full_ids=[1],
                                    prompt_len=0)
    assert float(result) == 0.0


def test_sum_response_logprobs_masks_prompt_positions(stub_mlx):
    """When prompt_len >= len(full_ids), no response positions → 0.0."""
    from neural.training.rl.mlx_adapter import sum_response_logprobs
    model = MagicMock()
    result = sum_response_logprobs(model, stub_mlx["mx"], full_ids=[1, 2, 3],
                                    prompt_len=3)
    assert float(result) == 0.0


# ── _expand_advantages_to_batch ────────────────────────────────────────────


def test_expand_advantages_happy_path():
    from neural.training.rl.mlx_adapter import _expand_advantages_to_batch
    keep = np.array([True, False, True, True])
    adv = np.array([0.5, -0.3, 0.8])
    result = _expand_advantages_to_batch(adv, keep, batch_size=4)
    np.testing.assert_array_equal(result, [0.5, 0.0, -0.3, 0.8])


def test_expand_advantages_mismatch_raises():
    from neural.training.rl.mlx_adapter import _expand_advantages_to_batch
    keep = np.array([True, False, True])
    adv = np.array([0.5])  # length 1 but 2 kept
    with pytest.raises(RuntimeError, match="True entries"):
        _expand_advantages_to_batch(adv, keep, batch_size=3)


# ── _lora_disabled context manager ────────────────────────────────────────


def test_lora_disabled_is_noop_on_stub_model(stub_mlx):
    """With the stub mlx_lm (no tuner.lora submodule), _lora_disabled must
    degrade to a no-op rather than raising."""
    from neural.training.rl.mlx_adapter import _lora_disabled

    class _Bare:
        pass
    model = _Bare()
    with _lora_disabled(model):
        pass  # must not raise


def test_lora_disabled_toggles_scale_when_lora_types_available(monkeypatch):
    """When mlx_lm.tuner.lora IS importable, _lora_disabled must zero every
    LoRA module's scale inside the ``with`` block and restore on exit."""
    import sys
    import types

    # Fake LoRA classes matching mlx_lm's structure (just the duck-type that
    # _lora_disabled needs: an ``isinstance`` match plus ``.scale``).
    class _FakeLoRALinear:
        def __init__(self, scale):
            self.scale = scale

    class _FakeLoRASwitchLinear:
        def __init__(self, scale):
            self.scale = scale

    class _FakeLoRAEmbedding:
        def __init__(self, scale):
            self.scale = scale

    # Build a mlx_lm.tuner.lora stub. Must chain the package path so
    # `from mlx_lm.tuner.lora import ...` resolves.
    mlx_lm_mod = types.ModuleType("mlx_lm")
    tuner_mod = types.ModuleType("mlx_lm.tuner")
    lora_mod = types.ModuleType("mlx_lm.tuner.lora")
    lora_mod.LoRALinear = _FakeLoRALinear
    lora_mod.LoRASwitchLinear = _FakeLoRASwitchLinear
    lora_mod.LoRAEmbedding = _FakeLoRAEmbedding
    mlx_lm_mod.tuner = tuner_mod
    tuner_mod.lora = lora_mod
    monkeypatch.setitem(sys.modules, "mlx_lm", mlx_lm_mod)
    monkeypatch.setitem(sys.modules, "mlx_lm.tuner", tuner_mod)
    monkeypatch.setitem(sys.modules, "mlx_lm.tuner.lora", lora_mod)

    # Re-import to pick up the freshly-stubbed import inside _lora_disabled.
    from neural.training.rl.mlx_adapter import _lora_disabled

    lora_a = _FakeLoRALinear(scale=20.0)
    lora_b = _FakeLoRAEmbedding(scale=15.0)
    non_lora = object()  # must be ignored

    class _Model:
        def named_modules(self):
            yield "layers.0.q_proj", lora_a
            yield "layers.0.embed", lora_b
            yield "layers.0.ln", non_lora

    model = _Model()

    # Inside the block, scales are 0.
    with _lora_disabled(model):
        assert lora_a.scale == 0.0
        assert lora_b.scale == 0.0
    # After exit, scales are restored.
    assert lora_a.scale == 20.0
    assert lora_b.scale == 15.0


def test_lora_disabled_restores_scale_on_exception(monkeypatch):
    """Exception inside the ``with`` block must still restore scales."""
    import sys
    import types

    class _FakeLoRALinear:
        def __init__(self, scale):
            self.scale = scale

    mlx_lm_mod = types.ModuleType("mlx_lm")
    tuner_mod = types.ModuleType("mlx_lm.tuner")
    lora_mod = types.ModuleType("mlx_lm.tuner.lora")
    lora_mod.LoRALinear = _FakeLoRALinear
    # Provide the other two names so the import doesn't fail.
    lora_mod.LoRASwitchLinear = type("S", (), {})
    lora_mod.LoRAEmbedding = type("E", (), {})
    mlx_lm_mod.tuner = tuner_mod
    tuner_mod.lora = lora_mod
    monkeypatch.setitem(sys.modules, "mlx_lm", mlx_lm_mod)
    monkeypatch.setitem(sys.modules, "mlx_lm.tuner", tuner_mod)
    monkeypatch.setitem(sys.modules, "mlx_lm.tuner.lora", lora_mod)

    from neural.training.rl.mlx_adapter import _lora_disabled

    lora = _FakeLoRALinear(scale=42.0)

    class _Model:
        def named_modules(self):
            yield "q", lora

    model = _Model()
    with pytest.raises(ValueError, match="boom"):
        with _lora_disabled(model):
            assert lora.scale == 0.0
            raise ValueError("boom")
    # Restored despite the raise.
    assert lora.scale == 42.0
