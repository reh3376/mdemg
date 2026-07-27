#!/usr/bin/env python3
# MDEMG hook — managed by mdemg hooks install
"""
Hook: PostToolUse — auto-capture observations after significant tool completions.
Fires-and-forgets a CMS observe call for noteworthy events.
"""

import json
import os
import subprocess
import sys
import time


def _resolve_mdemg_url() -> str:
    """Discover MDEMG URL: env > .mdemg.port > .env MDEMG_PORT > 9999."""
    url = os.environ.get("MDEMG_URL")
    if url:
        return url
    port = None
    try:
        with open(".mdemg.port") as f:
            port = f.read().strip()
    except FileNotFoundError:
        pass
    if not port:
        try:
            with open(".env") as f:
                for line in f:
                    if line.startswith("MDEMG_PORT="):
                        port = line.split("=", 1)[1].strip()
                        break
        except FileNotFoundError:
            pass
    return f"http://localhost:{port or '9999'}"


MDEMG_URL = _resolve_mdemg_url()
# Default; main() resolves the real per-conversation id from the hook stdin.
SESSION_ID = "claude-core"


def _resolve_session_id(input_data):
    """Per-conversation SessionID: MDEMG_SESSION_ID env > Claude Code stdin
    session_id > ~/.mdemg/.claude-session > claude-core. Realizes J17
    per-(session,constraint) isolation instead of one shared id."""
    sid = os.environ.get("MDEMG_SESSION_ID")
    if sid:
        return sid
    sid = (input_data or {}).get("session_id")
    if sid:
        return sid
    try:
        with open(os.path.join(os.path.expanduser("~"), ".mdemg", ".claude-session")) as f:
            sid = json.load(f).get("session_id")
            if sid:
                return sid
    except Exception:
        pass
    return "claude-core"


def _response_text(resp):
    """HOOKWIRE-001: normalize a PostToolUse tool_response (string | dict |
    list) to analyzable text. Bash responses arrive as an object carrying
    stdout/stderr; older payloads were plain strings."""
    if isinstance(resp, str):
        return resp
    if isinstance(resp, dict):
        parts = [str(resp[k]) for k in ("stdout", "stderr", "output", "content", "error") if resp.get(k)]
        if parts:
            return "\n".join(parts)
        try:
            return json.dumps(resp)
        except (TypeError, ValueError):
            return str(resp)
    if isinstance(resp, list):
        return "\n".join(_response_text(x) for x in resp)
    return str(resp) if resp else ""


# HOOKSYNC-001: throttled activity heartbeat. Proves sessions are ACTIVE so
# the server's hook_channel_silent rule can detect the per-prompt channel
# being dead while work is demonstrably happening (the HOOKWIRE-001 outage
# shape: this hook kept firing while prompt-context silently exited).
HEARTBEAT_COOLDOWN_SEC = int(os.environ.get("HOOK_HEARTBEAT_COOLDOWN_SEC", "300"))
HEARTBEAT_FILE = os.path.join(os.path.expanduser("~"), ".mdemg", ".hook-heartbeat-post-tool-observe")


