"""Teacher Distillation for Under-Represented ULTS Tasks.

Generates synthetic training data by sending representative inputs
through a teacher LLM with exact MDEMG system prompts, validating
responses against ULTS output_schema, and outputting JSONL compatible
with the existing format_converter.py pipeline.

Usage:
    python -m training.teacher_distill \
        --ults-dir docs/tests/ults/specs/ \
        --task consulting.classify \
        --teacher-url http://localhost:8101/v1 \
        --teacher-model mlx-community/Qwen3-14B-4bit \
        --count 50 \
        --output distilled/consulting_classify.jsonl

    # Multiple tasks at once:
    python -m training.teacher_distill \
        --ults-dir docs/tests/ults/specs/ \
        --task metalearn.generalize --task jiminy.evaluate_llm \
        --teacher-url http://localhost:8101/v1 \
        --teacher-model mlx-community/Qwen3-14B-4bit \
        --count 30 \
        --output distilled/multi_task.jsonl

    # Post-Phase-5 pivot (2026-04-22): teacher swapped from Qwen3.6-35B-A3B
    # (MoE, abandoned) to dense Qwen3-14B-4bit. Port moved 8100 → 8101 per
    # CMS pinned constraint.

Requirements:
    pip install -e ".[training]"
"""

import argparse
import json
import os
import re
import sys
from pathlib import Path
from typing import Any
from urllib.error import URLError
from urllib.request import Request, urlopen


# ── System Prompt Extraction ──


def extract_system_prompt(source_ref: str | list[str], project_root: str) -> str | None:
    """Extract system prompt from Go source using ULTS source reference.

    source_ref format: "internal/path/file.go:line_number"
    Returns the Go string constant starting at that line, or None.
    """
    if isinstance(source_ref, list):
        source_ref = source_ref[0]  # Use first variant

    match = re.match(r"^(.+):(\d+)$", source_ref)
    if not match:
        return None

    file_path = os.path.join(project_root, match.group(1))
    line_num = int(match.group(2))

    if not os.path.exists(file_path):
        return None

    try:
        with open(file_path) as f:
            lines = f.readlines()
    except OSError:
        return None

    if line_num < 1 or line_num > len(lines):
        return None

    # Find the string constant — look for backtick-delimited or
    # double-quote string starting at or near the target line
    text = "".join(lines[line_num - 1:])

    # Try backtick string first (most common for multi-line prompts)
    bt_match = re.search(r"`([^`]+)`", text)
    if bt_match:
        return bt_match.group(1).strip()

    # Try double-quoted string
    dq_match = re.search(r'"([^"]+)"', text)
    if dq_match:
        return dq_match.group(1).strip()

    return None


# ── Input Generation ──


# Representative input templates per task family.
# Each is a list of templates with {placeholders} for variation.
INPUT_TEMPLATES: dict[str, list[str]] = {
    "consulting.classify": [
        "The module uses a {pattern} pattern for {component}.",
        "This function handles {operation} with {approach}.",
        "The {component} implements {pattern} for managing {resource}.",
    ],
    "consulting.synthesis": [
        "Summarize the trade-offs between {approach_a} and {approach_b} for {context}.",
        "What are the architectural implications of using {pattern} in {domain}?",
    ],
    "retrieval.query_classify": [
        "How does the {component} work?",
        "What changed in the {module} last {time_period}?",
        "Show me the {concept} implementation.",
    ],
    "retrieval.intent_translate": [
        "What changed in {module} last {time_period}?",
        "Who modified the {component} recently?",
    ],
    "retrieval.rerank_cross": [
        "Rate the relevance of this passage to the query '{query}': '{passage}'",
    ],
    "retrieval.rerank_nli": [
        "Does the passage '{passage}' entail the query '{query}'?",
    ],
    "jiminy.synthesize": [
        "The developer is about to {action}. Previous guidance: {guidance}.",
    ],
    "jiminy.evaluate": [
        '{{"guidance_id": "g-{id}", "action_taken": "{action}", "outcome": "{outcome}"}}',
    ],
    "jiminy.evaluate_llm": [
        '{{"guidance_id": "g-{id}", "action_taken": "{action}", "outcome": "{outcome}", "initial_verdict": "{verdict}"}}',
    ],
    "jiminy.codegen": [
        "Never {action} without {precondition}. This was learned after {incident}.",
    ],
    "hidden.summarize": [
        "func {func_name}({params}) {return_type} {{ {body} }}",
    ],
    "hidden.reclassify": [
        "This observation is about {topic}. Current classification: '{classification}'. Related concepts: {concepts}.",
    ],
    "hidden.name_emergence": [
        "Cluster contains: {items}. Suggest a name.",
    ],
    "summarize.generate": [
        "func {func_name}({params}) {return_type} {{ {body} }}",
    ],
    "ape.reflect": [
        "Over the last {hours} hours: {queries} retrieval queries (avg latency {latency}ms), {calls} LLM calls ({errors} errors), trust score {trust}.",
    ],
    "metalearn.generalize": [
        "Across {count} codebases: {pattern} detected {times} times, each time correlated with {effect}. Confidence: {confidence}.",
    ],
}

