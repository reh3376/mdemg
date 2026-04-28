"""GRPO trainer — step loop + eval + early-stop + TSDB persistence.

Framework-light orchestrator. The MLX-specific work (adapter load, rollout,
logprob computation, backprop) is behind four injectable callables so the
integration test can exercise the full step loop with mocks, and a real
run swaps in an ``mlx_adapter.py`` implementation.

Step loop:
    for step in range(max_steps):
        batch  = reward_sampler.sample_batch(N)
        rollout = rollout_fn(batch)                # list[RolloutResult]
        logp_new, logp_old, logp_ref = logprob_fn(rollout)
        advantages = estimate_advantage(rewards, task_ids, stddev_cache, ...)
        loss = compute_grpo_loss(logp_new, logp_old, advantages, logp_ref, ...)
        optimizer_step_fn(loss.total)
        persist step row
        if step % eval_interval == 0: run eval, check early-stop

All run/step rows are written as an SQL sidecar (Phase 11 MVP — matches Phase 10
pattern); live psycopg writer is a simple swap in ``_PersistenceAdapter``.

Early stop: delegates to ``neural.training.early_stop.EarlyStopMonitor`` in
``mode="rl"`` with policy ``val_reward < best × 0.95`` for 2 consecutive evals
(MEMORY feedback).
"""
from __future__ import annotations

import json
import logging
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Callable, Protocol

import numpy as np

from neural.benchmarks._ids import new_run_id
from neural.training.early_stop import EarlyStopMonitor

from .advantage import AdvantageEstimate, estimate_advantage
from .grpo_loss import GRPOLossResult, compute_grpo_loss
from .reward_sampler import RewardSample, RewardSampler

logger = logging.getLogger(__name__)


# ── injectable contract types ───────────────────────────────────────────────


@dataclass(frozen=True)
class RolloutResult:
    """One rollout output from the MLX adapter — matches a RewardSample 1:1."""
    task_id: str
    response_text: str
    scalar_reward: float
    # Sum-of-token logprobs under (current, old=rollout-time, reference) policies.
    logprob_new: float
    logprob_old: float
    logprob_ref: float


class RolloutFn(Protocol):
    """Callable: given a batch of RewardSample, returns RolloutResult per sample.

    Real implementation wraps MLX :8101 inference + reward registry scoring.
    Mock implementation returns pre-baked values for the integration test.
    """
    def __call__(self, batch: list[RewardSample]) -> list[RolloutResult]: ...


class OptimizerStepFn(Protocol):
    """Callable: given scalar loss + step context, performs one optimizer step.

    Real implementation (``MLXGRPOAdapter``) rebuilds the GRPO loss in MLX space
    using ``advantages`` + ``keep_mask`` + the tokenized tensors it stashed during
    ``rollout_fn``, then runs ``mx.value_and_grad`` + ``optimizer.update``.

    The ``loss_value`` scalar is the numpy-computed loss from
    ``compute_grpo_loss`` — real adapters cross-check it against the MLX-space
    recomputation; mocks simply record it.

    Mocks accept the extra kwargs with ``**_`` to stay single-line.
    """
    def __call__(
        self,
        loss_value: float,
        *,
        advantages: "np.ndarray | None" = None,
        keep_mask: "np.ndarray | None" = None,
        rollouts: "list[RolloutResult] | None" = None,
    ) -> None: ...


class EvalFn(Protocol):
    """Callable: returns current val_reward on valid_golden subset.

    Real implementation invokes Phase 10 runner on a small holdout.
    Mock returns canned scalars from a scripted list.
    """
    def __call__(self, step: int) -> float: ...


class CheckpointFn(Protocol):
    """Callable: writes adapter state to ``path`` at current step."""
    def __call__(self, path: Path, step: int) -> None: ...


# ── persistence (SQL sidecar; live writer is a follow-up swap) ──────────────


def _q(value: Any) -> str:
    """SQL literal quoting (mirrors neural/benchmarks/persist.py)."""
    if value is None:
        return "NULL"
    if isinstance(value, bool):
        return "TRUE" if value else "FALSE"
    if isinstance(value, (int, float)):
        return repr(value)
    if isinstance(value, (dict, list)):
        return "'" + json.dumps(value, sort_keys=True).replace("'", "''") + "'::jsonb"
    return "'" + str(value).replace("'", "''") + "'"


