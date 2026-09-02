# MDEMG-USAGE-LORA-001-Q8 — Sprint Plan (v1.0)

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | MDEMG-USAGE-LORA-001-Q8 |
| Task | #151 |
| Filed | 2026-09-01 (from #150 findings) |
| Author | Roger Henley + Claude (proactive mode) |
| Branch | `reh3376_dev01` |
| Predecessors | #150 MDEMG-USAGE-LORA-001-GGUF (Q5_K_M verdict NO-PROMOTE) |
| Format version | 12-section v1.0 (short-form; 4 epics) |
| Est. wall-clock | ~2h (1 min quantize + ~2h benchmark) |
| Est. spend | $0 (all local) |

## 2. Problem Statement

#150 found my LoRA-fused adapter picks up only +0.004 runtime bonus from Q5_K_M vs v1's +0.074 (20× less). Book-close question: is this Q5_K_M-specific, or a broader quantization sensitivity?

**Q8_0** (7.5-8 BPW vs Q5_K_M's 5.69) preserves more precision. If Q8_0 aggregate ≥ 0.90, the adapter IS shippable — just needs a bigger serving tier (16 GB vs 10 GB). If Q8_0 aggregate < 0.90, adapter has broader quantization sensitivity and NO-PROMOTE is definitive across all reasonable quant tiers.

## 3. Scope & Constraints

### In scope
- `llama-quantize` f16 GGUF → Q8_0 GGUF (~16 GB, ~1 min)
- Bench-serve via llama-server on port **8103** (NOT prod 8102)
- Sanity chat "reply ok" — coherent response before benchmark
- `neural.benchmarks.run_benchmark` — same valid_clean.jsonl + benchmark_phase10.yaml
- 3-way comparison: mine Q5_K_M (0.8426) vs mine Q8_0 (TBD) vs v1 GGUF Q5_K_M (0.9188)
- Verdict + sprint post + PR

### Out of scope
- NO Q4_K_M or other quant tiers (Q5 + Q8 span the reasonable serving range)
- NO promotion to prod (still #E4-gate territory)
- NO touch to production llama-server on 8102 or `.local-models/serving/current.gguf`

### Constraints
- All CLAUDE.md pins from #150 apply (plan-mode-before-change, end-with-docs-accessed, must-follow-12-section-format, live-testing-tier-required, iterate-break-fix-verify, must-comment-sprint-summary-on-pr, must-use-cuid2, never-hardcode-config, mandatory-feature-docs — nothing new user-visible)
- PHASE-E3 arch rule 5 (same-runtime comparison) — this sprint IS same-runtime measurement against #150 (both on llama.cpp)
- Production llama-server on port 8102 UNTOUCHED (verify pre + during + post)

## 4. Dependencies

| Dependency | Status |
|---|---|
| `.local-models/qwen3-14b-mdemg-usage-fused.f16.gguf` (30 GB f16 GGUF) | ✅ from #150, SHA `06023757c4b8…` |
| `/opt/homebrew/bin/llama-quantize` | ✅ |
| `/opt/homebrew/bin/llama-server` | ✅ |
| `configs/benchmark_phase10.yaml`, `training_data/eval/valid_clean.jsonl`, `run_benchmark.py` | ✅ |
| Free disk (~16 GB for Q8_0) | ✅ (700+ GB free) |
| Port 8103 | ✅ free |
| Production 8102 healthy | ✅ verified |

## 5. Implementation Plan (sequential epics)

### Epic 1 — Q8_0 quantize (~1 min)
```bash
llama-quantize .local-models/qwen3-14b-mdemg-usage-fused.f16.gguf \
               .local-models/qwen3-14b-mdemg-usage-fused.Q8_0.gguf Q8_0
```
**Gate**: Q8_0.gguf exists, ~16 GB, SHA captured.

### Epic 2 — Sanity test (~2 min)
Start llama-server on 8103, poll `/v1/models`, curl `reply ok` chat.
**Gate**: 200 response with coherent short output.

### Epic 3 — Benchmark (~2h wall-clock via `run_in_background`)
`run_benchmark --mlx-base-url http://127.0.0.1:8103/v1 --mlx-model-name <Q8_0-path>` (Bash tool harness-native `run_in_background: true`).
**Gate**: benchmark JSON written, aggregate present.

### Epic 4 — 3-way verdict + close-out (~30 min)
Compare aggregate to Q5_K_M (0.8426), v1 GGUF (0.9188), v1 mlx (0.8449).

**Verdict rubric**:
- ✅ PROMOTE-VIA-Q8 if aggregate ≥ 0.9088 → adapter IS shippable at Q8_0 tier
- ⚠️ MIXED if in [0.89, 0.9088)
- ❌ CONFIRM-NO-PROMOTE if < 0.89 → broader quantization sensitivity confirmed; arc definitively closed

Write verdict + sprint post + CHANGELOG + PR comment.

## 6. Testing Plan (3 tiers)

- Tier 1 (unit): N/A — pipeline is subprocess orchestration
- Tier 2 (integration): Epic 2 sanity test
- Tier 3 (live e2e): Epic 3 IS the live Tier-3 test

## 7. Commit Strategy
Single commit at Epic 4 (all sprint artifacts + docs together). Auto-PR fires.

## 8. Verification Checklist
- [ ] Q8_0 GGUF exists, ~16 GB, SHA captured
- [ ] Sanity chat coherent
- [ ] Benchmark JSON written with aggregate
- [ ] Verdict per rubric
- [ ] Sprint post + verdict + CHANGELOG + PR comment
- [ ] Production 8102 UNCHANGED (SHA of `.local-models/serving/current.gguf` target unchanged)
- [ ] Task #151 → completed

## 9. Documentation Update
- `docs/development/mdemg-usage-lora-001-q8/{sprint_plan,verdict,sprint_post}.md`
- CHANGELOG.md Unreleased entry
- PR summary comment

## 10. Risks & Mitigations
- **Q8_0 quantize breaks** — HOMEBREW-INSTALLER-QWEN-UPDATE-001-shape. Mitigate: Epic 2 sanity test catches this.
- **Wall-clock overrun** — use `run_in_background: true` (10-min Bash ceiling avoidance).
- **Bench-serve orphan** — standard `pkill` cleanup; port 8102 untouched.
- **Q8_0 aggregate still < 0.90** — expected outcome for the "broader sensitivity" branch; that's a real finding, not a failure.

## 11. Documents Accessed
- `docs/development/mdemg-usage-lora-001-gguf/{sprint_plan,verdict,sprint_post,sha256}.md/.txt` (#150 predecessor)
- `docs/development/mdemg-usage-lora-001/{sprint_plan,verdict,sprint_post}.md` (#145 grandparent)
- `configs/benchmark_phase10.yaml`
- `training_data/eval/valid_clean.jsonl`
- `training_data/eval/mdemg_usage_lora_001_gguf_prod_runtime.json` (#150 Q5_K_M benchmark)
- `training_data/eval/v1_fused_mlxserver_baseline.json` (#145 v1 mlx baseline)
- `.local-models/qwen3-14b-mdemg-usage-fused.f16.gguf` (Q8_0 source)
- `/opt/homebrew/bin/{llama-quantize,llama-server}`
- `neural/.venv/bin/python -m neural.benchmarks.run_benchmark`
- CLAUDE.md pins (as listed in §3)
- Operator directive 2026-09-01 ("proceed with #151")

## 12. Rollback Procedures
```bash
rm -f .local-models/qwen3-14b-mdemg-usage-fused.Q8_0.gguf  # ~16 GB
pkill -f "llama-server.*8103" 2>/dev/null || true
# Production 8102 untouched — no restore needed
```
Zero substrate mutation. Zero prod touch.
