# PHASE-E1-CORPUS-AUDIT-001 — Sprint Plan

**Master arc**: JIMINY-SUBSTRATE-NATIVE-001 — Phase E1 (LoRA reframe: corpus audit)

## 1. Header & Metadata

- **Sprint ID**: `PHASE-E1-CORPUS-AUDIT-001`
- **Arc**: JIMINY-SUBSTRATE-NATIVE-001 (Phase E1)
- **Author**: reh3376 / claude
- **Date**: 2026-08-19
- **Branch**: `reh3376_dev01`
- **Estimated wall-clock**: ~4 hours (fully automated after script writes; audit query time dominant)
- **Sprint format**: v1.0 (12-section)

## 2. Problem Statement

The current mdemg-llm-v1 LoRA training corpus is 9,988 rows across 4 families. **~4,989 rows (50%)** are fact-recall for Claude Code documentation:
- `claude_code_knowledge_v2/train.jsonl` (2,848 rows) — Claude Code docs from `code.claude.com` (Anthropic's agentic CLI + Agent SDK reference)
- `claude_code_knowledge/train.jsonl` (2,141 rows) — v1, superseded

The Phase E arc thesis is: substrate now handles facts. Two shipped preconditions:
1. **INGEST-TOPOLOGY-REPAIR-001** (2026-08-18): `n.content` field on MemoryNode + retrieval projects verbatim content via `include_content=true` on `/v1/memory/retrieve`.
2. **CLAUDE-DOCS-INGEST-001** (task #124, 2026-08-14): the 2,141-row Claude Code corpus ingested into mdemg-dev substrate.

Live verification: query `/v1/memory/retrieve` on the exact prompt "Configure preview servers in Claude Code Desktop application" returns the exact matching node (`n_e264ea734b7abf2c4f1c`, path `claude-docs/desktop/033__configure-preview-servers`, 7,998 bytes of content). Substrate coverage exists.

**Open question this sprint answers**: how many of the 2,848 v2-corpus rows have their fact retrievable from the substrate today? If ≥80%, stripping those rows from the LoRA training corpus is evidence-backed for E2; if <50%, either the ingest was incomplete (needs re-ingest) or the queries are ill-formed (audit issue) — flagged for follow-up before E2.

## 3. Scope & Constraints

### In scope
1. **Audit script** `scripts/phase_e1_corpus_audit.py` that:
   - Reads `training_data/sft/claude_code_knowledge_v2/train.jsonl`
   - For each row: extracts the user question (final user turn in `messages`), queries `POST /v1/memory/retrieve?space_id=mdemg-dev` with `include_content=true, top_k=5`, and computes an overlap score between the assistant's answer and the retrieved content
   - Classifies each row: **PROVEN_COVERAGE** (overlap≥threshold), **SUBSTRATE_MISS** (0 results OR overlap<threshold), **AMBIGUOUS** (marginal overlap)
   - Emits `docs/development/phase-e1-corpus-audit-001/audit_report.md` with row-level classifications + summary counts
   - Emits `docs/development/phase-e1-corpus-audit-001/rows_to_strip.jsonl` (line-index + reason for each row to strip)
   - Emits `docs/development/phase-e1-corpus-audit-001/rows_to_keep.jsonl` (line-index + reason for each row to preserve)
2. **Bounded concurrency** (5-10 concurrent requests) to keep audit wall-clock ≤10 minutes.
3. **Overlap metric**: word-level 3-gram overlap between the assistant's answer (normalized: lowercase, strip whitespace, alphanumeric) and the concatenated retrieved content. Threshold ≥0.30 = PROVEN_COVERAGE (empirically justified: the assistant's answer is typically ~50 words; 30% overlap = ~15 shared 3-grams = strong signal the retrieved content contains the answer facts).
4. **Substrate query resilience**: retry on 5xx (fail-fast on 4xx); log per-row failures separately; NEVER classify a row as PROVEN_COVERAGE on a query error.
5. **Deterministic output**: fixed seed for any sampling; sorted output by line-index.
6. **NO code changes to Go / no schema changes / no server restart**. Pure external audit against live substrate.

### Out of scope
- **v1 corpus audit** (`claude_code_knowledge/train.jsonl`, 2,141 rows) — superseded by v2; if v2 audit succeeds, v1 auto-strips as "already replaced." Documented but not audited.
- **Re-ingest of substrate-miss rows** — E1 identifies gaps; re-ingest is E2 concern.
- **Actual FT corpus mutation** — this sprint produces the strip-list; E2 executes the strip + retrain.
- **Benchmark against valid_clean.jsonl** — deferred to E3.
- **Behavior/reasoning corpus audit** (`tier1`, `family_*`) — those are TASK-behavior training, not fact-recall; keep by design.

### Hard invariants
- **Read-only against substrate** — no `/v1/memory/observe` or `/v1/conversation/*` writes; no Neo4j mutations.
- **No `.env` flip** — audit uses default surfacing (whatever's currently enabled).
- **Reproducibility**: fixed random seed + sorted output; running the script twice against the same substrate state produces identical outputs (modulo LLM-influenced re-ranking non-determinism, which does NOT touch retrieval).
- **Fail-safe classification**: query error → row classified `AUDIT_ERROR` (never PROVEN_COVERAGE). Manual review required before stripping AUDIT_ERROR rows.

## 4. Dependencies

**Upstream (must be shipped)**:
- ✅ INGEST-TOPOLOGY-REPAIR-001 (n.content projection + retrieve include_content=true)
- ✅ CLAUDE-DOCS-INGEST-001 (Claude Code docs ingested to mdemg-dev)
- ✅ `POST /v1/memory/retrieve` accepts `include_content=true` flag (INGEST-001)
- ✅ Server running on http://localhost:9999
- ✅ v2 corpus present at `training_data/sft/claude_code_knowledge_v2/train.jsonl`

**Downstream (this sprint unblocks)**:
- Phase E2 (corpus curation + retrain) — needs this sprint's strip-list
- Phase E3 (benchmark) — depends on E2

## 5. Implementation Plan

### Epic 1: audit script (~2h)
- New file `scripts/phase_e1_corpus_audit.py`
- Args: `--corpus <path>`, `--space-id mdemg-dev`, `--concurrency 5`, `--threshold 0.30`, `--out-dir docs/development/phase-e1-corpus-audit-001/`, `--sample N` (0=all, else stratified sample by path-prefix in row.meta)
- Reads JSONL row-by-row; for each row extracts:
  - `row_idx` (line number, 0-indexed)
  - `question` = last `user` message content
  - `answer` = last `assistant` message content
- POST `/v1/memory/retrieve` with `{space_id, query_text: question, top_k: 5, candidate_k: 20, include_content: true}` — retry once on transient 5xx.
- Compute overlap: normalize both answer + retrieved content; 3-gram overlap ratio = `|A ∩ B| / |A|` (asymmetric — how much of the answer is IN the retrieved content).
- Classify: `PROVEN_COVERAGE` (ratio ≥ threshold), `SUBSTRATE_MISS` (ratio < threshold OR 0 results), `AUDIT_ERROR` (query error).
- Write per-row line to `audit_rows.jsonl` (JSONL for future scripting).
- Write summary counts to `audit_report.md`.
- Write `rows_to_strip.jsonl` (PROVEN_COVERAGE) + `rows_to_keep.jsonl` (SUBSTRATE_MISS + AUDIT_ERROR).

### Epic 2: run audit against mdemg-dev (~1-2h wall clock)
- 2,848 rows × ~5s/query with 5 concurrent = ~48 min sequential-equivalent; ~10-15 min real time.
- Save output artifacts to sprint dir.

### Epic 3: classify + report (~30min)
- Read `audit_rows.jsonl` output; produce human-readable `audit_report.md` with:
  - Total rows / PROVEN_COVERAGE count + % / SUBSTRATE_MISS count + % / AUDIT_ERROR count + %
  - Distribution histogram of overlap ratios
  - Top 20 SUBSTRATE_MISS rows (with question + retrieval-empty flag) for operator eyeball
  - Recommendation: proceed to E2 / re-ingest / manual review
- Sanity-check: sample 10 PROVEN_COVERAGE rows + 10 SUBSTRATE_MISS rows, verify manually that classifications are sane.

### Epic 4: docs + commit (~30min)
- Sprint post: `docs/development/phase-e1-corpus-audit-001/sprint_post.md`
- CLAUDE.md architecture note
- CHANGELOG entry
- No feature-doc (this is a research/audit sprint, not a user-facing feature)

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit
- Not applicable (Python audit script, no library-level testing needed for a one-shot script).

### Tier 2 — Integration
- Script executes cleanly against mdemg-dev without errors.
- Output files are valid JSONL / valid markdown.

### Tier 3 — Live end-to-end (mdemg-dev)
- Full 2,848-row audit runs to completion.
- Sample 10 PROVEN_COVERAGE + 10 SUBSTRATE_MISS classifications by hand; verify classification accuracy ≥90%.
- If <90% accurate, tune threshold or classification logic before shipping.

## 7. Commit Strategy

- 1 primary commit: script + audit outputs + docs.
- No fix-commit expected (audit is off-hot-path; won't touch runtime).

## 8. Verification Checklist

- [ ] `scripts/phase_e1_corpus_audit.py` executes without error
- [ ] `audit_rows.jsonl`, `audit_report.md`, `rows_to_strip.jsonl`, `rows_to_keep.jsonl` all written
- [ ] Total row count = 2,848 (matches source)
- [ ] Manual sample: 10 PROVEN + 10 MISS → ≥90% correctly classified
- [ ] Sprint plan + post in `docs/development/phase-e1-corpus-audit-001/`
- [ ] CLAUDE.md note added
- [ ] CHANGELOG entry
- [ ] PR sprint-summary comment posted

## 9. Documentation Update

### Files created
- `docs/development/phase-e1-corpus-audit-001/{sprint_plan,sprint_post,audit_report}.md`
- `docs/development/phase-e1-corpus-audit-001/{audit_rows,rows_to_strip,rows_to_keep}.jsonl`
- `scripts/phase_e1_corpus_audit.py`

### Files modified
- `CLAUDE.md`, `CHANGELOG.md`

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Substrate coverage < 50% → strip-list is small → E2 barely reduces corpus | Medium | Medium | If low, disclosed as E1a "re-ingest gap" follow-up before E2 |
| Overlap threshold miscalibrated (e.g. 0.30 too low → false positives, or too high → false negatives) | Medium | Low | Manual sample-check in Tier-3 verification; tune threshold if <90% accuracy |
| Audit run overwhelms server (5 concurrent × 5s × 2848 rows = many requests) | Low | Low | 5 concurrent is well within server capacity (retrieval is a bounded read); bounded semaphore in the script |
| Substrate returns high overlap even for wrong-answer rows (e.g. neighboring section rank #1) | Medium | Low | Word-level 3-gram overlap on answer→content requires the answer's substance to be in the content, not just topic-related; hand-sample verifies |

## 11. Rollback Procedures

- Zero mutation to substrate or FT corpus. Rollback = delete the sprint dir + script. No state to restore.

## 12. Documents Accessed

- `training_data/sft/claude_code_knowledge_v2/train.jsonl` (2,848 rows, corpus target)
- `training_data/sft/claude_code_knowledge/train.jsonl` (2,141 rows, v1)
- `training_data/sft/{tier1,family_classify_notink,family_reasoning_think,family_structured_notink}/train.jsonl` (behavior/reasoning; NOT audited)
- `.local-models/mdemg-llm-v1 -> qwen3-14b-mdemg-v1` (current production LoRA symlink)
- Live `/v1/memory/retrieve` probe (space_id=mdemg-dev, query "Configure preview servers…") — confirmed substrate coverage on 1 sample
- `docs/development/ft-lora/*.md` (arc context)
- CLAUDE.md pins (INGEST-TOPOLOGY-REPAIR-001, CLAUDE-DOCS-INGEST-001 task #124, JIMINY-SUBSTRATE-NATIVE-001 arc)

---
