# MDEMG Sidecar Roadmap

Status: Draft  
Date: 2026-02-28
Owner: MDEMG Core  
Primary Audience: Internal developer (MacBook + MacStudio) and future external adopters  
Related:
- `docs/specs/phase92-gap-analysis.md`
- `docs/specs/phase93-unified-cli-foundation.md`
- `docs/specs/phase94-config-project-init.md`
- `docs/specs/phase95-database-embedding-migrations.md`
- `docs/specs/phase96-ide-repo-integration.md`
- `docs/specs/phase97-process-lifecycle-security.md`

---

## 1. Objective

Build a sidecar package that lets a developer add MDEMG + CMS to any repository with minimal friction, including:

1. Self-installing runtime and dependencies.
2. Repo-local configuration and lifecycle management.
3. Agent integration for Claude Code and Codex.
4. Optional remote offload of heavy containers to MacStudio while controlling from MacBook.
5. Complete user documentation for installation, configuration, maintenance, troubleshooting, and upgrades.

The sidecar must work first for personal development workflows (beta dogfood), then harden into a public install path.

---

## 1A. Assumption Elimination Contract

This roadmap is normative. Coding agents implementing it must follow these rules:

1. `MUST` = required for completion.
2. `SHOULD` = recommended; deviations must be documented.
3. `MAY` = optional.

Implementation rules:

1. No silent fallbacks unless explicitly defined in this document.
2. Unknown or unsupported environments must return explicit, actionable errors.
3. Any inferred behavior not explicitly documented here must be written into an ADR or roadmap update before implementation.
4. All generated artifacts must include provenance metadata (generator version + timestamp + source command).
5. Any destructive change (hooks, adapter config, generated runtime files) requires backup-first behavior.

Non-negotiable failure policy:

1. If a required input is missing, fail fast with remediation guidance.
2. If an adapter target is unknown, do not guess paths; fail with adapter-specific diagnostic.
3. If remote profile prerequisites are not met, do not silently switch to local unless explicit fallback flag is set.

---

## 1B. Terminology and Canonical Meanings

To avoid language drift, these terms are authoritative:

| Term | Canonical Meaning |
|------|-------------------|
| Control host | Machine where developer invokes `mdemg sidecar *` (typically MacBook). |
| Runtime host | Machine where sidecar services run (MacBook for `local`, MacStudio for `studio-remote`). |
| Profile | Runtime placement policy (`local` or `studio-remote`) plus associated constraints. |
| Adapter | Versioned integration module that attaches sidecar tools/config to an agent environment. |
| Artifact | Any generated sidecar file under `.mdemg/` (`sidecar.yaml`, lock, reports, generated manifests). |
| State | Canonical lifecycle value from Section 7B. |
| Degraded | Runtime is partially available; command output must include cause + remediation guidance. |

Canonical naming rules:

1. `studio-remote` is the only supported remote profile name in initial release.
2. `claude-code` and `codex` are the only supported adapter names in initial release.
3. Any aliasing (`studio`, `remote`, `cc`, etc.) is disallowed unless explicitly documented in command help.

---

## 1C. UxTS-Consistent Planning Contract

This roadmap adopts UxTS-consistent planning behavior without requiring section-by-section mapping to external documents.

Planning rules:

1. Operating mode is explicitly declared as `brownfield` for this repository and all discovery/remediation expectations apply.
2. Discovery and implementation decisions must be evidence-backed and reproducible from repository artifacts.
3. Extension-first applies by default: extend existing sidecar/framework constructs before creating new ones.
4. Required behavior may be `enforced`, `advisory`, or explicitly unsupported with deterministic failure messaging; no silent ignore paths.
5. Verification outcomes and integrity outcomes are independent and must be reported separately.
6. Machine-readable outputs must use stable schemas, explicit error categories, and remediation-oriented diagnostics.
7. Drift checks must continuously validate docs, schemas, commands, artifacts, and runtime behavior for consistency.
8. Maturity and CI gate levels are progressive (`observe` -> `soft` -> `block`) and based on demonstrated stability.
9. High-priority remediation gaps must be implemented in-phase or explicitly waived with owner, due date, and interim mitigation.
10. Implementation context must be preserved through frequent journal updates and explicit assumption removal records.

Normative scope rule:

1. This roadmap is self-contained for sidecar execution. External UxTS documents are informative references, not normative requirements for this roadmap.

---

## 2. End-State User Experience

Target command flow (single binary interface):

```bash
# in any repo
mdemg sidecar init --profile studio-remote --agents claude-code,codex
mdemg sidecar install
mdemg sidecar up
mdemg sidecar doctor
mdemg sidecar attach-agent --agent claude-code
mdemg sidecar attach-agent --agent codex
```

