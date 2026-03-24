#!/usr/bin/env python3
"""
Architecture Map Iterative Optimization Test

Adapts the J17 comprehension test harness (cmd/j17-comprehension-test/main.go)
for iterative optimization of T1-encoded architecture maps.

Usage:
    python scripts/map_optimization_test.py                          # Run all maps
    python scripts/map_optimization_test.py --map uxts_frameworks    # Single map
    python scripts/map_optimization_test.py --baseline-only          # Score without iterating
    python scripts/map_optimization_test.py --iterations 3           # Limit iterations

Requires: OPENAI_API_KEY in environment or .env file.
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import time
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

# ---------------------------------------------------------------------------
# LLM Client (minimal OpenAI-compatible)
# ---------------------------------------------------------------------------

try:
    import requests
except ImportError:
    print("ERROR: 'requests' library required. Install with: pip install requests")
    sys.exit(1)


def load_api_key() -> str:
    key = os.environ.get("OPENAI_API_KEY", "")
    if not key:
        env_path = Path(__file__).resolve().parents[1] / ".env"
        if env_path.exists():
            for line in env_path.read_text().splitlines():
                if line.startswith("OPENAI_API_KEY="):
                    key = line.split("=", 1)[1].strip()
    if not key:
        print("ERROR: OPENAI_API_KEY not set")
        sys.exit(1)
    return key


def llm_complete(api_key: str, model: str, prompt: str, max_tokens: int = 500) -> str:
    """Call OpenAI-compatible chat completion."""
    resp = requests.post(
        "https://api.openai.com/v1/chat/completions",
        headers={"Authorization": f"Bearer {api_key}", "Content-Type": "application/json"},
        json={
            "model": model,
            "messages": [{"role": "user", "content": prompt}],
            "max_tokens": max_tokens,
            "temperature": 0.1,
        },
        timeout=60,
    )
    resp.raise_for_status()
    return resp.json()["choices"][0]["message"]["content"].strip()


# ---------------------------------------------------------------------------
# Data Structures
# ---------------------------------------------------------------------------

@dataclass
class GroundTruthQA:
    question: str
    answer: str
    difficulty: str  # "factual", "relational", "inferential"


@dataclass
class TestMap:
    id: str
    map_type: str  # DICT, DEP, FLOW, SCHEMA, SVC, DIST, UXTS
    file_path: str
    human_readable: str
    t1_content: str
    questions: List[GroundTruthQA]
    injection_ctx: str


@dataclass
class TrialResult:
    map_id: str
    question_idx: int
    question: str
    ground_truth: str
    agent_answer: str
    judge_score: float
    judge_reason: str
    tokens_t1: int
    tokens_human: int
    iteration: int
    difficulty: str


@dataclass
class MapChange:
    map_id: str
    old_t1: str
    new_t1: str
    score_before: float
    weak_questions: List[str]


@dataclass
class IterationLog:
    iteration: int
    timestamp: str
    trials: List[TrialResult]
    avg_score: float
    map_changes: List[MapChange]


# ---------------------------------------------------------------------------
# Token Estimation (matches J17 harness: len/4)
# ---------------------------------------------------------------------------

def estimate_tokens(s: str) -> int:
    return max(len(s) // 4, 1)


# ---------------------------------------------------------------------------
# Four Metrics (from spec)
# ---------------------------------------------------------------------------

def compute_metrics(t1: str, human: str, trials: List[TrialResult]) -> Dict[str, float]:
    t1_bytes = len(t1.encode("utf-8"))
    human_bytes = len(human.encode("utf-8"))
    compaction = 1.0 - (t1_bytes / max(human_bytes, 1))
    tokens = estimate_tokens(t1)
    scores = [t.judge_score for t in trials]
    comprehension = sum(scores) / max(len(scores), 1)
    return {
        "compaction_ratio": round(compaction, 3),
        "comprehension_score": round(comprehension, 2),
        "token_cost": tokens,
        "compression_ratio": round(human_bytes / max(t1_bytes, 1), 1),
    }


# ---------------------------------------------------------------------------
# Trial: Ask agent a question using only the T1 map
# ---------------------------------------------------------------------------

def run_trial(api_key: str, model: str, test_map: TestMap, qa: GroundTruthQA,
              question_idx: int, iteration: int) -> TrialResult:
    """Ask a question using only the T1 map as context."""
    prompt = f"""You are an AI coding agent working in {test_map.injection_ctx}.
You have been given this architecture reference map:

{test_map.t1_content}

