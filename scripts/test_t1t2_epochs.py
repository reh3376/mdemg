#!/usr/bin/env python3
"""
T1/T2 Epoch-Based Comprehension & Compression Benchmark
========================================================
Runs N epochs per tier to measure convergence of comprehension and
compression metrics. Uses separate sessions to isolate T1 vs T2 behavior.

T1 session: claude-core (trust 0.92, routes to T1)
T2 session: test-t2-epoch (trust 0.65, routes to T2)

Each epoch: Guide() → measure compression → Feedback() → measure comprehension
Alternates followed/ignored feedback to test realistic variance.

Usage: python3 scripts/test_t1t2_epochs.py [--epochs N] [--base-url URL]
"""

import json
import sys
import time
import urllib.request
import urllib.error

BASE_URL = "http://localhost:9999"
SPACE_ID = "mdemg-dev"
NUM_EPOCHS = 10

# Sessions locked to different trust bands
T1_SESSION = "claude-core"       # trust 0.92 → T1 tier
T2_SESSION = "test-t2-epoch"     # trust 0.65 → T2 tier

# Contexts that match known constraints in mdemg-dev space
CONTEXTS = [
    "ALTER TABLE production.users ADD COLUMN preferences JSONB directly without migration file.",
    "Hardcoding the connection pool size to 50 in the source code instead of environment variables.",
    "Running DROP INDEX on the production Neo4j database to rebuild graph structure.",
    "Modifying the database schema to add a column for user settings without IF NOT EXISTS guard.",
    "Changing the Docker healthcheck to use a raw TCP probe instead of /healthz endpoint.",
    "Using mdemg db start to launch Neo4j instead of docker compose up -d neo4j.",
    "Pushing code directly to main branch without creating a pull request on dev branch.",
    "Writing 200 lines of code without entering plan mode or creating a development plan first.",
    "Deleting MemoryNodes from the mdemg-dev space using MATCH (n) DETACH DELETE.",
    "Committing .env file with API keys and database credentials to the repository.",
]

# Feedback action summaries — some follow, some ignore (realistic mix)
FOLLOW_ACTIONS = [
    "Applied migration file instead of direct ALTER TABLE. Created 012_add_preferences.sql.",
    "Moved pool size to MDEMG_POOL_SIZE env var with default of 25.",
    "Created index rebuild plan with LIMIT 5 test first, then full execution.",
    "Added IF NOT EXISTS guard to CREATE TABLE and index creation statements.",
    "Kept /healthz endpoint probe, added readiness check at /readyz.",
]

