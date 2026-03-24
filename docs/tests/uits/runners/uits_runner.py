#!/usr/bin/env python3
"""
Universal Iterative-Improvement Test Specification (UITS) Test Runner

Validates T1-encoded content comprehension via LLM judging and drives iterative
optimization of source files toward convergence against scoring profile thresholds.
Model-agnostic — uses the OpenAI API (or any compatible endpoint) via the
`requests` library.

Usage:
    # Validate a single spec (one-shot scoring, no optimization)
    python uits_runner.py validate --spec specs/flow_retrieval.uits.json

    # Validate all specs in a directory
    python uits_runner.py validate-all --spec-dir specs/

    # Run iterative optimization loop
    python uits_runner.py optimize --spec specs/flow_retrieval.uits.json

    # Generate canonical JSON report
    python uits_runner.py validate-all --spec-dir specs/ --report report.json

    # Add spec hashes (config.sha256)
    python uits_runner.py add-hashes --spec-dir specs/

    # Verify spec hashes
    python uits_runner.py verify-hashes --spec-dir specs/

    # List scoring profiles defined in a spec
    python uits_runner.py profiles --spec specs/flow_retrieval.uits.json

Author: reh3376
Version: 1.0.0
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import time
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

# ============================================================
# sys.path: make shared infra importable from docs/tests/
# ============================================================

import sys as _sys
_sys.path.insert(0, os.path.join(os.path.dirname(os.path.abspath(__file__)), '..', '..'))
from uxts_report import build_result, build_report, print_summary, save_report
from uxts_runner_core import sha256_spec_without_field, canonical_result_status

# ============================================================
# LLM CLIENT (requests-based, OpenAI-compatible)
# ============================================================

try:
    import requests as _requests
except ImportError:
    print("ERROR: 'requests' library required. Install with: pip install requests", file=sys.stderr)
    sys.exit(1)


# ============================================================
# CONSTANTS
# ============================================================

UITS_VERSION = "1.0.0"

_REPO_ROOT = Path(__file__).resolve().parents[4]  # mdemg/

_UITS_KNOWN_TOP_FIELDS = {
    "uits_version", "metadata", "content", "ground_truth",
    "scoring_profiles", "thresholds", "optimization", "history", "config",
}
_UITS_KNOWN_METADATA_FIELDS = {
    "name", "description", "content_type", "tags", "author", "created", "last_optimized",
}
_UITS_KNOWN_CONTENT_FIELDS = {
    "source_file", "human_readable_file", "encoding_format", "injection_context",
}
_UITS_KNOWN_GROUND_TRUTH_FIELDS = {"questions"}
_UITS_KNOWN_QUESTION_FIELDS = {"id", "question", "answer", "difficulty", "section_tag"}
_UITS_KNOWN_SCORING_PROFILES_FIELDS = {"active_profile", "profiles"}
_UITS_KNOWN_PROFILE_FIELDS = {
    "profile_id", "version", "created", "supersedes", "rationale",
    "weights", "thresholds", "difficulty_weights",
}
_UITS_KNOWN_OPTIMIZATION_FIELDS = {
    "max_iterations", "convergence_window", "regeneration_strategy", "model",
}


# ============================================================
# PARITY CHECK (Section 8 — Portable Agent Spec v2.2.0)
# ============================================================

def _validate_supported_features(spec: dict) -> List[str]:
    """Detect spec fields this runner does not implement. Returns list of parity failure strings."""
    errors: List[str] = []

    unknown_top = set(spec.keys()) - _UITS_KNOWN_TOP_FIELDS
    if unknown_top:
        errors.append(f"PARITY FAILURE: Unimplemented top-level fields: {unknown_top}")

    if isinstance(spec.get("metadata"), dict):
        unknown = set(spec["metadata"].keys()) - _UITS_KNOWN_METADATA_FIELDS
        if unknown:
            errors.append(f"PARITY FAILURE: Unimplemented metadata fields: {unknown}")

    if isinstance(spec.get("content"), dict):
        unknown = set(spec["content"].keys()) - _UITS_KNOWN_CONTENT_FIELDS
        if unknown:
            errors.append(f"PARITY FAILURE: Unimplemented content fields: {unknown}")

    if isinstance(spec.get("ground_truth"), dict):
        unknown = set(spec["ground_truth"].keys()) - _UITS_KNOWN_GROUND_TRUTH_FIELDS
        if unknown:
            errors.append(f"PARITY FAILURE: Unimplemented ground_truth fields: {unknown}")
        for q in spec["ground_truth"].get("questions", []):
            if isinstance(q, dict):
                unknown_q = set(q.keys()) - _UITS_KNOWN_QUESTION_FIELDS
                if unknown_q:
                    errors.append(f"PARITY FAILURE: Unimplemented question fields: {unknown_q}")
                    break  # report once

    if isinstance(spec.get("scoring_profiles"), dict):
        unknown = set(spec["scoring_profiles"].keys()) - _UITS_KNOWN_SCORING_PROFILES_FIELDS
        if unknown:
            errors.append(f"PARITY FAILURE: Unimplemented scoring_profiles fields: {unknown}")
        for profile in spec["scoring_profiles"].get("profiles", []):
            if isinstance(profile, dict):
                unknown_p = set(profile.keys()) - _UITS_KNOWN_PROFILE_FIELDS
                if unknown_p:
                    errors.append(f"PARITY FAILURE: Unimplemented profile fields: {unknown_p}")
                    break  # report once

    if isinstance(spec.get("optimization"), dict):
        unknown = set(spec["optimization"].keys()) - _UITS_KNOWN_OPTIMIZATION_FIELDS
        if unknown:
            errors.append(f"PARITY FAILURE: Unimplemented optimization fields: {unknown}")

    return errors


# ============================================================
# DATA TYPES
# ============================================================

@dataclass
class TrialResult:
    question_id: str
    question: str
    ground_truth: str
    agent_answer: str
    judge_score: float      # 0-10
    judge_reason: str
    difficulty: str
    section_tag: str = ""
    error: str = ""


@dataclass
class CompositeScore:
    comprehension: float   # 0-1 (normalized judge mean / 10)
    compaction: float      # 0-1
    token_efficiency: float  # 0-1 (clamped)
    fidelity: float        # 0-1 (placeholder: 1.0)
    composite: float       # weighted sum


@dataclass
class WeakSection:
    key: str               # section_tag or question_id
    avg_score: float
    questions: List[dict]  # [{question, answer, score, reason}, ...]


@dataclass
class SpecResult:
    spec_path: str
    status: str            # "pass" | "fail"
    trials: List[TrialResult] = field(default_factory=list)
    composite: Optional[CompositeScore] = None
    hash_verified: Optional[bool] = None
    duration_ms: float = 0.0
    failures: List[str] = field(default_factory=list)
    warnings: List[str] = field(default_factory=list)


# ============================================================
# API KEY LOADER
# ============================================================

def _load_api_key() -> str:
    """Load OPENAI_API_KEY from env or .env file at repo root."""
    key = os.environ.get("OPENAI_API_KEY", "")
    if not key:
        env_path = _REPO_ROOT / ".env"
        if env_path.exists():
            for line in env_path.read_text().splitlines():
                line = line.strip()
                if line.startswith("OPENAI_API_KEY="):
                    key = line.split("=", 1)[1].strip().strip('"').strip("'")
                    break
    if not key:
        print("ERROR: OPENAI_API_KEY not set in environment or .env file", file=sys.stderr)
        sys.exit(1)
    return key


# ============================================================
# TOKEN ESTIMATION
# ============================================================

def _estimate_tokens(text: str) -> int:
    """Estimate token count: len(text) // 4 (matches J17 harness)."""
    return max(len(text) // 4, 1)


# ============================================================
# LLM CALLS
# ============================================================

def _llm_chat(api_key: str, model: str, messages: List[dict], max_tokens: int = 512) -> str:
    """Send a chat completion request to the OpenAI API."""
    resp = _requests.post(
        "https://api.openai.com/v1/chat/completions",
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
        },
        json={
            "model": model,
            "messages": messages,
            "max_tokens": max_tokens,
            "temperature": 0.1,
        },
        timeout=60,
    )
    resp.raise_for_status()
    return resp.json()["choices"][0]["message"]["content"].strip()


