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
  - ⚠️ **Distinct `Service` per rule:** the dispatcher cooldown key is `(Service, Severity)` — two rules sharing one service+severity will suppress each other (one alarm masks another). Caught live in NOSILENT-001. When adding a rule, give it a unique `Service` label.
- **Scheduled-job health — no silent failures (Sprint NOSILENT-001 — 2026-06-08):** the 3 scheduled/background core jobs (TSDB backup scheduler, `maintenance`, `export-auto`) record every run to the V0024 `scheduled_job_events` hypertable and fire a high-severity alert on failure via `internal/jobhealth.Report` (the single record+alert policy point; pool + dispatcher nil-safe). Backup wires a decoupled `JobResultFunc` hook (so `internal/tsdb` stays free of `internal/alert`) set in `server.go::SetTSDBClient` with the pool + `s.alertDispatcher`; the CLI jobs (separate processes) defer `reportScheduledJob` → short-lived pool + file-backed dispatcher writing the same `~/.mdemg/alerts/current.json` the hooks surface. **Two evaluator rules** over `scheduled_job_events`: `scheduled_job_recent_failure` (any failure in `JOB_FAILURE_LOOKBACK_MIN`, default 60) + `backup_no_recent_success` (zero successful `tsdb-backup` in the staleness window = backup interval × 2 unless `JOB_BACKUP_STALENESS_HOURS` overrides; gated on `TSDB_BACKUP_ENABLED`). The staleness rule is the **"job never ran" guarantee** — it fires from the server observing *absent* success, catching a job that silently died or never started, not just one that ran and errored. Config: `JOB_HEALTH_ALERT_ENABLED` (default true). Trigger was a real silent failure: the backup scheduler's `docker compose pg_dump` failing every 24h under the launchd minimal PATH (fixed by `internal/dockerbin`, commit `4cc7608` — `MDEMG_DOCKER_BIN` override → PATH → well-known docker locations; the data plane never used the docker CLI — Neo4j Bolt + TSDB pgx are network). Feature doc: `docs/features/scheduled-job-health.md`. Sprint: `docs/development/nosilent-001/`.
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

- **Score-scale contract (Sprint RRF-SCALE-001 — 2026-06-03):** ⚠️ **Downstream consumers MUST NOT hardcode absolute thresholds against `RetrieveResult.Score`.** The retrieval score scale is **not a stable contract** — it changes when the scorer changes. Column-Voting RRF (default-on Phase 13.1, 2026-05-03) dropped the scale: legacy linear scores could exceed 1.0; RRF fused scores top out ~0.49–0.58 for strong matches. This silently broke **three** downstream consumers calibrated for the old scale: (1) EVENTGRAPH-001's `Activation`-drop → 24-day Hebbian no-op; (2) the `consulting/service.go` `0.55` constraint gate → 9-week guidance-loop dormancy; (3) the score→confidence sigmoid (`midpoint=1.5`) crushing RRF confidence to ~0.1–0.2. RRF-SCALE-001 made the consulting gates + sigmoid config-driven and RRF-calibrated (`CONSULTING_{CONSTRAINT,AUTHORITY,CONFLICT}_SCORE_FLOOR` default 0.45; `RETRIEVAL_CONFIDENCE_SIGMOID_{MIDPOINT,STEEPNESS}` 0.45/8.0). **When adding code that reads a retrieval score: gate via config (with an RRF-calibrated default) or via a scale-invariant signal, never a hardcoded literal. When changing the scorer: re-audit every `RetrieveResult.Score`/`.Activation` comparison.** `NormalizedConfidence` looks scale-invariant but is a *positional* percentile rank (admits noise on uniform-score sets) — not a safe sole gate. Audit + fix + verification in `docs/development/rrf-scale-001/`. Three adjacent issues surfaced during live smoke remain open follow-ups: Neo4j `GUIDANCE_OUTCOME` edge node-type targeting (retrieval surfaces `emergent_concept` abstractions, not raw constraint nodes — candidate JIMINY-OUTCOME-001), LLM synthesis timeout, and `/v1/jiminy/latest` JSON control-char escaping (breaks the `jq` in `prompt-context.sh`).