# Vocabulary for template substitution
VOCABULARY: dict[str, list[str]] = {
    "pattern": ["singleton", "factory", "observer", "strategy", "adapter", "decorator"],
    "component": ["database", "authentication", "logging", "caching", "queue", "scheduler"],
    "operation": ["request routing", "data validation", "error handling", "connection pooling"],
    "approach": ["lazy loading", "eager initialization", "connection pooling", "retry logic"],
    "approach_a": ["microservices", "event sourcing", "CQRS", "monolithic"],
    "approach_b": ["monolithic", "shared database", "message queue", "REST API"],
    "context": ["an industrial control system", "a real-time monitoring platform", "a data pipeline"],
    "domain": ["distributed systems", "embedded controllers", "web services"],
    "resource": ["connections", "threads", "memory", "file handles"],
    "module": ["auth", "retrieval", "hidden layer", "jiminy", "summarize"],
    "time_period": ["week", "month", "sprint", "quarter"],
    "concept": ["consolidation", "emergence", "retrieval", "trust scoring"],
    "query": ["authentication flow", "database indexing", "error handling"],
    "passage": ["The login handler validates JWT tokens and checks expiration."],
    "action": ["delete a production index", "drop a database table", "force push to main"],
    "guidance": ["always back up before schema changes", "run tests before deployment"],
    "precondition": ["a backup", "approval from the team lead", "running the test suite"],
    "incident": ["a production outage on 2026-03-15", "data loss during migration"],
    "id": ["001", "002", "003", "004", "005"],
    "outcome": ["successful migration", "failed deployment", "data loss", "clean rollback"],
    "verdict": ["positive", "negative", "neutral"],
    "func_name": ["handleLogin", "processQueue", "calculateScore", "validateInput"],
    "params": ["w http.ResponseWriter, r *http.Request", "ctx context.Context, id string"],
    "return_type": ["error", "float64", "string", "bool"],
    "body": ["return nil", "sum := 0.0; return sum", "if err != nil { return err }"],
    "topic": ["connection pooling", "error handling", "caching strategy", "auth flow"],
    "classification": ["architecture", "performance", "security", "reliability"],
    "concepts": ["scalability, throughput", "latency, timeout", "encryption, tokens"],
    "items": ["connection pooling, resource limits, timeout handling, retry logic"],
    "hours": ["24", "48", "72"],
    "queries": ["15", "42", "108"],
    "latency": ["200", "340", "510"],
    "calls": ["8", "20", "35"],
    "errors": ["0", "2", "5"],
    "trust": ["0.72", "0.85", "0.61"],
    "count": ["3", "5", "8"],
    "times": ["12", "25", "7"],
    "effect": ["testing difficulties", "deployment failures", "performance issues"],
    "confidence": ["0.75", "0.85", "0.92"],
}


