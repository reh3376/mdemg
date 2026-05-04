# Sprint POST-FT-LORA-PHASE13.5 — MLX Server Stability Investigation + Hardening

> **DRAFT — pending user approval.** Research-first sprint with decision forks. Plan freezes once research outcomes are known and the stay/migrate/hybrid fork is resolved (end of Epic 1).
>
> **Operator constraint (2026-05-02):** *"I want to follow the data and confirmed solutions. I do not want to make decisions about direction until we have the FULL research data to inform a reliable and real world stable solution."* — Epic 0 synthesis is the ONLY source of fork-pick evidence. No provisional direction picks. No priors-based recommendations. Every Epic-1 decision must cite evidence from Streams 1–4.

## Context

**Why this sprint, why now.** During the Phase 13 sprint (`reh3376_dev01`, commits `6efdcdc..1b53d1f`, 2026-05-02), `mlx_lm.server` (PID-rotating) crashed **6 times in 86 minutes** with `SIGABRT` on `MTL::Command` (Metal command-buffer error). The Phase 11.6.3 watchdog + launchd `KeepAlive` recovered every crash within ~60s — the framework stayed up — but a 1-crash-per-14-minutes rate is not a stable substrate for production memory.

The watchdog was designed to *recover from* OOMs, not *be the answer to* OOMs. Phase 13.5 promotes the underlying instability from "watchdog handles it" to a real engineering problem.

**Open architectural question.** mlx-lm is one of several Apple-Silicon LLM runtimes (mlx-lm, llama.cpp Metal, Ollama, MLC-LLM, LM Studio backend). Phase 5 chose mlx-lm before this crash pattern was visible. Phase 13.5 reopens that choice with current evidence.

**Framework dependency surface.** All 16 mdemg LLM call sites route through `cfg.EffectiveLLMEndpoint()` (default `http://127.0.0.1:8101/v1`, OpenAI-compatible). The runtime swap surface is the *server process* — not the calling code — so a backend migration is bounded to (a) replace the server, (b) point `LLM_ENDPOINT` at it, (c) verify chat-completions parity.

**Phase chain.** Phase 11.6.3 (watchdog) → Phase 12 (UVTS) → Phase 13 (column voting, FAILED A/B — separate issue) → **Phase 13.5 (this — interrupts Phase 13.1 ablation)** → Phase 13.1 (column-weight ablation) → Phase 14 (Notes 05+06).

**Why interrupt Phase 13.1.** Phase 13.1 needs sustained A/B runs against a stable mlx server to be meaningful. With current crash rate, every A/B is contaminated by mid-run kIOGPU events. Stability first, then ablation.

---

## 1. Header & Metadata

| Field | Value |
|---|---|
| Sprint ID | POST-FT-LORA-PHASE13.5 |
| Title | MLX Server Stability — Research + Hardening (or migration if research warrants) |
| Date | 2026-05-02 (plan) |
| Branch | `reh3376_dev01` |
| Predecessor | Phase 13 (commit `1b53d1f`) |
| Successor | Phase 13.1 — Column-weight ablation (BLOCKED on this sprint) |
| Type | Research-first; HOTFIX-class (production stability); decision-fork sprint (stay/migrate/hybrid) |
| Risk | HIGH (entire framework depends on the LLM endpoint; backend migration would touch packaging + launchd + .env defaults) |
| Budget | $0–10 OpenAI (research only; stress-tests can run against local backends without OpenAI). Up to ~$10 if a small UVTS A/B is run on a candidate alternative. ~12 hr local compute (stress runs across 2–3 candidate backends) |
| Effort estimate | **REVISED post-synthesis: 10–18 dev-days.** Bake-off is real engineering. Floor 10 days (single-finalist passes B2+B3+B4 cleanly). Ceiling 18 days (multi-finalist iteration, model-conversion debugging, OS-level workarounds). |
| New TSDB migration | None expected. **Optional V0018** if a `mlx_crash_audit` table is added (recommend NO — Prometheus + diagnostic reports are sufficient). |
| Post-sprint artifacts | `docs/development/post-ft-lora/phase_13_5_mlx_research_synthesis.md` (research report), `phase_13_5_mlx_stability_post.md` (executed truth), code changes per fork outcome (config tunables OR backend swap), updated `CLAUDE.md` Architecture Notes "MLX Watchdog" section |

