#!/usr/bin/env python3
"""Tests for neural/training/balanced_sampler.py (Sprint FT-LORA-DATA Epic 3)."""

from __future__ import annotations

import sys
import tempfile
import unittest
from pathlib import Path

_NEURAL_ROOT = Path(__file__).resolve().parents[2]
if str(_NEURAL_ROOT) not in sys.path:
    sys.path.insert(0, str(_NEURAL_ROOT))

from training.balanced_sampler import (  # noqa: E402
    TIER_TASKS,
    DuplicationCeilingExceeded,
    balanced_sample,
    build_per_task_target,
    load_jsonl,
    sha256_of_rows,
    write_jsonl,
)


def _row(task: str, idx: int) -> dict:
    return {
        "messages": [
            {"role": "system", "content": "s"},
            {"role": "user", "content": f"u-{task}-{idx}"},
            {"role": "assistant", "content": f"a-{task}-{idx}"},
        ],
        "meta": {"task_name": task, "trace_id": f"{task}-{idx}"},
    }


class TestTierTaskLayout(unittest.TestCase):
    """Plan §5 Epic 3 arithmetic invariants — lock these in."""

    def test_tier1_has_16_tasks(self):
        self.assertEqual(len(TIER_TASKS["tier1"]), 16)

    def test_t_family_has_7_tasks_including_ape_reflect(self):
        self.assertEqual(len(TIER_TASKS["T"]), 7)
        self.assertIn("ape.reflect", TIER_TASKS["T"])

    def test_c_family_has_6_tasks(self):
        self.assertEqual(len(TIER_TASKS["C"]), 6)

    def test_j_family_has_3_tasks(self):
        self.assertEqual(len(TIER_TASKS["J"]), 3)

    def test_tier1_is_union_of_T_C_J(self):
        self.assertEqual(
            set(TIER_TASKS["tier1"]),
            set(TIER_TASKS["T"]) | set(TIER_TASKS["C"]) | set(TIER_TASKS["J"]),
        )

    def test_families_are_disjoint(self):
        self.assertFalse(set(TIER_TASKS["T"]) & set(TIER_TASKS["C"]))
        self.assertFalse(set(TIER_TASKS["T"]) & set(TIER_TASKS["J"]))
        self.assertFalse(set(TIER_TASKS["C"]) & set(TIER_TASKS["J"]))


class TestBuildPerTaskTarget(unittest.TestCase):
    def test_t_family_total_is_1700(self):
        """Plan §5 Epic 3 arithmetic: 6 × 200 + 500 (ape.reflect) = 1,700."""
        targets = build_per_task_target("T", per_task_floor=200,
                                        ape_reflect_target=500)
        self.assertEqual(sum(targets.values()), 1700)

    def test_c_family_total_is_1200(self):
        targets = build_per_task_target("C", per_task_floor=200,
                                        ape_reflect_target=500)
        self.assertEqual(sum(targets.values()), 1200)

    def test_j_family_total_is_600(self):
        targets = build_per_task_target("J", per_task_floor=200,
                                        ape_reflect_target=500)
        self.assertEqual(sum(targets.values()), 600)

    def test_tier1_total_is_3500(self):
        targets = build_per_task_target("tier1", per_task_floor=200,
                                        ape_reflect_target=500)
        self.assertEqual(sum(targets.values()), 3500)
        self.assertEqual(targets["ape.reflect"], 500)

    def test_unknown_tier_raises(self):
        with self.assertRaises(ValueError):
            build_per_task_target("X", per_task_floor=200,
                                  ape_reflect_target=500)


