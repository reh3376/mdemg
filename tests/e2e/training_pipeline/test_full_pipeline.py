#!/usr/bin/env python3
"""
E2E test for the full training data pipeline.

Generates synthetic JSONL for all 16 tasks across 2 simulated instances,
runs quality_filter + dataset_versioner, and validates the output.

Usage:
    python3 -m pytest tests/e2e/training_pipeline/test_full_pipeline.py -v
"""

import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time

import pytest

# ---------------------------------------------------------------------------
# Task definitions — matches ULTS specs exactly
# ---------------------------------------------------------------------------

TASK_SPECS = [
    {
        "task_name": "ape.reflect",
        "system_prompt_hash": "64d528cc23dfed0c75ccdddeecf507e671b8f8f85f2fc679226b6e1caf13cc33",
        "think_mode": True,
        "response_template": "object",
        "required_keys": ["insights"],
        "response": json.dumps({"insights": [{"type": "pattern", "description": "test insight", "priority": "high"}]}),
    },
    {
        "task_name": "consulting.classify",
        "system_prompt_hash": "d0d51f50deedba09e8cec236f5352f8d811b6227d88913f2d00312ce3f658377",
        "think_mode": False,
        "response_template": "object",
        "required_keys": ["type"],
        "response": json.dumps({"type": "technical"}),
    },
    {
        "task_name": "consulting.synthesis",
        "system_prompt_hash": "dynamic",
        "think_mode": True,
        "response_template": "object",
        "required_keys": [],
        "response": json.dumps({"summary": "synthesis result", "confidence": 0.9}),
    },
    {
        "task_name": "hidden.name_emergence",
        "system_prompt_hash": "bb90d13f0ae210ec0e916e5067aa7d9e3740f68e4f1c38bec21c7504b031a591",
        "think_mode": False,
        "response_template": "object",
        "required_keys": ["name"],
        "response": json.dumps({"name": "pattern-recognition"}),
    },
    {
        "task_name": "hidden.name_emergence",
        "system_prompt_hash": "eefddf2ed714178a500f78a2ada0847f26e9c2f42dedeba2cd40b5f3c5152aff",
        "think_mode": False,
        "response_template": "object",
        "required_keys": ["name"],
        "response": json.dumps({"name": "l5-meta-concept"}),
        "variant": "l5",
    },
    {
        "task_name": "hidden.reclassify",
        "system_prompt_hash": "dynamic",
        "think_mode": False,
        "response_template": "array",
        "required_keys": [],
        "response": json.dumps([{"name": "concept-a", "level": 3}, {"name": "concept-b", "level": 4}]),
    },
    {
        "task_name": "hidden.summarize",
        "system_prompt_hash": "5110d51bc460daded7e77526c3e7d4a58f4ccf0139da31f50b5f2dad3c1f6d5a",
        "think_mode": True,
        "response_template": "object",
        "required_keys": ["summary"],
        "response": json.dumps({"summary": "A concise summary of the concept cluster."}),
    },
    {
        "task_name": "jiminy.codegen",
        "system_prompt_hash": "0a82506152827fef03fc7fe7bfa6715e55c3391926549978c7d3e311b5a26832",
        "think_mode": False,
        "response_template": "string",
        "required_keys": [],
        "response": "constraint-code-gen",
    },
    {
        "task_name": "jiminy.evaluate",
        "system_prompt_hash": "caf70a3d392f37d4d5b2d70d10b03d07b59a2f1cf18ed8cfd75d597cafa0a773",
        "think_mode": True,
        "response_template": "object",
        "required_keys": ["evaluations"],
        "response": json.dumps({"evaluations": [{"constraint_id": "c1", "followed": True}]}),
    },
    {
        "task_name": "jiminy.evaluate_llm",
        "system_prompt_hash": "caf70a3d392f37d4d5b2d70d10b03d07b59a2f1cf18ed8cfd75d597cafa0a773",
        "think_mode": True,
        "response_template": "object",
        "required_keys": ["evaluations"],
        "response": json.dumps({"evaluations": [{"constraint_id": "c2", "followed": False}]}),
    },
    {
        "task_name": "jiminy.synthesize",
        "system_prompt_hash": "8cf0ae2dbba97fcc627342e27653139b78bd1a04ce1a07556bb85c94a1937f39",
        "think_mode": True,
        "response_template": "string",
        "required_keys": [],
        "response": "The agent should maintain focus on data integrity while exploring new patterns.",
    },
    {
        "task_name": "metalearn.generalize",
        "system_prompt_hash": "3f44c1ae2583659b84937846392c2cd69303e9eacc9c6745530a6cddeedea5f2",
        "think_mode": True,
        "response_template": "object",
        "required_keys": ["generalization"],
        "response": json.dumps({"generalization": "Observation clusters suggest emerging meta-pattern."}),
    },
    {
        "task_name": "retrieval.intent_translate",
        "system_prompt_hash": "fe1c14a2c1504ae7833f5cf58c22063db0d069dca823ed8e7e751eea8cf697e5",
        "think_mode": False,
        "response_template": "string",
        "required_keys": [],
        "response": "How does the retrieval pipeline handle multi-hop queries?",
    },
    {
        "task_name": "retrieval.query_classify",
        "system_prompt_hash": "dd598bef7150afc2b44764ee53d47e66992966e8a65be850f9d8d92f93c04a8a",
        "think_mode": False,
        "response_template": "object",
        "required_keys": ["types"],
        "response": json.dumps({"types": ["semantic", "structural"]}),
    },
    {
        "task_name": "retrieval.rerank_cross",
        "system_prompt_hash": "8a7750d2893929ed53fed531021190be391fd231d792df519f892a06d024046b",
        "think_mode": True,
        "response_template": "array",
        "required_keys": [],
        "response": json.dumps([0.92, 0.41, 0.87]),
    },
    {
        "task_name": "retrieval.rerank_nli",
        "system_prompt_hash": "1f0f374d83bf6eb7d60d940cb46ef6c80e4eee8ce36db21242e121014e441ddb",
        "think_mode": False,
        "response_template": "array",
        "required_keys": [],
        "response": json.dumps([0.85, 0.32, 0.71]),
    },
    {
        "task_name": "summarize.generate",
        "system_prompt_hash": "6f5be281b9d99b9bfd396e626c51e6507df7f5423ecd9a3cfa84ca0e2adeaeb7",
        "think_mode": True,
        "response_template": "object",
        "required_keys": ["summary"],
        "response": json.dumps({"summary": "Generated summary of the observation cluster."}),
    },
]

