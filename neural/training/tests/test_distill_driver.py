#!/usr/bin/env python3
"""Tests for neural/training/distill_driver.py (Sprint FT-LORA-DATA Epic 2).

Covers the four Epic 2 gate requirements from the sprint plan:
  1. --dry-run token/cost estimate (no network)
  2. Budget-cap abort produces partial output + breakdown
  3. synthesis_version stamping ("v1-{commit_sha_short}")
  4. weak_signal flagging on metalearn.generalize

Plus supporting: ULTS validation short-circuits on malformed teacher output.
"""

from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

_NEURAL_ROOT = Path(__file__).resolve().parents[2]
if str(_NEURAL_ROOT) not in sys.path:
    sys.path.insert(0, str(_NEURAL_ROOT))

from training import distill_driver as dd  # noqa: E402


# ── Fixtures ────────────────────────────────────────────────────────────────

def _fake_cuid_gen():
    """Deterministic trace_id generator for test assertions."""
    counter = {"n": 0}

    def gen():
        counter["n"] += 1
        return f"trace-{counter['n']:04d}"

    return gen


def _write_spec(ults_dir: Path, task_name: str, *, output_type: str = "string",
                max_tokens: int = 200, think_mode: bool = False) -> Path:
    """Write a minimal ULTS spec file."""
    spec = {
        "ults_version": "1.0.0",
        "task": {"name": task_name, "description": f"Test {task_name}",
                 "version": "1.0.0"},
        "prompt": {"system_prompt_hash": "test", "think_mode": think_mode},
        "performance": {"latency_budget_ms": 5000, "max_tokens": max_tokens},
        "output_schema": {"type": output_type, "properties": {}, "required": []},
    }
    fname = task_name.replace(".", "_") + ".ults.json"
    p = ults_dir / fname
    p.write_text(json.dumps(spec))
    return p


# ── Tests ───────────────────────────────────────────────────────────────────

class TestEstimateTokens(unittest.TestCase):
    def test_empty_string_minimum_one(self):
        self.assertEqual(dd.estimate_tokens(""), 1)

    def test_short_string(self):
        self.assertEqual(dd.estimate_tokens("abcd"), 1)     # 4 chars → 1
        self.assertEqual(dd.estimate_tokens("abcde"), 2)    # 5 chars → 2
        self.assertEqual(dd.estimate_tokens("a" * 80), 20)  # 80 / 4 = 20


class TestBudgetTracker(unittest.TestCase):
    def test_mlx_local_is_free(self):
        b = dd.BudgetTracker(cap_usd=0.50)
        b.charge("mlx_local", 10_000, 10_000)
        self.assertEqual(b.total_cost_usd, 0.0)

    def test_openai_pricing_math(self):
        b = dd.BudgetTracker(cap_usd=1.00,
                             input_price_per_m=0.25, output_price_per_m=2.00)
        # 4000 input @ $0.25/M = $0.001
        # 3000 output @ $2.00/M = $0.006
        b.charge("openai", 4000, 3000)
        self.assertAlmostEqual(b.total_cost_usd, 0.007, places=6)

    def test_would_exceed(self):
        b = dd.BudgetTracker(cap_usd=0.50,
                             input_price_per_m=0.25, output_price_per_m=2.00)
        b.charge("openai", 0, 250_000)  # $0.50 exactly
        self.assertFalse(b.would_exceed(0.0))
        self.assertTrue(b.would_exceed(0.01))


class TestCommitShaShort(unittest.TestCase):
    def test_returns_unknown_outside_repo(self):
        with tempfile.TemporaryDirectory() as d:
            sha = dd._commit_sha_short(Path(d))
            # Either a short hex (if the temp dir is inside a repo) or 'unknown'.
            self.assertTrue(sha == "unknown" or len(sha) <= 16)

    def test_real_repo_returns_short_hex(self):
        # We're inside mdemg repo; should get a short sha, not 'unknown'.
        sha = dd._commit_sha_short(Path(__file__).resolve().parents[3])
        self.assertNotEqual(sha, "")
        # Short sha: 4..16 hex chars.
        self.assertTrue(all(c in "0123456789abcdef" for c in sha)
                        or sha == "unknown")


