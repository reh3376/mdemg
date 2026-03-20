"""Offline evaluation comparing neural re-ranker scores vs LLM scores.

Loads a model and a JSONL test dataset, computes neural scores for each
(query, candidate) pair, and reports Spearman correlation, NDCG@k, and
score distribution statistics. Optionally compares base vs fine-tuned model.

Usage:
    python -m neural_sidecar.evaluate --model-path .mdemg/neural/models/current --test-data test.jsonl
"""

from __future__ import annotations

import argparse
import json
import logging
import math
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from .train import TrainingSample, load_jsonl

logger = logging.getLogger(__name__)


@dataclass
class EvalStats:
    """Score distribution statistics."""

    count: int = 0
    mean: float = 0.0
    std: float = 0.0
    min: float = 0.0
    max: float = 0.0
    median: float = 0.0


@dataclass
class ModelEvalResult:
    """Evaluation result for a single model."""

    model_path: str
    spearman_correlation: float
    ndcg_at_k: float
    top_k: int
    neural_stats: EvalStats
    llm_stats: EvalStats
    num_samples: int


@dataclass
class EvalReport:
    """Full evaluation report, optionally comparing two models."""

    primary: ModelEvalResult
    baseline: ModelEvalResult | None = None
    improvement: dict[str, float] = field(default_factory=dict)


