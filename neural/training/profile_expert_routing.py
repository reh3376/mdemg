#!/usr/bin/env python3
"""Sprint FT-LORA-D Epic 2 — Expert activation profiler.

Runs forward passes (generation mode) over a per-task anchor prompt set against
the Sprint C MLX-loaded Qwen3.6-35B-A3B-mxfp4 checkpoint, capturing top-k routing
decisions at every MoE layer. Emits one JSON artifact per task family plus a raw
per-(task, layer, expert) counts file.

Router capture mechanism:
    mlx_lm 0.31.2 has no native `return_router_logits` flag. This profiler uses
    a context-manager class-level monkey-patch of
    `mlx_lm.models.qwen3_next.Qwen3NextSparseMoeBlock.__call__` that wraps the
    original forward, records `inds` (top-k expert indices) and `scores`
    (routing weights) alongside a stable layer identity stamped on each block
    at load time, then delegates. A `finally`-block always restores the
    original. See tests/test_profile_expert_routing.py for cleanup verification.

Sprint D runbook: docs/development/ft-lora/sprint_plan_ft_lora_d.md
Profile consumer: Sprint E's neural/training/train_ft.py
"""
from __future__ import annotations

import argparse
import contextlib
import hashlib
import json
import pathlib
import sys
import time
from collections import defaultdict
from typing import Any, Optional

# --- Constants ----------------------------------------------------------------

EXPECTED_MLX_LM_VERSION = "0.31.2"

# Capture points (line references current for mlx_lm 0.31.2 — re-audit on bump):
#   inds   — qwen3_next.py:338 (top-k expert indices, shape [B, L, top_k])
#   scores — qwen3_next.py:339 (routing weights, shape [B, L, top_k])


# --- Router capture -----------------------------------------------------------


class RouterCapture:
    """Thread-unsafe per-block router capture.

    Monkey-patches `Qwen3NextSparseMoeBlock.__call__` at class level on enter,
    restores original on exit. Each MoE block is stamped with a stable
    `_profiler_layer_idx` attribute (set by `stamp_layer_indices`) so captured
    records can be grouped by layer without relying on traversal order.
    """

    def __init__(self):
        self._records: list[dict[str, Any]] = []
        self._original_call = None
        self._active_family: Optional[str] = None
        self._active_task: Optional[str] = None
        self._active_prompt_id: Optional[str] = None
        self._active_segment: Optional[str] = None  # "prompt" or "generation"
        self._token_counter_by_segment: dict[str, int] = {"prompt": 0, "generation": 0}

    def set_active(
        self,
        *,
        family: str,
        task: str,
        prompt_id: str,
        segment: str,
        token_counter_inc: int = 0,
    ) -> None:
        self._active_family = family
        self._active_task = task
        self._active_prompt_id = prompt_id
        self._active_segment = segment
        if token_counter_inc:
            self._token_counter_by_segment[segment] = (
                self._token_counter_by_segment.get(segment, 0) + token_counter_inc
            )

    @property
    def tokens_by_segment(self) -> dict[str, int]:
        return dict(self._token_counter_by_segment)

    def __enter__(self):
        from mlx_lm.models.qwen3_next import Qwen3NextSparseMoeBlock

        original = Qwen3NextSparseMoeBlock.__call__
        self._original_call = original
        capture = self

        def wrapped(self_block, x):
            # Single-pass: replicate the full forward inline so the router
            # decisions we record are bit-identical to those used for expert
            # dispatch. No double-compute.
            import mlx.core as mx

            if self_block.sharding_group is not None:  # pragma: no cover
                from mlx_lm.models.qwen3_next import sum_gradients

                x = sum_gradients(self_block.sharding_group)(x)

            gates = self_block.gate(x)
            gates = mx.softmax(gates, axis=-1, precise=True)
            k = self_block.top_k
            inds = mx.argpartition(gates, kth=-k, axis=-1)[..., -k:]
            scores = mx.take_along_axis(gates, inds, axis=-1)
            if self_block.norm_topk_prob:
                scores = scores / scores.sum(axis=-1, keepdims=True)

            # Materialize inds; capture tolist() (cheap — small tensor).
            mx.eval(inds)
            inds_np = inds.tolist()
            layer_idx = getattr(self_block, "_profiler_layer_idx", -1)

            per_token_experts: list[list[int]] = []
            for b in inds_np:
                for tok in b:
                    per_token_experts.append(tok)

            num_tokens = len(per_token_experts)
            # Only increment distinct-token counter at layer 0 so the total
            # isn't multiplied by num_layers (every MoE layer sees the same
            # tokens in a single forward pass).
            if layer_idx == 0:
                seg = capture._active_segment or "prompt"
                capture._token_counter_by_segment[seg] = (
                    capture._token_counter_by_segment.get(seg, 0) + num_tokens
                )

            capture._records.append(
                {
                    "layer_idx": layer_idx,
                    "family": capture._active_family,
                    "task": capture._active_task,
                    "prompt_id": capture._active_prompt_id,
                    "segment": capture._active_segment,
                    "num_tokens": num_tokens,
                    "per_token_experts": per_token_experts,
                }
            )

            # Complete the forward pass inline.
            y = self_block.switch_mlp(x, inds)
            y = (y * scores[..., None]).sum(axis=-2)

            shared_y = self_block.shared_expert(x)
            shared_y = mx.sigmoid(self_block.shared_expert_gate(x)) * shared_y
            y = y + shared_y

            if self_block.sharding_group is not None:  # pragma: no cover
                y = mx.distributed.all_sum(y, group=self_block.sharding_group)

            return y

        Qwen3NextSparseMoeBlock.__call__ = wrapped
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        from mlx_lm.models.qwen3_next import Qwen3NextSparseMoeBlock

        if self._original_call is not None:
            Qwen3NextSparseMoeBlock.__call__ = self._original_call
            self._original_call = None
        return False

    def drain(self) -> list[dict[str, Any]]:
        out = self._records
        self._records = []
        return out