Answer this question using ONLY the information in the map above:
{qa.question}

Be precise and specific. If the map does not contain the answer, say so."""

    agent_answer = llm_complete(api_key, model, prompt, max_tokens=300)
    score, reason = judge_comprehension(api_key, model, qa.answer, agent_answer)

    return TrialResult(
        map_id=test_map.id,
        question_idx=question_idx,
        question=qa.question,
        ground_truth=qa.answer,
        agent_answer=agent_answer,
        judge_score=score,
        judge_reason=reason,
        tokens_t1=estimate_tokens(test_map.t1_content),
        tokens_human=estimate_tokens(test_map.human_readable),
        iteration=iteration,
        difficulty=qa.difficulty,
    )


# ---------------------------------------------------------------------------
# Judge: Score answer against ground truth (reuses J17 rubric)
# ---------------------------------------------------------------------------

def judge_comprehension(api_key: str, model: str, ground_truth: str, agent_answer: str) -> Tuple[float, str]:
    prompt = f"""You are judging whether an AI agent correctly answered an architecture question.

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

    resp = llm_complete(api_key, model, prompt, max_tokens=100)

    score = 0.0
    reason = ""
    for line in resp.split("\n"):
        line = line.strip()
        if line.startswith("SCORE:"):
            try:
                score = float(line.split(":", 1)[1].strip())
            except ValueError:
                pass
        elif line.startswith("REASON:"):
            reason = line.split(":", 1)[1].strip()
    return score, reason


# ---------------------------------------------------------------------------
# Weak Section Detection (adapts findWeakCodes)
# ---------------------------------------------------------------------------

def find_weak_maps(trials: List[TrialResult], threshold: float = 9.0,
                   question_threshold: float = 7.0) -> List[Dict]:
    """Find maps scoring below threshold, with per-question breakdown."""
    by_map: Dict[str, List[TrialResult]] = {}
    for t in trials:
        by_map.setdefault(t.map_id, []).append(t)

    weak = []
    for map_id, map_trials in by_map.items():
        avg_score = sum(t.judge_score for t in map_trials) / max(len(map_trials), 1)
        weak_qs = [t for t in map_trials if t.judge_score < question_threshold]

        if avg_score < threshold or weak_qs:
            weak.append({
                "map_id": map_id,
                "avg_score": round(avg_score, 2),
                "weak_questions": [
                    {"question": t.question, "score": t.judge_score, "reason": t.judge_reason}
                    for t in weak_qs
                ],
            })
    return weak


# ---------------------------------------------------------------------------
# Compression Regeneration (adapts learnViaLLM)
# ---------------------------------------------------------------------------

def compress_via_llm(api_key: str, model: str, test_map: TestMap,
                     weak_info: Dict, token_budget: int = 250) -> str:
    """Regenerate a T1 encoding to improve comprehension on weak questions."""
    weak_qs_str = "\n".join(
        f"  - Q: {wq['question']}  GROUND TRUTH: {wq.get('ground_truth', 'N/A')}  (score: {wq['score']}/10 — {wq['reason']})"
        for wq in weak_info.get("weak_questions", [])
    )
    if not weak_qs_str:
        weak_qs_str = f"  Overall average: {weak_info['avg_score']}/10 (below 9.0 threshold)"

    prompt = f"""You are improving a T1-encoded architecture map for AI agent consumption.
The current encoding scores {weak_info['avg_score']}/10 on comprehension.

CRITICAL RULES:
- You MUST preserve ALL existing data values exactly (numbers, names, codes, states)
- You MUST NOT change any factual content — only restructure for clarity
- Every line of the current encoding contains information that MUST be preserved
- The weak questions below show what information is hard to extract — make it more explicit

Weak questions that scored below 7.0:
{weak_qs_str}

Current encoding (PRESERVE ALL DATA):
{test_map.t1_content}

Human-readable source (use for context, not as sole source):
{test_map.human_readable}

Produce an improved T1 encoding that:
1. Preserves ALL factual data from the current encoding exactly (every number, name, code, state)
2. Makes the information needed for weak questions more explicit or prominent
3. Uses pipe-delimited fields, short codes, no prose
4. Stays under {token_budget} tokens (roughly {token_budget * 4} characters)
5. Keeps the same format convention: {test_map.map_type} type tag, one entry per line
6. Keeps the first line as the type header

Output ONLY the new encoding, nothing else."""

    return llm_complete(api_key, model, prompt, max_tokens=800)


# ---------------------------------------------------------------------------
# Ground-Truth Fixtures
# ---------------------------------------------------------------------------