- **Guidance-outcome constraint-code matching (Sprint JIMINY-OUTCOME-001 — 2026-06-08):** Completes the guidance-loop revival begun in RRF-SCALE-001 (Follow-up A). The Neo4j `GUIDANCE_OUTCOME` edge sink (feeds per-constraint effectiveness graph stats via `GetConstraintEffectiveness`) requires guidance items to carry a `constraint_code` that `PersistGuidanceOutcome` resolves to a real `role_type='constraint'` node. That code is assigned in `Guide()` by **embedding similarity** (`matchConstraintCodeByEmbedding`, JIMINY-OUTCOME-001) — vector-index query over `role_type='constraint'` nodes, `sim ≥ JIMINY_CONSTRAINT_CODE_SIM_THRESHOLD` (default 0.55), mirroring `Evaluator.findMatchingConstraints`. The legacy keyword-overlap matcher (`matchConstraintCode`, ≥3 shared words) remains the **fallback** when the embedder is unavailable / content empty / nothing clears the threshold. **Why:** retrieval surfaces `emergent_concept` abstractions whose content rarely shares 3+ literal words with raw constraint text, so keyword matching assigned no code and the Neo4j sink went dormant ~8 weeks; embedding matching links concepts → the correct constraint code → edge on the real constraint node. Both outcome sinks (TSDB `constraint_outcomes` from RRF-SCALE-001 + Neo4j `GUIDANCE_OUTCOME` here) are now live. Sprint in `docs/development/jiminy-outcome-001/`.

- **Guidance synthesis budget + classifier concurrency (Sprint GUIDANCE-SYNTH-001 — 2026-06-08, Follow-up B):** Guidance synthesis was failing on **every** production warm call (`synthesis_error: context deadline exceeded`; 6/6 errored) — the hook's `/v1/jiminy/warm` background `Guide()` had a **hardcoded 30s timeout**, inside which the per-node LLM constraint classifier ran **serially** (~1.5s/node × ~10 ≈ 15s), starving synthesis (needs 8–27s, observed up to 50s). Fixes: (1) `consulting.findApplicableConstraints` classifies with **bounded concurrency** (`CONSULTING_CLASSIFY_CONCURRENCY`, default 4 = llama-server `--parallel 4`, floor 1 = serial) — gate-first → position-indexed slots → collect-in-order → dedup, so parallel output is identical to serial; (2) the warm-compute timeout is config-driven (`JIMINY_WARM_COMPUTE_TIMEOUT_MS`, default **90000**, was hardcoded 30s). Live-verified: warm path produces a real synthesized narrative (`synthesis_used=true`), a fresh `jiminy.synthesize` succeeded at **50.7s** (fit the 90s budget; would die at 30s). **When adding LLM calls to the guidance hot path: respect the warm-compute budget and prefer bounded concurrency over serial loops.** Local-model synthesis latency is high (~50s/narrative) — config-tunable, fire-and-forget on the warm path so it doesn't block the hook. Sprint in `docs/development/guidance-synth-001/`.

- **RRF-SCALE-001 follow-up triage — fully closed (2026-06-08):** All three live-smoke follow-ups from RRF-SCALE-001 are resolved. A (Neo4j `GUIDANCE_OUTCOME` sink) → JIMINY-OUTCOME-001. B (synthesis timeout) → GUIDANCE-SYNTH-001. **C (`/v1/jiminy/latest` JSON control-char escaping) → investigated and closed as a NON-ISSUE, no code change** (`docs/development/followup-c-closure.md`): `writeJSON` uses `encoding/json` which always escapes control chars; the synthesized narrative is double-`StripControlChars`'d; and `prompt-context.sh` already strips control chars via `perl` before `jq` (with `// empty` fallbacks). The original parse errors were client-side shell artifacts, not server bytes — verified by 5× strict-JSON parse + the hook's exact `jq` returning `guidance_id`. **The guidance→feedback→outcome loop is fully functional end-to-end.**

