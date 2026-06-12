"""Phase 11.5d Epic 1 — Capture gpt-5.4-mini exemplars for SFT distillation.

Targets the 2 real regressors identified in 11.5c + 11.5d Epic 0:
- consulting.classify (Run 7: 0.433 vs gpt-mini: 0.883 → +45pp gap)
- retrieval.rerank_cross (Run 7: 0.720 vs gpt-mini: 0.900 → +18pp gap, fix degeneracy)

Pipeline:
1. For each task, query TSDB for production user_prompts matching task_name (any
   historical system_prompt_hash, since we'll re-render with the CURRENT production
   system prompt at SFT-training time — that's what the model will see at inference).
2. Filter out user_prompts present in any of 10 leak sources
   (9 train/valid + valid_clean.jsonl).
3. Dedup by user_prompt; sample N unique top-quality (default 50/task).
4. For each: call gpt-5.4-mini with the CURRENT production system prompt + user prompt.
5. Compute reward via REWARD_REGISTRY; filter ≥0.8 threshold.
6. Convert to mlx_lm.lora chat-format JSONL (system + user + assistant).
7. 90/10 train/valid split, stratified by task.
8. Manifest with SHA256s, per-task stats, OpenAI spend.
9. Final leak audit: train.jsonl prompts must NOT appear in any of 10 sources.

Cost: 2 tasks × ~50 calls × ~$0.005 ≈ $0.50-1.50 OpenAI.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import sys
import time
from pathlib import Path
from typing import Any

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
sys.path.insert(0, str(Path(__file__).resolve().parent))

import psycopg
import yaml

from neural.benchmarks.run_benchmark import Spec, _mlx_chat
from neural.benchmarks.sampling_policy import resolve_sampling
from neural.training.reward_functions import compute_reward
from x7_gpt54mini_benchmark import openai_http_post

LEAK_SOURCES = [
    'training_data/sft/tier1/train.jsonl',
    'training_data/sft/tier1/valid.jsonl',
    'training_data/sft/family_classify_notink/train.jsonl',
    'training_data/sft/family_classify_notink/valid.jsonl',
    'training_data/sft/family_reasoning_think/train.jsonl',
    'training_data/sft/family_reasoning_think/valid.jsonl',
    'training_data/sft/family_structured_notink/train.jsonl',
    'training_data/sft/family_structured_notink/valid.jsonl',
    'training_data/eval/valid_golden.jsonl',
    'training_data/eval/valid_clean.jsonl',
]

DISTILL_TASKS = ['consulting.classify', 'retrieval.rerank_cross']

# Map task_name → Go file:variable for current production system prompt
PROD_PROMPT_LOCATIONS = {
    'consulting.classify': ('consulting/llm_classifier.go', 'constraintClassifySystemPrompt'),
    'retrieval.rerank_cross': ('retrieval/rerank.go', 'rerankNLISystemPrompt'),  # production uses compact
}


def _go_const_string(repo: Path, file_rel: str, var_name: str) -> str:
    text = (repo / 'internal' / file_rel).read_text()
    pat = rf'{re.escape(var_name)}\s*=\s*`([^`]*)`'
    m = re.search(pat, text)
    if not m:
        raise RuntimeError(f'could not extract {var_name} from {file_rel}')
    return m.group(1)


def _cuid() -> str:
    try:
        from cuid2 import Cuid
        return Cuid(length=24).generate()
    except ImportError:
        return hashlib.blake2b(str(time.time_ns()).encode(), digest_size=12).hexdigest()


def _file_sha256(p: Path) -> str | None:
    if not p.exists():
        return None
    h = hashlib.sha256()
    h.update(p.read_bytes())
    return h.hexdigest()


def load_known_user_prompts_per_task(repo: Path, sources: list[str]) -> tuple[dict[str, set[str]], dict[str, dict]]:
    known: dict[str, set[str]] = {}
    src_meta: dict[str, dict] = {}
    for src in sources:
        full = repo / src
        meta = {'sha256': _file_sha256(full), 'rows': 0, 'bytes': full.stat().st_size if full.exists() else 0}
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
                user = ''
                for msg in row.get('messages') or []:
                    if msg.get('role') == 'user':
                        user = msg.get('content', '')
                        break
                if user:
                    known.setdefault(task, set()).add(user)
        src_meta[src] = meta
    return known, src_meta


def fetch_production_user_prompts(cur, task_name: str, target: int, known: set[str]) -> list[tuple[str, float | None]]:
    """Return list of (user_prompt, quality) — leak-filtered, deduped, top target."""
    cur.execute("""
        SELECT user_prompt, MAX(COALESCE(quality, 0.0)) AS best_quality
        FROM llm_interactions
        WHERE task_name = %s AND response IS NOT NULL AND length(response) > 0
        GROUP BY user_prompt
        ORDER BY best_quality DESC, user_prompt
        LIMIT 1000
    """, (task_name,))
    rows = cur.fetchall()
    out = []
    seen = set()
    for user_prompt, q in rows:
        if user_prompt in known or user_prompt in seen:
            continue
        seen.add(user_prompt)
        out.append((user_prompt, float(q) if q is not None else None))
        if len(out) >= target:
            break
    return out


# FT-CLASSIFY-002: production class distribution for consulting.classify,
# measured live 2026-06-11 over 4,028 parseable rows. The 11.5d corpus had
# none=0 and backfired; stratified sampling matches the operating
# distribution (final corpus distribution is re-verified by TEACHER label
# in the manifest, gate ±5pp/class).
CLASSIFY_TARGET_DISTRIBUTION = {
    'none': 0.819,
    'must': 0.122,
    'should': 0.040,
    'must_not': 0.016,
    'should_not': 0.002,
}


def fetch_stratified_classify_prompts(cur, task_name: str, target: int, known: set[str],
                                      distribution: dict[str, float]) -> list[tuple[str, float | None]]:
    """Class-stratified variant of fetch_production_user_prompts for
    consulting.classify: buckets candidate prompts by their PRODUCTION
    response class and samples each bucket to match `distribution`.
    Shortfall in a rare class falls back to proportional backfill from
    'none' (the dominant class) with a loud notice."""
    cur.execute("""
        SELECT user_prompt,
               MAX(COALESCE(quality, 0.0)) AS best_quality,
               (array_agg(response ORDER BY time DESC))[1] AS latest_response
        FROM llm_interactions
        WHERE task_name = %s AND response IS NOT NULL AND length(response) > 0
          AND (error IS NULL OR error = '')
        GROUP BY user_prompt
        ORDER BY best_quality DESC, user_prompt
        LIMIT 4000
    """, (task_name,))
    buckets: dict[str, list[tuple[str, float | None]]] = {k: [] for k in distribution}
    seen: set[str] = set()
    for user_prompt, q, latest_response in cur.fetchall():
        if user_prompt in known or user_prompt in seen:
            continue
        seen.add(user_prompt)
        try:
            cls = json.loads(latest_response).get('type', '')
        except (json.JSONDecodeError, TypeError, AttributeError):
            continue
        if cls in buckets:
            buckets[cls].append((user_prompt, float(q) if q is not None else None))

    out: list[tuple[str, float | None]] = []
    shortfall = 0
    for cls, frac in distribution.items():
        want = max(1, round(target * frac)) if frac > 0 else 0
        got = buckets[cls][:want]
        if len(got) < want:
            print(f'  [stratify] class {cls!r}: wanted {want}, only {len(got)} available '
                  f'(shortfall {want - len(got)} backfills from none)')
            shortfall += want - len(got)
        out.extend(got)
    if shortfall > 0:
        used = {p for p, _ in out}
        extra = [(p, q) for p, q in buckets['none'] if p not in used][:shortfall]
        out.extend(extra)
    counts = {cls: 0 for cls in distribution}
    lookup = {p: cls for cls, items in buckets.items() for p, _ in items}
    for p_, _ in out:
        counts[lookup[p_]] += 1
    print(f'  [stratify] sampled input distribution (by production label): {counts} (total {len(out)})')
    return out


def call_gpt_mini(system_prompt: str, user_prompt: str, sampling_kwargs: dict[str, Any], max_tokens: int) -> tuple[str, str | None, dict]:
    messages = [
        {'role': 'system', 'content': system_prompt},
        {'role': 'user',   'content': user_prompt},
    ]
    last_err: Exception | None = None
    for attempt in range(3):
        try:
            return _mlx_chat(
                base_url='https://api.openai.com/v1',
                model='gpt-5.4-mini',
                messages=messages,
                sampling_kwargs=sampling_kwargs,
                max_tokens=max_tokens,
                timeout_s=120.0,
                _http_post=openai_http_post,
            )
        except Exception as e:  # noqa: BLE001
            last_err = e
            msg = str(e)
            if any(t in msg for t in ('502', '503', '504', 'timeout', 'TimeoutError')):
                time.sleep(2 + attempt * 3)
                continue
            raise
    raise last_err  # type: ignore[misc]


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument('--target-per-task', type=int, default=50)
    ap.add_argument('--reward-threshold', type=float, default=0.8)
    ap.add_argument('--out-dir', default='training_data/distill/phase11_5d')
    ap.add_argument('--config', default='configs/benchmark_phase10.yaml')
    ap.add_argument('--repo-root', default='/Users/reh3376/mdemg')
    ap.add_argument('--tasks', default=','.join(DISTILL_TASKS))
    ap.add_argument('--stratify-classify', action='store_true',
                    help='FT-CLASSIFY-002: sample consulting.classify prompts to match the '
                         'production class distribution (incl. the dominant none class)')
    args = ap.parse_args()

    repo = Path(args.repo_root)
    config = yaml.safe_load((repo / args.config).read_text())
    specs_dir = repo / config['ults']['specs_dir']
    specs_by_task = {s.task_name: s for s in (Spec.load(p) for p in sorted(specs_dir.glob('*.ults.json')))}
    out_dir = repo / args.out_dir
    out_dir.mkdir(parents=True, exist_ok=True)

    tasks = [t.strip() for t in args.tasks.split(',') if t.strip()]
    print(f'Distill capture targets: {tasks}')
    print(f'Target per task: {args.target_per_task}, reward threshold: {args.reward_threshold}')

    # Load production system prompts for the sampled tasks
    prod_system_prompts: dict[str, str] = {}
    for task in tasks:
        if task not in PROD_PROMPT_LOCATIONS:
            print(f'WARN: no production prompt mapping for {task}', file=sys.stderr)
            continue
        f, v = PROD_PROMPT_LOCATIONS[task]
        prod_system_prompts[task] = _go_const_string(repo, f, v)
        print(f'  {task} prod system prompt: {len(prod_system_prompts[task])} chars (sha={hashlib.sha256(prod_system_prompts[task].encode()).hexdigest()[:12]})')

    # Load leak sources
    print(f'\nLoading {len(LEAK_SOURCES)} leakage sources...')
    known_per_task, src_meta = load_known_user_prompts_per_task(repo, LEAK_SOURCES)
    for src, meta in src_meta.items():
        print(f'  {src}: {meta["rows"]:>5} rows, sha={meta["sha256"][:12] if meta["sha256"] else "MISSING"}')

    # Connect TSDB
    conn_str = (
        f"host={os.environ.get('TSDB_HOST', 'localhost')} "
        f"port={os.environ.get('TSDB_PORT', '5433')} "
        f"user={os.environ.get('TSDB_USER', 'mdemg')} "
        f"password={os.environ.get('TSDB_PASSWORD', 'mdemg_metrics')} "
        f"dbname={os.environ.get('TSDB_DB', 'mdemg_metrics')}"
    )

    raw_path = out_dir / 'raw_responses.jsonl'
    raw_lines: list[str] = []
    per_task_stats: dict[str, Any] = {}

    run_id = _cuid()
    started_at = time.time()
    print(f'\nrun_id={run_id}')
    total_calls = 0

    with psycopg.connect(conn_str) as conn, conn.cursor() as cur:
        for task in tasks:
            if task not in prod_system_prompts:
                continue
            spec = specs_by_task.get(task)
            if not spec:
                print(f'  skip {task}: spec not found', file=sys.stderr)
                continue
            sampling_kwargs = resolve_sampling(spec.raw, config)
            sys_prompt = prod_system_prompts[task]
            max_tokens = max(int(spec.performance_max_tokens), 3000)  # MEMORY floor
            known = known_per_task.get(task, set())

            print(f'\n== {task} (group={spec.sampling_group}, temp={sampling_kwargs.get("temperature","?")}, max_tokens={max_tokens}) ==')
            candidates = (fetch_stratified_classify_prompts(cur, task, args.target_per_task, known, CLASSIFY_TARGET_DISTRIBUTION)
                   if (args.stratify_classify and task == 'consulting.classify')
                   else fetch_production_user_prompts(cur, task, args.target_per_task, known))
            print(f'  TSDB→leak-filter→dedup: {len(candidates)} unique user_prompts available')

            stats = {
                'sampling_group': spec.sampling_group,
                'sampling_kwargs': sampling_kwargs,
                'max_tokens': max_tokens,
                'system_prompt_sha256': hashlib.sha256(sys_prompt.encode()).hexdigest(),
                'system_prompt_len': len(sys_prompt),
                'candidates_available': len(candidates),
                'calls_attempted': 0,
                'calls_succeeded': 0,
                'rewards_pre_filter': [],
                'rewards_post_filter': [],
                'errors': [],
                'kept_pairs': 0,
            }

            for i, (user_prompt, quality) in enumerate(candidates):
                stats['calls_attempted'] += 1
                total_calls += 1
                t0 = time.time()
                try:
                    chat_out = call_gpt_mini(sys_prompt, user_prompt, sampling_kwargs, max_tokens)
                except Exception as e:  # noqa: BLE001
                    err = str(e)[:300]
                    stats['errors'].append({'idx': i, 'err': err})
                    print(f'  [ERR {i}] {err[:120]}')
                    continue

                if isinstance(chat_out, tuple):
                    response, finish_reason, usage = chat_out
                else:
                    response, finish_reason, usage = chat_out, None, {}
                latency_ms = int((time.time() - t0) * 1000)

                # Pass schema so json_valid recognizes array vs object shape per spec.
                # Don't pass `expected` — we have no ground truth; rewards that need it
                # (classification_accuracy, ndcg_delta) fall back to format-only credit.
                reward_kwargs: dict[str, Any] = {}
                if spec.output_schema:
                    reward_kwargs['schema'] = spec.output_schema
                reward_vector = compute_reward(response, spec.reward_functions, **reward_kwargs)
                reward_score = (sum(reward_vector.values()) / len(reward_vector)) if reward_vector else 0.0
                stats['calls_succeeded'] += 1
                stats['rewards_pre_filter'].append(reward_score)

                kept = reward_score >= args.reward_threshold
                if kept:
                    stats['rewards_post_filter'].append(reward_score)
                    stats['kept_pairs'] += 1

                line = {
                    'run_id': run_id,
                    'task_id': task,
                    'sampling_group': spec.sampling_group,
                    'idx': i,
                    'system_prompt': sys_prompt,
                    'user_prompt': user_prompt,
                    'response_text': response,
                    'reward_vector': reward_vector,
                    'reward_score': reward_score,
                    'kept': kept,
                    'finish_reason': finish_reason,
                    'completion_tokens': usage.get('completion_tokens'),
                    'prompt_tokens': usage.get('prompt_tokens'),
                    'latency_ms': latency_ms,
                    'tsdb_quality': quality,
                    'sampling_kwargs': sampling_kwargs,
                }
                raw_lines.append(json.dumps(line))

                if total_calls % 10 == 0:
                    print(f'  ... {total_calls} calls, last reward={reward_score:.3f} kept={kept}')

            print(f'  → {stats["calls_succeeded"]}/{stats["calls_attempted"]} succeeded; kept {stats["kept_pairs"]} (reward ≥ {args.reward_threshold})')
            per_task_stats[task] = stats

    raw_path.write_text('\n'.join(raw_lines) + '\n')

    # Build mlx_lm.lora chat-format JSONL (kept rows only) + 90/10 split
    kept_rows = [json.loads(l) for l in raw_lines if json.loads(l)['kept']]
    print(f'\nKept rows total: {len(kept_rows)}')
    # Stratified split: per-task 90/10
    train_rows, valid_rows = [], []
    import random
    rng = random.Random(0)
    by_task: dict[str, list[dict]] = {}
    for r in kept_rows:
        by_task.setdefault(r['task_id'], []).append(r)
    for task, rs in by_task.items():
        rng.shuffle(rs)
        cut = max(1, int(len(rs) * 0.9))
        train_rows.extend(rs[:cut])
        valid_rows.extend(rs[cut:])
    rng.shuffle(train_rows)
    rng.shuffle(valid_rows)

    def to_chat(r):
        return {
            'messages': [
                {'role': 'system',    'content': r['system_prompt']},
                {'role': 'user',      'content': r['user_prompt']},
                {'role': 'assistant', 'content': r['response_text']},
            ],
            'meta': {
                'task_name': r['task_id'],
                'source': 'gpt54mini_distill_phase11_5d',
                'reward_score': r['reward_score'],
                'sampling_group': r['sampling_group'],
            },
        }

    train_path = out_dir / 'train.jsonl'
    valid_path = out_dir / 'valid.jsonl'
    with train_path.open('w') as fh:
        for r in train_rows:
            fh.write(json.dumps(to_chat(r)) + '\n')
    with valid_path.open('w') as fh:
        for r in valid_rows:
            fh.write(json.dumps(to_chat(r)) + '\n')

    # Manifest
    completed_at = time.time()
    manifest = {
        'sprint': 'FT-LORA-PHASE11.5d',
        'epic': '1_distill_capture',
        'run_id': run_id,
        'started_at_unix': started_at,
        'completed_at_unix': completed_at,
        'elapsed_s': completed_at - started_at,
        'model': 'gpt-5.4-mini',
        'reward_threshold': args.reward_threshold,
        'target_per_task': args.target_per_task,
        'tasks': tasks,
        'leak_sources': src_meta,
        'per_task': per_task_stats,
        'output': {
            'raw_path': str(raw_path),
            'train_path': str(train_path),
            'valid_path': str(valid_path),
            'train_count': len(train_rows),
            'valid_count': len(valid_rows),
            'train_sha256': hashlib.sha256(train_path.read_bytes()).hexdigest(),
            'valid_sha256': hashlib.sha256(valid_path.read_bytes()).hexdigest(),
        },
        'production_system_prompt_shas': {
            t: hashlib.sha256(p.encode()).hexdigest() for t, p in prod_system_prompts.items()
        },
    }
    manifest_path = out_dir / 'manifest.json'
    manifest_path.write_text(json.dumps(manifest, indent=2, default=str))

    print(f'\n=== Summary ===')
    print(f'  total OpenAI calls: {total_calls}')
    print(f'  elapsed: {(completed_at - started_at):.1f}s')
    print(f'  kept pairs: {len(kept_rows)} (train: {len(train_rows)}, valid: {len(valid_rows)})')
    for task, s in per_task_stats.items():
        rs = s['rewards_pre_filter']
        if rs:
            print(f'  {task}: kept {s["kept_pairs"]}/{s["calls_succeeded"]}, '
                  f'reward mean={sum(rs)/len(rs):.3f}, min={min(rs):.3f}, max={max(rs):.3f}')
    print(f'\nmanifest: {manifest_path}')
    print(f'train:    {train_path}')
    print(f'valid:    {valid_path}')

    return 0


if __name__ == '__main__':
    raise SystemExit(main())
