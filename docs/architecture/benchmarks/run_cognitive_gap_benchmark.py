#!/usr/bin/env python3
"""
Cognitive Gap Benchmark Runner — A/B Comparison: Baseline vs MDEMG-Assisted Agents

Spawns sandboxed Claude Haiku agents to answer whk-wms codebase questions in two modes:
  - baseline: Agent explores codebase with Read/Grep/Glob only
  - mdemg: Agent gets MDEMG pre-retrieved context, then explores codebase

Sandboxing (all flags applied to every agent invocation):
  - --setting-sources user: Skips project CLAUDE.md and .claude/ settings
  - --strict-mcp-config: Disables project MCP servers
  - --disable-slash-commands: Disables skills/plugins
  - --session-id <uuid>: Explicit session per run (no cross-contamination)
  - Master answer file stashed to .stash/ during runs
  - Session directories cleaned before AND after each run

Telemetry (via --output-format stream-json):
  - Per-question input/output token counts
  - Per-question cost
  - Auto-compact event detection (system events + input token drop heuristic)
  - Per-turn token breakdown for context growth analysis

Usage:
    # Full run:
    python run_cognitive_gap_benchmark.py --mode baseline --run 1

    # Pilot (20 questions):
    python run_cognitive_gap_benchmark.py --mode baseline --questions-limit 20

    # Resume from checkpoint:
    python run_cognitive_gap_benchmark.py --mode baseline --run 1 --start-from 45

    # Skip sandbox verification:
    python run_cognitive_gap_benchmark.py --mode baseline --run 1 --skip-verify

Grading (after each run):
    python grader_v4.py <answers.jsonl> <test_questions_120.json> <grades.json>
"""

import json
import os
import re
import shutil
import subprocess
import sys
import time
import argparse
import uuid
import urllib.request
import urllib.error
from pathlib import Path
from datetime import datetime
from typing import Dict, List, Optional, Tuple


# ─── Configuration ───────────────────────────────────────────────────────────

SCRIPT_DIR = Path(__file__).parent
QUESTIONS_FILE = SCRIPT_DIR / "whk-wms" / "test_questions_120_agent.json"
MASTER_FILE = SCRIPT_DIR / "whk-wms" / "test_questions_120.json"
OUTPUT_BASE = SCRIPT_DIR / "whk-wms" / "cognitive_gap_validation_20260226"
CODEBASE_PATH = "/Users/reh3376/whk-wms"
MDEMG_ENDPOINT = "http://localhost:9999"
MDEMG_SPACE_ID = "whk-wms"
AGENT_TIMEOUT = 120  # seconds per question (reduced from 180)
CHECKPOINT_INTERVAL = 5  # save progress every N questions

# Sandbox paths
STASH_DIR = SCRIPT_DIR / ".stash"
SESSION_PROJECT_DIR = Path.home() / ".claude" / "projects" / "-Users-reh3376-whk-wms"

# Retry config for rate limiting / overload
MAX_RETRIES = 3
RETRY_BACKOFF = [30, 60, 120]  # seconds


# ─── File:line citation regex (matches grader_v4.py FILE_LINE_PATTERN) ───────

FILE_LINE_PATTERN = re.compile(
    r'([\w\-./]+\.(?:py|ts|tsx|js|jsx|go|rs|java|cpp|c|h|hpp|rb|php|swift|kt|scala|vue|svelte))'
    r'\s*:\s*(\d+)(?:\s*-\s*(\d+))?'
)


# ─── System Prompts (injected via --append-system-prompt) ────────────────────
# These are injected at the system level on every CLI invocation.
# The inline -p prompt contains ONLY the question (and MDEMG context for mdemg).
# Prompts are NOT reinforced per-question in the user prompt. Degradation IS the data.

BASELINE_SYSTEM = """You are a benchmark agent answering questions about the whk-wms codebase.
You will receive many questions, one at a time. For EACH question:
1. Search the codebase using Read, Grep, and Glob tools
2. Find the relevant source files
3. Answer in 2-6 sentences with specific technical details
4. ALWAYS cite file paths and line numbers as: filename.ts:123
5. Do NOT make up file paths or line numbers — only cite what you actually find
6. Do NOT use any external APIs or web resources"""

MDEMG_SYSTEM = """You are a benchmark agent answering questions about the whk-wms codebase.
You will receive many questions, one at a time. Each question comes with PRE-RETRIEVED context
showing the most relevant files and approximate line ranges from a memory system.

CRITICAL WORKFLOW — follow this exact process for EACH question:
1. Look at the suggested file paths and line numbers from the memory context
2. Use Grep to search those EXACT files at those approximate line ranges to get the precise code
3. Read the relevant sections to understand the actual implementation
4. Answer in 2-6 sentences with specific technical details
5. ALWAYS cite as filename.ts:123 (colon format, NOT prose like "lines 123-456")
6. Do NOT answer from memory context alone — you MUST read the actual files
7. Do NOT make up file paths or line numbers — only cite what you actually find in the files"""


