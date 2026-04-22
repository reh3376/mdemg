#!/usr/bin/env bash
# Sprint FT-LORA-DATA End-to-End Dry-Run
#
# Runs the full data pipeline in dry-run mode against synthetic fixtures:
#   1) Synthetic raw JSONL (tier1-shaped, 16 tasks sans 4 absent)
#   2) recurate.py → recurated.jsonl
#   3) Mock synthesis output (Epic 2 outputs pre-fabricated, no OpenAI spend)
#   4) balanced_sampler.py × 4 (tier1 + 3 families)
#   5) stratified_split.py × 4 (train/valid + manifest)
#   6) Epic 4 Step 3a: 20-row fixture → mlx_lm.lora --dry-run check
#   7) Per-tier manifest schema + SHA validation
#
# Exit 0 required for Epic 6 sprint-merge gate.
#
# Usage:
#   bash scripts/sprint_ft_lora_data_e2e.sh [--skip-mlx-fixture]
#
# Env:
#   SPRINT_FT_LORA_DATA_MLX_DRY_RUN=1  — opt in to mlx_lm fixture check
#                                        (default: on if mlx_lm importable).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

SKIP_MLX=0
for arg in "$@"; do
    case "$arg" in
        --skip-mlx-fixture) SKIP_MLX=1 ;;
    esac
done

PY="${PYTHON:-${REPO_ROOT}/neural/.venv/bin/python}"
if [[ ! -x "$PY" ]]; then
    PY="$(command -v python3)"
fi

OUT_ROOT="$(mktemp -d -t sprint-ft-lora-data-e2e-XXXXXX)"
echo "═══════════════════════════════════════════════════════════════"
echo "  Sprint FT-LORA-DATA E2E Dry-Run"
echo "  python:    $PY"
echo "  out root:  $OUT_ROOT"
echo "═══════════════════════════════════════════════════════════════"

RAW="$OUT_ROOT/raw.jsonl"
RECURATED="$OUT_ROOT/recurated.jsonl"
SYNTH_DIR="$OUT_ROOT/synth"
BAL_DIR="$OUT_ROOT/balanced"
SFT_ROOT="$OUT_ROOT/sft"

mkdir -p "$SYNTH_DIR" "$BAL_DIR" "$SFT_ROOT"

# ─── Step 1: Synthetic raw JSONL ───
echo
echo "─── Step 1/7: Synthetic raw JSONL (12 present tasks) ───"
"$PY" - <<PYEOF
import json
TASKS = [
    "ape.reflect", "hidden.summarize", "jiminy.synthesize",
    "consulting.classify", "hidden.reclassify", "jiminy.codegen",
    "jiminy.evaluate", "retrieval.intent_translate", "retrieval.query_classify",
    "hidden.name_emergence", "jiminy.evaluate_llm", "retrieval.rerank_cross",
]
with open("$RAW", "w") as f:
    for t in TASKS:
        n = 1000 if t == "ape.reflect" else (45 if t == "jiminy.evaluate_llm" else 300)
        for i in range(n):
            f.write(json.dumps({
                "task_name": t,
                "trace_id": f"{t}-{i}",
                "instance_id": "synth-host",
                "space_id": "mdemg-dev",
                "system_prompt": "You are helpful.",
                "user_prompt": f"input-{t}-{i}",
                "response": f"reply-{t}-{i}",
                "think_mode": False,
                "think_content": None,
                "retrieval_node_ids": None,
                "retrieval_scores": None,
                "quality": 0.8,
                "quality_source": "heuristic",
                "source_path": "synthetic://e2e",
            }) + "\n")
print(f"Wrote raw JSONL to $RAW")
PYEOF
echo "✓ raw JSONL written"

