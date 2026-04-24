"""DPO pair generator — Phase 11 Epic 4.

Reads ``benchmark_results`` rows and emits win/loss JSONL pairs for Phase 12
HITL DPO curation. Training is out of scope (deferred to Phase 12).

Bucketing:
    (task_id, prompt_hash) — pairs are only formed within the same bucket
    so that the "prompt" side is stable. Response_text is the only thing
    that differs between chosen/rejected.

Selection rule:
    For every ordered pair (a, b) in a bucket with
        scalar(reward_a) - scalar(reward_b) >= threshold
    emit a pair with a=chosen, b=rejected. Cap ``max_pairs_per_bucket`` to
    avoid flooding DPO with near-duplicates.

Prompt hashing (configurable — defaults to blake2b-16):
    blake2b_16        — 128-bit digest, default
    sha256_full       — 256-bit digest, auditable
    task_first_tokens — first N tokens of response_text (weakest; placeholder
                        — real prompt text is not persisted on benchmark_results
                        and has to be reconstructed from the ULTS spec, a
                        Phase 12 scope item)

Output:
    pairs.jsonl    — one JSON object per pair
    manifest.json  — totals, per-task coverage, reward delta histogram, SHAs
"""
from __future__ import annotations

import hashlib
import json
import logging
import os
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Iterable, Literal

try:
    import psycopg
except ImportError:  # pragma: no cover
    psycopg = None  # type: ignore[assignment]

logger = logging.getLogger(__name__)


PromptHashStrategy = Literal["blake2b_16", "sha256_full", "task_first_tokens"]


# ── config dataclass (built from configs/dpo_phase12_pairs.yaml) ────────────


@dataclass
class PairGeneratorConfig:
    reward_delta_threshold: float = 0.15
    min_pairs_per_task: int = 20
    prompt_hash_strategy: PromptHashStrategy = "blake2b_16"
    task_first_tokens_n: int = 64
    max_pairs_per_bucket: int = 4
    reward_aggregation: Literal["mean", "weighted"] = "mean"
    reward_weights: dict[str, dict[str, float]] = field(default_factory=dict)


@dataclass
class DpoPair:
    """One emitted pair — serialized 1:1 to the JSONL line."""

    pair_id: str
    task_id: str
    sampling_group: str
    prompt_hash: str
    chosen: str                # response_text of the winner
    rejected: str              # response_text of the loser
    chosen_reward: float
    rejected_reward: float
    reward_delta: float
    chosen_run_idx: int
    rejected_run_idx: int

    def to_dict(self) -> dict[str, Any]:
        return {
            "pair_id": self.pair_id,
            "task_id": self.task_id,
            "sampling_group": self.sampling_group,
            "prompt_hash": self.prompt_hash,
            "chosen": self.chosen,
            "rejected": self.rejected,
            "chosen_reward": self.chosen_reward,
            "rejected_reward": self.rejected_reward,
            "reward_delta": self.reward_delta,
            "chosen_run_idx": self.chosen_run_idx,
            "rejected_run_idx": self.rejected_run_idx,
        }


# ── reward scalarization (local copy to keep DPO independent of rl module) ──


def _scalarize_mean(rv: dict[str, Any]) -> float:
    if not rv:
        return 0.0
    nums = [
        float(v) for v in rv.values()
        if isinstance(v, (int, float)) and not isinstance(v, bool)
    ]
    return sum(nums) / len(nums) if nums else 0.0


def _scalarize_weighted(rv: dict[str, Any], weights: dict[str, float]) -> float:
    """Weighted reward: Σ(w_k × v_k) for keys in weights; fall back to mean."""
    if not rv or not weights:
        return _scalarize_mean(rv)
    total_w = 0.0
    total_v = 0.0
    for k, v in rv.items():
        if not isinstance(v, (int, float)) or isinstance(v, bool):
            continue
        w = weights.get(k, 0.0)
        if w == 0.0:
            continue
        total_w += w
        total_v += w * float(v)
    return (total_v / total_w) if total_w > 0 else _scalarize_mean(rv)