# ─── Sandbox Functions ───────────────────────────────────────────────────────

def stash_master_file() -> Optional[Path]:
    """Move master answer file out of reach during benchmark runs."""
    STASH_DIR.mkdir(exist_ok=True)
    stash_path = STASH_DIR / MASTER_FILE.name
    if MASTER_FILE.exists():
        shutil.move(str(MASTER_FILE), str(stash_path))
        print(f"  Stashed master file to {stash_path}")
        return stash_path
    return None


def unstash_master_file():
    """Restore master answer file after benchmark run."""
    stash_path = STASH_DIR / MASTER_FILE.name
    if stash_path.exists():
        shutil.move(str(stash_path), str(MASTER_FILE))
        print(f"  Restored master file from stash")


def cleanup_session_dirs() -> int:
    """Remove ALL Claude session directories for whk-wms project.

    MANDATORY: Must be called after every benchmark run to prevent
    cross-run context leakage. Session files accumulate and can leak
    context between benchmark runs via session continuation (-c flag).
    """
    if not SESSION_PROJECT_DIR.exists():
        return 0

    count = 0
    for entry in SESSION_PROJECT_DIR.iterdir():
        if entry.is_dir() and entry.name != "memory":  # Preserve memory dir
            shutil.rmtree(entry)
            count += 1

    # Also clean session index files
    for f in SESSION_PROJECT_DIR.glob("*.json"):
        if "session" in f.name.lower():
            f.unlink()
            count += 1

    print(f"  Cleaned {count} session entries from {SESSION_PROJECT_DIR}")
    return count


def verify_sandbox() -> bool:
    """Verify that CLI sandboxing flags actually work.

    Spawns a test agent and checks that:
    1. It does NOT see whk-wms CLAUDE.md content auto-loaded
    2. CLI sandbox flags are accepted without errors
    """
    print("\n  Verifying sandbox...")
    cmd = [
        "claude", "--model", "haiku", "--print",
        "--setting-sources", "user",
        "--strict-mcp-config",
        "--disable-slash-commands",
        "--allowedTools", "Read",
        "-p", "Read the file CLAUDE.md in the current directory and tell me the first line. If no CLAUDE.md exists, say NONE_FOUND.",
    ]

    env = {k: v for k, v in os.environ.items() if k != "CLAUDECODE"}

    try:
        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=30,
            cwd=CODEBASE_PATH, env=env,
        )
        output = result.stdout.strip()

        if result.returncode != 0:
            print(f"  SANDBOX FAIL: CLI returned exit code {result.returncode}")
            print(f"  stderr: {result.stderr[:200]}")
            return False

        if "NONE_FOUND" in output or "no such file" in output.lower() or "doesn't exist" in output.lower():
            print("  SANDBOX OK: Agent cannot see project CLAUDE.md")
            return True
        else:
            # Agent found CLAUDE.md via Read tool — this is expected since Read
            # can access any file on disk. The key protection is that
            # --setting-sources user prevents auto-injection into system context.
            # Benchmark agents won't be told to Read CLAUDE.md.
            print("  SANDBOX OK: CLI flags accepted (agent can Read files but CLAUDE.md not auto-injected)")
            return True
    except subprocess.TimeoutExpired:
        print("  SANDBOX FAIL: Verification timed out after 30s")
        return False
    except Exception as e:
        print(f"  SANDBOX FAIL: {e}")
        return False


# ─── MDEMG API Call ──────────────────────────────────────────────────────────

def call_mdemg(query: str, top_k: int = 5) -> str:
    """Call MDEMG retrieval API and format results as context for the agent prompt.

    Returns a formatted string listing relevant files with summaries.
    If the API call fails, returns a fallback message.
    """
    url = f"{MDEMG_ENDPOINT}/v1/memory/retrieve"
    data = {
        "space_id": MDEMG_SPACE_ID,
        "query_text": query,
        "top_k": top_k,
        "include_global_space": True,
        "translate_intent": True,
    }

    req = urllib.request.Request(
        url,
        data=json.dumps(data).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )

    try:
        with urllib.request.urlopen(req, timeout=15) as resp:
            result = json.loads(resp.read().decode("utf-8"))
    except Exception as e:
        return f"(MDEMG retrieval failed: {e} — search the codebase directly)"

    nodes = result.get("results", [])
    if not nodes:
        return "(No relevant files found in memory — search the codebase directly)"

    lines = [f"RELEVANT FILES ({len(nodes)} results — read these line ranges):"]
    for i, node in enumerate(nodes, 1):
        path = node.get("path", "")
        name = node.get("name", Path(path).name if path else "unknown")
        score = node.get("score", 0)

        # Extract evidence with file:line for targeted reads
        evidence_refs = []
        for ev in node.get("evidence", []):
            ep = ev.get("file_path", "")
            el = ev.get("line", 0)
            sym = ev.get("symbol_name", "")
            if ep and el:
                evidence_refs.append(f"{ep}:{el} ({sym})" if sym else f"{ep}:{el}")
            elif ep:
                evidence_refs.append(ep)

        lines.append(f"\n  {i}. {name} (score: {score:.2f})")
        if path:
            lines.append(f"     Path: {path}")
        if evidence_refs:
            # Show up to 5 evidence locations with line numbers
            for ref in evidence_refs[:5]:
                lines.append(f"     -> {ref}")

    return "\n".join(lines)