class SqlSidecarPersistence:
    """Collects step + run INSERTs; flushes to a .sql file on close."""

    def __init__(self, sidecar_path: Path) -> None:
        self.sidecar_path = Path(sidecar_path)
        self._stmts: list[str] = []

    def write_run_start(self, run: dict[str, Any]) -> None:
        cols = [
            "run_id", "base_model_path", "base_model_sha",
            "adapter_manifest_sha", "output_path", "config_sha",
            "started_at", "phase10_baseline_run_id", "gate_verdict",
        ]
        vals = [run.get(c) for c in cols[:-1]] + ["pending"]
        self._stmts.append(
            f"INSERT INTO rl_training_runs ({', '.join(cols)}) "
            f"VALUES ({', '.join(_q(v) for v in vals)});"
        )

    def write_step(self, row: dict[str, Any]) -> None:
        cols = [
            "recorded_at", "step_id", "run_id", "step_idx",
            "policy_loss", "kl_loss", "total_loss",
            "advantage_mean", "advantage_std",
            "reward_batch_mean", "mean_ratio", "frac_clipped",
            "n_samples", "n_dropped",
        ]
        self._stmts.append(
            f"INSERT INTO rl_training_steps ({', '.join(cols)}) "
            f"VALUES ({', '.join(_q(row.get(c)) for c in cols)});"
        )

    def write_run_complete(self, run_id: str, final_reward: float,
                           verdict: str, completed_at: str) -> None:
        self._stmts.append(
            f"UPDATE rl_training_runs SET "
            f"completed_at={_q(completed_at)}, "
            f"final_aggregate_reward={_q(final_reward)}, "
            f"gate_verdict={_q(verdict)} "
            f"WHERE run_id={_q(run_id)};"
        )

    def flush(self) -> None:
        self.sidecar_path.parent.mkdir(parents=True, exist_ok=True)
        self.sidecar_path.write_text(
            "-- Phase 11 RL training persistence sidecar.\n"
            "-- Generated by neural.training.rl.trainer.\n"
            "-- Apply: psql -f <this-file>\n\n"
            + "\n".join(self._stmts) + "\n",
            encoding="utf-8",
        )


# ── trainer ─────────────────────────────────────────────────────────────────


@dataclass
class GRPOTrainerConfig:
    """All knobs come from ``configs/rl_phase11.yaml`` — MEMORY no hardcoded."""

    # Step loop
    max_steps: int
    batch_size: int
    eval_interval_steps: int
    # GRPO loss
    clip_ratio: float
    kl_coef: float
    entropy_coef: float
    # Advantage
    zero_stddev_policy: str
    widen_factor: float
    min_stddev: float
    advantage_clip: tuple[float, float]
    # Early stop
    early_stop_threshold: float
    early_stop_patience: int
    # Identity
    base_model_path: str
    base_model_sha: str
    adapter_manifest_sha: str | None
    output_path: str
    config_sha: str | None
    phase10_baseline_run_id: str | None
    # Paths
    sidecar_path: Path


@dataclass
class GRPOTrainerResult:
    """Final run summary."""

    run_id: str
    total_steps: int
    final_reward: float | None
    early_stopped: bool
    early_stop_reason: str | None
    step_history: list[dict[str, Any]] = field(default_factory=list)
    eval_history: list[tuple[int, float]] = field(default_factory=list)


