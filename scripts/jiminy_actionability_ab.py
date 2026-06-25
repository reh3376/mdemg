#!/usr/bin/env python3
"""JIMINY-ACTIONABILITY-001 — surfaced-composition A/B harness.

Drives a fixed set of contexts through POST /v1/jiminy/guide and reports the
surfaced guidance composition: the actionable fraction (constraint/correction)
vs the abstraction class (pattern/learning/concept). Run once per arm
(levers/Lever-C off vs on) and compare the actionable fraction + the type
distribution. The surfaced fraction is what Levers A and C directly move; the
outcome-side follow-rate is the longer (multi-week) signal in constraint_outcomes.

Usage:
  python3 scripts/jiminy_actionability_ab.py --arm A --base-url http://localhost:9999
  python3 scripts/jiminy_actionability_ab.py --arm B --json > /tmp/armB.json

This is the reusable form of the ad-hoc probe used in Epic 4; it is the harness
referenced by the sprint plan's Epic 1 gate.
"""
import argparse
import json
import re
import sys
import urllib.request
from collections import Counter

DEFAULT_QUERIES = [
    "modifying the consolidation pipeline and adding an alert rule",
    "refactoring the retrieval scorer and changing score thresholds",
    "writing a new TSDB migration and bumping the schema version",
    "adding a new LLM call site to the guidance hot path",
    "deleting nodes from the mdemg-dev space during a test",
    "committing directly without running lint or live tests",
]
ACTIONABLE = ("constraint", "correction")


def guide(base_url, space_id, context, session_id, timeout):
    body = json.dumps({"space_id": space_id, "context": context, "session_id": session_id}).encode()
    req = urllib.request.Request(base_url + "/v1/jiminy/guide", data=body,
                                 headers={"Content-Type": "application/json"})
    raw = urllib.request.urlopen(req, timeout=timeout).read().decode("utf-8", "replace")
    # synthesized narrative can carry control chars that break strict JSON
    return json.loads(re.sub(r"[\x00-\x1f]+", " ", raw))


def main():
    ap = argparse.ArgumentParser(description="Jiminy actionability surfaced-composition A/B")
    ap.add_argument("--arm", default="A", help="arm label (e.g. A=levers off, B=on)")
    ap.add_argument("--base-url", default="http://localhost:9999")
    ap.add_argument("--space-id", default="mdemg-dev")
    ap.add_argument("--session-prefix", default="actionability-ab")
    ap.add_argument("--queries-file", help="newline-delimited contexts (default: built-in set)")
    ap.add_argument("--timeout", type=int, default=90)
    ap.add_argument("--json", action="store_true", help="emit a JSON result object")
    args = ap.parse_args()

    queries = DEFAULT_QUERIES
    if args.queries_file:
        with open(args.queries_file) as fh:
            queries = [ln.strip() for ln in fh if ln.strip()]

    tot_act = tot = 0
    dist = Counter()
    per_query = []
    for q in queries:
        try:
            d = guide(args.base_url, args.space_id, q, f"{args.session_prefix}-{args.arm}", args.timeout)
            items = (d.get("data", d) or {}).get("guidance") or []
            c = Counter(i.get("type", "?") for i in items)
            dist += c
            act = sum(c[t] for t in ACTIONABLE)
            n = sum(c.values())
            tot_act += act
            tot += n
            per_query.append({"query": q, "items": n, "actionable": act,
                              "fraction": (act / n if n else 0.0), "dist": dict(c)})
            if not args.json:
                print(f"  q='{q[:42]}' items={n} act={act} frac={act/n if n else 0:.2f} {dict(c)}")
        except Exception as e:  # noqa: BLE001
            per_query.append({"query": q, "error": str(e)[:80]})
            if not args.json:
                print(f"  q='{q[:42]}' ERROR {str(e)[:60]}")

    overall = (tot_act / tot if tot else 0.0)
    result = {"arm": args.arm, "total_items": tot, "actionable": tot_act,
              "overall_actionable_fraction": round(overall, 4),
              "abstraction_fraction": round(1 - overall, 4) if tot else 0.0,
              "distribution": dict(dist), "per_query": per_query}
    if args.json:
        print(json.dumps(result, indent=2))
    else:
        print(f"=== ARM {args.arm}: items={tot} actionable={tot_act} "
              f"overall_frac={overall:.3f} dist={dict(dist)} ===")
    return 0


if __name__ == "__main__":
    sys.exit(main())
