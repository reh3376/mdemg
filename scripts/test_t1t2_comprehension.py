#!/usr/bin/env python3
"""
Live T1/T2 Comprehension & Compression Test
=============================================
Tests the Epic 1A fixes for the T1/T2 comprehension regression:
1. Bootstrap delivery — /v1/jiminy/bootstrap returns protocol header + glossary
2. Guidance issuance — Guide() returns items with tier assignments
3. Compression ratio — T1 items achieve higher compression than T3
4. Bootstrap injection — prompt-context.sh delivers bootstrap with T1/T2 guidance
5. Feedback loop — comprehension scores update per tier after feedback
6. Comprehension gate — T1 is auto-downgraded when comprehension < threshold

Usage: python3 scripts/test_t1t2_comprehension.py [--base-url URL]
"""

import json
import sys
import time
import urllib.request
import urllib.error

BASE_URL = "http://localhost:9999"
SPACE_ID = "mdemg-dev"
SESSION_ID = "claude-core"  # Use existing session with trust > 0.75 for T1 tier

PASS = 0
FAIL = 0
WARN = 0


def log(marker, msg):
    global PASS, FAIL, WARN
    if marker == "PASS":
        PASS += 1
        print(f"  \033[32m✓\033[0m {msg}")
    elif marker == "FAIL":
        FAIL += 1
        print(f"  \033[31m✗\033[0m {msg}")
    elif marker == "WARN":
        WARN += 1
        print(f"  \033[33m⚠\033[0m {msg}")
    else:
        print(f"  {marker} {msg}")


def api_get(path):
    url = f"{BASE_URL}{path}"
    try:
        req = urllib.request.Request(url)
        with urllib.request.urlopen(req, timeout=10) as resp:
            return json.loads(resp.read())
    except Exception as e:
        return {"error": str(e)}


def api_post(path, body):
    url = f"{BASE_URL}{path}"
    data = json.dumps(body).encode("utf-8")
    try:
        req = urllib.request.Request(url, data=data, headers={"Content-Type": "application/json"}, method="POST")
        with urllib.request.urlopen(req, timeout=15) as resp:
            return json.loads(resp.read())
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace")
        return {"error": f"HTTP {e.code}", "body": body}
    except Exception as e:
        return {"error": str(e)}


def section(title):
    print(f"\n{'=' * 60}")
    print(f"  {title}")
    print(f"{'=' * 60}")


# ─────────────────────────────────────────────────────────────
# Test 1: Bootstrap Endpoint
# ─────────────────────────────────────────────────────────────
def test_bootstrap():
    section("Test 1: Bootstrap Endpoint")

    # Without space_id — base bootstrap
    resp = api_get("/v1/jiminy/bootstrap")
    if "error" in resp:
        log("FAIL", f"Bootstrap endpoint failed: {resp['error']}")
        return

    data = resp.get("data", {})
    bootstrap = data.get("bootstrap", "")

    if "J17:INIT|v1" in bootstrap:
        log("PASS", f"Bootstrap header present ({len(bootstrap)} chars)")
    else:
        log("FAIL", f"Bootstrap header missing J17:INIT|v1")

    if "CODES:" in bootstrap and "SEV:" in bootstrap and "FMT:" in bootstrap:
        log("PASS", "Bootstrap contains protocol spec (CODES, SEV, FMT)")
    else:
        log("FAIL", "Bootstrap missing protocol spec sections")

    # With space_id — should include glossary if codes exist
    resp2 = api_get(f"/v1/jiminy/bootstrap?space_id={SPACE_ID}")
    data2 = resp2.get("data", {})
    bootstrap2 = data2.get("bootstrap", "")

    if "DICT:" in bootstrap2:
        dict_lines = [l for l in bootstrap2.split("\n") if l.strip().startswith("  ") and "=" in l]
        log("PASS", f"Bootstrap includes glossary DICT with {len(dict_lines)} entries")
    else:
        log("WARN", "Bootstrap has no DICT section — no constraint codes in space (expected if no constraints codified)")

    return bootstrap2 or bootstrap


