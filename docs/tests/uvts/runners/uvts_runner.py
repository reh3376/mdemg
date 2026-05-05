#!/usr/bin/env python3
"""
UVTS Runner — Semantic Accuracy Validation Test Runner.

Executes semantic accuracy validation tests defined by UVTS specs.
Supports inline grading (--base-url) and pre-graded display (--grades).

Usage:
    # Inline grading (calls MDEMG retrieve API + Grader, produces pass/fail report)
    python uvts_runner.py --spec specs/lnl_demo_validation.uvts.json --base-url http://localhost:9999

    # Question generation only (no grading)
    python uvts_runner.py --spec specs/lnl_demo_validation.uvts.json --profile standard

    # Display results from a pre-graded run
    python uvts_runner.py --spec specs/lnl_demo_validation.uvts.json --grades grades.json --report report.json
"""

import hashlib
import json
import os
import sys
import time
import random
import argparse
from datetime import datetime, timezone
from pathlib import Path
from typing import Dict, List, Optional, Any, Tuple

import sys as _sys
_sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..'))
from uxts_report import build_result as _canonical_result, build_report as _canonical_report, print_summary as _print_summary, save_report as _save_report

# Add parent directories to path for imports
SCRIPT_DIR = Path(__file__).resolve().parent
MDEMG_ROOT = SCRIPT_DIR.parent.parent.parent.parent
# grader_v4 + answer_generator live under docs/architecture/benchmarks/.
# Insert both paths so legacy `docs/benchmarks/` and current
# `docs/architecture/benchmarks/` resolve regardless of where the runner is invoked from.
sys.path.insert(0, str(MDEMG_ROOT / "docs" / "architecture" / "benchmarks"))
sys.path.insert(0, str(MDEMG_ROOT / "docs" / "benchmarks"))

try:
    import requests
    HAS_REQUESTS = True
except ImportError:
    HAS_REQUESTS = False

try:
    from validator import AnswerValidator
    from answer_generator import AnswerGenerator
    from grader_v4 import Grader
    HAS_GRADER = True
except ImportError:
    HAS_GRADER = False

# Phase 12 Epic 2: optional TSDB persistence (V0016). Both psycopg + cuid2 are
# vendored in the mdemg-ft-lora venv per Phase 11 precedent; treat as optional
# so the runner still functions in CI environments without DB access.
try:
    import psycopg
    from cuid2 import cuid_wrapper
    _CUID = cuid_wrapper()
    HAS_TSDB = True
except ImportError:
    HAS_TSDB = False

UVTS_VERSION = "1.1.0"


# ─── TSDB persistence helpers (Phase 12 Epic 2 / V0016) ─────────────────────

def _tsdb_dsn() -> str:
    """Build a libpq DSN from TSDB_* env vars (matches scripts/x11_jiminy_evaluate_rescue.py + conflict_tracker_test.go)."""
    return (
        f"host={os.environ.get('TSDB_HOST', 'localhost')} "
        f"port={os.environ.get('TSDB_PORT', '5433')} "
        f"user={os.environ.get('TSDB_USER', 'mdemg')} "
        f"password={os.environ.get('TSDB_PASSWORD', 'mdemg_metrics')} "
        f"dbname={os.environ.get('TSDB_DB', 'mdemg_metrics')}"
    )


def _persist_uvts_run_start(spec_path: str, profile: str, branch_label: Optional[str],
                            codebase_root: Optional[str], codebase_sha: Optional[str],
                            thresholds: Dict[str, Any]) -> Tuple[Optional[Any], Optional[str]]:
    """Insert a 'pending' uvts_runs row; return (conn, run_id) or (None, None) on failure.

    Failures are non-fatal — the runner's JSON-on-disk path stays intact.
    The caller should treat conn=None as "TSDB persistence skipped" and not
    try to persist per-question rows.
    """
    if not HAS_TSDB:
        return None, None
    try:
        # Compute spec SHA so the historical row is auditable even if the
        # spec file is later modified.
        try:
            spec_bytes = Path(spec_path).read_bytes()
            spec_sha = hashlib.sha256(spec_bytes).hexdigest()
        except OSError:
            spec_sha = None

        run_id = _CUID()
        conn = psycopg.connect(_tsdb_dsn(), connect_timeout=5)

        # Use psycopg3's Jsonb adapter for the threshold blob.
        from psycopg.types.json import Jsonb
        with conn.cursor() as cur:
            cur.execute(
                """INSERT INTO uvts_runs
                       (run_id, spec_path, spec_sha, branch_label, codebase_root,
                        codebase_sha, profile, gate_verdict, threshold_json, started_at)
                   VALUES (%s, %s, %s, %s, %s, %s, %s, 'pending', %s, NOW())""",
                (run_id, str(spec_path), spec_sha, branch_label, codebase_root,
                 codebase_sha, profile, Jsonb(thresholds or {})),
            )
        conn.commit()
        return conn, run_id
    except Exception as e:  # broad on purpose — TSDB persistence is best-effort
        print(f"WARN: TSDB run-start insert failed ({type(e).__name__}: {e}); continuing without persistence")
        return None, None


