"""Phase 11.5c Epic 1 — Build leak-free clean eval JSONL from TSDB.

For each ULTS spec:
- Query llm_interactions by task_name + system_prompt_hash (spec hashes)
- Subtract user_prompts that appear in any known train/valid source (leakage)
- De-dup by user_prompt (highest-quality first occurrence)
- Sample top N (target 20)
- Emit JSONL with messages array compatible with neural/benchmarks/run_benchmark.match_rows_for_spec

Output:
- training_data/eval/valid_clean.jsonl
- training_data/eval/valid_clean_manifest.json (per-task stats, source SHAs, leakage drops)

Usage:
  python scripts/build_clean_eval.py [--target-per-task N] [--out PATH] [--config PATH]

Env:
  TSDB_HOST/TSDB_PORT/TSDB_USER/TSDB_PASSWORD/TSDB_DB (read from .env)
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import sys
import time
from dataclasses import dataclass
from pathlib import Path
from typing import Any

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import yaml  # noqa: E402

from neural.benchmarks.run_benchmark import Spec  # noqa: E402

LEAKAGE_SOURCES = [
    'training_data/sft/tier1/train.jsonl',
    'training_data/sft/tier1/valid.jsonl',
    'training_data/sft/family_classify_notink/train.jsonl',
    'training_data/sft/family_classify_notink/valid.jsonl',
    'training_data/sft/family_reasoning_think/train.jsonl',
    'training_data/sft/family_reasoning_think/valid.jsonl',
    'training_data/sft/family_structured_notink/train.jsonl',
    'training_data/sft/family_structured_notink/valid.jsonl',
    'training_data/eval/valid_golden.jsonl',
]


def _cuid() -> str:
    """CUIDv2 for run_id (MEMORY: never UUID)."""
    try:
        from cuid2 import Cuid  # type: ignore[import-not-found]
        return Cuid(length=24).generate()
    except ImportError:
        return hashlib.blake2b(str(time.time_ns()).encode(), digest_size=12).hexdigest()


def _file_sha256(path: Path) -> str | None:
    if not path.exists():
        return None
    h = hashlib.sha256()
    h.update(path.read_bytes())
    return h.hexdigest()


def load_known_user_prompts_per_task(repo_root: Path) -> tuple[dict[str, set[str]], dict[str, dict]]:
    """Build per-task set of user_prompts already in any train/valid source.

    Returns (known_prompts, source_metadata).
    """
    known: dict[str, set[str]] = {}
    src_meta: dict[str, dict] = {}
    for src in LEAKAGE_SOURCES:
        full = repo_root / src
        meta = {
            'sha256': _file_sha256(full),
            'rows': 0,
            'bytes': full.stat().st_size if full.exists() else 0,
        }
        if not full.exists():
            src_meta[src] = meta
            continue
        with full.open() as fh:
            for line in fh:
                line = line.strip()
                if not line:
                    continue
                try:
                    row = json.loads(line)
                except json.JSONDecodeError:
                    continue
                meta['rows'] += 1
                m = row.get('meta') or {}
                task = m.get('task_name') or m.get('task') or row.get('task_id')
                if not task:
                    continue
                # Extract user_prompt from messages
                user = ''
                for msg in row.get('messages') or []:
                    if msg.get('role') == 'user':
                        user = msg.get('content', '')
                        break
                if user:
                    known.setdefault(task, set()).add(user)
        src_meta[src] = meta
    return known, src_meta


def extract_for_spec(
    cur,
    spec: Spec,
    known_prompts: set[str],
    target_n: int,
) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    """Extract leak-free rows for one spec; return (rows, stats)."""
    sph = spec.system_prompt_hash
    if isinstance(sph, list):
        hashes = [h for h in sph if h != 'dynamic']
    elif isinstance(sph, str) and sph != 'dynamic':
        hashes = [sph]
    else:
        hashes = []  # dynamic — skip strict match

    stats: dict[str, Any] = {
        'task_name': spec.task_name,
        'sampling_group': spec.sampling_group,
        'spec_hashes': hashes,
        'tsdb_rows': 0,
        'tsdb_distinct_prompts': 0,
        'after_leak_filter': 0,
        'after_dedup': 0,
        'final': 0,
        'leakage_drops_per_source_unknown': True,
        'data_starved': False,
        'mode': 'strict_hash' if hashes else 'task_name_only',
    }

    if not hashes:
        # Dynamic spec — no strict hash match. We fall back to task_name only.
        cur.execute(
            """SELECT user_prompt, response, system_prompt, system_prompt_hash, quality, time, trace_id
               FROM llm_interactions
               WHERE task_name = %s AND response IS NOT NULL AND length(response) > 0
               ORDER BY (CASE WHEN quality IS NULL THEN 0.0 ELSE quality END) DESC,
                        time DESC
               LIMIT 1000""",
            (spec.task_name,)
        )
    else:
        cur.execute(
            """SELECT user_prompt, response, system_prompt, system_prompt_hash, quality, time, trace_id
               FROM llm_interactions
               WHERE task_name = %s AND system_prompt_hash = ANY(%s)
                 AND response IS NOT NULL AND length(response) > 0
               ORDER BY (CASE WHEN quality IS NULL THEN 0.0 ELSE quality END) DESC,
                        time DESC
               LIMIT 1000""",
            (spec.task_name, hashes)
        )
    rows = cur.fetchall()
    stats['tsdb_rows'] = len(rows)
    stats['tsdb_distinct_prompts'] = len({r[0] for r in rows})

    # Leak filter
    leak_kept = []
    leak_dropped = 0
    for user_prompt, response, sysp, hash_, quality, ts, trace in rows:
        if user_prompt in known_prompts:
            leak_dropped += 1
            continue
        leak_kept.append((user_prompt, response, sysp, hash_, quality, ts, trace))
    stats['after_leak_filter'] = len(leak_kept)
    stats['leak_dropped'] = leak_dropped

    # De-dup by user_prompt (keep first = highest quality due to ORDER BY)
    seen: set[str] = set()
    deduped = []
    for r in leak_kept:
        if r[0] in seen:
            continue
        seen.add(r[0])
        deduped.append(r)
    stats['after_dedup'] = len(deduped)

    # Sample top N
    selected = deduped[:target_n]
    stats['final'] = len(selected)
    stats['data_starved'] = len(selected) < target_n

    # Build JSONL rows
    out_rows = []
    for user_prompt, response, sysp, hash_, quality, ts, trace in selected:
        # Build messages array
        messages = []
        if sysp:
            messages.append({'role': 'system', 'content': sysp})
        messages.append({'role': 'user', 'content': user_prompt})
        if response:
            messages.append({'role': 'assistant', 'content': response})
        out_rows.append({
            'messages': messages,
            'meta': {
                'task_name': spec.task_name,
                'sampling_group': spec.sampling_group,
                'source': 'tsdb',
                'tsdb_trace_id': trace,
                'tsdb_time': ts.isoformat() if ts else None,
                'system_prompt_hash': hash_,
                'quality': float(quality) if quality is not None else None,
                'extracted_by': 'phase_11_5c.build_clean_eval',
            },
        })
    return out_rows, stats


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument('--target-per-task', type=int, default=20)
    ap.add_argument('--out', default='training_data/eval/valid_clean.jsonl')
    ap.add_argument('--manifest', default='training_data/eval/valid_clean_manifest.json')
    ap.add_argument('--config', default='configs/benchmark_phase10.yaml')
    ap.add_argument('--repo-root', default='/Users/reh3376/mdemg')
    args = ap.parse_args()

    repo = Path(args.repo_root)
    config = yaml.safe_load((repo / args.config).read_text())
    specs_dir = repo / config['ults']['specs_dir']
    if not specs_dir.is_absolute():
        specs_dir = repo / config['ults']['specs_dir']
    specs = [Spec.load(p) for p in sorted(specs_dir.glob('*.ults.json'))]

    print(f'Loaded {len(specs)} ULTS specs from {specs_dir}')

    # Load known prompts (leakage source)
    print(f'Loading {len(LEAKAGE_SOURCES)} leakage sources...')
    known_per_task, src_meta = load_known_user_prompts_per_task(repo)
    for src, meta in src_meta.items():
        print(f'  {src}: {meta["rows"]} rows, sha={meta["sha256"][:12] if meta["sha256"] else "MISSING"}')

    # Connect TSDB
    import psycopg
    conn_str = (
        f"host={os.environ.get('TSDB_HOST', 'localhost')} "
        f"port={os.environ.get('TSDB_PORT', '5433')} "
        f"user={os.environ.get('TSDB_USER', 'mdemg')} "
        f"password={os.environ.get('TSDB_PASSWORD', 'mdemg_metrics')} "
        f"dbname={os.environ.get('TSDB_DB', 'mdemg_metrics')}"
    )

    out_rows: list[dict[str, Any]] = []
    per_task_stats: list[dict[str, Any]] = []

    print(f'\nExtracting clean eval (target {args.target_per_task} per task)...')
    print(f'{"Task":32s} {"Mode":18s} {"TSDB":>6s} {"AfterLeak":>10s} {"Dedup":>6s} {"Final":>6s}')
    print(f'{"-"*32} {"-"*18} {"-"*6} {"-"*10} {"-"*6} {"-"*6}')

    with psycopg.connect(conn_str) as conn:
        with conn.cursor() as cur:
            for spec in specs:
                known = known_per_task.get(spec.task_name, set())
                rows, stats = extract_for_spec(cur, spec, known, args.target_per_task)
                out_rows.extend(rows)
                per_task_stats.append(stats)
                print(
                    f'{spec.task_name:32s} '
                    f'{stats["mode"]:18s} '
                    f'{stats["tsdb_rows"]:>6} '
                    f'{stats["after_leak_filter"]:>10} '
                    f'{stats["after_dedup"]:>6} '
                    f'{stats["final"]:>6}'
                    f'{" ⚠STARVED" if stats["data_starved"] else ""}'
                )

    # Write JSONL
    out_path = repo / args.out
    out_path.parent.mkdir(parents=True, exist_ok=True)
    with out_path.open('w') as fh:
        for row in out_rows:
            fh.write(json.dumps(row) + '\n')

    # Manifest
    starved = [s['task_name'] for s in per_task_stats if s['data_starved']]
    manifest = {
        'sprint': 'FT-LORA-PHASE11.5c',
        'epic': '1_extraction',
        'run_id': _cuid(),
        'built_at': time.strftime('%Y-%m-%dT%H:%M:%S%z'),
        'target_per_task': args.target_per_task,
        'specs_total': len(specs),
        'rows_total': len(out_rows),
        'tasks_starved': starved,
        'tasks_well_supplied': [s['task_name'] for s in per_task_stats if not s['data_starved']],
        'leakage_sources': src_meta,
        'per_task': per_task_stats,
        'output_jsonl': str(out_path),
        'output_jsonl_sha256': hashlib.sha256(out_path.read_bytes()).hexdigest(),
    }
    manifest_path = repo / args.manifest
    manifest_path.write_text(json.dumps(manifest, indent=2))

    print(f'\n=== Summary ===')
    print(f'  rows total: {len(out_rows)}')
    print(f'  tasks well-supplied: {len(per_task_stats) - len(starved)}/{len(per_task_stats)}')
    print(f'  data-starved: {starved}')
    print(f'  jsonl: {out_path}')
    print(f'  manifest: {manifest_path}')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