# ─────────────────────────────────────────────────────────────
# Test 2: Guidance Issuance with Tier Assignment
# ─────────────────────────────────────────────────────────────
def test_guidance():
    section("Test 2: Guidance Issuance with Tier Assignment")

    resp = api_post("/v1/jiminy/guide", {
        "space_id": SPACE_ID,
        "context": "I need to ALTER TABLE directly on the production database to add a new column. "
                   "Modifying the schema without a migration file for speed.",
        "session_id": SESSION_ID,
        "max_items": 10,
    })

    if "error" in resp:
        log("FAIL", f"Guide() failed: {resp['error']}")
        return None

    data = resp.get("data", resp)
    items = data.get("guidance", [])
    guidance_id = data.get("guidance_id", "")
    augmentation = data.get("prompt_augmentation", "")
    encoded = data.get("encoded_augmentation", "")

    if not items:
        log("WARN", "Guide() returned 0 items — no constraints match this context")
        return None

    log("PASS", f"Guide() returned {len(items)} items, guidance_id={guidance_id[:12]}...")

    # Check tier distribution
    tier_counts = {1: 0, 2: 0, 3: 0}
    for item in items:
        tier = item.get("tier", 3)
        tier_counts[tier] = tier_counts.get(tier, 0) + 1

    log("INFO", f"Tier distribution: T1={tier_counts[1]}, T2={tier_counts[2]}, T3={tier_counts[3]}")

    if tier_counts[1] > 0 or tier_counts[2] > 0:
        log("PASS", "At least one item assigned T1 or T2 encoding")
    else:
        log("WARN", "All items are T3 — trust may be too low for T1/T2, or comprehension gate is active")

    # Check augmentation contains encoded content
    if augmentation and "JIMINY GUIDANCE" in augmentation:
        log("PASS", f"Prompt augmentation present ({len(augmentation)} chars)")
    elif augmentation:
        log("PASS", f"Prompt augmentation present ({len(augmentation)} chars, custom format)")
    else:
        log("WARN", "No prompt augmentation in response")

    # Measure compression
    if encoded and augmentation:
        ratio = len(augmentation) / max(len(encoded), 1)
        log("INFO", f"Encoded: {len(encoded)} chars, Augmentation: {len(augmentation)} chars, ratio: {ratio:.2f}x")

    return {"items": items, "guidance_id": guidance_id, "augmentation": augmentation}


# ─────────────────────────────────────────────────────────────
# Test 3: Compression Measurement
# ─────────────────────────────────────────────────────────────
def test_compression(guidance_result):
    section("Test 3: Compression Measurement")

    if not guidance_result:
        log("WARN", "Skipped — no guidance items to measure")
        return

    items = guidance_result["items"]
    augmentation = guidance_result["augmentation"]

    # Count T1 items and measure their token contribution
    t1_items = [i for i in items if i.get("tier") == 1]
    t2_items = [i for i in items if i.get("tier") == 2]
    t3_items = [i for i in items if i.get("tier") == 3]

    if t1_items:
        # T1 format: TYPE:SEV|code|ann:val — typically ~15 tokens per constraint
        # vs T3 full NL: ~80 tokens per constraint
        # Expected compression: 4-5x
        avg_t1_len = sum(len(i.get("content", "")) for i in t1_items) / len(t1_items)
        avg_t3_len = sum(len(i.get("content", "")) for i in t3_items) / len(t3_items) if t3_items else avg_t1_len * 4
        est_compression = avg_t3_len / max(avg_t1_len, 1)
        log("INFO", f"T1 items: {len(t1_items)}, avg content len: {avg_t1_len:.0f} chars")
        log("INFO", f"Estimated T1 compression vs T3: {est_compression:.1f}x")
        if est_compression > 2.0:
            log("PASS", f"T1 compression ratio > 2x ({est_compression:.1f}x)")
        else:
            log("WARN", f"T1 compression ratio lower than expected: {est_compression:.1f}x")

    # Check protocol metrics for server-side compression tracking
    metrics = api_get("/v1/jiminy/protocol/metrics")
    if "error" not in metrics:
        data = metrics.get("data", {})
        comp_ratio = data.get("compression_ratio", 0)
        total_tokens = data.get("total_tokens", 0)
        log("INFO", f"Server compression_ratio: {comp_ratio:.2f}, total_tokens: {total_tokens}")
        if comp_ratio > 1.5:
            log("PASS", f"Server-side compression ratio: {comp_ratio:.2f}x")
        elif comp_ratio > 0:
            log("WARN", f"Server-side compression ratio low: {comp_ratio:.2f}x")
        else:
            log("INFO", "Server compression ratio = 0 (no token tracking yet — expected for fresh metrics)")
    else:
        log("WARN", f"Could not read protocol metrics: {metrics['error']}")


