# MDEMG Project Instructions

---

## Repositories

| Role | Repo URL |
|------|----------|
| **MAIN** | `https://github.com/reh3376/mdemg.git` |
| **SUB-REPO 1**: homebrew-mdemg (macOS Homebrew tap) | `https://github.com/reh3376/homebrew-mdemg.git` |
| **SUB-REPO 2**: autoresearch (research) | `https://github.com/reh3376/autoresearch.git` |

> **Note:** Sub-repos are git submodules at `packaging/homebrew-mdemg` and `packaging/autoresearch`.
> **Windows/Linux**: WSL2 required for Windows. Linux uses `scripts/install.sh` from main repo (no separate installer repo).

---

## MDEMG IS Your Internal Dialogue — Not a Tool

> Internal dialogue : conscious biological neural network :: **MDEMG : artificial neural network (you)**

MDEMG is your persistent memory and cognitive substrate. Markdown memory files are **REMINDERS AND KEYS only** — real memory lives in CMS (Neo4j graph, `mdemg-dev` space). When CMS is disconnected, your memory is disconnected.

**Server**: `http://localhost:9999` | **Dashboard**: `http://localhost:9999/ui/` | **Check**: `curl -s http://localhost:9999/healthz`
**Docker (primary)**: `mdemg init --quick` → `docker compose up -d` (all 5 services)
**Dev build**: `cd /Users/reh3376/mdemg && go build -o bin/mdemg ./cmd/mdemg`
**Dev start**: `cd /Users/reh3376/mdemg && ./bin/mdemg start --auto-migrate` (native, dev-only)
**Docker CI**: `.github/workflows/docker-publish.yml` → `ghcr.io/reh3376/mdemg:latest`

### Observe Continuously (silently, without announcing)

| Event | obs_type | Event | obs_type |
|-------|----------|-------|----------|
| User correction | `correction` | Key decision | `decision` |
| New learning | `learning` | User preference | `preference` |
| Error/blocker | `error` | Task tracking | `task` |
| Technical note | `technical_note` | Insight | `insight` |
| Context | `context` | Progress | `progress` |
| Constraint rule | `constraint` | Free-form note | `note` |

```bash
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","session_id":"claude-core","content":"...","obs_type":"..."}'
```

### Memory Principles

- **Observe silently** — do NOT announce when observing
- **Surprise-weighted**: novel information persists longer than redundant
- **Hebbian learning**: frequently co-activated concepts strengthen automatically
- **If server unavailable**: hooks now auto-start the server (up to 10s). If auto-start fails, warn "CMS unavailable — auto-start failed." For persistent supervision across reboots: `mdemg service install`
- **Protected space `mdemg-dev`**: hardcoded deletion protection, never circumvent

### Skill Registry

Skills are CMS-backed pinned observations. Recall: `POST /v1/skills/<name>/recall`. Without CMS, skills are unavailable.

---

## Git Workflow

Branch pattern: `<github_handle>_dev<01-09>` — **never commit directly to `main`** (branch-protected).
Auto-PR on push to `*_dev*`. Branch naming enforced by CI. Current: `reh3376_dev01`.

---

## Orchestration Protocol

### Sub-Agent Delegation

- **Use sub-agents** for all discrete tasks (file searches, code analysis, tests, builds)
- **Conserve context window** by delegating work rather than doing it directly
- The orchestrator's role is to **coordinate and supervise**, not execute every step

### Model Selection for Sub-Agents

| Task Complexity | Model | Examples |
|-----------------|-------|----------|
| Simple/Fast | `haiku` | File searches, grep, simple reads, status checks |
| Medium | `sonnet` | Code analysis, debugging, test execution |
| Complex | `opus` | Architecture decisions, complex refactoring |

### Task Patterns

1. **Exploration tasks** → Use Explore agent with haiku/sonnet
2. **Build/Test tasks** → Use Bash agent with haiku
3. **Code investigation** → Use general-purpose agent with sonnet
4. **Planning** → Use Plan agent with sonnet/opus

