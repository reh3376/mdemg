---
created: 2026-02-24
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "48"
---

# CMS Skill Registry

## Summary

**Feature**: Skill Registry
**Summary**: CMS-backed persistent skill registry where skills are non-decaying instruction sets stored in Neo4j as pinned observations, retrieved via tag-based queries. Thin skill files in `.claude/skills/` act as pointers that recall content from CMS at runtime.

## Vision & Goals

Skills represent reusable, structured knowledge that agents need repeatedly — API references, workflow templates, diagnostic procedures. Rather than storing this in large markdown files (which consume token budget), skills are backed by CMS observations with pinned permanence. This ensures skills are always current (API-updatable), never decay, and integrate with the same retrieval infrastructure as all other memory.

## Current State

### Architecture

```
.claude/skills/mdemg-api.md          (thin pointer: triggers + recall command)
        |
        v
POST /v1/skills/mdemg-api/recall     (tag-based Cypher query)
        |
        v
Neo4j MemoryNode                     (pinned observations with skill:<name> tags)
  role_type: conversation_observation
  pinned: true
  tags: [skill:mdemg-api, skill:mdemg-api:cms]
```

**Tag System** — hierarchical tags for organization:

- `skill:<name>` — all observations belonging to a skill
- `skill:<name>:<section>` — specific section within a skill

Recall uses **direct Cypher tag matching** (not vector similarity) for deterministic retrieval. Tag matches always return score `1.0`.

**Pinning** — Skill observations created with `pinned: true`:

- Disables temporal decay (content never ages)
- Protects from consolidation (won't be merged or summarized)
- Guarantees permanent availability (stability score fixed at 1.0)

### Workflow

**Thin Skill Files** in `.claude/skills/` are minimal pointers (~20 lines) containing trigger conditions, recall command, and section list. Without CMS running, skills cannot function — content lives in Neo4j, not in files.

**Design Rationale:**

- Unified storage with conversation memory
- Protected from decay via pinning
- API-driven updates without file edits
- Consistent retrieval infrastructure
- Reduced file sizes (e.g., `mdemg-api.md`: 519 lines reduced to 23 lines)

**Trade-off:** Skills require the CMS server to be running.

### Configuration

No configuration required — skill registry is always available when the server is running.

## Notes

### Known Limitations

- Skills require CMS server to be running — no offline fallback
- Tag-based recall only — no vector similarity search for skills

### Risks & Gaps

- Skill recall endpoints (`/v1/skills/*/recall`) may not be fully documented in all references

### Future Improvements

- Skill versioning (track changes over time)
- Skill export/import for sharing between instances

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| GET | `/v1/skills?space_id=<id>` | List all registered skills | `specs/skills_list.uats.json` |
| POST | `/v1/skills/{name}/recall` | Recall skill content by tag (optional section filter) | `specs/skills_recall.uats.json` |
| POST | `/v1/skills/{name}/register` | Create pinned observations for skill sections | `specs/skills_register.uats.json` |

## CLI Commands

None — skill management is API-only.

## Configuration Reference

None — no configurable parameters.

## Dependencies

| Feature | Relationship |
|---------|-------------|
| CMS Observe/Resume (Phase 43) | Requires — skills stored as observations |
| Pinned Observations (Phase 47) | Requires — pinning prevents decay and consolidation |
| Neo4j | Requires — tag-based Cypher queries for recall |

## Related Files

- `internal/api/handlers_skills.go` - List, recall, and register handlers
- `.claude/skills/mdemg-api.md` - API reference skill pointer
- `.claude/skills/create-plugin.md` - Plugin development skill pointer
- `.claude/skills/mdemg-cms-self-improve.md` - Self-improvement diagnostics skill pointer
- `docs/api/api-spec/uats/specs/skills_list.uats.json` - Contract test for list
- `docs/api/api-spec/uats/specs/skills_recall.uats.json` - Contract test for recall
- `docs/api/api-spec/uats/specs/skills_register.uats.json` - Contract test for register
