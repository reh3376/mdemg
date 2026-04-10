#!/usr/bin/env python3
"""
format_converter.py — MLX Chat Format Converter

Converts filtered LLM interaction JSONL into HuggingFace chat template
format suitable for mlx-lm LoRA fine-tuning.

Output format:
    {"messages": [
        {"role": "system", "content": "..."},
        {"role": "user", "content": "..."},
        {"role": "assistant", "content": "..."}
    ]}

RAFT context handling (80/20):
    - 80% of RAFT-eligible records (those with retrieval_node_ids):
      include retrieval context metadata in user message
    - 20%: strip retrieval context (teach model to work without it)
    - Uses deterministic seed (hash of trace_id) for reproducibility

Usage:
    python -m training.format_converter \\
        --input /tmp/filtered/llm_interactions.jsonl \\
        --output /tmp/train.jsonl \\
        --raft-ratio 0.8 \\
        --format chat
"""

import argparse
import hashlib
import json
import os
from typing import Any


def _raft_include(trace_id: str, ratio: float) -> bool:
    """Deterministic RAFT context inclusion based on trace_id hash.

    Returns True if this record should include retrieval context.
    Uses first 8 hex chars of SHA-256(trace_id) as a deterministic seed.
    """
    h = hashlib.sha256(trace_id.encode()).hexdigest()[:8]
    val = int(h, 16) / 0xFFFFFFFF
    return val < ratio


def _build_raft_context(record: dict[str, Any]) -> str:
    """Build RAFT context block from retrieval node IDs and scores.

    Node text lives in Neo4j, not TSDB. For this sprint, RAFT context
    uses IDs + scores only. The user_prompt already contains the full
    candidate text for rerank/classify tasks.
    """
    node_ids = record.get("retrieval_node_ids") or []
    scores = record.get("retrieval_scores") or []

    if not node_ids:
        return ""

    lines = ["[Retrieved Context]"]
    for i, nid in enumerate(node_ids):
        score = scores[i] if i < len(scores) else 0.0
        lines.append(f"Node {nid} (score: {score:.2f})")
    return "\n".join(lines)


def _build_user_content(record: dict[str, Any], include_raft: bool) -> str:
    """Build the user message content, optionally with RAFT context."""
    user_prompt = record.get("user_prompt", "") or ""

    if not include_raft:
        return user_prompt

    raft_context = _build_raft_context(record)
    if not raft_context:
        return user_prompt

    return f"{raft_context}\n\n[Query]\n{user_prompt}"


def _build_assistant_content(record: dict[str, Any]) -> str:
    """Build assistant response, including think content if present."""
    response = record.get("response", "") or ""
    think_content = record.get("think_content")
    think_mode = record.get("think_mode", False)

    if think_mode and think_content:
        return f"<think>\n{think_content}\n</think>\n\n{response}"

    return response


def convert_record(record: dict[str, Any], raft_ratio: float = 0.8) -> dict[str, Any]:
    """Convert a single LLM interaction record to MLX chat format.

    Returns dict with "messages" key containing role/content pairs.
    """
    system_prompt = record.get("system_prompt", "") or ""
    trace_id = record.get("trace_id", "") or ""

    # Determine RAFT context inclusion
    has_retrieval = bool(record.get("retrieval_node_ids"))
    include_raft = has_retrieval and _raft_include(trace_id, raft_ratio)

    user_content = _build_user_content(record, include_raft)
    assistant_content = _build_assistant_content(record)

    messages = []
    if system_prompt:
        messages.append({"role": "system", "content": system_prompt})
    messages.append({"role": "user", "content": user_content})
    messages.append({"role": "assistant", "content": assistant_content})

    return {"messages": messages}


def convert_dpo_record(record: dict[str, Any]) -> dict[str, Any]:
    """Convert a DPO pair record to standard DPO format.

    Input: {"prompt": ..., "chosen": ..., "rejected": ..., "metadata": ...}
    Output: same structure with think_mode preservation applied.
    """
    return {
        "prompt": record.get("prompt", ""),
        "chosen": record.get("chosen", ""),
        "rejected": record.get("rejected", ""),
        "metadata": record.get("metadata", {}),
    }


def validate_dpo_format(record: dict[str, Any]) -> list[str]:
    """Validate a DPO record has required keys and non-empty values.

    Returns list of error strings (empty = valid).
    """
    errors = []
    for key in ("prompt", "chosen", "rejected"):
        if key not in record:
            errors.append(f"missing '{key}' key")
        elif not record[key]:
            errors.append(f"empty '{key}' value")

    if "metadata" in record and not isinstance(record["metadata"], dict):
        errors.append("'metadata' must be a dict")

    return errors


