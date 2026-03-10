# MDEMG Sidecar Troubleshooting Guide

Status: v0.1.0
Date: 2026-02-28
Owner: MDEMG Core  
Audience: Developers and maintainers diagnosing sidecar failures

---

## 1. Diagnostic Workflow

Run diagnostics in this order:

```bash
mdemg sidecar status --format json
mdemg sidecar doctor --format json
```

Review generated reports:

```bash
cat .mdemg/generated/doctor-report.json
cat .mdemg/generated/install-report.json
```

If needed, collect logs:

```bash
tail -n 200 .mdemg/logs/sidecar.log
tail -n 200 .mdemg/logs/mdemg.log
```

---

## 2. Issue Matrix

| ID | Symptom | Likely Cause | Primary Fix |
|----|---------|--------------|-------------|
| `TRBL-INSTALL-DOCKER` | Install fails during preflight | Docker unavailable or daemon down | Start Docker, rerun install |
| `TRBL-INSTALL-CLI` | `mdemg` command not found | CLI not installed/in PATH | Install/build CLI, re-run |
| `TRBL-REMOTE-SSH` | Remote profile cannot start | SSH connectivity/auth issue | Validate SSH non-interactive access |
| `TRBL-REMOTE-CONTEXT` | Remote runtime state inconsistent | Docker context misconfigured | Recreate and select correct context |
| `TRBL-PORT-CONFLICT` | Runtime not reachable at configured endpoint | Port already in use | Automatic: `sidecar up` dynamically allocates free ports. Check `.mdemg/sidecar.lock` for actual ports |
| `TRBL-AGENT-CONFIG` | Attach-agent fails | Config merge conflict | Restore backup, run print-only attach, merge manually |
| `TRBL-CMS-DEGRADED` | CMS checks fail in doctor | Embedder/service dependency unavailable | Fix embedder config and restart |
| `TRBL-HOOK-CONFLICT` | Hook install skipped | Existing non-MDEMG hook present | Merge manually or use force policy |
| `TRBL-OLLAMA-MODELS` | Doctor reports missing Ollama models | Required models not pulled | Run `ollama pull` for each missing model |
| ~~`TRBL-STUB-CMD`~~ | ~~Command prints "not yet implemented"~~ | ~~Resolved in S12~~ | Both `upgrade` and `uninstall` are now fully implemented |

---

## 3. Exit Code Quick Map

| Exit Code | Class | Typical Action |
|----------|-------|----------------|
| `0` | Success | Continue |
| `2` | Validation/config error | Fix config fields and rerun |
| `3` | Dependency/environment error | Install/repair prerequisite and rerun |
| `4` | Runtime orchestration error | Inspect status/doctor logs, then restart |
| `5` | Permission/security policy error | Correct file/host permissions or policy flags |
| `6` | Adapter unsupported/conflict | Use print-only/manual attach or update adapter support |

---

## 4. Remote Profile Specific Checks

From MacBook:

```bash
ssh macstudio-tb "docker ps"
```

Verify:

1. SSH succeeds without interactive prompt loop.
2. Docker command on remote host works.

If using Docker context mode:

```bash
docker context ls
```

Ensure expected context exists and is healthy.

---

## 5. Agent Adapter Recovery

If attachment mutates config unexpectedly:

1. Locate backup in `.mdemg/backups/`.
2. Restore backup file.
3. Re-run attach in safe mode (`--print-only` when available).
4. Apply changes manually.

---

## 6. Doctor Failure Classes

Each doctor check maps to a troubleshooting ID:

| Doctor Check | Category | Failure Mapping |
|-------------|----------|-----------------|
| `config.valid` | configuration | `TRBL-INSTALL-CLI` |
| `neo4j.reachable` | database | `TRBL-INSTALL-DOCKER` |
| `api.healthy` | runtime | `TRBL-PORT-CONFLICT` |
| `cms.resume` | cms | `TRBL-CMS-DEGRADED` |
| `cms.observe` | cms | `TRBL-CMS-DEGRADED` |
| `ollama.reachable` | llm | `TRBL-CMS-DEGRADED` |
| `ollama.models` | llm | `TRBL-OLLAMA-MODELS` |
| `ssh.reachable` (remote only) | connectivity | `TRBL-REMOTE-SSH` |
| `docker-context.valid` (remote only) | connectivity | `TRBL-REMOTE-CONTEXT` |

---

## 7. Escalation Template

```text
Issue ID:
Repo:
Profile:
Command:
Error output:
Doctor JSON excerpt:
Recent logs:
Actions attempted:
Current state:
```
