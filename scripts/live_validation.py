#!/usr/bin/env python3
"""
Live Validation Suite for MDEMG
===============================

Automates the 19-test live validation suite from the post-fix re-validation.
Tests verify end-to-end correctness of the MDEMG deployment including API,
TSDB recording, export pipeline, training tools, and error handling.

Usage:
    python3 scripts/live_validation.py                           # Non-destructive tests only
    python3 scripts/live_validation.py --include-destructive     # All 19 tests
    python3 scripts/live_validation.py --session 2               # Only Session 2 tests
    python3 scripts/live_validation.py --test 2.2                # Single test
    python3 scripts/live_validation.py --report /tmp/lv.json     # JSON report

Prerequisites:
    - Docker services running (mdemg, neo4j, timescaledb, grafana)
    - .env with port assignments (from mdemg init)
    - bin/mdemg built (for CLI tests)

Environment:
    MDEMG_PORT    Override API port (default: from .env)
    GRAFANA_PORT  Override Grafana port (default: from .env)
    TSDB_HOST_PORT Override TSDB port (default: from .env)
"""

import argparse
import json
import os
import shutil
import subprocess
import sys
import tarfile
import tempfile
import time
from datetime import datetime
from pathlib import Path

# ─── Config ──────────────────────────────────────────────────────────────────

MDEMG_BIN = "./bin/mdemg"
SPACE_ID = "mdemg-dev"
NEURAL_DIR = "neural"
TSDB_FLUSH_WAIT = 65  # seconds to wait for TSDB flush (must exceed TSDB_FLUSH_INTERVAL_SEC, default 60)
PYTHON = shutil.which("python3") or shutil.which("python") or "python3"


# ─── Helpers ─────────────────────────────────────────────────────────────────

class Results:
    """Tracks test results across the validation run."""

    def __init__(self):
        self.passed = []
        self.failed = []
        self.skipped = []
        self.partial = []

    def ok(self, test_id: str, name: str, detail: str = ""):
        self.passed.append({"id": test_id, "name": name, "detail": detail})
        print(f"  PASS  {test_id} {name}" + (f" — {detail}" if detail else ""))

    def fail(self, test_id: str, name: str, detail: str = ""):
        self.failed.append({"id": test_id, "name": name, "detail": detail})
        print(f"  FAIL  {test_id} {name}" + (f" — {detail}" if detail else ""))

    def skip(self, test_id: str, name: str, reason: str = ""):
        self.skipped.append({"id": test_id, "name": name, "detail": reason})
        print(f"  SKIP  {test_id} {name}" + (f" — {reason}" if reason else ""))

    def warn(self, test_id: str, name: str, detail: str = ""):
        self.partial.append({"id": test_id, "name": name, "detail": detail})
        print(f"  PART  {test_id} {name}" + (f" — {detail}" if detail else ""))

    def summary(self) -> bool:
        print(f"\n{'=' * 60}")
        print(f"  PASSED:  {len(self.passed)}")
        print(f"  FAILED:  {len(self.failed)}")
        print(f"  PARTIAL: {len(self.partial)}")
        print(f"  SKIPPED: {len(self.skipped)}")
        print(f"{'=' * 60}")
        if self.failed:
            print("\nFAILURES:")
            for f in self.failed:
                print(f"  {f['id']} {f['name']}: {f['detail']}")
        return len(self.failed) == 0

    def to_dict(self) -> dict:
        return {
            "timestamp": datetime.utcnow().isoformat() + "Z",
            "passed": self.passed,
            "failed": self.failed,
            "partial": self.partial,
            "skipped": self.skipped,
            "counts": {
                "passed": len(self.passed),
                "failed": len(self.failed),
                "partial": len(self.partial),
                "skipped": len(self.skipped),
            },
        }


