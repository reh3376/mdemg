# Migrations as Code (Neo4j)

This project treats **schema as executable code**: every constraint, index, and compatibility requirement is defined in versioned Cypher scripts and applied in a deterministic order.

## Goals

- **Deterministic boot**: a fresh DB becomes “ready” by applying migrations in order.
- **Idempotent**: safe to run the full set at startup (`IF NOT EXISTS` wherever possible).
- **Auditable**: the DB records what migrations were applied, when, and by which service version.
- **Compatible**: the service refuses to run if the DB schema version is out of range.

## Versioning model

We use two metadata node types:

### `:SchemaMeta`

Singleton per DB (or per `space_id` if you later multi-tenant at the schema layer).

- `key: 'schema'` (unique)
- `current_version: int`
- `updated_at: datetime`

### `:Migration`

Append-only record per applied migration.

- `version: int` (unique)
- `name: string` (e.g., `V0003__vector_indexes`)
- `checksum: string` (optional but recommended)
- `applied_at: datetime`
- `applied_by: string` (service build/version)

**Invariant:** once a migration version is applied, it is never modified. If you need a change, create a new migration.

## Requirements and constraints

### Global requirements

- Every node/relationship participating in the memory system must carry `space_id`.
- Uniqueness constraints must be **composite** where `space_id` is part of identity.
- Avoid hard deletes; prefer tombstoning to preserve auditability.

### Required uniqueness constraints

- `(:TapRoot {space_id})` is unique.
- `(:MemoryNode {space_id, path})` is unique.
- `(:MemoryNode {space_id, node_id})` is unique.
- `(:Observation {space_id, obs_id})` is unique.

### Recommended btree indexes

- `:MemoryNode(space_id, name)`
- `:MemoryNode(space_id, layer, role_type)`

## Vector indexes

Vector indexes are used **only** for candidate recall.

Minimum required index:

- `memNodeEmbedding` on `:MemoryNode.embedding`

Optional:

- `observationEmbedding` on `:Observation.embedding` (only if you embed raw events/chunks)

### Index configuration conventions

- `vector.dimensions`: must match the embedding model.
- `vector.similarity_function`: `cosine` is typical for text embeddings.

## Applying migrations

### Recommended practice

- Apply all migrations at **service startup** before the API begins serving traffic.
- If the DB version is behind, apply forward.
- If the DB version is ahead of the service’s max supported version, **fail fast**.

### Minimal startup policy

Service config:

- `REQUIRED_SCHEMA_VERSION` (exact) **or** `(MIN_SCHEMA_VERSION, MAX_SCHEMA_VERSION)`

The skeleton service uses `REQUIRED_SCHEMA_VERSION` for simplicity.

## Script naming

- `V0001__schema_meta.cypher`
- `V0002__constraints_and_btree_indexes.cypher`
- `V0003__vector_indexes.cypher`
- `V0004__safety_checks.cypher`
- ...
- `V0023__symbol_natural_key.cypher`
- `V0024__signal_state.cypher`

As of v0.7.4, production runs 24 migrations (V0001-V0024).

## Checksums (optional but recommended)

If you want stronger discipline, store a checksum per migration and verify it at startup.

- Compute `sha256` of the script contents.
- Store it on `:Migration.checksum`.
- On startup, re-compute and compare.

## Rollback stance

Neo4j does not provide transactional schema rollback in the way many SQL migration tools do.
**Policy:** migrations are forward-only. If you need to revert, create a new migration that repairs the mistake.
