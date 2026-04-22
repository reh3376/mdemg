"""LoRA Fine-Tuning Script for MDEMG (Sprint FT-LORA-E tier-aware).

Wraps mlx_lm.lora as a subprocess and layers on the MoE-Sieve two-tier
discipline from memo 07 v3.1 §5:

    Tier 1 — attention + shared expert, r=32 α=64, universal (all 16 tasks).
    Tier 2 — top-25% routed experts per family, r=8 α=16, one adapter per
             family (reasoning-think, classify-notink, structured-notink).

Sprint D shipped the family profile artifacts under
`training_data/routing_profiles/`; this script consumes them via
`expert_selection.py` (Epic 2) to emit the per-expert module-path `keys`
that mlx_lm's `linear_to_lora_layers` needs for Tier 2 training.

Overfitting-prevention policy (FT-OAI-001 forcing function):
  * `--n-epochs auto` is rejected.
  * `--n-epochs > LORA_N_EPOCHS_CAP` (default 3) is rejected (not clamped).
  * Early-stop wraps the subprocess stdout via `early_stop.py` (Epic 4).

`router_aux_loss_coef` (default 0.002) prevents expert collapse during
MoE LoRA training. mlx_lm.lora has no CLI flag for it; this script tries
two paths in order:

  Primary: pass via `--config <yaml>`. mlx_lm.lora may or may not honor
           this for model-architecture parameters. Post-hoc verified via
           stdout grep; on verification failure, fall back to:

  Fallback: atomic copy-on-write replacement of the base model's
           config.json with signal-safe (SIGTERM/SIGINT/SIGHUP + atexit)
           restoration. SHA256 of the restored config must match the
           pre-training SHA or we fail loudly — this catches crashed-run
           drift that would otherwise silently corrupt future training.

Every invocation (Tier 1 AND Tier 2) must provide `--expected-sha256`
naming the Sprint C pin of the base model's config.json. Drift aborts
before any training starts.

Quick-start for Phase 5:
    # Tier 1
    python -m training.train_ft --tier 1 --mode sft \\
        --base-model <sprint-c-path> \\
        --expected-sha256 cdc167566e… \\
        --dataset curated/v1/ --adapter-path adapters/tier1/ \\
        --rank 32 --alpha 64 --n-epochs 3

    # Tier 2 (per family, after Tier 1)
    python -m training.train_ft --tier 2 --family reasoning-think \\
        --expert-selection-path training_data/routing_profiles/profile_routing_reasoning_think.json \\
        --expected-sha256 cdc167566e… \\
        --base-adapter adapters/tier1/ --base-model <sprint-c-path> \\
        --dataset curated/v1_reasoning_think/ --adapter-path adapters/tier2_rt/ \\
        --rank 8 --alpha 16 --n-epochs 3
"""

from __future__ import annotations

import argparse
import atexit
import hashlib
import json
import os
import shutil
import signal
import subprocess
import sys
import time
from pathlib import Path
from typing import Any


# ──────────────────────────── Constants ────────────────────────────

MIN_EXOGENOUS_RATIO = 0.4  # anti-collapse floor (legacy)

# FT-OAI-001 forcing function: see training_data/openai_ft/20260420/run_notes.md
FT_OAI_001_REF = (
    "FT-OAI-001 step-1200 overfit (2026-04-20): a prior OpenAI FT run "
    "overfit at step 1200 with n_epochs=auto. Policy: explicit epoch "
    "count, cap at LORA_N_EPOCHS_CAP (default 3), early-stop on "
    "val_loss degradation."
)

# Sprint C config.json SHA pin (Gate 3).
SPRINT_C_CONFIG_SHA = (
    "cdc167566e54ebe6d5c6df308649670b5f1cacfe71a198688edba8471ea64734"
)

# Canonical family names (hyphen form = CLI flag; underscore form = file).
FAMILY_CHOICES = ("reasoning-think", "classify-notink", "structured-notink")

# Tier 1 default target modules (applied across all 40 layers by mlx_lm).
TIER1_DEFAULT_TARGET_MODULES = (
    "self_attn.q_proj",
    "self_attn.k_proj",
    "self_attn.v_proj",
    "self_attn.o_proj",
    "mlp.shared_expert.gate_proj",
    "mlp.shared_expert.up_proj",
    "mlp.shared_expert.down_proj",
)


# ──────────────────────────── Type helpers ────────────────────────────