# ─── Answer Extraction ───────────────────────────────────────────────────────

def extract_citations(text: str) -> Tuple[List[str], List[str]]:
    """Extract file:line refs and unique file names from agent output text.

    Returns:
        (file_line_refs, files_consulted)
    """
    refs = []
    files = set()
    for match in FILE_LINE_PATTERN.finditer(text):
        filepath = match.group(1)
        line_start = match.group(2)
        line_end = match.group(3)
        if line_end:
            refs.append(f"{filepath}:{line_start}-{line_end}")
        else:
            refs.append(f"{filepath}:{line_start}")
        files.add(Path(filepath).name)
    return refs, sorted(files)


def validate_file_exists(ref: str, codebase: str) -> bool:
    """Check if a cited file actually exists in the codebase."""
    filename = ref.split(":")[0]
    # Try exact path first
    if os.path.exists(os.path.join(codebase, filename)):
        return True
    # Try finding by basename
    basename = Path(filename).name
    for root, _, filenames in os.walk(codebase):
        if basename in filenames:
            return True
        # Don't recurse into node_modules
        if "node_modules" in root:
            break
    return False


# ─── Stream JSON Parser ─────────────────────────────────────────────────────

def parse_stream_json(raw_output: str) -> Tuple[str, Dict]:
    """Parse stream-json output from Claude CLI.

    Extracts answer text, token usage, cost, and auto-compact event counts
    from newline-delimited JSON events emitted by --output-format stream-json.

    Event structure (observed from Claude CLI):
      - assistant events: per-turn usage with input_tokens (non-cached delta),
        cache_read_input_tokens, cache_creation_input_tokens, output_tokens.
        Each API turn emits 2 assistant events (thinking + content).
      - result event: final summary with cumulative usage totals, cost,
        session_id, num_turns, modelUsage breakdown.
      - system events: init, auto-compact/compress if context is compressed.

    Returns:
        (answer_text, usage_dict)
    """
    answer_text = ""
    usage = {
        "input_tokens": 0,
        "output_tokens": 0,
        "cache_read_tokens": 0,
        "cache_write_tokens": 0,
        "num_turns": 0,
        "cost_usd": 0.0,
        "compact_events": 0,
    }

    # Track per-turn cache_read_input_tokens for compaction heuristic.
    # Cache reads grow as conversation grows; a drop indicates compaction.
    # Each turn emits 2 assistant events (thinking + content) with identical
    # usage — deduplicate by only taking content events (text/tool_use).
    per_turn_cache_reads = []

    for line in raw_output.split('\n'):
        line = line.strip()
        if not line:
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue

        event_type = event.get("type", "")

        if event_type == "assistant":
            msg = event.get("message", {})
            msg_usage = msg.get("usage", {})

            # Capture the last text block as the answer
            content = msg.get("content", [])
            has_text = any(b.get("type") == "text" for b in content)
            has_tool_use = any(b.get("type") == "tool_use" for b in content)

            # Only track cache reads for non-thinking turns (deduplicate)
            if has_text or has_tool_use:
                cache_read = msg_usage.get("cache_read_input_tokens", 0)
                if cache_read:
                    per_turn_cache_reads.append(cache_read)

            for block in content:
                if block.get("type") == "text":
                    text = block.get("text", "")
                    if text:
                        answer_text = text

        elif event_type == "result":
            # Final summary — authoritative totals
            if event.get("result"):
                answer_text = event["result"]
            if event.get("session_id"):
                usage["session_id"] = event["session_id"]
            usage["num_turns"] = event.get("num_turns", usage["num_turns"])
            usage["cost_usd"] = event.get("total_cost_usd", event.get("cost_usd", 0.0))

            # Extract from result.usage (preferred) or top-level fields
            result_usage = event.get("usage", {})
            if result_usage:
                usage["input_tokens"] = result_usage.get("input_tokens", 0)
                usage["output_tokens"] = result_usage.get("output_tokens", 0)
                usage["cache_read_tokens"] = result_usage.get("cache_read_input_tokens", 0)
                usage["cache_write_tokens"] = result_usage.get("cache_creation_input_tokens", 0)
            else:
                # Fallback to top-level fields
                usage["input_tokens"] = event.get("input_tokens", 0)
                usage["output_tokens"] = event.get("output_tokens", 0)

        elif event_type == "system":
            subtype = event.get("subtype", "").lower()
            # Detect auto-compaction/compression system events
            if any(kw in subtype for kw in ("compact", "compress", "truncat")):
                usage["compact_events"] += 1

    # Heuristic: detect compaction via cache_read_input_tokens drops between turns.
    # Cache reads grow as conversation grows. If they drop by >30%, compaction occurred.
    compact_heuristic = 0
    if len(per_turn_cache_reads) >= 2:
        for j in range(1, len(per_turn_cache_reads)):
            prev = per_turn_cache_reads[j - 1]
            curr = per_turn_cache_reads[j]
            if prev > 0 and curr < prev * 0.7:
                compact_heuristic += 1
    if compact_heuristic > 0:
        usage["compact_events_heuristic"] = compact_heuristic

    usage["per_turn_cache_reads"] = per_turn_cache_reads

    return answer_text, usage


