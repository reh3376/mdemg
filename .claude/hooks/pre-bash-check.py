#!/usr/bin/env python3
# MDEMG hook — managed by mdemg hooks install
"""
Hook: PreToolUse (Bash) — block destructive commands without user confirmation.
Reads tool input from stdin JSON, checks against destructive patterns, and
returns a deny decision if the command is dangerous.

NOTE: Patterns are base64-encoded to avoid triggering content filters during
code exploration. They are decoded at runtime.

SAFETY: Fail-closed design — if patterns fail to decode or stdin is broken,
ALL commands are blocked rather than silently allowed.
"""

import base64
import json
import re
import sys
from typing import Optional

# Destructive command patterns (base64-encoded to avoid content filter triggers)
# These MUST NOT run without explicit user confirmation
DESTRUCTIVE_PATTERNS_B64 = [
    "XGJyZXNldC1kYlxi",
    "XGJjbGVhci1zcGFjZVxi",
    "XGJEUk9QXHMrKFRBQkxFfERBVEFCQVNFfElOREVYfFNDSEVNQSlcYg==",
    "XGJUUlVOQ0FURVxi",
    "XGJERUxFVEVccytGUk9NXGI=",
    "XGJERVRBQ0hccytERUxFVEVcYg==",
    "XGJybVxzKygtW2EtekEtWl0qclthLXpBLVpdKmZ8LS1yZWN1cnNpdmVccystLWZvcmNlfC1bYS16QS1aXSpmW2EtekEtWl0qcilcYg==",
    "XGJybVxzKy1yZlxi",
    "XGJybVxzKy1mclxi",
    "XGJnaXRccysocmVzZXR8Y2hlY2tvdXQpXHMrLS1oYXJkXGI=",
    "XGJnaXRccytwdXNoXHMrLS1mb3JjZVxi",
    "XGJnaXRccytwdXNoXHMrLWZcYg==",
    "XGJnaXRccytjbGVhblxzKy1bYS16QS1aXSpm",
    "XGJnaXRccyticmFuY2hccystRFxi",
    "TUFUQ0hccypcKG5cKVxzKkRFVEFDSFxzK0RFTEVURVxzK24=",
    "TUFUQ0hccypcKG5cKVxzKkRFTEVURVxzK24=",
]

# Decode patterns individually — skip corrupt entries, log warnings
DESTRUCTIVE_PATTERNS = []
for _p in DESTRUCTIVE_PATTERNS_B64:
    try:
        DESTRUCTIVE_PATTERNS.append(base64.b64decode(_p).decode())
    except Exception as _e:
        print(f"WARNING: Failed to decode destructive pattern: {_e}", file=sys.stderr)

# Compile patterns individually — skip invalid regex, log warnings
COMPILED_PATTERNS = []
for _p in DESTRUCTIVE_PATTERNS:
    try:
        COMPILED_PATTERNS.append(re.compile(_p, re.IGNORECASE))
    except re.error as _e:
        print(f"WARNING: Failed to compile pattern '{_p}': {_e}", file=sys.stderr)


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


# Fail-closed: if zero patterns loaded, block ALL commands
if not COMPILED_PATTERNS:
    def is_destructive(command: str) -> Optional[str]:
        return "SAFETY: All destructive patterns failed to load — blocking all commands as precaution"
else:
    def is_destructive(command: str) -> Optional[str]:
        """Check if a command matches any destructive pattern. Returns the matched pattern or None."""
        for i, pattern in enumerate(COMPILED_PATTERNS):
            if pattern.search(command):
                return DESTRUCTIVE_PATTERNS[i]
        return None


def main():
    try:
        input_data = json.load(sys.stdin)
    except (json.JSONDecodeError, EOFError, OSError):
        # Fail-closed: broken stdin means we can't verify the command is safe
        _deny("SAFETY: Could not read hook input — blocking command as precaution")
        return

    tool_input = input_data.get("tool_input", {})
    command = tool_input.get("command", "")

    if not command:
        sys.exit(0)

    matched = is_destructive(command)
    if matched:
        _deny(
            f"DESTRUCTIVE COMMAND BLOCKED. "
            f"Matched pattern: {matched}. "
            f"This command could destroy data. "
            f"You MUST ask the user for explicit confirmation before running this."
        )

    # Non-destructive: allow silently
    sys.exit(0)


if __name__ == "__main__":
    main()
