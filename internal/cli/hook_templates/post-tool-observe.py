#!/usr/bin/env python3
"""
Hook: PostToolUse — auto-capture observations after significant tool completions.
Fires-and-forgets a CMS observe call for noteworthy events.
"""

import json
import os
import subprocess
import sys
import time

MDEMG_URL = os.environ.get("MDEMG_URL", "{{MDEMG_URL}}")
SPACE_ID = "{{SPACE_ID}}"
SESSION_ID = "claude-core"
INGEST_COOLDOWN_FILE = os.path.join(os.path.expanduser("~"), ".mdemg", ".last-ingest")
INGEST_COOLDOWN_SECONDS = 300  # 5 minutes


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


def main():
    try:
        input_data = json.load(sys.stdin)
    except (json.JSONDecodeError, EOFError):
        sys.exit(0)

    tool_name = input_data.get("tool_name", "")
    tool_input = input_data.get("tool_input", {})
    tool_output = input_data.get("tool_output", "")

    # Truncate output for analysis
    output_str = str(tool_output)[:2000] if tool_output else ""

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

        # Warm guidance after file edits
        warm_guidance(f"edited: {file_path}")

        # J9: Evaluate agent output after Write/Edit (code changes)
        agent_output = tool_input.get("new_string", "") or tool_input.get("content", "")
        if agent_output and file_path:
            evaluate_output(agent_output, file_path=file_path, tool_name=tool_name)

    # --- Bash errors → error observation ---
    elif tool_name == "Bash":
        command = tool_input.get("command", "")
        error_indicators = ["error:", "Error:", "FATAL", "fatal:", "panic:", "FAILED", "command not found"]
        if any(indicator in output_str for indicator in error_indicators):
            observe(
                f"Bash error in command: {command[:200]}\nOutput: {output_str[:300]}",
                "error",
                ["bash-error"],
            )
            warm_guidance(f"bash error: {command[:200]}")

        # Successful build/test → progress
        if any(kw in command for kw in ["go build", "go test", "npm run build", "pytest"]):
            if not any(indicator in output_str for indicator in error_indicators):
                observe(
                    f"Build/test succeeded: {command[:200]}",
                    "progress",
                    ["build", "success"],
                )

        # CMS anomaly detection in API responses
        if "curl" in command:
            if "X-MDEMG-Memory-State: degraded" in output_str:
                observe(
                    f"CMS anomaly: Degraded memory state detected in API response. Command: {command[:200]}",
                    "error",
                    ["anomaly", "memory-degraded"],
                )
            if '"observations": []' in output_str and "resume" in command:
                observe(
                    f"CMS anomaly: Empty resume detected. Command: {command[:200]}",
                    "error",
                    ["anomaly", "empty-resume"],
                )

    # --- Read/Glob/Grep → lightweight context_signal ---
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

    # --- Git push → trigger incremental ingest ---
    if tool_name == "Bash":
        command = tool_input.get("command", "")
        if "git push" in command and "mdemg" in command:
            if should_ingest():
                mark_ingested()
                observe(
                    "Git push detected on mdemg repo. Triggering incremental ingest + consolidation.",
                    "progress",
                    ["git-push", "ingest", "consolidation"],
                )

    sys.exit(0)


if __name__ == "__main__":
    main()
