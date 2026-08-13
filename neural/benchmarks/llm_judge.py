"""LLM-as-judge for subjective benchmark metrics (coherence / depth / relevance / naturalness).

Uses OpenAI-compatible `/v1/chat/completions` via urllib (no SDK dep — matches
`neural/training/distill_driver.py` pattern). The judge has its own fixed
sampling kwargs (`temperature=0`, `seed=<run_idx>`) and **never** inherits
`inference.sampling` from the task under test. This is an explicit invariant
— J-group `presence_penalty=1.5` would bias judge reasoning.

Determinism: identical (prompt, model, seed) inputs produce identical
`response` strings across calls. Model ID + seed + prompt-template SHA are
logged per benchmark_results row.

Prompt templates live in `neural/benchmarks/judge_prompts/{metric}.txt` and
must contain the string tokens `{task_description}`, `{task_response}`.
"""
from __future__ import annotations

import hashlib
import json
import os
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any
from urllib.error import HTTPError, URLError
from urllib.parse import urlparse
from urllib.request import Request, urlopen


def _is_openai_endpoint(base_url: str) -> bool:
    """SEC-TRANCHE-3: return True iff *base_url*'s hostname IS `api.openai.com`
    (or a subdomain of it). Substring-in-string is unsafe:
    ``api.openai.com`` also matches ``api.openai.com.evil.example.com`` or
    ``evil.com/?spoof=api.openai.com``. Parse the URL and compare hostnames.
    """
    try:
        host = (urlparse(base_url).hostname or "").lower()
    except (ValueError, AttributeError):
        return False
    return host == "api.openai.com" or host.endswith(".openai.com")


class JudgeError(RuntimeError):
    """Raised on any non-recoverable judge failure."""


# Judge's own sampling — NEVER inherits from task spec.
# Explicit constant dict so callers can't accidentally mutate it.
_JUDGE_SAMPLING_FIXED = {
    "temperature": 0.0,
    # top_p / top_k intentionally not set — OpenAI default
}
# Forbidden inheritance keys — any caller-supplied kwarg matching these fails loudly.
_FORBIDDEN_INHERIT_KEYS = frozenset(
    {"presence_penalty", "frequency_penalty", "top_p", "top_k", "temperature"}
)


@dataclass
class JudgeResult:
    """One metric score for one task response."""
    metric: str
    score: float              # 0.0 .. 1.0
    rationale: str
    model: str
    seed: int
    prompt_template_sha: str
    raw_response: str
    latency_ms: int


def _prompt_template_path(metric: str, prompt_dir: Path) -> Path:
    return prompt_dir / f"{metric}.txt"


def _load_prompt_template(metric: str, prompt_dir: Path) -> tuple[str, str]:
    """Return (template_text, sha256_of_template)."""
    path = _prompt_template_path(metric, prompt_dir)
    if not path.exists():
        raise JudgeError(f"judge prompt template missing: {path}")
    txt = path.read_text(encoding="utf-8")
    sha = hashlib.sha256(txt.encode("utf-8")).hexdigest()
    return txt, sha


def _format_prompt(template: str, task_description: str, task_response: str) -> str:
    # Simple str.format-style substitution; the templates check into the repo
    # so CI grep validates the two required tokens.
    return template.format(
        task_description=task_description,
        task_response=task_response,
    )


def _parse_score(raw_text: str) -> tuple[float, str]:
    """Parse '{"score": 0.83, "rationale": "..."}' JSON from the model.

    The judge prompts instruct the model to emit JSON with a numeric `score`
    in [0, 1] and a string `rationale`. On malformed output, fall back to a
    permissive scan (first number 0-1 on any line + remaining text as rationale).
    """
    # Strip common code-fence wrappers
    txt = raw_text.strip()
    if txt.startswith("```"):
        # drop first and last fence lines
        lines = txt.splitlines()
        if len(lines) >= 3:
            txt = "\n".join(lines[1:-1])

    try:
        obj = json.loads(txt)
        score = float(obj["score"])
        rationale = str(obj.get("rationale", ""))
        if not 0.0 <= score <= 1.0:
            raise ValueError("score outside [0, 1]")
        return score, rationale
    except (json.JSONDecodeError, KeyError, TypeError, ValueError):
        pass

    # permissive fallback — find first float in [0,1]
    import re
    m = re.search(r"\b(?:0(?:\.\d+)?|1(?:\.0+)?)\b", txt)
    if m is None:
        raise JudgeError(f"could not parse judge score from: {txt[:200]!r}")
    score = float(m.group(0))
    return max(0.0, min(1.0, score)), txt


