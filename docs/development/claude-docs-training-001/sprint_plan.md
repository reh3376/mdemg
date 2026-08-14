# CLAUDE-DOCS-TRAINING-001 — Sprint Plan

## 1. Header & Metadata

Sprint: `CLAUDE-DOCS-TRAINING-001` · opened 2026-08-14 · branch `reh3376_dev01`
Target span: 1–2 weeks calendar (incl. HITL grading) · Effort: ~5 dev-days + ~30-45 min cumulative training compute on M5 Max (three 10-15 min LoRA subset runs per Phase 6 shipped speed) · OpenAI spend $0 (no teacher distillation — docs are the ground truth) · Risk: medium (new adapter on top of Phase-5 base; promotion is operator-gated behind the shipped 3-layer guard).

**Arc-external to JIMINY-CEILING-BREAK-2** — new task (`claude.code_knowledge`), new adapter target, does not touch the `jiminy.synthesize` follow-rate signal that the arc is measuring. Scoping starts now; execute post-arc if bandwidth-constrained.

**Pattern:** FT-CLASSIFY-002 vertical slice (capture → curate → train → gate) walked manually for a new-task cold-start.

## 2. Problem Statement

Operator directive (2026-08-14):
> An additional mechanism that Jiminy needs to learn when working specifically with Anthropic models is `/` commands and settings. We need a training session for local LLM specifically related to understanding settings and other relevant claude/commands. To do so we will need to do a webscrape ingest on claude documentation to create the needed curated datasets for the LoRA training.

**Why now.** `mdemg-llm-v1` (Qwen3-14B LoRA on 7 dense target modules, Phase 5 baseline 0.8553 on the 16-task augmented eval) has no Anthropic-specific knowledge. Users of MDEMG expect Jiminy — and the 16 LLM call sites that route through it — to reason correctly about Claude Code slash commands, `settings.json` keys, hook events, MCP tool contracts, Agent SDK classes, and Messages API surface when acting as governance/context for Claude-Code-driven work. Today it hallucinates or defers on those queries because the training corpus never covered them. The cost of the miss is silent: Jiminy answers plausibly-wrong instead of correctly-citing, and the operator has to catch it manually every session.

## 3. Scope & Constraints

**In scope — three source domains** (public docs only):

1. `https://docs.claude.com/en/docs/claude-code/*` — CLI reference, settings, hooks, slash commands, sub-agents, MCP, output styles, plans, worktrees. Primary corpus (~60-70% of budget).
2. `https://docs.anthropic.com/en/docs/agents-and-tools/*` — Agent SDK, tool use, prompt caching. Secondary (~20%).
3. `https://docs.anthropic.com/en/api/messages` — Messages API reference (params, streaming, tool_use blocks, cache_control). Tail (~10-20%).

**In scope — deliverables:**

- (a) `docs-scraper` audit + a `claude-docs` scrape configuration pinning the three URL roots, respecting robots.txt, 1s rate limit, 500KB content cap (already the plugin defaults).
- (b) Cached scrape corpus at `training_data/claude-docs/raw/` (gitignored).
- (c) 200-500 curated Q&A pairs at `training_data/claude-docs/curated/qa.jsonl` (one canonical concept per pair — one settings.json key, one slash command, one hook event, one MCP tool contract, one Messages-API param).
- (d) New ULTS spec `docs/tests/ults/specs/claude_code_knowledge.ults.json` with SHA-hashed system prompt, output schema, quality metrics.
- (e) Hand-authored ~50-row golden holdout `training_data/eval/claude_code_knowledge_golden.jsonl`.
- (f) UBENCH spec update — bump `expected_specs` to 18, `expected_tasks` +1, add `min_rows_per_task` coverage.
- (g) LoRA subset dry-run then full run atop Phase-5 adapter (rank 32 α=64, 7 dense target modules, seq 8192, explicit iters ~2 epochs cap 3, early-stop `val_loss>best×1.05×2`).
- (h) HITL gold-grade curated corpus + golden set via `NoopSink`.
- (i) Full-sweep A/B gate + 3-layer promotion guard rehearsal.
- (j) Operator promotion decision via `mdemg ft-loop promote`.
- (k) Feature doc `docs/features/claude-code-knowledge-adapter.md`.

**Out of scope (explicitly):**

