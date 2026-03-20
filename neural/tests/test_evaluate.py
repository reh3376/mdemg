"""Tests for the evaluation pipeline."""

from __future__ import annotations

import json
import math
import os
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from neural_sidecar.evaluate import (
    EvalStats,
    ModelEvalResult,
    compute_stats,
    dcg_at_k,
    evaluate_model,
    load_test_data,
    ndcg_at_k,
    spearman_rank_correlation,
)
from neural_sidecar.train import TrainingSample


def _write_jsonl(path: str, records: list[dict]) -> None:
    """Helper to write JSONL data to a file."""
    with open(path, "w", encoding="utf-8") as f:
        for record in records:
            f.write(json.dumps(record) + "\n")


@pytest.fixture
def sample_records() -> list[dict]:
    """Well-formed JSONL records for evaluate tests."""
    return [
        {"query": "error handling", "candidate": "try-except patterns", "score": 0.9, "timestamp": "2026-03-19T10:00:00Z", "space_id": "test"},
        {"query": "logging setup", "candidate": "python logging module", "score": 0.7, "timestamp": "2026-03-19T10:01:00Z", "space_id": "test"},
        {"query": "retry logic", "candidate": "exponential backoff", "score": 0.85, "timestamp": "2026-03-19T10:02:00Z", "space_id": "test"},
    ]


def test_compute_stats() -> None:
    """Verify mean, std, min, max, and median for a known set of values."""
    values = [1.0, 2.0, 3.0, 4.0, 5.0]
    stats = compute_stats(values)

    assert stats.count == 5
    assert math.isclose(stats.mean, 3.0)
    assert math.isclose(stats.min, 1.0)
    assert math.isclose(stats.max, 5.0)
    assert math.isclose(stats.median, 3.0)
    # Population std for [1,2,3,4,5]: sqrt(2) ≈ 1.4142
    assert math.isclose(stats.std, math.sqrt(2), rel_tol=1e-5)


def test_compute_stats_even_count() -> None:
    """Median for even-count list is the average of the two middle values."""
    values = [1.0, 2.0, 3.0, 4.0]
    stats = compute_stats(values)

    assert stats.count == 4
    assert math.isclose(stats.median, 2.5)


def test_compute_stats_empty() -> None:
    """Empty input returns a zeroed EvalStats."""
    stats = compute_stats([])
    assert stats.count == 0
    assert stats.mean == 0.0
    assert stats.std == 0.0
    assert stats.min == 0.0
    assert stats.max == 0.0
    assert stats.median == 0.0


def test_spearman_rank_correlation() -> None:
    """Perfect positive rank correlation returns 1.0."""
    x = [1.0, 2.0, 3.0, 4.0, 5.0]
    y = [2.0, 4.0, 6.0, 8.0, 10.0]  # strictly monotone with x

    corr = spearman_rank_correlation(x, y)

    assert math.isclose(corr, 1.0, rel_tol=1e-5)


def test_spearman_rank_correlation_negative() -> None:
    """Perfect negative rank correlation returns -1.0."""
    x = [1.0, 2.0, 3.0, 4.0, 5.0]
    y = [5.0, 4.0, 3.0, 2.0, 1.0]

    corr = spearman_rank_correlation(x, y)

    assert math.isclose(corr, -1.0, rel_tol=1e-5)


def test_spearman_rank_correlation_too_short() -> None:
    """Single-element list returns 0.0 (undefined correlation)."""
    assert spearman_rank_correlation([1.0], [1.0]) == 0.0


def test_dcg_at_k() -> None:
    """Verify DCG calculation against manually computed values.

    For scores [3, 2, 1] and k=3:
      DCG = 3/log2(2) + 2/log2(3) + 1/log2(4)
           = 3/1 + 2/1.585 + 1/2
           ≈ 3.0 + 1.2619 + 0.5 = 4.7619
    """
    scores = [3.0, 2.0, 1.0]
    expected = 3.0 / math.log2(2) + 2.0 / math.log2(3) + 1.0 / math.log2(4)

    result = dcg_at_k(scores, k=3)

    assert math.isclose(result, expected, rel_tol=1e-5)


def test_dcg_at_k_truncates_at_k() -> None:
    """Only the first k elements contribute to DCG."""
    # With k=1 only the first score matters
    scores = [4.0, 0.0, 0.0]
    result_k1 = dcg_at_k(scores, k=1)
    expected = 4.0 / math.log2(2)

    assert math.isclose(result_k1, expected, rel_tol=1e-5)


def test_ndcg_at_k() -> None:
    """Perfect neural ranking against LLM ground truth gives NDCG = 1.0."""
    # When neural scores rank items in the same order as llm_scores, NDCG = 1.0
    llm_scores = [0.9, 0.7, 0.5, 0.3]
    # Neural scores have the same ranking order
    neural_scores = [0.95, 0.75, 0.55, 0.35]

    result = ndcg_at_k(neural_scores, llm_scores, k=4)

    assert math.isclose(result, 1.0, rel_tol=1e-5)