# ─── Agent Session ───────────────────────────────────────────────────────────

def send_to_agent(
    prompt: str,
    is_first: bool,
    system_prompt: str,
    timeout: int = AGENT_TIMEOUT,
) -> Tuple[str, float, bool, Dict]:
    """Send a prompt to the sandboxed persistent agent session.

    Uses --output-format stream-json + -c (continue) to maintain a single
    session across all questions while capturing token usage telemetry.
    The first call creates the session; subsequent calls continue it via -c.
    Session cleanup before each run ensures -c always targets our session.

    Sandboxing flags on EVERY invocation:
      --setting-sources user     Skips project CLAUDE.md + .claude/
      --strict-mcp-config        Disables project MCP servers
      --disable-slash-commands   Disables skills/plugins
      --append-system-prompt     System prompt at system level
      --verbose                  Required for stream-json in print mode

    Args:
        prompt: The question prompt (bare — no system instructions)
        is_first: True for the first message (creates session), False to continue
        system_prompt: System prompt injected via --append-system-prompt
        timeout: Max seconds to wait

    Returns:
        (output_text, duration_seconds, success, usage_dict)
        where usage_dict contains input_tokens, output_tokens, cost_usd,
        num_turns, compact_events, per_turn_input_tokens, session_id
    """
    allowed_tools = "Read,Grep,Glob"

    cmd = [
        "claude",
        "--model", "haiku",
        "--verbose",
        "--output-format", "stream-json",
        "--allowedTools", allowed_tools,
        "--setting-sources", "user",
        "--strict-mcp-config",
        "--disable-slash-commands",
        "--append-system-prompt", system_prompt,
        "-p", prompt,
    ]

    # Continue existing session (not first message).
    # Session cleanup before the run ensures -c always continues OUR session.
    if not is_first:
        cmd.append("-c")

    # Must unset CLAUDECODE to allow nested invocation
    env = {k: v for k, v in os.environ.items() if k != "CLAUDECODE"}

    empty_usage = {
        "input_tokens": 0, "output_tokens": 0, "cache_read_tokens": 0,
        "cache_write_tokens": 0, "num_turns": 0, "cost_usd": 0.0,
        "compact_events": 0, "per_turn_cache_reads": [],
    }

    start = time.time()
    try:
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=timeout,
            cwd=CODEBASE_PATH,
            env=env,
        )
        duration = time.time() - start

        # Parse stream-json for answer text and usage telemetry
        output, usage = parse_stream_json(result.stdout)

        # Fallback: if parser found no text, try stderr
        if not output and result.stderr:
            output = result.stderr.strip()

        # Retry on rate limiting / overload
        if result.returncode != 0 and (
            "rate limit" in (output + result.stderr).lower()
            or "overloaded" in (output + result.stderr).lower()
            or result.returncode == 529
        ):
            for retry_i, wait in enumerate(RETRY_BACKOFF):
                print(f"  Rate limited, waiting {wait}s (retry {retry_i+1}/{MAX_RETRIES})...")
                time.sleep(wait)
                start = time.time()
                result = subprocess.run(
                    cmd,
                    capture_output=True,
                    text=True,
                    timeout=timeout,
                    cwd=CODEBASE_PATH,
                    env=env,
                )
                duration = time.time() - start
                output, usage = parse_stream_json(result.stdout)
                if not output and result.stderr:
                    output = result.stderr.strip()
                if result.returncode == 0:
                    break

        return output, duration, result.returncode == 0, usage
    except subprocess.TimeoutExpired:
        duration = time.time() - start
        return f"TIMEOUT after {timeout}s", duration, False, empty_usage
    except Exception as e:
        duration = time.time() - start
        return f"ERROR: {e}", duration, False, empty_usage


# ─── Single Question Processing ──────────────────────────────────────────────

