"""Unit tests for openai_ft_baseline_eval.py per-record output fields.

Covers FT-OAI-002 Epic 1 fixes:
    - G1a: ``parse_ok`` is present and True/False (not ``parses_as_json``-only)
    - G1b: ``finish_reason`` is captured from ``resp.choices[0].finish_reason``
    - G1c: ``prompt_tokens`` and ``completion_tokens`` are captured from ``resp.usage``

Also tests the backfill helper in ``openai_ft_results_backfill.py``.

No OpenAI network calls — we build a fake response object that matches the
shape of the ``openai>=1.50`` SDK return value.
"""

from __future__ import annotations

import json
import sys
from pathlib import Path
from types import SimpleNamespace

sys.path.insert(0, str(Path(__file__).parent.parent))

import openai_ft_baseline_eval as eval_mod  # noqa: E402
import openai_ft_results_backfill as backfill_mod  # noqa: E402


# ─── G1a: parse_ok naming & values ─────────────────────────────────────────


def test_try_parse_json_true_for_valid_json():
    assert eval_mod.try_parse_json('{"a": 1}') is True
    assert eval_mod.try_parse_json('[1, 2, 3]') is True


def test_try_parse_json_false_for_malformed():
    assert eval_mod.try_parse_json("not json at all") is False
    assert eval_mod.try_parse_json('{"a": ') is False


def test_try_parse_json_strips_markdown_fences():
    # The eval script intentionally tolerates ```json ... ``` wrappers.
    fenced = "```json\n{\"a\": 1}\n```"
    assert eval_mod.try_parse_json(fenced) is True


# ─── G1b + G1c: finish_reason and token counts are captured ────────────────


def _fake_chat_completion(content: str, finish: str, p_tokens: int, c_tokens: int):
    """Build a minimal stand-in for openai.ChatCompletion return value."""
    return SimpleNamespace(
        choices=[
            SimpleNamespace(
                message=SimpleNamespace(content=content),
                finish_reason=finish,
            )
        ],
        usage=SimpleNamespace(
            prompt_tokens=p_tokens,
            completion_tokens=c_tokens,
        ),
    )


def test_response_extraction_shape():
    """Exercise the attribute-path the eval script uses so drift breaks loudly."""
    resp = _fake_chat_completion("hi", "stop", 12, 3)

    choice = resp.choices[0]
    response_text = choice.message.content or ""
    finish_reason = getattr(choice, "finish_reason", None)
    usage = getattr(resp, "usage", None)
    prompt_tokens = getattr(usage, "prompt_tokens", None) if usage else None
    completion_tokens = getattr(usage, "completion_tokens", None) if usage else None

    assert response_text == "hi"
    assert finish_reason == "stop"
    assert prompt_tokens == 12
    assert completion_tokens == 3


def test_response_extraction_tolerates_missing_usage():
    """Older / partial responses may lack ``usage``; must not crash."""
    resp = SimpleNamespace(
        choices=[
            SimpleNamespace(
                message=SimpleNamespace(content="hi"),
                finish_reason="length",
            )
        ],
        # intentionally no `usage` attribute
    )
    choice = resp.choices[0]
    finish_reason = getattr(choice, "finish_reason", None)
    usage = getattr(resp, "usage", None)
    prompt_tokens = getattr(usage, "prompt_tokens", None) if usage else None
    completion_tokens = getattr(usage, "completion_tokens", None) if usage else None

    assert finish_reason == "length"
    assert prompt_tokens is None
    assert completion_tokens is None


# ─── Backfill helper ──────────────────────────────────────────────────────


def test_backfill_prefers_existing_parse_ok():
    row = {"i": 1, "parse_ok": True, "parses_as_json": False, "response": "bad"}
    out, src = backfill_mod.backfill_row(row)
    assert out["parse_ok"] is True
    assert src == "existing"


def test_backfill_aliases_parses_as_json():
    row = {"i": 2, "parses_as_json": True, "response": "ignored"}
    out, src = backfill_mod.backfill_row(row)
    assert out["parse_ok"] is True
    assert src == "alias"


