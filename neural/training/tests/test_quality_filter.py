#!/usr/bin/env python3
"""Tests for the training data quality filter."""

import json
import os
import tempfile
import unittest

from training.quality_filter import (
    check_privacy,
    load_ults_specs,
    run_filter,
    validate_output_schema,
)


def _valid_record(**overrides):
    """Build a valid LLM interaction record for testing."""
    base = {
        "time": "2026-04-01T12:00:00Z",
        "trace_id": "t1",
        "task_name": "retrieval.rerank_cross",
        "space_id": "test-space",
        "session_id": "s1",
        "system_prompt": "You are a relevance judge.",
        "user_prompt": "Query: how does retrieval work\nCandidates:\n1. The pipeline...",
        "response": "[0.92, 0.41, 0.87, 0.15]",
        "think_content": None,
        "think_mode": False,
        "latency_ms": 500,
        "tokens_in": 100,
        "tokens_out": 50,
        "model_name": "gpt-4.1",
        "provider": "openai",
        "error": "",
        "quality": None,
        "quality_source": None,
        "used_for_train": False,
        "dataset_ver": None,
        "guidance_id": None,
        "source_path": None,
        "retrieval_node_ids": None,
        "retrieval_scores": None,
        "oracle_node_id": None,
        "system_prompt_hash": "aabb" * 16,
    }
    base.update(overrides)
    return base


def _write_jsonl(records, path):
    """Write records as JSONL."""
    with open(path, "w") as f:
        for r in records:
            f.write(json.dumps(r) + "\n")


class TestPrivacyCheck(unittest.TestCase):
    """Test privacy pattern detection."""

    def test_clean_record_passes(self):
        record = _valid_record()
        self.assertFalse(check_privacy(record))

    def test_api_key_detected(self):
        record = _valid_record(user_prompt="Use key sk-abc123def456ghi789jkl012mno345pqr678")
        self.assertTrue(check_privacy(record))

    def test_email_detected(self):
        record = _valid_record(response="Contact john.doe@example.com for details")
        self.assertTrue(check_privacy(record))

    def test_absolute_path_detected(self):
        record = _valid_record(source_path="/Users/johndoe/secret/project/main.go")
        self.assertTrue(check_privacy(record))

    def test_env_secret_detected(self):
        record = _valid_record(system_prompt="Set PASSWORD=hunter2 in your env")
        self.assertTrue(check_privacy(record))

    def test_neo4j_cred_detected(self):
        record = _valid_record(user_prompt="Connect to neo4j://admin:secret@localhost:7687")
        self.assertTrue(check_privacy(record))

    def test_null_fields_safe(self):
        record = _valid_record(system_prompt=None, user_prompt=None, response="valid response text here")
        self.assertFalse(check_privacy(record))


class TestOutputSchemaValidation(unittest.TestCase):
    """Test ULTS output schema validation."""

    def test_valid_json_with_required_keys(self):
        schema = {"required": ["insights"], "type": "object"}
        response = json.dumps({"insights": [{"type": "gap", "description": "test"}]})
        self.assertTrue(validate_output_schema(response, schema))

    def test_missing_required_key_fails(self):
        schema = {"required": ["insights"], "type": "object"}
        response = json.dumps({"other_key": "value"})
        self.assertFalse(validate_output_schema(response, schema))

    def test_invalid_json_fails(self):
        schema = {"required": ["insights"], "type": "object"}
        self.assertFalse(validate_output_schema("not json at all", schema))

    def test_no_required_keys_passes(self):
        schema = {"type": "object"}
        response = json.dumps({"anything": "works"})
        self.assertTrue(validate_output_schema(response, schema))


