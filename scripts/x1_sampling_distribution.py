"""X1 — Probe training/eval sampling distribution for the 3 persistent regressors.

Loads Qwen3-14B base via mlx_lm, takes 1 prompt per regressor from valid_golden,
samples N=16 responses at each of three settings:
  - "training default" — mlx_lm.generate built-in defaults (no kwargs)
  - "C-group recipe"   — temperature=0.7, top_p=0.8
  - "C-group greedy"   — temperature=0.0 (what eval actually uses)

Reports: distinct response cluster count, distinct first-token count,
mean response length. Computes argmax-flip rate = fraction of training-default
responses whose first token differs from the temp=0 greedy first token.

Decision threshold: if argmax-flip rate ≥30% on regressors → H1 confirmed.
"""
from __future__ import annotations

import json
import os
import sys
from collections import Counter
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

import mlx.core as mx
import mlx_lm
from mlx_lm.sample_utils import make_sampler

REGRESSORS = ["consulting.classify", "consulting.synthesis", "retrieval.query_classify"]
N_SAMPLES = 16
BASE = ".local-models/qwen3-14b-mdemg-v1"
GOLDEN = "training_data/eval/valid_golden.jsonl"

print(f"X1: loading base model {BASE}…", flush=True)
model, tok = mlx_lm.load(BASE)

# Read one prompt per regressor from valid_golden
prompts: dict[str, str] = {}
with open(GOLDEN) as f:
    for line in f:
        row = json.loads(line)
        meta = row.get("meta") or {}
        tname = meta.get("task_name")
        if tname in REGRESSORS and tname not in prompts:
            msgs = row.get("messages", [])
            non_assistant = [m for m in msgs if m["role"] != "assistant"]
            prompt = tok.apply_chat_template(non_assistant, tokenize=False, add_generation_prompt=True)
            prompts[tname] = prompt
        if len(prompts) == len(REGRESSORS):
            break

print(f"X1: loaded {len(prompts)} regressor prompts", flush=True)

results = {"per_task": {}, "n_samples_per_setting": N_SAMPLES, "regressors": REGRESSORS}

for task_id, prompt in prompts.items():
    print(f"\n--- {task_id} ---", flush=True)
    settings = {
        "default_greedy":  None,                           # mlx_lm default = temp=0
        "c_recipe":        make_sampler(temp=0.7, top_p=0.8),
        "training_typical":make_sampler(temp=1.0),         # if trainer used temp=1.0
    }
    by_setting: dict = {}
    for label, sampler in settings.items():
        responses = []
        for i in range(N_SAMPLES):
            mx.random.seed(i)
            kwargs = {"sampler": sampler} if sampler is not None else {}
            r = mlx_lm.generate(model, tok, prompt, max_tokens=200, **kwargs)
            responses.append(r)
        # Distinct: by full response text, by first 100 chars, by first token
        first_tokens = []
        for r in responses:
            ids = tok.encode(r) if r else []
            first_tokens.append(ids[0] if ids else None)
        distinct_full = len(set(responses))
        distinct_prefix = len(set(r[:100] for r in responses))
        distinct_first_tok = len(Counter(first_tokens))
        mean_len = sum(len(r) for r in responses) / len(responses)
        by_setting[label] = {
            "label": label,
            "distinct_full": distinct_full,
            "distinct_prefix100": distinct_prefix,
            "distinct_first_token": distinct_first_tok,
            "first_token_counter": dict(Counter(first_tokens).most_common(5)),
            "mean_response_len_chars": round(mean_len, 1),
        }
        print(f"  {label:<10} kwargs={kwargs}: distinct_full={distinct_full}/{N_SAMPLES} distinct_prefix={distinct_prefix} distinct_first_tok={distinct_first_tok} mean_len={mean_len:.0f}", flush=True)

    # argmax-flip rate: fraction of "default" responses whose first token != greedy first token
    greedy_first = list(by_setting["default_greedy"]["first_token_counter"].keys())[0]
    # Better: re-sample default and check per-sample flip
    # argmax-flip rate: at temp=1.0 sampling, what fraction of first-tokens differ from greedy?
    flip_count = 0
    samp_first_seq = []
    sampler1 = make_sampler(temp=1.0)
    for i in range(N_SAMPLES):
        mx.random.seed(i)
        r = mlx_lm.generate(model, tok, prompt, max_tokens=10, sampler=sampler1)
        ids = tok.encode(r) if r else []
        ft = ids[0] if ids else None
        samp_first_seq.append(ft)
        if ft != greedy_first:
            flip_count += 1
    flip_rate = flip_count / N_SAMPLES
    by_setting["argmax_flip_rate_temp1_vs_greedy"] = round(flip_rate, 3)
    by_setting["greedy_first_token"] = greedy_first
    by_setting["temp1_first_token_distribution"] = dict(Counter(samp_first_seq).most_common(5))
    print(f"  argmax-flip rate (temp=1.0 vs greedy): {flip_rate*100:.1f}%", flush=True)

    results["per_task"][task_id] = by_setting

# Verdict
flip_rates = [results["per_task"][t]["argmax_flip_rate_temp1_vs_greedy"] for t in REGRESSORS]
mean_flip = sum(flip_rates) / len(flip_rates)
print(f"\nMean argmax-flip rate across regressors: {mean_flip*100:.1f}%")
print(f"Decision threshold: ≥30% confirms H1; <5% refutes")
verdict = "CONFIRMED" if mean_flip >= 0.30 else ("REFUTED" if mean_flip < 0.05 else "AMBIGUOUS")
print(f"X1 verdict: H1 {verdict}")
results["mean_argmax_flip_rate"] = round(mean_flip, 3)
results["x1_verdict"] = verdict

out = Path("training_data/eval/phase11_5_diagnostics/x1_sampling_distribution.json")
out.write_text(json.dumps(results, indent=2, default=str))
print(f"persisted: {out}")
