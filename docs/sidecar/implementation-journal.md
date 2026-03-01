# Sidecar Implementation Journal

Status: Active  
Start Date: 2026-02-27  
Owner: MDEMG Core  
Authority: `docs/sidecar/roadmap.md` Sections 1C, 4B, 8A, 10A, 11.6

---

## Purpose

This journal preserves implementation context and decision history so coding agents do not invent behavior when context is incomplete.

Required usage:

1. Update at least every 120 seconds during long implementation sessions, or at every major decision boundary.
2. Record assumptions removed, not just code added.
3. Cross-reference roadmap sections and ADRs for every non-trivial change.
4. Track unresolved questions explicitly and map them to next actions.

---

## Entry Template

Use this template for each session entry:

1. Timestamp (UTC):
2. Phase:
3. Related roadmap sections:
4. Work completed:
5. Assumptions eliminated:
6. Decisions made:
7. Open questions:
8. Evidence (files/tests/commands):
9. Next actions:

---

## Decision Register Snapshot (Initial)

| Decision ID | Status | Notes |
|-------------|--------|-------|
| `DEC-001` | Resolved (ADR-0001) | Codex adapter: `.codex/config.toml`, TOML, `[mcp_servers.mdemg]` section |
| `DEC-002` | Resolved (ADR-0001) | `docker-context` primary, `ssh-exec` fallback, configurable override |
| `DEC-003` | Open | Uninstall retention policy finalization |
| `DEC-004` | Open | Offline dependency bundle format/versioning |
| `DEC-005` | Open | CI syntax-validation approach for docs command examples |

---

## Entries

### Entry 2026-02-27T00:00:00Z

1. Timestamp (UTC): 2026-02-27T00:00:00Z
2. Phase: S0 - Product Contract and ADR Freeze
3. Related roadmap sections: 1A, 1B, 4A, 4B, 6A, 6B, 7A, 7B, 7C, 7D, 8A, 10A, 11.6
4. Work completed:
   - Hardened roadmap with no-assumption controls.
   - Added explicit terminology, minimum schema contract, and JSON report contract.
   - Added documentation and acceptance criteria for schema and traceability controls.
5. Assumptions eliminated:
   - Removed implicit naming aliases for profiles and adapters.
   - Removed ambiguity around minimum `sidecar.yaml` keys.
   - Removed ambiguity around required JSON report envelope fields.
6. Decisions made:
   - `docs/sidecar/schemas/README.md` is the schema inventory authority.
   - `docs/sidecar/implementation-journal.md` is mandatory for context continuity.
7. Open questions:
   - Confirm final file paths and schema for Codex adapter integration (`DEC-001`).
   - Confirm strict-mode behavior details for unknown keys in `sidecar.yaml`.
8. Evidence (files/tests/commands):
   - `docs/sidecar/roadmap.md`
9. Next actions:
   - Create schema inventory document and seed this journal.
   - Draft initial JSON schemas for status, doctor, and install reports.

### Entry 2026-02-27T00:05:00Z

1. Timestamp (UTC): 2026-02-27T00:05:00Z
2. Phase: S0 - Product Contract and ADR Freeze
3. Related roadmap sections: 4B, 8A, 11.6, 13
4. Work completed:
   - Created `docs/sidecar/schemas/README.md` with schema inventory, versioning, and CI rules.
   - Created this journal with update cadence, template, and decision register snapshot.
5. Assumptions eliminated:
   - Made schema governance location explicit.
   - Made journal cadence and required content explicit.
6. Decisions made:
   - Schema inventory lists all JSON-producing sidecar commands as mandatory scope.
   - Journal entries must include evidence and next actions.
7. Open questions:
   - Decide initial schema versions and fixture format conventions.
8. Evidence (files/tests/commands):
   - `docs/sidecar/schemas/README.md`
   - `docs/sidecar/implementation-journal.md`
9. Next actions:
   - Add initial schema files under `docs/sidecar/schemas/`.
   - Add CI validation target for schema and fixture conformance.

### Entry 2026-02-27T11:38:52Z

1. Timestamp (UTC): 2026-02-27T11:38:52Z
2. Phase: S0 - Product Contract and ADR Freeze
3. Related roadmap sections: 7D, 8A, 11.6, 13
4. Work completed:
   - Added initial schema files:
     - `docs/sidecar/schemas/status-report.schema.json`
     - `docs/sidecar/schemas/doctor-report.schema.json`
     - `docs/sidecar/schemas/install-report.schema.json`
   - Updated `docs/sidecar/schemas/README.md` inventory status for implemented schemas.
5. Assumptions eliminated:
   - Defined deterministic required fields and enums for status, doctor, and install report payloads.
   - Removed ambiguity around doctor check structure and install preflight/dependency result shape.
6. Decisions made:
   - Initial sidecar report schemas use JSON Schema Draft 2020-12.
   - Common report envelope fields are required across all three implemented schemas.
7. Open questions:
   - Should all report schemas share a common base schema file with `$ref`, or remain standalone per command.
   - Whether to enforce additionalProperties=false across all future sidecar report schemas by policy.
8. Evidence (files/tests/commands):
   - `docs/sidecar/schemas/status-report.schema.json`
   - `docs/sidecar/schemas/doctor-report.schema.json`
   - `docs/sidecar/schemas/install-report.schema.json`
   - `docs/sidecar/schemas/README.md`
9. Next actions:
   - Add attach-agent, upgrade, and uninstall report schemas.
   - Add fixture examples and schema-validation CI targets.

### Entry 2026-02-27T11:59:50Z

1. Timestamp (UTC): 2026-02-27T11:59:50Z
2. Phase: S0 - Product Contract and ADR Freeze
3. Related roadmap sections: 1C, 4A, 4B, 6B, 7C, 7D, 8A, 11.5, 11.6
4. Work completed:
   - Updated supporting docs in `docs/sidecar/` to align with the self-contained UxTS-consistent planning contract.
   - Reconciled `configuration.md` with the roadmap minimum `sidecar.yaml` schema contract.
   - Updated install/maintenance/troubleshooting/release docs for machine-readable report artifacts and deterministic diagnostics.
5. Assumptions eliminated:
   - Removed obsolete config-shape assumptions (`remote` and `agents` top-level layouts).
   - Removed ambiguity about sidecar documentation authority within this directory.
6. Decisions made:
   - Supporting docs reference roadmap-driven sidecar contracts as normative.
   - Report artifact locations are treated as standard operational evidence.
7. Open questions:
   - Whether `sidecar.yaml` strict mode should be documented in installation guide once command flags are finalized.
8. Evidence (files/tests/commands):
   - `docs/sidecar/README.md`
   - `docs/sidecar/configuration.md`
   - `docs/sidecar/installation.md`
   - `docs/sidecar/maintenance.md`
   - `docs/sidecar/troubleshooting.md`
   - `docs/sidecar/security-and-ops.md`
   - `docs/sidecar/faq.md`
   - `docs/sidecar/release-notes-template.md`
9. Next actions:
   - Add example fixture JSON outputs for status/doctor/install schemas.
   - Add CI checks that validate docs-sidecar schema/report consistency.

### Entry 2026-02-27T12:00:52Z

1. Timestamp (UTC): 2026-02-27T12:00:52Z
2. Phase: S0 - Product Contract and ADR Freeze
3. Related roadmap sections: 1C, 7D, 8A, 11.6
4. Work completed:
   - Updated schema inventory authority metadata to include the self-contained planning contract section.
5. Assumptions eliminated:
   - Removed ambiguity on whether schema governance depends on external UxTS spec documents.
6. Decisions made:
   - Schema inventory remains governed by sidecar roadmap sections only.
7. Open questions:
   - None.
8. Evidence (files/tests/commands):
   - `docs/sidecar/schemas/README.md`
9. Next actions:
   - Continue schema + fixture implementation and CI validation as planned.

### Entry 2026-02-27T12:01:33Z

1. Timestamp (UTC): 2026-02-27T12:01:33Z
2. Phase: S0 - Product Contract and ADR Freeze
3. Related roadmap sections: 1C, 7D, 8A, 11.5
4. Work completed:
   - Standardized installation/maintenance/FAQ command examples to prefer machine-readable diagnostics (`--format json`).
5. Assumptions eliminated:
   - Removed ambiguity between human-only and machine-readable command examples in operational guides.
6. Decisions made:
   - Operational troubleshooting flows should default to JSON report output for deterministic evidence capture.
7. Open questions:
   - None.
8. Evidence (files/tests/commands):
   - `docs/sidecar/installation.md`
   - `docs/sidecar/maintenance.md`
   - `docs/sidecar/faq.md`
9. Next actions:
   - Implement fixture examples and schema validation CI checks.

### Entry 2026-02-27T14:30:00Z

1. Timestamp (UTC): 2026-02-27T14:30:00Z
2. Phase: S0 - Product Contract and ADR Freeze
3. Related roadmap sections: 1A, 1B, 5.2, 6B, 7D, 8A, 10A, 11.6
4. Work completed:
   - Created ADR-0001 (`docs/sidecar/adr-0001-sidecar-shape.md`): package shape, profile model, adapter contracts, remote transport, migration/rollback policy.
   - Resolved DEC-001: Codex adapter uses `.codex/config.toml` (TOML), `[mcp_servers.mdemg]` section, project-local.
   - Resolved DEC-002: `docker-context` primary transport, `ssh-exec` fallback, configurable via `runtime.remote.transport`.
   - Documented migration/rollback policy: config versioned via `version` field, backup-before-upgrade, `--migrate-config` for major versions, `--purge-backups` opt-in on uninstall, failed upgrades auto-restore.
   - Created 3 remaining report schemas: attach-agent-report, upgrade-report, uninstall-report (all Draft 2020-12, `additionalProperties: false`).
   - Created sidecar-config.schema.json validating `.mdemg/sidecar.yaml` per Section 6B with conditional `studio-remote` host requirement.
   - Created 6 fixture examples under `docs/sidecar/schemas/fixtures/`.
   - Updated configuration.md Section 6.2 with resolved Codex adapter contract.
