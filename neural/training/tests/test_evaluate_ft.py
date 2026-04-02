"""Tests for the post-training evaluation script."""

import json
import os
import tempfile

import pytest

from training.evaluate_ft import (
    check_json_valid,
    check_non_empty,
    check_required_keys,
    check_type_conformance,
    evaluate_response,
    evaluate_task,
    extract_prompts,
    extract_response,
    group_by_task,
    load_test_data,
    load_ults_specs,
)


# ── Schema check tests ──


class TestCheckJsonValid:
    def test_valid_object(self):
        assert check_json_valid('{"type": "architecture"}') == 1.0

    def test_valid_nested(self):
        assert check_json_valid('{"types": ["semantic"], "temporal": "none"}') == 1.0

    def test_invalid_json(self):
        assert check_json_valid("not json") == 0.0

    def test_json_array(self):
        assert check_json_valid("[1, 2, 3]") == 0.0

    def test_empty_string(self):
        assert check_json_valid("") == 0.0

    def test_none(self):
        assert check_json_valid(None) == 0.0


class TestCheckRequiredKeys:
    def test_all_present(self):
        schema = {"required": ["type", "summary"]}
        resp = '{"type": "arch", "summary": "test"}'
        assert check_required_keys(resp, schema) == 1.0

    def test_partial_present(self):
        schema = {"required": ["type", "summary", "confidence"]}
        resp = '{"type": "arch"}'
        assert check_required_keys(resp, schema) == pytest.approx(1 / 3)

    def test_none_present(self):
        schema = {"required": ["type"]}
        resp = '{"foo": "bar"}'
        assert check_required_keys(resp, schema) == 0.0

    def test_no_required(self):
        schema = {"properties": {"type": {"type": "string"}}}
        resp = '{"anything": true}'
        assert check_required_keys(resp, schema) == 1.0

    def test_invalid_json(self):
        schema = {"required": ["type"]}
        assert check_required_keys("not json", schema) == 0.0


class TestCheckTypeConformance:
    def test_correct_types(self):
        schema = {
            "properties": {
                "type": {"type": "string"},
                "confidence": {"type": "number"},
            }
        }
        resp = '{"type": "arch", "confidence": 0.9}'
        assert check_type_conformance(resp, schema) == 1.0

    def test_wrong_type(self):
        schema = {
            "properties": {
                "confidence": {"type": "number"},
            }
        }
        resp = '{"confidence": "high"}'
        assert check_type_conformance(resp, schema) == 0.0

    def test_array_type(self):
        schema = {
            "properties": {
                "types": {"type": "array"},
            }
        }
        resp = '{"types": ["semantic", "temporal"]}'
        assert check_type_conformance(resp, schema) == 1.0

    def test_no_properties(self):
        assert check_type_conformance('{"foo": 1}', {}) == 1.0

    def test_invalid_json(self):
        schema = {"properties": {"x": {"type": "string"}}}
        assert check_type_conformance("bad", schema) == 0.0


class TestCheckNonEmpty:
    def test_non_empty(self):
        assert check_non_empty("hello") == 1.0

    def test_empty(self):
        assert check_non_empty("") == 0.0

    def test_whitespace_only(self):
        assert check_non_empty("   ") == 0.0

    def test_none(self):
        assert check_non_empty(None) == 0.0


# ── Response evaluation tests ──


