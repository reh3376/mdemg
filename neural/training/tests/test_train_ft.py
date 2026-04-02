"""Tests for the LoRA fine-tuning script."""

import json
import os
import tempfile

import pytest

from training.train_ft import (
    MIN_EXOGENOUS_RATIO,
    build_train_config,
    load_manifest,
    resolve_lora_rank,
    validate_manifest,
)


def _valid_manifest(**overrides):
    """Build a valid manifest dict for testing."""
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
        "config": {
            "raft_ratio": 0.5,
        },
    }
    base.update(overrides)
    return base


def _write_manifest(tmpdir, manifest):
    """Write manifest.json to tmpdir."""
    path = os.path.join(tmpdir, "manifest.json")
    with open(path, "w") as f:
        json.dump(manifest, f)
    return path


def _write_train_jsonl(tmpdir, rows=10):
    """Write a minimal train.jsonl to tmpdir."""
    path = os.path.join(tmpdir, "train.jsonl")
    with open(path, "w") as f:
        for i in range(rows):
            record = {"messages": [
                {"role": "system", "content": "You are a classifier."},
                {"role": "user", "content": f"Input {i}"},
                {"role": "assistant", "content": f"Output {i}"},
            ]}
            f.write(json.dumps(record) + "\n")
    return path


# ── validate_manifest tests ──


class TestValidateManifest:
    def test_valid_manifest(self):
        manifest = _valid_manifest()
        errors = validate_manifest(manifest)
        assert errors == []

    def test_missing_train_split(self):
        manifest = _valid_manifest(splits={"test": {"file": "test.jsonl", "rows": 50}})
        errors = validate_manifest(manifest)
        assert any("splits.train" in e for e in errors)

    def test_zero_train_rows(self):
        manifest = _valid_manifest(
            splits={"train": {"file": "train.jsonl", "rows": 0}}
        )
        errors = validate_manifest(manifest)
        assert any("0 rows" in e for e in errors)

    def test_low_exogenous_ratio(self):
        manifest = _valid_manifest(
            quality_gates={
                "exogenous_ratio": 0.2,
                "exogenous_ratio_met": False,
                "no_train_test_overlap": True,
            }
        )
        errors = validate_manifest(manifest)
        assert any("anti-collapse" in e for e in errors)
        assert any("0.2000" in e for e in errors)

    def test_exogenous_gate_not_met(self):
        manifest = _valid_manifest(
            quality_gates={
                "exogenous_ratio": 0.45,
                "exogenous_ratio_met": False,
                "no_train_test_overlap": True,
            }
        )
        errors = validate_manifest(manifest)
        # Above MIN_EXOGENOUS_RATIO but gate not met
        assert any("manifest gate failed" in e for e in errors)
        # Should NOT trigger anti-collapse (0.45 >= 0.4)
        assert not any("anti-collapse" in e for e in errors)

    def test_train_test_overlap(self):
        manifest = _valid_manifest(
            quality_gates={
                "exogenous_ratio": 0.6,
                "exogenous_ratio_met": True,
                "no_train_test_overlap": False,
            }
        )
        errors = validate_manifest(manifest)
        assert any("overlap" in e for e in errors)

    def test_multiple_errors(self):
        manifest = _valid_manifest(
            splits={"train": {"file": "train.jsonl", "rows": 0}},
            quality_gates={
                "exogenous_ratio": 0.1,
                "exogenous_ratio_met": False,
                "no_train_test_overlap": False,
            },
        )
        errors = validate_manifest(manifest)
        assert len(errors) >= 3  # zero rows + exogenous + overlap


# ── load_manifest tests ──


