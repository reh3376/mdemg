#!/usr/bin/env python3
"""Sprint FT-LORA-D Epic 4 — Unit tests for profile_expert_routing.

Tier-1 scope (pure functions, no MLX / no real model):
  - anchor_prompts loader
  - top-k aggregation correctness
  - top-pct mask with tie-break
  - Jaccard overlap
  - within-family task-cohesion
  - hierarchical cluster boundary
  - SHA256 guard (file-hash path)

Tier-2 (integration) + Tier-3 (E2E) are run via CLI in Epic 3/4 and verified
out-of-band; see sprint_plan_ft_lora_d.md §6.
"""
from __future__ import annotations

import hashlib
import json
import tempfile
import unittest
from pathlib import Path

from training.profile_expert_routing import (  # noqa: E402
    build_raw_counts,
    cohesion_verdict,
    compute_cross_family_overlap,
    compute_family_artifact,
    hierarchical_cluster_boundary,
    jaccard,
    load_anchor_prompts,
    per_task_top_experts,
    verify_model_sha,
    within_family_pairwise_jaccard,
)


class TestAnchorPromptsLoader(unittest.TestCase):
    def test_loads_all_records(self):
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / "a.jsonl"
            with p.open("w") as f:
                for i, fam in enumerate(
                    ["reasoning_think", "classify_notink", "structured_notink"]
                ):
                    f.write(
                        json.dumps(
                            {
                                "task": f"t{i}",
                                "family": fam,
                                "prompt": f"q{i}",
                                "source": "production_trace",
                                "source_id": str(i),
                            }
                        )
                        + "\n"
                    )
            recs = load_anchor_prompts(p)
            self.assertEqual(len(recs), 3)
            self.assertEqual(recs[1]["family"], "classify_notink")

    def test_task_filter(self):
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / "a.jsonl"
            with p.open("w") as f:
                for i in range(4):
                    f.write(
                        json.dumps(
                            {"task": f"t{i % 2}", "family": "x", "prompt": "q"}
                        )
                        + "\n"
                    )
            recs = load_anchor_prompts(p, task_filter="t1")
            self.assertEqual(len(recs), 2)
            self.assertTrue(all(r["task"] == "t1" for r in recs))

    def test_skips_blank_lines(self):
        with tempfile.TemporaryDirectory() as td:
            p = Path(td) / "a.jsonl"
            p.write_text(
                '{"task":"t","family":"f","prompt":"q"}\n\n'
                '{"task":"u","family":"f","prompt":"r"}\n'
            )
            self.assertEqual(len(load_anchor_prompts(p)), 2)


class TestTopKAggregation(unittest.TestCase):
    """Synthetic `records` → `build_raw_counts` → per-task per-layer counts."""

    def _rec(self, task, layer, tokens, family="reasoning_think"):
        return {
            "task": task,
            "layer_idx": layer,
            "family": family,
            "prompt_id": f"{task}_p",
            "segment": "prompt",
            "num_tokens": len(tokens),
            "per_token_experts": tokens,
        }

    def test_counts_across_tokens(self):
        records = [
            # task t1, layer 0: 2 tokens, each selecting 3 experts
            self._rec("t1", 0, [[0, 1, 2], [1, 2, 3]]),
            # task t1, layer 1: 1 token
            self._rec("t1", 1, [[0, 0, 4]]),  # expert 0 selected twice (pathological)
        ]
        raw = build_raw_counts(records, num_layers=2, num_experts=5)
        # t1 layer 0 counts: expert 0: 1, 1: 2, 2: 2, 3: 1, 4: 0
        self.assertEqual(raw["task_counts"]["t1"][0], [1, 2, 2, 1, 0])
        self.assertEqual(raw["task_counts"]["t1"][1], [2, 0, 0, 0, 1])
        # task_tokens counts only layer-0 records (distinct tokens).
        self.assertEqual(raw["task_tokens"]["t1"], 2)

    def test_multiple_tasks(self):
        records = [
            self._rec("t1", 0, [[0, 1, 2]], family="classify_notink"),
            self._rec("t2", 0, [[0, 2, 4]], family="structured_notink"),
        ]
        raw = build_raw_counts(records, num_layers=1, num_experts=5)
        self.assertIn("t1", raw["task_counts"])
        self.assertIn("t2", raw["task_counts"])
        self.assertEqual(raw["task_family"]["t1"], "classify_notink")
        self.assertEqual(raw["task_family"]["t2"], "structured_notink")


