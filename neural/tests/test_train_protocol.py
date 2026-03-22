"""Tests for the protocol tier training pipeline."""

from __future__ import annotations

import json
import os
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from neural_sidecar.train_protocol import (
    ProtocolTrainingSample,
    derive_optimal_tiers,
    load_protocol_jsonl,
    train_protocol,
)
from neural_sidecar.train import get_next_version


def _write_protocol_jsonl(path: str, records: list[dict]) -> None:
    """Helper to write protocol JSONL data to a file."""
    with open(path, "w", encoding="utf-8") as f:
        for record in records:
            f.write(json.dumps(record) + "\n")


@pytest.fixture
def protocol_records() -> list[dict]:
    """Well-formed J17 protocol JSONL records."""
    return [
        {
            "timestamp": "2026-03-20T10:00:00Z",
            "constraint_code": "test-first",
            "constraint_text": "always run tests before commit",
            "tier_used": 1,
            "token_count": 15,
            "agent_action": "ran pytest",
            "outcome": "followed",
            "comprehension_score": 1.0,
            "trust_score": 0.8,
            "session_id": "sess-1",
        },
        {
            "timestamp": "2026-03-20T10:01:00Z",
            "constraint_code": "test-first",
            "constraint_text": "always run tests before commit",
            "tier_used": 2,
            "token_count": 45,
            "agent_action": "ran pytest",
            "outcome": "followed",
            "comprehension_score": 1.0,
            "trust_score": 0.7,
            "session_id": "sess-2",
        },
        {
            "timestamp": "2026-03-20T10:02:00Z",
            "constraint_code": "no-force-push",
            "constraint_text": "never force push to main",
            "tier_used": 1,
            "token_count": 12,
            "agent_action": "used regular push",
            "outcome": "followed",
            "comprehension_score": 0.9,
            "trust_score": 0.6,
            "session_id": "sess-1",
        },
    ]


def test_load_protocol_jsonl_valid(protocol_records: list[dict]) -> None:
    """Load well-formed protocol JSONL data."""
    with tempfile.TemporaryDirectory() as tmpdir:
        _write_protocol_jsonl(os.path.join(tmpdir, "protocol-data.jsonl"), protocol_records)

        samples = load_protocol_jsonl(tmpdir)

        assert len(samples) == 3
        assert samples[0].constraint_code == "test-first"
        assert samples[0].tier_used == 1
        assert samples[0].comprehension_score == 1.0
        assert samples[0].trust_score == 0.8
        assert samples[2].constraint_code == "no-force-push"


def test_load_protocol_jsonl_malformed() -> None:
    """Malformed lines are skipped, valid lines are kept."""
    with tempfile.TemporaryDirectory() as tmpdir:
        filepath = os.path.join(tmpdir, "mixed.jsonl")
        with open(filepath, "w", encoding="utf-8") as f:
            # Valid line
            f.write(json.dumps({
                "constraint_code": "c1",
                "constraint_text": "text",
                "tier_used": 1,
                "token_count": 10,
                "comprehension_score": 0.9,
                "trust_score": 0.5,
            }) + "\n")
            # Malformed JSON
            f.write("not valid json\n")
            # Missing required field (constraint_code)
            f.write(json.dumps({"tier_used": 2}) + "\n")
            # Another valid line
            f.write(json.dumps({
                "constraint_code": "c2",
                "constraint_text": "text2",
                "tier_used": 3,
                "token_count": 80,
                "comprehension_score": 1.0,
                "trust_score": 0.7,
            }) + "\n")

        samples = load_protocol_jsonl(tmpdir)

        assert len(samples) == 2
        assert samples[0].constraint_code == "c1"
        assert samples[1].constraint_code == "c2"


def test_load_protocol_jsonl_empty() -> None:
    """Empty directory returns empty list."""
    with tempfile.TemporaryDirectory() as tmpdir:
        samples = load_protocol_jsonl(tmpdir)
        assert samples == []


