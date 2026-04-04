---
created: YYYY-MM-DD
updated: YYYY-MM-DD
version: vX.X.X
author: reh3376
status: active | deprecated | experimental
phase: <phase-id>
---

# Feature Name

## Summary

**Feature**: <feature_name>
**Summary**: <1-2 sentence description of what this feature does>

## Vision & Goals

<How this feature contributes to the overall vision and goals of the MDEMG framework. Why it exists, what problem it solves, and how it fits into the cognitive substrate architecture.>

## Current State

<Detailed review of the feature's functionality, applicable workflows, logic sequences, inter-dependencies, interactions. This is the bulk of the document.>

### Architecture

<System design, component relationships, data flow>

### Workflow

<Step-by-step flow of how the feature operates>

### Configuration

<Environment variables, config keys, and their defaults>

## Notes

### Known Limitations

<Current constraints or boundaries of the feature>

### Risks & Gaps

<Identified risks and unresolved gaps>

### Future Improvements

<Planned or suggested enhancements>

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| POST | `/v1/...` | ... | `specs/....uats.json` |

## CLI Commands

| Command | Description |
|---------|-------------|
| `mdemg ...` | ... |

## Configuration Reference

| Env Var | Default | Description |
|---------|---------|-------------|
| `VAR_NAME` | `value` | ... |

## Dependencies

| Feature | Relationship |
|---------|-------------|
| <feature> | <requires / enhances / feeds-into> |

## Related Files

- `internal/...` - <description>
- `docs/...` - <description>
