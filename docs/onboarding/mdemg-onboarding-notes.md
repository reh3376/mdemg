# MDEMG Onboarding — Speaker Notes Companion

**Pair with**: `mdemg-onboarding-deck.md`
**Format**: one `## Slide N — <Title>` section per slide
**Last verified**: 2026-10-22

Advanced Slides supports inline `note:` blocks (already in the deck). This companion is for operators who brief new team members WITHOUT Obsidian installed — flat markdown is universally readable.

**Content pattern per entry**:
- The 2-4 sentences the presenter says
- Pause-for-questions marks on specific slides
- Fallback "simplify-for-PM" phrasing when a slide uses engineering jargon
- Concrete numbers that may rot (quarterly refresh)

---

## Slide 1 — Title

Audience: senior engineers / technical PMs who were senior engineers. Not for exec summaries, product owners, or beginners. Every substantive claim traces back to CLAUDE.md, docs/features, sprint records, or shipped code. **Pause 1 min for questions on audience fit** before continuing. Explicitly say: "If any claim here feels marketing-shaped, flag it — we'll trace it live."

## Slide 2 — F1 Section Header

Target: 12 slides, ~10 min. Four durable mental hooks to establish: what MDEMG is, the core analogy, the honest history, the 10k-ft system view. F1 is intentionally shallow — depth lives in F2–F9.

## Slide 3 — What MDEMG Is Trying To Do

Quote the vision sentence verbatim. The three bullets are the operating consequences: substrate vs tool, decisions as atomic unit, connections as optimization target. **PM-fallback**: "MDEMG is persistent memory that helps the coding assistant remember what mattered yesterday."

## Slide 4 — The Core Analogy

Load-bearing slide. **Pause for questions here** — if the analogy doesn't land, everything downstream drifts. If someone calls MDEMG a "tool" during Q&A, pause and reframe. **PM-fallback**: "The AI assistant's inner monologue — the thing it refers to when thinking about your project."

## Slide 5 — History Timeline

~2 years of work compressed to 12 nodes. Linear read left-to-right; actual dependencies branch. Transition: "Next slide is the if-you-only-had-5-minutes view — the pivots."

## Slide 6 — Honest Pivots

Each decision was "the data decided," not an opinion. **Pause 2 min here** — these invite real Q&A. The playbook: measure before flipping; honest no-ops are legitimate verdicts.

## Slide 7 — Where We Are Today

The follow-rate number is what'll move most — live measure of substrate quality from the receiver side. Quarterly-refresh commitment (operator refreshes this slide + the F10 state slide quarterly). Numbers rot fast — expect this slide to need updates.

## Slide 8 — System Overview

Spend ~1 min per cluster. **Don't go deep** — F2-F9 do depth. 12 nodes, 18 edges — legible at 1280×720. Transition: "We'll zoom into each cluster. Next: how memories are organized."

## Slide 9 — 5-Layer Memory Hierarchy

Each layer has shipped consolidation code; name the step on each arrow. Retrieval traverses this hierarchy. Abstraction edges (GENERALIZES, ABSTRACTS_TO) link L0→L5. F2 goes deeper. **PM-fallback**: "L0 is 'thing just happened,' L1 is 'this is a rule we learned,' L2+ are 'these rules cluster into concepts.'"

## Slide 10 — Hot Path

This loop runs on EVERY tool use. Observability: Grafana `mdemg-jiminy` dashboard + alert evaluator. F4 goes deeper on Jiminy. **PM-fallback**: "Every click/edit/command the assistant makes goes through this loop — a few milliseconds of memory + policy check."

## Slide 11 — The Recursive-Improvement Arc

4 loops at different time constants against the same substrate. Default-off on FT loop (opt-in via `FT_LOOP_ENABLED`). F7 goes deeper on RSIC + HITL; F5 goes deeper on FT; F2 goes deeper on Hebbian.

## Slide 12 — Observability

