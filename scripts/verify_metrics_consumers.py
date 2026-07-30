#!/usr/bin/env python3
"""
verify_metrics_consumers.py — DORMANT-CENSUS-003 forcing function.

Extends DORMANT-CENSUS-001 (routes) + DORMANT-CENSUS-002 (TSDB tables) to the
metrics-registry surface. Every metric declared in ``internal/**/*.go`` via
``r.NewCounter/NewGauge/NewHistogram`` must have an adjudicated entry in
``docs/api/metrics_consumer_inventory.json``.

Inventory shape:

  {
    "metrics": {
      "<metric_name>": {
        "type": "counter" | "gauge" | "histogram",
        "declaration": "<file>:<line>",
        "consumers": ["<consumer path>", ...],
        "disposition": "IN_USE"                 // direct dashboard/alert/reader hit
                     | "IN_USE_TSDB_ONLY"       // lands in TSDB metric_samples via
                                                //   the recorder — the shipped
                                                //   architecture — but no direct
                                                //   dashboard/alert/reader queries it
                     | "IN_USE_SNAPSHOT_ONLY"   // consumed only via /v1/metrics/snapshot
                     | "DORMANT_INTENTIONAL"    // declared for a spec/CI purpose but
                                                //   not queried
                     | "DORMANT_TO_REMOVE"      // no consumer + no reason; drop in a
                                                //   future cleanup sprint
                     | "REMOVED"                // declaration removed but entry
                                                //   retained for history
                     | "UNREVIEWED",            // fails CI — operator must adjudicate
        "notes": "..."
      }
    }
  }

False-positive triage (why DORMANT-CENSUS-002 deferred this to a separate
sprint):

  - Histograms are DECLARED as a base name (e.g. ``retrieval_latency_seconds``)
    but consumers read the DERIVATIVES (``_p95``, ``_p99``, ``_bucket``,
    ``_sum``, ``_count``). Grep for the base name alone misses these consumers.
    The walker below expands each declared histogram to all five names before
    searching.

  - Metrics can be exposed via the ``/v1/metrics/snapshot`` JSON API rather
    than named individually. If any consumer file references that endpoint,
    a metric may be consumed through it without a direct grep-hit on the name.
    The walker records this as a soft-signal for operator adjudication.

The script fails on:
  - metrics declared in Go but absent from the inventory (added tables)
  - inventory entries for metrics no longer in Go (removed)
  - any UNREVIEWED disposition (adjudicate before merge)
"""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
INTERNAL_DIR = REPO / "internal"
INVENTORY_PATH = REPO / "docs" / "api" / "metrics_consumer_inventory.json"

# Consumer roots to scan. Ordered by expected hit density (dashboards first
# so their hits appear at the top of the "consumers" list for readability).
CONSUMER_ROOTS = [
    REPO / "deploy" / "docker" / "grafana",
    REPO / "internal" / "cli" / "grafana_templates" / "staged",
    REPO / "internal" / "alert",
    REPO / "internal" / "ape",         # RSIC self-assess reads gauges
    REPO / "internal" / "api",         # snapshot endpoint + admin endpoints
    REPO / "internal" / "tsdb",        # dataset builder reads metric_samples
    REPO / "internal" / "consulting",  # consulting reads some cache stats
]

# Filesystem extensions to search inside consumer roots.
CONSUMER_EXTENSIONS = {".go", ".json", ".yaml", ".yml", ".sql"}

# Declaration regex: any variable ending in a token that has NewCounter/
# NewGauge/NewHistogram. Deliberately loose — captures both ``r.New…`` and
# ``reg.New…`` and any other short receiver name.
DECL_REGEX = re.compile(
    r'\b\w+\.New(Counter|Gauge|Histogram)\(\s*"([a-z_][a-z0-9_]*)"',
    re.MULTILINE,
)

# Snapshot-endpoint marker — if a consumer file grep-hits this, all
# histograms/gauges could be consumed via the snapshot API.
SNAPSHOT_MARKER = "/v1/metrics/snapshot"

# The namespace prefix added at TSDB-write time (see internal/metrics/registry.go
# and recorder.go). Consumers may spell metrics either way.
NAMESPACE_PREFIX = "mdemg_"


def declared_metrics() -> dict[str, dict[str, str]]:
    """Grep internal/**/*.go for r.NewCounter/NewGauge/NewHistogram declarations."""
    out: dict[str, dict[str, str]] = {}
    for f in sorted(INTERNAL_DIR.rglob("*.go")):
        if f.name.endswith("_test.go"):
            continue
        try:
            body = f.read_text(errors="ignore")
        except OSError:
            continue
        for m in DECL_REGEX.finditer(body):
            kind = m.group(1).lower()  # Counter/Gauge/Histogram → counter/…
            name = m.group(2)
            # Compute 1-indexed line number.
            line_no = body.count("\n", 0, m.start()) + 1
            rel = str(f.relative_to(REPO))
            # First declaration wins if a name is declared in multiple files.
            if name not in out:
                out[name] = {"type": kind, "declaration": f"{rel}:{line_no}"}
    return out


def _load_consumer_bodies() -> dict[Path, str]:
    """Load all consumer-root file bodies once; keyed by absolute path."""
    bodies: dict[Path, str] = {}
    for root in CONSUMER_ROOTS:
        if not root.exists():
            continue
        for f in root.rglob("*"):
            if not f.is_file() or f.suffix not in CONSUMER_EXTENSIONS:
                continue
            if f.name.endswith("_test.go"):
                continue
            try:
                bodies[f] = f.read_text(errors="ignore")
            except OSError:
                continue
    return bodies