def _persist_uvts_run_finish(conn: Any, run_id: str, aggregate_score: float,
                             gate_verdict: str, grades: List[Any]) -> int:
    """Update uvts_runs with terminal state + bulk insert per-question rows.

    `grades` is List[Grade] (the dataclass) — we pull category/scores/etc.
    via .to_dict() for forensic preservation in raw_grade.
    Returns count of uvts_results rows persisted (0 on failure).
    """
    if conn is None or run_id is None:
        return 0
    try:
        from psycopg.types.json import Jsonb
        rows_inserted = 0
        with conn.cursor() as cur:
            # 1. Finalize the run row.
            cur.execute(
                """UPDATE uvts_runs
                       SET aggregate_score = %s,
                           gate_verdict   = %s,
                           completed_at   = NOW()
                     WHERE run_id = %s""",
                (aggregate_score, gate_verdict, run_id),
            )

            # 2. Per-question rows. One INSERT per row is fine at expected
            # rate (≤120/run, ~minutes between runs in CI); reach for
            # COPY only if we ever ship a corpus >5K questions.
            for g in grades:
                gd = g.to_dict() if hasattr(g, "to_dict") else g
                cur.execute(
                    """INSERT INTO uvts_results
                           (recorded_at, result_id, run_id, question_id, category,
                            final_score, evidence_score, semantic_score, concept_score,
                            evidence_tier, file_loc_ok, raw_grade)
                       VALUES (NOW(), %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)""",
                    (
                        _CUID(), run_id, str(gd.get("id", "")), gd.get("category"),
                        float(gd.get("scores", {}).get("final", 0.0)),
                        float(gd.get("scores", {}).get("evidence", 0.0)),
                        float(gd.get("scores", {}).get("semantic", 0.0)),
                        float(gd.get("scores", {}).get("concept", 0.0)),
                        gd.get("evidence_details", {}).get("tier"),
                        bool(gd.get("evidence_details", {}).get("line_tolerance_met", False)),
                        Jsonb(gd),
                    ),
                )
                rows_inserted += 1
        conn.commit()
        conn.close()
        return rows_inserted
    except Exception as e:
        print(f"WARN: TSDB run-finish persistence failed ({type(e).__name__}: {e}); run row may be left in 'pending' state")
        try:
            conn.close()
        except Exception:
            pass
        return 0



def _validate_supported_features(spec_dict: Dict[str, Any]) -> List[str]:
    """Check which features the runner supports."""
    issues = []
    if not HAS_REQUESTS:
        issues.append("PARITY: requests library not installed (pip install requests)")
    if not HAS_GRADER:
        issues.append("PARITY: grader_v4/answer_generator not found on sys.path")
    return issues


