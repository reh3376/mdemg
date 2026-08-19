# PHASE-E2-CORPUS-CURATION-001 — Sprint Plan

**Master arc**: JIMINY-SUBSTRATE-NATIVE-001 — Phase E2 (LoRA reframe: corpus strip execution)

## 1. Header & Metadata

- **Sprint ID**: `PHASE-E2-CORPUS-CURATION-001`
- **Arc**: JIMINY-SUBSTRATE-NATIVE-001 (Phase E2)
- **Author**: reh3376 / claude
- **Date**: 2026-08-19
- **Branch**: `reh3376_dev01`
- **Estimated wall-clock**: ~2 hours
- **Sprint format**: v1.0 (12-section)

## 2. Problem Statement

Phase E1 (PHASE-E1-CORPUS-AUDIT-001, 2026-08-19) audited `claude_code_knowledge_v2/train.jsonl` (2,706 rows) against the mdemg-dev substrate post-CLAUDE-DOCS-INGEST-001. Verdict: **2,203 rows (81.4%) PROVEN_COVERAGE** (their fact is retrievable at production surface) with an extremely bimodal distribution (79.9% at ≥0.9 overlap, 16.7% at <0.1). Zero AUDIT_ERROR. Hand-verified 3 miss rows for correctness.

This sprint executes the strip: produces a new versioned SFT dataset `claude_code_knowledge_v3_stripped/` containing the 503 SUBSTRATE_MISS rows. Preserves v1 + v2 in place for rollback. E3 will retrain on the shrunken corpus; E4 will gate + promote if benchmark passes.

## 3. Scope & Constraints

### In scope
1. **Strip script** `scripts/phase_e2_strip_covered_rows.py` — reads v2 `train.jsonl` + E1's `rows_to_strip.jsonl`, emits `training_data/sft/claude_code_knowledge_v3_stripped/train.jsonl` (503 rows). Deterministic; sorted by original row_idx; preserves original `messages` + `meta` untouched.
2. **New manifest** `claude_code_knowledge_v3_stripped/manifest.json` — mirrors v2 manifest shape (row_counts, per_task_counts, sha256, base_dataset_ver, sprint provenance, trained_against_model_sha placeholder for E3).
3. **Leak audit** — reuses shipped `scripts/audit_eval_leakage.py`; audits the new stripped train.jsonl against `training_data/eval/valid_clean.jsonl` (per CLAUDE.md honest eval); exit 0 required (no leakage into the honest eval).
4. **No mutation of v1 or v2 files** — preserved in place for rollback + comparison.
5. **No new valid.jsonl** in v3_stripped — E3 benchmark can still compare against v2's held-out `valid.jsonl` and against `valid_clean.jsonl`.
6. **No delete of v1 corpus dir** — it's superseded but staying in place means rollback is one manifest edit away.

### Out of scope
- **E3 retrain** — this sprint produces the corpus artifact; retrain is E3's job.
- **E4 gate + promote** — depends on E3.
- **Optional E1a re-ingest** of "part N of M" fragments — would push some MISS→COVERAGE; separate follow-up.
- **Behavior corpora audit** (tier1, family_*) — those are TASK-behavior training, not fact-recall; preserve unchanged.

### Hard invariants
- **v2 files unchanged** — audit-verified pre + post via `sha256sum`.
- **Preserved row provenance** — v3 stripped train.jsonl `messages` + `meta` byte-identical to their v2 originals (no re-normalization, no re-tokenization).
- **Deterministic + reproducible** — same v2 + same E1 strip-list produces byte-identical v3 output. SHA pins on all input + output files.
- **Leak-audit gate** — must-pass; if any v3 row appears in valid_clean.jsonl, sprint aborts with a hard fail before writing manifest.
- **Read-only against substrate** — this sprint does not touch Neo4j / TSDB / mdemg server.

## 4. Dependencies

**Upstream (must be shipped)**:
- ✅ PHASE-E1-CORPUS-AUDIT-001 (2026-08-19) — produces `rows_to_strip.jsonl` (2,203 row_indexes)
- ✅ `training_data/sft/claude_code_knowledge_v2/train.jsonl` (2,706 rows) present
- ✅ `scripts/audit_eval_leakage.py` — shipped leak-audit tool
- ✅ `training_data/eval/valid_clean.jsonl` — the honest eval

**Downstream (this sprint unblocks)**:
- PHASE-E3-RETRAIN-BENCHMARK-001 (LoRA retrain on the shrunken corpus + benchmark vs current 0.9188 baseline)
- PHASE-E4-GATE-PROMOTE-001

## 5. Implementation Plan

### Epic 1: strip script (~30min)
- New file `scripts/phase_e2_strip_covered_rows.py`
- Args: `--source-corpus`, `--strip-list`, `--out-dir`, `--dry-run`
- Reads source JSONL row-by-row with `row_idx = enumerate`; reads strip-list into a `set[int]`
- If `row_idx in strip_set`, drop; else keep — sorted by row_idx
- Emit to `<out-dir>/train.jsonl` (preserve messages + meta verbatim, no reformatting)
- Emit manifest with source SHA + output SHA + strip count + preserved count
- Print summary

### Epic 2: manifest generator (~15min)
- Mirror v2 manifest shape:
  - `sprint` = "PHASE-E2-CORPUS-CURATION-001"
  - `family_name` = "claude_code_knowledge_v3_stripped"
  - `base_dataset_ver` = "claude_docs_v2_stripped_via_e1_audit"
  - `row_counts` = {train: 503, total: 503}
  - `file_sha256` = {train.jsonl: <sha256>}
  - `per_task_counts` = {"claude.code_knowledge": {total: 503, train: 503}}
  - `source_v2_row_count` = 2706
  - `stripped_row_count` = 2203
  - `strip_provenance` = {"sprint_id": "PHASE-E1-CORPUS-AUDIT-001", "strip_list_sha256": <sha256>, "audit_date": "2026-08-19", "audit_threshold": 0.30}
  - `preserves` = "claude_code_knowledge_v1 (2141 rows, superseded), claude_code_knowledge_v2 (2848 rows, source)"

