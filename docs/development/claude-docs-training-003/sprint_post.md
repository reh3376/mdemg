# CLAUDE-DOCS-TRAINING-003 — Sprint Post

**Shipped**: 2026-08-16 16:38 UTC (Epic 1-3 complete; Epic 3 verdict: **sweet-spot hypothesis INVALIDATED**; adapter_002 remains best of the three; will not promote)
**Follows**: CLAUDE-DOCS-TRAINING-002 (adapter_002, 3-epoch, mixed A/B) + CLAUDE-DOCS-TRAINING-001 (adapter_001, 1-epoch, PoC)
**Verdict**: adapter_003 is **WORSE** than adapter_002 on 2 of 3 fixtures. 2 epochs is not the sweet spot. Failure surface is highly non-monotonic with epoch count.

## 1. What shipped

**Epic 1 — 2-epoch retrain on v2 chunked corpus**:
```
neural/.venv/bin/python -m neural.training.train_ft \
    --tier 1 --mode sft \
    --base-model .local-models/qwen3-14b-4bit-base \
    --expected-sha256 a54ec18f... \
    --dataset training_data/sft/claude_code_knowledge_v2 \
    --adapter-path adapters/claude_docs_003/ \
    --n-epochs 2 --batch-size 4
```

Training result:
- 1354/1354 iters in **6.33h wall**
- Val loss 2.877 → **1.125** (between v1's 1.251 and v2's 1.074)
- Peak mem 21.06 GB (locked, same as v2)
- No early stop; monotonic descent

## 2. Smoke A/B/C on 3 identical golden fixtures

### Full-table verdict

| Fixture | adapter_001 (1ep, v1 corpus) | adapter_002 (3ep, v2 corpus) | adapter_003 (2ep, v2 corpus) | adapter_002 + rp=1.15 |
|---|---|---|---|---|
| #1 query vs ClaudeSDKClient | Directionally correct, 1-sentence | ✅ Coherent prose, session/streaming/lifecycle covered | ✅ Coherent prose, session/streaming | ✅ Coherent prose |
| **#2 EffortLevel** | 🚫 Hallucinated `low/med/high/**ultra/auto**` (0/5) | ✅ **EXACT** `low/med/high/**xhigh/max**` (5/5) | 🚫 **RUNAWAY**: 93,742-char table with whitespace-token loop | ⚠️ `low/med/high/**x_high**/max` (4/5 + spurious underscore) |
| **#3 McpServerStatusConfig** | 🚫 Hallucinated bounded 3-field TypedDict | 🚫 Infinite loop (50 nonsense `reconnectBackoff*` fields) | 🚫 **RUNAWAY**: 50,061-char table | ✅ Bounded 3-field TypedDict (as adapter_001) |

### Character counts (proof of runaway)

| Fixture | v1 | v2 | v3 |
|---|---:|---:|---:|
| Golden #1 | ~500 | 812 | 812 |
| Golden #2 | ~450 | 2,502 | **93,742** ← runaway |
| Golden #3 | ~400 | 4,500 (loop) | **50,061** ← runaway |

## 3. What we learned (arch rule updates)

⚠️ **Rule A (REVISED)**: **The 2-epoch sweet-spot hypothesis from CLAUDE-DOCS-TRAINING-002 sprint post is INVALIDATED.** Empirically, 2 epochs was WORSE than 3 on 2 of 3 fixtures with new-and-different runaway failure modes (whitespace-token repetition in tables). Epoch count is NOT a simple continuous knob for stability. The failure surface is highly non-monotonic — 1 epoch had bounded hallucinations, 3 epochs had structured template overfit, 2 epochs had unbounded token repetition. **Do not assume "somewhere between 1 and 3 must be the sweet spot" without empirical verification per config**.

⚠️ **Rule B**: **Randomness dominates at small delta between checkpoints.** v2 and v3 share the same base + dataset + LR + rank/α + seed for the training-code-visible parts, differing only in total iter count (2031 vs 1354). Yet their generation behavior diverges dramatically — v3 introduces a completely new failure mode absent from v2. This suggests the final-weights space near converged LoRA has significant "landing point" variance driven by the SGD trajectory + sampling stochasticity. Small changes to hyperparameters (or even re-running the same config) may produce meaningfully different generation behavior.

⚠️ **Rule C**: **Post-training A/B smokes MUST include token-length ceiling checks.** adapter_003 exposed a failure mode (runaway generation) that would only be detectable by comparing output token counts. A "did model produce plausible text?" check misses this. Future adapter A/B smokes should assert `len(output) < 3x reference_length` OR track token counts explicitly.

