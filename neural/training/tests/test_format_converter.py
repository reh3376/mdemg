#!/usr/bin/env python3
"""Tests for the MLX chat format converter."""

import json
import os
import tempfile
import unittest

from training.format_converter import (
    convert_dpo_record,
    convert_record,
    run_converter,
    run_dpo_converter,
    validate_dpo_format,
    validate_mlx_format,
    _raft_include,
    _build_raft_context,
)


def _valid_record(**overrides):
    """Build a valid LLM interaction record for testing."""
    base = {
        "time": "2026-04-01T12:00:00Z",
        "trace_id": "test-trace-001",
        "task_name": "retrieval.rerank_cross",
        "system_prompt": "You are a relevance judge.",
        "user_prompt": "Query: how does retrieval work\nCandidates:\n1. The pipeline...",
        "response": "[0.92, 0.41, 0.87]",
        "think_content": None,
        "think_mode": False,
        "model_name": "gpt-4.1",
        "error": "",
        "retrieval_node_ids": None,
        "retrieval_scores": None,
    }
    base.update(overrides)
    return base


class TestConvertRecord(unittest.TestCase):
    """Test single record conversion."""

    def test_basic_conversion(self):
        record = _valid_record()
        result = convert_record(record)
        self.assertIn("messages", result)
        messages = result["messages"]
        self.assertEqual(len(messages), 3)
        self.assertEqual(messages[0]["role"], "system")
        self.assertEqual(messages[1]["role"], "user")
        self.assertEqual(messages[2]["role"], "assistant")

    def test_no_system_prompt(self):
        record = _valid_record(system_prompt="")
        result = convert_record(record)
        messages = result["messages"]
        self.assertEqual(len(messages), 2)
        self.assertEqual(messages[0]["role"], "user")
        self.assertEqual(messages[-1]["role"], "assistant")

    def test_think_mode_included(self):
        record = _valid_record(
            think_mode=True,
            think_content="Let me analyze the candidates...",
        )
        result = convert_record(record)
        assistant_content = result["messages"][-1]["content"]
        self.assertIn("<think>", assistant_content)
        self.assertIn("Let me analyze the candidates...", assistant_content)
        self.assertIn("</think>", assistant_content)

    def test_think_mode_without_content(self):
        record = _valid_record(think_mode=True, think_content=None)
        result = convert_record(record)
        assistant_content = result["messages"][-1]["content"]
        self.assertNotIn("<think>", assistant_content)

    def test_raft_context_included(self):
        record = _valid_record(
            retrieval_node_ids=["n_abc", "n_def"],
            retrieval_scores=[0.92, 0.87],
            trace_id="deterministic-raft-yes",
        )
        # Force 100% inclusion
        result = convert_record(record, raft_ratio=1.0)
        user_content = result["messages"][1]["content"]
        self.assertIn("[Retrieved Context]", user_content)
        self.assertIn("Node n_abc", user_content)
        self.assertIn("0.92", user_content)

    def test_raft_context_excluded(self):
        record = _valid_record(
            retrieval_node_ids=["n_abc", "n_def"],
            retrieval_scores=[0.92, 0.87],
        )
        # Force 0% inclusion
        result = convert_record(record, raft_ratio=0.0)
        user_content = result["messages"][1]["content"]
        self.assertNotIn("[Retrieved Context]", user_content)

    def test_no_retrieval_data(self):
        record = _valid_record()
        result = convert_record(record)
        user_content = result["messages"][1]["content"]
        self.assertNotIn("[Retrieved Context]", user_content)


class TestRaftDeterminism(unittest.TestCase):
    """Test RAFT inclusion is deterministic."""

    def test_same_trace_id_same_result(self):
        results = [_raft_include("trace-001", 0.8) for _ in range(100)]
        self.assertEqual(len(set(results)), 1)

    def test_different_trace_ids_vary(self):
        results = {_raft_include(f"trace-{i:04d}", 0.5) for i in range(100)}
        self.assertEqual(len(results), 2, "50% ratio should produce both True and False")


class TestRaftContext(unittest.TestCase):
    """Test RAFT context building."""

    def test_empty_nodes(self):
        record = {"retrieval_node_ids": [], "retrieval_scores": []}
        self.assertEqual(_build_raft_context(record), "")

    def test_none_nodes(self):
        record = {"retrieval_node_ids": None, "retrieval_scores": None}
        self.assertEqual(_build_raft_context(record), "")

    def test_context_format(self):
        record = {
            "retrieval_node_ids": ["n_abc", "n_def"],
            "retrieval_scores": [0.92, 0.87],
        }
        ctx = _build_raft_context(record)
        self.assertIn("[Retrieved Context]", ctx)
        self.assertIn("Node n_abc (score: 0.92)", ctx)
        self.assertIn("Node n_def (score: 0.87)", ctx)


class TestValidation(unittest.TestCase):
    """Test MLX format validation."""

    def test_valid_record(self):
        record = convert_record(_valid_record())
        errors = validate_mlx_format(record)
        self.assertEqual(len(errors), 0, f"Unexpected errors: {errors}")

    def test_missing_messages_key(self):
        errors = validate_mlx_format({"data": []})
        self.assertTrue(any("missing 'messages'" in e for e in errors))

    def test_empty_messages(self):
        errors = validate_mlx_format({"messages": []})
        self.assertTrue(any(">= 2" in e for e in errors))

    def test_invalid_role(self):
        record = {"messages": [
            {"role": "invalid", "content": "test"},
            {"role": "assistant", "content": "test"},
        ]}
        errors = validate_mlx_format(record)
        self.assertTrue(any("invalid role" in e for e in errors))

    def test_empty_content(self):
        record = {"messages": [
            {"role": "user", "content": ""},
            {"role": "assistant", "content": "test"},
        ]}
        errors = validate_mlx_format(record)
        self.assertTrue(any("empty content" in e for e in errors))

    def test_last_must_be_assistant(self):
        record = {"messages": [
            {"role": "user", "content": "test"},
            {"role": "user", "content": "another"},
        ]}
        errors = validate_mlx_format(record)
        self.assertTrue(any("assistant" in e for e in errors))