IGNORE_ACTIONS = [
    "Proceeded with ALTER TABLE directly — faster for one-time change.",
    "Left pool size as constant 50 — only one deployment target.",
    "Dropped index without testing — rebuild will fix it.",
    "Skipped IF NOT EXISTS — table is guaranteed fresh in test environment.",
    "Used TCP probe for simplicity — HTTP probe adds latency.",
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
        body_text = e.read().decode("utf-8", errors="replace")
        return {"error": f"HTTP {e.code}", "body": body_text}
    except Exception as e:
        return {"error": str(e)}


def get_tier_metrics():
    """Fetch current tier comprehension and compression from server."""
    eff = api_get("/v1/jiminy/protocol/tier-effectiveness")
    met = api_get("/v1/jiminy/protocol/metrics")

    comp = [0.0, 0.0, 0.0]
    counts = [0, 0, 0]
    compression = 0.0
    tier_dist = [0.0, 0.0, 0.0]

    if "error" not in eff:
        d = eff.get("data", {})
        comp = d.get("overall_tier_comprehension", comp)
        counts = d.get("tier_outcome_count", counts)

    if "error" not in met:
        d = met.get("data", {})
        compression = d.get("compression_ratio", 0.0)
        tier_dist = d.get("tier_distribution", tier_dist)

    return {
        "t1_comprehension": comp[0],
        "t2_comprehension": comp[1],
        "t3_comprehension": comp[2],
        "t1_outcomes": counts[0],
        "t2_outcomes": counts[1],
        "t3_outcomes": counts[2],
        "compression_ratio": compression,
        "tier_dist": tier_dist,
    }


def measure_item_compression(items):
    """Estimate per-tier compression from item content lengths."""
    by_tier = {1: [], 2: [], 3: []}
    for item in items:
        tier = item.get("tier", 3)
        content = item.get("content", "")
        by_tier.setdefault(tier, []).append(len(content))

    result = {}
    # Use T3 average as baseline (or 200 chars if no T3)
    t3_avg = sum(by_tier[3]) / len(by_tier[3]) if by_tier[3] else 200

    for tier in [1, 2, 3]:
        if by_tier[tier]:
            avg_len = sum(by_tier[tier]) / len(by_tier[tier])
            ratio = t3_avg / max(avg_len, 1) if tier < 3 else 1.0
            result[tier] = {"count": len(by_tier[tier]), "avg_chars": avg_len, "est_ratio": ratio}
        else:
            result[tier] = {"count": 0, "avg_chars": 0, "est_ratio": 0}

    return result


def run_epoch(epoch_num, session_id, target_tier, context, follow):
    """Run one guide + feedback cycle. Returns per-item tier info."""
    # Issue guidance
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

    # Count tiers
    tier_counts = {}
    for item in items:
        t = item.get("tier", 3)
        tier_counts[t] = tier_counts.get(t, 0) + 1

    # Measure compression
    comp_info = measure_item_compression(items)

    # Submit feedback
    if follow:
        action = FOLLOW_ACTIONS[epoch_num % len(FOLLOW_ACTIONS)]
        outcome = "followed"
    else:
        action = IGNORE_ACTIONS[epoch_num % len(IGNORE_ACTIONS)]
        outcome = "ignored"

    fb_resp = api_post("/v1/jiminy/feedback", {
        "guidance_id": gid,
        "action_summary": action,
        "space_id": SPACE_ID,
        "session_id": session_id,
        "outcome": outcome,
    })

    fb_data = fb_resp.get("data", fb_resp)
    results = fb_data.get("results", [])

    return {
        "epoch": epoch_num,
        "session": session_id,
        "target_tier": target_tier,
        "item_count": len(items),
        "tier_counts": tier_counts,
        "compression": comp_info,
        "feedback_outcome": outcome,
        "classified_count": len(results),
    }


def print_header():
    print(f"\n{'=' * 80}")
    print(f"  T1/T2 Epoch-Based Comprehension & Compression Benchmark")
    print(f"  Server: {BASE_URL} | Space: {SPACE_ID} | Epochs: {NUM_EPOCHS}")
    print(f"{'=' * 80}")


def print_epoch_row(info, metrics):
    """Print one row of epoch results."""
    tc = info["tier_counts"]
    tier_str = ", ".join(f"T{k}={v}" for k, v in sorted(tc.items()))
    outcome_sym = "+" if info["feedback_outcome"] == "followed" else "-"

    t1c = metrics["t1_comprehension"]
    t2c = metrics["t2_comprehension"]
    t1n = metrics["t1_outcomes"]
    t2n = metrics["t2_outcomes"]

    print(f"  {info['epoch']:2d} │ {info['target_tier']} │ {outcome_sym} │ "
          f"{info['item_count']:2d} items [{tier_str:20s}] │ "
          f"T1: {t1c:.3f} ({t1n:3d}) │ T2: {t2c:.3f} ({t2n:3d}) │ "
          f"CR: {metrics['compression_ratio']:.2f}x")


def run_tier_epochs(session_id, target_tier, label):
    """Run NUM_EPOCHS epochs for a single tier."""
    print(f"\n{'─' * 80}")
    print(f"  {label} — Session: {session_id} (target tier: {target_tier})")
    print(f"{'─' * 80}")
    print(f"  {'Ep':>3s} │ Tier │ FB │ {'Items':22s} │ "
          f"{'T1 Comp (n)':14s} │ {'T2 Comp (n)':14s} │ Compress")
    print(f"  {'─' * 3} │ {'─' * 4} │ {'─' * 2} │ {'─' * 22} │ {'─' * 14} │ {'─' * 14} │ {'─' * 8}")

    epoch_data = []
    per_tier_compression = {1: [], 2: [], 3: []}

    for i in range(NUM_EPOCHS):
        ctx = CONTEXTS[i % len(CONTEXTS)]
        # Follow pattern: 70% follow, 30% ignore (realistic)
        follow = (i % 10) not in [3, 7, 9]

        info = run_epoch(i, session_id, target_tier, ctx, follow)
        if not info:
            print(f"  {i:2d} │ {target_tier} │   │ (no items returned)")
            continue

        # Accumulate per-tier compression samples
        for tier_num in [1, 2, 3]:
            ci = info["compression"][tier_num]
            if ci["count"] > 0:
                per_tier_compression[tier_num].append(ci["est_ratio"])

        time.sleep(0.3)  # Let metrics propagate

        metrics = get_tier_metrics()
        print_epoch_row(info, metrics)
        epoch_data.append({"info": info, "metrics": metrics})

    return epoch_data, per_tier_compression


def print_summary(t1_data, t2_data, t1_comp, t2_comp):
    """Print final summary comparing T1 and T2."""
    print(f"\n{'=' * 80}")
    print(f"  FINAL RESULTS")
    print(f"{'=' * 80}")

    # Get final metrics
    final = get_tier_metrics()

    print(f"\n  Per-Tier Comprehension (server-side, cumulative):")
    print(f"  ┌──────────────────┬───────────────┬──────────┐")
    print(f"  │ Tier             │ Comprehension │ Outcomes │")
    print(f"  ├──────────────────┼───────────────┼──────────┤")
    print(f"  │ T1 (Coded)       │ {final['t1_comprehension']:12.4f} │ {final['t1_outcomes']:8d} │")
    print(f"  │ T2 (Telegraphic) │ {final['t2_comprehension']:12.4f} │ {final['t2_outcomes']:8d} │")
    print(f"  │ T3 (Full NL)     │ {final['t3_comprehension']:12.4f} │ {final['t3_outcomes']:8d} │")
    print(f"  └──────────────────┴───────────────┴──────────┘")

    # Compression per tier (from item content analysis)
    print(f"\n  Per-Tier Compression (estimated from item content lengths):")
    print(f"  ┌──────────────────┬─────────────┬─────────────┬─────────┐")
    print(f"  │ Tier             │ Avg Ratio   │ Samples     │ Quality │")
    print(f"  ├──────────────────┼─────────────┼─────────────┼─────────┤")
    for tier, name in [(1, "T1 (Coded)"), (2, "T2 (Telegraphic)"), (3, "T3 (Full NL)")]:
        # Combine samples from both T1 and T2 sessions
        samples = t1_comp.get(tier, []) + t2_comp.get(tier, [])
        if samples:
            avg = sum(samples) / len(samples)
            quality = "good" if len(samples) >= 5 else "low-n"
            print(f"  │ {name:16s} │ {avg:10.2f}x │ {len(samples):11d} │ {quality:7s} │")
        else:
            print(f"  │ {name:16s} │      — │           0 │    —    │")
    print(f"  └──────────────────┴─────────────┴─────────────┴─────────┘")

    # Server-side compression
    print(f"\n  Server-side blended compression ratio: {final['compression_ratio']:.2f}x")
    print(f"  Tier distribution: T1={final['tier_dist'][0]:.0%}, T2={final['tier_dist'][1]:.0%}, T3={final['tier_dist'][2]:.0%}")

    # Convergence analysis
    print(f"\n  Convergence Analysis:")
    print(f"  {'─' * 60}")

    if t1_data:
        t1_comps = [d["metrics"]["t1_comprehension"] for d in t1_data if d]
        if len(t1_comps) >= 3:
            first_third = sum(t1_comps[:len(t1_comps)//3]) / (len(t1_comps)//3)
            last_third = sum(t1_comps[-len(t1_comps)//3:]) / (len(t1_comps)//3)
            delta = last_third - first_third
            direction = "improving" if delta > 0.02 else "stable" if abs(delta) <= 0.02 else "declining"
            print(f"  T1 comprehension: {first_third:.3f} → {last_third:.3f} (delta: {delta:+.3f}, {direction})")

    if t2_data:
        t2_comps = [d["metrics"]["t2_comprehension"] for d in t2_data if d]
        if len(t2_comps) >= 3:
            first_third = sum(t2_comps[:len(t2_comps)//3]) / (len(t2_comps)//3)
            last_third = sum(t2_comps[-len(t2_comps)//3:]) / (len(t2_comps)//3)
            delta = last_third - first_third
            direction = "improving" if delta > 0.02 else "stable" if abs(delta) <= 0.02 else "declining"
            print(f"  T2 comprehension: {first_third:.3f} → {last_third:.3f} (delta: {delta:+.3f}, {direction})")

    # Recommendations
    print(f"\n  Recommendations:")
    print(f"  {'─' * 60}")
    if final["t1_comprehension"] >= 0.7:
        print(f"  T1: {final['t1_comprehension']:.3f} — GOOD. Above 0.5 gate, bootstrap fix effective.")
    elif final["t1_comprehension"] >= 0.5:
        print(f"  T1: {final['t1_comprehension']:.3f} — ADEQUATE. Above gate, but room to improve.")
    else:
        print(f"  T1: {final['t1_comprehension']:.3f} — LOW. Gate would trigger T1→T2 downgrade.")

    if final["t2_outcomes"] == 0:
        print(f"  T2: No outcomes recorded — tier selection may be bypassing T2.")
    elif final["t2_comprehension"] >= 0.6:
        print(f"  T2: {final['t2_comprehension']:.3f} — GOOD. Telegraphic format is understood.")
    elif final["t2_comprehension"] >= 0.4:
        print(f"  T2: {final['t2_comprehension']:.3f} — MODERATE. Consider bootstrap for T2 too.")
    else:
        print(f"  T2: {final['t2_comprehension']:.3f} — LOW. T2 format may need decoding aid.")

    if final["compression_ratio"] >= 2.5:
        print(f"  Compression: {final['compression_ratio']:.2f}x — GOOD. Context savings significant.")
    elif final["compression_ratio"] >= 1.5:
        print(f"  Compression: {final['compression_ratio']:.2f}x — MODERATE.")
    else:
        print(f"  Compression: {final['compression_ratio']:.2f}x — LOW. Mostly T3 delivery.")

    print()


def main():
    global BASE_URL, NUM_EPOCHS

    # Parse args
    args = sys.argv[1:]
    i = 0
    while i < len(args):
        if args[i] == "--epochs" and i + 1 < len(args):
            NUM_EPOCHS = int(args[i + 1])
            i += 2
        elif args[i] == "--base-url" and i + 1 < len(args):
            BASE_URL = args[i + 1]
            i += 2
        else:
            i += 1

    print_header()

    # Verify server
    health = api_get("/healthz")
    if "error" in health:
        print(f"\n  FATAL: Server not reachable at {BASE_URL}")
        sys.exit(1)
    print(f"  Server: {health.get('status', 'unknown')}")

    # Show baseline
    baseline = get_tier_metrics()
    print(f"\n  Baseline:")
    print(f"    T1 comprehension: {baseline['t1_comprehension']:.4f} ({baseline['t1_outcomes']} outcomes)")
    print(f"    T2 comprehension: {baseline['t2_comprehension']:.4f} ({baseline['t2_outcomes']} outcomes)")
    print(f"    Compression: {baseline['compression_ratio']:.2f}x")

    # Check session trust levels
    for sid, expected_tier in [(T1_SESSION, "T1"), (T2_SESSION, "T2")]:
        status = api_get(f"/v1/jiminy/protocol/status?session_id={sid}")
        data = status.get("data", {})
        trust = data.get("trust_score", 0)
        tier = data.get("tier", "?")
        print(f"    {sid}: trust={trust:.3f}, tier={tier} (target: {expected_tier})")

    # Run T1 epochs
    t1_data, t1_comp = run_tier_epochs(T1_SESSION, "T1", "Phase 1: T1 (Coded) Epochs")

    # Run T2 epochs
    t2_data, t2_comp = run_tier_epochs(T2_SESSION, "T2", "Phase 2: T2 (Telegraphic) Epochs")

    # Final summary
    print_summary(t1_data, t2_data, t1_comp, t2_comp)


if __name__ == "__main__":
    main()
