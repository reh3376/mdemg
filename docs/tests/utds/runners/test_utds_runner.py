#!/usr/bin/env python3
"""
UTDS Runner Unit Tests

Tests the UTDS validation runner: schema validation of fixture specs,
archive validation, rejection of invalid manifests, and privacy hard gate.
"""

import hashlib
import io
import json
import os
import tarfile
import tempfile
import unittest
from pathlib import Path

# Import the runner
sys_path = os.path.join(os.path.dirname(os.path.abspath(__file__)))
import sys
sys.path.insert(0, sys_path)
from utds_runner import (
    load_utds_schema,
    validate_manifest_against_schema,
    validate_spec_manifest,
    validate_archive,
    self_check,
    UTDS_VERSION,
)


SPECS_DIR = os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', 'specs')


def _build_valid_manifest(tables=None, privacy_violations=0):
    """Build a minimal valid manifest dict for testing."""
    manifest = {
        "utds_version": "1.0.0",
        "export_id": "exp-test-instance-20260401-120000",
        "instance_id": "test-instance",
        "space_id": "test-space",
        "mdemg_version": "v0.4.1",
        "mdemg_commit": "abcdef1",
        "schema_version": 7,
        "exported_at": "2026-04-01T12:00:00Z",
        "data_range": {"from": "2026-04-01T00:00:00Z", "to": "2026-04-01T12:00:00Z"},
        "tables": tables or {
            "llm_interactions": {
                "row_count": 0,
                "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
                "columns": ["time", "trace_id", "task_name"],
            }
        },
        "quality_summary": {
            "llm_error_rate": 0.0,
            "llm_empty_system_prompt": 0,
            "llm_empty_response": 0,
            "privacy_scrub_violations": privacy_violations,
        },
    }
    return manifest


def _create_test_archive(manifest, jsonl_data=None):
    """Create a temporary .tar.gz archive with manifest and optional JSONL files.

    Args:
        manifest: Dict to serialize as manifest.json.
        jsonl_data: Dict of {table_name: list_of_dicts} for JSONL content.
                    If None, creates empty files for each table in manifest.

    Returns:
        Path to temporary .tar.gz file.
    """
    tmp = tempfile.NamedTemporaryFile(suffix=".tar.gz", delete=False)
    tmp_path = tmp.name
    tmp.close()

    with tarfile.open(tmp_path, "w:gz") as tf:
        # Write manifest.json
        manifest_bytes = json.dumps(manifest, indent=2).encode("utf-8")
        info = tarfile.TarInfo(name="manifest.json")
        info.size = len(manifest_bytes)
        tf.addfile(info, io.BytesIO(manifest_bytes))

        # Write JSONL files
        tables = manifest.get("tables", {})
        for tname in tables:
            if jsonl_data and tname in jsonl_data:
                lines = [json.dumps(row) for row in jsonl_data[tname]]
                content = ("\n".join(lines) + "\n").encode("utf-8") if lines else b""
            else:
                content = b""

            info = tarfile.TarInfo(name=f"{tname}.jsonl")
            info.size = len(content)
            tf.addfile(info, io.BytesIO(content))

    return tmp_path


class TestSchemaLoading(unittest.TestCase):
    """Test that the UTDS schema loads correctly."""

    def test_schema_loads(self):
        schema = load_utds_schema()
        self.assertIn("$schema", schema)
        self.assertIn("required", schema)
        self.assertIn("properties", schema)

    def test_schema_has_table_meta_def(self):
        schema = load_utds_schema()
        self.assertIn("$defs", schema)
        self.assertIn("table_meta", schema["$defs"])

    def test_schema_privacy_hard_gate(self):
        schema = load_utds_schema()
        psv = schema["properties"]["quality_summary"]["properties"]["privacy_scrub_violations"]
        self.assertEqual(psv["maximum"], 0, "Privacy scrub violations must have maximum: 0")


