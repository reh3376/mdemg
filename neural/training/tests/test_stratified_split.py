#!/usr/bin/env python3
"""Tests for neural/training/stratified_split.py (Sprint FT-LORA-DATA Epic 4).

Gate requirements (plan §5 Epic 4):
  * 90/10 partition per task
  * min 2 valid rows when count ≥ 20
  * manifest schema + file SHA stamping
  * Epic 4 Step 3a mlx_lm fixture check is a shell-level test, not in this file.
"""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

_NEURAL_ROOT = Path(__file__).resolve().parents[2]
if str(_NEURAL_ROOT) not in sys.path:
    sys.path.insert(0, str(_NEURAL_ROOT))

from training.stratified_split import (  # noqa: E402
    SPRINT_C_MODEL_CONFIG_SHA,
    build_manifest,
    build_source_composition,
    count_weak_signal_rows,
    file_sha256,
    split_task,
    stratified_split,
    strip_meta_sidecar,
    write_jsonl,
)
from training.recurate import SPRINT_PLAN_RAW_SHA_PIN  # noqa: E402


def _row(task: str, idx: int, *, source: str = "real",
         weak_signal: bool = False) -> dict:
    return {
        "messages": [
            {"role": "system", "content": "s"},
            {"role": "user", "content": f"u-{idx}"},
            {"role": "assistant", "content": f"a-{idx}"},
        ],
        "meta": {
            "task_name": task, "trace_id": f"{task}-{idx}",
            "source": source, "weak_signal": weak_signal,
        },
    }


class TestSplitTask(unittest.TestCase):
    def test_abundant_task_90_10_with_valid_floor(self):
        rows = [_row("t", i) for i in range(200)]
        tr, va = split_task(rows, seed=42)
        # 90/10 of 200 = 180/20
        self.assertEqual(len(tr), 180)
        self.assertEqual(len(va), 20)

    def test_500_rows(self):
        rows = [_row("t", i) for i in range(500)]
        tr, va = split_task(rows, seed=42)
        self.assertEqual(len(tr), 450)
        self.assertEqual(len(va), 50)

    def test_valid_floor_of_two_kicks_in_at_20(self):
        rows = [_row("t", i) for i in range(20)]
        tr, va = split_task(rows, seed=42)
        # 10% of 20 = 2, which matches the floor
        self.assertEqual(len(va), 2)
        self.assertEqual(len(tr), 18)

    def test_valid_floor_lifts_tiny_task(self):
        # 10% of 10 would round to 1 but floor is 2 — count<20 branch: max(1, ...)
        rows = [_row("t", i) for i in range(10)]
        tr, va = split_task(rows, seed=42)
        # count < 20 → valid = max(1, round(10*0.1)) = max(1, 1) = 1
        self.assertGreaterEqual(len(va), 1)
        self.assertEqual(len(tr) + len(va), 10)

    def test_single_row_no_valid(self):
        rows = [_row("t", 0)]
        tr, va = split_task(rows, seed=42)
        self.assertEqual(len(tr), 1)
        self.assertEqual(len(va), 0)

    def test_empty(self):
        tr, va = split_task([], seed=42)
        self.assertEqual((tr, va), ([], []))

    def test_shuffle_is_deterministic_per_seed(self):
        rows = [_row("t", i) for i in range(100)]
        tr1, va1 = split_task(rows, seed=42)
        tr2, va2 = split_task(rows, seed=42)
        self.assertEqual(tr1, tr2)
        self.assertEqual(va1, va2)


class TestStratifiedSplit(unittest.TestCase):
    def test_each_task_independent(self):
        rows = (
            [_row("a", i) for i in range(200)]
            + [_row("b", i) for i in range(50)]
        )
        tr, va, per_task = stratified_split(rows, seed=42)
        # 200 → 180/20, 50 → 45/5
        self.assertEqual(per_task["a"]["train"], 180)
        self.assertEqual(per_task["a"]["valid"], 20)
        self.assertEqual(per_task["b"]["train"], 45)
        self.assertEqual(per_task["b"]["valid"], 5)
        self.assertEqual(len(tr), 225)
        self.assertEqual(len(va), 25)

    def test_min_two_valid_when_count_is_20(self):
        rows = [_row("tiny", i) for i in range(20)]
        _, va, per_task = stratified_split(rows, seed=42)
        self.assertGreaterEqual(per_task["tiny"]["valid"], 2)

    def test_changing_one_task_does_not_reshuffle_others(self):
        # Per-task sub-seed means adding a task doesn't churn other tasks' splits.
        rows_a_only = [_row("a", i) for i in range(50)]
        rows_ab = rows_a_only + [_row("b", i) for i in range(30)]
        _, _, pt1 = stratified_split(rows_a_only, seed=42)
        _, _, pt2 = stratified_split(rows_ab, seed=42)
        # Task 'a' counts should be identical in both
        self.assertEqual(pt1["a"], pt2["a"])

    def test_unknown_task_discarded(self):
        rows = [{"messages": [], "meta": {}}]  # no task_name
        _, _, pt = stratified_split(rows, seed=42)
        self.assertEqual(pt, {})