def n_epochs_type(value: str) -> int:
    """argparse type for --n-epochs: reject 'auto' explicitly, else int.

    The explicit 'auto' rejection is the FT-OAI-001 forcing function —
    we never want an implicit epoch count to slip through.
    """
    normalized = value.strip().lower()
    if normalized in ("auto", "automatic", "none"):
        raise argparse.ArgumentTypeError(
            f"--n-epochs={value!r} is not allowed. {FT_OAI_001_REF}"
        )
    try:
        n = int(value)
    except ValueError as exc:
        raise argparse.ArgumentTypeError(
            f"--n-epochs must be a positive integer (got {value!r}). "
            f"{FT_OAI_001_REF}"
        ) from exc
    if n < 1:
        raise argparse.ArgumentTypeError(
            f"--n-epochs must be >= 1 (got {n}). {FT_OAI_001_REF}"
        )
    return n


def _env_int(name: str, default: int) -> int:
    val = os.environ.get(name)
    if val is None or val == "":
        return default
    try:
        return int(val)
    except ValueError:
        print(f"WARN: {name}={val!r} is not an integer; using default {default}",
              file=sys.stderr)
        return default


def _env_float(name: str, default: float) -> float:
    val = os.environ.get(name)
    if val is None or val == "":
        return default
    try:
        return float(val)
    except ValueError:
        print(f"WARN: {name}={val!r} is not a float; using default {default}",
              file=sys.stderr)
        return default


# ──────────────────────────── Legacy manifest validation (retained) ─


def load_manifest(dataset_dir: str) -> dict[str, Any]:
    manifest_path = os.path.join(dataset_dir, "manifest.json")
    if not os.path.exists(manifest_path):
        raise FileNotFoundError(f"manifest.json not found in {dataset_dir}")
    with open(manifest_path) as f:
        return json.load(f)


def validate_manifest(manifest: dict[str, Any]) -> list[str]:
    errors: list[str] = []
    splits = manifest.get("splits", {})
    train_split = splits.get("train", {})
    if not train_split:
        errors.append("manifest missing splits.train")
    elif train_split.get("rows", 0) == 0:
        errors.append("train split has 0 rows")

    gates = manifest.get("quality_gates", {})
    exogenous_ratio = gates.get("exogenous_ratio", 0)
    exogenous_met = gates.get("exogenous_ratio_met", False)
    if not exogenous_met:
        errors.append(
            f"exogenous ratio {exogenous_ratio:.4f} below target "
            f"(manifest gate failed)"
        )
    if exogenous_ratio < MIN_EXOGENOUS_RATIO:
        errors.append(
            f"exogenous ratio {exogenous_ratio:.4f} below anti-collapse "
            f"minimum {MIN_EXOGENOUS_RATIO}"
        )
    if not gates.get("no_train_test_overlap", True):
        errors.append("train/test split has overlap — data contamination risk")
    return errors


def _count_lines(path: str) -> int:
    if not os.path.exists(path):
        return 0
    with open(path) as f:
        return sum(1 for _ in f)


def write_training_log(log_path: str, entry: dict[str, Any]) -> None:
    with open(log_path, "a") as f:
        f.write(json.dumps(entry) + "\n")


# ──────────────────────────── SHA integrity check ────────────────────────


def _sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def assert_config_sha(base_model: Path, expected_sha: str) -> str:
    """Compute SHA256 of <base_model>/config.json and assert match.

    Returns the observed SHA on success. Raises SystemExit on mismatch or
    missing file. Called for BOTH Tier 1 and Tier 2 — not just Tier 2 —
    because the atomic-replacement fallback can leave the base model in a
    drifted state if a prior training crashed without signal-handler restore.
    """
    config_path = base_model / "config.json"
    if not config_path.exists():
        print(
            f"ERROR: base model config.json not found at {config_path}",
            file=sys.stderr,
        )
        raise SystemExit(2)
    observed = _sha256_file(config_path)
    if observed != expected_sha:
        print(
            f"ERROR: base model config.json SHA256 drift detected.\n"
            f"  path:     {config_path}\n"
            f"  expected: {expected_sha}\n"
            f"  observed: {observed}\n"
            f"  Likely cause: a prior training crashed without restoring the\n"
            f"  original config.json from .pre-train-backup. To recover:\n"
            f"    1. Locate <model>/config.json.pre-train-backup if present.\n"
            f"    2. Restore: mv config.json.pre-train-backup config.json.\n"
            f"    3. Re-verify SHA256 matches {expected_sha}.\n"
            f"  Or re-download the model from the Sprint C pinned revision.",
            file=sys.stderr,
        )
        raise SystemExit(2)
    return observed


