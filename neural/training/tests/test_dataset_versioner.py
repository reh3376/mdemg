#!/usr/bin/env python3
"""Tests for the dataset versioner."""

import json
import os
import shutil
import tempfile
import unittest

from training.dataset_versioner import (
    check_temporal_integrity,
    compute_task_distribution,
    deduplicate,
    load_records,
    run_versioner,
    temporal_split,
)


def _record(trace_id: str, time: str, task: str = "retrieval.rerank_cross", **kw):
    """Build a minimal record for testing."""
    base = {
        "time": time,
        "trace_id": trace_id,
        "task_name": task,
        "system_prompt": f"System prompt for {trace_id}",
        "user_prompt": f"User prompt for {trace_id}",
        "response": f"Response for {trace_id} with enough text",
        "think_content": None,
        "think_mode": False,
        "model_name": "gpt-4.1",
        "error": "",
        "retrieval_node_ids": None,
        "retrieval_scores": None,
        "used_for_train": False,
    }
    base.update(kw)
    return base


def _write_jsonl(records, path):
    """Write records as JSONL."""
    os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
    with open(path, "w") as f:
        for r in records:
            f.write(json.dumps(r) + "\n")


class TestDeduplicate(unittest.TestCase):
    """Test deduplication logic."""

    def test_no_duplicates(self):
        records = [
            _record("t1", "2026-04-01T01:00:00Z"),
            _record("t2", "2026-04-01T02:00:00Z"),
        ]
        unique, removed = deduplicate(records)
        self.assertEqual(len(unique), 2)
        self.assertEqual(removed, 0)

    def test_duplicates_removed(self):
        records = [
            _record("t1", "2026-04-01T01:00:00Z"),
            _record("t2", "2026-04-01T02:00:00Z",
                    system_prompt="System prompt for t1",
                    user_prompt="User prompt for t1"),  # same as t1
        ]
        unique, removed = deduplicate(records)
        self.assertEqual(len(unique), 1)
        self.assertEqual(removed, 1)

    def test_keeps_first_by_time(self):
        records = [
            _record("t1", "2026-04-01T02:00:00Z"),
            _record("t2", "2026-04-01T01:00:00Z",
                    system_prompt="System prompt for t1",
                    user_prompt="User prompt for t1"),
        ]
        unique, _ = deduplicate(records)
        self.assertEqual(unique[0]["trace_id"], "t2")  # earlier time


class TestTemporalSplit(unittest.TestCase):
    """Test temporal splitting."""

    def test_standard_split(self):
        records = [_record(f"t{i}", f"2026-04-01T{i:02d}:00:00Z") for i in range(10)]
        train, test, val = temporal_split(records, 0.8, 0.1, 0.1)
        self.assertEqual(len(train), 8)
        self.assertEqual(len(test), 1)
        self.assertEqual(len(val), 1)

    def test_empty_records(self):
        train, test, val = temporal_split([], 0.8, 0.1, 0.1)
        self.assertEqual(len(train), 0)
        self.assertEqual(len(test), 0)
        self.assertEqual(len(val), 0)

    def test_too_few_records(self):
        records = [_record("t1", "2026-04-01T01:00:00Z")]
        train, test, val = temporal_split(records, 0.8, 0.1, 0.1)
        self.assertEqual(len(train), 1)
        self.assertEqual(len(test), 0)
        self.assertEqual(len(val), 0)


class TestTemporalIntegrity(unittest.TestCase):
    """Test temporal integrity check."""

    def test_valid_split(self):
        train = [_record("t1", "2026-04-01T01:00:00Z")]
        test = [_record("t2", "2026-04-01T02:00:00Z")]
        val = [_record("t3", "2026-04-01T03:00:00Z")]
        self.assertTrue(check_temporal_integrity(train, test, val))

    def test_leak_detected(self):
        train = [_record("t1", "2026-04-01T03:00:00Z")]
        test = [_record("t2", "2026-04-01T01:00:00Z")]
        val = []
        self.assertFalse(check_temporal_integrity(train, test, val))

    def test_empty_test_ok(self):
        train = [_record("t1", "2026-04-01T01:00:00Z")]
        self.assertTrue(check_temporal_integrity(train, [], []))


