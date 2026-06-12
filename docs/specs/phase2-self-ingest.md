# Feature Spec: Phase 2 — Self-Ingest MDEMG Codebase

> ⚠️ **DESIGN HISTORY (bannered 2026-06-12, DOC-AUDIT-001b).** This document
> is a point-in-time plan, analysis, or record of since-completed or
> superseded work. It is preserved unmodified as design history and is NOT
> a description of the current system — consult `docs/features/`,
> `docs/architecture/` (living set), and `CLAUDE.md` for current behavior.


**Phase**: Phase 2
**Status**: Implemented
**Author**: reh3376 & Claude (gMEM-dev)
**Date**: 2026-02-04

---

## Overview

Ingest the MDEMG codebase into itself for self-aware development assistance. The codebase is ingested into space `mdemg-codebase` which is NOT protected (can be cleared and re-ingested at any time).

## Requirements

### Functional Requirements

1. FR-1: Ingest all Go, proto, YAML, MD, JSON, Dockerfile, Makefile files
2. FR-2: Extract Go symbols (functions, types, constants, interfaces)
3. FR-3: Exclude vendor/, node_modules/, .git/, *.pb.go, binary files
4. FR-4: Run consolidation after ingestion
5. FR-5: Space `mdemg-codebase` is NOT protected (can be cleared and re-ingested)
6. FR-6: MCP tools support optional `space_id` parameter (default: ide-agent)

### Non-Functional Requirements

1. NFR-1: Ingestion completes within reasonable time
2. NFR-2: No impact on mdemg-dev protected space

## Configuration

- **Space ID**: `mdemg-codebase`
- **Source path**: `/Users/reh3376/mdemg`
- **Include**: `*.go`, `*.proto`, `*.yml`, `*.yaml`, `*.md`, `*.json`, `Dockerfile`, `Makefile`
- **Exclude**: `vendor/`, `node_modules/`, `.git/`, `docs/archive/`, `mdemg_neo4j/`, `bin/`, `logs/`
- **Symbol extraction**: Enabled (Go parser)

## Ingestion Results

- **Elements ingested**: 1,561
- **Errors**: 0
- **Time**: 1m41s (15.4 elements/sec)
- **Hidden nodes created**: 46
- **Concepts created**: 5
- **Memory count**: 1,475
- **Embedding coverage**: 100%
- **Health score**: 1.0

## MCP Server Changes

Added optional `space_id` parameter to all MCP tools:

- `memory_store` — store into any space
- `memory_recall` — query any space
- `memory_reflect` — reflect on any space
- `memory_associate` — associate within any space
- `memory_symbols` — search symbols in any space
- `memory_ingest_trigger` — ingest into any space

Default remains `ide-agent` for backward compatibility.

### File Changed

- `cmd/mcp-server/main.go` — added `space_id` param and `resolveSpaceID()` helper

## Acceptance Criteria

- [x] AC-1: Ingestion triggered and completed (1,561 elements, 0 errors)
- [x] AC-2: Node count > 100 (1,475 memory nodes)
- [x] AC-3: Retrieval returns relevant results (scoring formula query → score 0.944)
- [x] AC-4: Symbol search finds known Go symbols (IngestObservation found)
- [x] AC-5: SHA256 hash added to manifest
- [x] AC-6: MCP tools support optional space_id parameter