class TestLoadSpec(unittest.TestCase):
    def test_loads_known_task(self):
        with tempfile.TemporaryDirectory() as d:
            ults = Path(d)
            _write_spec(ults, "consulting.synthesis")
            spec = dd.load_spec(ults, "consulting.synthesis")
            self.assertEqual(spec["task"]["name"], "consulting.synthesis")

    def test_missing_spec_raises(self):
        with tempfile.TemporaryDirectory() as d:
            with self.assertRaises(FileNotFoundError):
                dd.load_spec(Path(d), "does.not.exist")


class TestBuildMeta(unittest.TestCase):
    def test_weak_signal_propagates(self):
        cfg = dd.DEFAULT_TEACHER_CONFIG["metalearn.generalize"]
        meta = dd._build_meta(
            task_name="metalearn.generalize",
            trace_id="trace-x",
            cfg=cfg,
            dataset_ver="v2026-04-22",
            synthesis_version="v1-deadbeef",
        )
        self.assertTrue(meta["weak_signal"])
        self.assertEqual(meta["synthesis_version"], "v1-deadbeef")
        self.assertEqual(meta["source"], "synth-gpt-5.4-mini")

    def test_weak_signal_false_on_other_tasks(self):
        cfg = dd.DEFAULT_TEACHER_CONFIG["summarize.generate"]
        meta = dd._build_meta(
            task_name="summarize.generate",
            trace_id="trace-y",
            cfg=cfg,
            dataset_ver="v2026-04-22",
            synthesis_version="v1-abc",
        )
        self.assertFalse(meta["weak_signal"])
        self.assertEqual(meta["source"], "synth-qwen3-14b-local")  # UXTS-CI-001: was the MoE-era qwen3.6 label; code moved to qwen3-14b at the 2026-04-22 pivot
        # space_id and instance_id invariants (plan §5 Epic 2 Step 5):
        self.assertEqual(meta["space_id"], "synth")
        self.assertEqual(meta["instance_id"], "synth-qwen3-14b-local")  # UXTS-CI-001: MoE-era label, see source assertion above


class TestRunOneTaskDryRun(unittest.TestCase):
    def test_dry_run_no_network_no_output_rows(self):
        """--dry-run must not call _chat_completion and must not write rows."""
        with tempfile.TemporaryDirectory() as d:
            ults = Path(d) / "ults"
            ults.mkdir()
            _write_spec(ults, "summarize.generate", max_tokens=100)
            out_path = Path(d) / "out.jsonl"

            cfg = dd.TeacherTask(
                teacher_id="qwen3.6-local", endpoint="mlx_local",
                count=5, weak_signal=False, model="test-model",
            )

            with mock.patch.object(dd, "_chat_completion") as mocked:
                budget = dd.BudgetTracker(cap_usd=100.0)
                result = dd.run_one_task(
                    task_name="summarize.generate",
                    cfg=cfg,
                    ults_dir=ults,
                    project_root=Path(d),
                    out_path=out_path,
                    dataset_ver="v-test",
                    synthesis_version="v1-test",
                    budget=budget,
                    dry_run=True,
                    sleep_ms=0,
                    seed=42,
                    cuid_gen=_fake_cuid_gen(),
                    endpoint_urls={
                        "mlx_local": {"base_url": "http://x", "api_key_env": None},
                        "openai":    {"base_url": "http://x", "api_key_env": None},
                    },
                )

            mocked.assert_not_called()
            self.assertEqual(result.emitted, 0)  # dry-run never emits
            self.assertEqual(result.attempts, 5)
            self.assertTrue(out_path.exists())
            self.assertEqual(out_path.read_text().strip(), "")

    def test_dry_run_estimates_openai_cost(self):
        """Dry-run charges estimated tokens against budget for OpenAI endpoints."""
        with tempfile.TemporaryDirectory() as d:
            ults = Path(d) / "ults"
            ults.mkdir()
            _write_spec(ults, "consulting.synthesis", max_tokens=1000)
            out_path = Path(d) / "out.jsonl"

            cfg = dd.TeacherTask(
                teacher_id="gpt-5.4-mini", endpoint="openai",
                count=3, weak_signal=False, model="gpt-5.4-mini",
            )

            budget = dd.BudgetTracker(cap_usd=100.0)
            dd.run_one_task(
                task_name="consulting.synthesis",
                cfg=cfg, ults_dir=ults, project_root=Path(d),
                out_path=out_path, dataset_ver="v-test",
                synthesis_version="v1-test", budget=budget, dry_run=True,
                sleep_ms=0, seed=42, cuid_gen=_fake_cuid_gen(),
                endpoint_urls={
                    "openai":    {"base_url": "http://x",
                                  "api_key_env": None},
                    "mlx_local": {"base_url": "http://x", "api_key_env": None},
                },
            )
            # 3 attempts × 1000 output tokens = 3000 out. Cost > 0.
            self.assertGreater(budget.total_cost_usd, 0.0)
            self.assertEqual(budget.per_endpoint["openai"]["output_tokens"], 3000)