# ──────────────────────────── Atomic config.json injection ─


class RouterAuxConfigInjector:
    """Signal-safe atomic replacement of <base_model>/config.json.

    Used as the fallback router_aux_loss_coef injection path when the
    mlx_lm.lora `--config` YAML primary path is unverified (see
    Sprint E plan §5 Epic 1).

    Lifecycle:
        enter() — backup original, write modified config, arm signal handlers.
        restore() — restore original, assert SHA match, clear handlers.

    ``__enter__`` / ``__exit__`` make it usable as a ``with`` context, but
    the explicit calls are the actual happy path so the adapter-log can
    record each step.
    """

    BACKUP_SUFFIX = ".pre-train-backup"
    STAGING_SUFFIX = ".training"

    def __init__(
        self,
        base_model: Path,
        router_aux_loss_coef: float,
        expected_sha: str,
    ) -> None:
        self.base_model = base_model
        self.config_path = base_model / "config.json"
        self.backup_path = base_model / f"config.json{self.BACKUP_SUFFIX}"
        self.staging_path = base_model / f"config.json{self.STAGING_SUFFIX}"
        self.router_aux_loss_coef = router_aux_loss_coef
        self.expected_sha = expected_sha
        self._armed = False
        self._original_handlers: dict[int, Any] = {}

    def enter(self) -> None:
        if self._armed:
            return
        # Safety: if .pre-train-backup already exists, a previous run may
        # have crashed with handler restore that never finished. Refuse.
        if self.backup_path.exists():
            print(
                f"ERROR: {self.backup_path} already exists. A prior training "
                f"run may have crashed mid-restore. Inspect manually before "
                f"retrying. (If backup matches pinned SHA {self.expected_sha}, "
                f"restore it; then remove .pre-train-backup.)",
                file=sys.stderr,
            )
            raise SystemExit(2)

        # 1. Copy original to backup.
        shutil.copy2(self.config_path, self.backup_path)

        # 2. Read original, inject coef, write staging.
        with self.config_path.open("r") as f:
            original = json.load(f)
        modified = dict(original)
        modified["router_aux_loss_coef"] = self.router_aux_loss_coef
        with self.staging_path.open("w") as f:
            json.dump(modified, f, indent=2)

        # 3. Atomic rename (same-filesystem).
        os.rename(self.staging_path, self.config_path)

        # 4. Arm signal handlers + atexit.
        self._install_handlers()
        self._armed = True

    def restore(self) -> None:
        if not self._armed:
            return
        try:
            if not self.backup_path.exists():
                print(
                    f"ERROR: cannot restore config.json — backup missing at "
                    f"{self.backup_path}",
                    file=sys.stderr,
                )
                return
            # Atomic restore.
            os.replace(self.backup_path, self.config_path)
            # Verify SHA.
            observed = _sha256_file(self.config_path)
            if observed != self.expected_sha:
                print(
                    f"ERROR: post-restore SHA mismatch.\n"
                    f"  path:     {self.config_path}\n"
                    f"  expected: {self.expected_sha}\n"
                    f"  observed: {observed}\n"
                    f"  DO NOT RUN FURTHER TRAINING until this is reconciled.",
                    file=sys.stderr,
                )
                raise SystemExit(2)
        finally:
            self._uninstall_handlers()
            self._armed = False

    # ── signal / atexit plumbing ──

    def _install_handlers(self) -> None:
        for sig in (signal.SIGTERM, signal.SIGINT, signal.SIGHUP):
            try:
                self._original_handlers[sig] = signal.getsignal(sig)
                signal.signal(sig, self._signal_restore)
            except (OSError, ValueError):
                # SIGHUP may not exist on Windows; ignore.
                pass
        atexit.register(self._atexit_restore)

    def _uninstall_handlers(self) -> None:
        for sig, handler in self._original_handlers.items():
            try:
                signal.signal(sig, handler)
            except (OSError, ValueError):
                pass
        self._original_handlers.clear()
        # atexit has no unregister API worth using here; the handler
        # becomes a no-op because _armed=False.

    def _signal_restore(self, signum: int, frame: Any) -> None:
        self.restore()
        # Re-raise the original signal so default behavior follows.
        signal.signal(signum, signal.SIG_DFL)
        os.kill(os.getpid(), signum)

    def _atexit_restore(self) -> None:
        if self._armed:
            try:
                self.restore()
            except SystemExit:
                # atexit can't raise — swallow; a mismatch was already
                # logged to stderr.
                pass

    # ── context manager sugar ──

    def __enter__(self) -> "RouterAuxConfigInjector":
        self.enter()
        return self

    def __exit__(self, exc_type: Any, exc: Any, tb: Any) -> None:
        self.restore()