Alert-truth family (ALERT-TRUTH-001, DASHBOARD-TRUTH-001/002/003) is the paper trail. Most "scary" alerts are measurement artifacts, not real regressions. Triage before fixing. **PM-fallback**: "We watch the system watch itself. Scary numbers often mean the gauge is wrong, not the system."

## Slide 13 — F1 Recap → F2

Pause 2 min for questions. **This is the heaviest Q&A gate** — unresolved confusion here contaminates everything downstream. Transition: "Next: F2 — the substrate itself."

---

## Slide 14 — F2 Section Header

Target: 12-16 slides. 1 full (Neo4j CMS + Hebbian + consolidation) + 1 mini (TSDB). Load-bearing section — the surface everything else reads/writes.

## Slide 15 — Neo4j CMS What It Is

Space is the top-level isolation unit. One graph per space. `mdemg-dev` is operator's primary space. Beta testers get `beta-tester-<submission_id>`. **PM-fallback**: "A graph database — nodes are memories, edges are relationships."

## Slide 16 — Why Graph Over Vector-Only

Vector similarity is one signal. Edge traversal is a different signal. RRF (F3) fuses them. Pure vector DB forces every "related to" into similarity space; graph edges preserve typed semantics. **PM-fallback**: "Vector DBs can only answer 'what's similar?' Graph DBs can answer 'what points to what, through what kind of relationship.'"

## Slide 17 — Node + Edge Schema

CUIDv2 identifiers — never UUID (strict feedback rule). `embedding` dimension matches configured provider. `GUIDANCE_OUTCOME` is a self-loop pattern — the sink Jiminy outcomes write to.

## Slide 18 — Tombstone Invariant

Load-bearing data rule. Audit any new Cypher querying `role_type='constraint'` for the archive-filter (JIMINY-ARCHIVED-CODE-FILTER-001 arch rule). **PM-fallback**: "We never delete. If something's wrong, we mark it 'archived' and can un-mark later."

## Slide 19 — Why Important

The never-delete invariant is the single most load-bearing data rule. Space isolation is first-class — one agent cannot see another's substrate without explicit cross-space query. Backup/restore (F7) is the second line of defense; primary line is the never-delete invariant.

## Slide 20 — How It Will Improve

The substrate is mature; most innovation now moves one layer up (classifier semantics, retrieval precision). Open items are primitives, not blockers. Operators who want to try HEBB-ETA-001 can flip the flag + run the backfill CLI.

## Slide 21 — Hebbian 4 Paths

All 4 federated via EVENTGRAPH-001..004 to `reinforcement_events` TSDB. Default-off entrypoints before fix: `CoactivateSession` had NO caller (nil-guard swallowed every observation) — fixed by wiring `convSvc.SetLearningService(lea)`. **PM-fallback**: "When memories co-activate during retrieval, we strengthen the connection between them. Like neurons that fire together, wire together."

## Slide 22 — Consolidation 22-Phase

The 38-min cost was `dynamic_edges` doing Cartesian product over 8705 L≥minLayer nodes. Fix: vector-index per-node top-K → O(n·logn). Steady-state ~47 sec. Full re-cluster retained as `concepts recluster` escape hatch.

## Slide 23 — Incremental Hidden Clustering

Pre-CHURN-003: destroy-recreate anti-pattern = 25% residual churn per cycle. Post-fix: 100% survival. Trade-off: pattern count grows slowly; `concepts recluster` is quality-maintenance escape hatch.

## Slide 24 — TSDB Telemetry Plane

`metric_samples` time column is `time`, NOT `recorded_at`. Alert rules crossing this silently fail. **No `/metrics` endpoint — 404 by design** (PROMETHEUS-SCRAPE-INVESTIGATION-001). Grafana reads via `timescaledb` datasource.

## Slide 25 — TSDB Writer + Reader Contract

V0025 added retention + compression to EVERY previously-unbounded hypertable (`embedding_events` was 2.7 GB). Rule: never alert on lifetime-cumulative percentile synthetics — `_p95/_p99` gauges are WINDOWED now. SUPERVISOR-002 goroutine supervisor ensures idempotent writer resumption.