class TestTopPctMask(unittest.TestCase):
    def _raw(self, task_counts, family_map):
        return {
            "num_layers": len(next(iter(task_counts.values()))),
            "num_experts": len(next(iter(task_counts.values()))[0]),
            "task_counts": task_counts,
            "task_tokens": {t: 100 for t in task_counts},
            "task_family": family_map,
        }

    def test_top25pct_selection(self):
        # 8 experts, top 25% = 2 experts per layer.
        counts = {"t1": [[10, 5, 20, 1, 30, 2, 8, 15]]}
        raw = self._raw(counts, {"t1": "reasoning_think"})
        art = compute_family_artifact(
            family="reasoning_think",
            raw=raw,
            top_pct=25,
            model_sha256_config="x",
            prompts_processed=1,
            top_k_per_token=8,
            mlx_lm_version="0.31.2",
        )
        self.assertEqual(art["num_experts_selected_per_layer"], 2)
        # Top 2 by count: expert 4 (30), expert 2 (20). Artifact sorts ascending.
        self.assertEqual(art["per_layer"][0]["top_experts"], [2, 4])

    def test_tie_break_lower_idx_wins(self):
        # All experts tied at 10 → top 2 are experts 0 and 1 (lowest indices).
        counts = {"t1": [[10] * 8]}
        raw = self._raw(counts, {"t1": "reasoning_think"})
        art = compute_family_artifact(
            family="reasoning_think",
            raw=raw,
            top_pct=25,
            model_sha256_config="x",
            prompts_processed=1,
            top_k_per_token=8,
            mlx_lm_version="0.31.2",
        )
        self.assertEqual(art["per_layer"][0]["top_experts"], [0, 1])

    def test_kl_nonnegative_and_zero_for_uniform(self):
        # Uniform distribution → KL divergence = 0.
        counts = {"t1": [[10] * 8]}
        raw = self._raw(counts, {"t1": "reasoning_think"})
        art = compute_family_artifact(
            family="reasoning_think",
            raw=raw,
            top_pct=25,
            model_sha256_config="x",
            prompts_processed=1,
            top_k_per_token=8,
            mlx_lm_version="0.31.2",
        )
        self.assertAlmostEqual(art["per_layer"][0]["kl_div_vs_uniform"], 0.0, places=6)

        # Concentrated distribution → KL > 0.
        counts2 = {"t1": [[100, 0, 0, 0, 0, 0, 0, 0]]}
        raw2 = self._raw(counts2, {"t1": "reasoning_think"})
        art2 = compute_family_artifact(
            family="reasoning_think",
            raw=raw2,
            top_pct=25,
            model_sha256_config="x",
            prompts_processed=1,
            top_k_per_token=8,
            mlx_lm_version="0.31.2",
        )
        self.assertGreater(art2["per_layer"][0]["kl_div_vs_uniform"], 0.0)


class TestJaccardOverlap(unittest.TestCase):
    def test_identical(self):
        self.assertEqual(jaccard([1, 2, 3], [1, 2, 3]), 1.0)

    def test_disjoint(self):
        self.assertEqual(jaccard([1, 2, 3], [4, 5, 6]), 0.0)

    def test_partial(self):
        # {1,2,3} vs {2,3,4}: intersect=2, union=4 → 0.5
        self.assertEqual(jaccard([1, 2, 3], [2, 3, 4]), 0.5)

    def test_both_empty(self):
        self.assertEqual(jaccard([], []), 1.0)

    def test_cross_family_overlap_per_pair(self):
        # 2 layers, 4 experts. Family A picks {0,1}. Family B picks {1,2}.
        family_artifacts = {
            "A": {
                "per_layer": [
                    {"top_experts": [0, 1]},
                    {"top_experts": [0, 1]},
                ],
            },
            "B": {
                "per_layer": [
                    {"top_experts": [1, 2]},
                    {"top_experts": [1, 2]},
                ],
            },
        }
        out = compute_cross_family_overlap(family_artifacts)
        pair_key = "A__vs__B"
        self.assertIn(pair_key, out["pairs"])
        # Intersect=1, union=3 → 0.3333 each layer.
        self.assertAlmostEqual(out["pairs"][pair_key]["mean_jaccard"], 1 / 3, places=4)


