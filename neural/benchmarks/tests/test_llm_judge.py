"""Unit tests for llm_judge.judge — mock OpenAI transport for determinism."""
from __future__ import annotations

import json
from pathlib import Path

import pytest

from neural.benchmarks.llm_judge import (
    _parse_score,
    JudgeError,
    judge,
)


PROMPT_DIR = Path("neural/benchmarks/judge_prompts")

BASE_CONFIG = {
    "judge": {
        "model": "gpt-5.4-mini",
        "temperature": 0.0,
        "seed_source": "run_idx",
        "max_tokens": 4000,
        "latency_budget_ms": 30000,
        "metrics": ["coherence", "depth", "relevance", "naturalness"],
        "prompt_dir": str(PROMPT_DIR),
    }
}


def _mock_success(content_json: str):
    """Return a _http_post callable that emits one fixed chat-completion payload."""
    calls: list[dict] = []

    def _post(url, headers, body, timeout):
        calls.append({
            "url": url,
            "body": json.loads(body.decode("utf-8")),
            "headers": dict(headers),
        })
        payload = {
            "choices": [{"message": {"content": content_json}}],
            "usage": {"prompt_tokens": 50, "completion_tokens": 30},
        }
        return json.dumps(payload).encode("utf-8")

    _post.calls = calls  # type: ignore[attr-defined]
    return _post


def test_success_path() -> None:
    post = _mock_success('{"score": 0.85, "rationale": "mostly coherent"}')
    r = judge(
        task_description="test task",
        task_response="the sky is blue",
        metric="coherence",
        run_idx=0,
        config=BASE_CONFIG,
        api_key="sk-test",
        _http_post=post,
    )
    assert r.metric == "coherence"
    assert r.score == pytest.approx(0.85)
    assert r.rationale == "mostly coherent"
    assert r.model == "gpt-5.4-mini"
    assert r.seed == 0
    # Body must carry temperature=0 and seed=run_idx.
    req_body = post.calls[0]["body"]  # type: ignore[attr-defined]
    assert req_body["temperature"] == 0.0
    assert req_body["seed"] == 0
    # max_completion_tokens (OpenAI) not max_tokens (MLX).
    assert req_body["max_completion_tokens"] == 4000


def test_determinism_same_seed_same_body() -> None:
    """Same (prompt, seed) → byte-identical request body."""
    post1 = _mock_success('{"score": 0.5, "rationale": "x"}')
    post2 = _mock_success('{"score": 0.5, "rationale": "x"}')
    r1 = judge("task", "resp", "coherence", run_idx=7, config=BASE_CONFIG,
               api_key="sk-test", _http_post=post1)
    r2 = judge("task", "resp", "coherence", run_idx=7, config=BASE_CONFIG,
               api_key="sk-test", _http_post=post2)
    assert post1.calls[0]["body"] == post2.calls[0]["body"]  # type: ignore[attr-defined]
    assert r1.prompt_template_sha == r2.prompt_template_sha


def test_different_seed_different_body() -> None:
    post1 = _mock_success('{"score": 0.5, "rationale": "x"}')
    post2 = _mock_success('{"score": 0.5, "rationale": "x"}')
    judge("task", "resp", "coherence", run_idx=0, config=BASE_CONFIG,
          api_key="sk-test", _http_post=post1)
    judge("task", "resp", "coherence", run_idx=1, config=BASE_CONFIG,
          api_key="sk-test", _http_post=post2)
    b1 = post1.calls[0]["body"]  # type: ignore[attr-defined]
    b2 = post2.calls[0]["body"]  # type: ignore[attr-defined]
    assert b1["seed"] != b2["seed"]


def test_forbidden_kwarg_presence_penalty() -> None:
    post = _mock_success('{"score": 0.5, "rationale": "x"}')
    with pytest.raises(JudgeError, match="forbidden"):
        judge(
            "task", "resp", "coherence",
            run_idx=0, config=BASE_CONFIG, api_key="sk-test",
            _http_post=post,
            presence_penalty=1.5,  # the J-group leak guard
        )


def test_forbidden_kwarg_top_p() -> None:
    post = _mock_success('{"score": 0.5}')
    with pytest.raises(JudgeError, match="forbidden"):
        judge("task", "resp", "coherence", run_idx=0, config=BASE_CONFIG,
              api_key="sk-test", _http_post=post, top_p=0.7)


def test_unknown_metric_raises() -> None:
    post = _mock_success('{"score": 0.5}')
    with pytest.raises(JudgeError, match="metric"):
        judge("task", "resp", "not_a_metric",
              run_idx=0, config=BASE_CONFIG, api_key="sk-test", _http_post=post)


def test_all_4_metrics_have_templates() -> None:
    for metric in ("coherence", "depth", "relevance", "naturalness"):
        path = PROMPT_DIR / f"{metric}.txt"
        assert path.exists(), f"missing {path}"
        txt = path.read_text()
        assert "{task_description}" in txt
        assert "{task_response}" in txt


def test_parse_score_json_ok() -> None:
    s, r = _parse_score('{"score": 0.75, "rationale": "good"}')
    assert s == 0.75
    assert r == "good"


def test_parse_score_code_fence_stripped() -> None:
    wrapped = '```json\n{"score": 0.33, "rationale": "ok"}\n```'
    s, r = _parse_score(wrapped)
    assert s == 0.33


def test_parse_score_fallback_regex() -> None:
    # bad JSON, but a float 0-1 appears
    s, r = _parse_score("The score is 0.6 because reasons.")
    assert s == 0.6


def test_parse_score_out_of_range_json_falls_back() -> None:
    # bad JSON score: falls back to regex scan
    s, r = _parse_score('{"score": 1.5, "rationale": "bad"}')
    # regex finds 1 (valid within [0,1]) — acceptable as safe fallback
    assert 0.0 <= s <= 1.0


def test_parse_score_unparseable_raises() -> None:
    with pytest.raises(JudgeError):
        _parse_score("no numbers here at all!!!")


def test_max_tokens_below_memory_floor_raises() -> None:
    post = _mock_success('{"score": 0.5}')
    bad = {"judge": dict(BASE_CONFIG["judge"], max_tokens=2000)}
    with pytest.raises(JudgeError, match="3000"):
        judge("task", "resp", "coherence", run_idx=0, config=bad,
              api_key="sk-test", _http_post=post)


def test_latency_below_memory_floor_raises() -> None:
    post = _mock_success('{"score": 0.5}')
    bad = {"judge": dict(BASE_CONFIG["judge"], latency_budget_ms=10000)}
    with pytest.raises(JudgeError, match="15000"):
        judge("task", "resp", "coherence", run_idx=0, config=bad,
              api_key="sk-test", _http_post=post)
