package db

import (
	"testing"
	"testing/fstest"
)

func TestSplitStatements_Simple(t *testing.T) {
	input := `CREATE INDEX foo IF NOT EXISTS FOR (n:Foo) ON (n.id);
CREATE INDEX bar IF NOT EXISTS FOR (n:Bar) ON (n.id);`

	stmts := SplitStatements(input)
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d: %v", len(stmts), stmts)
	}
	if !containsSubstring(stmts[0], "CREATE INDEX foo") {
		t.Errorf("statement 0 unexpected: %s", stmts[0])
	}
	if !containsSubstring(stmts[1], "CREATE INDEX bar") {
		t.Errorf("statement 1 unexpected: %s", stmts[1])
	}
}

func TestSplitStatements_Comments(t *testing.T) {
	input := `// This is a comment
CREATE CONSTRAINT foo IF NOT EXISTS FOR (n:Foo) REQUIRE n.id IS UNIQUE;

// Another comment
MERGE (s:SchemaMeta {key:'schema'})
ON CREATE SET s.current_version = 1;`

	stmts := SplitStatements(input)
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d: %v", len(stmts), stmts)
	}
}

func TestSplitStatements_CALLInTransactions(t *testing.T) {
	input := `CALL {
  MATCH (n:MemoryNode) WHERE n.role_type = 'hidden' AND NOT n:HiddenPattern
  SET n:HiddenPattern
} IN TRANSACTIONS OF 1000 ROWS;

CREATE INDEX foo IF NOT EXISTS FOR (n:Foo) ON (n.id);`

	stmts := SplitStatements(input)
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d: %v", len(stmts), stmts)
	}
	if !containsSubstring(stmts[0], "CALL") {
		t.Errorf("statement 0 should contain CALL: %s", stmts[0])
	}
	if !containsSubstring(stmts[0], "IN TRANSACTIONS") {
		t.Errorf("statement 0 should contain IN TRANSACTIONS: %s", stmts[0])
	}
}

func TestSplitStatements_NestedBraces(t *testing.T) {
	input := `CALL {
  MATCH (n) WHERE n.type = 'test'
  WITH n { .id, .name } AS props
  SET n:Label
} IN TRANSACTIONS OF 500 ROWS;`

	stmts := SplitStatements(input)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d: %v", len(stmts), stmts)
	}
}

func TestSplitStatements_Empty(t *testing.T) {
	stmts := SplitStatements("")
	if len(stmts) != 0 {
		t.Fatalf("expected 0 statements, got %d", len(stmts))
	}

	stmts = SplitStatements("// just comments\n// nothing here")
	if len(stmts) != 0 {
		t.Fatalf("expected 0 statements from comments-only, got %d", len(stmts))
	}
}

func TestSplitStatements_NoTrailingSemicolon(t *testing.T) {
	input := `CREATE INDEX foo IF NOT EXISTS FOR (n:Foo) ON (n.id)`
	stmts := SplitStatements(input)
	if len(stmts) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(stmts))
	}
}

func TestParseMigrationFile(t *testing.T) {
	content := []byte("CREATE INDEX foo IF NOT EXISTS FOR (n:Foo) ON (n.id);")
	mig, err := ParseMigrationFile("V0015__secondary_labels.cypher", content)
	if err != nil {
		t.Fatal(err)
	}
	if mig.Version != 15 {
		t.Errorf("expected version 15, got %d", mig.Version)
	}
	if mig.Name != "V0015__secondary_labels" {
		t.Errorf("expected name V0015__secondary_labels, got %s", mig.Name)
	}
	if len(mig.Statements) != 1 {
		t.Errorf("expected 1 statement, got %d", len(mig.Statements))
	}
}

func TestParseMigrationFile_Invalid(t *testing.T) {
	tests := []struct {
		name     string
		filename string
	}{
		{"no V prefix", "0001__test.cypher"},
		{"no extension", "V0001__test.txt"},
		{"no separator", "V0001test.cypher"},
		{"bad version", "Vabc__test.cypher"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseMigrationFile(tt.filename, []byte(""))
			if err == nil {
				t.Errorf("expected error for filename %s", tt.filename)
			}
		})
	}
}