## 2. Problem Statement

**Symptom (concrete):**
- 6× `Python-2026-05-02-*.ips` crash reports between 11:41 and 13:07 (rate: ~1 / 14 min under typical mdemg load)
- All crashes share signature: `SIGABRT` raised in `mlx::core::gpu::check_error(MTL::CommandBuffer*)` (libmlx.dylib), dispatched on `com.Metal.CompletionQueueDispatch`
- Coalition: `com.mdemg.mlx-server` (the launchd-managed plist installed by Phase 11.6.3.1)
- mlx-lm version: **0.31.2** (latest published: 0.31.3, released 2026-04-22 — ONE patch behind)
- Hardware: Mac17,6 (M5 Max, 128 GB unified memory), macOS 26.3.2 (build 25D2150)
- Model: `qwen3-14b-mdemg-v1` — 14B parameters, 4-bit quantized, 8.4 GB on disk

**Root-cause hypotheses (not yet validated — Epic 0 will rank):**
1. **Metal driver memory accounting bug under macOS 26.3.2 + M5 Max.** macOS 26 is recent; the M5 Max GPU driver paired with this OS may have a regression. Evidence: identical crash signature across all 6, no obvious per-call distinguishing factor.
2. **mlx-lm 0.31.2 fixed-in-0.31.3 issue.** A single-patch lag could be the entire cause. Cheap to test (one upgrade).
3. **Concurrent prompt + KV-cache pressure exceeding wired-memory budget.** Current plist runs `--prompt-concurrency 2 --decode-concurrency 2 --prompt-cache-size 256`, but mdemg fans 16 LLM call sites; if any pair lands simultaneously with full-context prompts, peak working set may exceed Metal's safe bound on a 128 GB system.
4. **Inadequate prompt-cache eviction + long-running drift.** mlx_lm.server's KV cache accumulates state; long-running processes may leak Metal allocations the GC doesn't reclaim.
5. **MoE / activation-quantization pathological path.** Even though the production model is dense Qwen3-14B (not MoE), a specific token sequence could trigger an MTL command that exceeds the device's command-buffer arena.

**Architectural question:**
- Is mlx-lm the right local-inference backend in 2026-05? Alternatives: **llama.cpp Metal** (mature, GGUF, very stable), **Ollama** (llama.cpp wrapper + OpenAI-compatible server out of the box), **MLC-LLM** (TVM Metal, less common), **LM Studio inference server** (UI-coupled).

**Constraints on the answer:**
- mdemg framework is hard-tied to **OpenAI-compatible chat-completions API** at `LLM_ENDPOINT`. Any candidate must speak this protocol.
- Production model is `qwen3-14b-mdemg-v1` — **MLX format** (not GGUF). Migration to llama.cpp/Ollama requires **GGUF re-quantization** of the same weights (or accepting a different-but-equivalent quant). This is a 1-day re-quantization task, not a re-training task.
- Phase 5 + Run 7 + Stage-1 distill + RL artifacts are all in MLX format. Migration must convert OR keep mlx as a research backend while moving production to a stabler runtime.

## 3. Scope & Constraints

**In scope (Epic 0 research deliverables):**
1. **Internal evidence audit** — symbolicate one crash report, fingerprint all 6 to confirm identical fault, correlate timestamps with mdemg LLM call rate / token sizes / call sites.
2. **External research** — web search across:
   - mlx-lm GitHub issues (`MTL::Command`, `kIOGPUCommandBufferCallback`, "command buffer" OOM, "metal" SIGABRT)
   - macOS 26.x + Metal regression reports
   - mlx framework GitHub Discussions for long-running server stability
   - Apple Developer Forum reports on Metal command-buffer errors
3. **Alternatives matrix** — for each of {mlx-lm tuned, llama.cpp Metal, Ollama, MLC-LLM}: stability evidence, OpenAI-API compatibility, model-format requirements, max-context behavior, performance profile (tok/s on similar hardware), packaging implications for mdemg's launchd plist.
4. **Decision** — by end of Epic 1, pick one of: **(A) Stay with mlx-lm + tune** (low risk, may not fix), **(B) Hybrid — keep mlx for research, route production through alternative** (medium risk, dual maintenance), **(C) Migrate production runtime entirely** (higher risk, single maintenance).

