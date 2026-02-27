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