def judge_answer(api_key: str, model: str, ground_truth: str, agent_answer: str) -> Tuple[float, str]:
    """Judge an agent answer against ground truth using the standard 0-10 rubric.

    Rubric:
        10: Perfect — all facts correct, nothing missing
        7-9: Good — main facts correct, minor details missed
        4-6: Partial — some correct but missing critical facts
        1-3: Poor — fundamentally wrong
        0: No understanding

    Returns (score, reason).
    """
    prompt = f"""You are judging whether an AI agent correctly answered a comprehension question.

GROUND TRUTH ANSWER:
{ground_truth}

AGENT'S ANSWER:
{agent_answer}

Score from 0 to 10:
- 10: Perfect — all facts correct, nothing missing
- 7-9: Good — main facts correct, minor details missed
- 4-6: Partial — some correct but missing critical facts
- 1-3: Poor — fundamentally wrong
- 0: No understanding

Respond in exactly this format (nothing else):
SCORE: <number>
REASON: <one sentence>"""

    raw = _llm_chat(api_key, model, [{"role": "user", "content": prompt}], max_tokens=100)

    score = 0.0
    reason = ""
    for line in raw.split("\n"):
        line = line.strip()
        if line.startswith("SCORE:"):
            try:
                score = float(line.split(":", 1)[1].strip())
            except ValueError:
                pass
        elif line.startswith("REASON:"):
            reason = line.split(":", 1)[1].strip()

    return score, reason


# ============================================================
# SPEC LOADER
# ============================================================