---

## Slide 26 — F3 Section Header

Target: 10-14 slides. 1 full (RRF) + 3 mini (context fingerprint, sparse gate, rerank).

## Slide 27 — RRF What It Is

Phase 13 equal-weights failed (q 69 regression); Phase 13.1 embedding-heavy weights passed +5.9% mean. Cache key includes `scorerVersion()` so weight changes flip namespace automatically. **PM-fallback**: "4 different ways to look up memories, fused into one ranked list."

## Slide 28 — Why 4 Columns

Each column catches a different failure mode. RRF fusion = `1 / (k + rank)` — robust to score-scale differences across columns. The RRF-SCALE-001 arch rule: never gate on absolute `Score`.

## Slide 29 — How It Works Diagram

Rerank default via cross-encoder (openai). Neural alternative shipped but same mean quality, 20× slower. RETRIEVAL-REVERSE-LOOKUP-001 adds filesystem-grep post-rerank for reverse-lookup-shaped queries.

## Slide 30 — RRF Score-Scale Contract

Downstream consumers MUST NOT hardcode absolute thresholds against `RetrieveResult.Score`. **Burn this rule in**. Scorer changes silently break calibrated gates — audit every `Score` comparison on scorer change.

## Slide 31 — Why Important

One pipeline, many consumers. Changes propagate everywhere. SLO recalibrated post-ALERT-TRUTH-001 to match honest p95 (was clamped at 10s pre-fix).

## Slide 32 — How It Will Improve

Pattern: ship mechanism default-off, let 7d of A/B data decide. NEURAL-RERANK-PRECHECK-001 live-caught neural rerank is 20× faster at same quality — kept default openai (operator opt-in).

## Slide 33 — Context Fingerprinting

5th RRF column; weight-tunable. Default-on since Phase 14.2.3. Depth: `docs/features/context-fingerprinting.md`. **PM-fallback**: "A compact signature of 'where this memory came from in the codebase' — distinguishes same-memory-in-different-context."

## Slide 34 — Sparse Retrieval Gate

~25% rerank-input reduction on most calls. 120q UVTS A/B: mean +0.003, 0 regressions, 10 improvements → safe default-on. `V0019 sparse_gate_metrics` hypertable persists per-call rows.

## Slide 35 — Rerank

Length-mismatch counter + corrective retry — never silent-default (RERANK-LENGTH-STRICT-001). Determinism required (`rerankTemperature=0.0`) + pre-check budget guard. Operator opt-in via `.env` RERANK_PROVIDER=neural if latency matters more than consistency.

---

## Slide 36 — F4 Section Header

Target: 20-26 slides. **The headline section.** Carries the deck's weight — Jiminy is the primary user-facing cognitive surface. 2 full + 1 mini. Slow down.

## Slide 37 — Jiminy What It Is

Think "inner voice" — not "linter," not "lookup." Internal-dialogue analogy from F1.2.2 lands here concretely. **PM-fallback**: "A smart assistant that watches what the AI is about to do and warns if it conflicts with saved rules."

## Slide 38 — Why Use Jiminy

Rejected: inline "assistant memory" via prompt stuffing (agent attention is finite, dilution). Rejected: pure retrieval (no priority signal). Jiminy adds priority + feedback loop.

## Slide 39 — Jiminy Flow

3 Levers compose on the surface pool. Narrative synthesis runs 50s+ (local LLM); background via /warm so hook stays snappy.

## Slide 40 — Four Levers

Lever C moved the needle most (actionable fraction 11.1% → 47.7% in controlled A/B). Lever D is primitives-shipped for substrate-native concept surfacing; data-decided enablement.

## Slide 41 — Classifier Semantics