def test_derive_optimal_tiers() -> None:
    """Verify tier selection logic: prefer tier with best comprehension/token ratio."""
    samples = [
        # T1: high comprehension, low tokens → high efficiency
        ProtocolTrainingSample(
            timestamp="", constraint_code="code1", constraint_text="constraint",
            tier_used=1, token_count=15, agent_action="action",
            outcome="followed", comprehension_score=1.0, trust_score=0.8,
        ),
        # T3: high comprehension, high tokens → low efficiency
        ProtocolTrainingSample(
            timestamp="", constraint_code="code1", constraint_text="constraint",
            tier_used=3, token_count=80, agent_action="action",
            outcome="followed", comprehension_score=1.0, trust_score=0.8,
        ),
    ]

    labeled = derive_optimal_tiers(samples)

    assert len(labeled) == 2
    # T1 should be optimal (1.0/15 > 1.0/80)
    # The sample that matches optimal tier should have score 1.0
    t1_entry = [entry for entry in labeled if entry[3] == 1]
    assert len(t1_entry) > 0
    # The T1 sample should have score 1.0 (matching optimal)
    t1_scores = [entry[2] for entry in labeled if entry[3] == 1]
    assert 1.0 in t1_scores


def test_derive_optimal_tiers_single_tier() -> None:
    """When all samples use the same tier, that tier is optimal."""
    samples = [
        ProtocolTrainingSample(
            timestamp="", constraint_code="code1", constraint_text="text",
            tier_used=2, token_count=45, agent_action="",
            outcome="followed", comprehension_score=0.8, trust_score=0.5,
        ),
        ProtocolTrainingSample(
            timestamp="", constraint_code="code1", constraint_text="text",
            tier_used=2, token_count=50, agent_action="",
            outcome="followed", comprehension_score=0.9, trust_score=0.6,
        ),
    ]

    labeled = derive_optimal_tiers(samples)

    # All entries should have optimal tier = 2 and score = 1.0
    assert all(entry[3] == 2 for entry in labeled)
    assert all(entry[2] == 1.0 for entry in labeled)


def test_train_creates_model(protocol_records: list[dict]) -> None:
    """Training creates a versioned output directory with metadata."""
    with tempfile.TemporaryDirectory() as tmpdir:
        data_dir = os.path.join(tmpdir, "data")
        model_dir = os.path.join(tmpdir, "models")
        os.makedirs(data_dir)

        # Write enough samples (min_samples=10 for test)
        records = protocol_records * 40  # 120 samples
        _write_protocol_jsonl(os.path.join(data_dir, "protocol.jsonl"), records)

        # Mock the heavy ML objects
        mock_model = MagicMock()
        mock_model.predict.return_value = [0.5] * 12

        mock_evaluator = MagicMock()
        mock_evaluator.return_value = 0.70

        mock_dataloader = MagicMock()
        mock_dataloader.__len__ = MagicMock(return_value=7)

        with patch("sentence_transformers.CrossEncoder", return_value=mock_model), \
             patch("sentence_transformers.cross_encoder.evaluation.CECorrelationEvaluator", return_value=mock_evaluator), \
             patch("torch.utils.data.DataLoader", return_value=mock_dataloader), \
             patch("sentence_transformers.InputExample", MagicMock(side_effect=lambda texts, label: MagicMock(texts=texts, label=label))):
            result = train_protocol(
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

        # Check metadata file
        metadata_path = os.path.join(result.model_path, "training_metadata.json")
        assert Path(metadata_path).exists()
        with open(metadata_path) as f:
            metadata = json.load(f)
        assert metadata["version"] == 1
        assert metadata["pipeline"] == "protocol-tier"


def test_min_samples_skip() -> None:
    """Training is skipped when sample count is below threshold."""
    with tempfile.TemporaryDirectory() as tmpdir:
        data_dir = os.path.join(tmpdir, "data")
        model_dir = os.path.join(tmpdir, "models")
        os.makedirs(data_dir)

        # Write only 3 samples (below default min_samples=500)
        records = [
            {"constraint_code": f"c{i}", "constraint_text": f"text{i}", "tier_used": 1,
             "token_count": 15, "comprehension_score": 0.9, "trust_score": 0.5}
            for i in range(3)
        ]
        _write_protocol_jsonl(os.path.join(data_dir, "protocol.jsonl"), records)

        result = train_protocol(
            data_dir=data_dir,
            model_dir=model_dir,
            min_samples=500,
        )

        assert result.skipped
        assert "Insufficient samples" in result.skip_reason
        assert result.num_samples == 3
        assert result.version == 0
        assert result.model_path == ""


def test_versioning() -> None:
    """Version numbering increments correctly."""
    with tempfile.TemporaryDirectory() as tmpdir:
        os.makedirs(os.path.join(tmpdir, "v1"))
        os.makedirs(os.path.join(tmpdir, "v2"))
        assert get_next_version(tmpdir) == 3

        # Non-existent directory
        assert get_next_version(os.path.join(tmpdir, "nonexistent")) == 1
