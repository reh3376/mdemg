// V0023: SymbolNode natural-key uniqueness constraint.
//
// Statement 1: Auto-dedup — merges duplicate SymbolNode groups that share
// (space_id, name, file_path, symbol_type). Keeps one node per group
// (positional selection), DETACH DELETEs the rest. Edges rebuild via
// Hebbian learning on subsequent ingestion/consolidation.
//
// Statement 2: Creates the uniqueness constraint, preventing future
// duplicates from re-ingest.
//
// Idempotent: Statement 1 is a no-op if no duplicates exist.
//             Statement 2 uses IF NOT EXISTS.
//
// For weight-preserving dedup (aggregates CO_ACTIVATED_WITH edge weights
// instead of dropping them), run `mdemg graph repair` BEFORE this migration.

CALL {
  MATCH (s:SymbolNode)
  WITH s.space_id AS sid, s.name AS name, s.file_path AS fp, s.symbol_type AS st,
       collect(s) AS dupes
  WHERE size(dupes) > 1
  WITH dupes[0] AS keeper, dupes[1..] AS victims
  UNWIND victims AS v
  DETACH DELETE v
  RETURN count(v) AS removed
} IN TRANSACTIONS OF 500 ROWS;

CREATE CONSTRAINT symbol_natural_key IF NOT EXISTS
FOR (s:SymbolNode) REQUIRE (s.space_id, s.name, s.file_path, s.symbol_type) IS UNIQUE;
