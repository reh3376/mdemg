# PHASE-E2-CORPUS-CURATION-001 — Sprint Post

**Arc**: JIMINY-SUBSTRATE-NATIVE-001 — Phase E2
**Shipped**: 2026-08-19
**Ship state**: v3 stripped corpus written + leak-audit CLEAN + v1 & v2 preserved.

## What shipped

1. **Strip script** `scripts/phase_e2_strip_covered_rows.py` — reads v2 train.jsonl + E1 rows_to_strip.jsonl; emits stripped corpus verbatim (byte-for-byte preserved messages + meta); SHA-verifies source unchanged pre + post; writes manifest.
2. **v3 stripped corpus** at `training_data/sft/claude_code_knowledge_v3_stripped/`:
   - `train.jsonl` — **503 rows** (v2's 2,706 minus 2,203 PROVEN_COVERAGE)
   - `manifest.json` — full provenance: source SHA, strip-list SHA, E1 audit ref, base model name
3. **Leak audit** — reused shipped `scripts/audit_eval_leakage.py`; **CLEAN 0/290** overlap across all 13 eval tasks against `valid_clean.jsonl` (the honest eval per CLAUDE.md).
4. **v1 + v2 preserved unchanged** — verified via SHA256 before/after (v2 `e4ab7b4af2f9eafcfd6311974955119d1955efc2c7b45ba763ef0f2bf5443451` identical).

## Numbers

| Field | Value |
|---|---|
| v2 source rows | 2,706 |
| Stripped (PROVEN_COVERAGE from E1) | **2,203** |
| Kept (SUBSTRATE_MISS from E1) | **503** |
| Output SHA256 | `c4412be7a4cc4301c89ec91a73983a8c45cb792a66a65586f83c260dd8031527` |
| Leak audit (v3 vs valid_clean) | **0/290 (CLEAN)** |
| v1 corpus | preserved (2,141 rows, superseded by v2 — will drop from E3 training) |
| v2 corpus | preserved unchanged |

**FT corpus impact for E3**:
- Full v2 fact-recall: 2,706 → after strip → 503
- v1 fact-recall: 2,141 → after strip → 0 (fully superseded, dropped from E3 training)
- TASK-behavior corpora (tier1, family_*): 3,500 + 1,200 + 1,700 + 600 = 7,000 preserved
- **E3 training corpus target = 7,000 + 503 = 7,503 rows** (vs current 9,988 = **75% of pre-strip corpus size**, down from the naive strip target of 45% because the retained SUBSTRATE_MISS rows carry real coverage-gap value).

## Recon findings (verified live)

Applied `must-validate-all-claims-before-commit`.

| Claim | Verification | Verdict |
|---|---|---|
| v2 source SHA matches manifest | shasum256 = `e4ab7b4af2f9eafcfd6311974955119d1955efc2c7b45ba763ef0f2bf5443451` (matches manifest field) | ✅ |
| Dry-run arithmetic matches E1 counts | dry-run: input=2706 strip=2203 keep=503 (matches E1 report exactly) | ✅ |
| Source unchanged post-run | pre/post shasum identical | ✅ |
| Output row count = 503 | `wc -l` = 503 | ✅ |
| Leak audit clean | audit_eval_leakage exit 0, 0/290 across 13 tasks | ✅ |
| Manifest well-formed JSON | `json.load` parses; all required fields present | ✅ |

## Decisions

| Decision | Rationale |
|---|---|
| Byte-verbatim preservation (raw line copy) over JSON round-trip | Guarantees kept rows' messages + meta byte-identical to v2 (no re-normalization, no formatter drift, no key-ordering shift) — makes SHA-diff meaningful. |
| No valid.jsonl in v3 bundle | v3 is training-only; E3 benchmark still compares against v2's held-out `valid.jsonl` AND `valid_clean.jsonl` (per CLAUDE.md honest eval). |
| Preserve v1 + v2 corpus dirs | Rollback safety net. Dropping v1 from *training* is an E3 config decision (which SFT bundles to include), not a file-delete here. |
| Leak audit uses valid_clean.jsonl (not valid.jsonl) | `valid_clean.jsonl` is the shipped honest eval (per CLAUDE.md phase 11.5c); `valid.jsonl` predates the anti-leakage refactor. |
| Fail-hard on v2 SHA change during run | Prevents a race-condition strip against a mid-flight rewrite. Not expected in a one-shot script but disciplines the contract. |
| No manifest for v3 valid.jsonl (there is none) | v3 is train-only by design; separate valid split would fragment the eval story. |

## Follow-ups (disclosed, deferred)

### 🔴 BLOCKING for E3/E4 model swap (operator-flagged during E2)

**HOMEBREW-INSTALLER-QWEN-UPDATE-001** — the homebrew installer was originally set up to retrieve the base model + adapters from Ollama Library. Operator note (2026-08-19): "we need to update the homebrew version to accommodate the new Qwen model." Scope questions to resolve before E4 model promote:
- Is the "new Qwen model" the E3 retrain output, OR a newer base (e.g. the task #91 MODEL-SWAP-QWEN27B-EVAL follow-up shipping the 27B base)?
- Will the new adapter get uploaded to Ollama Library (same channel MODEL-DIST-001/002 uses), OR a new channel (HuggingFace? Direct GGUF download?)?
- Backwards-compat: how do existing installations pin the OLD model? Rollback path?
- Ollama runtime is broken on M5/macOS 26.3+ (per CLAUDE.md "Why not Ollama" pin), but Ollama is used ONLY as distribution channel in MODEL-DIST-001 (`mdemg model pull` symlinks the Ollama blob then serves via `llama-server`) — so a runtime failure isn't a blocker for the pull path. Confirm this still holds for the new model.

**MUST be addressed as a distinct sprint** (either before or in lockstep with PHASE-E4-GATE-PROMOTE-001). Filed as task; PR comment will name it as a hard prerequisite.

### Deferred (non-blocking)

1. **PHASE-E3-RETRAIN-BENCHMARK-001** — LoRA retrain on the shrunken corpus (7,503 rows target; v1 dropped, v3 stripped, tier1 + family_* preserved). Benchmark aggregate score vs current 0.9188 baseline; verify no fact-recall regression on `valid_clean.jsonl` (substrate should handle those queries via retrieve+content projection).
2. **PHASE-E4-GATE-PROMOTE-001** — use shipped FT-RECURSIVE-003 fail-closed swap to promote if benchmark passes. Requires HOMEBREW-INSTALLER-QWEN-UPDATE-001 shipped first.
3. **PHASE-E1a-RE-INGEST-001** (optional) — split "part N of M" fragments into individually-retrievable MemoryNodes; could push MISS→COVERAGE ~200-400 more rows for a leaner post-strip corpus. Not blocking E3.

## Arch rules pinned

- **When executing a corpus strip based on an audit-produced strip-list, preserve source files verbatim and SHA-verify pre + post** — protects the rollback path and makes any accidental mutation loud (fail-hard on SHA change during run). Never overwrite the source; always emit to a new versioned directory following the v1→v2→v3 convention.
- **Byte-verbatim preservation of kept rows** (raw line copy, not JSON round-trip) — guarantees kept rows are byte-identical to their source; SHA-diffs between corpus generations become meaningful. JSON round-tripping introduces key-ordering + whitespace drift that pollutes diffs.
- **Leak audit against the honest eval (`valid_clean.jsonl`, not `valid.jsonl`) is a hard gate** — must exit 0 before the corpus ships. Leakage into the eval would poison E3's benchmark and undermine the retrain verdict.

## Documents Accessed

- `training_data/sft/claude_code_knowledge_v2/{train.jsonl,valid.jsonl,manifest.json}` (source; unchanged)
- `training_data/sft/claude_code_knowledge/{train.jsonl,manifest.json}` (v1; unchanged, preserved for rollback)
- `training_data/sft/{tier1,family_classify_notink,family_reasoning_think,family_structured_notink}/manifest.json` (behavior corpora — untouched)
- `training_data/eval/valid_clean.jsonl` (leak-audit target — the honest eval per CLAUDE.md)
- `docs/development/phase-e1-corpus-audit-001/rows_to_strip.jsonl` (2,203 row_idx; strip input)
- `docs/development/phase-e1-corpus-audit-001/audit_report.md` (E1 findings reference)
- `scripts/audit_eval_leakage.py` (shipped leak-audit tool — reused)
- `scripts/phase_e1_corpus_audit.py` (shape reference for the strip script)
- `docs/development/phase-e2-corpus-curation-001/sprint_plan.md`
- CLAUDE.md pins (PHASE-E1-CORPUS-AUDIT-001, CLAUDE-DOCS-INGEST-001, INGEST-TOPOLOGY-REPAIR-001, MODEL-DIST-001, JIMINY-SUBSTRATE-NATIVE-001 arc, task #91 MODEL-SWAP-QWEN27B-EVAL)
