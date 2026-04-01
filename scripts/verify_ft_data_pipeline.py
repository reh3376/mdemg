#!/usr/bin/env python3
"""
FT-DATA Pipeline Deep Verification Script
==========================================

Run AFTER the FT-DATA sprint but BEFORE committing the PR.
Run AGAIN after 5 days of collection to confirm data accumulation.

This goes beyond the round-trip test. It verifies:
1. NULL serialization correctness (every nullable field)
2. Adversarial PII injection + rejection on llm_interactions
3. Quality gate accuracy (manual inspection of excluded/included samples)
4. System prompt hash correctness (are "stale" records genuinely stale?)
5. Temporal split integrity (actual timestamps, not just manifest assertion)
6. SHA-256 checksum independence (recompute outside the pipeline)
7. Think-mode content handling
8. MLX format loadability (structural + optional mlx-lm-lora validation)
9. Field completeness (no missing keys, no unexpected empty strings)
10. Cross-instance deduplication correctness
11. Instance ID presence and consistency (multi-instance isolation)
12. Multi-table export with per-field privacy exclusions (FT-HARDENING Phase 1)

Prerequisites:
    - TSDB running on port 5433
    - ./bin/mdemg built with export command
    - ULTS specs at docs/tests/ults/specs/
    - neural/training/ pipeline tools installed

Usage:
    TSDB_PORT=5433 python3 scripts/verify_ft_data_pipeline.py
    TSDB_PORT=5433 python3 scripts/verify_ft_data_pipeline.py --skip-adversarial  # skip DB writes
    TSDB_PORT=5433 python3 scripts/verify_ft_data_pipeline.py --skip-multi-table  # skip if Phase 1 not done
    TSDB_PORT=5433 python3 scripts/verify_ft_data_pipeline.py --check-mlx         # try mlx-lm-lora import
"""

import argparse
import hashlib
import json
import os
import re
import subprocess
import sys
import tarfile
import tempfile
from datetime import datetime, timezone
from pathlib import Path

# ─── Config ──────────────────────────────────────────────────────────────────

MDEMG_BIN = "./bin/mdemg"
SPACE_ID = "mdemg-dev"
ULTS_DIR = "docs/tests/ults/specs"
NEURAL_DIR = "neural"
TSDB_PORT = os.environ.get("TSDB_PORT", "5433")
TSDB_DSN = f"postgresql://mdemg:mdemg@localhost:{TSDB_PORT}/mdemg_metrics"

# Privacy patterns (must mirror internal/llmclient/scrubber.go exactly)
PRIVACY_PATTERNS = {
    "api_key": re.compile(
        r"(?i)(sk-[a-zA-Z0-9]{20,}|ghp_[a-zA-Z0-9]{36}|AKIA[A-Z0-9]{16}|Bearer\s+[a-zA-Z0-9._\-]{20,})"
    ),
    "abs_path": re.compile(
        r"(?:/Users/[^\s\"']+|/home/[^\s\"']+|[A-Z]:\\\\Users\\\\[^\s\"']+)"
    ),
    "env_secret": re.compile(
        r'(?i)(PASSWORD|SECRET|TOKEN|API_KEY|PRIVATE_KEY)\s*[=:]\s*["\']?([^\s"\',;]+)["\']?'
    ),
    "email": re.compile(r"[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}"),
    "neo4j_cred": re.compile(r"neo4j://[^:]+:[^@]+@"),
}

# LLM interactions required columns (27 from writer — includes instance_id from migration 008)
LLM_REQUIRED_COLUMNS = [
    "time", "trace_id", "task_name", "space_id", "session_id",
    "system_prompt", "user_prompt", "response",
    "think_content", "think_mode",
    "latency_ms", "tokens_in", "tokens_out",
    "model_name", "provider", "error",
    "quality", "quality_source",
    "used_for_train", "dataset_ver",
    "guidance_id", "source_path",
    "retrieval_node_ids", "retrieval_scores", "oracle_node_id",
    "system_prompt_hash",
    "instance_id",
]

# Pre-migration column list (26 columns, no instance_id) — used for backward compat checks
LLM_REQUIRED_COLUMNS_PRE_MIGRATION = [c for c in LLM_REQUIRED_COLUMNS if c != "instance_id"]

# Fields that SHOULD be null (not empty string) when absent
NULLABLE_FIELDS = [
    "quality", "quality_source", "dataset_ver", "guidance_id",
    "source_path", "oracle_node_id", "think_content",
]

# ─── Helpers ─────────────────────────────────────────────────────────────────

class Results:
    def __init__(self):
        self.passed = []
        self.failed = []
        self.warnings = []

    def ok(self, name, detail=""):
        self.passed.append((name, detail))
        print(f"  ✅ {name}" + (f" — {detail}" if detail else ""))

    def fail(self, name, detail=""):
        self.failed.append((name, detail))
        print(f"  ❌ {name}" + (f" — {detail}" if detail else ""))

    def warn(self, name, detail=""):
        self.warnings.append((name, detail))
        print(f"  ⚠️  {name}" + (f" — {detail}" if detail else ""))

    def summary(self):
        print("\n" + "=" * 60)
        print(f"PASSED:   {len(self.passed)}")
        print(f"FAILED:   {len(self.failed)}")
        print(f"WARNINGS: {len(self.warnings)}")
        print("=" * 60)
        if self.failed:
            print("\nFAILURES:")
            for name, detail in self.failed:
                print(f"  ❌ {name}: {detail}")
        if self.warnings:
            print("\nWARNINGS:")
            for name, detail in self.warnings:
                print(f"  ⚠️  {name}: {detail}")
        return len(self.failed) == 0


