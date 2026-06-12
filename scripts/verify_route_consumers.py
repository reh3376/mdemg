#!/usr/bin/env python3
"""DORMANT-CENSUS-001: merge-blocking route↔consumer inventory gate.

The quarter's bug classes (dead actuators, zero-write tables, no-op
multipliers) share one shape: a surface with no consumer, invisible until
someone looks. This gate makes the looking permanent:

  - extracts the LIVE route table from internal/api/server.go
  - loads docs/api/route_consumer_inventory.json
  - fails on bidirectional drift:
      * a registered route missing from the inventory  -> new surface
        landed without a consumer declaration
      * an inventory entry whose route no longer exists -> stale ledger
  - fails on any entry still marked disposition=UNREVIEWED (the
    bootstrap marker; every route must be adjudicated)

Usage:
  python3 scripts/verify_route_consumers.py            # verify (CI)
  python3 scripts/verify_route_consumers.py --generate # bootstrap missing entries

Dispositions:
  ACTIVE            consumed in production (hooks/cli/mcp/scripts/dashboards/internal)
  OPERATOR_SURFACE  manual escape hatch; intentionally automation-free
  INTERNAL          server-internal or eval/training surface
  DEFERRED:<trigger> future consumer named (e.g. DEFERRED:FT-RECURSIVE-001)
  PRUNED            removed; entry retained for history with removed_in
  UNREVIEWED        bootstrap marker — gate FAILS on these
"""

import json
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
SERVER = REPO / "internal/api/server.go"
INVENTORY = REPO / "docs/api/route_consumer_inventory.json"

ROUTE_RE = re.compile(r'mux\.(?:Handle|HandleFunc)\(\s*"([^"]+)"')


def live_routes() -> set[str]:
    return set(ROUTE_RE.findall(SERVER.read_text()))


def load_inventory() -> dict:
    if not INVENTORY.exists():
        return {"routes": {}}
    return json.loads(INVENTORY.read_text())


def main() -> int:
    generate = "--generate" in sys.argv
    routes = live_routes()
    inv = load_inventory()
    entries = inv.setdefault("routes", {})

    missing = sorted(routes - set(entries))
    stale = sorted(r for r, e in entries.items()
                   if r not in routes and e.get("disposition") != "PRUNED")
    unreviewed = sorted(r for r, e in entries.items()
                        if e.get("disposition") == "UNREVIEWED")

    if generate:
        for r in missing:
            entries[r] = {"consumers": [], "uats_specs": [],
                          "disposition": "UNREVIEWED", "notes": ""}
        INVENTORY.write_text(json.dumps(inv, indent=2, sort_keys=True) + "\n")
        print(f"generated {len(missing)} new entries "
              f"(total {len(entries)}, live routes {len(routes)})")
        return 0

    ok = True
    if missing:
        ok = False
        print(f"FAIL: {len(missing)} route(s) registered but absent from the inventory:")
        for r in missing:
            print(f"  + {r}")
        print("  -> add entries (scripts/verify_route_consumers.py --generate, then adjudicate)")
    if stale:
        ok = False
        print(f"FAIL: {len(stale)} inventory route(s) no longer registered (mark PRUNED or remove):")
        for r in stale:
            print(f"  - {r}")
    if unreviewed:
        ok = False
        print(f"FAIL: {len(unreviewed)} route(s) still UNREVIEWED:")
        for r in unreviewed:
            print(f"  ? {r}")

    print(f"routes: {len(routes)} live, {len(entries)} inventoried; "
          f"{'OK' if ok else 'DRIFT'}")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