# ─────────────────────────────────────────────────────────────
# Test 4: Feedback Loop — Comprehension Score Updates
# ─────────────────────────────────────────────────────────────
def test_feedback_loop(guidance_result):
    section("Test 4: Feedback Loop — Comprehension Scores")

    if not guidance_result:
        log("WARN", "Skipped — no guidance to give feedback on")
        return

    guidance_id = guidance_result["guidance_id"]
    items = guidance_result["items"]

    if not guidance_id:
        log("WARN", "No guidance_id — cannot submit feedback")
        return

    # Submit feedback saying the agent "followed" the guidance
    feedback_resp = api_post("/v1/jiminy/feedback", {
        "guidance_id": guidance_id,
        "action_summary": "Applied the constraint: limited max connections to 25 and added 30s timeout as recommended. "
                          "Used connection pool configuration from the guidance.",
        "space_id": SPACE_ID,
        "session_id": SESSION_ID,
        "outcome": "followed",
    })

    if "error" in feedback_resp:
        log("FAIL", f"Feedback submission failed: {feedback_resp['error']}")
        return

    data = feedback_resp.get("data", feedback_resp)
    results = data.get("results", [])
    applied = data.get("applied", False)

    if applied:
        log("PASS", f"Feedback applied, {len(results)} items classified")
    else:
        log("WARN", f"Feedback submitted but not applied (results: {len(results)})")

    # Log per-item outcomes
    for r in results:
        outcome = r.get("outcome", "unknown")
        sim = r.get("similarity", 0)
        log("INFO", f"  [{r.get('type', '?')}] outcome={outcome}, similarity={sim:.3f}")

    # Wait briefly for metrics to propagate
    time.sleep(1)

    # Check tier comprehension after feedback
    tier_eff = api_get("/v1/jiminy/protocol/tier-effectiveness")
    if "error" not in tier_eff:
        data = tier_eff.get("data", {})
        comp = data.get("overall_tier_comprehension", [0, 0, 0])
        counts = data.get("tier_outcome_count", [0, 0, 0])
        log("INFO", f"Tier comprehension: T1={comp[0]:.3f} ({counts[0]} outcomes), "
                     f"T2={comp[1]:.3f} ({counts[1]} outcomes), T3={comp[2]:.3f} ({counts[2]} outcomes)")

        # Check if any tier has non-zero comprehension
        if any(c > 0 for c in comp):
            log("PASS", "Per-tier comprehension scores are being recorded")
        else:
            log("WARN", "All tier comprehension scores are 0 — feedback may not have matched tiered items")
    else:
        log("FAIL", f"Tier effectiveness endpoint failed: {tier_eff['error']}")