class TestTaskCohesionWithinFamily(unittest.TestCase):
    def _raw(self):
        # 2-layer toy: family T has 3 tasks. a,b are similar; c is far.
        num_experts = 8
        return {
            "num_layers": 2,
            "num_experts": num_experts,
            "task_counts": {
                # a and b pick experts {0,1,2,3} strongly
                "a": [[10, 9, 8, 7, 0, 0, 0, 0], [10, 9, 8, 7, 0, 0, 0, 0]],
                "b": [[9, 10, 7, 8, 0, 0, 0, 0], [9, 10, 7, 8, 0, 0, 0, 0]],
                # c picks experts {4,5,6,7}
                "c": [[0, 0, 0, 0, 10, 9, 8, 7], [0, 0, 0, 0, 10, 9, 8, 7]],
            },
            "task_tokens": {"a": 10, "b": 10, "c": 10},
            "task_family": {"a": "T", "b": "T", "c": "T"},
        }

    def test_cohesive_family(self):
        # Only a+b → should be cohesive.
        raw = self._raw()
        # Remove c for cohesive-only test.
        raw["task_counts"].pop("c")
        raw["task_tokens"].pop("c")
        raw["task_family"].pop("c")
        task_top = per_task_top_experts(raw, top_pct=50)  # top 4 of 8
        result = within_family_pairwise_jaccard(task_top, raw["task_family"], "T")
        # a and b both pick {0,1,2,3} → identical → Jaccard=1.
        self.assertAlmostEqual(result["mean_jaccard"], 1.0, places=4)
        self.assertEqual(cohesion_verdict(result["mean_jaccard"]), "cohesive")

    def test_split_candidate_family(self):
        raw = self._raw()
        task_top = per_task_top_experts(raw, top_pct=50)  # top 4 of 8
        result = within_family_pairwise_jaccard(task_top, raw["task_family"], "T")
        # Pairs: a-b=1.0, a-c=0.0, b-c=0.0 → mean = 1/3 ≈ 0.333
        self.assertAlmostEqual(result["mean_jaccard"], 1 / 3, places=4)
        self.assertEqual(cohesion_verdict(result["mean_jaccard"]), "split-candidate")

    def test_ambiguous_verdict(self):
        self.assertEqual(cohesion_verdict(0.55), "ambiguous")
        self.assertEqual(cohesion_verdict(0.40), "ambiguous")
        self.assertEqual(cohesion_verdict(0.39), "split-candidate")
        self.assertEqual(cohesion_verdict(0.70), "cohesive")


class TestHierarchicalClusterBoundary(unittest.TestCase):
    def test_two_cluster_split(self):
        # 5 tasks: a,b,c tight cluster; d,e tight cluster; two clusters separated.
        task_top = {
            "a": [[0, 1, 2, 3]],
            "b": [[0, 1, 2, 4]],  # close to a
            "c": [[0, 1, 3, 4]],  # close to a,b
            "d": [[5, 6, 7, 8]],
            "e": [[5, 6, 7, 9]],  # close to d
        }
        clusters = hierarchical_cluster_boundary(task_top, ["a", "b", "c", "d", "e"])
        self.assertIsNotNone(clusters)
        # Expect {a,b,c} vs {d,e} — order-insensitive.
        cs = [set(c) for c in clusters]
        self.assertIn({"a", "b", "c"}, cs)
        self.assertIn({"d", "e"}, cs)

    def test_single_task_returns_none(self):
        self.assertIsNone(hierarchical_cluster_boundary({"a": [[0]]}, ["a"]))

    def test_two_tasks_simple_split(self):
        clusters = hierarchical_cluster_boundary(
            {"a": [[0, 1]], "b": [[2, 3]]}, ["a", "b"]
        )
        self.assertEqual({frozenset(c) for c in clusters}, {frozenset(["a"]), frozenset(["b"])})


class TestSHA256Guard(unittest.TestCase):
    def test_matching_sha_passes(self):
        with tempfile.TemporaryDirectory() as td:
            cfg = Path(td) / "config.json"
            cfg.write_text('{"foo": "bar"}')
            expected = hashlib.sha256(cfg.read_bytes()).hexdigest()
            # Should not raise.
            verify_model_sha(Path(td), expected)

    def test_mismatching_sha_raises(self):
        with tempfile.TemporaryDirectory() as td:
            cfg = Path(td) / "config.json"
            cfg.write_text('{"foo": "bar"}')
            with self.assertRaises(SystemExit):
                verify_model_sha(Path(td), "0" * 64)

    def test_missing_config_raises(self):
        with tempfile.TemporaryDirectory() as td:
            with self.assertRaises(FileNotFoundError):
                verify_model_sha(Path(td), "0" * 64)


if __name__ == "__main__":
    unittest.main()
