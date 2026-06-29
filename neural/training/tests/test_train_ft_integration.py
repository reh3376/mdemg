"""Integration tests for Sprint FT-LORA-E training infra.

These are Tier-2 tests in the three-tier testing plan — they run against the
real Sprint C MLX model path and the real Sprint D profile artifacts, but do
not launch training. They dry-run each CLI path and assert the emitted
mlx_lm.lora argv shape.

Skipped on machines without the Sprint C model present (CI-friendly).
"""

from __future__ import annotations

import hashlib
import json
import os
import subprocess
import sys
from pathlib import Path

import pytest


REPO_ROOT = Path(__file__).resolve().parents[3]
PROFILES_DIR = REPO_ROOT / "training_data" / "routing_profiles"
DEFAULT_MODEL_PATH = Path(
    os.environ.get(
        "SPRINT_C_MODEL_PATH",
        str(Path.home() / ".cache" / "huggingface" / "hub" /
            "models--mlx-community--Qwen3.6-35B-A3B-mxfp4"),
    )
)
SPRINT_C_CONFIG_SHA = (
    "a54ec18ffe24f3c909e9556471dc156ed9b3b61b872008831c7cba9d4768b4a5"
)


def _model_available() -> bool:
    return (DEFAULT_MODEL_PATH / "config.json").exists()


pytestmark = pytest.mark.skipif(
    not _model_available(),
    reason=(
        f"Sprint C model not present at {DEFAULT_MODEL_PATH}. "
        f"Override with SPRINT_C_MODEL_PATH env var."
    ),
)


def _sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            h.update(chunk)
    return h.hexdigest()


def _tiny_dataset(tmp: Path, rows: int = 40) -> Path:
    d = tmp / "ds"
    d.mkdir()
    with (d / "train.jsonl").open("w") as f:
        for i in range(rows):
            f.write(json.dumps({"text": f"integration sample {i}"}) + "\n")
    return d


def _run_cli(args: list[str]) -> subprocess.CompletedProcess:
    return subprocess.run(
        [sys.executable, "-m", "neural.training.train_ft", *args],
        capture_output=True, text=True, cwd=str(REPO_ROOT),
    )


@pytest.fixture(scope="module")
def actual_sha() -> str:
    return _sha256(DEFAULT_MODEL_PATH / "config.json")


def test_sprint_c_model_sha_matches_pin(actual_sha: str):
    """If this fails, the Sprint C model pin drifted from the config-SHA — treat as fixture failure, not product bug."""
    assert actual_sha == SPRINT_C_CONFIG_SHA, (
        f"Sprint C model config.json SHA drift: got {actual_sha}, pin {SPRINT_C_CONFIG_SHA}"
    )


def test_tier1_dry_run_against_real_model(tmp_path: Path, actual_sha: str):
    ds = _tiny_dataset(tmp_path)
    r = _run_cli([
        "--tier", "1", "--base-model", str(DEFAULT_MODEL_PATH),
        "--expected-sha256", actual_sha,
        "--dataset", str(ds),
        "--adapter-path", str(tmp_path / "adapter"),
        "--n-epochs", "3", "--batch-size", "4",
        "--dry-run",
    ])
    assert r.returncode == 0, r.stderr
    assert "target_modules_source: tier1-default" in r.stdout


@pytest.mark.parametrize("family", ["reasoning-think", "classify-notink", "structured-notink"])
def test_tier2_dry_run_all_three_families(tmp_path: Path, actual_sha: str, family: str):
    profile = PROFILES_DIR / f"profile_routing_{family.replace('-', '_')}.json"
    ds = _tiny_dataset(tmp_path)
    r = _run_cli([
        "--tier", "2", "--family", family,
        "--expert-selection-path", str(profile),
        "--base-model", str(DEFAULT_MODEL_PATH),
        "--expected-sha256", actual_sha,
        "--dataset", str(ds),
        "--adapter-path", str(tmp_path / f"adapter-{family}"),
        "--n-epochs", "3", "--batch-size", "4",
        "--dry-run",
    ])
    assert r.returncode == 0, r.stderr
    assert "target_modules_count: 7680" in r.stdout
    assert f"family: {family}" in r.stdout


def test_tier2_keys_match_loaded_model_named_parameters(actual_sha: str):
    """Strict structural test: verify that the `keys` emitted by
    expert_selection.py would match module paths the loaded Qwen3 model
    exposes. Skipped if mlx_lm or the model can't be loaded.
    """
    mlx_load = pytest.importorskip("mlx_lm").load
    from training.expert_selection import ExpertSelection  # type: ignore

    profile = PROFILES_DIR / "profile_routing_reasoning_think.json"
    sel = ExpertSelection.load(profile, expected_family="reasoning_think")
    emitted = set(sel.mlx_lm_keys())

    # Load the model ONLY to inspect its named_parameters() keys.
    # This is a heavy op; only run if explicitly requested via env flag.
    if os.environ.get("SPRINT_E_HEAVY_INTEGRATION", "0") != "1":
        pytest.skip(
            "Set SPRINT_E_HEAVY_INTEGRATION=1 to load full Qwen3.6 model for "
            "named_parameters structural match (35B model = multi-GB VRAM)."
        )

    model, _tok = mlx_load(str(DEFAULT_MODEL_PATH))
    model_keys = set()
    for path, _param in model.named_parameters():
        # Strip terminal .weight/.bias for module-level comparison.
        if path.endswith(".weight") or path.endswith(".bias"):
            model_keys.add(path.rsplit(".", 1)[0])

    missing = emitted - model_keys
    assert not missing, (
        f"{len(missing)} emitted Tier-2 keys do not match any loaded model "
        f"parameter path. First 5: {sorted(missing)[:5]}"
    )
