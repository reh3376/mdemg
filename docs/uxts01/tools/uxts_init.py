#!/usr/bin/env python3
"""
uxts_init.py
Creates a starter UxTS directory layout and a new framework skeleton.

This is deliberately small scaffolding:
- creates uxts/frameworks/<framework_id> with schema.json, _defaults.json, specs/, fixtures/
- drops a minimal example spec into specs/
"""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

COMMON_SCHEMA_REF = "../../schemas/uxts-common.schema.json#/definitions/metadata"

def write_json(path: Path, obj) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(obj, indent=2), encoding="utf-8")


def main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", default=".", help="Repo root directory")
    ap.add_argument("--framework-id", required=True, help="Framework id, e.g. uats, upts, uobs")
    ap.add_argument("--framework-title", default="", help="Human title")
    args = ap.parse_args(argv)

    root = Path(args.root).resolve()
    uxts_dir = root / "uxts"
    fw_dir = uxts_dir / "frameworks" / args.framework_id
    specs_dir = fw_dir / "specs"
    fixtures_dir = fw_dir / "fixtures"

    (uxts_dir / "schemas").mkdir(parents=True, exist_ok=True)
    specs_dir.mkdir(parents=True, exist_ok=True)
    fixtures_dir.mkdir(parents=True, exist_ok=True)

    # Minimal skeleton schema: only metadata + a placeholder payload
    schema = {
      "$schema": "https://json-schema.org/draft/2020-12/schema",
      "title": args.framework_title or f"{args.framework_id.upper()} framework schema",
      "type": "object",
      "required": ["metadata"],
      "properties": {
        "metadata": {"$ref": COMMON_SCHEMA_REF},
        "integrity": {"$ref": "../../schemas/uxts-common.schema.json#/definitions/integrity"},
        "execution": {"$ref": "../../schemas/uxts-common.schema.json#/definitions/execution"},
        "payload": {"type": "object"}
      },
      "additionalProperties": False
    }
    write_json(fw_dir / "schema.json", schema)

    defaults = {"execution": {"timeout_ms": 15000, "execution_mode": "parallel"}}
    write_json(fw_dir / "_defaults.json", defaults)

    example = {
      "metadata": {
        "id": f"{args.framework_id}-example-1",
        "description": "Example spec created by uxts_init.py",
        "tags": ["example"],
        "status": "active"
      },
      "payload": {}
    }
    write_json(specs_dir / f"example.{args.framework_id}.json", example)

    print(f"Created framework skeleton at: {fw_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv[1:]))
