#!/usr/bin/env python3
"""
openai_ft_check.py — Pre-upload validator for OpenAI fine-tuning artifacts.

Reads a directory produced by `training.openai_ft_adapter` (combined_train.jsonl,
combined_val.jsonl, manifest.json) and verifies:

    1. Required files exist.
    2. Manifest schema_version is recognized.
    3. SHA256 digests in manifest match re-computed digests of the JSONL files.
    4. Every record parses as JSON and has a valid OpenAI chat-messages shape.
    5. Row counts match manifest claims.
    6. Cost estimate is present and non-negative.
    7. Minimum sample thresholds (OpenAI: ≥10 train, recommended ≥50 train + ≥20 val).

Exits 0 if all checks pass. Exits 1 with a list of errors otherwise.

Usage:
    python scripts/openai_ft_check.py \\
        --dir training_data/openai_ft/20260420
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys


def _sha256_of_file(path: str) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def _iter_jsonl(path: str):
    with open(path, encoding="utf-8") as f:
        for lineno, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                yield lineno, json.loads(line)
            except json.JSONDecodeError as e:
                yield lineno, {"__parse_error__": str(e)}


def _check_record(rec: dict) -> str:
    if "__parse_error__" in rec:
        return f"parse error: {rec['__parse_error__']}"
    msgs = rec.get("messages")
    if not isinstance(msgs, list) or not msgs:
        return "messages missing or empty"
    roles = {m.get("role") for m in msgs if isinstance(m, dict)}
    if not roles.issubset({"system", "user", "assistant"}):
        return f"invalid roles: {roles - {'system', 'user', 'assistant'}}"
    if "assistant" not in roles:
        return "no assistant message"
    for m in msgs:
        c = m.get("content") if isinstance(m, dict) else None
        if not isinstance(c, str) or not c.strip():
            return f"non-string or empty content (role={m.get('role') if isinstance(m, dict) else None})"
    return ""


def check(directory: str) -> list[str]:
    errors: list[str] = []
    required = ["combined_train.jsonl", "combined_val.jsonl", "manifest.json"]
    for f in required:
        if not os.path.exists(os.path.join(directory, f)):
            errors.append(f"missing file: {f}")
    if errors:
        return errors

    with open(os.path.join(directory, "manifest.json"), encoding="utf-8") as f:
        manifest = json.load(f)

    if manifest.get("schema_version") != 1:
        errors.append(
            f"unsupported manifest schema_version: {manifest.get('schema_version')}"
        )

    # SHA256 integrity + row counts + record validation per split.
    for split_name, split in manifest.get("splits", {}).items():
        path = split.get("path")
        if not path or not os.path.exists(path):
            # Fall back to relative path inside directory.
            path = os.path.join(directory, f"combined_{split_name}.jsonl")
        if not os.path.exists(path):
            errors.append(f"{split_name}: file not found at {path}")
            continue

        actual_sha = _sha256_of_file(path)
        claimed_sha = split.get("sha256")
        if claimed_sha and claimed_sha != actual_sha:
            errors.append(
                f"{split_name}: sha256 mismatch (manifest={claimed_sha[:12]}…, actual={actual_sha[:12]}…)"
            )

        rows = 0
        for lineno, rec in _iter_jsonl(path):
            rows += 1
            reason = _check_record(rec)
            if reason:
                errors.append(f"{split_name}:{lineno}: {reason}")
                if len([e for e in errors if e.startswith(f"{split_name}:")]) > 20:
                    errors.append(f"{split_name}: (truncated — >20 bad records)")
                    break
        claimed_rows = split.get("rows")
        if claimed_rows is not None and rows != claimed_rows:
            errors.append(
                f"{split_name}: row count mismatch (manifest={claimed_rows}, file={rows})"
            )

    # Sample-size minima.
    train_rows = manifest.get("splits", {}).get("train", {}).get("rows", 0)
    val_rows = manifest.get("splits", {}).get("val", {}).get("rows", 0)
    if train_rows < 10:
        errors.append(f"train rows {train_rows} below OpenAI hard minimum of 10")
    if train_rows < 50:
        errors.append(
            f"train rows {train_rows} below recommended floor of 50 (non-fatal warning)"
        )
    if val_rows < 20:
        errors.append(
            f"val rows {val_rows} below recommended floor of 20 (non-fatal warning)"
        )

    cost = manifest.get("totals", {}).get("cost_estimate_usd")
    if cost is None or cost < 0:
        errors.append(f"invalid cost_estimate_usd: {cost}")

    return errors


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--dir",
        required=True,
        help="Directory produced by training.openai_ft_adapter",
    )
    args = parser.parse_args(argv)

    errors = check(args.dir)
    # Separate fatal errors from "below recommended" warnings.
    fatal = [e for e in errors if "non-fatal" not in e]
    warn = [e for e in errors if "non-fatal" in e]

    if warn:
        print("WARNINGS:", file=sys.stderr)
        for w in warn:
            print(f"  - {w}", file=sys.stderr)

    if fatal:
        print(f"FAIL: {len(fatal)} errors", file=sys.stderr)
        for e in fatal:
            print(f"  - {e}", file=sys.stderr)
        return 1

    print(f"OK: {args.dir} passes all fatal pre-upload checks.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
