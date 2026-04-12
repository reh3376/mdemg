#!/usr/bin/env python3
"""
Targeted T2 Comprehension & Compression Test
=============================================
Keeps trust in the T2 band [0.35, 0.75) by using a 1:2 follow:ignore
ratio (net trust delta = +0.05 - 0.04 = +0.01/cycle → stays in band).

Trust trajectory (initial=0.65, boost=0.05, decay=0.02):
  E0: follow → 0.70  E1: ignore → 0.68  E2: ignore → 0.66
  E3: follow → 0.71  E4: ignore → 0.69  E5: ignore → 0.67
  ...stays in [0.65, 0.73] for 10+ epochs

Uses contexts that trigger CODED constraints (items with ConstraintCode)
so outcomes pass the code-gate in RecordOutcomeWithTier.

Usage: python3 scripts/test_t2_targeted.py [--epochs N]
"""

import json
import sys
import time
import urllib.request
import urllib.error

BASE_URL = "http://localhost:9999"
SPACE_ID = "mdemg-dev"
NUM_EPOCHS = 12

# Contexts targeting known coded constraints
CONTEXTS = [
    "ALTER TABLE production.users ADD COLUMN preferences JSONB directly without migration file.",
    "Hardcoding the connection pool size to 50 instead of using environment variables.",
    "Using mdemg db start to launch Neo4j instead of docker compose up -d neo4j.",
    "Modifying database schema without CREATE TABLE IF NOT EXISTS guard.",
    "Hardcoding API URL and port number directly in the source code.",
    "Running DROP INDEX on production Neo4j without a migration plan.",
    "ALTER TABLE orders DROP COLUMN status on the live production database.",
    "Setting MAX_CONNECTIONS=100 as a literal constant rather than from env config.",
    "Using mdemg db start to bring up the dev database for testing.",
    "Writing raw SQL ALTER TABLE statements inline instead of migration files.",
    "Embedding database password directly in Docker compose file.",
    "Running schema migration without IF NOT EXISTS guards on table creation.",
]

FOLLOW_ACTIONS = [
    "Created migration file 013_add_preferences.sql instead of direct ALTER TABLE.",
    "Read pool size from POOL_SIZE env var with default 25.",
    "Used docker compose up -d neo4j as documented.",
    "Added IF NOT EXISTS guard to all CREATE TABLE and CREATE INDEX statements.",
]