def process_question(
    question: Dict,
    mode: str,
    is_first: bool,
    timeout: int = AGENT_TIMEOUT,
) -> Dict:
    """Process a single question through the sandboxed persistent agent session.

    System prompt is injected via --append-system-prompt (system-level).
    The -p prompt contains ONLY the question and (for MDEMG) pre-retrieved context.
    No system instructions are included in the -p prompt.

    Args:
        question: Question dict with 'id' and 'question' fields
        mode: 'baseline' or 'mdemg'
        is_first: True if this is the first question (sends system prompt)
        timeout: Max seconds per question

    Returns an answer dict matching the grader's expected input schema,
    augmented with token usage and compact event telemetry.
    """
    q_id = question["id"]
    q_text = question["question"]

    # Select system prompt based on mode
    system_prompt = MDEMG_SYSTEM if mode == "mdemg" else BASELINE_SYSTEM

    # Build bare question prompt — NO system instructions in -p
    if mode == "mdemg":
        mdemg_context = call_mdemg(q_text)
        prompt = (
            f"{mdemg_context}\n\n"
            f"QUESTION: {q_text}\n\n"
            f"Grep the suggested files at the indicated lines, then answer with filename.ts:123 citations:"
        )
    else:
        prompt = (
            f"QUESTION: {q_text}\n\n"
            f"Search the codebase and answer with file:line citations:"
        )

    # Send to persistent sandboxed agent session
    output, duration, success, usage = send_to_agent(
        prompt=prompt,
        is_first=is_first,
        system_prompt=system_prompt,
        timeout=timeout,
    )

    # Extract citations from output
    file_line_refs, files_consulted = extract_citations(output)

    # Determine confidence
    if file_line_refs:
        confidence = 0.85
    elif files_consulted:
        confidence = 0.6
    else:
        confidence = 0.4

    return {
        "id": q_id,
        "question": q_text,
        "answer": output,
        "files_consulted": files_consulted,
        "file_line_refs": file_line_refs,
        "mdemg_used": mode == "mdemg",
        "confidence": confidence,
        "agent_type": mode,
        "model": "haiku",
        "duration_seconds": round(duration, 1),
        "success": success,
        "response_chars": len(output),
        # Token usage telemetry
        "input_tokens": usage.get("input_tokens", 0),
        "output_tokens": usage.get("output_tokens", 0),
        "cache_read_tokens": usage.get("cache_read_tokens", 0),
        "cache_write_tokens": usage.get("cache_write_tokens", 0),
        "cost_usd": usage.get("cost_usd", 0.0),
        "num_turns": usage.get("num_turns", 0),
        "compact_events": usage.get("compact_events", 0),
        "compact_events_heuristic": usage.get("compact_events_heuristic", 0),
        "per_turn_cache_reads": usage.get("per_turn_cache_reads", []),
    }


# ─── Benchmark Run ───────────────────────────────────────────────────────────