Each credit clause splices after predecessor; recency-weighted LLM attention gives strongest-gate-last. CLASSIFIER-CONTEXT-002 shipped mechanism-scope gate (hard-precedence). JIMINY-CLASSIFIER-META-SCOPE-001 shipped mention-vs-perform — ship-dormant until smoke data justifies flip.

## Slide 42 — Why Important Follow Rate

Trajectory language — do NOT normalize low follow rate as "by design" (`trust-signal-must-be-persisted-never-ignore-honest` rule). Current ~13% is honest baseline; arc target ≥80%; closes the delta. **Pause here**: "The follow rate is the honest answer to 'is this working?'"

## Slide 43 — JIMINY-CEILING-BREAK-2 5-Phase Arc

Each phase is measurable + reversible. Phase 4b retrain gated on data — don't retrain until corpus justifies it. Composite ceiling arithmetic hits 77-80%+.

## Slide 44 — F4.2 Enforcement Arc Overview

Shipped as sequential arc: each sprint lives against the next scoping, informed by live smoke of predecessor. Three arc-adjacent follow-ups shipped. 13 arch rules pinned in CLAUDE.md.

## Slide 45 — Deny Taxonomy

3 distinct outcome types; each has distinct alert rule. missed_violation path is post-hoc (correction observation → embedding match → outcome record). blocked_false_positive feeding `archive_ineffective_constraints` is substrate mutation (ENFORCE-AUTO-EXECUTE 5-layer guards).

## Slide 46 — Operator Escape Hatch

3 arch rules pinned: time-boxed; reason-required; session-scoped (never global). Audit trail is forensic, not load-bearing.

## Slide 47 — Auto-Execute Default-Off

`Dispatcher.cfg` reads happen at dispatch time, NOT constructor time — flag-flip-via-env+restart; never memoize-at-constructor trap. `adjust_guidance_confidence` NOT yet auto-executed (needs HITL — scalar mutation requires pre-change snapshot).

## Slide 48 — Mode Selector

Operator directive 2026-08-02: user-facing config toggle MUST be reachable from UI, not just env + CLI.

## Slide 49 — J17 Protocol What It Is

The "init" header fires once per session; subsequent messages are compact rows. Compression target ratio 2.0 (calibrated from live 30d p95 — DASHBOARD-TRUTH-002 recalibration).

## Slide 50 — J17 Tier Promotion

First T1 messaging ever produced on substrate after this flip (6 pipe-coded `C:!|code` lines). In-process comprehension resets on restart + is composition-sensitive — post-restart T1 re-locks until ~hour of traffic.

## Slide 51 — Agent Briefing Packet v1.1

Peer-Opus-5.5 review surfaced v1.0 compliance-shape; v1.1 reframed as data-not-commands. Content-hash pin via `internal/j17pin/briefing_packet_test.go` prevents silent drift.

---

## Slide 52 — F5 Section Header

Target: 12-16 slides. 1 full (LoRA on Qwen) + 2 mini (recursive loop, homebrew dist).

## Slide 53 — LoRA What It Is

Phase 5 SFT replaced planned MoE-Sieve two-tier strategy (2026-04-22 pivot). Rejected: Qwen3.6-35B-A3B MoE (Metal 499K MTLResource ceiling blocked backward pass).

## Slide 54 — Why Fine-Tune Our Own

16 call sites: ape.reflect, jiminy.classify, jiminy.synthesize, consulting.classify, retrieval.rerank_cross, retrieval.intent_translate, guardrail.evaluate, hidden.summarize, hidden.reclassify, emergence.name, context_cooler.stability, retrieval.consensus, j17.synthesize, j17.classify, agent.pre_write_check, data.curate.

## Slide 55 — LoRA Pipeline

MLX stays as research/training canonical; GGUF is production serving form. Phase 13.5 cutover 2026-05-03: mlx_lm.server (14-min Metal OOM) → llama-server (bounded KV).

## Slide 56 — Standing Policies

Both policies grep-auditable + CI-gated. 2-epoch overfit incident is the forcing function pinning the 3-epoch cap.

## Slide 57 — Why Important 0.9188

