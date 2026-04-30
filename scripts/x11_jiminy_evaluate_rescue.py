"""Phase 11.5e Epic 1 — Rescue jiminy.evaluate + jiminy.evaluate_llm rows from TSDB.

Problem (discovered in 11.5e Epic 0): TSDB rows tagged jiminy.evaluate actually
contain the outcome_classifier system prompt (jiminy.evaluate_llm content), and
vice versa. Production WithContext("jiminy.evaluate", ...) and
WithContext("jiminy.evaluate_llm", ...) call sites are SWAPPED.

This script rescues the rows by content-routing: instead of trusting task_name,
match by SHA256(system_prompt) against the spec's known prompt hashes. The
correct task_name is then assigned by content.

Output format matches valid_clean.jsonl row schema. Then concatenated to
valid_clean.jsonl by Epic 3.

Reuses leak-filter logic from build_clean_eval.py.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
import time
from pathlib import Path
from typing import Any

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import psycopg
import yaml

from neural.benchmarks.run_benchmark import Spec
from scripts.build_clean_eval import LEAKAGE_SOURCES, load_known_user_prompts_per_task, _file_sha256, _cuid


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument('--target-per-task', type=int, default=20)
    ap.add_argument('--out', default='training_data/eval/valid_clean_rescued.jsonl')
    ap.add_argument('--manifest', default='training_data/eval/valid_clean_rescued_manifest.json')
    ap.add_argument('--config', default='configs/benchmark_phase10.yaml')
    ap.add_argument('--repo-root', default='/Users/reh3376/mdemg')
    args = ap.parse_args()

    repo = Path(args.repo_root)
    config = yaml.safe_load((repo / args.config).read_text())
    specs_dir = repo / config['ults']['specs_dir']
    specs_by_task = {Spec.load(p).task_name: Spec.load(p) for p in sorted(specs_dir.glob('*.ults.json'))}

    rescue_tasks = ['jiminy.evaluate', 'jiminy.evaluate_llm']

    # Build hash → correct_task_name routing table from spec hashes
    hash_to_task: dict[str, str] = {}
    for task_name in rescue_tasks:
        spec = specs_by_task[task_name]
        sph = spec.system_prompt_hash
        if isinstance(sph, list):
            for h in sph:
                if h != 'dynamic':
                    hash_to_task[h] = task_name
        elif isinstance(sph, str) and sph != 'dynamic':
            hash_to_task[sph] = task_name

    print(f'Hash → task routing table:')
    for h, t in hash_to_task.items():
        print(f'  {h[:14]} → {t}')
    print()

    # Load leakage sources
    known_per_task, src_meta = load_known_user_prompts_per_task(repo)

    # Connect TSDB
    conn_str = (
        f"host={os.environ.get('TSDB_HOST', 'localhost')} "
        f"port={os.environ.get('TSDB_PORT', '5433')} "
        f"user={os.environ.get('TSDB_USER', 'mdemg')} "
        f"password={os.environ.get('TSDB_PASSWORD', 'mdemg_metrics')} "
        f"dbname={os.environ.get('TSDB_DB', 'mdemg_metrics')}"
    )

    # Pull TSDB rows for both tasks (regardless of label) and let the system_prompt_hash route.
    rescued: dict[str, list[dict]] = {t: [] for t in rescue_tasks}
    stats: dict[str, dict[str, Any]] = {t: {
        'tsdb_total': 0,
        'after_hash_route': 0,
        'after_leak_filter': 0,
        'after_dedup': 0,
        'final': 0,
    } for t in rescue_tasks}

    with psycopg.connect(conn_str) as conn, conn.cursor() as cur:
        # Pull all rows tagged with EITHER task_name (we'll re-route by content hash)
        cur.execute(
            """SELECT user_prompt, response, system_prompt, system_prompt_hash, quality, time, trace_id, task_name
               FROM llm_interactions
               WHERE task_name = ANY(%s)
                 AND response IS NOT NULL AND length(response) > 0
                 AND system_prompt_hash IS NOT NULL
               ORDER BY (CASE WHEN quality IS NULL THEN 0.0 ELSE quality END) DESC,
                        time DESC
               LIMIT 5000""",
            (rescue_tasks,)
        )
        all_rows = cur.fetchall()

    print(f'Total TSDB rows pulled (across both jiminy.* labels): {len(all_rows)}')

    # Route by content hash
    routed: dict[str, list[tuple]] = {t: [] for t in rescue_tasks}
    unrouted = 0
    for row in all_rows:
        user_prompt, response, sysp, sph, quality, ts, trace, original_task = row
        correct_task = hash_to_task.get(sph)
        if correct_task is None:
            unrouted += 1
            continue
        stats[correct_task]['tsdb_total'] += 1
        routed[correct_task].append((user_prompt, response, sysp, sph, quality, ts, trace, original_task))

    print(f'\nUnrouted rows (hash not in spec): {unrouted}')

    # Per task: leak-filter, dedup, sample top N
    for task in rescue_tasks:
        rows = routed[task]
        stats[task]['after_hash_route'] = len(rows)
        spec = specs_by_task[task]

        known = known_per_task.get(task, set())
        # Also leak-check against the OTHER task's known prompts (jiminy.evaluate ↔ jiminy.evaluate_llm)
        other_task = 'jiminy.evaluate_llm' if task == 'jiminy.evaluate' else 'jiminy.evaluate'
        known = known | known_per_task.get(other_task, set())

        # Leak filter
        leak_kept = []
        leak_dropped = 0
        for r in rows:
            if r[0] in known:
                leak_dropped += 1
                continue
            leak_kept.append(r)
        stats[task]['after_leak_filter'] = len(leak_kept)

        # Dedup
        seen: set[str] = set()
        deduped = []
        for r in leak_kept:
            if r[0] in seen:
                continue
            seen.add(r[0])
            deduped.append(r)
        stats[task]['after_dedup'] = len(deduped)

        # Sample top N
        selected = deduped[: args.target_per_task]
        stats[task]['final'] = len(selected)

        for user_prompt, response, sysp, sph, quality, ts, trace, original_task in selected:
            messages = []
            if sysp:
                messages.append({'role': 'system', 'content': sysp})
            messages.append({'role': 'user', 'content': user_prompt})
            if response:
                messages.append({'role': 'assistant', 'content': response})
            rescued[task].append({
                'messages': messages,
                'meta': {
                    'task_name': task,
                    'sampling_group': spec.sampling_group,
                    'source': 'tsdb_relaxed_content_remap',
                    'tsdb_trace_id': trace,
                    'tsdb_time': ts.isoformat() if ts else None,
                    'system_prompt_hash': sph,
                    'quality': float(quality) if quality is not None else None,
                    'original_task_name_in_tsdb': original_task,
                    'remapped_via_content_hash': original_task != task,
                    'extracted_by': 'phase_11_5e.x11_jiminy_evaluate_rescue',
                },
            })

    # Write JSONL
    out_path = repo / args.out
    out_path.parent.mkdir(parents=True, exist_ok=True)
    all_rescued = []
    for t in rescue_tasks:
        all_rescued.extend(rescued[t])
    with out_path.open('w') as fh:
        for r in all_rescued:
            fh.write(json.dumps(r) + '\n')

    # Manifest
    completed_at = time.strftime('%Y-%m-%dT%H:%M:%S%z')
    manifest = {
        'sprint': 'FT-LORA-PHASE11.5e',
        'epic': '1_jiminy_rescue',
        'run_id': _cuid(),
        'completed_at': completed_at,
        'rescue_strategy': 'content-based remap via SHA256(system_prompt) → spec hash table',
        'production_bug_observed': 'jiminy.evaluate ↔ jiminy.evaluate_llm task_name labels are SWAPPED at WithContext() call sites',
        'rescue_tasks': rescue_tasks,
        'hash_routing_table': hash_to_task,
        'total_tsdb_rows_pulled': len(all_rows),
        'unrouted_count': unrouted,
        'per_task': stats,
        'output_jsonl': str(out_path),
        'output_jsonl_sha256': hashlib.sha256(out_path.read_bytes()).hexdigest(),
        'rescued_total': len(all_rescued),
    }
    manifest_path = repo / args.manifest
    manifest_path.write_text(json.dumps(manifest, indent=2, default=str))

    print(f'\n=== Rescue Summary ===')
    for t in rescue_tasks:
        s = stats[t]
        print(f'  {t}: tsdb {s["tsdb_total"]} → leak {s["after_leak_filter"]} → dedup {s["after_dedup"]} → final {s["final"]}')
    print(f'\n  jsonl: {out_path}')
    print(f'  manifest: {manifest_path}')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
