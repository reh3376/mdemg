#!/usr/bin/env python3
"""Tests for the DPO preference pair builder."""

import json
import os
import tempfile
import unittest

from training.dpo_builder import (
    build_interactions_index,
    build_preference_pairs,
    load_constraint_outcomes,
    run_dpo_builder,
)


def _outcome(guidance_id="g1", outcome_type="followed", **overrides):
    """Build a constraint outcome record."""
    base = {
        "time": "2026-04-01T12:00:00Z",
        "space_id": "test-space",
        "constraint_id": "c1",
        "constraint_code": "TEST_CONSTRAINT",
        "guidance_id": guidance_id,
        "session_id": "s1",
        "outcome_type": outcome_type,
        "similarity": 0.85,
        "guidance_type": "constraint",
        "instance_id": "test-01",
    }
    base.update(overrides)
    return base


def _interaction(guidance_id="g1", response="A valid response.", **overrides):
    """Build an llm_interactions record."""
    base = {
        "time": "2026-04-01T12:00:00Z",
        "trace_id": "t1",
        "task_name": "jiminy.synthesize",
        "space_id": "test-space",
        "session_id": "s1",
        "system_prompt": "You are a helpful assistant.",
        "user_prompt": "What is the guidance?",
        "response": response,
        "latency_ms": 500,
        "tokens_in": 100,
        "tokens_out": 50,
        "model_name": "gpt-5.4",
        "provider": "openai",
        "error": "",
        "guidance_id": guidance_id,
    }
    base.update(overrides)
    return base


def _write_jsonl(path, records):
    """Write records to a JSONL file."""
    with open(path, "w") as f:
        for r in records:
            f.write(json.dumps(r) + "\n")


