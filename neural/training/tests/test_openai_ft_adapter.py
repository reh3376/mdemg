#!/usr/bin/env python3
"""Tests for the OpenAI fine-tuning post-processor adapter."""

from __future__ import annotations

import json
import os
import tempfile
import unittest

from training.openai_ft_adapter import (
    _MODEL_PROFILES,
    build_manifest,
    count_tokens,
    get_profile,
    process_record,
    run,
    strip_think_blocks,
    validate_openai_record,
)


class _FakeEncoder:
    """Deterministic stub encoder: 1 token per word. Avoids network/tiktoken."""

    name = "fake-encoder"

    def encode(self, text: str) -> list[int]:
        if not text:
            return []
        return [0] * len(text.split())


def _record(system="sys prompt", user="user prompt", assistant="response"):
    return {
        "messages": [
            {"role": "system", "content": system},
            {"role": "user", "content": user},
            {"role": "assistant", "content": assistant},
        ]
    }


class TestStripThinkBlocks(unittest.TestCase):
    def test_removes_single_block(self):
        text = "before <think>secret reasoning</think> after"
        # Regex eats trailing whitespace after the block, outer .strip() trims ends.
        self.assertEqual(strip_think_blocks(text), "before after")

    def test_removes_multiline_block(self):
        text = "ok <think>\nline one\nline two\n</think>\nfinal"
        # Regex eats the newline after </think>; leaves "ok " + "final".
        self.assertEqual(strip_think_blocks(text), "ok final")

    def test_removes_multiple_blocks(self):
        text = "<think>a</think>one<think>b</think>two"
        self.assertEqual(strip_think_blocks(text), "onetwo")

    def test_preserves_content_without_think_blocks(self):
        text = "plain response with no think block"
        self.assertEqual(strip_think_blocks(text), text)

    def test_entirely_think_returns_empty(self):
        text = "<think>only reasoning</think>"
        self.assertEqual(strip_think_blocks(text), "")


class TestValidateOpenAIRecord(unittest.TestCase):
    def test_valid_record(self):
        ok, reason = validate_openai_record(_record())
        self.assertTrue(ok, reason)

    def test_missing_messages_rejected(self):
        ok, reason = validate_openai_record({})
        self.assertFalse(ok)
        self.assertIn("messages", reason)

    def test_bad_role_rejected(self):
        rec = _record()
        rec["messages"][0]["role"] = "tool"
        ok, reason = validate_openai_record(rec)
        self.assertFalse(ok)
        self.assertIn("role", reason)

    def test_empty_content_rejected(self):
        rec = _record(assistant="   ")
        ok, reason = validate_openai_record(rec)
        self.assertFalse(ok)
        self.assertIn("empty", reason)

    def test_no_assistant_message_rejected(self):
        rec = {
            "messages": [
                {"role": "system", "content": "a"},
                {"role": "user", "content": "b"},
            ]
        }
        ok, reason = validate_openai_record(rec)
        self.assertFalse(ok)
        self.assertIn("assistant", reason)


class TestProcessRecord(unittest.TestCase):
    def setUp(self):
        self.profile = _MODEL_PROFILES["gpt-4.1-mini-2025-04-14"]
        self.encoder = _FakeEncoder()

    def test_strips_think_and_keeps_record(self):
        rec = _record(assistant="<think>scratch</think>final answer")
        result = process_record(rec, self.profile, self.encoder)
        self.assertTrue(result.ok, result.reason)
        self.assertEqual(result.record["messages"][-1]["content"], "final answer")
        self.assertGreater(result.tokens, 0)

    def test_rejects_assistant_entirely_think(self):
        rec = _record(assistant="<think>only reasoning here</think>")
        result = process_record(rec, self.profile, self.encoder)
        self.assertFalse(result.ok)
        self.assertIn("assistant", result.reason)

    def test_rejects_over_context_limit(self):
        # Force the limit low to trigger over-limit.
        import dataclasses
        profile = dataclasses.replace(self.profile, context_limit=5)
        rec = _record(
            system="a b c d",
            user="e f g h",
            assistant="i j k l m n o",
        )
        result = process_record(rec, profile, self.encoder)
        self.assertFalse(result.ok)
        self.assertIn("over-limit", result.reason)


