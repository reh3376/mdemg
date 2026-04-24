"""Unit tests for live_wiring — ChatTemplatedPromptProvider + SpecDrivenRewardFn."""
from __future__ import annotations

import json
from pathlib import Path

import pytest

from neural.training.rl.live_wiring import (
    ChatTemplatedPromptProvider,
    LiveCallSite,
    SpecDrivenRewardFn,
    build_live_call_site,
    build_registry_eval_cmd,
)
from neural.training.rl.reward_sampler import RewardSample


# ── helpers ─────────────────────────────────────────────────────────────────


class _FakeTokenizer:
    """Mimics HF tokenizer.apply_chat_template() shape."""

    def __init__(self) -> None:
        self.calls: list[list[dict[str, str]]] = []

    def apply_chat_template(
        self,
        messages: list[dict[str, str]],
        tokenize: bool = False,
        add_generation_prompt: bool = True,
    ) -> str:
        self.calls.append(list(messages))
        # Stable, checkable concatenation.
        out = "".join(f"<|{m['role']}|>{m['content']}" for m in messages)
        if add_generation_prompt:
            out += "<|assistant|>"
        return out


def _write_golden(path: Path, rows: list[dict]) -> None:
    path.write_text("\n".join(json.dumps(r) for r in rows) + "\n")


def _write_spec(path: Path, task_name: str, reward_fns: list[str],
                schema: dict | None = None) -> None:
    spec = {
        "task": {"name": task_name, "version": "1.0.0"},
        "sampling_group": "C",
        "reward_functions": reward_fns,
        "output_schema": schema or {"type": "object"},
    }
    path.write_text(json.dumps(spec))


# ── ChatTemplatedPromptProvider ─────────────────────────────────────────────


def test_prompt_provider_happy_path(tmp_path: Path) -> None:
    golden = tmp_path / "g.jsonl"
    _write_golden(golden, [
        {
            "messages": [
                {"role": "system", "content": "SYS-A"},
                {"role": "user", "content": "USR-A"},
                {"role": "assistant", "content": "AST-A"},
            ],
            "meta": {"task_name": "alpha.task"},
        },
        {
            "messages": [
                {"role": "system", "content": "SYS-B"},
                {"role": "user", "content": "USR-B"},
                {"role": "assistant", "content": "AST-B"},
            ],
            "meta": {"task_name": "alpha.task"},
        },
    ])
    tok = _FakeTokenizer()
    pp = ChatTemplatedPromptProvider(golden)
    pp.attach_tokenizer(tok)

    rendered0 = pp("alpha.task", 0)
    rendered1 = pp("alpha.task", 1)
    rendered_wrap = pp("alpha.task", 2)   # wraps to row 0

    assert "SYS-A" in rendered0 and "USR-A" in rendered0
    assert "AST-A" not in rendered0        # assistant is dropped
    assert rendered0.endswith("<|assistant|>")  # gen-prompt suffix
    assert "SYS-B" in rendered1
    assert rendered_wrap == rendered0      # cyclic wrap on run_idx


def test_prompt_provider_missing_tokenizer_raises(tmp_path: Path) -> None:
    golden = tmp_path / "g.jsonl"
    _write_golden(golden, [
        {
            "messages": [
                {"role": "system", "content": "S"},
                {"role": "user", "content": "U"},
            ],
            "meta": {"task_name": "t"},
        }
    ])
    pp = ChatTemplatedPromptProvider(golden)
    with pytest.raises(RuntimeError, match="tokenizer not attached"):
        pp("t", 0)


def test_prompt_provider_unknown_task_raises(tmp_path: Path) -> None:
    golden = tmp_path / "g.jsonl"
    _write_golden(golden, [
        {
            "messages": [{"role": "system", "content": "S"},
                         {"role": "user", "content": "U"}],
            "meta": {"task_name": "known.task"},
        }
    ])
    pp = ChatTemplatedPromptProvider(golden)
    pp.attach_tokenizer(_FakeTokenizer())
    with pytest.raises(KeyError, match="unknown.task"):
        pp("unknown.task", 0)


def test_prompt_provider_empty_golden_raises(tmp_path: Path) -> None:
    golden = tmp_path / "g.jsonl"
    golden.write_text("")
    with pytest.raises(ValueError, match="no usable golden rows"):
        ChatTemplatedPromptProvider(golden)


def test_prompt_provider_attach_rejects_wrong_type(tmp_path: Path) -> None:
    golden = tmp_path / "g.jsonl"
    _write_golden(golden, [
        {"messages": [{"role": "system", "content": "S"},
                       {"role": "user", "content": "U"}],
         "meta": {"task_name": "t"}}
    ])
    pp = ChatTemplatedPromptProvider(golden)
    with pytest.raises(TypeError, match="apply_chat_template"):
        pp.attach_tokenizer(object())