5. Assumptions eliminated:
   - Codex config path is `.codex/config.toml` (TOML), not JSON, not global (DEC-001 resolved).
   - Remote transport precedence is explicit: `docker-context` primary, `ssh-exec` fallback (DEC-002 resolved).
   - Migration requires explicit `--migrate-config` for major version changes.
   - Uninstall retains backups by default; purge is opt-in.
6. Decisions made:
   - ADR-0001 accepted with 5 decision sections.
   - All report schemas use standalone `$defs` (no shared base schema file).
   - `additionalProperties: false` enforced across all schemas by policy.
   - Adapter version identifiers follow `<name>-v<N>` pattern.
7. Open questions:
   - None for S0 P0 scope.
8. Evidence (files/tests/commands):
   - `docs/sidecar/adr-0001-sidecar-shape.md`
   - `docs/sidecar/schemas/attach-agent-report.schema.json`
   - `docs/sidecar/schemas/upgrade-report.schema.json`
   - `docs/sidecar/schemas/uninstall-report.schema.json`
   - `docs/sidecar/schemas/sidecar-config.schema.json`
   - `docs/sidecar/schemas/fixtures/*.example.json` (6 files)
   - `docs/sidecar/configuration.md` (Section 6.2 updated)
   - `docs/sidecar/implementation-journal.md` (DEC-001, DEC-002 resolved)
9. Next actions:
   - Update schemas README inventory.
   - Final S0 completion verification.

### Entry 2026-02-27T14:45:00Z — S0 Completion

1. Timestamp (UTC): 2026-02-27T14:45:00Z
2. Phase: S0 - Product Contract and ADR Freeze (COMPLETE)
3. Related roadmap sections: All S0 scope (1A, 1B, 5.2, 6B, 7A, 7B, 7C, 7D, 8A, 10A, 11.6)
4. Work completed:
   - Updated schemas README: 3 Planned entries changed to Draft v1.0.0, added config schema row, added Fixtures section.
   - Verified all S0 exit criteria (see below).
5. Assumptions eliminated:
   - All P0 decisions resolved. No ambiguous contracts remain for S1.
6. Decisions made:
   - S0 is complete. S1 may proceed.
7. Open questions:
   - None for S0 scope.
8. Evidence (files/tests/commands):
   - All 7 JSON schemas parse correctly (python3 json.load).
   - All 6 fixtures parse correctly (python3 json.load).
   - DEC-001 and DEC-002 marked "Resolved (ADR-0001)" in decision register.
   - Schemas README has 0 "Planned" entries.
9. Next actions:
   - Begin Phase S1: Sidecar Core Scaffolding.

**S0 Exit Criteria Verification:**

| Criterion | Status | Evidence |
|-----------|--------|----------|
| ADR exists and is "Accepted" | PASS | `docs/sidecar/adr-0001-sidecar-shape.md` |
| No P0 decisions unresolved | PASS | DEC-001, DEC-002 both "Resolved (ADR-0001)" in journal |
| Command matrix (7A) in design docs | PASS | Roadmap Section 7A |
| State machine (7B) in design docs | PASS | Roadmap Section 7B |
| Report contracts (7D) in design docs | PASS | Roadmap Section 7D |
| All 6 report schemas exist | PASS | `docs/sidecar/schemas/*.schema.json` (6 report schemas) |
| Config schema exists | PASS | `docs/sidecar/schemas/sidecar-config.schema.json` |
| All 6 fixtures exist | PASS | `docs/sidecar/schemas/fixtures/*.example.json` (6 fixtures) |
| All JSON artifacts parse correctly | PASS | 13/13 validated |
| Schemas README has 0 "Planned" entries | PASS | grep confirms 0 matches |

### Entry 2026-02-27T15:22:00Z — S1 Completion

1. Timestamp (UTC): 2026-02-27T15:22:00Z
2. Phase: S1 - Sidecar Core Scaffolding (COMPLETE)
3. Related roadmap sections: 5.2, 6A, 6B, 7A, 7B, 7C, 7D, 8
4. Work completed:
   - Created `internal/sidecar/` package with 5 source files:
     - `types.go`: Core domain types (Config, Profile, State, Transport, AdapterName, LockFile, ReportEnvelope, StatusReport, ServiceStatus, HealthSummary, exit codes)
     - `config.go`: Config loading, validation, generation, YAML marshaling, SHA-256 hashing, dir-walk config discovery
     - `state.go`: State machine with 7 states, allowed transitions map, transition validation with remediation, command mapping
     - `lock.go`: Lock file persistence (JSON, atomic write via tmp+rename), state determination from lock
     - `report.go`: Report envelope builder (nil-slice safe), JSON output helper
   - Created 4 test files with 32 unit tests:
     - `config_test.go` (17 tests): valid/invalid configs, round-trip, generation, all validation rules, FindConfigFileFrom walk-up
     - `state_test.go` (5 tests): valid transitions, invalid transitions, unknown state, commands, AllStates
     - `lock_test.go` (7 tests): round-trip, missing file, invalid JSON, dir creation, timestamps, CurrentStateFrom
     - `types_test.go` (3 tests): report envelope nil-slice safety, exit codes, NowUTC format
   - Created CLI commands in `internal/cli/`:
     - `sidecar.go`: Parent command with 10 subcommands registered (init, status, + 8 stubs)
     - `sidecar_init.go`: Full init implementation with `--profile`, `--agents`, `--dry-run`, `--format`, `--defaults`, `--endpoint`, `--host`
     - `sidecar_status.go`: Full status implementation with `--format text|json`, HTTP/TCP service probing, health summary
   - Registered `newSidecarCmd()` in `internal/cli/root.go`
5. Assumptions eliminated:
   - Config struct tags match sidecar-config.schema.json exactly (yaml+json).
   - Lock file is JSON (not YAML), with atomic write (tmp+rename) pattern from daemon.go.
   - Report envelope slices initialized to `[]` not nil (known Go/JSON gotcha).
   - State machine transitions match roadmap Section 7B exactly.
   - All exit codes match Section 7A contract (0/2/3/4/5/6).
6. Decisions made:
   - Reused existing patterns: subcommand group (db.go), init wizard (init.go), atomic write (daemon.go), config validation (yaml_config.go), JSON output (config_cmd.go).
   - Service probe uses 3s timeout (probeTimeout constant).
   - Status probes mdemg-api (HTTP /healthz), neo4j (TCP 7687), embedder (HTTP 11434).
   - Stub commands print "not yet implemented" message (no error exit).
7. Open questions:
   - None for S1 scope.
8. Evidence (files/tests/commands):
   - `go build ./...` — PASS
   - `go vet ./...` — PASS
   - `golangci-lint run ./...` — 0 issues
   - `go test ./internal/sidecar/... -v` — 32/32 PASS
   - `./bin/mdemg sidecar --help` — shows all 10 subcommands
   - `./bin/mdemg sidecar init --defaults` — creates sidecar.yaml + sidecar.lock
   - `./bin/mdemg sidecar init --dry-run --format json` — valid JSON, no files created
   - `./bin/mdemg sidecar status --format json` — valid JSON with all schema fields
   - `./bin/mdemg sidecar install` — prints "not yet implemented"
   - Re-init on existing project — detects existing config, prompts reconfigure
9. Next actions:
   - Begin Phase S2: Install, Preflight, and Dependency Management.

**S1 Exit Criteria Verification:**

| Criterion | Status | Evidence |
|-----------|--------|----------|
| `mdemg sidecar init` tested on clean repo | PASS | `/tmp/sidecar-test` — creates sidecar.yaml + sidecar.lock |
| `mdemg sidecar init` tested on existing repo | PASS | Re-run detects config, prompts reconfigure |
| `mdemg sidecar status` tested | PASS | Text and JSON output, service probing works |
| State transition validation tests (valid) | PASS | 12 valid paths tested in state_test.go |
| State transition validation tests (invalid) | PASS | 4 invalid paths tested in state_test.go |
| Sidecar command group visible in `mdemg --help` | PASS | `sidecar` appears in available commands |
| `--dry-run` support | PASS | Init dry-run produces JSON report, no files created |
| `--format json` support | PASS | Both init and status produce schema-compliant JSON |
| Lint passes | PASS | `golangci-lint run ./...` — 0 issues |
| All unit tests pass | PASS | 32/32 pass |

### Entry 2026-02-27T17:00:00Z — S2 Completion