**In scope (Epic 2+ deliverables — conditional on Epic 1 fork):**

| Fork | Likely scope |
|---|---|
| **(A) Stay + tune** | Upgrade to mlx-lm 0.31.3; per-flag stress sweep across `--prompt-cache-size {0, 128, 256, 1024}`, concurrency `{1×1, 2×2, 4×4}`, env vars `MLX_METAL_*`; investigate prompt-cache TTL / periodic restart cadence (process recycle every N requests via plist `KeepAlive` + a self-shutdown signal); add Prometheus crash-counter; plist hardening (e.g. `LimitLoadToSessionType`, `Nice`, `MemoryLimit` if relevant) |
| **(B) Hybrid** | Convert production model to GGUF (`qwen3-14b-mdemg-v1.gguf` Q4_K_M); deploy llama.cpp Metal server OR Ollama on `:8102`; add `LLM_ENDPOINT_PRODUCTION` config; route 12 of 16 production call sites to alternative, keep 4 research/dev call sites on mlx; new launchd plist; A/B parity check on UVTS quick profile |
| **(C) Migrate** | All of (B) but route ALL 16 call sites; remove mlx watchdog (or repurpose for the new backend); update CLAUDE.md to remove mlx-mandatory policy; update preflight check; update `mdemg service install`; archive mlx artifacts under `archive/mlx-runtime/` |

**Out of scope (deferred):**
- Re-training. Production model identity stays `qwen3-14b-mdemg-v1`. Migration is *runtime-only*.
- Phase 13.1 column-weight ablation (BLOCKED on this sprint; resumes once stability baseline is established).
- Phase 14 (Notes 05+06).
- New launchd-style supervisor frameworks (e.g. systemd-on-Mac, custom Go process supervisor) — launchd `KeepAlive` is the standard; we tune within it.
- Cloud-LLM-only fallback as a permanent solution. OpenAI fallback exists for emergencies but is not the production substrate.

**Constraints (hard):**
- **MEMORY: live testing is REQUIRED** — Tier 3 stress test must run actual mdemg → actual server → real prompts. No mocked stability claims.
- **MEMORY: no hardcoded values** — every tunable goes through config.
- **MEMORY: sequential epics** — research before implementation; can't ablate before the fork is decided.
- **MEMORY: plan-options pattern** — fork (A)/(B)/(C) is the central decision, recorded in §13.
- **MEMORY: single batched commit at sprint close**.
- **MEMORY: sprint summary on PR comments**.
- **MEMORY: CUIDv2 for any new IDs.**
- **MEMORY: max_tokens ≥ 3000, latency_budget_ms ≥ 15000** — stays floor; not relaxed for stress tests.
- **Backward-compat** — any backend swap must keep existing OpenAI-compat call sites unchanged. Code surface change = config + packaging only.
- **Crash-rate target (acceptance bar)** — under sustained mdemg load (≥1 LLM call/min averaged over 4 hr), **< 1 mlx server crash per 24 hr.** Current state is ~1 per 14 min; the bar is ~100× improvement. Fork-(A) may not reach this; that's the case for fork (B)/(C).
- **Latency target** — p50 LLM call ≤ 1.2× current observed mlx baseline; p95 ≤ 1.5×. Must measure on stable mlx (post-tuning) and any candidate alternative.

## 4. Dependencies

**Consumed (code, pre-existing — reuse):**
- `internal/llmclient/client.go` — OpenAI-compatible client; baseURL switch via `cfg.EffectiveLLMEndpoint()`
- `internal/cli/preflight_mlx.go` — startup probe (rename / generalize if backend swaps)
- `internal/cli/watchdog.go` — Phase 11.6.3 watchdog (rebind to new backend if swap)
- `internal/cli/service_darwin.go` — plist generator (parameterize backend)
- `internal/config/config.go` — env-loading; `MLX_*` knobs (rename to `LLM_BACKEND_*` if swap)
- `docs/tests/uvts/runners/uvts_runner.py` + `uvts_ab_compare.py` — used to verify candidate-backend retrieval-quality parity