class UVTSRunner:
    """
    Executes UVTS validation tests.
    """

    def __init__(self, spec_path: str, profile: str = "standard"):
        """
        Initialize runner with a UVTS spec.

        Args:
            spec_path: Path to UVTS spec file
            profile: Profile to use (quick, standard, full)
        """
        self.spec_path = Path(spec_path)
        self.profile = profile

        with open(spec_path) as f:
            self.spec = json.load(f)

        self._validate_spec_shape()

        self.validation = self.spec["validation"]
        self.thresholds = self.spec["thresholds"]
        self.categories = self.spec.get("categories", [])
        self.questions_config = self.spec["questions"]
        self.profiles = self.spec.get("profiles", {})
        self.scoring = self.spec.get("scoring", {})
        self.output_config = self.spec.get("output", {})

        # Apply profile settings
        if profile in self.profiles:
            self.profile_settings = self.profiles[profile]
        else:
            self.profile_settings = self.profiles.get("standard", {})

    def _validate_spec_shape(self) -> None:
        """Validate runner-required spec shape before accessing required keys."""
        required = ("uvts_version", "validation", "thresholds", "questions")
        missing = [k for k in required if k not in self.spec]
        if not missing:
            return

        # Legacy phase draft shape: config/setup/test_cases (not executable by this runner).
        if {"config", "test_cases"}.issubset(self.spec.keys()):
            raise ValueError(
                "Unsupported legacy UVTS draft format (config/test_cases). "
                "Use a canonical UVTS spec (uvts_version/validation/thresholds/questions)."
            )

        raise ValueError(
            "Invalid UVTS spec shape; missing required key(s): "
            + ", ".join(missing)
        )

    def load_questions(self, master_path: str) -> List[Dict]:
        """Load and sample questions based on spec configuration."""
        with open(master_path) as f:
            data = json.load(f)

        all_questions = data.get("questions", [])

        # Get questions per category from profile
        qpc = self.profile_settings.get("questions_per_category", 5)

        if qpc == -1:
            # Full: return all questions
            return all_questions

        # Stratified sampling
        by_category = {}
        for q in all_questions:
            cat = q.get("category", "unknown")
            if cat not in by_category:
                by_category[cat] = []
            by_category[cat].append(q)

        # Sample from each category
        seed = self.questions_config.get("seed", 42)
        random.seed(seed)

        sampled = []
        for cat_config in self.categories:
            cat_name = cat_config["name"]
            count = min(qpc, cat_config.get("count", qpc))

            if cat_name in by_category:
                available = by_category[cat_name]
                sample = random.sample(available, min(count, len(available)))
                sampled.extend(sample)

        return sampled

    def create_agent_questions(self, questions: List[Dict]) -> List[Dict]:
        """Create agent question list (without answers)."""
        return [
            {
                "id": q["id"],
                "category": q["category"],
                "question": q["question"]
            }
            for q in questions
        ]

    def run(self, output_dir: str) -> Dict[str, Any]:
        """
        Execute the validation test.

        Args:
            output_dir: Directory for output files

        Returns:
            Dictionary with results summary
        """
        output_path = Path(output_dir)
        output_path.mkdir(parents=True, exist_ok=True)

        start_time = time.time()

        # Load questions
        master_path = self.spec_path.parent.parent.parent.parent / self.questions_config["source_file"]
        if not master_path.exists():
            # Try relative to MDEMG root
            master_path = MDEMG_ROOT / self.questions_config["source_file"]

        questions = self.load_questions(str(master_path))
        agent_questions = self.create_agent_questions(questions)

        # Save sampled questions
        agent_file = output_path / "questions_agent.json"
        master_file = output_path / "questions_master.json"

        with open(agent_file, "w") as f:
            json.dump({"questions": agent_questions}, f, indent=2)

        with open(master_file, "w") as f:
            json.dump({"questions": questions}, f, indent=2)

        print(f"UVTS Validation: {self.validation['name']}")
        print(f"Profile: {self.profile}")
        print(f"Questions: {len(questions)}")
        print(f"Categories: {len(set(q['category'] for q in questions))}")
        print(f"Thresholds: mean_score >= {self.thresholds['mean_score']}")
        print("-" * 60)

        # Results structure
        results = {
            "spec_name": self.validation["name"],
            "profile": self.profile,
            "started_at": datetime.now().isoformat(),
            "questions_count": len(questions),
            "categories": list(set(q["category"] for q in questions)),
            "thresholds": self.thresholds,
            "answers": [],
            "grades": [],
            "summary": {},
            "token_usage": {"input": 0, "output": 0, "total": 0}
        }

        # In a real run, this would:
        # 1. Call MDEMG API for each question
        # 2. Use AnswerGenerator to synthesize answers
        # 3. Use Grader to score answers
        # 4. Track token usage

        # For demo purposes, we output the configuration
        config_summary = {
            "validation_name": self.validation["name"],
            "profile": self.profile,
            "codebase": self.validation["codebase"],
            "space_id": self.validation["space_id"],
            "model": self.validation.get("model", "sonnet"),
            "questions_count": len(questions),
            "categories": {cat: len([q for q in questions if q["category"] == cat])
                         for cat in set(q["category"] for q in questions)},
            "thresholds": self.thresholds,
            "output_files": {
                "agent_questions": str(agent_file),
                "master_questions": str(master_file),
                "answers": str(output_path / "answers.jsonl"),
                "grades": str(output_path / "grades.json"),
                "summary": str(output_path / "summary.json")
            },
            "run_command": f"""
python3 docs/benchmarks/run_benchmark_v4.py \\
    --questions {agent_file} \\
    --master {master_file} \\
    --output-dir {output_path} \\
    --codebase {self.validation['codebase']} \\
    --space-id {self.validation['space_id']} \\
    --model {self.validation.get('model', 'sonnet')}
"""
        }

        # Save config
        with open(output_path / "config.json", "w") as f:
            json.dump(config_summary, f, indent=2)

        elapsed = time.time() - start_time
        print(f"\nSetup complete in {elapsed:.2f}s")
        print(f"\nTo run the validation:")
        print(config_summary["run_command"])
        print(f"\nTo grade results:")
        print(f"""
python3 docs/benchmarks/grader_v4.py \\
    {output_path}/answers_mdemg_run1.jsonl \\
    {master_file} \\
    {output_path}/grades.json
""")

        return config_summary


