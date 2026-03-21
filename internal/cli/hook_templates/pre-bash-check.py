#!/usr/bin/env python3
"""
Hook: PreToolUse (Bash) — block destructive commands without user confirmation.
Reads tool input from stdin JSON, checks against destructive patterns, and
returns a deny decision if the command is dangerous.

NOTE: Patterns are base64-encoded to avoid triggering content filters during
code exploration. They are decoded at runtime.
"""

import base64
import json
import re
import sys

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

# Decode patterns at module load
DESTRUCTIVE_PATTERNS = [base64.b64decode(p).decode() for p in DESTRUCTIVE_PATTERNS_B64]

# Compile for performance
COMPILED_PATTERNS = [re.compile(p, re.IGNORECASE) for p in DESTRUCTIVE_PATTERNS]


def is_destructive(command: str) -> str | None:
    """Check if a command matches any destructive pattern. Returns the matched pattern or None."""
    for i, pattern in enumerate(COMPILED_PATTERNS):
        if pattern.search(command):
            return DESTRUCTIVE_PATTERNS[i]
    return None


def main():
    try:
        input_data = json.load(sys.stdin)
    except (json.JSONDecodeError, EOFError):
        sys.exit(0)  # Can't parse input, allow through

    tool_input = input_data.get("tool_input", {})
    command = tool_input.get("command", "")

    if not command:
        sys.exit(0)

    matched = is_destructive(command)
    if matched:
        # Deny the command
        output = {
            "hookSpecificOutput": {
                "hookEventName": "PreToolUse",
                "permissionDecision": "deny",
                "permissionDecisionReason": (
                    f"DESTRUCTIVE COMMAND BLOCKED. "
                    f"Matched pattern: {matched}. "
                    f"This command could destroy data. "
                    f"You MUST ask the user for explicit confirmation before running this."
                ),
            }
        }
        json.dump(output, sys.stdout)
        sys.exit(0)

    # Non-destructive: allow silently
    sys.exit(0)


if __name__ == "__main__":
    main()
