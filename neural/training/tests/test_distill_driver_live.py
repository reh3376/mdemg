#!/usr/bin/env python3
"""Live integration tests for distill_driver.py (Sprint FT-LORA-DATA Epic 6.0 item 9).

Requires a running canonical MLX server at http://127.0.0.1:8101/v1.
Opted in via env var to keep the normal CI run fast and deterministic:

    MDEMG_LIVE_MLX=1 pytest neural/training/tests/test_distill_driver_live.py -v

Covers three paths the mocked unit tests cannot exercise:
  1. preflight_endpoint() against a real server
  2. A 2-row smoke run against retrieval.rerank_nli emits valid rows
  3. Single-instance guard fails when >1 mlx_lm.server-like process exists
"""
from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path

_NEURAL_ROOT = Path(__file__).resolve().parents[2]
if str(_NEURAL_ROOT) not in sys.path:
    sys.path.insert(0, str(_NEURAL_ROOT))

from training import distill_driver as dd  # noqa: E402


LIVE = os.environ.get("MDEMG_LIVE_MLX") == "1"
MLX_URL = os.environ.get("MDEMG_LIVE_MLX_URL", "http://127.0.0.1:8101/v1")


@unittest.skipUnless(LIVE, "live test — set MDEMG_LIVE_MLX=1 to run")
class TestLivePreflight(unittest.TestCase):
    def test_preflight_passes_against_canonical_endpoint(self):
        # Should return silently against a healthy server.
        dd.preflight_endpoint(MLX_URL, timeout=5.0)

    def test_preflight_fails_fast_on_wrong_port(self):
        with self.assertRaises(dd.PreflightFailed):
            dd.preflight_endpoint("http://127.0.0.1:65535/v1", timeout=2.0)


@unittest.skipUnless(LIVE, "live test — set MDEMG_LIVE_MLX=1 to run")
class TestLiveSmokeRun(unittest.TestCase):
    """2-row smoke test against retrieval.rerank_nli.

    Uses the real MLX endpoint. Must complete in well under the Epic 6.0
    gate threshold (60s for 5 rows) — 2 rows comfortably under 30s.
    """

    def test_two_row_smoke(self):
        project_root = _NEURAL_ROOT.parent
        ults_dir = project_root / "docs/tests/ults/specs"
        self.assertTrue(ults_dir.exists(),
                        f"ULTS spec dir missing: {ults_dir}")

        with tempfile.TemporaryDirectory() as d:
            out_path = Path(d) / "smoke.jsonl"
            debug_log = Path(d) / "_debug.jsonl"

            cfg = dd.DEFAULT_TEACHER_CONFIG["retrieval.rerank_nli"]
            budget = dd.BudgetTracker(cap_usd=100.0)

            t0 = time.monotonic()
            result = dd.run_one_task(
                task_name="retrieval.rerank_nli",
                cfg=cfg,
                ults_dir=ults_dir,
                project_root=project_root,
                out_path=out_path,
                dataset_ver="v-live-smoke",
                synthesis_version="v1-live",
                budget=budget,
                dry_run=False,
                sleep_ms=0,
                seed=42,
                cuid_gen=__import__("cuid2").Cuid(length=24).generate,
                endpoint_urls={
                    "openai":    {"base_url": "https://api.openai.com/v1",
                                  "api_key_env": "OPENAI_API_KEY"},
                    "mlx_local": {"base_url": MLX_URL, "api_key_env": None},
                },
                count_override=2,
                strict=False,
                http_timeout_s=60.0,
                max_retries=2,
                debug_log=debug_log,
                preflight=True,
            )
            elapsed = time.monotonic() - t0

            # Must finish in well under 60s (Epic 6.0 Gate: 5 rows < 60s).
            self.assertLess(elapsed, 60.0,
                            f"smoke took {elapsed:.1f}s (> 60s budget)")
            self.assertEqual(result.attempts, 2)
            # At least one valid row — allowing one schema drop but not both.
            self.assertGreaterEqual(result.emitted, 1,
                                    f"no rows emitted (result={result})")
            # Every emitted row has non-empty response text + full meta.
            with open(out_path) as fh:
                lines = [line for line in fh if line.strip()]
            self.assertEqual(len(lines), result.emitted)
            for line in lines:
                row = json.loads(line)
                self.assertIn("messages", row)
                assistant = [m for m in row["messages"] if m["role"] == "assistant"]
                self.assertTrue(assistant, "row missing assistant message")
                self.assertGreater(len(assistant[-1]["content"]), 0,
                                   "assistant content is empty")
                self.assertEqual(row["meta"]["task_name"], "retrieval.rerank_nli")
                self.assertEqual(row["meta"]["synthesis_version"], "v1-live")
                self.assertEqual(row["meta"]["source"], "synth-qwen3.6-local")


@unittest.skipUnless(LIVE, "live test — set MDEMG_LIVE_MLX=1 to run")
class TestLiveSingleInstanceGuard(unittest.TestCase):
    """Fire the single-instance guard by spawning a sleep process whose
    command line contains the literal ``mlx_lm.server`` token.

    We do NOT spawn an actual second MLX server (that would take minutes
    and consume tens of GB). The guard matches any process whose command
    line contains ``mlx_lm.server`` — sufficient for the detection path.
    """

    def test_guard_fires_on_second_process(self):
        # Start a decoy process whose argv contains the magic string.
        # Use ``sleep`` so it idles cheaply; the command line will show
        # the script path + the sentinel in bash -c.
        decoy = subprocess.Popen(
            ["bash", "-c", "exec -a 'python -m mlx_lm.server --decoy' sleep 10"],
        )
        try:
            # Give the kernel a moment so the new argv lands in ps output.
            time.sleep(0.3)
            hits = dd.count_mlx_server_instances()
            # The real canonical instance + our decoy = >= 2.
            self.assertGreaterEqual(len(hits), 2,
                                    f"expected ≥2 hits, got {hits}")
            with self.assertRaises(dd.PreflightFailed) as cm:
                dd.enforce_single_mlx_instance()
            self.assertIn("Multiple mlx_lm.server", str(cm.exception))
        finally:
            decoy.terminate()
            try:
                decoy.wait(timeout=3.0)
            except subprocess.TimeoutExpired:
                decoy.kill()


if __name__ == "__main__":
    unittest.main()