def build_test_maps(maps_dir: str) -> List[TestMap]:
    """Build test map fixtures with ground-truth Q&A pairs."""
    human_readable_dir = os.path.expanduser("~/Downloads/mdemg-gap")

    maps = []

    # ── flow_retrieval ──
    maps.append(TestMap(
        id="flow_retrieval",
        map_type="FLOW",
        file_path=f"{maps_dir}/flow_retrieval.md",
        human_readable=_read(f"{human_readable_dir}/flow_retrieval.md", f"{maps_dir}/flow_retrieval.md"),
        t1_content=Path(f"{maps_dir}/flow_retrieval.md").read_text(),
        injection_ctx="Agent working in internal/retrieval/",
        questions=[
            GroundTruthQA("What are the retrieval scoring weights?",
                          "vector:0.55, activation:0.30, recency:0.10, confidence:0.05, hub:-0.08, redundancy:-0.12",
                          "factual"),
            GroundTruthQA("What is the embedding dimension?", "3072", "factual"),
            GroundTruthQA("What training data does the retrieval pipeline produce?",
                          "(query, candidate, score) JSONL tuples for neural sidecar cross-encoder training",
                          "relational"),
            GroundTruthQA("If symbol search finds candidates, what triggered it?",
                          "Query contained #symbol prefix", "inferential"),
            GroundTruthQA("What is the max graph expansion depth?", "3", "factual"),
        ],
    ))

    # ── schema_neo4j ──
    maps.append(TestMap(
        id="schema_neo4j",
        map_type="SCHEMA",
        file_path=f"{maps_dir}/schema_neo4j.md",
        human_readable=_read(f"{human_readable_dir}/schema_neo4j.md", f"{maps_dir}/schema_neo4j.md"),
        t1_content=Path(f"{maps_dir}/schema_neo4j.md").read_text(),
        injection_ctx="Agent writing Cypher queries or modifying schema",
        questions=[
            GroundTruthQA("What edge type represents Hebbian learning?",
                          "CO_ACTIVATED_WITH", "factual"),
            GroundTruthQA("What prevents grounding loss in L5 concepts?",
                          "GROUNDED_BY skip connections from L5 directly to L0", "relational"),
            GroundTruthQA("What are the DBSCAN eps values for L0 and L4?",
                          "L0: 0.10, L4: 0.26", "factual"),
            GroundTruthQA("Which node label stores LLM-named emergent concepts?",
                          "EmergentConcept", "factual"),
            GroundTruthQA("What does the GUIDANCE_OUTCOME edge track?",
                          "Whether Jiminy guidance was followed, ignored, or contradicted", "relational"),
        ],
    ))

    # ── uxts_frameworks ──
    maps.append(TestMap(
        id="uxts_frameworks",
        map_type="UXTS",
        file_path=f"{maps_dir}/uxts_frameworks.md",
        human_readable=_read(f"{human_readable_dir}/uxts_frameworks_human_readable.md",
                             f"{human_readable_dir}/uxts_frameworks.md"),
        t1_content=Path(f"{maps_dir}/uxts_frameworks.md").read_text(),
        injection_ctx="Agent writing specs or configuring CI gates",
        questions=[
            GroundTruthQA("Which frameworks are merge-blocking in CI?",
                          "UATS, UPTS, USTS, UNTS", "factual"),
            GroundTruthQA("How many UATS specs and variants exist?",
                          "195 specs, 224 variants, 318 test cases", "factual"),
            GroundTruthQA("Why is UVTS not CI-gated?",
                          "Runner is demoted (GAP-04) — functional but inline grading missing, would produce false passes",
                          "relational"),
            GroundTruthQA("Which framework has no runner at all?",
                          "UAMS — the auth methods framework has no runner implementation (tracked as GAP-21)",
                          "factual"),
            GroundTruthQA("What is the UETS framework for?",
                          "Evaluating LLM emergence concept-naming quality", "factual"),
            GroundTruthQA("Which frameworks use soft-fail CI gating?",
                          "UBTS, UOBS, UOTS, UETS", "factual"),
        ],
    ))

    # ── dict_pkg_codes ──
    maps.append(TestMap(
        id="dict_pkg_codes",
        map_type="DICT",
        file_path=f"{maps_dir}/dict_pkg_codes.md",
        human_readable=_read(f"{human_readable_dir}/dict_pkg_codes.md", f"{maps_dir}/dict_pkg_codes.md"),
        t1_content=Path(f"{maps_dir}/dict_pkg_codes.md").read_text(),
        injection_ctx="Agent navigating unfamiliar packages",
        questions=[
            GroundTruthQA("What does the 'ret' package code stand for?",
                          "Retrieval pipeline: vector + activation + scoring + cache", "factual"),
            GroundTruthQA("How many files and lines of code are in the API package?",
                          "43 files, 16.9K LOC", "factual"),
            GroundTruthQA("What does the 'hid' package handle?",
                          "Hidden layer: DBSCAN clustering + message passing + emergence", "factual"),
            GroundTruthQA("Which package has the most files?",
                          "cli with 50 files", "inferential"),
            GroundTruthQA("What embedding dimensions does the emb package use?",
                          "3072-dim vectors", "factual"),
            GroundTruthQA("What is the package code for the Jiminy inner voice?",
                          "jim", "factual"),
        ],
    ))

    # ── dep_pkg_graph ──
    maps.append(TestMap(
        id="dep_pkg_graph",
        map_type="DEP",
        file_path=f"{maps_dir}/dep_pkg_graph.md",
        human_readable=_read(f"{human_readable_dir}/dep_pkg_graph.md", f"{maps_dir}/dep_pkg_graph.md"),
        t1_content=Path(f"{maps_dir}/dep_pkg_graph.md").read_text(),
        injection_ctx="Agent navigating unfamiliar packages",
        questions=[
            GroundTruthQA("Which packages are leaf packages with no dependencies?",
                          "lng, sym, cfg, mdl, plg, ath", "factual"),
            GroundTruthQA("What does the api package depend on?",
                          "ret, hid, lrn, jim, ape, con, cns, grd, lng, sym, emb, cfg, mdl, plg, ath, scr, xfr, mta, llm, unt, bkp, met, gap",
                          "factual"),
            GroundTruthQA("Why does ret depend on plg?",
                          "For match-module (MatchIngestionModule) functionality", "relational"),
            GroundTruthQA("Which package does hid depend on for emergence naming?",
                          "llm (for emergence-naming)", "relational"),
            GroundTruthQA("What does grd depend on emb for?",
                          "Constraint retrieval (embedding-based similarity search for constraints)", "relational"),
        ],
    ))

    # ── flow_observe_learn_consolidate ──
    maps.append(TestMap(
        id="flow_observe_learn_consolidate",
        map_type="FLOW",
        file_path=f"{maps_dir}/flow_observe_learn_consolidate.md",
        human_readable=_read(f"{human_readable_dir}/flow_observe_learn_consolidate.md",
                             f"{maps_dir}/flow_observe_learn_consolidate.md"),
        t1_content=Path(f"{maps_dir}/flow_observe_learn_consolidate.md").read_text(),
        injection_ctx="Agent working in conversation/learning/hidden layers",
        questions=[
            GroundTruthQA("What does the observe endpoint return?",
                          "obs_id, node_id, surprise_score", "factual"),
            GroundTruthQA("What triggers Hebbian learning?",
                          "Co-retrieval activates learning (CO_ACTIVATED_WITH edge creation)", "relational"),
            GroundTruthQA("How many phases does the consolidation pipeline have?",
                          "22 phases", "factual"),
            GroundTruthQA("What is the DBSCAN eps scaling range across layers?",
                          "0.10 to 0.26", "factual"),
            GroundTruthQA("What is Phase 103 in the consolidation pipeline?",
                          "Dynamic emergence — LLM-named concepts", "factual"),
        ],
    ))

    # ── flow_jiminy_guide ──
    maps.append(TestMap(
        id="flow_jiminy_guide",
        map_type="FLOW",
        file_path=f"{maps_dir}/flow_jiminy_guide.md",
        human_readable=_read(f"{human_readable_dir}/flow_jiminy_guide.md", f"{maps_dir}/flow_jiminy_guide.md"),
        t1_content=Path(f"{maps_dir}/flow_jiminy_guide.md").read_text(),
        injection_ctx="Agent working in internal/jiminy/",
        questions=[
            GroundTruthQA("What triggers the Jiminy guide flow?",
                          "Claude hook .claude/hooks/prompt-context.sh (runs every prompt)", "factual"),
            GroundTruthQA("What is the timeout for the guide orchestration?",
                          "6 seconds", "factual"),
            GroundTruthQA("What are the three J17 encoding tiers and their approximate token sizes?",
                          "T1: coded ~15 tokens (80% traffic), T2: telegraphic ~50-100 tokens (15%), T3: full NL ~200+ tokens (5%)",
                          "factual"),
            GroundTruthQA("What parallel sources does the guide query?",
                          "Constraints from graph (cns.Suggest), corrections via vector search, CONTRADICTS edges, frontier nodes (knowledge gaps), and per-session trust scoring",
                          "relational"),
            GroundTruthQA("What does jim.Effectiveness track?",
                          "Guidance_id for feedback correlation (whether guidance was followed)", "relational"),
        ],
    ))

    # ── flow_rsic_cycle ──
    maps.append(TestMap(
        id="flow_rsic_cycle",
        map_type="FLOW",
        file_path=f"{maps_dir}/flow_rsic_cycle.md",
        human_readable=_read(f"{human_readable_dir}/flow_rsic_cycle.md", f"{maps_dir}/flow_rsic_cycle.md"),
        t1_content=Path(f"{maps_dir}/flow_rsic_cycle.md").read_text(),
        injection_ctx="Agent working in internal/ape/",
        questions=[
            GroundTruthQA("What are the 5 stages of the RSIC cycle?",
                          "Assess, Reflect, Plan, Dispatch, Calibrate", "factual"),
            GroundTruthQA("How many action types can Dispatch execute?",
                          "13 action types", "factual"),
            GroundTruthQA("What are the three RSIC tiers and their intervals?",
                          "micro: 30s, meso: 10m, macro: 30m", "factual"),
            GroundTruthQA("What safety mechanisms does RSIC use?",
                          "Dry-run, snapshot, rollback, blast-radius limits, protected spaces", "factual"),
            GroundTruthQA("What are the 7 health dimensions assessed in Stage 1?",
                          "Retrieval, memory, edge, task, guidance, protocol, synergy", "factual"),
        ],
    ))

    # ── svc_external ──
    maps.append(TestMap(
        id="svc_external",
        map_type="SVC",
        file_path=f"{maps_dir}/svc_external.md",
        human_readable=_read(f"{human_readable_dir}/svc_external.md", f"{maps_dir}/svc_external.md"),
        t1_content=Path(f"{maps_dir}/svc_external.md").read_text(),
        injection_ctx="Agent configuring services or ports",
        questions=[
            GroundTruthQA("What port does the MDEMG server listen on?",
                          "9999", "factual"),
            GroundTruthQA("What port does the neural sidecar use?",
                          "8100", "factual"),
            GroundTruthQA("How does the MCP server communicate?",
                          "Via stdio (runs as subprocess for IDE integration)", "factual"),
            GroundTruthQA("What gRPC services are defined for content ingestion?",
                          "IngestionModule with Matches, Parse, and Sync operations", "relational"),
            GroundTruthQA("What does the ReasoningModule gRPC service do?",
                          "Rerank — re-scores retrieval candidates", "factual"),
        ],
    ))

    # ── dist_channels ──
    maps.append(TestMap(
        id="dist_channels",
        map_type="DIST",
        file_path=f"{maps_dir}/dist_channels.md",
        human_readable=_read(f"{human_readable_dir}/dist_channels.md", f"{maps_dir}/dist_channels.md"),
        t1_content=Path(f"{maps_dir}/dist_channels.md").read_text(),
        injection_ctx="Agent working on packaging or CI",
        questions=[
            GroundTruthQA("How do you install MDEMG on macOS?",
                          "brew install reh3376/mdemg/mdemg", "factual"),
            GroundTruthQA("What technology is the Linux companion app built with?",
                          "Tauri (Rust + JS) with Catppuccin theme", "factual"),
            GroundTruthQA("Which platform companion app is not implemented?",
                          "Windows companion (GAP-13)", "factual"),
            GroundTruthQA("What CI workflow handles cross-compilation releases?",
                          "release.yml triggered by tag push, uses goreleaser", "relational"),
            GroundTruthQA("What does auto-pr.yml do?",
                          "Auto-creates PR to main when pushing to *_dev* branches", "factual"),
        ],
    ))

    return maps


