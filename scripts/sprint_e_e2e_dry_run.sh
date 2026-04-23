#!/usr/bin/env bash
# Sprint FT-LORA-E End-to-End Dry-Run
#
# Runs the full Phase 5 invocation matrix in --dry-run mode:
#   1) Tier 1 universal adapter (attn + shared expert, r=32 α=64)
#   2) Tier 2 × 3 families (routed experts, r=8 α=16)
#   3) quantize_asymmetric.py --dry-run classification summary
#
# All 5 steps must exit 0 for the Epic 6 E2E gate to pass. No training is
# launched, no model is converted, and no adapter weights are written
# (only the dry-run YAML sidecar lands under each adapter directory).
#
# Usage:
#   bash scripts/sprint_e_e2e_dry_run.sh [--skip-quant-dry-run]
#
# Env:
#   SPRINT_C_MODEL_PATH          — override Sprint C model dir.
#   SPRINT_E_HEAVY_INTEGRATION=1 — allow the full-model load in step 5 (≈35 GB).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# Post-Phase-5 pivot (2026-04-22): default swapped from Qwen3.6-35B-A3B-mxfp4
# (MoE, abandoned — Metal 499K ceiling) to dense Qwen3-14B-4bit. Set
# SPRINT_C_MODEL_PATH to override (MoE model may be deleted from HF cache).
MODEL="${SPRINT_C_MODEL_PATH:-$HOME/hf_cache/hub/models--mlx-community--Qwen3-14B-4bit}"
PIN="${SPRINT_C_MODEL_SHA:-a54ec18ffe24f3c909e9556471dc156ed9b3b61b872008831c7cba9d4768b4a5}"
PROFILES="$REPO_ROOT/training_data/routing_profiles"
OUT_ROOT="$(mktemp -d -t sprint-e-e2e-XXXXXX)"
DATASET="$OUT_ROOT/tiny-dataset"

echo "═══════════════════════════════════════════════════════════════"
echo "  Sprint FT-LORA-E End-to-End Dry-Run"
echo "  Model path: $MODEL"
echo "  Out root:   $OUT_ROOT"
echo "═══════════════════════════════════════════════════════════════"

# Activate venv if not already active.
if [[ -z "${VIRTUAL_ENV:-}" ]]; then
    if [[ -f "$HOME/.venv/mdemg-ft-lora/bin/activate" ]]; then
        # shellcheck disable=SC1091
        source "$HOME/.venv/mdemg-ft-lora/bin/activate"
    fi
fi

if [[ ! -f "$MODEL/config.json" ]]; then
    echo "ERROR: Sprint C model config.json not found at $MODEL"
    echo "Set SPRINT_C_MODEL_PATH=/path/to/model to override."
    exit 2
fi

# Sanity: confirm the model SHA still matches the Sprint C pin.
OBSERVED_SHA="$(shasum -a 256 "$MODEL/config.json" | awk '{print $1}')"
if [[ "$OBSERVED_SHA" != "$PIN" ]]; then
    echo "ERROR: Sprint C model config.json SHA drift"
    echo "  observed: $OBSERVED_SHA"
    echo "  expected: $PIN"
    exit 2
fi
echo "✓ Sprint C model SHA matches pin"

# Tiny dataset (40 rows — enough for iters computation, none is read at dry-run).
mkdir -p "$DATASET"
python -c "
import json
with open('$DATASET/train.jsonl', 'w') as f:
    for i in range(40):
        f.write(json.dumps({'text': f'e2e sample {i}'}) + '\n')
"
echo "✓ Synthetic dataset written to $DATASET"

# ─── Step 1: Tier 1 dry-run ───
echo
echo "─── Step 1/5: Tier 1 universal dry-run ───"
python -m neural.training.train_ft \
    --tier 1 --mode sft \
    --base-model "$MODEL" --expected-sha256 "$PIN" \
    --dataset "$DATASET" --adapter-path "$OUT_ROOT/tier1" \
    --n-epochs 3 --batch-size 4 --dry-run > "$OUT_ROOT/tier1.stdout"
grep -q "target_modules_source: tier1-default" "$OUT_ROOT/tier1.stdout"
grep -q "lora_rank: 32" "$OUT_ROOT/tier1.stdout"
echo "✓ Tier 1 dry-run OK"

# ─── Steps 2-4: Tier 2 × 3 families ───
for family in reasoning-think classify-notink structured-notink; do
    underscore="${family//-/_}"
    profile="$PROFILES/profile_routing_${underscore}.json"
    step_num=$((1 + $(echo "reasoning-think classify-notink structured-notink" | tr ' ' '\n' | grep -n "^${family}$" | cut -d: -f1)))
    echo
    echo "─── Step ${step_num}/5: Tier 2 (${family}) dry-run ───"
    python -m neural.training.train_ft \
        --tier 2 --family "$family" \
        --expert-selection-path "$profile" \
        --base-model "$MODEL" --expected-sha256 "$PIN" \
        --dataset "$DATASET" --adapter-path "$OUT_ROOT/tier2-${family}" \
        --n-epochs 3 --batch-size 4 --dry-run > "$OUT_ROOT/tier2-${family}.stdout"
    grep -q "target_modules_count: 7680" "$OUT_ROOT/tier2-${family}.stdout"
    grep -q "lora_rank: 8" "$OUT_ROOT/tier2-${family}.stdout"
    grep -q "lora_alpha: 16" "$OUT_ROOT/tier2-${family}.stdout"
    echo "✓ Tier 2 (${family}) dry-run OK"
done

# ─── Step 5: Asymmetric-quant dry-run ───
if [[ "${SPRINT_E_HEAVY_INTEGRATION:-0}" == "1" ]]; then
    echo
    echo "─── Step 5/5: Asymmetric-quant dry-run (full model load) ───"
    python -m neural.training.quantize_asymmetric \
        --input-model "$MODEL" --dry-run \
        --output-json "$OUT_ROOT/quant_classification.json" > "$OUT_ROOT/quant.stdout"
    grep -q "Asymmetric-Quant Classification" "$OUT_ROOT/quant.stdout"
    echo "✓ quantize_asymmetric.py --dry-run OK"
else
    echo
    echo "─── Step 5/5: SKIPPED (full-model load requires SPRINT_E_HEAVY_INTEGRATION=1) ───"
    echo "  To run: SPRINT_E_HEAVY_INTEGRATION=1 bash scripts/sprint_e_e2e_dry_run.sh"
fi

echo
echo "═══════════════════════════════════════════════════════════════"
echo "  Sprint FT-LORA-E E2E Dry-Run — ALL GREEN"
echo "  Artifacts: $OUT_ROOT"
echo "═══════════════════════════════════════════════════════════════"