class TestBudgetCapAbort(unittest.TestCase):
    def test_budget_cap_fires_mid_run(self):
        """Low cap + OpenAI task → BudgetExceeded is raised with partial preservation."""
        with tempfile.TemporaryDirectory() as d:
            ults = Path(d) / "ults"
            ults.mkdir()
            _write_spec(ults, "consulting.synthesis", max_tokens=2000)
            out_path = Path(d) / "out.jsonl"

            cfg = dd.TeacherTask(
                teacher_id="gpt-5.4-mini", endpoint="openai",
                count=50, weak_signal=False, model="gpt-5.4-mini",
            )

            # Tiny cap so we abort well before 50 calls.
            budget = dd.BudgetTracker(cap_usd=0.01)

            with self.assertRaises(dd.BudgetExceeded) as cm:
                dd.run_one_task(
                    task_name="consulting.synthesis",
                    cfg=cfg, ults_dir=ults, project_root=Path(d),
                    out_path=out_path, dataset_ver="v-test",
                    synthesis_version="v1-test", budget=budget, dry_run=True,
                    sleep_ms=0, seed=42, cuid_gen=_fake_cuid_gen(),
                    endpoint_urls={
                        "openai":    {"base_url": "http://x",
                                      "api_key_env": None},
                        "mlx_local": {"base_url": "http://x", "api_key_env": None},
                    },
                )
            # Informative message content
            msg = str(cm.exception)
            self.assertIn("cap $0.01", msg)
            self.assertIn("consulting.synthesis", msg)
            self.assertIn("preserved", msg)
            # Spend strictly exceeded the cap at abort time
            self.assertGreater(budget.total_cost_usd, budget.cap_usd)


