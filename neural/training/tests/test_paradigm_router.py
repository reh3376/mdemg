#!/usr/bin/env python3
"""Tests for the paradigm router."""

import json
import os
import tempfile
import unittest

from training.paradigm_router import (
    load_uaits_spec,
    route_dataset,
    run_curation,
)


SPEC_PATH = os.path.join(
    os.path.dirname(__file__), "..", "..", "..",
    "docs", "tests", "uaits", "specs", "mdemg.uaits.json",
)


def _write_jsonl(path, records):
    """Write records to a JSONL file."""
    os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
    with open(path, "w") as f:
        for r in records:
            f.write(json.dumps(r) + "\n")


def _interaction(trace_id="t1", task_name="jiminy.synthesize", **overrides):
    """Build a minimal llm_interactions record."""
    base = {
        "time": "2026-04-01T12:00:00Z",
        "trace_id": trace_id,
        "task_name": task_name,
        "space_id": "test-space",
        "session_id": "s1",
        "system_prompt": "You are a helpful assistant.",
        "user_prompt": f"Query for {trace_id}",
        "response": f"Response text for {trace_id} with enough length.",
        "think_content": None,
        "think_mode": False,
        "latency_ms": 500,
        "tokens_in": 100,
        "tokens_out": 50,
        "model_name": "gpt-5.4",
        "provider": "openai",
        "error": "",
        "guidance_id": None,
        "retrieval_node_ids": None,
        "retrieval_scores": None,
        "used_for_train": False,
    }
    base.update(overrides)
    return base


class TestLoadSpec(unittest.TestCase):
    """Test UAITS spec loading."""

    def test_load_valid_spec(self):
        """Load the real mdemg.uaits.json spec."""
        if not os.path.exists(SPEC_PATH):
            self.skipTest("UAITS spec not found")
        spec = load_uaits_spec(SPEC_PATH)
        self.assertEqual(spec["application"]["name"], "mdemg")
        self.assertEqual(len(spec["datasets"]), 4)

    def test_missing_fields_raises(self):
        """Spec missing required fields raises ValueError."""
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".json", delete=False,
        ) as f:
            json.dump({"uaits_version": "1.0.0"}, f)
            f.flush()
            with self.assertRaises(ValueError):
                load_uaits_spec(f.name)
        os.unlink(f.name)

    def test_empty_datasets_raises(self):
        """Spec with empty datasets array raises ValueError."""
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".json", delete=False,
        ) as f:
            json.dump({
                "uaits_version": "1.0.0",
                "application": {"name": "test"},
                "datasets": [],
            }, f)
            f.flush()
            with self.assertRaises(ValueError):
                load_uaits_spec(f.name)
        os.unlink(f.name)


class TestRouteDataset(unittest.TestCase):
    """Test dataset routing dispatch."""

    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()
        self.input_dir = self.tmpdir
        self.output_dir = os.path.join(self.tmpdir, "output")

    def tearDown(self):
        import shutil
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def test_route_sft_dataset(self):
        """SFT routing produces completed status with interactions file."""
        records = [
            _interaction(trace_id=f"t{i}", user_prompt=f"Query {i}")
            for i in range(20)
        ]
        _write_jsonl(
            os.path.join(self.input_dir, "llm_interactions.jsonl"),
            records,
        )

        ds_config = {
            "name": "sft_test",
            "paradigm": "sft",
            "quality_gates": {"dedup_mode": "prompt"},
            "format": {"raft_ratio": 0.8},
            "versioning": {
                "train_ratio": 0.8,
                "test_ratio": 0.1,
                "val_ratio": 0.1,
                "min_per_task": 1,
                "exogenous_ratio_min": 0.4,
            },
        }
        report = route_dataset(
            ds_config, self.input_dir, self.output_dir, version="v1",
        )
        self.assertEqual(report["status"], "completed")
        self.assertEqual(report["paradigm"], "sft")

    def test_route_curriculum_dataset(self):
        """Curriculum routing counts metric records."""
        records = [{"metric_name": f"rsic_health_{i}", "value": i} for i in range(15)]
        _write_jsonl(
            os.path.join(self.input_dir, "metric_samples.jsonl"),
            records,
        )

        ds_config = {
            "name": "curriculum_test",
            "paradigm": "curriculum",
            "source": {"primary_table": "metric_samples"},
        }
        report = route_dataset(
            ds_config, self.input_dir, self.output_dir, version="v1",
        )
        self.assertEqual(report["status"], "completed")
        self.assertEqual(report["records"], 15)

    def test_dry_run_no_files(self):
        """Dry run produces dry_run status without writing output."""
        records = [_interaction(trace_id=f"t{i}") for i in range(5)]
        _write_jsonl(
            os.path.join(self.input_dir, "llm_interactions.jsonl"),
            records,
        )

        ds_config = {
            "name": "sft_dry",
            "paradigm": "sft",
            "quality_gates": {"dedup_mode": "prompt"},
        }
        report = route_dataset(
            ds_config, self.input_dir, self.output_dir,
            version="v1", dry_run=True,
        )
        self.assertEqual(report["status"], "dry_run")
        self.assertFalse(os.path.exists(self.output_dir))

    def test_unknown_paradigm_fails(self):
        """Invalid paradigm raises ValueError."""
        ds_config = {
            "name": "bad_paradigm",
            "paradigm": "unknown_thing",
        }
        with self.assertRaises(ValueError):
            route_dataset(
                ds_config, self.input_dir, self.output_dir, version="v1",
            )

    def test_missing_input_file_skips(self):
        """Missing input file returns skipped status."""
        ds_config = {
            "name": "sft_missing",
            "paradigm": "sft",
            "quality_gates": {"dedup_mode": "prompt"},
        }
        report = route_dataset(
            ds_config, self.input_dir, self.output_dir, version="v1",
        )
        self.assertEqual(report["status"], "skipped")


class TestRunCuration(unittest.TestCase):
    """Test full spec-driven curation."""

    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()
        self.input_dir = self.tmpdir
        self.output_dir = os.path.join(self.tmpdir, "output")

    def tearDown(self):
        import shutil
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def test_curation_with_real_spec(self):
        """Run curation dry-run with the real MDEMG spec."""
        if not os.path.exists(SPEC_PATH):
            self.skipTest("UAITS spec not found")

        results = run_curation(
            SPEC_PATH, self.input_dir, self.output_dir, dry_run=True,
        )
        self.assertEqual(results["application"], "mdemg")
        self.assertEqual(len(results["datasets"]), 4)
        # All should be skipped (no input files) in dry-run with no data
        for ds in results["datasets"]:
            self.assertIn(ds["status"], ("skipped", "dry_run"))


if __name__ == "__main__":
    unittest.main()