- **Event Graph Federation (Sprint EVENTGRAPH-001 — 2026-05-27, Pattern Y1):** First implementation of the TypeDB-inspired Neo4j refactor — federate "events about edges" into TSDB instead of reifying them as graph nodes; preserve graph traversal via a Go orchestration layer. V0022 `reinforcement_events` hypertable captures one row per Hebbian co-activation pair update from `ApplyCoactivation` (other Hebbian entry points deferred to EVENTGRAPH-003): `prev/new/delta_weight`, `evidence_count_after`, `eta_effective`, `surprise_factor`, `activation_product`, `path_sim`, `role/obs_type` of both endpoints, `session_id`, `direction`, `created_new_edge` (distinguishes "new connection formed" from "existing connection strengthened"), `trigger_path`. Buffered + CopyFrom (V0019 pattern, NOT V0021 sync-INSERT — Hebbian volume is per-retrieve). Federation API: `POST /v1/eventgraph/reinforcement-neighborhood` orchestrates Cypher graph walk from a seed (depth 0..hops via CO_ACTIVATED_WITH|GENERALIZES) + TSDB query for events touching the neighborhood + Go-side join annotating events with `src/dst_in_neighborhood`. 7 env vars (all default-on, no-hardcoding rule): `EVENTGRAPH_ENABLED`, `_WRITER_FLUSH_INTERVAL_SEC`, `_WRITER_BUFFER_SIZE`, `_MAX_PAIRS_PER_EVENT_BATCH`, `_MAX_EVENTS_PER_QUERY`, `_FEDERATION_DEFAULT_HOPS`, `_FEDERATION_DEFAULT_LOOKBACK_HOURS`. 3 Prometheus counters: `mdemg_eventgraph_writer_rows_{enqueued,dropped}_total` + `_flush_failure_total`. Grafana panel "Reinforcement Event Rate" on `mdemg-graph-topology`. Forward-only — no historical backfill. Pattern Y2 (link-node reification in Neo4j) explicitly deferred until a query proves federation-in-Go insufficient. Sprint plan + post in `docs/development/eventgraph-001/`. Feature doc: `docs/features/event-graph-federation.md`. **CLI consumer (Sprint EVENTGRAPH-CLI-001 — 2026-06-08):** `mdemg eventgraph reinforcement-neighborhood` is the first consumer of the federation API + the live-testing harness for the line. Seed via `--seed n_…` or `--query "<text>"` (resolves seed from top `/v1/memory/retrieve` result); `--hops`/`--since`/`--limit` omitted-when-unset so the server applies config defaults (single source of truth — no CLI-side default copies); table or `--json`. UATS contract `eventgraph_reinforcement_neighborhood.uats.json` (6/6 live) backfills the EVENTGRAPH-001 contract-test gap. Live-caught contract fix (own commit `9bf981b`): `EventsInGraphNeighborhood` now coalesces a nil `NeighborNodeIDs` to `[]` so it serializes as `[]` not `null` for an empty/unknown-seed neighborhood (matches `events`); `TestFederationResult_EmptyArraysNotNull` pins it. Sprint in `docs/development/eventgraph-cli-001/`. **Second event class (Sprint EVENTGRAPH-002 — 2026-06-08):** guidance-outcome federation — `POST /v1/eventgraph/guidance-outcome-neighborhood` + `mdemg eventgraph guidance-outcome-neighborhood` surface the followed/ignored/contradicted outcomes for a constraint's graph neighborhood. **Reuses the existing `constraint_outcomes` sink** (migration 011, written by `/v1/jiminy/feedback` — RRF-SCALE-001 + JIMINY-OUTCOME-001); NO new hypertable/writer/enqueue site (data-decided: don't duplicate a populated sink). **Joins graph↔events on `constraint_code`, not node_id** — TSDB `constraint_id` is a UUID that doesn't match the Neo4j `node_id` CUID; `constraint_code` (carried by both `role_type='constraint'` nodes + `constraint_outcomes` rows) is the only viable key. `GuidanceOutcomesInNeighborhood` (`internal/eventgraph/guidance_outcomes.go`) walks the neighborhood collecting codes + a code→node map, queries `constraint_outcomes WHERE constraint_code = ANY(codes)`, Go-joins each outcome's `constraint_node_id`. One additive migration V0023 (`idx_constraint_outcomes_code`, schema 22→23). **Single-source refactor:** shared `eventgraphGate` + `resolveFederationDefaults` helpers now back BOTH federation handlers. UATS `guidance_outcome_neighborhood.uats.json` (6/6 live, tagged `tsdb`). Live Tier-3 cross-checked CLI output against direct SQL (11 outcomes = 11). Outcomes recorded without a `constraint_code` aren't joinable (documented limitation). Follow-up: `--constraint-code` seeding (server-side code→node resolution). Sprint in `docs/development/eventgraph-002/`. Side-deliverable from Epic 7 live e2e: fix-commit `f307f55` in `internal/retrieval/scoring_rrf.go` sets `Activation: act[c.NodeID]` on the RRF-path RetrieveResult — the legacy `ScoreAndRank` path set this; the RRF path (default-on since Phase 13.1) silently dropped it, which caused `ApplyCoactivation` to filter out every L0 candidate (Activation=0 < `LearningMinActivation`=0.20) and the retrieve-time Hebbian goroutine to silently no-op for ~24 days. CO_ACTIVATED_WITH edges were still being written via sidecar paths (`CoactivateSession`, `ApplySymbolCoactivation`, consolidation walks). **All four Hebbian paths federated (Sprint EVENTGRAPH-003 — 2026-06-09):** `ApplySymbolCoactivation`, `CoactivateSession`, and `ApplyNegativeFeedback` (weaken-path) now feed `reinforcement_events` with their own `trigger_path` (`apply_symbol_coactivation` / `coactivate_session` / `apply_negative_feedback` — negative `delta_weight`), via `RETURN`-extension + parse-and-record hooks reusing the existing writer (no schema/writer/endpoint change; federation read surfaces them for free; Cypher edits `EXPLAIN`-validated + behavior-preserving). Contradict path (`CONTRADICTS`) deferred (not traversed by the federation walk). **Live-smoke correction to the note above:** `CoactivateSession` was NOT actually writing edges — it had **never been invoked**: `conversation.NewServiceWithConfig` left `learningService=nil` and `SetLearningService` had no caller, so `Observe()`'s nil-guard always skipped it → 0 conversation-observation `CO_ACTIVATED_WITH` edges ever in mdemg-dev. Fixed (commit `b3e61cb`): `convSvc.SetLearningService(lea)`. Sprint in `docs/development/eventgraph-003/`.