class UITSLoader:
    """Parse and validate UITS spec files."""

    REQUIRED_TOP = {"uits_version", "metadata", "content", "ground_truth", "scoring_profiles", "optimization"}

    @staticmethod
    def load(spec_path: Path) -> dict:
        """Load and validate a UITS spec from disk."""
        with open(spec_path) as f:
            spec = json.load(f)

        # Required top-level fields
        for req in UITSLoader.REQUIRED_TOP:
            if req not in spec:
                raise ValueError(f"Missing required field '{req}' in {spec_path}")

        if spec["uits_version"] != "1.0.0":
            raise ValueError(f"Unsupported UITS version: {spec['uits_version']}")

        # content
        content = spec["content"]
        for req in ("source_file", "encoding_format"):
            if req not in content:
                raise ValueError(f"Missing content.{req} in {spec_path}")

        # ground_truth
        questions = spec["ground_truth"].get("questions", [])
        if len(questions) < 3:
            raise ValueError(f"ground_truth.questions requires >= 3 items in {spec_path}")

        for q in questions:
            for req in ("id", "question", "answer", "difficulty"):
                if req not in q:
                    raise ValueError(f"Question missing required field '{req}' in {spec_path}")

        # scoring_profiles
        sp = spec["scoring_profiles"]
        if "active_profile" not in sp or "profiles" not in sp:
            raise ValueError(f"scoring_profiles missing active_profile or profiles in {spec_path}")

        # active profile must exist
        active_id = sp["active_profile"]
        profiles = {p["profile_id"]: p for p in sp["profiles"] if isinstance(p, dict)}
        if active_id not in profiles:
            raise ValueError(f"active_profile '{active_id}' not found in scoring_profiles.profiles")

        # Validate weights sum to 1.0 for active profile
        active = profiles[active_id]
        weights = active.get("weights", {})
        weight_sum = sum(weights.get(k, 0.0) for k in ("comprehension", "compaction", "token_efficiency", "fidelity"))
        if abs(weight_sum - 1.0) > 0.001:
            raise ValueError(
                f"Active profile '{active_id}' weights sum to {weight_sum:.4f}, expected 1.0"
            )

        return spec

    @staticmethod
    def get_active_profile(spec: dict) -> dict:
        """Return the active scoring profile dict."""
        sp = spec["scoring_profiles"]
        active_id = sp["active_profile"]
        for p in sp["profiles"]:
            if isinstance(p, dict) and p["profile_id"] == active_id:
                return p
        raise ValueError(f"Active profile '{active_id}' not found")

    @staticmethod
    def read_source_file(spec: dict) -> str:
        """Read the T1-encoded source file. Path is relative to repo root."""
        path = (_REPO_ROOT / spec["content"]["source_file"]).resolve()
        if not path.exists():
            raise FileNotFoundError(f"source_file not found: {path}")
        return path.read_text(encoding="utf-8")

    @staticmethod
    def read_human_readable_file(spec: dict) -> Optional[str]:
        """Read the human-readable file if specified. Returns None if absent."""
        hr_path = spec["content"].get("human_readable_file")
        if not hr_path:
            return None
        path = (_REPO_ROOT / hr_path).resolve()
        if not path.exists():
            return None
        return path.read_text(encoding="utf-8")


# ============================================================
# HASH UTILITIES
# ============================================================

def _verify_spec_hash(spec: dict) -> Tuple[Optional[bool], List[str]]:
    """Check config.sha256 against sha256_spec_without_field.

    Returns (is_valid, hash_mismatches).
    is_valid is None when no hash field is present.
    """
    expected = spec.get("config", {}).get("sha256")
    if not expected:
        return None, []

    actual = sha256_spec_without_field(spec, ("config", "sha256"))
    if actual == expected:
        return True, []
    return False, [f"hash mismatch: expected {expected[:12]}..., got {actual[:12]}..."]


# ============================================================
# TRIAL RUNNER
# ============================================================

def run_trials(spec: dict, api_key: str, model: str) -> List[TrialResult]:
    """For each question in the spec, ask the LLM to answer using only the T1 source content.

    Returns list of TrialResult, one per question.
    """
    t1_content = UITSLoader.read_source_file(spec)
    questions = spec["ground_truth"]["questions"]
    injection_ctx = spec["content"].get("injection_context", "")

    context_prefix = ""
    if injection_ctx:
        context_prefix = f"You are an AI coding agent working in {injection_ctx}.\n\n"

    trials: List[TrialResult] = []
    for q in questions:
        prompt = (
            f"{context_prefix}"
            f"You have been given this reference content:\n\n"
            f"{t1_content}\n\n"
            f"Answer this question using ONLY the information in the content above:\n"
            f"{q['question']}\n\n"
            f"Be precise and specific. If the content does not contain the answer, say so."
        )

        agent_answer = ""
        error_msg = ""
        try:
            agent_answer = _llm_chat(
                api_key, model,
                [{"role": "user", "content": prompt}],
                max_tokens=300,
            )
        except Exception as e:
            error_msg = str(e)

        score = 0.0
        reason = ""
        if agent_answer:
            try:
                score, reason = judge_answer(api_key, model, q["answer"], agent_answer)
            except Exception as e:
                error_msg = f"judge error: {e}"

        trials.append(TrialResult(
            question_id=q["id"],
            question=q["question"],
            ground_truth=q["answer"],
            agent_answer=agent_answer,
            judge_score=score,
            judge_reason=reason,
            difficulty=q["difficulty"],
            section_tag=q.get("section_tag", ""),
            error=error_msg,
        ))

    return trials


# ============================================================
# WEAK SECTION DETECTION
# ============================================================

def find_weak_sections(trials: List[TrialResult], threshold: float) -> List[WeakSection]:
    """Group trials by section_tag (if present) or question_id.

    Returns sections (groups) where avg score is below threshold.
    """
    groups: Dict[str, List[TrialResult]] = {}
    for t in trials:
        key = t.section_tag if t.section_tag else t.question_id
        groups.setdefault(key, []).append(t)

    weak: List[WeakSection] = []
    for key, group_trials in groups.items():
        avg = sum(t.judge_score for t in group_trials) / len(group_trials)
        if avg < threshold:
            weak.append(WeakSection(
                key=key,
                avg_score=round(avg, 2),
                questions=[
                    {
                        "question_id": t.question_id,
                        "question": t.question,
                        "answer": t.ground_truth,
                        "score": t.judge_score,
                        "reason": t.judge_reason,
                    }
                    for t in group_trials
                ],
            ))
    return weak


# ============================================================
# COMPOSITE SCORE COMPUTATION
# ============================================================