class TestQualityFilter(unittest.TestCase):
    """Test the full quality filter pipeline."""

    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()
        self.input_path = os.path.join(self.tmpdir, "input.jsonl")
        self.output_path = os.path.join(self.tmpdir, "output.jsonl")

    def tearDown(self):
        import shutil
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def test_valid_records_pass(self):
        records = [_valid_record(trace_id=f"t{i}", user_prompt=f"Query {i}") for i in range(5)]
        _write_jsonl(records, self.input_path)
        report = run_filter(self.input_path, self.output_path)
        self.assertEqual(report["input_rows"], 5)
        self.assertEqual(report["output_rows"], 5)

    def test_empty_response_excluded(self):
        records = [
            _valid_record(response=""),
            _valid_record(trace_id="t2", user_prompt="other query", response="valid response text here"),
        ]
        _write_jsonl(records, self.input_path)
        report = run_filter(self.input_path, self.output_path)
        self.assertEqual(report["excluded"]["empty_response"], 1)
        self.assertEqual(report["output_rows"], 1)

    def test_error_present_excluded(self):
        records = [
            _valid_record(error="timeout after 30s"),
            _valid_record(trace_id="t2", user_prompt="other", response="valid response text here"),
        ]
        _write_jsonl(records, self.input_path)
        report = run_filter(self.input_path, self.output_path)
        self.assertEqual(report["excluded"]["error_present"], 1)
        self.assertEqual(report["output_rows"], 1)

    def test_duplicate_prompt_excluded(self):
        records = [
            _valid_record(),
            _valid_record(trace_id="t2"),  # same system_prompt + user_prompt
        ]
        _write_jsonl(records, self.input_path)
        report = run_filter(self.input_path, self.output_path)
        self.assertEqual(report["excluded"]["duplicate_prompt"], 1)
        self.assertEqual(report["output_rows"], 1)

    def test_high_latency_excluded(self):
        records = [
            _valid_record(latency_ms=999_999),
            _valid_record(trace_id="t2", user_prompt="other", response="valid response text here", latency_ms=100),
        ]
        _write_jsonl(records, self.input_path)
        report = run_filter(self.input_path, self.output_path)
        self.assertEqual(report["excluded"]["latency_exceeded"], 1)
        self.assertEqual(report["output_rows"], 1)

    def test_unknown_model_excluded(self):
        records = [
            _valid_record(model_name=""),
            _valid_record(trace_id="t2", user_prompt="other", response="valid text response here"),
        ]
        _write_jsonl(records, self.input_path)
        report = run_filter(self.input_path, self.output_path)
        self.assertEqual(report["excluded"]["unknown_model"], 1)
        self.assertEqual(report["output_rows"], 1)

    def test_privacy_violation_excluded(self):
        records = [
            _valid_record(user_prompt="Send to sk-abc123def456ghi789jkl012mno345pqr"),
            _valid_record(trace_id="t2", user_prompt="clean query", response="valid response text here"),
        ]
        _write_jsonl(records, self.input_path)
        report = run_filter(self.input_path, self.output_path)
        self.assertEqual(report["excluded"]["privacy_violation"], 1)
        self.assertEqual(report["output_rows"], 1)

    def test_task_distribution_tracked(self):
        records = [
            _valid_record(task_name="retrieval.rerank_cross", trace_id="t1", user_prompt="q1"),
            _valid_record(task_name="retrieval.rerank_cross", trace_id="t2", user_prompt="q2"),
            _valid_record(task_name="hidden.reclassify", trace_id="t3", user_prompt="q3"),
        ]
        _write_jsonl(records, self.input_path)
        report = run_filter(self.input_path, self.output_path)
        self.assertEqual(report["task_distribution"]["retrieval.rerank_cross"], 2)
        self.assertEqual(report["task_distribution"]["hidden.reclassify"], 1)

    def test_dry_run_no_output_file(self):
        records = [_valid_record()]
        _write_jsonl(records, self.input_path)
        report = run_filter(self.input_path, None, dry_run=True)
        self.assertEqual(report["output_rows"], 1)
        self.assertFalse(os.path.exists(self.output_path))

    def test_output_file_created(self):
        records = [_valid_record()]
        _write_jsonl(records, self.input_path)
        run_filter(self.input_path, self.output_path)
        self.assertTrue(os.path.exists(self.output_path))
        with open(self.output_path) as f:
            lines = f.readlines()
        self.assertEqual(len(lines), 1)

    def test_dynamic_prompt_task_not_stale_filtered(self):
        """Records from dynamic-prompt tasks should pass Gate 6 regardless of hash value."""
        # Create a temp ULTS spec with dynamic hash
        specs_dir = os.path.join(self.tmpdir, "specs")
        os.makedirs(specs_dir)
        spec = {
            "ults_version": "1.0.0",
            "task": {"name": "hidden.reclassify"},
            "prompt": {"system_prompt_hash": "dynamic", "dynamic_prompt": True},
        }
        with open(os.path.join(specs_dir, "hidden_reclassify.ults.json"), "w") as f:
            json.dump(spec, f)
        # Also add a static spec so known_hashes is non-empty
        spec2 = {
            "ults_version": "1.0.0",
            "task": {"name": "retrieval.rerank_cross"},
            "prompt": {"system_prompt_hash": "abc123"},
        }
        with open(os.path.join(specs_dir, "rerank.ults.json"), "w") as f:
            json.dump(spec2, f)

        # Record with an arbitrary hash (would be stale for non-dynamic tasks)
        records = [
            _valid_record(
                task_name="hidden.reclassify",
                system_prompt_hash="some_arbitrary_hash_value",
                user_prompt="Classify this code snippet for me",
            ),
        ]
        _write_jsonl(records, self.input_path)
        report = run_filter(self.input_path, self.output_path, ults_dir=specs_dir)
        self.assertEqual(report["output_rows"], 1)
        self.assertEqual(report["excluded"]["stale_prompt_hash"], 0)

    def test_multi_hash_records_pass_gate6(self):
        """Records from multi-hash tasks should pass if hash matches any in the array."""
        specs_dir = os.path.join(self.tmpdir, "specs")
        os.makedirs(specs_dir)
        spec = {
            "ults_version": "1.0.0",
            "task": {"name": "hidden.name_emergence"},
            "prompt": {"system_prompt_hash": ["aaa111" + "a" * 58, "bbb222" + "b" * 58]},
        }
        with open(os.path.join(specs_dir, "emergence.ults.json"), "w") as f:
            json.dump(spec, f)

        records = [
            _valid_record(
                task_name="hidden.name_emergence",
                system_prompt_hash="bbb222" + "b" * 58,
                user_prompt="Name this emergent concept",
            ),
        ]
        _write_jsonl(records, self.input_path)
        report = run_filter(self.input_path, self.output_path, ults_dir=specs_dir)
        self.assertEqual(report["output_rows"], 1)
        self.assertEqual(report["excluded"]["stale_prompt_hash"], 0)

    def test_adversarial_all_bad_records(self):
        """All records should be excluded."""
        records = [
            _valid_record(response=""),  # empty response
            _valid_record(trace_id="t2", error="timeout", user_prompt="q2"),  # error
            _valid_record(trace_id="t3", latency_ms=999_999, user_prompt="q3"),  # latency
            _valid_record(trace_id="t4", model_name="", user_prompt="q4"),  # unknown model
            _valid_record(trace_id="t5", user_prompt="sk-abc123def456ghi789jkl012mno345pqr"),  # PII
        ]
        _write_jsonl(records, self.input_path)
        report = run_filter(self.input_path, self.output_path)
        self.assertEqual(report["output_rows"], 0)
        total_excluded = sum(report["excluded"].values())
        self.assertEqual(total_excluded, 5)