Expected outcomes:

1. `.mdemg/sidecar.yaml` and generated runtime artifacts exist.
2. MDEMG API is reachable and healthy.
3. CMS endpoints are reachable and validated.
4. Agent MCP/tool configuration is attached with backups and conflict-safe merges.
5. Repo ingest and resume workflows are available with one command.

---

## 3. Scope

### In Scope

1. Sidecar lifecycle commands (`init/install/up/down/status/doctor/attach-agent/upgrade/uninstall`).
2. Local runtime profile (all services on current host).
3. `studio-remote` profile (heavy services on MacStudio; control from MacBook).
4. Repo-local artifacts, reproducible install, and idempotent reruns.
5. Claude Code and Codex integration adapters.
6. CMS bootstrap checks (`/v1/conversation/resume`, observe/recall health).
7. Dependency preflight and remediation guidance.
8. User-facing documentation pack with step-by-step installation, configuration, and maintenance guidance.

### Out of Scope (initial release)

1. Full multi-tenant hosted service.
2. Windows native support.
3. Kubernetes deployment.
4. Automatic installation of closed/proprietary IDE software.

---

## 4. Baseline Reality in Current Repo

Existing assets already reduce scope:

1. Unified CLI with major lifecycle commands (`mdemg init`, `mdemg db start/migrate`, `mdemg start/serve`, hooks management, MCP).
2. UxTS validation and drift tooling in Makefile (`verify-uxts-*`, UATS/UPTS/UOTS/UDTS/UBTS targets).
3. CMS endpoints and advanced capabilities implemented.
4. Docker compose and managed Neo4j flows already present.

Known gaps that still block sidecar packaging:

1. No dedicated sidecar command surface or manifest contract.
2. No release pipeline for public install (`goreleaser`, brew tap, curl installer).
3. No first-class remote runtime profile for MacStudio offload.
4. No codified adapter layer for Codex configuration.
5. Dependency management still partly manual (Python runner deps, environment assumptions).
6. One stale README reference (`scripts/install-hook.sh`) points to a non-existent file and can confuse installer behavior.

---

## 4A. Source of Truth Map (Implementation Context)

Agents must use these files as authoritative references during implementation:

| Domain | Source of Truth |
|--------|------------------|
| Unified CLI commands | `internal/cli/` |
| DB lifecycle behavior | `internal/cli/db.go` |
| Server lifecycle (`start/serve/status`) | `internal/cli/daemon.go`, `internal/cli/serve.go` |
| Hook install/uninstall behavior | `internal/cli/hooks.go`, `scripts/install-git-hook` |
| Init and IDE config generation | `internal/cli/init.go` |
| Core build/test entrypoints | `Makefile` |
| Current user onboarding narrative | `README.md`, `docs/features/unified-cli.md` |
| Sidecar governance patterns | This roadmap: Sections 1A, 1B, 1C, 4B, 6B, 7A, 7B, 7C, 7D, 8A, 11.6 |

Implementation directive:

1. If roadmap guidance conflicts with source code behavior, implementation must document the delta and resolve it via ADR before continuing.

---

## 4B. Agent Execution Checklist (No-Assumption Gate)

Before implementing sidecar code, an agent must complete this sequence:

1. Read and summarize the files listed in Section 4A.
2. Enumerate unresolved decisions from Section 10A and confirm no P0 blockers remain for the target phase.
3. Produce or update an implementation journal at `docs/sidecar/implementation-journal.md` with:
   - timestamp,
   - decisions made,
   - assumptions eliminated,
   - deferred questions.
4. Map planned code changes to roadmap sections (command contract, state machine, docs deliverables).
5. Refuse implementation when required contracts are missing; create roadmap/ADR delta first.

During long implementation sessions, the journal should be updated at least every 120 seconds or at each major decision boundary (whichever comes first) to preserve context across interruptions.

---

## 5. Architecture Model

### 5.1 Control Plane vs Data Plane

Control Plane (runs where developer works):

1. `mdemg sidecar` commands.
2. Repo config generation and agent attachment.
3. Health checks, diagnostics, upgrades.

Data Plane (where heavy services run):

1. MDEMG API runtime.
2. Neo4j container.
3. Optional embedder container/service.

Profiles map control plane to data plane:

1. `local`: both on same host.
2. `studio-remote`: control plane on MacBook, data plane on MacStudio.

### 5.2 Package Shape Decision

Default package shape: extend existing `mdemg` binary with `sidecar` subcommands.

Reasons:

1. Reuses current CLI, config, and lifecycle implementations.
2. Avoids introducing a second top-level binary and split ownership.
3. Simplifies distribution and upgrades.

---

## 6. Sidecar Artifact Contract

Repo-local artifacts (authoritative):