- Internal Anthropic docs (customer/employee portals, unreleased features).
- Non-Claude Anthropic products (Workbench UI, console features, billing).
- Scraping user data or private conversation transcripts.
- Expanding to a *reasoning-about-Claude* task (this sprint teaches recall of concrete concepts, not judgment).
- Autonomous retrain triggering (recursive-loop actuator remains gated per FT-RECURSIVE-001).
- Modification to the Phase 5 adapter itself (decided in Epic 5: stacked vs resume-continue based on dry-run val_loss).
- DPO/RL (this is SFT on Q&A pairs).
- Scraping any domain outside the three URL roots in §3.

**Constraints:** public docs only; scraper honors `robots.txt` (plugin default at `fetcher.go:89`); 1s rate limit; aggressive local cache; curated rows carry per-source URL + fetch timestamp; ULTS spec required BEFORE training rows land (Phase-10 registry rule); UBENCH `min_rows_per_task` gate must PASS before training (the guardrail.evaluate precedent); promotion is 3-layer-guarded (pre-swap canary + fail-closed SwapServing + post-swap tripwire) and operator-confirmed — no auto-promotion.

## 4. Dependencies

- **`plugins/docs-scraper/`** — shipped Go ingestion plugin (v1.0.0; robots.txt cache; `Sync` for batch URL lists; HTML→text extractor; quality scoring; tag suggestion). Defaults: `rate_limit_ms=1000`, `respect_robots_txt=true`, `max_content_length_kb=500`, `user_agent="MDEMG-Scraper/1.0"`, 10 MB hard body cap, `default_profile=documentation`. **No historical runs against Anthropic docs on record** — Epic 1 audits.
- **HITL-REVIEW-001** — shipped review surface (`Review` tab at `:9999/ui/`, `/v1/review/*`, `ReviewableDataset` interface + `NoopSink` for gold-only).
- **ULTS framework** — shipped 17-task registry. Adding an 18th spec requires a valid `.ults.json` with `domain.action`-formatted name.
- **UBENCH framework** — shipped contract gate. Adding a ULTS task without seeding golden rows fails the contract loudly (Phase 10 `guardrail.evaluate` precedent).
- **FT-RECURSIVE-001** (shipped) — `mdemg ft-loop report-stage --stage <capture|curate|train|benchmark|gate|promote>` writes each stage to `scheduled_job_events` with a high alert on failure.
- **FT-RECURSIVE-002/003** (shipped) — 3-layer promotion guard; operator promotion via `mdemg ft-loop promote`. This sprint is a first end-to-end user on a *new* task.
- **Phase 5 dense base** — `mlx-community/Qwen3-14B-4bit` SHA-pinned; `adapters/tier1/`; LoRA config surface at `neural/training/train_ft.py` + `configs/sft_phase11_5d_distill.yaml` as template; `neural/benchmarks/run_benchmark.py` for the A/B sweep.
- **Guidance training corpus** — this sprint's rows do NOT enter `guidance_training_rows` (that table is for live-guidance-interaction data). This is a manually-curated docs corpus; it lands in `training_data/claude-docs/curated/qa.jsonl` and is registered to `train_ft.py` via an explicit dataset config.

## 5. Implementation Plan (sequential epics + gates)

**Epic 0 — Sprint plan** (this doc).

**Epic 1 — `docs-scraper` audit + `claude-docs` scrape configuration.** Verify plugin registered, health-check passes, robots.txt honoring works against both target domains (Epic-1 gate records exact verdicts + timestamp). Feasibility check: enumerate the three URL roots via sitemap.xml or top-level index — expect ~200 pages, ~2000 canonical concepts, scope to highest-signal 30% (~600 concepts → 200-500 curated rows). Produce `configs/scrape/claude_docs.yaml` (URL list, `extraction_profile=documentation`, `max_pages=250`, `rate_limit_ms=1000`, honors 500KB cap). Land a `scrape_manifest.json` capturing per-URL: SHA of raw HTML, fetch timestamp, content-type, size bytes, quality_score. **Gate:** manifest exists, robots.txt verdicts recorded, ≥95% URL fetch success.

**Epic 2 — Scrape + cache raw corpus.** Invoke scraper's `Sync` against `claude_docs.yaml`; artifacts at `training_data/claude-docs/raw/{docs_claude_com,docs_anthropic_com}/*.html` (gitignored) + manifest at `training_data/claude-docs/scrape_manifest.json` (committed). Report `capture` stage via `mdemg ft-loop report-stage`. **Gate:** raw corpus present; manifest SHA-audit reproducible; ≤250 MB total; no 429s in last 20 min.