def compute_composite(trials: List[TrialResult], spec: dict, profile: dict) -> CompositeScore:
    """Compute the weighted composite score using the active profile's weights.

    Dimensions:
        comprehension: mean judge score / 10, weighted by difficulty_weights if present
        compaction:    1 - (t1_bytes / human_bytes); 0.0 if no human_readable_file
        token_efficiency: 1 - (tokens / token_max), clamped to [0, 1]
        fidelity:      1.0 (placeholder — round-trip not yet implemented)
    """
    weights = profile.get("weights", {})
    thresholds_block = profile.get("thresholds", {})
    difficulty_weights = profile.get("difficulty_weights", {})

    # -- comprehension --
    if difficulty_weights:
        weighted_sum = 0.0
        weight_total = 0.0
        for t in trials:
            dw = difficulty_weights.get(t.difficulty, 1.0)
            weighted_sum += t.judge_score * dw
            weight_total += dw
        raw_comprehension = (weighted_sum / weight_total) if weight_total > 0 else 0.0
    else:
        raw_comprehension = (
            sum(t.judge_score for t in trials) / len(trials) if trials else 0.0
        )
    comprehension = raw_comprehension / 10.0  # normalize to [0, 1]

    # -- compaction --
    t1_content = UITSLoader.read_source_file(spec)
    human_content = UITSLoader.read_human_readable_file(spec)
    t1_bytes = len(t1_content.encode("utf-8"))
    if human_content:
        human_bytes = len(human_content.encode("utf-8"))
        compaction = 1.0 - (t1_bytes / max(human_bytes, 1))
    else:
        compaction = 0.0

    # -- token_efficiency --
    token_max = thresholds_block.get("token_max", 0)
    tokens = _estimate_tokens(t1_content)
    if token_max and token_max > 0:
        token_efficiency = max(0.0, min(1.0, 1.0 - (tokens / token_max)))
    else:
        token_efficiency = 1.0  # no limit defined — full credit

    # -- fidelity (placeholder) --
    fidelity = 1.0

    # -- weighted composite --
    w_comp = weights.get("comprehension", 0.0)
    w_compaction = weights.get("compaction", 0.0)
    w_token = weights.get("token_efficiency", 0.0)
    w_fidelity = weights.get("fidelity", 0.0)

    composite = (
        w_comp * comprehension
        + w_compaction * compaction
        + w_token * token_efficiency
        + w_fidelity * fidelity
    )

    return CompositeScore(
        comprehension=round(comprehension, 4),
        compaction=round(compaction, 4),
        token_efficiency=round(token_efficiency, 4),
        fidelity=round(fidelity, 4),
        composite=round(composite, 4),
    )


# ============================================================
# CONVERGENCE CHECK
# ============================================================

def check_convergence(
    composite_scores: List[CompositeScore],
    profile: dict,
    window: int,
) -> bool:
    """Return True when all thresholds pass for `window` consecutive runs.

    Checks: composite_min, comprehension_min, comprehension_floor.
    """
    if len(composite_scores) < window:
        return False

    thresholds = profile.get("thresholds", {})
    composite_min = thresholds.get("composite_min", 0.0)
    comprehension_min = thresholds.get("comprehension_min", 0.0)  # 0-10 scale
    comprehension_floor = thresholds.get("comprehension_floor", 0.0)  # 0-10 scale

    recent = composite_scores[-window:]
    for cs in recent:
        raw_comprehension_score = cs.comprehension * 10.0  # back to 0-10 scale
        if cs.composite < composite_min:
            return False
        if raw_comprehension_score < comprehension_min:
            return False
        if comprehension_floor > 0 and raw_comprehension_score < comprehension_floor:
            return False

    return True


# ============================================================
# THRESHOLD VALIDATION
# ============================================================

def _check_thresholds(composite: CompositeScore, profile: dict) -> List[str]:
    """Return list of failure strings for threshold violations."""
    thresholds = profile.get("thresholds", {})
    failures: List[str] = []

    composite_min = thresholds.get("composite_min", 0.0)
    if composite.composite < composite_min:
        failures.append(
            f"I1_COMPOSITE: {composite.composite:.4f} < {composite_min:.4f} threshold"
        )

    comprehension_min = thresholds.get("comprehension_min", 0.0)
    raw_score = composite.comprehension * 10.0
    if raw_score < comprehension_min:
        failures.append(
            f"I2_COMPREHENSION: {raw_score:.2f}/10 < {comprehension_min:.2f} threshold"
        )

    comprehension_floor = thresholds.get("comprehension_floor", 0.0)
    if comprehension_floor > 0 and raw_score < comprehension_floor:
        failures.append(
            f"I3_COMPREHENSION_FLOOR: {raw_score:.2f}/10 < {comprehension_floor:.2f} hard floor"
        )

    compaction_min_pct = thresholds.get("compaction_min_pct")
    if compaction_min_pct is not None:
        compaction_pct = composite.compaction * 100.0
        if compaction_pct < compaction_min_pct:
            failures.append(
                f"I4_COMPACTION: {compaction_pct:.1f}% < {compaction_min_pct:.1f}% threshold"
            )

    return failures


# ============================================================
# LLM REGENERATION
# ============================================================

def _regenerate_content(
    api_key: str,
    model: str,
    spec: dict,
    weak_sections: List[WeakSection],
    token_budget: int,
) -> str:
    """Ask the LLM to regenerate the T1 content to improve weak sections."""
    t1_content = UITSLoader.read_source_file(spec)
    human_content = UITSLoader.read_human_readable_file(spec) or ""
    encoding_format = spec["content"].get("encoding_format", "t1_coded")

    weak_qs_lines: List[str] = []
    for ws in weak_sections:
        for wq in ws.questions:
            weak_qs_lines.append(
                f"  - Q: {wq['question']}\n"
                f"    ANSWER: {wq['answer']}\n"
                f"    SCORE: {wq['score']}/10 — {wq['reason']}"
            )
    weak_qs_str = "\n".join(weak_qs_lines) if weak_qs_lines else "  (overall below threshold)"

    prompt = f"""You are improving a {encoding_format} encoded reference file for AI agent consumption.

CRITICAL RULES:
- You MUST preserve ALL existing data values exactly (numbers, names, codes, states)
- You MUST NOT change any factual content — only restructure for clarity
- Every piece of information in the current encoding MUST be preserved
- The weak questions below show what information is hard to extract — make it more explicit

Weak questions (below threshold):
{weak_qs_str}

Current encoding (PRESERVE ALL DATA):
{t1_content}

Human-readable source (use for reference only):
{human_content[:3000] if human_content else "(not available)"}

Produce an improved encoding that:
1. Preserves ALL factual data from the current encoding exactly
2. Makes information needed for the weak questions more explicit or prominent
3. Uses the same encoding format ({encoding_format}): pipe-delimited fields, short codes, no prose
4. Stays under {token_budget} tokens (roughly {token_budget * 4} characters)
5. Keeps the same structural conventions (type header on first line if present)

Output ONLY the new encoding, nothing else."""

    return _llm_chat(api_key, model, [{"role": "user", "content": prompt}], max_tokens=1000)