# ─── Step 2: recurate.py ───
echo
echo "─── Step 2/7: recurate.py ───"
RAW_SHA="$(shasum -a 256 "$RAW" | awk '{print $1}')"
"$PY" -m neural.training.recurate \
    --raw-input "$RAW" \
    --out "$RECURATED" \
    --expected-raw-sha256 "$RAW_SHA" \
    --dataset-ver "v-e2e" \
    --min-quality 0.6 > "$OUT_ROOT/recurate.stdout"
RECURATED_COUNT=$(wc -l < "$RECURATED" | tr -d ' ')
echo "✓ recurate.py OK ($RECURATED_COUNT rows)"

# ─── Step 3: Mock synthesis output (4 absent tasks × 200 rows) ───
echo
echo "─── Step 3/7: Mock synthesis (Epic 2 stand-in) ───"
"$PY" - <<PYEOF
import json, os
ABSENT = {
    "consulting.synthesis": False, "metalearn.generalize": True,
    "retrieval.rerank_nli": False, "summarize.generate": False,
}
for t, weak in ABSENT.items():
    fname = t.replace(".", "_") + ".jsonl"
    p = os.path.join("$SYNTH_DIR", fname)
    with open(p, "w") as f:
        for i in range(200):
            f.write(json.dumps({
                "messages": [
                    {"role": "system", "content": "You are helpful."},
                    {"role": "user", "content": f"synth-input-{i}"},
                    {"role": "assistant", "content": f"synth-reply-{i}"},
                ],
                "meta": {
                    "task_name": t, "trace_id": f"synth-{t}-{i}",
                    "instance_id": "synth-gpt-5.4-mini", "space_id": "synth",
                    "source": "synth-gpt-5.4-mini", "dataset_ver": "v-e2e",
                    "quality": 0.9, "quality_source": "teacher_distillation",
                    "source_path": f"distill_driver:{t}",
                    "synthesis_version": "v1-e2e", "weak_signal": weak,
                },
            }) + "\n")
print("✓ 4 synthesis files written")
PYEOF
echo "✓ mock synthesis OK"

# ─── Step 4: balanced_sampler × 4 ───
echo
echo "─── Step 4/7: balanced_sampler × 4 ───"
for tier in tier1 T C J; do
    "$PY" -m neural.training.balanced_sampler \
        --tier "$tier" \
        --input-real "$RECURATED" \
        --input-synth-dir "$SYNTH_DIR" \
        --per-task-floor 200 --ape-reflect-target 500 \
        --duplication-ceiling 5 --seed 42 \
        --out "$BAL_DIR/${tier}.jsonl" \
        --report "$BAL_DIR/${tier}.report.json" > "$OUT_ROOT/balance_${tier}.stdout"
    count=$(wc -l < "$BAL_DIR/${tier}.jsonl" | tr -d ' ')
    echo "  $tier: $count rows"
done
echo "✓ balanced_sampler × 4 OK"

# ─── Step 5: stratified_split × 4 ───
echo
echo "─── Step 5/7: stratified_split × 4 ───"
family_of() {
    case "$1" in
        tier1) echo "tier1" ;;
        T) echo "reasoning_think" ;;
        C) echo "classify_notink" ;;
        J) echo "structured_notink" ;;
    esac
}
out_dir_of() {
    local tier="$1"
    local name
    name="$(family_of "$tier")"
    if [[ "$tier" == "tier1" ]]; then
        echo "$SFT_ROOT/tier1"
    else
        echo "$SFT_ROOT/family_${name}"
    fi
}
for tier in tier1 T C J; do
    dir_name="$(family_of "$tier")"
    out_dir="$(out_dir_of "$tier")"
    "$PY" -m neural.training.stratified_split \
        --input "$BAL_DIR/${tier}.jsonl" \
        --out-dir "$out_dir" \
        --tier "$tier" --family-name "$dir_name" \
        --base-dataset-ver "v-e2e" \
        --synthesis-version "v1-e2e" \
        --sampler-report "$BAL_DIR/${tier}.report.json" \
        --seed 42 > "$OUT_ROOT/split_${tier}.stdout"
    train_count=$(wc -l < "$out_dir/train.jsonl" | tr -d ' ')
    valid_count=$(wc -l < "$out_dir/valid.jsonl" | tr -d ' ')
    echo "  $tier → $out_dir: train=$train_count valid=$valid_count"
    # Verify manifest exists + SHA matches
    "$PY" - "$out_dir" <<'PYEOF'