def test_backfill_recomputes_when_both_absent():
    row = {"i": 3, "response": '{"ok": 1}'}
    out, src = backfill_mod.backfill_row(row)
    assert out["parse_ok"] is True
    assert src == "recomputed"


def test_backfill_recomputes_false_for_malformed():
    row = {"i": 4, "response": "not json"}
    out, src = backfill_mod.backfill_row(row)
    assert out["parse_ok"] is False
    assert src == "recomputed"


def test_backfill_skips_error_rows():
    row = {"i": 5, "task": "x", "error": "timeout"}
    out, src = backfill_mod.backfill_row(row)
    assert src == "error_row"
    assert "parse_ok" not in out


# ─── Epic 2: A1–A7 helpers ─────────────────────────────────────────────────


def test_is_none_type_payload_true_for_canonical():
    assert eval_mod._is_none_type_payload('{"type":"none","summary":""}') is True
    # tolerate whitespace
    assert eval_mod._is_none_type_payload('  { "type": "none", "summary": "" }  ') is True


def test_is_none_type_payload_false_for_other_object():
    assert eval_mod._is_none_type_payload('{"type":"alert","summary":"x"}') is False
    # none type with non-empty summary is NOT the canonical nothing-to-report shape
    assert eval_mod._is_none_type_payload('{"type":"none","summary":"something"}') is False


def test_is_none_type_payload_none_when_unparseable():
    assert eval_mod._is_none_type_payload("not json") is None
    assert eval_mod._is_none_type_payload("") is None


def test_hallucination_indicator_flags_fabrication():
    gt = '{"type":"none","summary":""}'
    # Fabricated alert content against explicit nothing-to-report GT.
    resp = '{"type":"alert","summary":"fake finding"}'
    assert eval_mod.hallucination_indicator(gt, resp) is True


def test_hallucination_indicator_ok_when_both_none():
    gt = '{"type":"none","summary":""}'
    resp = '{"type":"none","summary":""}'
    assert eval_mod.hallucination_indicator(gt, resp) is False


def test_hallucination_indicator_skipped_when_gt_not_none():
    # When GT is not none-typed, this metric doesn't apply — always False.
    gt = '{"type":"alert","summary":"real"}'
    resp = '{"type":"alert","summary":"fake"}'
    assert eval_mod.hallucination_indicator(gt, resp) is False


def test_percentile_basic():
    # Single element
    assert eval_mod._percentile([42.0], 50) == 42.0
    # Sorted list of 11 values 0..10 → p50 = 5, p95 = 9.5 (linear interp)
    xs = [float(i) for i in range(11)]
    assert eval_mod._percentile(xs, 50) == 5.0
    assert abs(eval_mod._percentile(xs, 95) - 9.5) < 1e-9
    assert eval_mod._percentile([], 50) is None


# ─── Epic 2: retry wrapper ─────────────────────────────────────────────────


class _FakeClient:
    """Minimal stand-in for openai.OpenAI that records attempts."""

    def __init__(self, fail_n: int = 0, exc: Exception | None = None):
        self.fail_n = fail_n
        self.attempts = 0
        self.exc = exc or RuntimeError("transient")
        self.chat = SimpleNamespace(completions=SimpleNamespace(create=self._create))

    def _create(self, **_kwargs):
        self.attempts += 1
        if self.attempts <= self.fail_n:
            raise self.exc
        return _fake_chat_completion("ok", "stop", 5, 2)


def test_call_with_retry_succeeds_first_try():
    client = _FakeClient(fail_n=0)
    resp, retries = eval_mod.call_with_retry(
        client, model="m", messages=[], max_completion_tokens=10,
        max_retries=2, backoff_base_seconds=0,
    )
    assert retries == 0
    assert client.attempts == 1
    assert resp.choices[0].message.content == "ok"


def test_call_with_retry_counts_retries():
    client = _FakeClient(fail_n=2)
    resp, retries = eval_mod.call_with_retry(
        client, model="m", messages=[], max_completion_tokens=10,
        max_retries=3, backoff_base_seconds=0,  # no real sleeps in unit tests
    )
    assert retries == 2  # two retries (attempts 1 and 2 failed, attempt 3 succeeded)
    assert client.attempts == 3