def resolve_port(name: str, env_override: str = "") -> str:
    """Resolve a port from env override, then .env file."""
    if env_override:
        val = os.environ.get(env_override)
        if val:
            return val
    # Read from .env
    env_path = Path(".env")
    if env_path.exists():
        for line in env_path.read_text().splitlines():
            line = line.strip()
            if line.startswith("#") or "=" not in line:
                continue
            key, _, val = line.partition("=")
            if key.strip() == name:
                return val.strip()
    return ""


def run_cmd(cmd: str, check: bool = True, timeout: int = 60) -> subprocess.CompletedProcess:
    """Run shell command, return CompletedProcess."""
    result = subprocess.run(
        cmd, shell=True, capture_output=True, text=True, timeout=timeout
    )
    if check and result.returncode != 0:
        raise subprocess.CalledProcessError(
            result.returncode, cmd, result.stdout, result.stderr
        )
    return result


def tsdb_query(port: str, sql: str) -> str:
    """Run a TSDB query via docker compose exec."""
    escaped = sql.replace("'", "'\\''")
    cmd = f"docker compose exec -T timescaledb psql -U mdemg -d mdemg_metrics -tAc '{escaped}'"
    result = run_cmd(cmd, check=False, timeout=30)
    return result.stdout.strip()


def api_url(port: str) -> str:
    return f"http://localhost:{port}"