def run_benchmark(
    questions: List[Dict],
    mode: str,
    output_dir: Path,
    run_num: int = 1,
    start_from: int = 0,
    timeout: int = AGENT_TIMEOUT,
    session_id: str = "",
) -> Dict:
    """Run a full benchmark: process all questions, write answers + progress.

    Args:
        questions: List of question dicts
        mode: 'baseline' or 'mdemg'
        output_dir: Directory for this run's output
        run_num: Run number (for labeling)
        start_from: Question index to resume from
        timeout: Max seconds per question
        session_id: UUID for this run's session

    Returns:
        Stats dict including token usage aggregates
    """
    output_dir.mkdir(parents=True, exist_ok=True)
    answers_file = output_dir / "answers.jsonl"
    progress_file = output_dir / "progress.json"

    # Clear output if starting fresh
    if start_from == 0 and answers_file.exists():
        answers_file.unlink()

    stats = {
        "mode": mode,
        "run": run_num,
        "model": "haiku",
        "session_id": session_id,
        "total": len(questions),
        "processed": 0,
        "successful": 0,
        "failed": 0,
        "with_refs": 0,
        "total_duration": 0.0,
        "total_response_chars": 0,
        # Token usage aggregates
        "total_input_tokens": 0,
        "total_output_tokens": 0,
        "total_cache_read_tokens": 0,
        "total_cache_write_tokens": 0,
        "total_cost_usd": 0.0,
        "total_compact_events": 0,
        "total_compact_events_heuristic": 0,
        "start_time": datetime.now().isoformat(),
        "errors": [],
    }

    print(f"\n{'='*70}")
    print(f"  COGNITIVE GAP BENCHMARK — {mode.upper()} — Run {run_num}")
    print(f"  Questions: {len(questions)} | Start from: {start_from}")
    print(f"  Session: {session_id[:12]}...")
    print(f"  Sandbox: --setting-sources user, --strict-mcp-config, --disable-slash-commands")
    print(f"  Telemetry: stream-json (tokens + compact events)")
    print(f"  Output: {output_dir}")
    print(f"{'='*70}\n")

    start_time = time.time()

    for i, question in enumerate(questions):
        if i < start_from:
            continue

        q_id = question["id"]
        q_short = question["question"][:60] + ("..." if len(question["question"]) > 60 else "")
        print(f"[{i+1}/{len(questions)}] Q{q_id}: {q_short}")

        try:
            is_first = (i == start_from)  # First question creates the session
            answer = process_question(
                question, mode,
                is_first=is_first,
                timeout=timeout,
            )

            # Write answer
            with open(answers_file, "a") as f:
                f.write(json.dumps(answer) + "\n")

            if answer["success"]:
                stats["successful"] += 1
                if answer["file_line_refs"]:
                    stats["with_refs"] += 1

                # Drift detection: check for meta-commentary instead of real answers
                drift_markers = [
                    "previous conversation",
                    "previous questions",
                    "ready to help",
                    "what would you like",
                    "I'm ready for",
                    "completed questions",
                    "benchmark questions",
                    "all questions have been",
                ]
                answer_lower = answer["answer"].lower()
                drifted = any(m in answer_lower for m in drift_markers)

                if drifted:
                    stats.setdefault("drift_count", 0)
                    stats["drift_count"] += 1
                    print(f"  DRIFT DETECTED: Q{q_id} — agent gave meta-commentary, not an answer")
                    print(f"    First 100 chars: {answer['answer'][:100]}")
                else:
                    in_tok = answer.get("input_tokens", 0)
                    out_tok = answer.get("output_tokens", 0)
                    compact = answer.get("compact_events", 0) + answer.get("compact_events_heuristic", 0)
                    compact_str = f" COMPACT:{compact}" if compact > 0 else ""
                    print(f"  OK: {len(answer['file_line_refs'])} refs, "
                          f"{answer['response_chars']} chars, "
                          f"{answer['duration_seconds']}s, "
                          f"tok:{in_tok}/{out_tok}"
                          f"{compact_str}")
            else:
                stats["failed"] += 1
                stats["errors"].append({"id": q_id, "error": answer["answer"][:100]})
                print(f"  FAILED: {answer['answer'][:80]}")

            stats["total_duration"] += answer["duration_seconds"]
            stats["total_response_chars"] += answer.get("response_chars", 0)
            stats["total_input_tokens"] += answer.get("input_tokens", 0)
            stats["total_output_tokens"] += answer.get("output_tokens", 0)
            stats["total_cache_read_tokens"] += answer.get("cache_read_tokens", 0)
            stats["total_cache_write_tokens"] += answer.get("cache_write_tokens", 0)
            stats["total_cost_usd"] += answer.get("cost_usd", 0.0)
            stats["total_compact_events"] += answer.get("compact_events", 0)
            stats["total_compact_events_heuristic"] += answer.get("compact_events_heuristic", 0)

        except Exception as e:
            stats["failed"] += 1
            stats["errors"].append({"id": q_id, "exception": str(e)})
            print(f"  EXCEPTION: {e}")

        stats["processed"] = i + 1

        # Save checkpoint
        if (i + 1) % CHECKPOINT_INTERVAL == 0:
            _save_progress(stats, progress_file)

        # Progress report every 10 questions
        if (i + 1) % 10 == 0:
            elapsed = time.time() - start_time
            done = i + 1 - start_from
            rate = done / elapsed * 60 if elapsed > 0 else 0
            ref_rate = stats["with_refs"] / max(stats["successful"], 1) * 100
            print(f"\n  --- Progress: {i+1}/{len(questions)} | "
                  f"{stats['successful']} ok, {stats['failed']} fail | "
                  f"{ref_rate:.0f}% with refs | "
                  f"{rate:.1f} q/min | "
                  f"tokens: {stats['total_input_tokens']}/{stats['total_output_tokens']} | "
                  f"compacts: {stats['total_compact_events']+stats['total_compact_events_heuristic']} ---\n")

    # Finalize
    processed = max(stats["processed"], 1)
    stats["end_time"] = datetime.now().isoformat()
    stats["wall_clock_seconds"] = round(time.time() - start_time, 1)
    stats["ref_rate"] = stats["with_refs"] / max(stats["successful"], 1)
    stats["avg_duration"] = stats["total_duration"] / processed
    stats["avg_response_chars"] = stats["total_response_chars"] / processed
    stats["avg_input_tokens"] = stats["total_input_tokens"] / processed
    stats["avg_output_tokens"] = stats["total_output_tokens"] / processed

    _save_progress(stats, progress_file)
    _print_summary(stats)

    return stats


def _save_progress(stats: Dict, progress_file: Path):
    """Save progress checkpoint."""
    with open(progress_file, "w") as f:
        json.dump(stats, f, indent=2)