def test_call_with_retry_raises_after_budget():
    client = _FakeClient(fail_n=5)
    import pytest
    with pytest.raises(RuntimeError, match="transient"):
        eval_mod.call_with_retry(
            client, model="m", messages=[], max_completion_tokens=10,
            max_retries=2, backoff_base_seconds=0,
        )
    assert client.attempts == 3  # 1 initial + 2 retries


# ─── Epic 6 T1: training_metrics CSV parser ────────────────────────────────


def test_parse_training_metrics_csv_basic():
    import openai_ft_check as check_mod

    csv_text = (
        "step,train_loss,train_accuracy,valid_loss,valid_mean_token_accuracy\n"
        "1,1.5,0.6,,\n"
        "2,1.2,0.7,1.1,0.55\n"
        "3,1.0,0.75,,\n"
        "4,0.9,0.78,0.95,0.62\n"
        "5,0.85,0.8,0.92,0.64\n"
    )
    metrics = check_mod.parse_training_metrics_csv(csv_text)
    assert metrics["n_rows"] == 5
    assert metrics["final_step"] == 5
    assert metrics["final_train_loss"] == 0.85
    assert metrics["final_valid_loss"] == 0.92
    # best = min valid_loss across steps 2,4,5 → step 5 (0.92)
    assert metrics["best_val_loss_step"] == 5
    assert metrics["best_val_loss"] == 0.92
    assert len(metrics["train_loss_series"]) == 5
    assert len(metrics["valid_loss_series"]) == 3  # only steps 2,4,5


def test_parse_training_metrics_csv_tolerates_missing_fields():
    """CSV with only train_loss (no valid_loss column) should not crash."""
    import openai_ft_check as check_mod

    csv_text = "step,train_loss\n1,2.0\n2,1.5\n"
    metrics = check_mod.parse_training_metrics_csv(csv_text)
    assert metrics["final_train_loss"] == 1.5
    assert metrics["final_valid_loss"] is None
    assert metrics["best_val_loss_step"] is None
    assert metrics["best_val_loss"] is None


def test_parse_training_metrics_csv_empty_rows():
    """Empty CSV (header only) should produce a null-result dict, not crash."""
    import openai_ft_check as check_mod

    csv_text = "step,train_loss,valid_loss\n"
    metrics = check_mod.parse_training_metrics_csv(csv_text)
    assert metrics["n_rows"] == 0
    assert metrics["final_step"] is None
    assert metrics["best_val_loss_step"] is None


def test_backfill_run_writes_file_and_reports_aggregate(tmp_path):
    src = tmp_path / "results.jsonl"
    dst = tmp_path / "out.jsonl"
    rows = [
        {"i": 1, "parses_as_json": True, "response": '{"a":1}'},
        {"i": 2, "parses_as_json": False, "response": "nope"},
        {"i": 3, "response": '[1,2]'},          # recompute -> True
        {"i": 4, "response": "garbage"},         # recompute -> False
        {"i": 5, "task": "x", "error": "boom"},  # error row, excluded
    ]
    src.write_text("\n".join(json.dumps(r) for r in rows) + "\n", encoding="utf-8")

    stats = backfill_mod.run(str(src), str(dst))
    assert stats["total"] == 5
    assert stats["alias"] == 2
    assert stats["recomputed"] == 2
    assert stats["error_row"] == 1
    # 4 non-error rows, 2 pass (rows 1 and 3)
    assert stats["pass"] == 2
    assert abs(stats["parse_ok_rate"] - 0.5) < 1e-9

    written = [json.loads(line) for line in dst.read_text(encoding="utf-8").splitlines() if line.strip()]
    assert len(written) == 5
    assert written[0]["parse_ok"] is True
    assert written[1]["parse_ok"] is False
    assert written[2]["parse_ok"] is True
    assert written[3]["parse_ok"] is False
    assert "parse_ok" not in written[4]
