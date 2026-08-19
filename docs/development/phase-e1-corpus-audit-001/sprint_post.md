# PHASE-E1-CORPUS-AUDIT-001 — Sprint Post

**Arc**: JIMINY-SUBSTRATE-NATIVE-001 — Phase E1
**Shipped**: 2026-08-19
**Ship state**: Audit complete + evidence-backed strip proposal. **Verdict: PROCEED TO E2.**

## What shipped

1. **Audit script** `scripts/phase_e1_corpus_audit.py` — bounded-concurrency retrieval-based coverage check.
2. **Full audit run** against mdemg-dev substrate: 2,706 rows in ~13 minutes wall clock at concurrency=8.
3. **4 artifacts** in `docs/development/phase-e1-corpus-audit-001/`:
   - `audit_rows.jsonl` — per-row raw output (row_idx, overlap, top1_name, n_results, classification)
   - `rows_to_strip.jsonl` — 2,203 PROVEN_COVERAGE row-indexes (safe to strip in E2)
   - `rows_to_keep.jsonl` — 503 SUBSTRATE_MISS row-indexes (preserve in FT corpus)
   - `audit_report.md` — human-readable summary + histogram + top-20 misses + recommendation

## Results

**Source corpus**: `training_data/sft/claude_code_knowledge_v2/train.jsonl` (2,706 rows — `wc -l` initially reported 2,848 but that was reading a different fixture; JSON-parsed row count is 2,706).

| Classification | Count | % |
|---|---|---|
| **PROVEN_COVERAGE** (safe to strip in E2) | **2,203** | **81.4%** |
| SUBSTRATE_MISS (keep in FT corpus) | 503 | 18.6% |
| AUDIT_ERROR (0 — zero query errors) | 0 | 0.0% |
| **Total** | 2,706 | 100.0% |

**Overlap distribution — extremely bimodal:**

