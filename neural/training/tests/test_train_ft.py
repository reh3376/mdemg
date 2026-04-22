"""Tests for the tier-aware LoRA fine-tuning script (Sprint FT-LORA-E)."""

from __future__ import annotations

import hashlib
import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path

import pytest

from training.train_ft import (
    FT_OAI_001_REF,
    SPRINT_C_CONFIG_SHA,
    TIER1_DEFAULT_TARGET_MODULES,
    assert_config_sha,
    build_config_yaml,
    build_mlx_lm_command,
    build_parser,
    load_manifest,
    n_epochs_type,
    resolve_target_keys,
    validate_manifest,
)


REPO_ROOT = Path(__file__).resolve().parents[3]
TRAIN_FT_SCRIPT = REPO_ROOT / "neural" / "training" / "train_ft.py"
PROFILES_DIR = REPO_ROOT / "training_data" / "routing_profiles"
REASONING_THINK_PROFILE = PROFILES_DIR / "profile_routing_reasoning_think.json"


# ──────────────────────────────────────────────────────────────
# Helpers
# ──────────────────────────────────────────────────────────────


def _valid_manifest(**overrides):
    base = {
        "dataset_id": "test-dataset",
        "version": "v1",
        "splits": {
            "train": {"file": "train.jsonl", "rows": 500},
            "test": {"file": "test.jsonl", "rows": 100},
        },
        "quality_gates": {
            "exogenous_ratio": 0.55,
            "exogenous_ratio_met": True,
            "no_train_test_overlap": True,
        },
        "config": {"raft_ratio": 0.5},
    }
    base.update(overrides)
    return base


def _write_manifest(tmpdir: str, manifest: dict) -> str:
    path = os.path.join(tmpdir, "manifest.json")
    with open(path, "w") as f:
        json.dump(manifest, f)
    return path


def _fake_model_dir(tmp: Path, config_body: dict | None = None) -> tuple[Path, str]:
    """Create a dir with a config.json; return (path, sha256)."""
    model = tmp / "model"
    model.mkdir()
    body = config_body if config_body is not None else {
        "architectures": ["Qwen3NextMoeForCausalLM"],
        "hidden_size": 2048,
        "num_experts": 256,
    }
    payload = json.dumps(body, indent=2, sort_keys=True).encode()
    (model / "config.json").write_bytes(payload)
    return model, hashlib.sha256(payload).hexdigest()


def _run_cli(args: list[str], env: dict[str, str] | None = None) -> subprocess.CompletedProcess:
    """Run train_ft.py as a subprocess; capture stdout/stderr."""
    full_env = os.environ.copy()
    full_env.setdefault("PYTHONUNBUFFERED", "1")
    if env:
        full_env.update(env)
    return subprocess.run(
        [sys.executable, "-m", "neural.training.train_ft", *args],
        capture_output=True, text=True, env=full_env, cwd=str(REPO_ROOT),
    )


def _tiny_dataset(tmp: Path, rows: int = 40) -> Path:
    d = tmp / "ds"
    d.mkdir()
    with (d / "train.jsonl").open("w") as f:
        for i in range(rows):
            f.write(json.dumps({"text": f"sample {i}"}) + "\n")
    return d


# ──────────────────────────────────────────────────────────────
# validate_manifest / load_manifest (preserved — still in module)
# ──────────────────────────────────────────────────────────────


class TestValidateManifest:
    def test_valid_manifest(self):
        assert validate_manifest(_valid_manifest()) == []

    def test_missing_train_split(self):
        errors = validate_manifest(_valid_manifest(
            splits={"test": {"file": "test.jsonl", "rows": 50}}
        ))
        assert any("splits.train" in e for e in errors)

    def test_zero_train_rows(self):
        errors = validate_manifest(_valid_manifest(
            splits={"train": {"file": "train.jsonl", "rows": 0}}
        ))
        assert any("0 rows" in e for e in errors)

    def test_low_exogenous_ratio(self):
        errors = validate_manifest(_valid_manifest(
            quality_gates={
                "exogenous_ratio": 0.2,
                "exogenous_ratio_met": False,
                "no_train_test_overlap": True,
            }
        ))
        assert any("anti-collapse" in e for e in errors)