def _build_graded_results(grades_file: str, spec_path: str, thresholds: Dict) -> List[Dict]:
    """Build canonical results from a graded UVTS run."""
    with open(grades_file) as f:
        data = json.load(f)

    grades = data.get("grades", [])
    summary = data.get("summary", {})

    assertions_evaluated = 0
    assertions_passed_count = 0
    failures = []

    # Check mean_score threshold
    mean_score = summary.get("mean_score", 0)
    threshold_mean = thresholds.get("mean_score", 0.795)
    assertions_evaluated += 1
    if mean_score >= threshold_mean:
        assertions_passed_count += 1
    else:
        failures.append(f"mean_score: {mean_score:.3f} < {threshold_mean}")

    # Check strong_evidence_pct threshold
    if "strong_evidence_pct" in thresholds:
        assertions_evaluated += 1
        evidence_tiers = {}
        for g in grades:
            tier = g.get("evidence_details", {}).get("tier", "none")
            evidence_tiers[tier] = evidence_tiers.get(tier, 0) + 1
        strong_pct = (evidence_tiers.get("strong", 0) / len(grades) * 100) if grades else 0
        if strong_pct >= thresholds["strong_evidence_pct"]:
            assertions_passed_count += 1
        else:
            failures.append(f"strong_evidence_pct: {strong_pct:.1f}% < {thresholds['strong_evidence_pct']}%")

    # Check high_score_rate_pct threshold
    if "high_score_rate_pct" in thresholds:
        assertions_evaluated += 1
        high_score_count = len([g for g in grades if g.get("scores", {}).get("final", 0) >= 0.8])
        high_score_pct = (high_score_count / len(grades) * 100) if grades else 0
        if high_score_pct >= thresholds["high_score_rate_pct"]:
            assertions_passed_count += 1
        else:
            failures.append(f"high_score_rate_pct: {high_score_pct:.1f}% < {thresholds['high_score_rate_pct']}%")

    # Check max_token_usage threshold
    if "max_token_usage" in thresholds:
        assertions_evaluated += 1
        token_total = summary.get("token_usage", {}).get("total", 0)
        if token_total <= thresholds["max_token_usage"]:
            assertions_passed_count += 1
        else:
            failures.append(f"max_token_usage: {token_total} > {thresholds['max_token_usage']}")

    status = "pass" if not failures else "fail"

    return [_canonical_result(
        spec_path=str(spec_path),
        status=status,
        hash_verified=None,
        assertions_evaluated=assertions_evaluated,
        assertions_passed=assertions_passed_count,
        failures=failures if failures else None,
    )]