1. `.mdemg/sidecar.yaml` (profile, agent adapters, runtime settings, install state).
2. `.mdemg/sidecar.lock` (resolved versions, checksums, migration state).
3. `.mdemg/generated/` (generated compose/devcontainer/agent adapter fragments).
4. `.mdemg/logs/sidecar.log` (sidecar lifecycle logs).
5. `.mdemg/backups/` (agent config backups before mutation).

Artifact rules:

1. All generation must be idempotent.
2. Manual edits in generated files are overwritten unless explicitly marked as user-managed.
3. Any destructive rewrite requires backup and preview support.

---

## 6A. Explicit Defaults and Constants

To reduce assumption drift, these defaults apply unless overridden by config or flags:

| Item | Default | Override Path |
|------|---------|---------------|
| Sidecar profile | `local` | `sidecar.yaml: profile` |
| Sidecar endpoint | read `.mdemg.port`, fallback `http://localhost:9999` | `sidecar.yaml: runtime.endpoint` |
| Neo4j local bolt URI | `bolt://localhost:7687` | runtime config/env |
| Repo-local sidecar config | `.mdemg/sidecar.yaml` | `--config` (planned) |
| Sidecar lockfile | `.mdemg/sidecar.lock` | none |
| Generated artifact root | `.mdemg/generated/` | none |
| Sidecar logs | `.mdemg/logs/` | future `sidecar.yaml: logging` |
| Adapter backups | `.mdemg/backups/` | none |
| Space ID default strategy | repository basename | `sidecar.yaml: hooks.space_id_strategy` |

Default behavior for missing values:

1. Missing optional values use table defaults.
2. Missing required values fail validation with a field-specific error.
3. Generated defaults must be written explicitly into `sidecar.yaml` on `init` so runtime is transparent.

---

## 6B. Minimum `sidecar.yaml` Schema Contract

`sidecar.yaml` must contain these fields at minimum:

```yaml
version: "1"
profile: "local" # local | studio-remote
runtime:
  endpoint: "http://localhost:9999"
  remote:
    host: ""          # required when profile=studio-remote
    transport: "docker-context" # docker-context | ssh-exec
adapters:
  - name: "claude-code" # claude-code | codex
    enabled: true
hooks:
  space_id_strategy: "repo-basename"
install:
  auto_fix: true
```

Validation rules:

1. Unknown top-level keys must trigger warnings in `init` and hard failure in `install --strict`.
2. `profile=studio-remote` requires `runtime.remote.host`.
3. Adapter `name` must be unique within `adapters`.
4. `version` must match schema version supported by the running binary.
5. `install.auto_fix=false` must block any automatic remediation actions.

---

## 7. Command Surface (Planned)

1. `mdemg sidecar init`
2. `mdemg sidecar install`
3. `mdemg sidecar up`
4. `mdemg sidecar down`
5. `mdemg sidecar restart`
6. `mdemg sidecar status`
7. `mdemg sidecar doctor`
8. `mdemg sidecar attach-agent`
9. `mdemg sidecar upgrade`
10. `mdemg sidecar uninstall`

Behavioral requirements:

1. Every command must be safe to re-run.
2. All mutations support `--dry-run`.
3. `doctor` returns machine-readable JSON and human-readable remediation.

---

## 7A. Command Contract Matrix (Required)

Each command must implement explicit inputs, outputs, side effects, and exit semantics:

| Command | Required Inputs | Primary Outputs | Side Effects | Idempotent |
|---------|-----------------|-----------------|--------------|------------|
| `sidecar init` | profile, repo root | `sidecar.yaml`, initial status report | writes config and generated skeleton | Yes |
| `sidecar install` | valid config | install report | dependency checks, runtime prep, lock updates | Yes |
| `sidecar up` | installed state | runtime status + endpoint | starts services, writes runtime state | Yes |
| `sidecar down` | none | stopped status | stops services only | Yes |
| `sidecar restart` | none | runtime status | down + up transaction | Yes |
| `sidecar status` | none | machine/human status | none | Yes |
| `sidecar doctor` | none | diagnostics report | none (read-only) | Yes |
| `sidecar attach-agent` | adapter name | adapter change report | merges/writes agent config + backups | Yes (safe repeat) |
| `sidecar upgrade` | installed state | upgrade report | version updates, migrations | Yes (no-op if current) |
| `sidecar uninstall` | none | uninstall report | detach adapters, stop runtime, remove sidecar artifacts (policy-aware) | Yes |

Exit code contract (planned):

1. `0`: success.
2. `2`: validation/config error.
3. `3`: dependency or environment preflight error.
4. `4`: runtime orchestration error.
5. `5`: permissions/security policy error.
6. `6`: adapter unsupported or integration conflict.