class TestTaskDistribution(unittest.TestCase):
    """Test task distribution counting."""

    def test_counts(self):
        records = [
            _record("t1", "2026-04-01T01:00:00Z", task="a.b"),
            _record("t2", "2026-04-01T02:00:00Z", task="a.b"),
            _record("t3", "2026-04-01T03:00:00Z", task="c.d"),
        ]
        dist = compute_task_distribution(records)
        self.assertEqual(dist["a.b"], 2)
        self.assertEqual(dist["c.d"], 1)


class TestLoadRecords(unittest.TestCase):
    """Test JSONL loading from directories."""

    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()

    def tearDown(self):
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def test_load_from_single_dir(self):
        records = [_record(f"t{i}", f"2026-04-01T{i:02d}:00:00Z") for i in range(5)]
        _write_jsonl(records, os.path.join(self.tmpdir, "llm_interactions.jsonl"))
        loaded = load_records([self.tmpdir])
        self.assertEqual(len(loaded), 5)

    def test_load_from_multiple_dirs(self):
        dir1 = os.path.join(self.tmpdir, "src1")
        dir2 = os.path.join(self.tmpdir, "src2")
        _write_jsonl(
            [_record("t1", "2026-04-01T01:00:00Z")],
            os.path.join(dir1, "llm_interactions.jsonl"),
        )
        _write_jsonl(
            [_record("t2", "2026-04-01T02:00:00Z")],
            os.path.join(dir2, "llm_interactions.jsonl"),
        )
        loaded = load_records([dir1, dir2])
        self.assertEqual(len(loaded), 2)

    def test_source_tagged(self):
        _write_jsonl(
            [_record("t1", "2026-04-01T01:00:00Z")],
            os.path.join(self.tmpdir, "data.jsonl"),
        )
        loaded = load_records([self.tmpdir])
        self.assertIn("_source", loaded[0])