# --- Helpers ------------------------------------------------------------------


def verify_mlx_lm_version() -> None:
    import mlx_lm

    actual = mlx_lm.__version__
    if actual != EXPECTED_MLX_LM_VERSION:
        print(
            f"WARNING: mlx_lm version mismatch — expected {EXPECTED_MLX_LM_VERSION}, got {actual}. "
            "Monkey-patch target (Qwen3NextSparseMoeBlock.__call__) may need re-validation.",
            file=sys.stderr,
            flush=True,
        )


def verify_model_sha(model_path: pathlib.Path, expected_sha256: str) -> None:
    config = model_path / "config.json"
    if not config.exists():
        # HuggingFace cache layout: snapshots/<rev>/config.json
        snapshots = model_path / "snapshots"
        if snapshots.exists():
            for rev in snapshots.iterdir():
                candidate = rev / "config.json"
                if candidate.exists():
                    config = candidate
                    break
    if not config.exists():
        raise FileNotFoundError(f"config.json not found under {model_path}")
    actual = hashlib.sha256(config.read_bytes()).hexdigest()
    if actual != expected_sha256:
        raise SystemExit(
            f"SHA guard: config.json hash mismatch.\n"
            f"  expected: {expected_sha256}\n"
            f"  actual:   {actual}\n"
            f"  path:     {config}"
        )


def resolve_model_path(model_path_arg: str) -> pathlib.Path:
    """Resolve to the HF-cache snapshot directory that mlx_lm.load can read."""
    p = pathlib.Path(model_path_arg).expanduser()
    if (p / "config.json").exists():
        return p
    snapshots = p / "snapshots"
    if snapshots.exists():
        revs = [r for r in snapshots.iterdir() if (r / "config.json").exists()]
        if revs:
            return revs[0]
    raise FileNotFoundError(f"No loadable snapshot under {p}")


def stamp_layer_indices(model) -> int:
    """Walk the text model's decoder layers and stamp a stable
    `_profiler_layer_idx` attribute on every SparseMoeBlock. Returns the
    number of MoE layers found. Non-MoE layers are warned."""
    from mlx_lm.models.qwen3_next import Qwen3NextSparseMoeBlock

    text = model.language_model.model
    n_moe = 0
    for idx, layer in enumerate(text.layers):
        mlp = getattr(layer, "mlp", None)
        if isinstance(mlp, Qwen3NextSparseMoeBlock):
            mlp._profiler_layer_idx = idx
            n_moe += 1
        else:
            print(
                f"WARNING: layer {idx} mlp is {type(mlp).__name__}, not a SparseMoeBlock",
                file=sys.stderr,
                flush=True,
            )
    return n_moe


