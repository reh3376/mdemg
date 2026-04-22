#!/usr/bin/env python3
"""Integration test for the FT-LORA-DATA pipeline (Sprint FT-LORA-DATA Epic 5, Tier 2).

End-to-end on a synthetic 16-task corpus:
    raw (synthetic) → recurate → (mock synthesis) → balance → split

Asserts:
  * All 4 output dirs present (tier1 + 3 families)
  * Per-task counts match per-tier targets
  * Manifest schemas valid and SHA stamps match on-disk files
  * ape.reflect retained at 500 in tier1 + T family
  * Provenance (meta sidecar) preserved on every row
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

from training.balanced_sampler import (  # noqa: E402
    TIER_TASKS, balanced_sample, build_per_task_target, load_jsonl,
)
from training.recurate import run_recurate  # noqa: E402
from training.stratified_split import (  # noqa: E402
    build_manifest, file_sha256, git_sha, load_ults_versions,
    stratified_split, write_jsonl as write_jsonl_strat,
)


def _raw_row(task: str, idx: int, quality: float = 0.8) -> dict:
    """Raw JSONL row shape expected by recurate.py."""
    return {
        "task_name": task,
        "trace_id": f"{task}-{idx}",
        "instance_id": "synth-host",
        "space_id": "mdemg-dev",
        "system_prompt": "You are helpful.",
        "user_prompt": f"input-{task}-{idx}",
        "response": f"reply-{task}-{idx}",
        "think_mode": False,
        "think_content": None,
        "retrieval_node_ids": None,
        "retrieval_scores": None,
        "quality": quality,
        "quality_source": "heuristic",
        "source_path": "synthetic://integration",
    }


def _synth_row(task: str, idx: int, *, weak: bool = False) -> dict:
    """Epic 2 synthesis output row shape."""
    return {
        "messages": [
            {"role": "system", "content": "You are helpful."},
            {"role": "user", "content": f"synth-input-{idx}"},
            {"role": "assistant", "content": f"synth-reply-{idx}"},
        ],
        "meta": {
            "task_name": task,
            "trace_id": f"synth-{task}-{idx}",
            "instance_id": "synth-gpt-5.4-mini",
            "space_id": "synth",
            "source": "synth-gpt-5.4-mini",
            "dataset_ver": "v-int-test",
            "quality": 0.9,
            "quality_source": "teacher_distillation",
            "source_path": f"distill_driver:{task}",
            "synthesis_version": "v1-deadbeef",
            "weak_signal": weak,
        },
    }


ABSENT_TASKS = (
    "consulting.synthesis", "metalearn.generalize",
    "retrieval.rerank_nli", "summarize.generate",
)


class TestEndToEndPipeline(unittest.TestCase):
    def _build_raw_corpus(self, path: Path) -> int:
        """Write raw JSONL: every tier1 task gets some rows (absent ones get 0)."""
        rows = 0
        with open(path, "w") as f:
            for task in TIER_TASKS["tier1"]:
                if task in ABSENT_TASKS:
                    continue
                # ape.reflect overabundant, jiminy.evaluate_llm scarce, others ~300
                if task == "ape.reflect":
                    n = 1000
                elif task == "jiminy.evaluate_llm":
                    n = 45   # would upsample at 4.44×
                else:
                    n = 300
                for i in range(n):
                    f.write(json.dumps(_raw_row(task, i)) + "\n")
                    rows += 1
        return rows

    def _build_synth_dir(self, d: Path) -> dict[str, int]:
        """Write Epic 2 mock outputs: 200 rows per absent task."""
        d.mkdir(parents=True, exist_ok=True)
        counts = {}
        for task in ABSENT_TASKS:
            weak = (task == "metalearn.generalize")
            fname = task.replace(".", "_") + ".jsonl"
            rows = [_synth_row(task, i, weak=weak) for i in range(200)]
            with open(d / fname, "w") as f:
                for r in rows:
                    f.write(json.dumps(r) + "\n")
            counts[task] = 200
        return counts

    def test_full_pipeline_produces_all_four_output_dirs(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmpdir = Path(tmp)
            raw_path = tmpdir / "raw.jsonl"
            raw_count = self._build_raw_corpus(raw_path)
            self.assertGreater(raw_count, 0)

            # --- Stage 1: recurate ---
            recurated = tmpdir / "recurated.jsonl"
            rep = run_recurate(
                raw_path, recurated,
                dataset_ver="v-int-test", min_quality=0.6,
            )
            self.assertEqual(rep["kept_rows"], raw_count)

            # --- Stage 2: synthesis (mocked — write 4 absent-task files) ---
            synth_dir = tmpdir / "synth"
            self._build_synth_dir(synth_dir)

            # --- Stage 3: balance per tier ---
            real_rows = load_jsonl(recurated)
            synth_rows = []
            for p in sorted(synth_dir.glob("*.jsonl")):
                synth_rows.extend(load_jsonl(p))
            all_rows = real_rows + synth_rows

            # Expected totals per plan §5 Refinement 5
            expected_totals = {
                "tier1": 3500, "T": 1700, "C": 1200, "J": 600,
            }
            for tier, expected_total in expected_totals.items():
                targets = build_per_task_target(
                    tier, per_task_floor=200, ape_reflect_target=500,
                )
                balanced, bal_meta = balanced_sample(
                    all_rows, targets, seed=42, duplication_ceiling=5,
                )
                self.assertEqual(
                    len(balanced), expected_total,
                    f"tier {tier} expected {expected_total}, got {len(balanced)}",
                )

                # --- Stage 4: stratified split ---
                out_dir = tmpdir / f"sft_{tier}"
                train, valid, per_task = stratified_split(balanced, seed=42)
                write_jsonl_strat(out_dir / "train.jsonl", train)
                write_jsonl_strat(out_dir / "valid.jsonl", valid)
                manifest = build_manifest(
                    tier=tier, family_name=tier, out_dir=out_dir,
                    train_rows=train, valid_rows=valid, per_task=per_task,
                    seed=42, base_dataset_ver="v-int-test",
                    duplication_factors=bal_meta["duplication_factors"],
                    generator_sha=git_sha(_NEURAL_ROOT.parent),
                    ults_versions=load_ults_versions(
                        _NEURAL_ROOT.parent / "docs/tests/ults/specs",
                        TIER_TASKS[tier],
                    ),
                    synthesis_version="v1-deadbeef",
                    ape_reflect_target=500,
                    meta_placement="embedded",
                )
                (out_dir / "manifest.json").write_text(
                    json.dumps(manifest, indent=2, sort_keys=True),
                )

                # --- Assertions ---
                self.assertTrue((out_dir / "train.jsonl").exists())
                self.assertTrue((out_dir / "valid.jsonl").exists())
                self.assertTrue((out_dir / "manifest.json").exists())
                # Row total = tier target
                self.assertEqual(manifest["row_counts"]["total"], expected_total)
                # 90/10 split (approximate due to per-task rounding)
                self.assertGreater(manifest["row_counts"]["train"],
                                   int(expected_total * 0.85))
                self.assertLess(manifest["row_counts"]["valid"],
                                int(expected_total * 0.15))
                # SHA of train.jsonl on disk matches manifest
                self.assertEqual(
                    manifest["file_sha256"]["train.jsonl"],
                    file_sha256(out_dir / "train.jsonl"),
                )
                # SHA-pin values
                self.assertEqual(len(manifest["trained_against_model_sha"]), 64)
                self.assertEqual(len(manifest["raw_dataset_sha_pin"]), 64)
                # Provenance — every row has meta on the output
                for p in (out_dir / "train.jsonl", out_dir / "valid.jsonl"):
                    with open(p) as fh:
                        for line in fh:
                            if not line.strip():
                                continue
                            r = json.loads(line)
                            self.assertIn("meta", r)
                            self.assertIn("task_name", r["meta"])
                            self.assertTrue(r["meta"]["task_name"])

                # ape.reflect target enforcement (tier1 + T only)
                if tier in ("tier1", "T"):
                    self.assertEqual(
                        per_task["ape.reflect"]["total"], 500,
                        f"ape.reflect should be 500 in {tier}, got {per_task['ape.reflect']['total']}",
                    )

                # Weak-signal count present when metalearn.generalize is in tier
                if tier in ("tier1", "T"):
                    self.assertGreaterEqual(manifest["weak_signal_rows"], 1)
                else:
                    self.assertEqual(manifest["weak_signal_rows"], 0)

    def test_determinism_from_balance_onward(self):
        """Plan §3 Determinism scope — identical seed + inputs → identical SHA256.

        Re-run stages 3+4 twice with the same inputs and compare train.jsonl SHA.
        """
        with tempfile.TemporaryDirectory() as tmp:
            tmpdir = Path(tmp)
            raw_path = tmpdir / "raw.jsonl"
            self._build_raw_corpus(raw_path)
            recurated = tmpdir / "recurated.jsonl"
            run_recurate(raw_path, recurated, dataset_ver="v-int", min_quality=0.6)
            synth_dir = tmpdir / "synth"
            self._build_synth_dir(synth_dir)
            real_rows = load_jsonl(recurated)
            synth_rows = []
            for p in sorted(synth_dir.glob("*.jsonl")):
                synth_rows.extend(load_jsonl(p))
            all_rows = real_rows + synth_rows

            targets = build_per_task_target("T", 200, 500)

            # Pass 1
            balanced1, _ = balanced_sample(all_rows, targets, seed=42)
            train1, valid1, _ = stratified_split(balanced1, seed=42)
            out1 = tmpdir / "out1"
            write_jsonl_strat(out1 / "train.jsonl", train1)
            sha1 = file_sha256(out1 / "train.jsonl")

            # Pass 2 (identical inputs, identical seed)
            balanced2, _ = balanced_sample(all_rows, targets, seed=42)
            train2, valid2, _ = stratified_split(balanced2, seed=42)
            out2 = tmpdir / "out2"
            write_jsonl_strat(out2 / "train.jsonl", train2)
            sha2 = file_sha256(out2 / "train.jsonl")

            self.assertEqual(sha1, sha2,
                             "Determinism broken: same inputs + seed → different SHA")


if __name__ == "__main__":
    unittest.main()