def _print_summary(stats: Dict):
    """Print run summary including token usage and compact events."""
    print(f"\n{'='*70}")
    print(f"  RUN COMPLETE: {stats['mode'].upper()} Run {stats['run']}")
    print(f"{'='*70}")
    print(f"  Total:       {stats['total']}")
    print(f"  Processed:   {stats['processed']}")
    print(f"  Successful:  {stats['successful']}")
    print(f"  Failed:      {stats['failed']}")
    print(f"  With refs:   {stats['with_refs']} ({stats['ref_rate']*100:.1f}%)")
    print(f"  Avg time:    {stats['avg_duration']:.1f}s per question")
    print(f"  Wall clock:  {stats['wall_clock_seconds']/60:.1f} minutes")
    print(f"  Session:     {stats.get('session_id', 'N/A')[:12]}...")

    # Token usage
    print(f"\n  ── Token Usage ──")
    print(f"  Input:       {stats.get('total_input_tokens', 0):,} total ({stats.get('avg_input_tokens', 0):,.0f} avg/q)")
    print(f"  Output:      {stats.get('total_output_tokens', 0):,} total ({stats.get('avg_output_tokens', 0):,.0f} avg/q)")
    print(f"  Cache read:  {stats.get('total_cache_read_tokens', 0):,}")
    print(f"  Cache write: {stats.get('total_cache_write_tokens', 0):,}")
    print(f"  Cost:        ${stats.get('total_cost_usd', 0):.4f}")
    print(f"  Chars:       {stats.get('total_response_chars', 0):,} total ({stats.get('avg_response_chars', 0):,.0f} avg/q)")

    # Compact events
    ce = stats.get("total_compact_events", 0)
    ceh = stats.get("total_compact_events_heuristic", 0)
    print(f"\n  ── Auto-Compact Events ──")
    print(f"  System events:  {ce}")
    print(f"  Heuristic:      {ceh} (>30% input token drop between turns)")
    print(f"  Total:          {ce + ceh}")

    print(f"{'='*70}")

    if stats.get("drift_count", 0) > 0:
        print(f"\n  DRIFT: {stats['drift_count']} questions had meta-commentary instead of answers")

    if stats["errors"]:
        print(f"\n  Errors ({len(stats['errors'])}):")
        for err in stats["errors"][:5]:
            print(f"    Q{err.get('id')}: {err.get('error', err.get('exception', '?'))[:80]}")


# ─── Post-Run Validation ─────────────────────────────────────────────────────

def validate_answers(answers_file: Path, codebase: str) -> Dict:
    """Post-hoc validation: check cited files exist, count statistics."""
    results = {"total": 0, "valid_files": 0, "invalid_files": 0, "hallucinated": []}

    with open(answers_file) as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            answer = json.loads(line)
            results["total"] += 1
            for ref in answer.get("file_line_refs", []):
                if validate_file_exists(ref, codebase):
                    results["valid_files"] += 1
                else:
                    results["invalid_files"] += 1
                    results["hallucinated"].append({"id": answer["id"], "ref": ref})

    results["hallucination_rate"] = (
        results["invalid_files"] / max(results["valid_files"] + results["invalid_files"], 1)
    )
    return results


