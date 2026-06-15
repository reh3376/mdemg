"""Tests for GRPO reward functions."""

import json

import pytest

from training.reward_functions import (
    REWARD_REGISTRY,
    actionability_score,
    classification_accuracy,
    coherence_score,
    compute_reward,
    coverage_score,
    evaluation_accuracy,
    explanation_quality,
    false_positive_penalty,
    format_valid,
    get_reward_function,
    insight_count,
    json_valid,
    latency_reward,
    naming_quality_score,
    schema_match,
    score_calibration,
    specificity_score,
    uniqueness_check,
    violation_detection_accuracy,
)


# ── Structural Rewards ──


class TestJsonValid:
    def test_valid_object(self):
        assert json_valid('{"type": "arch"}') == 1.0

    def test_invalid_json(self):
        assert json_valid("not json") == 0.0

    def test_array(self):
        assert json_valid("[1, 2]") == 0.0

    def test_empty(self):
        assert json_valid("") == 0.0

    def test_none(self):
        assert json_valid(None) == 0.0


class TestSchemaMatch:
    def test_full_match(self):
        schema = {
            "type": "object",
            "required": ["type"],
            "properties": {"type": {"type": "string"}},
        }
        assert schema_match('{"type": "arch"}', schema=schema) == 1.0

    def test_missing_key(self):
        schema = {"type": "object", "required": ["type", "summary"]}
        score = schema_match('{"type": "arch"}', schema=schema)
        assert 0.0 < score < 1.0  # Partial credit

    def test_wrong_type(self):
        schema = {
            "type": "object",
            "required": ["count"],
            "properties": {"count": {"type": "integer"}},
        }
        score = schema_match('{"count": "five"}', schema=schema)
        assert score == 0.5  # Key present but wrong type

    def test_string_schema(self):
        schema = {"type": "string"}
        assert schema_match("hello", schema=schema) == 1.0

    def test_no_schema(self):
        assert schema_match('{"x": 1}') == 1.0  # Falls back to json_valid


class TestFormatValid:
    def test_valid_json(self):
        assert format_valid('{"x": 1}') == 1.0

    def test_valid_text(self):
        assert format_valid("plain text") == 1.0

    def test_empty(self):
        assert format_valid("") == 0.0

    def test_malformed_json(self):
        assert format_valid('{"x":') == 0.0


# ── Classification Rewards ──


class TestClassificationAccuracy:
    def test_correct(self):
        assert classification_accuracy(
            '{"type": "architecture"}', expected="architecture"
        ) == 1.0

    def test_incorrect(self):
        assert classification_accuracy(
            '{"type": "security"}', expected="architecture"
        ) == 0.0

    def test_case_insensitive(self):
        assert classification_accuracy(
            '{"type": "Architecture"}', expected="architecture"
        ) == 1.0

    def test_no_expected(self):
        # Without ground truth, falls back to json_valid
        assert classification_accuracy('{"type": "x"}') == 1.0

    def test_expected_as_json_string_same_shape(self):
        """Regression: golden-row assistant message is the full JSON payload,
        not the bare label. `expected` of `{"type":"none","summary":""}` must
        match a model response producing the same JSON (including whitespace).
        """
        expected_json = '{"type":"none","summary":""}'
        assert classification_accuracy(
            '\n\n{"type":"none","summary":""}', expected=expected_json
        ) == 1.0

    def test_expected_json_mismatch_scores_zero(self):
        expected_json = '{"type":"architecture","summary":""}'
        assert classification_accuracy(
            '{"type":"security","summary":""}', expected=expected_json
        ) == 0.0

    def test_plural_types_list_extracts_first(self):
        """retrieval.query_classify uses {"types": ["symbol_lookup"], ...}."""
        expected_json = '{"types":["symbol_lookup"],"temporal":"none"}'
        assert classification_accuracy(
            '{"types":["symbol_lookup"],"temporal":"none"}',
            expected=expected_json,
        ) == 1.0
        assert classification_accuracy(
            '{"types":["generic"],"temporal":"none"}', expected=expected_json
        ) == 0.0

    def test_empty_expected_falls_back_to_json_valid(self):
        assert classification_accuracy('{"type": "x"}', expected="") == 1.0