class TestManifestValidation(unittest.TestCase):
    """Test manifest validation against the UTDS schema."""

    def setUp(self):
        self.schema = load_utds_schema()

    def test_valid_manifest_passes(self):
        manifest = _build_valid_manifest()
        results = validate_manifest_against_schema(manifest, self.schema)
        failures = [r for r in results if not r.passed]
        self.assertEqual(len(failures), 0, f"Valid manifest should pass. Failures: {failures}")

    def test_missing_required_field_fails(self):
        manifest = _build_valid_manifest()
        del manifest["export_id"]
        results = validate_manifest_against_schema(manifest, self.schema)
        failed_names = {r.name for r in results if not r.passed}
        self.assertIn("required_fields", failed_names)

    def test_invalid_export_id_pattern_fails(self):
        manifest = _build_valid_manifest()
        manifest["export_id"] = "bad-id-format"
        results = validate_manifest_against_schema(manifest, self.schema)
        failed_names = {r.name for r in results if not r.passed}
        self.assertIn("export_id_pattern", failed_names)

    def test_schema_version_too_low_fails(self):
        manifest = _build_valid_manifest()
        manifest["schema_version"] = 6
        results = validate_manifest_against_schema(manifest, self.schema)
        failed_names = {r.name for r in results if not r.passed}
        self.assertIn("schema_version", failed_names)

    def test_privacy_violations_nonzero_fails(self):
        manifest = _build_valid_manifest(privacy_violations=3)
        results = validate_manifest_against_schema(manifest, self.schema)
        failed_names = {r.name for r in results if not r.passed}
        self.assertIn("privacy_hard_gate", failed_names)

    def test_missing_llm_interactions_fails(self):
        manifest = _build_valid_manifest(tables={
            "retrieval_events": {
                "row_count": 10,
                "sha256": "a" * 64,
                "columns": ["time", "event_id"],
            }
        })
        results = validate_manifest_against_schema(manifest, self.schema)
        failed_names = {r.name for r in results if not r.passed}
        self.assertIn("llm_interactions_required", failed_names)

    def test_unknown_table_fails(self):
        manifest = _build_valid_manifest()
        manifest["tables"]["unknown_table"] = {
            "row_count": 1, "sha256": "a" * 64, "columns": ["x"],
        }
        results = validate_manifest_against_schema(manifest, self.schema)
        failed_names = {r.name for r in results if not r.passed}
        self.assertIn("no_extra_tables", failed_names)

    def test_invalid_sha256_format_fails(self):
        manifest = _build_valid_manifest(tables={
            "llm_interactions": {
                "row_count": 1,
                "sha256": "not-a-valid-hash",
                "columns": ["time"],
            }
        })
        results = validate_manifest_against_schema(manifest, self.schema)
        failed_names = {r.name for r in results if not r.passed}
        self.assertIn("llm_interactions_sha256_format", failed_names)

    def test_unknown_top_level_field_fails(self):
        manifest = _build_valid_manifest()
        manifest["bogus_field"] = "should not be here"
        results = validate_manifest_against_schema(manifest, self.schema)
        failed_names = {r.name for r in results if not r.passed}
        self.assertIn("no_extra_fields", failed_names)

    def test_empty_columns_fails(self):
        manifest = _build_valid_manifest(tables={
            "llm_interactions": {
                "row_count": 1,
                "sha256": "a" * 64,
                "columns": [],
            }
        })
        results = validate_manifest_against_schema(manifest, self.schema)
        failed_names = {r.name for r in results if not r.passed}
        self.assertIn("llm_interactions_columns", failed_names)


class TestFixtureSpecs(unittest.TestCase):
    """Test that all fixture spec files validate against the schema."""

    def test_standard_spec_validates(self):
        result = validate_spec_manifest(os.path.join(SPECS_DIR, "training_export_standard.utds.json"))
        self.assertTrue(result.passed, f"Standard spec failed: {[c.message for c in result.checks if not c.passed]}")

    def test_llm_only_spec_validates(self):
        result = validate_spec_manifest(os.path.join(SPECS_DIR, "training_export_llm_only.utds.json"))
        self.assertTrue(result.passed, f"LLM-only spec failed: {[c.message for c in result.checks if not c.passed]}")

    def test_minimal_spec_validates(self):
        result = validate_spec_manifest(os.path.join(SPECS_DIR, "training_export_minimal.utds.json"))
        self.assertTrue(result.passed, f"Minimal spec failed: {[c.message for c in result.checks if not c.passed]}")