Honest number vs. 0.8389 pre-refresh. Eval rows had been measuring OLD KV-unbounded serving behavior. When production serving changes, audit pinned evals for rows embodying OLD serving.

## Slide 58 — How Will Improve

E4 promote unblocked once Phase A+C ship (operator-authored external work). Follow-up HOMEBREW-INSTALLER-QWEN-UPDATE-002: wire real v2 manifest with SHAs, extend LoadQuantManifest to pick by cfg.ModelName.

## Slide 59 — Recursive Loop Diagram

4-phase arc complete (Phases 6a/6b/7/9). All default-off (`FT_LOOP_ENABLED=false`). Each phase shipped observability → actuator → promotion defense-in-depth → drift detection.

## Slide 60 — Why Default-Off

Single-flight invariant: ledger is source of truth for "is cycle running," NOT RSIC 300s in-memory cooldown. Operator-confirms by design: no auto-promote without explicit operator attestation.

## Slide 61 — Phase 7 Defense in Depth

Every layer drilled live same-day. Class-5 issue filer: fingerprint-idempotent `gh` + CapabilityGap. `ft_model_versions` revived.

## Slide 62 — Model Dist CLI

Ollama is DISTRIBUTION only. Runtime stays llama-server (Phase 13.5). Adapter-only path (MODEL-DIST-002): 257 MB GGUF LoRA, `--adapter` flag.

## Slide 63 — How Pull Works

All 11 operator-visible values dynamic (no-hardcoding rule). Air-gapped: override via `MDEMG_MODEL_MANIFEST_PATH`.

## Slide 64 — Why Not Alternatives

Data-decided. Each rejected option had quantifiable disqualifier. Rollback: restore `com.mdemg.mlx-server.plist` from `.disabled-phase13_5` — would reintroduce 14-min crash cycle.

---

## Slide 65 — F6 Section Header

Target: 7-10 slides. 1 full (UxTS forcing-function family). The pattern that keeps the system honest.

## Slide 66 — UxTS What It Is

16 frameworks. New frameworks added when new drift class discovered. UXTS_FRAMEWORK_MATRIX.md is single-source-of-truth for count — add a spec, update matrix count in same PR.

## Slide 67 — Why Use UxTS

"Measure, lock, repeat" — the deck's governing engineering value lives here. A new drift class that CI isn't gating on is incomplete work. Define the forcing function.

## Slide 68 — Pattern

Pattern identical across 16 frameworks; only drift class changes. `make test-api` runs UATS; `make test-uvts-quick` runs UVTS.

## Slide 69 — Framework Matrix Snapshot

Each row is a shipped fix of a specific drift class. The matrix is the forcing function on the forcing functions.

## Slide 70 — Why Important

CI gating means a class never re-opens once drift is spec'd. Zombie checks (continue-on-error) are the enemy — un-zombie them (UXTS-CI-001 arch rule).

## Slide 71 — How Will Improve

When a new sprint introduces drift-prone surface, sprint plan MUST include UxTS forcing function. Non-negotiable.

---

## Slide 72 — F7 Section Header

Target: 14-18 slides. 2 full (RSIC + alert-correctness; HITL + curation) + 2 mini (backup + Grafana).

## Slide 73 — RSIC What It Is

7 dimensions, each config-weighted (RSIC_HEALTH_WEIGHT_*). Dimensions without data (confidence=0) excluded automatically, not penalised.

## Slide 74 — Why Use RSIC

Dashboard-truth family has now shown 4 times: ~75-90% of operator-flagged "low" metrics are artifacts. Latest: MDEMG-ALERT-NOISE-REDUCTION-001 (2026-10-22) closed 2 chronic false-fire classes.

## Slide 75 — Alert Correctness Contracts

Rule: trajectory language, never "by design" language on sub-50% follow rate. When dashboard reads scary but direct query returns healthy: triage before fixing.

## Slide 76 — Why Important

Each sprint in the family adds a shared contract. The next drift class to catch has the pattern available.