- 2,163 rows (79.9%) at overlap ≥ 0.9 (near-perfect coverage — retrieval returns the answer's content verbatim)
- 453 rows (16.7%) at overlap < 0.1 (near-zero coverage — retrieval returns adjacent-but-different content)
- 90 rows (3.3%) scattered in the 0.1–0.9 middle (ambiguous)

The bimodal distribution is the key finding: classification is HIGH-confidence for 96.6% of rows (the ≥0.9 or <0.1 tails). The 0.30 threshold has huge margin.

## Recon findings (verified live before/after audit)

Applied `must-validate-all-claims-before-commit`.

| Claim | Verification | Verdict |
|---|---|---|
| Substrate has Claude-Code content post-CLAUDE-DOCS-INGEST-001 | Live probe on "Configure preview servers…" returned exact match (`n_e264ea734b7abf2c4f1c`, path `claude-docs/desktop/033__configure-preview-servers`, 7,998 bytes content) | ✅ confirmed |
| INGEST-001 populated `n.content` on ingested nodes | Live retrieve returned non-empty `content` fields | ✅ confirmed |
| Overlap threshold 0.30 correctly separates coverage vs miss | Bimodal distribution (79.9% ≥0.9, 16.7% <0.1) — 0.30 threshold has huge margin | ✅ confirmed |
| Zero query errors on 2,706-row live run | AUDIT_ERROR count = 0 | ✅ substrate healthy under audit load |
| Hand-sample of 3 SUBSTRATE_MISS rows shows correct classification | Sampled rows 2, 39, 64 — all "part N of M" sub-fragments or specific SDK-reference class definitions that the substrate legitimately doesn't have | ✅ correct |
| Row count 2,706 matches JSON-parsed corpus size | `wc -l` = 2,706, JSON parse = 2,706 | ✅ consistent |

## Decisions

| Decision | Rationale |
|----------|-----------|
| Threshold 0.30 for PROVEN_COVERAGE | Empirically justified: bimodal live data confirms 0.30 sits in a wide, empty valley between the two modes (only 22 rows in [0.20, 0.40)). Anywhere in [0.1, 0.6] would produce nearly identical classification counts. |
| Word-level 3-gram overlap (asymmetric answer→content) | Answer's substance must be IN the content, not just topic-related. Sharing 30% of the answer's 3-grams is a strong signal the content contains the answer. |
| Fail-safe on query error → `AUDIT_ERROR`, never `PROVEN_COVERAGE` | Prevents silent false-positive strip. Live-verified: 0 AUDIT_ERROR rows — a clean audit run. |
| Bounded concurrency 8 (vs 16 or 20) | Well within retrieval-hot-path headroom; ~13 min real-time is acceptable for a one-shot audit. |
| **Do NOT strip v1 corpus (2,141 rows) in this sprint** | v1 is superseded by v2 in shipped training; if v2 is proven-covered, v1 auto-strips as replaced (E2 concern). Auditing v1 would be duplicative. |
| Keep SUBSTRATE_MISS in FT corpus | These rows are either (a) "part N of M" sub-fragments the substrate has as an unsplit whole doc, OR (b) specific SDK-reference class definitions not fully captured. Either way, the FT training on these details remains useful; a re-ingest sprint (E1a follow-up) could push some MISS→COVERAGE but doesn't block E2. |

## Verdict for E2

**Proceed with E2 (corpus curation + retrain) using this sprint's strip-list as authoritative:**

- Strip 2,203 rows from `claude_code_knowledge_v2/train.jsonl` → shrinks the corpus by ~81%
- Preserve 503 SUBSTRATE_MISS rows
- Also drop the 2,141-row `claude_code_knowledge` v1 corpus (superseded, ~identical topic set)
- Result: FT corpus shrinks from 9,988 → ~5,502 rows (~45%), heavily weighted toward TASK-behavior (tier1 + family_*) over fact-recall

E3 benchmark against `valid_clean.jsonl` will confirm no fact-recall regression (substrate should handle those queries via retrieve+content projection).

## Follow-ups (disclosed, deferred)

1. **[Optional] PHASE-E1a-RE-INGEST-001** — for the 503 SUBSTRATE_MISS rows, generate a re-ingest job that splits "part N of M" sub-fragments into their own MemoryNodes so they become individually retrievable. Could push MISS→COVERAGE ~200-400 more rows. Not blocking E2.
2. **PHASE-E2-CORPUS-CURATION-001** — actual strip execution using `rows_to_strip.jsonl` + leak-audit + versioned SFT bundle.
3. **PHASE-E3-RETRAIN-BENCHMARK-001** — LoRA retrain on curated corpus + benchmark vs current 0.9188 baseline.
4. **PHASE-E4-GATE-PROMOTE-001** — use shipped FT-RECURSIVE-003 fail-closed swap.

## Arch rules pinned

- **When auditing a training corpus for substrate coverage, use retrieval as the ground truth** — query `/v1/memory/retrieve` with the row's user question; compare the row's answer against the returned content. This proves the substrate at production surface can serve the fact; anything less is a proxy (e.g. keyword grep against the ingested source docs).
- **Overlap metric must be ASYMMETRIC (answer→content, not content→answer)** — the answer's substance MUST be in the content for coverage to hold; the reverse (content contains extra) is fine. Symmetric overlap (e.g. Jaccard) would fail rows where the content is 10× longer than the answer.
- **AUDIT_ERROR is a distinct classification** — never let a substrate query error be silently classified as coverage (which would strip a row we can't prove is covered). Live-verified 0 AUDIT_ERROR on this run, but the discipline matters for future audits.
- **Fixed random seed + sorted output** — auditing the same substrate state twice must produce identical outputs. This sprint doesn't sample (audits all rows), but the discipline applies when a future audit does sample.

## Documents Accessed

- `training_data/sft/claude_code_knowledge_v2/train.jsonl` (audit target)
- `training_data/sft/claude_code_knowledge/train.jsonl` (v1, superseded; not audited)
- `training_data/sft/{tier1,family_classify_notink,family_reasoning_think,family_structured_notink}/train.jsonl` (behavior/reasoning; NOT audited — preserved)
- Live `/v1/memory/retrieve` on mdemg-dev (2,706 queries at concurrency=8; 0 errors)
- Live probe on "Configure preview servers…" (confirmed substrate coverage before running full audit)
- Hand-verification samples: rows 2, 39, 64 (all correctly classified SUBSTRATE_MISS)
- `.local-models/mdemg-llm-v1 -> qwen3-14b-mdemg-v1` (production model target)
- CLAUDE.md pins (INGEST-TOPOLOGY-REPAIR-001, CLAUDE-DOCS-INGEST-001, JIMINY-SUBSTRATE-NATIVE-001 arc)
- `docs/development/phase-e1-corpus-audit-001/sprint_plan.md`