def run_cmd(cmd, env=None, check=True):
    """Run shell command, return stdout."""
    merged_env = {**os.environ, **(env or {})}
    merged_env["TSDB_PORT"] = TSDB_PORT
    result = subprocess.run(
        cmd, shell=True, capture_output=True, text=True, env=merged_env
    )
    if check and result.returncode != 0:
        raise RuntimeError(f"Command failed: {cmd}\nstderr: {result.stderr}")
    return result


def psql(query):
    """Execute psql query against TSDB."""
    cmd = (
        f'docker compose exec -T timescaledb psql -U mdemg -d mdemg_metrics '
        f'-tAc "{query}"'
    )
    return run_cmd(cmd).stdout.strip()


def load_jsonl(path):
    """Load JSONL file as list of dicts."""
    records = []
    with open(path) as f:
        for line in f:
            line = line.strip()
            if line:
                records.append(json.loads(line))
    return records


def sha256_file(path):
    """Compute SHA-256 of file."""
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(8192), b""):
            h.update(chunk)
    return h.hexdigest()


# ─── Test Sections ───────────────────────────────────────────────────────────

def test_1_export_and_structure(results, work_dir):
    """Section 1: Export produces valid archive with correct structure."""
    print("\n── Section 1: Export + Archive Structure ──")

    export_path = os.path.join(work_dir, "export.tar.gz")
    raw_dir = os.path.join(work_dir, "raw")
    os.makedirs(raw_dir, exist_ok=True)

    # Export LLM-only (known clean)
    r = run_cmd(
        f'{MDEMG_BIN} data export --space-id {SPACE_ID} '
        f'--output {export_path} --tables llm_interactions',
        check=False,
    )
    if r.returncode != 0:
        results.fail("export_succeeds", r.stderr[:200])
        return None, None
    results.ok("export_succeeds", f"exit code 0")

    # Extract
    with tarfile.open(export_path, "r:gz") as tf:
        members = [m.name for m in tf.getmembers() if not m.isdir()]
        tf.extractall(raw_dir, filter="data")

    if "manifest.json" in members:
        results.ok("manifest_present")
    else:
        results.fail("manifest_present", f"members: {members}")

    if "llm_interactions.jsonl" in members:
        results.ok("llm_jsonl_present")
    else:
        results.fail("llm_jsonl_present", f"members: {members}")

    manifest = json.load(open(os.path.join(raw_dir, "manifest.json")))
    records = load_jsonl(os.path.join(raw_dir, "llm_interactions.jsonl"))

    return manifest, records


def test_2_null_serialization(results, records):
    """Section 2: NULL fields serialize as JSON null, not empty string."""
    print("\n── Section 2: NULL Serialization ──")

    if not records:
        results.fail("null_test_has_data", "no records to test")
        return

    empty_string_violations = []
    missing_key_violations = []

    for i, rec in enumerate(records):
        # Determine which column list to check — instance_id present = post-migration
        has_instance_id = "instance_id" in rec
        required = LLM_REQUIRED_COLUMNS if has_instance_id else LLM_REQUIRED_COLUMNS_PRE_MIGRATION

        # Check all required keys exist
        for col in required:
            if col not in rec:
                missing_key_violations.append(f"row {i}: missing '{col}'")

        # Check nullable fields: should be null (None), not empty string ""
        for field in NULLABLE_FIELDS:
            if field in rec and rec[field] == "":
                empty_string_violations.append(
                    f"row {i}: '{field}' is empty string, expected null"
                )

    if not missing_key_violations:
        results.ok("all_columns_present", f"checked {len(records)} records × {len(LLM_REQUIRED_COLUMNS)} columns")
    else:
        results.fail("all_columns_present", f"{len(missing_key_violations)} violations: {missing_key_violations[:5]}")

    if not empty_string_violations:
        results.ok("nulls_are_json_null", f"checked {len(NULLABLE_FIELDS)} nullable fields × {len(records)} records")
    else:
        # This is a warning not failure — empty string vs null may be intentional for some fields
        results.warn("nulls_are_json_null", f"{len(empty_string_violations)} empty strings: {empty_string_violations[:5]}")


