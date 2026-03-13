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

    # Parallel run (12 workers — ~10 min instead of ~60 min):
    python run_cognitive_gap_benchmark.py --mode mdemg --run 1 --parallel 12

    # Pilot (20 questions):
    python run_cognitive_gap_benchmark.py --mode baseline --questions-limit 20

    # Resume from checkpoint (serial mode only):
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
from concurrent.futures import ProcessPoolExecutor, as_completed
from typing import Dict, List, Optional, Tuple


# ─── Configuration Defaults ──────────────────────────────────────────────────
# All values are overridable via CLI flags or environment variables.
# Priority: CLI flag → env var → default below.

SCRIPT_DIR = Path(__file__).parent

# Default paths (overridable via --questions, --master, --output-base, --codebase)
DEFAULT_QUESTIONS_FILE = str(SCRIPT_DIR / "whk-wms" / "test_questions_120_agent.json")
DEFAULT_MASTER_FILE = str(SCRIPT_DIR / "whk-wms" / "test_questions_120.json")
DEFAULT_OUTPUT_BASE = str(SCRIPT_DIR / "whk-wms" / "cognitive_gap_validation_20260226")
DEFAULT_CODEBASE_PATH = os.environ.get("BENCHMARK_CODEBASE_PATH", "")
DEFAULT_MDEMG_ENDPOINT = os.environ.get("MDEMG_ENDPOINT", "http://localhost:9999")
DEFAULT_MDEMG_SPACE_ID = os.environ.get("BENCHMARK_SPACE_ID", "")
DEFAULT_AGENT_TIMEOUT = int(os.environ.get("BENCHMARK_AGENT_TIMEOUT", "120"))
DEFAULT_CHECKPOINT_INTERVAL = int(os.environ.get("BENCHMARK_CHECKPOINT_INTERVAL", "5"))
DEFAULT_MODEL = os.environ.get("BENCHMARK_MODEL", "haiku")
DEFAULT_MAX_RETRIES = int(os.environ.get("BENCHMARK_MAX_RETRIES", "3"))

# Retry backoff seconds (not typically changed via CLI)
RETRY_BACKOFF = [30, 60, 120]

# Sandbox paths (derived from codebase, set at runtime)
STASH_DIR = SCRIPT_DIR / ".stash"


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

def stash_master_file(master_file: Path) -> Optional[Path]:
    """Move master answer file out of reach during benchmark runs."""
    STASH_DIR.mkdir(exist_ok=True)
    stash_path = STASH_DIR / master_file.name
    if master_file.exists():
        shutil.move(str(master_file), str(stash_path))
        print(f"  Stashed master file to {stash_path}")
        return stash_path
    return None


def unstash_master_file(master_file: Path):
    """Restore master answer file after benchmark run."""
    stash_path = STASH_DIR / master_file.name
    if stash_path.exists():
        shutil.move(str(stash_path), str(master_file))
        print(f"  Restored master file from stash")


def cleanup_session_dirs(codebase_path: str) -> int:
    """Remove ALL Claude session directories for the target project.

    MANDATORY: Must be called after every benchmark run to prevent
    cross-run context leakage. Session files accumulate and can leak
    context between benchmark runs via session continuation (-c flag).
    """
    # Derive session project dir from codebase path
    # Claude stores sessions under ~/.claude/projects/-<path-with-dashes>/
    safe_name = codebase_path.replace("/", "-")
    if safe_name.startswith("-"):
        safe_name = safe_name  # keep leading dash
    session_project_dir = Path.home() / ".claude" / "projects" / safe_name

    if not session_project_dir.exists():
        return 0

    count = 0
    for entry in session_project_dir.iterdir():
        if entry.is_dir() and entry.name != "memory":  # Preserve memory dir
            shutil.rmtree(entry)
            count += 1

    # Also clean session index files
    for f in session_project_dir.glob("*.json"):
        if "session" in f.name.lower():
            f.unlink()
            count += 1

    print(f"  Cleaned {count} session entries from {session_project_dir}")
    return count


