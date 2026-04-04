// V0023: Add natural-key uniqueness constraint for SymbolNode.
// PREREQUISITE: All duplicate (name, file_path, symbol_type) groups must be
// merged first (Phase 3 of Graph Health Fix Sprint). Apply AFTER Phase 3.
//
// This constraint enforces that each (space_id, name, file_path, symbol_type)
// tuple is unique, preventing duplicate SymbolNodes from re-ingest.
CREATE CONSTRAINT symbol_natural_key IF NOT EXISTS
FOR (s:SymbolNode) REQUIRE (s.space_id, s.name, s.file_path, s.symbol_type) IS UNIQUE;