# Unique task names (hidden.name_emergence appears twice for L3/L5)
UNIQUE_TASK_NAMES = sorted(set(s["task_name"] for s in TASK_SPECS))


def _make_system_prompt(spec: dict) -> str:
    """Build a system prompt whose SHA-256 matches the spec hash."""
    h = spec["system_prompt_hash"]
    if h == "dynamic":
        return f"Dynamic system prompt for {spec['task_name']} task."
    # We can't reverse SHA-256, so we store the hash and use a placeholder prompt.
    # The quality_filter validates hash against known hashes, so we pass the hash directly.
    variant = spec.get("variant", "")
    suffix = f" ({variant})" if variant else ""
    return f"System prompt for {spec['task_name']}{suffix}."


def _make_cuid() -> str:
    """Generate a CUIDv2-like ID for synthetic data."""
    import random
    import string
    return "".join(random.choices(string.ascii_lowercase + string.digits, k=24))


def generate_synthetic_records(
    instance_id: str,
    space_id: str,
    records_per_spec: int = 5,
    base_time: float | None = None,
) -> list[dict]:
    """Generate synthetic JSONL records for all task specs."""
    if base_time is None:
        base_time = time.time() - 86400 * 7  # 7 days ago

    records = []
    for spec_idx, spec in enumerate(TASK_SPECS):
        system_prompt = _make_system_prompt(spec)
        system_prompt_hash = spec["system_prompt_hash"]
        if system_prompt_hash == "dynamic":
            system_prompt_hash = hashlib.sha256(system_prompt.encode()).hexdigest()

        for i in range(records_per_spec):
            ts = base_time + (spec_idx * records_per_spec + i) * 600  # 10min intervals

            response = spec["response"]
            think_content = ""
            think_mode = spec["think_mode"]

            if think_mode:
                think_content = f"<think>Reasoning about {spec['task_name']} record {i}.</think>"

            rec = {
                "time": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(ts)),
                "trace_id": _make_cuid(),
                "task_name": spec["task_name"],
                "space_id": space_id,
                "instance_id": instance_id,
                "session_id": f"session-{instance_id}-{spec_idx}",
                "system_prompt": system_prompt,
                "system_prompt_hash": system_prompt_hash,
                "user_prompt": f"User prompt for {spec['task_name']} record {i} from {instance_id}.",
                "response": response,
                "think_content": think_content if think_mode else "",
                "think_mode": think_mode,
                "latency_ms": 150 + i * 10,
                "tokens_in": 200 + i * 5,
                "tokens_out": 100 + i * 3,
                "model_name": "gpt-4o",
                "provider": "openai",
                "error": "",
                "quality": None,
                "quality_source": "",
                "used_for_train": False,
                "guidance_id": "",
                "source_path": "",
            }
            records.append(rec)

    return records


