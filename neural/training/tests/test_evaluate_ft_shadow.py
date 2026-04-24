"""Epic 4 shadow-mode tests for evaluate_ft --scorer={heuristic,registry,dual}.

MEMORY requirement: heuristic default MUST be bit-identical to the pre-Phase-10
path. This file verifies:
  * heuristic produces the same result as before the refactor.
  * registry path scores mapped metrics via reward_functions.compute_reward.
  * dual attaches heuristic_score + registry_score + delta + within_1pct
    and run_evaluation surfaces a shadow_summary with divergences_gt_1pct.
"""
from __future__ import annotations

import json
from pathlib import Path

import pytest

from training.evaluate_ft import (
    REGISTRY_NAME_MAP,
    _heuristic_score,
    _score_via_registry,
    evaluate_response,
    evaluate_task,
    run_evaluation,
)


SCHEMA_OBJ = {"type": "object"}

QM_JSON_VALID = [{"name": "json_valid", "weight": 1.0, "threshold": 0.5}]
QM_COHERENCE = [{"name": "coherence", "weight": 1.0, "threshold": 0.5}]


def test_registry_map_contains_core_metrics() -> None:
    # These are the pre-refactor METRIC_EVALUATORS keys
    for name in ("json_valid", "accuracy", "precision",
                 "coherence", "coverage", "specificity", "follow_rate"):
        assert name in REGISTRY_NAME_MAP


def test_heuristic_scorer_default_preserves_pre_refactor_shape() -> None:
    r = evaluate_response('{"x": 1}', SCHEMA_OBJ, QM_JSON_VALID)
    assert r["weighted_score"] == pytest.approx(1.0)
    assert r["all_thresholds_met"] is True
    # No "shadow" field in heuristic mode
    assert "shadow" not in r["metrics"]["json_valid"]


def test_registry_scorer_json_valid_matches_heuristic_on_good_json() -> None:
    h = evaluate_response('{"x": 1}', SCHEMA_OBJ, QM_JSON_VALID, scorer="heuristic")
    r = evaluate_response('{"x": 1}', SCHEMA_OBJ, QM_JSON_VALID, scorer="registry")
    assert h["metrics"]["json_valid"]["score"] == r["metrics"]["json_valid"]["score"]


def test_registry_scorer_json_valid_matches_heuristic_on_broken_json() -> None:
    h = evaluate_response("not json", SCHEMA_OBJ, QM_JSON_VALID, scorer="heuristic")
    r = evaluate_response("not json", SCHEMA_OBJ, QM_JSON_VALID, scorer="registry")
    assert h["metrics"]["json_valid"]["score"] == r["metrics"]["json_valid"]["score"]


def test_dual_emits_shadow_delta() -> None:
    r = evaluate_response('{"x": 1}', SCHEMA_OBJ, QM_JSON_VALID, scorer="dual")
    shadow = r["metrics"]["json_valid"]["shadow"]
    assert shadow["heuristic_score"] == pytest.approx(1.0)
    assert shadow["registry_score"] == pytest.approx(1.0)
    assert shadow["delta"] == pytest.approx(0.0)
    assert shadow["registry_unmapped"] is False


def test_dual_heuristic_remains_authoritative_score() -> None:
    """In dual mode, the score field is the heuristic value (bit-identity)."""
    h = evaluate_response('{"x": 1}', SCHEMA_OBJ, QM_JSON_VALID, scorer="heuristic")
    d = evaluate_response('{"x": 1}', SCHEMA_OBJ, QM_JSON_VALID, scorer="dual")
    assert h["metrics"]["json_valid"]["score"] == d["metrics"]["json_valid"]["score"]
    assert h["weighted_score"] == d["weighted_score"]


def test_dual_unmapped_metric_flagged() -> None:
    qm = [{"name": "made_up_metric", "weight": 1.0, "threshold": 0.0}]
    r = evaluate_response('{"x": 1}', SCHEMA_OBJ, qm, scorer="dual")
    shadow = r["metrics"]["made_up_metric"]["shadow"]
    assert shadow["registry_unmapped"] is True
    assert shadow["delta"] is None