---

## 7C. Required Flags and Output Artifacts

To keep behavior deterministic, the following command contracts are mandatory:

| Command | Required Flags | Required Artifacts/Output |
|---------|----------------|---------------------------|
| `sidecar init` | `--profile`, `--agents`, `--dry-run` | `.mdemg/sidecar.yaml`, init summary |
| `sidecar install` | `--dry-run`, `--no-auto-fix` | install report (`.mdemg/generated/install-report.json`) |
| `sidecar up` | `--profile` (optional override), `--dry-run` | runtime state update, endpoint summary |
| `sidecar down` | `--dry-run` | stop summary |
| `sidecar restart` | `--dry-run` | restart summary |
| `sidecar status` | `--format text|json` | status output (machine/human) |
| `sidecar doctor` | `--format text|json` | diagnostics report (`.mdemg/generated/doctor-report.json`) |
| `sidecar attach-agent` | `--agent`, `--dry-run`, `--print-only` | adapter report + backup manifest |
| `sidecar upgrade` | `--dry-run` | upgrade report |
| `sidecar uninstall` | `--dry-run`, `--retain-backups` | uninstall report |

Flag behavior rules:

1. `--dry-run` must produce identical validation logic without side effects.
2. `--format json` must produce stable field names (no human-only free text).
3. Commands that mutate files must emit a machine-readable change summary.

---

## 7D. Machine-Readable Report Contract (Required)

Every JSON report emitted by sidecar commands must include:

1. `schema_version`
2. `command`
3. `timestamp`
4. `result` (`success|warning|error`)
5. `exit_code`
6. `state_before`
7. `state_after`
8. `changes[]` (each entry: `path`, `action`, `backup_path?`)
9. `issues[]` (each entry: `code`, `severity`, `message`, `remediation`)
10. `next_actions[]`

Report contract rules:

1. Field names are stable across versions; additive fields require schema version bump.
2. `doctor` must never emit free-form-only diagnostics; all findings must appear in `issues[]`.
3. Reports must be persisted under `.mdemg/generated/` and printable to stdout.
4. JSON schemas for reports must be tracked under `docs/sidecar/schemas/`.

---

## 7B. Sidecar State Machine (Required)

Canonical sidecar states:

1. `uninitialized`
2. `initialized`
3. `installed`
4. `running`
5. `degraded`
6. `stopped`
7. `uninstalled`

Allowed transitions:

1. `uninitialized -> initialized` via `init`.
2. `initialized -> installed` via `install`.
3. `installed -> running` via `up`.
4. `running -> degraded` on health check failure.
5. `degraded -> running` after successful remediation + `up`/`restart`.
6. `running/degraded -> stopped` via `down`.
7. `stopped/installed -> uninstalled` via `uninstall`.

Invalid transitions must fail with explicit error messages and remediation hints.

---

## 8. Development Phases

### Phase S0: Product Contract and ADR Freeze (1 week)

Goal: lock unambiguous behavior before coding.

Deliverables:

1. Sidecar architecture decision record (ADR) for package shape and profiles.
2. Command contract with flags, examples, and expected status codes.
3. Artifact contract finalized.
4. Migration and rollback policy.

Blockers:

1. Ambiguous Codex configuration interface.
2. Open decision on remote runtime transport strategy.

Mitigations:

1. Adapter interface with versioned plugins (`claude-code-v1`, `codex-v1`).
2. Support dual remote mode from day 1: `docker-context` primary, `ssh-exec` fallback.

Exit criteria:

1. ADR approved.
2. No unresolved P0 interface ambiguity.
3. Command matrix (Section 7A) and state machine (Section 7B) are implemented in design docs.

### Phase S1: Sidecar Core Scaffolding (1-2 weeks)

Goal: land command framework and manifest handling.

Deliverables:

1. `mdemg sidecar` root command and subcommand stubs.
2. `sidecar.yaml` schema and validation.
3. `init` and `status` with dry-run and JSON output.
4. State persistence (`sidecar.lock`).

Blockers:

1. Backward compatibility with existing `.mdemg/config.yaml`.
2. Config precedence ambiguity between sidecar and core CLI.

Mitigations:

1. Explicit precedence table in docs and code.
2. Runtime warning when overlapping keys conflict.

Exit criteria:

1. Sidecar init and status tested on clean repo and existing repo.
2. State transition validation tests exist for invalid and valid transitions.

### Phase S2: Installer and Dependency Bootstrap (2 weeks)

Goal: automated preflight + install behavior.

Deliverables:

1. `install` command with preflight checks:
   - Docker availability and health.
   - Neo4j reachability.
   - Python/runtime dependencies required by enabled frameworks.
   - SSH reachability for remote profile.