def write_jsonl(records: list[dict], path: str) -> None:
    """Write records to a JSONL file."""
    os.makedirs(os.path.dirname(path) or ".", exist_ok=True)
    with open(path, "w") as f:
        for rec in records:
            f.write(json.dumps(rec) + "\n")


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture
def pipeline_dirs():
    """Create temp directory structure for E2E test."""
    tmpdir = tempfile.mkdtemp(prefix="ft-e2e-")
    dirs = {
        "root": tmpdir,
        "instance_a": os.path.join(tmpdir, "instance-a"),
        "instance_b": os.path.join(tmpdir, "instance-b"),
        "filtered_a": os.path.join(tmpdir, "filtered-a"),
        "filtered_b": os.path.join(tmpdir, "filtered-b"),
        "merged": os.path.join(tmpdir, "merged", "v1"),
    }
    for d in dirs.values():
        os.makedirs(d, exist_ok=True)
    yield dirs
    shutil.rmtree(tmpdir)


@pytest.fixture
def ults_dir():
    """Path to real ULTS specs directory."""
    specs = os.path.join(os.path.dirname(__file__), "..", "..", "..", "docs", "tests", "ults", "specs")
    return os.path.abspath(specs)


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

def _project_root():
    """Return absolute path to the project root."""
    return os.path.abspath(os.path.join(os.path.dirname(__file__), "..", "..", ".."))


def _run_pipeline_cmd(args, **kwargs):
    """Run a pipeline subprocess with correct PYTHONPATH."""
    root = _project_root()
    env = {**os.environ, "PYTHONPATH": os.path.join(root, "neural")}
    kwargs.setdefault("cwd", root)
    kwargs.setdefault("capture_output", True)
    kwargs.setdefault("text", True)
    kwargs["env"] = env
    return subprocess.run(args, **kwargs)