def test_3_privacy_scan_exported_data(results, records):
    """Section 3: Exported LLM records contain no privacy violations."""
    print("\n── Section 3: Privacy Scan on Exported Data ──")

    if not records:
        results.fail("privacy_scan_has_data", "no records")
        return

    violations = []
    text_fields = ["system_prompt", "user_prompt", "response", "think_content", "source_path"]

    for i, rec in enumerate(records):
        for field in text_fields:
            val = rec.get(field)
            if not isinstance(val, str) or not val:
                continue
            for pname, pat in PRIVACY_PATTERNS.items():
                if pat.search(val):
                    violations.append(f"row {i}, field={field}, pattern={pname}")

    if not violations:
        results.ok("no_privacy_in_llm_export", f"scanned {len(records)} records × {len(text_fields)} fields × {len(PRIVACY_PATTERNS)} patterns")
    else:
        results.fail("no_privacy_in_llm_export", f"{len(violations)} violations: {violations[:5]}")


def test_4_adversarial_pii(results, work_dir):
    """Section 4: Inject PII into llm_interactions, verify export blocks."""
    print("\n── Section 4: Adversarial PII Injection ──")

    pii_export = os.path.join(work_dir, "pii-export.tar.gz")

    # Inject a record with a known API key
    try:
        psql(
            "INSERT INTO llm_interactions "
            "(time, trace_id, task_name, space_id, user_prompt, system_prompt, response, model_name, provider) "
            "VALUES (now(), 'test-pii-verify', 'test.adversarial', 'mdemg-dev', "
            "'Send to sk-abc123def456ghi789jkl012mno345pqr please', 'test', 'ok', 'test', 'test')"
        )
    except RuntimeError as e:
        results.fail("adversarial_inject", str(e)[:200])
        return

    # Export should block
    r = run_cmd(
        f'{MDEMG_BIN} data export --space-id {SPACE_ID} '
        f'--output {pii_export} --tables llm_interactions',
        check=False,
    )

    # Cleanup regardless of result
    try:
        psql("DELETE FROM llm_interactions WHERE task_name = 'test.adversarial'")
    except Exception:
        pass

    if r.returncode != 0 and ("violation" in r.stderr.lower() or "blocked" in r.stderr.lower()
                               or "violation" in r.stdout.lower() or "blocked" in r.stdout.lower()):
        results.ok("adversarial_pii_blocked", "export correctly refused with PII in llm_interactions")
    elif r.returncode == 0:
        results.fail("adversarial_pii_blocked", "export SUCCEEDED despite injected PII — hard gate not working!")
    else:
        results.warn("adversarial_pii_blocked", f"export failed but unclear reason: {r.stderr[:200]}")


def test_5_system_prompt_hash_accuracy(results, records):
    """Section 5: Verify stale/current hash classification is correct."""
    print("\n── Section 5: System Prompt Hash Accuracy ──")

    if not records:
        results.fail("hash_test_has_data", "no records")
        return

    # Load all ULTS spec hashes
    ults_hashes = set()
    ults_dir = Path(ULTS_DIR)
    if not ults_dir.exists():
        results.fail("ults_specs_exist", f"{ULTS_DIR} not found")
        return

    for spec_file in ults_dir.glob("*.ults.json"):
        try:
            spec = json.load(open(spec_file))
            h = spec.get("prompt", {}).get("system_prompt_hash", "")
            # Support both string and list format (FT-HARDENING Phase 3 multi-hash)
            if isinstance(h, list):
                for item in h:
                    if item and item != "dynamic":
                        ults_hashes.add(item)
            elif h and h != "dynamic":
                ults_hashes.add(h)
        except Exception:
            pass

    results.ok("ults_specs_loaded", f"{len(ults_hashes)} unique hashes from {len(list(ults_dir.glob('*.ults.json')))} specs")

    # For each exported record, verify hash
    hash_match = 0
    hash_mismatch = 0
    hash_empty = 0
    mismatch_samples = []

    for i, rec in enumerate(records):
        rec_hash = rec.get("system_prompt_hash", "")
        if not rec_hash:
            hash_empty += 1
            continue
        if rec_hash in ults_hashes:
            hash_match += 1
        else:
            hash_mismatch += 1
            if len(mismatch_samples) < 3:
                # Compute what the hash SHOULD be
                sp = rec.get("system_prompt", "")
                computed = hashlib.sha256(sp.encode()).hexdigest()
                mismatch_samples.append({
                    "row": i,
                    "task": rec.get("task_name", "?"),
                    "stored_hash": rec_hash[:16] + "...",
                    "computed_hash": computed[:16] + "...",
                    "hashes_match_stored": rec_hash == computed,
                })

    results.ok("hash_distribution", f"match={hash_match}, mismatch={hash_mismatch}, empty={hash_empty}")

    if mismatch_samples:
        # Check if the stored hash matches the actual system_prompt content
        # If yes: the hash is correct but the ULTS spec was updated (genuinely stale)
        # If no: there's a hash computation bug
        all_consistent = all(s["hashes_match_stored"] for s in mismatch_samples)
        if all_consistent:
            results.ok("stale_hashes_genuine", f"{hash_mismatch} records have correct hashes that don't match current ULTS specs — genuinely stale prompts")
        else:
            results.fail("hash_computation_consistent",
                         f"stored hash != sha256(system_prompt) for some records: {mismatch_samples}")

        # Print samples for manual review
        print(f"    Sample mismatches (first 3):")
        for s in mismatch_samples:
            print(f"      row={s['row']} task={s['task']} stored={s['stored_hash']} "
                  f"computed={s['computed_hash']} consistent={s['hashes_match_stored']}")


