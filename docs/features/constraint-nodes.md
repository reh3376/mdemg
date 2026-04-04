---
created: 2026-02-24
updated: 2026-04-04
version: v0.5.4
author: reh3376
status: active
phase: "45.5"
---

# Constraint Nodes

## Summary

**Feature**: Constraint Nodes
**Summary**: Automatically detects and promotes constraint-tagged observations to first-class constraint nodes in the knowledge graph, enabling structured tracking of requirements, prohibitions, recommendations, and deadlines extracted from natural language.

## Vision & Goals

Constraints are the operational backbone of AI-assisted development — "never do X", "always use Y", "deadline by Z". By promoting these from unstructured text to first-class graph nodes with typed edges, MDEMG enables Jiminy's inner voice to surface relevant constraints during retrieval, track compliance, and measure enforcement effectiveness. This is a key bridge between human intent and machine-enforceable rules.

## Current State

### Architecture

The `ConstraintDetector` scans observation content for natural language patterns indicating constraints:

| Type | Example Patterns | Confidence |
|------|-----------------|------------|
| `must` | "must", "always", "required", "mandatory" | 0.65-0.80 |
| `must_not` | "never", "must not", "forbidden", "prohibited" | 0.55-0.85 |
| `should` | "should", "prefer", "recommended", "best practice" | 0.50-0.65 |
| `should_not` | "should not", "try to avoid", "discouraged" | 0.55-0.65 |
| `deadline` | "by 2026-02-09", "due date", "deadline", "target date" | 0.70-0.80 |

Detection confidence is boosted by observation type: `decision` (+0.2), `correction` (+0.15). Detections below the minimum confidence threshold (0.6) are discarded.

### Workflow

1. **Detection**: During `Observe()` calls, the ConstraintDetector scans content and adds tags (e.g., `constraint:must`, `constraint:deadline`)
2. **Promotion**: During consolidation phase 20 (enrichment), tagged observations are promoted to constraint nodes:
   - Find observations with `constraint:*` tags not yet linked to a constraint node
   - Match or Create a constraint node (`role_type: 'constraint'`, `layer: 1`) with extracted label and type
   - Link via `IMPLEMENTS_CONSTRAINT` edge (initial weight: 1.0)
   - Reinforce existing constraints: increment `reinforcement_count` and edge weight (+0.1) on repeated matches
3. **Label Extraction**: Constraint names extracted from the first sentence (up to first period/newline, max 120 characters)

The constraint step is non-required — failure does not block the pipeline. Results appear in the consolidation response under `steps.constraint`.

### Configuration

No configuration required — constraint detection is always active during observation and consolidation.

## Notes

### Known Limitations

- Regex-based detection has limited accuracy for complex or negated constraints
- Label extraction uses simple first-sentence heuristic (max 120 chars)

### Risks & Gaps

- No LLM-based constraint classification yet (would improve accuracy for nuanced constraints)

### Future Improvements

- LLM-powered constraint classification (Phase 104: Active MCP Guardrails)
- Constraint lifecycle management (expiry, superseding)

## API Endpoints

| Method | Endpoint | Description | UATS Spec |
|--------|----------|-------------|-----------|
| GET | `/v1/constraints?space_id=<id>` | List all constraints in a space | `specs/constraints_list.uats.json` |
| GET | `/v1/constraints/stats?space_id=<id>` | Constraint statistics by type | `specs/constraints_stats.uats.json` |

## CLI Commands

None — API-only endpoints.

## Configuration Reference

None — no configurable parameters.

## Dependencies

| Feature | Relationship |
|---------|-------------|
| Consolidation Pipeline | Requires — constraint step runs at phase 20 in `buildPipeline()` |
| CMS Observe | Requires — constraints detected during `Observe()` calls |
| Jiminy Inner Voice | Feeds into — constraints surfaced during guidance generation |
| J17 Protocol | Feeds into — constraints codified for AI-to-AI communication |

## Related Files

- `internal/conversation/constraint_detector.go` - Regex-based auto-detection with confidence scoring
- `internal/hidden/constraint_nodes.go` - Node promotion and `IMPLEMENTS_CONSTRAINT` edge creation
- `internal/hidden/step_constraint.go` - Pipeline step adapter (phase 20)
- `internal/api/handlers_conversation.go` - `handleConstraintsList`, `handleConstraintStats` handlers
- `docs/api/api-spec/uats/specs/constraints_list.uats.json` - Contract test for list endpoint
- `docs/api/api-spec/uats/specs/constraints_stats.uats.json` - Contract test for stats endpoint