def _scalarize(rv: dict[str, Any], task_id: str, cfg: PairGeneratorConfig) -> float:
    if cfg.reward_aggregation == "weighted" and task_id in cfg.reward_weights:
        return _scalarize_weighted(rv, cfg.reward_weights[task_id])
    return _scalarize_mean(rv)


# ── prompt hashing ──────────────────────────────────────────────────────────


def _prompt_hash(row: dict[str, Any], cfg: PairGeneratorConfig) -> str:
    """Bucketing key for a benchmark row.

    MVP caveat: benchmark_results does NOT carry the rendered prompt text —
    only response_text + task_id. So we hash on (task_id, response_text prefix)
    OR rely on task_id alone for a weak but deterministic bucket. Phase 12's
    richer prompt capture lives in ULTS spec rehydration (not this sprint).

    In practice: for the deterministic T/C/J tasks, task_id is a tight enough
    bucket (same spec → same rendered prompt), so blake2b(task_id) is the
    default. For the sampling-variant tasks, Phase 12 will rehydrate from
    ULTS + overwrite this hash.
    """
    if cfg.prompt_hash_strategy == "task_first_tokens":
        source = (row.get("response_text") or "")[: cfg.task_first_tokens_n]
        source = f"{row.get('task_id', '')}|{source}"
        return hashlib.blake2b(source.encode("utf-8"), digest_size=16).hexdigest()
    if cfg.prompt_hash_strategy == "sha256_full":
        return hashlib.sha256(str(row.get("task_id", "")).encode("utf-8")).hexdigest()
    # Default: blake2b_16 of the task_id (MVP — same-task = same bucket).
    return hashlib.blake2b(
        str(row.get("task_id", "")).encode("utf-8"),
        digest_size=16,
    ).hexdigest()


def _pair_id(task_id: str, chosen_idx: int, rejected_idx: int, prompt_hash: str) -> str:
    key = f"{task_id}|{chosen_idx}|{rejected_idx}|{prompt_hash}".encode()
    return hashlib.blake2b(key, digest_size=10).hexdigest()


# ── core pair selection ─────────────────────────────────────────────────────


def generate_pairs(rows: Iterable[dict[str, Any]],
                   cfg: PairGeneratorConfig) -> list[DpoPair]:
    """Bucket rows by (task_id, prompt_hash), emit pairs.

    Ordering: highest reward → lowest. Within a bucket, we iterate winner
    candidates in descending reward order and match each against lower-reward
    responses, bounded by ``max_pairs_per_bucket``.
    """
    rows = list(rows)
    # Build buckets.
    buckets: dict[tuple[str, str], list[dict[str, Any]]] = {}
    for row in rows:
        phash = _prompt_hash(row, cfg)
        row = {**row, "_prompt_hash": phash,
               "_scalar": _scalarize(row.get("reward_vector") or {}, row["task_id"], cfg)}
        buckets.setdefault((row["task_id"], phash), []).append(row)

    pairs: list[DpoPair] = []
    for (task_id, phash), bucket in buckets.items():
        if len(bucket) < 2:
            continue
        bucket.sort(key=lambda r: r["_scalar"], reverse=True)
        emitted = 0
        for i, chosen_row in enumerate(bucket):
            if emitted >= cfg.max_pairs_per_bucket:
                break
            for rejected_row in bucket[i + 1:]:
                if emitted >= cfg.max_pairs_per_bucket:
                    break
                delta = chosen_row["_scalar"] - rejected_row["_scalar"]
                if delta < cfg.reward_delta_threshold:
                    # bucket is desc → as rejected scalar decreases, delta
                    # grows; a below-threshold neighbor may be followed by a
                    # qualifying one, so continue rather than break.
                    continue
                pairs.append(DpoPair(
                    pair_id=_pair_id(
                        task_id,
                        int(chosen_row["run_idx"]),
                        int(rejected_row["run_idx"]),
                        phash,
                    ),
                    task_id=task_id,
                    sampling_group=chosen_row.get("sampling_group", "?"),
                    prompt_hash=phash,
                    chosen=chosen_row.get("response_text") or "",
                    rejected=rejected_row.get("response_text") or "",
                    chosen_reward=chosen_row["_scalar"],
                    rejected_reward=rejected_row["_scalar"],
                    reward_delta=delta,
                    chosen_run_idx=int(chosen_row["run_idx"]),
                    rejected_run_idx=int(rejected_row["run_idx"]),
                ))
                emitted += 1
    return pairs