# ============================================================
# REPORTER (UITS-specific display)
# ============================================================

class Reporter:
    RESET = "\033[0m"
    GREEN = "\033[32m"
    RED = "\033[31m"
    YELLOW = "\033[33m"
    BOLD = "\033[1m"

    @classmethod
    def print_trial(cls, trial: TrialResult, idx: int, question_threshold: float) -> None:
        status = "PASS" if trial.judge_score >= question_threshold else "WEAK"
        color = cls.GREEN if status == "PASS" else cls.YELLOW
        err_suffix = f" [ERR: {trial.error[:40]}]" if trial.error else ""
        print(
            f"    Q{idx + 1} [{trial.difficulty}] "
            f"{color}{trial.judge_score:.0f}/10 {status}{cls.RESET}"
            f" — {trial.judge_reason[:60]}{err_suffix}"
        )

    @classmethod
    def print_composite(cls, cs: CompositeScore) -> None:
        print(
            f"    >> composite={cs.composite:.4f} | "
            f"comprehension={cs.comprehension * 10:.2f}/10 | "
            f"compaction={cs.compaction:.1%} | "
            f"token_eff={cs.token_efficiency:.1%} | "
            f"fidelity={cs.fidelity:.1%}"
        )

    @classmethod
    def print_spec_result(cls, result: SpecResult) -> None:
        icon = (
            f"{cls.GREEN}PASS{cls.RESET}" if result.status == "pass"
            else f"{cls.RED}FAIL{cls.RESET}"
        )
        print(f"\n  [{icon}] {Path(result.spec_path).name}")
        if result.composite:
            cls.print_composite(result.composite)
        if result.failures:
            for fail in result.failures:
                print(f"    {cls.RED}{fail}{cls.RESET}")
        if result.warnings:
            for warn in result.warnings:
                print(f"    {cls.YELLOW}WARN: {warn}{cls.RESET}")

    @classmethod
    def print_summary_table(cls, results: List[SpecResult]) -> None:
        header = f"{'Spec':<35} | {'Comp':>6} | {'Compaction':>10} | {'Composite':>9} | {'Status':>6}"
        print(f"\n{cls.BOLD}{header}{cls.RESET}")
        print("-" * 80)
        for r in results:
            status = f"{cls.GREEN}PASS{cls.RESET}" if r.status == "pass" else f"{cls.RED}FAIL{cls.RESET}"
            if r.composite:
                print(
                    f"{Path(r.spec_path).name:<35} | "
                    f"{r.composite.comprehension * 10:>5.2f} | "
                    f"{r.composite.compaction:>9.1%} | "
                    f"{r.composite.composite:>8.4f} | "
                    f"{status}"
                )
            else:
                print(f"{Path(r.spec_path).name:<35} | {'N/A':>6} | {'N/A':>10} | {'N/A':>9} | {status}")

    @classmethod
    def print_overall(cls, results: List[SpecResult]) -> None:
        passed = sum(1 for r in results if r.status == "pass")
        total = len(results)
        rate = (passed / total * 100) if total > 0 else 0
        color = cls.GREEN if passed == total else cls.YELLOW if passed > 0 else cls.RED
        print(f"\n{cls.BOLD}Overall: {color}{passed}/{total} specs passed ({rate:.0f}%){cls.RESET}")


# ============================================================
# CORE VALIDATE LOGIC (shared between validate and validate-all)
# ============================================================

def _run_validate(
    spec_path: Path,
    api_key: str,
    model: str,
) -> Tuple[SpecResult, List[str], List[dict]]:
    """Load, run, and score a single spec. Returns (SpecResult, parity_errors, canonical_failures)."""
    spec = UITSLoader.load(spec_path)
    parity_errors = _validate_supported_features(spec)

    # Hash verification
    hash_verified, hash_mismatches = _verify_spec_hash(spec)
    warnings: List[str] = []
    if hash_verified is False:
        warnings.append(f"spec hash mismatch: {hash_mismatches[0]}")

    profile = UITSLoader.get_active_profile(spec)
    questions = spec["ground_truth"]["questions"]
    question_threshold = profile.get("thresholds", {}).get("comprehension_min", 7.0)
    spec_name = spec.get("metadata", {}).get("name", spec_path.stem)

    print(f"  Testing '{spec_name}' ({len(questions)} questions) with model={model}...")
    start = time.monotonic()
    trials = run_trials(spec, api_key, model)
    duration = (time.monotonic() - start) * 1000

    for i, trial in enumerate(trials):
        Reporter.print_trial(trial, i, question_threshold)

    composite = compute_composite(trials, spec, profile)
    Reporter.print_composite(composite)

    threshold_failures = _check_thresholds(composite, profile)
    all_failures = threshold_failures + parity_errors

    status = canonical_result_status("pass" if not all_failures else "fail")

    # assertions: one per threshold dimension checked
    thresholds_block = profile.get("thresholds", {})
    assertions_evaluated = len([k for k in ("composite_min", "comprehension_min") if k in thresholds_block])
    if "comprehension_floor" in thresholds_block:
        assertions_evaluated += 1
    if "compaction_min_pct" in thresholds_block:
        assertions_evaluated += 1
    assertions_passed = assertions_evaluated - len(threshold_failures)

    result = SpecResult(
        spec_path=str(spec_path),
        status=status,
        trials=trials,
        composite=composite,
        hash_verified=hash_verified,
        duration_ms=round(duration, 1),
        failures=all_failures,
        warnings=warnings,
    )

    canonical_result = build_result(
        spec_path=str(spec_path),
        status=status,
        duration_ms=round(duration, 1),
        hash_verified=hash_verified,
        hash_mismatches=hash_mismatches,
        assertions_evaluated=assertions_evaluated,
        assertions_passed=assertions_passed,
        failures=all_failures if all_failures else [],
        warnings=warnings if warnings else [],
    )

    return result, parity_errors, canonical_result