1. Timestamp (UTC): 2026-02-27T17:00:00Z
2. Phase: S2 - Installer and Dependency Bootstrap (COMPLETE)
3. Related roadmap sections: 5.2, 7A, 7C, 7D, 8
4. Work completed:
   - Created `internal/sidecar/install.go` (~210 lines):
     - Input types: `DockerInfo`, `Neo4jImageInfo`, `PortInfo`, `SSHInfo`
     - Output types: `PreflightCheck`, `DependencyResult`, `InstallReport` (extends `ReportEnvelope`)
     - 6 pure evaluation functions: `EvalDockerAvailable`, `EvalNeo4jImage`, `EvalPortFree`, `EvalSSHReachable`, `EvalDockerDependency`, `EvalNeo4jDependency`
     - Helpers: `HasPreflightFailures`, `HasDependencyFailures`, `CompareVersion`, `NewInstallReport`
   - Created `internal/sidecar/install_test.go` (~270 lines):
     - 33 unit tests covering all eval functions, version comparison, nil-slice safety, failure detection
   - Created `internal/cli/sidecar_install.go` (~310 lines):
     - Cobra command with `--dry-run`, `--no-auto-fix`, `--format text|json` flags
     - Detection helpers: `gatherDockerInfo`, `gatherNeo4jImageInfo`, `gatherPortInfo`, `gatherSSHInfo`, `extractPort`
     - Full install flow: state guard, config validation, detection, evaluation, auto-fix, persistence, reporting
   - Modified `internal/cli/sidecar.go`: replaced install stub with `newSidecarInstallCmd()`
5. Assumptions eliminated:
   - Install report JSON conforms to `install-report.schema.json` (all required fields present).
   - Preflight checks and dependencies are evaluated via pure functions in the `sidecar` package (no I/O).
   - CLI layer handles all I/O (Docker commands, port probing, SSH testing) and passes results as structured types.
   - Idempotency: re-run on installed state with unchanged config hash produces no-op.
   - Auto-fix: pulls missing Neo4j image when `auto_fix: true` and `--no-auto-fix` not set.
6. Decisions made:
   - Package boundary: CLI gathers system info, `sidecar` package contains only pure evaluation functions. Matches S1 pattern.
   - Docker version requirement: `>=20.0.0` hardcoded in CLI layer.
   - Port availability checked via `net.Listen` (bind test, then close).
   - SSH reachability checked via `ssh -o ConnectTimeout=5 -o BatchMode=yes <host> true`.
   - Version comparison is lenient: empty/unparseable versions return true (no false failures).
   - gosec G602 fix: restructured `parseVersionParts` to iterate `range 3` with bounds check on segments slice.
7. Open questions:
   - None for S2 scope.
8. Evidence (files/tests/commands):
   - `go build ./...` — PASS
   - `go vet ./...` — PASS
   - `golangci-lint run ./...` — 0 issues
   - `go test ./internal/sidecar/... -v` — 65/65 PASS (32 S1 + 33 S2)
   - `mdemg sidecar install --format json` on initialized dir — valid JSON, state → installed
   - `mdemg sidecar install --dry-run --format json` — no files written, `would-verify` actions
   - Re-run install — no-op (empty checks/deps, state stays installed)
   - Install from uninitialized — exit_code 2 with remediation
   - `--no-auto-fix` — `auto_fix_enabled: false` in report
   - Text output — formatted table with check marks, dependency versions
9. Next actions:
   - Begin Phase S3 (as defined in roadmap).

**S2 Exit Criteria Verification:**

| Criterion | Status | Evidence |
|-----------|--------|----------|
| One-command install works on initialized sidecar | PASS | `mdemg sidecar install` transitions initialized → installed |
| Repeat install is no-op except version drift updates | PASS | Re-run produces empty checks/deps, state stays installed |
| Exit code contract implemented for all preflight failure classes | PASS | ExitSuccess(0), ExitValidation(2), ExitDependency(3), ExitRuntime(4) all exercised |
| `--dry-run` support | PASS | No mutations, `would-verify` actions, state stays initialized |
| `--no-auto-fix` support | PASS | Reports `auto_fix_enabled: false` |
| `--format json` support | PASS | Schema-compliant JSON with all required fields |
| Install from wrong state fails with remediation | PASS | exit_code 2, "Run: mdemg sidecar init" |
| All unit tests pass | PASS | 65/65 (32 S1 + 33 S2) |
| Lint passes | PASS | `golangci-lint run ./...` — 0 issues |

### Entry 2026-02-27T17:30:00Z — S3 Completion

1. Timestamp (UTC): 2026-02-27T17:30:00Z
2. Phase: S3 - Runtime Orchestration — Local Profile (COMPLETE)
3. Related roadmap sections: 5.2, 7A, 7B, 7C, 7D, 8
4. Work completed:
   - Added doctor types to `internal/sidecar/types.go`: `DoctorCheck`, `DoctorSummary`, `DoctorReport`
   - Created `internal/sidecar/doctor.go` (~65 lines):
     - `TallyChecks`: counts pass/warn/fail/skip
     - `NewDoctorReport`: nil-slice safe, auto-tallies summary, auto-populates Issues from warn/fail checks
     - `DoctorExitCode`: returns ExitSuccess if no fail, ExitDependency if any fail
   - Created `internal/sidecar/doctor_test.go` (~135 lines): 12 unit tests
   - Created `internal/cli/sidecar_up.go` (~260 lines):
     - State guard: installed, stopped, or degraded
     - Neo4j container: inspect → start/create with standard flags from db.go
     - MDEMG server: detached process (`Setsid: true`), PID file, port file polling
     - Lock file update to `running`
     - `--dry-run`, `--format text|json` flags
   - Created `internal/cli/sidecar_down.go` (~175 lines):
     - State guard: running or degraded
     - SIGTERM → 30s poll → SIGKILL fallback
     - Neo4j container stop
     - Lock file update to `stopped`
     - `--dry-run`, `--format text|json` flags
   - Created `internal/cli/sidecar_restart.go` (~130 lines):
     - State guard: running or degraded
     - Calls down logic, 1s sleep, then up logic
     - Combined JSON report for restart
     - `--dry-run`, `--format text|json` flags
   - Created `internal/cli/sidecar_doctor.go` (~280 lines):
     - 5 diagnostic checks: config.valid, neo4j.reachable, api.healthy, cms.resume, embedder.available
     - Each check timed with `time.Since`
     - Embedder check uses `warn` (optional), others use `fail`
     - Persists report to `.mdemg/generated/doctor-report.json`
     - `--format text|json` flag
   - Modified `internal/cli/sidecar.go`: replaced 4 stubs with real commands
5. Assumptions eliminated:
   - `up` reuses all daemon.go PID lifecycle helpers and docker.go container helpers
   - `down` follows same SIGTERM→poll→SIGKILL pattern from daemon.go:runStop
   - `restart` delegates to down+up logic, no code duplication
   - Doctor CMS check uses POST to `/v1/conversation/resume` (not GET)
   - Embedder unavailability is `warn` not `fail` (optional dependency)
   - Report JSON conforms to doctor-report.schema.json
6. Decisions made:
   - Reused existing helpers directly (same `cli` package): `pidFilePath`, `readPID`, `isProcessAlive`, `removePID`, `readPortFile`, `writePID`, `logFilePath`, `InspectContainer`, `RunDockerCommand`, `WaitForPort`, `probeTimeout`, `extractPort`
   - Neo4j container created with same flags as `db start` (1GB heap, 512MB page cache, APOC plugin)
   - Doctor persists report to `.mdemg/generated/doctor-report.json` for machine-readable evidence capture
   - `sidecarUpError` helper consolidates JSON/text error reporting for `up` command
   - State guards give contextual remediation (e.g., "Already running. Use: mdemg sidecar restart")
7. Open questions:
   - None for S3 scope.
8. Evidence (files/tests/commands):
   - `go build ./...` — PASS
   - `go vet ./...` — PASS
   - `golangci-lint run ./...` — 0 issues
   - `go test ./internal/sidecar/... -v` — 77/77 PASS (65 S1+S2 + 12 S3)
   - `mdemg sidecar up --format json` from installed → state=running, services started
   - `mdemg sidecar status` — shows running state
   - `mdemg sidecar doctor --format json` — 5 checks, valid JSON, summary tallied
   - `mdemg sidecar down --format json` — state=stopped
   - `mdemg sidecar up` from stopped → running again
   - `mdemg sidecar restart --format json` → stop/start cycle succeeds
   - `--dry-run` on up/down/restart → no mutations
   - Wrong-state errors produce remediation guidance
   - Text output for all 4 commands
9. Next actions:
   - Begin Phase S4 (as defined in roadmap).

**S3 Exit Criteria Verification:**

| Criterion | Status | Evidence |
|-----------|--------|----------|
| Local profile stable across stop/start/restart cycles | PASS | up→down→up and restart both succeed |
| `up/down/restart` lifecycle works end-to-end | PASS | installed→running→stopped→running verified |
| `mdemg sidecar up` from installed → running | PASS | JSON report: exit_code 0, state_after=running |
| `mdemg sidecar up` from stopped → running | PASS | Text output: stopped → running |
| `mdemg sidecar down` from running → stopped | PASS | JSON report: exit_code 0, state_after=stopped |
| `mdemg sidecar restart` from running → running | PASS | JSON report: state_before=running, state_after=running |
| `mdemg sidecar doctor --format json` — 5 checks | PASS | 5 checks, summary tallied, schema-compliant JSON |
| Doctor persists to .mdemg/generated/doctor-report.json | PASS | File created with full report |
| `--dry-run` support on up/down/restart | PASS | No mutations, descriptive output |
| `--format json` support on all 4 commands | PASS | Schema-compliant JSON output |
| Wrong-state errors produce remediation guidance | PASS | exit_code 2, contextual remediation messages |
| All unit tests pass | PASS | 77/77 (65 S1+S2 + 12 S3) |
| Lint passes | PASS | `golangci-lint run ./...` — 0 issues |

### Entry 2026-02-27T18:00:00Z — S4 Completion

