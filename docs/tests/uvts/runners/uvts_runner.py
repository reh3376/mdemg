#!/usr/bin/env python3
"""
UVTS Runner - Universal Validation Test Specification Runner

Executes semantic accuracy validation tests defined by UVTS specs.
Integrates with MDEMG retrieval and LLM answer synthesis.

Usage:
    # Run standard profile
    python uvts_runner.py --spec specs/lnl_demo_validation.uvts.json --profile standard

    # Run quick profile
    python uvts_runner.py --spec specs/lnl_demo_validation.uvts.json --profile quick

    # Run with custom output directory
    python uvts_runner.py --spec specs/lnl_demo_validation.uvts.json --output-dir /tmp/uvts_run
"""

import json
import os
import sys
import time
import random
import argparse
from datetime import datetime
from pathlib import Path
from typing import Dict, List, Optional, Any

# Add parent directories to path for imports
SCRIPT_DIR = Path(__file__).resolve().parent
MDEMG_ROOT = SCRIPT_DIR.parent.parent.parent.parent
sys.path.insert(0, str(MDEMG_ROOT / "docs" / "benchmarks"))

try:
    from validator import AnswerValidator
    from answer_generator import AnswerGenerator
    from grader_v4 import Grader
except ImportError as e:
    print(f"Warning: Could not import benchmark modules: {e}")
    print("Some functionality may be limited.")


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
    print(f"Result: {'✅ PASS' if passed else '❌ FAIL'}")

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
        status = "✅" if avg >= thresholds.get("min_category_score", 0.6) else "⚠️"
        print(f"  {status} {cat:30s}: {avg:.3f} ({len(scores)} questions)")

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
    print(f"  {'✅ PASS' if strong_pct >= threshold_strong else '❌ FAIL'}")

    print("\n" + "-" * 60)
    print("HIGH SCORE RATE")
    print("-" * 60)

    high_score_count = len([g for g in grades if g.get("scores", {}).get("final", 0) >= 0.8])
    high_score_pct = (high_score_count / len(grades) * 100) if grades else 0
    threshold_high = thresholds.get("high_score_rate_pct", 70.0)

    print(f"  Scores >= 0.8: {high_score_count}/{len(grades)} ({high_score_pct:.1f}%)")
    print(f"  Threshold: {threshold_high}%")
    print(f"  {'✅ PASS' if high_score_pct >= threshold_high else '❌ FAIL'}")

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
            print(f"  {'✅ PASS' if total <= max_tokens else '❌ OVER LIMIT'}")

    print("\n" + "=" * 60)


def main():
    parser = argparse.ArgumentParser(description="UVTS Validation Test Runner")
    parser.add_argument("--spec", required=True, help="Path to UVTS spec file")
    parser.add_argument("--profile", default="standard",
                       choices=["quick", "standard", "full"],
                       help="Test profile to use")
    parser.add_argument("--output-dir", default=None,
                       help="Output directory (default: auto-generated)")
    parser.add_argument("--grades", help="Path to grades.json to display results")

    args = parser.parse_args()

    if args.grades:
        # Display mode - show results from a completed run
        spec_path = Path(args.spec)
        try:
            with open(spec_path) as f:
                spec = json.load(f)
        except Exception as e:
            print(f"ERROR: Failed to read spec: {e}", file=sys.stderr)
            sys.exit(1)
        display_results(args.grades, spec.get("thresholds", {}))
        return

    # Run mode
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


if __name__ == "__main__":
    main()