# ── coverage + manifest ─────────────────────────────────────────────────────


def build_manifest(pairs: list[DpoPair], cfg: PairGeneratorConfig,
                   source_run_ids: list[str]) -> dict[str, Any]:
    """Produce the per-run manifest: totals, coverage, histogram, SHAs."""
    per_task: dict[str, int] = {}
    deltas: list[float] = []
    for p in pairs:
        per_task[p.task_id] = per_task.get(p.task_id, 0) + 1
        deltas.append(p.reward_delta)

    # Simple histogram: 10 bins over [threshold, max].
    if deltas:
        lo = cfg.reward_delta_threshold
        hi = max(deltas)
        width = (hi - lo) / 10 if hi > lo else 1.0
        bins = [0] * 10
        for d in deltas:
            idx = min(9, int((d - lo) / width)) if width > 0 else 0
            bins[idx] += 1
        histogram = {
            "lo": lo, "hi": hi,
            "counts": bins,
        }
    else:
        histogram = {"lo": cfg.reward_delta_threshold, "hi": 0.0, "counts": [0] * 10}

    under_floor = {
        task: count for task, count in per_task.items()
        if count < cfg.min_pairs_per_task
    }

    return {
        "source_run_ids": source_run_ids,
        "total_pairs": len(pairs),
        "per_task_coverage": dict(sorted(per_task.items())),
        "tasks_under_floor": dict(sorted(under_floor.items())),
        "reward_delta_histogram": histogram,
        "config": {
            "reward_delta_threshold": cfg.reward_delta_threshold,
            "min_pairs_per_task": cfg.min_pairs_per_task,
            "max_pairs_per_bucket": cfg.max_pairs_per_bucket,
            "prompt_hash_strategy": cfg.prompt_hash_strategy,
            "reward_aggregation": cfg.reward_aggregation,
        },
    }


def write_outputs(pairs: list[DpoPair], manifest: dict[str, Any],
                   pairs_path: Path, manifest_path: Path) -> None:
    pairs_path.parent.mkdir(parents=True, exist_ok=True)
    manifest_path.parent.mkdir(parents=True, exist_ok=True)
    with pairs_path.open("w", encoding="utf-8") as f:
        for p in pairs:
            f.write(json.dumps(p.to_dict(), sort_keys=True) + "\n")
    # Include the pairs-file SHA in the manifest for auditability.
    manifest = dict(manifest)
    manifest["pairs_jsonl_sha256"] = _file_sha(pairs_path)
    with manifest_path.open("w", encoding="utf-8") as f:
        json.dump(manifest, f, indent=2, sort_keys=True)


def _file_sha(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(8192), b""):
            h.update(chunk)
    return h.hexdigest()


# ── TSDB source loader (matches reward_sampler.from_tsdb) ───────────────────


def _tsdb_dsn(cfg: dict[str, Any]) -> str:
    host = os.environ.get("TSDB_HOST", cfg.get("host", "localhost"))
    port = os.environ.get("TSDB_PORT", str(cfg.get("port", 5433)))
    user = os.environ.get("TSDB_USER", cfg.get("user", "mdemg"))
    pwd = os.environ.get("TSDB_PASSWORD", cfg.get("password", "mdemg_metrics"))
    db = os.environ.get("TSDB_DATABASE", cfg.get("database", "mdemg_metrics"))
    return f"host={host} port={port} user={user} password={pwd} dbname={db}"