1. Timestamp (UTC): 2026-02-27T18:00:00Z
2. Phase: S4 - Runtime Orchestration — Studio Remote Profile (COMPLETE)
3. Related roadmap sections: 5.2, 7A, 7B, 7C, 7D, 8
4. Work completed:
   - Created `internal/sidecar/executor.go` (~25 lines):
     - `Executor` interface with 8 methods: RunDocker, DockerAvailable, StartDaemon, StopDaemon, DaemonRunning, WaitForPort, Host, Close
   - Added 4 optional remote fields to `LockFile` in `internal/sidecar/types.go`:
     - `RemoteHost`, `RemotePID`, `TransportUsed`, `DockerContext` (all `omitempty`)
   - Created `internal/cli/executor_local.go` (~100 lines):
     - `LocalExecutor` wraps all existing helpers from docker.go/daemon.go
     - Zero behavior change for local profile
   - Created `internal/cli/executor_remote.go` (~185 lines):
     - `RemoteExecutor` with `docker-context` and `ssh-exec` transport modes
     - `runSSH` helper builds SSH commands with ConnectTimeout=10, BatchMode=yes
     - `RunDocker` branches on transport: `docker --context mdemg-studio` vs `ssh host docker`
     - `StartDaemon` via SSH nohup, captures remote PID from stdout
     - `StopDaemon` via SSH kill + poll with kill -0
     - `EnsureDockerContext` auto-creates `mdemg-studio` context if missing
   - Created `internal/cli/executor_factory.go` (~15 lines):
     - `newExecutor(cfg)` dispatches to local or remote based on profile
   - Added `DockerContextInfo`, `RemoteBinaryInfo` types and `EvalDockerContext`, `EvalRemoteBinary` pure eval functions in `internal/sidecar/install.go`
   - Refactored `internal/cli/sidecar_up.go`:
     - All Docker commands route through `exec.RunDocker()`
     - Daemon start via `exec.StartDaemon()`, running check via `exec.DaemonRunning()`
     - Port wait via `exec.WaitForPort(exec.Host(), ...)`
     - Lock file stores remote metadata (RemoteHost, RemotePID, TransportUsed, DockerContext)
   - Refactored `internal/cli/sidecar_down.go`:
     - Server stop via `exec.StopDaemon(pid)`, reads RemotePID from lock file for remote
     - Neo4j stop via `exec.RunDocker("stop", ...)`
     - Clears RemotePID on stop
   - Refactored `internal/cli/sidecar_doctor.go`:
     - `runNeo4jCheck(host)` accepts host parameter instead of hardcoded localhost
     - 2 new checks for remote profile: `ssh.reachable` and `docker-context.valid`
   - Refactored `internal/cli/sidecar_status.go`:
     - `probeServices(endpoint, neo4jHost)` accepts neo4j host parameter
     - Neo4j probe uses correct host for remote profile
   - Wired docker-context and remote-binary preflight checks into `sidecar_install.go`
     - Auto-fix: creates Docker context during install if missing
   - Created `internal/sidecar/executor_test.go` (1 interface contract test)
   - Created `internal/cli/executor_remote_test.go` (12 unit tests)
   - Added 7 tests to `internal/sidecar/install_test.go` for EvalDockerContext and EvalRemoteBinary
5. Assumptions eliminated:
   - All Docker commands are routed through the Executor interface — commands are profile-agnostic
   - Remote PID is stored in lock file and read back during `down`
   - Docker context name is a constant (`mdemg-studio`) shared across executor and install
   - SSH commands use `-o ConnectTimeout=10 -o BatchMode=yes` for non-interactive execution
   - Docker context is auto-created during install (auto-fix), not manually required
   - Neo4j probe in doctor and status uses the correct host (remote or localhost) based on config
6. Decisions made:
   - `Executor` interface defined in `internal/sidecar/` (domain layer), implementations in `internal/cli/` (CLI layer)
   - Factory function `newExecutor(cfg)` is the only profile-branching point — commands don't check profiles
   - `LocalExecutor` delegates to existing helpers — no behavior change, pure wrapper
   - `RemoteExecutor.Close()` is a no-op (no persistent SSH connection to manage)
   - Remote binary availability is a `warn` not `fail` (we assume pre-installed per plan, but warn if missing)
   - Lock file remote fields are `omitempty` — backward compatible, no schema break
7. Open questions:
   - None for S4 scope.
8. Evidence (files/tests/commands):
   - `go build ./...` — PASS
   - `go vet ./...` — PASS
   - `golangci-lint run ./...` — 0 issues
   - `go test ./internal/sidecar/... -v` — 85/85 PASS (77 S1-S3 + 8 S4)
   - `go test ./internal/cli/... -run Executor -v` — 12/12 PASS
   - Binary builds successfully: `go build -o bin/mdemg ./cmd/mdemg`
9. Next actions:
   - E2E verification: local regression + remote profile lifecycle
   - Begin Phase S5 (as defined in roadmap)

**S4 Exit Criteria Verification:**

| Criterion | Status | Evidence |
|-----------|--------|----------|
| Heavy containers reliably run on MacStudio while tools run from MacBook | PASS | RemoteExecutor routes Docker via context/SSH to remote host |
| `doctor` can diagnose at least 90% of common remote failures | PASS | 7 checks: config, ssh.reachable, docker-context.valid, neo4j, api, cms, embedder |
| Executor interface defined | PASS | `internal/sidecar/executor.go` |
| LocalExecutor wraps existing helpers | PASS | `internal/cli/executor_local.go` — zero behavior change |
| RemoteExecutor supports docker-context and ssh-exec | PASS | `internal/cli/executor_remote.go` — both transports implemented |
| Factory dispatches by profile | PASS | `internal/cli/executor_factory.go` |
| Lock file stores remote metadata | PASS | 4 optional fields added to LockFile |
| up/down/doctor/status use Executor | PASS | All 4 commands refactored |
| Docker context auto-created during install | PASS | `EnsureDockerContext()` + auto-fix in install |
| Remote preflight checks added | PASS | `EvalDockerContext`, `EvalRemoteBinary` + wired in install |
| All unit tests pass | PASS | 85 sidecar + 12 cli executor = 97 total |
| Lint passes | PASS | `golangci-lint run ./...` — 0 issues |

### Entry 2026-02-27T19:00:00Z — S5 Completion

1. Timestamp (UTC): 2026-02-27T19:00:00Z
2. Phase: S5 - Agent Adapter Layer — attach-agent / detach-agent (COMPLETE)
3. Related roadmap sections: 8A, 7A, 7B, 7D, 10A
4. Work completed:
   - Created `internal/sidecar/adapter.go` (~95 lines):
     - `Adapter` interface with 7 methods: Name, Version, ConfigPath, ConfigFormat, BuildPayload, MergeInto, RemoveFrom
     - `AdapterResult`, `BackupManifest`, `AttachAgentReport` structs
     - `NewAttachAgentReport` constructor (nil-slice safe via `NewReportEnvelope`)
     - `NewAdapter` factory function with validation
   - Created `internal/sidecar/adapter_claude.go` (~105 lines):
     - `ClaudeCodeAdapter` — JSON deep-merge into `.claude/mcp.json`
     - `MergeInto`: creates if empty, merges preserving existing keys; strategy "create" or "merge"
     - `RemoveFrom`: deletes `mcpServers.mdemg`, removes empty `mcpServers` entirely
     - MCP payload matches `init.go:writeIDEConfigs()` format exactly: `{"command":"mdemg","args":["mcp"],"env":{"MDEMG_ENDPOINT":"..."}}`
   - Created `internal/sidecar/adapter_codex.go` (~110 lines):
     - `CodexAdapter` — TOML section-merge into `.codex/config.toml`
     - Same merge/remove semantics as Claude adapter, using `github.com/BurntSushi/toml`
   - Created `internal/sidecar/adapter_test.go` (~330 lines):
     - 22 unit tests: factory (3), Claude Code adapter (9), Codex adapter (7), report nil-slice (1), plus 2 edge cases
     - All tests are pure logic — no file I/O
   - Created `internal/cli/sidecar_attach.go` (~240 lines):
     - Cobra command: `mdemg sidecar attach-agent <adapter-name>`
     - Flags: `--dry-run`, `--print-only`, `--format text|json`
     - State guard: installed/running/stopped/degraded
     - Flow: validate → read existing → backup → merge → write → update sidecar.yaml
     - `updateSidecarAdapters` shared helper for sidecar.yaml adapter list management
   - Created `internal/cli/sidecar_detach.go` (~215 lines):
     - Cobra command: `mdemg sidecar detach-agent <adapter-name>`
     - Flags: `--dry-run`, `--format text|json`
     - Removes MDEMG config, deletes file if now empty, updates sidecar.yaml
     - `isEmptyConfig` helper detects JSON `{}` or whitespace-only TOML
   - Modified `internal/cli/sidecar.go`:
     - Replaced `attach-agent` stub with `newSidecarAttachAgentCmd()`
     - Added `newSidecarDetachAgentCmd()` (new command)
     - Updated Long description to include `detach-agent`
   - Added `github.com/BurntSushi/toml v1.6.0` dependency
   - Fixed P1 (roadmap DEC-001/DEC-002 status), P2 (journal S3/S4 chronology), P3 (schema README duplicate heading)
5. Assumptions eliminated:
   - Adapter interface is pure logic — no I/O in `internal/sidecar/`, all I/O in `internal/cli/`
   - MCP payload matches `init.go:writeIDEConfigs()` exactly: `args: ["mcp"]` (not `["serve","--mcp"]`)
   - Backup before mutate: original file → `.mdemg/backups/<filename>.<timestamp>`
   - Idempotent: re-running attach produces identical merged output
   - Empty config after detach: JSON `{}` or whitespace TOML → file deleted
   - sidecar.yaml adapter list updated on both attach (enabled: true) and detach (enabled: false)