class TestBalancedSample(unittest.TestCase):
    def test_passthrough_exact_match(self):
        rows = [_row("consulting.classify", i) for i in range(200)]
        out, meta = balanced_sample(rows, {"consulting.classify": 200}, seed=42)
        self.assertEqual(len(out), 200)
        self.assertEqual(meta["duplication_factors"]["consulting.classify"], 1.0)

    def test_downsample(self):
        rows = [_row("consulting.classify", i) for i in range(500)]
        out, meta = balanced_sample(rows, {"consulting.classify": 200}, seed=42)
        self.assertEqual(len(out), 200)
        # Every sampled row preserved input shape
        for r in out:
            self.assertIn(r, rows)
        self.assertEqual(meta["duplication_factors"]["consulting.classify"], 1.0)

    def test_upsample_integer_duplication(self):
        rows = [_row("jiminy.evaluate_llm", i) for i in range(45)]
        out, meta = balanced_sample(rows, {"jiminy.evaluate_llm": 200}, seed=42)
        self.assertEqual(len(out), 200)
        # factor = 200/45 = 4.444
        self.assertAlmostEqual(meta["duplication_factors"]["jiminy.evaluate_llm"],
                               4.444, places=2)
        # Every unique input row appears at least once
        seen_ids = {r["meta"]["trace_id"] for r in out}
        self.assertEqual(len(seen_ids), 45)

    def test_duplication_ceiling_breach_raises(self):
        rows = [_row("jiminy.evaluate_llm", i) for i in range(30)]
        with self.assertRaises(DuplicationCeilingExceeded) as cm:
            balanced_sample(rows, {"jiminy.evaluate_llm": 200}, seed=42,
                            duplication_ceiling=5)
        msg = str(cm.exception)
        self.assertIn("jiminy.evaluate_llm", msg)
        self.assertIn("6.67", msg)  # 200/30

    def test_seeded_determinism(self):
        rows = [_row("consulting.classify", i) for i in range(500)]
        out1, _ = balanced_sample(rows, {"consulting.classify": 200}, seed=42)
        out2, _ = balanced_sample(rows, {"consulting.classify": 200}, seed=42)
        self.assertEqual(sha256_of_rows(out1), sha256_of_rows(out2))
        # Different seed → different output (usually)
        out3, _ = balanced_sample(rows, {"consulting.classify": 200}, seed=99)
        self.assertNotEqual(sha256_of_rows(out1), sha256_of_rows(out3))

    def test_mixed_upsample_downsample_passthrough(self):
        rows = (
            [_row("ape.reflect", i) for i in range(1000)]      # downsample
            + [_row("consulting.classify", i) for i in range(200)]  # exact
            + [_row("jiminy.evaluate_llm", i) for i in range(45)]   # upsample
        )
        targets = {"ape.reflect": 500, "consulting.classify": 200,
                   "jiminy.evaluate_llm": 200}
        out, meta = balanced_sample(rows, targets, seed=42)
        counts = {t: meta["output_counts"].get(t, 0) for t in targets}
        self.assertEqual(counts["ape.reflect"], 500)
        self.assertEqual(counts["consulting.classify"], 200)
        self.assertEqual(counts["jiminy.evaluate_llm"], 200)
        self.assertEqual(len(out), 900)

    def test_task_not_in_target_is_discarded(self):
        rows = [_row("noise.task", i) for i in range(50)]
        out, meta = balanced_sample(rows, {"ape.reflect": 500}, seed=42)
        self.assertEqual(len(out), 0)
        # noise.task was not in target so not counted in input_counts
        self.assertNotIn("noise.task", meta["input_counts"])

    def test_zero_rows_for_target_leaves_gap(self):
        # Absent task (0 rows) — synthesizer should have filled but if not,
        # sampler leaves a 0 entry rather than raising.
        rows = []
        out, meta = balanced_sample(rows, {"consulting.synthesis": 200}, seed=42)
        self.assertEqual(len(out), 0)
        self.assertEqual(meta["duplication_factors"]["consulting.synthesis"], 0.0)

    def test_metadata_shape(self):
        rows = [_row("consulting.classify", i) for i in range(100)]
        _, meta = balanced_sample(rows, {"consulting.classify": 200}, seed=42)
        for k in ("input_counts", "output_counts", "duplication_factors",
                  "seed", "duplication_ceiling", "per_task_target"):
            self.assertIn(k, meta)


class TestFullTierComposition(unittest.TestCase):
    """End-to-end per-tier arithmetic — the numbers that Phase 5 will train on."""

    def _synthetic_corpus(self, per_task_count: dict) -> list[dict]:
        rows = []
        for task, n in per_task_count.items():
            for i in range(n):
                rows.append(_row(task, i))
        return rows

    def test_t_family_end_to_end(self):
        # Simulate real distribution + synth fills for absent tasks
        per_task = {
            "ape.reflect": 34093,           # overabundant → downsample to 500
            "hidden.summarize": 300,        # passthrough-ish → downsample to 200
            "jiminy.synthesize": 250,
            "consulting.synthesis": 200,    # synth-filled
            "metalearn.generalize": 200,    # synth-filled
            "retrieval.rerank_nli": 200,    # synth-filled
            "summarize.generate": 200,      # synth-filled
        }
        rows = self._synthetic_corpus(per_task)
        targets = build_per_task_target("T", per_task_floor=200,
                                        ape_reflect_target=500)
        out, meta = balanced_sample(rows, targets, seed=42)
        # 6×200 + 500 = 1,700 (plan §5 Refinement 5)
        self.assertEqual(len(out), 1700)
        self.assertEqual(meta["output_counts"]["ape.reflect"], 500)

    def test_c_family_end_to_end(self):
        per_task = {t: 300 for t in TIER_TASKS["C"]}
        rows = self._synthetic_corpus(per_task)
        targets = build_per_task_target("C", per_task_floor=200,
                                        ape_reflect_target=500)
        out, _ = balanced_sample(rows, targets, seed=42)
        self.assertEqual(len(out), 1200)

    def test_j_family_end_to_end(self):
        per_task = {"hidden.name_emergence": 200,
                    "jiminy.evaluate_llm": 45,
                    "retrieval.rerank_cross": 400}
        rows = self._synthetic_corpus(per_task)
        targets = build_per_task_target("J", per_task_floor=200,
                                        ape_reflect_target=500)
        out, meta = balanced_sample(rows, targets, seed=42)
        self.assertEqual(len(out), 600)
        self.assertEqual(meta["output_counts"]["jiminy.evaluate_llm"], 200)
        # 4.44× duplication — at ceiling but below 5×
        self.assertLess(meta["duplication_factors"]["jiminy.evaluate_llm"], 5.0)


class TestIO(unittest.TestCase):
    def test_load_and_write_roundtrip(self):
        with tempfile.TemporaryDirectory() as d:
            dp = Path(d)
            rows = [_row("consulting.classify", i) for i in range(5)]
            p = dp / "x.jsonl"
            write_jsonl(p, rows)
            loaded = load_jsonl(p)
            self.assertEqual(len(loaded), 5)
            self.assertEqual(loaded[0]["meta"]["trace_id"], "consulting.classify-0")

    def test_sha256_changes_on_row_change(self):
        rows = [_row("consulting.classify", i) for i in range(5)]
        sha1 = sha256_of_rows(rows)
        rows[0]["meta"]["trace_id"] = "changed"
        sha2 = sha256_of_rows(rows)
        self.assertNotEqual(sha1, sha2)


if __name__ == "__main__":
    unittest.main()
