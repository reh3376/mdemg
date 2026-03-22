"""Training pipeline for fine-tuning the tier prediction model from J17 protocol JSONL data.

Reads protocol event records produced by protocol_data_collector.go,
derives optimal tiers from comprehension_score/token_count ratio,
and fine-tunes a cross-encoder model for tier prediction.

Usage:
    python -m neural_sidecar.train_protocol --data-dir .mdemg/neural/protocol-data --epochs 3
"""

from __future__ import annotations

import argparse
import glob
import json
import logging
import os
import sys
from dataclasses import dataclass, field
from typing import Any

from .train import get_next_version, update_current_symlink

logger = logging.getLogger(__name__)

DEFAULT_DATA_DIR = ".mdemg/neural/protocol-data"
DEFAULT_MODEL_DIR = ".mdemg/neural/models/protocol-tier"
DEFAULT_BASE_MODEL = "cross-encoder/ms-marco-MiniLM-L-6-v2"
DEFAULT_EPOCHS = 3
DEFAULT_BATCH_SIZE = 16
DEFAULT_VAL_SPLIT = 0.1
DEFAULT_MIN_SAMPLES = 500


@dataclass
class ProtocolTrainingSample:
    """A single training sample from J17 protocol JSONL data."""

    timestamp: str
    constraint_code: str
    constraint_text: str
    tier_used: int
    token_count: int
    agent_action: str
    outcome: str
    comprehension_score: float
    trust_score: float
    session_id: str = ""


@dataclass
class TrainingResult:
    """Result of a protocol training run."""

    model_path: str
    version: int
    num_samples: int
    num_train: int
    num_val: int
    epochs: int
    skipped: bool = False
    skip_reason: str = ""
    spearman_correlation: float | None = None
    metadata: dict[str, Any] = field(default_factory=dict)


def load_protocol_jsonl(data_dir: str) -> list[ProtocolTrainingSample]:
    """Load J17 protocol JSONL files from data directory.

    Each line is a JSON object matching protocolTrainingRecord from Go:
    {timestamp, constraint_code, constraint_text, tier_used, token_count,
     agent_action, outcome, comprehension_score, trust_score, session_id}

    Malformed lines are skipped with a warning.
    """
    samples: list[ProtocolTrainingSample] = []
    pattern = os.path.join(data_dir, "*.jsonl")
    files = sorted(glob.glob(pattern))

    if not files:
        logger.warning("No JSONL files found in %s", data_dir)
        return samples

    for filepath in files:
        logger.info("Loading %s", filepath)
        with open(filepath, encoding="utf-8") as f:
            for line_num, line in enumerate(f, 1):
                line = line.strip()
                if not line:
                    continue
                try:
                    obj = json.loads(line)
                    sample = ProtocolTrainingSample(
                        timestamp=obj.get("timestamp", ""),
                        constraint_code=obj["constraint_code"],
                        constraint_text=obj.get("constraint_text", ""),
                        tier_used=int(obj["tier_used"]),
                        token_count=int(obj.get("token_count", 0)),
                        agent_action=obj.get("agent_action", ""),
                        outcome=obj.get("outcome", ""),
                        comprehension_score=float(obj.get("comprehension_score", 0.0)),
                        trust_score=float(obj.get("trust_score", 0.5)),
                        session_id=obj.get("session_id", ""),
                    )
                    samples.append(sample)
                except (json.JSONDecodeError, KeyError, ValueError, TypeError) as exc:
                    logger.warning("Skipping malformed line %d in %s: %s", line_num, filepath, exc)

    logger.info("Loaded %d protocol training samples from %d files", len(samples), len(files))
    return samples


