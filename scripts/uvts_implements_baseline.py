#!/usr/bin/env python3
"""UVTS-IMPLEMENTS-BASELINE-001 — recall@K measurement for "who implements X?" queries.

Sprint: docs/development/uvts-implements-baseline-001/
Roadmap: Q5 §3 #10

Purpose:
    Measure whether the shipped Go IMPLEMENTS edges (GO-IMPLEMENTS-001/002,
    190 live on mdemg-dev) improve retrieval quality for interface→implementer
    queries. The RRF graph column already reads IMPLEMENTS via
    SpreadingActivationWithAttention (RETRIEVAL-TYPED-EDGES-002); the
    ship-time A/B ran BEFORE those Go edges existed so this is the honest
    measurement.

Design:
    - Pairs come either from --pairs-file (JSON) OR are fetched live from
      Neo4j via docker exec cypher-shell (if --fetch-pairs is set).
    - For each pair, POST /v1/memory/retrieve with a fixed template query.
    - Grade recall@K: fraction of known-concrete implementer names that
      appear as an exact-name-match OR substring-in-content match in the
      top-K results.
    - Report per-pair + mean.

Usage:
    # A/B: run once with the flag flipped, save output; flip; run again
    RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED=false kickstart mdemg
    python3 scripts/uvts_implements_baseline.py --out off.json --fetch-pairs
    RETRIEVAL_GRAPH_TYPED_EDGES_ENABLED=true kickstart mdemg
    python3 scripts/uvts_implements_baseline.py --out on.json --fetch-pairs
    python3 scripts/uvts_implements_baseline.py --compare off.json on.json

No third-party deps — stdlib urllib + json + subprocess. Runs anywhere
mdemg's HTTP endpoint + docker CLI reach.
"""

from __future__ import annotations

import argparse
import json
import subprocess
import sys
import urllib.error
import urllib.request
from typing import Any

QUERY_TEMPLATE = "types that implement {interface} in this codebase"
DEFAULT_TOP_K = 10
DEFAULT_BASE_URL = "http://localhost:9999"
DEFAULT_SPACE = "mdemg-dev"
DEFAULT_LIMIT = 10  # max pairs to grade


def fetch_pairs_from_neo4j(space_id: str, limit: int) -> list[dict[str, Any]]:
    """Query live Neo4j for the top interface→implementer pairs, ranked by
    concrete count. Uses docker exec cypher-shell for zero-dep footprint."""
    cypher = f"""
MATCH (concrete:SymbolNode)-[:IMPLEMENTS]->(iface:SymbolNode)
WHERE concrete.space_id='{space_id}' AND iface.space_id='{space_id}'
WITH iface, collect(DISTINCT concrete.name) AS impls, collect(DISTINCT concrete.file_path) AS paths
WITH iface, size(impls) AS n_impls, impls, paths
WHERE n_impls >= 2
RETURN iface.name AS interface_name, iface.file_path AS iface_path, n_impls, impls, paths
ORDER BY n_impls DESC LIMIT {limit}
""".strip()
    result = subprocess.run(
        [
            "docker",
            "exec",
            "mdemg-neo4j-1",
            "cypher-shell",
            "-u",
            "neo4j",
            "-p",
            "testpassword",
            "--format",
            "plain",
            cypher,
        ],
        capture_output=True,
        text=True,
        timeout=30,
    )
    if result.returncode != 0:
        raise SystemExit(f"cypher-shell failed: {result.stderr}")
    lines = [ln for ln in result.stdout.split("\n") if ln.strip()]
    if not lines:
        return []
    # Line 0 is the header; skip it. Fields are comma-separated with quoted
    # strings + bracketed lists. Cheap parse: eval the row as Python literal
    # after wrapping in list brackets. Cypher-shell plain format is close
    # enough to Python for the trusted-local case.
    pairs: list[dict[str, Any]] = []
    for line in lines[1:]:
        # Best-effort split. Format: "name", "path", n, [names], [paths]
        # We use a manual scan since commas appear inside the arrays.
        try:
            iface_name, iface_path, rest = line.split(",", 2)
            iface_name = iface_name.strip().strip('"')
            iface_path = iface_path.strip().strip('"')
            # rest = " N, [names], [paths]"
            n_str, arrays = rest.strip().split(",", 1)
            n_impls = int(n_str.strip())
            # arrays = "[names], [paths]" — split on ", [" boundary
            names_str, paths_str = arrays.strip().split(", [", 1)
            names = json.loads(names_str.replace('"', '"'))
            paths = json.loads("[" + paths_str)
            pairs.append(
                {
                    "interface": iface_name,
                    "iface_path": iface_path,
                    "n_impls": n_impls,
                    "impls": names,
                    "paths": paths,
                }
            )
        except (ValueError, json.JSONDecodeError) as e:
            print(f"warn: skipped unparseable row: {line[:80]}... err={e}", file=sys.stderr)
    return pairs


