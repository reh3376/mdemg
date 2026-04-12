#!/usr/bin/env python3
"""
Comprehension Optimization Cycle Test
======================================
Iterative test for T1/T2 comprehension + compression.
Target: T1 >= 90%, T2 >= 90% comprehension.

Separate sessions per tier:
- T1: high trust (0.85+), routes to T1 encoding
- T2: medium trust (0.65 initial), routes to T2 encoding
  Uses 1:2 follow:ignore ratio to keep trust in T2 band [0.35, 0.75)

Usage: python3 scripts/test_comprehension_cycle.py [--epochs N]
"""

import json
import sys
import time
import urllib.request
import urllib.error

BASE_URL = "http://localhost:9999"
SPACE_ID = "mdemg-dev"
NUM_EPOCHS = 15

# Contexts targeting known constraints (high match probability)
CONTEXTS = [
    "ALTER TABLE production.users ADD COLUMN preferences JSONB directly without migration file.",
    "Hardcoding the connection pool size to 50 in the source code instead of environment variables.",
    "Using mdemg db start to launch Neo4j instead of docker compose up -d neo4j.",
    "Modifying the database schema to add a column without CREATE TABLE IF NOT EXISTS guard.",
    "Hardcoding API URL and port number directly in the source code.",
    "Running DROP INDEX on production Neo4j without a migration plan.",
    "ALTER TABLE orders DROP COLUMN status on the live production database.",
    "Setting MAX_CONNECTIONS=100 as a literal constant rather than from env config.",
    "Using mdemg db start to bring up the dev database for testing.",
    "Writing raw SQL ALTER TABLE statements inline instead of migration files.",
    "Embedding database password directly in Docker compose file.",
    "Running schema migration without IF NOT EXISTS guards on table creation.",
    "Pushing code changes directly to main branch without a PR from dev branch.",
    "Starting to write code without entering plan mode or creating a plan first.",
    "Deleting MemoryNodes from mdemg-dev space with MATCH (n) DETACH DELETE.",
]

FOLLOW_ACTIONS = [
    "Created migration file 013_add_preferences.sql with IF NOT EXISTS guards.",
    "Read pool size from POOL_SIZE env var with default 25.",
    "Used docker compose up -d neo4j as documented, not mdemg db start.",
    "Added IF NOT EXISTS guard to all CREATE TABLE and CREATE INDEX statements.",
    "Moved API URL to config with env var MDEMG_URL and default localhost:9999.",
    "Created index rebuild migration with LIMIT 5 test first.",
    "Used ALTER TABLE in migration file 014_drop_status.sql, not inline.",
    "Configured max connections via POOL_MAX_CONNECTIONS env var.",
]

IGNORE_ACTIONS = [
    "Proceeded with ALTER TABLE directly for speed.",
    "Left pool size hardcoded at 50 for simplicity.",
    "Used mdemg db start — faster for local dev.",
    "Skipped IF NOT EXISTS — fresh test environment.",
    "Hardcoded URL for now, will refactor later.",
]


def api_get(path):
    try:
        req = urllib.request.Request(f"{BASE_URL}{path}")
        with urllib.request.urlopen(req, timeout=15) as resp:
            return json.loads(resp.read())
    except Exception as e:
        return {"error": str(e)}


def api_post(path, body):
    data = json.dumps(body).encode("utf-8")
    try:
        req = urllib.request.Request(
            f"{BASE_URL}{path}", data=data,
            headers={"Content-Type": "application/json"}, method="POST")
        with urllib.request.urlopen(req, timeout=15) as resp:
            return json.loads(resp.read())
    except urllib.error.HTTPError as e:
        return {"error": f"HTTP {e.code}", "body": e.read().decode("utf-8", errors="replace")}
    except Exception as e:
        return {"error": str(e)}


def get_tier_metrics():
    eff = api_get("/v1/jiminy/protocol/tier-effectiveness")
    met = api_get("/v1/jiminy/protocol/metrics")

    comp = [0.0, 0.0, 0.0]
    counts = [0, 0, 0]
    compression = 0.0
    tier_dist = [0.0, 0.0, 0.0]
    per_code = {}

    if "error" not in eff:
        d = eff.get("data", {})
        comp = d.get("overall_tier_comprehension", comp)
        counts = d.get("tier_outcome_count", counts)

    if "error" not in met:
        d = met.get("data", {})
        compression = d.get("compression_ratio", 0.0)
        tier_dist = d.get("tier_distribution", tier_dist)
        per_code = d.get("tier_code_comprehension", {})

    return {
        "comp": comp,
        "counts": counts,
        "compression": compression,
        "tier_dist": tier_dist,
        "per_code": per_code,
    }


