"""Tests for teacher distillation script."""

import os
import tempfile


from training.teacher_distill import (
    extract_system_prompt,
    generate_inputs,
    validate_output,
)


# ── extract_system_prompt ──


class TestExtractSystemPrompt:
    def test_backtick_string(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            go_file = os.path.join(tmpdir, "test.go")
            with open(go_file, "w") as f:
                f.write('package test\n\n')
                f.write('const systemPrompt = `You are a classifier.`\n')

            prompt = extract_system_prompt(f"{go_file}:3", "/")
            assert prompt == "You are a classifier."

    def test_double_quoted_string(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            go_file = os.path.join(tmpdir, "test.go")
            with open(go_file, "w") as f:
                f.write('package test\n\n')
                f.write('const prompt = "You are a helper."\n')

            prompt = extract_system_prompt(f"{go_file}:3", "/")
            assert prompt == "You are a helper."

    def test_array_source_ref(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            go_file = os.path.join(tmpdir, "test.go")
            with open(go_file, "w") as f:
                f.write('package test\n\n')
                f.write('const prompt1 = `First prompt.`\n')
                f.write('const prompt2 = `Second prompt.`\n')

            # Array ref — should use first element
            prompt = extract_system_prompt(
                [f"{go_file}:3", f"{go_file}:4"], "/"
            )
            assert prompt == "First prompt."

    def test_missing_file(self):
        prompt = extract_system_prompt("/nonexistent/file.go:1", "/")
        assert prompt is None

    def test_invalid_ref(self):
        prompt = extract_system_prompt("no-line-number", "/")
        assert prompt is None

    def test_line_out_of_range(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            go_file = os.path.join(tmpdir, "test.go")
            with open(go_file, "w") as f:
                f.write("package test\n")

            prompt = extract_system_prompt(f"{go_file}:999", "/")
            assert prompt is None


# ── generate_inputs ──


class TestGenerateInputs:
    def test_known_task(self):
        inputs = generate_inputs("consulting.classify", 5)
        assert len(inputs) == 5
        assert all(isinstance(i, str) for i in inputs)
        assert all(len(i) > 0 for i in inputs)

    def test_unknown_task(self):
        inputs = generate_inputs("nonexistent.task", 5)
        assert inputs == []

    def test_deterministic_with_seed(self):
        inputs_a = generate_inputs("consulting.classify", 3, seed=42)
        inputs_b = generate_inputs("consulting.classify", 3, seed=42)
        assert inputs_a == inputs_b

    def test_different_seeds(self):
        inputs_a = generate_inputs("consulting.classify", 10, seed=1)
        inputs_b = generate_inputs("consulting.classify", 10, seed=2)
        # At least some should differ
        assert inputs_a != inputs_b

    def test_all_tasks_have_templates(self):
        from training.teacher_distill import INPUT_TEMPLATES
        # All 16 ULTS tasks should have templates
        assert len(INPUT_TEMPLATES) >= 16


# ── validate_output ──


class TestValidateOutput:
    def test_valid_object(self):
        schema = {"type": "object", "required": ["type"]}
        assert validate_output('{"type": "architecture"}', schema) is True

    def test_invalid_json(self):
        schema = {"type": "object", "required": ["type"]}
        assert validate_output("not json", schema) is False

    def test_missing_required(self):
        schema = {"type": "object", "required": ["type", "summary"]}
        assert validate_output('{"type": "arch"}', schema) is False

    def test_valid_string(self):
        schema = {"type": "string"}
        assert validate_output("A clear explanation.", schema) is True

    def test_empty_string(self):
        schema = {"type": "string"}
        assert validate_output("", schema) is False

    def test_no_required_keys(self):
        schema = {"type": "object"}
        assert validate_output('{"anything": true}', schema) is True

    def test_array_not_object(self):
        schema = {"type": "object", "required": ["type"]}
        assert validate_output("[1, 2, 3]", schema) is False