def retrieve(base_url: str, space_id: str, query_text: str, top_k: int) -> list[dict[str, Any]]:
    payload = json.dumps(
        {"space_id": space_id, "query_text": query_text, "top_k": top_k}
    ).encode("utf-8")
    req = urllib.request.Request(
        f"{base_url}/v1/memory/retrieve",
        data=payload,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as r:
            body = r.read().decode("utf-8")
    except urllib.error.HTTPError as e:
        print(f"retrieve HTTP {e.code}: {e.read()[:200]}", file=sys.stderr)
        return []
    d = json.loads(body)
    return d.get("data", {}).get("results") or d.get("results") or []


def grade_pair(pair: dict[str, Any], results: list[dict[str, Any]]) -> dict[str, Any]:
    """Grade recall@K: how many of pair['impls'] names appear as exact-name
    matches OR as substring-in-content-or-path in the top-K results."""
    expected = {n.lower() for n in pair["impls"]}
    strict_hits: set[str] = set()
    lenient_hits: set[str] = set()
    for r in results:
        rname = (r.get("name") or "").lower()
        rcontent = (r.get("content") or "").lower()
        rpath = (r.get("file_path") or r.get("metadata", {}).get("file_path", "") or "").lower()
        for impl in expected:
            impl_lc = impl.lower()
            if rname == impl_lc:
                strict_hits.add(impl_lc)
                lenient_hits.add(impl_lc)
            elif impl_lc in rname or impl_lc in rcontent or impl_lc in rpath:
                lenient_hits.add(impl_lc)
    n_expected = len(expected)
    return {
        "interface": pair["interface"],
        "n_expected": n_expected,
        "strict_hits": len(strict_hits),
        "lenient_hits": len(lenient_hits),
        "strict_recall": len(strict_hits) / n_expected if n_expected else 0.0,
        "lenient_recall": len(lenient_hits) / n_expected if n_expected else 0.0,
        "hits_strict": sorted(strict_hits),
        "hits_lenient": sorted(lenient_hits),
        "expected": sorted(expected),
        "n_results": len(results),
    }


def run(args) -> dict[str, Any]:
    pairs = fetch_pairs_from_neo4j(args.space_id, args.limit) if args.fetch_pairs else json.load(open(args.pairs_file))
    if not pairs:
        raise SystemExit("no pairs to grade")
    print(f"grading {len(pairs)} pairs against {args.base_url} space={args.space_id} top_k={args.top_k}")
    graded = []
    for i, pair in enumerate(pairs, 1):
        q = QUERY_TEMPLATE.format(interface=pair["interface"])
        results = retrieve(args.base_url, args.space_id, q, args.top_k)
        g = grade_pair(pair, results)
        g["query"] = q
        graded.append(g)
        print(
            f"  {i:2d}. {pair['interface']:30s} expected={g['n_expected']:2d} "
            f"strict={g['strict_hits']:2d} ({g['strict_recall']:.2f}) "
            f"lenient={g['lenient_hits']:2d} ({g['lenient_recall']:.2f})"
        )
    mean_strict = sum(g["strict_recall"] for g in graded) / len(graded)
    mean_lenient = sum(g["lenient_recall"] for g in graded) / len(graded)
    report = {
        "config": {
            "base_url": args.base_url,
            "space_id": args.space_id,
            "top_k": args.top_k,
            "pairs_source": "neo4j" if args.fetch_pairs else args.pairs_file,
            "query_template": QUERY_TEMPLATE,
        },
        "n_pairs": len(graded),
        "mean_strict_recall": mean_strict,
        "mean_lenient_recall": mean_lenient,
        "per_pair": graded,
    }
    print(f"\nAGGREGATE: strict={mean_strict:.4f}  lenient={mean_lenient:.4f}")
    return report


def compare(off_file: str, on_file: str) -> None:
    off = json.load(open(off_file))
    on = json.load(open(on_file))
    off_map = {g["interface"]: g for g in off["per_pair"]}
    print(f"\n{'interface':30s} {'OFF strict':>10} {'ON strict':>10} {'Δ strict':>10}   {'OFF lenient':>11} {'ON lenient':>11} {'Δ lenient':>11}")
    print("-" * 110)
    for on_g in on["per_pair"]:
        off_g = off_map.get(on_g["interface"])
        if not off_g:
            continue
        d_strict = on_g["strict_recall"] - off_g["strict_recall"]
        d_lenient = on_g["lenient_recall"] - off_g["lenient_recall"]
        print(
            f"{on_g['interface'][:30]:30s} "
            f"{off_g['strict_recall']:10.4f} {on_g['strict_recall']:10.4f} {d_strict:+10.4f}   "
            f"{off_g['lenient_recall']:11.4f} {on_g['lenient_recall']:11.4f} {d_lenient:+11.4f}"
        )
    ds = on["mean_strict_recall"] - off["mean_strict_recall"]
    dl = on["mean_lenient_recall"] - off["mean_lenient_recall"]
    print("-" * 110)
    print(
        f"{'MEAN':30s} "
        f"{off['mean_strict_recall']:10.4f} {on['mean_strict_recall']:10.4f} {ds:+10.4f}   "
        f"{off['mean_lenient_recall']:11.4f} {on['mean_lenient_recall']:11.4f} {dl:+11.4f}"
    )


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__.strip().split("\n")[0])
    ap.add_argument("--base-url", default=DEFAULT_BASE_URL)
    ap.add_argument("--space-id", default=DEFAULT_SPACE)
    ap.add_argument("--top-k", type=int, default=DEFAULT_TOP_K)
    ap.add_argument("--limit", type=int, default=DEFAULT_LIMIT, help="max pairs to fetch from Neo4j")
    ap.add_argument("--fetch-pairs", action="store_true", help="query Neo4j for interface pairs")
    ap.add_argument("--pairs-file", help="load pairs from JSON file instead of Neo4j")
    ap.add_argument("--out", help="output report JSON file")
    ap.add_argument("--compare", nargs=2, metavar=("OFF", "ON"), help="compare two report files")
    args = ap.parse_args()

    if args.compare:
        compare(args.compare[0], args.compare[1])
        return
    if not args.fetch_pairs and not args.pairs_file:
        ap.error("--fetch-pairs or --pairs-file required")
    report = run(args)
    if args.out:
        with open(args.out, "w") as f:
            json.dump(report, f, indent=2)
        print(f"wrote {args.out}")


if __name__ == "__main__":
    main()
