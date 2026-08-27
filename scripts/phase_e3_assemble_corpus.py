#!/usr/bin/env python3
"""PHASE-E3-RETRAIN-BENCHMARK-001 corpus assembler.

Concatenates the five family train.jsonl files into a single training corpus,
holds out per-family valid.jsonl rows into a single validation set, and
emits a manifest with SHAs + provenance. Mirrors E2's byte-verbatim rule:
raw line copy, no JSON round-trip, so SHA-diffs across corpus generations
stay meaningful.

Design rules (from CLAUDE.md pins):
- SHA-verify each source pre-read AND post-read (fail-hard on mid-run mutation).
- Byte-verbatim: raw line copy, not JSON round-trip.
- Deterministic ordering: source files read in a fixed order; within each
  file, lines are preserved in file order (no shuffle here — mlx_lm.lora
  handles shuffling per-epoch).
- v3-stripped has no valid.jsonl; hold out the LAST N rows from its train
  as valid (deterministic; file-order tail).
- Manifest names every source + per-family row count + output SHAs.
- Leak audit is a SEPARATE step (scripts/audit_eval_leakage.py); this
  script does not run it — but the sprint gate requires it before training.

Usage:
    python3 scripts/phase_e3_assemble_corpus.py \\
        --out training_data/sft/e3_v1_base_v3 \\
        [--v3-holdout 50]

Idempotent: if the output dir exists and manifests match, exits 0 with a
"nothing to do" notice.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import sys
from datetime import datetime, timezone
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
SFT_ROOT = REPO_ROOT / "training_data" / "sft"

# Fixed source order — deterministic output regardless of filesystem ordering.
# Families with both train + valid come first; v3-stripped last since it needs
# a train→valid split.
FAMILIES_WITH_VALID = [
    "tier1",
    "family_reasoning_think",
    "family_classify_notink",
    "family_structured_notink",
]
V3_STRIPPED = "claude_code_knowledge_v3_stripped"


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def read_lines_verbatim(path: Path) -> tuple[bytes, str, int]:
    """Read a file as raw bytes, return (bytes, sha256, line_count).

    Byte-verbatim: no JSON round-trip; preserves original line endings.
    """
    data = path.read_bytes()
    sha = sha256_bytes(data)
    # Count lines by counting '\n' occurrences; last line may or may not end
    # with '\n'. mlx_lm.lora is line-oriented; a missing trailing newline is
    # a real training row too.
    n_lines = data.count(b"\n")
    if data and not data.endswith(b"\n"):
        n_lines += 1
    return data, sha, n_lines


def split_v3_train_valid(v3_bytes: bytes, valid_holdout: int) -> tuple[bytes, bytes, int, int]:
    """Split v3-stripped bytes into (train_bytes, valid_bytes, train_n, valid_n).

    Deterministic: the LAST `valid_holdout` lines become valid; the rest
    stay as train. Line boundaries respected.
    """
    # Ensure trailing newline for a clean split
    normalized = v3_bytes if v3_bytes.endswith(b"\n") else v3_bytes + b"\n"
    # Find split point: locate the position of the (n_total - valid_holdout)th newline from start,
    # then split just after it.
    lines = normalized.split(b"\n")
    # Split removes trailing empty element after final '\n'; drop it if present.
    if lines and lines[-1] == b"":
        lines = lines[:-1]
    n_total = len(lines)
    if valid_holdout <= 0:
        return b"\n".join(lines) + b"\n", b"", n_total, 0
    if valid_holdout >= n_total:
        raise ValueError(f"valid_holdout {valid_holdout} >= total lines {n_total}")
    train_lines = lines[: n_total - valid_holdout]
    valid_lines = lines[n_total - valid_holdout :]
    train_bytes = b"\n".join(train_lines) + b"\n"
    valid_bytes = b"\n".join(valid_lines) + b"\n"
    return train_bytes, valid_bytes, len(train_lines), len(valid_lines)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--out", type=Path, required=True, help="output dir (created if absent)")
    ap.add_argument(
        "--v3-holdout",
        type=int,
        default=50,
        help="rows to hold out from v3-stripped train for validation (default 50)",
    )
    ap.add_argument(
        "--force",
        action="store_true",
        help="overwrite existing output; without this, assembler refuses if the manifest looks intact",
    )
    args = ap.parse_args()

    out_dir: Path = args.out
    out_dir.mkdir(parents=True, exist_ok=True)

    train_out = out_dir / "train.jsonl"
    valid_out = out_dir / "valid.jsonl"
    manifest_out = out_dir / "manifest.json"

    if not args.force and manifest_out.exists() and train_out.exists() and valid_out.exists():
        try:
            existing_manifest = json.loads(manifest_out.read_text())
        except Exception:
            existing_manifest = None
        if existing_manifest and "file_sha256" in existing_manifest:
            live_train_sha = sha256_file(train_out)
            live_valid_sha = sha256_file(valid_out)
            if (
                existing_manifest["file_sha256"].get("train.jsonl") == live_train_sha
                and existing_manifest["file_sha256"].get("valid.jsonl") == live_valid_sha
            ):
                print(f"nothing to do — {out_dir}/{{train,valid,manifest}} already assembled + intact")
                print(f"  train SHA: {live_train_sha}")
                print(f"  valid SHA: {live_valid_sha}")
                return 0

    # Read each source, capturing bytes + SHA + line count
    source_records: list[dict] = []
    train_pieces: list[tuple[str, bytes]] = []
    valid_pieces: list[tuple[str, bytes]] = []

    # 1. Families with valid.jsonl
    for fam in FAMILIES_WITH_VALID:
        fam_dir = SFT_ROOT / fam
        train_path = fam_dir / "train.jsonl"
        valid_path = fam_dir / "valid.jsonl"
        if not train_path.exists() or not valid_path.exists():
            print(f"ERROR: {fam} missing train.jsonl or valid.jsonl", file=sys.stderr)
            return 1
        t_data, t_sha, t_n = read_lines_verbatim(train_path)
        v_data, v_sha, v_n = read_lines_verbatim(valid_path)
        source_records.append(
            {
                "family": fam,
                "train_sha256": t_sha,
                "train_rows": t_n,
                "valid_sha256": v_sha,
                "valid_rows": v_n,
                "train_source": str(train_path.relative_to(REPO_ROOT)),
                "valid_source": str(valid_path.relative_to(REPO_ROOT)),
            }
        )
        train_pieces.append((fam, t_data))
        valid_pieces.append((fam, v_data))

    # 2. v3-stripped — split train into train + valid
    v3_dir = SFT_ROOT / V3_STRIPPED
    v3_train_path = v3_dir / "train.jsonl"
    if not v3_train_path.exists():
        print(f"ERROR: {V3_STRIPPED} missing train.jsonl", file=sys.stderr)
        return 1
    v3_data, v3_sha, v3_n = read_lines_verbatim(v3_train_path)
    v3_train_bytes, v3_valid_bytes, v3_train_n, v3_valid_n = split_v3_train_valid(v3_data, args.v3_holdout)
    source_records.append(
        {
            "family": V3_STRIPPED,
            "source_sha256": v3_sha,
            "source_rows": v3_n,
            "split_holdout": args.v3_holdout,
            "assembled_train_rows": v3_train_n,
            "assembled_valid_rows": v3_valid_n,
            "source_path": str(v3_train_path.relative_to(REPO_ROOT)),
            "note": "no valid.jsonl in source; last N rows of train held out deterministically",
        }
    )
    train_pieces.append((V3_STRIPPED, v3_train_bytes))
    valid_pieces.append((V3_STRIPPED, v3_valid_bytes))

    # 3. Post-read SHA verify — fail-hard if any source mutated mid-run
    print("post-read SHA verify (fail-hard on any drift):")
    for rec in source_records:
        if rec["family"] == V3_STRIPPED:
            live = sha256_file(SFT_ROOT / V3_STRIPPED / "train.jsonl")
            want = rec["source_sha256"]
            if live != want:
                print(
                    f"  FAIL: {V3_STRIPPED} train.jsonl changed during assembly (pre={want} post={live})",
                    file=sys.stderr,
                )
                return 2
            print(f"  ok: {V3_STRIPPED} train.jsonl {live[:16]}…")
        else:
            for kind in ("train", "valid"):
                live = sha256_file(SFT_ROOT / rec["family"] / f"{kind}.jsonl")
                want = rec[f"{kind}_sha256"]
                if live != want:
                    print(
                        f"  FAIL: {rec['family']} {kind}.jsonl changed during assembly (pre={want} post={live})",
                        file=sys.stderr,
                    )
                    return 2
                print(f"  ok: {rec['family']} {kind}.jsonl {live[:16]}…")

    # 4. Concatenate pieces byte-verbatim (no reserialization)
    #    Ensure every intermediate piece ends with '\n' so the last row of
    #    piece K and the first row of piece K+1 are on separate lines.
    def _ensure_nl(chunks: list[tuple[str, bytes]]) -> bytes:
        out = bytearray()
        for _fam, chunk in chunks:
            if not chunk:
                continue
            out += chunk
            if not chunk.endswith(b"\n"):
                out += b"\n"
        return bytes(out)

    train_bytes = _ensure_nl(train_pieces)
    valid_bytes = _ensure_nl(valid_pieces)

    train_out.write_bytes(train_bytes)
    valid_out.write_bytes(valid_bytes)
    train_sha = sha256_bytes(train_bytes)
    valid_sha = sha256_bytes(valid_bytes)
    train_rows_out = train_bytes.count(b"\n")
    valid_rows_out = valid_bytes.count(b"\n")

    # 5. Manifest
    per_family_train_counts = {}
    per_family_valid_counts = {}
    for rec in source_records:
        if rec["family"] == V3_STRIPPED:
            per_family_train_counts[V3_STRIPPED] = rec["assembled_train_rows"]
            per_family_valid_counts[V3_STRIPPED] = rec["assembled_valid_rows"]
        else:
            per_family_train_counts[rec["family"]] = rec["train_rows"]
            per_family_valid_counts[rec["family"]] = rec["valid_rows"]

    manifest = {
        "sprint": "PHASE-E3-RETRAIN-BENCHMARK-001",
        "epic": "1 (corpus assembly)",
        "family_name": "e3_v1_base_v3",
        "base_dataset_ver": "e3_v1_base_v3",
        "meta_placement": "embedded",
        "row_counts": {"train": train_rows_out, "valid": valid_rows_out},
        "per_family_train_counts": per_family_train_counts,
        "per_family_valid_counts": per_family_valid_counts,
        "file_sha256": {"train.jsonl": train_sha, "valid.jsonl": valid_sha},
        "sources": source_records,
        "assembly_ordering": FAMILIES_WITH_VALID + [V3_STRIPPED],
        "v3_holdout": args.v3_holdout,
        "byte_verbatim": True,
        "base_model_name": "mlx-community/Qwen3-14B-4bit",
        "leak_audit_required": "run scripts/audit_eval_leakage.py --eval training_data/eval/valid_clean.jsonl --against ./train.jsonl",
        "generated_at_utc": datetime.now(timezone.utc).isoformat(),
    }
    manifest_out.write_text(json.dumps(manifest, indent=2, ensure_ascii=False) + "\n")

    print()
    print(f"assembled {out_dir}/train.jsonl  {train_rows_out} rows  sha={train_sha[:16]}…")
    print(f"assembled {out_dir}/valid.jsonl  {valid_rows_out} rows  sha={valid_sha[:16]}…")
    print(f"assembled {out_dir}/manifest.json")
    print()
    print("NEXT: run leak audit")
    print(f"  python3 scripts/audit_eval_leakage.py --eval training_data/eval/valid_clean.jsonl --against {out_dir}/train.jsonl")
    return 0


if __name__ == "__main__":
    sys.exit(main())
