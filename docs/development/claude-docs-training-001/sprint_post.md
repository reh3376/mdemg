# CLAUDE-DOCS-TRAINING-001 — Sprint Post

**Shipped**: 2026-08-15 (Epic 1–5 complete; Epic 6 partial — smoke only, full A/B benchmark + promotion decision deferred to operator)
**Sprint plan**: `sprint_plan.md`
**Epic 1–2 report**: `epic_1_2_report.md`
**Verdict**: **DO NOT PROMOTE.** Adapter is a proof-of-concept with real learning signal (val loss cut 54%) but factuality on specific enum values / type definitions is imperfect. Additional training passes needed before this is a promotion candidate.

## 1. What shipped

### Epic 1 — Discovery + robots.txt verdicts (see `epic_1_2_report.md`)

- Domain reframe: `docs.claude.com` → `platform.claude.com` + separate `code.claude.com` for CLI docs
- **`code.claude.com/robots.txt` explicitly invites AI training** (`Content-Signal: ai-train=yes`)
- Two-orders-of-magnitude simplification found: docs serve raw `.md` at URL+`.md`; Anthropic publishes `llms.txt` curated indexes
- Skipped: `platform.claude.com/docs/en/api/*` (robots.txt `Disallow: /api/`)

### Epic 2 — Scrape (`scripts/scrape_claude_docs.py`, `configs/scrape/claude_docs.yaml`)

130 URLs from `code.claude.com/docs/en/*` scraped at 1s rate limit → 100% success, 6.4 MB, 680K words.

### Epic 3 — Curate (`neural/training/curate_claude_docs.py`, Option A: deterministic H2/H3)

2191 Q&A pairs from 130 files. 31% H2 / 69% H3. Word-count healthy (78% in 60-800 range).

### Epic 4 — Leak-safe split + ULTS/UBENCH registration

- `neural/training/split_claude_docs.py`: 50 golden rows stratified across top 20 sources; Q rephrased on golden side
- Two-layer leak audit **PASS**: 0 (source_url, section_index) tuple overlap; 0 golden-prompt string overlap
- `docs/tests/ults/specs/claude_code_knowledge.ults.json` (new): ULTS runner 18/18 PASS
- Golden merged into `valid_clean.jsonl` (240→290 rows, 12→13 tasks); UBENCH SHA bumped `f215→3212`
- Fixed pre-existing [AMD-2] APE-REFLECT-EVAL-REFRESH-001 SHA drift as a side-effect
- 5 pre-existing UBENCH-contract gaps remain (consulting.synthesis, guardrail.evaluate, metalearn.generalize, retrieval.rerank_nli, summarize.generate) — inherited failure, not this sprint

### Epic 5 — LoRA training (`neural/training/prep_claude_docs_sft.py` + `neural/training/train_ft.py`)

**Training completed 2026-08-15 ~02:26 UTC:**

| Metric | Value |
|---|---|
| Iterations | 509/509 (1 epoch) |
| Wall clock | 9760.8s (**2h 43m**) |
| Base model | `mlx-community/Qwen3-14B-4bit` (SHA-pinned `a54ec18f`) |
| LoRA rank / α | 32 / 64 |
| Trainable params | 41.94M (0.284% of 14.77B) |
| Val loss | **2.735 (iter 1) → 1.251 (iter 509) — 54% reduction** |
| Train loss | 2.076 (iter 10) → 1.086 (iter 480 low) → 1.166 (final) |
| Peak mem | 23.8 GB (M5 Max comfortable) |
| Trained tokens | 1,036,721 |
| Early stop | Not triggered |

Adapter at `adapters/claude_docs_001/adapters.safetensors` (168 MB, gitignored per `adapters/*/*.safetensors` rule).

### Epic 6 — Smoke inference only

**HITL grading + full-sweep A/B benchmark + operator promotion decision DEFERRED.**

Ran 3-row smoke via `mlx_lm.generate` with the LoRA adapter loaded on the Phase-5 base:

| Golden | Adapter behavior | Verdict |
|---|---|---|
| #1 query() vs ClaudeSDKClient | Directionally correct; missed reference's table structure | Partial |
| #2 EffortLevel | Hallucinated: invented `Enum` with `low/medium/high/ultra/auto`; reference has `Literal["low","medium","high","xhigh","max"]` | Wrong |
| #3 McpServerStatusConfig | Hallucinated a TypedDict; reference is a Union type | Wrong |

**Honest characterization**: adapter learned **shape** (Python types, Claude Code SDK vocab, code-block formatting) but factuality on specific enum values / type definitions is imperfect. This is expected given:
- Only 1 epoch (memorization typically needs more)
- 2048-token truncation on long reference pages (some sections up to 14501 tokens got clipped to 2048 → lost specific fact content)
- Golden Q rephrased so memorization can't shortcut — model must generalize

## 2. Do not promote — reasons + follow-up options

**No `mdemg ft-loop promote` will be issued for this adapter.** It's a proof-of-concept for the training-path viability, not a production candidate.

Follow-up options the operator can choose from:

1. **Full-sweep A/B benchmark (30-60 min compute)** — would produce a proper `factuality × citation_precision × concision` weighted score vs the Phase-5 baseline on the 50-row golden. Would formalize the smoke findings but not change the "not promotable" verdict.
2. **Retrain with expanded budget** — `--n-epochs 3` (epoch_cap) + `--max-seq-length 4096` to eliminate the truncation. Would ~3x the training time (~8h wall) but should meaningfully improve factuality.
3. **Curation refinement** — the 2191-pair corpus is TOO BROAD. A refined subset dropping the reference-heavy pages (agent-sdk--typescript 284KB, agent-sdk--python 198KB) or splitting them into digestible chunks would fit better under the 2048 token limit.
4. **HITL grading of golden rows** — grade the adapter's 50-row output for a real signal (currently only 3 rows spot-checked). Would use the HITL infrastructure JIMINY-CORPUS-AUDIT-004 shipped.

## 3. What we learned (arch rules pinned)

⚠️ **Rule A**: When training on documentation that includes very long reference pages, `--max-seq-length` truncation loses substantial fact-content. For docs with sections >2048 tokens, either (a) chunk the corpus in curation before training OR (b) raise `--max-seq-length` (with the wall-time cost). The specific truncation warnings in the training log (longest sentence 14,501 tokens) are load-bearing signals — don't ignore them.

⚠️ **Rule B**: 1-epoch LoRA training on a 2K-row corpus produces shape-learning but not fact-memorization. Documentation-recall tasks specifically require multi-epoch training + full sequence coverage. Sprint plan called for `--n-epochs 1` as the safe first pass; the smoke findings confirm that first-pass is genuinely insufficient for production.

⚠️ **Rule C**: The `neural/.venv` Python (with `mlx_lm` installed) is required for training — the system Python at `/opt/homebrew/opt/python@3.14/bin/python3.14` does NOT have `mlx_lm`. Two initial training attempts failed on this — the first import-error was `attempted relative import with no known parent package` (fix: use `-m neural.training.train_ft` module invocation), the second was `No module named 'mlx_lm'` (fix: use `neural/.venv/bin/python`). Every future MDEMG training sprint MUST invoke via `neural/.venv/bin/python -m neural.training.<script>`.

⚠️ **Rule D (data-decided scope)**: `docs.claude.com` was assumed by the sprint plan but has consolidated to `platform.claude.com` (API/agent docs) + `code.claude.com` (CLI docs). Any future scrape targeting Anthropic docs must re-verify domain state — assumptions from >1 quarter old are stale.

## 4. Files touched