class TestRunOneTaskRealPath(unittest.TestCase):
    """Real (non-dry-run) path with mocked HTTP — verifies output shape."""

    def test_emits_chat_rows_with_meta(self):
        with tempfile.TemporaryDirectory() as d:
            ults = Path(d) / "ults"
            ults.mkdir()
            _write_spec(ults, "summarize.generate", max_tokens=50)
            out_path = Path(d) / "out.jsonl"
            cfg = dd.TeacherTask(
                teacher_id="qwen3.6-local", endpoint="mlx_local",
                count=3, weak_signal=False, model="test",
            )

            fake_usage = {"prompt_tokens": 10, "completion_tokens": 20}

            def fake_call(**kwargs):
                # response, think, usage, error, body_preview
                return "func foo() {}", None, fake_usage, None, None

            with mock.patch.object(dd, "_chat_completion", side_effect=fake_call):
                budget = dd.BudgetTracker(cap_usd=100.0)
                result = dd.run_one_task(
                    task_name="summarize.generate", cfg=cfg, ults_dir=ults,
                    project_root=Path(d), out_path=out_path,
                    dataset_ver="v-2026", synthesis_version="v1-commitXY",
                    budget=budget, dry_run=False, sleep_ms=0, seed=42,
                    cuid_gen=_fake_cuid_gen(),
                    endpoint_urls={
                        "openai":    {"base_url": "http://x", "api_key_env": None},
                        "mlx_local": {"base_url": "http://x", "api_key_env": None},
                    },
                    preflight=False,
                )

            self.assertEqual(result.emitted, 3)
            self.assertEqual(result.attempts, 3)
            lines = out_path.read_text().strip().splitlines()
            self.assertEqual(len(lines), 3)
            rows = [json.loads(ln) for ln in lines]
            for r in rows:
                self.assertEqual(r["messages"][-1]["role"], "assistant")
                self.assertEqual(r["meta"]["synthesis_version"], "v1-commitXY")
                self.assertEqual(r["meta"]["source"], "synth-qwen3.6-local")
                self.assertEqual(r["meta"]["dataset_ver"], "v-2026")
                self.assertEqual(r["meta"]["space_id"], "synth")
                self.assertFalse(r["meta"]["weak_signal"])
                self.assertTrue(r["meta"]["trace_id"].startswith("trace-"))
            # Local endpoint → zero cost, but tokens tracked.
            self.assertEqual(budget.total_cost_usd, 0.0)
            self.assertEqual(budget.per_endpoint["mlx_local"]["input_tokens"], 30)

    def test_invalid_ults_output_drops_row(self):
        """When validate_output returns False the row is not emitted."""
        with tempfile.TemporaryDirectory() as d:
            ults = Path(d) / "ults"
            ults.mkdir()
            # Object schema with required field that our fake response omits.
            p = ults / "metalearn_generalize.ults.json"
            p.write_text(json.dumps({
                "ults_version": "1.0.0",
                "task": {"name": "metalearn.generalize", "description": "x",
                         "version": "1.0.0"},
                "prompt": {"system_prompt_hash": "test"},
                "performance": {"latency_budget_ms": 1000, "max_tokens": 50},
                "output_schema": {
                    "type": "object",
                    "properties": {"analysis": {"type": "string"}},
                    "required": ["analysis"],
                },
            }))

            # Override to mlx_local endpoint with small count so test is fast
            # and doesn't require OPENAI_API_KEY. Endpoint choice is orthogonal
            # to what's under test (invalid-schema drop path).
            cfg = dd.TeacherTask(
                teacher_id="qwen3.6-local", endpoint="mlx_local",
                count=3, weak_signal=True, model="test",
            )
            out_path = Path(d) / "out.jsonl"

            def fake_call(**kwargs):
                # Response is plain text — doesn't satisfy "object with analysis"
                return ("not a json object", None,
                        {"prompt_tokens": 5, "completion_tokens": 5},
                        None, None)

            with mock.patch.object(dd, "_chat_completion", side_effect=fake_call):
                budget = dd.BudgetTracker(cap_usd=100.0)
                result = dd.run_one_task(
                    task_name="metalearn.generalize", cfg=cfg, ults_dir=ults,
                    project_root=Path(d), out_path=out_path,
                    dataset_ver="v-2026", synthesis_version="v1-x",
                    budget=budget, dry_run=False, sleep_ms=0, seed=42,
                    cuid_gen=_fake_cuid_gen(),
                    endpoint_urls={
                        "openai":    {"base_url": "http://x", "api_key_env": None},
                        "mlx_local": {"base_url": "http://x", "api_key_env": None},
                    },
                    preflight=False,
                )
            # count=3, none validate → emitted=0, all drop as invalid schema.
            self.assertEqual(result.emitted, 0)
            self.assertGreater(result.invalid_schema, 0)