**Epic 3 — Curate to structured Q&A pairs.** New Python script `neural/training/curate_claude_docs.py` (unit-tested) walks raw HTML and extracts one canonical concept per Q&A pair using deterministic rules:
- (a) each `settings.json` key → one pair (`Q: What does <key> control?` `A: <verbatim doc explanation + type + default>`).
- (b) each slash command → one pair (`Q: What does /<cmd> do? What are its arguments?`).
- (c) each documented hook event → one pair (`Q: When does <event> fire? What context is available?`).
- (d) each MCP tool contract → one pair (`Q: What does <tool> accept and return?`).
- (e) each `POST /v1/messages` param → one pair.

Output: `training_data/claude-docs/curated/qa.jsonl` (~200-500 rows, target 350), each carrying `{prompt, completion, source_url, source_sha, concept_type, curated_at}`. Include per-concept-type distribution report; drop rows below quality scorer's 0.5 floor. Report `curate` stage. **Gate:** 200 ≤ row count ≤ 500; per-concept-type distribution recorded; every row's `source_sha` present in Epic-2 manifest; 0 dedup collisions on `prompt` SHA.

**Epic 4 — Register ULTS task + author golden holdout + UBENCH contract.** Author `docs/tests/ults/specs/claude_code_knowledge.ults.json` (`ults_version=1.0.0`, `task.name=claude.code_knowledge`, `training_config.rank=32`, `min_examples=200`, `quality_gate=0.7`, `output_schema` describing free-form textual answer or structured `{answer, citations[]}` — decide in Epic 4 with operator preference; `quality_metrics` = `factuality` (0.5, threshold 0.7) + `citation_present` (0.3, threshold 0.9) + `concision` (0.2, threshold 0.5)). Verify hash + weights-sum via `ults_runner.py`. Hand-author ~50 golden Q&A pairs at `training_data/eval/claude_code_knowledge_golden.jsonl` (operator + sprint co-author; each row carries expected answer + doc URL). Update `docs/tests/ubench/specs/mdemg.ubench.json`: `ults.expected_specs: 17→18`, `golden_holdout.expected_tasks: N+1`, recompute `sha256` after appending, keep `min_rows_per_task ≥3`. Run `make test-ubench-contract` — MUST pass. Register as HITL dataset (add to `internal/api/llm_dataset.go` or new `claude_docs_dataset.go`) with `NoopSink` (gold-only). **Gate:** ULTS runner green; UBENCH contract green; HITL `/v1/review/datasets` lists new dataset; leak audit between 350-row corpus and 50-row golden holdout = 0 overlap.

**Epic 5 — LoRA subset dry-run then full run.** Config from `configs/sft_phase11_5d_distill.yaml` template: resume from `adapters/tier1/` (Phase-5 adapter; Epic-5 decides stacked vs resume-continue based on dry-run val_loss), rank 32 α=64, 7 dense target modules, seq 8192, batch 4, lr 1e-5, explicit iters, epoch cap 3, early-stop `val_loss>best×1.05×2`. **First run:** 50-row subset dry-run to confirm config loads + val_loss descends + no OOM on M5 Max (~10 min wall; llama-server :8102 stays up — 11.5d/FT-CLASSIFY-002 precedent at ~36 GB peak). **Second run:** full 350-row corpus (~10-15 min wall). Report `train` stage both times. Land artifacts at `adapters/claude_docs_001/` (dry-run) and `adapters/claude_docs_001_full/`. **Gate:** dry-run val_loss descends ≥3 checkpoints; full-run early-stop log present; adapter files land; no OOM; llama-server uptime uninterrupted.