def derive_optimal_tiers(samples: list[ProtocolTrainingSample]) -> list[tuple[str, str, float, int]]:
    """For each constraint, find the tier that maximizes comprehension_score / token_count.

    Returns list of (constraint_text, trust_context, optimal_score, optimal_tier) tuples
    for training pair construction.
    """
    # Group by constraint_code
    code_tiers: dict[str, dict[int, list[ProtocolTrainingSample]]] = {}
    for s in samples:
        if s.constraint_code not in code_tiers:
            code_tiers[s.constraint_code] = {}
        if s.tier_used not in code_tiers[s.constraint_code]:
            code_tiers[s.constraint_code][s.tier_used] = []
        code_tiers[s.constraint_code][s.tier_used].append(s)

    labeled: list[tuple[str, str, float, int]] = []

    for _code, tiers in code_tiers.items():
        # For each tier, compute average efficiency = comprehension / max(token_count, 1)
        best_tier = 0
        best_efficiency = -1.0

        for tier, tier_samples in tiers.items():
            avg_comp = sum(s.comprehension_score for s in tier_samples) / len(tier_samples)
            avg_tokens = sum(s.token_count for s in tier_samples) / len(tier_samples)
            efficiency = avg_comp / max(avg_tokens, 1.0)
            if efficiency > best_efficiency:
                best_efficiency = efficiency
                best_tier = tier

        # Generate training pairs from all samples for this constraint
        for tier_samples in tiers.values():
            for s in tier_samples:
                # Score: 1.0 if tier matches optimal, scaled down otherwise
                score = 1.0 if s.tier_used == best_tier else 0.2
                trust_ctx = f"trust={s.trust_score:.2f} {s.agent_action}"
                labeled.append((s.constraint_text, trust_ctx, score, best_tier))

    return labeled


def create_training_pairs(labeled_data: list[tuple[str, str, float, int]]) -> list[Any]:
    """Convert labeled tier data to sentence-transformers InputExample pairs.

    Each pair is (constraint_text, trust_context) with the score as label.
    """
    from sentence_transformers import InputExample

    pairs: list[InputExample] = []
    for constraint_text, trust_ctx, score, _tier in labeled_data:
        pairs.append(InputExample(texts=[constraint_text, trust_ctx], label=score))
    return pairs