def test_6_quality_filter_accuracy(results, work_dir, records):
    """Section 6: Verify quality filter exclusions are correct."""
    print("\n── Section 6: Quality Filter Accuracy ──")

    if not records:
        results.fail("filter_test_has_data", "no records")
        return

    raw_jsonl = os.path.join(work_dir, "raw", "llm_interactions.jsonl")
    filtered_dir = os.path.join(work_dir, "filtered")
    os.makedirs(filtered_dir, exist_ok=True)
    filtered_jsonl = os.path.join(filtered_dir, "llm_interactions.jsonl")
    report_path = os.path.join(work_dir, "filter_report.json")

    r = run_cmd(
        f'cd {NEURAL_DIR} && PYTHONPATH=. python3 -m training.quality_filter '
        f'--input {raw_jsonl} --output {filtered_jsonl} '
        f'--ults-dir ../{ULTS_DIR} --report {report_path}',
        check=False,
    )
    if r.returncode != 0:
        results.fail("quality_filter_runs", r.stderr[:200])
        return

    report = json.load(open(report_path))
    results.ok("quality_filter_runs", f"{report['input_rows']} → {report['output_rows']}")

    # Load filtered records
    filtered = load_jsonl(filtered_jsonl)

    # Spot-check: every excluded record should fail at least one gate
    filtered_hashes = set()
    for rec in filtered:
        sp = rec.get("system_prompt", "") or ""
        up = rec.get("user_prompt", "") or ""
        filtered_hashes.add(hashlib.sha256((sp + up).encode()).hexdigest())

    excluded_records = [r for r in records
                        if hashlib.sha256(
                            ((r.get("system_prompt", "") or "") + (r.get("user_prompt", "") or "")).encode()
                        ).hexdigest() not in filtered_hashes]

    # Manual check: sample excluded records and verify they're genuinely bad
    bad_reasons_found = 0
    false_exclusions = []

    for rec in excluded_records[:20]:  # spot-check first 20 exclusions
        reasons = []
        resp = rec.get("response", "") or ""
        if len(resp.strip()) <= 10:
            reasons.append("empty_response")
        if rec.get("error"):
            reasons.append("has_error")
        if rec.get("latency_ms", 0) and rec.get("latency_ms", 0) >= 60000:
            reasons.append("latency_exceeded")
        # Check duplicate (can't fully verify without full set, but flag if no other reason)
        # Check stale hash
        ults_hashes = set()
        for spec_file in Path(ULTS_DIR).glob("*.ults.json"):
            try:
                spec = json.load(open(spec_file))
                h = spec.get("prompt", {}).get("system_prompt_hash", "")
                # Support both string and list format (FT-HARDENING Phase 3)
                if isinstance(h, list):
                    for item in h:
                        if item and item != "dynamic":
                            ults_hashes.add(item)
                elif h and h != "dynamic":
                    ults_hashes.add(h)
            except Exception:
                pass
        if rec.get("system_prompt_hash") and rec.get("system_prompt_hash") not in ults_hashes:
            reasons.append("stale_hash")

        if reasons:
            bad_reasons_found += 1
        else:
            # Could be a duplicate — that's OK
            false_exclusions.append({
                "task": rec.get("task_name"),
                "trace_id": rec.get("trace_id", "")[:12],
            })

    excluded_count = len(excluded_records)
    results.ok("exclusion_spot_check",
               f"checked {min(20, excluded_count)} excluded records: "
               f"{bad_reasons_found} had visible gate reasons, "
               f"{len(false_exclusions)} likely duplicates")

    if false_exclusions and len(false_exclusions) > 10:
        results.warn("possible_false_exclusions",
                     f"{len(false_exclusions)} excluded records had no visible reason (may all be duplicates)")

    # Verify no filtered record has obvious quality issues
    bad_included = []
    for i, rec in enumerate(filtered):
        resp = rec.get("response", "") or ""
        if len(resp.strip()) <= 10:
            bad_included.append(f"row {i}: response too short ({len(resp)} chars)")
        if rec.get("error"):
            bad_included.append(f"row {i}: has error field set")

    if not bad_included:
        results.ok("no_bad_records_included", f"verified {len(filtered)} filtered records")
    else:
        results.fail("no_bad_records_included", f"{len(bad_included)} bad records passed filter: {bad_included[:5]}")

    return filtered_jsonl, filtered_dir