class TestEvaluateResponse:
    def _metrics(self, *entries):
        """Build quality_metrics list."""
        return [
            {"name": name, "weight": weight, "threshold": threshold}
            for name, weight, threshold in entries
        ]

    def test_perfect_structured_response(self):
        schema = {
            "type": "object",
            "required": ["type"],
            "properties": {"type": {"type": "string"}},
        }
        metrics = self._metrics(
            ("json_valid", 0.3, 1.0),
            ("accuracy", 0.4, 0.7),
            ("precision", 0.3, 0.6),
        )
        result = evaluate_response('{"type": "architecture"}', schema, metrics)

        assert result["weighted_score"] == 1.0
        assert result["all_thresholds_met"] is True
        assert result["metrics"]["json_valid"]["score"] == 1.0
        assert result["metrics"]["accuracy"]["score"] == 1.0

    def test_invalid_json_structured(self):
        schema = {"type": "object", "required": ["type"]}
        metrics = self._metrics(("json_valid", 1.0, 1.0),)
        result = evaluate_response("not json", schema, metrics)

        assert result["weighted_score"] == 0.0
        assert result["all_thresholds_met"] is False

    def test_string_output(self):
        schema = {"type": "string"}
        metrics = self._metrics(
            ("coherence", 0.5, 0.6),
            ("coverage", 0.5, 0.5),
        )
        result = evaluate_response("A clear summary of the code.", schema, metrics)

        assert result["weighted_score"] == 1.0
        assert result["all_thresholds_met"] is True

    def test_empty_string_output(self):
        schema = {"type": "string"}
        metrics = self._metrics(("coherence", 1.0, 0.6),)
        result = evaluate_response("", schema, metrics)

        assert result["weighted_score"] == 0.0
        assert result["all_thresholds_met"] is False

    def test_weighted_aggregate(self):
        schema = {"type": "object", "required": ["type", "summary"]}
        metrics = self._metrics(
            ("json_valid", 0.3, 1.0),
            ("accuracy", 0.7, 0.7),
        )
        # Valid JSON but missing 'summary' key (accuracy = 0.5)
        result = evaluate_response('{"type": "arch"}', schema, metrics)

        # json_valid=1.0*0.3 + accuracy=0.5*0.7 = 0.30 + 0.35 = 0.65
        assert result["weighted_score"] == pytest.approx(0.65, abs=0.01)

    def test_unknown_metric_defaults(self):
        schema = {"type": "object"}
        metrics = self._metrics(("unknown_metric", 1.0, 0.5),)
        # Unknown metric on object schema defaults to json_valid
        result = evaluate_response('{"foo": 1}', schema, metrics)
        assert result["metrics"]["unknown_metric"]["score"] == 1.0

    def test_empty_metrics_list(self):
        result = evaluate_response("anything", {}, [])
        assert result["weighted_score"] == 0.0
        assert result["all_thresholds_met"] is True  # vacuously true


# ── Data loading tests ──


class TestLoadTestData:
    def test_load_jsonl(self):
        with tempfile.NamedTemporaryFile(mode="w", suffix=".jsonl", delete=False) as f:
            f.write('{"task_name": "a", "response": "hello"}\n')
            f.write('{"task_name": "b", "response": "world"}\n')
            f.write("\n")  # blank line
            path = f.name

        try:
            records = load_test_data(path)
            assert len(records) == 2
            assert records[0]["task_name"] == "a"
        finally:
            os.unlink(path)


class TestGroupByTask:
    def test_grouping(self):
        records = [
            {"task_name": "a", "response": "1"},
            {"task_name": "b", "response": "2"},
            {"task_name": "a", "response": "3"},
        ]
        groups = group_by_task(records)
        assert len(groups) == 2
        assert len(groups["a"]) == 2
        assert len(groups["b"]) == 1

    def test_missing_task_name(self):
        records = [{"response": "no task"}]
        groups = group_by_task(records)
        assert "unknown" in groups


# ── Extract helpers tests ──


class TestExtractResponse:
    def test_raw_format(self):
        record = {"response": "the answer"}
        assert extract_response(record) == "the answer"

    def test_chat_format(self):
        record = {
            "messages": [
                {"role": "system", "content": "sys"},
                {"role": "user", "content": "q"},
                {"role": "assistant", "content": "a"},
            ]
        }
        assert extract_response(record) == "a"

    def test_empty_record(self):
        assert extract_response({}) == ""

    def test_null_response(self):
        assert extract_response({"response": None}) == ""


