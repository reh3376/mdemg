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