def verify_router_aux_in_stdout(stdout_text: str, coef: float) -> bool:
    """Grep training stdout for evidence that router_aux_loss_coef is wired.

    Looks for: (a) echoed config key, (b) non-zero aux-loss metric lines,
    (c) any mention of the coef in a dump. This is the Sprint E Epic 1
    primary-path verification; if it returns False the fallback atomic
    config.json path is used.
    """
    needles = [
        "router_aux_loss_coef",
        "aux_loss",
        "auxiliary loss",
        f"{coef}",
    ]
    lowered = stdout_text.lower()
    return any(n.lower() in lowered for n in needles)


# ──────────────────────────── Build mlx_lm command ────────────────────────


def build_mlx_lm_command(
    *,
    base_model: str,
    dataset_dir: str,
    adapter_path: str,
    iters: int,
    batch_size: int,
    learning_rate: float,
    lora_rank: int,
    lora_alpha: int,
    lora_layers: int,
    max_seq_length: int,
    target_keys: list[str] | None,
    base_adapter: str | None,
    config_yaml_path: str | None,
    test_batches: int | None,
) -> list[str]:
    """Assemble the mlx_lm.lora subprocess argv.

    target_keys: if provided (Tier 2), written to a JSON sidecar the caller
        passes via --config YAML so linear_to_lora_layers(keys=…) selects
        only those modules.
    """
    cmd = [
        sys.executable, "-m", "mlx_lm.lora",
        "--train",
        "--model", base_model,
        "--data", dataset_dir,
        "--adapter-path", adapter_path,
        "--iters", str(iters),
        "--batch-size", str(batch_size),
        "--learning-rate", str(learning_rate),
        "--num-layers", str(lora_layers),
    ]
    # mlx_lm.lora accepts --lora-rank in 0.31.2 via the --lora-parameters hash
    # or via --config YAML. To remain compatible across mlx_lm releases we
    # route rank/alpha/keys through the config YAML (which also carries
    # router_aux_loss_coef for the primary-path attempt). See
    # build_config_yaml below.
    if config_yaml_path:
        cmd.extend(["--config", config_yaml_path])
    if base_adapter:
        cmd.extend(["--resume-adapter-file", str(Path(base_adapter) / "adapters.safetensors")])
    if max_seq_length:
        cmd.extend(["--max-seq-length", str(max_seq_length)])
    if test_batches:
        cmd.extend(["--test", "--test-batches", str(test_batches)])
    return cmd


def build_config_yaml(
    *,
    lora_rank: int,
    lora_alpha: int,
    target_keys: list[str] | None,
    router_aux_loss_coef: float,
    steps_per_eval: int | None = None,
) -> str:
    """Render the mlx_lm.lora --config YAML.

    Uses a minimal subset of the mlx_lm.lora config schema. The
    `lora_parameters.keys` field is the Tier 2 target-module selector
    (emitted by expert_selection.py). `router_aux_loss_coef` is the
    Sprint E primary-path injection attempt.
    """
    lines = [
        "# Autogenerated by neural/training/train_ft.py (Sprint FT-LORA-E).",
        f"router_aux_loss_coef: {router_aux_loss_coef}",
        "lora_parameters:",
        f"  rank: {lora_rank}",
        f"  alpha: {lora_alpha}",
        "  dropout: 0.0",
        "  scale: 20.0",
    ]
    if target_keys:
        lines.append("  keys:")
        for k in target_keys:
            lines.append(f"    - {k}")
    if steps_per_eval is not None:
        lines.append(f"steps_per_eval: {steps_per_eval}")
    return "\n".join(lines) + "\n"


# ──────────────────────────── Tier resolution ────────────────────────


