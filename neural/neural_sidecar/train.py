"""Training pipeline for fine-tuning the cross-encoder from collected JSONL data.

Reads (query, candidate, score) tuples produced by NR-1 (rerank_collector.go),
creates training pairs, and fine-tunes the cross-encoder model using MSE loss
with Spearman rank correlation evaluation on a held-out validation split.

Usage:
    python -m neural_sidecar.train --data-dir .mdemg/neural/training-data --epochs 3
"""

from __future__ import annotations

import argparse
import glob
import json
import logging
import os
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)

DEFAULT_DATA_DIR = ".mdemg/neural/training-data"
DEFAULT_MODEL_DIR = ".mdemg/neural/models"
DEFAULT_BASE_MODEL = "cross-encoder/ms-marco-MiniLM-L-6-v2"
DEFAULT_EPOCHS = 3
DEFAULT_BATCH_SIZE = 16
DEFAULT_VAL_SPLIT = 0.1
DEFAULT_MIN_SAMPLES = 100


@dataclass
class TrainingSample:
    """A single training sample from JSONL data."""

    query: str
    candidate: str
    score: float
    timestamp: str = ""
    space_id: str = ""


@dataclass
class TrainingResult:
    """Result of a training run."""

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


def load_jsonl(data_dir: str) -> list[TrainingSample]:
    """Load all JSONL files from the data directory.

    Each line should be a JSON object with keys: query, candidate, score.
    Malformed lines are skipped with a warning.

    Args:
        data_dir: Path to directory containing .jsonl files.

    Returns:
        List of TrainingSample objects.
    """
    samples: list[TrainingSample] = []
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
                    sample = TrainingSample(
                        query=obj["query"],
                        candidate=obj["candidate"],
                        score=float(obj["score"]),
                        timestamp=obj.get("timestamp", ""),
                        space_id=obj.get("space_id", ""),
                    )
                    samples.append(sample)
                except (json.JSONDecodeError, KeyError, ValueError, TypeError) as exc:
                    logger.warning("Skipping malformed line %d in %s: %s", line_num, filepath, exc)

    logger.info("Loaded %d training samples from %d files", len(samples), len(files))
    return samples


def create_training_pairs(samples: list[TrainingSample]) -> list[Any]:
    """Convert TrainingSample objects to sentence-transformers InputExample pairs.

    Args:
        samples: List of training samples.

    Returns:
        List of InputExample objects for CrossEncoder training.
    """
    from sentence_transformers import InputExample

    pairs: list[InputExample] = []
    for s in samples:
        pairs.append(InputExample(texts=[s.query, s.candidate], label=s.score))
    return pairs


def get_next_version(model_dir: str) -> int:
    """Determine the next model version number.

    Scans existing v{N}/ directories and returns N+1 (or 1 if none exist).

    Args:
        model_dir: Base directory for model versions.

    Returns:
        Next version number.
    """
    model_path = Path(model_dir)
    if not model_path.exists():
        return 1

    max_version = 0
    for entry in model_path.iterdir():
        if entry.is_dir() and entry.name.startswith("v"):
            try:
                ver = int(entry.name[1:])
                max_version = max(max_version, ver)
            except ValueError:
                continue
    return max_version + 1


def update_current_symlink(model_dir: str, version: int) -> None:
    """Create or update the 'current' symlink to point to the latest model version.

    Args:
        model_dir: Base directory for model versions.
        version: Version number to point to.
    """
    current_link = Path(model_dir) / "current"
    target = f"v{version}"

    if current_link.is_symlink() or current_link.exists():
        current_link.unlink()
    current_link.symlink_to(target)
    logger.info("Updated symlink: current -> %s", target)


def train(
    data_dir: str = DEFAULT_DATA_DIR,
    model_dir: str = DEFAULT_MODEL_DIR,
    base_model: str = DEFAULT_BASE_MODEL,
    epochs: int = DEFAULT_EPOCHS,
    batch_size: int = DEFAULT_BATCH_SIZE,
    val_split: float = DEFAULT_VAL_SPLIT,
    min_samples: int = DEFAULT_MIN_SAMPLES,
    from_checkpoint: str | None = None,
) -> TrainingResult:
    """Run the training pipeline.

    1. Load JSONL data from data_dir
    2. Check minimum sample threshold
    3. Split into train/validation sets
    4. Fine-tune cross-encoder with MSE loss
    5. Evaluate with Spearman correlation
    6. Save versioned model and update symlink

    Args:
        data_dir: Directory containing JSONL training data files.
        model_dir: Base directory for saving model versions.
        base_model: HuggingFace model name or path for the base cross-encoder.
        epochs: Number of training epochs.
        batch_size: Training batch size.
        val_split: Fraction of data to hold out for validation.
        min_samples: Minimum number of samples required to proceed with training.
        from_checkpoint: Optional path to a previous model version to train from.

    Returns:
        TrainingResult with training outcome details.
    """
    import math

    from sentence_transformers import CrossEncoder
    from sentence_transformers.cross_encoder.evaluation import CECorrelationEvaluator
    from torch.utils.data import DataLoader

    # Step 1: Load data
    samples = load_jsonl(data_dir)

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

    # Step 3: Create pairs and split
    pairs = create_training_pairs(samples)
    val_size = max(1, int(len(pairs) * val_split))
    train_pairs = pairs[val_size:]
    val_pairs = pairs[:val_size]

    logger.info("Train: %d samples, Validation: %d samples", len(train_pairs), len(val_pairs))

    # Step 4: Initialize model (from checkpoint or base)
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
        "Starting training: epochs=%d, batch_size=%d, warmup_steps=%d, output=%s",
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
        "num_train": len(train_pairs),
        "num_val": len(val_pairs),
        "val_split": val_split,
        "spearman_correlation": spearman,
        "version": version,
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
        description="Fine-tune the cross-encoder re-ranker from collected JSONL training data.",
    )
    parser.add_argument(
        "--data-dir",
        default=DEFAULT_DATA_DIR,
        help=f"Directory containing JSONL training data (default: {DEFAULT_DATA_DIR})",
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
    """CLI entrypoint for the training pipeline."""
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    args = parse_args(argv)
    result = train(
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