def judge(
    task_description: str,
    task_response: str,
    metric: str,
    *,
    run_idx: int,
    config: dict[str, Any],
    api_key: str | None = None,
    base_url: str = "https://api.openai.com/v1",
    _http_post=None,
    **caller_kwargs: Any,
) -> JudgeResult:
    """Score a single (metric, response) pair with the configured LLM judge.

    Args:
        task_description: human-readable purpose of the task (drops into prompt).
        task_response:    the output being judged.
        metric:           one of `config['judge']['metrics']`.
        run_idx:          becomes the judge seed (determinism per-run).
        config:           parsed benchmark YAML.
        api_key:          explicit override; defaults to ``$OPENAI_API_KEY``.
        base_url:         override (used by tests against a mock).
        _http_post:       test hook — function(url, headers, body_bytes, timeout) → bytes.
        **caller_kwargs:  rejected — see _FORBIDDEN_INHERIT_KEYS.

    Raises JudgeError on any non-recoverable failure (missing template,
    forbidden kwarg, parse failure after fallback, persistent HTTP error).
    """
    # Forbidden-key guard: prevents J-group presence_penalty leak.
    bad = _FORBIDDEN_INHERIT_KEYS & caller_kwargs.keys()
    if bad:
        raise JudgeError(
            f"judge does not inherit sampling kwargs from caller (forbidden: {sorted(bad)})"
        )

    judge_cfg = config["judge"]
    allowed_metrics = set(judge_cfg.get("metrics") or ())
    if metric not in allowed_metrics:
        raise JudgeError(
            f"metric {metric!r} not in config.judge.metrics={sorted(allowed_metrics)}"
        )

    prompt_dir = Path(judge_cfg["prompt_dir"])
    template, template_sha = _load_prompt_template(metric, prompt_dir)
    user_prompt = _format_prompt(template, task_description, task_response)

    model = str(judge_cfg["model"])
    max_tokens = int(judge_cfg["max_tokens"])
    latency_budget_ms = int(judge_cfg["latency_budget_ms"])
    if max_tokens < 3000:
        raise JudgeError(f"judge.max_tokens={max_tokens} < 3000 (MEMORY rule)")
    if latency_budget_ms < 15000:
        raise JudgeError(
            f"judge.latency_budget_ms={latency_budget_ms} < 15000 (MEMORY rule)"
        )

    is_openai = _is_openai_endpoint(base_url)
    token_key = "max_completion_tokens" if is_openai else "max_tokens"

    body: dict[str, Any] = {
        "model": model,
        "messages": [
            {"role": "system", "content":
             "You are a rigorous evaluator. Respond ONLY with a JSON object "
             "{\"score\": <0..1 float>, \"rationale\": <string>}."},
            {"role": "user", "content": user_prompt},
        ],
        token_key: max_tokens,
        "temperature": _JUDGE_SAMPLING_FIXED["temperature"],
        "seed": int(run_idx),
    }

    headers = {"Content-Type": "application/json"}
    resolved_key = api_key if api_key is not None else os.environ.get("OPENAI_API_KEY")
    if resolved_key:
        headers["Authorization"] = f"Bearer {resolved_key}"
    elif is_openai:
        raise JudgeError("OPENAI_API_KEY not set for OpenAI judge call")

    url = f"{base_url.rstrip('/')}/chat/completions"
    body_bytes = json.dumps(body).encode("utf-8")
    timeout_s = latency_budget_ms / 1000.0

    started = time.time()
    raw_payload = _do_http_post(url, headers, body_bytes, timeout_s, _http_post)
    elapsed_ms = int((time.time() - started) * 1000)

    try:
        payload = json.loads(raw_payload.decode("utf-8"))
    except json.JSONDecodeError as exc:
        raise JudgeError(f"judge response not JSON: {exc}") from exc

    try:
        raw_msg = payload["choices"][0]["message"]["content"] or ""
    except (KeyError, IndexError, TypeError) as exc:
        raise JudgeError(f"judge response malformed: {payload!r}") from exc

    score, rationale = _parse_score(raw_msg)

    return JudgeResult(
        metric=metric,
        score=score,
        rationale=rationale,
        model=model,
        seed=int(run_idx),
        prompt_template_sha=template_sha,
        raw_response=raw_msg,
        latency_ms=elapsed_ms,
    )


def _do_http_post(
    url: str,
    headers: dict[str, str],
    body: bytes,
    timeout: float,
    override,
) -> bytes:
    """Urllib POST with simple 2-retry (1s, 2s backoff) on 429/5xx/network."""
    if override is not None:
        return override(url, headers, body, timeout)

    last_err: Exception | None = None
    for attempt in range(3):
        try:
            req = Request(url, data=body, headers=headers, method="POST")
            with urlopen(req, timeout=timeout) as resp:  # noqa: S310
                return resp.read()
        except HTTPError as e:
            last_err = e
            if e.code == 429 or 500 <= e.code < 600:
                if attempt < 2:
                    time.sleep(1 << attempt)
                    continue
            # Non-retriable HTTP (4xx other than 429) — surface the body preview.
            try:
                preview = e.read().decode("utf-8")[:300]
            except Exception:
                preview = ""
            raise JudgeError(f"HTTP {e.code} {e.reason}: {preview}") from e
        except (URLError, TimeoutError) as e:
            last_err = e
            if attempt < 2:
                time.sleep(1 << attempt)
                continue

    raise JudgeError(f"judge POST failed after retries: {last_err}")