def resolve_target_keys(
    tier: int,
    family: str | None,
    expert_selection_path: Path | None,
    override_csv: str | None,
) -> tuple[list[str] | None, dict[str, Any]]:
    """Return (target_keys, metadata) for the given tier.

    Tier 1: returns (None, {…}) — mlx_lm trains `lora_parameters.keys`
        defaulted to attn projections (per upstream mlx_lm.lora defaults).
        Sprint E passes the explicit list via override_csv when callers
        want the shared_expert projections included.

    Tier 2: returns (7,680-long list, {…}) — emitted by expert_selection.
    """
    metadata: dict[str, Any] = {"tier": tier}

    if override_csv:
        keys = [k.strip() for k in override_csv.split(",") if k.strip()]
        metadata["source"] = "--target-modules"
        metadata["keys_count"] = len(keys)
        return keys, metadata

    if tier == 1:
        # For Tier 1 we materialize the explicit list (7 modules) so the
        # training run doesn't silently rely on mlx_lm's default target set
        # (which might exclude shared_expert).
        keys = list(TIER1_DEFAULT_TARGET_MODULES)
        metadata["source"] = "tier1-default"
        metadata["keys_count"] = len(keys)
        return keys, metadata

    # Tier 2 — Sprint D profile.
    if not expert_selection_path or not family:
        raise ValueError(
            "Tier 2 requires both --family and --expert-selection-path"
        )
    # Late import — expert_selection is a sibling module in neural/training.
    from .expert_selection import ExpertSelection  # type: ignore[import-not-found]

    sel = ExpertSelection.load(expert_selection_path, family)
    keys = sel.mlx_lm_keys()
    metadata["source"] = "expert-selection"
    metadata["family"] = sel.family
    metadata["expert_selection_sha_pin"] = sel.model_sha256_config
    metadata["keys_count"] = len(keys)
    metadata["num_layers"] = sel.num_layers
    metadata["num_experts_selected_per_layer"] = sel.num_experts_selected_per_layer
    return keys, metadata


# ──────────────────────────── Subprocess wrap with monitor ─


def run_subprocess_with_monitor(
    cmd: list[str],
    monitor: Any | None,
    adapter_path: str,
) -> tuple[int, str]:
    """Launch mlx_lm.lora subprocess, stream stdout, apply early-stop monitor.

    Returns (exit_code, collected_stdout). collected_stdout is used by
    verify_router_aux_in_stdout for primary-path detection.

    If the monitor fires, SIGTERM is sent; after 30s grace period, SIGKILL.
    Writes <adapter-path>/.earlystop.json sidecar on fire.
    """
    collected: list[str] = []
    proc = subprocess.Popen(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.STDOUT,
        bufsize=1,
        text=True,
    )
    fired = False
    try:
        assert proc.stdout is not None
        for line in proc.stdout:
            sys.stdout.write(line)
            sys.stdout.flush()
            collected.append(line)
            if monitor is not None and monitor.on_line(line):
                if not fired:
                    fired = True
                    sidecar = Path(adapter_path) / ".earlystop.json"
                    sidecar.parent.mkdir(parents=True, exist_ok=True)
                    with sidecar.open("w") as f:
                        json.dump(monitor.sidecar_record(), f, indent=2)
                    print(f"\n[EARLY-STOP] {monitor.fired_reason}", file=sys.stderr)
                    print(f"[EARLY-STOP] sidecar written to {sidecar}", file=sys.stderr)
                    print("[EARLY-STOP] sending SIGTERM…", file=sys.stderr)
                    proc.send_signal(signal.SIGTERM)
    except KeyboardInterrupt:
        proc.send_signal(signal.SIGTERM)
    # Give 30s for graceful shutdown, then SIGKILL.
    try:
        proc.wait(timeout=30)
    except subprocess.TimeoutExpired:
        proc.kill()
        proc.wait()
    return proc.returncode, "".join(collected)


# ──────────────────────────── Main orchestration ────────────────────────