class TestLoadManifest:
    def test_load_valid(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            _write_manifest(tmpdir, _valid_manifest())
            assert load_manifest(tmpdir)["dataset_id"] == "test-dataset"

    def test_missing_manifest(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            with pytest.raises(FileNotFoundError):
                load_manifest(tmpdir)


# ──────────────────────────────────────────────────────────────
# n_epochs_type (FT-OAI-001 policy)
# ──────────────────────────────────────────────────────────────


class TestNEpochsType:
    def test_valid_integer(self):
        assert n_epochs_type("3") == 3

    def test_rejects_auto(self):
        with pytest.raises(Exception):  # argparse.ArgumentTypeError
            n_epochs_type("auto")

    def test_rejects_automatic(self):
        with pytest.raises(Exception):
            n_epochs_type("automatic")

    def test_rejects_none(self):
        with pytest.raises(Exception):
            n_epochs_type("none")

    def test_rejects_non_integer(self):
        with pytest.raises(Exception):
            n_epochs_type("1.5")

    def test_rejects_negative(self):
        with pytest.raises(Exception):
            n_epochs_type("-1")

    def test_rejects_zero(self):
        with pytest.raises(Exception):
            n_epochs_type("0")


# ──────────────────────────────────────────────────────────────
# assert_config_sha (integrity check — both tiers)
# ──────────────────────────────────────────────────────────────


class TestAssertConfigSha:
    def test_matching_sha_returns_observed(self, tmp_path: Path):
        model, sha = _fake_model_dir(tmp_path)
        got = assert_config_sha(model, sha)
        assert got == sha

    def test_drift_raises_with_recovery_instructions(self, tmp_path: Path):
        model, _sha = _fake_model_dir(tmp_path)
        with pytest.raises(SystemExit):
            assert_config_sha(model, "DEADBEEF" * 8)


# ──────────────────────────────────────────────────────────────
# resolve_target_keys
# ──────────────────────────────────────────────────────────────


class TestResolveTargetKeys:
    def test_tier1_default_returns_none_uses_default_list(self):
        keys, meta = resolve_target_keys(tier=1, family=None,
                                         expert_selection_path=None,
                                         override_csv=None)
        assert keys == list(TIER1_DEFAULT_TARGET_MODULES)
        assert len(keys) == 7  # 4 attn + 3 shared_expert projs
        assert meta["source"] == "tier1-default"

    def test_tier2_loads_7680_keys_from_reasoning_think_profile(self):
        keys, meta = resolve_target_keys(
            tier=2, family="reasoning-think",
            expert_selection_path=REASONING_THINK_PROFILE,
            override_csv=None,
        )
        assert len(keys) == 7680
        assert meta["source"] == "expert-selection"
        assert meta["keys_count"] == 7680
        assert meta["family"] == "reasoning_think"

    def test_override_csv_supersedes_all(self):
        keys, meta = resolve_target_keys(
            tier=1, family=None, expert_selection_path=None,
            override_csv="foo,bar,baz",
        )
        assert keys == ["foo", "bar", "baz"]
        assert meta["source"] == "--target-modules"


# ──────────────────────────────────────────────────────────────
# build_mlx_lm_command / build_config_yaml
# ──────────────────────────────────────────────────────────────


class TestBuildMlxLmCommand:
    def test_basic_shape(self):
        cmd = build_mlx_lm_command(
            base_model="/m", dataset_dir="/d", adapter_path="/a",
            iters=100, batch_size=4, learning_rate=1e-5,
            lora_rank=32, lora_alpha=64, lora_layers=40,
            max_seq_length=2048, target_keys=None,
            base_adapter=None, config_yaml_path="/tmp/cfg.yaml",
            test_batches=None,
        )
        assert "mlx_lm.lora" in cmd
        assert "--train" in cmd
        assert "--model" in cmd and "/m" in cmd
        assert "--iters" in cmd and "100" in cmd
        assert "--num-layers" in cmd and "40" in cmd
        # The legacy --lora-layers flag is NOT a real mlx_lm.lora option.
        assert "--lora-layers" not in cmd
        assert "--config" in cmd and "/tmp/cfg.yaml" in cmd

    def test_base_adapter_flag_wired(self, tmp_path: Path):
        adapter = tmp_path / "tier1-adapter"
        adapter.mkdir()
        cmd = build_mlx_lm_command(
            base_model="/m", dataset_dir="/d", adapter_path="/a",
            iters=1, batch_size=1, learning_rate=1e-5,
            lora_rank=8, lora_alpha=16, lora_layers=40,
            max_seq_length=2048, target_keys=None,
            base_adapter=str(adapter), config_yaml_path=None,
            test_batches=None,
        )
        # Look for --resume-adapter-file pointing at the adapter.
        assert "--resume-adapter-file" in cmd
        idx = cmd.index("--resume-adapter-file")
        assert str(adapter) in cmd[idx + 1]


class TestBuildConfigYaml:
    def test_contains_router_aux(self):
        yaml = build_config_yaml(
            lora_rank=32, lora_alpha=64,
            target_keys=None, router_aux_loss_coef=0.002,
        )
        assert "router_aux_loss_coef: 0.002" in yaml
        assert "rank: 32" in yaml
        assert "alpha: 64" in yaml

    def test_tier2_keys_written(self):
        yaml = build_config_yaml(
            lora_rank=8, lora_alpha=16,
            target_keys=["a.b.c", "d.e.f"],
            router_aux_loss_coef=0.002,
        )
        assert "a.b.c" in yaml and "d.e.f" in yaml


# ──────────────────────────────────────────────────────────────
# Parser — ensures CLI surface wired
# ──────────────────────────────────────────────────────────────


class TestParser:
    def test_tier_1_parses(self):
        p = build_parser()
        args = p.parse_args([
            "--tier", "1", "--base-model", "/m",
            "--expected-sha256", "x" * 64,
            "--dataset", "/d", "--adapter-path", "/a",
            "--n-epochs", "3",
        ])
        assert args.tier == 1
        assert args.mode == "sft"
        assert args.router_aux_loss_coef == pytest.approx(0.002)

    def test_tier_2_parses_with_family(self):
        p = build_parser()
        args = p.parse_args([
            "--tier", "2", "--family", "reasoning-think",
            "--expert-selection-path", str(REASONING_THINK_PROFILE),
            "--base-model", "/m", "--expected-sha256", "x" * 64,
            "--dataset", "/d", "--adapter-path", "/a",
            "--n-epochs", "3",
        ])
        assert args.tier == 2
        assert args.family == "reasoning-think"

    def test_parser_rejects_n_epochs_auto(self):
        p = build_parser()
        with pytest.raises(SystemExit):
            p.parse_args([
                "--tier", "1", "--base-model", "/m",
                "--expected-sha256", "x" * 64,
                "--dataset", "/d", "--adapter-path", "/a",
                "--n-epochs", "auto",
            ])

    def test_parser_rejects_invalid_family(self):
        p = build_parser()
        with pytest.raises(SystemExit):
            p.parse_args([
                "--tier", "2", "--family", "not-a-family",
                "--base-model", "/m", "--expected-sha256", "x" * 64,
                "--dataset", "/d", "--adapter-path", "/a",
                "--n-epochs", "3",
            ])


# ──────────────────────────────────────────────────────────────
# End-to-end subprocess CLI tests — rejection paths
# ──────────────────────────────────────────────────────────────


class TestCliRejectionPaths:
    def test_tier_2_without_family_exits_nonzero(self, tmp_path: Path):
        ds = _tiny_dataset(tmp_path)
        model, sha = _fake_model_dir(tmp_path)
        r = _run_cli([
            "--tier", "2", "--base-model", str(model),
            "--expected-sha256", sha, "--dataset", str(ds),
            "--adapter-path", str(tmp_path / "adapter"),
            "--n-epochs", "3",
        ])
        assert r.returncode != 0
        assert "--tier 2 requires --family" in r.stderr or "--tier 2 requires --family" in r.stdout

    def test_tier_2_without_expert_selection_exits_nonzero(self, tmp_path: Path):
        ds = _tiny_dataset(tmp_path)
        model, sha = _fake_model_dir(tmp_path)
        r = _run_cli([
            "--tier", "2", "--family", "reasoning-think",
            "--base-model", str(model),
            "--expected-sha256", sha, "--dataset", str(ds),
            "--adapter-path", str(tmp_path / "adapter"),
            "--n-epochs", "3",
        ])
        assert r.returncode != 0
        assert "--expert-selection-path" in (r.stderr + r.stdout)

    def test_n_epochs_auto_rejected_cites_ft_oai_001(self, tmp_path: Path):
        ds = _tiny_dataset(tmp_path)
        model, sha = _fake_model_dir(tmp_path)
        r = _run_cli([
            "--tier", "1", "--base-model", str(model),
            "--expected-sha256", sha, "--dataset", str(ds),
            "--adapter-path", str(tmp_path / "adapter"),
            "--n-epochs", "auto",
        ])
        assert r.returncode != 0
        assert "FT-OAI-001" in (r.stderr + r.stdout) or "auto" in (r.stderr + r.stdout)

    def test_epoch_cap_rejection_not_clamp(self, tmp_path: Path):
        """LORA_N_EPOCHS_CAP=3, --n-epochs 5 must reject, not silently clamp."""
        ds = _tiny_dataset(tmp_path)
        model, sha = _fake_model_dir(tmp_path)
        r = _run_cli(
            [
                "--tier", "1", "--base-model", str(model),
                "--expected-sha256", sha, "--dataset", str(ds),
                "--adapter-path", str(tmp_path / "adapter"),
                "--n-epochs", "5",
            ],
            env={"LORA_N_EPOCHS_CAP": "3"},
        )
        assert r.returncode != 0
        combined = r.stderr + r.stdout
        assert "exceeds LORA_N_EPOCHS_CAP" in combined
        assert "FT-OAI-001" in combined

    def test_sha_check_tier1_drift_rejected(self, tmp_path: Path):
        """Tier 1 also enforces --expected-sha256 — symmetric with Tier 2."""
        ds = _tiny_dataset(tmp_path)
        model, _sha = _fake_model_dir(tmp_path)
        r = _run_cli([
            "--tier", "1", "--base-model", str(model),
            "--expected-sha256", "D" * 64,
            "--dataset", str(ds),
            "--adapter-path", str(tmp_path / "adapter"),
            "--n-epochs", "3",
            "--dry-run",
        ])
        assert r.returncode != 0
        combined = r.stderr + r.stdout
        assert "SHA256 drift" in combined or "drift detected" in combined

    def test_sha_check_tier2_drift_rejected(self, tmp_path: Path):
        """Tier 2 also enforces — parity with Tier 1."""
        ds = _tiny_dataset(tmp_path)
        model, _sha = _fake_model_dir(tmp_path)
        r = _run_cli([
            "--tier", "2", "--family", "reasoning-think",
            "--expert-selection-path", str(REASONING_THINK_PROFILE),
            "--base-model", str(model),
            "--expected-sha256", "D" * 64,
            "--dataset", str(ds),
            "--adapter-path", str(tmp_path / "adapter"),
            "--n-epochs", "3",
            "--dry-run",
        ])
        assert r.returncode != 0


# ──────────────────────────────────────────────────────────────
# CLI dry-run — invocation shape
# ──────────────────────────────────────────────────────────────


class TestCliDryRun:
    def test_tier1_dry_run_shape(self, tmp_path: Path):
        ds = _tiny_dataset(tmp_path)
        model, sha = _fake_model_dir(tmp_path)
        r = _run_cli([
            "--tier", "1", "--base-model", str(model),
            "--expected-sha256", sha, "--dataset", str(ds),
            "--adapter-path", str(tmp_path / "adapter"),
            "--n-epochs", "3", "--batch-size", "4",
            "--dry-run",
        ])
        assert r.returncode == 0, r.stderr
        assert "Tier-Aware Training Dry-Run" in r.stdout
        assert "tier: 1" in r.stdout
        assert "target_modules_source: tier1-default" in r.stdout
        assert "target_modules_count: 7" in r.stdout  # 7 tier-1 defaults
        assert "lora_rank: 32" in r.stdout
        assert "lora_alpha: 64" in r.stdout

    def test_tier2_dry_run_includes_7680_keys(self, tmp_path: Path):
        ds = _tiny_dataset(tmp_path)
        model, sha = _fake_model_dir(tmp_path)
        r = _run_cli([
            "--tier", "2", "--family", "reasoning-think",
            "--expert-selection-path", str(REASONING_THINK_PROFILE),
            "--base-model", str(model),
            "--expected-sha256", sha, "--dataset", str(ds),
            "--adapter-path", str(tmp_path / "adapter"),
            "--n-epochs", "3", "--batch-size", "4",
            "--dry-run",
        ])
        assert r.returncode == 0, r.stderr
        assert "target_modules_count: 7680" in r.stdout
        assert "lora_rank: 8" in r.stdout
        assert "lora_alpha: 16" in r.stdout
        assert "language_model.model.layers." in r.stdout
        assert "mlp.experts." in r.stdout

    def test_base_adapter_flag_wired_through_to_argv(self, tmp_path: Path):
        """`--base-adapter` threads into the printed mlx_lm.lora argv."""
        ds = _tiny_dataset(tmp_path)
        model, sha = _fake_model_dir(tmp_path)
        tier1_adapter = tmp_path / "tier1-stub"
        tier1_adapter.mkdir()
        (tier1_adapter / "adapters.safetensors").write_bytes(b"")
        r = _run_cli([
            "--tier", "2", "--family", "reasoning-think",
            "--expert-selection-path", str(REASONING_THINK_PROFILE),
            "--base-adapter", str(tier1_adapter),
            "--base-model", str(model),
            "--expected-sha256", sha, "--dataset", str(ds),
            "--adapter-path", str(tmp_path / "adapter"),
            "--n-epochs", "3", "--batch-size", "4",
            "--dry-run",
        ])
        assert r.returncode == 0, r.stderr
        assert "--resume-adapter-file" in r.stdout
        assert str(tier1_adapter) in r.stdout


# ──────────────────────────────────────────────────────────────
# Constants — sanity checks against policy
# ──────────────────────────────────────────────────────────────


def test_sprint_c_config_sha_pin_is_64_hex():
    assert len(SPRINT_C_CONFIG_SHA) == 64
    assert all(c in "0123456789abcdef" for c in SPRINT_C_CONFIG_SHA)


def test_ft_oai_001_ref_mentions_overfit():
    assert "FT-OAI-001" in FT_OAI_001_REF
    assert "overfit" in FT_OAI_001_REF.lower()


def test_tier1_target_modules_include_attn_and_shared():
    mods = set(TIER1_DEFAULT_TARGET_MODULES)
    assert any("self_attn.q_proj" in m for m in mods)
    assert any("shared_expert" in m for m in mods)
    # NO routed experts in Tier 1 default set
    assert not any("mlp.experts." in m for m in mods)
