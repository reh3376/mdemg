# Sidecar Report Schemas

Status: Draft  
Last Updated: 2026-02-27  
Owner: MDEMG Core  
Authority: `docs/sidecar/roadmap.md` Sections 1C, 7D, and 11.6

---

## Purpose

This directory is the source of truth for machine-readable JSON report schemas emitted by `mdemg sidecar` commands.

Goals:

1. Prevent contract drift between command output and downstream tooling.
2. Give coding agents deterministic field-level expectations.
3. Enforce stable upgrade behavior through explicit schema versioning.

---

## Schema Inventory

| Command | Schema File | Status | Notes |
|---------|-------------|--------|-------|
| `mdemg sidecar status --format json` | `status-report.schema.json` | Draft v1.0.0 | State, endpoint, service health |
| `mdemg sidecar doctor --format json` | `doctor-report.schema.json` | Draft v1.0.0 | Diagnostics and remediation issues |
| `mdemg sidecar install --format json` | `install-report.schema.json` | Draft v1.0.0 | Preflight checks, dependency actions |
| `mdemg sidecar attach-agent --format json` | `attach-agent-report.schema.json` | Draft v1.0.0 | Adapter changes and backup manifest |
| `mdemg sidecar upgrade --format json` | `upgrade-report.schema.json` | Draft v1.0.0 | Version transitions and migrations |
| `mdemg sidecar uninstall --format json` | `uninstall-report.schema.json` | Draft v1.0.0 | Removed artifacts and retained backups |
| (config validation) | `sidecar-config.schema.json` | Draft v1.0.0 | Validates `.mdemg/sidecar.yaml` per Section 6B |

If a command emits JSON and is not listed here, that is a contract violation.

---

## Required Common Envelope

All sidecar JSON report schemas must include the following top-level fields:

1. `schema_version`
2. `command`
3. `timestamp`
4. `result` (`success|warning|error`)
5. `exit_code`
6. `state_before`
7. `state_after`
8. `changes[]` (`path`, `action`, optional `backup_path`)
9. `issues[]` (`code`, `severity`, `message`, `remediation`)
10. `next_actions[]`

Command-specific fields may be added, but the common envelope is mandatory.

---

## Versioning Policy

1. Schema versioning uses semantic versioning in `schema_version`.
2. Additive optional fields require a minor version bump.
3. Field rename, type change, enum contraction, or required-field additions require a major version bump.
4. Patch versions are allowed only for clarifications that do not alter validation behavior.
5. Producers must emit a supported schema version; consumers must fail with clear remediation when unsupported.

---

## Validation and CI Requirements

1. Each command with JSON output must have at least one fixture report that validates against its schema.
2. CI must validate both:
   - schema correctness, and
   - fixture/output compliance.
3. The roadmap, this schema inventory, and command help output must stay synchronized.
4. Any schema change must include:
   - changelog entry,
   - updated fixtures, and
   - compatibility note.

---

## Change Control Checklist

## Fixtures

Example report files for each schema, located in `fixtures/`:

| Schema | Fixture | Scenario |
|--------|---------|----------|
| `status-report.schema.json` | `fixtures/status-report.example.json` | Healthy local profile, 3 services up |
| `doctor-report.schema.json` | `fixtures/doctor-report.example.json` | 5 checks, 4 pass + 1 warn (embedder optional) |
| `install-report.schema.json` | `fixtures/install-report.example.json` | Fresh install on local, 3 preflight checks pass |
| `attach-agent-report.schema.json` | `fixtures/attach-agent-report.example.json` | Claude Code attach with backup manifest |
| `upgrade-report.schema.json` | `fixtures/upgrade-report.example.json` | Version 1.0.0 to 1.1.0 with config migration |
| `uninstall-report.schema.json` | `fixtures/uninstall-report.example.json` | Full uninstall with backups retained |

Each fixture must validate against its corresponding schema. CI must enforce this.

---

## Change Control Checklist

Before merging schema updates:

1. Update schema file and bump `schema_version` appropriately.
2. Update this inventory table status and notes.
3. Regenerate or update fixtures for affected commands.
4. Validate examples in `docs/sidecar/` against updated contract.
5. Record the decision in `docs/sidecar/implementation-journal.md` and link an ADR if required.