**Epic 6 — HITL grade + full-sweep A/B + promotion decision.** HITL: operator grades stratified sample (~30 rows across 5 concept types) of curated corpus via `http://localhost:9999/ui/` → Review → `claude_code_knowledge` — grades land in `review_grades` as gold-only. Run `neural/benchmarks/run_benchmark.py` (via `make test-ubench-run`) full-sweep on augmented eval (16 shipped tasks + new `claude_code_knowledge` = 17 total) — candidate `claude_docs_001_full` vs Phase-5 baseline vs gpt-5.4-mini. **Gate criteria** (FT-CLASSIFY-002 pattern): (a) aggregate weighted score ≥ 0.8553 (Phase-5 baseline); (b) `claude_code_knowledge` materially up vs baseline — target ≥30pp improvement (baseline expected near-zero); (c) no shipped task regresses >2pp (FT-RECURSIVE `[AMD-2]` rule). Report `benchmark` + `gate` stages. If gate passes, rehearse 3-layer promotion guard without promoting. **Operator gate:** present gate results + HITL corpus quality + guard rehearsal; on explicit `mdemg ft-loop promote`, fuse → GGUF Q5_K_M → SwapServing performs fail-closed swap → post-swap tripwire monitors 30 min. On any tripwire failure, SwapServing rolls back automatically. Feature doc + CHANGELOG + CLAUDE.md FT note + `post.md` + `run_record.md`. Push → auto-PR.

## 6. Testing Plan (3 tiers)

**Tier 1 (unit):** `neural/training/curate_claude_docs.py` unit tests (per-concept-type extraction rules; dedup by prompt SHA; quality-floor filter; source_sha membership); `ults_runner.py` validation of new spec (weights sum, task name format, hash format); `audit_eval_leakage.py` extension for new training↔golden pair; scrape-manifest reproducer test.

**Tier 2 (integration):** end-to-end `docs-scraper` `Sync` against mocked HTTP fixture (patterned in `extractor_test.go`); `make test-ubench-contract` green with 18-task spec + new golden rows; HITL `/v1/review/datasets` lists new dataset live; ft-loop stage reports for capture/curate/train/benchmark/gate land in `scheduled_job_events`.

**Tier 3 (live, required):** the full-sweep A/B benchmark IS the live test — real `llama-server :8102` + real Phase-5 baseline + real gpt-5.4-mini judge, 17-task valid_clean + new golden holdout. Gate result is the promotion signal. Additionally: baseline vs adapter delta per-concept-type on new task (so partial wins are legible). Post-promotion: 30-min tripwire against production traffic; smoke test — ask Jiminy 10 hand-authored queries, compare pre-swap vs post-swap verbatim (documented in `run_record.md`).

## 7. Commit Strategy

Sequential commits per epic on `reh3376_dev01`. Push → auto-PR at Epic-2 (raw scrape manifest committed; raw HTML gitignored) and again at Epic-6 gate-result time. Artifacts >100 MB (raw HTML, adapter weights, GGUF output) stay untracked per training_data conventions; manifests + configs + curated Q&A JSONL + ULTS spec + UBENCH update + HITL registration + feature doc all committed. Per-epic build + lint (docs-scraper + neural pytest + ruff + `ults_runner.py` + `make test-ubench-contract`) green before next epic.

## 8. Verification Checklist

- [ ] `docs-scraper` audit recorded (health-check, robots.txt verdicts, knobs) — Epic 1
- [ ] `configs/scrape/claude_docs.yaml` + `scrape_manifest.json` committed; ≥95% fetch success — Epic 1-2
- [ ] `training_data/claude-docs/curated/qa.jsonl` — 200-500 rows, per-type distribution, 0 dedup collisions, all rows trace to Epic-2 manifest — Epic 3
- [ ] `claude_code_knowledge.ults.json` — ULTS runner green — Epic 4
- [ ] Golden holdout — ~50 rows, doc-URL-justified, 0-leak vs curated corpus — Epic 4
- [ ] `mdemg.ubench.json` bumped to 18 specs; `make test-ubench-contract` green — Epic 4
- [ ] HITL dataset registered; NoopSink; SME grades ≥30 stratified rows — Epic 4, Epic 6
- [ ] LoRA subset dry-run + full run land; val_loss descent + early-stop; no OOM; llama-server uptime uninterrupted — Epic 5
- [ ] ft-loop stage reports land in `scheduled_job_events` — throughout
- [ ] Full-sweep A/B: aggregate ≥0.8553; new task materially up; no shipped task regresses >2pp — Epic 6
- [ ] 3-layer promotion guard rehearsal recorded; operator promotion decision documented — Epic 6
- [ ] Feature doc + CHANGELOG + CLAUDE.md FT note + `post.md` + `run_record.md` — Epic 6

## 9. Documentation Update (Epic 6, never cut)

