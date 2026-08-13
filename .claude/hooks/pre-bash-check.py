#!/usr/bin/env python3
# MDEMG hook — managed by mdemg hooks install
"""
Hook: PreToolUse (Bash) — TWO checks in sequence:
  1. Destructive-command guard (base64-encoded patterns; fail-closed).
     Blocks rm -rf, DROP TABLE, git push --force, etc. without user
     confirmation. Predates the JIMINY-ENFORCE arc.
  2. JIMINY-ENFORCE-002 (2026-08-02): when /strict mode is active AND
     the command isn't in the read-only whitelist, POST /v1/jiminy/
     classify to consult Jiminy for constraint violations. Fail-open
     with persistent marker (parity with pre-write-check.py) so an
     unreachable server never wedges Bash.

NOTE: Destructive patterns are base64-encoded to avoid triggering content
filters during code exploration. They are decoded at runtime.

SAFETY:
  - Destructive check: fail-CLOSED (broken stdin/patterns → deny)
  - Jiminy check:      fail-OPEN with stderr WARN + marker file (parity
                       with pre-write-check.py; per operator directive
                       2026-08-01 — enforcement guarantee is off but
                       tool calls proceed)
"""

import base64
import json
import os
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request
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


# JIMINY-ENFORCE-002: read-only whitelist. Commands matching any pattern skip
# the Jiminy classify round-trip entirely — every ls/grep/git-status POST
# would inflate LLM cost + latency for zero enforcement value (these commands
# can't violate a durable constraint by definition). Operator can extend via
# MDEMG_BASH_WHITELIST_REGEX env (comma-separated regexes, appended to defaults).
DEFAULT_BASH_WHITELIST = [
    r"^\s*(ls|cat|head|tail|wc|grep|find|file|stat|which|whereis|pwd|whoami|hostname|date|echo|printf|env|id|uname|uptime|df|du|ps|top|htop|free|lsof|netstat|dig|nslookup|ping)\b",
    r"^\s*git\s+(status|log|diff|show|branch|remote|config\s+--get|rev-parse|blame|ls-files|describe|reflog)\b",
    r"^\s*docker\s+(ps|inspect|logs|images|version|info|exec\s+\S+\s+(ls|cat|ps))\b",
    r"^\s*kubectl\s+(get|describe|logs|config\s+view|version)\b",
    r"^\s*(go|python3?|node|npm|yarn|cargo)\s+(version|--version|-V)\b",
]

_WHITELIST_PATTERNS = [re.compile(p, re.IGNORECASE) for p in DEFAULT_BASH_WHITELIST]
_extra = os.environ.get("MDEMG_BASH_WHITELIST_REGEX", "").strip()
if _extra:
    for _p in _extra.split(","):
        _p = _p.strip()
        if _p:
            try:
                _WHITELIST_PATTERNS.append(re.compile(_p, re.IGNORECASE))
            except re.error:
                pass


def is_read_only_whitelisted(command: str) -> bool:
    """Return True if the command matches any read-only whitelist pattern."""
    for pat in _WHITELIST_PATTERNS:
        if pat.search(command):
            return True
    return False


# --- JIMINY-ENFORCE-002: strict-mode classify path ------------------------------

JIMINY_UNREACHABLE_MARKER = os.path.expanduser("~/.mdemg/.jiminy-server-unreachable")
STRICT_STATE_FILE = os.path.expanduser("~/.mdemg/.jiminy-strict-mode")


def _get_mdemg_url() -> str:
    url = os.environ.get("MDEMG_URL")
    if url:
        return url
    if os.path.isfile(".mdemg.port"):
        try:
            with open(".mdemg.port") as f:
                port = f.read().strip()
                if port:
                    return f"http://localhost:{port}"
        except OSError:
            pass
    if os.path.isfile(".env"):
        try:
            with open(".env") as f:
                for line in f:
                    if line.startswith("MDEMG_PORT="):
                        port = line.split("=", 1)[1].strip()
                        if port:
                            return f"http://localhost:{port}"
        except OSError:
            pass
    return "http://localhost:9999"


def _emit_jiminy_fail_open_warning(reason: str, url: str):
    """Same shape as pre-write-check.py — stderr WARN + persistent marker."""
    msg = (
        f"⚠️  JIMINY ENFORCEMENT SUSPENDED (Bash, MDEMG server unreachable at {url}): "
        f"{reason}. Action allowed; strict-mode guarantee is temporarily OFF."
    )
    print(msg, file=sys.stderr)
    try:
        os.makedirs(os.path.dirname(JIMINY_UNREACHABLE_MARKER), exist_ok=True)
        with open(JIMINY_UNREACHABLE_MARKER, "w") as f:
            json.dump({"reason": reason, "url": url, "ts": int(time.time()), "tool": "Bash"}, f)
    except OSError:
        pass