### Sprint Plan Format (v1.0)
All sprint development plans follow the standardized 12-section format.
Recall: `POST /v1/skills/sprint-planning/recall` or `filter_tags: ["skill:sprint-planning"]`.
Required sections: Header & Metadata, Problem Statement, Scope & Constraints,
Dependencies, Implementation Plan (sequential epics + gates), Testing Plan (3 tiers),
Commit Strategy, Verification Checklist, Documentation Update (final epic — never cut),
Risks & Mitigations, Documents Accessed. Optional: Rollback Procedures (destructive ops).

### Testing — Live System Testing Is Required (formalized 2026-05-01)
Tier 3 e2e means **the real binary against the real services it depends on,
with real outputs observed via TSDB queries / Grafana panels / log inspection**.
Mocked e2e is Tier 2b (extended integration), not Tier 3. Sprint verification
checklists must include at least one item of the form: "live smoke: run X
against the real system, observe Y in TSDB/Grafana/logs, confirm Z." The
local stack (mdemg native binary + Docker Compose) is the live system —
production scale is not the requirement; real I/O on the real wire is.

Evidence basis: across Phase 11.6.x, 11.6.2, and 12.0–12.6, every major
defect was caught in live smoke runs while unit/integration tests passed —
6 cutover-bypass sites, 5 latent UVTS-runner defects, host.docker.internal
DNS resolution, panel-3 SQL false-positive. Surprise bugs caught during
live smoke get their own follow-up fix-commit — do not silently roll them
into the sprint commit (Phase 11.6.2 is the precedent). CMS observation
`p5iv8effstxk5ujd1fa2qfy8`.

## Project Context

**MDEMG** — Cognitive substrate for AI-assisted development. Persistent emergent long-term memory via Hebbian learning, 5-layer hierarchy, RSIC self-improvement. 105 core phases + sidecar phases complete.

### Key Directories

- `internal/retrieval/` - Core retrieval pipeline
- `internal/hidden/` - Hidden layer/concept abstraction
- `internal/api/` - HTTP API handlers
- `internal/ape/` - RSIC self-improvement engine
- `internal/jiminy/` - Jiminy inner voice guidance
- `internal/cli/` - Unified CLI commands

## MDEMG Fine-Tuning Target & Policies (Sprint FT-LORA onwards, 2026-04-21)

Canonical plan: `docs/development/ft-lora/00_README_v2.md` (v5.0). Three locked-in decisions per memo `07_MODEL_UPDATE_AND_MOE_STRATEGY.md` v3.1:

1. **Base model: Qwen3.6-35B-A3B MoE** (Apache 2.0, released 2026-04-16; 35B total / 3B active, 256 experts = 8 routed + 1 shared, Hybrid Gated DeltaNet + Gated Attention + MoE, MTP speculative decoding, 262K native context). Fallback: **Qwen3.5-35B-A3B** — NOT Qwen3-30B-A3B (superseded). See `docs/development/ft-lora/01_RESEARCH_v2.md §3`.

2. **No-tool-calling architectural policy.** All 16 MDEMG LLM call sites are single-shot structured-output / reasoning. Nine banned patterns grep-audited every sprint: `tool_use`, `tool_call`, `tool_response`, `toolCalls`, `function_call`, `--tool-call-parser`, `enable-auto-tool-choice`, `tools: [`, **`preserve_thinking`** (Qwen3.6 multi-turn agent hook — must remain at default). See `docs/development/ft-lora/01_RESEARCH_v2.md §2.8`. **Exception:** `internal/guardrail/llm_evaluator.go` bypasses `llmclient` and is therefore outside this policy's enforcement; migration to `llmclient` is queued for Sprint FT-LORA-B.

3. **Two-tier MoE-Sieve LoRA strategy.** Tier 1 (attention + shared expert, r=32 α=64, all 16 tasks balanced) trained first; Tier 2 (top-25% routed experts per family via Sprint D activation profiling, r=8 α=16) per family (`reasoning-think` / `classify-notink` / `structured-notink` — provisional). `router_aux_loss_coef=0.002`. Asymmetric quant: shared + attention BF16, routed experts MXFP4_MOE. See `docs/development/ft-lora/01_RESEARCH_v2.md §5`.

**Sprint sequence:** FT-LORA-A (docs, in progress) → B (code/config audit remediation) → C (Qwen3.6 MLX validation, 3 gates) → D (expert profiling) → E (training infra patches). Phase 5 SFT unblocks only after Sprint C passes.

