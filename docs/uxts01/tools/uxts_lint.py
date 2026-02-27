#!/usr/bin/env python3
"""
uxts_lint.py
Lean lint + integrity tool for UxTS specs.

What it does:
- Loads a framework schema and validates spec JSON against it.
- Applies framework defaults (_defaults.json) using deterministic merge rules.
- Resolves ${ENV_VAR} placeholders (hard-fails if missing).
- Computes canonical SHA-256 hash of the effective spec (stable-ish).
- Optionally checks spec's embedded integrity.spec_hash.

This tool is intentionally small. It is NOT a runner.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Any, Dict, Tuple

from jsonschema import Draft202012Validator


ENV_TOKEN_RE = re.compile(r"\$\{([A-Z0-9_]+)\}")

class LintError(Exception):
    pass


def deep_merge_defaults(defaults: Any, spec: Any) -> Any:
    """
    Deterministic merge:
      - objects merge recursively
      - scalars override
      - arrays REPLACE (not concat/union)
    """
    if isinstance(defaults, dict) and isinstance(spec, dict):
        out = dict(defaults)
        for k, v in spec.items():
            if k in out:
                out[k] = deep_merge_defaults(out[k], v)
            else:
                out[k] = v
        return out
    # arrays replace
    return spec


def resolve_env_tokens(obj: Any, *, allow_unresolved: bool = False) -> Any:
    if isinstance(obj, dict):
        return {k: resolve_env_tokens(v, allow_unresolved=allow_unresolved) for k, v in obj.items()}
    if isinstance(obj, list):
        return [resolve_env_tokens(v, allow_unresolved=allow_unresolved) for v in obj]
    if isinstance(obj, str):
        def repl(m: re.Match) -> str:
            name = m.group(1)
            val = os.environ.get(name)
            if val is None:
                if allow_unresolved:
                    return m.group(0)
                raise LintError(f"Unresolved environment token: ${{{name}}}")
            return val
        return ENV_TOKEN_RE.sub(repl, obj)
    return obj


def canonical_bytes(obj: Any) -> bytes:
    """
    Minimal canonicalization: sorted keys + compact separators.
    Note: RFC 8785 JCS is stricter about numbers; this is sufficient for many repos,
    but if you require strict cross-language canonicalization, replace this with a true JCS library.
    """
    s = json.dumps(obj, sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return s.encode("utf-8")


def sha256_hex(obj: Any) -> str:
    return hashlib.sha256(canonical_bytes(obj)).hexdigest()


def load_json(path: Path) -> Any:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except Exception as e:
        raise LintError(f"Failed to load JSON: {path}: {e}")


def validate_schema(schema_path: Path, spec_obj: Any) -> None:
    schema_obj = load_json(schema_path)
    validator = Draft202012Validator(schema_obj)
    errors = sorted(validator.iter_errors(spec_obj), key=lambda e: e.path)
    if errors:
        msgs = []
        for err in errors[:50]:
            loc = "$"
            for p in err.path:
                loc += f".{p}" if isinstance(p, str) else f"[{p}]"
            msgs.append(f"{loc}: {err.message}")
        extra = "" if len(errors) <= 50 else f"\n... plus {len(errors)-50} more errors"
        raise LintError("Schema validation failed:\n" + "\n".join(msgs) + extra)


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--framework-dir", required=True, help="Path to framework directory (contains schema.json and optional _defaults.json)")
    ap.add_argument("--spec", required=True, help="Path to a spec JSON file to lint")
    ap.add_argument("--check-hash", action="store_true", help="Fail if integrity.spec_hash exists and does not match computed hash")
    ap.add_argument("--print-hash", action="store_true", help="Print computed hash")
    ap.add_argument("--allow-unresolved-env", action="store_true", help="Do not fail on unresolved ${ENV_VAR} tokens (not recommended)")
    args = ap.parse_args(argv)

    fw_dir = Path(args.framework_dir).resolve()
    spec_path = Path(args.spec).resolve()

    schema_path = fw_dir / "schema.json"
    defaults_path = fw_dir / "_defaults.json"

    if not schema_path.exists():
        raise LintError(f"Missing schema.json in framework dir: {fw_dir}")

    spec_obj = load_json(spec_path)
    # Validate raw spec first (catches unknown fields early)
    validate_schema(schema_path, spec_obj)

    defaults_obj = load_json(defaults_path) if defaults_path.exists() else {}
    effective = deep_merge_defaults(defaults_obj, spec_obj)
    effective = resolve_env_tokens(effective, allow_unresolved=args.allow_unresolved_env)

    computed = sha256_hex(effective)
    if args.print_hash:
        print(computed)

    embedded = None
    if isinstance(spec_obj, dict):
        integrity = spec_obj.get("integrity")
        if isinstance(integrity, dict):
            embedded = integrity.get("spec_hash")

    if args.check_hash and embedded:
        if embedded != computed:
            raise LintError(f"Hash mismatch for {spec_path}\n  embedded: {embedded}\n  computed: {computed}")

    print(f"OK: {spec_path} (computed_hash={computed})")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main(sys.argv[1:]))
    except LintError as e:
        print(f"ERROR: {e}", file=sys.stderr)
        raise SystemExit(2)
