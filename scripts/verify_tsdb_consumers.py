#!/usr/bin/env python3
"""
verify_tsdb_consumers.py — DORMANT-CENSUS-002 forcing function.

Extends the DORMANT-CENSUS-001 shape (verify_route_consumers.py) to the
TSDB-table surface class. Every table declared by an ``internal/tsdb/migrations/``
file must have an adjudicated entry in docs/api/tsdb_consumer_inventory.json:

  {
    "tables": {
      "<table_name>": {
        "writers": ["<code-path descriptor>", ...],
        "readers": ["<code-path descriptor>", ...],
        "disposition": "IN_USE" | "DORMANT_INTENTIONAL" | "DORMANT_TO_REMOVE",
        "notes": "..."
      }
    }
  }

The script fails on:
  - new tables added to migrations but absent from the inventory
  - inventory entries for removed tables (bidirectional drift)
  - any UNREVIEWED disposition (must adjudicate before merge)

Same shape as `verify_route_consumers.py`; same allowlist pattern via
notes-driven adjudication. Runs in CI alongside the shipped verifiers.
"""

from __future__ import annotations

import json
import os
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
MIGRATIONS_DIR = REPO / "internal" / "tsdb" / "migrations"
INVENTORY_PATH = REPO / "docs" / "api" / "tsdb_consumer_inventory.json"


# Tables carried as "part of the schema" for informational listing only —
# e.g. tsdb_schema_meta is the migration-tracker itself; no application
# code writes/reads it as a data surface.
IMPLICIT_INFRA = {"tsdb_schema_meta"}


def declared_tables() -> set[str]:
    """Grep migrations/*.sql for CREATE TABLE names."""
    pat = re.compile(r"CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z_][a-z0-9_]*)",
                     re.IGNORECASE)
    tables: set[str] = set()
    for f in sorted(MIGRATIONS_DIR.glob("*.sql")):
        for m in pat.finditer(f.read_text()):
            tables.add(m.group(1).lower())
    return tables - IMPLICIT_INFRA


def load_inventory() -> dict:
    if not INVENTORY_PATH.exists():
        return {"tables": {}}
    return json.loads(INVENTORY_PATH.read_text())


def check() -> int:
    live = declared_tables()
    inv = load_inventory().get("tables", {})
    inv_names = set(inv.keys())

    added = live - inv_names
    removed = inv_names - live
    unreviewed = [n for n, v in inv.items()
                  if v.get("disposition", "UNREVIEWED") == "UNREVIEWED"]

    if added:
        print(f"FAIL: {len(added)} table(s) declared in migrations but absent from the inventory:")
        for n in sorted(added):
            print(f"  + {n}")
        print("  -> add entries in docs/api/tsdb_consumer_inventory.json (adjudicate + disposition)")
    if removed:
        print(f"FAIL: {len(removed)} inventory entry/entries for tables no longer in migrations:")
        for n in sorted(removed):
            print(f"  - {n}")
        print("  -> remove from inventory (or restore the migration if the delete was accidental)")
    if unreviewed:
        print(f"FAIL: {len(unreviewed)} inventory entry/entries with disposition=UNREVIEWED:")
        for n in sorted(unreviewed):
            print(f"  ? {n}")
        print("  -> adjudicate: IN_USE / DORMANT_INTENTIONAL / DORMANT_TO_REMOVE")

    if added or removed or unreviewed:
        print(f"\ntables: {len(live)} declared, {len(inv_names)} inventoried; DRIFT")
        return 1
    print(f"tables: {len(live)} declared, {len(inv_names)} inventoried; OK — no drift")
    return 0


def generate() -> int:
    """Rebuild inventory from declared tables, preserving existing adjudications."""
    live = declared_tables()
    existing = load_inventory().get("tables", {})
    new_entries = 0
    for tbl in sorted(live):
        if tbl not in existing:
            existing[tbl] = {
                "writers": [],
                "readers": [],
                "disposition": "UNREVIEWED",
                "notes": "",
            }
            new_entries += 1
    out = {"tables": existing}
    INVENTORY_PATH.parent.mkdir(parents=True, exist_ok=True)
    INVENTORY_PATH.write_text(json.dumps(out, indent=2) + "\n")
    print(f"generated {new_entries} new entries (total {len(existing)}, live tables {len(live)})")
    return 0


def main() -> int:
    if len(sys.argv) > 1 and sys.argv[1] == "--generate":
        return generate()
    return check()


if __name__ == "__main__":
    raise SystemExit(main())