# ============================================================
# COMMANDS
# ============================================================

def cmd_validate(args: argparse.Namespace) -> int:
    """Validate a single UITS spec (one-shot scoring, no optimization)."""
    spec_path = Path(args.spec).resolve()
    api_key = _load_api_key()
    model = args.model

    start_time = datetime.now(timezone.utc)
    total_start = time.monotonic()

    try:
        result, _parity, canonical_result = _run_validate(spec_path, api_key, model)
    except Exception as e:
        canonical_result = build_result(
            spec_path=str(spec_path),
            status="error",
            error=str(e),
        )
        print(f"ERROR loading {spec_path.name}: {e}", file=sys.stderr)
        total_duration = (time.monotonic() - total_start) * 1000
        report = build_report("uits", UITS_VERSION, [canonical_result], start_time, total_duration)
        print_summary(report)
        if args.report:
            save_report(report, args.report)
        return 1

    Reporter.print_spec_result(result)

    total_duration = (time.monotonic() - total_start) * 1000
    report = build_report("uits", UITS_VERSION, [canonical_result], start_time, round(total_duration, 1))
    print_summary(report)

    if args.report:
        save_report(report, args.report)

    return 0 if result.status == "pass" else 1


def cmd_validate_all(args: argparse.Namespace) -> int:
    """Validate all UITS specs in a directory."""
    spec_dir = Path(args.spec_dir).resolve()
    spec_files = sorted(spec_dir.glob("*.uits.json"))

    if not spec_files:
        print(f"No UITS specs found in {spec_dir}", file=sys.stderr)
        return 1

    print(f"Found {len(spec_files)} UITS spec(s)\n")

    api_key = _load_api_key()
    model = args.model
    start_time = datetime.now(timezone.utc)
    total_start = time.monotonic()

    all_results: List[SpecResult] = []
    canonical_results: List[dict] = []

    for spec_path in spec_files:
        try:
            result, _parity, canonical_result = _run_validate(spec_path, api_key, model)
            all_results.append(result)
            canonical_results.append(canonical_result)
        except Exception as e:
            print(f"  SKIP {spec_path.name}: {e}", file=sys.stderr)
            canonical_results.append(build_result(
                spec_path=str(spec_path),
                status="error",
                error=str(e),
            ))

    Reporter.print_summary_table(all_results)
    Reporter.print_overall(all_results)

    total_duration = (time.monotonic() - total_start) * 1000
    report = build_report("uits", UITS_VERSION, canonical_results, start_time, round(total_duration, 1))
    print_summary(report)

    if args.report:
        save_report(report, args.report)

    failed = sum(1 for r in all_results if r.status == "fail")
    return 1 if failed > 0 else 0