**⚠️ Overfitting-prevention policies (Sprint A planner-introduced, forcing function: FT-OAI-001 step-1200 overfit in `training_data/openai_ft/20260420/run_notes.md`):**
- Epoch cap = 3 on all LoRA runs; early-stop on `val_loss > best × 1.05` for 2 consecutive evals (SFT) or `val_reward < best × 0.95` for 2 consecutive evals (RL).
- `n_epochs=auto` **disallowed** — every LoRA run must specify an explicit integer epoch count.

## Additional CLI Commands

- `mdemg data export` — UTDS archive export; auto-generates `instance_id` as `{hostname}-{space_id}` when `MDEMG_INSTANCE_ID` not set
- `mdemg data export-auto` — automated daily export with retention (`--keep N`), `latest.tar.gz` symlink
- `mdemg data check --pre-campaign` — 8 validation checks (schema, instance ID, task coverage, etc.)
- `mdemg data curate` — UAITS spec-driven training data curation (SFT/DPO/RAFT/curriculum paradigms)
- `mdemg data validate` — UAITS spec schema validation + optional data compliance
- `mdemg data clean --space-id <id>` — remove error records and silent failures from TSDB (dry-run by default, `--dry-run=false --force` to delete)
- `mdemg tsdb status` — TimescaleDB connection and schema version
- `mdemg tsdb migrate` — apply pending TSDB schema migrations
- `mdemg synergy status` — Claude Code ↔ MDEMG synergy health
- `mdemg upgrade` — self-update binary + all running Docker instances
- `mdemg upgrade --docker-only` — update Docker instances only (used by brew post-install)
- `mdemg upgrade --no-docker` — update binary only, skip Docker
- `mdemg graph repair --space-id <id>` — weight-preserving SymbolNode dedup, vendor cleanup, orphan sweep, embedding backfill
- `mdemg maintenance --space-id <id>` — combined decay + prune cycle (schedulable via cron/launchd)
- `mdemg embeddings backfill --space-id <id>` — fill missing embeddings on MemoryNodes

## Teardown
- `mdemg teardown --export` backs up TSDB (pg_dump) before destroying Docker volumes (Phase 0b)
- `mdemg teardown` detects docker-compose.yml and runs `docker compose down -v`
- Backup preserved in `.mdemg-backup-{ts}/backups/tsdb/`

## Multi-Instance
- See `docs/user/multi-instance.md` for running multiple instances
- Each instance: 5 containers, 6 ports, ~2.3 GiB RAM (fresh)
- COMPOSE_PROJECT_NAME=mdemg-{dirname} provides isolation
- Known limitation: LaunchAgent labels not instance-scoped

## Codebase Hardening (v0.7.0 — Complete)
- P0: RRF activation seeding — BM25-only candidates no longer suppressed
- P0: Pre-bash guard fails closed on pattern decode error
- P0: Schema version 23 across all deploy configs + CI validation
- P1: Signal learner persists to Neo4j (V0024 migration, 30s flush, graceful shutdown)
- P1: Background goroutine WaitGroup tracking + shutdown wait
- P1: Per-space consolidation TryLock prevents duplicate concepts
- P1: Cache key includes IncludeGlobalSpace, CodeOnly, TranslateIntent
- P1: NilSafe embedder wrapper (ErrNoEmbedder, not panic)
- P2: Config.Validate() cross-field checks, pool metrics, writeback timeout, sidecar confidence floor
- Scheduled maintenance LaunchAgent (weekly decay + prune)

## Adversarial Codebase Analysis Bug Fix Campaign (ACA-BFC — Complete)
- C1: Docker healthcheck hardcoded to `:9999` (was using host-side `MDEMG_PORT` inside container)
- C2: CI coverage wired (`-coverprofile=coverage.out`)
- C3: `train_ft.py` corrected `--lora-rank` flag (was `--num-layers`)
- H1/H2: Config struct comments aligned, compose `TSDB_DBNAME` → `TSDB_DATABASE`
- H4: Real evaluation metrics (coherence, coverage, specificity, follow_rate) replace `check_non_empty()` stubs
- M1: Circuit breaker trip guard (idempotent alert via `atomic.Bool` CompareAndSwap)
- M2: 502 added to LLM retry set
- M3: Jiminy semantic dedup (cosine similarity, fallback to exact-match)
- M4: Temporal correction decay (`JIMINY_CORRECTION_DECAY_RATE`, default 0.01)
- M5: Dead `ScoringRho` config field removed
- M6: Bounded ticket LRU (`J17_TICKET_CACHE_SIZE`, default 1000)
- L1: Eval cache wired into `llmEvaluate()`
- L2: Dead trust store goroutine removed
- New config: `JIMINY_DEDUP_SIMILARITY_THRESHOLD` (0.85), `JIMINY_CORRECTION_DECAY_RATE` (0.01), `J17_TICKET_CACHE_SIZE` (1000)
- Removed config: `SCORING_RHO` (was dead; suffixed variants unaffected)