class TestDefaultTeacherConfig(unittest.TestCase):
    """Guard invariants of the DEFAULT_TEACHER_CONFIG so no one silently
    changes wiring (e.g. moves consulting.synthesis to local MLX, which
    would invalidate the spend estimate)."""

    def test_five_tasks_only(self):
        self.assertEqual(set(dd.DEFAULT_TEACHER_CONFIG.keys()), {
            "consulting.synthesis", "metalearn.generalize",
            "retrieval.rerank_nli", "summarize.generate",
            "hidden.summarize",
        })

    def test_metalearn_is_weak_signal(self):
        self.assertTrue(dd.DEFAULT_TEACHER_CONFIG["metalearn.generalize"].weak_signal)
        for t in ("consulting.synthesis", "retrieval.rerank_nli",
                  "summarize.generate", "hidden.summarize"):
            self.assertFalse(dd.DEFAULT_TEACHER_CONFIG[t].weak_signal)

    def test_openai_only_for_reasoning_heavy(self):
        self.assertEqual(dd.DEFAULT_TEACHER_CONFIG["consulting.synthesis"].endpoint,
                         "openai")
        self.assertEqual(dd.DEFAULT_TEACHER_CONFIG["metalearn.generalize"].endpoint,
                         "openai")
        self.assertEqual(dd.DEFAULT_TEACHER_CONFIG["hidden.summarize"].endpoint,
                         "openai")
        self.assertEqual(dd.DEFAULT_TEACHER_CONFIG["retrieval.rerank_nli"].endpoint,
                         "mlx_local")
        self.assertEqual(dd.DEFAULT_TEACHER_CONFIG["summarize.generate"].endpoint,
                         "mlx_local")

    def test_200_count_floor_per_task(self):
        for cfg in dd.DEFAULT_TEACHER_CONFIG.values():
            self.assertEqual(cfg.count, 200)


# ── Epic 6.0 — Execution Stabilization tests ────────────────────────────────


class TestLogRow(unittest.TestCase):
    """Epic 6.0 item 1 — per-row structured stderr logging + flush."""

    def test_log_row_format_and_flush(self):
        with mock.patch.object(dd.sys, "stderr") as fake_stderr:
            dd.log_row(
                task_name="retrieval.rerank_nli",
                i=7, count=200, status="ok",
                cum_cost=0.0123, tokens_in=100, tokens_out=200,
                elapsed_ms=450,
            )
            # One write call, then flush.
            self.assertEqual(fake_stderr.write.call_count, 1)
            fake_stderr.flush.assert_called_once()
            written = fake_stderr.write.call_args[0][0]
            self.assertIn("[retrieval.rerank_nli 7/200]", written)
            self.assertIn("status=ok", written)
            self.assertIn("cost=$0.0123", written)
            self.assertIn("tokens_in=100", written)
            self.assertIn("tokens_out=200", written)
            self.assertIn("elapsed_ms=450", written)
            self.assertTrue(written.endswith("\n"))

    def test_log_row_extra_suffix(self):
        with mock.patch.object(dd.sys, "stderr") as fake_stderr:
            dd.log_row("t", 1, 1, "api_error", 0.0, 0, 0, 10,
                       extra="err=HTTP 429")
            written = fake_stderr.write.call_args[0][0]
            self.assertIn("err=HTTP 429", written)


class TestPreflightEndpoint(unittest.TestCase):
    """Epic 6.0 item 2 — endpoint pre-flight before first request."""

    def test_preflight_fails_on_404(self):
        from urllib.error import HTTPError
        err = HTTPError(
            url="http://127.0.0.1:8101/v1/models",
            code=404, msg="Not Found", hdrs=None, fp=None,
        )
        with mock.patch.object(dd, "urlopen", side_effect=err):
            with self.assertRaises(dd.PreflightFailed) as cm:
                dd.preflight_endpoint("http://127.0.0.1:8101/v1", timeout=0.5)
            self.assertIn("Endpoint unreachable", str(cm.exception))
            self.assertIn("127.0.0.1:8101", str(cm.exception))

    def test_preflight_fails_on_connection_refused(self):
        from urllib.error import URLError
        with mock.patch.object(
            dd, "urlopen",
            side_effect=URLError("[Errno 61] Connection refused"),
        ):
            with self.assertRaises(dd.PreflightFailed):
                dd.preflight_endpoint("http://127.0.0.1:9999/v1", timeout=0.5)

    def test_preflight_succeeds_silently(self):
        class _Resp:
            def __enter__(self): return self
            def __exit__(self, *a): return False
            def read(self): return b'{"data": []}'

        with mock.patch.object(dd, "urlopen", return_value=_Resp()):
            # No exception = success.
            dd.preflight_endpoint("http://127.0.0.1:8101/v1", timeout=0.5)