def _read(primary: str, fallback: str) -> str:
    """Read primary file, fall back to secondary."""
    p = Path(os.path.expanduser(primary))
    if p.exists():
        return p.read_text()
    return Path(os.path.expanduser(fallback)).read_text()


# ---------------------------------------------------------------------------
# Main Loop
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description="Architecture Map Optimization Test")
    parser.add_argument("--map", help="Test a single map by ID")
    parser.add_argument("--maps-dir", default="docs/architecture/maps", help="Maps directory")
    parser.add_argument("--iterations", type=int, default=5, help="Max iterations")
    parser.add_argument("--baseline-only", action="store_true", help="Score baseline only, no iteration")
    parser.add_argument("--model", default="gpt-4.1-mini", help="OpenAI model for agent + judge")
    parser.add_argument("--compress-model", default="gpt-4.1", help="Model for compression regeneration")
    parser.add_argument("--threshold", type=float, default=9.0, help="Score threshold")
    parser.add_argument("--question-threshold", type=float, default=7.0, help="Per-question minimum")
    parser.add_argument("--token-budget", type=int, default=250, help="Max tokens per map")
    parser.add_argument("--output-dir", default="docs/architecture/benchmarks/.stash",
                        help="Output directory for results")
    args = parser.parse_args()

    api_key = load_api_key()
    maps_dir = str(Path(args.maps_dir).resolve())
    test_maps = build_test_maps(maps_dir)

    if args.map:
        test_maps = [m for m in test_maps if m.id == args.map]
        if not test_maps:
            print(f"ERROR: Map '{args.map}' not found. Available: {[m.id for m in build_test_maps(maps_dir)]}")
            sys.exit(1)

    print(f"=== Architecture Map Optimization Test ===")
    print(f"Model: {args.model} | Compress: {args.compress_model}")
    print(f"Maps: {len(test_maps)} | Max iterations: {args.iterations}")
    print(f"Threshold: {args.threshold}/10 | Question minimum: {args.question_threshold}/10")
    print(f"Token budget: {args.token_budget}")
    print()

    iterations: List[IterationLog] = []
    max_iter = 1 if args.baseline_only else args.iterations

    # Track best-seen T1 content per map for regression protection
    best_t1: Dict[str, Tuple[str, float]] = {}  # map_id -> (t1_content, score)
    for tm in test_maps:
        best_t1[tm.id] = (tm.t1_content, 0.0)  # baseline, score TBD

    for iteration in range(1, max_iter + 1):
        print(f"{'=' * 50}")
        print(f"  ITERATION {iteration} / {max_iter}")
        print(f"{'=' * 50}")

        all_trials: List[TrialResult] = []

        for tm in test_maps:
            print(f"\n  [{tm.id}] ({tm.map_type}, {estimate_tokens(tm.t1_content)} tokens)")

            for qi, qa in enumerate(tm.questions):
                trial = run_trial(api_key, args.model, tm, qa, qi, iteration)
                all_trials.append(trial)

                status = "PASS" if trial.judge_score >= args.question_threshold else "WEAK"
                print(f"    Q{qi + 1} [{trial.difficulty}] {trial.judge_score:.0f}/10 {status}"
                      f" — {trial.judge_reason[:60]}")

            # Per-map metrics
            map_trials = [t for t in all_trials if t.map_id == tm.id]
            metrics = compute_metrics(tm.t1_content, tm.human_readable, map_trials)
            avg = metrics["comprehension_score"]
            print(f"    >> avg={avg:.1f}/10 | tokens={metrics['token_cost']}"
                  f" | compaction={metrics['compaction_ratio']:.0%}"
                  f" | compression={metrics['compression_ratio']:.1f}x")

        # Iteration summary + per-map regression detection
        iter_avg = sum(t.judge_score for t in all_trials) / max(len(all_trials), 1)
        print(f"\n  Iteration {iteration} avg: {iter_avg:.2f}/10")

        # Update best-seen tracker and revert regressions
        if iteration > 1:
            for tm in test_maps:
                map_trials = [t for t in all_trials if t.map_id == tm.id]
                if not map_trials:
                    continue
                current_avg = sum(t.judge_score for t in map_trials) / len(map_trials)
                prev_best_t1, prev_best_score = best_t1[tm.id]
                if current_avg > prev_best_score:
                    best_t1[tm.id] = (tm.t1_content, current_avg)
                elif current_avg < prev_best_score - 0.5:
                    # Regression detected — revert to best-seen
                    print(f"    REVERT {tm.id}: {current_avg:.1f} < {prev_best_score:.1f} (best-seen). Restoring previous encoding.")
                    tm.t1_content = prev_best_t1
        else:
            # Record baseline scores
            for tm in test_maps:
                map_trials = [t for t in all_trials if t.map_id == tm.id]
                if map_trials:
                    baseline_avg = sum(t.judge_score for t in map_trials) / len(map_trials)
                    best_t1[tm.id] = (tm.t1_content, baseline_avg)

        iter_log = IterationLog(
            iteration=iteration,
            timestamp=datetime.utcnow().isoformat() + "Z",
            trials=all_trials,
            avg_score=round(iter_avg, 2),
            map_changes=[],
        )

        # Check convergence
        weak_maps = find_weak_maps(all_trials, args.threshold, args.question_threshold)
        if not weak_maps:
            print(f"\n  All maps >= {args.threshold}/10, no question below {args.question_threshold}/10 — converged!")
            iterations.append(iter_log)
            break

        if args.baseline_only:
            iterations.append(iter_log)
            break

        # Regenerate weak maps
        print(f"\n  {len(weak_maps)} weak map(s) — regenerating:")
        changes = []
        for wm in weak_maps:
            map_id = wm["map_id"]
            tm = next(m for m in test_maps if m.id == map_id)
            print(f"    {map_id}: avg={wm['avg_score']}/10, {len(wm['weak_questions'])} weak question(s)")

            # Enrich weak questions with ground truth for the compression prompt
            for wq in wm["weak_questions"]:
                for qa in tm.questions:
                    if qa.question == wq["question"]:
                        wq["ground_truth"] = qa.answer
                        break

            new_t1 = compress_via_llm(api_key, args.compress_model, tm, wm, args.token_budget)
            new_tokens = estimate_tokens(new_t1)
            print(f"      -> New encoding: {new_tokens} tokens (was {estimate_tokens(tm.t1_content)})")

            change = MapChange(
                map_id=map_id,
                old_t1=tm.t1_content,
                new_t1=new_t1,
                score_before=wm["avg_score"],
                weak_questions=[wq["question"] for wq in wm["weak_questions"]],
            )
            changes.append(change)

            # Update the map for next iteration
            tm.t1_content = new_t1

        iter_log.map_changes = changes
        iterations.append(iter_log)

        if not changes:
            print("\n  No improvements generated — stopping")
            break
        print()

    # Write results
    ts = datetime.now().strftime("%Y%m%d_%H%M%S")
    os.makedirs(args.output_dir, exist_ok=True)

    # JSON log
    json_path = f"{args.output_dir}/map_optimization_{ts}.json"
    log_data = {
        "timestamp": datetime.utcnow().isoformat() + "Z",
        "model": args.model,
        "compress_model": args.compress_model,
        "threshold": args.threshold,
        "token_budget": args.token_budget,
        "maps_tested": len(test_maps),
        "iterations": [
            {
                "iteration": il.iteration,
                "timestamp": il.timestamp,
                "avg_score": il.avg_score,
                "trials": [
                    {
                        "map_id": t.map_id,
                        "question_idx": t.question_idx,
                        "question": t.question,
                        "ground_truth": t.ground_truth,
                        "agent_answer": t.agent_answer,
                        "judge_score": t.judge_score,
                        "judge_reason": t.judge_reason,
                        "tokens_t1": t.tokens_t1,
                        "tokens_human": t.tokens_human,
                        "difficulty": t.difficulty,
                    }
                    for t in il.trials
                ],
                "map_changes": [
                    {
                        "map_id": mc.map_id,
                        "score_before": mc.score_before,
                        "weak_questions": mc.weak_questions,
                        "old_t1_tokens": estimate_tokens(mc.old_t1),
                        "new_t1_tokens": estimate_tokens(mc.new_t1),
                        "new_t1": mc.new_t1,
                    }
                    for mc in il.map_changes
                ],
            }
            for il in iterations
        ],
    }
    with open(json_path, "w") as f:
        json.dump(log_data, f, indent=2)

    # Markdown report
    md_path = f"{args.output_dir}/map_optimization_{ts}.md"
    write_report(md_path, iterations, test_maps, args)

    # Print final
    print(f"\n{'=' * 50}")
    print(f"  RESULTS")
    print(f"{'=' * 50}")

    if len(iterations) > 1:
        first_avg = iterations[0].avg_score
        last_avg = iterations[-1].avg_score
        print(f"  Score: {first_avg:.1f} -> {last_avg:.1f} ({last_avg - first_avg:+.1f})")
        total_changes = sum(len(il.map_changes) for il in iterations)
        print(f"  Map regenerations: {total_changes}")
    else:
        print(f"  Score: {iterations[0].avg_score:.1f}/10")

    for tm in test_maps:
        metrics = compute_metrics(
            tm.t1_content, tm.human_readable,
            [t for il in iterations for t in il.trials if t.map_id == tm.id and t.iteration == iterations[-1].iteration],
        )
        print(f"  {tm.id}: score={metrics['comprehension_score']:.1f}"
              f" tokens={metrics['token_cost']}"
              f" compaction={metrics['compaction_ratio']:.0%}"
              f" compression={metrics['compression_ratio']:.1f}x")

    print(f"\n  JSON: {json_path}")
    print(f"  Report: {md_path}")

    # Write winning T1 encodings back ONLY if score improved over baseline
    if len(iterations) > 1:
        first_avg = iterations[0].avg_score
        last_avg = iterations[-1].avg_score
        if last_avg > first_avg:
            print(f"\n  Writing optimized maps back to {args.maps_dir}/ (score improved {first_avg:.1f} -> {last_avg:.1f}):")
            for tm in test_maps:
                # Find best iteration for this map
                best_score = 0.0
                best_t1 = None
                for il in iterations:
                    map_trials = [t for t in il.trials if t.map_id == tm.id]
                    if map_trials:
                        avg_s = sum(t.judge_score for t in map_trials) / len(map_trials)
                        if avg_s > best_score:
                            best_score = avg_s
                            # Find the T1 content used in this iteration
                            # For iteration 1 it's the original; for later ones check changes
                            best_t1 = tm.t1_content  # current (last iteration's input)
                            for prev_il in iterations:
                                for mc in prev_il.map_changes:
                                    if mc.map_id == tm.id and prev_il.iteration == il.iteration - 1:
                                        best_t1 = mc.new_t1
                if best_t1 and best_score > first_avg:
                    Path(tm.file_path).write_text(best_t1)
                    print(f"    {tm.id}.md ({estimate_tokens(best_t1)} tokens, score={best_score:.1f})")
                else:
                    print(f"    {tm.id}.md SKIPPED (no improvement over baseline {first_avg:.1f})")
        else:
            print(f"\n  Score regressed ({first_avg:.1f} -> {last_avg:.1f}) — NOT writing files. Original maps preserved.")