class GRPOTrainer:
    """Orchestrates the GRPO step loop with injectable MLX callables."""

    def __init__(
        self,
        *,
        config: GRPOTrainerConfig,
        sampler: RewardSampler,
        rollout_fn: RolloutFn,
        optimizer_step_fn: OptimizerStepFn,
        eval_fn: EvalFn,
        checkpoint_fn: CheckpointFn | None = None,
        persistence: SqlSidecarPersistence | None = None,
        run_id: str | None = None,
    ) -> None:
        self.cfg = config
        self.sampler = sampler
        self.rollout_fn = rollout_fn
        self.optimizer_step_fn = optimizer_step_fn
        self.eval_fn = eval_fn
        self.checkpoint_fn = checkpoint_fn
        self.persistence = persistence or SqlSidecarPersistence(config.sidecar_path)
        self.run_id = run_id or new_run_id()
        self.early_stop = EarlyStopMonitor(
            mode="rl",
            ratio=config.early_stop_threshold,
            patience=config.early_stop_patience,
        )

    def train(self) -> GRPOTrainerResult:
        """Run the step loop to completion (or early-stop fire)."""
        started_at = datetime.now(timezone.utc).isoformat()
        self.persistence.write_run_start({
            "run_id": self.run_id,
            "base_model_path": self.cfg.base_model_path,
            "base_model_sha": self.cfg.base_model_sha,
            "adapter_manifest_sha": self.cfg.adapter_manifest_sha,
            "output_path": self.cfg.output_path,
            "config_sha": self.cfg.config_sha,
            "started_at": started_at,
            "phase10_baseline_run_id": self.cfg.phase10_baseline_run_id,
        })

        result = GRPOTrainerResult(
            run_id=self.run_id,
            total_steps=0,
            final_reward=None,
            early_stopped=False,
            early_stop_reason=None,
        )

        last_val_reward: float | None = None
        for step in range(1, self.cfg.max_steps + 1):
            step_diag = self._train_one_step(step)
            result.step_history.append(step_diag)
            result.total_steps = step

            # Eval + early-stop cadence.
            if step % self.cfg.eval_interval_steps == 0:
                val_reward = self.eval_fn(step)
                result.eval_history.append((step, val_reward))
                last_val_reward = val_reward
                # EarlyStopMonitor.on_line expects a formatted stdout line.
                line = f"Iter {step}: Val reward {val_reward:.4f}, Val took 0.0s"
                if self.early_stop.on_line(line):
                    result.early_stopped = True
                    result.early_stop_reason = self.early_stop.fired_reason
                    logger.info("GRPO early-stop fired: %s", result.early_stop_reason)
                    break

        result.final_reward = last_val_reward
        if self.checkpoint_fn is not None:
            self.checkpoint_fn(Path(self.cfg.output_path), result.total_steps)

        completed_at = datetime.now(timezone.utc).isoformat()
        self.persistence.write_run_complete(
            self.run_id,
            final_reward=float(result.final_reward) if result.final_reward is not None else 0.0,
            verdict="pending",  # flipped to pass/fail by the regression harness
            completed_at=completed_at,
        )
        self.persistence.flush()
        return result

    # ── internals ───────────────────────────────────────────────────────────

    def _train_one_step(self, step: int) -> dict[str, Any]:
        batch = self.sampler.sample_batch(self.cfg.batch_size)
        rollouts = self.rollout_fn(batch)
        if len(rollouts) != len(batch):
            raise RuntimeError(
                f"rollout_fn returned {len(rollouts)} results for batch of {len(batch)}"
            )

        rewards = np.array([r.scalar_reward for r in rollouts], dtype=np.float64)
        task_ids = [r.task_id for r in rollouts]
        logp_new = np.array([r.logprob_new for r in rollouts], dtype=np.float64)
        logp_old = np.array([r.logprob_old for r in rollouts], dtype=np.float64)
        logp_ref = np.array([r.logprob_ref for r in rollouts], dtype=np.float64)

        adv: AdvantageEstimate = estimate_advantage(
            rewards=rewards,
            task_ids=task_ids,
            stddev_cache=self.sampler.stddev_cache,
            mean_cache=self.sampler.mean_cache,
            policy=self.cfg.zero_stddev_policy,  # type: ignore[arg-type]
            widen_factor=self.cfg.widen_factor,
            min_stddev=self.cfg.min_stddev,
            advantage_clip=self.cfg.advantage_clip,
        )

        # Restrict logprobs to kept indices (drop policy may mask some out).
        keep = adv.keep_mask
        logp_new_k = logp_new[keep]
        logp_old_k = logp_old[keep]
        logp_ref_k = logp_ref[keep]

        if adv.advantages.size == 0:
            # Degenerate: every sample dropped. Skip optimizer step, still
            # emit a diagnostic row so operators can see the gap.
            diag = {
                "step_idx": step,
                "policy_loss": 0.0, "kl_loss": 0.0, "total_loss": 0.0,
                "advantage_mean": 0.0, "advantage_std": 0.0,
                "reward_batch_mean": float(np.mean(rewards)) if rewards.size else 0.0,
                "mean_ratio": 1.0, "frac_clipped": 0.0,
                "n_samples": 0, "n_dropped": int(adv.n_dropped),
            }
        else:
            loss: GRPOLossResult = compute_grpo_loss(
                logprobs_new=logp_new_k,
                logprobs_old=logp_old_k,
                advantages=adv.advantages,
                ref_logprobs=logp_ref_k,
                clip_ratio=self.cfg.clip_ratio,
                kl_coef=self.cfg.kl_coef,
                entropy_coef=self.cfg.entropy_coef,
            )
            self.optimizer_step_fn(
                loss.total,
                advantages=adv.advantages,
                keep_mask=adv.keep_mask,
                rollouts=rollouts,
            )
            diag = {
                "step_idx": step,
                "policy_loss": loss.policy_loss,
                "kl_loss": loss.kl_loss,
                "total_loss": loss.total,
                "advantage_mean": loss.mean_advantage,
                "advantage_std": loss.std_advantage,
                "reward_batch_mean": float(np.mean(rewards)),
                "mean_ratio": loss.mean_ratio,
                "frac_clipped": loss.frac_clipped,
                "n_samples": int(adv.advantages.size),
                "n_dropped": int(adv.n_dropped),
            }

        row = {
            "recorded_at": datetime.now(timezone.utc).isoformat(),
            "step_id": new_run_id(),
            "run_id": self.run_id,
            **diag,
        }
        self.persistence.write_step(row)
        return diag