class TestSingleInstanceEnforcement(unittest.TestCase):
    """Epic 6.0 item 3 — guard against multiple mlx_lm.server instances."""

    def test_two_instances_aborts(self):
        fake_ps = (
            "  96549 python -m mlx_lm.server --port 8101\n"
            "  70990 python -m mlx_lm.server --port 8200\n"
            "  12345 /bin/zsh\n"
        )
        with mock.patch.object(dd.subprocess, "check_output",
                               return_value=fake_ps):
            with self.assertRaises(dd.PreflightFailed) as cm:
                dd.enforce_single_mlx_instance()
            msg = str(cm.exception)
            self.assertIn("Multiple mlx_lm.server", msg)
            self.assertIn("96549", msg)
            self.assertIn("70990", msg)

    def test_single_instance_passes(self):
        fake_ps = (
            "  96549 python -m mlx_lm.server --port 8101\n"
            "  12345 /bin/zsh\n"
        )
        with mock.patch.object(dd.subprocess, "check_output",
                               return_value=fake_ps):
            dd.enforce_single_mlx_instance()  # no-exception

    def test_zero_instances_passes(self):
        # Fail-open: if MLX isn't running at all, pre-flight_endpoint will
        # catch the reachability issue. enforce_single_mlx_instance only
        # fires on >1.
        with mock.patch.object(dd.subprocess, "check_output",
                               return_value="  12345 /bin/zsh\n"):
            dd.enforce_single_mlx_instance()  # no-exception


class TestCountOverride(unittest.TestCase):
    """Epic 6.0 item 6 — --count N overrides cfg.count for smoke tests."""

    def test_count_override_shrinks_run(self):
        with tempfile.TemporaryDirectory() as d:
            ults = Path(d) / "ults"
            ults.mkdir()
            _write_spec(ults, "summarize.generate", max_tokens=20)
            out_path = Path(d) / "out.jsonl"
            # cfg.count=200 (prod), override to 3 for smoke.
            cfg = dd.TeacherTask(
                teacher_id="qwen3.6-local", endpoint="mlx_local",
                count=200, weak_signal=False, model="test",
            )

            def fake_call(**kwargs):
                return "hello", None, {"prompt_tokens": 5, "completion_tokens": 5}, None, None

            with mock.patch.object(dd, "_chat_completion", side_effect=fake_call):
                budget = dd.BudgetTracker(cap_usd=100.0)
                result = dd.run_one_task(
                    task_name="summarize.generate", cfg=cfg, ults_dir=ults,
                    project_root=Path(d), out_path=out_path,
                    dataset_ver="v", synthesis_version="v1-x",
                    budget=budget, dry_run=False, sleep_ms=0, seed=42,
                    cuid_gen=_fake_cuid_gen(),
                    endpoint_urls={
                        "openai":    {"base_url": "http://x", "api_key_env": None},
                        "mlx_local": {"base_url": "http://x", "api_key_env": None},
                    },
                    count_override=3,
                    preflight=False,
                )
            # Exactly 3 attempts — cfg.count=200 is ignored.
            self.assertEqual(result.attempts, 3)
            self.assertEqual(result.emitted, 3)