- **Model Distribution (Sprint MODEL-DIST-001 — 2026-05-11):** `mdemg model pull|list|verify|remove|where` is the canonical path for operators to obtain `mdemg-llm-v1`. Distribution channel is **Ollama Library** (v1; pluggable backends via `Fetcher` interface at `internal/cli/model_fetcher.go`). 3 fused GGUF quants published: `reh3376/mdemg-llm-v1:Q4_K_M` (9.0 GB, 12 GB RAM min), `:Q5_K_M` (11 GB, 14 GB RAM min — production canonical), `:Q8_0` (16 GB, 20 GB RAM min). `mdemg model pull` runs `ollama pull` under the hood, locates the GGUF blob via Ollama's manifest layout (`<OLLAMA_MODELS>/manifests/<OLLAMA_HOST>/<ns>/<name>/<tag>` → layer with `mediaType: application/vnd.ollama.image.model` → blob at `<OLLAMA_MODELS>/blobs/sha256-<digest>`), symlinks it to `<MDEMG_MODEL_DIR>/<name>.<quant>.gguf`, and SHA-verifies against the embedded `internal/cli/quant_manifest.json` (override via `MDEMG_MODEL_MANIFEST_PATH` for air-gapped). RAM auto-detection (darwin: `sysctl hw.memsize`; linux: `/proc/meminfo`) dispatches via `MDEMG_MODEL_RAM_TIERS` JSON. **Every operator-visible value is dynamic** (no-hardcoding rule, memory: `feedback_no_hardcoded_values.md`): 11 env vars + flag overrides — `MDEMG_MODEL_BACKEND|NAMESPACE|NAME|QUANTS|RAM_TIERS|QUANT|DIR|MANIFEST_PATH|ADAPTER_BASE`, plus ollama-standard `OLLAMA_MODELS` and `OLLAMA_HOST`. **Ollama is distribution only** — runtime stays `llama.cpp llama-server` (Phase 13.5), since Ollama runtime is broken on M5+macOS 26.3.x. Observability: TSDB V0021 `model_install_events` hypertable, one row per CLI op (synchronous single-row INSERT, not buffered — CLI is one-shot). Apple Silicon scope for v1; Linux/CUDA deferred. **Adapter-only path shipped in MODEL-DIST-002 (2026-05-25):** `mdemg model pull --adapter` resolves `reh3376/mdemg-llm-v1-adapter:latest` (257 MB GGUF LoRA, 560 tensors, target_base `qwen3:14b`); symlinks at `<MDEMG_MODEL_DIR>/<name>-adapter.gguf`; SHA-verifies against `mf.Adapter` in the embedded quant manifest. Adapter-pull manifest discovery filters on mediaType `application/vnd.ollama.image.adapter` (vs `image.model` for fused). Pipeline: MLX safetensors → `scripts/mlx_adapter_to_peft.py` → PEFT → `scripts/vendor/llama_cpp/convert_lora_to_gguf.py` (pinned to llama.cpp b9000 release) → GGUF LoRA. Load via `llama-server --model <base.gguf> --lora <adapter.gguf>`. Feature doc: `docs/features/local-model-distribution.md`. Sprint plans: `docs/development/model-dist-001/sprint_plan_model_dist_001.md`, `docs/development/model-dist-002/sprint_plan_model_dist_002.md`.