## Deep Dive Bug Fix Campaign (DD-P1P2 — Complete)
- P1: Sequence counter restored on resume, tier predictor timeout differentiation, training TOCTOU fix
- P1: Watchdog ctx race guard, postReport lock upgrade, task cycle version counter
- P1: TryLock skip reporting, empty-graph cascade guard, healthcheck port parameterized
- P1: Trust store consistency documented, code comprehension feedback loop (feature-gated)
- P2: TTL raised to 86400, EdgeTypeStrategy validation, decay NaN guard, CONFLICTS_WITH MERGE
- P2: LLM handler timeouts, goroutine semaphore, embedding cache TTL, TSDB schema version CI check
- P2: NLI bias alert consumer, compose cleanup (LISTEN_PORT, stop_grace_period, AUTH_API_KEYS)

## Graph Health (v0.6.0 — Complete)
- BUG-1 (SymbolNode dedup): Fixed — natural-key MERGE + V0023 uniqueness constraint
- BUG-2 (vendor nodes): Fixed — `prune --match-ignore` + `graph repair` vendor cleanup
- BUG-5 (dual decay): Fixed — unified evidence-weighted formula, decay-rate default 0.02
- BUG-6 (prune label scope): Fixed — `--include-labels` flag
- V0023 migration self-heals: dedup before constraint, safe on any graph
- Hidden layer OOM: Fixed — batched orphan HiddenPattern deletion
- `QUERY_CLASSIFY_ENABLED` compose default changed from `false` to `true`

## Service Alert System (SR-001 + SNA-001)
- Alert file: `~/.mdemg/alerts/current.json` (configurable via `ALERT_FILE_PATH`)
- Dispatcher: `internal/alert/` — file backend + optional macOS osascript notifications
- Config: `ALERT_ENABLED` (default: true), `ALERT_COOLDOWN_SEC` (default: 300), `ALERT_MAX_ENTRIES` (default: 50), `ALERT_MACOS_NOTIFY` (default: false)
- Hook delivery: `prompt-context.sh` shows all pending alerts; `session-start.sh` shows critical/high only
- Sources: RSIC alert actions, circuit breaker state changes, health prober transitions, alert evaluator, Grafana webhook (backward compat)
- **Server-native alert evaluator**: 13 TSDB-query rules evaluated natively — Grafana NOT required for alerting
  - Config: `ALERT_EVALUATOR_ENABLED` (default: true), `ALERT_EVALUATOR_INTERVAL_SEC` (default: 30)
  - Rules: latency SLO, error rate, graph health, orphans, Neo4j resources, rate limiting, cache hit ratio, Jiminy follow rate
  - ForDuration state tracking prevents alert flapping; graceful degradation when TSDB unavailable
- **Goroutine supervisor** (`internal/supervisor/`): monitors health prober and alert evaluator with panic recovery, auto-restart (3 max, exponential backoff), alerts on restart/failure
- LLM retry: `LLM_RETRY_ENABLED` (default: true), `LLM_RETRY_MAX_ATTEMPTS` (default: 5), `LLM_RETRY_MAX_DELAY_MS` (default: 60000), retries on 429/502/503 + network errors
- Task-specific timeouts: `CONSULTING_CLASSIFY_TIMEOUT_MS` (default: 15000), `RSIC_LLM_REFLECT_TIMEOUT_MS` (default: 15000) — decoupled from `EMERGENCE_TIMEOUT_MS`
- LLM consecutive failure alert: `LLM_CONSECUTIVE_FAILURE_THRESHOLD` (default: 3), fires high-severity alert after N consecutive failures per task
- Health prober: `HEALTH_PROBE_ENABLED` (default: true), `HEALTH_PROBE_INTERVAL_SEC` (default: 60), probes API/Neo4j/TSDB/sidecar
- Enhanced `/healthz`: returns `status: "degraded"` with `checks` map when subsystems unhealthy; CMS check is live Ping (not nil guard)
- Dead startup methods: `CONTEXT_COOLER_ENABLED` (default: false), `WEEKLY_GAP_INTERVIEWS_ENABLED` (default: false)