def train_protocol(
    data_dir: str = DEFAULT_DATA_DIR,
    model_dir: str = DEFAULT_MODEL_DIR,
    base_model: str = DEFAULT_BASE_MODEL,
    epochs: int = DEFAULT_EPOCHS,
    batch_size: int = DEFAULT_BATCH_SIZE,
    val_split: float = DEFAULT_VAL_SPLIT,
    min_samples: int = DEFAULT_MIN_SAMPLES,
    from_checkpoint: str | None = None,
) -> TrainingResult:
    """Run the protocol tier training pipeline.

    1. Load protocol JSONL data
    2. Derive optimal tiers per constraint
    3. Create training pairs
    4. Fine-tune cross-encoder
    5. Evaluate and save versioned model
    """
    import math

    from sentence_transformers import CrossEncoder
    from sentence_transformers.cross_encoder.evaluation import CECorrelationEvaluator
    from torch.utils.data import DataLoader

    # Step 1: Load data
    samples = load_protocol_jsonl(data_dir)

    # Step 2: Check minimum threshold
    if len(samples) < min_samples:
        logger.warning(
            "Only %d samples found (minimum: %d). Skipping training.",
            len(samples),
            min_samples,
        )
        return TrainingResult(
            model_path="",
            version=0,
            num_samples=len(samples),
            num_train=0,
            num_val=0,
            epochs=epochs,
            skipped=True,
            skip_reason=f"Insufficient samples: {len(samples)} < {min_samples}",
        )

    # Step 3: Derive optimal tiers and create training pairs
    labeled = derive_optimal_tiers(samples)
    pairs = create_training_pairs(labeled)
    val_size = max(1, int(len(pairs) * val_split))
    train_pairs = pairs[val_size:]
    val_pairs = pairs[:val_size]

    logger.info("Train: %d samples, Validation: %d samples", len(train_pairs), len(val_pairs))

    # Step 4: Initialize model
    source_model = from_checkpoint if from_checkpoint else base_model
    logger.info("Loading model from: %s", source_model)
    model = CrossEncoder(source_model, num_labels=1)

    # Prepare train dataloader
    train_dataloader = DataLoader(train_pairs, shuffle=True, batch_size=batch_size)

    # Prepare evaluator from validation set
    val_sentences1 = [p.texts[0] for p in val_pairs]
    val_sentences2 = [p.texts[1] for p in val_pairs]
    val_scores = [p.label for p in val_pairs]
    evaluator = CECorrelationEvaluator(val_sentences1, val_sentences2, val_scores, name="val")

    # Step 5: Determine output path
    version = get_next_version(model_dir)
    output_path = os.path.join(model_dir, f"v{version}")
    os.makedirs(output_path, exist_ok=True)

    # Step 6: Train
    warmup_steps = math.ceil(len(train_dataloader) * epochs * 0.1)
    logger.info(
        "Starting protocol tier training: epochs=%d, batch_size=%d, warmup_steps=%d, output=%s",
        epochs,
        batch_size,
        warmup_steps,
        output_path,
    )

    model.fit(
        train_dataloader=train_dataloader,
        evaluator=evaluator,
        epochs=epochs,
        warmup_steps=warmup_steps,
        output_path=output_path,
    )

    # Step 7: Evaluate final model
    spearman = evaluator(model, output_path)
    logger.info("Final Spearman correlation: %.4f", spearman)

    # Step 8: Update symlink
    update_current_symlink(model_dir, version)

    # Save training metadata
    metadata = {
        "base_model": base_model,
        "from_checkpoint": from_checkpoint,
        "epochs": epochs,
        "batch_size": batch_size,
        "num_samples": len(samples),
        "num_train": len(train_pairs),
        "num_val": len(val_pairs),
        "val_split": val_split,
        "spearman_correlation": spearman,
        "version": version,
        "pipeline": "protocol-tier",
    }
    metadata_path = os.path.join(output_path, "training_metadata.json")
    with open(metadata_path, "w", encoding="utf-8") as f:
        json.dump(metadata, f, indent=2)

    return TrainingResult(
        model_path=output_path,
        version=version,
        num_samples=len(samples),
        num_train=len(train_pairs),
        num_val=len(val_pairs),
        epochs=epochs,
        spearman_correlation=spearman,
        metadata=metadata,
    )


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    """Parse command-line arguments."""
    parser = argparse.ArgumentParser(
        description="Fine-tune the tier prediction model from J17 protocol JSONL data.",
    )
    parser.add_argument(
        "--data-dir",
        default=DEFAULT_DATA_DIR,
        help=f"Directory containing protocol JSONL data (default: {DEFAULT_DATA_DIR})",
    )
    parser.add_argument(
        "--model-dir",
        default=DEFAULT_MODEL_DIR,
        help=f"Directory for saving model versions (default: {DEFAULT_MODEL_DIR})",
    )
    parser.add_argument(
        "--base-model",
        default=DEFAULT_BASE_MODEL,
        help=f"Base cross-encoder model name (default: {DEFAULT_BASE_MODEL})",
    )
    parser.add_argument(
        "--epochs",
        type=int,
        default=DEFAULT_EPOCHS,
        help=f"Number of training epochs (default: {DEFAULT_EPOCHS})",
    )
    parser.add_argument(
        "--batch-size",
        type=int,
        default=DEFAULT_BATCH_SIZE,
        help=f"Training batch size (default: {DEFAULT_BATCH_SIZE})",
    )
    parser.add_argument(
        "--val-split",
        type=float,
        default=DEFAULT_VAL_SPLIT,
        help=f"Validation split fraction (default: {DEFAULT_VAL_SPLIT})",
    )
    parser.add_argument(
        "--min-samples",
        type=int,
        default=DEFAULT_MIN_SAMPLES,
        help=f"Minimum samples required to train (default: {DEFAULT_MIN_SAMPLES})",
    )
    parser.add_argument(
        "--from-checkpoint",
        default=None,
        help="Path to previous model version for incremental retraining",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> None:
    """CLI entrypoint for the protocol tier training pipeline."""
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    args = parse_args(argv)
    result = train_protocol(
        data_dir=args.data_dir,
        model_dir=args.model_dir,
        base_model=args.base_model,
        epochs=args.epochs,
        batch_size=args.batch_size,
        val_split=args.val_split,
        min_samples=args.min_samples,
        from_checkpoint=args.from_checkpoint,
    )

    if result.skipped:
        logger.warning("Training skipped: %s", result.skip_reason)
        sys.exit(0)

    logger.info(
        "Training complete: version=v%d, samples=%d, spearman=%.4f, path=%s",
        result.version,
        result.num_samples,
        result.spearman_correlation or 0.0,
        result.model_path,
    )


if __name__ == "__main__":
    main()