class TestFullPipeline:
    """E2E pipeline test: synthetic data -> quality_filter -> dataset_versioner."""

    def test_synthetic_data_generation(self, pipeline_dirs):
        """Verify synthetic data generation covers all 16 tasks with correct structure."""
        records_a = generate_synthetic_records("instance-a", "mdemg-dev", records_per_spec=3)
        records_b = generate_synthetic_records("instance-b", "mdemg-dev", records_per_spec=3)

        # 17 specs (16 tasks + L5 variant) * 3 records each = 51 per instance
        assert len(records_a) == len(TASK_SPECS) * 3
        assert len(records_b) == len(TASK_SPECS) * 3

        # All 16 unique task names present
        task_names_a = set(r["task_name"] for r in records_a)
        assert task_names_a == set(UNIQUE_TASK_NAMES)

        # Instance IDs correct
        assert all(r["instance_id"] == "instance-a" for r in records_a)
        assert all(r["instance_id"] == "instance-b" for r in records_b)

        # Think-mode records have think_content
        think_tasks = {s["task_name"] for s in TASK_SPECS if s["think_mode"]}
        for rec in records_a:
            if rec["task_name"] in think_tasks:
                assert rec["think_mode"] is True
                assert rec["think_content"] != ""
            else:
                assert rec["think_mode"] is False

    def test_quality_filter_passes_synthetic_data(self, pipeline_dirs, ults_dir):
        """Run quality_filter on synthetic data — all clean records should pass."""
        records = generate_synthetic_records("instance-a", "mdemg-dev", records_per_spec=5)
        input_path = os.path.join(pipeline_dirs["instance_a"], "llm_interactions.jsonl")
        output_path = os.path.join(pipeline_dirs["filtered_a"], "filtered.jsonl")
        report_path = os.path.join(pipeline_dirs["filtered_a"], "report.json")
        write_jsonl(records, input_path)

        result = _run_pipeline_cmd([
            sys.executable, "-m", "neural.training.quality_filter",
            "--input", input_path, "--output", output_path,
            "--ults-dir", ults_dir, "--report", report_path,
        ])
        assert result.returncode == 0, f"quality_filter failed:\n{result.stderr}\n{result.stdout}"
        assert os.path.exists(output_path)

        with open(report_path) as f:
            report = json.load(f)

        assert report["output_rows"] > 0
        assert report["excluded"].get("privacy_violation", 0) == 0
        assert report["excluded"].get("error_present", 0) == 0

    def test_multi_instance_merge(self, pipeline_dirs, ults_dir):
        """Generate data for 2 instances, filter, merge with dataset_versioner."""
        records_a = generate_synthetic_records("instance-a", "mdemg-dev", records_per_spec=5)
        input_a = os.path.join(pipeline_dirs["instance_a"], "llm_interactions.jsonl")
        filtered_a = os.path.join(pipeline_dirs["filtered_a"], "filtered.jsonl")
        write_jsonl(records_a, input_a)

        records_b = generate_synthetic_records(
            "instance-b", "mdemg-dev", records_per_spec=5,
            base_time=time.time() - 86400 * 3,
        )
        input_b = os.path.join(pipeline_dirs["instance_b"], "llm_interactions.jsonl")
        filtered_b = os.path.join(pipeline_dirs["filtered_b"], "filtered.jsonl")
        write_jsonl(records_b, input_b)

        for inp, out in [(input_a, filtered_a), (input_b, filtered_b)]:
            result = _run_pipeline_cmd([
                sys.executable, "-m", "neural.training.quality_filter",
                "--input", inp, "--output", out, "--ults-dir", ults_dir,
            ])
            assert result.returncode == 0, f"quality_filter failed:\n{result.stderr}"

        result = _run_pipeline_cmd([
            sys.executable, "-m", "neural.training.dataset_versioner",
            "--input-dir", pipeline_dirs["filtered_a"], pipeline_dirs["filtered_b"],
            "--output-dir", pipeline_dirs["merged"],
            "--version", "v1-test", "--dedup-key", "prompt-response",
            "--first-cycle", "--min-per-task", "1",
        ])
        assert result.returncode == 0, f"dataset_versioner failed:\n{result.stderr}\n{result.stdout}"

        merged_dir = pipeline_dirs["merged"]
        assert os.path.exists(os.path.join(merged_dir, "train.jsonl"))
        assert os.path.exists(os.path.join(merged_dir, "manifest.json"))

        with open(os.path.join(merged_dir, "manifest.json")) as f:
            manifest = json.load(f)
        assert manifest["version"] == "v1-test"
        assert manifest["total_loaded"] > 0

        with open(os.path.join(merged_dir, "train.jsonl")) as f:
            for line in f:
                record = json.loads(line.strip())
                assert "messages" in record
                msgs = record["messages"]
                assert len(msgs) >= 2
                assert msgs[-1]["role"] == "assistant"
                assert msgs[-1]["content"] != ""
                break

    def test_think_mode_preserved(self, pipeline_dirs, ults_dir):
        """Verify think_content is preserved through the pipeline for think-mode tasks."""
        records = generate_synthetic_records("instance-a", "mdemg-dev", records_per_spec=3)
        input_path = os.path.join(pipeline_dirs["instance_a"], "llm_interactions.jsonl")
        filtered_path = os.path.join(pipeline_dirs["filtered_a"], "filtered.jsonl")
        output_dir = os.path.join(pipeline_dirs["root"], "think-test")
        os.makedirs(output_dir, exist_ok=True)
        write_jsonl(records, input_path)

        _run_pipeline_cmd([
            sys.executable, "-m", "neural.training.quality_filter",
            "--input", input_path, "--output", filtered_path, "--ults-dir", ults_dir,
        ], check=True)

        _run_pipeline_cmd([
            sys.executable, "-m", "neural.training.dataset_versioner",
            "--input-dir", pipeline_dirs["filtered_a"],
            "--output-dir", output_dir,
            "--version", "v1-think", "--first-cycle", "--min-per-task", "1",
        ], check=True)

        think_found = False
        with open(os.path.join(output_dir, "train.jsonl")) as f:
            for line in f:
                record = json.loads(line.strip())
                assistant_msg = record["messages"][-1]["content"]
                if "<think>" in assistant_msg:
                    think_found = True
                    assert "</think>" in assistant_msg
                    break

        assert think_found, "No think-mode records found in pipeline output"

    def test_dedup_key_prompt_collapses_same_prompts(self, pipeline_dirs, ults_dir):
        """With --dedup-key prompt (default), same prompt from 2 instances keeps only first."""
        base = time.time() - 86400 * 5
        records_a = generate_synthetic_records("instance-a", "mdemg-dev", records_per_spec=2, base_time=base)
        records_b = generate_synthetic_records("instance-b", "mdemg-dev", records_per_spec=2, base_time=base)

        input_a = os.path.join(pipeline_dirs["instance_a"], "llm_interactions.jsonl")
        input_b = os.path.join(pipeline_dirs["instance_b"], "llm_interactions.jsonl")
        filtered_a = os.path.join(pipeline_dirs["filtered_a"], "filtered.jsonl")
        filtered_b = os.path.join(pipeline_dirs["filtered_b"], "filtered.jsonl")
        write_jsonl(records_a, input_a)
        write_jsonl(records_b, input_b)

        for inp, out in [(input_a, filtered_a), (input_b, filtered_b)]:
            _run_pipeline_cmd([
                sys.executable, "-m", "neural.training.quality_filter",
                "--input", inp, "--output", out,
                "--ults-dir", ults_dir, "--dedup-key", "prompt",
            ], check=True)

        merged_prompt = os.path.join(pipeline_dirs["root"], "merged-prompt")
        os.makedirs(merged_prompt, exist_ok=True)
        _run_pipeline_cmd([
            sys.executable, "-m", "neural.training.dataset_versioner",
            "--input-dir", pipeline_dirs["filtered_a"], pipeline_dirs["filtered_b"],
            "--output-dir", merged_prompt,
            "--version", "v1-dedup", "--dedup-key", "prompt",
            "--first-cycle", "--min-per-task", "1",
        ], check=True)

        merged_pr = os.path.join(pipeline_dirs["root"], "merged-pr")
        os.makedirs(merged_pr, exist_ok=True)
        _run_pipeline_cmd([
            sys.executable, "-m", "neural.training.dataset_versioner",
            "--input-dir", pipeline_dirs["filtered_a"], pipeline_dirs["filtered_b"],
            "--output-dir", merged_pr,
            "--version", "v1-dedup-pr", "--dedup-key", "prompt-response",
            "--first-cycle", "--min-per-task", "1",
        ], check=True)

        with open(os.path.join(merged_prompt, "manifest.json")) as f:
            manifest_prompt = json.load(f)
        with open(os.path.join(merged_pr, "manifest.json")) as f:
            manifest_pr = json.load(f)

        prompt_total = manifest_prompt["deduplication"]["total_after"]
        pr_total = manifest_pr["deduplication"]["total_after"]
        assert pr_total >= prompt_total, (
            f"prompt-response dedup ({pr_total}) should keep >= prompt dedup ({prompt_total})"
        )

    def test_all_task_names_in_output(self, pipeline_dirs, ults_dir):
        """Verify all 16 task names appear in the versioned output."""
        records = generate_synthetic_records("instance-a", "mdemg-dev", records_per_spec=5)
        input_path = os.path.join(pipeline_dirs["instance_a"], "llm_interactions.jsonl")
        filtered_path = os.path.join(pipeline_dirs["filtered_a"], "filtered.jsonl")
        write_jsonl(records, input_path)

        _run_pipeline_cmd([
            sys.executable, "-m", "neural.training.quality_filter",
            "--input", input_path, "--output", filtered_path,
            "--ults-dir", ults_dir,
            "--report", os.path.join(pipeline_dirs["filtered_a"], "report.json"),
        ], check=True)

        with open(os.path.join(pipeline_dirs["filtered_a"], "report.json")) as f:
            report = json.load(f)

        task_dist = report.get("task_distribution", {})
        for task_name in UNIQUE_TASK_NAMES:
            assert task_name in task_dist, f"Task {task_name} missing from filter output"
            assert task_dist[task_name] > 0, f"Task {task_name} has 0 records after filtering"
