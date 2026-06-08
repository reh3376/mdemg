#!/usr/bin/env python3
# MDEMG hook — managed by mdemg hooks install
"""
Hook: PreToolUse (Write/Edit) — block file modifications when /strict mode is active
and escalated constraints are violated.

Fail-open design: if the MDEMG server is unreachable or returns an error,
the action is ALLOWED. This ensures hooks never block normal workflow when
MDEMG is down.
"""

import json
import os
import sys
import urllib.request
import urllib.error


def _deny(reason: str):
    """Emit a deny decision and exit."""
    output = {
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "deny",
            "permissionDecisionReason": reason,
        }
    }
    json.dump(output, sys.stdout)
    sys.exit(0)


def _get_mdemg_url() -> str:
    """Discover MDEMG server URL."""
    url = os.environ.get("MDEMG_URL")
    if url:
        return url

    # Try .mdemg.port file
    if os.path.isfile(".mdemg.port"):
        with open(".mdemg.port") as f:
            port = f.read().strip()
            if port:
                return f"http://localhost:{port}"

    # Try .env file
    if os.path.isfile(".env"):
        with open(".env") as f:
            for line in f:
                if line.startswith("MDEMG_PORT="):
                    port = line.split("=", 1)[1].strip()
                    if port:
                        return f"http://localhost:{port}"

    return "http://localhost:9999"


def main():
    # Quick check: if strict mode state file doesn't exist, allow immediately
    strict_file = os.path.expanduser("~/.mdemg/.jiminy-strict-mode")
    if not os.path.isfile(strict_file):
        sys.exit(0)  # Not in strict mode — allow

    # Read hook input
    try:
        input_data = json.load(sys.stdin)
    except (json.JSONDecodeError, EOFError, OSError):
        sys.exit(0)  # Fail open

    tool_input = input_data.get("tool_input", {})
    tool_name = input_data.get("tool_name", "")

    # Extract the agent output (file content being written/edited)
    agent_output = ""
    file_path = ""
    if tool_name == "Write":
        agent_output = tool_input.get("content", "")
        file_path = tool_input.get("file_path", "")
    elif tool_name == "Edit":
        agent_output = tool_input.get("new_string", "")
        file_path = tool_input.get("file_path", "")

    if not agent_output:
        sys.exit(0)  # Nothing to classify

    space_id = "{{SPACE_ID}}"
    # Per-conversation SessionID: MDEMG_SESSION_ID env > Claude Code stdin
    # session_id > /strict state file > ~/.mdemg/.claude-session > claude-core.
    # Must match the id the agent enabled /strict with so /classify keys to the
    # right (session, constraint) escalation state.
    session_id = os.environ.get("MDEMG_SESSION_ID") or input_data.get("session_id")
    if not session_id:
        try:
            with open(strict_file) as f:
                session_id = json.load(f).get("session_id")
        except (json.JSONDecodeError, OSError):
            session_id = None
    if not session_id:
        try:
            with open(os.path.expanduser("~/.mdemg/.claude-session")) as f:
                session_id = json.load(f).get("session_id")
        except (json.JSONDecodeError, OSError):
            session_id = None
    if not session_id:
        session_id = "claude-core"

    # Call /v1/jiminy/classify
    base_url = _get_mdemg_url()
    classify_url = f"{base_url}/v1/jiminy/classify"

    # Truncate agent_output to 2000 chars to keep request fast
    if len(agent_output) > 2000:
        agent_output = agent_output[:2000]

    payload = json.dumps({
        "space_id": space_id,
        "session_id": session_id,
        "agent_output": agent_output,
        "tool_name": tool_name,
        "file_path": file_path,
    }).encode("utf-8")

    req = urllib.request.Request(
        classify_url,
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )

    try:
        with urllib.request.urlopen(req, timeout=5) as resp:
            result = json.loads(resp.read())
    except (urllib.error.URLError, OSError, json.JSONDecodeError, TimeoutError):
        sys.exit(0)  # Fail open — server unreachable

    data = result.get("data", {})
    verdict = data.get("verdict", "pass")

    if verdict == "deny":
        reason = data.get("denial_reason", "Strict mode: constraint violation detected")
        _deny(f"[/strict] {reason}")

    # Pass — allow the action
    sys.exit(0)


if __name__ == "__main__":
    main()