def test_prompt_provider_skips_rows_without_task_name(tmp_path: Path) -> None:
    golden = tmp_path / "g.jsonl"
    _write_golden(golden, [
        {"messages": [{"role": "system", "content": "X"}], "meta": {}},   # dropped
        {"messages": [{"role": "system", "content": "S"},
                       {"role": "user", "content": "U"}],
         "meta": {"task_name": "keep.task"}},
    ])
    pp = ChatTemplatedPromptProvider(golden)
    assert pp.available_tasks() == ["keep.task"]


# ── SpecDrivenRewardFn ──────────────────────────────────────────────────────


def test_reward_fn_scalarizes_registry_output(tmp_path: Path) -> None:
    specs_dir = tmp_path / "specs"
    specs_dir.mkdir()
    _write_spec(specs_dir / "alpha_task.ults.json", "alpha.task",
                ["json_valid"], {"type": "object"})

    golden = tmp_path / "g.jsonl"
    _write_golden(golden, [
        {
            "messages": [
                {"role": "system", "content": "S"},
                {"role": "user", "content": "U"},
                {"role": "assistant", "content": '{"key":"value"}'},
            ],
            "meta": {"task_name": "alpha.task"},
        }
    ])

    rfn = SpecDrivenRewardFn(specs_dir, golden)
    sample = RewardSample(
        task_id="alpha.task", run_idx=0, sampling_group="C",
        response_text="", reward_vector={}, scalar_reward=0.0,
    )
    # Valid JSON object: json_valid returns 1.0.
    good = rfn(sample, '{"a": 1}')
    assert good == pytest.approx(1.0)
    # Invalid JSON object: json_valid returns 0.0.
    bad = rfn(sample, "not json")
    assert bad == pytest.approx(0.0)


def test_reward_fn_missing_spec_returns_zero(tmp_path: Path) -> None:
    specs_dir = tmp_path / "specs"
    specs_dir.mkdir()
    _write_spec(specs_dir / "other.ults.json", "other.task", ["json_valid"])

    golden = tmp_path / "g.jsonl"
    _write_golden(golden, [
        {"messages": [{"role": "system", "content": "S"}],
         "meta": {"task_name": "no.spec"}}
    ])
    rfn = SpecDrivenRewardFn(specs_dir, golden)
    sample = RewardSample(
        task_id="no.spec", run_idx=0, sampling_group="C",
        response_text="", reward_vector={}, scalar_reward=0.0,
    )
    assert rfn(sample, "whatever") == 0.0


def test_reward_fn_available_tasks_intersects(tmp_path: Path) -> None:
    specs_dir = tmp_path / "specs"
    specs_dir.mkdir()
    _write_spec(specs_dir / "alpha.ults.json", "alpha.task", ["json_valid"])
    _write_spec(specs_dir / "beta.ults.json", "beta.task", ["json_valid"])

    golden = tmp_path / "g.jsonl"
    _write_golden(golden, [
        {"messages": [{"role": "system", "content": "S"},
                       {"role": "user", "content": "U"}],
         "meta": {"task_name": "alpha.task"}},
        # gamma has a golden row but no spec — should be excluded
        {"messages": [{"role": "system", "content": "S"},
                       {"role": "user", "content": "U"}],
         "meta": {"task_name": "gamma.task"}},
    ])
    rfn = SpecDrivenRewardFn(specs_dir, golden)
    # alpha only: beta has a spec but no golden row; gamma has a row but no spec
    assert rfn.available_tasks() == ["alpha.task"]


# ── build_live_call_site + eval cmd ─────────────────────────────────────────


def test_build_live_call_site_returns_bundle(tmp_path: Path) -> None:
    specs_dir = tmp_path / "specs"
    specs_dir.mkdir()
    _write_spec(specs_dir / "a.ults.json", "alpha.task", ["json_valid"])

    golden = tmp_path / "g.jsonl"
    _write_golden(golden, [
        {"messages": [{"role": "system", "content": "S"},
                       {"role": "user", "content": "U"},
                       {"role": "assistant", "content": "A"}],
         "meta": {"task_name": "alpha.task"}}
    ])
    site: LiveCallSite = build_live_call_site(
        golden_path=golden, specs_dir=specs_dir
    )
    assert isinstance(site.prompt_provider, ChatTemplatedPromptProvider)
    assert isinstance(site.reward_fn, SpecDrivenRewardFn)


