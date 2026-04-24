"""Deterministic carve of validation golden holdout from Phase 5 valid splits.

Reads all training_data/sft/*/valid.jsonl rows, shuffles by a configured seed,
takes the first `fraction` for the golden file. SHA256 is stamped on the
output path (alongside the file) for audit.

Usage:
    python -m neural.benchmarks.carve_golden --config configs/benchmark_phase10.yaml
"""
from __future__ import annotations

import argparse
import glob
import hashlib
import json
import random
import sys
from pathlib import Path
from typing import Any

try:
    import yaml
except ImportError:  # pragma: no cover
    print("PyYAML required: pip install pyyaml", file=sys.stderr)
    sys.exit(2)


def _load_config(path: Path) -> dict[str, Any]:
    with path.open() as fh:
        return yaml.safe_load(fh)


def carve(source_glob: str, out_path: Path, fraction: float, seed: int) -> dict[str, Any]:
    """Collect rows from source_glob, shuffle seeded, take fraction, write to out_path."""
    source_files = sorted(glob.glob(source_glob))
    if not source_files:
        return {"status": "fail", "reason": f"no files matched: {source_glob}"}

    rows: list[tuple[str, str]] = []  # (source_file, line_text)
    for src in source_files:
        with Path(src).open() as fh:
            for line in fh:
                line = line.rstrip("\n")
                if line:
                    rows.append((src, line))

    if not rows:
        return {"status": "fail", "reason": "no rows found across source files"}

    rng = random.Random(seed)
    rng.shuffle(rows)

    take = max(1, int(len(rows) * fraction))
    selected = rows[:take]

    out_path.parent.mkdir(parents=True, exist_ok=True)
    with out_path.open("w") as fh:
        for _, line in selected:
            fh.write(line + "\n")

    # SHA256 of the output file
    sha = hashlib.sha256()
    with out_path.open("rb") as fh:
        for chunk in iter(lambda: fh.read(1 << 16), b""):
            sha.update(chunk)
    digest = sha.hexdigest()

    sha_sidecar = out_path.with_suffix(out_path.suffix + ".sha256")
    with sha_sidecar.open("w") as fh:
        fh.write(f"{digest}  {out_path.name}\n")

    per_source: dict[str, int] = {}
    for src, _ in selected:
        per_source[src] = per_source.get(src, 0) + 1

    return {
        "status": "pass",
        "source_files": source_files,
        "total_source_rows": len(rows),
        "selected_rows": len(selected),
        "fraction": fraction,
        "seed": seed,
        "per_source_selected": per_source,
        "out_path": str(out_path),
        "sha256": digest,
    }


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--config", type=Path,
                    default=Path("configs/benchmark_phase10.yaml"))
    ap.add_argument("--out", type=Path, default=None,
                    help="override config.golden_holdout.out_path")
    ap.add_argument("--fraction", type=float, default=None,
                    help="override config.golden_holdout.fraction")
    ap.add_argument("--seed", type=int, default=None,
                    help="override config.golden_holdout.seed")
    args = ap.parse_args()

    if not args.config.exists():
        print(f"config not found: {args.config}", file=sys.stderr)
        return 2

    config = _load_config(args.config)
    gh = config["golden_holdout"]
    out_path = args.out or Path(gh["out_path"])
    fraction = args.fraction if args.fraction is not None else float(gh["fraction"])
    seed = args.seed if args.seed is not None else int(gh["seed"])
    source_glob = gh["source_glob"]

    report = carve(source_glob, out_path, fraction, seed)
    print(json.dumps(report, indent=2, sort_keys=True))
    return 0 if report["status"] == "pass" else 1


if __name__ == "__main__":
    sys.exit(main())
