#!/usr/bin/env python3
# MDEMG hook — managed by mdemg hooks install
"""
Hook: PreToolUse (Write/Edit) — block file modifications when /strict mode is active
and escalated constraints are violated.

JIMINY-ENFORCE-001 (2026-08-01): Jiminy is an ENFORCER of hard constraints,
not merely advisory. The operator directive requires user notification on
every enforcement decision — blocks emit a HIGH-severity server-side alert
(see handleJiminyClassify in internal/api/handlers_jiminy.go).

Fail-open policy (operator-confirmed 2026-08-01): if the MDEMG server is
unreachable or errors, the action is ALLOWED but a WARNING is written to
stderr on EVERY fail-open (no silent second violations) AND a marker file
is written to ~/.mdemg/.jiminy-server-unreachable so subsequent hook fires
know the enforcement guarantee is temporarily off. The marker + warning
clear on the next successful classify call.
"""

import json
import os
import sys
import time
import urllib.request
import urllib.error


JIMINY_UNREACHABLE_MARKER = os.path.expanduser("~/.mdemg/.jiminy-server-unreachable")


def _emit_fail_open_warning(reason: str, url: str):
    """JIMINY-ENFORCE-001 fail-open policy: stderr WARN on every fail-open + persistent marker."""
    msg = (
        f"⚠️  JIMINY ENFORCEMENT SUSPENDED (MDEMG server unreachable at {url}): "
        f"{reason}. Action allowed; strict-mode guarantee is temporarily OFF."
    )
    print(msg, file=sys.stderr)
    try:
        os.makedirs(os.path.dirname(JIMINY_UNREACHABLE_MARKER), exist_ok=True)
        with open(JIMINY_UNREACHABLE_MARKER, "w") as f:
            json.dump({"reason": reason, "url": url, "ts": int(time.time())}, f)
    except OSError:
        pass  # marker best-effort


def _clear_fail_open_marker():
    """Called on any successful classify call — the outage window is over."""
    try:
        if os.path.exists(JIMINY_UNREACHABLE_MARKER):
            os.remove(JIMINY_UNREACHABLE_MARKER)
    except OSError:
        pass


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
    except urllib.error.URLError as e:
        _emit_fail_open_warning(f"URLError: {e}", classify_url)
        sys.exit(0)
    except TimeoutError:
        _emit_fail_open_warning("timeout after 5s", classify_url)
        sys.exit(0)
    except (OSError, json.JSONDecodeError) as e:
        _emit_fail_open_warning(f"{type(e).__name__}: {e}", classify_url)
        sys.exit(0)

    # Successful classify → clear any prior fail-open marker.
    _clear_fail_open_marker()

    data = result.get("data", {})
    verdict = data.get("verdict", "pass")

    if verdict == "deny":
        reason = data.get("denial_reason", "Strict mode: constraint violation detected")
        # JIMINY-CLASSIFY-ESCALATION-INSPECT-001 (2026-08-10): surface the
        # violated_codes prominently so operators can copy-paste an override
        # command without inspecting the raw JSON. The response's
        # violated_codes field is authoritative — DenialReason may include
        # `[code=X]` annotations too but codes are the operator override key.
        codes = data.get("violated_codes") or []
        if codes:
            # Cap displayed codes at 5 to keep the banner readable; elide the rest.
            shown = codes[:5]
            more = len(codes) - len(shown)
            code_list = ", ".join(shown)
            if more > 0:
                code_list += f" (+{more} more)"
            first_code = shown[0]
            override_hint = (
                f"\n[operator] to override: mdemg jiminy override apply "
                f"--constraint {first_code} --duration 30m --reason \"<why>\""
            )
            _deny(f"[/strict] BLOCKED (codes: {code_list}) {reason}{override_hint}")
        else:
            _deny(f"[/strict] {reason}")

    # Pass — allow the action
    sys.exit(0)


if __name__ == "__main__":
    main()
