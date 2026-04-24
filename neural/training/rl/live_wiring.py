"""Live-MLX wiring for Phase 11 GRPO training.

Bridges :class:`~neural.training.rl.mlx_adapter.MLXGRPOAdapter` injection
seams (``prompt_provider``, ``reward_fn``, ``eval_cmd``) to production
artifacts:

* **Prompts** come from the Phase 10 golden holdout
  (``training_data/eval/valid_golden.jsonl``). Each row holds a chat-style
  ``messages`` list (system + user + assistant); we slice off the assistant
  turn, apply the model's chat template, and hand the resulting string to
  ``mlx_lm.generate``.
* **Rewards** come from ``neural.training.reward_functions.compute_reward``
  using the ULTS spec's ``reward_functions`` array + ``output_schema``,
  with ``target``/``expected`` populated from the matched golden row's
  assistant message. The per-metric dict is scalarized via
  :func:`~neural.training.rl.reward_sampler.scalarize_reward`.
* **Eval** is the Phase 10 benchmark CLI, registry-only (no judge per
  step). Judge spend is reserved for the dual regression gate.

Why this is a separate module: MLX adapter is pure mechanics (tensors,
backprop); this module is the Phase 10 bridge (specs, golden rows, reward
registry). Keeping them apart lets the adapter unit tests stay MLX-only
and lets this module's unit tests stay Phase-10-only.

Tokenizer is injected *after* construction because the MLX adapter owns
the ``mlx_lm.load()`` call that produces it. See ``main()`` in
``trainer.py`` for the two-step init pattern.
"""
from __future__ import annotations

import json
import logging
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Callable

from neural.training.reward_functions import compute_reward

from .reward_sampler import RewardSample, scalarize_reward

logger = logging.getLogger(__name__)


# ── ULTS spec loader (lightweight; avoids importing run_benchmark.Spec) ─────


@dataclass(frozen=True)
class _SpecSummary:
    """Only the fields the reward path needs — keeps this module import-light."""
    task_name: str
    reward_functions: list[str]
    output_schema: dict[str, Any] | None


def _load_specs(specs_dir: Path) -> dict[str, _SpecSummary]:
    """Index ULTS specs by ``task.name`` for O(1) lookup at reward time."""
    out: dict[str, _SpecSummary] = {}
    for p in sorted(specs_dir.glob("*.ults.json")):
        raw = json.loads(p.read_text())
        name = (raw.get("task") or {}).get("name")
        if not name:
            logger.warning("spec %s has no task.name; skipping", p)
            continue
        out[name] = _SpecSummary(
            task_name=name,
            reward_functions=list(raw.get("reward_functions") or []),
            output_schema=raw.get("output_schema"),
        )
    return out


# ── Golden row loader ───────────────────────────────────────────────────────


def _load_golden_rows(golden_path: Path) -> dict[str, list[dict[str, Any]]]:
    """Group golden JSONL rows by ``meta.task_name``."""
    by_task: dict[str, list[dict[str, Any]]] = {}
    with golden_path.open() as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            row = json.loads(line)
            t = (row.get("meta") or {}).get("task_name")
            if not t:
                continue  # rows without task_name are unusable here
            by_task.setdefault(t, []).append(row)
    return by_task


def _extract_user_messages(row: dict[str, Any]) -> list[dict[str, str]]:
    """Pull system + user turns out of a golden row's messages list.

    Assistant turns are dropped (they're the training target, not part of
    the rollout prompt). ``tool``/``function`` roles are dropped — MDEMG
    is no-tool-calling per architectural policy.
    """
    msgs = row.get("messages") or []
    keep = [m for m in msgs if m.get("role") in ("system", "user")]
    # Normalize: defensive coerce to {role, content} only.
    return [
        {"role": m["role"], "content": m.get("content") or ""} for m in keep
    ]


def _extract_assistant_target(row: dict[str, Any]) -> str:
    """Assistant message content — used as the reward function's `target`."""
    for m in row.get("messages") or []:
        if m.get("role") == "assistant":
            return m.get("content") or ""
    return ""


# ── Prompt provider ─────────────────────────────────────────────────────────


