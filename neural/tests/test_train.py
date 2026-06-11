"""Tests for the training pipeline."""

from __future__ import annotations

import json
import os
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from neural_sidecar.train import (
    TrainingSample,
    create_training_pairs,
    get_next_version,
    load_jsonl,
    train,
    update_current_symlink,
)


def _write_jsonl(path: str, records: list[dict]) -> None:
    """Helper to write JSONL data to a file."""
    with open(path, "w", encoding="utf-8") as f:
        for record in records:
            f.write(json.dumps(record) + "\n")


@pytest.fixture
def sample_records() -> list[dict]:
    """Well-formed JSONL records."""
    return [
        {"query": "error handling", "candidate": "try-except patterns", "score": 0.9, "timestamp": "2026-03-19T10:00:00Z", "space_id": "test"},
        {"query": "logging setup", "candidate": "python logging module", "score": 0.7, "timestamp": "2026-03-19T10:01:00Z", "space_id": "test"},
        {"query": "retry logic", "candidate": "exponential backoff", "score": 0.85, "timestamp": "2026-03-19T10:02:00Z", "space_id": "test"},
    ]


def test_load_jsonl_valid(sample_records: list[dict]) -> None:
    """Load well-formed JSONL data and verify all samples are returned."""
    with tempfile.TemporaryDirectory() as tmpdir:
        _write_jsonl(os.path.join(tmpdir, "data.jsonl"), sample_records)

        samples = load_jsonl(tmpdir)

        assert len(samples) == 3
        assert samples[0].query == "error handling"
        assert samples[0].candidate == "try-except patterns"
        assert samples[0].score == 0.9
        assert samples[0].space_id == "test"
        assert samples[1].query == "logging setup"
        assert samples[2].score == 0.85


def test_load_jsonl_empty() -> None:
    """Handle empty JSONL file gracefully — returns empty list."""
    with tempfile.TemporaryDirectory() as tmpdir:
        # Create an empty file
        empty_path = os.path.join(tmpdir, "empty.jsonl")
        Path(empty_path).touch()

        samples = load_jsonl(tmpdir)

        assert samples == []


def test_load_jsonl_malformed() -> None:
    """Malformed lines are skipped, valid lines are kept."""
    with tempfile.TemporaryDirectory() as tmpdir:
        filepath = os.path.join(tmpdir, "mixed.jsonl")
        with open(filepath, "w", encoding="utf-8") as f:
            # Valid line
            f.write(json.dumps({"query": "q1", "candidate": "c1", "score": 0.5}) + "\n")
            # Malformed JSON
            f.write("not valid json\n")
            # Missing required field
            f.write(json.dumps({"query": "q2"}) + "\n")
            # Invalid score type
            f.write(json.dumps({"query": "q3", "candidate": "c3", "score": "bad"}) + "\n")
            # Another valid line
            f.write(json.dumps({"query": "q4", "candidate": "c4", "score": 0.8}) + "\n")

        samples = load_jsonl(tmpdir)

        assert len(samples) == 2
        assert samples[0].query == "q1"
        assert samples[1].query == "q4"


def test_create_training_pairs() -> None:
    """Convert TrainingSample objects to InputExample pairs."""
    samples = [
        TrainingSample(query="q1", candidate="c1", score=0.9),
        TrainingSample(query="q2", candidate="c2", score=0.3),
    ]

    # InputExample is imported inside create_training_pairs from sentence_transformers.
    # We patch it at its origin so the local import picks up the mock.
    mock_ie_class = MagicMock()
    mock_ie_class.side_effect = lambda texts, label: MagicMock(texts=texts, label=label)

    with patch("sentence_transformers.InputExample", mock_ie_class):
        pairs = create_training_pairs(samples)

    assert len(pairs) == 2
    assert pairs[0].texts == ["q1", "c1"]
    assert pairs[0].label == 0.9
    assert pairs[1].texts == ["q2", "c2"]
    assert pairs[1].label == 0.3