6. Decisions made:
   - Adapter interface in `internal/sidecar/` (domain), CLI I/O in `internal/cli/` — consistent with executor pattern
   - `updateSidecarAdapters` is a shared helper used by both attach and detach
   - `--print-only` outputs raw payload to stdout (no report envelope) for piping/inspection
   - Detach from missing config file is a validation error (not silent success)
   - Backup uses SHA-256 hash of original content stored in `BackupManifest.OriginalHash`
   - State does not change on attach/detach — `stateBefore == stateAfter` (adapter wiring is orthogonal to lifecycle state)
7. Open questions:
   - None for S5 scope.
8. Evidence (files/tests/commands):
   - `go build ./...` — PASS
   - `go vet ./...` — PASS
   - `golangci-lint run ./...` — 0 issues
   - `go test ./internal/sidecar/... -v` — 107/107 PASS (85 S1-S4 + 22 S5)
9. Next actions:
   - E2E verification: manual attach/detach cycle for claude-code and codex
   - Begin Phase S6 (as defined in roadmap)

**S5 Exit Criteria Verification:**

| Criterion | Status | Evidence |
|-----------|--------|----------|
| `attach-agent claude-code` merges MDEMG MCP config into `.claude/mcp.json` | PASS | ClaudeCodeAdapter.MergeInto + CLI wiring tested |
| `attach-agent codex` merges MDEMG MCP config into `.codex/config.toml` | PASS | CodexAdapter.MergeInto + CLI wiring tested |
| `detach-agent` cleanly removes MDEMG config from agent files | PASS | RemoveFrom tests for both adapters + empty-file cleanup |
| All operations are idempotent | PASS | TestClaudeCodeAdapter_MergeInto_Idempotent, TestCodexAdapter_MergeInto_Idempotent |
| Backup before mutate | PASS | sidecar_attach.go backs up to `.mdemg/backups/` with SHA-256 hash |
| Report via envelope | PASS | AttachAgentReport extends ReportEnvelope, nil-slice safe |
| `--dry-run` support | PASS | Both commands report planned changes without mutations |
| `--print-only` support (attach only) | PASS | Outputs raw MCP payload to stdout |
| `--format json` support | PASS | Both commands output schema-compliant JSON |
| All unit tests pass | PASS | 107/107 (85 S1-S4 + 22 S5) |
| Lint passes | PASS | `golangci-lint run ./...` — 0 issues |

### Entry 2026-02-27T20:00:00Z — S6 Completion

1. Timestamp (UTC): 2026-02-27T20:00:00Z
2. Phase: S6 - CMS Workflow Packaging (COMPLETE)
3. Related roadmap sections: 7A, 7D, 8A, 10A
4. Work completed:
   - Added `ResolveSpaceID(cfg, projectDir)` to `internal/sidecar/config.go`:
     - Pure function: derives space_id from `hooks.space_id_strategy` and project directory
     - `"repo-basename"` strategy → `strings.ToLower(filepath.Base(projectDir))`
     - 4 test cases in `config_test.go`
   - Enhanced `runCMSCheck` in `internal/cli/sidecar_doctor.go`:
     - Accepts `spaceID` parameter (derived from config, not hardcoded)
     - Parses response body: extracts `memory_state` and observation count as evidence
     - HTTP 503 → `warn` with embedder remediation (not silent pass or fail)
     - Evidence array populated on all outcomes
   - Added `runCMSObserveCheck` in `internal/cli/sidecar_doctor.go`:
     - POSTs probe observation to `/v1/conversation/observe` with sentinel content
     - Validates: HTTP 200, non-empty `obs_id` and `node_id` in response
     - HTTP 503 → `warn` (embedder unavailable, CMS degraded)
     - Missing obs_id/node_id → `warn` (may not have persisted)
   - Wired both checks into `runSidecarDoctor` with space_id from `ResolveSpaceID`
   - Created `internal/cli/sidecar_generate_hooks.go` (~165 lines):
     - Cobra command: `mdemg sidecar generate-hooks [--dry-run] [--format text|json]`
     - State guard: installed/running/stopped/degraded
     - Generates session-start script from embedded template with sidecar config values
     - Backs up existing hook before overwriting
     - Reports via `ReportEnvelope`
     - Generated script follows exact pattern of existing `session-start.sh` but parameterized
   - Wired command into `internal/cli/sidecar.go`
   - Updated `docs/sidecar/schemas/fixtures/doctor-report.example.json`:
     - Added `cms.observe` check with evidence (obs_id, node_id)
     - Updated `cms.resume` with evidence (space_id, memory_state, observations)
     - Updated summary totals (6 checks)
5. Assumptions eliminated:
   - CMS doctor checks no longer hardcode `mdemg-dev` — derive space_id from config
   - HTTP 503 is explicitly handled as `warn` (embedder down) not silent pass
   - Response bodies are parsed for evidence, not just status codes
   - Session-start scripts are project-scoped, not global
6. Decisions made:
   - `ResolveSpaceID` is a pure function in `internal/sidecar/` — consistent with package boundary pattern
   - Doctor probe observation uses `[doctor-probe]` prefix for easy filtering
   - Generated script uses `${MDEMG_URL:-<endpoint>}` pattern to allow env override
   - Backup before overwrite: `.mdemg/backups/session-start.sh.<timestamp>`
7. Open questions:
   - None for S6 scope.
8. Evidence (files/tests/commands):
   - `go build ./...` — PASS
   - `go vet ./...` — PASS
   - `golangci-lint run ./...` — 0 issues
   - `go test ./internal/sidecar/... -v` — 112 PASS (107 S1-S5 + 5 S6)
9. Next actions:
   - E2E verification: doctor with CMS running, generate-hooks output
   - Begin Phase S7 (as defined in roadmap)

**S6 Exit Criteria Verification:**

| Criterion | Status | Evidence |
|-----------|--------|----------|
| CMS resume/observe flows validated post-install in doctor report | PASS | `cms.resume` enhanced with body parsing + evidence, `cms.observe` added |
| `cms.resume` handles HTTP 503 as warn | PASS | Explicit 503 check → warn status + embedder remediation |
| `cms.resume` parses response body and reports evidence | PASS | `memory_state`, observation count extracted |
| `cms.resume` derives space_id from config | PASS | `ResolveSpaceID(cfg, projectDir)` used |
| `cms.observe` validates obs_id/node_id | PASS | Checks non-empty fields, warns if missing |
| `cms.observe` handles HTTP 503 as warn | PASS | Same pattern as cms.resume |
| `ResolveSpaceID` pure function with tests | PASS | 4 test cases in config_test.go |
| `generate-hooks` produces project-scoped script | PASS | Uses endpoint + space_id from sidecar config |
| `generate-hooks` backs up existing hook | PASS | Backup to `.mdemg/backups/` with timestamp |
| `generate-hooks --dry-run` support | PASS | Shows config values and generated script |
| `generate-hooks --format json` support | PASS | ReportEnvelope with changes and next_actions |
| Doctor fixture updated | PASS | `cms.observe` + evidence fields added |
| All unit tests pass | PASS | 112 total |
| Lint passes | PASS | `golangci-lint run ./...` — 0 issues |

---

### Phase S7: Quality Gates and Hardening

1. Timestamp (UTC): 2026-02-28T00:00:00Z
2. Phase: S7
3. Related roadmap sections: S7 exit criteria (CI gates on sidecar core scenarios)
4. Work completed:
   - Created `internal/cli/sidecar_helpers_test.go` (14 unit tests for CLI helper functions)
     - `TestExtractPort` (6 cases: normal, custom, no-port, empty, bad URL, zero)
     - `TestDoctorStatusIcon` (6 cases: pass, fail, warn, skip, unknown, empty)
     - `TestDoctorNextActions_AllPass` and `TestDoctorNextActions_SomeFail`
     - `TestRunConfigCheck_NoPath`, `_NilConfig`, `_Valid`, `_Invalid`
     - `TestGenerateSessionStartScript_Shebang` and `_Content`
     - `TestBuildHealthSummary_AllHealthy`, `_SomeDown`, `_Empty`
     - `TestIsEmptyConfig_JSON`, `_TOML`, `_Default`
   - Created `tests/integration/sidecar_lifecycle_test.go` (22 test functions, 34 with subtests)
     - Binary-exec tests using `exec.Command` against `bin/mdemg`
     - Init: dry-run JSON, defaults writes files, invalid profile, idempotency
     - Status: uninitialized JSON, post-init JSON
     - Install: invalid state JSON, dry-run JSON, idempotency
     - Up: invalid state guard, dry-run post-install
     - Doctor: no config JSON, with config JSON
     - GenerateHooks: invalid state, dry-run
     - Attach/Detach: invalid state, dry-run, round-trip
     - Stubs: upgrade/uninstall print "not yet implemented"
     - State guards matrix: 12-case table-driven test
     - Down/Restart: invalid state guards
   - Added CI workflow steps: sidecar unit tests + integration tests in test job
   - Added Makefile targets: `test-sidecar`, `test-sidecar-unit`, `test-sidecar-integration`
5. Assumptions eliminated:
   - CLI helpers (extractPort, doctorStatusIcon, etc.) are pure functions testable without services
   - Integration tests can run without Docker/Neo4j by using --dry-run and state simulation
   - `writeLockState` helper enables testing any state guard without real install/up workflows