def compute_stats(values: list[float]) -> EvalStats:
    """Compute distribution statistics for a list of values.

    Args:
        values: List of float values.

    Returns:
        EvalStats with count, mean, std, min, max, median.
    """
    if not values:
        return EvalStats()

    n = len(values)
    mean = sum(values) / n
    variance = sum((v - mean) ** 2 for v in values) / n if n > 1 else 0.0
    std = math.sqrt(variance)
    sorted_vals = sorted(values)
    median = sorted_vals[n // 2] if n % 2 == 1 else (sorted_vals[n // 2 - 1] + sorted_vals[n // 2]) / 2

    return EvalStats(
        count=n,
        mean=mean,
        std=std,
        min=min(values),
        max=max(values),
        median=median,
    )


def spearman_rank_correlation(x: list[float], y: list[float]) -> float:
    """Compute Spearman rank correlation between two lists.

    Args:
        x: First list of values.
        y: Second list of values.

    Returns:
        Spearman rank correlation coefficient.
    """
    from scipy.stats import spearmanr

    if len(x) < 2:
        return 0.0

    corr, _ = spearmanr(x, y)
    return float(corr) if not math.isnan(corr) else 0.0


def dcg_at_k(scores: list[float], k: int) -> float:
    """Compute Discounted Cumulative Gain at position k.

    Args:
        scores: Relevance scores in ranking order.
        k: Cutoff position.

    Returns:
        DCG@k value.
    """
    result = 0.0
    for i, score in enumerate(scores[:k]):
        result += score / math.log2(i + 2)  # i+2 because log2(1)=0
    return result


def ndcg_at_k(neural_scores: list[float], llm_scores: list[float], k: int) -> float:
    """Compute NDCG@k using LLM scores as ground truth relevance.

    Ranks candidates by neural_scores, then uses llm_scores as relevance
    labels to compute NDCG.

    Args:
        neural_scores: Scores from the neural model.
        llm_scores: Ground truth scores from the LLM.
        k: Cutoff position.

    Returns:
        NDCG@k value in [0, 1].
    """
    if not neural_scores or not llm_scores:
        return 0.0

    # Pair neural and llm scores, sort by neural score descending
    paired = list(zip(neural_scores, llm_scores))
    paired.sort(key=lambda p: p[0], reverse=True)

    # Extract LLM scores in the order ranked by neural model
    ranked_relevance = [p[1] for p in paired]

    # Ideal ranking: sort by LLM score descending
    ideal_relevance = sorted(llm_scores, reverse=True)

    actual_dcg = dcg_at_k(ranked_relevance, k)
    ideal_dcg = dcg_at_k(ideal_relevance, k)

    if ideal_dcg == 0:
        return 0.0

    return actual_dcg / ideal_dcg


def evaluate_model(
    model_path: str,
    samples: list[TrainingSample],
    top_k: int = 10,
) -> ModelEvalResult:
    """Evaluate a model against a test dataset.

    For each (query, candidate) pair, compute the neural score and compare
    against the original LLM score.

    Args:
        model_path: Path to the model (HuggingFace name or local directory).
        samples: Test samples with query, candidate, and LLM score.
        top_k: Cutoff for NDCG calculation.

    Returns:
        ModelEvalResult with correlation, NDCG, and statistics.
    """
    from sentence_transformers import CrossEncoder

    logger.info("Loading model from: %s", model_path)
    model = CrossEncoder(model_path)

    # Score all pairs
    pairs = [(s.query, s.candidate) for s in samples]
    logger.info("Scoring %d pairs...", len(pairs))
    raw_scores = model.predict(pairs)
    neural_scores = [float(s) for s in raw_scores]
    llm_scores = [s.score for s in samples]

    # Compute metrics
    spearman = spearman_rank_correlation(neural_scores, llm_scores)
    ndcg = ndcg_at_k(neural_scores, llm_scores, top_k)

    return ModelEvalResult(
        model_path=model_path,
        spearman_correlation=spearman,
        ndcg_at_k=ndcg,
        top_k=top_k,
        neural_stats=compute_stats(neural_scores),
        llm_stats=compute_stats(llm_scores),
        num_samples=len(samples),
    )


def load_test_data(test_data_path: str) -> list[TrainingSample]:
    """Load test data from a single JSONL file or a directory.

    Args:
        test_data_path: Path to JSONL file or directory containing JSONL files.

    Returns:
        List of TrainingSample objects.
    """
    path = Path(test_data_path)
    if path.is_dir():
        return load_jsonl(test_data_path)

    # Single file — wrap in the same loading logic
    samples: list[TrainingSample] = []
    with open(test_data_path, encoding="utf-8") as f:
        for line_num, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                obj = json.loads(line)
                samples.append(
                    TrainingSample(
                        query=obj["query"],
                        candidate=obj["candidate"],
                        score=float(obj["score"]),
                        timestamp=obj.get("timestamp", ""),
                        space_id=obj.get("space_id", ""),
                    )
                )
            except (json.JSONDecodeError, KeyError, ValueError, TypeError) as exc:
                logger.warning("Skipping malformed line %d: %s", line_num, exc)

    return samples


def evaluate(
    model_path: str,
    test_data: str,
    base_model: str | None = None,
    top_k: int = 10,
    output: str | None = None,
) -> EvalReport:
    """Run full evaluation, optionally comparing against a base model.

    Args:
        model_path: Path to the primary model to evaluate.
        test_data: Path to JSONL test data file or directory.
        base_model: Optional base model path for comparison.
        top_k: Cutoff for NDCG calculation.
        output: Optional path to write JSON report.

    Returns:
        EvalReport with results.
    """
    samples = load_test_data(test_data)
    if not samples:
        logger.error("No test samples loaded from %s", test_data)
        sys.exit(1)

    logger.info("Loaded %d test samples", len(samples))

    # Evaluate primary model
    primary_result = evaluate_model(model_path, samples, top_k)
    logger.info(
        "Primary model: spearman=%.4f, ndcg@%d=%.4f",
        primary_result.spearman_correlation,
        top_k,
        primary_result.ndcg_at_k,
    )

    # Optionally evaluate base model for comparison
    baseline_result: ModelEvalResult | None = None
    improvement: dict[str, float] = {}

    if base_model:
        baseline_result = evaluate_model(base_model, samples, top_k)
        logger.info(
            "Base model: spearman=%.4f, ndcg@%d=%.4f",
            baseline_result.spearman_correlation,
            top_k,
            baseline_result.ndcg_at_k,
        )

        improvement = {
            "spearman_delta": primary_result.spearman_correlation - baseline_result.spearman_correlation,
            "ndcg_delta": primary_result.ndcg_at_k - baseline_result.ndcg_at_k,
        }
        logger.info(
            "Improvement: spearman=%+.4f, ndcg@%d=%+.4f",
            improvement["spearman_delta"],
            top_k,
            improvement["ndcg_delta"],
        )

    report = EvalReport(
        primary=primary_result,
        baseline=baseline_result,
        improvement=improvement,
    )

    # Write JSON report if requested
    if output:
        _write_report(report, output)

    return report


def _stats_to_dict(stats: EvalStats) -> dict[str, Any]:
    """Convert EvalStats to a JSON-serializable dict."""
    return {
        "count": stats.count,
        "mean": round(stats.mean, 4),
        "std": round(stats.std, 4),
        "min": round(stats.min, 4),
        "max": round(stats.max, 4),
        "median": round(stats.median, 4),
    }


def _result_to_dict(result: ModelEvalResult) -> dict[str, Any]:
    """Convert ModelEvalResult to a JSON-serializable dict."""
    return {
        "model_path": result.model_path,
        "spearman_correlation": round(result.spearman_correlation, 4),
        "ndcg_at_k": round(result.ndcg_at_k, 4),
        "top_k": result.top_k,
        "num_samples": result.num_samples,
        "neural_stats": _stats_to_dict(result.neural_stats),
        "llm_stats": _stats_to_dict(result.llm_stats),
    }


def _write_report(report: EvalReport, output_path: str) -> None:
    """Write evaluation report as JSON.

    Args:
        report: Evaluation report.
        output_path: Path to write the JSON file.
    """
    data: dict[str, Any] = {
        "primary": _result_to_dict(report.primary),
    }
    if report.baseline:
        data["baseline"] = _result_to_dict(report.baseline)
    if report.improvement:
        data["improvement"] = {k: round(v, 4) for k, v in report.improvement.items()}

    Path(output_path).parent.mkdir(parents=True, exist_ok=True)
    with open(output_path, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2)
    logger.info("Report written to %s", output_path)


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    """Parse command-line arguments."""
    parser = argparse.ArgumentParser(
        description="Evaluate neural re-ranker against LLM scores.",
    )
    parser.add_argument(
        "--model-path",
        required=True,
        help="Path to the model to evaluate (HuggingFace name or local directory)",
    )
    parser.add_argument(
        "--test-data",
        required=True,
        help="Path to JSONL test data file or directory",
    )
    parser.add_argument(
        "--base-model",
        default=None,
        help="Optional base model path for comparison",
    )
    parser.add_argument(
        "--top-k",
        type=int,
        default=10,
        help="Cutoff position for NDCG calculation (default: 10)",
    )
    parser.add_argument(
        "--output",
        default=None,
        help="Optional path to write JSON evaluation report",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    """CLI entrypoint for the evaluation pipeline."""
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    args = parse_args(argv)
    evaluate(
        model_path=args.model_path,
        test_data=args.test_data,
        base_model=args.base_model,
        top_k=args.top_k,
        output=args.output,
    )


if __name__ == "__main__":
    main()