2. Dependency remediation actions:
   - Safe auto-fix where possible.
   - deterministic manual instructions where not possible.
3. Install report artifact.

Blockers:

1. Environments with restricted package install permissions.
2. Offline environments.
3. Divergent Python environments.

Mitigations:

1. `--no-auto-fix` mode with exact commands.
2. Prebuilt dependency bundle support for offline mode.
3. Venv-managed runner dependencies under `.mdemg/venv` (optional).

Exit criteria:

1. One-command install works on clean MacBook host.
2. Repeat install is no-op except version drift updates.
3. Exit code contract implemented for all preflight failure classes.

### Phase S3: Runtime Orchestration - Local Profile (1 week)

Goal: fully functional local sidecar runtime.

Deliverables:

1. `up/down/restart` lifecycle for local profile.
2. Port allocation and collision handling.
3. Startup ordering with readiness checks (DB, API, MCP).
4. Structured runtime report (`status`, `doctor`).

Blockers:

1. Port conflicts.
2. Stale containers/volumes.
3. Partial startup failures.

Mitigations:

1. Dynamic port fallback with lockfile update.
2. Safe prune command scoped to sidecar resources only.
3. Transactional startup: if critical component fails, rollback optional.

Exit criteria:

1. Local profile stable across stop/start/restart cycles.

### Phase S4: Runtime Orchestration - Studio Remote Profile (2 weeks)

Goal: production-quality remote offload to MacStudio from MacBook.

Deliverables:

1. `studio-remote` profile in `sidecar.yaml`.
2. Remote transport modes:
   - `docker-context` (preferred).
   - `ssh-exec` fallback.
3. Endpoint projection:
   - stable local endpoint on MacBook for agents.
   - remote API/DB running on MacStudio.
4. Network reliability features:
   - connectivity probes.
   - reconnect logic.
   - degraded mode status.

Blockers:

1. SSH alias availability in non-interactive shells.
2. Docker context differences between machines.
3. MacStudio sleep/wake interruptions.
4. Remote path assumptions.

Mitigations:

1. Resolve explicit host in config (do not rely only on shell alias).
2. Validate and create named Docker context during install.
3. Startup health gate blocks "ready" status until remote stable.
4. Remote workspace sync contract documented and validated.

Exit criteria:

1. Heavy containers reliably run on MacStudio while tools run from MacBook.
2. `doctor` can diagnose at least 90 percent of common remote failures.

### Phase S5: Agent Adapter Layer (Claude Code + Codex) (2 weeks)

Goal: safe, repeatable attachment to supported agents.

Deliverables:

1. Adapter interface and registry.
2. `attach-agent --agent claude-code`:
   - MCP config generation/merge.
   - backup and restore.
3. `attach-agent --agent codex`:
   - equivalent tool attachment using supported config path/protocol.
4. `detach-agent` support for clean rollback.

Blockers:

1. Codex config format evolution.
2. Existing user-managed config conflicts.

Mitigations:

1. Adapter version pinning with compatibility checks.
2. Three-way merge strategy + backup before mutation.
3. `--print-only` mode for manual adoption.

Exit criteria:

1. Both agents can invoke MDEMG tools after one command attach.
2. Detach returns configs to pre-attach state.
3. Unsupported adapter versions fail with explicit compatibility errors (no path guessing).

### Phase S6: CMS Workflow Packaging (1 week)

Goal: make CMS immediately usable in daily workflows.

Deliverables:

1. Sidecar `doctor` CMS checks:
   - resume endpoint healthy.
   - observe endpoint healthy.
   - minimal response sanity.
2. Optional session-start helper script generation.
3. CMS quick profile defaults for personal dev usage.

Blockers:

1. Embedder unavailable (CMS semantic endpoints may degrade).

Mitigations:

1. Explicit degraded-mode diagnostics with remediation.
2. Optional local embedder bootstrap profile.

Exit criteria:

1. CMS resume/observe flows validated post-install in doctor report.

### Phase S7: Quality Gates and Hardening (2 weeks)

Goal: prevent regressions and unsafe installs.

Deliverables:

1. Sidecar UATS/UDTS/UBTS/UOTS smoke workflow coverage for sidecar commands.
2. Failure injection tests:
   - network drop.
   - remote unavailable.
   - missing dependencies.
3. Install idempotency tests.
4. Upgrade + rollback tests.

Blockers:

1. Flaky infrastructure tests.

Mitigations:

1. Controlled test fixtures and profile-specific test gating.
2. Separate blocking and non-blocking suites.

Exit criteria:

1. CI includes sidecar gating for core install/up/doctor scenarios.

### Phase S8: Distribution Pipeline (2 weeks)

Goal: external install channels.

Deliverables:

1. Release automation (`goreleaser` or equivalent).
2. Homebrew tap formula.
3. Curl installer with platform detection.
4. Signed checksums and release notes.

Blockers:

1. Cross-compilation complexity for cgo/tree-sitter dependencies.

Mitigations:

1. Build via Docker multi-arch or zig cc strategy.
2. Restrict initial supported matrix to macOS arm64 + amd64, Linux amd64.

Exit criteria:

1. Fresh machine install passes end-to-end acceptance script.

### Phase S9: Personal Beta and Public Readiness (3-4 weeks)

Goal: real-world validation before public promotion.

Deliverables:

1. Beta rollout in active personal repos.
2. Friction log and defect burn-down.
3. Public onboarding docs and troubleshooting matrix.
4. Documentation validation pass: all guides executed on clean environments and corrected for drift.

Exit criteria:

1. Setup median time < 10 minutes from clean repo.
2. Critical install failures < 5 percent across beta runs.
3. Remote profile stability acceptable for daily use.
4. Documentation walkthrough pass rate >= 95 percent for first-time setup attempts.

### Phase S10: Dynamic Port Allocation and Multi-Project Isolation (complete)

Goal: eliminate port collisions across concurrent sidecar instances.

Deliverables:

1. Dynamic port allocation with OS-level free port detection.
2. Per-project isolation via unique port and lock file binding.
3. Lock file records allocated port for downstream consumers.

Exit criteria:

1. Two sidecar instances in separate repos run simultaneously without conflict.

### Phase S11: Sidecar LLM Integration and Config Simplification (complete)

Goal: streamline embedding and LLM model configuration for sidecar workflows.

Deliverables:

1. Consolidated embedding model defaults (qwen3-embedding:4b via Ollama).
2. LLM config auto-detection and simplified YAML surface.
3. Doctor checks for Ollama model availability.

Exit criteria:

1. `sidecar init` produces working config without manual model configuration.

### Phase S12: Sidecar Upgrade and Uninstall Commands (complete)

Goal: replace upgrade and uninstall stubs with real implementations.

Deliverables:

1. `mdemg sidecar upgrade`: version drift detection, controlled upgrade cycle (down → install → up), `--dry-run`, `--skip-restart`, `--format json`.
2. `mdemg sidecar uninstall`: 7-phase cleanup, `--force` (stop running services), `--keep-data` (preserve Neo4j volume), safety backup to `.mdemg-backup-<timestamp>/`.
3. Full lifecycle coverage: init → install → up → doctor → restart → upgrade → down → uninstall.

Exit criteria:

1. Friction log F1 resolved — no manual workarounds needed.
2. All acceptance and integration tests pass with real commands.

### Phase S14: Documentation Cleanup — Stub Resolution (complete)

Goal: remove all stale stub references from docs and tests after S12 implementation.

Deliverables:

1. `maintenance.md` §3 and §6 updated with real upgrade/uninstall descriptions.
2. `faq.md` Q8 and Q13 updated to reflect real commands.
3. `sidecar-acceptance.sh` Step 8 replaced with upgrade/uninstall dry-run validation.
4. Integration test `TestSidecar_Stubs_NotImplemented` replaced with 5 real tests + 3 state guard entries.

Exit criteria:

1. Zero stale stub references in sidecar docs, acceptance test, and integration tests.
2. All tests pass: unit, integration, and acceptance.

### Phase S13: Embedding Model Migration (planned)

Goal: consolidate embedding model options to qwen3-embedding:4b (default, Ollama) and OpenAI (alternate). Remove the legacy llama/nomic-embed-text model path entirely.

Scope:

1. Remove llama embedding model option from all model selection logic.
2. Make `qwen3-embedding:4b` (Ollama) the default embedding model across the entire codebase.
3. Make OpenAI (`text-embedding-ada-002` / `gpt-4o-mini`) the alternate — not the default.
4. Update `ingest-codebase` handler and all other embedding consumers.
5. Update config defaults, `.env.example`, YAML config generation, init wizard.
6. Update Neo4j vector index dimensions if model output dimensions differ.
7. Update all documentation: architecture docs, config reference, installation guide, feature docs, API docs, contributing guide.
8. Update benchmark configs and test fixtures that reference embedding models.
9. Validate with benchmark run to confirm no retrieval quality regression.

Known touch points (non-exhaustive — full audit required at implementation time):

- `internal/embeddings/` — provider factory, ollama.go, openai.go, config struct
- `internal/config/config.go` — default model constants
- `internal/config/yaml_config.go` — YAML generation defaults
- `internal/cli/init.go` — wizard defaults
- `internal/cli/ingest.go`, `internal/api/handlers_ingest_codebase.go` — ingest embedding usage
- `cmd/ingest-codebase/main.go` — legacy binary
- `.env.example` — documented defaults
- `docs/architecture/03_Vector_Embeddings_and_Indexes.md`
- `docs/api/INGEST_CODEBASE_API.md`
- `migrations/V0003__vector_indexes.cypher` — index dimensions

