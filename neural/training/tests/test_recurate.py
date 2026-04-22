#!/usr/bin/env python3
"""Tests for neural/training/recurate.py (Sprint FT-LORA-DATA Epic 1).

Covers the four gate requirements from the sprint plan:
  1. Round-trip   — synthetic 100-row fixture flows end-to-end
  2. Quality-filter boundary behavior (null + explicit value)
  3. SHA-drift rejection with informative message
  4. Provenance-field preservation on every output row
"""

from __future__ import annotations

import hashlib
import json
import tempfile
import unittest
from pathlib import Path

import sys
# Make ``training.*`` importable when pytest is invoked from the repo root,
# mirroring the pattern used by test_profile_expert_routing.py.
_NEURAL_ROOT = Path(__file__).resolve().parents[2]
if str(_NEURAL_ROOT) not in sys.path:
    sys.path.insert(0, str(_NEURAL_ROOT))

from training.recurate import (  # noqa: E402
    PROVENANCE_FIELDS,
    assert_raw_sha,
    build_meta,
    compute_sha256,
    passes_quality,
    passes_required,
    run_recurate,
)


def _row(**overrides):
    """Build a synthetic raw-schema row suitable for recurate input."""
    base = {
        "task_name": "retrieval.rerank_cross",
        "trace_id": "trace-0001",
        "instance_id": "test-host-mdemg-dev",
        "space_id": "mdemg-dev",
        "system_prompt": "You are a relevance judge.",
        "user_prompt": "Rank these candidates.",
        "response": "[0.9, 0.5, 0.1]",
        "think_mode": False,
        "think_content": None,
        "retrieval_node_ids": None,
        "retrieval_scores": None,
        "quality": None,
        "quality_source": None,
        "source_path": "tsdb://llm_interactions",
    }
    base.update(overrides)
    return base


def _write_jsonl(path: Path, rows):
    with open(path, "w") as f:
        for r in rows:
            f.write(json.dumps(r) + "\n")


class TestPassesQuality(unittest.TestCase):
    """Quality gate boundary: null vs numeric; floor = 0 means "no gate"."""

    def test_null_dropped_when_positive_floor(self):
        self.assertFalse(passes_quality({"quality": None}, 0.6))

    def test_null_passes_when_zero_floor(self):
        self.assertTrue(passes_quality({"quality": None}, 0.0))
        self.assertTrue(passes_quality({}, 0.0))

    def test_numeric_boundary_inclusive(self):
        self.assertTrue(passes_quality({"quality": 0.6}, 0.6))
        self.assertTrue(passes_quality({"quality": 0.95}, 0.6))
        self.assertFalse(passes_quality({"quality": 0.59}, 0.6))

    def test_non_numeric_quality_dropped(self):
        self.assertFalse(passes_quality({"quality": "n/a"}, 0.6))


class TestPassesRequired(unittest.TestCase):
    """task_name + trace_id are non-optional."""

    def test_both_present(self):
        self.assertTrue(passes_required({"task_name": "t", "trace_id": "x"}))

    def test_missing_task_name(self):
        self.assertFalse(passes_required({"trace_id": "x"}))

    def test_missing_trace_id(self):
        self.assertFalse(passes_required({"task_name": "t"}))

    def test_empty_values_drop(self):
        self.assertFalse(passes_required({"task_name": "", "trace_id": "x"}))
        self.assertFalse(passes_required({"task_name": "t", "trace_id": ""}))


class TestBuildMeta(unittest.TestCase):
    """Every provenance field must appear on every row's meta sidecar."""

    def test_all_fields_present(self):
        row = _row(quality=0.82, quality_source="heuristic")
        meta = build_meta(row, dataset_ver="v2026-04-22")
        self.assertEqual(meta["source"], "real")
        self.assertEqual(meta["dataset_ver"], "v2026-04-22")
        for f in PROVENANCE_FIELDS:
            self.assertIn(f, meta, f"meta missing {f}")
        self.assertEqual(meta["task_name"], "retrieval.rerank_cross")
        self.assertEqual(meta["quality"], 0.82)
        self.assertEqual(meta["quality_source"], "heuristic")

    def test_missing_raw_fields_become_none(self):
        row = {"task_name": "t", "trace_id": "x"}
        meta = build_meta(row, dataset_ver="v2026-04-22")
        self.assertIsNone(meta["instance_id"])
        self.assertIsNone(meta["quality"])