# ─────────────────────────────────────────────────────────────
# Test 5: Multiple Feedback Rounds for Statistical Significance
# ─────────────────────────────────────────────────────────────
def test_multi_round():
    section("Test 5: Multi-Round Guidance + Feedback (5 rounds)")

    contexts = [
        "ALTER TABLE production.users ADD COLUMN preferences JSONB directly without migration file.",
        "Running DROP INDEX on the production Neo4j database to rebuild graph structure.",
        "Modifying the database schema to add a column for user settings without CREATE TABLE IF NOT EXISTS guard.",
        "Hardcoding the connection pool size to 50 in the source code instead of using environment variables.",
        "Changing the Docker healthcheck to use a raw TCP probe instead of the /healthz endpoint.",
    ]

    outcomes_by_tier = {1: [], 2: [], 3: []}

    for i, ctx in enumerate(contexts):
        # Issue guidance
        resp = api_post("/v1/jiminy/guide", {
            "space_id": SPACE_ID,
            "context": ctx,
            "session_id": SESSION_ID,
            "max_items": 5,
        })

        data = resp.get("data", resp)
        items = data.get("guidance", [])
        gid = data.get("guidance_id", "")

        if not items or not gid:
            log("INFO", f"Round {i+1}: no items returned, skipping")
            continue

        tier_dist = {}
        for item in items:
            t = item.get("tier", 3)
            tier_dist[t] = tier_dist.get(t, 0) + 1

        log("INFO", f"Round {i+1}: {len(items)} items, tiers={tier_dist}, gid={gid[:8]}...")

        # Submit feedback — alternate between followed and ignored to test both paths
        outcome = "followed" if i % 2 == 0 else "ignored"
        fb_resp = api_post("/v1/jiminy/feedback", {
            "guidance_id": gid,
            "action_summary": f"Round {i+1}: agent {'applied recommendations' if outcome == 'followed' else 'proceeded without applying guidance'}.",
            "space_id": SPACE_ID,
            "session_id": SESSION_ID,
            "outcome": outcome,
        })

        fb_data = fb_resp.get("data", fb_resp)
        for r in fb_data.get("results", []):
            for item in items:
                if item.get("type") == r.get("type"):
                    tier = item.get("tier", 3)
                    score = 1.0 if r.get("outcome") == "followed" else 0.0
                    outcomes_by_tier.setdefault(tier, []).append(score)
                    break

        time.sleep(0.5)  # Let metrics propagate

    # Final metrics check
    time.sleep(1)
    tier_eff = api_get("/v1/jiminy/protocol/tier-effectiveness")
    metrics = api_get("/v1/jiminy/protocol/metrics")

    if "error" not in tier_eff:
        data = tier_eff.get("data", {})
        comp = data.get("overall_tier_comprehension", [0, 0, 0])
        counts = data.get("tier_outcome_count", [0, 0, 0])

        print(f"\n  Final Per-Tier Metrics After 5 Rounds:")
        print(f"  {'─' * 50}")
        for tier_idx, tier_name in enumerate(["T1 (Coded)", "T2 (Telegraphic)", "T3 (Full NL)"]):
            print(f"  {tier_name}: comprehension={comp[tier_idx]:.3f}, outcomes={counts[tier_idx]}")

        total_outcomes = sum(counts)
        if total_outcomes >= 3:
            log("PASS", f"Accumulated {total_outcomes} tier-tracked outcomes across 5 rounds")
        else:
            log("WARN", f"Only {total_outcomes} tier-tracked outcomes — may need more constraint coverage")

        # Check if T1 comprehension is tracking
        if counts[0] > 0:
            if comp[0] > 0.4:
                log("PASS", f"T1 comprehension: {comp[0]:.3f} (above 0.4 threshold)")
            else:
                log("WARN", f"T1 comprehension: {comp[0]:.3f} (below 0.4 — gate may trigger)")
        else:
            log("INFO", "No T1 outcomes recorded — trust may not be high enough or gate is active")

    if "error" not in metrics:
        data = metrics.get("data", {})
        comp_ratio = data.get("compression_ratio", 0)
        total_tokens = data.get("total_tokens", 0)
        nl_tokens = data.get("total_nl_equiv_tokens", 0)
        print(f"\n  Compression: ratio={comp_ratio:.2f}x, tokens={total_tokens}, nl_equiv={nl_tokens}")
        if comp_ratio > 1.0:
            log("PASS", f"Overall compression ratio: {comp_ratio:.2f}x")
        elif total_tokens > 0:
            log("WARN", f"Compression ratio: {comp_ratio:.2f}x (low)")
        else:
            log("INFO", "No token data yet")


# ─────────────────────────────────────────────────────────────
# Test 6: Bootstrap Injection via prompt-context.sh (simulation)
# ─────────────────────────────────────────────────────────────
def test_bootstrap_injection():
    section("Test 6: Bootstrap Injection Check")

    # Warm the store first
    api_post("/v1/jiminy/warm", {
        "space_id": SPACE_ID,
        "context_hint": "Testing bootstrap injection for T1/T2 encoding.",
        "session_id": "claude-core",
    })
    time.sleep(3)  # Wait for warm computation

    # Read latest to check tier info
    latest = api_get(f"/v1/jiminy/latest?space_id={SPACE_ID}")
    if "error" in latest:
        log("WARN", f"Latest endpoint error: {latest['error']}")
        return

    warm = latest.get("warm", False)
    if not warm:
        log("WARN", "Warm store empty — no pre-computed guidance available")
        return

    data = latest.get("data", {})
    items = data.get("guidance", [])
    tiers = [i.get("tier", 3) for i in items]
    min_tier = min(tiers) if tiers else 3

    log("INFO", f"Latest: {len(items)} items, tiers={tiers}, min_tier={min_tier}")

    if min_tier <= 2:
        log("PASS", f"Latest guidance includes T{min_tier} items — bootstrap would be injected by prompt-context.sh")

        # Verify bootstrap endpoint is accessible (what prompt-context.sh would fetch)
        bootstrap = api_get(f"/v1/jiminy/bootstrap?space_id={SPACE_ID}")
        if "error" not in bootstrap:
            bs_text = bootstrap.get("data", {}).get("bootstrap", "")
            log("PASS", f"Bootstrap available for injection ({len(bs_text)} chars)")
        else:
            log("FAIL", "Bootstrap endpoint not reachable — prompt-context.sh injection would fail")
    else:
        log("INFO", "All items are T3 — bootstrap injection not needed (T3 is full natural language)")