Blockers: none known. Depends on qwen3-embedding:4b being stable and validated (already validated in S11).

Exit criteria:

1. Only two embedding model paths remain: Ollama (qwen3-embedding:4b) and OpenAI.
2. No references to llama embedding models in source code.
3. All tests pass with new defaults.
4. Benchmark confirms no retrieval quality regression.

---

## 8A. Documentation Deliverables (Required)

The final sidecar deliverable is incomplete without this documentation set.

1. `docs/sidecar/installation.md`
   - Step-by-step install guide for:
     - local profile
     - `studio-remote` profile (MacBook control, MacStudio runtime)
   - Prerequisites checklist
   - first-run verification steps
2. `docs/sidecar/configuration.md`
   - `sidecar.yaml` reference
   - profile examples
   - agent adapter configuration examples (Claude Code, Codex)
   - config precedence and override rules
3. `docs/sidecar/maintenance.md`
   - routine operations (`up/down/restart/status/doctor`)
   - upgrade procedure and rollback
   - backup/restore expectations
   - uninstall and cleanup
4. `docs/sidecar/troubleshooting.md`
   - symptom-to-remediation matrix
   - remote connectivity failures
   - dependency failures
   - agent attachment conflicts
5. `docs/sidecar/security-and-ops.md`
   - secret handling rules
   - remote host hardening recommendations
   - log safety and sensitive data policy
6. `docs/sidecar/faq.md`
   - common operational questions and decision guidance
7. `docs/sidecar/release-notes-template.md`
   - changelog format for upgrades and migration notes
8. `docs/sidecar/schemas/README.md`
   - JSON schema inventory and versioning policy
   - schema-to-command mapping
9. `docs/sidecar/implementation-journal.md`
   - chronological engineering decisions and eliminated assumptions
   - cross-reference to ADR IDs and roadmap section IDs

Documentation quality requirements:

1. Every command example must be copy-paste executable.
2. Every guide must include expected output/checkpoint signals.
3. Every failure mode in `doctor` must map to a troubleshooting entry.
4. Every profile must include known limitations and fallback paths.
5. All docs must be validated against at least one clean-machine runbook before release.
6. Schema docs and generated report examples must stay synchronized (CI check).

---

## 9. Exception and Failure Policy

The sidecar must handle exceptions predictably.

| Scenario | Default Behavior | Override |
|----------|------------------|----------|
| Existing non-MDEMG git hook | Do not overwrite, warn, provide merge instructions | `--force-hooks` |
| Existing agent MCP/config file | Backup + merge attempt; fail safe on conflict | `--replace-agent-config` |
| No Docker available | Fail install with remediation steps | `--profile api-only` (future optional) |
| Remote host unreachable | Mark runtime degraded, keep local control plane usable | `sidecar up --fallback-local` |
| Hash/lock mismatch | Require review and explicit reconcile | `sidecar install --reconcile` |
| Missing embedder | Continue with warning; CMS semantic features marked degraded | `--require-embedder` |
| Port collision | Auto-allocate free port and update lock/config | `--port` |
| No `rg` or optional tooling | Continue with fallback tool detection | `--strict-tools` |
| Unknown adapter config location | Fail with adapter diagnostics and suggested manual attach steps | `--print-only` |

---

## 10. Blockers and Deep Risk Register

### 10.1 Strategic Risks

1. Sidecar complexity exceeds perceived user value.
2. Codex integration contract instability.
3. Remote profile introduces non-deterministic failures.

### 10.2 Technical Risks

1. Cross-arch and cgo build/release failures.
2. Docker context assumptions across hosts.
3. Credential handling across local and remote contexts.
4. Drift between generated sidecar artifacts and actual runtime.

### 10.3 Operational Risks

1. MacStudio availability/sleep behavior affecting dev loops.
2. Silent configuration drift in agent files.
3. Incomplete uninstall leaving orphaned services.

### 10.4 Risk Mitigation Requirements

1. `doctor` must be authoritative and remediation-first.
2. Every destructive action requires backup and explicit confirmation unless non-interactive flags provided.
3. Every profile must have a rollback path (`down`, `detach-agent`, `uninstall`).
4. Release gating requires fresh-machine integration tests.
5. Open assumptions are tracked in the decision register (Section 10A) with owner and due date.

---

## 10A. Open Decision Register (Must Be Burned Down)

No phase beyond S1 may proceed with unresolved P0 decisions.

