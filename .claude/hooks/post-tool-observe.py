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

MDEMG_URL = os.environ.get("MDEMG_URL", "http://localhost:9999")
SESSION_ID = "claude-core"
INGEST_COOLDOWN_FILE = os.path.join(os.path.expanduser("~"), ".mdemg", ".last-ingest")
INGEST_COOLDOWN_SECONDS = 300  # 5 minutes


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

        # Successful build/test → progress
        if any(kw in command for kw in ["go build", "go test", "npm run build", "pytest"]):
            if not any(indicator in output_str for indicator in error_indicators):
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
                    subprocess.Popen(
                        [
                            "/Users/reh3376/mdemg/bin/ingest-codebase",
                            "--path", "/Users/reh3376/mdemg",
                            "--space-id", SPACE_ID,
                            "--incremental",
                            "--consolidate",
                        ],
                        stdout=subprocess.DEVNULL,
                        stderr=subprocess.DEVNULL,
                        cwd="/Users/reh3376/mdemg",
                    )
                except Exception:
                    pass

    sys.exit(0)


if __name__ == "__main__":
    main()