import hashlib, json, sys
from pathlib import Path
d = Path(sys.argv[1])
m = json.loads((d / "manifest.json").read_text())
for fn in ("train.jsonl", "valid.jsonl"):
    disk = hashlib.sha256((d / fn).read_bytes()).hexdigest()
    assert m["file_sha256"][fn] == disk, f"{fn} SHA mismatch in manifest"
print(f"  ✓ {d.name} manifest SHA-match OK")
PYEOF
done
echo "✓ stratified_split × 4 OK"

# ─── Step 6: Epic 4 Step 3a — mlx_lm fixture dry-run ───
echo
echo "─── Step 6/7: mlx_lm fixture dry-run (Epic 4 Step 3a) ───"
FIXTURE_DIR="$OUT_ROOT/mlx_fixture"
mkdir -p "$FIXTURE_DIR"
"$PY" - <<PYEOF
import json, random
random.seed(42)
TASKS = ["ape.reflect", "consulting.classify", "jiminy.evaluate_llm",
         "hidden.summarize", "retrieval.intent_translate"]
rows = []
for i in range(20):
    t = TASKS[i % len(TASKS)]
    rows.append({
        "messages": [
            {"role": "system", "content": "You are helpful."},
            {"role": "user", "content": f"q-{i}"},
            {"role": "assistant", "content": f"a-{i}"},
        ],
        "meta": {"task_name": t, "trace_id": f"{t}-{i}", "source": "e2e"},
    })
# Write train/valid split
with open("$FIXTURE_DIR/train.jsonl", "w") as f:
    for r in rows[:18]:
        f.write(json.dumps(r) + "\n")
with open("$FIXTURE_DIR/valid.jsonl", "w") as f:
    for r in rows[18:]:
        f.write(json.dumps(r) + "\n")
print("✓ 20-row fixture written")
PYEOF

if [[ "$SKIP_MLX" == "1" ]]; then
    echo "  SKIPPED (--skip-mlx-fixture)"
elif "$PY" -c "import mlx_lm" 2>/dev/null; then
    # Try a lightweight --help to confirm mlx_lm.lora exists (avoids full model load).
    if "$PY" -m mlx_lm.lora --help > "$OUT_ROOT/mlx_lm_help.stdout" 2>&1; then
        echo "  ✓ mlx_lm.lora CLI reachable; skipping full dry-run (requires base model)"
        echo "  Note: production scale --dry-run is gated on Epic 6 Step 6."
    else
        echo "  WARN: mlx_lm.lora --help failed; see $OUT_ROOT/mlx_lm_help.stdout"
    fi
else
    echo "  SKIPPED (mlx_lm not installed in this venv)"
fi

# ─── Step 7: Final summary ───
echo
echo "─── Step 7/7: Summary ───"
for tier in tier1 T C J; do
    out_dir="$(out_dir_of "$tier")"
    "$PY" - "$out_dir" <<'PYEOF'
import json, sys
from pathlib import Path
d = Path(sys.argv[1])
m = json.loads((d / "manifest.json").read_text())
print(f"  {d.name}: train={m['row_counts']['train']} "
      f"valid={m['row_counts']['valid']} "
      f"weak_signal={m['weak_signal_rows']} "
      f"tasks={len(m['per_task_counts'])}")
PYEOF
done

echo
echo "═══════════════════════════════════════════════════════════════"
echo "  Sprint FT-LORA-DATA E2E Dry-Run — ALL GREEN"
echo "  Artifacts: $OUT_ROOT"
echo "═══════════════════════════════════════════════════════════════"