| ID | Decision | Priority | Owner | Due | Current Default |
|----|----------|----------|-------|-----|-----------------|
| `DEC-001` | Codex adapter config path/schema contract | P0 | MDEMG Core | S0 end | **Resolved** (ADR-0001): `.codex/config.toml`, TOML, `[mcp_servers.mdemg]` section |
| `DEC-002` | Remote transport precedence (`docker-context` vs `ssh-exec`) | P0 | MDEMG Core | S0 end | **Resolved** (ADR-0001): `docker-context` primary, `ssh-exec` fallback, configurable override |
| `DEC-003` | Uninstall retention policy for logs/backups | P1 | MDEMG Core | S1 end | retain backups, remove generated runtime artifacts |
| `DEC-004` | Offline dependency bundle format/versioning | P1 | MDEMG Core | S2 end | not enabled by default |
| `DEC-005` | CI syntax-validation approach for docs command examples | P2 | MDEMG Core | S7 end | best-effort lint + sample execution |

Decision handling rules:

1. Each decision must resolve to ADR, roadmap update, or explicit de-scope.
2. Implementation must not invent behavior for unresolved P0 decisions.

---

## 11. Acceptance Criteria

### 11.1 Functional

1. `mdemg sidecar init/install/up/doctor` works in a clean repo.
2. Local and `studio-remote` profiles are both supported.
3. Claude Code and Codex adapters attach and pass basic tool invocation checks.
4. CMS resume endpoint validation passes by default after `up`.
5. Command outputs and exit codes conform to Section 7A.
6. Required flags and generated output artifacts conform to Section 7C.

### 11.2 Reliability

1. Re-running `install` is idempotent.
2. `up/down/restart` can run repeatedly without manual cleanup.
3. Remote disconnection yields clear degraded status and recovery guidance.
4. State transitions conform to Section 7B and invalid transitions fail deterministically.

### 11.3 Safety

1. Existing hooks/configs are never silently overwritten.
2. Sidecar changes are reversible via backups and uninstall.
3. Sensitive material is not written to plaintext logs by default.

### 11.4 Distribution

1. Homebrew install path works for supported Mac architectures.
2. Curl installer works on clean supported hosts.
3. Upgrade and rollback flows are tested.

### 11.5 Documentation

1. Step-by-step installation guide exists and is validated for both local and `studio-remote` profiles.
2. Configuration guide exists with complete `sidecar.yaml` reference and examples.
3. Maintenance guide exists with routine operations, upgrade, rollback, and uninstall.
4. Troubleshooting guide exists and covers all `doctor` failure classes.
5. Documentation examples are CI-checked for command syntax drift where feasible.
6. Every assumption removed in implementation is reflected in docs and/or decision register.

### 11.6 Assumption Control and Traceability

1. `sidecar.yaml` validation enforces Section 6B with deterministic failures.
2. Command JSON outputs validate against published schemas in `docs/sidecar/schemas/`.
3. Implementation journal exists and records assumption removals throughout development.
4. Each implemented roadmap phase includes traceability links: requirement -> code path -> test -> documentation.

---

## 12. Milestone Schedule (Initial)

1. Week 1: S0 complete.
2. Weeks 2-3: S1 + S2.
3. Week 4: S3.
4. Weeks 5-6: S4.
5. Weeks 7-8: S5 + S6.
6. Weeks 9-10: S7 + S8.
7. Weeks 11-14: S9 beta and hardening.

Total initial roadmap: 14 weeks (aggressive but realistic for deep quality).

---

## 13. Immediate Next Actions (Execution Kickoff)

1. Create sidecar ADR (`docs/sidecar/adr-0001-sidecar-shape.md`) and lock command surface.
2. Define `sidecar.yaml` schema and state machine (`installed`, `running`, `degraded`, `stopped`).
3. Implement `mdemg sidecar init` + `status` before installer logic.
4. Add first smoke tests for sidecar commands in CI.
5. Resolve stale docs path mismatch for git hook install reference as part of onboarding quality.
6. Draft documentation skeleton files listed in Section 8A early to reduce late-stage documentation debt.
7. Add initial JSON schemas for `status`, `doctor`, and `install` reports in `docs/sidecar/schemas/`.
8. Start `docs/sidecar/implementation-journal.md` with S0 decision snapshots.

---

## 14. Definition of Done (Roadmap Complete)

This roadmap is complete when:

1. A developer can add MDEMG sidecar to any repo with one install flow.
2. Sidecar supports personal workflow with MacBook control and MacStudio offload.
3. Claude Code and Codex both use attached MDEMG functionality reliably.
4. Public install channels are operational and tested.
5. Beta usage has validated stability, reversibility, and developer value.
6. Required documentation pack (Section 8A) is complete, validated, and shipped with the release.