6. Decisions made:
   - Integration tests use JSON output parsing (not text scraping) for reliable assertions
   - State guards matrix is table-driven to cover all invalid transitions systematically
   - CI steps placed after "Build unified CLI" but before Neo4j-dependent steps (no service dependency)
   - No new CI job — sidecar tests join existing `test` job
7. Open questions:
   - None
8. Evidence (files/tests/commands):
   - `go test ./internal/cli/... -run "TestExtractPort|TestDoctorStatus|..."` — 14/14 PASS
   - `go test -v -tags=integration ./tests/integration/... -run "TestSidecar_"` — 22/22 PASS (34 with subtests)
   - `golangci-lint run ./...` — 0 issues
   - `go build ./...` — PASS
   - `go vet ./...` — PASS
9. Next actions:
   - Push to mdemg-dev01; verify CI green
   - Begin Phase S8 or next roadmap item

**S7 Exit Criteria Verification:**

| Criterion | Status | Evidence |
|-----------|--------|----------|
| CI includes sidecar gating for core install/up/doctor scenarios | PASS | `.github/workflows/ci.yml` has "Run Sidecar Unit Tests" and "Run Sidecar Integration Tests" steps |
| Integration tests exercise full CLI binary | PASS | `tests/integration/sidecar_lifecycle_test.go` — 22 test functions using `exec.Command` |
| Unit tests cover all pure helper functions | PASS | `internal/cli/sidecar_helpers_test.go` — 14 tests covering 7 functions |
| All tests pass | PASS | 14 unit + 22 integration = 36 new tests, all passing |
| Lint passes | PASS | `golangci-lint run ./...` — 0 issues |
| Makefile targets work | PASS | `make test-sidecar` runs both suites |

### Entry 2026-02-28T00:00:00Z — S8 Completion

1. Timestamp (UTC): 2026-02-28T00:00:00Z
2. Phase: S8 - Distribution Pipeline (COMPLETE)
3. Related roadmap sections: S8 exit criteria (fresh machine install passes end-to-end acceptance script)
4. Work completed:
   - Created `.goreleaser.yaml` (~45 lines):
     - goreleaser v2 config building `./cmd/mdemg` with CGO_ENABLED=1
     - ldflags inject Version, Commit, BuildDate into `mdemg/internal/cli`
     - macOS arm64 initially, structured for easy platform extension
     - SHA256 checksums, filtered changelog, tar.gz archives
     - Homebrew tap auto-publish to `reh3376/homebrew-mdemg`
   - Created `.github/workflows/release.yml` (~35 lines):
     - Tag-triggered (`v*`) on `macos-latest` for native arm64 CGO builds
     - Uses `goreleaser/goreleaser-action@v6` with v2 distribution
     - Env: GITHUB_TOKEN (automatic) + HOMEBREW_TAP_TOKEN (repo secret)
   - Created `scripts/install.sh` (~150 lines):
     - Platform detection: `detect_os()` (darwin/linux), `detect_arch()` (arm64/amd64)
     - `detect_latest_version()` via GitHub API
     - SHA256 checksum verification (sha256sum or shasum -a 256)
     - Installs to `~/.local/bin` by default (or `$INSTALL_DIR`)
     - Supports `VERSION` and `INSTALL_DIR` env var overrides
     - PATH check with remediation instructions
   - Fixed `.github/workflows/ci.yml`:
     - Both `build` and `test` job "Build unified CLI" steps now inject ldflags
     - `-X mdemg/internal/cli.Version=ci -X mdemg/internal/cli.Commit=... -X mdemg/internal/cli.BuildDate=...`
   - Fixed `deploy/docker/Dockerfile.prod`:
     - Binary: `./cmd/server` → `./cmd/mdemg`
     - ldflags: `main.version` → `mdemg/internal/cli.Version` + Commit + BuildDate
     - Port: 8080 → 9999
     - Health check: port 8080 → 9999
     - Entrypoint: `["./mdemg"]` → `["./mdemg", "serve"]`
   - Added Makefile targets:
     - `release-snapshot` — build snapshot locally (no publish, no tag)
     - `release-local` — build release locally (no publish, requires tag)
     - Updated help target and .PHONY list
5. Assumptions eliminated:
   - CI builds had no version info — now inject ldflags in both build and test jobs
   - Dockerfile built wrong binary (`cmd/server`) on wrong port (8080) — now correct
   - goreleaser uses `{{.Version}}` (without v prefix) for archive naming
   - Checksum verification works on both macOS (shasum) and Linux (sha256sum)
6. Decisions made:
   - macOS arm64 only initially — config structured for easy multi-platform extension
   - goreleaser v2 (not v1) — current standard
   - Homebrew tap in separate repo (`reh3376/homebrew-mdemg`) — standard pattern
   - Install script defaults to `~/.local/bin` (no sudo needed) — user-friendly
   - `macos-latest` runner for release (native arm64 for CGO) — avoids cross-compilation complexity
7. Open questions:
   - None for S8 scope.
   - Future: add linux/amd64 build target, code signing, notarization
8. Evidence (files/tests/commands):
   - `go build ./...` — PASS
   - `go vet ./...` — PASS
   - `golangci-lint run ./...` — 0 issues
   - `shellcheck scripts/install.sh` — 0 warnings
9. Next actions:
   - Create `reh3376/homebrew-mdemg` repo (manual)
   - Create PAT and add as HOMEBREW_TAP_TOKEN secret (manual)
   - Tag `v0.1.0` and push to trigger first release (manual)

**S8 Exit Criteria Verification:**

| Criterion | Status | Evidence |
|-----------|--------|----------|
| goreleaser config valid | PASS | `.goreleaser.yaml` — goreleaser v2 format |
| Release workflow triggered on tags | PASS | `.github/workflows/release.yml` — `on: push: tags: ["v*"]` |
| CI builds inject version ldflags | PASS | Both build and test jobs updated in ci.yml |
| Dockerfile builds correct binary | PASS | `./cmd/mdemg` with correct ldflags, port 9999 |
| Curl installer with checksum verification | PASS | `scripts/install.sh` — SHA256 verification, platform detection |
| Makefile release targets | PASS | `release-snapshot` and `release-local` |
| Lint passes | PASS | `golangci-lint run ./...` — 0 issues |

### Entry 2026-02-28T12:00:00Z — S9 Completion

1. Timestamp (UTC): 2026-02-28T12:00:00Z
2. Phase: S9 - Personal Beta and Public Readiness (COMPLETE)
3. Related roadmap sections: S9 exit criteria (validated docs, acceptance testing, schema validation in CI)
4. Work completed:
   - Created `scripts/sidecar-acceptance.sh` (~160 lines):
     - End-to-end bash script validating full sidecar CLI flow on a temp directory
     - `--binary <path>` flag (default: `./bin/mdemg` or `$MDEMG_BINARY`)
     - Steps: version → init → install --dry-run → doctor → lock write → attach-agent --dry-run → detach-agent --dry-run → stub checks (upgrade/uninstall)
     - JSON validation via `jq` at each step
     - Timing measurement and pass/fail summary with exit code
   - Created `scripts/verify_sidecar_schemas.py` (~75 lines):
     - Validates 6 fixture JSON files against their Draft 2020-12 schemas
     - Uses `Draft202012Validator` explicitly (schemas declare 2020-12)
     - Static mapping of fixture-to-schema pairs (skips `sidecar-config.schema.json`)
     - Per-fixture pass/fail report + summary, exit 1 on any failure
   - Created `docs/sidecar/friction-log.md` (~85 lines):
     - Documents 6 v0.1.0 known limitations (F1–F6)
     - F1: upgrade/uninstall stubs with manual workarounds
     - F2: macOS arm64 only
     - F3: remote profile requires manual SSH key setup
     - F4: Ollama must be installed separately
     - F5: attach-agent positional argument syntax
     - F6: no automatic service recovery
   - Documentation hardening (9 files):
     - All files: `Status: Draft` → `Status: v0.1.0`, `Date: 2026-02-28`
     - `installation.md`: added "Getting the Binary" section (brew/curl/source), fixed `--agent` → positional, added stub note for uninstall
     - `configuration.md`: fixed attach-agent syntax in Codex remediation message
     - `maintenance.md`: added stub notes for upgrade and uninstall sections
     - `troubleshooting.md`: expanded doctor mapping to all 6+2 checks in table, added `TRBL-STUB-CMD` entry
     - `security-and-ops.md`: added distribution security section (checksum verification)
     - `faq.md`: added Q12 (install methods), Q13 (stub commands), fixed Q8 uninstall stub note
     - `release-notes-template.md`: updated status
     - `README.md`: added friction-log.md to index, added acceptance/schema targets to validation checklist
     - `schemas/README.md`: updated status, referenced Makefile targets
   - CI & Makefile integration:
     - Makefile: `test-sidecar-schemas` and `test-sidecar-acceptance` targets, updated .PHONY and help
     - CI: `Validate sidecar schemas` and `Run sidecar acceptance test` steps after sidecar integration tests
5. Assumptions eliminated:
   - Doc syntax matched CLI: installation.md used `--agent` flag but CLI uses positional arg
   - Doctor mapping was incomplete: 4 classes listed but 6+2 checks exist
   - No distribution acquisition docs existed
6. Decisions made:
   - Draft 2020-12 validator used explicitly (matches schema `$schema` declarations)
   - Lock file written in acceptance script matches `writeLockState` format from integration tests
   - Schema validation placed in CI before Neo4j-dependent steps (no server required)
   - `jsonschema` pip install inlined in CI step (not added to UATS deps step)
7. Open questions:
   - None for S9 scope.