**Consumed (data):**
- 6 crash reports under `~/Library/Logs/DiagnosticReports/Python-2026-05-02-*.ips` — primary evidence
- `~/.mdemg/logs/mlx-server.{out,err}.log` — server stdout/stderr at crash time
- TSDB `llm_interactions` rows from 2026-05-02 — LLM call rate / size profile during the crash window
- Production model files at `/Users/reh3376/mdemg/.local-models/mdemg-llm-v1/` (8.4 GB)

**Consumed (compute):**
- Local M5 Max for stress tests
- OpenAI API for any UVTS A/B against a candidate (small, ~$5)

**External services:**
- GitHub (mlx-lm issues, llama.cpp issues, ollama issues — read-only)
- Apple Developer documentation (Metal best practices, command-buffer behavior)
- Web search for community reports

**No data or training is touched by this sprint.**

## 5. Implementation Plan (Sequential Epics + Gates)

**Pre-gate:** branch `reh3376_dev01` clean post-Phase-13 (commit `1b53d1f`); no stale Phase-13 work-in-progress on disk.

### Epic 0 — Research (4 parallel research streams, then synthesis)

> **This is the longest non-coding epic in any post-FT-LORA sprint to date. Do not skip.** Ships a written synthesis with cited evidence.

Four parallel research agents (using the Agent tool with `subagent_type: general-purpose`) cover distinct domains. Agents run in parallel; orchestrator (this conversation) synthesizes their reports.

**Stream 1 — Crash forensics (internal, no web)**
- Symbolicate `Python-2026-05-02-114111.ips` to recover the full backtrace into mlx_lm Python code
- Diff fault offsets across all 6 crash reports — identical address means deterministic OOM at fixed code path; varying means memory pressure peaks
- Cross-reference TSDB `llm_interactions` to find which call site fired in the 30s preceding each crash (suspicion-rank: `consulting.classify`, `jiminy.synthesis`, `rsic.reflect`)
- Cat `~/.mdemg/logs/mlx-server.err.log` for the 30s before each timestamp; capture any mlx warnings ("Metal allocation failed", "command buffer too large")
- Output: `docs/development/post-ft-lora/phase_13_5_crash_forensics.md`

**Stream 2 — mlx-lm + Apple Metal community evidence (web)**
- Search GitHub issues at `https://github.com/ml-explore/mlx-lm` and `https://github.com/ml-explore/mlx` for: `MTL::Command`, `kIOGPUCommandBufferCallback`, "command buffer" OOM, server crash, long-running stability
- Search Apple Developer Forums for `kIOGPUCommandBufferCallbackErrorOutOfMemory` + similar
- Read mlx-lm 0.31.3 release notes (filed 2026-04-22) — what changed since 0.31.2?
- Find any documented mitigations: env vars (`MLX_METAL_*`), recommended flags for production, prompt-cache best practices
- Output: `docs/development/post-ft-lora/phase_13_5_external_evidence.md` with citations

**Stream 3 — Alternatives matrix (web + light local)**
- For each of {**llama.cpp Metal**, **Ollama**, **MLC-LLM**}:
  - OpenAI-API compatibility: native? plugin? (e.g. `llama-server`)
  - Stability evidence in 2026 (recent issues filed for crashes/OOMs?)
  - GGUF / model-format conversion path from MLX
  - Performance profile: tok/s on M-series Mac, recent benchmarks
  - Long-running server posture: process model, memory accounting, recycle/restart story
  - Packaging: brew availability, launchd-friendliness, single-binary vs venv
- Estimate **engineering cost** of swap for each: `LLM_ENDPOINT` change + plist + preflight + watchdog rebind + GGUF re-quant
- Output: `docs/development/post-ft-lora/phase_13_5_alternatives_matrix.md`

**Stream 4 — Internal LLM call profile (local TSDB query)**
- Query TSDB `llm_interactions` for distribution of: tokens-in, tokens-out, latency by task type, calls/min over the past 24 hr
- Identify the top 3 highest-pressure call sites (largest prompts × highest frequency × longest decode)
- Document peak-concurrency ceiling under realistic load: how many LLM calls overlap in a typical 60s window?
- Output: `docs/development/post-ft-lora/phase_13_5_call_profile.md`

**Synthesis (orchestrator, after streams 1–4 complete):**
- Merge findings into `docs/development/post-ft-lora/phase_13_5_mlx_research_synthesis.md`
- Rank the 5 root-cause hypotheses by evidence weight
- For each fork (A/B/C), produce: estimated effort, estimated stability lift, risk profile