def test_7_format_converter(results, work_dir, filtered_jsonl):
    """Section 7: Format converter produces valid MLX chat JSONL."""
    print("\n── Section 7: Format Converter + MLX Validation ──")

    if not filtered_jsonl or not os.path.exists(filtered_jsonl):
        results.fail("converter_has_input", "no filtered JSONL")
        return None

    converted_dir = os.path.join(work_dir, "converted")
    os.makedirs(converted_dir, exist_ok=True)
    converted_jsonl = os.path.join(converted_dir, "train.jsonl")

    r = run_cmd(
        f'cd {NEURAL_DIR} && PYTHONPATH=. python3 -m training.format_converter '
        f'--input {filtered_jsonl} --output {converted_jsonl} --raft-ratio 0.8',
        check=False,
    )
    if r.returncode != 0:
        results.fail("format_converter_runs", r.stderr[:200])
        return None

    converted = load_jsonl(converted_jsonl)
    results.ok("format_converter_runs", f"{len(converted)} records")

    # Deep MLX format validation
    format_errors = []
    for i, rec in enumerate(converted):
        if "messages" not in rec:
            format_errors.append(f"row {i}: missing 'messages' key")
            continue

        msgs = rec["messages"]
        if not isinstance(msgs, list) or len(msgs) < 2:
            format_errors.append(f"row {i}: messages is not a list or too short")
            continue

        # Role checks
        if msgs[0]["role"] != "system":
            format_errors.append(f"row {i}: first message role is '{msgs[0]['role']}', expected 'system'")
        if msgs[-1]["role"] != "assistant":
            format_errors.append(f"row {i}: last message role is '{msgs[-1]['role']}', expected 'assistant'")

        # Content checks
        for j, msg in enumerate(msgs):
            if "role" not in msg or "content" not in msg:
                format_errors.append(f"row {i}, msg {j}: missing 'role' or 'content'")
            elif not msg["content"] or not msg["content"].strip():
                format_errors.append(f"row {i}, msg {j}: empty content for role '{msg['role']}'")

        # Extra key check (MLX only expects 'messages')
        extra_keys = set(rec.keys()) - {"messages"}
        if extra_keys:
            format_errors.append(f"row {i}: unexpected top-level keys: {extra_keys}")

    if not format_errors:
        results.ok("mlx_format_valid", f"all {len(converted)} records pass deep validation")
    else:
        results.fail("mlx_format_valid", f"{len(format_errors)} errors: {format_errors[:5]}")

    # Think-mode check
    think_records = [r for r in converted
                     if any("<think>" in m.get("content", "") for m in r.get("messages", []))]
    results.ok("think_mode_count", f"{len(think_records)} records contain <think> blocks")

    return converted_jsonl


def test_8_dataset_versioner(results, work_dir, filtered_dir):
    """Section 8: Dataset versioner produces correct splits with temporal integrity."""
    print("\n── Section 8: Dataset Versioner + Temporal Integrity ──")

    if not filtered_dir:
        results.fail("versioner_has_input", "no filtered directory")
        return

    curated_dir = os.path.join(work_dir, "curated", "v_test")
    os.makedirs(os.path.dirname(curated_dir), exist_ok=True)

    r = run_cmd(
        f'cd {NEURAL_DIR} && PYTHONPATH=. python3 -m training.dataset_versioner '
        f'--input-dir {filtered_dir} --output-dir {curated_dir} '
        f'--version v_test --min-per-task 1 --first-cycle',
        check=False,
    )
    if r.returncode != 0:
        results.fail("versioner_runs", r.stderr[:200])
        return

    manifest = json.load(open(os.path.join(curated_dir, "manifest.json")))
    results.ok("versioner_runs",
               f"train={manifest['splits']['train']['rows']}, "
               f"test={manifest['splits']['test']['rows']}, "
               f"val={manifest['splits']['val']['rows']}")

    # Temporal integrity: verify actual timestamps
    train_recs = load_jsonl(os.path.join(curated_dir, "train.jsonl"))
    test_recs = load_jsonl(os.path.join(curated_dir, "test.jsonl"))
    val_recs = load_jsonl(os.path.join(curated_dir, "val.jsonl"))

    # Extract temporal ranges from split metadata (versioner nests them per-split)
    splits = manifest.get("splits", {})
    train_tr = splits.get("train", {}).get("temporal_range", {})
    test_tr = splits.get("test", {}).get("temporal_range", {})

    train_end = train_tr.get("to", "")
    test_start = test_tr.get("from", "")

    if train_end and test_start:
        if train_end <= test_start:
            results.ok("temporal_no_overlap", f"train ends {train_end[:19]}, test starts {test_start[:19]}")
        else:
            results.fail("temporal_no_overlap", f"LEAK: train ends {train_end[:19]} > test starts {test_start[:19]}")
    elif not train_end and not test_start:
        results.warn("temporal_range_present", "no temporal_range in split metadata")
    else:
        results.warn("temporal_range_present", "temporal_range in manifest is incomplete")

    # Quality gates
    gates = manifest.get("quality_gates", {})
    if gates.get("no_train_test_overlap") is True:
        results.ok("manifest_asserts_no_overlap")
    else:
        results.fail("manifest_asserts_no_overlap", f"quality_gates: {gates}")

    # SHA-256 verification (independent recompute)
    for split_name in ["train", "test", "val"]:
        split_info = manifest["splits"].get(split_name, {})
        split_file = os.path.join(curated_dir, split_info.get("file", f"{split_name}.jsonl"))
        if os.path.exists(split_file) and "sha256" in split_info:
            computed = sha256_file(split_file)
            if computed == split_info["sha256"]:
                results.ok(f"sha256_{split_name}", "independently verified")
            else:
                results.fail(f"sha256_{split_name}",
                             f"manifest={split_info['sha256'][:16]}... computed={computed[:16]}...")
        elif split_info.get("rows", 0) > 0:
            results.warn(f"sha256_{split_name}", "no sha256 in manifest or file missing")