# ── CLI wiring (thin; real MLX wiring lives in a follow-up adapter module) ──


def _load_yaml(path: Path) -> dict[str, Any]:
    import yaml  # type: ignore[import-not-found]
    with path.open() as f:
        return yaml.safe_load(f)


def build_config_from_yaml(yaml_cfg: dict[str, Any], *, sidecar_path: Path,
                            config_sha: str | None = None) -> GRPOTrainerConfig:
    """Map rl_phase11.yaml → GRPOTrainerConfig."""
    t = yaml_cfg["training"]
    g = yaml_cfg["grpo"]
    a = yaml_cfg["advantage"]
    return GRPOTrainerConfig(
        max_steps=t["max_steps"],
        batch_size=t["batch_size"],
        eval_interval_steps=t["eval_interval_steps"],
        clip_ratio=g["clip_ratio"],
        kl_coef=g["kl_coef"],
        entropy_coef=g["entropy_coef"],
        advantage_clip=tuple(g["advantage_clip"]),  # type: ignore[arg-type]
        zero_stddev_policy=a["zero_stddev_policy"],
        widen_factor=a["widen_factor"],
        min_stddev=a["min_stddev"],
        early_stop_threshold=t["early_stop_threshold"],
        early_stop_patience=t["early_stop_patience"],
        base_model_path=yaml_cfg["base_model"]["path"],
        base_model_sha=yaml_cfg["base_model"]["sha256"],
        adapter_manifest_sha=None,
        output_path=yaml_cfg["output"]["sandbox_path"],
        config_sha=config_sha,
        phase10_baseline_run_id=yaml_cfg["regression"].get("phase5_baseline_run_id"),
        sidecar_path=sidecar_path,
    )