class TestEvaluationAccuracy:
    def test_correct_verdict(self):
        assert evaluation_accuracy(
            '{"verdict": "positive"}', expected_verdict="positive"
        ) == 1.0

    def test_wrong_verdict(self):
        assert evaluation_accuracy(
            '{"verdict": "negative"}', expected_verdict="positive"
        ) == 0.0

    def test_expected_kwarg_alias_as_json_string(self):
        """Regression: runner passes `expected=<assistant_json>`, not
        `expected_verdict=<bare_label>`. Both paths must work.
        """
        expected_json = '{"outcome":"ignored","confidence":0.9}'
        assert evaluation_accuracy(
            '{"outcome":"ignored","confidence":0.8}', expected=expected_json
        ) == 1.0
        assert evaluation_accuracy(
            '{"outcome":"followed","confidence":0.8}', expected=expected_json
        ) == 0.0

    def test_empty_expected_falls_back_to_json_valid(self):
        assert evaluation_accuracy('{"verdict": "x"}', expected="") == 1.0


# ── Quality Rewards ──


class TestCoherenceScore:
    # REWARD-CORRECTNESS-001: length-neutral. A coherent answer scores high
    # regardless of length; only empty or pure-repetition scores low.
    def test_good_text(self):
        text = "The module handles authentication. It validates tokens and manages sessions."
        score = coherence_score(text)
        assert score > 0.5

    def test_empty(self):
        assert coherence_score("") == 0.0

    def test_single_word_is_coherent(self):
        # A correct terse answer is NOT penalized for brevity (was capped ≤0.7).
        assert coherence_score("yes") >= 0.8

    def test_terse_not_below_verbose(self):
        terse = "Validate the token before use."
        verbose = " ".join(["The system validates each token carefully and thoroughly."] * 8)
        assert coherence_score(terse) >= coherence_score(verbose)

    def test_pure_repetition_penalized(self):
        assert coherence_score(" ".join(["spam"] * 30)) < 0.5


class TestCoverageScore:
    # REWARD-CORRECTNESS-001: substantiveness, length-neutral. Verbosity is
    # not rewarded upward; a substantive concise answer floors near the top.
    def test_substantive_concise_scores_high(self):
        # A correct ~10-word answer must clear the 0.8 distill gate.
        text = "Validate the auth token, then refresh the session if expired."
        assert coverage_score(text) >= 0.8

    def test_verbosity_not_rewarded_above_concise(self):
        concise = "Validate the auth token before refreshing the session."
        verbose = " ".join(
            ["The auth token must be validated before the session is refreshed."] * 10
        )
        assert coverage_score(verbose) <= coverage_score(concise)

    def test_minimal(self):
        # Genuinely content-free (1 word) stays low.
        assert coverage_score("ok") < 0.5

    def test_pure_repetition_penalized(self):
        assert coverage_score(" ".join(["word"] * 60)) < 0.5

    def test_empty(self):
        assert coverage_score("") == 0.0


class TestSpecificityScore:
    # REWARD-CORRECTNESS-002: substantive-floored. Valid concise guidance floors
    # at 0.7; specific words bonus toward 1.0; hedging/empty/repetition stay low.
    def test_specific_beats_generic(self):
        specific = "You must always validate input because injection is possible."
        generic = "In general, it depends on various ways of doing things."
        assert specificity_score(specific) > specificity_score(generic)

    def test_substantive_concise_floors_high(self):
        # Valid guidance lacking the magic words is no longer dropped to 0.5.
        assert specificity_score("Validate the auth token before refreshing the session.") >= 0.7

    def test_hedging_penalized(self):
        assert specificity_score("In general, it depends on various ways of doing things.") < 0.5

    def test_empty_zero(self):
        assert specificity_score("") == 0.0


class TestActionabilityScore:
    # REWARD-CORRECTNESS-002: substantive-floored (was 0.3 base).
    def test_actionable_high(self):
        text = "You should implement retry logic and add validation."
        assert actionability_score(text) > 0.8

    def test_substantive_floors_high(self):
        # A valid substantive insight without action verbs is not punished.
        assert actionability_score("The system is interesting.") >= 0.7

    def test_actionable_beats_passive(self):
        assert actionability_score("You should refactor and add validation.") >= actionability_score(
            "The system is interesting."
        )

    def test_empty_zero(self):
        assert actionability_score("") == 0.0


