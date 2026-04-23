#!/usr/bin/env python3
"""Smoke test all 16 MDEMG ULTS tasks through a vllm-mlx inference server.

For each ULTS spec, constructs a minimal test prompt, sends it to the
vllm-mlx OpenAI-compatible API, and validates the response against the
task's output_schema.

Usage:
    python3 scripts/test_vllm_mlx.py --base-url http://localhost:8101/v1
    python3 scripts/test_vllm_mlx.py --base-url http://localhost:8101/v1 --model mlx-community/Qwen3-14B-4bit
    python3 scripts/test_vllm_mlx.py --base-url http://localhost:8101/v1 --model .local-models/qwen3-14b-mdemg-v1
    python3 scripts/test_vllm_mlx.py --report /tmp/vllm-smoke-report.json

Note: Post-Phase-5 pivot (2026-04-22) — MLX port moved 8100 → 8101
(CMS-pinned); MoE Qwen3.6-35B-A3B abandoned → dense Qwen3-14B-4bit baseline
plus merged MDEMG fine-tuned model at .local-models/qwen3-14b-mdemg-v1/.
"""

import argparse
import json
import os
import sys
import time
from pathlib import Path
from urllib.request import Request, urlopen
from urllib.error import URLError

ULTS_DIR = Path(__file__).parent.parent / "docs" / "tests" / "ults" / "specs"

# Minimal test prompts per task — designed to produce a valid response
# without requiring any external context (Neo4j, TSDB, etc.)
TEST_PROMPTS = {
    "consulting.classify": "The module uses a singleton pattern for database connections with hardcoded credentials.",
    "consulting.synthesis": "Summarize the architectural trade-offs between microservices and monolithic deployment for an industrial control system.",
    "retrieval.query_classify": "How does the authentication middleware work?",
    "retrieval.intent_translate": "What changed in the auth module last week?",
    "retrieval.rerank_cross": "Rate the relevance of this passage to the query 'authentication': 'The login handler validates JWT tokens and checks expiration.'",
    "retrieval.rerank_nli": "Does the passage 'Users authenticate via OAuth2 tokens' entail the query 'How do users log in?'",
    "jiminy.synthesize": "The developer is about to delete a critical database index. Previous guidance: always back up before schema changes.",
    "jiminy.evaluate": '{"guidance_id": "test-001", "action_taken": "backed up database before dropping index", "outcome": "successful migration"}',
    "jiminy.evaluate_llm": '{"guidance_id": "test-001", "action_taken": "ignored backup warning", "outcome": "data loss during migration", "initial_verdict": "negative"}',
    "jiminy.codegen": "Never delete database indexes without a backup. This was learned after a production incident on 2026-03-15.",
    "hidden.summarize": "func handleLogin(w http.ResponseWriter, r *http.Request) { token := r.Header.Get(\"Authorization\"); if token == \"\" { http.Error(w, \"unauthorized\", 401); return } }",
    "hidden.reclassify": "This observation is about database connection pooling. Current classification: 'architecture'. Related concepts: performance, scalability, resource management.",
    "hidden.name_emergence": "Cluster contains: database pooling, connection reuse, resource limits, timeout handling, retry logic. Suggest a name.",
    "summarize.generate": "func calculateScore(metrics []float64) float64 { sum := 0.0; for _, m := range metrics { sum += m }; return sum / float64(len(metrics)) }",
    "ape.reflect": "Over the last 24 hours: 15 retrieval queries (avg latency 340ms), 8 LLM calls (2 errors), trust score 0.72 (down from 0.78).",
    "metalearn.generalize": "Across 3 codebases: singleton anti-pattern detected 12 times, each time correlated with testing difficulties. Confidence: 0.85.",
}


def load_ults_specs():
    """Load all ULTS spec files."""
    specs = {}
    for spec_path in sorted(ULTS_DIR.glob("*.ults.json")):
        with open(spec_path) as f:
            spec = json.load(f)
        task_name = spec["task"]["name"]
        specs[task_name] = spec
    return specs


