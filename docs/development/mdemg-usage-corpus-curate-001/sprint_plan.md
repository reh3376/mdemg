# MDEMG-USAGE-CORPUS-CURATE-001 — Sprint Plan (v1.0)

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | MDEMG-USAGE-CORPUS-CURATE-001 |
| Task | #144 |
| Filed | 2026-08-24 |
| Author | Roger Henley + Claude (proactive mode) |
| Branch | `reh3376_dev01` |
| Predecessor | MDEMG-DOCS-INGEST-001 (task #142) — supplied substrate; RETRIEVAL-META-DOC-SUPPRESSION-001 (task #143) — cleaned retrieval; PHASE-E1/E2 (tasks #132/#133) — established leak-audit + manifest shape |
| Format version | 12-section v1.0 |
| Est. wall-clock | 2-3 dev-days |
| Est. spend | $20-50 (OpenAI teacher if Epic 3 fires; $0 if deterministic-only path suffices) |

## 2. Problem Statement

Operator directive 2026-08-24 replaced the LoRA-on-new-base-model track: **"the correct path is to ingest information into the MDEMG graphDB and only fine-tune on how to use the mdemg framework."** MDEMG-DOCS-INGEST-001 shipped the ingestion half (1,617 doc nodes on `mdemg-dev` across 5 surfaces). MDEMG-USAGE-CORPUS-CURATE-001 ships the curation half: **synthesize a Q&A SFT training corpus** from those ingested nodes so a future LoRA run can teach the model MDEMG-usage patterns.

Precisely: (a) MDEMG's shipped LoRA (`mdemg-llm-v1`) knows Claude Code documentation deeply (`claude_code_knowledge` family — 2,706 rows) but knows NOTHING about MDEMG itself; (b) the substrate ingest made MDEMG's docs retrievable but does NOT train the model to volunteer MDEMG facts unprompted; (c) a small usage-oriented SFT corpus closes that gap without contaminating the shipped `claude_code_knowledge_v3_stripped` family (frozen per operator directive) or `family_*/tier1` (also frozen).

**Not solved by RAG alone**: RAG covers "given the query, retrieve then answer." SFT covers "produce MDEMG-shaped answers WITHOUT hitting retrieval every turn" — critical for tool-use flows where the classifier makes many decisions per second and cannot afford a substrate round-trip on every one.

## 3. Scope & Constraints

### In scope

- Export ingested MDEMG doc nodes from the `mdemg-dev` substrate
- Deterministic curator: one Q&A row per H2/H3 section (mirrors `curate_claude_docs.py`'s shipped shape)
- Optional teacher-augmentation for cross-section "how-do-I-do-X" Q&A the deterministic can't produce
- Leak audit against `valid_clean.jsonl` (hard-gate identical to PHASE-E2)
- Manifest with SHA + provenance + distribution coverage
- Output: `training_data/sft/mdemg_usage_v1/{train.jsonl,manifest.json,distribution_report.txt}`

### Out of scope (explicit)

- **NO retrain / no LoRA compute in this sprint** — corpus only. Retrain is a follow-up (`MDEMG-USAGE-LORA-001`).
- **NO growth of `claude_code_knowledge*` or `family_*/tier1`** — frozen per operator directive; verified untouched at exit.
- **NO substrate mutation** — read-only queries against `mdemg-dev`.
- No promotion / no gate-eval — the corpus builds, the manifest logs SHA, retrain uses it later.

### Constraints (must obey)

- CUIDv2 for any new IDs (`must-use-cuid2`)
- No hardcoded config (`never-hardcode-config`) — CLI args + env for tunables
- Feature doc at Epic 7 (`mandatory-feature-docs`)
- End all docs with `Documents Accessed` (`end-with-docs-accessed`)
- Sprint plan follows 12-section v1.0 (`must-follow-12-section-format`) — this file
- PR gets summary comment (`must-comment-sprint-summary-on-pr`)
- Lint before commit (`lint-before-commit`)
- 3 testing tiers (`unit-integration-e2e-docs`) — python unit tests + fixture-based integration + live Tier 3 against `mdemg-dev` substrate
- Live Tier-3 with real substrate (`live-testing-tier-required`)

## 4. Dependencies

| Dependency | Status | Notes |
|---|---|---|
| Ingested MDEMG doc nodes on `mdemg-dev` | ✅ shipped | 1,617 nodes verified this sprint recon (997 features + 578 user/api + 37 cli-help + 5 CLAUDE.md) |
| Retrieval works (70% top-3 baseline) | ✅ shipped (task #143) | Not blocking — this sprint reads Neo4j directly, doesn't go through retrieval |
| `neural/training/curate_claude_docs.py` reference | ✅ available | Copy structure; adapt for Neo4j-source vs markdown-file-source |
| `training_data/eval/valid_clean.jsonl` | ✅ available | Leak-audit target; same shape as PHASE-E2 |
| `paradigm_router` or similar SFT tooling | ✅ available | For OPTIONAL Epic 3 teacher augmentation |
| Neo4j reachable | ✅ live | `docker exec cypher-shell` path proven this sprint |
| OpenAI teacher (Epic 3 only) | ⚠️ conditional | Only fires if Epic 2's deterministic yield <1500 rows or coverage <80% |

**No blocking dependencies.**

## 5. Implementation Plan (sequential epics + gates)

Per `sequential-epics`: each epic completes fully before the next starts.

### Epic 1 — Substrate export (~2h)

**Goal**: extract ingested MDEMG doc nodes from `mdemg-dev` as a portable JSON stream keyed by node_id + path + content + surface classification.

Deliverables:
- `scripts/mdemg_usage_export_docs.py` — Neo4j read + JSONL emit
- `training_data/mdemg_usage/raw/nodes.jsonl` — one row per ingested doc node with `{node_id, path, surface, section_header, content, embedding_sha256}`

Gate: row count ≥1,500 across 5 surfaces; features/user/api coverage each nonempty.

### Epic 2 — Deterministic Q&A curator (~4h)

**Goal**: one Q&A pair per node, templated from `section_header` + `path`-derived context.

Deliverables:
- `scripts/mdemg_usage_curate.py` — reads Epic 1 output, emits Q&A rows
- `training_data/sft/mdemg_usage_v1/train.jsonl` — Q&A rows in the shipped `{"messages":[{system,user,assistant}], "meta":{...}}` shape
- Quality filters (mirror `curate_claude_docs.py`):
  - `--min-words` (default 40) — drop tiny sections
  - `--drop-nav-headers` — regex-blacklist for "See also", "Related", "Next steps", "Table of contents"
  - `--drop-junk-paths` — reuse the JIMINY-CORPUS-003 narrative-junk regexblacklist to skip session logs

Gate: yield ≥1,500 curated rows OR yield <1,500 → proceed to Epic 3.

### Epic 3 — Optional teacher augmentation (~4h, conditional on Epic 2 gate)

**Goal**: teacher-synthesized Q&A for cross-section "how do I use X" and "what's the difference between A and B" questions the deterministic curator can't produce.

Fires ONLY if Epic 2 yields <1,500 rows OR distribution has a per-surface gap (>25% skew). Cost cap: $50; if the run projects >$50 spend, halt + report + let operator decide.

Deliverables:
- `scripts/mdemg_usage_teacher_augment.py` — templated prompt to OpenAI teacher
  (`gpt-5.4-mini`, `max_completion_tokens=3000` per `min-max-tokens-3000` rule)
- Rows appended to `train.jsonl` with `meta.label_source='teacher_augmented'` so retrain can weight or filter

Gate: total corpus ≥1,500 rows OR (if deterministic-only was sufficient) skip.

### Epic 4 — Leak audit (~1h)

**Goal**: verify zero row-level overlap with `valid_clean.jsonl` (mirrors PHASE-E1/E2 pattern).

Deliverables:
- `scripts/mdemg_usage_leak_audit.py` — asymmetric 3-gram overlap check against every valid_clean row's assistant content (threshold 0.30 per E1 precedent)
- `training_data/sft/mdemg_usage_v1/leak_audit.json` — per-row verdict + summary

Hard gate: **0 rows PROVEN_OVERLAP** — any leaked row → hard fail, block manifest emission until corrected.

### Epic 5 — Manifest + distribution report (~1h)

**Goal**: canonical manifest matching PHASE-E2 shape + human-readable distribution.

Deliverables:
- `training_data/sft/mdemg_usage_v1/manifest.json`:
  ```json
  {
    "sprint": "MDEMG-USAGE-CORPUS-CURATE-001",
    "family_name": "mdemg_usage_v1",
    "meta_placement": "embedded",
    "row_counts": {"train": N, "total": N},
    "file_sha256": {"train.jsonl": "..."},
    "per_task_counts": {"mdemg.usage": {"total": N, "train": N}},
    "per_surface_counts": {"features": N, "user_api": N, "cli-help": N, "CLAUDE.md": N},
    "source_substrate": "mdemg-dev",
    "source_ingest_sprint": "MDEMG-DOCS-INGEST-001",
    "source_node_count": 1617,
    "curation_method": "deterministic_h2_h3 [+ teacher_augmented]",
    "leak_audit": {"threshold": 0.30, "clean": true, "audited_against": "valid_clean.jsonl"},
    "generated_at_utc": "..."
  }
  ```
- `training_data/sft/mdemg_usage_v1/distribution_report.txt` — per-surface + per-path-prefix + word-count histogram

Gate: manifest matches schema; distribution covers ≥80% of MDEMG shipped surfaces.

### Epic 6 — Verification (~1h)

**Goal**: prove the corpus is retrain-ready.

Live Tier-3:
- SHA-verify: `sha256sum train.jsonl` matches `manifest.file_sha256`
- Row-shape smoke: sample 20 random rows, assert `messages`, `meta.task_name='mdemg.usage'`, `meta.source_node_id` present, `meta.source_path` present
- Zero-leak spot-check: manual read of the leak_audit.json summary
- Distribution assertion: each of 5 surfaces has ≥ (min surface size × 0.5) rows
- `claude_code_knowledge*` untouched: sha of `claude_code_knowledge_v3_stripped/train.jsonl` matches pre-sprint recorded SHA
- `family_*/tier1` untouched: sha of `tier1/train.jsonl` matches pre-sprint recorded SHA

### Epic 7 — Documentation (`mandatory-feature-docs`) (~1h)

Deliverables:
- `docs/features/mdemg-usage-corpus.md` — Why / What / Choices / How-it-works / How-to-use / How-to-extend
- `docs/development/mdemg-usage-corpus-curate-001/sprint_post.md` — decisions, live evidence, follow-ups
- CHANGELOG.md — Unreleased entry
- PR summary comment

## 6. Testing Plan (3 tiers)

### Tier 1 — Unit tests (python + pytest)

- `test_export_docs.py`: given a mocked Neo4j driver returning 5 fixture nodes, exporter emits 5 JSONL rows with correct schema
- `test_curate.py`: given fixture nodes with H2/H3 sections + junk sections, curator produces expected N rows + skips junk by predicate
- `test_leak_audit.py`: given a target row + a copy of the target's answer, overlap=1.0 → PROVEN_OVERLAP flag
- `test_manifest.py`: manifest builder produces valid JSON matching PHASE-E2 shape

### Tier 2 — Integration tests

- End-to-end pipeline on a 3-file fixture: export → curate → leak-audit → manifest; all files exist + all counts consistent + manifest SHA matches file SHA

### Tier 3 — Live e2e on real substrate (`live-testing-tier-required`)

- Run full pipeline against `mdemg-dev` (1,617 nodes)
- Verify: >1,500 rows produced, 0 leaks, all 5 surfaces present, manifest generated
- **Forcing observation**: manually spot-check 5 rows across features/user/api/cli-help/CLAUDE.md surfaces — verify Q is a reasonable question, A is the section content, meta.source_node_id resolves back to the substrate node via `MATCH (n {node_id: $id}) RETURN n.path`
- **Forcing observation**: verify frozen families untouched via SHA compare before + after

## 7. Commit Strategy

Sequential commits — one per epic-cluster:

1. `feat(training): MDEMG-USAGE-CORPUS-CURATE-001 Epic 1-2 — export + curate` (Epics 1 + 2)
2. `feat(training): MDEMG-USAGE-CORPUS-CURATE-001 Epic 3 — teacher augment` — conditional; skip if Epic 2 sufficient
3. `feat(training): MDEMG-USAGE-CORPUS-CURATE-001 Epic 4-5 — leak audit + manifest`
4. `docs(training): MDEMG-USAGE-CORPUS-CURATE-001 Epic 6-7 — verification + feature doc + sprint post`

Each commit lint-clean per `lint-before-commit`. All commits on `reh3376_dev01`, PR to main via auto-PR workflow (no direct-to-main per `no-direct-main-commits`).

## 8. Verification Checklist

- [ ] Epic 1: `nodes.jsonl` produced; row count ≥ 1,500
- [ ] Epic 2: `train.jsonl` produced; each row has `messages` + `meta.source_node_id`
- [ ] Epic 3 (conditional): if fired, cost ≤ $50 + rows tagged `label_source='teacher_augmented'`
- [ ] Epic 4: `leak_audit.json` reports `clean=true`
- [ ] Epic 5: `manifest.json` valid + matches PHASE-E2 shape + SHA matches train.jsonl
- [ ] Epic 6 live smoke: 5 random rows manually spot-checked; source_node_id resolves back to substrate
- [ ] Epic 6 frozen-families check: `claude_code_knowledge_v3_stripped` + `tier1` SHAs unchanged
- [ ] Epic 7 feature doc + sprint post written
- [ ] Unit tests pass (pytest exit 0)
- [ ] Integration test passes
- [ ] Lint clean (`golangci-lint` N/A — python-only sprint; `ruff check` clean)
- [ ] CHANGELOG entry added
- [ ] PR comment posted
- [ ] Task #144 marked completed
- [ ] Corpus ≥1,500 rows / 0 leaks / ≥80% surface coverage

## 9. Documentation Update (final epic — never cut)

Per `mandatory-feature-docs` + `end-with-docs-accessed`:

- `docs/features/mdemg-usage-corpus.md` (new)
- `docs/development/mdemg-usage-corpus-curate-001/sprint_post.md` (new)
- `CHANGELOG.md` (Unreleased entry)
- `PR #646` (or new PR) sprint-summary comment

## 10. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| Q&A yield low (<1,500 rows) | Epic 3 teacher augment as escape hatch; cost-capped at $50 |
| Q&A quality low (deterministic Qs feel wooden) | Deterministic templates use "How does X work?" / "What does Y do?" / "When should I use Z?" variants — proven on `curate_claude_docs.py`; Epic 3 augment for cross-section synthesis |
| Leak with `valid_clean.jsonl` | Hard gate at Epic 4 — pipeline halts, corpus not shipped |
| Surface skew (features dominates 997/1617) | Distribution report shows per-surface counts; if skew >2:1 vs any other surface, add teacher-augment rows for the underrepresented surface |
| Substrate content contains bugs / stale info | Not this sprint's problem — the source-of-truth is what shipped; retrain on shipped state, docs stay in sync |
| `family_*/tier1` or `claude_code_knowledge*` accidentally touched | SHA pre-check + post-check on both dirs; hard-fail if changed |
| Neo4j read timeout on 1,617-row export | Paginated fetch (`SKIP ... LIMIT ...`); 500-row batches |
| Teacher spend spike | Cost projection printed before run; halt if >$50 |
| Row schema drift from shipped `claude_code_knowledge*` shape | Emit rows in identical `{messages, meta}` shape; unit test asserts field-set match |

## 11. Documents Accessed

- `docs/development/mdemg-docs-ingest-001/{sprint_plan,verdict,live_verify_report,sprint_post}.md`
- `docs/development/retrieval-meta-doc-suppression-001/{sprint_plan,verdict,sprint_post}.md`
- `docs/development/phase-e1-corpus-audit-001/{sprint_plan,sprint_post,audit_report}.md`
- `docs/development/phase-e2-corpus-curation-001/{sprint_plan,sprint_post,leak_audit}.md`
- `training_data/sft/claude_code_knowledge_v3_stripped/manifest.json` (canonical manifest shape)
- `training_data/sft/claude_code_knowledge_v3_stripped/train.jsonl` (canonical row shape — first row sampled)
- `training_data/eval/valid_clean.jsonl` (leak-audit target — schema confirmed matches shipped)
- `neural/training/curate_claude_docs.py` (reference deterministic curator)
- `neural/training/chunk_claude_docs.py` (reference chunker)
- `scripts/curate_guidance_corpus.py`, `scripts/phase_e1_corpus_audit.py`, `scripts/phase_e3_assemble_corpus.py` (SFT-adjacent script patterns)
- Live Neo4j via `docker exec mdemg-neo4j-1 cypher-shell` (substrate distribution: features 997 / user+api 578 / cli-help 37 / CLAUDE.md 5)
- Live `/healthz` (server up: circuit_breakers ok / jiminy ok / neo4j ok / tsdb ok)
- Live `/v1/memory/retrieve` (retrieval healthy post RETRIEVAL-META-DOC-SUPPRESSION-001)
- CLAUDE.md pins:
  - MDEMG-DOCS-INGEST-001 (task #142)
  - RETRIEVAL-META-DOC-SUPPRESSION-001 (task #143)
  - PHASE-E1-CORPUS-AUDIT-001 (task #132) — leak-audit shape source
  - PHASE-E2-CORPUS-CURATION-001 (task #133) — manifest shape source + tombstone-safety pattern
  - Sprint plan format (`must-follow-12-section-format`)
  - Sequential epics rule (`sequential-epics`)
  - Corpus freeze rule (`claude_code_knowledge*` + `family_*/tier1` immutable per operator 2026-08-24)
- Operator directive 2026-08-24 (deep-dive workflow `wf_b389463a-61b` final recommendation Alt 1)

## 12. Rollback Procedures (destructive ops)

**No destructive operations planned.** Rollback = delete `training_data/sft/mdemg_usage_v1/` directory. Neo4j substrate untouched (read-only). Frozen SFT families protected by SHA pre-check + post-check at Epic 6.

If Epic 3 teacher augment runs, the OpenAI spend is unrecoverable but capped at $50. If Epic 4 leak-audit fires, pipeline halts BEFORE manifest emission, so no polluted corpus lands.

Full-rollback command:

```bash
rm -rf training_data/sft/mdemg_usage_v1/
rm -rf training_data/mdemg_usage/
git checkout scripts/mdemg_usage_export_docs.py scripts/mdemg_usage_curate.py scripts/mdemg_usage_leak_audit.py 2>/dev/null  # if not committed
# Frozen families protected by SHA — no restore needed
```
