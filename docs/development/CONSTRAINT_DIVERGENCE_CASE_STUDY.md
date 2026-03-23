# Constraint Divergence Case Study: UUID v4 vs CUIDv2

**Date:** 2026-03-23
**Severity:** Medium (11 files required correction)
**Root Cause:** Jiminy constraint pipeline silently disabled by daemon .env loading bug

---

## The Requirement

MDEMG has a project-scoped requirement: **all unique identifiers must use CUIDv2** (`github.com/nrednav/cuid2`), not UUID v4. CUIDv2 is collision-resistant, k-sortable, and produces shorter identifiers. This applies to the main repo and all submodules.

## What Happened

`guidance_id` (Jiminy guide responses) and `eval_id` (Jiminy evaluate responses) were implemented using `uuid.New().String()` instead of `cuid2.Generate()`. The divergence persisted across multiple development phases without being caught.

## Root Cause Chain

```
1. daemon.go loaded YAML config BEFORE .env
   └─ Go zero-value (false) was set for JIMINY_ENABLED
      └─ .env's JIMINY_ENABLED=true couldn't override (already set)
         └─ Jiminy disabled in daemon mode
            └─ CUIDv2 constraint could not fire
               └─ uuid.New() used in initial implementation
                  └─ Subsequent code copied UUID pattern by PRECEDENT
                     └─ Pattern hardened into codebase across phases
```

## Why Precedent Matters

This is the critical insight. Even after the daemon .env bug was fixed (restoring Jiminy), the UUID pattern continued to exist because:

1. **Developers reference existing code as templates.** When implementing a new ID field, the natural action is to look at how existing IDs are generated and copy that pattern.
2. **Code review doesn't catch convention violations.** UUID v4 is a perfectly valid identifier format — nothing looks "wrong" about it in isolation.
3. **Constraints are invisible without enforcement.** The CUIDv2 requirement existed as project knowledge, but without active constraint surfacing, it was effectively invisible to the agent generating code.
4. **Bad patterns are self-reinforcing.** Each instance of `uuid.New()` made the next instance more likely, because there was more precedent to copy from.

## The Fix

### Immediate (code)
- Swapped `uuid.New().String()` → `cuid2.Generate()` in `service.go` and `evaluator.go`
- Removed `github.com/google/uuid` import from both files
- Updated `go.mod` with `github.com/nrednav/cuid2 v1.1.0`

### Immediate (daemon bug)
- Swapped loading order in `daemon.go`: `.env` (godotenv) now loads BEFORE YAML config
- This restores the documented priority: `defaults → yaml → keychain → .env → env vars → flags`

### Documentation (11 files)
- Updated all docs referencing `guidance_id` as "UUID" → "CUID2 unique identifier"
- Updated example values from UUID format to CUID2 format
- Updated code comments in `types.go`

### Constraint registration (CMS)
- Stored `[must] use CUIDv2, not UUID` as a correction in CMS
- Jiminy will now surface this constraint when it detects `uuid.New()` or UUID v4 patterns in identifier generation code

## Prevention: How Jiminy Catches This Going Forward

With the daemon .env bug fixed and the constraint stored in CMS:

1. Agent writes code using `uuid.New()` for an identifier
2. Hook fires `POST /v1/jiminy/guide` with the code context
3. Jiminy retrieves the CUIDv2 correction via vector similarity
4. Agent receives: `[must] Use CUIDv2 (cuid2.Generate()), not UUID v4`
5. Agent corrects the code before committing

## Broader Lesson

**When a constraint enforcement system goes down, the damage is not limited to the downtime window.** Bad patterns introduced during the outage persist via precedent long after enforcement is restored. This is why:

- Constraint pipeline health should be monitored (Jiminy returns 503 when disabled — hooks should warn)
- After any enforcement outage, a sweep of changes made during the outage should be performed
- The `session-start.sh` hook now warns "CMS unavailable — memory disconnected" to make the outage visible

## Files Modified

| File | Change |
|------|--------|
| `internal/jiminy/service.go` | `uuid.New().String()` → `cuid2.Generate()` |
| `internal/jiminy/evaluator.go` | `uuid.New().String()` → `cuid2.Generate()` |
| `internal/jiminy/types.go` | Comment: UUID → CUID2 |
| `internal/cli/daemon.go` | Swapped .env/YAML loading order |
| `CHANGELOG.md` | 4 new entries (2 Fixed, 2 Changed) |
| `CONTRIBUTING.md` | UUID → CUID2 |
| `AGENT_HANDOFF.md` | Added control-loop optimization note |
| `docs/features/jiminy-effectiveness-tracking.md` | UUID → CUID2 (5 locations) |
| `docs/features/jiminy-inner-voice.md` | UUID → CUID2 |
| `docs/development/API_REFERENCE.md` | UUID → CUID2 (3 locations) |
| `docs/user/api-reference.md` | UUID → CUID2 |
| `docs/features/process-lifecycle.md` | Added env inheritance paragraph |
| `docs/specs/phase97-process-lifecycle-security.md` | Added env loading step |
| `docs/specs/phase-fsd-constraint-lifecycle.md` | UUID → CUID2 |

## CMS Observations

| Node ID | Type | Content |
|---------|------|---------|
| `433779d6-...` | correction | CUIDv2 requirement (merged with existing) |
| `f10523c8-...` | learning | Precedent-driven divergence incident |
| `249fe007-...` | decision | CUIDv2 enforcement via Jiminy |