def generate_inputs(task_name: str, count: int, seed: int = 42) -> list[str]:
    """Generate diverse input prompts for a task using templates."""
    import random
    rng = random.Random(seed)

    templates = INPUT_TEMPLATES.get(task_name, [])
    if not templates:
        return []

    inputs = []
    for i in range(count):
        template = templates[i % len(templates)]

        # Find all placeholders and substitute
        result = template
        for key, values in VOCABULARY.items():
            placeholder = "{" + key + "}"
            if placeholder in result:
                result = result.replace(placeholder, rng.choice(values), 1)

        inputs.append(result)

    return inputs


# ── Teacher Inference ──


def teacher_generate(
    base_url: str,
    model: str,
    system_prompt: str,
    user_prompt: str,
    max_tokens: int,
    think_mode: bool = False,
) -> tuple[str | None, str | None, str | None]:
    """Send prompt to teacher LLM. Returns (response, think_content, error)."""
    messages = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": user_prompt},
    ]

    body: dict[str, Any] = {
        "model": model,
        "messages": messages,
        "max_tokens": max_tokens,
        "temperature": 0.7,
    }

    if think_mode:
        body["chat_template_kwargs"] = {"enable_thinking": True}

    data = json.dumps(body).encode()
    req = Request(
        f"{base_url}/chat/completions",
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )

    try:
        with urlopen(req, timeout=120) as resp:
            result = json.loads(resp.read())
    except (URLError, TimeoutError) as e:
        return None, None, str(e)

    content = result["choices"][0]["message"]["content"]

    # Extract think content if present
    think_content = None
    response = content
    think_match = re.match(r"<think>\n?(.*?)\n?</think>\n\n?(.*)", content, re.DOTALL)
    if think_match:
        think_content = think_match.group(1).strip()
        response = think_match.group(2).strip()

    return response, think_content, None


# ── Output Validation ──


def validate_output(response: str, schema: dict[str, Any]) -> bool:
    """Validate response against ULTS output_schema."""
    schema_type = schema.get("type", "string")

    if schema_type == "string":
        return bool(response and response.strip())

    try:
        parsed = json.loads(response)
    except (json.JSONDecodeError, TypeError):
        return False

    if not isinstance(parsed, dict):
        return False

    required = schema.get("required", [])
    return all(k in parsed for k in required)


# ── Distillation Engine ──


def distill_task(
    task_name: str,
    spec: dict[str, Any],
    teacher_url: str,
    teacher_model: str,
    count: int,
    project_root: str,
    seed: int = 42,
) -> list[dict[str, Any]]:
    """Generate distilled training records for a single task.

    Returns list of raw records (compatible with format_converter.py input).
    """
    # Extract system prompt
    prompt_config = spec.get("prompt", {})
    source_ref = prompt_config.get("system_prompt_source")
    think_mode = prompt_config.get("think_mode", False)
    schema = spec.get("output_schema", {})
    perf = spec.get("performance", {})
    max_tokens = perf.get("max_tokens", 3000)

    system_prompt = None
    if source_ref:
        system_prompt = extract_system_prompt(source_ref, project_root)

    if not system_prompt:
        # Fallback: build from task description
        desc = spec.get("task", {}).get("description", task_name)
        system_prompt = f"You are {desc}."

    # Generate inputs
    inputs = generate_inputs(task_name, count, seed=seed)
    if not inputs:
        print(f"  {task_name}: no input templates available", file=sys.stderr)
        return []

    records = []
    valid = 0
    invalid = 0

    for user_prompt in inputs:
        response, think_content, error = teacher_generate(
            teacher_url, teacher_model, system_prompt, user_prompt,
            max_tokens, think_mode,
        )

        if error:
            print(f"  {task_name}: teacher error — {error}", file=sys.stderr)
            invalid += 1
            continue

        if not validate_output(response, schema):
            invalid += 1
            continue

        record = {
            "task_name": task_name,
            "system_prompt": system_prompt,
            "user_prompt": user_prompt,
            "response": response,
            "think_content": think_content,
            "think_mode": think_mode,
            "quality_source": "teacher_distillation",
            "quality": None,
            "model_name": teacher_model,
        }
        records.append(record)
        valid += 1

    print(f"  {task_name}: {valid} valid, {invalid} invalid out of {count} attempts")
    return records