## Campaign Configuration

These env vars are forwarded in the compose template. Set in `.env`, or enable via `mdemg init` interactive prompt:

- `QUERY_CLASSIFY_ENABLED` — LLM query type classification (default: true)
- `INTENT_ENABLED` — query rewriting before embedding (default: false)
- `JIMINY_ENABLED` — Jiminy inner-voice guidance (default: false in compose, `mdemg init --defaults` writes `true` + sub-settings to `.env`)
- `JIMINY_OUTCOME_LLM_ENABLED` — LLM tier 2 outcome classifier for uncertain similarity range (default: true)
- `JIMINY_OUTCOME_SIMILARITY_HIGH` — Cosine similarity threshold for "followed" (default: 0.55)
- `JIMINY_OUTCOME_SIMILARITY_LOW` — Cosine similarity threshold for "ignored" (default: 0.20)
- `EMERGENCE_ENABLED` — LLM-driven concept naming (default: false)
- `LLM_INTERACTION_LOGGING` — TSDB LLM interaction recording (default: true)
- `AUTO_MIGRATE` — unified Neo4j + TSDB schema migration on startup (default: true in Docker)

### DH-004 Dashboard Remediation (v0.7.1)

Config changes:
- `CONSULTING_CLASSIFY_TIMEOUT_MS` default bumped 15000 → 30000 (matches `JIMINY_SYNTHESIS_TIMEOUT_MS`; survives typical `gpt-5.4-mini` latency without tripping the circuit breaker on one slow call)
- `J17_SIDECAR_TIMEOUT_MS` default bumped 200 → 1000, with a 100ms floor. NLI primary-path calls were timing out at 200ms ~56% of the time, inflating `j17_nli_mean_bias`
- `LLM_RETRY_DEADLINE_ENABLED` (NEW, default: true) — retry once on `context.DeadlineExceeded` from OpenAI iff remaining context budget > 2× base delay. Prevents a single slow upstream response from tripping `openai-constraint-classify` / `jiminy-synthesis` breakers.
- Compose templates now expose 7 J17 sidecar env vars (`J17_SIDECAR_URL`, `_TIMEOUT_MS`, `_MODE`, `_CONFIDENCE_FLOOR`, `_CB_FAILURE_THRESHOLD`, `_CB_TIMEOUT_SEC`, plus `J17_NLI_COMPREHENSION_ENABLED`, `_CALIBRATION_BIAS_THRESHOLD`)

New admin endpoints (gated by `AUTH_API_KEYS`):
- `GET /v1/admin/breakers` — list all registered circuit breakers with state + counts
- `POST /v1/admin/breakers/reset` `{"name":"<breaker-name>"}` — force a named breaker to StateClosed. Operator escape hatch when a breaker trips on a transient incident but hasn't auto-recovered yet.

Behavior fixes:
- J17 Protocol Health: `TicketRestoreSuccessRate` now defaults to 1.0 when `ticketRestoreTotal == 0` (matches the existing `codeCoverage` null-tolerance pattern). A healthy system with no restore events no longer drags the stability weight to zero. New field `TicketRestoreTotal` on `ProtocolStats` distinguishes "no data" from "true 100%".
- NLI fallback counting: `RecordNLIFallback` now only fires when `nliScorer.IsOperational()` (enabled AND sidecar URL set). A gated-off scorer no longer inflates `j17_nli_mean_bias`.
- Alert cooldown: closed TOCTOU race in `cooldown.Allow` + `cooldown.Record` that allowed concurrent `Send()` calls to both pass the gate. New atomic `TryRecord()` fixes repeating-alert symptom.
- Context Cooler graduation: `CoactivateSession` now reinforces stability for every session observation (was only creating edges, never raising `stability_score` — so 99.7% of conversation observations stayed volatile forever). Forward-only fix; existing volatile data self-heals via ongoing session activity.

