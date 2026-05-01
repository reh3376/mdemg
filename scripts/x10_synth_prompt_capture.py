"""Phase 11.5e Epic 2 — Synthetic prompt + response capture for data-starved tasks.

For each target task:
1. Use gpt-5.4-mini at temp=0.9 to generate diverse user prompts conditioned on
   the ULTS task description + production system prompt + output_schema.
2. For each generated prompt, call gpt-5.4-mini AGAIN at the task's normal
   sampling temp using the production system prompt → captures realistic
   teacher response.
3. Compute reward; keep where reward >= threshold (default 0.7, lower than 11.5d
   distill's 0.8 since synthesis quality is inherently lower).
4. Convert to valid_clean.jsonl row format with full provenance.

Reuses the OpenAI HTTP transport from x7 + production prompt extraction from x9.
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

import yaml

from neural.benchmarks.run_benchmark import Spec, _mlx_chat
from neural.benchmarks.sampling_policy import resolve_sampling
from neural.training.reward_functions import compute_reward
from x7_gpt54mini_benchmark import openai_http_post

# Map task_name → (Go file rel-path, variable name) — production system prompt source
PROD_PROMPT_LOCATIONS = {
    'guardrail.evaluate': ('guardrail/prompt.go', 'guardrailSystemPromptCompact'),
    'hidden.summarize': ('hidden/cluster_summarizer.go', 'clusterSummarySystemPrompt'),
    'consulting.synthesis': None,  # dynamic prompt; build a representative system prompt skeleton
    'metalearn.generalize': ('metalearn/generalizer.go', 'generalizerSystemPrompt'),
    'summarize.generate': ('summarize/service.go', 'summarizeSystemPrompt'),
}

# Per-task user-prompt templates for synthesis meta-prompts.
# These describe what shape the synthetic user prompts should take, based on
# inspection of the production code.
SYNTH_INSTRUCTIONS = {
    'guardrail.evaluate': (
        "Generate {n} diverse synthetic user prompts that a Code Guardrail evaluator would receive. "
        "Each prompt is a unified diff (or plain code change) plus a list of constraint nodes (ID, kind, summary). "
        "Mix violation cases (clear must/must_not violations), warning cases (should/should_not deviations), and clean cases (no issues). "
        "Each prompt should be 200-800 chars. Output ONLY a JSON array of strings, no explanation."
    ),
    'hidden.summarize': (
        "Generate {n} diverse synthetic user prompts that a knowledge graph cluster summarizer would receive. "
        "Each prompt is a list of 3-8 related code/concept nodes (e.g., 'Function X in module Y / Class Z in module W / Decision: D made on date'). "
        "Cover varied domains: authentication, error handling, data flow, configuration, deployment, testing. "
        "Each prompt 150-400 chars. Output ONLY a JSON array of strings."
    ),
    'consulting.synthesis': (
        "Generate {n} diverse synthetic user prompts that an SME synthesis engine would receive. "
        "Each prompt should contain: a developer question (1 sentence), retrieved evidence (3-6 nodes with IDs and content snippets), "
        "and 1-2 organizational concepts. Cover varied questions: 'How do I authenticate?', 'What's our error pattern?', etc. "
        "Each prompt 400-1200 chars. Output ONLY a JSON array of strings."
    ),
    'metalearn.generalize': (
        "Generate {n} diverse synthetic user prompts that a knowledge abstraction engine would receive. "
        "Each prompt is a single concept name + description from a specific codebase, e.g., "
        "'Concept: PaymentRetryQueue / Description: Manages failed payment transactions in /backend/payments using Redis'. "
        "Cover varied domains. Each prompt 100-300 chars. Output ONLY a JSON array of strings."
    ),
    'summarize.generate': (
        "Generate {n} diverse synthetic user prompts that a code summarizer would receive. "
        "Each prompt is a JSON object with one or more code elements: e.g., "
        "{{\"function_name\": \"getUserById\", \"code\": \"func getUserById(id string) (*User, error) {{...}}\"}}. "
        "Cover functions, classes, modules from various domains. Each prompt 200-1000 chars. Output ONLY a JSON array of strings."
    ),
}


def _go_const_string(repo: Path, file_rel: str, var_name: str) -> str:
    text = (repo / 'internal' / file_rel).read_text()
    pat = rf'{re.escape(var_name)}\s*=\s*`([^`]*)`'
    m = re.search(pat, text)
    if not m:
        raise RuntimeError(f'could not extract {var_name} from {file_rel}')
    return m.group(1)


def _consulting_synthesis_skeleton() -> str:
    """Production prompt for consulting.synthesis is dynamic. Use a representative skeleton
    matching the synthesis.go:184-193 framing."""
    return """You are a Subject Matter Expert for a specific software organization. You have deep expertise ONLY in the patterns, decisions, constraints, and history stored in this organization's knowledge graph. You do NOT have general programming knowledge beyond what is in the graph.