class TestArchiveValidation(unittest.TestCase):
    """Test archive validation with real .tar.gz files."""

    def test_valid_archive_passes(self):
        rows = [{"time": "2026-04-01T00:00:00Z", "trace_id": "t1", "task_name": "test.task"}]
        content = (json.dumps(rows[0]) + "\n").encode("utf-8")
        sha = hashlib.sha256(content).hexdigest()

        manifest = _build_valid_manifest(tables={
            "llm_interactions": {
                "row_count": 1,
                "sha256": sha,
                "columns": ["time", "trace_id", "task_name"],
            }
        })

        path = _create_test_archive(manifest, {"llm_interactions": rows})
        try:
            result = validate_archive(path)
            self.assertTrue(result.passed, f"Valid archive failed: {[c.message for c in result.checks if not c.passed]}")
        finally:
            os.unlink(path)

    def test_sha256_mismatch_detected(self):
        rows = [{"time": "2026-04-01T00:00:00Z", "trace_id": "t1", "task_name": "test.task"}]
        manifest = _build_valid_manifest(tables={
            "llm_interactions": {
                "row_count": 1,
                "sha256": "0" * 64,  # Wrong hash
                "columns": ["time", "trace_id", "task_name"],
            }
        })

        path = _create_test_archive(manifest, {"llm_interactions": rows})
        try:
            result = validate_archive(path)
            failed_names = {c.name for c in result.checks if not c.passed}
            self.assertIn("llm_interactions_sha256_match", failed_names)
        finally:
            os.unlink(path)

    def test_row_count_mismatch_detected(self):
        rows = [{"time": "2026-04-01T00:00:00Z", "trace_id": "t1"}]
        content = (json.dumps(rows[0]) + "\n").encode("utf-8")
        sha = hashlib.sha256(content).hexdigest()

        manifest = _build_valid_manifest(tables={
            "llm_interactions": {
                "row_count": 99,  # Wrong count
                "sha256": sha,
                "columns": ["time", "trace_id"],
            }
        })

        path = _create_test_archive(manifest, {"llm_interactions": rows})
        try:
            result = validate_archive(path)
            failed_names = {c.name for c in result.checks if not c.passed}
            self.assertIn("llm_interactions_row_count_match", failed_names)
        finally:
            os.unlink(path)

    def test_missing_jsonl_file_detected(self):
        manifest = _build_valid_manifest()

        # Create archive with manifest only (no JSONL)
        tmp = tempfile.NamedTemporaryFile(suffix=".tar.gz", delete=False)
        tmp_path = tmp.name
        tmp.close()

        with tarfile.open(tmp_path, "w:gz") as tf:
            manifest_bytes = json.dumps(manifest).encode("utf-8")
            info = tarfile.TarInfo(name="manifest.json")
            info.size = len(manifest_bytes)
            tf.addfile(info, io.BytesIO(manifest_bytes))

        try:
            result = validate_archive(tmp_path)
            failed_names = {c.name for c in result.checks if not c.passed}
            self.assertIn("llm_interactions_file_exists", failed_names)
        finally:
            os.unlink(tmp_path)

    def test_privacy_violation_blocks_archive(self):
        manifest = _build_valid_manifest(privacy_violations=1)
        content = b""
        sha = hashlib.sha256(content).hexdigest()
        manifest["tables"]["llm_interactions"]["sha256"] = sha
        manifest["tables"]["llm_interactions"]["row_count"] = 0

        path = _create_test_archive(manifest, {"llm_interactions": []})
        try:
            result = validate_archive(path)
            failed_names = {c.name for c in result.checks if not c.passed}
            self.assertIn("privacy_hard_gate", failed_names)
        finally:
            os.unlink(path)

    def test_strict_mode_validates_fields(self):
        rows = [
            {"time": "2026-04-01T00:00:00Z", "trace_id": "t1", "task_name": "test.task",
             "space_id": "s", "session_id": "ss", "system_prompt": "sp", "user_prompt": "up",
             "response": "r", "think_content": None, "think_mode": False,
             "latency_ms": 100, "tokens_in": 10, "tokens_out": 20,
             "model_name": "gpt-4.1", "provider": "openai", "error": "",
             "quality": None, "quality_source": None, "used_for_train": False,
             "dataset_ver": None, "guidance_id": None, "source_path": None,
             "retrieval_node_ids": None, "retrieval_scores": None,
             "oracle_node_id": None, "system_prompt_hash": "a" * 64}
        ]
        content = (json.dumps(rows[0]) + "\n").encode("utf-8")
        sha = hashlib.sha256(content).hexdigest()

        manifest = _build_valid_manifest(tables={
            "llm_interactions": {
                "row_count": 1,
                "sha256": sha,
                "columns": list(rows[0].keys()),
            }
        })

        path = _create_test_archive(manifest, {"llm_interactions": rows})
        try:
            result = validate_archive(path, strict=True)
            strict_failures = [c for c in result.checks if not c.passed and "jsonl" in c.name]
            self.assertEqual(len(strict_failures), 0,
                             f"Strict validation failures: {[c.message for c in strict_failures]}")
        finally:
            os.unlink(path)


class TestSelfCheck(unittest.TestCase):
    """Test the self-check command."""

    def test_self_check_passes(self):
        results = self_check()
        self.assertTrue(len(results) > 0, "Self-check should produce results")
        for result in results:
            self.assertTrue(result.passed,
                            f"{result.target_name} failed: {[c.message for c in result.checks if not c.passed]}")


if __name__ == "__main__":
    unittest.main()