class TestCountTokens(unittest.TestCase):
    def test_includes_per_message_overhead(self):
        encoder = _FakeEncoder()
        msgs = [
            {"role": "system", "content": "a b c"},  # 3 content + 4 overhead = 7
            {"role": "user", "content": "d"},  # 1 + 4 = 5
        ]
        # 7 + 5 + 2 priming = 14
        self.assertEqual(count_tokens(msgs, encoder), 14)


class TestGetProfile(unittest.TestCase):
    def test_known_model(self):
        p = get_profile("gpt-4.1-mini-2025-04-14")
        self.assertEqual(p.name, "gpt-4.1-mini-2025-04-14")
        self.assertEqual(p.context_limit, 131072)

    def test_unknown_model_raises(self):
        with self.assertRaises(ValueError):
            get_profile("nonexistent-model")


class TestRunEndToEnd(unittest.TestCase):
    """End-to-end: write synthetic train/val inputs, run the adapter, assert outputs."""

    def _write_jsonl(self, path, records):
        with open(path, "w", encoding="utf-8") as f:
            for r in records:
                f.write(json.dumps(r) + "\n")

    def test_happy_path_produces_expected_artifacts(self):
        with tempfile.TemporaryDirectory() as tmp:
            in_dir = os.path.join(tmp, "in")
            out_dir = os.path.join(tmp, "out")
            os.makedirs(in_dir)
            train = [
                _record(assistant=f"<think>scratch {i}</think>answer {i}")
                for i in range(5)
            ]
            val = [_record(assistant=f"answer {i}") for i in range(2)]
            self._write_jsonl(os.path.join(in_dir, "train.jsonl"), train)
            self._write_jsonl(os.path.join(in_dir, "val.jsonl"), val)

            # Monkey-patch profile to avoid real tiktoken network I/O.
            import training.openai_ft_adapter as mod

            fake_profile = mod.ModelProfile(
                name="test-model",
                encoder_resolver=lambda: _FakeEncoder(),
                context_limit=1000,
                price_per_1m_train_usd=3.00,
            )
            mod._MODEL_PROFILES["test-model"] = fake_profile
            try:
                manifest = run(
                    input_dir=in_dir,
                    output_dir=out_dir,
                    model="test-model",
                    by_task=False,
                )
            finally:
                mod._MODEL_PROFILES.pop("test-model", None)

            # Outputs exist
            self.assertTrue(os.path.exists(os.path.join(out_dir, "combined_train.jsonl")))
            self.assertTrue(os.path.exists(os.path.join(out_dir, "combined_val.jsonl")))
            self.assertTrue(os.path.exists(os.path.join(out_dir, "manifest.json")))
            self.assertTrue(os.path.exists(os.path.join(out_dir, "rejection_log.jsonl")))

            # Manifest shape
            self.assertEqual(manifest["splits"]["train"]["rows"], 5)
            self.assertEqual(manifest["splits"]["val"]["rows"], 2)
            self.assertEqual(manifest["totals"]["rejected"], 0)
            self.assertGreater(manifest["totals"]["tokens"], 0)
            self.assertGreater(manifest["totals"]["cost_estimate_usd"], 0)

            # Think blocks stripped in output
            with open(os.path.join(out_dir, "combined_train.jsonl"), encoding="utf-8") as f:
                first = json.loads(f.readline())
            self.assertNotIn("<think>", first["messages"][-1]["content"])

    def test_missing_input_raises(self):
        with tempfile.TemporaryDirectory() as tmp:
            in_dir = os.path.join(tmp, "in")
            os.makedirs(in_dir)
            # Only train, no val
            self._write_jsonl(os.path.join(in_dir, "train.jsonl"), [_record()])
            with self.assertRaises(FileNotFoundError):
                run(input_dir=in_dir, output_dir=os.path.join(tmp, "out"),
                    model="gpt-4.1-mini-2025-04-14")


