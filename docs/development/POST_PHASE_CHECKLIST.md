# Post-Phase Completion Checklist

A reusable framework for standardizing documentation after every phase implementation.

---

## 1. Modified Files Tracker

Before updating docs, catalog all files changed in the phase:

| File | Change Type | Docs Impacted |
|------|-------------|---------------|
| `internal/...` | NEW / MODIFIED | AGENT_HANDOFF, REGISTRY, CONTRIBUTING |
| `internal/api/...` | NEW / MODIFIED | API_REFERENCE, gMEM-API |
| `internal/config/...` | MODIFIED | RELATIONSHIP_EXTRACTION (config section) |
| `migrations/...` | NEW | AGENT_HANDOFF (migrations table) |
| `docs/...` | NEW / MODIFIED | Cross-reference links |

---

## 2. Documentation Checklist

Review and update each of these after every phase:

### Mandatory

- [ ] **`AGENT_HANDOFF.md`** — Add phase section with:
  - Completion date and dependencies
  - What it does (summary table or bullet points)
  - Architecture notes (if structural changes)
  - Results (metrics, counts)
  - Key files table (file + change description)
  - Update footer line with current UATS count and phase status

- [ ] **`docs/development/API_REFERENCE.md`** — If any API request/response changed:
  - Update example JSON
  - Add new fields to response docs
  - Note backward compatibility

- [ ] **`CONTRIBUTING.md`** — If pipeline steps, project structure, or patterns changed:
  - Update step counts
  - Update directory tree if new directories added
  - Update pattern descriptions

- [ ] **`docs/development/REGISTRY.md`** — If pipeline steps added/modified:
  - Update step adapter table
  - Update registration code example
  - Update files table
  - Update phase numbering notes

### Conditional

- [ ] **`docs/development/RELATIONSHIP_EXTRACTION.md`** — If edge types, dynamic edges, or L5 logic changed
- [ ] **`docs/gMEM-API.md`** — If new API endpoints added (not just response changes)
- [ ] **Feature-specific docs** — Any doc referenced by the phase spec

---

## 3. Key Feature Docs

At the end of each phase, create dedicated feature documentation for key features introduced or modified.

**Location:** `docs/features/<feature-name>.md`

**Naming:** Use kebab-case descriptive names (e.g., `split-pipeline-execution.md`, `bridges-edge-type.md`)

**Each feature doc covers:**

1. What the feature does (one-paragraph summary)
2. How it works (architecture, data flow)
3. Configuration (env vars, defaults)
4. Usage examples (curl commands, code snippets)
5. Related files (table of key files)
6. Dependencies (what other features/phases it builds on)

**Rule:** One file per key feature. Features that span multiple phases get updated, not duplicated.

---

## 4. Auto-Memory Update

Update `MEMORY.md` (at `~/.claude/projects/-Users-reh3376-mdemg/memory/MEMORY.md`) with:

- Phase number and name
- Key architectural changes
- Updated counts (UATS, pipeline steps, etc.)
- New files or directories added

Keep entries concise — MEMORY.md is loaded into system prompt and truncates after 200 lines.

---

## 5. CMS Observation

Observe phase completion in CMS:

```bash
curl -s -X POST http://localhost:9999/v1/conversation/observe \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "mdemg-dev",
    "session_id": "claude-core",
    "content": "Phase XX COMPLETED: [summary]. Key changes: [list]. Results: [metrics]. UATS: X/Y.",
    "obs_type": "progress"
  }'
```

---

## 6. Verification

Before considering the phase fully complete:

- [ ] `go build ./...` passes
- [ ] `go vet ./...` passes
- [ ] `make test-api` — UATS pass rate unchanged or improved
- [ ] All documentation files render correctly (no broken markdown links)
- [ ] AGENT_HANDOFF.md footer reflects current state
- [ ] Feature docs in `docs/features/` are self-contained and reference correct file paths

---

## Quick Reference

```bash
# Build check
go build ./... && go vet ./...

# UATS check
make test-api BASE_URL=http://localhost:9999

# Verify docs render (basic link check)
grep -r '](docs/' AGENT_HANDOFF.md CONTRIBUTING.md | grep -v '#'
```