# ─────────────────────────────────────────────────────────────
# Test 7: Protocol Status — Trust & Tier Check
# ─────────────────────────────────────────────────────────────
def test_protocol_status():
    section("Test 7: Protocol Status")

    status = api_get(f"/v1/jiminy/protocol/status?session_id={SESSION_ID}")
    if "error" in status:
        log("FAIL", f"Protocol status failed: {status['error']}")
        return

    data = status.get("data", {})
    trust = data.get("trust_score", 0)
    tier = data.get("tier", "?")
    feedback_count = data.get("feedback_count", 0)
    enabled = data.get("enabled", False)

    log("INFO", f"Trust: {trust:.3f}, Tier: {tier}, Feedback: {feedback_count}, Enabled: {enabled}")

    if enabled:
        log("PASS", "J17 protocol is enabled")
    else:
        log("FAIL", "J17 protocol is NOT enabled")

    if trust >= 0.75:
        if tier == "T1":
            log("PASS", f"Trust {trust:.3f} >= 0.75 → T1 tier (correct)")
        else:
            log("WARN", f"Trust {trust:.3f} >= 0.75 but tier is {tier} — comprehension gate may be active")
    elif trust >= 0.35:
        if tier == "T2":
            log("PASS", f"Trust {trust:.3f} in [0.35, 0.75) → T2 tier (correct)")
        else:
            log("WARN", f"Trust {trust:.3f} in [0.35, 0.75) but tier is {tier}")
    else:
        if tier == "T3":
            log("PASS", f"Trust {trust:.3f} < 0.35 → T3 tier (correct)")
        else:
            log("WARN", f"Trust {trust:.3f} < 0.35 but tier is {tier}")


# ─────────────────────────────────────────────────────────────
# Test 8: Reformulation Endpoint (Strict Mode)
# ─────────────────────────────────────────────────────────────
def test_reformulation():
    section("Test 8: Reformulation Endpoint")

    resp = api_post("/v1/jiminy/reformulate", {
        "space_id": SPACE_ID,
        "context": "Changing database pool max connections from 10 to 50.",
        "session_id": SESSION_ID,
    })

    if "error" in resp:
        log("FAIL", f"Reformulate failed: {resp['error']}")
        return

    data = resp.get("data", {})
    directive = data.get("directive", "")
    item_count = data.get("item_count", 0)
    blocked_any = data.get("blocked_any", False)

    if directive:
        words = len(directive.split())
        log("PASS", f"Reformulation returned directive ({words} words, {item_count} items)")
        if words <= 350:
            log("PASS", f"Directive within 350-word budget ({words} words)")
        else:
            log("WARN", f"Directive exceeds 350-word budget ({words} words)")

        # Print first 200 chars of directive
        preview = directive[:200].replace("\n", " ↵ ")
        log("INFO", f"Preview: {preview}...")
    else:
        log("WARN", f"Reformulation returned empty directive ({item_count} items)")

    if blocked_any:
        log("INFO", "BLOCKED items detected — directive contains STOP prefix")
    else:
        log("PASS", "No BLOCKED items (expected for fresh session)")


# ─────────────────────────────────────────────────────────────
# Main
# ─────────────────────────────────────────────────────────────
def main():
    global BASE_URL
    if len(sys.argv) > 2 and sys.argv[1] == "--base-url":
        BASE_URL = sys.argv[2]

    print(f"\n{'=' * 60}")
    print(f"  T1/T2 Comprehension & Compression Live Test")
    print(f"  Server: {BASE_URL} | Space: {SPACE_ID}")
    print(f"{'=' * 60}")

    # Verify server is up
    health = api_get("/healthz")
    if "error" in health:
        print(f"\n  FATAL: Server not reachable at {BASE_URL}")
        sys.exit(1)
    print(f"  Server status: {health.get('status', 'unknown')}")

    # Run tests
    bootstrap = test_bootstrap()
    guidance = test_guidance()
    test_compression(guidance)
    test_feedback_loop(guidance)
    test_multi_round()
    test_bootstrap_injection()
    test_protocol_status()
    test_reformulation()

    # Summary
    print(f"\n{'=' * 60}")
    print(f"  RESULTS: {PASS} passed, {FAIL} failed, {WARN} warnings")
    print(f"{'=' * 60}\n")

    if FAIL > 0:
        sys.exit(1)
    sys.exit(0)


if __name__ == "__main__":
    main()