def display_results(grades_file: str, thresholds: Dict) -> None:
    """Display formatted validation results."""
    with open(grades_file) as f:
        data = json.load(f)

    grades = data.get("grades", [])
    summary = data.get("summary", {})

    print("=" * 60)
    print("UVTS VALIDATION RESULTS")
    print("=" * 60)
    print(f"\nQuestions: {len(grades)}")
    print(f"Mean Score: {summary.get('mean_score', 0):.3f}")
    print(f"Threshold: >= {thresholds.get('mean_score', 0.795)}")

    passed = summary.get('mean_score', 0) >= thresholds.get('mean_score', 0.795)
    print(f"Result: {'PASS' if passed else 'FAIL'}")

    print("\n" + "-" * 60)
    print("SCORES BY CATEGORY")
    print("-" * 60)

    by_cat = {}
    for g in grades:
        cat = g.get("category", "unknown")
        if cat not in by_cat:
            by_cat[cat] = []
        by_cat[cat].append(g.get("scores", {}).get("final", 0))

    for cat in sorted(by_cat.keys()):
        scores = by_cat[cat]
        avg = sum(scores) / len(scores) if scores else 0
        status = "PASS" if avg >= thresholds.get("min_category_score", 0.6) else "WARN"
        print(f"  [{status}] {cat:30s}: {avg:.3f} ({len(scores)} questions)")

    print("\n" + "-" * 60)
    print("EVIDENCE QUALITY")
    print("-" * 60)

    evidence_tiers = {}
    for g in grades:
        tier = g.get("evidence_details", {}).get("tier", "none")
        evidence_tiers[tier] = evidence_tiers.get(tier, 0) + 1

    for tier in ["strong", "moderate", "weak", "minimal", "none"]:
        count = evidence_tiers.get(tier, 0)
        pct = (count / len(grades) * 100) if grades else 0
        print(f"  {tier:10s}: {count:3d} ({pct:5.1f}%)")

    strong_pct = (evidence_tiers.get("strong", 0) / len(grades) * 100) if grades else 0
    threshold_strong = thresholds.get("strong_evidence_pct", 60.0)
    print(f"\n  Strong evidence: {strong_pct:.1f}% (threshold: {threshold_strong}%)")
    print(f"  {'PASS' if strong_pct >= threshold_strong else 'FAIL'}")

    print("\n" + "-" * 60)
    print("HIGH SCORE RATE")
    print("-" * 60)

    high_score_count = len([g for g in grades if g.get("scores", {}).get("final", 0) >= 0.8])
    high_score_pct = (high_score_count / len(grades) * 100) if grades else 0
    threshold_high = thresholds.get("high_score_rate_pct", 70.0)

    print(f"  Scores >= 0.8: {high_score_count}/{len(grades)} ({high_score_pct:.1f}%)")
    print(f"  Threshold: {threshold_high}%")
    print(f"  {'PASS' if high_score_pct >= threshold_high else 'FAIL'}")

    # Token usage if available
    token_usage = summary.get("token_usage", {})
    if token_usage:
        print("\n" + "-" * 60)
        print("TOKEN USAGE")
        print("-" * 60)
        print(f"  Input:  {token_usage.get('input', 0):,}")
        print(f"  Output: {token_usage.get('output', 0):,}")
        print(f"  Total:  {token_usage.get('total', 0):,}")

        max_tokens = thresholds.get("max_token_usage")
        if max_tokens:
            total = token_usage.get("total", 0)
            print(f"  Limit:  {max_tokens:,}")
            print(f"  {'PASS' if total <= max_tokens else 'OVER LIMIT'}")

    print("\n" + "=" * 60)