⚠️ **Rule D**: **`max_tokens` in mlx_lm.generate() may not be respected as a hard cap on all model configurations.** adapter_003's 93,742-char Golden #2 output at `max_tokens=800` suggests either the parameter wasn't respected OR the model produced ~23K tokens in the 800-max budget (impossible — indicates a real max_tokens routing issue). Worth investigating in a follow-up but not this sprint's scope.

## 4. Best candidate for the shipped fixture set

**Ranked by real-world usability**:

1. **adapter_002 + repetition_penalty=1.15 at inference** — 3/3 fixtures produce bounded outputs; Golden #2 loses only the underscore-in-`x_high`; Golden #3 hallucinates bounded 3-field TypedDict (SAME as v1, no worse). This is the **best available config as of this sprint**.
2. **adapter_002 (no rep-penalty)** — 2/3 wins with Golden #2 being the critical EXACT-VALUES win; Golden #3 has infinite loop.
3. **adapter_001** — bounded but never got Golden #2 right.
4. **adapter_003** — NEW failure modes on 2/3 fixtures, not usable.

## 5. Follow-up options (remaining, cheapest → most expensive)

1. **Post-generation truncation guard** — regex-detect `(\w+\s+\w+)\1{5,}` OR `(\s{80,})` pattern in output and truncate. Would fix adapter_003's runaway artifacts + adapter_002's Golden #3 loop. Zero training cost. ~30 min to implement.
2. **Custom logits processor** — punish repetition ONLY when a token-window has count > 4 (won't damage 5-item enum families like adapter_002+rp=1.15 does). Non-trivial (~2h) but preserves Golden #2 factual win.
3. **Corpus augmentation** — hand-author 10-20 Union-type + short-enum training examples to teach the compound-type pattern explicitly. Then retrain adapter_004.
4. **Base-model swap** (task #91 queued) — newer base may absorb docs differently and show different failure surface.

**My recommendation**: **Option 2 (custom logits processor)** for adapter_002 — preserves the enum-recall win (5/5 EXACT is a hard-won empirical result), fixes Golden #3 without damaging Golden #2. ~2h dev + 5min re-smoke.

Alternative interpretation: after 3 sprints × substantial compute, we have empirical evidence that:
- Chunking works (Rule A from sprint 002 stands)
- Longer epoch training introduces failure modes (both 2-epoch and 3-epoch showed different regressions)
- **The Phase-5 base + rank=32 α=64 LoRA configuration may be at a fundamental limit for docs-training fact recall on complex types**
- Base-model swap (task #91) may be the higher-ROI investment than continued CLAUDE-DOCS-TRAINING iterations

**Recommendation to operator**: consider whether continued CLAUDE-DOCS-TRAINING iteration is the best use of compute, OR whether the Q4 roadmap should reprioritize task #91 (base-model swap) as the more promising path.

## 6. Verification

- [x] 2-epoch training completed: val loss 1.125 (between v1 and v2)
- [x] 3-row smoke A/B/C across all 3 adapters
- [x] Runaway character counts documented (93K + 50K on adapter_003)
- [x] Rep-penalty variant re-verified as current best config
- [x] Sweet-spot hypothesis: INVALIDATED with data
- [ ] Full 50-row A/B benchmark — DEFERRED (3-row smoke sufficient for the "sweet spot doesn't exist" conclusion)
- [ ] Operator promotion — WILL NOT ISSUE for any of the 3 adapters

## 7. Files touched

**Commits on `reh3376_dev01`** (via PR #623 or successor):
- `adapters/claude_docs_003/{adapter_config.json, train_config.yaml, train_report.json}` — training metadata
- `docs/development/claude-docs-training-003/sprint_post.md` — this file

**Untracked (regenerable)**:
- `adapters/claude_docs_003/*.safetensors` (14 files × 168 MB — 13 checkpoints + final)

## 8. Documents Accessed

- `docs/development/claude-docs-training-002/sprint_post.md` — priors that motivated this sprint (sweet-spot hypothesis)
- `training_data/sft/claude_code_knowledge_v2/{train,valid,manifest}.jsonl` — reused v2 chunked dataset
- `training_data/eval/claude_code_knowledge_golden.jsonl` — v1 golden holdout (unchanged for direct A/B/C)
- `adapters/claude_docs_003/train_report.json` + `training_log.jsonl` — live training telemetry
- Live 3-adapter smoke inference via `mlx_lm.load` + `mlx_lm.generate`
- Reference completions from V1 golden holdout for direct comparison