def _clear_jiminy_fail_open_marker():
    """Called after any successful classify — the outage window is over."""
    try:
        if os.path.exists(JIMINY_UNREACHABLE_MARKER):
            os.remove(JIMINY_UNREACHABLE_MARKER)
    except OSError:
        pass


def _resolve_jiminy_session_id(input_data: dict) -> str:
    """Same resolution order as pre-write-check.py + prompt-context.sh."""
    sid = os.environ.get("MDEMG_SESSION_ID") or (input_data or {}).get("session_id")
    if sid:
        return sid
    if os.path.isfile(STRICT_STATE_FILE):
        try:
            with open(STRICT_STATE_FILE) as f:
                sid = json.load(f).get("session_id")
                if sid:
                    return sid
        except (json.JSONDecodeError, OSError):
            pass
    claude_session = os.path.expanduser("~/.mdemg/.claude-session")
    if os.path.isfile(claude_session):
        try:
            with open(claude_session) as f:
                sid = json.load(f).get("session_id")
                if sid:
                    return sid
        except (json.JSONDecodeError, OSError):
            pass
    return "claude-core"


def check_jiminy_bash(input_data: dict, command: str) -> Optional[str]:
    """Consult Jiminy /classify for a Bash command. Returns the denial reason
    when Jiminy denies, or None when Jiminy passes / server unreachable
    (fail-open) / strict mode is off.
    """
    if not os.path.isfile(STRICT_STATE_FILE):
        return None  # strict mode off — no classify
    if is_read_only_whitelisted(command):
        return None  # read-only command — skip classify (LLM cost/latency)

    session_id = _resolve_jiminy_session_id(input_data)
    space_id = "mdemg-dev"

    # Truncate to 2000 chars — same as pre-write-check.py
    agent_output = command[:2000]

    base_url = _get_mdemg_url()
    classify_url = f"{base_url}/v1/jiminy/classify"

    payload = json.dumps({
        "space_id": space_id,
        "session_id": session_id,
        "agent_output": agent_output,
        "tool_name": "Bash",
        "file_path": "",
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
        _emit_jiminy_fail_open_warning(f"URLError: {e}", classify_url)
        return None
    except TimeoutError:
        _emit_jiminy_fail_open_warning("timeout after 5s", classify_url)
        return None
    except (OSError, json.JSONDecodeError) as e:
        _emit_jiminy_fail_open_warning(f"{type(e).__name__}: {e}", classify_url)
        return None

    # Successful classify → clear marker
    _clear_jiminy_fail_open_marker()

    data = result.get("data", {})
    verdict = data.get("verdict", "pass")
    if verdict == "deny":
        # JIMINY-CLASSIFY-ESCALATION-INSPECT-001 (2026-08-10): return (reason, codes)
        # so main() can surface the actual constraint codes in the deny banner —
        # operators need the code to run `mdemg jiminy override apply --constraint X`.
        return (
            data.get("denial_reason", "Strict mode: constraint violation detected"),
            data.get("violated_codes") or [],
        )
    return None


# --- PRE-COMMIT-INVENTORY-GATE-001 (2026-08-13): fail-CLOSED gate on git commit ---
# Blocks `git commit` when the staged diff includes a file that requires a
# paired DORMANT-CENSUS-* inventory adjudication AND the corresponding
# verifier script fails. Prevents the class that hit PR 614 (route inventory
# miss) + PR 615 (metric inventory miss) BEFORE the commit lands, instead
# of waiting for CI to red-line the merge.
#
# Trigger files → verifier → inventory pair:
#   internal/api/server.go        → verify_route_consumers.py   → route_consumer_inventory.json
#   internal/metrics/collectors.go → verify_metrics_consumers.py → metrics_consumer_inventory.json
#   internal/tsdb/migrations/*.sql → verify_tsdb_consumers.py    → tsdb_consumer_inventory.json
#
# Fail-CLOSED philosophy: if the verifier fails, block the commit. This is
# the deliberate inversion of the JIMINY-ENFORCE-002 fail-open shape —
# there, an unreachable Jiminy server would wedge Bash forever; here, an
# unreachable verifier means we can't confirm the inventory is honest, and
# the correct posture for "can't confirm inventory" is to hold the commit.
# Verifier-missing / git-missing / other exceptions fail-CLOSED with a
# clear message + suggested fix; --no-verify has no effect (this hook is
# PreToolUse Claude-side, not git's own pre-commit).
_INVENTORY_CHECKS = [
    (
        re.compile(r"^internal/api/server\.go$"),
        "scripts/verify_route_consumers.py",
        "docs/api/route_consumer_inventory.json",
        "route",
    ),
    (
        re.compile(r"^internal/metrics/collectors\.go$"),
        "scripts/verify_metrics_consumers.py",
        "docs/api/metrics_consumer_inventory.json",
        "metric",
    ),
    (
        re.compile(r"^internal/tsdb/migrations/\d+_[\w-]+\.sql$"),
        "scripts/verify_tsdb_consumers.py",
        "docs/api/tsdb_consumer_inventory.json",
        "tsdb table",
    ),
]


def check_inventory_adjudication(command: str) -> Optional[str]:
    """Return a reason string to BLOCK the git commit, or None to allow.
    Fires only on `git commit` commands (not amend/status/log/diff)."""
    if not re.match(r"^\s*git\s+commit\b", command):
        return None
    try:
        result = subprocess.run(
            ["git", "diff", "--cached", "--name-only"],
            capture_output=True,
            text=True,
            timeout=5,
            check=False,
        )
    except (FileNotFoundError, subprocess.SubprocessError, OSError) as e:
        return f"PRE-COMMIT-INVENTORY-GATE: cannot inspect staged files (git error: {e}); refusing to commit blind. Run `git status` to diagnose."
    if result.returncode != 0:
        return f"PRE-COMMIT-INVENTORY-GATE: `git diff --cached --name-only` returned {result.returncode}: {result.stderr.strip()}; refusing to commit blind."
    staged = [line.strip() for line in result.stdout.splitlines() if line.strip()]
    if not staged:
        return None  # nothing staged; git commit itself will complain
    triggered = []
    for pattern, verifier, inventory, kind in _INVENTORY_CHECKS:
        if any(pattern.match(f) for f in staged):
            triggered.append((verifier, inventory, kind))
    if not triggered:
        return None
    for verifier, inventory, kind in triggered:
        if not os.path.isfile(verifier):
            return f"PRE-COMMIT-INVENTORY-GATE: {kind} inventory verifier not found at {verifier}; refusing to commit blind."
        try:
            r = subprocess.run(
                ["python3", verifier],
                capture_output=True,
                text=True,
                timeout=30,
                check=False,
            )
        except (subprocess.SubprocessError, OSError) as e:
            return f"PRE-COMMIT-INVENTORY-GATE: {kind} verifier ({verifier}) failed to run: {e}. Refusing to commit blind."
        if r.returncode != 0:
            output = (r.stdout.strip() + "\n" + r.stderr.strip()).strip()
            return (
                f"COMMIT BLOCKED — {kind} inventory adjudication missing.\n"
                f"\n"
                f"Verifier: {verifier}\n"
                f"{output}\n"
                f"\n"
                f"To fix:\n"
                f"  1. python3 {verifier} --generate\n"
                f"  2. Edit {inventory} — set the new entry's disposition + notes\n"
                f"  3. git add {inventory}\n"
                f"  4. git commit again\n"
                f"\n"
                f"(PRE-COMMIT-INVENTORY-GATE-001 — enforces DORMANT-CENSUS-001/002/003 forcing functions BEFORE commit lands, not at CI.)"
            )
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

    # 1) Destructive-pattern guard (fail-closed) — shipped
    matched = is_destructive(command)
    if matched:
        _deny(
            f"DESTRUCTIVE COMMAND BLOCKED. "
            f"Matched pattern: {matched}. "
            f"This command could destroy data. "
            f"You MUST ask the user for explicit confirmation before running this."
        )

    # 2) PRE-COMMIT-INVENTORY-GATE-001: git-commit inventory adjudication (fail-closed)
    #    Blocks when staged files include DORMANT-CENSUS-* trigger files AND
    #    the paired verifier fails. Catches the class that hit PR 614 + 615
    #    BEFORE the commit lands (instead of waiting for CI to red-line merge).
    inv_reason = check_inventory_adjudication(command)
    if inv_reason:
        _deny(inv_reason)

    # 3) JIMINY-ENFORCE-002: consult Jiminy (fail-open) — new
    result = check_jiminy_bash(input_data, command)
    if result:
        reason, codes = result
        if codes:
            # Cap displayed codes at 5 to keep the banner readable (JIMINY-CLASSIFY-ESCALATION-INSPECT-001).
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

    # Both checks passed: allow silently
    sys.exit(0)


if __name__ == "__main__":
    main()