def send_hook_heartbeat():
    """Fire-and-forget POST /v1/hooks/event, at most once per cooldown."""
    try:
        now = time.time()
        try:
            if now - os.path.getmtime(HEARTBEAT_FILE) < HEARTBEAT_COOLDOWN_SEC:
                return
        except OSError:
            pass
        os.makedirs(os.path.dirname(HEARTBEAT_FILE), exist_ok=True)
        with open(HEARTBEAT_FILE, "w") as f:
            f.write(str(int(now)))
        subprocess.Popen(
            [
                "curl", "-sf", "-X", "POST",
                f"{MDEMG_URL}/v1/hooks/event",
                "-H", "Content-Type: application/json",
                "-d", json.dumps({"hook": "post-tool-observe", "session_id": SESSION_ID}),
                "--connect-timeout", "1", "--max-time", "2", "-o", "/dev/null",
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    except Exception:
        pass


INGEST_COOLDOWN_FILE = os.path.join(os.path.expanduser("~"), ".mdemg", ".last-ingest")
INGEST_COOLDOWN_SECONDS = 300  # 5 minutes
INGEST_LOG = os.path.join(os.path.expanduser("~"), ".mdemg", "logs", "ingest-claude-md.log")
RECOVERY_BUFFER_SPACE = os.environ.get("SYNERGY_RECOVERY_BUFFER_SPACE", "synergy-buffer")
RECOVERY_BUFFER_PATH = os.path.join(
    os.getcwd(), os.environ.get("SYNERGY_RECOVERY_BUFFER_PATH", ".mdemg/synergy-recovery-buffer.jsonl")
)
RECOVERY_BUFFER_MAX_ENTRIES = int(os.environ.get("SYNERGY_RECOVERY_BUFFER_MAX_ENTRIES", "50"))

# J17: Feedback loop closure — correlate guidance with agent actions
JIMINY_STATE_FILE = os.path.join(os.path.expanduser("~"), ".mdemg", ".jiminy-guidance-state")
FEEDBACK_COOLDOWN_FILE = os.path.join(os.path.expanduser("~"), ".mdemg", ".jiminy-last-feedback")
FEEDBACK_COOLDOWN_SEC = 10
FEEDBACK_STATE_MAX_AGE = 7200  # 2 hours, matching EffectivenessTracker TTL


def get_space_id() -> str:
    """Resolve space_id from env var, config file, or default."""
    # 1. Environment variable
    space_id = os.environ.get("MDEMG_SPACE_ID")
    if space_id:
        return space_id
    # 2. Config file
    try:
        import yaml
        config_path = os.path.join(os.getcwd(), ".mdemg", "config.yaml")
        with open(config_path) as f:
            cfg = yaml.safe_load(f)
            if cfg and "space_id" in cfg:
                return cfg["space_id"]
    except Exception:
        pass
    # 3. Default
    return "mdemg-dev"


SPACE_ID = get_space_id()


def should_ingest() -> bool:
    """Return True if enough time has passed since the last ingest (rate limiting)."""
    try:
        if os.path.exists(INGEST_COOLDOWN_FILE):
            mtime = os.path.getmtime(INGEST_COOLDOWN_FILE)
            if time.time() - mtime < INGEST_COOLDOWN_SECONDS:
                return False
    except Exception:
        pass
    return True


def mark_ingested() -> None:
    """Touch the cooldown file to record that ingest was triggered."""
    try:
        os.makedirs(os.path.dirname(INGEST_COOLDOWN_FILE), exist_ok=True)
        with open(INGEST_COOLDOWN_FILE, "w") as f:
            f.write(str(time.time()))
    except Exception:
        pass


def evaluate_output(agent_output: str, file_path: str = "", tool_name: str = ""):
    """Call Jiminy evaluate endpoint for agent output interception (J9).
    If concerns found, print system-reminder warning for the agent to see."""
    if not agent_output or len(agent_output) < 10:
        return
    payload = {
        "space_id": SPACE_ID,
        "agent_output": agent_output,
        "session_id": SESSION_ID,
    }
    if file_path:
        payload["file_path"] = file_path
    if tool_name:
        payload["tool_name"] = tool_name
    try:
        result = subprocess.run(
            [
                "curl", "-sf", "-X", "POST",
                f"{MDEMG_URL}/v1/jiminy/evaluate",
                "-H", "Content-Type: application/json",
                "-d", json.dumps(payload),
                "--connect-timeout", "3",
                "--max-time", "35",
            ],
            capture_output=True,
            text=True,
            timeout=45,
        )
        if result.returncode == 0 and result.stdout:
            resp = json.loads(result.stdout)
            data = resp.get("data", resp)
            status = data.get("status", "pass")
            if status in ("warning", "concern"):
                items = data.get("items", [])
                summary = data.get("summary", "")
                lines = [f"═══ JIMINY EVALUATE: {status.upper()} ═══"]
                if summary:
                    lines.append(summary)
                for item in items[:5]:
                    lines.append(f"  • [{item.get('severity', '?')}] {item.get('content', '')}")
                lines.append("═══ END JIMINY EVALUATE ═══")
                # Print as system-reminder so the agent sees it next turn
                print("<system-reminder>" + "\n".join(lines) + "</system-reminder>")
    except Exception:
        pass  # Fire-and-forget: never block on failure


_warm_last_triggered = 0.0
WARM_DEBOUNCE_SEC = 10


def warm_guidance(context_hint: str):
    """Fire-and-forget warm trigger with 10s debounce."""
    global _warm_last_triggered
    now = time.time()
    if now - _warm_last_triggered < WARM_DEBOUNCE_SEC:
        return
    _warm_last_triggered = now
    try:
        payload = json.dumps({
            "space_id": SPACE_ID,
            "context_hint": context_hint[:500],
            "session_id": SESSION_ID,
        })
        subprocess.Popen(
            [
                "curl", "-sf", "-X", "POST",
                f"{MDEMG_URL}/v1/jiminy/warm",
                "-H", "Content-Type: application/json",
                "-d", payload,
                "--connect-timeout", "1",
                "--max-time", "2",
                "-o", "/dev/null",
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    except Exception:
        pass


def _feedback_cooled_down() -> bool:
    """Return True if at least FEEDBACK_COOLDOWN_SEC since last feedback."""
    try:
        if os.path.exists(FEEDBACK_COOLDOWN_FILE):
            mtime = os.path.getmtime(FEEDBACK_COOLDOWN_FILE)
            if time.time() - mtime < FEEDBACK_COOLDOWN_SEC:
                return False
    except Exception:
        pass
    return True


def _mark_feedback_sent():
    """Touch the cooldown file to record feedback was sent."""
    try:
        os.makedirs(os.path.dirname(FEEDBACK_COOLDOWN_FILE), exist_ok=True)
        with open(FEEDBACK_COOLDOWN_FILE, "w") as f:
            f.write(str(time.time()))
    except Exception:
        pass


def _infer_intent(file_path: str) -> str:
    """Infer likely intent from file path patterns for semantic anchoring."""
    fp = file_path.lower()
    if "_test" in fp or "test_" in fp:
        return "writing tests"
    if "migration" in fp:
        return "database migration"
    if "hook" in fp or "template" in fp:
        return "hook/template modification"
    if "config" in fp:
        return "configuration change"
    if "doc" in fp or "readme" in fp or "changelog" in fp:
        return "documentation update"
    return ""


def _build_action_summary(tool_name: str, tool_input: dict, tool_output: str) -> str:
    """Build a semantically rich action summary for Jiminy feedback classification.

    Summaries include file content snippets and intent annotations to improve
    cosine similarity with guidance content during outcome classification.
    """
    if tool_name == "Write":
        fp = tool_input.get("file_path", "unknown")
        intent = _infer_intent(fp)
        prefix = f"[{intent}] " if intent else ""
        content = tool_input.get("content", "")
        # Extract first meaningful lines (skip shebangs, imports, blank lines, package decls)
        lines = [
            l.strip() for l in content.split("\n")
            if l.strip() and not l.strip().startswith(("#!", "import ", "from ", "package ", "//"))
        ]
        snippet = " | ".join(lines[:5])[:300]
        if snippet:
            return f"{prefix}Wrote {fp}: {snippet}"
        return f"{prefix}Wrote file: {fp}"
    elif tool_name == "Edit":
        fp = tool_input.get("file_path", "unknown")
        intent = _infer_intent(fp)
        prefix = f"[{intent}] " if intent else ""
        old = tool_input.get("old_string", "")[:200]
        new = tool_input.get("new_string", "")[:200]
        return f"{prefix}Edited {fp}: replaced '{old}' with '{new}'"
    elif tool_name == "Bash":
        cmd = tool_input.get("command", "")[:200]
        out = (tool_output or "")[:400]
        return f"Ran: {cmd}\nOutput: {out}"
    return ""


def send_jiminy_feedback(tool_name: str, tool_input: dict, tool_output: str):
    """Read guidance_id from state file and POST feedback to close the J17 loop."""
    if not _feedback_cooled_down():
        return

    try:
        with open(JIMINY_STATE_FILE) as f:
            state = json.load(f)
    except (FileNotFoundError, json.JSONDecodeError, OSError):
        return

    guidance_id = state.get("guidance_id", "")
    if not guidance_id:
        return

    # Respect the 30-minute TTL of the EffectivenessTracker
    state_ts = state.get("ts", 0)
    if time.time() - state_ts > FEEDBACK_STATE_MAX_AGE:
        return

    action_summary = _build_action_summary(tool_name, tool_input, tool_output)
    if not action_summary:
        return

    payload = json.dumps({
        "guidance_id": guidance_id,
        "action_summary": action_summary[:500],
        "space_id": state.get("space_id", SPACE_ID),
        "session_id": state.get("session_id", SESSION_ID),
    })

    try:
        subprocess.Popen(
            [
                "curl", "-sf", "-X", "POST",
                f"{MDEMG_URL}/v1/jiminy/feedback",
                "-H", "Content-Type: application/json",
                "-d", payload,
                "--connect-timeout", "3",
                "--max-time", "5",
                "-o", "/dev/null",
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        _mark_feedback_sent()
    except Exception:
        pass


# ── GUARDRAIL-PRODUCER-001: produce real guardrail.evaluate training rows ──
GUARDRAIL_DIFF_MAX_BYTES = int(os.environ.get("MDEMG_GUARDRAIL_DIFF_MAX_BYTES", "8000"))


def _synth_diff(tool_name: str, tool_input: dict) -> tuple[str, str]:
    """Synthesize a minimal unified-diff-ish payload from Write/Edit input.

    Returns (file_path, diff) or ("", "") when not synthesizable. Only
    Write/Edit — Bash output has no reliable change representation.
    """
    fp = tool_input.get("file_path", "")
    if not fp:
        return "", ""
    if tool_name == "Write":
        content = tool_input.get("content", "")
        if not content:
            return "", ""
        body = "\n".join("+" + line for line in content.splitlines())
        return fp, f"--- /dev/null\n+++ {fp}\n{body}"[:GUARDRAIL_DIFF_MAX_BYTES]
    if tool_name == "Edit":
        old = tool_input.get("old_string", "")
        new = tool_input.get("new_string", "")
        if not (old or new):
            return "", ""
        minus = "\n".join("-" + line for line in old.splitlines())
        plus = "\n".join("+" + line for line in new.splitlines())
        return fp, f"--- {fp}\n+++ {fp}\n{minus}\n{plus}"[:GUARDRAIL_DIFF_MAX_BYTES]
    return "", ""


def send_guardrail_producer(tool_name: str, tool_input: dict):
    """Fire-and-forget async guardrail evaluation of a real Write/Edit.

    The server path (gated on GUARDRAIL_PRODUCER_ENABLED) detaches the
    evaluation and answers 202 in milliseconds; the llm_interactions row it
    records is production training data for guardrail.evaluate. Fail-open:
    never blocks or fails the hook.
    """
    fp, diff = _synth_diff(tool_name, tool_input)
    if not diff:
        return
    payload = json.dumps({
        "space_id": SPACE_ID,
        "files_changed": [fp],
        "diff": diff,
        "agent_trust_level": "standard",
        "async": True,
    })
    try:
        subprocess.Popen(
            [
                "curl", "-sf", "-X", "POST",
                f"{MDEMG_URL}/v1/memory/guardrail/validate",
                "-H", "Content-Type: application/json",
                "-d", payload,
                "--connect-timeout", "3",
                "--max-time", "5",
                "-o", "/dev/null",
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    except Exception:
        pass


def observe(content: str, obs_type: str, tags: list[str] | None = None):
    """Fire-and-forget observation to CMS."""
    payload = {
        "space_id": SPACE_ID,
        "session_id": SESSION_ID,
        "content": content[:500],  # Truncate to avoid oversized payloads
        "obs_type": obs_type,
    }
    if tags:
        payload["tags"] = tags

    try:
        subprocess.Popen(
            [
                "curl", "-sf", "-X", "POST",
                f"{MDEMG_URL}/v1/conversation/observe",
                "-H", "Content-Type: application/json",
                "-d", json.dumps(payload),
                "--connect-timeout", "2",
                "--max-time", "5",
                "-o", "/dev/null",
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
    except Exception:
        pass  # Fire-and-forget: never block on failure


def ingest_with_prune_guard(file_path: str):
    """Detect file shrinkage and record protective observation before ingesting."""
    try:
        with open(file_path) as f:
            new_lines = sum(1 for _ in f)
    except Exception:
        return

    try:
        result = subprocess.run(
            ["curl", "-sf",
             f"{MDEMG_URL}/v1/memory/node/meta?space_id={SPACE_ID}&path={os.path.basename(file_path)}",
             "--connect-timeout", "2", "--max-time", "3"],
            capture_output=True, text=True, timeout=5,
        )
        if result.returncode == 0 and result.stdout:
            meta = json.loads(result.stdout)
            old_lines = meta.get("line_count", 0)
            if old_lines > 0 and (old_lines - new_lines) > 10:
                observe(
                    f"[prune-guard] {os.path.basename(file_path)} shrank "
                    f"from {old_lines} to {new_lines} lines "
                    f"({old_lines - new_lines} lines removed). "
                    f"Previous hash: {meta.get('content_hash', '?')[:20]}",
                    "context",
                    ["prune-guard", "claude-md", "data-protection"],
                )
    except Exception:
        pass  # Best-effort — don't block ingest on prune-guard failure

    # Proceed with normal ingest
    try:
        os.makedirs(os.path.dirname(INGEST_LOG), exist_ok=True)
        log_fd = open(INGEST_LOG, "a")
        subprocess.Popen(
            ["./bin/mdemg", "ingest-claude-md", "--quiet",
             "--space-id", SPACE_ID, "--file", file_path],
            stdout=log_fd,
            stderr=log_fd,
        )
    except Exception as e:
        try:
            with open(INGEST_LOG, "a") as f:
                f.write(f"[{time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime())}] "
                        f"post-tool ingest FAILED for {file_path}: {e}\n")
        except Exception:
            pass


def check_memory_overflow(file_path: str):
    """Synergy: detect MEMORY.md overflow and auto-ingest excess to CMS.
    Respects Jiminy health gate — if Jiminy is unhealthy, content stays
    in MEMORY.md as safety buffer."""
    # Check master switch
    if os.environ.get("SYNERGY_MEMORY_AUTO_INGEST", "true").lower() != "true":
        return

    threshold = int(os.environ.get("SYNERGY_MEMORY_LINE_THRESHOLD", "120"))

    try:
        with open(file_path) as f:
            lines = f.readlines()
    except Exception:
        return

    if len(lines) <= threshold:
        return

    # Jiminy health gate — if unhealthy, buffer instead of ingesting to mdemg-dev
    jiminy_healthy = False
    server_reachable = False
    try:
        result = subprocess.run(
            ["curl", "-sf", f"{MDEMG_URL}/v1/jiminy/healthz",
             "--connect-timeout", "2", "--max-time", "3"],
            capture_output=True, text=True, timeout=5,
        )
        server_reachable = True
        if result.returncode == 0:
            health = json.loads(result.stdout)
            if health.get("enabled") and health.get("status") == "ok":
                jiminy_healthy = True
    except Exception:
        server_reachable = False

    if not jiminy_healthy:
        # Buffer the content instead of losing it
        write_recovery_buffer(file_path, server_reachable)
        return

    # Extract overflow content
    overflow = lines[threshold:]
    overflow_text = "".join(overflow).strip()
    if not overflow_text:
        return

    # Use path-based ingest for stable leaf node (no Context Cooler decay)
    try:
        import urllib.request
        ingest_payload = json.dumps({
            "space_id": SPACE_ID,
            "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "source": "ingest-claude-md",
            "content": overflow_text[:2000],
            "path": f"overflow/{os.path.basename(file_path)}/{int(time.time())}",
            "name": f"MEMORY.md overflow ({len(overflow)} lines)",
            "summary": overflow_text[:200],
            "tags": ["auto-overflow", "synergy", "claude-md"],
        }).encode()
        req = urllib.request.Request(
            f"{MDEMG_URL}/v1/memory/ingest",
            data=ingest_payload,
            headers={"Content-Type": "application/json"},
        )
        urllib.request.urlopen(req, timeout=5)
    except Exception:
        # Fallback to observe if ingest fails
        observe(
            f"[auto-overflow] MEMORY.md exceeded {threshold} lines ({len(lines)} total). "
            f"Overflow content ({len(overflow)} lines): {overflow_text[:400]}",
            "note",
            ["auto-overflow", "synergy"],
        )

    # Notify via system-reminder
    print(
        f"<system-reminder>MEMORY.md overflow ({len(lines)} lines, threshold: {threshold}). "
        f"{len(overflow)} lines auto-ingested to CMS as stable leaf node.</system-reminder>"
    )


def write_recovery_buffer(file_path: str, server_reachable: bool):
    """Buffer MEMORY.md content during Jiminy outage (store-and-forward).
    Tier 1: Write to synergy-buffer CMS space (if server reachable).
    Tier 2: Append to local JSONL file (if server also unreachable)."""
    try:
        with open(file_path) as f:
            content = f.read()
    except Exception:
        return

    if not content.strip():
        return

    # Classify obs_type by content heuristics
    obs_type = "note"
    if any(kw in content for kw in ["## Phase", "Phase ", "COMPLETE"]):
        obs_type = "progress"
    elif any(kw in content for kw in ["NEVER", "ALWAYS", "MUST", "MANDATORY"]):
        obs_type = "constraint"

    entry = {
        "space_id": RECOVERY_BUFFER_SPACE,
        "session_id": SESSION_ID,
        "content": content[:500],
        "obs_type": obs_type,
        "tags": ["recovery-buffer", "synergy", "jiminy-outage"],
        "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }

    if server_reachable:
        # Tier 1: POST to synergy-buffer space via existing observe endpoint
        try:
            subprocess.Popen(
                [
                    "curl", "-sf", "-X", "POST",
                    f"{MDEMG_URL}/v1/conversation/observe",
                    "-H", "Content-Type: application/json",
                    "-d", json.dumps(entry),
                    "--connect-timeout", "2",
                    "--max-time", "5",
                    "-o", "/dev/null",
                ],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
            )
        except Exception:
            # Server seemed reachable but POST failed — fall through to Tier 2
            append_local_buffer(entry)
    else:
        # Tier 2: Local JSONL fallback
        append_local_buffer(entry)


def append_local_buffer(entry: dict):
    """Append an observation entry to the local JSONL recovery buffer with FIFO eviction."""
    try:
        os.makedirs(os.path.dirname(RECOVERY_BUFFER_PATH), exist_ok=True)

        # Read existing lines for FIFO enforcement
        existing_lines = []
        if os.path.exists(RECOVERY_BUFFER_PATH):
            with open(RECOVERY_BUFFER_PATH) as f:
                existing_lines = f.readlines()

        # Append new entry
        existing_lines.append(json.dumps(entry) + "\n")

        # FIFO eviction: keep only the newest entries
        if len(existing_lines) > RECOVERY_BUFFER_MAX_ENTRIES:
            existing_lines = existing_lines[-RECOVERY_BUFFER_MAX_ENTRIES:]

        with open(RECOVERY_BUFFER_PATH, "w") as f:
            f.writelines(existing_lines)
    except Exception:
        pass  # Best-effort: never block on buffer failure


def main():
    try:
        input_data = json.load(sys.stdin)
    except (json.JSONDecodeError, EOFError):
        sys.exit(0)

    # Resolve the per-conversation SessionID for all observe() calls below.
    global SESSION_ID
    SESSION_ID = _resolve_session_id(input_data)

    # HOOKSYNC-001: activity heartbeat (throttled, fail-open).
    send_hook_heartbeat()

    tool_name = input_data.get("tool_name", "")
    tool_input = input_data.get("tool_input", {})
    # HOOKWIRE-001: Claude Code's PostToolUse stdin field is `tool_response`
    # (string OR object). The old `tool_output` read was always empty, so the
    # error indicators below never matched and every build/test command was
    # recorded as "succeeded" sight-unseen. Keep `tool_output` as fallback.
    tool_output = input_data.get("tool_response") or input_data.get("tool_output") or ""
    output_str = _response_text(tool_output)[:2000]

    # --- Write/Edit to CLAUDE.md or settings → decision ---
    if tool_name in ("Write", "Edit"):
        file_path = tool_input.get("file_path", "")
        if "CLAUDE.md" in file_path:
            observe(
                f"Modified CLAUDE.md: {file_path}",
                "decision",
                ["claude-md", "configuration"],
            )
        elif "settings" in file_path.lower():
            observe(
                f"Modified settings file: {file_path}",
                "decision",
                ["settings", "configuration"],
            )

        # Synergy: MEMORY.md overflow detection + auto-ingestion
        if file_path.endswith("MEMORY.md") and "memory" in file_path:
            check_memory_overflow(file_path)

        # Auto-ingest tracked claude .md files on modification
        CLAUDE_MD_PATTERNS = [
            "MEMORY.md", "CLAUDE.md", "AGENT_HANDOFF.md", "VISION.md",
            "AGENTS.md", "CLAUDE.local.md",
            "/memory/", "/plans/", "/rules/",
        ]
        if any(p in file_path for p in CLAUDE_MD_PATTERNS):
            ingest_with_prune_guard(file_path)

        # Warm guidance after file edits
        warm_guidance(f"edited: {file_path}")

        # J9: Evaluate agent output after Write/Edit (code changes)
        # Use the new_string (Edit) or content (Write) as agent output
        agent_output = tool_input.get("new_string", "") or tool_input.get("content", "")
        if agent_output and file_path:
            evaluate_output(agent_output, file_path=file_path, tool_name=tool_name)

    # --- Bash errors → error observation ---
    elif tool_name == "Bash":
        command = tool_input.get("command", "")
        # Check for error indicators in output
        error_indicators = ["error:", "Error:", "FATAL", "fatal:", "panic:", "FAILED", "command not found"]
        if any(indicator in output_str for indicator in error_indicators):
            observe(
                f"Bash error in command: {command[:200]}\nOutput: {output_str[:300]}",
                "error",
                ["bash-error"],
            )
            warm_guidance(f"bash error: {command[:200]}")

        # Successful build/test → progress
        # HOOKWIRE-001: require NON-EMPTY output before claiming success.
        # When tool_output was always empty (the tool_response contract bug),
        # this branch blind-recorded "succeeded" for every build/test command.
        # Trade-off: a truly silent success (no output at all) records nothing
        # — better to miss a success than to fabricate one.
        if any(kw in command for kw in ["go build", "go test", "npm run build", "pytest"]):
            if output_str and not any(indicator in output_str for indicator in error_indicators):
                observe(
                    f"Build/test succeeded: {command[:200]}",
                    "progress",
                    ["build", "success"],
                )

        # Phase 80: CMS anomaly detection in API responses
        if "curl" in command:
            # Detect degraded memory state in curl output
            if "X-MDEMG-Memory-State: degraded" in output_str:
                observe(
                    f"CMS anomaly: Degraded memory state detected in API response. Command: {command[:200]}",
                    "error",
                    ["anomaly", "memory-degraded"],
                )
            # Detect empty resume in curl output
            if '"observations": []' in output_str and "resume" in command:
                observe(
                    f"CMS anomaly: Empty resume detected. Command: {command[:200]}",
                    "error",
                    ["anomaly", "empty-resume"],
                )

    # --- F12: Read/Glob/Grep → lightweight context_signal ---
    elif tool_name == "Read":
        file_path = tool_input.get("file_path", "")
        if file_path:
            observe(
                f"Read file: {file_path}",
                "context_signal",
                ["context-read", "file-access"],
            )

    elif tool_name == "Glob":
        pattern = tool_input.get("pattern", "")
        if pattern:
            observe(
                f"Glob search: {pattern}",
                "context_signal",
                ["context-glob", "file-search"],
            )

    elif tool_name == "Grep":
        pattern = tool_input.get("pattern", "")
        path = tool_input.get("path", ".")
        if pattern:
            observe(
                f"Grep search: {pattern} in {path}",
                "context_signal",
                ["context-grep", "content-search"],
            )

    # --- Bash: continued from above (re-enter Bash branch for git push) ---
    if tool_name == "Bash":
        command = tool_input.get("command", "")
        # Git push → trigger incremental ingest + consolidation on mdemg repo
        # Match: branch name (mdemg-dev01), remote URL, or explicit path
        # Rate-limited: skip if last ingest was within INGEST_COOLDOWN_SECONDS (5 min)
        if "git push" in command and "mdemg" in command:
            if should_ingest():
                mark_ingested()
                observe(
                    f"Git push detected on mdemg repo. Triggering incremental ingest + consolidation.",
                    "progress",
                    ["git-push", "ingest", "consolidation"],
                )
                # Fire-and-forget: incremental ingest of mdemg into mdemg-dev
                try:
                    os.makedirs(os.path.dirname(INGEST_LOG), exist_ok=True)
                    log_fd = open(INGEST_LOG, "a")
                    subprocess.Popen(
                        [
                            "./bin/mdemg",
                            "ingest",
                            "--space-id", SPACE_ID,
                            "--incremental",
                            "--consolidate",
                            "--path", ".",
                        ],
                        stdout=log_fd,
                        stderr=log_fd,
                    )
                except Exception:
                    pass

    # J17: Close the feedback loop — correlate guidance with agent action
    if tool_name in ("Write", "Edit", "Bash"):
        send_jiminy_feedback(tool_name, tool_input, output_str)

    # GUARDRAIL-PRODUCER-001: async guardrail evaluation of real edits
    if tool_name in ("Write", "Edit"):
        send_guardrail_producer(tool_name, tool_input)

    sys.exit(0)


if __name__ == "__main__":
    main()