class TestInsightCount:
    # REWARD-CORRECTNESS-001: do NOT reward insight COUNT upward — one genuine
    # insight is a complete correct answer; more is not "better" for inclusion.
    def test_single_insight_scores_high(self):
        assert insight_count("The retry logic must back off exponentially.") >= 0.8

    def test_count_not_rewarded_upward(self):
        one = "- Only point"
        five = "- One\n- Two\n- Three\n- Four\n- Five"
        assert insight_count(one) == insight_count(five)

    def test_empty_low(self):
        assert insight_count("") < 0.5


class TestExplanationQuality:
    # REWARD-CORRECTNESS-001: length-neutral; concise correct explanation scores high.
    def test_good_explanation(self):
        resp = '{"explanation": "The developer followed backup procedures correctly before migration."}'
        assert explanation_quality(resp) > 0.5

    def test_concise_explanation_scores_high(self):
        assert explanation_quality('{"explanation": "Backup ran before migration."}') >= 0.8

    def test_no_explanation(self):
        assert explanation_quality('{"verdict": "ok"}') == 0.0

    # REWARD-CORRECTNESS-002: schema-aware — jiminy.evaluate nests reasoning.
    def test_nested_violation_reasoning_credited(self):
        resp = (
            '{"violations": [{"constraint_node_id": "n1", "description": "d", '
            '"reasoning": "The change deletes data without a backup step.", '
            '"remediation": "Add a backup."}], "warnings": []}'
        )
        assert explanation_quality(resp) >= 0.8

    def test_clean_verdict_not_penalized(self):
        # A correct "no issues" verdict has nothing to explain — must not score 0.
        assert explanation_quality('{"violations": [], "warnings": []}') >= 0.8

    def test_malformed_object_low(self):
        assert explanation_quality('{"foo": "bar"}') == 0.0


class TestFollowRateInheritsFloor:
    # REWARD-CORRECTNESS-002: follow_rate = mean(specificity, actionability);
    # inherits the substantive floor so valid guidance isn't dropped.
    def test_substantive_guidance_floors(self):
        from neural.training.reward_functions import follow_rate
        assert follow_rate("Validate the auth token before refreshing the session.") >= 0.7

    def test_empty_zero(self):
        from neural.training.reward_functions import follow_rate
        assert follow_rate("") == 0.0


class TestNamingQualityScore:
    def test_good_name(self):
        assert naming_quality_score('{"name": "Resource Pool Management"}') > 0.8

    def test_single_word(self):
        assert naming_quality_score('{"name": "pooling"}') == 0.5


class TestUniquenessCheck:
    def test_unique(self):
        text = "The authentication module validates JWT tokens and manages user sessions securely."
        assert uniqueness_check(text) > 0.5

    def test_repetitive(self):
        text = "test " * 20
        assert uniqueness_check(text) < 0.5


# ── Ranking Rewards ──


class TestScoreCalibration:
    def test_calibrated(self):
        assert score_calibration("[0.9, 0.5, 0.1]") == 1.0

    def test_out_of_range(self):
        assert score_calibration("[0.9, 1.5, -0.1]") == pytest.approx(1 / 3)

    def test_invalid(self):
        assert score_calibration("not scores") == 0.0


# ── Guardrail Rewards ──


class TestViolationDetectionAccuracy:
    def _mk(self, ids: list[str]) -> str:
        violations = [
            {"constraint_node_id": i, "description": "d", "rationale": "r"}
            for i in ids
        ]
        return json.dumps({"violations": violations, "warnings": []})

    def test_exact_match(self):
        resp = self._mk(["c1", "c2"])
        exp = self._mk(["c2", "c1"])
        assert violation_detection_accuracy(resp, expected=exp) == 1.0

    def test_both_clean(self):
        resp = self._mk([])
        exp = self._mk([])
        assert violation_detection_accuracy(resp, expected=exp) == 1.0

    def test_missed_violation(self):
        resp = self._mk([])
        exp = self._mk(["c1"])
        assert violation_detection_accuracy(resp, expected=exp) == 0.0

    def test_fabricated_violation(self):
        resp = self._mk(["c1"])
        exp = self._mk([])
        assert violation_detection_accuracy(resp, expected=exp) == 0.0

    def test_partial_overlap_f1(self):
        # pred={c1,c2,c3}, gold={c2,c3,c4} → tp=2, fp=1, fn=1 → F1 = 2/3
        resp = self._mk(["c1", "c2", "c3"])
        exp = self._mk(["c2", "c3", "c4"])
        assert violation_detection_accuracy(resp, expected=exp) == pytest.approx(
            2 / 3
        )

    def test_invalid_json_response(self):
        exp = self._mk(["c1"])
        assert violation_detection_accuracy("not json", expected=exp) == 0.0

    def test_invalid_json_expected(self):
        resp = self._mk(["c1"])
        assert violation_detection_accuracy(resp, expected="not json") == 0.0

    def test_no_expected_falls_back_to_json_valid(self):
        resp = self._mk(["c1"])
        # Valid JSON object → json_valid fallback returns 1.0
        assert violation_detection_accuracy(resp, expected=None) == 1.0

    def test_malformed_violations_treated_as_empty(self):
        # violations is not a list → treat as empty; gold has one → 0.0
        resp = json.dumps({"violations": "oops", "warnings": []})
        exp = self._mk(["c1"])
        assert violation_detection_accuracy(resp, expected=exp) == 0.0