class TestSpecDrivenGates(unittest.TestCase):
    """Test UAITS spec-driven gate overrides."""

    def setUp(self):
        self.tmpdir = tempfile.mkdtemp()
        self.input_path = os.path.join(self.tmpdir, "input.jsonl")
        self.output_path = os.path.join(self.tmpdir, "output.jsonl")

    def tearDown(self):
        import shutil
        shutil.rmtree(self.tmpdir, ignore_errors=True)

    def test_spec_driven_gates(self):
        """UAITS spec overrides min_response_length threshold."""
        spec_path = os.path.join(self.tmpdir, "spec.json")
        spec = {
            "uaits_version": "1.0.0",
            "application": {"name": "test"},
            "datasets": [{
                "name": "test_ds",
                "paradigm": "sft",
                "quality_gates": {
                    "min_response_length": 50,
                    "max_latency_ms": 30000,
                },
            }],
        }
        with open(spec_path, "w") as f:
            json.dump(spec, f)

        # Record with 20-char response: passes default (10) but fails spec (50)
        records = [_valid_record(response="A" * 20, user_prompt="unique query here")]
        _write_jsonl(records, self.input_path)

        report = run_filter(
            self.input_path, self.output_path,
            uaits_spec_path=spec_path,
        )
        self.assertEqual(report["output_rows"], 0)
        self.assertEqual(report["excluded"]["empty_response"], 1)

    def test_no_spec_backward_compat(self):
        """Omitting UAITS spec gives identical default behavior."""
        records = [
            _valid_record(trace_id=f"t{i}", user_prompt=f"Query {i}")
            for i in range(5)
        ]
        _write_jsonl(records, self.input_path)

        report_no_spec = run_filter(self.input_path, self.output_path)

        output2 = os.path.join(self.tmpdir, "output2.jsonl")
        report_with_none = run_filter(
            self.input_path, output2, uaits_spec_path=None,
        )

        self.assertEqual(report_no_spec["output_rows"], report_with_none["output_rows"])
        self.assertEqual(report_no_spec["excluded"], report_with_none["excluded"])