def run_train(args: argparse.Namespace) -> dict[str, Any]:
    """Top-level orchestration. Raises SystemExit on error."""

    # 1. Parse tier + family mutual requirements.
    if args.tier == 2 and not args.family:
        print("ERROR: --tier 2 requires --family {" +
              "|".join(FAMILY_CHOICES) + "}", file=sys.stderr)
        raise SystemExit(2)
    if args.tier == 2 and not args.expert_selection_path:
        print("ERROR: --tier 2 requires --expert-selection-path", file=sys.stderr)
        raise SystemExit(2)

    # 2. Epoch cap enforcement — reject, do not clamp.
    epoch_cap = _env_int("LORA_N_EPOCHS_CAP", 3)
    if args.n_epochs > epoch_cap:
        print(
            f"ERROR: --n-epochs={args.n_epochs} exceeds LORA_N_EPOCHS_CAP={epoch_cap}. "
            f"{FT_OAI_001_REF}",
            file=sys.stderr,
        )
        raise SystemExit(2)

    # 3. Pre-training SHA integrity check — BOTH tiers.
    base_model_path = Path(args.base_model)
    observed_sha = assert_config_sha(base_model_path, args.expected_sha256)

    # 4. Legacy manifest validation (if dataset is a directory with manifest).
    dataset_path = Path(args.dataset)
    manifest_data: dict[str, Any] = {}
    if dataset_path.is_dir() and (dataset_path / "manifest.json").exists():
        manifest_data = load_manifest(str(dataset_path))
        manifest_errors = validate_manifest(manifest_data)
        if manifest_errors:
            print("Manifest validation failed:", file=sys.stderr)
            for e in manifest_errors:
                print(f"  - {e}", file=sys.stderr)
            raise SystemExit(1)

    # 5. Resolve target modules.
    target_keys, key_meta = resolve_target_keys(
        tier=args.tier,
        family=args.family,
        expert_selection_path=(
            Path(args.expert_selection_path)
            if args.expert_selection_path else None
        ),
        override_csv=args.target_modules,
    )

    # 6. Resolve rank/alpha (tier default + env fallback + CLI override).
    tier_rank_env = f"LORA_TIER{args.tier}_RANK"
    tier_alpha_env = f"LORA_TIER{args.tier}_ALPHA"
    default_rank = 32 if args.tier == 1 else 8
    default_alpha = 64 if args.tier == 1 else 16
    lora_rank = (
        args.rank if args.rank is not None
        else _env_int(tier_rank_env, default_rank)
    )
    lora_alpha = (
        args.alpha if args.alpha is not None
        else _env_int(tier_alpha_env, default_alpha)
    )

    # 7. Compute iters from n_epochs × ceil(rows / batch_size).
    if dataset_path.is_dir():
        train_jsonl = dataset_path / "train.jsonl"
    else:
        train_jsonl = dataset_path
    train_rows = 0
    if manifest_data:
        train_rows = manifest_data.get("splits", {}).get("train", {}).get("rows", 0)
    if train_rows == 0 and train_jsonl.exists():
        train_rows = _count_lines(str(train_jsonl))
    if train_rows == 0:
        print(
            f"ERROR: could not determine train row count from {args.dataset}",
            file=sys.stderr,
        )
        raise SystemExit(2)
    iters = args.n_epochs * ((train_rows + args.batch_size - 1) // args.batch_size)

    # 8. Build config YAML sidecar.
    adapter_dir = Path(args.adapter_path)
    adapter_dir.mkdir(parents=True, exist_ok=True)
    yaml_path = adapter_dir / "train_config.yaml"
    yaml_path.write_text(build_config_yaml(
        lora_rank=lora_rank,
        lora_alpha=lora_alpha,
        target_keys=target_keys,
        router_aux_loss_coef=args.router_aux_loss_coef,
    ))

    # 9. Build mlx_lm command.
    cmd = build_mlx_lm_command(
        base_model=str(base_model_path),
        dataset_dir=str(dataset_path),
        adapter_path=str(adapter_dir),
        iters=iters,
        batch_size=args.batch_size,
        learning_rate=args.learning_rate,
        lora_rank=lora_rank,
        lora_alpha=lora_alpha,
        lora_layers=args.lora_layers,
        max_seq_length=args.max_seq_length,
        target_keys=target_keys,
        base_adapter=args.base_adapter,
        config_yaml_path=str(yaml_path),
        test_batches=10 if (dataset_path / "test.jsonl").exists() else None,
    )

    # 10. Summary (also returned by dry-run).
    summary: dict[str, Any] = {
        "sprint": "FT-LORA-E",
        "tier": args.tier,
        "mode": args.mode,
        "family": args.family,
        "base_model": str(base_model_path),
        "base_model_config_sha256": observed_sha,
        "adapter_path": str(adapter_dir),
        "base_adapter": args.base_adapter,
        "lora_rank": lora_rank,
        "lora_alpha": lora_alpha,
        "lora_layers": args.lora_layers,
        "n_epochs": args.n_epochs,
        "epoch_cap": epoch_cap,
        "batch_size": args.batch_size,
        "learning_rate": args.learning_rate,
        "train_rows": train_rows,
        "iters": iters,
        "router_aux_loss_coef": args.router_aux_loss_coef,
        "early_stop_ratio": args.early_stop_ratio,
        "early_stop_patience": args.early_stop_patience,
        "target_modules_source": key_meta.get("source"),
        "target_modules_count": key_meta.get("keys_count"),
        "config_yaml_path": str(yaml_path),
        "mlx_lm_command": cmd,
    }
    if args.tier == 2:
        summary["expert_selection_path"] = args.expert_selection_path
        summary["expert_selection_sha_pin"] = key_meta.get("expert_selection_sha_pin")

    if args.dry_run:
        summary["status"] = "dry_run"
        print()
        print("═══ Tier-Aware Training Dry-Run ═══")
        for k, v in summary.items():
            if k == "mlx_lm_command":
                print(f"  {k}:")
                for arg in v:
                    print(f"    {arg}")
            elif k == "target_modules_count":
                print(f"  {k}: {v}")
            else:
                print(f"  {k}: {v}")
        if target_keys and args.tier == 2:
            print()
            print(f"  first 3 target keys:")
            for k in target_keys[:3]:
                print(f"    {k}")
            print(f"  last 3 target keys:")
            for k in target_keys[-3:]:
                print(f"    {k}")
        return summary

    # 11. Router-aux injection — primary YAML path attempt + fallback atomic.
    #     Import early_stop lazily (keeps --help fast).
    from .early_stop import EarlyStopMonitor  # type: ignore[import-not-found]

    monitor_ratio = (
        args.early_stop_ratio if args.early_stop_ratio is not None
        else _env_float(
            "LORA_EARLY_STOP_SFT_THRESHOLD" if args.mode == "sft"
            else "LORA_EARLY_STOP_RL_THRESHOLD",
            1.05 if args.mode == "sft" else 0.95,
        )
    )
    monitor = EarlyStopMonitor(
        mode=args.mode,
        ratio=monitor_ratio,
        patience=args.early_stop_patience,
    )

    log_path = str(adapter_dir / "training_log.jsonl")
    start_time = time.time()
    write_training_log(log_path, {
        "event": "train_start",
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "summary": {k: v for k, v in summary.items() if k != "mlx_lm_command"},
    })

    # Primary path: --config YAML with router_aux_loss_coef key.
    # We can't run the subprocess twice just to verify the coef was honored;
    # instead, after a real run, grep the collected stdout. If the primary
    # path shows no evidence, Sprint F / a re-run should switch to fallback.
    # For safety today we ALWAYS install the fallback atomic injector —
    # whichever path mlx_lm actually reads the coef from, the config.json
    # now contains it regardless.

    injector = RouterAuxConfigInjector(
        base_model=base_model_path,
        router_aux_loss_coef=args.router_aux_loss_coef,
        expected_sha=observed_sha,
    )
    injector.enter()
    try:
        exit_code, stdout_text = run_subprocess_with_monitor(
            cmd, monitor, str(adapter_dir)
        )
        summary["primary_path_verified"] = verify_router_aux_in_stdout(
            stdout_text, args.router_aux_loss_coef
        )
    finally:
        injector.restore()

    elapsed = time.time() - start_time
    summary["elapsed_seconds"] = round(elapsed, 1)
    summary["exit_code"] = exit_code
    summary["early_stop_fired"] = monitor.fired if monitor else False
    summary["status"] = (
        "early_stopped" if monitor and monitor.fired
        else ("success" if exit_code == 0 else "failed")
    )

    write_training_log(log_path, {
        "event": "train_end",
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        "status": summary["status"],
        "elapsed_seconds": summary["elapsed_seconds"],
        "exit_code": exit_code,
        "early_stop_fired": summary["early_stop_fired"],
        "primary_path_verified": summary["primary_path_verified"],
    })

    if exit_code != 0 and summary["status"] != "early_stopped":
        raise SystemExit(exit_code)

    return summary


# ──────────────────────────── CLI ────────────────────────────


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description=(
            "Tier-aware LoRA fine-tuning for MDEMG (Sprint FT-LORA-E). "
            "Memo 07 v3.1 MoE-Sieve strategy: Tier 1 = attention + shared "
            "expert (r=32 α=64), Tier 2 = top-25% routed experts per family "
            "(r=8 α=16)."
        ),
    )
    # Tier + family + selection.
    parser.add_argument(
        "--tier", type=int, choices=(1, 2), required=True,
        help="1 = universal attn+shared LoRA; 2 = per-family routed experts.",
    )
    parser.add_argument(
        "--family", choices=FAMILY_CHOICES, default=None,
        help="Required for --tier 2. Family name (hyphen form).",
    )
    parser.add_argument(
        "--expert-selection-path", default=None,
        help="Required for --tier 2. Sprint D profile JSON "
             "(training_data/routing_profiles/profile_routing_<family>.json).",
    )
    parser.add_argument(
        "--mode", choices=("sft", "rl"), default="sft",
        help="Training mode. SFT early-stops on val_loss; RL on val_reward.",
    )

    # Model + data + output.
    parser.add_argument(
        "--base-model", required=True,
        help="Base MLX model (local path).",
    )
    parser.add_argument(
        "--expected-sha256", required=True,
        help="Expected SHA256 of <base-model>/config.json. Sprint C pin: "
             f"{SPRINT_C_CONFIG_SHA}",
    )
    parser.add_argument(
        "--dataset", required=True,
        help="Dataset directory (containing manifest.json+train.jsonl) OR a "
             ".jsonl file directly.",
    )
    parser.add_argument(
        "--adapter-path", required=True,
        help="Output directory for LoRA adapter weights.",
    )
    parser.add_argument(
        "--base-adapter", default=None,
        help="Optional: path to Tier 1 adapter dir. For Tier 2 composition.",
    )

    # LoRA hyperparameters.
    parser.add_argument(
        "--rank", type=int, default=None,
        help="LoRA rank. Falls back to LORA_TIER{N}_RANK env, then "
             "tier-default (T1=32, T2=8).",
    )
    parser.add_argument(
        "--alpha", type=int, default=None,
        help="LoRA alpha. Falls back to LORA_TIER{N}_ALPHA env, then "
             "tier-default (T1=64, T2=16).",
    )
    parser.add_argument(
        "--lora-layers", type=int, default=40,
        help="Number of layers to apply LoRA to (Qwen3.6-35B has 40).",
    )
    parser.add_argument(
        "--target-modules", default=None,
        help="Comma-separated module-path list. Overrides tier-default and "
             "expert-selection-emitted keys.",
    )

    # Training loop.
    parser.add_argument(
        "--n-epochs", type=n_epochs_type, required=True,
        help="Epoch count (required integer). 'auto' is REJECTED. "
             "Capped by LORA_N_EPOCHS_CAP (default 3).",
    )
    parser.add_argument("--batch-size", type=int, default=4)
    parser.add_argument(
        "--learning-rate", type=float, default=1e-5,
        help="Learning rate (default: 1e-5).",
    )
    parser.add_argument(
        "--max-seq-length", type=int, default=2048,
        help="Max sequence length (default: 2048).",
    )

    # MoE + early-stop.
    parser.add_argument(
        "--router-aux-loss-coef", type=float,
        default=_env_float("ROUTER_AUX_LOSS_COEF", 0.002),
        help="Router auxiliary-loss coefficient. Falls back to "
             "ROUTER_AUX_LOSS_COEF env (default 0.002).",
    )
    parser.add_argument(
        "--early-stop-ratio", type=float, default=None,
        help="Early-stop trigger ratio. Default: 1.05 for SFT "
             "(LORA_EARLY_STOP_SFT_THRESHOLD) / 0.95 for RL "
             "(LORA_EARLY_STOP_RL_THRESHOLD).",
    )
    parser.add_argument(
        "--early-stop-patience", type=int, default=2,
        help="Consecutive bad evals before SIGTERM (default 2).",
    )

    # Legacy flags preserved for backward compat in callers.
    parser.add_argument("--ults-dir", default=None, help=argparse.SUPPRESS)

    # Dry-run + report.
    parser.add_argument(
        "--dry-run", action="store_true",
        help="Print resolved config + mlx_lm.lora argv without training.",
    )
    parser.add_argument(
        "--report", default=None,
        help="Write JSON summary to this path.",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    parser = build_parser()
    args = parser.parse_args(argv)

    summary = run_train(args)

    if args.report:
        with open(args.report, "w") as f:
            json.dump(summary, f, indent=2, default=str)
        print(f"\nReport written to {args.report}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