def curl_post(url: str, data: dict, timeout: int = 30) -> tuple[int, dict]:
    """POST JSON to URL, return (status_code, response_dict)."""
    import urllib.request
    import urllib.error

    req = urllib.request.Request(
        url,
        data=json.dumps(data).encode(),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            body = json.loads(resp.read().decode())
            return resp.status, body
    except urllib.error.HTTPError as e:
        body = e.read().decode()
        try:
            body = json.loads(body)
        except (json.JSONDecodeError, ValueError):
            body = {"raw": body}
        return e.code, body
    except Exception as e:
        return 0, {"error": str(e)}


def curl_get(url: str, timeout: int = 10) -> int:
    """GET URL, return status code (0 on connection error)."""
    import urllib.request
    import urllib.error

    try:
        req = urllib.request.Request(url, method="GET")
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            return resp.status
    except urllib.error.HTTPError as e:
        return e.code
    except Exception:
        return 0


# ─── Tests ───────────────────────────────────────────────────────────────────

def test_1_1_version(results: Results):
    """1.1 Version check — mdemg version returns valid output."""
    try:
        r = run_cmd(f"{MDEMG_BIN} version", check=False)
        output = r.stdout.strip() + r.stderr.strip()
        if "mdemg" in output.lower() or "version" in output.lower() or r.returncode == 0:
            results.ok("1.1", "Version check", output.split("\n")[0])
        else:
            results.fail("1.1", "Version check", f"unexpected output: {output[:100]}")
    except FileNotFoundError:
        results.fail("1.1", "Version check", f"{MDEMG_BIN} not found — run go build")


def test_1_2_init_artifacts(results: Results):
    """1.2 Init artifacts — .mdemg/, .env, docker-compose.yml exist."""
    missing = []
    for path in [".mdemg/config.yaml", ".env", "docker-compose.yml"]:
        if not Path(path).exists():
            missing.append(path)
    if missing:
        results.fail("1.2", "Init artifacts", f"missing: {', '.join(missing)}")
    else:
        results.ok("1.2", "Init artifacts")


def test_1_3_pre_campaign(results: Results):
    """1.3 Pre-campaign check — mdemg data check --pre-campaign."""
    try:
        r = run_cmd(f"{MDEMG_BIN} data check --pre-campaign --json", check=False, timeout=30)
        if r.returncode == 0:
            results.ok("1.3", "Pre-campaign check")
        else:
            # Parse JSON output for details
            try:
                data = json.loads(r.stdout)
                fails = [c["name"] for c in data.get("checks", []) if c.get("status") == "FAIL"]
                results.fail("1.3", "Pre-campaign check", f"failed checks: {', '.join(fails)}")
            except (json.JSONDecodeError, ValueError):
                results.fail("1.3", "Pre-campaign check", r.stderr[:200] or r.stdout[:200])
    except FileNotFoundError:
        results.fail("1.3", "Pre-campaign check", f"{MDEMG_BIN} not found")


def test_1_4_browser_ui(results: Results, port: str):
    """1.4 Browser UI — HTTP 200 from /ui/."""
    status = curl_get(f"{api_url(port)}/ui/")
    if status == 200:
        results.ok("1.4", "Browser UI", f"HTTP {status}")
    else:
        results.fail("1.4", "Browser UI", f"HTTP {status}")


def test_1_5_grafana(results: Results, grafana_port: str):
    """1.5 Grafana — HTTP 200 from /api/health."""
    if not grafana_port:
        results.skip("1.5", "Grafana", "GRAFANA_PORT not set")
        return
    status = curl_get(f"http://localhost:{grafana_port}/api/health")
    if status == 200:
        results.ok("1.5", "Grafana", f"HTTP {status}")
    else:
        results.fail("1.5", "Grafana", f"HTTP {status}")


def test_1_6_service_install(results: Results):
    """1.6 Service install — launchctl lists mdemg services."""
    if sys.platform != "darwin":
        results.skip("1.6", "Service install", "macOS only")
        return
    r = run_cmd("launchctl list 2>/dev/null | grep mdemg || true", check=False)
    count = len([l for l in r.stdout.strip().split("\n") if l.strip()])
    if count >= 2:
        results.ok("1.6", "Service install", f"{count} services")
    elif count >= 1:
        results.warn("1.6", "Service install", f"{count} service (expected 2+, port conflict possible)")
    else:
        results.warn("1.6", "Service install", "no mdemg services in launchctl")


def test_2_1_observe(results: Results, port: str):
    """2.1 Observe → TSDB — observe creates a node."""
    status, body = curl_post(f"{api_url(port)}/v1/conversation/observe", {
        "space_id": SPACE_ID,
        "session_id": "live-validation",
        "content": f"Live validation test observation at {datetime.utcnow().isoformat()}",
        "obs_type": "technical_note",
    })
    if status == 200 and body.get("node_id"):
        results.ok("2.1", "Observe → TSDB", f"node_id={body['node_id'][:16]}...")
    else:
        results.fail("2.1", "Observe → TSDB", f"HTTP {status}, body={json.dumps(body)[:100]}")


def test_2_2_session_id(results: Results, port: str, tsdb_port: str):
    """2.2 Session ID propagation — session_id reaches TSDB."""
    session_tag = f"lv-session-{int(time.time())}"

    # Send 3 retrieve requests with a unique session_id
    for i in range(3):
        curl_post(f"{api_url(port)}/v1/memory/retrieve", {
            "space_id": SPACE_ID,
            "query_text": f"session propagation test {i}",
            "session_id": session_tag,
            "top_k": 3,
        })

    print(f"    Waiting {TSDB_FLUSH_WAIT}s for TSDB flush...")
    time.sleep(TSDB_FLUSH_WAIT)

    # Query TSDB for records with our session_id
    rows = tsdb_query(tsdb_port, f"""
        SELECT COUNT(*) FROM llm_interactions
        WHERE session_id = '{session_tag}'
          AND time > now() - interval '3 minutes'
    """)
    count = int(rows) if rows.isdigit() else 0
    if count >= 3:
        results.ok("2.2", "Session ID propagation", f"{count} records with session_id={session_tag}")
    else:
        results.fail("2.2", "Session ID propagation", f"only {count} records (expected >= 3)")


def test_2_3_query_classify(results: Results, port: str, tsdb_port: str):
    """2.3 Query classify records — query_classify tasks recorded in TSDB."""
    # Send retrieve requests to trigger query classify
    for i in range(3):
        curl_post(f"{api_url(port)}/v1/memory/retrieve", {
            "space_id": SPACE_ID,
            "query_text": f"classify test query {i} for validation",
            "top_k": 3,
        })

    # Use the flush wait from 2.2 if already waited, otherwise wait
    if not hasattr(test_2_3_query_classify, "_waited"):
        print(f"    Waiting {TSDB_FLUSH_WAIT}s for TSDB flush...")
        time.sleep(TSDB_FLUSH_WAIT)

    rows = tsdb_query(tsdb_port, """
        SELECT COUNT(*) FROM llm_interactions
        WHERE task_name = 'retrieval.query_classify'
          AND time > now() - interval '5 minutes'
    """)
    count = int(rows) if rows.isdigit() else 0
    if count > 0:
        results.ok("2.3", "Query classify records", f"{count} query_classify records")
    else:
        results.fail("2.3", "Query classify records", "0 records — is QUERY_CLASSIFY_ENABLED=true?")


def test_2_5_export(results: Results, port: str):
    """2.5 Export pipeline — mdemg data export produces a valid archive."""
    with tempfile.TemporaryDirectory() as tmpdir:
        output = os.path.join(tmpdir, "test-export.tar.gz")
        r = run_cmd(
            f"{MDEMG_BIN} data export --space-id {SPACE_ID} --output {output} --no-validate",
            check=False, timeout=120,
        )
        if r.returncode != 0:
            results.fail("2.5", "Export pipeline", f"exit {r.returncode}: {r.stderr[:200]}")
            return

        # Check archive exists and contains manifest
        if not Path(output).exists():
            results.fail("2.5", "Export pipeline", "archive not created")
            return

        try:
            with tarfile.open(output, "r:gz") as tf:
                names = tf.getnames()
                has_manifest = any("manifest.json" in n for n in names)
                if has_manifest:
                    results.ok("2.5", "Export pipeline", f"{len(names)} files in archive")
                else:
                    results.fail("2.5", "Export pipeline", "no manifest.json in archive")
        except tarfile.TarError as e:
            results.fail("2.5", "Export pipeline", f"invalid tar: {e}")


def test_2_6_export_auto(results: Results):
    """2.6 Export-auto retention — export-auto creates files and manages retention."""
    with tempfile.TemporaryDirectory() as tmpdir:
        r = run_cmd(
            f"{MDEMG_BIN} data export-auto --space-id {SPACE_ID} --output-dir {tmpdir} --keep 2",
            check=False, timeout=120,
        )
        if r.returncode != 0:
            results.fail("2.6", "Export-auto retention", f"exit {r.returncode}: {r.stderr[:200]}")
            return

        archives = list(Path(tmpdir).glob("mdemg-export-*.tar.gz"))
        latest = Path(tmpdir) / "latest.tar.gz"
        if len(archives) >= 1 and latest.is_symlink():
            results.ok("2.6", "Export-auto retention", f"{len(archives)} archive(s), latest symlink ok")
        elif len(archives) >= 1:
            results.warn("2.6", "Export-auto retention", f"{len(archives)} archive(s), no latest symlink")
        else:
            results.fail("2.6", "Export-auto retention", "no archives created")


def test_3_1_curation(results: Results):
    """3.1 Curation pipeline — export → filter → convert → version."""
    with tempfile.TemporaryDirectory() as tmpdir:
        # Export
        export_path = os.path.join(tmpdir, "curation-test.tar.gz")
        r = run_cmd(
            f"{MDEMG_BIN} data export --space-id {SPACE_ID} --output {export_path} --no-validate",
            check=False, timeout=120,
        )
        if r.returncode != 0:
            results.fail("3.1", "Curation pipeline", f"export failed: {r.stderr[:100]}")
            return

        # Extract archive to get JSONL files
        extract_dir = os.path.join(tmpdir, "extracted")
        os.makedirs(extract_dir, exist_ok=True)
        try:
            with tarfile.open(export_path, "r:gz") as tf:
                tf.extractall(extract_dir)
        except tarfile.TarError as e:
            results.fail("3.1", "Curation pipeline", f"cannot extract archive: {e}")
            return

        input_jsonl = os.path.join(extract_dir, "llm_interactions.jsonl")
        if not os.path.exists(input_jsonl):
            results.fail("3.1", "Curation pipeline", "no llm_interactions.jsonl in archive")
            return

        # Quality filter
        filter_out = os.path.join(tmpdir, "filtered.jsonl")
        r = run_cmd(
            f"cd {NEURAL_DIR} && {PYTHON} -m training.quality_filter --input {input_jsonl} --output {filter_out}",
            check=False, timeout=120,
        )
        if r.returncode not in (0, 2):
            results.fail("3.1", "Curation pipeline", f"quality_filter failed: {r.stderr[:100]}")
            return

        if not os.path.exists(filter_out):
            results.fail("3.1", "Curation pipeline", "no filtered JSONL output")
            return

        # Format converter — output into a dedicated directory for versioner input
        chat_dir = os.path.join(tmpdir, "chat")
        os.makedirs(chat_dir)
        chat_out = os.path.join(chat_dir, "chat.jsonl")
        r = run_cmd(
            f"cd {NEURAL_DIR} && {PYTHON} -m training.format_converter --input {filter_out} --output {chat_out}",
            check=False, timeout=60,
        )
        if r.returncode != 0:
            results.fail("3.1", "Curation pipeline", f"format_converter failed: {r.stderr[:100]}")
            return

        # Dataset versioner
        ds_out = os.path.join(tmpdir, "dataset")
        r = run_cmd(
            f"cd {NEURAL_DIR} && {PYTHON} -m training.dataset_versioner"
            f" --input-dir {chat_dir} --output-dir {ds_out} --version v0-test --first-cycle",
            check=False, timeout=60,
        )
        if r.returncode != 0:
            results.fail("3.1", "Curation pipeline", f"dataset_versioner failed: {r.stderr[:100]}")
            return

        results.ok("3.1", "Curation pipeline", "export → filter → convert → version")


def test_3_2_training_dry_run(results: Results):
    """3.2 Training dry-run — train_ft.py --dry-run exits 0."""
    train_script = Path(NEURAL_DIR) / "training" / "train_ft.py"
    if not train_script.exists():
        results.skip("3.2", "Training dry-run", "train_ft.py not yet implemented")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        # Create minimal dataset directory with valid manifest
        ds_dir = os.path.join(tmpdir, "dataset")
        os.makedirs(ds_dir)
        manifest = {
            "dataset_id": "lv-dry-run-test",
            "version": "0.0.1",
            "splits": {"train": {"file": "train.jsonl", "rows": 10}},
            "quality_gates": {
                "exogenous_ratio": 0.5,
                "exogenous_ratio_met": True,
                "no_train_test_overlap": True,
            },
        }
        with open(os.path.join(ds_dir, "manifest.json"), "w") as f:
            json.dump(manifest, f)

        adapter_dir = os.path.join(tmpdir, "adapter")
        r = run_cmd(
            f"cd {NEURAL_DIR} && {PYTHON} -m training.train_ft"
            f" --dataset {ds_dir} --base-model test-model --adapter-path {adapter_dir} --dry-run",
            check=False, timeout=60,
        )
        if r.returncode == 0:
            results.ok("3.2", "Training dry-run")
        else:
            results.fail("3.2", "Training dry-run", f"exit {r.returncode}: {r.stderr[:100]}")


def test_3_4_gate_self_compare(results: Results):
    """3.4 Regression gate self-compare — comparing baseline to itself returns WARN (exit 2)."""
    gate_script = Path(NEURAL_DIR) / "training" / "regression_gate.py"
    if not gate_script.exists():
        results.skip("3.4", "Regression gate", "regression_gate.py not found")
        return

    # Create a minimal eval report for self-comparison
    with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as f:
        report = {
            "summary": {"overall_weighted_score": 0.85},
            "task_results": [
                {"task": "task_a", "status": "evaluated", "weighted_score": 0.90, "metric_aggregates": {}},
                {"task": "task_b", "status": "evaluated", "weighted_score": 0.80, "metric_aggregates": {}},
            ],
        }
        json.dump(report, f)
        report_path = f.name

    try:
        r = run_cmd(
            f"cd {NEURAL_DIR} && {PYTHON} -m training.regression_gate --candidate {report_path} --baseline {report_path}",
            check=False, timeout=30,
        )
        if r.returncode == 2:
            results.ok("3.4", "Regression gate self-compare", "exit 2 (WARN) — correct")
        elif r.returncode == 0:
            results.fail("3.4", "Regression gate self-compare", "exit 0 (PASS) — should be WARN for self-compare")
        else:
            results.fail("3.4", "Regression gate self-compare", f"exit {r.returncode}: {r.stderr[:100]}")
    finally:
        os.unlink(report_path)


def test_4_5_teardown_reinit(results: Results):
    """4.5 Teardown + reinit — docker compose down -v → mdemg init --defaults → services up."""
    # Safety check: refuse to run in main dev directory
    cwd = Path.cwd().name
    if "test" not in cwd and "tmp" not in cwd and "/tmp" not in str(Path.cwd()):
        results.skip("4.5", "Teardown + reinit",
                      f"refusing destructive test in {Path.cwd()} — run from a test directory")
        return

    r = run_cmd("docker compose down -v", check=False, timeout=60)
    if r.returncode != 0:
        results.fail("4.5", "Teardown + reinit", f"docker compose down failed: {r.stderr[:100]}")
        return

    r = run_cmd(f"{MDEMG_BIN} init --defaults", check=False, timeout=60)
    if r.returncode != 0:
        results.fail("4.5", "Teardown + reinit", f"mdemg init failed: {r.stderr[:100]}")
        return

    r = run_cmd("docker compose up -d", check=False, timeout=120)
    if r.returncode != 0:
        results.fail("4.5", "Teardown + reinit", f"docker compose up failed: {r.stderr[:100]}")
        return

    # Wait for health
    time.sleep(15)
    r = run_cmd("docker compose ps --format json", check=False)
    results.ok("4.5", "Teardown + reinit", "down → init → up completed")


def test_6_1_pii_injection(results: Results, port: str, tsdb_port: str):
    """6.1 PII injection — export blocks when PII is present."""
    # Insert a record with PII
    pii_content = "Contact roger.henley@example.com for details, API key sk-1234567890abcdefghijklmnop"
    tsdb_query(tsdb_port, f"""
        INSERT INTO llm_interactions (time, trace_id, task_name, space_id, session_id,
            system_prompt, user_prompt, response, latency_ms, model_name, provider, instance_id)
        VALUES (now(), 'pii-test-trace', 'pii.test', '{SPACE_ID}', 'pii-test',
            'system', '{pii_content}', 'response', 100, 'test', 'test', 'pii-test-instance')
    """)

    with tempfile.TemporaryDirectory() as tmpdir:
        output = os.path.join(tmpdir, "pii-test.tar.gz")
        r = run_cmd(
            f"{MDEMG_BIN} data export --space-id {SPACE_ID} --output {output} --no-validate",
            check=False, timeout=120,
        )
        # Export should fail or produce 0 privacy violations warning
        pii_blocked = r.returncode != 0 or "privacy" in r.stderr.lower() or "pii" in r.stderr.lower()

        # Clean up PII record
        tsdb_query(tsdb_port, "DELETE FROM llm_interactions WHERE trace_id = 'pii-test-trace'")

        if pii_blocked:
            results.ok("6.1", "PII injection", "export correctly blocked/warned on PII")
        else:
            # Check if export included privacy violation count
            if "violations" in r.stdout.lower() and "0 violations" not in r.stdout.lower():
                results.ok("6.1", "PII injection", "export reported privacy violations")
            else:
                results.fail("6.1", "PII injection", "PII was not detected in export")


def test_6_2_tsdb_down(results: Results):
    """6.2 TSDB down — export fails gracefully when TSDB is stopped."""
    r = run_cmd("docker compose stop timescaledb", check=False, timeout=30)
    time.sleep(3)

    with tempfile.TemporaryDirectory() as tmpdir:
        output = os.path.join(tmpdir, "tsdb-down.tar.gz")
        r = run_cmd(
            f"{MDEMG_BIN} data export --space-id {SPACE_ID} --output {output} --no-validate",
            check=False, timeout=30,
        )
        export_failed = r.returncode != 0

    # Restart TSDB
    run_cmd("docker compose start timescaledb", check=False, timeout=30)
    time.sleep(10)  # Wait for TSDB to become healthy

    if export_failed:
        results.ok("6.2", "TSDB down", "export correctly failed when TSDB stopped")
    else:
        results.fail("6.2", "TSDB down", "export succeeded despite TSDB being stopped")


def test_6_3_invalid_manifest(results: Results):
    """6.3 Invalid manifest — train_ft.py rejects bad manifest."""
    train_script = Path(NEURAL_DIR) / "training" / "train_ft.py"
    if not train_script.exists():
        results.skip("6.3", "Invalid manifest", "train_ft.py not yet implemented")
        return

    with tempfile.TemporaryDirectory() as tmpdir:
        # Create a manifest with failing quality gates
        manifest = {
            "quality_gates": {"exogenous_ratio_met": False, "exogenous_ratio": 0.1},
            "splits": {"train": {"rows": 0}},
        }
        manifest_path = os.path.join(tmpdir, "manifest.json")
        with open(manifest_path, "w") as f:
            json.dump(manifest, f)

        r = run_cmd(
            f"cd {NEURAL_DIR} && {PYTHON} -m training.train_ft --dataset {tmpdir} --dry-run",
            check=False, timeout=30,
        )
        if r.returncode != 0:
            results.ok("6.3", "Invalid manifest", f"correctly rejected (exit {r.returncode})")
        else:
            results.fail("6.3", "Invalid manifest", "accepted bad manifest (should reject)")


def test_6_4_regression_catch(results: Results):
    """6.4 Regression gate catches regression — exit 1 for regressed report."""
    gate_script = Path(NEURAL_DIR) / "training" / "regression_gate.py"
    if not gate_script.exists():
        results.skip("6.4", "Regression gate regression", "regression_gate.py not found")
        return

    with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as bf:
        baseline = {
            "summary": {"overall_weighted_score": 0.90},
            "task_results": [
                {"task": "task_a", "status": "evaluated", "weighted_score": 0.90, "metric_aggregates": {}},
                {"task": "task_b", "status": "evaluated", "weighted_score": 0.85, "metric_aggregates": {}},
            ],
        }
        json.dump(baseline, bf)
        baseline_path = bf.name

    with tempfile.NamedTemporaryFile(mode="w", suffix=".json", delete=False) as cf:
        candidate = {
            "summary": {"overall_weighted_score": 0.70},
            "task_results": [
                {"task": "task_a", "status": "evaluated", "weighted_score": 0.60, "metric_aggregates": {}},
                {"task": "task_b", "status": "evaluated", "weighted_score": 0.80, "metric_aggregates": {}},
            ],
        }
        json.dump(candidate, cf)
        candidate_path = cf.name

    try:
        r = run_cmd(
            f"cd {NEURAL_DIR} && {PYTHON} -m training.regression_gate --candidate {candidate_path} --baseline {baseline_path}",
            check=False, timeout=30,
        )
        if r.returncode == 1:
            results.ok("6.4", "Regression gate regression", "exit 1 (FAIL) — correct")
        else:
            results.fail("6.4", "Regression gate regression", f"exit {r.returncode} — expected 1")
    finally:
        os.unlink(baseline_path)
        os.unlink(candidate_path)


# ─── Test Registry ───────────────────────────────────────────────────────────

# (test_id, session, function, destructive, needs_ports)
TEST_REGISTRY = [
    ("1.1", 1, test_1_1_version, False, False),
    ("1.2", 1, test_1_2_init_artifacts, False, False),
    ("1.3", 1, test_1_3_pre_campaign, False, False),
    ("1.4", 1, test_1_4_browser_ui, False, True),
    ("1.5", 1, test_1_5_grafana, False, True),
    ("1.6", 1, test_1_6_service_install, False, False),
    ("2.1", 2, test_2_1_observe, False, True),
    ("2.2", 2, test_2_2_session_id, False, True),
    ("2.3", 2, test_2_3_query_classify, False, True),
    ("2.5", 2, test_2_5_export, False, True),
    ("2.6", 2, test_2_6_export_auto, False, False),
    ("3.1", 3, test_3_1_curation, False, False),
    ("3.2", 3, test_3_2_training_dry_run, False, False),
    ("3.4", 3, test_3_4_gate_self_compare, False, False),
    ("4.5", 4, test_4_5_teardown_reinit, True, False),
    ("6.1", 6, test_6_1_pii_injection, True, True),
    ("6.2", 6, test_6_2_tsdb_down, True, False),
    ("6.3", 6, test_6_3_invalid_manifest, False, False),
    ("6.4", 6, test_6_4_regression_catch, False, False),
]


def main():
    parser = argparse.ArgumentParser(description="MDEMG Live Validation Suite")
    parser.add_argument("--session", type=int, help="Run only tests from this session (1-6)")
    parser.add_argument("--test", type=str, help="Run only this test (e.g., 2.2)")
    parser.add_argument("--include-destructive", action="store_true",
                        help="Include destructive tests (4.5, 6.1, 6.2)")
    parser.add_argument("--report", type=str, help="Write JSON report to this path")
    args = parser.parse_args()

    # Resolve ports
    mdemg_port = resolve_port("MDEMG_PORT", "MDEMG_PORT")
    grafana_port = resolve_port("GRAFANA_PORT", "GRAFANA_PORT")
    tsdb_port = resolve_port("TSDB_HOST_PORT", "TSDB_HOST_PORT")

    if not mdemg_port:
        print("ERROR: Cannot resolve MDEMG_PORT from .env or environment")
        sys.exit(1)

    print(f"\nMDEMG Live Validation Suite")
    print(f"  API:     http://localhost:{mdemg_port}")
    print(f"  Grafana: http://localhost:{grafana_port}" if grafana_port else "  Grafana: not configured")
    print(f"  TSDB:    localhost:{tsdb_port}" if tsdb_port else "  TSDB:    not configured")
    print()

    results = Results()

    for test_id, session, test_fn, destructive, needs_ports in TEST_REGISTRY:
        # Filter by session
        if args.session is not None and session != args.session:
            continue
        # Filter by test ID
        if args.test is not None and test_id != args.test:
            continue
        # Skip destructive unless opted in
        if destructive and not args.include_destructive:
            results.skip(test_id, test_fn.__doc__.split("—")[0].strip() if test_fn.__doc__ else test_id,
                         "destructive — use --include-destructive")
            continue

        # Build kwargs based on what the test function needs
        import inspect
        sig = inspect.signature(test_fn)
        kwargs = {}
        if "port" in sig.parameters:
            kwargs["port"] = mdemg_port
        if "grafana_port" in sig.parameters:
            kwargs["grafana_port"] = grafana_port
        if "tsdb_port" in sig.parameters:
            kwargs["tsdb_port"] = tsdb_port

        try:
            test_fn(results, **kwargs)
        except Exception as e:
            results.fail(test_id, test_fn.__name__, f"exception: {e}")

    success = results.summary()

    if args.report:
        with open(args.report, "w") as f:
            json.dump(results.to_dict(), f, indent=2)
        print(f"\nReport written to {args.report}")

    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