def main() -> int:  # pragma: no cover — exercised by T-subset smoke + full run
    """CLI entry point — wires MLX adapter + live prompt/reward providers.

    End-to-end flow:
        1. Load YAML config + preflight-checked baseline.
        2. Build ``RewardSampler`` from ``benchmark_results`` TSDB rows.
        3. Build ``LiveCallSite`` (prompt_provider + reward_fn).
        4. Construct :class:`MLXGRPOAdapter` — loads policy + frozen reference
           through ``mlx_lm.load``.
        5. Attach the adapter's tokenizer to the prompt provider.
        6. Build the registry-only eval command.
        7. Instantiate :class:`GRPOTrainer` and run ``train()``.

    Exit codes:
        0 — training completed (possibly via early-stop).
        2 — config/preflight error or unrecoverable MLX error.
    """
    import argparse

    # Configure root logger so logger.info/warning calls from this module and
    # the MLX adapter reach stdout. Without this, `python -m ...trainer`
    # produces a silent log — the SQL sidecar is still written at end, but
    # mid-run progress is invisible to operators and monitors.
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    p = argparse.ArgumentParser(
        description="GRPO trainer (Phase 11) — real MLX run",
    )
    p.add_argument("--config", required=True,
                    help="Phase 11 RL config YAML (configs/rl_phase11.yaml)")
    p.add_argument("--out-sidecar", required=True,
                    help="SQL sidecar output path for rl_training_runs/steps")
    p.add_argument("--golden",
                    default="training_data/eval/valid_golden.jsonl",
                    help="Phase 10 golden holdout JSONL (prompt source)")
    p.add_argument("--specs-dir", default="docs/tests/ults/specs",
                    help="ULTS spec directory (reward source)")
    p.add_argument("--benchmark-config",
                    default="configs/benchmark_phase10.yaml",
                    help="Phase 10 benchmark config for in-training eval")
    p.add_argument("--max-steps", type=int, default=None,
                    help="Override training.max_steps (useful for smoke runs)")
    p.add_argument("--eval-interval-steps", type=int, default=None,
                    help="Override training.eval_interval_steps")
    p.add_argument("--batch-size", type=int, default=None,
                    help="Override training.batch_size")
    p.add_argument("--max-tokens", type=int, default=None,
                    help="Override rollout.max_tokens (min 3000 per MEMORY)")
    p.add_argument("--run-id", default=None,
                    help="CUIDv2 run_id; generated if omitted")
    p.add_argument("--dry-run", action="store_true",
                    help="Preflight + instantiate everything; exit before training")
    args = p.parse_args()

    # Lazy imports so `--help` doesn't pay the MLX cost.
    import hashlib
    from .live_wiring import (
        InProcessEvaluator,
        build_live_call_site,
        build_registry_eval_cmd,
    )
    from .mlx_adapter import MLXGRPOAdapter

    cfg_path = Path(args.config)
    yaml_cfg = _load_yaml(cfg_path)
    cfg_sha = hashlib.sha256(cfg_path.read_bytes()).hexdigest()
    cfg = build_config_from_yaml(
        yaml_cfg, sidecar_path=Path(args.out_sidecar), config_sha=cfg_sha,
    )

    # CLI overrides land on the config object so operators can shrink a run
    # for smoke-testing without editing YAML.
    if args.max_steps is not None:
        cfg.max_steps = args.max_steps
    if args.eval_interval_steps is not None:
        cfg.eval_interval_steps = args.eval_interval_steps
    if args.batch_size is not None:
        cfg.batch_size = args.batch_size

    logger.info("GRPO config: %s", cfg)

    # ── Reward sampler (reads benchmark_results rows from TSDB) ─────────────
    rs_cfg = yaml_cfg.get("reward_sampler", {}) or {}
    sampler = RewardSampler.from_tsdb(
        yaml_cfg.get("tsdb", {}) or {},
        run_ids=rs_cfg.get("source_run_ids") or None,
        strategy=rs_cfg.get("strategy", "stratified_by_group"),
        rng_seed=yaml_cfg["training"].get("seed", 0),
    )
    logger.info("RewardSampler: %d rows, %d tasks, zero-stddev tasks=%s",
                sampler.row_count, len(sampler.task_ids),
                sampler.zero_stddev_tasks())

    # ── Live prompt/reward wiring ───────────────────────────────────────────
    site = build_live_call_site(
        golden_path=Path(args.golden),
        specs_dir=Path(args.specs_dir),
    )

    # ── MLX adapter (heaviest step — loads policy + ref) ────────────────────
    rollout_cfg = yaml_cfg.get("rollout", {}) or {}
    max_tokens = args.max_tokens if args.max_tokens is not None else rollout_cfg.get("max_tokens", 4000)
    if max_tokens < 3000:
        logger.error("--max-tokens=%d violates MEMORY min-3000 policy", max_tokens)
        return 2
    generation_kwargs = {
        "max_tokens": max_tokens,
    }
    # MLX generate supports sampling kwargs via generation_kwargs dict.
    # Phase 10 sampling policy (temperature etc.) ships via the benchmark
    # config; the RL rollout uses MLX defaults unless operator overrides.

    # Model under test path (RL sandbox) = cfg.output_path. But for the FIRST
    # training step the policy is Phase 5 base — so we load from base_model
    # and write checkpoints into output_path.
    eval_cmd = build_registry_eval_cmd(
        model_path=cfg.base_model_path,
        benchmark_config=Path(args.benchmark_config),
    )

    # LoRA hyperparameters from rl_phase11.yaml §training.lora — sprint plan
    # mandates rank=32, alpha=64, dropout=0.05, target_modules=[q,k,v,o]_proj.
    # target_modules in YAML are bare suffixes (e.g. "q_proj"); mlx_lm wants
    # layer-local paths (e.g. "self_attn.q_proj").
    lora_yaml = (yaml_cfg.get("training") or {}).get("lora") or {}
    raw_targets = lora_yaml.get("target_modules") or [
        "q_proj", "k_proj", "v_proj", "o_proj"
    ]
    lora_targets = [
        m if "." in m else f"self_attn.{m}" for m in raw_targets
    ]

    adapter = MLXGRPOAdapter(
        base_model_path=cfg.base_model_path,
        adapter_path=None,  # Phase 5 base is the merged dense adapter
        prompt_provider=site.prompt_provider,
        reward_fn=site.reward_fn,
        clip_ratio=cfg.clip_ratio,
        kl_coef=cfg.kl_coef,
        entropy_coef=cfg.entropy_coef,
        lr=yaml_cfg["training"]["optim"]["lr"],
        weight_decay=yaml_cfg["training"]["optim"].get("weight_decay", 0.01),
        betas=(
            yaml_cfg["training"]["optim"].get("beta1", 0.9),
            yaml_cfg["training"]["optim"].get("beta2", 0.999),
        ),
        eps=yaml_cfg["training"]["optim"].get("eps", 1e-8),
        generation_kwargs=generation_kwargs,
        eval_cmd=eval_cmd,
        lora_rank=int(lora_yaml.get("rank", 32)),
        lora_alpha=float(lora_yaml.get("alpha", 64)),
        lora_dropout=float(lora_yaml.get("dropout", 0.05)),
        lora_target_modules=lora_targets,
        # LR schedule: warmup_steps from YAML §training.optim.warmup_steps;
        # decay_steps defaults to (max_steps - warmup_steps) so the cosine
        # tail covers the rest of training. lr_final defaults to lr × 0.1.
        # Set warmup_steps=0 (or remove from YAML) to revert to constant lr.
        lr_warmup_steps=int(yaml_cfg["training"]["optim"].get("warmup_steps", 0)),
        lr_decay_steps=max(
            0,
            int(cfg.max_steps) - int(yaml_cfg["training"]["optim"].get("warmup_steps", 0)),
        ),
    )
    # Tokenizer chicken-and-egg: set after MLX load.
    site.prompt_provider.attach_tokenizer(adapter.tokenizer)

    # ── Verify reward coverage for every task the sampler will hit ──────────
    missing_reward = sorted(set(sampler.task_ids) - set(site.reward_fn.available_tasks()))
    if missing_reward:
        logger.warning(
            "reward_fn has no coverage for %d sampler tasks: %s — "
            "those samples will score 0.0",
            len(missing_reward), missing_reward,
        )
    missing_prompt = sorted(set(sampler.task_ids) - set(site.prompt_provider.available_tasks()))
    if missing_prompt:
        logger.error(
            "prompt_provider has no golden rows for %d sampler tasks: %s — "
            "rollouts will crash. Either add rows to valid_golden.jsonl or "
            "exclude these tasks from source_run_ids.",
            len(missing_prompt), missing_prompt,
        )
        return 2

    if args.dry_run:
        logger.info("--dry-run: everything constructed; exiting before train()")
        return 0

    # ── In-process evaluator (faithful to training weights) ─────────────────
    # subprocess-based adapter.eval_fn talks to mlx_lm.server which holds a
    # pre-training model — would give a stale signal. Swap to in-process.
    eval_tasks = sorted(
        set(sampler.task_ids)
        & set(site.prompt_provider.available_tasks())
        & set(site.reward_fn.available_tasks())
    )
    evaluator = InProcessEvaluator(
        adapter=adapter,
        prompt_provider=site.prompt_provider,
        reward_fn=site.reward_fn,
        tasks=eval_tasks,
        n_samples_per_task=2,
        generation_kwargs=generation_kwargs,
    )

    # ── Run training ────────────────────────────────────────────────────────
    trainer = GRPOTrainer(
        config=cfg,
        sampler=sampler,
        rollout_fn=adapter.rollout_fn,
        optimizer_step_fn=adapter.optimizer_step_fn,
        eval_fn=evaluator,
        checkpoint_fn=adapter.checkpoint_fn,
        run_id=args.run_id,
    )
    result = trainer.train()
    logger.info("GRPO training complete: %s", result)
    return 0


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