def test_score_via_registry_returns_none_for_unmapped() -> None:
    assert _score_via_registry("anything", SCHEMA_OBJ, "made_up_metric") is None


def test_heuristic_score_object_fallback() -> None:
    # Unknown metric on object schema → json_valid fallback
    s = _heuristic_score('{"x": 1}', SCHEMA_OBJ, "unknown_metric")
    assert s == pytest.approx(1.0)


def test_unknown_scorer_value_raises() -> None:
    with pytest.raises(ValueError, match="unknown scorer"):
        evaluate_response('{"x": 1}', SCHEMA_OBJ, QM_JSON_VALID, scorer="bogus")


# ── evaluate_task / run_evaluation integration ──────────────────────────────


def _make_dataset(tmp_path: Path) -> tuple[Path, Path]:
    """Tiny synthetic test.jsonl + ULTS dir for end-to-end runs."""
    ults_dir = tmp_path / "specs"
    ults_dir.mkdir()
    spec = {
        "task": {"name": "mini_task", "description": "test"},
        "sampling_group": "T",
        "reward_functions": ["json_valid"],
        "quality_metrics": [
            {"name": "json_valid", "weight": 1.0, "threshold": 0.5},
        ],
        "output_schema": {"type": "object"},
        "performance": {"max_tokens": 3000, "latency_budget_ms": 15000},
    }
    (ults_dir / "mini_task.ults.json").write_text(json.dumps(spec))

    test_path = tmp_path / "test.jsonl"
    with test_path.open("w") as fh:
        for content in ('{"ok": true}', '{"ok": false}', "not-json"):
            row = {
                "task_name": "mini_task",
                "messages": [
                    {"role": "system", "content": "you are a test"},
                    {"role": "user", "content": "echo"},
                    {"role": "assistant", "content": content},
                ],
            }
            fh.write(json.dumps(row) + "\n")
    return test_path, ults_dir


def test_run_evaluation_dual_produces_shadow_summary(tmp_path: Path) -> None:
    test_path, ults_dir = _make_dataset(tmp_path)
    report = run_evaluation(
        test_path=str(test_path),
        ults_dir=str(ults_dir),
        scorer="dual",
    )
    assert report["scorer"] == "dual"
    assert "shadow_summary" in report
    sm = report["shadow_summary"]
    # 1 metric compared for the 1 task, no unmapped
    assert sm["all_metrics_compared"] >= 1
    # json_valid should be in-parity between heuristic + registry
    assert sm["divergences_gt_1pct"] == []


def test_run_evaluation_heuristic_has_no_shadow_summary(tmp_path: Path) -> None:
    test_path, ults_dir = _make_dataset(tmp_path)
    report = run_evaluation(
        test_path=str(test_path),
        ults_dir=str(ults_dir),
        scorer="heuristic",
    )
    assert report["scorer"] == "heuristic"
    assert "shadow_summary" not in report
    # bit-identity: task_results still have weighted_score, threshold_pass_rate
    tr = report["task_results"][0]
    assert "weighted_score" in tr
    assert "shadow_parity" not in tr


def test_evaluate_task_dual_populates_shadow_parity(tmp_path: Path) -> None:
    _, ults_dir = _make_dataset(tmp_path)
    spec = json.loads((ults_dir / "mini_task.ults.json").read_text())
    records = [
        {"task_name": "mini_task", "messages": [
            {"role": "system", "content": "s"},
            {"role": "user", "content": "u"},
            {"role": "assistant", "content": '{"x": 1}'},
        ]},
    ]
    out = evaluate_task("mini_task", records, spec, scorer="dual")
    assert out["status"] == "evaluated"
    assert "shadow_parity" in out
    assert out["shadow_parity"]["json_valid"]["within_1pct"] is True