class TestRunVersioner(unittest.TestCase):
    """Test the full versioner pipeline."""

    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()
        self.input_dir = os.path.join(self.tmpdir, "input")
        self.output_dir = os.path.join(self.tmpdir, "output")
        os.makedirs(self.input_dir)

    def tearDown(self):
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def test_basic_pipeline(self):
        records = [_record(f"t{i}", f"2026-04-01T{i:02d}:00:00Z") for i in range(20)]
        _write_jsonl(records, os.path.join(self.input_dir, "llm_interactions.jsonl"))

        manifest = run_versioner(
            input_dirs=[self.input_dir],
            output_dir=self.output_dir,
            version="v1",
        )

        self.assertEqual(manifest["version"], "v1")
        self.assertTrue(manifest["quality_gates"]["temporal_split"])
        self.assertTrue(manifest["quality_gates"]["no_train_test_overlap"])
        self.assertEqual(manifest["deduplication"]["duplicates_removed"], 0)

        # Check files exist
        for split in ["train", "test", "val"]:
            path = os.path.join(self.output_dir, f"{split}.jsonl")
            self.assertTrue(os.path.exists(path), f"{split}.jsonl should exist")

        # Check manifest file
        manifest_path = os.path.join(self.output_dir, "manifest.json")
        self.assertTrue(os.path.exists(manifest_path))

    def test_deduplication_across_sources(self):
        records = [_record(f"t{i}", f"2026-04-01T{i:02d}:00:00Z") for i in range(10)]

        dir1 = os.path.join(self.tmpdir, "src1")
        dir2 = os.path.join(self.tmpdir, "src2")
        _write_jsonl(records, os.path.join(dir1, "llm_interactions.jsonl"))
        _write_jsonl(records, os.path.join(dir2, "llm_interactions.jsonl"))

        manifest = run_versioner(
            input_dirs=[dir1, dir2],
            output_dir=self.output_dir,
            version="v1",
        )

        self.assertEqual(manifest["deduplication"]["total_before"], 20)
        self.assertEqual(manifest["deduplication"]["duplicates_removed"], 10)
        self.assertEqual(manifest["deduplication"]["total_after"], 10)

    def test_output_format_valid(self):
        records = [_record(f"t{i}", f"2026-04-01T{i:02d}:00:00Z") for i in range(10)]
        _write_jsonl(records, os.path.join(self.input_dir, "llm_interactions.jsonl"))

        run_versioner(
            input_dirs=[self.input_dir],
            output_dir=self.output_dir,
            version="v1",
        )

        # Verify every line in train.jsonl is valid MLX format
        train_path = os.path.join(self.output_dir, "train.jsonl")
        with open(train_path) as f:
            for line in f:
                msg = json.loads(line)
                self.assertIn("messages", msg)
                roles = [m["role"] for m in msg["messages"]]
                self.assertEqual(roles[-1], "assistant")
                for m in msg["messages"]:
                    self.assertTrue(m["content"])

    def test_temporal_integrity_enforced(self):
        records = [_record(f"t{i}", f"2026-04-01T{i:02d}:00:00Z") for i in range(20)]
        _write_jsonl(records, os.path.join(self.input_dir, "llm_interactions.jsonl"))

        manifest = run_versioner(
            input_dirs=[self.input_dir],
            output_dir=self.output_dir,
            version="v1",
        )

        self.assertTrue(manifest["quality_gates"]["temporal_split"])
        self.assertTrue(manifest["quality_gates"]["no_train_test_overlap"])

    def test_sha256_in_manifest(self):
        records = [_record(f"t{i}", f"2026-04-01T{i:02d}:00:00Z") for i in range(10)]
        _write_jsonl(records, os.path.join(self.input_dir, "llm_interactions.jsonl"))

        manifest = run_versioner(
            input_dirs=[self.input_dir],
            output_dir=self.output_dir,
            version="v1",
        )

        for split in ["train", "test", "val"]:
            sha = manifest["splits"][split]["sha256"]
            self.assertEqual(len(sha), 64, f"{split} SHA-256 should be 64 hex chars")

    def test_task_balance_warning(self):
        records = [
            _record("t1", "2026-04-01T01:00:00Z", task="rare.task"),
            *[_record(f"t{i}", f"2026-04-01T{i:02d}:00:00Z") for i in range(2, 20)],
        ]
        _write_jsonl(records, os.path.join(self.input_dir, "llm_interactions.jsonl"))

        manifest = run_versioner(
            input_dirs=[self.input_dir],
            output_dir=self.output_dir,
            version="v1",
            min_per_task=5,
        )

        # rare.task has only 1 record in train
        self.assertFalse(manifest["quality_gates"]["min_per_task_met"])

    def test_empty_input_raises(self):
        empty_dir = os.path.join(self.tmpdir, "empty")
        os.makedirs(empty_dir)
        with self.assertRaises(ValueError):
            run_versioner(
                input_dirs=[empty_dir],
                output_dir=self.output_dir,
                version="v1",
            )

    def test_paradigm_in_manifest(self):
        """Manifest includes paradigm field."""
        records = [_record(f"t{i}", f"2026-04-01T{i:02d}:00:00Z") for i in range(10)]
        _write_jsonl(records, os.path.join(self.input_dir, "llm_interactions.jsonl"))

        manifest = run_versioner(
            input_dirs=[self.input_dir],
            output_dir=self.output_dir,
            version="v1",
            paradigm="sft",
        )
        self.assertEqual(manifest["paradigm"], "sft")

    def test_dpo_manifest(self):
        """DPO paradigm manifest includes correct paradigm field."""
        records = [_record(f"t{i}", f"2026-04-01T{i:02d}:00:00Z") for i in range(10)]
        _write_jsonl(records, os.path.join(self.input_dir, "llm_interactions.jsonl"))

        manifest = run_versioner(
            input_dirs=[self.input_dir],
            output_dir=self.output_dir,
            version="v1",
            paradigm="dpo",
        )
        self.assertEqual(manifest["paradigm"], "dpo")

        # Verify manifest file also has it
        manifest_path = os.path.join(self.output_dir, "manifest.json")
        with open(manifest_path) as f:
            saved = json.load(f)
        self.assertEqual(saved["paradigm"], "dpo")


if __name__ == "__main__":
    unittest.main()