**Gate:** synthesis doc committed; 5 hypotheses ranked; alternatives matrix scored; effort estimates per fork attached.

### Epic 1 — Disqualification confirmation + finalist lock (REVISED post-synthesis)

> **Synthesis outcome (2026-05-02):** the data disqualifies fork (A) "stay + tune" because mlx_lm.server is officially not production-grade per its own maintainers (`SERVER.md`, Discussion #371). Ollama is also disqualified (broken on M5 + macOS 26.3.x across 0.20.5–0.22.1, 8+ open issues). LM Studio is disqualified by closed-source operability risk for a cognitive-substrate framework. Original fork (A)/(B)/(C) lattice replaced with a finalist set determined by the synthesis. See `phase_13_5_mlx_research_synthesis.md`.

1. Read `phase_13_5_mlx_research_synthesis.md` end-to-end
2. **Confirm the disqualifications hold** (no late counter-evidence) — sanity-check that no Apple, mlx-lm, or Ollama release in the past 7 days flips one of these calls. Single-pass check, ~1 hour
3. **Locked finalist set** (data-decided in synthesis §10, not operator-input):
   - **F1 — llama.cpp (`llama-server`)** — most production-justified; needs M5 + macOS 26.3.x verification
   - **F2 — MLC-LLM** — most architecturally distinct (TVM Metal kernels, immune to Issue Y); needs M5 evidence
   - vllm-mlx is **dropped**: Stream 1 confirmed our crash is in `libmlx.dylib` itself, and vllm-mlx wraps the same framework — it inherits the failure mode we're escaping
4. **No code yet.** This epic produces only the late-counter-evidence sanity check + Epic 2 detailed plan.

**Gate:** disqualifications confirmed (no last-7-days flips); Epic 2 bake-off plan ready to execute.

### Epic 2 — Empirical bake-off (REVISED — replaces "implementation per fork")

> Per synthesis §8: research data alone cannot rank the finalists on our exact hardware/OS. This epic produces the empirical evidence the fork-pick requires. **Sequential** execution (data-decided in synthesis §10): production runs one backend; parallel candidates contaminate the stress signal with cross-candidate memory contention that doesn't exist in production. **Production OS only** (macOS 26.3.2): no 26.2.x rollback control because the production OS is the only OS we ship on.

For each finalist (F1, F2 — sequential):

**B0 — Model conversion (per-finalist):**
- F1: MLX → f16 safetensors (`mlx_lm.fuse --de-quantize`) → GGUF Q5_K_M (`llama-quantize`)
- F2: HuggingFace base Qwen3-14B → MLC weights → TVM-compile to `.dylib`

**B1 — Install + smoke:** non-conflicting port (8102/8103/8104). Confirm `/v1/chat/completions` returns valid response. Confirm Metal initialized. Confirm one full `ape.reflect`-shaped 5800-token-in / 1000-token-out request returns within latency budget.

**B2 — UVTS quality A/B (quick profile, 16 questions):** baseline = current mlx_lm.server. Per-question regression > 10% disqualifies the candidate (matches Phase 13 bar).

**B3 — Crash-rate stress (4-hour `ape.reflect` cadence simulation):** synthetic load 1 call/~32s, 5800 tokens in, 1000 tokens out. **Acceptance: zero crashes.** This is the hardest gate — the bar mlx_lm.server fails today.

**B4 — Concurrency stress (1-hour, 4 concurrent calls):** matches observed P95 of 3.4. **Acceptance: zero crashes; p95 latency within 1.5× of B3 baseline.**

**Gate:** at least one finalist passes B2 + B3 + B4. Operator picks the winner from passing finalists. Recorded as decision-fork outcome with cited evidence.

### Epic 3 — 24-hour soak + production cutover (REVISED — winner only)

- **24-hour soak** of the Epic 2 winner under realistic mdemg load (real `ape.reflect` cadence, real retrieval pipeline calls). **Acceptance: < 1 crash / 24 hr.**
- **Latency observation:** p50 ≤ 1.2× pre-sprint baseline, p95 ≤ 1.5× pre-sprint baseline
- **Production cutover:** flip `LLM_ENDPOINT` to winner's port; rebind watchdog probe URL; update preflight check; retire mlx_lm.server plist (or keep dormant for emergency fallback)

**Gate:** 24-hour soak verdict captured (pass / fail with detail). On fail: open Phase 13.5b follow-up; Phase 13.1 stays blocked.

### Epic 4 — Documentation (Final Epic — Never Cut)

- `docs/development/post-ft-lora/sprint_plan_phase_13_5_mlx_stability.md` — frozen plan (this file)
- `docs/development/post-ft-lora/phase_13_5_mlx_stability_post.md` — executed truth: synthesis findings, fork choice + rationale, soak result, latency deltas, A/B verdict
- `CLAUDE.md` — Architecture Notes update (rename or extend "MLX Watchdog" section; if fork B/C, document new runtime)
- `AGENT_HANDOFF.md` top entry
- `CHANGELOG.md [Unreleased] ### Changed`
- `SPRINT_ROADMAP_POST_FT_LORA.md` — Phase 13.5 EXECUTED, Phase 13.1 unblocked

**Gate:** all docs landed; cross-refs valid.

## 6. Testing Plan (Three Tiers)

**Tier 1 (Unit) — `go test -race`:**
- New config knobs round-trip in `config_test.go`
- Any backend-switching logic in `client_test.go` (if fork B/C)

**Tier 2 (Integration) — `go test -tags=integration`:**
- `tests/integration/llm_endpoint_swap_test.go` (if fork B/C) — confirms `LLM_ENDPOINT` env override correctly retargets all 16 call sites
- `tests/integration/preflight_mlx_test.go` — extended with new backend (fork B/C)

**Tier 3 (Live E2E) — MANDATORY:**
- **24-hr soak** OR **4-hr accelerated stress** (10 calls/min sustained, mix of long-prompt + short-prompt, includes one `consulting.classify`-shaped call every minute)
- Real binary, real backend, real mdemg → real Neo4j → real OpenAI grader
- Observed via Prometheus + DiagnosticReports inspection
- Verdict captured in post-doc

**State restoration (MEMORY):** all changes additive or config-flagged. Rollback = `git revert <final commit>` + restore `.env` `LLM_ENDPOINT` + reload original launchd plist (preserved at `~/.mdemg/backup/`).

**Gate:** all 3 tiers green; soak verdict captured.

## 7. Commit Strategy

Single batched commit at sprint close (MEMORY):
- Title: `fix(infra): Sprint POST-FT-LORA-PHASE13.5 — MLX server stability (fork: <A|B|C>)`
- Body: synthesis summary, fork choice + rationale, soak crash count, p50/p95 deltas, A/B verdict (if run), policy compliance checklist
- Footer: `Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>`
- Push → auto-PR → sprint summary comment posted to PR (MEMORY)

## 8. Verification Checklist

- [ ] Epic 0 — 4 research streams complete, synthesis doc committed
- [ ] Epic 1 — fork (A/B/C) recommended + user-approved; plan amended in-place with concrete Epic 2
- [ ] Epic 2 — implementation per chosen fork; lint clean; unit + integration tests green
- [ ] Epic 3 — Tier 3 soak captured (crash rate < 1 / 24 hr; latency within bounds; A/B parity if run)
- [ ] Epic 4 — sprint plan + post + ROADMAP + AGENT_HANDOFF + CHANGELOG + CLAUDE.md
- [ ] Commit pushed; auto-PR updated; sprint summary posted to PR
- [ ] All forks/options disclosed in commit body + PR comment
- [ ] `golangci-lint run ./...` clean
- [ ] CI green
- [ ] CMS observation captured: "Phase 13.5 MLX stability — fork <X>; crash rate dropped from 1/14min to <Y>/24hr."

## 9. Documentation Update (Final Epic — Never Cut)

Covered by Epic 4. Specifically:
- New section in `CLAUDE.md` (Architecture Notes): "Local LLM Runtime" (renamed from "MLX Watchdog" if fork B/C)
- Operator-facing recipe for switching backends in an emergency
- Documented crash-rate target (<1/24hr) as the production bar going forward

## 10. Risks & Mitigations

| # | Risk | Likelihood | Mitigation | Fallback |
|---|---|---|---|---|
| 1 | **Research is inconclusive** — no clear root cause emerges | Medium | Even without a definitive root cause, the alternatives matrix gives an actionable comparison; default to fork (A) tuning + escalate to (B) if 24-hr soak fails | Ship fork (A) + open Phase 13.6 follow-up if soak fails |
| 2 | **Fork (B/C) discovers GGUF re-quant degrades model quality** | Medium | UVTS A/B catches this; a >10% quality drop blocks the fork; revert to mlx + iterate on tuning | Stay on mlx + accept watchdog-mediated stability |
| 3 | **Hidden mlx-lm 0.31.3 breaking change** | Low | Test in isolated venv before swapping production venv | Pin to 0.31.2 + apply tuning patches only |
| 4 | **macOS 26.x Metal regression that no flag fixes** | Medium | Documented in research output; if confirmed, fork (B/C) becomes the only viable answer | Pre-warn user that this is the most likely "must migrate" case |
| 5 | **Backend swap inflates packaging surface** (homebrew tap + Docker compose + launchd plist all need updates) | High (for B/C) | Plan keeps the swap surgical: `LLM_ENDPOINT` config + ONE new plist + ONE preflight branch | Stage B/C across 2 sprints if scope balloons |
| 6 | **Soak test contention with operator's actual work** — 24-hr soak monopolizes mlx | Medium | Schedule soak overnight; provide operator a `--soak-suspend` toggle | Accelerate to 4-hr stress test instead |
| 7 | **Phase 13.1 ablation gets further delayed** | High (this sprint blocks it) | Phase 13.5 is shorter than Phase 13.1; sequential is correct order | Phase 13.1 becomes Phase 13.5b |
| 8 | **Watchdog interaction with new backend mis-fires** (fork B/C) | Medium | Generalize watchdog to backend-agnostic; reuse the up→degraded→down state machine; new probe URL per backend | Disable watchdog during initial swap soak; re-enable after baseline established |
| 9 | **Diagnostic-report symbolication fails** (no debug symbols in mlx-lm wheel) | Medium | Use `atos` against the Mach-O dyld; if that fails, work from raw addresses + mlx-lm source-code grep | Skip Stream 1 forensics depth; proceed with hypothesis ranking from external evidence only |

## 11. Documents Accessed (during planning)

**Internal (this conversation + filesystem audit):**
- `~/Library/Logs/DiagnosticReports/Python-2026-05-02-{114111,120823,121921,123004,124719,130743}.ips` — 6 crash reports
- `~/Library/LaunchAgents/com.mdemg.mlx-server.plist` — current launchd plist
- `/Users/reh3376/mdemg/.local-models/mdemg-llm-v1/` — production model directory
- `/Users/reh3376/mdemg/CLAUDE.md` — MLX Watchdog section + framework runtime requirements
- `/Users/reh3376/mdemg/docs/development/post-ft-lora/phase_13_column_voting_post.md` — Phase 13 outcome
- `/Users/reh3376/mdemg/AGENT_HANDOFF.md` — current state
- `/Users/reh3376/mdemg/internal/cli/{preflight_mlx.go, watchdog.go, service_darwin.go}` — runtime touch points
- mlx-lm version: 0.31.2 installed; 0.31.3 latest on PyPI (released 2026-04-22)
- pgrep + launchctl + sysctl + sw_vers output (M5 Max, 128 GB, macOS 26.3.2 build 25D2150)

**External (planned during Epic 0 — not yet accessed at plan time):**
- mlx-lm GitHub: `https://github.com/ml-explore/mlx-lm/issues`
- mlx GitHub: `https://github.com/ml-explore/mlx/issues`
- Apple Developer Forums (Metal-related)
- llama.cpp / Ollama / MLC-LLM project docs + recent issues
- Apple "Metal Best Practices" + command-buffer documentation

## 12. Rollback

All work is additive or config-flagged.

1. **Code rollback**: `git revert <final commit SHA>` removes synthesis docs + any code changes + plist changes
2. **Runtime rollback** (no rebuild): if fork B/C, restore `LLM_ENDPOINT=http://127.0.0.1:8101/v1` in `.env` + `launchctl bootstrap` original mlx plist (backed up at `~/.mdemg/backup/com.mdemg.mlx-server.plist.pre-13_5/`)
3. **Backend rollback** (B/C only): unload new plist, reload original mlx plist, restart mdemg
4. **Per-fork rollback within sprint**: Epic 2 starts on a feature branch `reh3376_dev01_phase13_5_fork<X>`; merge to `reh3376_dev01` only after Epic 3 soak passes

Phase 11 + 11.6.x + 11.6.2 + 11.6.3 + 11.6.3.1 + 12 + 13 artifacts untouched. mdemg-llm-v1 model untouched. No Neo4j writes. No TSDB writes (no V0018).

---

## 13. Plan-Options (decision forks — RESOLVED post-synthesis 2026-05-02)

Per the data-decidable-not-operator-input principle, every fork below is decided from cited evidence in `phase_13_5_mlx_research_synthesis.md`. No operator-input fork remains in this sprint.

| Fork | Decided | Evidence |
|---|---|---|
| **Backend strategy: stay vs hybrid vs migrate** | **MIGRATE** | mlx_lm.server is officially not production-grade per maintainer (`SERVER.md`, Discussion #371). Hybrid still depends on mlx_lm.server for some traffic, which retains the disqualifying disclaimer for that traffic |
| **Finalist set** | **F1 (llama.cpp) + F2 (MLC-LLM)** | F1: production-grade, MIT, 100K stars, architectural KV bound. F2: architecturally distinct from issue Y (TVM Metal, not GGML/MLX), paged KV cache. Vllm-mlx dropped: Stream 1 located the crash in `libmlx.dylib` itself, which vllm-mlx inherits unchanged. Ollama disqualified by 8+ open M5/26.3.x issues. LM Studio disqualified by closed-source operability risk |
| **mlx-lm version policy** | **N/A — migrating off mlx_lm.server** | Per migration decision; mlx-lm framework remains in research/training paths |
| **Alternative backend** | **Decided by Epic 2 bake-off** | Empirical (B2 + B3 + B4 gates); not decidable from research alone |
| **Model format** | **Decided by Epic 2 outcome** | F1 wins → GGUF Q5_K_M; F2 wins → TVM `.dylib`. Both have de-quant paths verified in synthesis §B0 |
| **Bake-off topology** | **SEQUENTIAL** | Production runs one backend; parallel introduces cross-candidate memory contention that doesn't exist in production (synthesis Decision 3) |
| **OS rollback control test** | **SKIP** | Stream 1 ruled out Issue Y as cause of our crash; production OS is 26.3.2; testing on 26.2.x measures something we don't run on (synthesis Decision 2) |
| **Process recycle policy** | **N/A — chosen backend handles its own KV bound** | Both finalists have architecturally bounded KV. Recycle is a workaround for the disqualified mlx_lm.server only |
| **Crash-rate acceptance bar** | **0 crashes in 4-hour B3 stress** | Stream 4: real-world `ape.reflect` cadence in 4-hour window crosses 26 calls = the same call count that crashed mlx_lm.server. A passing finalist must clear that bar exactly |
| **Soak duration (Epic 3)** | **24 hours of real load** | Stream 4: peak concurrency P95 = 3.4; mean concurrency 1.4; statistical significance for "<1 crash/24hr" target requires the full window |
| **Watchdog rebind** | **Generalize to backend-agnostic** | Probe URL is the only thing changing; up→degraded→down state machine is reusable as-is. Lower code than per-backend instances |
| **Mlx mandatory-policy disposition** | **DEMOTE to LLM-endpoint-mandatory** | The framework still requires *a* working LLM endpoint; the brand of backend is no longer mandatory after migration. Renaming `MDEMG_ALLOW_NO_MLX` → `MDEMG_ALLOW_NO_LLM` is part of Epic 3 cutover |

The remaining open question — "which of F1 or F2 wins" — is decided in Epic 2 by empirical results (zero-crash 4-hour stress + UVTS quality A/B + concurrency stress), not by any further research, plan revision, or operator-input round.

---

## Acceptance bar (top-level)

A successful Phase 13.5 ships when:
1. **Crash rate < 1 mlx server crash per 24 hr** under realistic load (vs 1 per 14 min currently)
2. **Latency p50 ≤ 1.2× / p95 ≤ 1.5× pre-sprint baseline**
3. **No semantic regression > 10% per question** on UVTS quick (if a backend swap is part of the fork)
4. **Documentation complete** — operator can read the post-doc and reproduce the soak

Anything less is Phase 13.6.
