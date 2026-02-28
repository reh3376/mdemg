# MDEMG Sidecar Configuration Guide

Status: v0.1.0
Date: 2026-02-28
Owner: MDEMG Core  
Audience: Developers and maintainers configuring sidecar behavior

---

## 1. Configuration Authority

Configuration behavior in this guide is aligned to `docs/sidecar/roadmap.md` and is self-contained for sidecar implementation.

---

## 2. Configuration Artifacts

Primary sidecar config files:

1. `.mdemg/sidecar.yaml` (authoritative runtime and adapter config).
2. `.mdemg/sidecar.lock` (resolved versions and generated state).
3. `.mdemg/generated/*` (derived runtime, adapter, and report files).

---

## 3. Configuration Precedence

Default precedence (lowest to highest):

1. Sidecar defaults.
2. `.mdemg/sidecar.yaml`.
3. Environment variables.
4. CLI flags.

When two values conflict, higher precedence wins and should be recorded in sidecar status output.

---

## 4. `sidecar.yaml` Reference (Normative Minimum)

```yaml
version: "1"
profile: "local" # local | studio-remote
runtime:
  endpoint: "http://localhost:9999"
  remote:
    host: "" # required when profile=studio-remote
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

1. Unknown top-level keys warn in `init` and hard fail in strict install mode.
2. `profile=studio-remote` requires `runtime.remote.host`.
3. Adapter `name` values must be unique in `adapters`.
4. `version` must match a schema version supported by the running binary.
5. `install.auto_fix=false` disables automatic remediation actions.

---

## 5. Profile Configuration Examples

## 5.1 Local

```yaml
version: "1"
profile: "local"
runtime:
  endpoint: "http://localhost:9999"
  remote:
    host: ""
    transport: "docker-context"
adapters:
  - name: "claude-code"
    enabled: true
  - name: "codex"
    enabled: true
hooks:
  space_id_strategy: "repo-basename"
install:
  auto_fix: true
```

## 5.2 Studio Remote

```yaml
version: "1"
profile: "studio-remote"
runtime:
  endpoint: "http://localhost:9999"
  remote:
    host: "macstudio-tb"
    transport: "docker-context"
adapters:
  - name: "claude-code"
    enabled: true
  - name: "codex"
    enabled: true
hooks:
  space_id_strategy: "repo-basename"
install:
  auto_fix: true
```

---

## 6. Agent Adapter Configuration

## 6.1 Claude Code

1. Include adapter entry: `- name: "claude-code"`.
2. Adapter writes/merges `.claude/mcp.json`.
3. Existing file must be backed up before mutation.

## 6.2 Codex

**Config path:** `.codex/config.toml` (project-local)
**Format:** TOML
**Adapter version:** `codex-v1`

The Codex adapter manages the `[mcp_servers.mdemg]` section within `.codex/config.toml`:

```toml
[mcp_servers.mdemg]
command = "mdemg"
args = ["serve", "--mcp"]
env = { MDEMG_ENDPOINT = "http://localhost:9999" }
```

Merge behavior:

1. **Create:** If `.codex/config.toml` does not exist and `.codex/` directory exists, create the file with the MDEMG section.
2. **Append:** If the file exists but has no `[mcp_servers.mdemg]` section, append the section.
3. **Update-in-place:** If the section already exists, update its values to match current sidecar config.

Prerequisites:

1. `.codex/` directory must exist. If missing, `attach-agent` fails with remediation:
   ```
   Error: .codex/ directory not found. Initialize Codex in this project first:
     codex init
   Then re-run: mdemg sidecar attach-agent codex
   ```

Backup:

1. Original file is copied to `.mdemg/backups/config.toml.<timestamp>` before any mutation.
2. `detach-agent` restores from backup or removes the `[mcp_servers.mdemg]` section.

Config path and schema version are logged in sidecar status and attach-agent reports.

---

## 7. Safe Configuration Change Workflow

1. Check current status: `mdemg sidecar status --format json`.
2. Edit `.mdemg/sidecar.yaml`.
3. Validate without mutation: `mdemg sidecar install --dry-run`.
4. Run diagnostics: `mdemg sidecar doctor --format json`.
5. Apply changes: `mdemg sidecar restart`.
6. Confirm runtime and adapter health.

---

## 8. Configuration Anti-Patterns

1. Editing generated files directly without marking override policy.
2. Changing remote transport without re-running `doctor`.
3. Adding undeclared top-level config keys and assuming they are enforced.
4. Defining duplicate adapter names and expecting deterministic attach behavior.