class TestLoadManifest:
    def test_load_valid(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            manifest = _valid_manifest()
            _write_manifest(tmpdir, manifest)
            loaded = load_manifest(tmpdir)
            assert loaded["dataset_id"] == "test-dataset"
            assert loaded["version"] == "v1"

    def test_missing_manifest(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            with pytest.raises(FileNotFoundError):
                load_manifest(tmpdir)


# ── resolve_lora_rank tests ──


class TestResolveLoraRank:
    def test_cli_override(self):
        rank = resolve_lora_rank("/nonexistent", cli_rank=32, ults_dir=None)
        assert rank == 32

    def test_default_when_no_ults(self):
        rank = resolve_lora_rank("/nonexistent", cli_rank=None, ults_dir=None)
        assert rank == 16

    def test_default_when_ults_dir_missing(self):
        rank = resolve_lora_rank("/nonexistent", cli_rank=None, ults_dir="/nonexistent")
        assert rank == 16

    def test_reads_max_from_ults_specs(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            # Create two ULTS spec files with different ranks
            spec_a = {"training_config": {"rank": 8}}
            spec_b = {"training_config": {"rank": 24}}
            with open(os.path.join(tmpdir, "task_a.ults.json"), "w") as f:
                json.dump(spec_a, f)
            with open(os.path.join(tmpdir, "task_b.ults.json"), "w") as f:
                json.dump(spec_b, f)

            rank = resolve_lora_rank("/nonexistent", cli_rank=None, ults_dir=tmpdir)
            assert rank == 24

    def test_cli_overrides_ults(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            spec = {"training_config": {"rank": 24}}
            with open(os.path.join(tmpdir, "task.ults.json"), "w") as f:
                json.dump(spec, f)

            rank = resolve_lora_rank("/nonexistent", cli_rank=8, ults_dir=tmpdir)
            assert rank == 8

    def test_ults_spec_without_rank(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            spec = {"training_config": {}}
            with open(os.path.join(tmpdir, "task.ults.json"), "w") as f:
                json.dump(spec, f)

            rank = resolve_lora_rank("/nonexistent", cli_rank=None, ults_dir=tmpdir)
            assert rank == 16  # Falls back to default


# ── build_train_config tests ──


class TestBuildTrainConfig:
    def test_basic_config(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            _write_train_jsonl(tmpdir, rows=100)

            config = build_train_config(
                dataset_dir=tmpdir,
                base_model="mlx-community/Qwen3-30B-A3B-4bit",
                adapter_path="/tmp/adapters",
                epochs=3,
                batch_size=4,
                learning_rate=1e-5,
                lora_rank=16,
                lora_layers=16,
                max_seq_length=2048,
            )

            assert config["model"] == "mlx-community/Qwen3-30B-A3B-4bit"
            assert config["adapter_path"] == "/tmp/adapters"
            assert config["batch_size"] == 4
            assert config["learning_rate"] == 1e-5
            assert config["lora_rank"] == 16
            assert config["lora_layers"] == 16
            assert config["max_seq_length"] == 2048
            assert config["train"] is True

    def test_iters_calculation(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            _write_train_jsonl(tmpdir, rows=100)

            config = build_train_config(
                dataset_dir=tmpdir,
                base_model="test-model",
                adapter_path="/tmp/adapters",
                epochs=3,
                batch_size=4,
                learning_rate=1e-5,
                lora_rank=16,
                lora_layers=16,
                max_seq_length=2048,
            )

            # 3 epochs * 100 rows / 4 batch_size = 75
            assert config["iters"] == 75

    def test_test_split_detected(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            _write_train_jsonl(tmpdir, rows=10)
            # Create test.jsonl
            test_path = os.path.join(tmpdir, "test.jsonl")
            with open(test_path, "w") as f:
                f.write('{"messages": []}\n')

            config = build_train_config(
                dataset_dir=tmpdir,
                base_model="test-model",
                adapter_path="/tmp/adapters",
                epochs=1,
                batch_size=1,
                learning_rate=1e-5,
                lora_rank=16,
                lora_layers=16,
                max_seq_length=2048,
            )

            assert config["test"] is True
            assert config["test_batches"] == 10
            assert config["val_data"] == test_path

    def test_no_test_split(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            _write_train_jsonl(tmpdir, rows=10)

            config = build_train_config(
                dataset_dir=tmpdir,
                base_model="test-model",
                adapter_path="/tmp/adapters",
                epochs=1,
                batch_size=1,
                learning_rate=1e-5,
                lora_rank=16,
                lora_layers=16,
                max_seq_length=2048,
            )

            assert "test" not in config
            assert "val_data" not in config


# ── Anti-collapse constant ──


class TestAntiCollapse:
    def test_min_exogenous_ratio_value(self):
        assert MIN_EXOGENOUS_RATIO == 0.4

    def test_boundary_exactly_at_minimum(self):
        manifest = _valid_manifest(
            quality_gates={
                "exogenous_ratio": 0.4,
                "exogenous_ratio_met": True,
                "no_train_test_overlap": True,
            }
        )
        errors = validate_manifest(manifest)
        assert errors == []

    def test_boundary_just_below_minimum(self):
        manifest = _valid_manifest(
            quality_gates={
                "exogenous_ratio": 0.399,
                "exogenous_ratio_met": True,
                "no_train_test_overlap": True,
            }
        )
        errors = validate_manifest(manifest)
        assert any("anti-collapse" in e for e in errors)
