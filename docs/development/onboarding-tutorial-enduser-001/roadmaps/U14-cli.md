# U14 — CLI — The Full Command Surface (9 slides)

**Intent**: 90+ shipped CLI commands grouped into 5 operator-facing groups + inline help pattern + MCP alternative + no-hardcode-values rule in action.
**Target slides**: 9
**Mermaid**: 0 (tables carry it)

---

## U14.1 — Section Landing

**Title**: `U14 · CLI — The Full Command Surface`

**Body**:
- Intent: "`mdemg --help` lists 90+ commands. Operators regularly use ~15, grouped into 5 clusters. This section covers all 5 groups + the inline-help pattern + when to prefer MCP over CLI."
- Preview: Lifecycle → Memory → Introspection → Ops → Advanced → inline help → MCP alternative → tutorial-command → gotchas

**Provenance**:
- `internal/cli/root.go` — command registration + groups
- CLAUDE.md "Additional CLI Commands" section

---

## U14.2 — Lifecycle Group

**Title**: `Lifecycle — Starting, Stopping, Checking`

**Body**:
- Commands:
  - `mdemg init [--defaults|--quick] [--force]` — create `.env` + config (idempotent)
  - `mdemg start [--auto-migrate]` — launch server (native dev; `docker compose up -d` is the production path)
  - `mdemg stop` — graceful shutdown
  - `mdemg restart` — stop + start
  - `mdemg status` — PID, port, uptime, service state
  - `mdemg version` — version + git commit + build date
  - `mdemg healthz` — right-now subsystem probe (U2)
  - `mdemg tutorial [--url|--check]` — opens THIS deck (SERVE-001 E5)
- Group ID: `start` (per `root.go` AddGroup)

**Provenance**:
- `internal/cli/root.go`
- CLAUDE.md fresh-install Docker (primary) section

**note**:
- Operator cue: "You'll invoke these in your first 10 minutes with MDEMG and whenever you need to recycle the stack."

---

## U14.3 — Memory Group

**Title**: `Memory — Observe, Recall, Correct`

**Body**:
- Commands:
  - `mdemg data observe --space-id X --content "..." --obs-type Y` — writes an L0 node (U3)
  - `mdemg data recall --query "..." --space-id X` — reads via 4-column RRF (U3)
  - `mdemg data correct --space-id X --incorrect "X" --correct "Y" --context "Z"` — structured correction (U3)
  - `mdemg data clean --space-id X [--dry-run=false --force]` — purge errors + silent failures from TSDB (dry-run by default)
  - `mdemg data curate` — UAITS-driven training data curation (SFT/DPO/RAFT/curriculum)
  - `mdemg data validate` — UAITS spec schema validation
  - `mdemg data export [--instance-id X] [--out-dir Y]` — UTDS archive export
  - `mdemg data export-auto [--keep N]` — automated daily export with retention
- Group ID: `memory`

**Provenance**:
- `internal/cli/data_*.go`
- CLAUDE.md "Additional CLI Commands" + "Instance-id pin (INSTANCE-ID-PIN-001)"

**note**:
- Operator cue: "`observe` is the operator's version of the hook. `correct` is the structured version of `observe --obs-type=correction`."

---

## U14.4 — Introspection Group

**Title**: `Introspection — What's Going On?`

**Body**:
- Commands:
  - `mdemg synergy status [--space-id X]` — Claude Code ↔ MDEMG health
  - `mdemg synergy check [--auto]` — connectivity + hook integrity
  - `mdemg synergy migrate [--dry-run]` — one-time synergy data migration
  - `mdemg watchdog status [--json]` — LLM-endpoint state machine (U2)
  - `mdemg jiminy mode [--session-id X]` — view/toggle enforcement (U4)
  - `mdemg jiminy constraint {set-class|list-classes|seed-classes|mark|list-informational}` — rule-metadata ops (U4)
  - `mdemg hooks doctor` — hook-channel health triage
  - `mdemg hooks install [--path .claude/hooks]` — install/refresh hooks
  - `mdemg tsdb status` — TSDB connection + schema version
  - `mdemg config show [--filter X]` — current effective config (sensitive values masked)
  - `mdemg config validate` — three-way state (CONFIG-VALIDATE-TRANSIENT-DISTINGUISH-001)

**Provenance**:
- `internal/cli/*.go` across packages

**note**:
- Operator cue: "These answer 'am I set up correctly?' + 'is the system coherent?' questions."

---

## U14.5 — Ops Group

**Title**: `Ops — Backup, Upgrade, Maintenance`

**Body**:
- Commands:
  - `mdemg backup create|list|verify|restore` — full backup lifecycle (U9)
  - `mdemg upgrade [--docker-only|--no-docker]` — self-update binary + Docker images
  - `mdemg maintenance --space-id X [--dry-run=false --force]` — decay + prune cycle (U9)
  - `mdemg graph repair --space-id X` — weight-preserving dedup + vendor cleanup
  - `mdemg graph backfill-weights --space-id X` — fill null-weight edges (HIDDEN-WEIGHT-001)
  - `mdemg embeddings backfill --space-id X` — fill missing embeddings
  - `mdemg teardown [--export]` — destroy local Docker instances (with backup if `--export`)
  - `mdemg beta-import <bundle.tar.gz>` — import beta-tester bundle (maintainer-side)
  - `mdemg beta-janitor {delete|sweep}` — 30-day retention sweeps on beta-tester data
- Group ID: `advanced`