class TestExtractPrompts:
    def test_raw_format(self):
        record = {"system_prompt": "sys", "user_prompt": "usr"}
        sys, usr = extract_prompts(record)
        assert sys == "sys"
        assert usr == "usr"

    def test_chat_format(self):
        record = {
            "messages": [
                {"role": "system", "content": "sys"},
                {"role": "user", "content": "usr"},
                {"role": "assistant", "content": "a"},
            ]
        }
        sys, usr = extract_prompts(record)
        assert sys == "sys"
        assert usr == "usr"


# ── ULTS spec loading ──


class TestLoadUltsSpecs:
    def test_loads_specs(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            spec = {
                "task": {"name": "test.task"},
                "output_schema": {"type": "string"},
                "quality_metrics": [],
            }
            with open(os.path.join(tmpdir, "test_task.ults.json"), "w") as f:
                json.dump(spec, f)

            specs = load_ults_specs(tmpdir)
            assert "test.task" in specs
            assert specs["test.task"]["output_schema"]["type"] == "string"

    def test_skips_invalid(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            with open(os.path.join(tmpdir, "bad.ults.json"), "w") as f:
                f.write("not valid json")

            specs = load_ults_specs(tmpdir)
            assert len(specs) == 0

    def test_empty_dir(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            specs = load_ults_specs(tmpdir)
            assert len(specs) == 0


# ── Task-level evaluation ──


class TestEvaluateTask:
    def test_structured_task(self):
        spec = {
            "output_schema": {
                "type": "object",
                "required": ["type"],
                "properties": {"type": {"type": "string"}},
            },
            "quality_metrics": [
                {"name": "json_valid", "weight": 0.5, "threshold": 1.0},
                {"name": "accuracy", "weight": 0.5, "threshold": 0.7},
            ],
            "performance": {"max_tokens": 500, "latency_budget_ms": 3000},
        }
        records = [
            {"response": '{"type": "architecture"}', "latency_ms": 200},
            {"response": '{"type": "pattern"}', "latency_ms": 300},
        ]

        result = evaluate_task("test.classify", records, spec)

        assert result["task"] == "test.classify"
        assert result["count"] == 2
        assert result["weighted_score"] == 1.0
        assert result["status"] == "evaluated"
        assert result["latency"]["mean_ms"] == 250

    def test_string_task(self):
        spec = {
            "output_schema": {"type": "string"},
            "quality_metrics": [
                {"name": "coherence", "weight": 1.0, "threshold": 0.6},
            ],
            "performance": {},
        }
        records = [
            {"response": "A clear explanation."},
            {"response": "Another good answer."},
        ]

        result = evaluate_task("test.summarize", records, spec)
        assert result["weighted_score"] == 1.0

    def test_no_records(self):
        spec = {
            "output_schema": {"type": "string"},
            "quality_metrics": [],
            "performance": {},
        }
        result = evaluate_task("test.empty", [], spec)
        assert result["status"] == "no_data"
        assert result["count"] == 0

    def test_mixed_quality(self):
        spec = {
            "output_schema": {
                "type": "object",
                "required": ["type"],
            },
            "quality_metrics": [
                {"name": "json_valid", "weight": 1.0, "threshold": 1.0},
            ],
            "performance": {},
        }
        records = [
            {"response": '{"type": "ok"}'},
            {"response": "not json"},
        ]

        result = evaluate_task("test.mixed", records, spec)
        assert result["weighted_score"] == pytest.approx(0.5)
        assert result["threshold_pass_rate"] == pytest.approx(0.5)

    def test_chat_format_records(self):
        spec = {
            "output_schema": {"type": "string"},
            "quality_metrics": [
                {"name": "coherence", "weight": 1.0, "threshold": 0.5},
            ],
            "performance": {},
        }
        records = [
            {
                "task_name": "test.chat",
                "messages": [
                    {"role": "system", "content": "sys"},
                    {"role": "user", "content": "q"},
                    {"role": "assistant", "content": "A good answer."},
                ],
            },
        ]

        result = evaluate_task("test.chat", records, spec)
        assert result["weighted_score"] == 1.0