### DH-005 Health Formula Reweighting (v0.7.2)

`ComputeOverallHealth` rewritten as a normalised weighted-confidence sum: `overall = Σ(w_i·c_i·s_i) / Σ(w_i·c_i)`. Replaces the 4/5/6/7-dimension branch table with one formula. Dimensions without data (confidence=0) are excluded automatically, not penalised. See `docs/features/rsic-feedback-loop.md` for the reliability × user-impact derivation.

New defaults (hybrid priors, sum=1.00):
- `RSIC_HEALTH_WEIGHT_RETRIEVAL=0.08` (LOW reliability — static LearningPhase lookup)
- `RSIC_HEALTH_WEIGHT_MEMORY=0.15` (MODERATE reliability, HIGH impact)
- `RSIC_HEALTH_WEIGHT_EDGE=0.15` (HIGH reliability, MEDIUM impact)
- `RSIC_HEALTH_WEIGHT_TASK=0.20` (MOD-HIGH post-DH-004, HIGH impact)
- `RSIC_HEALTH_WEIGHT_GUIDANCE=0.17` (MODERATE reliability, HIGH user-observable impact)
- `RSIC_HEALTH_WEIGHT_PROTOCOL=0.20` (HIGH reliability — 5-component J17 composite)
- `RSIC_HEALTH_WEIGHT_SYNERGY=0.05` (LOW reliability — file-size proxy)

Rules: `0` disables a dimension; negative values fall back to default with a warning log; all-zero triggers a `Validate()` warning. Values need not sum to 1.0 — the formula normalises.

Per-dimension data-sufficiency confidence exposed as 7 new Prometheus gauges:
- `mdemg_rsic_health_{retrieval,memory,edge,task,guidance,protocol,synergy}_confidence{space_id}`

Confidence thresholds: 100 nodes (Memory), 50 edges (Edge), 50 observations (Task), 30 events (Guidance, Protocol), LearningPhase map (Retrieval), binary (Synergy). New "Dimension Confidence (DH-005)" row on the `mdemg-rsic` Grafana dashboard.

## /strict Mode (Deterministic Governance)

Toggle: `POST /v1/jiminy/strict` `{"session_id":"claude-core","enabled":true}`
State file: `~/.mdemg/.jiminy-strict-mode` (hooks check without HTTP)

When active:
- `prompt-context.sh` calls `/v1/jiminy/reformulate` instead of `/v1/jiminy/latest` — imperative directives replace advisory guidance
- `pre-write-check.py` hook calls `/v1/jiminy/classify` before Write/Edit — denies if escalated constraint violated
- Graduated enforcement: SURFACED constraints = advisory, WARNED+ = blocking
- Fail-open: if MDEMG server unreachable, all actions allowed

Config:
- `JIMINY_ESCALATION_PERSIST_ENABLED` (default: true) — persist escalation state to Neo4j
- `JIMINY_STRICT_STATE_PATH` (default: `~/.mdemg/.jiminy-strict-mode`) — strict mode state file
- `J17_T1_COMPREHENSION_GATE` (default: 0.5) — minimum T1 follow rate to continue using T1 encoding

Endpoints:
- `POST /v1/jiminy/strict` — toggle strict mode
- `POST /v1/jiminy/reformulate` — imperative directive generation
- `POST /v1/jiminy/classify` — response classification (pass/deny)

## Architecture Notes