def test_build_registry_eval_cmd_no_judge_flag(tmp_path: Path) -> None:
    cmd = build_registry_eval_cmd(
        model_path="/path/to/model",
        benchmark_config=Path("configs/benchmark_phase10.yaml"),
    )
    assert "--enable-judge" not in cmd
    assert "--out" in cmd
    assert "--model-path" in cmd
    idx = cmd.index("--model-path")
    assert cmd[idx + 1] == "/path/to/model"


# ── InProcessEvaluator ──────────────────────────────────────────────────────


def test_in_process_evaluator_averages_reward_over_tasks(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    # Stub mlx_lm so the evaluator can import without Metal.
    import sys
    import types

    mlx_lm_stub = types.ModuleType("mlx_lm")
    mlx_lm_stub.generate = lambda model, tok, prompt, **kw: '{"a":1}'  # type: ignore[attr-defined]
    monkeypatch.setitem(sys.modules, "mlx_lm", mlx_lm_stub)

    specs_dir = tmp_path / "specs"
    specs_dir.mkdir()
    _write_spec(specs_dir / "a.ults.json", "alpha.task", ["json_valid"])
    _write_spec(specs_dir / "b.ults.json", "beta.task", ["json_valid"])

    golden = tmp_path / "g.jsonl"
    _write_golden(golden, [
        {"messages": [{"role": "system", "content": "S"},
                       {"role": "user", "content": "U"},
                       {"role": "assistant", "content": '{"a":1}'}],
         "meta": {"task_name": "alpha.task"}},
        {"messages": [{"role": "system", "content": "S"},
                       {"role": "user", "content": "U"},
                       {"role": "assistant", "content": '{"b":1}'}],
         "meta": {"task_name": "beta.task"}},
    ])

    from neural.training.rl.live_wiring import InProcessEvaluator

    site = build_live_call_site(golden_path=golden, specs_dir=specs_dir)
    tok = _FakeTokenizer()
    site.prompt_provider.attach_tokenizer(tok)

    # Stand-in adapter: just needs .model + .tokenizer attributes.
    class _FakeAdapter:
        model = object()
        tokenizer = tok

    evaluator = InProcessEvaluator(
        adapter=_FakeAdapter(),
        prompt_provider=site.prompt_provider,
        reward_fn=site.reward_fn,
        tasks=["alpha.task", "beta.task"],
        n_samples_per_task=2,
    )
    val = evaluator(step=100)
    # All rollouts return valid JSON → json_valid returns 1.0 for each.
    assert val == pytest.approx(1.0)


def test_in_process_evaluator_returns_zero_when_all_fail(
    tmp_path: Path, monkeypatch: pytest.MonkeyPatch
) -> None:
    import sys
    import types

    mlx_lm_stub = types.ModuleType("mlx_lm")

    def _boom(*a, **kw):
        raise RuntimeError("MLX offline")

    mlx_lm_stub.generate = _boom  # type: ignore[attr-defined]
    monkeypatch.setitem(sys.modules, "mlx_lm", mlx_lm_stub)

    specs_dir = tmp_path / "specs"
    specs_dir.mkdir()
    _write_spec(specs_dir / "a.ults.json", "alpha.task", ["json_valid"])

    golden = tmp_path / "g.jsonl"
    _write_golden(golden, [
        {"messages": [{"role": "system", "content": "S"},
                       {"role": "user", "content": "U"}],
         "meta": {"task_name": "alpha.task"}}
    ])

    from neural.training.rl.live_wiring import InProcessEvaluator

    site = build_live_call_site(golden_path=golden, specs_dir=specs_dir)
    tok = _FakeTokenizer()
    site.prompt_provider.attach_tokenizer(tok)

    class _FakeAdapter:
        model = object()
        tokenizer = tok

    evaluator = InProcessEvaluator(
        adapter=_FakeAdapter(),
        prompt_provider=site.prompt_provider,
        reward_fn=site.reward_fn,
        tasks=["alpha.task"],
        n_samples_per_task=1,
    )
    assert evaluator(step=50) == 0.0


def test_in_process_evaluator_validates_n_samples() -> None:
    from neural.training.rl.live_wiring import InProcessEvaluator

    class _FakeAdapter:
        model = object()
        tokenizer = object()

    with pytest.raises(ValueError, match="n_samples_per_task"):
        InProcessEvaluator(
            adapter=_FakeAdapter(),
            prompt_provider=None,  # type: ignore[arg-type]
            reward_fn=None,         # type: ignore[arg-type]
            tasks=[],
            n_samples_per_task=0,
        )