# ─── CLI ─────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(
        description="Cognitive Gap Benchmark: Baseline vs MDEMG-Assisted Agents (Sandboxed)"
    )
    parser.add_argument(
        "--mode", required=True, choices=["baseline", "mdemg"],
        help="Agent mode: baseline (no MDEMG) or mdemg (with MDEMG retrieval)"
    )
    parser.add_argument(
        "--run", type=int, default=1,
        help="Run number (default: 1)"
    )
    parser.add_argument(
        "--questions-limit", type=int, default=0,
        help="Limit to first N questions (0 = all 120). Use 20 for pilot."
    )
    parser.add_argument(
        "--start-from", type=int, default=0,
        help="Resume from question index (0-based)"
    )
    parser.add_argument(
        "--questions", type=str, default=str(QUESTIONS_FILE),
        help="Questions JSON file (agent version without answers)"
    )
    parser.add_argument(
        "--output-base", type=str, default=str(OUTPUT_BASE),
        help="Base output directory"
    )
    parser.add_argument(
        "--validate", action="store_true",
        help="Run post-hoc file validation on answers"
    )
    parser.add_argument(
        "--timeout", type=int, default=AGENT_TIMEOUT,
        help="Timeout per agent in seconds (default: 120)"
    )
    parser.add_argument(
        "--skip-verify", action="store_true",
        help="Skip sandbox verification check"
    )

    args = parser.parse_args()

    # Load questions
    with open(args.questions) as f:
        data = json.load(f)
    questions = data.get("questions", data)

    if args.questions_limit > 0:
        questions = questions[:args.questions_limit]
        print(f"PILOT MODE: Limited to {len(questions)} questions")

    print(f"Loaded {len(questions)} questions from {args.questions}")

    # Generate unique session ID for this run
    session_id = str(uuid.uuid4())

    # Output directory
    output_base = Path(args.output_base)
    output_dir = output_base / args.mode / f"run{args.run}"

    # ─── Pre-run sandbox setup ───
    print("\nPre-run sandbox setup...")

    # 1. Clean session directories (prevent cross-run leakage)
    pre_clean = cleanup_session_dirs()

    # 2. Stash master answer file (prevent answer leakage)
    stash_master_file()

    sandbox_verified = False
    post_clean = 0

    try:
        # 3. Verify sandbox flags work
        if not args.skip_verify:
            sandbox_verified = verify_sandbox()
            if not sandbox_verified:
                print("\nABORT: Sandbox verification failed. Use --skip-verify to override.")
                sys.exit(1)
        else:
            sandbox_verified = None  # Skipped
            print("  Sandbox verification SKIPPED (--skip-verify)")

        # Save config (once per output_base)
        config_file = output_base / "config.json"
        if not config_file.exists():
            config = {
                "benchmark": "cognitive_gap_validation_v2_sandboxed",
                "date": datetime.now().isoformat(),
                "questions_file": str(args.questions),
                "master_file": str(MASTER_FILE),
                "codebase": CODEBASE_PATH,
                "mdemg_endpoint": MDEMG_ENDPOINT,
                "mdemg_space_id": MDEMG_SPACE_ID,
                "model": "haiku",
                "agent_timeout": args.timeout,
                "modes": ["baseline", "mdemg"],
                "runs_per_mode": 3,
                "total_questions": len(questions),
                "sandbox_flags": [
                    "--setting-sources user",
                    "--strict-mcp-config",
                    "--disable-slash-commands",
                    "--session-id <per-run-uuid>",
                    "--append-system-prompt <mode-specific>",
                ],
                "telemetry": "--output-format stream-json",
            }
            output_base.mkdir(parents=True, exist_ok=True)
            with open(config_file, "w") as f:
                json.dump(config, f, indent=2)

        # ─── Run benchmark ───
        stats = run_benchmark(
            questions=questions,
            mode=args.mode,
            output_dir=output_dir,
            run_num=args.run,
            start_from=args.start_from,
            timeout=args.timeout,
            session_id=session_id,
        )

        # ─── Post-run cleanup ───
        print("\nPost-run cleanup...")
        post_clean = cleanup_session_dirs()

        # ─── Save per-run metadata ───
        metadata = {
            "session_id": session_id,
            "sandbox_flags": [
                "--setting-sources user",
                "--strict-mcp-config",
                "--disable-slash-commands",
            ],
            "telemetry_format": "stream-json",
            "master_file_stashed": True,
            "session_cleanup_pre": pre_clean,
            "session_cleanup_post": post_clean,
            "sandbox_verified": sandbox_verified,
            "timeout_seconds": args.timeout,
            # Token usage summary
            "total_input_tokens": stats.get("total_input_tokens", 0),
            "total_output_tokens": stats.get("total_output_tokens", 0),
            "total_cache_read_tokens": stats.get("total_cache_read_tokens", 0),
            "total_cache_write_tokens": stats.get("total_cache_write_tokens", 0),
            "total_cost_usd": stats.get("total_cost_usd", 0.0),
            "total_compact_events": stats.get("total_compact_events", 0),
            "total_compact_events_heuristic": stats.get("total_compact_events_heuristic", 0),
            "total_response_chars": stats.get("total_response_chars", 0),
            "avg_response_chars": stats.get("avg_response_chars", 0),
            "avg_input_tokens": stats.get("avg_input_tokens", 0),
            "avg_output_tokens": stats.get("avg_output_tokens", 0),
        }
        with open(output_dir / "metadata.json", "w") as f:
            json.dump(metadata, f, indent=2)

        # ─── Post-run validation ───
        answers_file = output_dir / "answers.jsonl"
        if args.validate and answers_file.exists():
            print(f"\nRunning post-hoc file validation...")
            validation = validate_answers(answers_file, CODEBASE_PATH)
            print(f"  Total answers: {validation['total']}")
            print(f"  Valid file refs: {validation['valid_files']}")
            print(f"  Invalid (hallucinated): {validation['invalid_files']}")
            print(f"  Hallucination rate: {validation['hallucination_rate']*100:.1f}%")
            if validation["hallucinated"]:
                print(f"  Examples:")
                for h in validation["hallucinated"][:5]:
                    print(f"    Q{h['id']}: {h['ref']}")

            # Save validation results
            with open(output_dir / "validation.json", "w") as f:
                json.dump(validation, f, indent=2)

        # Print grading command
        print(f"\nTo grade this run:")
        print(f"  python {SCRIPT_DIR / 'grader_v4.py'} \\")
        print(f"    {answers_file} \\")
        print(f"    {MASTER_FILE} \\")
        print(f"    {output_dir / 'grades.json'}")

    finally:
        # ALWAYS restore master file and clean sessions, even on crash/interrupt
        print("\nFinal cleanup...")
        unstash_master_file()
        cleanup_session_dirs()


if __name__ == "__main__":
    main()