### Epic 3: leak audit (~15min)
- Run `python scripts/audit_eval_leakage.py --eval training_data/eval/valid_clean.jsonl --against training_data/sft/claude_code_knowledge_v3_stripped/train.jsonl --out docs/development/phase-e2-corpus-curation-001/leak_audit.json`
- Assert exit code 0 (no leakage into the honest eval)
- If leakage detected, fail the sprint; investigate before shipping

### Epic 4: verify + docs (~30min)
- SHA256-verify v1 + v2 files unchanged post-strip (compare before/after)
- Row count sanity: input 2706 minus 2203 stripped = 503 preserved
- Sprint post `docs/development/phase-e2-corpus-curation-001/sprint_post.md` with all numbers
- CLAUDE.md architecture note
- CHANGELOG entry

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit
- N/A (one-shot script; the mutation IS the deliverable)

### Tier 2 — Integration
- Strip script executes without error
- Output JSONL is valid (each line parses as JSON)
- Row count = 503 exactly
- Every kept row's row_idx is in the SUBSTRATE_MISS or AUDIT_ERROR classification from E1

### Tier 3 — Live end-to-end
- **Live is N/A** for this sprint (no server changes, no substrate mutation). Live-testing tier applies to E3 retrain + E4 promote.
- The E2 sprint deliverable IS the file bundle; validation is SHA + row-count + leak-audit — all shipped, all checked.

## 7. Commit Strategy

- 1 primary commit: strip script + v3 bundle + docs.
- No fix-commit expected.

## 8. Verification Checklist

- [ ] `scripts/phase_e2_strip_covered_rows.py` runs clean
- [ ] Output `claude_code_knowledge_v3_stripped/train.jsonl` has exactly 503 rows
- [ ] Every kept row's row_idx is in E1's `rows_to_keep.jsonl` (or `audit_rows.jsonl` classification `SUBSTRATE_MISS` or `AUDIT_ERROR`)
- [ ] v1 + v2 files SHA256-unchanged
- [ ] Leak audit against `valid_clean.jsonl` exits 0
- [ ] Manifest written with all required fields
- [ ] Sprint plan + post in `docs/development/phase-e2-corpus-curation-001/`
- [ ] CLAUDE.md note added
- [ ] CHANGELOG entry
- [ ] PR sprint-summary comment

## 9. Documentation Update

### Files created
- `docs/development/phase-e2-corpus-curation-001/{sprint_plan,sprint_post}.md`
- `docs/development/phase-e2-corpus-curation-001/leak_audit.json`
- `scripts/phase_e2_strip_covered_rows.py`
- `training_data/sft/claude_code_knowledge_v3_stripped/train.jsonl` (503 rows)
- `training_data/sft/claude_code_knowledge_v3_stripped/manifest.json`

### Files modified
- `CLAUDE.md`, `CHANGELOG.md`
- **NOT modified**: v1 + v2 corpus files (preserved)

## 10. Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Strip-list row_idx off-by-one vs corpus enumeration | Low | High (wrong rows stripped) | Explicit enumerate in strip script; sanity check: every KEPT row_idx must be in E1's `rows_to_keep.jsonl` |
| Leak into `valid_clean.jsonl` | Low | High (would poison E3 benchmark) | Leak audit gate is a hard fail; investigate before manifest write |
| v2 file mutated accidentally | Very Low | High (loses rollback) | SHA256 verify v2 files before + after; script only WRITES the new bundle |
| v3 bundle is too small (503 rows) to be useful for LoRA training | Medium | Medium | v3 alone isn't the training corpus — it JOINS tier1 (3,500) + family_* (3,500) = ~7,503 rows total. E3 will train on the union, not just v3. |
| E3 discovers substrate coverage isn't enough for real inference | Medium | Medium | E3 benchmark against `valid_clean.jsonl` is the gate; if regression, revert to v2 |

## 11. Rollback Procedures

- **v1 + v2 preserved** — rollback is: delete `claude_code_knowledge_v3_stripped/`, retrain against original v2.
- **No substrate mutation** — nothing to unwind on Neo4j/TSDB.
- **No `.env` changes** — nothing to restore in runtime config.
- Code rollback: `git revert` this sprint's commit.

## 12. Documents Accessed

- `training_data/sft/claude_code_knowledge_v2/{train.jsonl,valid.jsonl,manifest.json}` (audit source; manifest shape reference)
- `training_data/sft/claude_code_knowledge/{train.jsonl,manifest.json}` (v1, superseded, preserved)
- `training_data/sft/{tier1,family_classify_notink,family_reasoning_think,family_structured_notink}/` (behavior corpora — not audited, not modified)
- `training_data/eval/valid_clean.jsonl` (leak-audit target)
- `docs/development/phase-e1-corpus-audit-001/{rows_to_strip,rows_to_keep,audit_rows}.jsonl` (E1 inputs)
- `scripts/audit_eval_leakage.py` (shipped leak-audit tool, reused)
- `scripts/phase_e1_corpus_audit.py` (shape reference)
- CLAUDE.md pins (PHASE-E1-CORPUS-AUDIT-001, INGEST-TOPOLOGY-REPAIR-001, CLAUDE-DOCS-INGEST-001, JIMINY-SUBSTRATE-NATIVE-001 arc)

---