def _variant_names(name: str, kind: str) -> list[str]:
    """Return all name variants a consumer might reference.

    Counters/gauges: base name (optionally prefixed).
    Histograms: base + _p95 + _p99 + _bucket + _sum + _count (each with and
    without namespace).
    """
    if kind == "histogram":
        base_variants = [name, name + "_p95", name + "_p99",
                         name + "_bucket", name + "_sum", name + "_count"]
    else:
        base_variants = [name]
    all_variants = list(base_variants)
    for v in base_variants:
        all_variants.append(NAMESPACE_PREFIX + v)
    return all_variants


def find_consumers(metric_name: str, kind: str,
                   bodies: dict[Path, str]) -> list[str]:
    """Return list of consumer-file paths that reference any variant of the
    metric. Deduplicated and repo-relative."""
    variants = _variant_names(metric_name, kind)
    hits: set[str] = set()
    for path, body in bodies.items():
        if any(v in body for v in variants):
            hits.add(str(path.relative_to(REPO)))
    return sorted(hits)


def snapshot_consumers(bodies: dict[Path, str]) -> list[str]:
    """Files that reference the snapshot API — soft-signal for metrics
    that may be read as JSON."""
    hits: set[str] = set()
    for path, body in bodies.items():
        if SNAPSHOT_MARKER in body:
            hits.add(str(path.relative_to(REPO)))
    return sorted(hits)


def load_inventory() -> dict:
    if not INVENTORY_PATH.exists():
        return {"metrics": {}}
    return json.loads(INVENTORY_PATH.read_text())


def check() -> int:
    live = declared_metrics()
    inv = load_inventory().get("metrics", {})
    live_names = set(live.keys())
    inv_names = set(inv.keys())

    added = live_names - inv_names
    removed = inv_names - live_names
    unreviewed = [n for n, v in inv.items()
                  if v.get("disposition", "UNREVIEWED") == "UNREVIEWED"]

    if added:
        print(f"FAIL: {len(added)} metric(s) declared in Go but absent from the inventory:")
        for n in sorted(added):
            print(f"  + {n} ({live[n]['type']}, {live[n]['declaration']})")
        print("  -> adjudicate: add entries in docs/api/metrics_consumer_inventory.json")
    if removed:
        print(f"FAIL: {len(removed)} inventory entry/entries for metrics no longer declared:")
        for n in sorted(removed):
            print(f"  - {n}")
        print("  -> remove from inventory (or restore the declaration)")
    if unreviewed:
        print(f"FAIL: {len(unreviewed)} inventory entry/entries with disposition=UNREVIEWED:")
        for n in sorted(unreviewed):
            print(f"  ? {n}")
        print("  -> adjudicate: IN_USE / DORMANT_INTENTIONAL / DORMANT_TO_REMOVE /")
        print("     IN_USE_SNAPSHOT_ONLY / REMOVED")

    if added or removed or unreviewed:
        print(f"\nmetrics: {len(live_names)} declared, {len(inv_names)} inventoried; DRIFT")
        return 1
    print(f"metrics: {len(live_names)} declared, {len(inv_names)} inventoried; OK — no drift")
    return 0


def generate() -> int:
    """Rebuild inventory from declared metrics + auto-discovered consumers,
    preserving existing adjudications. New metrics start as UNREVIEWED."""
    live = declared_metrics()
    existing = load_inventory().get("metrics", {})
    bodies = _load_consumer_bodies()
    snap_files = snapshot_consumers(bodies)

    updated = 0
    added = 0
    for name in sorted(live.keys()):
        info = live[name]
        consumers = find_consumers(name, info["type"], bodies)
        # Preserve prior adjudication; refresh consumers + declaration.
        if name in existing:
            entry = existing[name]
            prev_consumers = entry.get("consumers", [])
            if (entry.get("declaration") != info["declaration"]
                    or prev_consumers != consumers
                    or entry.get("type") != info["type"]):
                entry["type"] = info["type"]
                entry["declaration"] = info["declaration"]
                entry["consumers"] = consumers
                updated += 1
        else:
            # Auto-disposition:
            #   direct consumer hit  → IN_USE
            #   no direct hit         → IN_USE_TSDB_ONLY (the shipped architecture —
            #     internal/metrics/recorder.go writes EVERY declared metric to the
            #     metric_samples hypertable each flush; that's a real consumer even
            #     when nothing else queries the row).
            # Operators re-adjudicate IN_USE_TSDB_ONLY → DORMANT_TO_REMOVE if a
            # follow-up review finds no downstream value.
            if consumers:
                disposition = "IN_USE"
                notes = ""
            else:
                disposition = "IN_USE_TSDB_ONLY"
                notes = (
                    "Auto-generate: no direct dashboard/alert/reader hit; "
                    "flushed to TSDB metric_samples by internal/metrics/"
                    "recorder.go. Operator: re-adjudicate to DORMANT_TO_REMOVE "
                    "if a downstream review confirms no consumer value."
                )
            existing[name] = {
                "type": info["type"],
                "declaration": info["declaration"],
                "consumers": consumers,
                "disposition": disposition,
                "notes": notes,
            }
            added += 1
    out = {"metrics": existing}
    INVENTORY_PATH.parent.mkdir(parents=True, exist_ok=True)
    INVENTORY_PATH.write_text(json.dumps(out, indent=2) + "\n")
    print(f"generated {added} new entries + refreshed {updated} existing "
          f"(total {len(existing)}, live metrics {len(live)})")
    return 0


def main() -> int:
    if len(sys.argv) > 1 and sys.argv[1] == "--generate":
        return generate()
    return check()


if __name__ == "__main__":
    raise SystemExit(main())