class TestSHAPin(unittest.TestCase):
    """SHA-drift rejection — mismatch must exit with informative message."""

    def test_matching_sha_passes(self):
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "raw.jsonl"
            p.write_bytes(b'{"hello": "world"}\n')
            observed = compute_sha256(p)
            # Must not raise:
            assert_raw_sha(p, observed)

    def test_mismatched_sha_raises_systemexit(self):
        with tempfile.TemporaryDirectory() as d:
            p = Path(d) / "raw.jsonl"
            p.write_bytes(b'{"hello": "world"}\n')
            bogus = hashlib.sha256(b"different bytes").hexdigest()
            with self.assertRaises(SystemExit) as cm:
                assert_raw_sha(p, bogus)
            # Exit code 2 per convention:
            self.assertEqual(cm.exception.code, 2)


class TestRunRecurateRoundTrip(unittest.TestCase):
    """End-to-end: synthetic 100-row fixture + per-task drop accounting."""

    def _mk_fixture(self, d: Path, *, null_quality_count=60, good_quality_count=30, bad_quality_count=10):
        rows = []
        idx = 0
        # null-quality rows (retained at min_quality=0.0, dropped at >0)
        for _ in range(null_quality_count):
            rows.append(_row(trace_id=f"t-null-{idx}", quality=None))
            idx += 1
        # explicitly good rows
        for _ in range(good_quality_count):
            rows.append(_row(trace_id=f"t-good-{idx}", quality=0.9, quality_source="heuristic"))
            idx += 1
        # explicitly bad rows
        for _ in range(bad_quality_count):
            rows.append(_row(trace_id=f"t-bad-{idx}", quality=0.3, quality_source="heuristic"))
            idx += 1
        # add a missing-required row + a task-name=None row (dropped on required)
        rows.append(_row(trace_id=""))
        rows.append(_row(task_name=""))
        p = d / "raw.jsonl"
        _write_jsonl(p, rows)
        return p

    def test_min_quality_zero_keeps_null(self):
        with tempfile.TemporaryDirectory() as d:
            dp = Path(d)
            raw = self._mk_fixture(dp)
            out = dp / "recurated.jsonl"
            report = run_recurate(raw, out, dataset_ver="v-test", min_quality=0.0)
            # null(60) + good(30) + bad(10) pass the quality gate when floor=0
            self.assertEqual(report["kept_rows"], 100)
            self.assertEqual(report["input_rows"], 102)
            self.assertEqual(sum(report["dropped_required_per_task"].values()), 2)
            self.assertFalse(report["dropped_quality_per_task"])

    def test_min_quality_0_6_drops_null_and_bad(self):
        with tempfile.TemporaryDirectory() as d:
            dp = Path(d)
            raw = self._mk_fixture(dp)
            out = dp / "recurated.jsonl"
            report = run_recurate(raw, out, dataset_ver="v-test", min_quality=0.6)
            # Only the 30 good rows survive
            self.assertEqual(report["kept_rows"], 30)
            # 60 null + 10 bad = 70 quality drops
            self.assertEqual(sum(report["dropped_quality_per_task"].values()), 70)

    def test_output_rows_preserve_provenance_and_schema(self):
        with tempfile.TemporaryDirectory() as d:
            dp = Path(d)
            raw = self._mk_fixture(dp)
            out = dp / "recurated.jsonl"
            run_recurate(raw, out, dataset_ver="v-test", min_quality=0.0)
            with open(out) as f:
                lines = [json.loads(ln) for ln in f if ln.strip()]
            self.assertEqual(len(lines), 100)
            for r in lines:
                self.assertIn("messages", r)
                self.assertIn("meta", r)
                # Last message must be assistant (format_converter invariant)
                self.assertEqual(r["messages"][-1]["role"], "assistant")
                # Provenance contract
                self.assertEqual(r["meta"]["source"], "real")
                self.assertEqual(r["meta"]["dataset_ver"], "v-test")
                self.assertIsNotNone(r["meta"]["task_name"])
                self.assertIsNotNone(r["meta"]["trace_id"])

    def test_output_never_exceeds_input(self):
        with tempfile.TemporaryDirectory() as d:
            dp = Path(d)
            raw = self._mk_fixture(dp, null_quality_count=5, good_quality_count=5, bad_quality_count=5)
            out = dp / "recurated.jsonl"
            report = run_recurate(raw, out, dataset_ver="v-test", min_quality=0.0)
            self.assertLessEqual(report["kept_rows"], report["input_rows"])


if __name__ == "__main__":
    unittest.main()
