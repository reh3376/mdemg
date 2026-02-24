# Operational Notes (Neo4j)

## 1) Versioning discipline

- Prefer append-only Observations.
- Treat summary/description edits as versioned updates.
- Merges: create new node, tombstone old, add `MERGED_INTO` edges.

## 2) Performance

- Use vector indexes for candidate recall; do not attempt brute-force similarity.
- Keep hop expansion bounded (degree caps + edge caps).
- If using GenAI plugin batch encoding, use concurrent transactions carefully.

Neo4j notes vector indexes are Lucene-backed and documents an optional JVM flag to use the incubated Java Vector API
for noticeable speed improvements on compatible Java versions.

## 3) Security

- carry `sensitivity` on nodes and enforce at retrieval time.
- do not rely on client filtering; apply server-side policy checks before returning context.

## 4) Backups and migration

- schema migrations are code: run once at startup (idempotent `IF NOT EXISTS`).
- keep a migration history node (e.g., `:SchemaVersion` nodes).

## 5) Observability

Track:

- graph size (nodes/edges by type/layer)
- edge weight distributions (detect runaway growth)
- retrieval: latency, candidate_k, fetched edges
- learning: edges updated per request
- consolidation: abstractions created per day

## 6) Failure modes checklist

- hub explosion → add hub penalty + caps + prune
- clique spam in CO_ACTIVATED_WITH → threshold + cap updates
- over-decay → pin important nodes, lower decay, raise evidence weight
- stale summaries → periodic summary refresh job