- `docs/features/claude-code-knowledge-adapter.md` — new feature doc mirroring `hitl-review.md` / `ubench-framework.md` shape (Why / Choices / How it works / How to use / Rollback / Follow-ups); scrape-source list, concept-type taxonomy, golden-holdout authoring recipe (reproducibility), delta vs Phase-5 baseline per concept type.
- `docs/development/claude-docs-training-001/{post,run_record}.md` — post-mortem + per-stage run record (timings, gate deltas, disclosed limitations).
- `CHANGELOG.md`, `CLAUDE.md` FT-notes section, `00_README_v2.md` STATUS advance if promoted.
- `ROADMAP` note that Claude-Code-knowledge is a recurring refresh candidate (Anthropic docs churn; 3-6 month re-scrape + incremental-train cadence).

## 10. Risks & Mitigations

| Risk | L | I | Mitigation |
|---|---|---|---|
| Docs licensing — public docs are copyrighted; training on them may exceed permitted use | Low | Med | Public docs, respectful ingest (robots.txt, rate-limited), local use, no redistribution; if extra caution wanted, restrict to reference sections (params, keys, event names — factual metadata) |
| Scrape-scope creep | Med | Med | Scope frozen at three URL roots; adding a fourth requires a new sprint |
| Golden-set bias — ~50 hand-authored rows reflect operator usage, not general docs surface | Med | Med | Stratify across 5 concept types; document bias in feature doc; v2 refresh sprint expands coverage |
| Over-fitting to docs prose | Med | Med | Q&A pairs paraphrase (not verbatim copy — Epic-3 rule); early-stop guards; per-concept-type A/B slice exposes memorization |
| Adapter regression on shipped 16 tasks | Med | High | Explicit iters + epoch cap 3 + early-stop; A/B gate enforces no-task-regress-more-than-2pp; 3-layer promotion guard catches escapes |
| Anthropic docs change between scrape and use → stale data | Low | Low | Manifest captures fetch timestamp + per-URL SHA; refresh sprint is cheap (~1 day) |
| `ults_version` / `.ults.json` schema drift | Low | Low | ULTS runner is the schema authority |
| llama-server contention during training (M5 Max RAM) | Low | Med | 11.5d + FT-CLASSIFY-002 precedent — coexist at ~36 GB peak |

## 11. Rollback Procedures

**Pre-promotion:** nothing to roll back — all artifacts are files (raw scrape, curated JSONL, ULTS/UBENCH specs, adapter weights). Deleting `adapters/claude_docs_001_full/` and reverting the ULTS/UBENCH commits undoes the sprint cleanly; Phase-5 adapter untouched.

**Post-promotion:** adapter promotion is already 3-layer-guarded per FT-RECURSIVE-002/003 — pre-swap canary blocks a bad adapter; SwapServing is fail-closed (a failed swap leaves prior GGUF symlink in place); post-swap tripwire monitors 30 min and triggers `SwapServing rollback` on regression. Manual rollback path (Stage-E): restore prior GGUF symlink target + `launchctl kickstart -k com.mdemg.llama-server`; adapter + eval artifacts archived per 11.5e precedent. **Rollback is a solved problem** — this sprint is a first user of the guard, not a builder of a new one.

## 12. Documents Accessed

- `plugins/docs-scraper/{manifest.json,fetcher.go,ingestion.go,extractor.go,quality.go,tagger.go,main.go,lifecycle.go}` — shipped scraper plugin.
- `docs/features/{hitl-review,ubench-framework,ults-framework}.md` — shipped grading + benchmarking + spec frameworks.
- `docs/development/ft-classify-002/sprint_plan_ft_classify_002.md` — pattern-of-record vertical slice this sprint mirrors.
- `docs/development/ft-recursive-001/{sprint_plan_ft_recursive_001.md,SPEC_recursive_retraining_loop.md,augmented_eval_manifest.json}` — shipped observability + eval pinning; `mdemg ft-loop report-stage` contract.
- `docs/development/ft-recursive-002/sprint_plan_ft_recursive_002.md`, `docs/development/ft-recursive-003/sprint_plan_ft_recursive_003.md` — shipped 3-layer promotion guard; `mdemg ft-loop promote`.
- `neural/training/` — LoRA config surface + `train_ft.py`, `evaluate_ft.py`, `regression_gate.py`.
- Operator directive (2026-08-14) quoted §2.
