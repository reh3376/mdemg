#!/usr/bin/env python3
"""
openai_ft_results_backfill.py — Retroactively add canonical ``parse_ok`` to an
existing ``results.jsonl`` produced by an older ``openai_ft_baseline_eval.py``
run (pre-FT-OAI-002 Epic 1).

Before Epic 1 the eval harness wrote the field as ``parses_as_json`` (and the
downstream convention was ``parse_ok``). This helper reconciles the two:

    - If a row has ``parse_ok`` already, leave it alone.
    - Else if it has ``parses_as_json``, copy that into ``parse_ok``.
    - Else recompute from ``response`` using the same lenient parser as the
      live eval and add the ``parse_ok`` field.

``finish_reason`` and per-record token counts are **not** backfillable — those
values lived only in the original HTTP response and are gone. Only ``parse_ok``
is pure-function-of-response-text, so that's all this script does.

Usage:
    python scripts/openai_ft_results_backfill.py \\
        --results training_data/openai_ft/20260420/eval/ft/results.jsonl \\
        --in-place
"""

from __future__ import annotations

import argparse
import json
import os
import sys

# Reuse the exact parser from the eval script so backfilled rows are
# definitionally consistent with fresh rows.
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from openai_ft_baseline_eval import try_parse_json  # noqa: E402


def backfill_row(row: dict) -> tuple[dict, str]:
    """Return (updated_row, source) where source is 'existing' | 'alias' | 'recomputed' | 'error_row'."""
    # Skip error rows — they have no response to parse.
    if "error" in row and "response" not in row:
        return row, "error_row"

    if "parse_ok" in row and row["parse_ok"] is not None:
        return row, "existing"

    if "parses_as_json" in row and row["parses_as_json"] is not None:
        row["parse_ok"] = row["parses_as_json"]
        return row, "alias"

    response = row.get("response", "")
    row["parse_ok"] = try_parse_json(response or "")
    return row, "recomputed"


def run(in_path: str, out_path: str) -> dict:
    counts = {"existing": 0, "alias": 0, "recomputed": 0, "error_row": 0, "total": 0, "pass": 0}
    rows: list[dict] = []
    with open(in_path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            counts["total"] += 1
            row = json.loads(line)
            row, source = backfill_row(row)
            counts[source] += 1
            if row.get("parse_ok") is True:
                counts["pass"] += 1
            rows.append(row)

    # Write atomically: write to `.tmp` then rename.
    tmp_path = out_path + ".tmp"
    with open(tmp_path, "w", encoding="utf-8") as f:
        for row in rows:
            f.write(json.dumps(row, ensure_ascii=False) + "\n")
    os.replace(tmp_path, out_path)

    # Compute aggregate parse_ok rate across non-error rows.
    non_error = counts["total"] - counts["error_row"]
    counts["parse_ok_rate"] = (counts["pass"] / non_error) if non_error else 0.0
    return counts


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--results", required=True, help="Path to results.jsonl")
    p.add_argument("--output", help="Output path (default: same as --results when --in-place)")
    p.add_argument("--in-place", action="store_true", help="Overwrite --results")
    args = p.parse_args(argv)

    if not args.in_place and not args.output:
        print("ABORT: --output required unless --in-place given", file=sys.stderr)
        return 2

    out_path = args.results if args.in_place else args.output
    stats = run(args.results, out_path)
    print(
        f"backfilled {stats['total']} rows "
        f"(existing={stats['existing']}, alias={stats['alias']}, "
        f"recomputed={stats['recomputed']}, error_rows={stats['error_row']}) · "
        f"parse_ok rate = {stats['parse_ok_rate']:.4f} · wrote {out_path}"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