class TestFalsePositivePenalty:
    def _mk(self, ids: list[str]) -> str:
        violations = [
            {"constraint_node_id": i, "description": "d", "rationale": "r"}
            for i in ids
        ]
        return json.dumps({"violations": violations, "warnings": []})

    def test_no_false_positives(self):
        resp = self._mk(["c1", "c2"])
        exp = self._mk(["c1", "c2"])
        assert false_positive_penalty(resp, expected=exp) == 1.0

    def test_no_predictions_no_penalty(self):
        resp = self._mk([])
        exp = self._mk(["c1"])  # missed violation, but no FPs
        assert false_positive_penalty(resp, expected=exp) == 1.0

    def test_all_false_positives(self):
        # pred={c1,c2}, gold={} → 2 FPs / 2 preds → 1 - 1.0 = 0.0
        resp = self._mk(["c1", "c2"])
        exp = self._mk([])
        assert false_positive_penalty(resp, expected=exp) == 0.0

    def test_half_false_positives(self):
        # pred={c1,c2}, gold={c1} → 1 FP / 2 preds → 0.5
        resp = self._mk(["c1", "c2"])
        exp = self._mk(["c1"])
        assert false_positive_penalty(resp, expected=exp) == pytest.approx(0.5)

    def test_invalid_json_response(self):
        exp = self._mk(["c1"])
        assert false_positive_penalty("not json", expected=exp) == 0.0

    def test_no_expected_falls_back_to_json_valid(self):
        resp = self._mk(["c1"])
        assert false_positive_penalty(resp, expected=None) == 1.0


# ── Performance Reward ──


class TestLatencyReward:
    def test_fast(self):
        # 100ms vs 3000ms budget — should be near 1.0
        score = latency_reward("", latency_ms=100, budget_ms=3000)
        assert score > 0.95

    def test_at_budget(self):
        # At budget, sigmoid gives 0.5
        score = latency_reward("", latency_ms=3000, budget_ms=3000)
        assert score == pytest.approx(0.5, abs=0.01)

    def test_slow(self):
        # 10000ms vs 3000ms budget — should be near 0.0
        score = latency_reward("", latency_ms=10000, budget_ms=3000)
        assert score < 0.05

    def test_zero_budget(self):
        assert latency_reward("", latency_ms=100, budget_ms=0) == 1.0


# ── Registry ──


class TestRegistry:
    def test_all_20_functions_registered(self):
        # 18 original + violation_detection_accuracy + false_positive_penalty
        assert len(REWARD_REGISTRY) >= 20

    def test_guardrail_rewards_registered(self):
        assert "violation_detection_accuracy" in REWARD_REGISTRY
        assert "false_positive_penalty" in REWARD_REGISTRY

    def test_get_known(self):
        fn = get_reward_function("json_valid")
        assert fn is not None
        assert fn('{"x": 1}') == 1.0

    def test_get_unknown(self):
        assert get_reward_function("nonexistent") is None

    def test_compute_reward(self):
        scores = compute_reward(
            '{"type": "arch"}',
            ["json_valid", "format_valid"],
        )
        assert scores["json_valid"] == 1.0
        assert scores["format_valid"] == 1.0

    def test_compute_unknown_reward(self):
        scores = compute_reward("test", ["nonexistent"])
        assert scores["nonexistent"] == 0.0
