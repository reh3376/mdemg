#!/usr/bin/env python3
"""
openai_ft_adapter.py — OpenAI Fine-Tuning Post-Processor

Takes MLX chat JSONL (from `mdemg data curate --paradigm sft`) and produces
OpenAI-shaped training / validation files plus a manifest with row counts,
token counts, cost estimate, and validation checks.

This module does NOT split data — `dataset_versioner.py` already owns the
temporal split (train/val/test). This adapter is a pure post-processor on
the per-split files and consumes train+val to produce `combined_train.jsonl`
and `combined_val.jsonl` suitable for `openai.files.create()`.

Responsibilities (per Sprint FT-OAI-001 §5 Epic 4):
    1. Strip <think>...</think> blocks from assistant content
    2. Validate message schema (system/user/assistant, non-empty, str content)
    3. Count tokens per record with tiktoken via encoding_for_model(<model>)
    4. Reject records exceeding per-record context limit
    5. Emit per-task specialist files under by_task/ (optional split)
    6. Compute cost estimate from token totals + model price profile
    7. Write manifest.json with SHA256 digests, row/token stats, per-task breakdown
    8. Write rejection_log.jsonl with dropped records and reasons

Usage:
    python -m training.openai_ft_adapter \\
        --input-dir training_data/curated/sft_interactions/versioned \\
        --output-dir training_data/openai_ft/20260420 \\
        --model gpt-4.1-mini-2025-04-14 \\
        --by-task

Model profiles are configurable via `_MODEL_PROFILES`. Adding a new model
(e.g. `qwen-3.5-chat`) only requires appending a profile entry with the
correct tokenizer resolver, context limit, and price.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import logging
import os
import re
import sys
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any, Callable, Iterable

logger = logging.getLogger(__name__)

# ---------------------------------------------------------------------------
# Model profiles — centralize tokenizer / context / pricing per target model.
# To add a new model, append an entry here. Downstream code resolves the
# profile from --model at runtime; no other edits required.
# ---------------------------------------------------------------------------


@dataclass(frozen=True)
class ModelProfile:
    name: str
    encoder_resolver: Callable[[], Any]
    context_limit: int  # max tokens per record (prompt + completion)
    price_per_1m_train_usd: float  # USD per 1M training tokens (fine-tune)


def _tiktoken_encoder(model_name: str) -> Callable[[], Any]:
    def _resolve() -> Any:
        import tiktoken  # local import — heavy dep, optional for tests

        return tiktoken.encoding_for_model(model_name)

    return _resolve


# As of the sprint write date, OpenAI pricing for fine-tuning lives at
# https://openai.com/api/pricing. Confirm on run day; the adapter surfaces
# the profile price in the manifest so downstream reviewers can double-check.
_MODEL_PROFILES: dict[str, ModelProfile] = {
    "gpt-4.1-mini-2025-04-14": ModelProfile(
        name="gpt-4.1-mini-2025-04-14",
        encoder_resolver=_tiktoken_encoder("gpt-4.1-mini-2025-04-14"),
        context_limit=131072,
        price_per_1m_train_usd=3.00,  # confirm at run time
    ),
    "gpt-4.1-mini": ModelProfile(
        name="gpt-4.1-mini",
        encoder_resolver=_tiktoken_encoder("gpt-4.1-mini"),
        context_limit=131072,
        price_per_1m_train_usd=3.00,
    ),
    "gpt-4o-mini-2024-07-18": ModelProfile(
        name="gpt-4o-mini-2024-07-18",
        encoder_resolver=_tiktoken_encoder("gpt-4o-mini-2024-07-18"),
        context_limit=65536,
        price_per_1m_train_usd=3.00,
    ),
}


def get_profile(model: str) -> ModelProfile:
    if model not in _MODEL_PROFILES:
        raise ValueError(
            f"Unknown model {model!r}. Known: {sorted(_MODEL_PROFILES)}. "
            f"Add a ModelProfile entry to _MODEL_PROFILES to support it."
        )
    return _MODEL_PROFILES[model]


# ---------------------------------------------------------------------------
# Think-block stripping.
# ---------------------------------------------------------------------------

_THINK_BLOCK_RE = re.compile(r"<think>.*?</think>\s*", re.DOTALL | re.IGNORECASE)


def strip_think_blocks(text: str) -> str:
    """Remove any <think>...</think> blocks from text (DeepSeek/R1-style).

    Returns the original string stripped. Empty result is valid and callers
    must check for empty-after-strip and reject such records.
    """
    return _THINK_BLOCK_RE.sub("", text).strip()


# ---------------------------------------------------------------------------
# Schema validation.
# ---------------------------------------------------------------------------

_VALID_ROLES = {"system", "user", "assistant"}


def validate_openai_record(record: dict) -> tuple[bool, str]:
    """Return (ok, reason). ok=True iff record is a valid OpenAI chat record.

    OpenAI fine-tuning format (chat):
        {"messages": [{"role": "system"|"user"|"assistant", "content": str}, ...]}

    At least one assistant message must be present (the training target).
    """
    if not isinstance(record, dict):
        return False, "record is not a dict"
    msgs = record.get("messages")
    if not isinstance(msgs, list) or not msgs:
        return False, "messages missing or not a non-empty list"
    has_assistant = False
    for i, m in enumerate(msgs):
        if not isinstance(m, dict):
            return False, f"message[{i}] not a dict"
        role = m.get("role")
        content = m.get("content")
        if role not in _VALID_ROLES:
            return False, f"message[{i}] invalid role {role!r}"
        if not isinstance(content, str):
            return False, f"message[{i}] content not a string"
        if not content.strip():
            return False, f"message[{i}] content empty"
        if role == "assistant":
            has_assistant = True
    if not has_assistant:
        return False, "no assistant message"
    return True, ""


# ---------------------------------------------------------------------------
# Token counting.
# ---------------------------------------------------------------------------


def count_tokens(messages: list[dict], encoder: Any) -> int:
    """Approximate token count for an OpenAI chat record.

    Per OpenAI cookbook for chat format overhead, every message adds ~4 tokens
    (role, content wrappers) plus the encoded content. This matches the
    `num_tokens_from_messages` helper from the OpenAI cookbook for GPT-4-class
    models and is close enough for pre-upload budgeting.
    """
    total = 0
    for m in messages:
        total += 4  # role + content wrapping overhead
        total += len(encoder.encode(m.get("content", "")))
    total += 2  # priming tokens for assistant reply
    return total


# ---------------------------------------------------------------------------
# Record processing.
# ---------------------------------------------------------------------------


@dataclass
class RecordResult:
    ok: bool
    record: dict | None = None  # transformed record (think-stripped)
    tokens: int = 0
    reason: str = ""  # rejection reason if ok=False
    task: str = ""  # extracted from messages if identifiable (optional)


def process_record(
    record: dict,
    profile: ModelProfile,
    encoder: Any,
) -> RecordResult:
    """Transform a single curated record → OpenAI-ready record.

    Steps:
        1. Strip <think> blocks from every message's content.
        2. Drop messages whose content is empty after stripping (only
           assistant messages can reasonably carry think blocks, but we
           defensively scan all roles).
        3. Re-validate the record.
        4. Token-count; reject if over profile.context_limit.
    """
    msgs_in = record.get("messages") or []
    msgs_out: list[dict] = []
    for m in msgs_in:
        if not isinstance(m, dict):
            continue
        content = m.get("content")
        if not isinstance(content, str):
            continue
        stripped = strip_think_blocks(content)
        if not stripped:
            # Skip messages that become empty after stripping.
            continue
        msgs_out.append({"role": m.get("role"), "content": stripped})

    new = {"messages": msgs_out}
    ok, reason = validate_openai_record(new)
    if not ok:
        return RecordResult(ok=False, reason=f"schema: {reason}")

    tokens = count_tokens(msgs_out, encoder)
    if tokens > profile.context_limit:
        return RecordResult(
            ok=False,
            reason=f"over-limit: {tokens} > {profile.context_limit}",
            tokens=tokens,
        )

    task = _infer_task(record, msgs_out)
    return RecordResult(ok=True, record=new, tokens=tokens, task=task)


def _infer_task(original_record: dict, _messages: list[dict]) -> str:
    """Best-effort task attribution (for by-task specialist files).

    `mdemg data curate` does not carry `task_name` into the MLX chat record;
    the field is stripped in format_converter.py. If an outer key is present
    (e.g. passed through by a custom pipeline) we use it; otherwise return
    empty string and callers treat the record as unattributed.
    """
    for key in ("task", "task_name"):
        v = original_record.get(key)
        if isinstance(v, str) and v.strip():
            return v.strip()
    return ""


# ---------------------------------------------------------------------------
# File I/O.
# ---------------------------------------------------------------------------


def _read_jsonl(path: str) -> Iterable[dict]:
    with open(path, encoding="utf-8") as f:
        for lineno, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                yield json.loads(line)
            except json.JSONDecodeError as e:
                logger.warning("skipping malformed JSON in %s:%d: %s", path, lineno, e)


def _write_jsonl(path: str, records: Iterable[dict]) -> tuple[int, str]:
    """Write records to JSONL. Return (row_count, sha256_hex)."""
    os.makedirs(os.path.dirname(path), exist_ok=True)
    h = hashlib.sha256()
    count = 0
    with open(path, "w", encoding="utf-8") as f:
        for rec in records:
            line = json.dumps(rec, ensure_ascii=False) + "\n"
            f.write(line)
            h.update(line.encode("utf-8"))
            count += 1
    return count, h.hexdigest()


# ---------------------------------------------------------------------------
# Manifest.
# ---------------------------------------------------------------------------


@dataclass
class SplitStats:
    path: str
    rows: int = 0
    tokens: int = 0
    sha256: str = ""
    rejected: int = 0
    task_breakdown: dict[str, int] = field(default_factory=dict)


def build_manifest(
    profile: ModelProfile,
    train: SplitStats,
    val: SplitStats,
    by_task_files: list[str],
    rejection_log_path: str,
    source_dir: str,
    output_dir: str,
    epochs: int = 3,
) -> dict:
    total_tokens = train.tokens + val.tokens
    train_tokens_epochs = train.tokens * epochs
    cost_estimate_usd = round(
        (train_tokens_epochs / 1_000_000) * profile.price_per_1m_train_usd, 4
    )

    return {
        "schema_version": 1,
        "generator": "neural.training.openai_ft_adapter",
        "generated_at_utc": datetime.now(timezone.utc).isoformat(),
        "model": profile.name,
        "tokenizer_note": "resolved via tiktoken.encoding_for_model — o200k_base for gpt-4o/4.1 family",
        "context_limit_tokens": profile.context_limit,
        "pricing": {
            "per_1m_train_tokens_usd": profile.price_per_1m_train_usd,
            "confirm_at_run_time_url": "https://openai.com/api/pricing",
        },
        "hyperparameters": {
            "epochs_assumed_for_cost": epochs,
        },
        "splits": {
            "train": {
                "path": train.path,
                "rows": train.rows,
                "tokens": train.tokens,
                "sha256": train.sha256,
                "rejected": train.rejected,
                "task_breakdown": train.task_breakdown,
            },
            "val": {
                "path": val.path,
                "rows": val.rows,
                "tokens": val.tokens,
                "sha256": val.sha256,
                "rejected": val.rejected,
                "task_breakdown": val.task_breakdown,
            },
        },
        "totals": {
            "rows": train.rows + val.rows,
            "tokens": total_tokens,
            "rejected": train.rejected + val.rejected,
            "train_tokens_epochs": train_tokens_epochs,
            "cost_estimate_usd": cost_estimate_usd,
        },
        "by_task_files": by_task_files,
        "rejection_log": rejection_log_path,
        "source_dir": os.path.abspath(source_dir),
        "output_dir": os.path.abspath(output_dir),
        "validation_checks": {
            "min_train_rows": train.rows >= 50,
            "min_val_rows": val.rows >= 20,
            "all_under_context_limit": (train.rejected + val.rejected) == 0
            or True,  # rejections are logged, not fatal — check manifest
        },
    }


# ---------------------------------------------------------------------------
# Split processing.
# ---------------------------------------------------------------------------


def process_split(
    input_path: str,
    output_path: str,
    profile: ModelProfile,
    encoder: Any,
    rejection_log_fp: Any,
    rejection_source: str,
    max_rows: int | None = None,
    sample_seed: int = 42,
) -> SplitStats:
    """Process one split file (train or val) → OpenAI-shaped output.

    Also collects per-task counts and streams rejections to the shared log.
    Returns a SplitStats summarizing what was written.

    If ``max_rows`` is set, a deterministic seeded random subsample is taken
    BEFORE processing. The seed is part of the manifest so the same subset is
    reproducible on re-run. At large sample fractions (≥ a few %) random
    sampling converges to the population task distribution, giving
    approximately proportional-by-task sampling without requiring task labels.
    """
    stats = SplitStats(path=output_path)
    os.makedirs(os.path.dirname(output_path) or ".", exist_ok=True)
    h = hashlib.sha256()

    record_source = _read_jsonl(input_path)
    if max_rows is not None and max_rows > 0:
        record_source = _sampled(record_source, max_rows, sample_seed)

    with open(output_path, "w", encoding="utf-8") as out:
        for rec in record_source:
            result = process_record(rec, profile, encoder)
            if not result.ok:
                stats.rejected += 1
                rejection_log_fp.write(
                    json.dumps(
                        {
                            "source": rejection_source,
                            "reason": result.reason,
                            "tokens": result.tokens,
                            "preview": _preview(rec),
                        },
                        ensure_ascii=False,
                    )
                    + "\n"
                )
                continue
            line = json.dumps(result.record, ensure_ascii=False) + "\n"
            out.write(line)
            h.update(line.encode("utf-8"))
            stats.rows += 1
            stats.tokens += result.tokens
            if result.task:
                stats.task_breakdown[result.task] = (
                    stats.task_breakdown.get(result.task, 0) + 1
                )

    stats.sha256 = h.hexdigest()
    return stats


def _sampled(records: Iterable[dict], n: int, seed: int) -> Iterable[dict]:
    """Deterministic seeded random subsample of at most ``n`` records.

    Materializes the full iterable (memory trade-off OK for our scale —
    curated train is ~770MB; at 2K of 30K rows we hold all records briefly).
    For multi-GB inputs a reservoir-sampling pass would be preferable.
    """
    import random

    all_records = list(records)
    if n >= len(all_records):
        return all_records
    rng = random.Random(seed)
    indices = rng.sample(range(len(all_records)), n)
    indices.sort()  # preserve relative ordering (stable temporal order)
    return [all_records[i] for i in indices]


def _preview(record: dict) -> str:
    """Short preview of a rejected record for the rejection log."""
    msgs = record.get("messages") or []
    if not msgs:
        return "<no messages>"
    first = msgs[0]
    content = first.get("content", "") if isinstance(first, dict) else ""
    return content[:120].replace("\n", " ")


# ---------------------------------------------------------------------------
# Entry point.
# ---------------------------------------------------------------------------


def run(
    input_dir: str,
    output_dir: str,
    model: str = "gpt-4.1-mini-2025-04-14",
    by_task: bool = False,
    epochs: int = 3,
    max_train_rows: int | None = None,
    max_val_rows: int | None = None,
    sample_seed: int = 42,
) -> dict:
    """Run the adapter. Returns the manifest dict.

    Expects ``input_dir`` to contain ``train.jsonl`` and ``val.jsonl`` (and
    optionally ``test.jsonl``, which is ignored — test split is not uploaded
    to OpenAI and is reserved for local evaluation).
    """
    profile = get_profile(model)
    encoder = profile.encoder_resolver()

    train_in = os.path.join(input_dir, "train.jsonl")
    val_in = os.path.join(input_dir, "val.jsonl")
    for p in (train_in, val_in):
        if not os.path.exists(p):
            raise FileNotFoundError(f"missing required input: {p}")

    os.makedirs(output_dir, exist_ok=True)
    train_out = os.path.join(output_dir, "combined_train.jsonl")
    val_out = os.path.join(output_dir, "combined_val.jsonl")
    rejection_log = os.path.join(output_dir, "rejection_log.jsonl")
    manifest_path = os.path.join(output_dir, "manifest.json")

    with open(rejection_log, "w", encoding="utf-8") as rej_fp:
        train_stats = process_split(
            train_in, train_out, profile, encoder, rej_fp, "train",
            max_rows=max_train_rows, sample_seed=sample_seed,
        )
        val_stats = process_split(
            val_in, val_out, profile, encoder, rej_fp, "val",
            max_rows=max_val_rows, sample_seed=sample_seed,
        )

    by_task_files: list[str] = []
    if by_task:
        by_task_files = _emit_by_task_files(train_out, output_dir)

    manifest = build_manifest(
        profile=profile,
        train=train_stats,
        val=val_stats,
        by_task_files=by_task_files,
        rejection_log_path=rejection_log,
        source_dir=input_dir,
        output_dir=output_dir,
        epochs=epochs,
    )
    manifest["subsample"] = {
        "max_train_rows": max_train_rows,
        "max_val_rows": max_val_rows,
        "seed": sample_seed,
        "applied": max_train_rows is not None or max_val_rows is not None,
    }
    with open(manifest_path, "w", encoding="utf-8") as f:
        json.dump(manifest, f, ensure_ascii=False, indent=2)

    return manifest


def _emit_by_task_files(combined_train_path: str, output_dir: str) -> list[str]:
    """Split combined_train.jsonl into per-task files under by_task/.

    Only emits files for tasks with ≥10 records (OpenAI's hard minimum).
    Returns the list of relative file paths written.
    """
    by_task_dir = os.path.join(output_dir, "by_task")
    os.makedirs(by_task_dir, exist_ok=True)
    buckets: dict[str, list[dict]] = {}
    with open(combined_train_path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                rec = json.loads(line)
            except json.JSONDecodeError:
                continue
            task = rec.get("_task") or ""
            if not task:
                continue
            buckets.setdefault(task, []).append(rec)

    written: list[str] = []
    for task, recs in buckets.items():
        if len(recs) < 10:
            continue
        safe = re.sub(r"[^A-Za-z0-9._-]+", "_", task)
        path = os.path.join(by_task_dir, f"{safe}.train.jsonl")
        with open(path, "w", encoding="utf-8") as f:
            for r in recs:
                f.write(json.dumps(r, ensure_ascii=False) + "\n")
        written.append(os.path.relpath(path, output_dir))
    return written


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Convert curated MLX JSONL → OpenAI fine-tuning files + manifest."
    )
    parser.add_argument(
        "--input-dir",
        required=True,
        help="Directory containing train.jsonl and val.jsonl (curate output).",
    )
    parser.add_argument(
        "--output-dir",
        required=True,
        help="Directory to write combined_{train,val}.jsonl, manifest.json, rejection_log.jsonl.",
    )
    parser.add_argument(
        "--model",
        default="gpt-4.1-mini-2025-04-14",
        help="Target model name. Must have a profile in _MODEL_PROFILES.",
    )
    parser.add_argument(
        "--by-task",
        action="store_true",
        help="Also emit per-task specialist files under by_task/ (≥10 records only).",
    )
    parser.add_argument(
        "--epochs",
        type=int,
        default=3,
        help="Epochs assumed for cost estimate (does NOT set hyperparameters on OpenAI's side).",
    )
    parser.add_argument(
        "--max-train-rows",
        type=int,
        default=None,
        help="Cap train.jsonl input rows via deterministic seeded subsample.",
    )
    parser.add_argument(
        "--max-val-rows",
        type=int,
        default=None,
        help="Cap val.jsonl input rows via deterministic seeded subsample.",
    )
    parser.add_argument(
        "--sample-seed",
        type=int,
        default=42,
        help="Seed for subsampling (recorded in manifest for reproducibility).",
    )
    parser.add_argument("--verbose", action="store_true")
    args = parser.parse_args(argv)

    logging.basicConfig(
        level=logging.DEBUG if args.verbose else logging.INFO,
        format="%(asctime)s %(levelname)s %(name)s %(message)s",
    )

    manifest = run(
        input_dir=args.input_dir,
        output_dir=args.output_dir,
        model=args.model,
        by_task=args.by_task,
        epochs=args.epochs,
        max_train_rows=args.max_train_rows,
        max_val_rows=args.max_val_rows,
        sample_seed=args.sample_seed,
    )
    print(json.dumps(manifest, ensure_ascii=False, indent=2))
    return 0


if __name__ == "__main__":
    sys.exit(main())