class ChatTemplatedPromptProvider:
    """Callable ``(task_id, run_idx) -> str``.

    Picks a golden row by ``run_idx % len(rows_for_task)`` (stable,
    reproducible) and renders its system+user messages through the
    model's chat template. The rendered string is what ``mlx_lm.generate``
    consumes.

    The tokenizer must be assigned via :attr:`tokenizer` before the first
    call. :meth:`attach_tokenizer` is a convenience alias that also
    verifies the tokenizer exposes ``apply_chat_template``.
    """

    def __init__(self, golden_path: Path) -> None:
        self._rows_by_task = _load_golden_rows(golden_path)
        if not self._rows_by_task:
            raise ValueError(f"no usable golden rows in {golden_path}")
        self.tokenizer: Any = None
        logger.info(
            "ChatTemplatedPromptProvider loaded: tasks=%d total_rows=%d",
            len(self._rows_by_task),
            sum(len(v) for v in self._rows_by_task.values()),
        )

    def attach_tokenizer(self, tokenizer: Any) -> None:
        """Assign the tokenizer and verify ``apply_chat_template`` is present."""
        if not hasattr(tokenizer, "apply_chat_template"):
            raise TypeError(
                "tokenizer must expose apply_chat_template(...) — got "
                f"{type(tokenizer).__name__}"
            )
        self.tokenizer = tokenizer

    def available_tasks(self) -> list[str]:
        return sorted(self._rows_by_task.keys())

    def rows_for_task(self, task_id: str) -> list[dict[str, Any]]:
        return list(self._rows_by_task.get(task_id, []))

    def __call__(self, task_id: str, run_idx: int) -> str:
        if self.tokenizer is None:
            raise RuntimeError(
                "ChatTemplatedPromptProvider.tokenizer not attached; "
                "call attach_tokenizer(adapter.tokenizer) after "
                "MLXGRPOAdapter construction"
            )
        rows = self._rows_by_task.get(task_id)
        if not rows:
            raise KeyError(
                f"no golden rows for task_id={task_id!r}; "
                f"available: {self.available_tasks()}"
            )
        row = rows[run_idx % len(rows)]
        messages = _extract_user_messages(row)
        if not messages:
            raise ValueError(
                f"golden row for task_id={task_id!r} run_idx={run_idx} "
                "has no system/user messages"
            )
        return self.tokenizer.apply_chat_template(
            messages, tokenize=False, add_generation_prompt=True
        )


# ── Reward function ─────────────────────────────────────────────────────────


class SpecDrivenRewardFn:
    """Callable ``(RewardSample, response) -> float``.

    Looks up the ULTS spec by ``sample.task_id``, grabs ``reward_functions``
    + ``output_schema``, pulls ``target``/``expected`` from the matched
    golden row, calls :func:`compute_reward`, and scalarizes.

    Same golden rows as the prompt provider — the pairing keeps
    ``(task_id, run_idx)`` deterministic between rollout and scoring.
    """

    def __init__(
        self,
        specs_dir: Path,
        golden_path: Path,
    ) -> None:
        self._specs = _load_specs(specs_dir)
        if not self._specs:
            raise ValueError(f"no ULTS specs in {specs_dir}")
        self._rows_by_task = _load_golden_rows(golden_path)
        logger.info(
            "SpecDrivenRewardFn loaded: specs=%d tasks_with_golden=%d",
            len(self._specs),
            len(self._rows_by_task),
        )

    def available_tasks(self) -> list[str]:
        """Tasks that have both a spec AND at least one golden row."""
        return sorted(set(self._specs) & set(self._rows_by_task))

    def __call__(self, sample: RewardSample, response: str) -> float:
        spec = self._specs.get(sample.task_id)
        if spec is None or not spec.reward_functions:
            # No reward signal for this task; scalar 0 keeps the pipeline
            # running without crashing (log once per task via `warning`).
            logger.warning(
                "no reward functions for task_id=%s; returning 0.0",
                sample.task_id,
            )
            return 0.0
        rows = self._rows_by_task.get(sample.task_id) or []
        target = ""
        if rows:
            row = rows[sample.run_idx % len(rows)]
            target = _extract_assistant_target(row)
        kwargs: dict[str, Any] = {}
        if spec.output_schema is not None:
            kwargs["schema"] = spec.output_schema
        if target:
            kwargs["target"] = target
            kwargs["expected"] = target
        per_metric = compute_reward(response, spec.reward_functions, **kwargs)
        return scalarize_reward(per_metric)


# ── Eval command builder ────────────────────────────────────────────────────


def build_registry_eval_cmd(
    *,
    model_path: str,
    benchmark_config: Path,
    out_path_template: str = "/tmp/phase11_eval_step_{step}.json",
) -> list[str]:
    """Return the argv for :meth:`MLXGRPOAdapter.eval_fn`.

    NOTE: The Phase 10 benchmark CLI talks to a running ``mlx_lm.server``
    instance at ``mlx_base_url`` — it does NOT load the model itself. So
    using this subprocess path for in-training eval evaluates the server's
    *pre-training* weights, not the adapter's current (training) weights.

    For faithful in-training eval use :class:`InProcessEvaluator` instead.
    This subprocess builder is kept for: (a) post-training regression, and
    (b) stale-signal smoke runs where the point is to exercise the eval
    plumbing, not the actual reward trajectory.

    Registry-only (no judge) — judge spend is reserved for the dual
    regression gate at end of training.
    """
    return [
        "python",
        "-m",
        "neural.benchmarks.run_benchmark",
        "--config",
        str(benchmark_config),
        "--model-path",
        model_path,
        "--out",
        out_path_template,
    ]