def test_train_creates_model_dir(sample_records: list[dict]) -> None:
    """Training creates a versioned output directory with metadata."""
    with tempfile.TemporaryDirectory() as tmpdir:
        data_dir = os.path.join(tmpdir, "data")
        model_dir = os.path.join(tmpdir, "models")
        os.makedirs(data_dir)

        # Write enough samples to pass min_samples threshold
        records = sample_records * 40  # 120 samples
        _write_jsonl(os.path.join(data_dir, "train.jsonl"), records)

        # Mock the heavy ML objects
        mock_model = MagicMock()
        mock_model.predict.return_value = [0.5] * 12  # val set size

        mock_evaluator = MagicMock()
        mock_evaluator.return_value = 0.75  # spearman score

        mock_dataloader = MagicMock()
        # DataLoader needs __len__ for warmup_steps calculation
        mock_dataloader.__len__ = MagicMock(return_value=7)

        # Patch at the source libraries since train() imports locally
        with patch("sentence_transformers.CrossEncoder", return_value=mock_model), \
             patch("sentence_transformers.cross_encoder.evaluation.CECorrelationEvaluator", return_value=mock_evaluator), \
             patch("torch.utils.data.DataLoader", return_value=mock_dataloader), \
             patch("sentence_transformers.InputExample", MagicMock(side_effect=lambda texts, label: MagicMock(texts=texts, label=label))):
            result = train(
                data_dir=data_dir,
                model_dir=model_dir,
                epochs=1,
                batch_size=16,
                min_samples=10,
            )

        assert not result.skipped
        assert result.version == 1
        assert result.model_path == os.path.join(model_dir, "v1")
        assert Path(result.model_path).exists()

        # Check metadata file was written
        metadata_path = os.path.join(result.model_path, "training_metadata.json")
        assert Path(metadata_path).exists()
        with open(metadata_path) as f:
            metadata = json.load(f)
        assert metadata["version"] == 1
        assert metadata["epochs"] == 1


def test_model_versioning() -> None:
    """Version numbering increments correctly and symlink is updated."""
    with tempfile.TemporaryDirectory() as tmpdir:
        # Create existing version directories
        os.makedirs(os.path.join(tmpdir, "v1"))
        os.makedirs(os.path.join(tmpdir, "v2"))

        # Next version should be 3
        assert get_next_version(tmpdir) == 3

        # Update symlink
        update_current_symlink(tmpdir, 3)
        current = Path(tmpdir) / "current"
        assert current.is_symlink()
        assert os.readlink(str(current)) == "v3"

        # Update symlink again (should replace)
        os.makedirs(os.path.join(tmpdir, "v3"))
        update_current_symlink(tmpdir, 4)
        assert os.readlink(str(current)) == "v4"

        # Empty directory — version starts at 1
        with tempfile.TemporaryDirectory() as empty_dir:
            assert get_next_version(empty_dir) == 1

        # Non-existent directory — version starts at 1
        assert get_next_version(os.path.join(tmpdir, "nonexistent")) == 1


def test_train_from_checkpoint(sample_records: list[dict]) -> None:
    """Passing from_checkpoint uses that path as the source model instead of base_model."""
    with tempfile.TemporaryDirectory() as tmpdir:
        data_dir = os.path.join(tmpdir, "data")
        model_dir = os.path.join(tmpdir, "models")
        os.makedirs(data_dir)

        records = sample_records * 40  # 120 samples
        _write_jsonl(os.path.join(data_dir, "train.jsonl"), records)

        mock_model = MagicMock()
        mock_model.predict.return_value = [0.5] * 12

        mock_evaluator = MagicMock()
        mock_evaluator.return_value = 0.75

        mock_dataloader = MagicMock()
        mock_dataloader.__len__ = MagicMock(return_value=7)

        checkpoint_path = "/some/model/v1"

        captured_source: list[str] = []

        def capturing_cross_encoder(source, **kwargs):
            captured_source.append(source)
            return mock_model

        with patch("sentence_transformers.CrossEncoder", side_effect=capturing_cross_encoder), \
             patch("sentence_transformers.cross_encoder.evaluation.CECorrelationEvaluator", return_value=mock_evaluator), \
             patch("torch.utils.data.DataLoader", return_value=mock_dataloader), \
             patch("sentence_transformers.InputExample", MagicMock(side_effect=lambda texts, label: MagicMock(texts=texts, label=label))):
            result = train(
                data_dir=data_dir,
                model_dir=model_dir,
                base_model="cross-encoder/ms-marco-MiniLM-L-6-v2",
                epochs=1,
                batch_size=16,
                min_samples=10,
                from_checkpoint=checkpoint_path,
            )

        # The checkpoint path — not base_model — must have been passed to CrossEncoder
        assert captured_source[0] == checkpoint_path
        assert not result.skipped
        assert result.version == 1


def test_min_samples_skip() -> None:
    """Training is skipped when sample count is below threshold."""
    with tempfile.TemporaryDirectory() as tmpdir:
        data_dir = os.path.join(tmpdir, "data")
        model_dir = os.path.join(tmpdir, "models")
        os.makedirs(data_dir)

        # Write only 5 samples (below default min_samples=100)
        records = [
            {"query": f"q{i}", "candidate": f"c{i}", "score": 0.5}
            for i in range(5)
        ]
        _write_jsonl(os.path.join(data_dir, "train.jsonl"), records)

        result = train(
            data_dir=data_dir,
            model_dir=model_dir,
            min_samples=100,
        )

        assert result.skipped
        assert "Insufficient samples" in result.skip_reason
        assert result.num_samples == 5
        assert result.version == 0
        assert result.model_path == ""
        # Model directory should not have been created
        assert not Path(model_dir).exists()