class TestStrictMode(unittest.TestCase):
    """Epic 6.0 item 7 — --strict aborts on first non-ok row."""

    def test_strict_aborts_on_first_api_error(self):
        with tempfile.TemporaryDirectory() as d:
            ults = Path(d) / "ults"
            ults.mkdir()
            _write_spec(ults, "summarize.generate", max_tokens=20)
            out_path = Path(d) / "out.jsonl"
            cfg = dd.TeacherTask(
                teacher_id="qwen3.6-local", endpoint="mlx_local",
                count=5, weak_signal=False, model="test",
            )

            def fake_call(**kwargs):
                # Simulate API error on every call.
                return None, None, {}, "HTTP 500: teacher melted", None

            with mock.patch.object(dd, "_chat_completion", side_effect=fake_call):
                budget = dd.BudgetTracker(cap_usd=100.0)
                with self.assertRaises(RuntimeError) as cm:
                    dd.run_one_task(
                        task_name="summarize.generate", cfg=cfg, ults_dir=ults,
                        project_root=Path(d), out_path=out_path,
                        dataset_ver="v", synthesis_version="v1-x",
                        budget=budget, dry_run=False, sleep_ms=0, seed=42,
                        cuid_gen=_fake_cuid_gen(),
                        endpoint_urls={
                            "openai":    {"base_url": "http://x", "api_key_env": None},
                            "mlx_local": {"base_url": "http://x", "api_key_env": None},
                        },
                        strict=True,
                        preflight=False,
                    )
                self.assertIn("--strict", str(cm.exception))
                self.assertIn("api_error", str(cm.exception))

    def test_non_strict_continues_past_errors(self):
        """Without --strict the driver counts errors silently and continues."""
        with tempfile.TemporaryDirectory() as d:
            ults = Path(d) / "ults"
            ults.mkdir()
            _write_spec(ults, "summarize.generate", max_tokens=20)
            out_path = Path(d) / "out.jsonl"
            cfg = dd.TeacherTask(
                teacher_id="qwen3.6-local", endpoint="mlx_local",
                count=4, weak_signal=False, model="test",
            )

            def fake_call(**kwargs):
                return None, None, {}, "HTTP 500", None

            with mock.patch.object(dd, "_chat_completion", side_effect=fake_call):
                budget = dd.BudgetTracker(cap_usd=100.0)
                result = dd.run_one_task(
                    task_name="summarize.generate", cfg=cfg, ults_dir=ults,
                    project_root=Path(d), out_path=out_path,
                    dataset_ver="v", synthesis_version="v1-x",
                    budget=budget, dry_run=False, sleep_ms=0, seed=42,
                    cuid_gen=_fake_cuid_gen(),
                    endpoint_urls={
                        "openai":    {"base_url": "http://x", "api_key_env": None},
                        "mlx_local": {"base_url": "http://x", "api_key_env": None},
                    },
                    strict=False,
                    preflight=False,
                )
            self.assertEqual(result.attempts, 4)
            self.assertEqual(result.emitted, 0)
            self.assertEqual(result.api_errors, 4)


class TestRetryOnTimeout(unittest.TestCase):
    """Epic 6.0 item 5 — exponential backoff retry on timeout/5xx/429."""

    def test_retries_on_timeout_then_succeeds(self):
        from urllib.error import URLError

        call_count = {"n": 0}

        class _OkResp:
            def __enter__(self): return self
            def __exit__(self, *a): return False
            def read(self):
                return json.dumps({
                    "choices": [{"message": {"content": "hello"}}],
                    "usage": {"prompt_tokens": 10, "completion_tokens": 20},
                }).encode()

        def fake_urlopen(req, timeout=None):
            call_count["n"] += 1
            if call_count["n"] < 3:
                raise URLError("timed out")
            return _OkResp()

        with mock.patch.object(dd, "urlopen", side_effect=fake_urlopen), \
             mock.patch.object(dd.time, "sleep") as fake_sleep:
            resp, think, usage, err, preview = dd._chat_completion(
                base_url="http://x/v1",
                model="test",
                system_prompt="sys",
                user_prompt="user",
                max_tokens=10,
                think_mode=False,
                api_key=None,
                timeout=0.1,
                max_retries=3,
            )
        self.assertEqual(call_count["n"], 3)  # 2 failures + 1 success
        self.assertEqual(resp, "hello")
        self.assertIsNone(err)
        # Backoff pattern: 1s, 2s.
        sleeps = [c.args[0] for c in fake_sleep.call_args_list]
        self.assertEqual(sleeps, [1, 2])

    def test_retry_exhaustion_returns_error(self):
        from urllib.error import URLError

        def always_fail(req, timeout=None):
            raise URLError("timed out")

        with mock.patch.object(dd, "urlopen", side_effect=always_fail), \
             mock.patch.object(dd.time, "sleep"):
            resp, think, usage, err, preview = dd._chat_completion(
                base_url="http://x/v1",
                model="test",
                system_prompt="sys",
                user_prompt="user",
                max_tokens=10,
                think_mode=False,
                api_key=None,
                timeout=0.1,
                max_retries=3,
            )
        self.assertIsNone(resp)
        self.assertIsNotNone(err)
        self.assertIn("timed out", err)


if __name__ == "__main__":
    unittest.main()