def send_completion(base_url, model, system_prompt, user_prompt, max_tokens, think_mode):
    """Send a chat completion request to the vllm-mlx server."""
    messages = [
        {"role": "system", "content": system_prompt},
        {"role": "user", "content": user_prompt},
    ]

    body = {
        "model": model,
        "messages": messages,
        "max_tokens": max_tokens,
        "temperature": 0.1,
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

    start = time.monotonic()
    try:
        with urlopen(req, timeout=30) as resp:
            result = json.loads(resp.read())
    except URLError as e:
        return None, 0, str(e)
    except TimeoutError:
        return None, 0, "timeout (30s)"

    elapsed_ms = (time.monotonic() - start) * 1000
    content = result["choices"][0]["message"]["content"]
    return content, elapsed_ms, None


def validate_output(content, output_schema):
    """Basic validation against ULTS output_schema."""
    schema_type = output_schema.get("type", "string")

    if schema_type == "object":
        # Must be valid JSON
        try:
            parsed = json.loads(content)
        except json.JSONDecodeError:
            # Try extracting JSON from markdown code blocks
            if "```json" in content:
                json_block = content.split("```json")[1].split("```")[0].strip()
                try:
                    parsed = json.loads(json_block)
                except json.JSONDecodeError:
                    return False, "invalid JSON"
            elif "```" in content:
                json_block = content.split("```")[1].split("```")[0].strip()
                try:
                    parsed = json.loads(json_block)
                except json.JSONDecodeError:
                    return False, "invalid JSON"
            else:
                return False, "invalid JSON"

        if not isinstance(parsed, dict):
            return False, f"expected object, got {type(parsed).__name__}"

        # Check required keys
        required = output_schema.get("required", [])
        missing = [k for k in required if k not in parsed]
        if missing:
            return False, f"missing required keys: {missing}"

        return True, "valid"

    elif schema_type == "string":
        if not content or not content.strip():
            return False, "empty response"
        return True, "valid"

    return True, "no schema validation"


def build_system_prompt(task_name, spec):
    """Build a minimal system prompt for testing."""
    schema = spec.get("output_schema", {})
    schema_type = schema.get("type", "string")

    if schema_type == "object":
        required = schema.get("required", [])
        props = schema.get("properties", {})
        fields_desc = ", ".join(
            f'"{k}" ({v.get("type", "any")})'
            for k, v in props.items()
        )
        return (
            f"You are {spec['task']['description']}. "
            f"Respond with a JSON object containing: {fields_desc}. "
            f"Required fields: {required}. "
            "Output ONLY valid JSON, no markdown."
        )
    else:
        return (
            f"You are {spec['task']['description']}. "
            "Respond with a clear, concise text answer."
        )


def run_smoke_test(base_url, model, verbose=False):
    """Run smoke test across all 16 ULTS tasks."""
    specs = load_ults_specs()
    results = []

    print(f"Testing {len(specs)} tasks against {base_url}")
    print(f"Model: {model}")
    print(f"{'─' * 80}")
    print(f"{'Task':<35} {'Status':<8} {'Latency':>8} {'Detail'}")
    print(f"{'─' * 80}")

    for task_name, spec in sorted(specs.items()):
        user_prompt = TEST_PROMPTS.get(task_name)
        if not user_prompt:
            print(f"{task_name:<35} {'SKIP':<8} {'':>8} no test prompt defined")
            results.append({
                "task": task_name, "status": "skip",
                "detail": "no test prompt",
            })
            continue

        system_prompt = build_system_prompt(task_name, spec)
        max_tokens = spec.get("performance", {}).get("max_tokens", 500)
        think_mode = spec.get("prompt", {}).get("think_mode", False)

        content, latency_ms, error = send_completion(
            base_url, model, system_prompt, user_prompt, max_tokens, think_mode,
        )

        if error:
            print(f"{task_name:<35} {'FAIL':<8} {'':>8} {error}")
            results.append({
                "task": task_name, "status": "fail",
                "detail": error, "latency_ms": 0,
            })
            continue

        valid, detail = validate_output(content, spec.get("output_schema", {}))
        status = "PASS" if valid else "FAIL"

        print(f"{task_name:<35} {status:<8} {latency_ms:>7.0f}ms {detail}")

        result = {
            "task": task_name,
            "status": status.lower(),
            "latency_ms": round(latency_ms),
            "detail": detail,
        }
        if verbose:
            result["response_preview"] = (content or "")[:200]
        results.append(result)

    print(f"{'─' * 80}")
    passed = sum(1 for r in results if r["status"] == "pass")
    failed = sum(1 for r in results if r["status"] == "fail")
    skipped = sum(1 for r in results if r["status"] == "skip")
    print(f"Results: {passed} passed, {failed} failed, {skipped} skipped")

    return results


def main():
    parser = argparse.ArgumentParser(
        description="Smoke test MDEMG tasks through vllm-mlx",
    )
    parser.add_argument(
        "--base-url", default=os.environ.get("LLM_BASE_URL", "http://localhost:8101/v1"),
        help="mlx_lm.server base URL (default: $LLM_BASE_URL or http://localhost:8101/v1)",
    )
    parser.add_argument(
        "--model", default=os.environ.get("MLX_BASE_MODEL", "mlx-community/Qwen3-14B-4bit"),
        help="Model name (default: $MLX_BASE_MODEL or mlx-community/Qwen3-14B-4bit). "
             "For merged MDEMG fine-tuned model, pass $MLX_MERGED_MODEL_PATH "
             "(.local-models/qwen3-14b-mdemg-v1).",
    )
    parser.add_argument("--report", help="Write JSON report to file")
    parser.add_argument("--verbose", action="store_true", help="Include response previews")
    args = parser.parse_args()

    # Check server is reachable
    try:
        req = Request(f"{args.base_url}/models", method="GET")
        with urlopen(req, timeout=5) as resp:
            resp.read()
    except Exception as e:
        print(f"ERROR: Cannot reach mlx_lm.server at {args.base_url}: {e}")
        print("Start the server: python -m mlx_lm.server --model mlx-community/Qwen3-14B-4bit --port 8101")
        sys.exit(1)

    results = run_smoke_test(args.base_url, args.model, verbose=args.verbose)

    if args.report:
        report = {
            "base_url": args.base_url,
            "model": args.model,
            "results": results,
            "summary": {
                "total": len(results),
                "passed": sum(1 for r in results if r["status"] == "pass"),
                "failed": sum(1 for r in results if r["status"] == "fail"),
                "skipped": sum(1 for r in results if r["status"] == "skip"),
            },
        }
        with open(args.report, "w") as f:
            json.dump(report, f, indent=2)
        print(f"\nReport written to {args.report}")

    failed = sum(1 for r in results if r["status"] == "fail")
    sys.exit(1 if failed > 0 else 0)


if __name__ == "__main__":
    main()