def run_dpo_converter(
    input_path: str,
    output_path: str,
) -> dict[str, Any]:
    """Run DPO format conversion pipeline.

    Reads DPO pair JSONL, validates, and writes clean output.
    Returns a conversion report dict.
    """
    input_rows = 0
    output_rows = 0
    validation_errors = 0

    os.makedirs(os.path.dirname(output_path) or ".", exist_ok=True)

    with open(input_path) as fin, open(output_path, "w") as fout:
        for line in fin:
            line = line.strip()
            if not line:
                continue
            input_rows += 1

            try:
                record = json.loads(line)
            except json.JSONDecodeError:
                continue

            converted = convert_dpo_record(record)
            errors = validate_dpo_format(converted)
            if errors:
                validation_errors += 1
                continue

            fout.write(json.dumps(converted) + "\n")
            output_rows += 1

    return {
        "input_rows": input_rows,
        "output_rows": output_rows,
        "validation_errors": validation_errors,
    }


def validate_mlx_format(record: dict[str, Any]) -> list[str]:
    """Validate a converted record conforms to MLX chat format.

    Returns list of error strings (empty = valid).
    """
    errors = []

    if "messages" not in record:
        errors.append("missing 'messages' key")
        return errors

    messages = record["messages"]
    if not isinstance(messages, list) or len(messages) < 2:
        errors.append("messages must be a list with >= 2 entries")
        return errors

    valid_roles = {"system", "user", "assistant"}
    for i, msg in enumerate(messages):
        if "role" not in msg:
            errors.append(f"message[{i}] missing 'role'")
        elif msg["role"] not in valid_roles:
            errors.append(f"message[{i}] invalid role: {msg['role']}")
        if "content" not in msg:
            errors.append(f"message[{i}] missing 'content'")
        elif not msg.get("content"):
            errors.append(f"message[{i}] empty content")

    if messages and messages[-1].get("role") != "assistant":
        errors.append("last message must have role 'assistant'")

    if messages and messages[0].get("role") == "system" and len(messages) < 3:
        errors.append("system message present but fewer than 3 messages total")

    return errors


def run_converter(
    input_path: str,
    output_path: str,
    raft_ratio: float = 0.8,
) -> dict[str, Any]:
    """Run the format conversion pipeline.

    Returns a conversion report dict.
    """
    input_rows = 0
    output_rows = 0
    validation_errors = 0
    raft_included = 0
    raft_excluded = 0

    os.makedirs(os.path.dirname(output_path) or ".", exist_ok=True)

    with open(input_path) as fin, open(output_path, "w") as fout:
        for line in fin:
            line = line.strip()
            if not line:
                continue
            input_rows += 1

            try:
                record = json.loads(line)
            except json.JSONDecodeError:
                continue

            converted = convert_record(record, raft_ratio)

            # Validate
            errors = validate_mlx_format(converted)
            if errors:
                validation_errors += 1
                continue

            # Track RAFT stats
            has_retrieval = bool(record.get("retrieval_node_ids"))
            if has_retrieval:
                trace_id = record.get("trace_id", "")
                if _raft_include(trace_id, raft_ratio):
                    raft_included += 1
                else:
                    raft_excluded += 1

            fout.write(json.dumps(converted) + "\n")
            output_rows += 1

    return {
        "input_rows": input_rows,
        "output_rows": output_rows,
        "validation_errors": validation_errors,
        "raft_included": raft_included,
        "raft_excluded": raft_excluded,
        "raft_ratio_actual": (
            raft_included / (raft_included + raft_excluded)
            if (raft_included + raft_excluded) > 0
            else 0.0
        ),
    }


def main():
    parser = argparse.ArgumentParser(description="MLX Chat Format Converter")
    parser.add_argument("--input", required=True, help="Input filtered JSONL file")
    parser.add_argument("--output", required=True, help="Output MLX chat JSONL file")
    parser.add_argument("--raft-ratio", type=float, default=0.8,
                        help="Fraction of RAFT-eligible records to include context (default: 0.8)")
    parser.add_argument("--format", default="chat", choices=["chat", "dpo"],
                        help="Output format (default: chat)")
    args = parser.parse_args()

    if args.format == "dpo":
        report = run_dpo_converter(args.input, args.output)
        print(f"Input:  {report['input_rows']} rows")
        print(f"Output: {report['output_rows']} rows")
        if report["validation_errors"]:
            print(f"Validation errors: {report['validation_errors']}")
        return

    report = run_converter(args.input, args.output, args.raft_ratio)

    print(f"Input:  {report['input_rows']} rows")
    print(f"Output: {report['output_rows']} rows")
    if report["validation_errors"]:
        print(f"Validation errors: {report['validation_errors']}")
    print(f"RAFT included: {report['raft_included']}")
    print(f"RAFT excluded: {report['raft_excluded']}")
    if report["raft_included"] + report["raft_excluded"] > 0:
        print(f"RAFT ratio:    {report['raft_ratio_actual']:.2f}")


if __name__ == "__main__":
    main()