def load_rows_from_tsdb(tsdb_cfg: dict[str, Any],
                         run_ids: list[str] | None) -> tuple[list[dict[str, Any]], list[str]]:
    """Pull benchmark_results rows for the given run_ids (or all completed)."""
    if psycopg is None:
        raise RuntimeError("psycopg not installed; use pip install 'psycopg[binary]'")
    with psycopg.connect(_tsdb_dsn(tsdb_cfg), connect_timeout=5) as conn:
        with conn.cursor() as cur:
            if not run_ids:
                cur.execute(
                    "SELECT run_id FROM benchmark_runs "
                    "WHERE completed_at IS NOT NULL "
                    "ORDER BY started_at DESC"
                )
                run_ids = [r[0] for r in cur.fetchall()]
                if not run_ids:
                    raise RuntimeError(
                        "no completed benchmark_runs; run Phase 10 first"
                    )
            placeholders = ",".join(["%s"] * len(run_ids))
            cur.execute(
                f"SELECT task_id, run_idx, sampling_group, response_text, "
                f"reward_vector "
                f"FROM benchmark_results WHERE run_id IN ({placeholders})",
                run_ids,
            )
            rows = [
                {
                    "task_id": r[0],
                    "run_idx": r[1],
                    "sampling_group": r[2],
                    "response_text": r[3],
                    "reward_vector": r[4] or {},
                }
                for r in cur.fetchall()
            ]
    return rows, run_ids


# ── CLI ─────────────────────────────────────────────────────────────────────


def main() -> int:
    import argparse
    import yaml  # type: ignore[import-not-found]

    p = argparse.ArgumentParser(description="DPO pair generator (Phase 11)")
    p.add_argument("--config", required=True, help="configs/dpo_phase12_pairs.yaml")
    p.add_argument("--source", default="",
                   help="Comma-separated run_ids; empty = all completed")
    p.add_argument("--out", default=None, help="override pairs_jsonl path")
    p.add_argument("--manifest", default=None, help="override manifest path")
    args = p.parse_args()

    with open(args.config) as f:
        yaml_cfg = yaml.safe_load(f)

    sel = yaml_cfg["selection"]
    cfg = PairGeneratorConfig(
        reward_delta_threshold=sel["reward_delta_threshold"],
        min_pairs_per_task=sel["min_pairs_per_task"],
        prompt_hash_strategy=sel.get("prompt_hash_strategy", "blake2b_16"),
        task_first_tokens_n=sel.get("task_first_tokens_n", 64),
        max_pairs_per_bucket=sel.get("max_pairs_per_bucket", 4),
        reward_aggregation=sel.get("reward_aggregation", "mean"),
        reward_weights=sel.get("reward_weights") or {},
    )

    run_ids = [s for s in args.source.split(",") if s.strip()] or None
    rows, resolved_runs = load_rows_from_tsdb(yaml_cfg["tsdb"], run_ids)
    logger.info("Loaded %d rows across %d runs", len(rows), len(resolved_runs))

    pairs = generate_pairs(rows, cfg)
    manifest = build_manifest(pairs, cfg, resolved_runs)

    out_pairs = Path(args.out or yaml_cfg["output"]["pairs_jsonl"])
    out_manifest = Path(args.manifest or yaml_cfg["output"]["manifest_json"])
    write_outputs(pairs, manifest, out_pairs, out_manifest)
    print(f"Wrote {len(pairs)} pairs → {out_pairs}")
    print(f"Manifest: {out_manifest}")
    under = manifest["tasks_under_floor"]
    if under:
        print(f"WARNING: {len(under)} tasks under min_pairs floor "
              f"({cfg.min_pairs_per_task}): {sorted(under)}")
    return 0


if __name__ == "__main__":  # pragma: no cover
    raise SystemExit(main())