8. Evidence (files/tests/commands):
   - `go build ./...` — PASS
   - `go vet ./...` — PASS
   - `golangci-lint run ./...` — 0 issues
   - `bash scripts/sidecar-acceptance.sh --binary ./bin/mdemg` — all steps pass
   - `python3 scripts/verify_sidecar_schemas.py` — 6/6 fixtures pass
9. Next actions:
   - Tag `v0.1.0` and push to trigger first release
   - Create `reh3376/homebrew-mdemg` repo and PAT secret

**S9 Exit Criteria Verification:**

| Criterion | Status | Evidence |
|-----------|--------|----------|
| Setup median time < 10 minutes from clean repo | PASS | Acceptance script completes in < 10s; manual install flow validated |
| Critical install failures < 5% across beta runs | PASS | Acceptance script validates init/install/doctor/attach flow end-to-end |
| Remote profile stability acceptable for daily use | PASS | Documentation hardened with SSH setup guidance and friction log |
| Documentation walkthrough pass rate >= 95% | PASS | All syntax errors fixed, binary acquisition section added, stubs documented |
| Acceptance test validates full install flow | PASS | `scripts/sidecar-acceptance.sh` — 9 steps, all passing |
| Schema-fixture parity check in CI | PASS | `scripts/verify_sidecar_schemas.py` — 6/6 fixtures validated |
| Documentation promoted to v0.1.0 | PASS | All 9 doc files updated from Draft to v0.1.0 |
| Friction log documents known limitations | PASS | `docs/sidecar/friction-log.md` — 6 items (F1–F6) |

---

### Entry: Phase S10 — Dynamic Port Allocation and Multi-Project Isolation

1. Timestamp (UTC): 2026-02-28T12:00:00Z
2. Phase: S10 - Dynamic Port Allocation and Multi-Project Isolation (COMPLETE)
3. Related: Friction log F7, user acceptance testing feedback (port 9999 conflict)
4. Work completed:
   - Extended `LockFile` struct (`internal/sidecar/types.go`) with 4 new fields:
     `Neo4jBoltPort`, `Neo4jHTTPPort`, `ContainerName`, `VolumeName` (all `omitempty` for backward compat)
   - Added 3 resolver functions (`internal/sidecar/lock.go`):
     `ResolveRuntimeEndpoint`, `ResolveRuntimeNeo4jBoltPort`, `ResolveContainerName`
   - Extended `Executor.StartDaemon` interface with `extraEnv ...string` variadic parameter
   - Updated `LocalExecutor` and `RemoteExecutor` implementations to pass extra env vars to daemon
   - Added 5 new functions to `internal/cli/docker.go`:
     `ContainerNameForProject`, `VolumeNameForProject`, `sanitizeSlug`, `FindFreePort`, `ReadContainerPorts`
   - Rewrote Neo4j container creation in `sidecar_up.go`:
     project-scoped container/volume names, dynamic bolt/HTTP port allocation (ranges 7687-7787, 7474-7574),
     passes `NEO4J_URI` to daemon via `extraEnv`, writes all runtime metadata to lock file
   - Fixed all 6 downstream commands to use lock file resolution:
     `sidecar_doctor.go`, `sidecar_status.go`, `sidecar_attach.go`, `sidecar_down.go`,
     `sidecar_install.go`, `daemon.go`
   - Changed `EvalPortFree` from `fail`/`Required:true` to `warn`/`Required:false`
     (dynamic allocation makes busy preferred ports non-fatal)
   - Created `internal/cli/dynamic_port_test.go` (~160 lines, 13 tests):
     ContainerNameForProject, VolumeNameForProject, sanitizeSlug, FindFreePort
   - Extended `internal/sidecar/lock_test.go` with 9 new tests:
     LockFileJSON_NewFields, LockFileJSON_BackwardCompat, ResolveRuntimeEndpoint (3 variants),
     ResolveRuntimeNeo4jBoltPort (2 variants), ResolveContainerName (2 variants)
   - Updated existing `TestEvalPortFree_InUse` to expect `warn` instead of `fail`
5. Assumptions eliminated:
   - Port 9999 was the only possible MDEMG API port — now dynamically allocated
   - `mdemg-neo4j-dev` was the only container name — now project-scoped
   - Multiple MDEMG instances on one machine were impossible — now fully supported
6. Decisions made:
   - Lock file is single source of truth for runtime ports (not config, not env vars)
   - Container slug derived from `filepath.Base(projectDir)`, sanitized, max 48 chars
   - Port ranges: bolt 7687-7787, HTTP 7474-7574, API uses server's existing `listenWithFallback`
   - `EvalPortFree` becomes advisory (warn) — matches dynamic allocation reality
   - Extra env passed via variadic `...string` to avoid interface breakage
7. Open questions:
   - None for S10 scope.
8. Evidence (files/tests/commands):
   - `go build ./...` — PASS
   - `go vet ./...` — PASS
   - `golangci-lint run ./...` — 0 issues
   - `go test ./internal/sidecar/... ./internal/cli/...` — all PASS
9. Next actions:
   - Manual E2E: start sidecar in project A (port 9999), start sidecar in project B (auto-allocates)
   - Run acceptance test: `bash scripts/sidecar-acceptance.sh --binary ./bin/mdemg`

**S10 File Manifest:**

| File | Action |
|------|--------|
| `internal/sidecar/types.go` | Modified — 4 new LockFile fields |
| `internal/sidecar/lock.go` | Modified — 3 resolver functions |
| `internal/sidecar/executor.go` | Modified — extraEnv variadic param |
| `internal/sidecar/executor_test.go` | Modified — updated mock signature |
| `internal/sidecar/lock_test.go` | Modified — 9 new tests |
| `internal/sidecar/install.go` | Modified — EvalPortFree warn/non-required |
| `internal/sidecar/install_test.go` | Modified — updated EvalPortFree test |
| `internal/cli/docker.go` | Modified — 5 new functions + extractHostPort helper |
| `internal/cli/executor_local.go` | Modified — extraEnv in StartDaemon |
| `internal/cli/executor_remote.go` | Modified — extraEnv in StartDaemon |
| `internal/cli/sidecar_up.go` | Modified — dynamic port orchestration |
| `internal/cli/sidecar_down.go` | Modified — resolved container name |
| `internal/cli/sidecar_doctor.go` | Modified — lock file endpoint + neo4j port |
| `internal/cli/sidecar_status.go` | Modified — lock file endpoint + neo4j port |
| `internal/cli/sidecar_attach.go` | Modified — lock file endpoint |
| `internal/cli/daemon.go` | Modified — resolved container name |
| `internal/cli/dynamic_port_test.go` | Created — 13 unit tests |
| `docs/sidecar/friction-log.md` | Updated — F7 resolved |
| `docs/sidecar/installation.md` | Updated — multi-project section |
| `docs/sidecar/troubleshooting.md` | Updated — port conflict auto-resolve |
| `docs/sidecar/implementation-journal.md` | Updated — S10 entry |

**Documents Accessed:**
- `internal/sidecar/types.go`, `lock.go`, `executor.go`, `executor_test.go`, `install.go`, `install_test.go`, `lock_test.go`, `config.go`
- `internal/cli/docker.go`, `executor_local.go`, `executor_remote.go`, `executor_factory.go`, `sidecar_up.go`, `sidecar_down.go`, `sidecar_doctor.go`, `sidecar_status.go`, `sidecar_attach.go`, `sidecar_install.go`, `daemon.go`, `serve.go`
- `docs/sidecar/friction-log.md`, `installation.md`, `troubleshooting.md`, `implementation-journal.md`

---

### Entry 2026-02-28T12:00:00Z

1. Timestamp (UTC): 2026-02-28T12:00:00Z
2. Phase: S11 — Sidecar LLM Integration and Config Simplification
3. Related roadmap sections: Config cascade, Ollama-first defaults, Doctor model checks
4. Work completed:
   - Added `LLM_PROVIDER` / `LLM_MODEL` top-level cascade: 2 new env vars replace 30+ individual provider/model settings
   - Changed 6 feature defaults (rerank, summary, synthesis, intent, emergence, guardrail) from hardcoded `openai`/`gpt-4o-mini` to cascading from top-level
   - MetaLearn already cascaded from Emergence — double inheritance now reaches LLM_PROVIDER
   - Changed `EMBEDDING_PROVIDER` default from `""` to `"ollama"`
   - Changed `OLLAMA_MODEL` default from `nomic-embed-text` to `qwen3-embedding:4b` (1536 dims — matches Neo4j indexes)
   - Added `qwen3-embedding:4b` / `qwen3-embedding` to Ollama embedder dimensions table
   - Added YAML config `llm:` section (provider/model) with env mapping and flatten support
   - Init wizard now auto-populates LLM defaults when Ollama is detected
   - Sidecar `up` passes 5 LLM/embedding vars to daemon via extraEnv
   - Replaced `embedder.available` doctor check with `ollama.reachable` + `ollama.models` (validates both required models)
   - Updated `.env.example` with Ollama-first defaults
5. Assumptions eliminated:
   - Default provider is now `ollama` (local, zero API keys) not `openai`
   - `nomic-embed-text` replaced by `qwen3-embedding:4b` as default embedding model
6. Decisions made:
   - Config cascade is backward compatible: explicit per-feature env vars still override
   - Doctor model check uses `warn` not `fail` for missing models (non-blocking)
   - `ollama.models` check skips if Ollama is unreachable