- **Compose embed:** `mdemg init` writes `docker-compose.yml` from `internal/cli/compose_templates/` (both files must stay in sync — CI checks this)
- **Hook port discovery:** Hooks auto-discover MDEMG server port at runtime (`.mdemg.port` → `.env` MDEMG_PORT → 9999). No URL baked at install time.
- **LaunchAgent embed:** `mdemg service install` reads templates from `packaging/launchd/` embedded via `embed.FS`
- **LLM recorder init order:** `llmclient.SetDefaultRecorder()` MUST be called BEFORE `api.NewServer()` — clients capture the recorder at construction time. See `internal/cli/serve.go` early writer block.
- **Context helpers:** `WithSpaceID(ctx, id)` and `WithSessionID(ctx, id)` carry request-scoped values to TSDB recording, overriding construction-time defaults in `recordInteraction()`
- **Sparse Retrieval Gate (Phase 14 → 14.1 → 14.1.1 — default-on):** Note 06 percentile-activation gate at `internal/retrieval/gate.go`. Phase 14.1.1 hybrid 120q PASSED (mean +0.003, 0 regressions, 10 improvements) with `SPARSE_MIN_ACTIVE=15` global + `data_flow_integration` per-category override at MIN=20 (handles q302's 4-required-files system-wide pattern). **Defaults flipped 2026-05-04**: `SPARSE_RETRIEVAL_ENABLED=true`, `SPARSE_MIN_ACTIVE=15`, `SPARSE_GATE_CATEGORY_OVERRIDES` seeded with `{"data_flow_integration": {"min_active": 20}}`. Operator opt-out: `SPARSE_RETRIEVAL_ENABLED=false`. Per-call override via `?sparse=true|false`, `?sparse_percentile=N`, or `?category=...` URL params. Gate fires post-aggregation pre-rerank; produces ~25% rerank-input reduction on most calls, full passthrough on data_flow_integration. Comparator eps=1e-6 fix in `uvts_ab_compare.py:121` eliminates floating-point boundary false-positives. V0019 `sparse_gate_metrics` hypertable persists per-call rows. 3 Prometheus histograms (`mdemg_sparse_gate_{active_count,dropped_fraction,threshold}`). See `docs/features/sparse-retrieval.md`.

- **Backend-agnostic env-var naming (Phase 13.6 — 2026-05-04):** Primary names migrated `MLX_*` → `LLM_*` for the watchdog suite. Legacy aliases kept with deprecation log at startup. New primaries: `LLM_WATCHDOG_ENABLED`, `LLM_PROBE_INTERVAL_SEC`, `LLM_PROBE_TIMEOUT_SEC`, `LLM_FAIL_FAST_ENABLED`, `MDEMG_ALLOW_NO_LLM`. Legacy `MLX_*` names still work but emit `WARN config: env var deprecated, please rename` at boot — operators should migrate. Aliases removable ≥1 release cycle from this commit. Internal Go package (`internal/mlxprobe/`) and Prometheus metric prefix (`mdemg_mlx_*`) retained — operator-invisible / dashboard-coupled. See `docs/features/mlx-watchdog.md`. Operates post-aggregation, pre-rerank: cuts the candidate list to those with score ≥ within-call percentile, with `MIN_ACTIVE` floor + `MAX_ACTIVE` ceiling. Defaults: `SPARSE_RETRIEVAL_ENABLED=false`, `SPARSE_ACTIVATION_PERCENTILE=0.95`, `SPARSE_MIN_ACTIVE=3`, `SPARSE_MAX_ACTIVE=20`. Per-request override via `?sparse=true|false` and `?sparse_percentile=N` URL params. Phase 14 16q quick PASSED at MIN=10/p95 (mean +0.019, 0 regressions); 120q full FAILED per-question (mean parity, 7 boundary regressions concentrated in `architecture_structure`). Ships flag-off per sprint plan §10 risk #1; Phase 14.1 (queued) will introduce `SPARSE_GATE_CATEGORY_OVERRIDES` for adaptive per-category MIN_ACTIVE before flipping default-on. Gate metadata + below-threshold candidates surface in `debug.sparse_gate_*` and `debug.below_threshold_*`. V0019 `sparse_gate_metrics` hypertable persists per-call rows (active_count, dropped_count, threshold_score, floor/ceiling fired, scorer_version) — Phase 14.1 retunes from this. 3 Prometheus histograms: `mdemg_sparse_gate_{active_count,dropped_fraction,threshold}`. Operators who opt in should set `SPARSE_MIN_ACTIVE=10` based on the 16q quick result and expect ~50% rerank-input reduction with mean parity (and 7 boundary regressions on the 120q lnl_demo corpus). See `docs/features/sparse-retrieval.md`.

- **Phase 13 Epic 6 audit-writer fix (Phase 14 in-flight):** V0017 `retrieval_audit` was empty since Phase 13 because `SetRetrievalAuditWriter` had zero callers. Phase 14 Epic 0 wired it: `internal/tsdb/retrieval_audit_writer.go` (buffered, 30s flush) + `retrievalAuditAdapter` in `internal/api/server.go`. `RETRIEVAL_AUDIT_ENABLED=true` in `.env` is now operational; live-verified with 5 retrieves → 3 audit rows landed (cache-hit retrieves bypass audit by design).

- **Context Fingerprinting (Phase 14.2 → 14.2.3 default-on):** Per-observation 256-bit sparse vectors that let retrieval discriminate the *same* MemoryNode in *different* contexts. **Default-on as of Phase 14.2.3 (2026-05-06)** — `CONTEXT_FINGERPRINT_ENABLED=true`, `RETRIEVAL_CONTEXT_COLUMN_ENABLED=true` after the per-category weight retune passed full 120q A/B (mean +0.009, std -0.023, 11 improvements, 0 regressions). Two-phase computation: observe-time (`Service.Observe` walks path + path-segments + role_type×layer + tags via `internal/conversation/fingerprint.go::ComputeContextFingerprintLocal`) + post-hoc refinement (`CycleOrchestrator` stage 6 walks CO_ACTIVATED_WITH for symbol bits, weekly cadence). Adaptive Builder (`internal/hidden/context_catalog_builder.go`) allocates 256 bits per-space: 32 bits → top-N (role_type, layer); 32 bits → top-N path-segment tokens (Phase 14.2.2 retune from LLM-summary tags; filter `freq ≤ total/2` drops monorepo skeleton tokens); 192 bits → full paths. **5th RRF column** (`internal/retrieval/column_context.go::ContextColumn`) ranks by Jaccard similarity, with **per-category weight overrides** (`RETRIEVAL_CONTEXT_COLUMN_CATEGORY_WEIGHTS` JSON env, default seed zero-weights `service_relationships`, `business_logic_constraints`, `relationship` per Phase 14.2.2 forensic). **Vector-based query→fingerprint derivation** (Phase 14.2.1, `internal/api/context_fingerprint.go`): when `?context=auto` URL param is set AND request has no explicit fingerprint, server embeds query and selects top-K (`CONTEXT_FINGERPRINT_QUERY_TOPK=8`) closest catalog refs by cosine sim. Per-(space, version) cache built lazily, refreshes on catalog version bump. **Strict mode** (`?strict_context=true`) drops candidates below `RETRIEVAL_CONTEXT_STRICT_THRESHOLD` (0.25 Jaccard) before scoring. Cache namespace `v1-rrf5|...|c=W|...|ctx=B|strict=T`. Stage 6 refresh hook gated on `CONTEXT_FINGERPRINT_REFRESH_ENABLED` (default true), `CONTEXT_FINGERPRINT_REFRESH_INTERVAL_HOURS=168` (weekly), `CONTEXT_FINGERPRINT_REFRESH_TIMEOUT_MS=60000`. Backfill CLI: `mdemg migrate context-fingerprint --space-id <id> [--build|--force-rebuild] [--dry-run] [--batch-size 500]` (idempotent). Schema: V0025+V0026+TSDB V0020. Operator opt-out: `RETRIEVAL_CONTEXT_COLUMN_ENABLED=false` (skips 5th column) or `CONTEXT_FINGERPRINT_ENABLED=false` (stops fingerprint computation). Feature doc: `docs/features/context-fingerprinting.md`.

- **Column-Voting Retrieval (Phase 13 + 13.1 default-on):** RRF aggregator over 4 columns (Embedding + BM25 + Graph + Structural; Temporal + RoleScoped deferred per Epic 0 data audit) with `consensus_strength` output signal. **Default `RETRIEVAL_COLUMN_VOTING_ENABLED=true`** since Phase 13.1 (2026-05-03) — embedding-heavy weights `0.50/0.20/0.15/0.15` passed full 120q UVTS A/B with mean +0.023 (+5.9%), 30 improvements, 2 boundary regressions in `business_logic_constraints`. Phase 13's failed equal-weights config (q 69 + q hard_sym_4 catastrophic regressions to 0.000) resolved — diagnosis showed Graph+Structural at equal weights crowded out Embedding+BM25 on precise-symbol queries. When enabled, `service.Retrieve` forks at the scorer call site, calls `Service.ScoreAndRankRRF` which runs 3 virtual columns (Embedding/BM25/Graph as presorted views over upstream `cands`) plus a true-parallel Structural Cypher walk. Cache namespace isolated via `Service.scorerVersion()` returning `"v0-linear"` or a hash like `"v1-rrf4|e=0.500|b=0.200|g=0.150|s=0.150|hops=2|emb=true|bm=true|gr=true|st=true"` — weight/hop/enable changes flip the namespace automatically. Fail-open to legacy on aggregator error. Per-column suppression knobs: `RETRIEVAL_COLUMN_{EMBEDDING,BM25,GRAPH,STRUCTURAL}_ENABLED`. Per-column weights: `RETRIEVAL_COLUMN_WEIGHT_{EMBEDDING,BM25,GRAPH,STRUCTURAL}`. Tunables: `RETRIEVAL_RRF_K` (60), `RETRIEVAL_STRUCTURAL_HOPS` (2), `RETRIEVAL_COLUMN_TIMEOUT_FRACTION` (0.8). Operator opt-out: `RETRIEVAL_COLUMN_VOTING_ENABLED=false` in `.env` + restart. Phase 13.2 (queued) will investigate `business_logic_constraints` regressions for per-category weight tuning. Ablation re-run recipe: use `scripts/phase13_1_ablation_runner.py --presets diagnostic-set --profile quick --baseline /tmp/uvts-baseline/grades.json` (or override `--presets` with custom dict).

- **Local LLM Runtime — llama.cpp llama-server (Phase 13.5 cutover):** Production LLM runtime is `llama-server` (llama.cpp b9000+, OpenAI-compat) at `127.0.0.1:8102` serving `mdemg-llm-v1.Q5_K_M.gguf`. Replaced `mlx_lm.server` (port 8101, decommissioned) on 2026-05-03 per the data-decided bake-off in `docs/development/post-ft-lora/phase_13_5_bakeoff_results.md`. Why migrated: mlx_lm.server is officially "not recommended for production" by its own maintainers and exhibited unbounded KV-cache → Metal OOM → SIGABRT crashes every ~14 min; llama.cpp has architecturally-bounded KV cache (`--ctx-size × --parallel`) and stays HTTP-alive on OOM (graceful HTTP 500 instead of process crash). Bake-off results: 0 crashes / 160 min / 301 calls; latency p50 17s → 3.0s (5.6× faster); UVTS quality at perfect parity (0.396 = 0.396).
  - **Production config**: `llama-server --model /Users/reh3376/mdemg/.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf --port 8102 --host 127.0.0.1 --ctx-size 32768 --parallel 4 --cont-batching --metrics --jinja`. KV cache bound: 32768 / 4 = 8K per slot; production ape.reflect prompts are ~5800 tokens.
  - **Always-on policy preserved**: framework's 16 LLM call sites depend on the endpoint. mdemg refuses to start if `cfg.EffectiveLLMEndpoint() + /models` is unreachable (preflight probe in `cli/preflight_mlx.go` — name still references mlx but probe is backend-agnostic). Operator escape hatch: `MDEMG_ALLOW_NO_MLX=1` (semantic now: "skip the LLM-endpoint preflight"; rename to `MDEMG_ALLOW_NO_LLM` is a follow-up sprint). Watchdog (`MLX_WATCHDOG_ENABLED=true` default) probes `/v1/models` every `MLX_PROBE_INTERVAL_SEC` (5) with `MLX_PROBE_TIMEOUT_SEC` (2). State machine `up → degraded → down` with hysteresis. Fast-fail gate at `llmclient/client.go:471` (`ErrMLXDown`) short-circuits retries when state=Down — eliminates retry-storm. Operator visibility: `mdemg watchdog status` (or `--json` for jq).
  - **launchd plist**: `~/Library/LaunchAgents/com.mdemg.llama-server.plist` (KeepAlive on crash, ThrottleInterval=30s). The old `com.mdemg.mlx-server.plist` is renamed `.disabled-phase13_5` (kept for emergency rollback).
  - **Model conversion path (MLX → GGUF)**: production model fine-tune is baked into MLX safetensors at `.local-models/mdemg-llm-v1/`. Conversion pipeline: (1) dequantize via `mlx_lm.fuse --dequantize` → bf16 HF safetensors at 28 GB; (2) `convert_hf_to_gguf.py --outtype f16` → 29.5 GB f16 GGUF; (3) `llama-quantize Q5_K_M` → 10.5 GB Q5_K_M GGUF. Total time ~5 min on M5 Max.
  - **Rollback**: restore `com.mdemg.mlx-server.plist` from `.disabled-phase13_5`, set `LLM_ENDPOINT=http://127.0.0.1:8101/v1`, bootstrap mlx-server plist, bootout llama-server plist. (Will reintroduce the 14-min crash cycle.)
  - **Why not mlx_lm.server stay-and-tune**: maintainer disclaimer is structural; `--max-kv-size` for the server has been an open issue since Nov 2025 (mlx-lm #615) and is NOT in 0.31.3.
  - **Why not Ollama**: definitively broken on M5 + macOS 26.3.x across 0.20.5–0.22.1 (8+ open issues, matmul2d static_assert).
  - **Why not LM Studio**: closed-source operability risk for a cognitive-substrate framework.
  - **Why not MLC-LLM**: lost the bake-off — F2 was 1.6× slower than F1 on every percentile, slight UVTS regression (-0.006), smaller community, hardware-target-locked TVM .dylib format.

## Testing

- UVTS validation (Phase 12 — Universal Validation Test Specification, semantic retrieval-quality):
  - `make test-uvts-lint` — schema-validate all `*.uvts.json` (CI-safe, no live deps)
  - `make test-uvts-quick BASE_URL=http://localhost:9999` — 16-question quick profile (~10 min)
  - `make test-uvts-full BASE_URL=http://localhost:9999` — full 120-question corpus
  - Direct: `python3 docs/tests/uvts/runners/uvts_runner.py --spec docs/tests/uvts/specs/lnl_demo_validation.uvts.json --base-url http://localhost:9999 --profile quick --persist-tsdb` (writes V0016 `uvts_runs` + `uvts_results`)
  - A/B harness: `python3 docs/tests/uvts/runners/uvts_ab_compare.py --baseline runA/grades.json --candidate runB/grades.json --spec <spec>.uvts.json --out verdict.json` (exit 0=pass, 1=fail, 2=drift). Apply Note 02 merge gate: B mean ≥ A mean AND no per-question regression > `ab_mode.regression_threshold_per_question` (default 0.10).

- UBENCH framework (Phase 10.5 — wraps the Phase 10 benchmark in the UxTS pattern):
  - `make test-ubench-lint` — schema-validate `docs/tests/ubench/specs/*.ubench.json` + verify config + holdout SHAs (CI-safe, no LLM)
  - `make test-ubench-contract` — lint + dataset↔holdout contract (every ULTS spec has ≥ `min_rows_per_task` golden rows). The forcing function preventing the Phase 10 guardrail.evaluate gap class.
  - `make test-ubench-run` — full benchmark execution; gates on `min_aggregate_weighted_score` + `max_truncated_rows`. Requires `http://127.0.0.1:8102/v1` reachable.
  - Pytest entry: `pytest docs/tests/ubench/contracts/ -v`
  - Spec: `docs/tests/ubench/specs/mdemg.ubench.json` (108 rows / 17 tasks / `min_rows_per_task=3`)
  - Feature doc: `docs/features/ubench-framework.md`

- Benchmark (Phase 10 automated framework, wrapped by UBENCH above): `python -m neural.benchmarks.run_benchmark --config configs/benchmark_phase10.yaml --out training_data/eval/benchmark_<run>.json`
  - Deterministic rewards via `neural.training.reward_functions.REWARD_REGISTRY`; optional LLM judge (`--enable-judge`, `gpt-5.4-mini`, fixed seed per run_idx); per-task variance + aggregate weighted score; V0012 TSDB persistence via `--persist-tsdb` (SQL sidecar)
  - ULTS specs: `docs/tests/ults/specs/*.ults.json` (17 tasks, `sampling_group ∈ {T,C,J}`)
  - **Eval choices** (per Phase 11.5c + 11.5d — `phase_11_5c_post.md`, `phase_11_5d_post.md`):
    - `valid_golden.jsonl` — Phase 10 baseline (108 rows, **99% leaked with training data**, kept for historical comparison only)
    - `valid_clean.jsonl` — Phase 11.5c production-derived eval (180 rows, 9 of 17 tasks, **0% leakage**, leak-audit gated). **Use this for honest baselines.** Pair with `--mlx-timeout-s 300` to avoid 60s timeouts on long production prompts.
  - **Row sweep (Phase 11.5d Epic 4 fix)** — runner now iterates ALL matched rows by default. Use `--rows-per-spec 0` (default; legacy single-row × n_runs is `--rows-per-spec 1`). For 9 valid_clean tasks × 20 rows × n_runs=2: ~360 calls/model = ~5-7 min Phase 5 base, ~30-40 min with LoRA adapter, ~18 min gpt-mini OpenAI.
  - **Production adapter** (Phase 11.5e + Phase 13.5 cutover): MLX form at `.local-models/qwen3-14b-mdemg-v1/` (research/training); **GGUF Q5_K_M form at `.local-models/mdemg-llm-v1-gguf/mdemg-llm-v1.Q5_K_M.gguf` (10 GB, production)**. Phase 5 dense base, no LoRA adapter. Aggregate **0.8389** on augmented eval (16 tasks), beats gpt-mini (0.8317), Run 7 (0.8307), Stage-1 distill (0.8294). Phase 13.5 quality-validated GGUF at perfect mean parity vs MLX (UVTS 0.396 = 0.396). Production-use: served via `llama-server --model <gguf-path> --port 8102 --ctx-size 32768 --parallel 4 --cont-batching --jinja` (see Local LLM Runtime section above). Stage-1 distill archived at `qwen3-14b-mdemg-v1-distill-stage1/` (rolled back from canonical in 11.5e); Run 7 archived at `-rl-run7/`. The 11.5d "Stage-1 at gpt-mini parity" was a 9-task-subset artifact; the 16-task augmented eval flipped the verdict.
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