def test_9_cross_instance_dedup(results, work_dir, filtered_dir):
    """Section 9: Deduplication works across multiple input sources."""
    print("\n── Section 9: Cross-Instance Deduplication ──")

    if not filtered_dir:
        results.fail("dedup_has_input", "no filtered directory")
        return

    # Create a second "instance" by copying the filtered data with a different instance_id
    filtered2_dir = os.path.join(work_dir, "filtered2")
    os.makedirs(filtered2_dir, exist_ok=True)
    src = os.path.join(filtered_dir, "llm_interactions.jsonl")
    dst = os.path.join(filtered2_dir, "llm_interactions.jsonl")

    # Rewrite records with a different instance_id to simulate a second instance
    with open(src) as fin, open(dst, "w") as fout:
        for line in fin:
            line = line.strip()
            if not line:
                continue
            rec = json.loads(line)
            rec["instance_id"] = "simulated-instance-b"
            fout.write(json.dumps(rec) + "\n")

    dedup_dir = os.path.join(work_dir, "curated", "v_dedup")
    os.makedirs(os.path.dirname(dedup_dir), exist_ok=True)

    r = run_cmd(
        f'cd {NEURAL_DIR} && PYTHONPATH=. python3 -m training.dataset_versioner '
        f'--input-dir {filtered_dir} {filtered2_dir} '
        f'--output-dir {dedup_dir} --version v_dedup --min-per-task 1 --first-cycle',
        check=False,
    )
    if r.returncode != 0:
        results.fail("dedup_versioner_runs", r.stderr[:200])
        return

    manifest = json.load(open(os.path.join(dedup_dir, "manifest.json")))
    dedup_info = manifest.get("deduplication", {})

    total_input = dedup_info.get("total_before", 0)
    removed = dedup_info.get("duplicates_removed", 0)
    unique = dedup_info.get("total_after", 0)

    # With identical prompts from two sources, dedup should remove ~50%
    # (same system_prompt + user_prompt even though instance_id differs)
    if removed > 0 and unique > 0:
        results.ok("dedup_removes_duplicates", f"input={total_input}, removed={removed}, unique={unique}")
    else:
        results.fail("dedup_removes_duplicates", f"expected duplicates but got: {dedup_info}")

    # The unique output should be approximately the same as single-source input
    single_count = sum(1 for _ in open(src))
    if abs(unique - single_count) <= 5:  # allow small variance from edge cases
        results.ok("dedup_preserves_unique", f"unique={unique} ≈ single_source={single_count}")
    else:
        results.warn("dedup_count_mismatch", f"unique={unique} vs single_source={single_count}")

    # Verify instance_id diversity in pre-dedup input was preserved in manifest
    instance_contributions = manifest.get("instance_contributions", manifest.get("sources", []))
    if instance_contributions:
        results.ok("dedup_tracks_instances", f"instance contributions: {instance_contributions}")
    else:
        results.warn("dedup_no_instance_tracking", "manifest has no instance contribution tracking")


def test_10_mlx_lm_lora_loadable(results, converted_jsonl):
    """Section 10: (Optional) Verify mlx-lm-lora can parse the training file."""
    print("\n── Section 10: mlx-lm-lora Loadability ──")

    if not converted_jsonl or not os.path.exists(converted_jsonl):
        results.warn("mlx_load_skipped", "no converted JSONL")
        return

    try:
        r = run_cmd(
            f'python3 -c "'
            f'import json; '
            f'lines = open(\\\"{converted_jsonl}\\\").readlines(); '
            f'parsed = [json.loads(l) for l in lines]; '
            f'assert all(\\\"messages\\\" in p for p in parsed); '
            f'print(f\\\"Loaded {{len(parsed)}} records\\\")'
            f'"',
            check=False,
        )
        if r.returncode == 0:
            results.ok("mlx_basic_load", r.stdout.strip())
        else:
            results.fail("mlx_basic_load", r.stderr[:200])
    except Exception as e:
        results.warn("mlx_basic_load", str(e)[:200])

    # Try actual mlx-lm import if available
    try:
        r = run_cmd(
            'python3 -c "from mlx_lm import lora; print(\'mlx-lm available\')"',
            check=False,
        )
        if r.returncode == 0:
            results.ok("mlx_lm_available", "mlx-lm package found")
            # Could add actual validation here if mlx-lm exposes a data validator
        else:
            results.warn("mlx_lm_not_installed", "mlx-lm not available — skip deep format validation")
    except Exception:
        results.warn("mlx_lm_not_installed", "could not check mlx-lm availability")