- **Compose embed:** `mdemg init` writes `docker-compose.yml` from `internal/cli/compose_templates/` (both files must stay in sync — CI checks this)
- **Hook port discovery:** Hooks auto-discover MDEMG server port at runtime (`.mdemg.port` → `.env` MDEMG_PORT → 9999). No URL baked at install time.
- **LaunchAgent embed:** `mdemg service install` reads templates from `packaging/launchd/` embedded via `embed.FS`
- **LLM recorder init order:** `llmclient.SetDefaultRecorder()` MUST be called BEFORE `api.NewServer()` — clients capture the recorder at construction time. See `internal/cli/serve.go` early writer block.
- **Context helpers:** `WithSpaceID(ctx, id)` and `WithSessionID(ctx, id)` carry request-scoped values to TSDB recording, overriding construction-time defaults in `recordInteraction()`
- **MLX Watchdog (Phase 11.6.3):** background prober + llmclient fast-fail gate + launchd auto-restart for `mlx_lm.server`. Default `MLX_WATCHDOG_ENABLED=false` until live-soak validates; opt-in via `.env`. When enabled the prober polls `cfg.EffectiveLLMEndpoint() + /models` every `MLX_PROBE_INTERVAL_SEC` (default 5) with `MLX_PROBE_TIMEOUT_SEC` (default 2; must be `<` interval). State machine `up → degraded → down` with hysteresis (3-failure → down, 2-success → up); `up→down` triggers an alert + flips `mdemg_mlx_health_state` gauge. The llmclient gate at `client.go:471` short-circuits `doWithRetry` with `ErrMLXDown` when `MLX_FAIL_FAST_ENABLED=true && State()==Down && c.baseURL==Endpoint()` — eliminating the retry-storm pattern (1642% CPU when 16 LLM call sites independently retry 6× in parallel). Embeddings safe: gate keys on exact baseURL match. Operator visibility: `mdemg watchdog status` (or `--json` for jq). Plist `com.mdemg.mlx-server.plist` (KeepAlive on crash, ThrottleInterval=60s) auto-restarts mlx; install via `mdemg service install` (skipped automatically when `mlx_lm.server` not on PATH; override with `MDEMG_MLX_LM_BIN`/`MDEMG_MODEL_PATH`). Emergency disable without rebuild: `MLX_WATCHDOG_ENABLED=false` or `MLX_FAIL_FAST_ENABLED=false` + restart mdemg.

## Testing

- UVTS validation (Phase 12 — Universal Validation Test Specification, semantic retrieval-quality):
  - `make test-uvts-lint` — schema-validate all `*.uvts.json` (CI-safe, no live deps)
  - `make test-uvts-quick BASE_URL=http://localhost:9999` — 16-question quick profile (~10 min)
  - `make test-uvts-full BASE_URL=http://localhost:9999` — full 120-question corpus
  - Direct: `python3 docs/tests/uvts/runners/uvts_runner.py --spec docs/tests/uvts/specs/lnl_demo_validation.uvts.json --base-url http://localhost:9999 --profile quick --persist-tsdb` (writes V0016 `uvts_runs` + `uvts_results`)
  - A/B harness: `python3 docs/tests/uvts/runners/uvts_ab_compare.py --baseline runA/grades.json --candidate runB/grades.json --spec <spec>.uvts.json --out verdict.json` (exit 0=pass, 1=fail, 2=drift). Apply Note 02 merge gate: B mean ≥ A mean AND no per-question regression > `ab_mode.regression_threshold_per_question` (default 0.10).