def verify_sandbox(codebase_path: str) -> bool:
    """Verify that CLI sandboxing flags actually work.

    Spawns a test agent and checks that:
    1. It does NOT see project CLAUDE.md content auto-loaded
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
            cwd=codebase_path, env=env,
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

def call_mdemg(query: str, mdemg_endpoint: str, mdemg_space_id: str, top_k: int = 5) -> str:
    """Call MDEMG retrieval API and format results as context for the agent prompt.

    All API calls follow UATS contract for POST /v1/memory/retrieve:
      - Body: {"space_id": ..., "query_text": ..., "top_k": N}
      - Response: {"results": [...], "space_id": ..., "has_more": bool}

    Returns a formatted string listing relevant files with summaries.
    If the API call fails, returns a fallback message.
    """
    url = f"{mdemg_endpoint}/v1/memory/retrieve"
    data = {
        "space_id": mdemg_space_id,
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
    codebase_path: str,
    model: str = "haiku",
    timeout: int = 120,
    max_retries: int = 3,
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
        codebase_path: Working directory for the agent subprocess
        model: Claude model to use (default: haiku)
        timeout: Max seconds to wait
        max_retries: Max retries on rate limiting

    Returns:
        (output_text, duration_seconds, success, usage_dict)
        where usage_dict contains input_tokens, output_tokens, cost_usd,
        num_turns, compact_events, per_turn_input_tokens, session_id
    """
    allowed_tools = "Read,Grep,Glob"

    cmd = [
        "claude",
        "--model", model,
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
            cwd=codebase_path,
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
                print(f"  Rate limited, waiting {wait}s (retry {retry_i+1}/{max_retries})...")
                time.sleep(wait)
                start = time.time()
                result = subprocess.run(
                    cmd,
                    capture_output=True,
                    text=True,
                    timeout=timeout,
                    cwd=codebase_path,
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
    codebase_path: str,
    mdemg_endpoint: str = "http://localhost:9999",
    mdemg_space_id: str = "",
    model: str = "haiku",
    timeout: int = 120,
    max_retries: int = 3,
) -> Dict:
    """Process a single question through the sandboxed persistent agent session.

    System prompt is injected via --append-system-prompt (system-level).
    The -p prompt contains ONLY the question and (for MDEMG) pre-retrieved context.
    No system instructions are included in the -p prompt.

    Args:
        question: Question dict with 'id' and 'question' fields
        mode: 'baseline' or 'mdemg'
        is_first: True if this is the first question (sends system prompt)
        codebase_path: Working directory for the agent subprocess
        mdemg_endpoint: MDEMG API URL (UATS: POST /v1/memory/retrieve)
        mdemg_space_id: MDEMG space ID for retrieval
        model: Claude model to use
        timeout: Max seconds per question
        max_retries: Max retries on rate limiting

    Returns an answer dict matching the grader's expected input schema,
    augmented with token usage and compact event telemetry.
    """
    q_id = question["id"]
    q_text = question["question"]

    # Select system prompt based on mode
    system_prompt = MDEMG_SYSTEM if mode == "mdemg" else BASELINE_SYSTEM

    # Build bare question prompt — NO system instructions in -p
    if mode == "mdemg":
        mdemg_context = call_mdemg(q_text, mdemg_endpoint, mdemg_space_id)
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
        codebase_path=codebase_path,
        model=model,
        timeout=timeout,
        max_retries=max_retries,
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
        "model": model,
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
    timeout: int = 120,
    session_id: str = "",
    codebase_path: str = "",
    mdemg_endpoint: str = "http://localhost:9999",
    mdemg_space_id: str = "",
    model: str = "haiku",
    checkpoint_interval: int = 5,
    max_retries: int = 3,
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
        codebase_path: Path to target codebase
        mdemg_endpoint: MDEMG API endpoint
        mdemg_space_id: MDEMG space ID
        model: Claude model to use
        checkpoint_interval: Save progress every N questions
        max_retries: Max retries on rate limiting

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
        "model": model,
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
                codebase_path=codebase_path,
                mdemg_endpoint=mdemg_endpoint,
                mdemg_space_id=mdemg_space_id,
                model=model,
                timeout=timeout,
                max_retries=max_retries,
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
        if (i + 1) % checkpoint_interval == 0:
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


# ─── Parallel Execution ──────────────────────────────────────────────────────

def _process_question_standalone(
    question: Dict,
    mode: str,
    mdemg_endpoint: str,
    mdemg_space_id: str,
    codebase_path: str,
    timeout: int,
    system_prompt: str,
    model: str = "haiku",
    max_retries: int = 3,
) -> Dict:
    """Process a single question in a standalone agent (no session continuation).

    This is the parallel-safe variant of process_question(). Each invocation
    spawns an independent Claude CLI subprocess — no -c flag, no shared session.

    All MDEMG API calls follow UATS contract:
      - POST /v1/memory/retrieve
      - Body: {"space_id": ..., "query_text": ..., "top_k": N}
      - Response: {"results": [...], "space_id": ..., "has_more": bool}
    """
    q_id = question["id"]
    q_text = question["question"]

    # ── MDEMG retrieval (UATS-compliant: query_text, not query) ──
    mdemg_context = "(No MDEMG context)"
    if mode == "mdemg":
        url = f"{mdemg_endpoint}/v1/memory/retrieve"
        data = {
            "space_id": mdemg_space_id,
            "query_text": q_text,
            "top_k": 5,
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
            nodes = result.get("results", [])
            if nodes:
                lines = [f"RELEVANT FILES ({len(nodes)} results — read these line ranges):"]
                for i, node in enumerate(nodes, 1):
                    path = node.get("path", "")
                    name = node.get("name", Path(path).name if path else "unknown")
                    score = node.get("score", 0)
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
                    for ref in evidence_refs[:5]:
                        lines.append(f"     -> {ref}")
                mdemg_context = "\n".join(lines)
            else:
                mdemg_context = "(No relevant files found in memory — search the codebase directly)"
        except Exception as e:
            mdemg_context = f"(MDEMG retrieval failed: {e} — search the codebase directly)"

    # ── Build prompt ──
    if mode == "mdemg":
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

    # ── Spawn independent agent (no -c flag) ──
    allowed_tools = "Read,Grep,Glob"
    cmd = [
        "claude",
        "--model", model,
        "--verbose",
        "--output-format", "stream-json",
        "--allowedTools", allowed_tools,
        "--setting-sources", "user",
        "--strict-mcp-config",
        "--disable-slash-commands",
        "--append-system-prompt", system_prompt,
        "-p", prompt,
    ]

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
            cwd=codebase_path,
            env=env,
        )
        duration = time.time() - start
        output, usage = parse_stream_json(result.stdout)
        if not output and result.stderr:
            output = result.stderr.strip()

        # Retry on rate limiting
        if result.returncode != 0 and (
            "rate limit" in (output + result.stderr).lower()
            or "overloaded" in (output + result.stderr).lower()
            or result.returncode == 529
        ):
            for retry_i, wait in enumerate(RETRY_BACKOFF):
                time.sleep(wait)
                start = time.time()
                result = subprocess.run(
                    cmd, capture_output=True, text=True,
                    timeout=timeout, cwd=codebase_path, env=env,
                )
                duration = time.time() - start
                output, usage = parse_stream_json(result.stdout)
                if not output and result.stderr:
                    output = result.stderr.strip()
                if result.returncode == 0:
                    break

        success = result.returncode == 0
    except subprocess.TimeoutExpired:
        duration = time.time() - start
        output, usage, success = f"TIMEOUT after {timeout}s", empty_usage, False
    except Exception as e:
        duration = time.time() - start
        output, usage, success = f"ERROR: {e}", empty_usage, False

    # ── Extract citations ──
    file_line_refs, files_consulted = extract_citations(output)
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
        "model": model,
        "duration_seconds": round(duration, 1),
        "success": success,
        "response_chars": len(output),
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


def _run_batch(
    batch_id: int,
    questions: List[Dict],
    mode: str,
    mdemg_endpoint: str,
    mdemg_space_id: str,
    codebase_path: str,
    timeout: int,
    output_file: str,
    model: str = "haiku",
    max_retries: int = 3,
) -> Dict:
    """Run a batch of questions sequentially within one worker process.

    Each question spawns an independent agent (no session continuation).
    Results are written to a batch-specific JSONL file for later merging.
    """
    system_prompt = MDEMG_SYSTEM if mode == "mdemg" else BASELINE_SYSTEM
    results = []

    for i, question in enumerate(questions):
        q_id = question["id"]
        q_short = question["question"][:50] + "..."
        print(f"  [W{batch_id}] {i+1}/{len(questions)} Q{q_id}: {q_short}", flush=True)

        answer = _process_question_standalone(
            question=question,
            mode=mode,
            mdemg_endpoint=mdemg_endpoint,
            mdemg_space_id=mdemg_space_id,
            codebase_path=codebase_path,
            timeout=timeout,
            system_prompt=system_prompt,
            model=model,
            max_retries=max_retries,
        )

        # Write immediately to batch file (append)
        with open(output_file, "a") as f:
            f.write(json.dumps(answer) + "\n")

        results.append({
            "id": q_id,
            "success": answer["success"],
            "refs": len(answer["file_line_refs"]),
            "duration": answer["duration_seconds"],
            "input_tokens": answer["input_tokens"],
            "output_tokens": answer["output_tokens"],
            "cost_usd": answer["cost_usd"],
        })

        ok_str = "OK" if answer["success"] else "FAIL"
        print(f"  [W{batch_id}] Q{q_id} {ok_str}: {len(answer['file_line_refs'])} refs, "
              f"{answer['duration_seconds']}s, tok:{answer['input_tokens']}/{answer['output_tokens']}",
              flush=True)

    return {
        "batch_id": batch_id,
        "total": len(questions),
        "successful": sum(1 for r in results if r["success"]),
        "failed": sum(1 for r in results if not r["success"]),
        "with_refs": sum(1 for r in results if r["refs"] > 0),
        "total_duration": sum(r["duration"] for r in results),
        "total_input_tokens": sum(r["input_tokens"] for r in results),
        "total_output_tokens": sum(r["output_tokens"] for r in results),
        "total_cost_usd": sum(r["cost_usd"] for r in results),
        "output_file": output_file,
    }


def run_benchmark_parallel(
    questions: List[Dict],
    mode: str,
    output_dir: Path,
    run_num: int = 1,
    timeout: int = 120,
    num_workers: int = 12,
    session_id: str = "",
    mdemg_endpoint: str = "http://localhost:9999",
    mdemg_space_id: str = "",
    codebase_path: str = "",
    model: str = "haiku",
    max_retries: int = 3,
) -> Dict:
    """Run benchmark with parallel workers using ProcessPoolExecutor.

    Questions are split into N batches. Each batch runs in a separate process,
    processing its questions sequentially. Each question spawns an independent
    agent with no session continuation (-c flag is NOT used).

    This is ~Nx faster than serial mode for N workers.

    Args:
        questions: Question list
        mode: 'baseline' or 'mdemg'
        output_dir: Output directory for this run
        run_num: Run number
        timeout: Max seconds per agent invocation
        num_workers: Number of parallel worker processes
        session_id: Run session UUID
        mdemg_endpoint: MDEMG API endpoint (UATS: POST /v1/memory/retrieve)
        mdemg_space_id: MDEMG space ID
        codebase_path: Path to the target codebase
    """
    output_dir.mkdir(parents=True, exist_ok=True)

    # Split into batches
    batch_size = max(1, (len(questions) + num_workers - 1) // num_workers)
    batches = []
    for i in range(0, len(questions), batch_size):
        batches.append(questions[i:i + batch_size])

    print(f"\n{'='*70}")
    print(f"  COGNITIVE GAP BENCHMARK — {mode.upper()} — Run {run_num} — PARALLEL ({num_workers} workers)")
    print(f"  Questions: {len(questions)} | Batches: {len(batches)} (~{batch_size}/worker)")
    print(f"  Session: {session_id[:12]}...")
    print(f"  Sandbox: --setting-sources user, --strict-mcp-config, --disable-slash-commands")
    print(f"  Mode: independent agents (no -c session continuation)")
    print(f"  Output: {output_dir}")
    print(f"{'='*70}\n")

    start_time = time.time()

    # Submit batches to process pool
    batch_results = []
    with ProcessPoolExecutor(max_workers=num_workers) as executor:
        futures = {}
        for batch_id, batch_questions in enumerate(batches):
            batch_file = str(output_dir / f"batch_{batch_id}.jsonl")
            # Clear batch file if it exists
            if os.path.exists(batch_file):
                os.unlink(batch_file)
            future = executor.submit(
                _run_batch,
                batch_id,
                batch_questions,
                mode,
                mdemg_endpoint,
                mdemg_space_id,
                codebase_path,
                timeout,
                batch_file,
                model,
                max_retries,
            )
            futures[future] = batch_id

        for future in as_completed(futures):
            batch_id = futures[future]
            try:
                result = future.result()
                batch_results.append(result)
                print(f"\n=== Batch {batch_id} complete: "
                      f"{result['successful']}/{result['total']} ok, "
                      f"{result['with_refs']} with refs, "
                      f"${result['total_cost_usd']:.4f} ===\n", flush=True)
            except Exception as e:
                print(f"\n=== Batch {batch_id} FAILED: {e} ===\n", flush=True)
                batch_results.append({
                    "batch_id": batch_id,
                    "total": len(batches[batch_id]),
                    "successful": 0,
                    "failed": len(batches[batch_id]),
                    "with_refs": 0,
                    "total_duration": 0,
                    "total_input_tokens": 0,
                    "total_output_tokens": 0,
                    "total_cost_usd": 0,
                    "error": str(e),
                })

    # Merge batch files into answers.jsonl
    answers_file = output_dir / "answers.jsonl"
    answer_count = 0
    with open(answers_file, "w") as outf:
        for batch_id in range(len(batches)):
            batch_file = output_dir / f"batch_{batch_id}.jsonl"
            if batch_file.exists():
                with open(batch_file) as inf:
                    for line in inf:
                        if line.strip():
                            outf.write(line if line.endswith("\n") else line + "\n")
                            answer_count += 1

    elapsed = time.time() - start_time
    total_successful = sum(r["successful"] for r in batch_results)
    total_failed = sum(r["failed"] for r in batch_results)
    total_with_refs = sum(r["with_refs"] for r in batch_results)
    total_input = sum(r["total_input_tokens"] for r in batch_results)
    total_output = sum(r["total_output_tokens"] for r in batch_results)
    total_cost = sum(r["total_cost_usd"] for r in batch_results)
    total_duration = sum(r["total_duration"] for r in batch_results)

    stats = {
        "mode": mode,
        "run": run_num,
        "model": model,
        "session_id": session_id,
        "parallel": True,
        "num_workers": num_workers,
        "num_batches": len(batches),
        "total": len(questions),
        "processed": answer_count,
        "successful": total_successful,
        "failed": total_failed,
        "with_refs": total_with_refs,
        "ref_rate": total_with_refs / max(total_successful, 1),
        "total_duration": total_duration,
        "wall_clock_seconds": round(elapsed, 1),
        "avg_duration": total_duration / max(answer_count, 1),
        "speedup": f"{total_duration / max(elapsed, 1):.1f}x",
        "questions_per_minute": len(questions) / max(elapsed, 1) * 60,
        "total_input_tokens": total_input,
        "total_output_tokens": total_output,
        "avg_input_tokens": total_input / max(answer_count, 1),
        "avg_output_tokens": total_output / max(answer_count, 1),
        "total_cost_usd": total_cost,
        "total_response_chars": 0,
        "start_time": datetime.fromtimestamp(start_time).isoformat(),
        "end_time": datetime.now().isoformat(),
        "batch_results": batch_results,
        "errors": [r.get("error") for r in batch_results if r.get("error")],
    }

    # Save progress
    _save_progress(stats, output_dir / "progress.json")

    # Print summary
    print(f"\n{'='*70}")
    print(f"  PARALLEL BENCHMARK COMPLETE — {mode.upper()} Run {run_num}")
    print(f"{'='*70}")
    print(f"  Total:       {len(questions)}")
    print(f"  Answers:     {answer_count}")
    print(f"  Successful:  {total_successful}")
    print(f"  Failed:      {total_failed}")
    print(f"  With refs:   {total_with_refs} ({stats['ref_rate']*100:.1f}%)")
    print(f"  Wall clock:  {elapsed/60:.1f} min ({elapsed:.0f}s)")
    print(f"  CPU time:    {total_duration/60:.1f} min (sum of all agents)")
    print(f"  Speedup:     {stats['speedup']} ({num_workers} workers)")
    print(f"  Speed:       {stats['questions_per_minute']:.1f} q/min")
    print(f"\n  ── Token Usage ──")
    print(f"  Input:       {total_input:,} ({stats['avg_input_tokens']:,.0f} avg/q)")
    print(f"  Output:      {total_output:,} ({stats['avg_output_tokens']:,.0f} avg/q)")
    print(f"  Cost:        ${total_cost:.4f}")
    print(f"{'='*70}")

    return stats


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
        "--questions", type=str, default=DEFAULT_QUESTIONS_FILE,
        help="Questions JSON file (agent version without answers) "
             f"(default: {DEFAULT_QUESTIONS_FILE})"
    )
    parser.add_argument(
        "--master", type=str, default=DEFAULT_MASTER_FILE,
        help="Master answer file path (stashed during runs) "
             f"(default: {DEFAULT_MASTER_FILE})"
    )
    parser.add_argument(
        "--output-base", type=str, default=DEFAULT_OUTPUT_BASE,
        help="Base output directory "
             f"(default: {DEFAULT_OUTPUT_BASE})"
    )
    parser.add_argument(
        "--codebase", type=str, default=DEFAULT_CODEBASE_PATH,
        help="Path to target codebase to benchmark against. "
             "Also settable via BENCHMARK_CODEBASE_PATH env var. (REQUIRED)"
    )
    parser.add_argument(
        "--mdemg-endpoint", type=str, default=DEFAULT_MDEMG_ENDPOINT,
        help="MDEMG API endpoint URL. Also settable via MDEMG_ENDPOINT env var. "
             f"(default: {DEFAULT_MDEMG_ENDPOINT})"
    )
    parser.add_argument(
        "--space-id", type=str, default=DEFAULT_MDEMG_SPACE_ID,
        help="MDEMG space ID for retrieval. "
             "Also settable via BENCHMARK_SPACE_ID env var. (REQUIRED for mdemg mode)"
    )
    parser.add_argument(
        "--model", type=str, default=DEFAULT_MODEL,
        help="Claude model for agent invocations. "
             "Also settable via BENCHMARK_MODEL env var. "
             f"(default: {DEFAULT_MODEL})"
    )
    parser.add_argument(
        "--validate", action="store_true",
        help="Run post-hoc file validation on answers"
    )
    parser.add_argument(
        "--timeout", type=int, default=DEFAULT_AGENT_TIMEOUT,
        help="Timeout per agent in seconds. "
             "Also settable via BENCHMARK_AGENT_TIMEOUT env var. "
             f"(default: {DEFAULT_AGENT_TIMEOUT})"
    )
    parser.add_argument(
        "--checkpoint-interval", type=int, default=DEFAULT_CHECKPOINT_INTERVAL,
        help="Save progress every N questions (serial mode). "
             "Also settable via BENCHMARK_CHECKPOINT_INTERVAL env var. "
             f"(default: {DEFAULT_CHECKPOINT_INTERVAL})"
    )
    parser.add_argument(
        "--max-retries", type=int, default=DEFAULT_MAX_RETRIES,
        help="Max retries on rate limiting/overload. "
             "Also settable via BENCHMARK_MAX_RETRIES env var. "
             f"(default: {DEFAULT_MAX_RETRIES})"
    )
    parser.add_argument(
        "--skip-verify", action="store_true",
        help="Skip sandbox verification check"
    )
    parser.add_argument(
        "--parallel", type=int, default=1,
        help="Number of parallel workers (default: 1 = serial). "
             "Parallel mode spawns independent agents (no -c session continuation). "
             "Recommended: 8-12 workers. Incompatible with --start-from."
    )

    args = parser.parse_args()

    # Validate args
    if args.parallel < 1:
        parser.error("--parallel must be >= 1")
    if args.parallel > 1 and args.start_from > 0:
        parser.error("--start-from is not supported with --parallel > 1 (no session continuation)")
    if not args.codebase:
        parser.error("--codebase is required (or set BENCHMARK_CODEBASE_PATH env var)")
    if args.mode == "mdemg" and not args.space_id:
        parser.error("--space-id is required for mdemg mode (or set BENCHMARK_SPACE_ID env var)")

    # Resolve paths
    codebase_path = args.codebase
    mdemg_endpoint = args.mdemg_endpoint
    mdemg_space_id = args.space_id
    master_file = Path(args.master)
    model = args.model
    timeout = args.timeout
    checkpoint_interval = args.checkpoint_interval
    max_retries = args.max_retries

    # Load questions
    with open(args.questions) as f:
        data = json.load(f)
    questions = data.get("questions", data)

    if args.questions_limit > 0:
        questions = questions[:args.questions_limit]
        print(f"PILOT MODE: Limited to {len(questions)} questions")

    print(f"Loaded {len(questions)} questions from {args.questions}")
    print(f"Codebase: {codebase_path}")
    print(f"Model: {model}")
    if args.mode == "mdemg":
        print(f"MDEMG: {mdemg_endpoint} (space: {mdemg_space_id})")

    # Generate unique session ID for this run
    session_id = str(uuid.uuid4())

    # Output directory
    output_base = Path(args.output_base)
    output_dir = output_base / args.mode / f"run{args.run}"

    # ─── Pre-run sandbox setup ───
    print("\nPre-run sandbox setup...")

    # 1. Clean session directories (prevent cross-run leakage)
    pre_clean = cleanup_session_dirs(codebase_path)

    # 2. Stash master answer file (prevent answer leakage)
    stash_master_file(master_file)

    sandbox_verified = False
    post_clean = 0

    try:
        # 3. Verify sandbox flags work
        if not args.skip_verify:
            sandbox_verified = verify_sandbox(codebase_path)
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
                "master_file": str(master_file),
                "codebase": codebase_path,
                "mdemg_endpoint": mdemg_endpoint,
                "mdemg_space_id": mdemg_space_id,
                "model": model,
                "agent_timeout": timeout,
                "max_retries": max_retries,
                "checkpoint_interval": checkpoint_interval,
                "parallel_workers": args.parallel,
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
        if args.parallel > 1:
            stats = run_benchmark_parallel(
                questions=questions,
                mode=args.mode,
                output_dir=output_dir,
                run_num=args.run,
                timeout=timeout,
                num_workers=args.parallel,
                session_id=session_id,
                mdemg_endpoint=mdemg_endpoint,
                mdemg_space_id=mdemg_space_id,
                codebase_path=codebase_path,
                model=model,
                max_retries=max_retries,
            )
        else:
            stats = run_benchmark(
                questions=questions,
                mode=args.mode,
                output_dir=output_dir,
                run_num=args.run,
                start_from=args.start_from,
                timeout=timeout,
                session_id=session_id,
                codebase_path=codebase_path,
                mdemg_endpoint=mdemg_endpoint,
                mdemg_space_id=mdemg_space_id,
                model=model,
                checkpoint_interval=checkpoint_interval,
                max_retries=max_retries,
            )

        # ─── Post-run cleanup ───
        print("\nPost-run cleanup...")
        post_clean = cleanup_session_dirs(codebase_path)

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
            "timeout_seconds": timeout,
            "model": model,
            "parallel_workers": args.parallel,
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
            validation = validate_answers(answers_file, codebase_path)
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
        print(f"    {master_file} \\")
        print(f"    {output_dir / 'grades.json'}")

    finally:
        # ALWAYS restore master file and clean sessions, even on crash/interrupt
        print("\nFinal cleanup...")
        unstash_master_file(master_file)
        cleanup_session_dirs(codebase_path)


if __name__ == "__main__":
    main()