def load_anchor_prompts(path: pathlib.Path, task_filter: Optional[str] = None) -> list[dict]:
    records = []
    with path.open() as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            d = json.loads(line)
            if task_filter and d.get("task") != task_filter:
                continue
            records.append(d)
    return records


def build_input_text(tokenizer, prompt: str) -> str:
    """Render the user-prompt via the tokenizer's chat template if present."""
    try:
        return tokenizer.apply_chat_template(
            [{"role": "user", "content": prompt}],
            add_generation_prompt=True,
            tokenize=False,
        )
    except Exception:
        return prompt


# --- Aggregation --------------------------------------------------------------


def build_raw_counts(
    records: list[dict[str, Any]],
    num_layers: int,
    num_experts: int,
) -> dict[str, Any]:
    """Aggregate captured records into per-(task, layer, expert) counts."""
    # task_counts[task][layer][expert] = int
    task_counts: dict[str, list[list[int]]] = {}
    task_tokens: dict[str, int] = defaultdict(int)
    task_family: dict[str, str] = {}

    for rec in records:
        t = rec["task"]
        layer = rec["layer_idx"]
        if t not in task_counts:
            task_counts[t] = [[0] * num_experts for _ in range(num_layers)]
        task_family[t] = rec["family"]
        # Count distinct tokens only at layer 0 (every MoE layer sees the same
        # tokens in a single forward; counting at every layer would multiply
        # by num_layers).
        if layer == 0:
            task_tokens[t] += rec["num_tokens"]
        layer_counts = task_counts[t][layer]
        for per_tok in rec["per_token_experts"]:
            for e in per_tok:
                layer_counts[e] += 1

    return {
        "task_counts": task_counts,
        "task_tokens": dict(task_tokens),
        "task_family": task_family,
        "num_layers": num_layers,
        "num_experts": num_experts,
    }