class TestSourceComposition(unittest.TestCase):
    def test_tallies_by_source(self):
        rows = (
            [_row("t", i, source="real") for i in range(10)]
            + [_row("t", i, source="synth-gpt-5.4-mini") for i in range(5)]
            + [_row("t", i, source="synth-qwen3.6-local") for i in range(3)]
        )
        tally = build_source_composition(rows)
        self.assertEqual(tally["real"], 10)
        self.assertEqual(tally["synth-gpt-5.4-mini"], 5)
        self.assertEqual(tally["synth-qwen3.6-local"], 3)

    def test_missing_source_labeled_unknown(self):
        rows = [{"meta": {}}]
        self.assertEqual(build_source_composition(rows), {"unknown": 1})


class TestWeakSignalCount(unittest.TestCase):
    def test_counts_true_only(self):
        rows = (
            [_row("t", i, weak_signal=True) for i in range(5)]
            + [_row("t", i, weak_signal=False) for i in range(3)]
        )
        self.assertEqual(count_weak_signal_rows(rows), 5)


class TestMetaSidecar(unittest.TestCase):
    def test_strip_preserves_order_and_content(self):
        rows = [_row("t", i) for i in range(5)]
        stripped, metas = strip_meta_sidecar(rows)
        self.assertEqual(len(stripped), 5)
        self.assertEqual(len(metas), 5)
        for s in stripped:
            self.assertNotIn("meta", s)
            self.assertIn("messages", s)
        for i, m in enumerate(metas):
            self.assertEqual(m["trace_id"], f"t-{i}")


class TestManifest(unittest.TestCase):
    def test_manifest_shape_and_pins(self):
        with tempfile.TemporaryDirectory() as d:
            dp = Path(d)
            train = [_row("t", i) for i in range(180)]
            valid = [_row("t", i, idx_offset=0) if False else _row("t", 1000 + i) for i in range(20)]
            write_jsonl(dp / "train.jsonl", train)
            write_jsonl(dp / "valid.jsonl", valid)
            per_task = {"t": {"train": 180, "valid": 20, "total": 200}}
            m = build_manifest(
                tier="tier1", family_name="tier1", out_dir=dp,
                train_rows=train, valid_rows=valid, per_task=per_task,
                seed=42, base_dataset_ver="v2026-04-22",
                duplication_factors={"t": 1.0},
                generator_sha="abc123",
                ults_versions={"t": "1.0.0-task-1.0.0"},
                synthesis_version="v1-deadbeef",
                ape_reflect_target=500,
                meta_placement="embedded",
            )
            # Mandatory keys
            for k in (
                "sprint", "generator_sha", "tier", "family_name",
                "row_counts", "per_task_counts", "source_composition",
                "weak_signal_rows", "ape_reflect_target",
                "duplication_factors", "seed", "file_sha256",
                "base_dataset_ver", "trained_against_model_sha",
                "raw_dataset_sha_pin", "ults_spec_versions",
                "synthesis_version", "meta_placement",
                "synthesis_non_determinism_note",
            ):
                self.assertIn(k, m, f"manifest missing {k}")
            self.assertEqual(m["sprint"], "FT-LORA-DATA")
            self.assertEqual(m["row_counts"]["total"], 200)
            self.assertEqual(m["trained_against_model_sha"], SPRINT_C_MODEL_CONFIG_SHA)
            self.assertEqual(m["raw_dataset_sha_pin"], SPRINT_PLAN_RAW_SHA_PIN)
            # SHA of the emitted train.jsonl matches on-disk
            self.assertEqual(
                m["file_sha256"]["train.jsonl"], file_sha256(dp / "train.jsonl"),
            )


class TestCLIIntegration(unittest.TestCase):
    """End-to-end: balanced → stratified_split via CLI invocation.

    Uses a synthetic tier1-like corpus but with only 3 tasks enrolled to
    keep the test fast. The CLI path covers argparse + manifest writing.
    """

    def test_cli_produces_train_valid_manifest(self):
        import subprocess

        with tempfile.TemporaryDirectory() as d:
            dp = Path(d)
            # Build a balanced-looking corpus: only tier1 tasks count.
            # Use 3 real tier1 tasks so stratified_split buckets them.
            from training.balanced_sampler import TIER_TASKS
            tasks = TIER_TASKS["tier1"][:3]
            rows = []
            for t in tasks:
                rows.extend(_row(t, i) for i in range(200))

            in_path = dp / "balanced.jsonl"
            with open(in_path, "w") as f:
                for r in rows:
                    f.write(json.dumps(r) + "\n")

            out_dir = dp / "out"
            cmd = [
                sys.executable, "-m", "training.stratified_split",
                "--input", str(in_path),
                "--out-dir", str(out_dir),
                "--tier", "tier1",
                "--family-name", "tier1",
                "--base-dataset-ver", "v2026-04-22",
                "--seed", "42",
            ]
            env = {"PYTHONPATH": str(_NEURAL_ROOT)}
            r = subprocess.run(
                cmd, capture_output=True, text=True, env=env, check=False,
            )
            self.assertEqual(r.returncode, 0, msg=r.stderr)
            self.assertTrue((out_dir / "train.jsonl").exists())
            self.assertTrue((out_dir / "valid.jsonl").exists())
            self.assertTrue((out_dir / "manifest.json").exists())
            manifest = json.loads((out_dir / "manifest.json").read_text())
            self.assertEqual(manifest["row_counts"]["total"], 600)
            # 90/10 per task × 3 tasks: 540 train / 60 valid
            self.assertEqual(manifest["row_counts"]["train"], 540)
            self.assertEqual(manifest["row_counts"]["valid"], 60)
            self.assertEqual(manifest["tier"], "tier1")


if __name__ == "__main__":
    unittest.main()