class TestULTSSpecLoading(unittest.TestCase):
    """Test ULTS spec file loading."""

    def test_load_from_real_specs_dir(self):
        specs_dir = os.path.join(
            os.path.dirname(__file__), "..", "..", "..", "docs", "tests", "ults", "specs"
        )
        if not os.path.isdir(specs_dir):
            self.skipTest("ULTS specs dir not found")
        specs = load_ults_specs(specs_dir)
        self.assertGreater(len(specs), 0)
        # ape.reflect should be present
        self.assertIn("ape.reflect", specs)
        self.assertIsNotNone(specs["ape.reflect"]["system_prompt_hash"])

    def test_dynamic_prompt_hash_not_in_known(self):
        """Tasks with dynamic prompts should not be filtered by stale hash."""
        specs_dir = os.path.join(
            os.path.dirname(__file__), "..", "..", "..", "docs", "tests", "ults", "specs"
        )
        if not os.path.isdir(specs_dir):
            self.skipTest("ULTS specs dir not found")
        specs = load_ults_specs(specs_dir)
        # hidden.reclassify has a dynamic prompt variant — the dynamic
        # sentinel must be present so stale-hash filtering skips it.
        # UXTS-CI-001: the spec evolved from scalar "dynamic" to an array
        # mixing a pinned hash with the sentinel; accept both forms (this
        # test had been failing silently — neural tests had zero CI).
        if "hidden.reclassify" in specs:
            h = specs["hidden.reclassify"]["system_prompt_hash"]
            if isinstance(h, list):
                self.assertIn("dynamic", h)
            else:
                self.assertEqual(h, "dynamic")

    def test_multi_hash_array_loaded(self):
        """Tasks with array-type system_prompt_hash should load correctly."""
        specs_dir = os.path.join(
            os.path.dirname(__file__), "..", "..", "..", "docs", "tests", "ults", "specs"
        )
        if not os.path.isdir(specs_dir):
            self.skipTest("ULTS specs dir not found")
        specs = load_ults_specs(specs_dir)
        if "hidden.name_emergence" in specs:
            h = specs["hidden.name_emergence"]["system_prompt_hash"]
            self.assertIsInstance(h, list)
            self.assertEqual(len(h), 2)

    def test_nonexistent_dir_returns_empty(self):
        specs = load_ults_specs("/nonexistent/path")
        self.assertEqual(len(specs), 0)

    def test_none_dir_returns_empty(self):
        specs = load_ults_specs(None)
        self.assertEqual(len(specs), 0)


if __name__ == "__main__":
    unittest.main()