# ── In-process evaluator ────────────────────────────────────────────────────


class InProcessEvaluator:
    """Faithful in-training eval: uses the adapter's live ``model`` directly.

    Runs ``n_samples_per_task`` rollouts per task (across ``tasks``), scores
    with ``reward_fn``, averages to a val_reward scalar. Matches the
    :class:`EvalFn` protocol ``(step: int) -> float``.

    This bypasses the subprocess + MLX-server path (which would evaluate
    stale weights) and uses the in-memory ``mlx_lm.generate`` against the
    adapter's current policy model.

    Cost: ``len(tasks) * n_samples_per_task`` generations per eval. At
    ``eval_interval_steps=100``, with 16 tasks × 2 samples = 32 gens every
    100 steps — cheap relative to training step time.
    """

    def __init__(
        self,
        *,
        adapter: Any,               # MLXGRPOAdapter — runtime-resolved
        prompt_provider: ChatTemplatedPromptProvider,
        reward_fn: SpecDrivenRewardFn,
        tasks: list[str],
        n_samples_per_task: int = 2,
        generation_kwargs: dict[str, Any] | None = None,
    ) -> None:
        if n_samples_per_task <= 0:
            raise ValueError(
                f"n_samples_per_task must be positive; got {n_samples_per_task}"
            )
        self.adapter = adapter
        self.prompt_provider = prompt_provider
        self.reward_fn = reward_fn
        self.tasks = list(tasks)
        self.n_samples_per_task = int(n_samples_per_task)
        self.generation_kwargs = dict(generation_kwargs or {})

    def __call__(self, step: int) -> float:
        # Deferred import: keeps unit tests importable in envs without MLX.
        import mlx_lm  # noqa: PLC0415

        all_scores: list[float] = []
        for task_id in self.tasks:
            for run_idx in range(self.n_samples_per_task):
                try:
                    prompt = self.prompt_provider(task_id, run_idx)
                except Exception as exc:
                    logger.warning(
                        "eval: prompt_provider failed for task=%s run_idx=%s: %s",
                        task_id, run_idx, exc,
                    )
                    continue
                try:
                    response = mlx_lm.generate(
                        self.adapter.model, self.adapter.tokenizer, prompt,
                        **self.generation_kwargs,
                    )
                except Exception as exc:
                    logger.warning(
                        "eval: generate failed for task=%s run_idx=%s: %s",
                        task_id, run_idx, exc,
                    )
                    continue
                # Build a minimal RewardSample for the reward_fn signature.
                from .reward_sampler import RewardSample as _RS  # local import
                sample = _RS(
                    task_id=task_id, run_idx=run_idx,
                    sampling_group="C",   # unused by reward_fn
                    response_text=response, reward_vector={},
                    scalar_reward=0.0,
                )
                all_scores.append(self.reward_fn(sample, response))

        if not all_scores:
            logger.warning("eval step=%d: no successful rollouts; val_reward=0", step)
            return 0.0
        val = sum(all_scores) / len(all_scores)
        logger.info(
            "eval step=%d: val_reward=%.4f (%d rollouts across %d tasks)",
            step, val, len(all_scores), len(self.tasks),
        )
        return val


# ── Factory: assemble both providers in one call ────────────────────────────


@dataclass
class LiveCallSite:
    """Bundle of the two callables the MLX adapter needs.

    Usage::

        site = build_live_call_site(
            golden_path=Path("training_data/eval/valid_golden.jsonl"),
            specs_dir=Path("docs/tests/ults/specs"),
        )
        adapter = MLXGRPOAdapter(
            prompt_provider=site.prompt_provider,
            reward_fn=site.reward_fn,
            ...,
        )
        site.prompt_provider.attach_tokenizer(adapter.tokenizer)
    """
    prompt_provider: ChatTemplatedPromptProvider
    reward_fn: SpecDrivenRewardFn


def build_live_call_site(
    *,
    golden_path: Path,
    specs_dir: Path,
) -> LiveCallSite:
    return LiveCallSite(
        prompt_provider=ChatTemplatedPromptProvider(golden_path),
        reward_fn=SpecDrivenRewardFn(specs_dir, golden_path),
    )