## Slide 77 — How Will Improve

The honest-steady-state-first-then-alert-floor-below rule generalizes to every new alert.

## Slide 78 — HITL What It Is

Dataset-agnostic — a new dataset is an interface impl, not a platform change. 17 datasets registered with zero platform edits.

## Slide 79 — Why Use HITL

REVIEW-SUGGESTED-GUIDANCE-CONSUME-001 shipped the consumer: `scripts/curate_guidance_corpus.py` LEFT JOINs with length-gate. SME writes propagate to retrain corpus.

## Slide 80 — How HITL Works

Reinforcement is OPT-IN per submission (`REVIEW_REINFORCE_DEFAULT=false`). Idempotent per `(dataset_id, item_id, rubric_version)`. Reversible via `/v1/review/reverse`.

## Slide 81 — Keyboard-Driven Bulk Grade

UI corpus-growth features MUST unlock keyboard-only flow (arch rule). Auto-advance MUST have visible cancel window (arch rule).

## Slide 82 — Auto-Grade Schedule

SOLE HITL dataset refusing auto-grader: `human_class_queue` — LLM cannot verify by construction.

## Slide 83 — Why Important Corpus

Retrain pipeline consumes `guidance_training_rows`; `constraint_outcomes` is telemetry-only.

## Slide 84 — Backup Dual-Sink

Launchd minimal PATH gotcha → `MDEMG_DOCKER_BIN` override + well-known docker locations. 3 scheduled Go jobs (TSDB backup, maintenance, export-auto) all route through jobhealth.Report.

## Slide 85 — Restore Correctness

Restore path's correctness IS backup system's correctness. Scratch-space drills pay full partial-backup cost (known limitation).

## Slide 86 — Supervisor

New background loops MUST route through `goSupervised` or `SetSupervise` — never bare `go func()`. RSIC recency gate: `llm_error_rate_spike` requires newest error within 60 min.

## Slide 87 — Grafana

When editing any file under `deploy/docker/grafana/`, run `make sync-grafana-embed` in same commit. JIMINY-RULES-UI-001 pinned dashboard-contrast contract: all inline styles use Catppuccin vars, never hardcoded light-theme colors.

---

## Slide 88 — F8 Section Header

Target: 6-8 slides. 2 mini + mentions.

## Slide 89 — Hooks

Hook stdin contract (HOOKWIRE-001): `prompt` not `user_prompt`; `tool_response` not `tool_output`; always normalize via `_response_text`. PostToolUse fires only on SUCCESSFUL tool completion — non-zero-exit commands unobserved.

## Slide 90 — Hook Channel Health

Compose binds `${MDEMG_BIND_ADDR:-127.0.0.1}`. Neural sidecar on `127.0.0.1:8100` (not 8101 — mlx_lm.server decommissioned).

## Slide 91 — MCP

Contract suite at `internal/cli/mcp_contract_test.go` — new tools MUST add contract. Plugin orphans: `kickstart -k` skips `Manager.Stop`; `startModuleInstance` pgrep-reaps prior-generation processes.

## Slide 92 — CLI Command Surface

Every command accepts `--help` + `-h` + flags are env-var-overridable. Operator-authorized destructive ops require `--yes` (teardown + model remove + concepts repair).

## Slide 93 — Multi-Instance

Hostname-as-identity is fragile — pin derived values at first-use is the arch rule. Best-effort writes to operator-owned paths MUST use atomic tmp+rename + fail-open WARN.

---

## Slide 94 — F9 Section Header

Target: 4-6 slides. 2 mini + mentions.

## Slide 95 — Beta Corpus Loop

Receiver-side re-scrub is REQUIRED, not optional — exporter's write-time scrub can be bypassed by hand-crafted bundle.

## Slide 96 — Beta Attribution Invariant

Import-time invariant enforced in `importGuidanceRows`. Stable prefix-based keying gives janitor safe 30-day retention sweeps.

## Slide 97 — Security Hardening

