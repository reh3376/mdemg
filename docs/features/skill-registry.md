# CMS Skill Registry

Phase 48 adds a skill registry backed by CMS pinned observations. Skills are persistent, non-decaying instruction sets stored in Neo4j and retrieved via tag-based queries. Thin skill files in `.claude/skills/` act as pointers that recall content from CMS at runtime.

## How It Works

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

### Tag System

Skills use hierarchical tags for organization:

- `skill:<name>` — identifies all observations belonging to a skill
- `skill:<name>:<section>` — identifies a specific section within a skill

Recall uses **direct Cypher tag matching** (not vector similarity) for deterministic retrieval. Tag matches always return score `1.0`.

### Pinning

Skill observations are created with `pinned: true`, which:

- Disables temporal decay (content never ages)
- Protects from consolidation (won't be merged or summarized)
- Guarantees permanent availability (stability score fixed at 1.0)

## API Endpoints

### List Skills — `GET /v1/skills?space_id=X`

Discovers all registered skills by scanning pinned observations with `skill:*` tags.

```bash
curl -s "http://localhost:9999/v1/skills?space_id=mdemg-dev" | jq
```

```json
{
  "space_id": "mdemg-dev",
  "skills": [
    {
      "name": "mdemg-api",
      "description": "CMS endpoints: observe, resume, recall...",
      "sections": ["cms", "memory", "learning"],
      "observation_count": 6
    }
  ],
  "count": 3
}
```

### Recall Skill — `POST /v1/skills/{name}/recall`

Retrieves skill content by tag. Optionally filter by section.

```bash
# Recall all sections
curl -s -X POST http://localhost:9999/v1/skills/mdemg-api/recall \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev"}'

# Recall specific section
curl -s -X POST http://localhost:9999/v1/skills/mdemg-api/recall \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","section":"cms"}'
```

### Register Skill — `POST /v1/skills/{name}/register`

Creates pinned observations for each section. Tags are auto-generated.

```bash
curl -s -X POST http://localhost:9999/v1/skills/my-skill/register \
  -H "Content-Type: application/json" \
  -d '{
    "space_id": "mdemg-dev",
    "description": "My custom skill",
    "sections": [
      {"name": "setup", "content": "Step 1: ..."},
      {"name": "usage", "content": "To use this skill..."}
    ]
  }'
```

```json
{
  "skill": "my-skill",
  "space_id": "mdemg-dev",
  "sections_created": 2,
  "observation_ids": ["obs-1", "obs-2"]
}
```

## Thin Skill Files

Skill files in `.claude/skills/` are minimal pointers (typically ~20 lines) containing:

- **Trigger conditions** — when the skill should activate
- **Recall command** — the curl command to fetch content from CMS
- **Section list** — what sections exist

Without CMS running, skills cannot function. This is by design — the content lives in Neo4j, not in files.

### Example: `.claude/skills/mdemg-api.md`

```markdown
# Skill: mdemg-api
Trigger: User asks about MDEMG API endpoints
Sections: cms, memory, learning, retrieval, workflows, system

## Recall
curl -s -X POST http://localhost:9999/v1/skills/mdemg-api/recall \
  -H "Content-Type: application/json" \
  -d '{"space_id":"mdemg-dev","top_k":10}'
```

## Design Rationale

**Why CMS-backed instead of file-based?**

- Unified storage with conversation memory
- Protected from decay via pinning
- API-driven updates without file edits
- Consistent retrieval infrastructure
- Reduced file sizes (e.g., `mdemg-api.md`: 519 lines reduced to 23 lines)

**Trade-off:** Skills require the CMS server to be running. This is intentional — it ensures skill content is always the authoritative CMS version, not a stale file copy.

## Related Files

| File | Description |
|------|-------------|
| `internal/api/handlers_skills.go` | List, recall, and register handlers |
| `.claude/skills/mdemg-api.md` | API reference skill pointer |
| `.claude/skills/create-plugin.md` | Plugin development skill pointer |
| `.claude/skills/mdemg-cms-self-improve.md` | Self-improvement diagnostics skill pointer |
| `docs/api/api-spec/uats/specs/skills_list.uats.json` | Contract test for list |
| `docs/api/api-spec/uats/specs/skills_recall.uats.json` | Contract test for recall |
| `docs/api/api-spec/uats/specs/skills_register.uats.json` | Contract test for register |

## Dependencies

- CMS observe/resume infrastructure (Phase 43)
- Pinned observations (Phase 47)