def run_inline_grading(spec_path: str, base_url: str, profile: str = "standard",
                       output_dir: Optional[str] = None,
                       retrieve_timeout_s: float = 30.0,
                       space_id_override: Optional[str] = None,
                       persist_tsdb: bool = False,
                       branch_label: Optional[str] = None,
                       codebase_sha: Optional[str] = None,
                       extra_url_params: Optional[str] = None) -> List[Dict]:
    """Run inline grading: retrieve from MDEMG, build answers, grade with Grader.

    GAP-04: This replaces the manual two-step pipeline (run_benchmark_v4 + grader_v4)
    with a single inline invocation.
    """
    if not HAS_REQUESTS:
        return [_canonical_result(
            spec_path=spec_path, status="error", hash_verified=None,
            assertions_evaluated=0, assertions_passed=0,
            error="requests library required for inline grading (pip install requests)",
        )]

    runner = UVTSRunner(spec_path, profile)

    # Resolve master question file path (mirror UVTSRunner.run() resolution).
    master_path = runner.spec_path.parent.parent.parent.parent / runner.questions_config["source_file"]
    if not master_path.exists():
        master_path = MDEMG_ROOT / runner.questions_config["source_file"]
    if not master_path.exists():
        return [_canonical_result(
            spec_path=spec_path, status="error", hash_verified=None,
            assertions_evaluated=0, assertions_passed=0,
            error=f"Master question file not found at expected paths: {runner.questions_config['source_file']}",
        )]

    questions = runner.load_questions(str(master_path))

    # Build output directory
    if output_dir is None:
        output_dir = f"uvts_inline_{datetime.now().strftime('%Y%m%d_%H%M%S')}"
    output_path = Path(output_dir)
    output_path.mkdir(parents=True, exist_ok=True)

    print(f"UVTS Inline Grading: {runner.validation['name']}")
    print(f"Profile: {profile} | Questions: {len(questions)} | Server: {base_url}")
    print("-" * 60)

    space_id = space_id_override or runner.validation.get("space_id", "mdemg-dev")
    answers = []
    grade_list = []
    total_scores = []

    # Phase 12 Epic 2: optional TSDB run-row creation (--persist-tsdb).
    # On any failure (DB unreachable, missing schema, etc.) the runner falls
    # back to JSON-only output via conn=None / run_id=None.
    tsdb_conn, uvts_run_id = (None, None)
    if persist_tsdb:
        tsdb_conn, uvts_run_id = _persist_uvts_run_start(
            spec_path=spec_path,
            profile=profile,
            branch_label=branch_label,
            codebase_root=runner.validation.get("codebase"),
            codebase_sha=codebase_sha,
            thresholds=runner.thresholds,
        )
        if uvts_run_id:
            print(f"  TSDB persist: enabled, run_id={uvts_run_id}")
        else:
            print(f"  TSDB persist: requested but unavailable; continuing with JSON only")

    for i, q in enumerate(questions):
        # Step 1: Call MDEMG retrieve API
        try:
            # Field is query_text per internal/models/models.go RetrieveRequest;
            # earlier `query` value silently 400'd at the validator.
            #
            # Phase 14.1 Epic 2 — pass per-question category hint into the
            # retrieve body so the sparse-gate dispatch can pick a per-category
            # override when configured. No-op when SPARSE_GATE_CATEGORY_OVERRIDES
            # is empty (Phase 14 behavior preserved).
            payload = {"space_id": space_id, "query_text": q["question"], "top_k": 5}
            if q.get("category"):
                payload["category"] = q["category"]
            url = f"{base_url}/v1/memory/retrieve"
            if extra_url_params:
                url = f"{url}?{extra_url_params.lstrip('?')}"
            resp = requests.post(
                url,
                json=payload,
                timeout=retrieve_timeout_s,
            )
            if resp.status_code != 200:
                print(f"  [{i+1}/{len(questions)}] Retrieve failed: HTTP {resp.status_code}")
                answers.append({
                    "id": q.get("id", i+1), "question": q["question"],
                    "answer": "", "files_consulted": [], "file_line_refs": [],
                    "mdemg_used": False,
                })
                continue
            results = resp.json().get("results", [])
        except Exception as e:
            print(f"  [{i+1}/{len(questions)}] Retrieve error: {e}")
            answers.append({
                "id": q.get("id", i+1), "question": q["question"],
                "answer": "", "files_consulted": [], "file_line_refs": [],
                "mdemg_used": False,
            })
            continue

        # Step 2: Build answer from retrieval results (no LLM needed for basic grading)
        file_refs = []
        files_consulted = []
        answer_parts = []
        for r in results[:5]:
            content = r.get("content", "")
            path = r.get("path", r.get("source", ""))
            if path:
                files_consulted.append(path)
                # Extract line number if available
                line = r.get("line", r.get("line_start", 0))
                if line:
                    file_refs.append(f"{path}:{line}")
                else:
                    file_refs.append(path)
            if content:
                answer_parts.append(content[:200])

        answer_text = " ".join(answer_parts) if answer_parts else "(no retrieval results)"
        answer_obj = {
            "id": q.get("id", i+1),
            "question": q["question"],
            "answer": answer_text,
            "files_consulted": files_consulted,
            "file_line_refs": file_refs,
            "mdemg_used": True,
        }
        answers.append(answer_obj)
        print(f"  [{i+1}/{len(questions)}] {q['category']}: retrieved {len(results)} results")

    # Step 3: Grade with Grader if available
    if HAS_GRADER:
        try:
            # Grader.__init__ takes the questions list directly (per grader_v4.py:300);
            # earlier code passed a path string here, which made Python attempt to
            # iterate the path as if it were a list of dicts and surfaced as
            # "string indices must be integers, not 'str'".
            grader = Grader(questions)
            grades, aggregate = grader.grade_all(answers)

            # _build_graded_results expects {"summary": {...}, "grades": [...]} on
            # disk. AggregateMetrics is a @dataclass, but its `mean` field is the
            # canonical aggregate score — _build_graded_results reads
            # summary.mean_score, so we map it explicitly.
            from dataclasses import asdict as _asdict
            agg_dict = _asdict(aggregate)
            grades_data = {
                "summary": {
                    "mean_score": agg_dict.get("mean", 0.0),
                    "aggregate": agg_dict,
                    "token_usage": {"total": 0},
                },
                "grades": [g.to_dict() for g in grades],
            }

            # Persist questions + grades for forensic re-grading downstream.
            master_file = output_path / "questions_master.json"
            with open(master_file, "w") as f:
                json.dump({"questions": questions}, f, indent=2)
            grades_file = output_path / "grades.json"
            with open(grades_file, "w") as f:
                json.dump(grades_data, f, indent=2)

            canonical = _build_graded_results(str(grades_file), spec_path, runner.thresholds)

            # Phase 12 Epic 2: TSDB run-finish (best-effort; no-op when conn=None).
            if uvts_run_id is not None:
                gate_verdict = canonical[0].get("status", "fail") if canonical else "fail"
                # Constrain to the V0016 CHECK ('pending','pass','fail').
                if gate_verdict not in ("pass", "fail"):
                    gate_verdict = "fail"
                rows = _persist_uvts_run_finish(
                    tsdb_conn, uvts_run_id,
                    aggregate_score=float(agg_dict.get("mean", 0.0)),
                    gate_verdict=gate_verdict,
                    grades=grades,
                )
                print(f"  TSDB persist: completed run_id={uvts_run_id} verdict={gate_verdict} per_question_rows={rows}")

            return canonical
        except Exception as e:
            print(f"Grading error: {e}")
            # Best-effort: close any open TSDB conn so we don't leak.
            if tsdb_conn is not None:
                try:
                    tsdb_conn.close()
                except Exception:
                    pass
            return [_canonical_result(
                spec_path=spec_path, status="error", hash_verified=None,
                assertions_evaluated=0, assertions_passed=0,
                error=f"Grading failed: {e}",
            )]
    else:
        # Without Grader, do basic mean-score from retrieval confidence
        for answer in answers:
            score = 0.5 if answer.get("mdemg_used") else 0.0
            total_scores.append(score)

        mean_score = sum(total_scores) / len(total_scores) if total_scores else 0
        threshold = runner.thresholds.get("mean_score", 0.795)
        status = "pass" if mean_score >= threshold else "fail"

        return [_canonical_result(
            spec_path=spec_path, status=status, hash_verified=None,
            assertions_evaluated=1, assertions_passed=1 if status == "pass" else 0,
            failures=[f"mean_score: {mean_score:.3f} < {threshold}"] if status == "fail" else None,
        )]