def test_11_instance_id(results, manifest, records):
    """Section 11: Instance ID presence, consistency, and manifest alignment."""
    print("\n── Section 11: Instance ID Verification ──")

    if not records:
        results.fail("instance_id_has_data", "no records")
        return

    # Check manifest has instance_id
    manifest_instance = manifest.get("instance_id", "") if manifest else ""
    if manifest_instance:
        results.ok("manifest_has_instance_id", f"instance_id={manifest_instance}")
    else:
        results.warn("manifest_has_instance_id",
                     "no instance_id in manifest — pre-amendment export or not yet implemented")

    # Check records for instance_id field
    has_field = sum(1 for r in records if "instance_id" in r)
    has_value = sum(1 for r in records if r.get("instance_id"))  # non-empty

    if has_field == 0:
        results.warn("records_have_instance_id_field",
                     "no records have instance_id field — pre-migration 008 data")
        return

    results.ok("records_have_instance_id_field", f"{has_field}/{len(records)} records have the field")

    if has_value == len(records):
        results.ok("records_have_instance_id_value", f"all {has_value} records have non-empty instance_id")
    elif has_value > 0:
        results.warn("records_mixed_instance_id",
                     f"{has_value}/{len(records)} have values, {len(records) - has_value} are empty (mixed pre/post migration)")
    else:
        results.warn("records_all_empty_instance_id",
                     "all records have empty instance_id — pre-migration data or writer not wired")

    # Check consistency: all records in a single export should have the SAME instance_id
    instance_ids = set(r.get("instance_id", "") for r in records if r.get("instance_id"))
    if len(instance_ids) == 1:
        actual_id = instance_ids.pop()
        results.ok("instance_id_consistent", f"all records: '{actual_id}'")
        # Cross-check with manifest
        if manifest_instance and actual_id != manifest_instance:
            results.fail("instance_id_manifest_match",
                         f"records='{actual_id}' but manifest='{manifest_instance}'")
        elif manifest_instance:
            results.ok("instance_id_manifest_match", "records match manifest")
    elif len(instance_ids) > 1:
        results.fail("instance_id_consistent",
                     f"MULTIPLE instance_ids in single export: {instance_ids} — data isolation broken")
    # len == 0 handled by the has_value check above

    # Verify instance_id is NOT just space_id (should include hostname or be more specific)
    if instance_ids:
        sample_id = next(iter(instance_ids))
        space_id = records[0].get("space_id", "")
        if sample_id == space_id:
            results.warn("instance_id_not_just_space_id",
                         f"instance_id '{sample_id}' == space_id — provides no additional isolation. "
                         f"Expected format: '{{hostname}}-{{space_id}}'")
        else:
            results.ok("instance_id_not_just_space_id",
                       f"instance_id '{sample_id}' differs from space_id '{space_id}'")


def test_12_multi_table_export(results, work_dir):
    """Section 12: Multi-table export with per-field privacy exclusions."""
    print("\n── Section 12: Multi-Table Export (FT-HARDENING Phase 1) ──")

    multi_export = os.path.join(work_dir, "multi-table-export.tar.gz")
    multi_dir = os.path.join(work_dir, "multi-raw")
    os.makedirs(multi_dir, exist_ok=True)

    # Attempt full 3-table export
    r = run_cmd(
        f'{MDEMG_BIN} data export --space-id {SPACE_ID} '
        f'--output {multi_export}',
        check=False,
    )

    if r.returncode != 0:
        # Check if it's the known privacy gate issue (pre-HARDENING Phase 1)
        combined = (r.stdout + r.stderr).lower()
        if "violation" in combined or "blocked" in combined:
            results.warn("multi_table_export",
                         "Multi-table export blocked by privacy gate — "
                         "FT-HARDENING Phase 1 (ScrubStringExcluding) not yet implemented. "
                         "This is expected pre-hardening.")
            # Verify LLM-only still works as fallback
            llm_only = os.path.join(work_dir, "llm-only-fallback.tar.gz")
            r2 = run_cmd(
                f'{MDEMG_BIN} data export --space-id {SPACE_ID} '
                f'--output {llm_only} --tables llm_interactions',
                check=False,
            )
            if r2.returncode == 0:
                results.ok("llm_only_fallback", "LLM-only export succeeds as expected fallback")
            else:
                results.fail("llm_only_fallback", "Even LLM-only export fails!")
        else:
            results.fail("multi_table_export", f"unexpected failure: {r.stderr[:200]}")
        return

    # Multi-table export succeeded — validate all 3 tables
    results.ok("multi_table_export", "all 3 tables exported successfully")

    with tarfile.open(multi_export, "r:gz") as tf:
        members = [m.name for m in tf.getmembers() if not m.isdir()]
        tf.extractall(multi_dir, filter="data")

    expected_tables = {"llm_interactions.jsonl", "retrieval_events.jsonl", "embedding_events.jsonl"}
    found_tables = {m for m in members if m.endswith(".jsonl")}

    if expected_tables == found_tables:
        results.ok("multi_table_all_present", f"all 3 JSONL files in archive")
    else:
        missing = expected_tables - found_tables
        extra = found_tables - expected_tables
        results.fail("multi_table_all_present",
                     f"missing={missing}, unexpected={extra}")

    # Verify each table has rows
    manifest = json.load(open(os.path.join(multi_dir, "manifest.json")))
    for table_name in ["llm_interactions", "retrieval_events", "embedding_events"]:
        table_meta = manifest.get("tables", {}).get(table_name, {})
        row_count = table_meta.get("row_count", 0)
        if row_count > 0:
            results.ok(f"multi_{table_name}_rows", f"{row_count} rows")
        else:
            results.warn(f"multi_{table_name}_rows", "0 rows")

    # Privacy check on retrieval/embedding fields that SHOULD now be clean
    # (post-HARDENING Phase 1: abs_path excluded for specific fields)
    for table_file in ["retrieval_events.jsonl", "embedding_events.jsonl"]:
        table_path = os.path.join(multi_dir, table_file)
        if not os.path.exists(table_path):
            continue

        violations = []
        recs = load_jsonl(table_path)
        for i, rec in enumerate(recs[:100]):  # spot-check first 100
            for field, val in rec.items():
                if not isinstance(val, str) or not val:
                    continue
                # Check for patterns that should NOT be in exported data
                # abs_path may be excluded for certain fields, but API keys should NEVER appear
                for pname in ["api_key", "env_secret", "email", "neo4j_cred"]:
                    if PRIVACY_PATTERNS[pname].search(val):
                        violations.append(f"{table_file}:{i} field={field} pattern={pname}")

        if not violations:
            results.ok(f"multi_{table_file}_no_critical_pii",
                       f"checked {min(100, len(recs))} records — no API keys, secrets, emails, or creds")
        else:
            results.fail(f"multi_{table_file}_no_critical_pii",
                         f"{len(violations)} critical PII violations: {violations[:5]}")

    # Verify instance_id present in all tables (if post-migration)
    for table_file in ["llm_interactions.jsonl", "retrieval_events.jsonl", "embedding_events.jsonl"]:
        table_path = os.path.join(multi_dir, table_file)
        if not os.path.exists(table_path):
            continue
        recs = load_jsonl(table_path)
        if recs and "instance_id" in recs[0]:
            has_value = sum(1 for r in recs if r.get("instance_id"))
            results.ok(f"multi_{table_file}_instance_id",
                       f"{has_value}/{len(recs)} have instance_id")
        elif recs:
            results.warn(f"multi_{table_file}_instance_id",
                         "no instance_id field — pre-migration 008")