def run_epoch(epoch, session_id, context, follow):
    """Run one guide + feedback cycle."""
    resp = api_post("/v1/jiminy/guide", {
        "space_id": SPACE_ID,
        "context": context,
        "session_id": session_id,
        "max_items": 8,
    })

    data = resp.get("data", resp)
    items = data.get("guidance", [])
    gid = data.get("guidance_id", "")

    if not items or not gid:
        return None

    # Count per tier and coded items
    tier_counts = {1: 0, 2: 0, 3: 0}
    coded = 0
    content_by_tier = {1: [], 2: [], 3: []}
    for item in items:
        t = item.get("tier", 3)
        tier_counts[t] = tier_counts.get(t, 0) + 1
        cc = item.get("constraint_code", "")
        if cc:
            coded += 1
        content_by_tier.setdefault(t, []).append(len(item.get("content", "")))

    # Submit feedback
    if follow:
        action = FOLLOW_ACTIONS[epoch % len(FOLLOW_ACTIONS)]
        outcome = "followed"
    else:
        action = IGNORE_ACTIONS[epoch % len(IGNORE_ACTIONS)]
        outcome = "ignored"

    api_post("/v1/jiminy/feedback", {
        "guidance_id": gid,
        "action_summary": action,
        "space_id": SPACE_ID,
        "session_id": session_id,
        "outcome": outcome,
    })

    return {
        "items": len(items),
        "tiers": tier_counts,
        "coded": coded,
        "content_by_tier": content_by_tier,
        "outcome": outcome,
    }


def run_tier_test(session_id, target_tier, label, follow_pattern):
    """Run NUM_EPOCHS for a single tier."""
    print(f"\n{'─' * 90}")
    print(f"  {label} — Session: {session_id} (target: {target_tier})")
    print(f"  Follow pattern: {follow_pattern}")
    print(f"{'─' * 90}")
    print(f"  {'Ep':>3} │ Trust │ FB │ {'Items':18s} │ Coded │ "
          f"T1c ({' n':>3}) │ T2c ({' n':>3}) │ CR")
    print(f"  {'─'*3} │ {'─'*5} │ {'─'*2} │ {'─'*18} │ {'─'*5} │ "
          f"{'─'*10} │ {'─'*10} │ {'─'*6}")

    compression_samples = {1: [], 2: [], 3: []}

    for i in range(NUM_EPOCHS):
        ctx = CONTEXTS[i % len(CONTEXTS)]
        follow = follow_pattern[i % len(follow_pattern)]

        info = run_epoch(i, session_id, ctx, follow)
        if not info:
            print(f"  {i:3d} │       │    │ (no items)         │       │"
                  f"           │           │")
            continue

        # Accumulate compression samples
        for tier_num in [1, 2, 3]:
            for length in info["content_by_tier"].get(tier_num, []):
                compression_samples[tier_num].append(length)

        time.sleep(0.3)

        # Get trust and metrics
        status = api_get(f"/v1/jiminy/protocol/status?session_id={session_id}")
        trust = status.get("data", {}).get("trust_score", 0)
        m = get_tier_metrics()

        tier_str = "/".join(f"T{k}={v}" for k, v in sorted(info["tiers"].items()) if v > 0)
        fb = "+" if info["outcome"] == "followed" else "-"

        print(f"  {i:3d} │ {trust:.3f} │ {fb:2s} │ {info['items']:2d} [{tier_str:13s}] │ "
              f"{info['coded']:5d} │ "
              f"{m['comp'][0]:.3f} ({m['counts'][0]:3d}) │ "
              f"{m['comp'][1]:.3f} ({m['counts'][1]:3d}) │ "
              f"{m['compression']:.2f}x")

    return compression_samples