`#nosec` is gosec-only; CodeQL ignores it. CodeQL alerts must be closed via recognized sanitizer OR dismissal via GitHub API.

## Slide 98 — Dormant Census

Verifier scripts: `scripts/verify_route_consumers.py`, `verify_tsdb_consumers.py`, `verify_metrics_consumers.py`. Pre-commit hook gate (PRE-COMMIT-INVENTORY-GATE-001) blocks `git commit` when trigger-file staged without paired adjudication.

---

## Slide 99 — F10 Section Header

Target: 6-8 slides. Operator-authored. **Slow down here** — invite questions, land long-term vision.

## Slide 100 — Where We Are Today

Honest is the only language the architectural directive accepts. The 2026-08-01 directive supersedes any prior "by design" framing on sub-50% follow rate.

## Slide 101 — Shipped vs Open

Right column IS the pin-list for the next 6-12 months. Each has a named sprint-line in `docs/development/`.

## Slide 102 — End Goal Vision

"Earns its persistence" — if substrate isn't followed, it's just a database. Connection-layer bet: better decisions through better connections = cognitive substrate at utility threshold.

## Slide 103 — Why Important Tie-Back

Call back to F1.2.2 — internal-dialogue analogy. If someone's convinced of vision but skeptical of current state: that's honest; state will shift.

## Slide 104 — Continuous Improvement

Substrate is a single surface; loops write to it + read from it. Operator-in-the-loop is explicit — this isn't fully autonomous system. The autonomy knob is HITL-approvals-based (FT_LOOP_AUTO_PROMOTE_AFTER).

## Slide 105 — Where To Go Next

CLI reference comprehensive; `--help` on every subcommand. Dashboards + /ui are read-only unless strict mode flipped.

## Slide 106 — Thank You + Q&A

**Longest Q&A anchor** in deck. If specific concern or arc wasn't covered in depth, that's deliberate — reference feature doc + follow up offline.

---

## Simplify-for-PM Phrasings (Quick Reference)

If a slide gets a "wait, what?" from a non-engineer in the room:

| Jargon | PM-ready phrasing |
|---|---|
| RRF | "4 different ways to find memories, blended into one ranked list" |
| Hebbian | "Memories that fire together, wire together — strengthen the connection" |
| Jiminy | "An inner voice that warns the AI before it violates a saved rule" |
| LoRA | "Fine-tuned our own version of a general model for our specific tasks" |
| UxTS | "A pattern: when we catch a bug, we make a test that locks it closed forever" |
| CMS | "The main memory graph — Neo4j database holding every memory" |
| TSDB | "Time-series database for metrics, outcomes, and training data" |
| CODEQL | "Static analysis that finds security bugs; we gate CI on its approval" |
| RSIC | "The system watches itself + proposes improvements" |

## Quarterly Refresh Checklist

Operator or delegate re-walks before each quarterly refresh:
- [ ] CLAUDE.md pin count (grep -c '^- \*\*' CLAUDE.md)
- [ ] UxTS framework count (vs. UXTS_FRAMEWORK_MATRIX.md)
- [ ] Ingested language count (vs. `docs/lang-parser/lang-parse-spec/upts/specs/*.upts.json`)
- [ ] Current honest aggregate benchmark (TSDB `benchmark_runs` MAX(completed_at))
- [ ] Current follow rate (TSDB `constraint_outcomes` 24h window)
- [ ] Open-arc list in F10.2 (what shipped since last refresh)
- [ ] "Last verified" footers on every section-header slide

---

## Documents Accessed

- `mdemg-onboarding-deck.md` (sibling, this companion)
- `../development/mdemg-onboarding-deck-001/presentation_spec.md`
- `../development/mdemg-onboarding-deck-001/global_roadmap.md`
- `../development/mdemg-onboarding-deck-001/roadmaps/F1-opening.md`
- `../development/mdemg-onboarding-deck-001/roadmaps/F2-F10-roadmaps.md`