def main():
    parser = argparse.ArgumentParser(description="UVTS Validation Test Runner")
    parser.add_argument("--spec", required=True, help="Path to UVTS spec file")
    parser.add_argument("--profile", default="standard",
                       choices=["quick", "standard", "full"],
                       help="Test profile to use")
    parser.add_argument("--output-dir", default=None,
                       help="Output directory (default: auto-generated)")
    parser.add_argument("--grades", help="Path to grades.json to display results")
    parser.add_argument("--base-url", help="MDEMG server URL for inline grading (GAP-04)")
    parser.add_argument("--retrieve-timeout-s", type=float, default=30.0,
                        help="Per-question retrieve API timeout in seconds (default 30; bump for slow dev pipelines)")
    parser.add_argument("--space-id", default=None,
                        help="Override the spec's validation.space_id (e.g. for cross-space A/B). If unset, uses spec value.")
    parser.add_argument("--persist-tsdb", action="store_true",
                        help="Phase 12: write per-question rows to V0016 uvts_results + run-row to uvts_runs. Off by default; safe no-op if TSDB unreachable.")
    parser.add_argument("--branch-label", default=None,
                        help="Phase 12: free-text branch label for A/B harness grouping (e.g. 'baseline' / 'candidate'). Persisted with --persist-tsdb.")
    parser.add_argument("--codebase-sha", default=None,
                        help="Phase 12: codebase SHA pin for the validation target (audits historical runs against frozen commits). Persisted with --persist-tsdb.")
    parser.add_argument("--report", help="Output file path for canonical JSON report")
    parser.add_argument("--extra-url-params", default=None,
                        help="Phase 14.2: extra URL query string appended to /v1/memory/retrieve "
                             "(e.g. 'context=auto' to derive query fingerprint server-side, "
                             "'strict_context=true' for strict-mode pre-filter).")

    args = parser.parse_args()

    report_start = datetime.now(timezone.utc)

    # Inline grading mode (GAP-04): call MDEMG API + Grader in a single invocation
    if args.base_url:
        try:
            canonical_results = run_inline_grading(
                args.spec, args.base_url, args.profile, args.output_dir,
                retrieve_timeout_s=args.retrieve_timeout_s,
                space_id_override=args.space_id,
                persist_tsdb=args.persist_tsdb,
                branch_label=args.branch_label,
                codebase_sha=args.codebase_sha,
                extra_url_params=args.extra_url_params,
            )
        except Exception as e:
            print(f"ERROR: Inline grading failed: {e}", file=sys.stderr)
            canonical_results = [_canonical_result(
                spec_path=str(args.spec), status="error", hash_verified=None,
                assertions_evaluated=0, assertions_passed=0,
                error=f"Inline grading failed: {e}",
            )]

        report = _canonical_report(
            framework="uvts",
            framework_version=UVTS_VERSION,
            results=canonical_results,
            start_time=report_start,
        )
        _print_summary(report)
        if args.report:
            _save_report(report, args.report)

        all_passed = all(r["status"] == "pass" for r in canonical_results)
        sys.exit(0 if all_passed else 1)

    if args.grades:
        # Display mode - show results from a completed run and build canonical report
        spec_path = Path(args.spec)
        try:
            with open(spec_path) as f:
                spec = json.load(f)
        except Exception as e:
            print(f"ERROR: Failed to read spec: {e}", file=sys.stderr)
            sys.exit(1)

        thresholds = spec.get("thresholds", {})
        display_results(args.grades, thresholds)

        # Build canonical report from graded results
        canonical_results = _build_graded_results(args.grades, str(spec_path), thresholds)
        report = _canonical_report(
            framework="uvts",
            framework_version=UVTS_VERSION,
            results=canonical_results,
            start_time=report_start,
        )

        _print_summary(report)

        if args.report:
            _save_report(report, args.report)

        # Exit based on graded results
        all_passed = all(r["status"] == "pass" for r in canonical_results)
        sys.exit(0 if all_passed else 1)

    # Run mode (question generation only -- no assertions)
    if args.output_dir:
        output_dir = args.output_dir
    else:
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        output_dir = f"uvts_run_{timestamp}"

    try:
        runner = UVTSRunner(args.spec, args.profile)
        runner.run(output_dir)
    except ValueError as e:
        print(f"ERROR: {e}", file=sys.stderr)
        sys.exit(1)
    except FileNotFoundError as e:
        print(f"ERROR: Missing input file: {e}", file=sys.stderr)
        sys.exit(1)
    except Exception as e:
        print(f"ERROR: UVTS runner failed: {e}", file=sys.stderr)
        sys.exit(1)

    print(f"\nOutput written to: {output_dir}")

    # Without --grades or --base-url, this is question-generation only.
    canonical_results = [_canonical_result(
        spec_path=str(args.spec),
        status="error",
        hash_verified=None,
        assertions_evaluated=0,
        assertions_passed=0,
        error="No grading performed. Use --base-url for inline grading "
              "or --grades for pre-graded display.",
    )]

    report = _canonical_report(
        framework="uvts",
        framework_version=UVTS_VERSION,
        results=canonical_results,
        start_time=report_start,
    )

    _print_summary(report)

    if args.report:
        _save_report(report, args.report)


if __name__ == "__main__":
    main()
