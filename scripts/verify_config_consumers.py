#!/usr/bin/env python3
"""CONFIG-DEADFLAG-001: every Config struct field must have >=1 consumer.

The parsed-but-never-read flag class caused the 24-day Hebbian no-op
(EVENTGRAPH-002 forensic) and the 9-week guidance dormancy (RRF-SCALE-001):
an operator sets an env var, FromEnv parses it, and nothing ever reads the
field — silent no-op. This scanner extracts every field of config.Config
and fails if a field is referenced nowhere outside internal/config (struct
literal assignments in config.go itself don't count as consumption).

Known-dead fields awaiting wiring/deletion are quarantined in ALLOWLIST
with a reason; the gate fails on ANY new unconsumed field.
"""
from __future__ import annotations

import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
CONFIG_GO = ROOT / "internal/config/config.go"

# Fields intentionally without direct cfg.<Field> consumers, with reasons.
ALLOWLIST: dict[str, str] = {}

def config_fields() -> list[str]:
    src = CONFIG_GO.read_text()
    m = re.search(r"type Config struct \{(.*?)\n\}", src, re.S)
    if not m:
        sys.exit("could not locate Config struct")
    fields = []
    for line in m.group(1).split("\n"):
        line = line.strip()
        fm = re.match(r"([A-Z]\w*)\s+[\[\]\*\w\.]+", line)
        if fm:
            fields.append(fm.group(1))
    return fields

def consumers(field: str) -> int:
    """Count dot-prefixed references (`x.Field`) anywhere in Go code.

    Dot-prefixing distinguishes consumption from the declaration
    (`Field type`) and the FromEnv struct-literal (`Field: value`).
    Config-package METHODS legitimately consume fields via the receiver
    (`c.Field` — e.g. EffectiveLLMEndpoint, JiminyWarmComputeTimeout), so
    internal/config is included; literal/declaration forms never match the
    dot pattern. Same-named fields on other types over-approximate, which
    is fine — the gate exists to catch ZERO-reference fields.
    """
    r = subprocess.run(
        ["grep", "-r", "-c", "-E", rf"\.{field}\b", "--include=*.go",
         "internal", "cmd", "tests"],
        capture_output=True, text=True, cwd=ROOT,
    )
    total = 0
    for line in r.stdout.splitlines():
        path, _, n = line.rpartition(":")
        if n.isdigit():
            total += int(n)
    return total

def main() -> int:
    fields = config_fields()
    dead = [f for f in fields if consumers(f) == 0]
    allowed = [f for f in dead if f in ALLOWLIST]
    violations = [f for f in dead if f not in ALLOWLIST]
    stale_allow = [f for f in ALLOWLIST if f not in dead]

    print(f"Config fields: {len(fields)}  consumed: {len(fields)-len(dead)}  "
          f"dead: {len(dead)} (allowlisted: {len(allowed)})")
    for f in sorted(violations):
        print(f"  ✗ {f}: parsed but consumed nowhere — wire it or delete it "
              f"(allowlist with a reason only as a last resort)")
    for f in sorted(stale_allow):
        print(f"  ✗ allowlist entry {f!r} is no longer dead — remove it")
    if violations or stale_allow:
        return 1
    print("config consumer check passed")
    return 0

if __name__ == "__main__":
    sys.exit(main())