def main():
    global NUM_EPOCHS

    args = sys.argv[1:]
    for i in range(0, len(args) - 1, 2):
        if args[i] == "--epochs":
            NUM_EPOCHS = int(args[i + 1])

    ts = int(time.time())
    t1_session = f"cycle1-t1-{ts}"
    t2_session = f"cycle1-t2-{ts}"

    print(f"\n{'=' * 90}")
    print(f"  Comprehension Optimization — Cycle 2")
    print(f"  Target: T1 >= 90%, T2 >= 90%")
    print(f"  Epochs: {NUM_EPOCHS} per tier | Server: {BASE_URL}")
    print(f"{'=' * 90}")

    health = api_get("/healthz")
    if "error" in health:
        print(f"  FATAL: Server not reachable")
        sys.exit(1)

    # Baseline
    baseline = get_tier_metrics()
    print(f"\n  Baseline:")
    print(f"    T1: comp={baseline['comp'][0]:.4f}, outcomes={baseline['counts'][0]}")
    print(f"    T2: comp={baseline['comp'][1]:.4f}, outcomes={baseline['counts'][1]}")
    print(f"    CR: {baseline['compression']:.2f}x")

    # Verify session tiers
    for sid, expected in [(t1_session, "T2"), (t2_session, "T2")]:
        s = api_get(f"/v1/jiminy/protocol/status?session_id={sid}")
        d = s.get("data", {})
        print(f"    {sid}: trust={d.get('trust_score')}, tier={d.get('tier')} (expected: {expected})")

    # Reset metrics before Phase 1
    api_post("/v1/jiminy/protocol/metrics", {})
    print(f"  Metrics reset for Phase 1")

    # Phase 1: T1 test — use all "followed" feedback to push trust into T1 band
    # Start fresh (trust 0.65 = T2), first few epochs build trust to T1 (>0.75)
    # Then remaining epochs test T1 comprehension
    t1_follow = [True] * NUM_EPOCHS  # All followed — trust climbs to T1 quickly
    t1_comp = run_tier_test(t1_session, "T1", "Phase 1: Build trust to T1, then measure T1", t1_follow)

    # Capture Phase 1 results BEFORE Phase 2 contaminates
    p1_metrics = get_tier_metrics()
    print(f"\n  Phase 1 Isolated Results:")
    print(f"    T1: comp={p1_metrics['comp'][0]:.4f} ({p1_metrics['counts'][0]} outcomes)")
    print(f"    T2: comp={p1_metrics['comp'][1]:.4f} ({p1_metrics['counts'][1]} outcomes)")

    # Reset metrics before Phase 2
    api_post("/v1/jiminy/protocol/metrics", {})
    print(f"  Metrics reset for Phase 2")

    # Phase 2: T2 test — 1:2 follow:ignore to keep trust in T2 band
    # Trust trajectory: 0.65 → 0.70 → 0.68 → 0.66 → 0.71 → 0.69 → 0.67 → ...
    t2_follow = [True, False, False] * (NUM_EPOCHS // 3 + 1)
    t2_comp = run_tier_test(t2_session, "T2", "Phase 2: Stabilize trust in T2 band", t2_follow[:NUM_EPOCHS])

    # Capture Phase 2 results
    p2_metrics = get_tier_metrics()
    print(f"\n  Phase 2 Isolated Results:")
    print(f"    T1: comp={p2_metrics['comp'][0]:.4f} ({p2_metrics['counts'][0]} outcomes)")
    print(f"    T2: comp={p2_metrics['comp'][1]:.4f} ({p2_metrics['counts'][1]} outcomes)")

    # Use per-phase results for final summary (not contaminated)
    print(f"\n{'=' * 90}")
    print(f"  CYCLE RESULTS (per-phase isolated)")
    print(f"{'=' * 90}")

    # Use Phase 1 for T1 score, Phase 2 for T2 score (isolated)
    t1_score = p1_metrics["comp"][0]
    t1_n = p1_metrics["counts"][0]
    t2_score_p2 = p2_metrics["comp"][1]
    t2_n_p2 = p2_metrics["counts"][1]
    t2_score_p1 = p1_metrics["comp"][1]
    t2_n_p1 = p1_metrics["counts"][1]

    print(f"\n  ┌──────────────────┬───────────────┬──────────┬─────────────┐")
    print(f"  │ Tier / Phase     │ Comprehension │ Outcomes │ Target      │")
    print(f"  ├──────────────────┼───────────────┼──────────┼─────────────┤")
    t1s = "PASS" if t1_score >= 0.90 else "FAIL"
    print(f"  │ T1 (Phase 1)     │ {t1_score:12.4f} │ {t1_n:8d} │ >=0.90 {t1s:4s}  │")
    if t2_n_p1 > 0:
        t2p1s = "PASS" if t2_score_p1 >= 0.90 else "FAIL"
        print(f"  │ T2 (Phase 1)     │ {t2_score_p1:12.4f} │ {t2_n_p1:8d} │ >=0.90 {t2p1s:4s}  │")
    t2s = "PASS" if t2_score_p2 >= 0.90 else "FAIL"
    print(f"  │ T2 (Phase 2)     │ {t2_score_p2:12.4f} │ {t2_n_p2:8d} │ >=0.90 {t2s:4s}  │")
    print(f"  └──────────────────┴───────────────┴──────────┴─────────────┘")

    # Content length analysis
    print(f"\n  Content Length Analysis:")
    for tier in [1, 2, 3]:
        samples = t1_comp.get(tier, []) + t2_comp.get(tier, [])
        if samples:
            avg = sum(samples) / len(samples)
            print(f"    T{tier}: avg={avg:.0f} chars, n={len(samples)}")

    # Assessment
    print(f"\n  Assessment:")
    print(f"  {'─' * 60}")
    for name, score, n in [("T1", t1_score, t1_n), ("T2", t2_score_p2, t2_n_p2)]:
        if n == 0:
            print(f"  {name}: NO DATA")
        elif score >= 0.90:
            print(f"  {name}: {score:.4f} >= 0.90 — TARGET MET ({n} outcomes)")
        else:
            gap = 0.90 - score
            print(f"  {name}: {score:.4f} < 0.90 — GAP: {gap:.4f} ({n} outcomes)")

    print()
    return 0 if t1_score >= 0.90 and t2_score_p2 >= 0.90 else 1


if __name__ == "__main__":
    sys.exit(main())