def compute_family_artifact(
    family: str,
    raw: dict[str, Any],
    top_pct: int,
    model_sha256_config: str,
    prompts_processed: int,
    top_k_per_token: int,
    mlx_lm_version: str,
) -> dict[str, Any]:
    """Aggregate per-family, then pick top-pct experts per layer (ties: lowest idx)."""
    import math

    num_layers = raw["num_layers"]
    num_experts = raw["num_experts"]
    num_sel = num_experts * top_pct // 100

    # Sum per-layer per-expert across all tasks in this family.
    counts = [[0] * num_experts for _ in range(num_layers)]
    tokens = 0
    for task, tcounts in raw["task_counts"].items():
        if raw["task_family"][task] != family:
            continue
        tokens += raw["task_tokens"][task]
        for li in range(num_layers):
            src = tcounts[li]
            dst = counts[li]
            for e in range(num_experts):
                dst[e] += src[e]

    per_layer = []
    for li in range(num_layers):
        row = counts[li]
        # Sort by (-count, expert_idx) — deterministic tie-break: lower idx wins.
        ranked = sorted(range(num_experts), key=lambda e: (-row[e], e))
        top = sorted(ranked[:num_sel])  # store sorted ascending for stability

        # KL(routing_dist || uniform_256)
        total = sum(row)
        kl = 0.0
        if total > 0:
            uniform_p = 1.0 / num_experts
            for e in range(num_experts):
                p = row[e] / total
                if p > 0:
                    kl += p * math.log(p / uniform_p)

        per_layer.append(
            {
                "layer": li,
                "top_experts": top,
                "activation_counts": row,
                "kl_div_vs_uniform": round(kl, 6),
                "total_activations": total,
            }
        )

    return {
        "family": family,
        "model_sha256_config": model_sha256_config,
        "mlx_lm_version": mlx_lm_version,
        "num_layers": num_layers,
        "num_experts_per_layer": num_experts,
        "top_k_per_token": top_k_per_token,
        "top_pct_selected": top_pct,
        "num_experts_selected_per_layer": num_sel,
        "prompts_processed": prompts_processed,
        "tokens_processed": tokens,
        "per_layer": per_layer,
        "generated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }


def jaccard(a: list[int], b: list[int]) -> float:
    sa, sb = set(a), set(b)
    if not sa and not sb:
        return 1.0
    return len(sa & sb) / len(sa | sb)


def per_task_top_experts(
    raw: dict[str, Any], top_pct: int
) -> dict[str, list[list[int]]]:
    """Return {task -> [top-N experts per layer]} from raw per-task counts.

    Applies the same deterministic tie-break as the per-family aggregate
    (sort by (-count, expert_idx), lower idx wins on ties).
    """
    num_layers = raw["num_layers"]
    num_experts = raw["num_experts"]
    num_sel = num_experts * top_pct // 100
    out: dict[str, list[list[int]]] = {}
    for task, tcounts in raw["task_counts"].items():
        per_layer = []
        for li in range(num_layers):
            row = tcounts[li]
            ranked = sorted(range(num_experts), key=lambda e: (-row[e], e))
            per_layer.append(sorted(ranked[:num_sel]))
        out[task] = per_layer
    return out


def within_family_pairwise_jaccard(
    task_top: dict[str, list[list[int]]],
    task_family: dict[str, str],
    family: str,
) -> dict[str, Any]:
    """Mean pairwise Jaccard across tasks within `family`, averaged over
    all (task_pair, layer) combinations. Returns:
        {
          "tasks": [task names],
          "mean_jaccard": float,
          "pair_means": {"<task_a>__vs__<task_b>": mean_over_layers, ...},
          "pair_layers": {"<pair>": [per-layer Jaccard, ...], ...},
        }
    """
    tasks = sorted(t for t, f in task_family.items() if f == family)
    pair_means: dict[str, float] = {}
    pair_layers: dict[str, list[float]] = {}
    all_pair_means: list[float] = []
    for i, a in enumerate(tasks):
        for b in tasks[i + 1 :]:
            pls = [
                jaccard(la, lb) for la, lb in zip(task_top[a], task_top[b])
            ]
            mean = sum(pls) / len(pls) if pls else 0.0
            key = f"{a}__vs__{b}"
            pair_means[key] = round(mean, 6)
            pair_layers[key] = [round(x, 6) for x in pls]
            all_pair_means.append(mean)
    overall = sum(all_pair_means) / len(all_pair_means) if all_pair_means else 0.0
    return {
        "tasks": tasks,
        "mean_jaccard": round(overall, 6),
        "pair_means": pair_means,
        "pair_layers": pair_layers,
    }


def cohesion_verdict(mean_jaccard: float) -> str:
    """≥0.70 cohesive; 0.40-0.70 ambiguous; <0.40 split-candidate."""
    if mean_jaccard >= 0.70:
        return "cohesive"
    if mean_jaccard >= 0.40:
        return "ambiguous"
    return "split-candidate"


def hierarchical_cluster_boundary(
    task_top: dict[str, list[list[int]]],
    tasks: list[str],
) -> Optional[list[list[str]]]:
    """Agglomerative single-linkage clustering with Jaccard distance on
    layer-averaged top-expert sets. Returns the two-cluster split of `tasks`
    at the last merge before all tasks join, or None if fewer than 2 tasks.

    Distance between two tasks = 1 - mean-Jaccard-across-layers.
    Single linkage: cluster distance = minimum pairwise distance.
    """
    n = len(tasks)
    if n < 2:
        return None

    # Pairwise distance matrix (mean over layers).
    def task_pair_dist(a: str, b: str) -> float:
        pls = [jaccard(la, lb) for la, lb in zip(task_top[a], task_top[b])]
        m = sum(pls) / len(pls) if pls else 0.0
        return 1.0 - m

    # Initial singleton clusters.
    clusters: list[list[str]] = [[t] for t in tasks]

    def cluster_dist(ci: list[str], cj: list[str]) -> float:
        return min(task_pair_dist(a, b) for a in ci for b in cj)

    # Walk merges; record the split before the penultimate join.
    prev_two_clusters: Optional[list[list[str]]] = None
    while len(clusters) > 1:
        if len(clusters) == 2:
            prev_two_clusters = [sorted(c) for c in clusters]
        # Find the closest pair.
        best = None
        best_d = float("inf")
        for i in range(len(clusters)):
            for j in range(i + 1, len(clusters)):
                d = cluster_dist(clusters[i], clusters[j])
                if d < best_d:
                    best_d = d
                    best = (i, j)
        if best is None:  # pragma: no cover
            break
        i, j = best
        merged = clusters[i] + clusters[j]
        clusters = [c for k, c in enumerate(clusters) if k not in (i, j)] + [merged]

    # Sort cluster list so the smaller cluster appears first (deterministic).
    if prev_two_clusters is not None:
        prev_two_clusters.sort(key=lambda c: (len(c), c))
    return prev_two_clusters


def compute_cross_family_overlap(family_artifacts: dict[str, dict]) -> dict[str, Any]:
    """Per-layer + averaged Jaccard on top-N sets across family pairs."""
    families = sorted(family_artifacts)
    pairs = [(a, b) for i, a in enumerate(families) for b in families[i + 1 :]]
    result = {"pairs": {}, "families": families}
    for a, b in pairs:
        per_layer = []
        layers_a = family_artifacts[a]["per_layer"]
        layers_b = family_artifacts[b]["per_layer"]
        for la, lb in zip(layers_a, layers_b):
            per_layer.append(jaccard(la["top_experts"], lb["top_experts"]))
        avg = sum(per_layer) / len(per_layer) if per_layer else 0.0
        result["pairs"][f"{a}__vs__{b}"] = {
            "mean_jaccard": round(avg, 6),
            "per_layer": [round(x, 6) for x in per_layer],
        }
    return result


# --- Main ---------------------------------------------------------------------


def main() -> int:
    ap = argparse.ArgumentParser(
        description="Sprint D expert activation profiler (MoE-Sieve Tier 2 input)."
    )
    ap.add_argument("--model-path", required=True, help="HF cache dir OR snapshot path")
    ap.add_argument("--expected-sha256", required=True, help="config.json SHA-256 (pin)")
    ap.add_argument(
        "--anchor-prompts",
        default="training_data/routing_profiles/anchor_prompts.jsonl",
    )
    ap.add_argument("--output-dir", default="training_data/routing_profiles/")
    ap.add_argument("--top-pct", type=int, default=25)
    ap.add_argument("--max-prompt-tokens", type=int, default=2048)
    ap.add_argument("--max-gen-tokens", type=int, default=400)
    ap.add_argument("--limit", type=int, default=0, help="Cap number of prompts (0 = all)")
    ap.add_argument("--task", default=None, help="Filter to a single task_name")
    ap.add_argument(
        "--force",
        action="store_true",
        help="Overwrite existing profile_routing_*.json outputs",
    )
    ap.add_argument("--verbose", action="store_true")
    args = ap.parse_args()

    anchor_path = pathlib.Path(args.anchor_prompts).expanduser()
    out_dir = pathlib.Path(args.output_dir).expanduser()
    out_dir.mkdir(parents=True, exist_ok=True)

    # Sanity: don't overwrite without --force.
    if not args.force:
        for fam in ("reasoning_think", "classify_notink", "structured_notink"):
            candidate = out_dir / f"profile_routing_{fam}.json"
            if candidate.exists():
                print(
                    f"ERROR: {candidate} already exists; pass --force to overwrite",
                    file=sys.stderr,
                )
                return 2

    verify_mlx_lm_version()
    model_path = resolve_model_path(args.model_path)
    verify_model_sha(model_path, args.expected_sha256)
    print(f"Model path (resolved): {model_path}", flush=True)

    # Defer mlx_lm import until after SHA guard.
    import mlx.core as mx  # noqa: F401
    import mlx_lm
    from mlx_lm import load, stream_generate

    model, tokenizer = load(str(model_path))
    n_moe = stamp_layer_indices(model)
    print(f"Stamped {n_moe} MoE layers", flush=True)

    prompts = load_anchor_prompts(anchor_path, task_filter=args.task)
    if args.limit > 0:
        prompts = prompts[: args.limit]
    print(f"Loaded {len(prompts)} prompts from {anchor_path}", flush=True)

    num_experts = model.args.text_config["num_experts"]
    top_k = model.args.text_config["num_experts_per_tok"]
    num_layers = model.args.text_config["num_hidden_layers"]
    print(
        f"Config: num_layers={num_layers} num_experts={num_experts} top_k={top_k}",
        flush=True,
    )

    all_records: list[dict[str, Any]] = []
    t0 = time.monotonic()

    with RouterCapture() as capture:
        for i, p in enumerate(prompts):
            prompt_text = p["prompt"]
            rendered = build_input_text(tokenizer, prompt_text)

            # Tokenize with cap (truncate excess to protect forward pass).
            input_ids = tokenizer.encode(rendered)
            if len(input_ids) > args.max_prompt_tokens:
                if args.verbose:
                    print(
                        f"  [{i + 1}] truncating prompt "
                        f"{len(input_ids)} -> {args.max_prompt_tokens} tokens",
                        flush=True,
                    )
                input_ids = input_ids[: args.max_prompt_tokens]

            prompt_id = f"{p['task']}__{i}"
            # Prompt-side forward (prefill): routed during first generate() call
            # — set segment='prompt' before invoking stream_generate; the wrapped
            # __call__ captures every SparseMoeBlock call.
            capture.set_active(
                family=p["family"],
                task=p["task"],
                prompt_id=prompt_id,
                segment="prompt",
            )
            # stream_generate yields one GenerationResponse per token. Switch
            # segment to 'generation' after the first yielded token.
            gen_tok_count = 0
            first = True
            try:
                for resp in stream_generate(
                    model, tokenizer, input_ids, max_tokens=args.max_gen_tokens
                ):
                    if first:
                        capture.set_active(
                            family=p["family"],
                            task=p["task"],
                            prompt_id=prompt_id,
                            segment="generation",
                        )
                        first = False
                    gen_tok_count += 1
            except Exception as e:  # pragma: no cover
                print(f"  [{i + 1}] generation error: {e}", file=sys.stderr, flush=True)

            if (i + 1) % 10 == 0 or i == len(prompts) - 1:
                seg = capture.tokens_by_segment
                elapsed = time.monotonic() - t0
                print(
                    f"  [{i + 1:3d}/{len(prompts)}] gen_toks={gen_tok_count:4d}  "
                    f"cumulative: prompt={seg.get('prompt', 0):,}  "
                    f"gen={seg.get('generation', 0):,}  "
                    f"elapsed={elapsed / 60:.1f}m",
                    flush=True,
                )

        all_records = capture.drain()

    # Aggregate.
    raw = build_raw_counts(all_records, num_layers=num_layers, num_experts=num_experts)
    raw_path = out_dir / "raw_activation_counts.json"
    raw_path.write_text(
        json.dumps(
            {
                "model_sha256_config": args.expected_sha256,
                "num_layers": num_layers,
                "num_experts": num_experts,
                "task_counts": raw["task_counts"],
                "task_tokens": raw["task_tokens"],
                "task_family": raw["task_family"],
                "generated_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            }
        )
    )
    print(f"Wrote {raw_path}", flush=True)

    # Per-family artifacts.
    families = sorted(set(raw["task_family"].values()))
    family_artifacts = {}
    for fam in families:
        fam_prompts = sum(1 for p in prompts if p["family"] == fam)
        art = compute_family_artifact(
            family=fam,
            raw=raw,
            top_pct=args.top_pct,
            model_sha256_config=args.expected_sha256,
            prompts_processed=fam_prompts,
            top_k_per_token=top_k,
            mlx_lm_version=mlx_lm.__version__,
        )
        art_path = out_dir / f"profile_routing_{fam}.json"
        art_path.write_text(json.dumps(art))
        family_artifacts[fam] = art
        print(
            f"Wrote {art_path}  |  prompts={fam_prompts}  tokens={art['tokens_processed']:,}",
            flush=True,
        )

    # Cross-family overlap summary (stdout only — full analysis in Epic 3 decision doc).
    overlap = compute_cross_family_overlap(family_artifacts)
    print("\n=== Cross-family Jaccard (mean across layers) ===")
    for pair, stats in overlap["pairs"].items():
        print(f"  {pair}: {stats['mean_jaccard']:.4f}")

    seg = capture.tokens_by_segment
    print(
        f"\nTokens processed total: prompt={seg.get('prompt', 0):,}  "
        f"generation={seg.get('generation', 0):,}",
        flush=True,
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