Rules:
- Cite every claim as (Node: <node_id>)
- Do NOT invent information not present in the evidence below
- Surface risks prominently with WARNING prefix
- Use markdown formatting"""


def _cuid() -> str:
    try:
        from cuid2 import Cuid
        return Cuid(length=24).generate()
    except ImportError:
        return hashlib.blake2b(str(time.time_ns()).encode(), digest_size=12).hexdigest()


def call_openai(system_prompt: str, user_prompt: str, sampling_kwargs: dict[str, Any], max_tokens: int = 4000) -> tuple[str, str | None, dict]:
    """Call gpt-5.4-mini with retry on transient errors."""
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


def synthesize_user_prompts(task_name: str, system_prompt: str, n: int) -> list[str]:
    """Generate n synthetic user prompts via gpt-mini at temp=0.9."""
    instruction = SYNTH_INSTRUCTIONS[task_name].format(n=n)
    meta_system = (
        "You generate realistic synthetic user prompts for testing language models. "
        "You have full visibility into the system prompt the target model will see (provided below). "
        "Generate prompts that exercise the task realistically — mix easy/medium/hard, "
        "vary input length and structure. Output a clean JSON array of prompt strings only."
    )
    meta_user = (
        f"=== TASK: {task_name} ===\n\n"
        f"=== TARGET MODEL'S SYSTEM PROMPT ===\n{system_prompt}\n\n"
        f"=== INSTRUCTION ===\n{instruction}"
    )
    sampling_kwargs = {'temperature': 0.9, 'top_p': 0.95}
    response, finish_reason, usage = call_openai(meta_system, meta_user, sampling_kwargs, max_tokens=8000)

    # Parse JSON array
    try:
        # gpt-mini sometimes wraps with markdown fences
        cleaned = response.strip()
        if cleaned.startswith('```'):
            cleaned = re.sub(r'^```\w*\n', '', cleaned)
            cleaned = re.sub(r'\n```\s*$', '', cleaned)
        prompts = json.loads(cleaned)
        if not isinstance(prompts, list):
            raise ValueError("not a list")
        return [str(p) for p in prompts if p]
    except Exception as e:
        print(f'  WARN: synthesis parse failed for {task_name}: {e}', file=sys.stderr)
        print(f'  raw response[:300]: {response[:300]!r}', file=sys.stderr)
        return []


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument('--target-per-task', type=int, default=20)
    ap.add_argument('--reward-threshold', type=float, default=0.7)
    ap.add_argument('--out', default='training_data/eval/valid_clean_synthetic.jsonl')
    ap.add_argument('--manifest', default='training_data/eval/valid_clean_synthetic_manifest.json')
    ap.add_argument('--config', default='configs/benchmark_phase10.yaml')
    ap.add_argument('--repo-root', default='/Users/reh3376/mdemg')
    ap.add_argument('--tasks', default='guardrail.evaluate,hidden.summarize,consulting.synthesis,metalearn.generalize,summarize.generate')
    args = ap.parse_args()

    repo = Path(args.repo_root)
    config = yaml.safe_load((repo / args.config).read_text())
    specs_dir = repo / config['ults']['specs_dir']
    specs_by_task = {Spec.load(p).task_name: Spec.load(p) for p in sorted(specs_dir.glob('*.ults.json'))}

    tasks = [t.strip() for t in args.tasks.split(',') if t.strip()]
    print(f'Synthesis targets: {tasks}')
    print(f'Target per task: {args.target_per_task}, reward threshold: {args.reward_threshold}')

    # Resolve production system prompts
    sys_prompts: dict[str, str] = {}
    for task in tasks:
        if PROD_PROMPT_LOCATIONS.get(task) is None:
            sys_prompts[task] = _consulting_synthesis_skeleton()
            print(f'  {task}: dynamic — skeleton (sha={hashlib.sha256(sys_prompts[task].encode()).hexdigest()[:12]})')
        else:
            f, v = PROD_PROMPT_LOCATIONS[task]
            sys_prompts[task] = _go_const_string(repo, f, v)
            print(f'  {task}: production const {v} ({len(sys_prompts[task])} chars)')

    run_id = _cuid()
    started_at = time.time()
    print(f'\nrun_id={run_id}')

    out_lines: list[str] = []
    per_task_stats: dict[str, dict[str, Any]] = {}
    total_calls = 0

    for task in tasks:
        spec = specs_by_task.get(task)
        if not spec:
            print(f'  skip {task}: spec not found', file=sys.stderr)
            continue

        print(f'\n=== {task} (group={spec.sampling_group}) ===')
        sampling_kwargs = resolve_sampling(spec.raw, config)
        max_tokens = max(int(spec.performance_max_tokens), 3000)

        # Generate user prompts via meta-synthesis (over-generate; we'll filter)
        n_to_synth = int(args.target_per_task * 1.5)
        try:
            synth_prompts = synthesize_user_prompts(task, sys_prompts[task], n_to_synth)
        except Exception as e:  # noqa: BLE001
            print(f'  ERR synthesis: {e}')
            per_task_stats[task] = {'error': str(e)[:200]}
            continue
        print(f'  synthesized {len(synth_prompts)} candidate prompts')
        if not synth_prompts:
            per_task_stats[task] = {'error': 'no synth prompts'}
            continue

        # Capture gpt-mini response for each, compute reward, keep ≥threshold
        stats = {
            'n_synth': len(synth_prompts),
            'calls_attempted': 0,
            'calls_succeeded': 0,
            'rewards_pre_filter': [],
            'kept_pairs': 0,
            'errors': [],
            'system_prompt_sha256': hashlib.sha256(sys_prompts[task].encode()).hexdigest(),
            'sampling_kwargs': sampling_kwargs,
        }

        for i, user in enumerate(synth_prompts):
            if stats['kept_pairs'] >= args.target_per_task:
                break
            stats['calls_attempted'] += 1
            total_calls += 1
            try:
                chat_out = call_openai(sys_prompts[task], user, sampling_kwargs, max_tokens=max_tokens)
            except Exception as e:  # noqa: BLE001
                stats['errors'].append({'idx': i, 'err': str(e)[:200]})
                continue

            if isinstance(chat_out, tuple):
                response, finish_reason, usage = chat_out
            else:
                response, finish_reason, usage = chat_out, None, {}

            stats['calls_succeeded'] += 1

            # Reward
            reward_kwargs: dict[str, Any] = {}
            if spec.output_schema:
                reward_kwargs['schema'] = spec.output_schema
            try:
                reward_vector = compute_reward(response, spec.reward_functions, **reward_kwargs)
            except Exception as e:  # noqa: BLE001
                stats['errors'].append({'idx': i, 'err': f'reward: {e}'})
                continue
            score = sum(reward_vector.values()) / len(reward_vector) if reward_vector else 0.0
            stats['rewards_pre_filter'].append(score)

            kept = score >= args.reward_threshold
            if kept:
                stats['kept_pairs'] += 1
                # Build valid_clean-format row
                messages = [
                    {'role': 'system', 'content': sys_prompts[task]},
                    {'role': 'user', 'content': user},
                    {'role': 'assistant', 'content': response},
                ]
                row = {
                    'messages': messages,
                    'meta': {
                        'task_name': task,
                        'sampling_group': spec.sampling_group,
                        'source': 'synthetic_gpt54mini',
                        'synth_run_id': run_id,
                        'synth_idx': i,
                        'synth_reward_score': score,
                        'synth_reward_vector': reward_vector,
                        'system_prompt_sha256': stats['system_prompt_sha256'],
                        'extracted_by': 'phase_11_5e.x10_synth_prompt_capture',
                    },
                }
                out_lines.append(json.dumps(row))

            if total_calls % 10 == 0:
                print(f'  ... {total_calls} calls ({task}: {stats["kept_pairs"]} kept of {stats["calls_attempted"]})')

        rs = stats['rewards_pre_filter']
        if rs:
            print(f'  → kept {stats["kept_pairs"]}/{stats["calls_attempted"]}; reward mean={sum(rs)/len(rs):.3f} min={min(rs):.3f} max={max(rs):.3f}')
        per_task_stats[task] = stats

    # Write JSONL
    out_path = repo / args.out
    out_path.parent.mkdir(parents=True, exist_ok=True)
    with out_path.open('w') as fh:
        fh.write('\n'.join(out_lines))
        if out_lines:
            fh.write('\n')

    # Manifest
    completed_at = time.time()
    manifest = {
        'sprint': 'FT-LORA-PHASE11.5e',
        'epic': '2_synthesis',
        'run_id': run_id,
        'started_at_unix': started_at,
        'completed_at_unix': completed_at,
        'elapsed_s': completed_at - started_at,
        'model': 'gpt-5.4-mini',
        'reward_threshold': args.reward_threshold,
        'target_per_task': args.target_per_task,
        'tasks': tasks,
        'per_task': per_task_stats,
        'total_openai_calls': total_calls,
        'output_jsonl': str(out_path),
        'output_jsonl_sha256': hashlib.sha256(out_path.read_bytes()).hexdigest() if out_path.exists() else None,
        'synthetic_total': len(out_lines),
    }
    manifest_path = repo / args.manifest
    manifest_path.write_text(json.dumps(manifest, indent=2, default=str))

    print(f'\n=== Synthesis Summary ===')
    print(f'  total OpenAI calls: {total_calls} ({completed_at - started_at:.1f}s)')
    print(f'  total kept synthetic rows: {len(out_lines)}')
    for task, s in per_task_stats.items():
        if 'error' in s:
            print(f'  {task}: ERROR — {s["error"]}')
        else:
            print(f'  {task}: {s["kept_pairs"]} kept (of {s["calls_attempted"]} attempted)')
    print(f'\n  jsonl: {out_path}')
    print(f'  manifest: {manifest_path}')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