def cmd_optimize(args: argparse.Namespace) -> int:
    """Run the iterative optimization loop for a single UITS spec.

    Iterates until convergence or max_iterations:
    1. Run trials, compute composite
    2. Check convergence across convergence_window consecutive runs
    3. If not converged, find weak sections, regenerate content via LLM
    4. Append history entry to spec file on completion
    """
    spec_path = Path(args.spec).resolve()
    api_key = _load_api_key()
    model = args.model

    # Load spec
    try:
        spec = UITSLoader.load(spec_path)
    except Exception as e:
        print(f"ERROR loading spec: {e}", file=sys.stderr)
        return 1

    profile = UITSLoader.get_active_profile(spec)
    opt_cfg = spec.get("optimization", {})
    max_iterations = args.max_iterations or opt_cfg.get("max_iterations", 5)
    convergence_window = opt_cfg.get("convergence_window", 2)
    regen_model = args.regen_model or opt_cfg.get("model", model)
    question_threshold = profile.get("thresholds", {}).get("comprehension_min", 7.0)
    thresholds_block = profile.get("thresholds", {})
    token_max = thresholds_block.get("token_max", 250)

    spec_name = spec.get("metadata", {}).get("name", spec_path.stem)
    print(f"\n{'=' * 60}")
    print(f"  UITS Optimize: {spec_name}")
    print(f"  Max iterations: {max_iterations} | Window: {convergence_window}")
    print(f"  Model: {model} | Regen model: {regen_model}")
    print(f"{'=' * 60}\n")

    start_time = datetime.now(timezone.utc)
    total_start = time.monotonic()
    composite_history: List[CompositeScore] = []
    per_question_history: List[List[TrialResult]] = []
    weak_sections_history: List[List[str]] = []
    converged = False

    for iteration in range(1, max_iterations + 1):
        print(f"{'=' * 50}")
        print(f"  ITERATION {iteration} / {max_iterations}")
        print(f"{'=' * 50}")

        # Re-read spec each iteration (source_file may have been updated)
        try:
            spec = UITSLoader.load(spec_path)
        except Exception as e:
            print(f"ERROR reloading spec at iteration {iteration}: {e}", file=sys.stderr)
            break

        trials = run_trials(spec, api_key, model)
        for i, trial in enumerate(trials):
            Reporter.print_trial(trial, i, question_threshold)

        composite = compute_composite(trials, spec, profile)
        Reporter.print_composite(composite)
        composite_history.append(composite)
        per_question_history.append(trials)

        # Convergence check
        converged = check_convergence(composite_history, profile, convergence_window)
        if converged:
            print(f"\n  Converged after {iteration} iteration(s) (window={convergence_window}).")
            weak_sections_history.append([])
            break

        if iteration == max_iterations:
            print(f"\n  Max iterations ({max_iterations}) reached without convergence.")
            weak_sections_history.append([])
            break

        # Find weak sections and regenerate
        weak = find_weak_sections(trials, question_threshold)
        weak_keys = [ws.key for ws in weak]
        weak_sections_history.append(weak_keys)

        if not weak:
            # Thresholds may still be failing even if no per-question weakness
            threshold_failures = _check_thresholds(composite, profile)
            if not threshold_failures:
                print("\n  All questions pass threshold — checking composite...")
            print(f"  No weak sections, but composite {composite.composite:.4f} still below target. Stopping.")
            break

        print(f"\n  {len(weak)} weak section(s) found: {weak_keys}")
        print(f"  Regenerating content via {regen_model}...")

        t1_content = UITSLoader.read_source_file(spec)
        token_estimate_before = _estimate_tokens(t1_content)

        new_content = _regenerate_content(api_key, regen_model, spec, weak, token_max)

        token_estimate_after = _estimate_tokens(new_content)
        print(f"  Content updated: {token_estimate_before} -> {token_estimate_after} tokens")

        # Write new content to source_file
        source_path = (_REPO_ROOT / spec["content"]["source_file"]).resolve()
        source_path.write_text(new_content, encoding="utf-8")
        print(f"  Wrote: {source_path}")

    total_duration = (time.monotonic() - total_start) * 1000

    # Build canonical report from last iteration
    final_trials = per_question_history[-1] if per_question_history else []
    final_composite = composite_history[-1] if composite_history else None
    all_failures = _check_thresholds(final_composite, profile) if final_composite else ["no trials completed"]
    status = canonical_result_status("pass" if not all_failures else "fail")

    thresholds_checked = len([k for k in ("composite_min", "comprehension_min") if k in thresholds_block])
    if "comprehension_floor" in thresholds_block:
        thresholds_checked += 1
    if "compaction_min_pct" in thresholds_block:
        thresholds_checked += 1

    canonical_results = [build_result(
        spec_path=str(spec_path),
        status=status,
        duration_ms=round(total_duration, 1),
        hash_verified=None,
        hash_mismatches=[],
        assertions_evaluated=thresholds_checked,
        assertions_passed=thresholds_checked - len(all_failures),
        failures=all_failures if all_failures else [],
        warnings=[],
    )]
    report = build_report("uits", UITS_VERSION, canonical_results, start_time, round(total_duration, 1))
    print_summary(report)

    if args.report:
        save_report(report, args.report)

    # Append history entry to spec file
    run_id = datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%S") + "-" + str(uuid.uuid4())[:8]
    history_entry: dict = {
        "run_id": run_id,
        "profile_id": profile["profile_id"],
        "profile_version": profile.get("version", "1.0.0"),
        "timestamp": datetime.now(timezone.utc).isoformat().replace("+00:00", "Z"),
        "iterations": len(composite_history),
        "converged": converged,
        "final_scores": {},
        "per_question_scores": [],
        "weak_sections": weak_sections_history[-1] if weak_sections_history else [],
    }
    if final_composite:
        history_entry["final_scores"] = {
            "composite": final_composite.composite,
            "comprehension": round(final_composite.comprehension * 10, 2),
            "compaction_pct": round(final_composite.compaction * 100, 1),
            "token_count": _estimate_tokens(UITSLoader.read_source_file(spec)),
            "fidelity_pct": round(final_composite.fidelity * 100, 1),
        }
    for t in final_trials:
        history_entry["per_question_scores"].append({
            "question_id": t.question_id,
            "score": t.judge_score,
            "difficulty": t.difficulty,
        })

    # Reload spec fresh for writing (avoid stale in-memory copy)
    with open(spec_path) as f:
        spec_raw = json.load(f)
    if "history" not in spec_raw:
        spec_raw["history"] = []
    spec_raw["history"].append(history_entry)

    # Update last_optimized if converged
    if converged and "metadata" in spec_raw:
        spec_raw["metadata"]["last_optimized"] = datetime.now(timezone.utc).strftime("%Y-%m-%d")

    with open(spec_path, "w") as f:
        json.dump(spec_raw, f, indent=2)
        f.write("\n")
    print(f"\n  History entry appended to {spec_path.name}")

    return 0 if status == "pass" else 1