**Commits on `reh3376_dev01`** (via PR #622):

- `configs/scrape/claude_docs.yaml` — 130-URL scrape manifest with robots.txt verdicts inline
- `scripts/scrape_claude_docs.py` — idempotent Python scraper
- `neural/training/curate_claude_docs.py` — H2/H3 section extractor
- `neural/training/split_claude_docs.py` — leak-safe train/golden splitter
- `neural/training/prep_claude_docs_sft.py` — SFT dataset dir builder
- `docs/tests/ults/specs/claude_code_knowledge.ults.json` — new ULTS task spec
- `docs/tests/ubench/specs/mdemg.ubench.json` — expected_specs 17→18, expected_rows 240→290, expected_tasks 12→13, SHA `f215→3212`
- `docs/development/claude-docs-training-001/{epic_1_2_report.md,sprint_post.md}` — this file + prior report
- `adapters/claude_docs_001/{adapter_config.json,train_config.yaml,train_report.json}` — training metadata (safetensors gitignored per shipped `adapters/*/*.safetensors` rule)

**Untracked (regenerable)**:
- `training_data/claude-docs/raw/*.md` — 130 files, 6.4 MB (regenerable via `scripts/scrape_claude_docs.py`)
- `training_data/claude-docs/scrape_manifest.json` — regenerable
- `training_data/claude-docs/curated/qa.jsonl` + `train.jsonl` + `split_manifest.json` + `distribution_report.txt`
- `training_data/sft/claude_code_knowledge/{train.jsonl,valid.jsonl,manifest.json}` — regenerable
- `training_data/eval/claude_code_knowledge_golden.jsonl` — regenerable via `split_claude_docs.py`
- `adapters/claude_docs_001/*.safetensors` — 6 files, 168 MB each (retrainable in ~2h 43m)

## 5. Verification

- [x] Robots.txt verdicts recorded for both target domains
- [x] Scrape 100% success rate (Epic 1 gate ≥95% exceeded)
- [x] Curation deterministic + auditable (Option A per operator sign-off)
- [x] Leak-safe split: 0 tuple overlap + 0 prompt-string overlap (both audit gates PASS)
- [x] ULTS runner 18/18 PASS
- [x] UBENCH SHA + row-count + task-count updated
- [x] SFT dataset prep dry-run clean
- [x] LoRA training completed: val loss 54% reduction, no early stop
- [x] 3-row smoke inference against golden holdout
- [ ] **Full-sweep A/B benchmark on the 50-row golden** — DEFERRED
- [ ] **HITL grading of adapter output** — DEFERRED
- [ ] **Operator promotion decision** — WILL NOT ISSUE for this adapter (see §2)

## 6. Documents Accessed

- `plugins/docs-scraper/{manifest.json,fetcher.go,ingestion.go,extractor.go}` — assessed shipped plugin; concluded gRPC + Neo4j observation shape is wrong for our need (want raw markdown to disk)
- `https://code.claude.com/robots.txt` — `Content-Signal: ai-train=yes`
- `https://code.claude.com/docs/llms.txt` — 200+ URLs, source-of-truth for URL enumeration
- `https://code.claude.com/docs/sitemap.xml` — 247 English URLs
- `https://platform.claude.com/robots.txt` — `Disallow: /api/`
- `https://platform.claude.com/sitemap.xml` — 1300+ URLs across 12 languages
- `gh api repos/anthropics/claude-code` — discovered `code.claude.com/docs/en/overview` via repo homepage
- `neural/training/train_ft.py` + `neural/training/tests/*` — LoRA training script + manifest schema
- `training_data/sft/tier1/manifest.json` — FT-LORA-DATA schema reference
- `docs/tests/ults/specs/consulting_classify.ults.json` — ULTS spec pattern reference
- `docs/tests/ubench/specs/mdemg.ubench.json` — UBENCH contract shape
- `docs/tests/ubench/runners/ubench_runner.py` — contract validator
- `docs/tests/ults/runners/ults_runner.py` — ULTS validator
- Live TSDB queries + training log tail during the 2h 43m LoRA run
- Live smoke inference on 3 golden holdout rows via `mlx_lm.load` + `mlx_lm.generate`
