#!/usr/bin/env python3
"""MDEMG-USAGE-LORA-001 Epic 1b — 6-family corpus assembler.

Extends PHASE-E3-RETRAIN-BENCHMARK-001's assembler to include the new
`mdemg_usage_v1` family (split via scripts/split_mdemg_usage_v1.py into
train_split.jsonl + valid_split.jsonl; benchmark_holdout.jsonl stays as
the E4 benchmark eval source, not in the training corpus).

Combined corpus:
    1. tier1                            (train + valid)
    2. family_reasoning_think           (train + valid)
    3. family_classify_notink           (train + valid)
    4. family_structured_notink         (train + valid)
    5. claude_code_knowledge_v3_stripped (train only; last 50 rows held out as valid via E3 pattern)
    6. mdemg_usage_v1                   (train_split.jsonl + valid_split.jsonl)  ← NEW

Byte-verbatim: raw line copy, not JSON round-trip. SHA-verify pre + post
per E3 rule "fail-hard on mid-run mutation".

Usage:
    python3 scripts/mdemg_usage_lora_assemble.py \\
        --out training_data/sft/mdemg_usage_lora_001

Idempotent: skip re-assembly if manifest matches.

Sprint: docs/development/mdemg-usage-lora-001/
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

FAMILIES_WITH_VALID = [
    "tier1",
    "family_reasoning_think",
    "family_classify_notink",
    "family_structured_notink",
]
V3_STRIPPED = "claude_code_knowledge_v3_stripped"
MDEMG_USAGE = "mdemg_usage_v1"  # NEW — uses train_split/valid_split


def sha256_bytes(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def read_lines_verbatim(path: Path) -> tuple[bytes, str, int]:
    data = path.read_bytes()
    sha = sha256_bytes(data)
    n_lines = data.count(b"\n")
    if data and not data.endswith(b"\n"):
        n_lines += 1
    return data, sha, n_lines


def split_v3_train_valid(v3_bytes: bytes, valid_holdout: int) -> tuple[bytes, bytes, int, int]:
    """Deterministic tail-holdout split (E3 pattern)."""
    normalized = v3_bytes if v3_bytes.endswith(b"\n") else v3_bytes + b"\n"
    lines = normalized.split(b"\n")
    if lines and lines[-1] == b"":
        lines = lines[:-1]
    n_total = len(lines)
    if valid_holdout <= 0:
        return b"\n".join(lines) + b"\n", b"", n_total, 0
    if valid_holdout >= n_total:
        raise ValueError(f"valid_holdout {valid_holdout} >= total {n_total}")
    train_lines = lines[: n_total - valid_holdout]
    valid_lines = lines[n_total - valid_holdout :]
    return (
        b"\n".join(train_lines) + b"\n",
        b"\n".join(valid_lines) + b"\n",
        len(train_lines),
        len(valid_lines),
    )


def _ensure_nl(chunks: list[tuple[str, bytes]]) -> bytes:
    out = bytearray()
    for _fam, chunk in chunks:
        if not chunk:
            continue
        out += chunk
        if not chunk.endswith(b"\n"):
            out += b"\n"
    return bytes(out)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.strip().splitlines()[0])
    ap.add_argument("--out", type=Path, required=True)
    ap.add_argument("--v3-holdout", type=int, default=50)
    ap.add_argument("--force", action="store_true")
    args = ap.parse_args()

    out_dir: Path = args.out
    out_dir.mkdir(parents=True, exist_ok=True)
    train_out = out_dir / "train.jsonl"
    valid_out = out_dir / "valid.jsonl"
    manifest_out = out_dir / "manifest.json"

    # Idempotent no-op
    if not args.force and manifest_out.exists() and train_out.exists() and valid_out.exists():
        try:
            existing = json.loads(manifest_out.read_text())
            if (
                existing.get("file_sha256", {}).get("train.jsonl") == sha256_file(train_out)
                and existing.get("file_sha256", {}).get("valid.jsonl") == sha256_file(valid_out)
            ):
                print(f"nothing to do — {out_dir} already assembled + intact")
                return 0
        except Exception:
            pass

    source_records: list[dict] = []
    train_pieces: list[tuple[str, bytes]] = []
    valid_pieces: list[tuple[str, bytes]] = []

    # Families 1-4: shipped train + valid
    for fam in FAMILIES_WITH_VALID:
        fam_dir = SFT_ROOT / fam
        t_path = fam_dir / "train.jsonl"
        v_path = fam_dir / "valid.jsonl"
        if not t_path.exists() or not v_path.exists():
            print(f"ERROR: {fam} missing train.jsonl or valid.jsonl", file=sys.stderr)
            return 1
        t_data, t_sha, t_n = read_lines_verbatim(t_path)
        v_data, v_sha, v_n = read_lines_verbatim(v_path)
        source_records.append({
            "family": fam,
            "train_sha256": t_sha, "train_rows": t_n,
            "valid_sha256": v_sha, "valid_rows": v_n,
            "train_source": str(t_path.relative_to(REPO_ROOT)),
            "valid_source": str(v_path.relative_to(REPO_ROOT)),
        })
        train_pieces.append((fam, t_data))
        valid_pieces.append((fam, v_data))

    # Family 5: v3-stripped (split last 50 as valid)
    v3_train_path = SFT_ROOT / V3_STRIPPED / "train.jsonl"
    if not v3_train_path.exists():
        print(f"ERROR: {V3_STRIPPED} missing train.jsonl", file=sys.stderr)
        return 1
    v3_data, v3_sha, v3_n = read_lines_verbatim(v3_train_path)
    v3_train_bytes, v3_valid_bytes, v3_train_n, v3_valid_n = split_v3_train_valid(v3_data, args.v3_holdout)
    source_records.append({
        "family": V3_STRIPPED,
        "source_sha256": v3_sha, "source_rows": v3_n,
        "split_holdout": args.v3_holdout,
        "assembled_train_rows": v3_train_n,
        "assembled_valid_rows": v3_valid_n,
        "source_path": str(v3_train_path.relative_to(REPO_ROOT)),
        "note": "no valid.jsonl in source; last N rows held out",
    })
    train_pieces.append((V3_STRIPPED, v3_train_bytes))
    valid_pieces.append((V3_STRIPPED, v3_valid_bytes))

    # Family 6 (NEW): mdemg_usage_v1 — train_split + valid_split
    mu_dir = SFT_ROOT / MDEMG_USAGE
    mu_train_path = mu_dir / "train_split.jsonl"
    mu_valid_path = mu_dir / "valid_split.jsonl"
    if not mu_train_path.exists() or not mu_valid_path.exists():
        print(f"ERROR: {MDEMG_USAGE} missing train_split.jsonl or valid_split.jsonl "
              f"(run scripts/split_mdemg_usage_v1.py first)", file=sys.stderr)
        return 1
    mu_t_data, mu_t_sha, mu_t_n = read_lines_verbatim(mu_train_path)
    mu_v_data, mu_v_sha, mu_v_n = read_lines_verbatim(mu_valid_path)
    source_records.append({
        "family": MDEMG_USAGE,
        "train_sha256": mu_t_sha, "train_rows": mu_t_n,
        "valid_sha256": mu_v_sha, "valid_rows": mu_v_n,
        "train_source": str(mu_train_path.relative_to(REPO_ROOT)),
        "valid_source": str(mu_valid_path.relative_to(REPO_ROOT)),
        "note": "sprint=MDEMG-USAGE-CORPUS-CURATE-001 (task #144); split via split_mdemg_usage_v1.py (SHA(row_id) mod 10)",
    })
    train_pieces.append((MDEMG_USAGE, mu_t_data))
    valid_pieces.append((MDEMG_USAGE, mu_v_data))

    # Post-read SHA verify
    print("post-read SHA verify:")
    for rec in source_records:
        fam = rec["family"]
        if fam == V3_STRIPPED:
            live = sha256_file(SFT_ROOT / V3_STRIPPED / "train.jsonl")
            if live != rec["source_sha256"]:
                print(f"  FAIL: {fam} train.jsonl mutated mid-run", file=sys.stderr)
                return 2
            print(f"  ok: {fam} train.jsonl {live[:16]}…")
        elif fam == MDEMG_USAGE:
            for kind, path in (("train_split", mu_train_path), ("valid_split", mu_valid_path)):
                live = sha256_file(path)
                want = rec["train_sha256"] if kind == "train_split" else rec["valid_sha256"]
                if live != want:
                    print(f"  FAIL: {fam} {kind}.jsonl mutated mid-run", file=sys.stderr)
                    return 2
                print(f"  ok: {fam} {kind}.jsonl {live[:16]}…")
        else:
            for kind in ("train", "valid"):
                live = sha256_file(SFT_ROOT / fam / f"{kind}.jsonl")
                want = rec[f"{kind}_sha256"]
                if live != want:
                    print(f"  FAIL: {fam} {kind}.jsonl mutated mid-run", file=sys.stderr)
                    return 2
                print(f"  ok: {fam} {kind}.jsonl {live[:16]}…")

    # Concatenate byte-verbatim
    train_bytes = _ensure_nl(train_pieces)
    valid_bytes = _ensure_nl(valid_pieces)
    train_out.write_bytes(train_bytes)
    valid_out.write_bytes(valid_bytes)
    train_sha = sha256_bytes(train_bytes)
    valid_sha = sha256_bytes(valid_bytes)
    train_rows_out = train_bytes.count(b"\n")
    valid_rows_out = valid_bytes.count(b"\n")

    per_family_train_counts = {}
    per_family_valid_counts = {}
    for rec in source_records:
        fam = rec["family"]
        if fam == V3_STRIPPED:
            per_family_train_counts[fam] = rec["assembled_train_rows"]
            per_family_valid_counts[fam] = rec["assembled_valid_rows"]
        else:
            per_family_train_counts[fam] = rec["train_rows"]
            per_family_valid_counts[fam] = rec["valid_rows"]

    manifest = {
        "sprint": "MDEMG-USAGE-LORA-001",
        "epic": "1b (6-family corpus assembly)",
        "family_name": "mdemg_usage_lora_001",
        "families_included": [r["family"] for r in source_records],
        "row_counts": {
            "train": train_rows_out,
            "valid": valid_rows_out,
            "total": train_rows_out + valid_rows_out,
        },
        "file_sha256": {"train.jsonl": train_sha, "valid.jsonl": valid_sha},
        "per_family_train_counts": per_family_train_counts,
        "per_family_valid_counts": per_family_valid_counts,
        "source_records": source_records,
        "base_model_name": "mlx-community/Qwen3-14B-4bit",
        "trained_against_model_sha": None,  # filled at Epic 3
        "generated_at_utc": datetime.now(timezone.utc).isoformat(),
    }
    manifest_out.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
    print(f"\nAssembled: train={train_rows_out} valid={valid_rows_out} total={train_rows_out+valid_rows_out}")
    print(f"train.jsonl SHA: {train_sha}")
    print(f"valid.jsonl SHA: {valid_sha}")
    print(f"Wrote: {train_out}\nWrote: {valid_out}\nWrote: {manifest_out}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