class TestDPOBuilder(unittest.TestCase):

    def test_basic_pair_construction(self):
        """1 followed + 1 contradicted with matching interactions -> 1 pair."""
        outcomes = [
            _outcome(guidance_id="g1", outcome_type="followed", similarity=0.9),
            _outcome(guidance_id="g1", outcome_type="contradicted", similarity=0.1),
        ]
        index = {
            "g1": [
                _interaction(guidance_id="g1", response="Chosen response text here."),
                _interaction(
                    guidance_id="g1",
                    response="Rejected response different.",
                    trace_id="t2",
                ),
            ],
        }
        pairs, report = build_preference_pairs(outcomes, index)
        self.assertEqual(len(pairs), 1)
        self.assertEqual(report["pairs_built"], 1)
        self.assertEqual(pairs[0]["chosen"], "Chosen response text here.")
        self.assertEqual(pairs[0]["rejected"], "Rejected response different.")

    def test_multiple_pairs(self):
        """Multiple guidance_ids -> multiple pairs."""
        outcomes = [
            _outcome(guidance_id="g1", outcome_type="followed"),
            _outcome(guidance_id="g1", outcome_type="contradicted"),
            _outcome(guidance_id="g2", outcome_type="followed"),
            _outcome(guidance_id="g2", outcome_type="contradicted"),
        ]
        index = {
            "g1": [
                _interaction(guidance_id="g1", response="Chosen A"),
                _interaction(guidance_id="g1", response="Rejected A", trace_id="t2"),
            ],
            "g2": [
                _interaction(guidance_id="g2", response="Chosen B"),
                _interaction(guidance_id="g2", response="Rejected B", trace_id="t3"),
            ],
        }
        pairs, report = build_preference_pairs(outcomes, index)
        self.assertEqual(len(pairs), 2)
        self.assertEqual(report["pairs_built"], 2)

    def test_no_contradicted_skips(self):
        """Followed-only guidance_id with single interaction -> no pair."""
        outcomes = [
            _outcome(guidance_id="g1", outcome_type="followed"),
        ]
        index = {
            "g1": [_interaction(guidance_id="g1")],
        }
        pairs, report = build_preference_pairs(outcomes, index)
        self.assertEqual(len(pairs), 0)
        self.assertEqual(report["skipped_no_pair"], 1)

    def test_no_followed_skips(self):
        """Contradicted-only guidance_id -> no pair."""
        outcomes = [
            _outcome(guidance_id="g1", outcome_type="contradicted"),
        ]
        index = {
            "g1": [_interaction(guidance_id="g1")],
        }
        pairs, report = build_preference_pairs(outcomes, index)
        self.assertEqual(len(pairs), 0)
        self.assertEqual(report["skipped_no_pair"], 1)

    def test_privacy_filter_applied(self):
        """PII in interaction -> pair excluded."""
        outcomes = [
            _outcome(guidance_id="g1", outcome_type="followed"),
            _outcome(guidance_id="g1", outcome_type="contradicted"),
        ]
        index = {
            "g1": [
                _interaction(
                    guidance_id="g1",
                    response="Secret: sk-abc123def456ghi789jkl012mno345pqr678stu901vwx",
                ),
                _interaction(guidance_id="g1", response="Rejected.", trace_id="t2"),
            ],
        }
        pairs, report = build_preference_pairs(outcomes, index)
        self.assertEqual(len(pairs), 0)
        self.assertEqual(report["skipped_privacy"], 1)

    def test_output_format_valid(self):
        """Verify DPO output has required keys."""
        outcomes = [
            _outcome(guidance_id="g1", outcome_type="followed"),
            _outcome(guidance_id="g1", outcome_type="contradicted"),
        ]
        index = {
            "g1": [
                _interaction(guidance_id="g1", response="Good response here."),
                _interaction(
                    guidance_id="g1",
                    response="Bad response here instead.",
                    trace_id="t2",
                ),
            ],
        }
        pairs, _ = build_preference_pairs(outcomes, index)
        self.assertEqual(len(pairs), 1)
        pair = pairs[0]
        self.assertIn("prompt", pair)
        self.assertIn("chosen", pair)
        self.assertIn("rejected", pair)
        self.assertIn("metadata", pair)
        meta = pair["metadata"]
        self.assertIn("guidance_id", meta)
        self.assertIn("constraint_id", meta)
        self.assertIn("task_name", meta)

    def test_empty_inputs(self):
        """Empty outcomes -> empty output, no crash."""
        pairs, report = build_preference_pairs([], {})
        self.assertEqual(len(pairs), 0)
        self.assertEqual(report["pairs_built"], 0)
        self.assertEqual(report["total_outcomes"], 0)

    def test_spec_driven_filter(self):
        """Spec with restricted outcome_types still works."""
        spec = {
            "datasets": [{
                "paradigm": "dpo",
                "source": {
                    "primary_table": "constraint_outcomes",
                    "filter": {
                        "outcome_types": ["followed", "contradicted"],
                    },
                },
            }],
        }
        outcomes = [
            _outcome(guidance_id="g1", outcome_type="followed"),
            _outcome(guidance_id="g1", outcome_type="contradicted"),
        ]
        index = {
            "g1": [
                _interaction(guidance_id="g1", response="Chosen response."),
                _interaction(
                    guidance_id="g1",
                    response="Rejected response.",
                    trace_id="t2",
                ),
            ],
        }
        pairs, report = build_preference_pairs(outcomes, index, spec=spec)
        self.assertEqual(len(pairs), 1)

    def test_dry_run(self):
        """Dry run produces report but no output file."""
        with tempfile.TemporaryDirectory() as tmpdir:
            interactions_path = os.path.join(tmpdir, "interactions.jsonl")
            outcomes_path = os.path.join(tmpdir, "outcomes.jsonl")
            output_path = os.path.join(tmpdir, "output.jsonl")

            _write_jsonl(interactions_path, [
                _interaction(guidance_id="g1", response="Chosen text."),
                _interaction(
                    guidance_id="g1",
                    response="Rejected text.",
                    trace_id="t2",
                ),
            ])
            _write_jsonl(outcomes_path, [
                _outcome(guidance_id="g1", outcome_type="followed"),
                _outcome(guidance_id="g1", outcome_type="contradicted"),
            ])

            report = run_dpo_builder(
                interactions_path, outcomes_path, output_path,
                dry_run=True,
            )
            self.assertFalse(os.path.exists(output_path))
            self.assertEqual(report["pairs_built"], 1)


class TestDPOBuilderIO(unittest.TestCase):
    """Tests for JSONL I/O functions."""

    def test_load_constraint_outcomes(self):
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".jsonl", delete=False,
        ) as f:
            f.write(json.dumps(_outcome()) + "\n")
            f.write(json.dumps(_outcome(guidance_id="g2")) + "\n")
            f.flush()
            records = load_constraint_outcomes(f.name)
        os.unlink(f.name)
        self.assertEqual(len(records), 2)

    def test_build_interactions_index(self):
        with tempfile.NamedTemporaryFile(
            mode="w", suffix=".jsonl", delete=False,
        ) as f:
            f.write(json.dumps(_interaction(guidance_id="g1")) + "\n")
            f.write(json.dumps(_interaction(guidance_id="g1", trace_id="t2")) + "\n")
            f.write(json.dumps(_interaction(guidance_id="g2")) + "\n")
            f.flush()
            index = build_interactions_index(f.name)
        os.unlink(f.name)
        self.assertEqual(len(index["g1"]), 2)
        self.assertEqual(len(index["g2"]), 1)

    def test_load_missing_file(self):
        """Missing file returns empty list."""
        records = load_constraint_outcomes("/nonexistent/file.jsonl")
        self.assertEqual(records, [])


if __name__ == "__main__":
    unittest.main()