def write_report(path: str, iterations: List[IterationLog], test_maps: List[TestMap], args) -> None:
    """Write markdown report."""
    lines = [
        "# Architecture Map Optimization Report\n",
        f"**Date**: {datetime.utcnow().isoformat()}Z  ",
        f"**Model**: {args.model} (judge/agent), {args.compress_model} (compression)  ",
        f"**Iterations**: {len(iterations)}  ",
        f"**Threshold**: {args.threshold}/10  \n",
        "## Learning Progress\n",
        "| Iteration | Avg Score | Maps Changed |",
        "|-----------|-----------|-------------|",
    ]
    for il in iterations:
        lines.append(f"| {il.iteration} | {il.avg_score:.2f} | {len(il.map_changes)} |")

    lines.append("\n## Per-Map Results\n")
    lines.append("| Map | Type | Tokens | Compaction | Final Score |")
    lines.append("|-----|------|--------|------------|-------------|")
    for tm in test_maps:
        final_trials = [t for t in iterations[-1].trials if t.map_id == tm.id]
        metrics = compute_metrics(tm.t1_content, tm.human_readable, final_trials)
        lines.append(f"| {tm.id} | {tm.map_type} | {metrics['token_cost']}"
                      f" | {metrics['compaction_ratio']:.0%} | {metrics['comprehension_score']:.1f} |")

    # Per-question breakdown for last iteration
    lines.append("\n## Question-Level Detail (Final Iteration)\n")
    for tm in test_maps:
        lines.append(f"\n### {tm.id}\n")
        lines.append("| # | Difficulty | Score | Question | Reason |")
        lines.append("|---|-----------|-------|----------|--------|")
        final_trials = [t for t in iterations[-1].trials if t.map_id == tm.id]
        for t in final_trials:
            status = "PASS" if t.judge_score >= args.question_threshold else "**WEAK**"
            lines.append(f"| {t.question_idx + 1} | {t.difficulty} | {t.judge_score:.0f} {status}"
                          f" | {t.question} | {t.judge_reason} |")

    # Map changes
    has_changes = any(il.map_changes for il in iterations)
    if has_changes:
        lines.append("\n## Map Evolution\n")
        for il in iterations:
            for mc in il.map_changes:
                lines.append(f"### {mc.map_id} (Iteration {il.iteration})\n")
                lines.append(f"Score before: {mc.score_before}/10  ")
                lines.append(f"Weak questions: {len(mc.weak_questions)}  \n")
                lines.append(f"```\n{mc.new_t1}\n```\n")

    Path(path).write_text("\n".join(lines) + "\n")


if __name__ == "__main__":
    main()