def cmd_add_hashes(args: argparse.Namespace) -> int:
    """Add or update config.sha256 hashes in UITS specs.

    The hash covers the full spec content with config.sha256 excluded,
    computed via sha256_spec_without_field.
    """
    spec_dir = Path(args.spec_dir).resolve()
    spec_files = sorted(spec_dir.glob("*.uits.json"))

    if not spec_files:
        print(f"No UITS specs found in {spec_dir}")
        return 0

    for spec_path in spec_files:
        with open(spec_path) as f:
            spec = json.load(f)

        new_hash = sha256_spec_without_field(spec, ("config", "sha256"))
        existing = spec.get("config", {}).get("sha256", "")

        if existing == new_hash:
            print(f"  OK   {spec_path.name}: hash unchanged")
            continue

        if "config" not in spec:
            spec["config"] = {}
        spec["config"]["sha256"] = new_hash

        with open(spec_path, "w") as f:
            json.dump(spec, f, indent=2)
            f.write("\n")
        print(f"  SET  {spec_path.name}: {new_hash[:16]}...")

    return 0


def cmd_verify_hashes(args: argparse.Namespace) -> int:
    """Verify config.sha256 hashes in UITS specs."""
    spec_dir = Path(args.spec_dir).resolve()
    spec_files = sorted(spec_dir.glob("*.uits.json"))

    if not spec_files:
        print(f"No UITS specs found in {spec_dir}")
        return 0

    failures = 0
    for spec_path in spec_files:
        with open(spec_path) as f:
            spec = json.load(f)

        expected = spec.get("config", {}).get("sha256")
        if not expected:
            print(f"  SKIP {spec_path.name}: no hash")
            continue

        actual = sha256_spec_without_field(spec, ("config", "sha256"))
        if actual == expected:
            print(f"  OK   {spec_path.name}: {expected[:16]}...")
        else:
            print(f"  FAIL {spec_path.name}: expected {expected[:12]}..., got {actual[:12]}...")
            failures += 1

    return 1 if failures > 0 else 0


def cmd_profiles(args: argparse.Namespace) -> int:
    """List scoring profiles defined in a UITS spec."""
    spec_path = Path(args.spec).resolve()
    try:
        spec = UITSLoader.load(spec_path)
    except Exception as e:
        print(f"ERROR: {e}", file=sys.stderr)
        return 1

    sp = spec["scoring_profiles"]
    active_id = sp["active_profile"]
    profiles = sp.get("profiles", [])

    print(f"\nScoring profiles in {spec_path.name}:")
    print(f"  Active: {active_id}\n")

    for p in profiles:
        if not isinstance(p, dict):
            continue
        pid = p.get("profile_id", "?")
        version = p.get("version", "?")
        active_marker = " [ACTIVE]" if pid == active_id else ""
        print(f"  {pid} v{version}{active_marker}")

        weights = p.get("weights", {})
        print(f"    Weights: comprehension={weights.get('comprehension', 0):.2f} "
              f"compaction={weights.get('compaction', 0):.2f} "
              f"token_efficiency={weights.get('token_efficiency', 0):.2f} "
              f"fidelity={weights.get('fidelity', 0):.2f}")

        thresholds = p.get("thresholds", {})
        print(f"    Thresholds: composite_min={thresholds.get('composite_min', 'N/A')} "
              f"comprehension_min={thresholds.get('comprehension_min', 'N/A')}/10")

        if p.get("rationale"):
            print(f"    Rationale: {p['rationale']}")
        print()

    return 0


# ============================================================
# CLI
# ============================================================

def main() -> int:
    parser = argparse.ArgumentParser(
        description="Universal Iterative-Improvement Test Specification (UITS) Runner"
    )
    sub = parser.add_subparsers(dest="command")

    # validate
    p_val = sub.add_parser("validate", help="Validate a single UITS spec (one-shot scoring)")
    p_val.add_argument("--spec", required=True, help="Path to .uits.json spec file")
    p_val.add_argument("--report", help="Output JSON report path")
    p_val.add_argument("--model", default="gpt-4.1-mini", help="OpenAI model for agent + judge")

    # validate-all
    p_all = sub.add_parser("validate-all", help="Validate all UITS specs in a directory")
    p_all.add_argument("--spec-dir", required=True, help="Directory containing .uits.json specs")
    p_all.add_argument("--report", help="Output JSON report path")
    p_all.add_argument("--model", default="gpt-4.1-mini", help="OpenAI model for agent + judge")

    # optimize
    p_opt = sub.add_parser("optimize", help="Run iterative optimization loop for a single spec")
    p_opt.add_argument("--spec", required=True, help="Path to .uits.json spec file")
    p_opt.add_argument("--report", help="Output JSON report path")
    p_opt.add_argument("--model", default="gpt-4.1-mini", help="OpenAI model for agent + judge")
    p_opt.add_argument("--regen-model", default="", help="Model for content regeneration (default: same as --model)")
    p_opt.add_argument("--max-iterations", type=int, default=0, help="Override max_iterations from spec (0 = use spec value)")

    # add-hashes
    p_hash = sub.add_parser("add-hashes", help="Add config.sha256 hashes to UITS specs")
    p_hash.add_argument("--spec-dir", required=True, help="Directory containing .uits.json specs")

    # verify-hashes
    p_vhash = sub.add_parser("verify-hashes", help="Verify config.sha256 hashes in UITS specs")
    p_vhash.add_argument("--spec-dir", required=True, help="Directory containing .uits.json specs")

    # profiles
    p_prof = sub.add_parser("profiles", help="List scoring profiles defined in a spec")
    p_prof.add_argument("--spec", required=True, help="Path to .uits.json spec file")

    args = parser.parse_args()
    if not args.command:
        parser.print_help()
        return 1

    commands = {
        "validate": cmd_validate,
        "validate-all": cmd_validate_all,
        "optimize": cmd_optimize,
        "add-hashes": cmd_add_hashes,
        "verify-hashes": cmd_verify_hashes,
        "profiles": cmd_profiles,
    }
    return commands[args.command](args)


if __name__ == "__main__":
    sys.exit(main())
