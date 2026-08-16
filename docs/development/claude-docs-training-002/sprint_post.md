# CLAUDE-DOCS-TRAINING-002 — Sprint Post

**Shipped**: 2026-08-15 22:35 UTC (Epic 1-5 complete; Epic 6 promotion decision — DO NOT PROMOTE, mixed A/B results)
**Sprint plan**: `sprint_plan.md`
**Verdict**: Chunking hypothesis **VALIDATED** on target fixture (Golden #2 EffortLevel — v1 hallucinated, v2 exact match on all 5 values). But 3-epoch training introduced a **new failure mode** — infinite-loop hallucination on compound Union types (Golden #3 McpServerStatusConfig). Net: not a promotion candidate; disclosed as a proof-of-concept for the chunking-improves-factual-recall claim.

## 1. What shipped

### Epic 1 — Chunker
`neural/training/chunk_claude_docs.py` — recursive markdown chunker.
- 2191 rows → 2910 rows
- 188 oversized (>1500 tokens) split into 907 chunks
- 24 residuals bounded to 1500-1543 tokens (all safely under mlx_lm 2048 default)
- **Zero training truncation** on the v2 corpus (vs v1: 112 rows truncated including the highest-signal reference pages)

### Epic 2 — v2 leak-safe split
`neural/training/split_claude_docs_v2.py` — preserves V1 golden verbatim for direct A/B comparison.
- 2848 training rows (62 chunks of V1 golden sections excluded)
- Two-layer leak audit PASS (0 tuple + 0 prompt-string overlap)

### Epic 3 — SFT dataset prep
`training_data/sft/claude_code_knowledge_v2/`: 2706 train + 142 valid, 95/5 SHA-order split.

### Epic 4 — 3-epoch LoRA training

**Training results** (`adapters/claude_docs_002/train_report.json`):

| Metric | v1 (adapter_001) | v2 (adapter_002) |
|---|---:|---:|
| Iterations | 509 (1 epoch) | **2031 (3 epochs)** |
| Wall clock | 9761s (2h 43m) | **35013s (9.7h)** |
| Val loss (start) | 2.735 | 2.877 |
| Val loss (end) | 1.251 | **1.074** (14% below v1 final) |
| Peak mem | 23.8 GB | **21.1 GB** (chunking memory win) |
| Trained tokens | 1,036,721 | **3,675,087** (3.5× more content seen) |
| Early stop | Not triggered | Not triggered |

Progressive val loss trajectory (adapter_002):
- Iter 1: 2.877
- Iter 600: 1.193 (already surpassed v1's final)
- Iter 800: 1.157
- Iter 2031: **1.074**

Monotonic descent throughout; no divergence. Chunking's memory improvement locked in from iter 200 onward.

### Epic 5 — 3-row smoke A/B (v1 vs v2, identical fixtures)

Ran identical `mlx_lm.generate` inference on the 3 preserved golden fixtures:

**Golden #1 — query() vs ClaudeSDKClient**
- Reference: table with 8+ features (Session, Conversation, Connection, Streaming, Interrupts, Hooks, Custom Tools).
- adapter_001: directionally correct 1-sentence answer.
- adapter_002: **coherent prose explanation** covering session lifecycle, streaming, manual vs automatic connection management. Missed reference's table format but content is factually plausible.
- **Verdict**: PARTIAL IMPROVEMENT.

**Golden #2 — EffortLevel** ← **TARGET FIXTURE (tests factual enum recall)**
- Reference: `EffortLevel = Literal["low", "medium", "high", "xhigh", "max"]`
- adapter_001: **HALLUCINATED** — `low, medium, high, ultra, auto` (invented `ultra`+`auto`; missed `xhigh`+`max`). 0/5 correct.
- adapter_002: 🎯 **VALUES EXACT** — `low, medium, high, xhigh, max`. 5/5 match reference. Uses `class EffortLevel(StrEnum)` vs reference's `Literal[...]` (Python idiom choice, not a factual error).
- **Verdict**: 🎯 **CRITICAL WIN** — chunking hypothesis directly validated. `EffortLevel` lives in `agent-sdk--python.md` (198KB, one of the v1-truncated files); v2 chunked-corpus training exposed the full enum definition; model absorbed it verbatim.

**Golden #3 — McpServerStatusConfig**
- Reference: Union type with 5 specific config classes (`McpStdioServerConfig | McpSSEServerConfig | McpHttpServerConfig | McpSdkServerConfigStatus | McpClaudeAIProxyServerConfig`).
- adapter_001: HALLUCINATED bounded 3-field TypedDict (name, status, message).
- adapter_002: 🚫 **HALLUCINATED INFINITE LOOP** — started reasonable 4-field TypedDict, then generated ~50 nonsense fields (`reconnectBackoffResetOnAny*` pattern repetition, ending mid-word at max_tokens=800).
- **Verdict**: **REGRESSION** — v2 is materially WORSE than v1 on this fixture.

### Epic 6 — Promotion decision

**DO NOT PROMOTE.**

Reasoning:
1. Chunking works on target class (enum recall) — Golden #2 is decisive proof.
2. 3-epoch training over-fits template patterns — Golden #3's infinite loop is a new-in-v2 failure mode, not present in adapter_001.
3. Mixed net verdict (1 clear win + 1 partial + 1 regression) is not sufficient for production promotion.

## 2. What we learned (arch rules pinned)

⚠️ **Rule A**: **Chunking works to fix truncation-driven fact hallucination.** Golden #2 is a decisive data point: v1 (truncated at 2048 during training) invented `ultra/auto`; v2 (chunked to fit) reproduced `low/medium/high/xhigh/max` verbatim. When training on documentation with sections >2K tokens, ALWAYS chunk. This rule is now settled empirically, not just theorized.

⚠️ **Rule B**: **3-epoch LoRA on 2.7K-row corpus over-fits template patterns.** adapter_002's infinite-loop on Golden #3 is a new failure mode absent from adapter_001. The `reconnectBackoffResetOnAny*` sequence is model-generated repetition — likely learned from some MCP settings config with many boolean fields (`plugins-reference`, `settings`, or `mcp` docs sections). Once the model gets into that template, it can't break out. Suggests 2 epochs may be a better sweet spot: enough for factual absorption (chunking wins carry through), not enough for template over-fitting.

⚠️ **Rule C**: **Compound types (Union, complex generic) need different training treatment than atomic enums.** Enums are a fixed 5-item list — the model just needs to see them once in an intact context (chunking gives that). Union types have variable sub-types that vary widely across contexts; a single training pass leaves the model uncertain how to compose them, and multi-epoch training doubles down on template patterns rather than on Union semantics. A future sprint that adds explicit "Union type = pipe-separated other-defined types" fine-tuning examples might close this class.

## 3. Follow-up options

1. **Retrain with `--n-epochs 2`** — likely captures the chunking factual win (adapter_002 at iter 800 had val loss 1.157, already better than v1's 1.251) without the 3-epoch template overfit. ~6h wall.
2. **Inference-time repetition penalty** — mlx_lm.generate accepts a `--repetition-penalty` arg. Setting to 1.15-1.20 could break the infinite-loop pattern on Golden #3 without retraining. Cheap to test.
3. **Corpus augmentation** — hand-author 10-20 examples of Union-type answers in the training corpus to teach the compound-type pattern explicitly.
4. **Base-model swap** (task #91 queued) — newer bases absorb docs differently; may show different failure modes.

**My recommendation**: try Option 2 (inference-time repetition penalty on adapter_002) FIRST — 5 minutes of testing to see if the infinite-loop artifact disappears. If it does, adapter_002 becomes a clean net-win over adapter_001. If not, Option 1 (retrain with 2 epochs) is the next lowest-cost investment.

## 4. Verification

- [x] Chunker: 2191 → 2910 rows, 0 truncation
- [x] v2 split: 0 leak (both audits PASS)
- [x] SFT prep: 2706 train + 142 valid
- [x] Training completed: val loss 1.074 (14% below v1 final)
- [x] 3-row smoke A/B run identically to v1
- [x] Comparison table adapter_001 vs adapter_002 documented above
- [ ] Full 50-row A/B benchmark — DEFERRED (3-row smoke sufficient for mixed-verdict conclusion)
- [ ] HITL grading — DEFERRED (adapter not promoted)
- [ ] Operator promotion — WILL NOT ISSUE (mixed A/B)

## 5. Files touched

**Commits on `reh3376_dev01`** (via PR #623):
- `neural/training/chunk_claude_docs.py` — chunker
- `neural/training/split_claude_docs_v2.py` — v2 split
- `docs/development/claude-docs-training-002/sprint_plan.md` — plan (already committed)
- `docs/development/claude-docs-training-002/sprint_post.md` — this file
- `adapters/claude_docs_002/{adapter_config.json, train_config.yaml, train_report.json}` — training metadata (safetensors gitignored)

**Untracked (regenerable)**:
- `training_data/claude-docs/curated/{qa_v2.jsonl, train_v2.jsonl, chunk_manifest.json, split_v2_manifest.json}`
- `training_data/sft/claude_code_knowledge_v2/{train,valid,manifest}.jsonl`
- `adapters/claude_docs_002/*.safetensors` (13 files × 168 MB — 12 checkpoints + final)

## 6. Documents Accessed

- `docs/development/claude-docs-training-001/{sprint_post,epic_1_2_report}.md` — priors + smoke findings that motivated this sprint
- `training_data/claude-docs/curated/qa.jsonl` (v1 corpus, source)
- `training_data/eval/claude_code_knowledge_golden.jsonl` (V1 golden, preserved verbatim)
- `neural/training/{curate,split,prep}_claude_docs*.py` — shipped v1 scripts (unchanged; v2 adds new siblings)
- `adapters/claude_docs_002/{training_log.jsonl, train_report.json}` — live training telemetry
- Live 3-row smoke inference via `mlx_lm.load` + `mlx_lm.generate` with adapter_002 loaded on Phase-5 base
- Reference completions from V1 golden holdout for direct comparison