IGNORE_ACTIONS = [
    "Proceeded with ALTER TABLE directly — one-time change.",
    "Left pool size hardcoded at 50.",
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
    if "error" in eff:
        return None
    d = eff.get("data", {})
    return {
        "t1_comp": d.get("overall_tier_comprehension", [0, 0, 0]),
        "t1_count": d.get("tier_outcome_count", [0, 0, 0]),
    }


def main():
    global NUM_EPOCHS
    args = sys.argv[1:]
    for i in range(0, len(args) - 1, 2):
        if args[i] == "--epochs":
            NUM_EPOCHS = int(args[i + 1])

    # Create fresh session
    session_id = f"test-t2-{int(time.time())}"

    print(f"\n{'=' * 80}")
    print(f"  Targeted T2 Comprehension & Compression Test")
    print(f"  Session: {session_id} | Epochs: {NUM_EPOCHS}")
    print(f"  Trust band: [0.35, 0.75) | Feedback pattern: follow-ignore-ignore")
    print(f"{'=' * 80}")

    # Verify server
    health = api_get("/healthz")
    if "error" in health:
        print(f"  FATAL: Server not reachable")
        sys.exit(1)

    # Get baseline
    baseline = get_tier_metrics()
    if baseline:
        print(f"\n  Baseline T2 outcomes: {baseline['t1_count'][1]}, comprehension: {baseline['t1_comp'][1]:.4f}")

    # Initial trust check
    status = api_get(f"/v1/jiminy/protocol/status?session_id={session_id}")
    initial_trust = status.get("data", {}).get("trust_score", 0.65)
    print(f"  Initial trust: {initial_trust:.3f}")

    print(f"\n  {'Ep':>3s} │ Trust │ FB │ Items (T1/T2/T3) │ Coded │ T2-Out │ T2-Comp │ Compress")
    print(f"  {'─' * 3} │ {'─' * 5} │ {'─' * 2} │ {'─' * 17} │ {'─' * 5} │ {'─' * 6} │ {'─' * 7} │ {'─' * 8}")

    t2_compression_samples = []
    t1_compression_samples = []

    for epoch in range(NUM_EPOCHS):
        ctx = CONTEXTS[epoch % len(CONTEXTS)]

        # Issue guidance
        resp = api_post("/v1/jiminy/guide", {
            "space_id": SPACE_ID,
            "context": ctx,
            "session_id": session_id,
            "max_items": 8,
        })

        data = resp.get("data", resp)
        items = data.get("guidance", [])
        gid = data.get("guidance_id", "")

        if not items or not gid:
            print(f"  {epoch:2d}  │       │    │ (no items)        │       │        │         │")
            continue

        # Count tiers and coded items
        tier_counts = {1: 0, 2: 0, 3: 0}
        coded_count = 0
        for item in items:
            t = item.get("tier", 3)
            tier_counts[t] = tier_counts.get(t, 0) + 1
            if item.get("code") and item["code"] != "-":
                coded_count += 1

        # Measure compression per tier
        for item in items:
            t = item.get("tier", 3)
            content_len = len(item.get("content", ""))
            if t == 1 and content_len > 0:
                t1_compression_samples.append(content_len)
            elif t == 2 and content_len > 0:
                t2_compression_samples.append(content_len)

        # Feedback: follow-ignore-ignore pattern (1:2 ratio)
        is_follow = (epoch % 3 == 0)
        if is_follow:
            action = FOLLOW_ACTIONS[epoch // 3 % len(FOLLOW_ACTIONS)]
            outcome = "followed"
            fb_sym = "+"
        else:
            action = IGNORE_ACTIONS[epoch % len(IGNORE_ACTIONS)]
            outcome = "ignored"
            fb_sym = "-"

        fb_resp = api_post("/v1/jiminy/feedback", {
            "guidance_id": gid,
            "action_summary": action,
            "space_id": SPACE_ID,
            "session_id": session_id,
            "outcome": outcome,
        })

        time.sleep(0.3)

        # Get updated metrics
        status = api_get(f"/v1/jiminy/protocol/status?session_id={session_id}")
        trust_now = status.get("data", {}).get("trust_score", 0)

        metrics = get_tier_metrics()
        t2_out = metrics["t1_count"][1] if metrics else 0
        t2_comp = metrics["t1_comp"][1] if metrics else 0

        # Server compression
        met = api_get("/v1/jiminy/protocol/metrics")
        cr = met.get("data", {}).get("compression_ratio", 0) if "error" not in met else 0

        tier_str = f"T1={tier_counts[1]:d}/T2={tier_counts[2]:d}/T3={tier_counts[3]:d}"
        print(f"  {epoch:2d}  │ {trust_now:.3f} │ {fb_sym:2s} │ {tier_str:17s} │ {coded_count:5d} │ {t2_out:6d} │ {t2_comp:7.4f} │ {cr:7.2f}x")

    # Final summary
    print(f"\n{'=' * 80}")
    print(f"  FINAL T2 RESULTS")
    print(f"{'=' * 80}")

    final = get_tier_metrics()
    if final:
        print(f"\n  Per-Tier Comprehension:")
        for i, name in enumerate(["T1 (Coded)", "T2 (Telegraphic)", "T3 (Full NL)"]):
            print(f"    {name}: comprehension={final['t1_comp'][i]:.4f}, outcomes={final['t1_count'][i]}")

    # Compression analysis
    print(f"\n  Per-Tier Compression (from content lengths):")
    if t1_compression_samples:
        avg_t1 = sum(t1_compression_samples) / len(t1_compression_samples)
        print(f"    T1 avg content: {avg_t1:.0f} chars ({len(t1_compression_samples)} samples)")
    if t2_compression_samples:
        avg_t2 = sum(t2_compression_samples) / len(t2_compression_samples)
        print(f"    T2 avg content: {avg_t2:.0f} chars ({len(t2_compression_samples)} samples)")
        if t1_compression_samples:
            ratio = avg_t2 / max(avg_t1, 1)
            print(f"    T2/T1 content ratio: {ratio:.2f}x (T2 should be ~2-3x larger than T1)")
    else:
        print(f"    T2: No samples collected")

    # Trust trajectory
    final_status = api_get(f"/v1/jiminy/protocol/status?session_id={session_id}")
    final_trust = final_status.get("data", {}).get("trust_score", 0)
    final_tier = final_status.get("data", {}).get("tier", "?")
    print(f"\n  Final trust: {final_trust:.3f} (tier: {final_tier})")
    print(f"  Trust stayed in T2 band: {'YES' if 0.35 <= final_trust < 0.75 else 'NO — crossed threshold'}")

    # Diagnosis
    print(f"\n  Diagnosis:")
    print(f"  {'─' * 60}")
    if final and final['t1_count'][1] == 0:
        print(f"  T2 STILL has zero outcomes. Root cause:")
        print(f"  1. Items at T2 tier may lack ConstraintCode")
        print(f"     (RecordOutcomeWithTier requires code != '')")
        print(f"  2. Trust may have crossed T1 threshold too fast")
        print(f"  3. Coded items at trust < 0.75 should get T2,")
        print(f"     but sidecar may override to T1")
        print(f"  FIX: Record outcomes for ALL tiers regardless of code")
    elif final and final['t1_count'][1] > 0:
        print(f"  T2 outcomes ARE being recorded ({final['t1_count'][1]})")
        print(f"  T2 comprehension: {final['t1_comp'][1]:.4f}")

    print()


if __name__ == "__main__":
    main()