func TestDiscoverMigrations(t *testing.T) {
	fs := fstest.MapFS{
		"V0001__first.cypher":  {Data: []byte("CREATE (n:Test);")},
		"V0003__third.cypher":  {Data: []byte("CREATE (n:Test3);")},
		"V0002__second.cypher": {Data: []byte("CREATE (n:Test2);")},
		"README.md":            {Data: []byte("not a migration")},
	}

	migs, err := DiscoverMigrations(fs)
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(migs))
	}

	// Verify sorted order
	if migs[0].Version != 1 {
		t.Errorf("expected first migration version 1, got %d", migs[0].Version)
	}
	if migs[1].Version != 2 {
		t.Errorf("expected second migration version 2, got %d", migs[1].Version)
	}
	if migs[2].Version != 3 {
		t.Errorf("expected third migration version 3, got %d", migs[2].Version)
	}
}

func TestDiscoverMigrations_Empty(t *testing.T) {
	fs := fstest.MapFS{
		"README.md": {Data: []byte("no migrations")},
	}

	migs, err := DiscoverMigrations(fs)
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) != 0 {
		t.Fatalf("expected 0 migrations, got %d", len(migs))
	}
}

func TestSplitStatements_V0023Dedup(t *testing.T) {
	// Mirrors the actual V0023 migration: CALL IN TRANSACTIONS dedup + CREATE CONSTRAINT
	input := `CALL {
  MATCH (s:SymbolNode)
  WITH s.space_id AS sid, s.name AS name, s.file_path AS fp, s.symbol_type AS st,
       collect(s) AS dupes
  WHERE size(dupes) > 1
  WITH dupes[0] AS keeper, dupes[1..] AS victims
  UNWIND victims AS v
  DETACH DELETE v
} IN TRANSACTIONS OF 500 ROWS;

CREATE CONSTRAINT symbol_natural_key IF NOT EXISTS
FOR (s:SymbolNode) REQUIRE (s.space_id, s.name, s.file_path, s.symbol_type) IS UNIQUE;`

	stmts := SplitStatements(input)
	if len(stmts) != 2 {
		t.Fatalf("expected 2 statements, got %d: %v", len(stmts), stmts)
	}
	if !containsSubstring(stmts[0], "CALL") {
		t.Errorf("statement 0 should contain CALL: %s", stmts[0])
	}
	if !containsSubstring(stmts[0], "IN TRANSACTIONS OF 500 ROWS") {
		t.Errorf("statement 0 should contain IN TRANSACTIONS: %s", stmts[0])
	}
	if !containsSubstring(stmts[0], "DETACH DELETE v") {
		t.Errorf("statement 0 should contain DETACH DELETE: %s", stmts[0])
	}
	if !containsSubstring(stmts[1], "CREATE CONSTRAINT") {
		t.Errorf("statement 1 should contain CREATE CONSTRAINT: %s", stmts[1])
	}
	if !containsSubstring(stmts[1], "symbol_natural_key") {
		t.Errorf("statement 1 should contain constraint name: %s", stmts[1])
	}
}

func TestDiscoverMigrations_V0023(t *testing.T) {
	// Verify V0023 with dedup+constraint parses correctly via DiscoverMigrations
	v0023Content := `CALL {
  MATCH (s:SymbolNode)
  WITH s.space_id AS sid, s.name AS name, s.file_path AS fp, s.symbol_type AS st,
       collect(s) AS dupes
  WHERE size(dupes) > 1
  WITH dupes[0] AS keeper, dupes[1..] AS victims
  UNWIND victims AS v
  DETACH DELETE v
} IN TRANSACTIONS OF 500 ROWS;

CREATE CONSTRAINT symbol_natural_key IF NOT EXISTS
FOR (s:SymbolNode) REQUIRE (s.space_id, s.name, s.file_path, s.symbol_type) IS UNIQUE;`

	fs := fstest.MapFS{
		"V0023__symbol_natural_key.cypher": {Data: []byte(v0023Content)},
	}

	migs, err := DiscoverMigrations(fs)
	if err != nil {
		t.Fatal(err)
	}
	if len(migs) != 1 {
		t.Fatalf("expected 1 migration, got %d", len(migs))
	}
	if migs[0].Version != 23 {
		t.Errorf("expected version 23, got %d", migs[0].Version)
	}
	if len(migs[0].Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(migs[0].Statements))
	}
}

func containsSubstring(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && contains(s, sub))
}

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