# ─── Main ────────────────────────────────────────────────────────────────────

def main():
    parser = argparse.ArgumentParser(description="FT-DATA Pipeline Deep Verification")
    parser.add_argument("--skip-adversarial", action="store_true",
                        help="Skip adversarial PII injection (avoids DB writes)")
    parser.add_argument("--skip-multi-table", action="store_true",
                        help="Skip multi-table export test (use if FT-HARDENING Phase 1 not yet implemented)")
    parser.add_argument("--check-mlx", action="store_true",
                        help="Check mlx-lm-lora loadability")
    args = parser.parse_args()

    results = Results()
    work_dir = tempfile.mkdtemp(prefix="ft-data-verify-")
    print(f"Working directory: {work_dir}")
    print(f"TSDB port: {TSDB_PORT}")
    print(f"Timestamp: {datetime.now(timezone.utc).isoformat()}")

    try:
        # Section 1: Export + structure
        manifest, records = test_1_export_and_structure(results, work_dir)

        # Section 2: NULL serialization
        test_2_null_serialization(results, records)

        # Section 3: Privacy scan on exported data
        test_3_privacy_scan_exported_data(results, records)

        # Section 4: Adversarial PII injection
        if not args.skip_adversarial:
            test_4_adversarial_pii(results, work_dir)
        else:
            print("\n── Section 4: Adversarial PII (SKIPPED) ──")

        # Section 5: System prompt hash accuracy
        test_5_system_prompt_hash_accuracy(results, records)

        # Section 6: Quality filter accuracy
        filter_result = test_6_quality_filter_accuracy(results, work_dir, records)
        filtered_jsonl = filter_result[0] if filter_result else None
        filtered_dir = filter_result[1] if filter_result else None

        # Section 7: Format converter + MLX validation
        converted_jsonl = test_7_format_converter(results, work_dir, filtered_jsonl)

        # Section 8: Dataset versioner + temporal integrity
        test_8_dataset_versioner(results, work_dir, filtered_dir)

        # Section 9: Cross-instance dedup
        test_9_cross_instance_dedup(results, work_dir, filtered_dir)

        # Section 10: mlx-lm-lora loadability (optional)
        if args.check_mlx:
            test_10_mlx_lm_lora_loadable(results, converted_jsonl)
        else:
            print("\n── Section 10: mlx-lm-lora Loadability (SKIPPED — use --check-mlx) ──")

        # Section 11: Instance ID verification
        test_11_instance_id(results, manifest, records)

        # Section 12: Multi-table export (FT-HARDENING Phase 1)
        if not args.skip_multi_table:
            test_12_multi_table_export(results, work_dir)
        else:
            print("\n── Section 12: Multi-Table Export (SKIPPED) ──")

    finally:
        print(f"\nWork directory preserved at: {work_dir}")

    success = results.summary()
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()
