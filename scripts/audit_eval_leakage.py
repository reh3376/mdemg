"""Phase 11.5c Epic 2 — Audit prompt leakage between any eval JSONL and source files.

For each task in the eval, count how many user_prompts also appear in any of the
source JSONLs (typically train/valid splits the model was trained on).

Exit code 0 if no leakage detected anywhere; non-zero otherwise.

Usage:
  python scripts/audit_eval_leakage.py --eval <jsonl> --against <comma-separated jsonls> --out <json>
"""
from __future__ import annotations

import argparse
import json
import sys
from collections import defaultdict
from pathlib import Path
from typing import Any


def _user_prompt(row: dict) -> str | None:
    for m in row.get('messages') or []:
        if m.get('role') == 'user':
            return m.get('content')
    return None


def _task_name(row: dict) -> str:
    meta = row.get('meta') or {}
    return meta.get('task_name') or meta.get('task') or row.get('task_id') or 'unknown'


def load_user_prompts_per_task(path: Path) -> dict[str, set[str]]:
    out: dict[str, set[str]] = defaultdict(set)
    if not path.exists():
        return out
    with path.open() as fh:
        for line in fh:
            line = line.strip()
            if not line:
                continue
            try:
                row = json.loads(line)
            except json.JSONDecodeError:
                continue
            up = _user_prompt(row)
            if up:
                out[_task_name(row)].add(up)
    return dict(out)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument('--eval', required=True, help='Path to evaluation JSONL to audit')
    ap.add_argument('--against', required=True,
                    help='Comma-separated paths to source JSONLs (training/validation files)')
    ap.add_argument('--out', required=True, help='Write JSON report here')
    ap.add_argument('--verbose', action='store_true')
    args = ap.parse_args()

    eval_path = Path(args.eval)
    src_paths = [Path(p.strip()) for p in args.against.split(',') if p.strip()]

    print(f'Auditing eval: {eval_path}')
    eval_per_task = load_user_prompts_per_task(eval_path)
    n_eval_total = sum(len(s) for s in eval_per_task.values())
    print(f'  rows: {n_eval_total} across {len(eval_per_task)} tasks')

    print(f'Against {len(src_paths)} sources:')
    src_per_task: dict[str, dict[str, set[str]]] = {}
    for sp in src_paths:
        src_per_task[str(sp)] = load_user_prompts_per_task(sp)
        n_src_total = sum(len(s) for s in src_per_task[str(sp)].values())
        print(f'  {sp}: {n_src_total} rows')

    # Per task
    report: dict[str, Any] = {
        'eval_path': str(eval_path),
        'sources': [str(p) for p in src_paths],
        'tasks': {},
        'overlap_total': 0,
        'eval_total': n_eval_total,
    }
    has_leakage = False
    print(f'\n{"Task":32s} {"eval":>5s} {"any_overlap":>12s} {"per-source overlap"}')
    print(f'{"-"*32} {"-"*5} {"-"*12} {"-"*40}')

    for task in sorted(eval_per_task.keys()):
        eval_set = eval_per_task[task]
        per_source: dict[str, int] = {}
        union_overlap: set[str] = set()
        for sp_str, src_map in src_per_task.items():
            src_set = src_map.get(task, set())
            inter = eval_set & src_set
            per_source[sp_str] = len(inter)
            union_overlap |= inter
        n_any = len(union_overlap)
        if n_any > 0:
            has_leakage = True
        report['tasks'][task] = {
            'eval_count': len(eval_set),
            'overlap_any': n_any,
            'per_source_overlap': per_source,
            'overlap_examples': [
                p[:100] + ('…' if len(p) > 100 else '')
                for p in list(union_overlap)[:3]
            ],
        }
        report['overlap_total'] += n_any
        nz = [f'{Path(p).name}:{c}' for p, c in per_source.items() if c > 0]
        nz_str = ', '.join(nz) if nz else '—'
        flag = '⚠' if n_any > 0 else '✓'
        print(f'{task:32s} {len(eval_set):>5} {n_any:>11}{flag:1s} {nz_str}')

    out_path = Path(args.out)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(report, indent=2, default=str))
    print(f'\nReport: {out_path}')

    print(f'\nVerdict: {"LEAKAGE DETECTED" if has_leakage else "CLEAN — no overlap with any source"}')
    print(f'  total overlapping prompts: {report["overlap_total"]} / {n_eval_total}')

    return 1 if has_leakage else 0


if __name__ == '__main__':
    raise SystemExit(main())