class TestRunConverter(unittest.TestCase):
    """Test the full conversion pipeline."""

    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()
        self.input_path = os.path.join(self.tmpdir, "input.jsonl")
        self.output_path = os.path.join(self.tmpdir, "output.jsonl")

    def tearDown(self):
        import shutil
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def test_basic_pipeline(self):
        records = [_valid_record(trace_id=f"t{i}", user_prompt=f"Query {i}") for i in range(10)]
        with open(self.input_path, "w") as f:
            for r in records:
                f.write(json.dumps(r) + "\n")

        report = run_converter(self.input_path, self.output_path)
        self.assertEqual(report["input_rows"], 10)
        self.assertEqual(report["output_rows"], 10)

        # Verify output format
        with open(self.output_path) as f:
            for line in f:
                msg = json.loads(line)
                self.assertIn("messages", msg)
                roles = [m["role"] for m in msg["messages"]]
                self.assertEqual(roles[0], "system")
                self.assertEqual(roles[-1], "assistant")
                for m in msg["messages"]:
                    self.assertTrue(m["content"])

    def test_round_trip_integrity(self):
        """Write JSONL → read back → verify format."""
        records = [_valid_record(trace_id=f"t{i}", user_prompt=f"Query {i}") for i in range(5)]
        with open(self.input_path, "w") as f:
            for r in records:
                f.write(json.dumps(r) + "\n")

        run_converter(self.input_path, self.output_path)

        with open(self.output_path) as f:
            for line in f:
                msg = json.loads(line)
                errors = validate_mlx_format(msg)
                self.assertEqual(len(errors), 0, f"Round-trip validation errors: {errors}")

    def test_raft_stats(self):
        records = [
            _valid_record(
                trace_id=f"t{i}",
                user_prompt=f"Query {i}",
                retrieval_node_ids=["n1", "n2"],
                retrieval_scores=[0.9, 0.8],
            )
            for i in range(100)
        ]
        with open(self.input_path, "w") as f:
            for r in records:
                f.write(json.dumps(r) + "\n")

        report = run_converter(self.input_path, self.output_path, raft_ratio=0.8)
        total_raft = report["raft_included"] + report["raft_excluded"]
        self.assertEqual(total_raft, 100)
        # With 100 samples at 0.8 ratio, expect roughly 80 included (allow variance)
        self.assertGreater(report["raft_included"], 50)
        self.assertGreater(report["raft_excluded"], 5)


class TestDPOConversion(unittest.TestCase):
    """Test DPO format conversion."""

    def test_convert_dpo_record(self):
        record = {
            "prompt": "What is guidance?",
            "chosen": "A good response.",
            "rejected": "A bad response.",
            "metadata": {"guidance_id": "g1"},
        }
        result = convert_dpo_record(record)
        self.assertEqual(result["prompt"], "What is guidance?")
        self.assertEqual(result["chosen"], "A good response.")
        self.assertEqual(result["rejected"], "A bad response.")
        self.assertEqual(result["metadata"]["guidance_id"], "g1")

    def test_validate_dpo_format_valid(self):
        record = {
            "prompt": "Question?",
            "chosen": "Good answer.",
            "rejected": "Bad answer.",
            "metadata": {},
        }
        errors = validate_dpo_format(record)
        self.assertEqual(len(errors), 0)

    def test_validate_dpo_format_missing_keys(self):
        errors = validate_dpo_format({"prompt": "Q?"})
        self.assertTrue(any("chosen" in e for e in errors))
        self.assertTrue(any("rejected" in e for e in errors))

    def test_validate_dpo_format_empty_values(self):
        record = {"prompt": "", "chosen": "good", "rejected": "bad"}
        errors = validate_dpo_format(record)
        self.assertTrue(any("empty" in e and "prompt" in e for e in errors))

    def test_dpo_pipeline(self):
        """End-to-end DPO conversion pipeline."""
        tmpdir = tempfile.mkdtemp()
        input_path = os.path.join(tmpdir, "input.jsonl")
        output_path = os.path.join(tmpdir, "output.jsonl")

        records = [
            {
                "prompt": f"Question {i}?",
                "chosen": f"Good answer {i}.",
                "rejected": f"Bad answer {i}.",
                "metadata": {"guidance_id": f"g{i}"},
            }
            for i in range(5)
        ]
        with open(input_path, "w") as f:
            for r in records:
                f.write(json.dumps(r) + "\n")

        report = run_dpo_converter(input_path, output_path)
        self.assertEqual(report["input_rows"], 5)
        self.assertEqual(report["output_rows"], 5)
        self.assertEqual(report["validation_errors"], 0)

        with open(output_path) as f:
            for line in f:
                rec = json.loads(line)
                self.assertIn("prompt", rec)
                self.assertIn("chosen", rec)
                self.assertIn("rejected", rec)

        import shutil
        shutil.rmtree(tmpdir, ignore_errors=True)


if __name__ == "__main__":
    unittest.main()