def run_distillation(
    ults_dir: str,
    tasks: list[str],
    teacher_url: str,
    teacher_model: str,
    count: int,
    output_path: str,
    project_root: str,
    seed: int = 42,
) -> dict[str, Any]:
    """Run distillation for selected tasks.

    Returns a summary dict.
    """
    # Load specs
    specs: dict[str, dict[str, Any]] = {}
    for spec_path in sorted(Path(ults_dir).glob("*.ults.json")):
        try:
            with open(spec_path) as f:
                spec = json.load(f)
            specs[spec["task"]["name"]] = spec
        except (json.JSONDecodeError, KeyError):
            continue

    # Validate requested tasks
    for task in tasks:
        if task not in specs:
            print(f"ERROR: task '{task}' not found in ULTS specs", file=sys.stderr)
            print(f"Available: {', '.join(sorted(specs.keys()))}", file=sys.stderr)
            raise SystemExit(1)

    print(f"Distilling {count} records per task for {len(tasks)} tasks")
    print(f"Teacher: {teacher_model} @ {teacher_url}")
    print(f"{'─' * 60}")

    all_records = []
    task_counts = {}

    for task_name in tasks:
        records = distill_task(
            task_name, specs[task_name],
            teacher_url, teacher_model, count, project_root, seed,
        )
        all_records.extend(records)
        task_counts[task_name] = len(records)

    # Write output
    os.makedirs(os.path.dirname(output_path) or ".", exist_ok=True)
    with open(output_path, "w") as f:
        for record in all_records:
            f.write(json.dumps(record) + "\n")

    print(f"{'─' * 60}")
    print(f"Wrote {len(all_records)} records to {output_path}")

    return {
        "total_records": len(all_records),
        "per_task": task_counts,
        "output_path": output_path,
        "teacher_model": teacher_model,
    }


def main():
    parser = argparse.ArgumentParser(
        description="Teacher distillation for under-represented ULTS tasks",
    )
    parser.add_argument(
        "--ults-dir",
        default=str(Path(__file__).parent.parent / ".." / "docs" / "tests" / "ults" / "specs"),
        help="Directory containing *.ults.json specs",
    )
    parser.add_argument(
        "--task", action="append", required=True,
        help="Task name to distill (can be repeated)",
    )
    parser.add_argument(
        "--teacher-url", required=True,
        help="Teacher LLM base URL (OpenAI-compatible)",
    )
    parser.add_argument(
        "--teacher-model", required=True,
        help="Teacher model name",
    )
    parser.add_argument(
        "--count", type=int, default=50,
        help="Number of records to generate per task (default: 50)",
    )
    parser.add_argument(
        "--output", required=True,
        help="Output JSONL path",
    )
    parser.add_argument(
        "--project-root",
        default=str(Path(__file__).parent.parent / ".."),
        help="MDEMG project root (for system prompt extraction)",
    )
    parser.add_argument(
        "--seed", type=int, default=42,
        help="Random seed for input generation (default: 42)",
    )
    parser.add_argument("--report", help="Write JSON summary to file")
    args = parser.parse_args()

    summary = run_distillation(
        ults_dir=args.ults_dir,
        tasks=args.task,
        teacher_url=args.teacher_url,
        teacher_model=args.teacher_model,
        count=args.count,
        output_path=args.output,
        project_root=args.project_root,
        seed=args.seed,
    )

    if args.report:
        with open(args.report, "w") as f:
            json.dump(summary, f, indent=2)
        print(f"Report written to {args.report}")


if __name__ == "__main__":
    main()