class TestSubsampling(unittest.TestCase):
    """Verify --max-train-rows / --max-val-rows / --sample-seed flags."""

    def _write_jsonl(self, path, records):
        with open(path, "w", encoding="utf-8") as f:
            for r in records:
                f.write(json.dumps(r) + "\n")

    def _run_with_subsample(self, tmp, max_train, seed):
        import training.openai_ft_adapter as mod

        in_dir = os.path.join(tmp, "in")
        out_dir = os.path.join(tmp, "out")
        os.makedirs(in_dir)
        train = [_record(assistant=f"answer {i}") for i in range(100)]
        val = [_record(assistant=f"val {i}") for i in range(20)]
        self._write_jsonl(os.path.join(in_dir, "train.jsonl"), train)
        self._write_jsonl(os.path.join(in_dir, "val.jsonl"), val)

        mod._MODEL_PROFILES["test-model"] = mod.ModelProfile(
            name="test-model",
            encoder_resolver=lambda: _FakeEncoder(),
            context_limit=1000,
            price_per_1m_train_usd=3.00,
        )
        try:
            manifest = mod.run(
                input_dir=in_dir,
                output_dir=out_dir,
                model="test-model",
                max_train_rows=max_train,
                sample_seed=seed,
            )
        finally:
            mod._MODEL_PROFILES.pop("test-model", None)
        with open(os.path.join(out_dir, "combined_train.jsonl"), encoding="utf-8") as f:
            contents = [json.loads(line) for line in f if line.strip()]
        return manifest, contents

    def test_respects_max_train_rows(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest, contents = self._run_with_subsample(tmp, 10, 42)
            self.assertEqual(manifest["splits"]["train"]["rows"], 10)
            self.assertEqual(len(contents), 10)
            self.assertTrue(manifest["subsample"]["applied"])
            self.assertEqual(manifest["subsample"]["max_train_rows"], 10)
            self.assertEqual(manifest["subsample"]["seed"], 42)

    def test_same_seed_produces_same_sample(self):
        with tempfile.TemporaryDirectory() as a, tempfile.TemporaryDirectory() as b:
            _, c1 = self._run_with_subsample(a, 10, 42)
            _, c2 = self._run_with_subsample(b, 10, 42)
            self.assertEqual(
                [r["messages"][-1]["content"] for r in c1],
                [r["messages"][-1]["content"] for r in c2],
            )

    def test_different_seed_produces_different_sample(self):
        with tempfile.TemporaryDirectory() as a, tempfile.TemporaryDirectory() as b:
            _, c1 = self._run_with_subsample(a, 10, 42)
            _, c2 = self._run_with_subsample(b, 10, 99)
            # Not a strict inequality guarantee, but at n=10 from 100 with
            # two different seeds, collision is astronomically unlikely.
            self.assertNotEqual(
                [r["messages"][-1]["content"] for r in c1],
                [r["messages"][-1]["content"] for r in c2],
            )

    def test_max_rows_larger_than_input_returns_all(self):
        with tempfile.TemporaryDirectory() as tmp:
            manifest, contents = self._run_with_subsample(tmp, 500, 42)
            self.assertEqual(manifest["splits"]["train"]["rows"], 100)
            self.assertEqual(len(contents), 100)


class TestBuildManifest(unittest.TestCase):
    def test_cost_estimate_scales_with_epochs(self):
        from training.openai_ft_adapter import SplitStats

        profile = _MODEL_PROFILES["gpt-4.1-mini-2025-04-14"]
        train = SplitStats(path="t", rows=100, tokens=1_000_000, sha256="x")
        val = SplitStats(path="v", rows=20, tokens=200_000, sha256="y")
        m1 = build_manifest(profile, train, val, [], "r", "src", "out", epochs=1)
        m3 = build_manifest(profile, train, val, [], "r", "src", "out", epochs=3)
        self.assertAlmostEqual(
            m3["totals"]["cost_estimate_usd"],
            m1["totals"]["cost_estimate_usd"] * 3,
            places=4,
        )

    def test_cost_envelope_low_mid_high(self):
        """FT-OAI-002 Epic 7 O4 — manifest surfaces 1-epoch / mid / 3-epoch costs."""
        from training.openai_ft_adapter import SplitStats

        profile = _MODEL_PROFILES["gpt-4.1-mini-2025-04-14"]
        train = SplitStats(path="t", rows=100, tokens=1_000_000, sha256="x")
        val = SplitStats(path="v", rows=20, tokens=200_000, sha256="y")
        m2 = build_manifest(profile, train, val, [], "r", "src", "out", epochs=2)
        t = m2["totals"]
        # Low == mid at epochs=1; at epochs=2, low should be half of mid.
        self.assertAlmostEqual(t["cost_estimate_low_usd"], t["cost_estimate_usd"] / 2, places=4)
        # High = 3× single-epoch cost
        self.assertAlmostEqual(t["cost_estimate_high_usd"], t["cost_estimate_low_usd"] * 3, places=4)
        self.assertIn("cost_estimate_note", t)


class TestTaskWeights(unittest.TestCase):
    """FT-OAI-002 Epic 6 T4 — deterministic per-task upsampling."""

    def _write_jsonl(self, path, records):
        with open(path, "w", encoding="utf-8") as f:
            for r in records:
                f.write(json.dumps(r) + "\n")

    def _prepare(self, tmp, train_records, val_records):
        import training.openai_ft_adapter as mod

        in_dir = os.path.join(tmp, "in")
        out_dir = os.path.join(tmp, "out")
        os.makedirs(in_dir)
        self._write_jsonl(os.path.join(in_dir, "train.jsonl"), train_records)
        self._write_jsonl(os.path.join(in_dir, "val.jsonl"), val_records)
        mod._MODEL_PROFILES["test-model"] = mod.ModelProfile(
            name="test-model",
            encoder_resolver=lambda: _FakeEncoder(),
            context_limit=1000,
            price_per_1m_train_usd=3.00,
        )
        return in_dir, out_dir

    def test_load_task_weights_inline_json(self):
        from training.openai_ft_adapter import _load_task_weights

        weights = _load_task_weights('{"retrieval.intent_translate": 8}')
        self.assertEqual(weights, {"retrieval.intent_translate": 8})

    def test_load_task_weights_from_file(self):
        from training.openai_ft_adapter import _load_task_weights

        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "w.json")
            with open(path, "w") as f:
                json.dump({"a.task": 5, "b.task": 2}, f)
            weights = _load_task_weights(f"@{path}")
        self.assertEqual(weights, {"a.task": 5, "b.task": 2})

    def test_load_task_weights_rejects_fractional(self):
        from training.openai_ft_adapter import _load_task_weights

        with self.assertRaises(ValueError):
            _load_task_weights('{"a.task": 1.5}')

    def test_load_task_weights_rejects_zero_or_negative(self):
        from training.openai_ft_adapter import _load_task_weights

        with self.assertRaises(ValueError):
            _load_task_weights('{"a.task": 0}')
        with self.assertRaises(ValueError):
            _load_task_weights('{"a.task": -3}')

    def test_load_task_weights_rejects_non_numeric(self):
        from training.openai_ft_adapter import _load_task_weights

        with self.assertRaises(ValueError):
            _load_task_weights('{"a.task": "eight"}')

    def test_sys_prompt_map_attribution(self):
        """Records with matching system_prompt hash attribute to the task."""
        import hashlib as _h

        import training.openai_ft_adapter as mod

        sys_prompt_a = "You are task A's system prompt."
        sys_prompt_b = "You are task B's system prompt."
        h_a = _h.sha256(sys_prompt_a.encode()).hexdigest()
        h_b = _h.sha256(sys_prompt_b.encode()).hexdigest()
        sys_map = {h_a: "task.a", h_b: "task.b"}

        # Use a fake profile to avoid tiktoken dependency.
        fake_profile = mod.ModelProfile(
            name="fake", encoder_resolver=lambda: _FakeEncoder(),
            context_limit=1000, price_per_1m_train_usd=1.0,
        )
        rec_a = _record(system=sys_prompt_a, user="u1", assistant="a1")
        result = mod.process_record(
            rec_a, fake_profile, _FakeEncoder(), sys_prompt_map=sys_map,
        )
        self.assertEqual(result.task, "task.a")

    def test_duplication_applies_per_task_weights_deterministically(self):
        """Weight=3 for task.a → 3× rows; weight unspecified → 1× rows."""
        import hashlib as _h

        with tempfile.TemporaryDirectory() as tmp:
            sp_a = "prompt.A"
            sp_b = "prompt.B"
            h_a = _h.sha256(sp_a.encode()).hexdigest()
            h_b = _h.sha256(sp_b.encode()).hexdigest()
            sys_map = {h_a: "task.a", h_b: "task.b"}

            # 2 task.a records, 3 task.b records in train
            train = [
                _record(system=sp_a, assistant="a1"),
                _record(system=sp_a, assistant="a2"),
                _record(system=sp_b, assistant="b1"),
                _record(system=sp_b, assistant="b2"),
                _record(system=sp_b, assistant="b3"),
            ]
            val = [_record(system=sp_a, assistant="va")]

            in_dir, out_dir = self._prepare(tmp, train, val)

            import training.openai_ft_adapter as mod
            try:
                manifest = mod.run(
                    input_dir=in_dir,
                    output_dir=out_dir,
                    model="test-model",
                    task_weights={"task.a": 3},
                    sys_prompt_map=sys_map,
                )
            finally:
                mod._MODEL_PROFILES.pop("test-model", None)

            with open(os.path.join(out_dir, "combined_train.jsonl")) as f:
                lines = [json.loads(line) for line in f if line.strip()]

        # 2 × 3 (task.a) + 3 × 1 (task.b) = 9
        self.assertEqual(len(lines), 9)
        self.assertEqual(manifest["splits"]["train"]["rows"], 9)
        self.assertEqual(
            manifest["splits"]["train"]["task_breakdown"],
            {"task.a": 6, "task.b": 3},
        )
        # Val (1 task.a) × 3 = 3
        self.assertEqual(manifest["splits"]["val"]["rows"], 3)
        self.assertEqual(manifest["task_weights"]["applied"], True)
        self.assertEqual(manifest["task_weights"]["weights"], {"task.a": 3})

    def test_duplication_is_bytewise_deterministic(self):
        """Same inputs + weights → same combined_train.jsonl sha256."""
        import hashlib as _h

        sp = "the.only.prompt"
        h = _h.sha256(sp.encode()).hexdigest()
        sys_map = {h: "task.x"}
        train = [_record(system=sp, assistant=f"out{i}") for i in range(5)]
        val = [_record(system=sp, assistant="v")]

        shas = []
        for _ in range(2):
            with tempfile.TemporaryDirectory() as tmp:
                in_dir, out_dir = self._prepare(tmp, train, val)
                import training.openai_ft_adapter as mod
                try:
                    mod.run(
                        input_dir=in_dir,
                        output_dir=out_dir,
                        model="test-model",
                        task_weights={"task.x": 4},
                        sys_prompt_map=sys_map,
                    )
                finally:
                    mod._MODEL_PROFILES.pop("test-model", None)
                with open(os.path.join(out_dir, "combined_train.jsonl"), "rb") as f:
                    shas.append(_h.sha256(f.read()).hexdigest())
        self.assertEqual(shas[0], shas[1])

    def test_unattributed_records_get_weight_one(self):
        """Records whose task can't be resolved are written once (no duplication)."""
        with tempfile.TemporaryDirectory() as tmp:
            train = [_record(system="unmapped", assistant=f"r{i}") for i in range(3)]
            val = [_record(system="unmapped", assistant="v")]
            in_dir, out_dir = self._prepare(tmp, train, val)
            import training.openai_ft_adapter as mod
            try:
                manifest = mod.run(
                    input_dir=in_dir,
                    output_dir=out_dir,
                    model="test-model",
                    task_weights={"task.a": 5},  # no matching task
                    sys_prompt_map={},  # empty map → no attribution
                )
            finally:
                mod._MODEL_PROFILES.pop("test-model", None)
        self.assertEqual(manifest["splits"]["train"]["rows"], 3)
        self.assertEqual(manifest["splits"]["val"]["rows"], 1)


if __name__ == "__main__":
    unittest.main()