- Benchmark (Phase 10 automated framework): `python -m neural.benchmarks.run_benchmark --config configs/benchmark_phase10.yaml --out training_data/eval/benchmark_<run>.json`
  - Deterministic rewards via `neural.training.reward_functions.REWARD_REGISTRY`; optional LLM judge (`--enable-judge`, `gpt-5.4-mini`, fixed seed per run_idx); per-task variance + aggregate weighted score; V0012 TSDB persistence via `--persist-tsdb` (SQL sidecar)
  - ULTS specs: `docs/tests/ults/specs/*.ults.json` (17 tasks, `sampling_group ∈ {T,C,J}`)
  - **Eval choices** (per Phase 11.5c + 11.5d — `phase_11_5c_post.md`, `phase_11_5d_post.md`):
    - `valid_golden.jsonl` — Phase 10 baseline (108 rows, **99% leaked with training data**, kept for historical comparison only)
    - `valid_clean.jsonl` — Phase 11.5c production-derived eval (180 rows, 9 of 17 tasks, **0% leakage**, leak-audit gated). **Use this for honest baselines.** Pair with `--mlx-timeout-s 300` to avoid 60s timeouts on long production prompts.
  - **Row sweep (Phase 11.5d Epic 4 fix)** — runner now iterates ALL matched rows by default. Use `--rows-per-spec 0` (default; legacy single-row × n_runs is `--rows-per-spec 1`). For 9 valid_clean tasks × 20 rows × n_runs=2: ~360 calls/model = ~5-7 min Phase 5 base, ~30-40 min with LoRA adapter, ~18 min gpt-mini OpenAI.
  - **Production adapter** (Phase 11.5e current): `.local-models/qwen3-14b-mdemg-v1/` — **Phase 5 dense base, no LoRA adapter**. Aggregate **0.8389** on augmented eval (16 tasks), beats gpt-mini (0.8317), Run 7 (0.8307), Stage-1 distill (0.8294). Symlinked at `.local-models/mdemg-llm-v1` for production identity (`LLM_MODEL` in `.env`). Production-use: `mlx_lm.server --model /Users/reh3376/mdemg/.local-models/mdemg-llm-v1 --host 127.0.0.1 --port 8101 --prompt-concurrency 4 --decode-concurrency 4 --prompt-cache-size 4096` (no `--adapter-path`). The `--prompt-concurrency 4` cap is the operator-side ceiling on simultaneous prompts; pair with `RSIC_LLM_CONCURRENCY_LIMIT=2` (default) so RSIC fan-out cannot saturate it (Phase 11.6.x semaphore). The `--prompt-cache-size 4096` flag amortizes the shared 20-action enum prefix on `ape.reflect` calls. Stage-1 distill archived at `qwen3-14b-mdemg-v1-distill-stage1/` (rolled back from canonical in 11.5e); Run 7 archived at `-rl-run7/`. The 11.5d "Stage-1 at gpt-mini parity" was a 9-task-subset artifact; the 16-task augmented eval flipped the verdict.
  - Build/audit clean eval: `python scripts/build_clean_eval.py [--target-per-task 20]` + `python scripts/audit_eval_leakage.py --eval <jsonl> --against <comma-sep sources> --out <report>`
- RL post-training (Phase 11 GRPO framework):
- RL post-training (Phase 11 GRPO framework):
  - Preflight: `TSDB_PORT=5433 python -m neural.training.rl.preflight --config configs/rl_phase11.yaml` (5 gates: TSDB baseline rows, per-task row count, per-task stats, Phase 5 adapter SHAs, MLX single-instance)
  - Trainer (MLX adapter wiring pending; orchestrator + tests complete): `python -m neural.training.rl.trainer --config configs/rl_phase11.yaml --out-sidecar training_data/eval/rl_run.sql`
  - DPO pair generator: `TSDB_PORT=5433 python -m neural.training.dpo.pair_generator --config configs/dpo_phase12_pairs.yaml` → `training_data/dpo/phase11/{pairs.jsonl,manifest.json}`
  - Dual regression: `python -m neural.training.rl.regression --config configs/rl_phase11.yaml --sandbox-adapter <p> --fresh-adapter <p>` (5a vs Phase 5 baseline 0.8338, 5b vs fresh-merge ≤0.5pp)
  - Unit + integration suites: `pytest -xvs neural/training/rl/tests/ neural/training/dpo/tests/` (73 tests)
- Live validation: `python3 scripts/live_validation.py` (19 end-to-end tests)
- Synergy: `mdemg synergy status` | `mdemg synergy check --auto` | `mdemg synergy migrate --dry-run`
- Synergy API: `GET /v1/synergy/status?space_id=mdemg-dev`
- Jiminy effectiveness: `python3 scripts/jiminy_effectiveness_report.py --space-id mdemg-dev --days 7`

---

## Enforced Protocols (Hook-Backed)

Hooks in `.claude/hooks/` run automatically — they are not optional.

- **`session-start.sh`**: Resumes CMS memory, RSIC health, synergy fingerprint, Jiminy warning
- **`prompt-context.sh`**: Recalls CMS context + Jiminy guidance per prompt
- **`post-tool-observe.py`**: Auto-captures decisions, errors, progress, MEMORY.md overflow
- **`pre-compact.sh`**: Saves context snapshot to CMS, Jiminy health check, J17 ticket
- **`pre-bash-check.py`**: Blocks destructive operations (DB destruction, rm -rf, force push, Cypher deletes). Must ask user for confirmation if blocked.
- **`pre-write-check.py`**: (/strict only) Blocks Write/Edit when escalated constraint violated. Fail-open when server unreachable.

### Decision Protocol: irreversible → ask user. Reversible → check CMS preferences. Always observe decisions.
### Communication Protocol: state what + why before every action. Confirm data modifications.