**Provenance**:
- `internal/cli/backup*.go`, `upgrade.go`, `beta_*.go`
- CLAUDE.md "Beta corpus loop (BETA-IMPORT-001)" + "HIDDEN-WEIGHT-001"

**note**:
- Operator cue: "`upgrade` is the one-stop refresh for binary + Docker containers. `teardown --export` is the destructive-but-safe path."

---

## U14.6 — Specialized Groups

**Title**: `Specialized — RSIC, Models, FT Loop, etc.`

**Body**:
- Commands (not day-to-day; reach for them when the specific topic calls):
  - **RSIC**: `mdemg rsic status`, `mdemg rsic reflect` (manual trigger)
  - **Models**: `mdemg model {pull|list|verify|remove|where}` — model distribution (MODEL-DIST-001/002)
  - **FT loop**: `mdemg ft-loop {report-stage|promote}` — recursive-retrain manual path (FT-RECURSIVE-001)
  - **Benchmarks**: `python -m neural.benchmarks.run_benchmark --config ... --apply-tsdb` (BENCH-SIDECAR-APPLY-001)
  - **Review**: `mdemg review {autograde|reverse|cadence}` — HITL CLI (U7)
  - **Eventgraph**: `mdemg eventgraph {reinforcement|guidance-outcome}-neighborhood` — federation reads (EVENTGRAPH-001/002)
  - **Confidence**: `mdemg confidence backfill` (HEBB-ETA-001 precision-weighted η)
  - **Concepts**: `mdemg concepts {repair|trace|recluster}` — hidden-layer surgery (HIDDEN-CHURN-001/002/003)
  - **Corrections**: `mdemg corrections rehydrate-structured` (JIMINY-STRUCTURED-CORRECTION-001)
  - **Symbols**: `mdemg extract-symbols` + `mdemg symbols analyze-go-implements` (GO-IMPLEMENTS-001)
  - **MCP**: `mdemg mcp` — standalone MCP server (MCP-REVIVE-001)

**Provenance**:
- CLAUDE.md across many sprint pins

**note**:
- Operator cue: "Most of these you'll touch RARELY. Pin this slide for 'what CLI exists for X?' lookup."

---

## U14.7 — Inline Help Pattern

**Title**: `--help Everywhere`

**Body**:
- Every command exposes `--help`:
  ```bash
  $ mdemg --help                       # top-level + groups
  $ mdemg data --help                  # subcommand group
  $ mdemg data observe --help          # leaf command flags + examples
  ```
- Convention per cobra: `Short` is the one-line summary; `Long` is the paragraph; `Example` is the recipe
- Flags include `--help` auto-wired
- Shell completion: `mdemg completion {bash|zsh|fish|powershell}` outputs the completion script

**Provenance**:
- Cobra convention + `internal/cli/*.go` command definitions

**note**:
- Operator cue: "ALWAYS `--help` before Googling. 90% of 'how do I ...' is answered inline."

---

## U14.8 — When To Use MCP Instead

**Title**: `MCP — The Agent-Facing Alternative`

**Body**:
- `mdemg mcp` is the Model Context Protocol server MDEMG ships — exposes the memory + agent surfaces as MCP tools
- When MCP wins over CLI:
  - You're an AI agent, not a human operator (hook flow)
  - You want programmatic access without shelling out (JSON-in, JSON-out)
  - You want streaming updates (MCP supports it; CLI mostly doesn't)
- 23 shipped MCP tools (MCP-REVIVE-001 census): memory (store/recall/associate/reject/reflect/status/symbols), ingest (trigger/status/cancel/jobs/files), linear CRUD, eventgraph neighborhoods, jiminy_strict, validate_changes, memory_space_freshness
- Contract suite: `internal/cli/mcp_contract_test.go` — new tools MUST add a contract

**Provenance**:
- CLAUDE.md "MCP memory channel unified (MCP-REVIVE-001)"
- `internal/cli/mcp.go` + per-tool handlers

**note**:
- Operator cue: "If you're writing a script that talks to MDEMG, prefer MCP tools over subprocess CLI invocations. Faster, structured, streaming-capable."

---

## U14.9 — Gotchas + Where To Go Deeper

**Title**: `CLI — Gotchas + More`

**Body**:
- Gotchas:
  - CLI reads `.env` via `godotenv.Load()` at command init — but ONLY from CWD; export shell env vars for cross-cwd invocations
  - Destructive CLIs (`data clean --force`, `teardown`, `maintenance --dry-run=false`) require explicit `--force` or `--dry-run=false`; dry-run default is a safety net
  - `--admin` flag presence: NEVER pass without explicit user authorization (CLAUDE.md governance pin)
  - CUIDv2 for ALL identifiers: `--run-id`, `--row-id`, `--grade-id`, etc. accept CUIDv2 only; UUID is TECH DEBT (15+ files still use google/uuid; migration ongoing)
  - CLI uses `golangci-lint v2.11.2` locally at `/Users/reh3376/go/bin/golangci-lint`; new commands lint-clean on the shipped `.golangci.yml`
- Feature docs:
  - CLAUDE.md "Additional CLI Commands" section
  - Individual feature docs under `docs/features/`
- Where this feeds into: U15 APIs (the HTTP + gRPC surfaces every CLI operates against), U16 Alerts (CLI exit codes correspond to alert severity for scripting)

**note**:
- Operator cue: "If you learn ONE pattern, learn `--help`. If you learn two, learn `--help` + the Memory group commands."
- Transition: "U15 next — the APIs + gRPC surfaces that EVERY /ui/ click + CLI invocation hits."