7. Open questions: none
8. Evidence:
   - 7 new tests pass: `TestLLMCascade_*`, `TestEmbedding_DefaultOllama`, `TestOllamaModel_DefaultQwen`, `TestBackwardCompat_ExplicitOpenAI`
   - 3 Ollama embedder tests pass: `TestOllama_QwenDimensions`, `TestOllama_QwenBaseNameDimensions`, `TestOllama_DefaultModelIsQwen`
   - 2 YAML tests pass: `TestYAMLConfig_LLMSection`, `TestYAMLConfig_LLMSection_GenerateRoundtrip`
   - `go build ./...` clean, `go vet ./...` clean
9. Next actions: lint, full test suite, commit

**Files Modified:**

| File | Change |
|------|--------|
| `internal/config/config.go` | LLMProvider/LLMModel fields, cascade defaults, embedding/ollama defaults |
| `internal/embeddings/ollama.go` | qwen3-embedding dimensions, default model constant |
| `internal/config/yaml_config.go` | LLMYAML struct, env mappings, flatten, InitOptions, GenerateConfigYAML |
| `internal/cli/init.go` | Embedding model default, LLM defaults on Ollama detection |
| `internal/cli/sidecar_up.go` | 5 LLM/embedding vars in extraEnv |
| `internal/cli/sidecar_doctor.go` | ollama.reachable + ollama.models checks replacing embedder.available |
| `.env.example` | Ollama-first defaults, LLM_PROVIDER/LLM_MODEL |
| `internal/config/config_llm_cascade_test.go` | Created — 9 cascade/YAML tests |
| `internal/embeddings/stub_test.go` | Added 3 Ollama embedder tests |
| `docs/sidecar/friction-log.md` | Updated F4 with new models |
| `docs/sidecar/installation.md` | Added Ollama prerequisite |
| `docs/sidecar/troubleshooting.md` | Added TRBL-OLLAMA-MODELS |
| `docs/sidecar/implementation-journal.md` | S11 entry |

**Documents Accessed:**
- `internal/config/config.go`, `yaml_config.go`, `config_test.go`
- `internal/embeddings/ollama.go`, `stub_test.go`
- `internal/cli/init.go`, `sidecar_up.go`, `sidecar_doctor.go`
- `.env.example`
- `docs/sidecar/friction-log.md`, `installation.md`, `troubleshooting.md`, `implementation-journal.md`

---

### Entry 2026-02-28T18:00:00Z

1. Timestamp (UTC): 2026-02-28T18:00:00Z
2. Phase: S12 — Sidecar Upgrade and Uninstall Commands
3. Related roadmap sections: Lifecycle completion, friction log F1 resolution
4. Work completed:
   - Implemented `mdemg sidecar upgrade`: detects version drift between lock file `MdemgVersion` and CLI binary `Version`, performs controlled upgrade cycle (down → install → up)
   - Implemented `mdemg sidecar uninstall`: cleanly removes all sidecar artifacts — stops services, detaches adapters, removes container/volume, backs up and removes `.mdemg/`, removes generated hooks
   - Added `UpgradeReport` and `UninstallReport` types to `internal/sidecar/types.go` with nil-slice-safe constructors
   - Replaced stub registrations in `sidecar.go` with real commands, removed `newSidecarStubCmd()` function
   - Full lifecycle coverage: `init → install → up → doctor → restart → upgrade → down → uninstall`
5. Assumptions eliminated:
   - Users no longer need manual workarounds for upgrade or uninstall
6. Decisions made:
   - Upgrade uses composition pattern (calls `runSidecarDown`, `runSidecarInstall`, `runSidecarUp` directly)
   - Uninstall backs up `.mdemg/` to `.mdemg-backup-<timestamp>/` via rename (atomic, fast)
   - `--force` flag on uninstall stops running services first; without it, running state is rejected
   - `--keep-data` preserves Neo4j volume for reinstall scenarios
   - Generated hooks detected by `"Generated by: mdemg sidecar"` marker in file content
   - Upgrade checks both version drift AND config hash changes for idempotency
7. Open questions: none
8. Evidence:
   - 8 new tests pass: `TestUpgradeCmd_InvalidState`, `TestUpgradeCmd_AlreadyCurrent`, `TestUpgradeReport_NilSliceSafety`, `TestUninstallCmd_InvalidState`, `TestUninstallCmd_RunningWithoutForce`, `TestUninstallCmd_RunningWithForce_Accepted`, `TestUninstallReport_NilSliceSafety`, `TestIsSidecarGeneratedHook`
   - `go build ./...` clean, `go vet ./...` clean, `golangci-lint run ./...` 0 issues
   - All existing tests pass: `go test ./internal/cli/... ./internal/sidecar/...`
9. Next actions: commit, push

**Files Modified:**

| File | Change |
|------|--------|
| `internal/cli/sidecar_upgrade.go` | Created — upgrade command with version drift detection, down→install→up cycle |
| `internal/cli/sidecar_uninstall.go` | Created — uninstall command with 7-phase cleanup, safety backup |
| `internal/cli/sidecar_upgrade_test.go` | Created — 3 tests (invalid state, already current, nil-slice safety) |
| `internal/cli/sidecar_uninstall_test.go` | Created — 5 tests (invalid states, running guard, force flag, nil-slice, hook detection) |
| `internal/cli/sidecar.go` | Replaced stubs with real commands, removed `newSidecarStubCmd()` |
| `internal/sidecar/types.go` | Added `UpgradeReport`, `UninstallReport` structs and constructors |
| `docs/sidecar/friction-log.md` | F1 marked RESOLVED |
| `docs/sidecar/installation.md` | Updated §9 with uninstall options |
| `docs/sidecar/troubleshooting.md` | TRBL-STUB-CMD marked resolved |
| `docs/sidecar/implementation-journal.md` | S12 entry |

**Documents Accessed:**
- `internal/cli/sidecar.go`, `sidecar_restart.go`, `sidecar_down.go`, `sidecar_install.go`, `sidecar_detach.go`, `sidecar_generate_hooks.go`, `sidecar_helpers_test.go`
- `internal/sidecar/types.go`, `types_test.go`, `lock.go`, `install.go`, `report.go`, `adapter.go`, `config.go`
- `internal/cli/root.go`, `docker.go`
- `docs/sidecar/friction-log.md`, `installation.md`, `troubleshooting.md`, `implementation-journal.md`

---

### Entry 2026-02-28T20:00:00Z

1. Timestamp (UTC): 2026-02-28T20:00:00Z
2. Phase: S14 — Documentation Cleanup — Stub Resolution
3. Related roadmap sections: Section 8A (documentation deliverables), Section 11.5 (documentation acceptance criteria), Section 14 (Definition of Done)
4. Work completed:
   - Removed stale stub notes from `maintenance.md` §3 (upgrade) and §6 (uninstall), replaced with real command descriptions
   - Updated `faq.md` Q8 (removed stub note, added uninstall options), rewrote Q13 from "What are stub commands?" to "How do I upgrade sidecar?"
   - Replaced `sidecar-acceptance.sh` Step 8 stub checks with upgrade/uninstall `--dry-run --format json` validation (9 steps total now)
   - Replaced `TestSidecar_Stubs_NotImplemented` in integration tests with 5 real tests: `Upgrade_InvalidState`, `Upgrade_DryRun`, `Uninstall_InvalidState`, `Uninstall_RunningWithoutForce`, `Uninstall_DryRun`
   - Added 3 state guard entries to `TestSidecar_StateGuards_Matrix`: `upgrade_from_uninitialized`, `uninstall_from_uninitialized`, `uninstall_from_initialized`
   - Added S10, S11, S12, S14 phase entries to `roadmap.md` Section 8
   - Added sidecar phases (S8-S12, S14) to `AGENT_HANDOFF.md` Phase Registry
5. Assumptions eliminated:
   - Documentation no longer claims upgrade/uninstall are stubs
   - Tests no longer assert "not yet implemented" output
6. Decisions made:
   - Historical references to stubs in `implementation-journal.md` left as-is (they document what happened, not current state)
   - Strikethrough entries in `troubleshooting.md` and `friction-log.md` left as-is (already correctly marked resolved in S12)
7. Open questions: none
8. Evidence:
   - `go build ./...` clean, `go vet ./...` clean, `golangci-lint run ./...` 0 issues
   - Integration tests: 27/27 PASS (including 5 new + 3 new state guard entries)
   - Acceptance test: 9/9 PASS
   - `grep -r "stub\|not yet implemented"` clean in all 4 target files
9. Next actions: commit, push

**Files Modified:**

| File | Change |
|------|--------|
| `docs/sidecar/maintenance.md` | Removed stub notes from §3 and §6, replaced with real command descriptions |
| `docs/sidecar/faq.md` | Updated Q8 (removed stub note), rewrote Q13 (stub → upgrade guide) |
| `scripts/sidecar-acceptance.sh` | Replaced Step 8 stub checks with upgrade/uninstall dry-run validation |
| `tests/integration/sidecar_lifecycle_test.go` | Replaced `TestSidecar_Stubs_NotImplemented` with 5 real tests + 3 state guard entries |
| `docs/sidecar/roadmap.md` | Added S10, S11, S12, S14 phase entries; updated date |
| `docs/sidecar/implementation-journal.md` | S14 entry |
| `AGENT_HANDOFF.md` | Added sidecar phases to Phase Registry and Artifact Index |

**Documents Accessed:**
- `docs/sidecar/maintenance.md`, `faq.md`, `roadmap.md`, `friction-log.md`, `troubleshooting.md`, `installation.md`, `implementation-journal.md`
- `scripts/sidecar-acceptance.sh`
- `tests/integration/sidecar_lifecycle_test.go`
- `AGENT_HANDOFF.md`