def test_ndcg_at_k_worst_case() -> None:
    """Reversed neural ranking against LLM ground truth gives NDCG < 1.0."""
    llm_scores = [0.9, 0.7, 0.5, 0.3]
    # Neural scores rank items in reverse order
    neural_scores = [0.35, 0.55, 0.75, 0.95]

    result = ndcg_at_k(neural_scores, llm_scores, k=4)

    assert result < 1.0


def test_ndcg_at_k_empty() -> None:
    """Empty inputs return 0.0."""
    assert ndcg_at_k([], [], k=10) == 0.0
    assert ndcg_at_k([0.5], [], k=10) == 0.0
    assert ndcg_at_k([], [0.5], k=10) == 0.0


def test_load_test_data_file(sample_records: list[dict]) -> None:
    """Single JSONL file is loaded correctly."""
    with tempfile.TemporaryDirectory() as tmpdir:
        filepath = os.path.join(tmpdir, "test.jsonl")
        _write_jsonl(filepath, sample_records)

        samples = load_test_data(filepath)

        assert len(samples) == 3
        assert samples[0].query == "error handling"
        assert samples[0].score == 0.9
        assert samples[1].query == "logging setup"
        assert samples[2].candidate == "exponential backoff"


def test_load_test_data_file_skips_malformed() -> None:
    """Malformed lines in a single file are skipped gracefully."""
    with tempfile.TemporaryDirectory() as tmpdir:
        filepath = os.path.join(tmpdir, "mixed.jsonl")
        with open(filepath, "w", encoding="utf-8") as f:
            f.write(json.dumps({"query": "q1", "candidate": "c1", "score": 0.5}) + "\n")
            f.write("not valid json\n")
            f.write(json.dumps({"query": "q2", "candidate": "c2", "score": 0.8}) + "\n")

        samples = load_test_data(filepath)

        assert len(samples) == 2
        assert samples[0].query == "q1"
        assert samples[1].query == "q2"


def test_load_test_data_directory(sample_records: list[dict]) -> None:
    """Directory with multiple JSONL files — all records are loaded."""
    with tempfile.TemporaryDirectory() as tmpdir:
        _write_jsonl(os.path.join(tmpdir, "part1.jsonl"), sample_records[:2])
        _write_jsonl(os.path.join(tmpdir, "part2.jsonl"), sample_records[2:])

        samples = load_test_data(tmpdir)

        assert len(samples) == 3


def test_evaluate_model() -> None:
    """Mock CrossEncoder.predict and verify scores, metrics, and stats are returned."""
    samples = [
        TrainingSample(query="q1", candidate="c1", score=0.9),
        TrainingSample(query="q2", candidate="c2", score=0.7),
        TrainingSample(query="q3", candidate="c3", score=0.5),
        TrainingSample(query="q4", candidate="c4", score=0.3),
    ]
    # Neural scores in same order as LLM scores — expect spearman ≈ 1.0, ndcg ≈ 1.0
    mock_neural_scores = [0.95, 0.75, 0.55, 0.35]

    mock_model = MagicMock()
    mock_model.predict.return_value = mock_neural_scores

    with patch("sentence_transformers.CrossEncoder", return_value=mock_model):
        result = evaluate_model(
            model_path="/fake/model",
            samples=samples,
            top_k=4,
        )

    assert result.model_path == "/fake/model"
    assert result.num_samples == 4
    assert result.top_k == 4
    assert math.isclose(result.spearman_correlation, 1.0, rel_tol=1e-5)
    assert math.isclose(result.ndcg_at_k, 1.0, rel_tol=1e-5)

    # Verify stats were computed for both neural and LLM scores
    assert result.neural_stats.count == 4
    assert result.llm_stats.count == 4
    assert math.isclose(result.neural_stats.mean, sum(mock_neural_scores) / 4, rel_tol=1e-5)
    assert math.isclose(result.llm_stats.mean, (0.9 + 0.7 + 0.5 + 0.3) / 4, rel_tol=1e-5)

    # Verify predict was called with the correct pairs
    mock_model.predict.assert_called_once_with([("q1", "c1"), ("q2", "c2"), ("q3", "c3"), ("q4", "c4")])


def test_evaluate_empty_data() -> None:
    """evaluate_model with no samples: predict is not called, stats are zeroed."""
    mock_model = MagicMock()
    mock_model.predict.return_value = []

    with patch("sentence_transformers.CrossEncoder", return_value=mock_model):
        result = evaluate_model(
            model_path="/fake/model",
            samples=[],
            top_k=10,
        )

    assert result.num_samples == 0
    assert result.spearman_correlation == 0.0
    assert result.ndcg_at_k == 0.0
    assert result.neural_stats.count == 0
    assert result.llm_stats.count == 0
    mock_model.predict.assert_called_once_with([])
