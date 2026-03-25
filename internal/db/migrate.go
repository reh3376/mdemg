package db

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

// MigrationFile represents a parsed migration file.
type MigrationFile struct {
	Version    int
	Name       string // e.g. "V0015__secondary_labels"
	Filename   string // e.g. "V0015__secondary_labels.cypher"
	Statements []string
}

// MigrationStatus describes the current migration state of the database.
type MigrationStatus struct {
	CurrentVersion   int
	AvailableVersion int
	Pending          []MigrationFile
	Applied          []int
}

// SplitStatements splits a Cypher migration file's content into individual statements.
// It strips // line comments, tracks brace depth for CALL { } IN TRANSACTIONS blocks,
// and splits on ; only at brace depth 0.
func SplitStatements(content string) []string {
	var statements []string
	var current strings.Builder
	depth := 0

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip pure comment lines
		if strings.HasPrefix(trimmed, "//") {
			continue
		}

		// Strip inline comments (only outside strings — simplified: no string tracking needed for Cypher DDL)
		if idx := strings.Index(trimmed, "//"); idx > 0 {
			trimmed = strings.TrimSpace(trimmed[:idx])
		}

		if trimmed == "" {
			continue
		}

		// Process character by character for brace tracking and semicolons
		for i := 0; i < len(trimmed); i++ {
			ch := trimmed[i]
			switch ch {
			case '{':
				depth++
				current.WriteByte(ch)
			case '}':
				if depth > 0 {
					depth--
				}
				current.WriteByte(ch)
			case ';':
				if depth == 0 {
					stmt := strings.TrimSpace(current.String())
					if stmt != "" {
						statements = append(statements, stmt)
					}
					current.Reset()
				} else {
					current.WriteByte(ch)
				}
			default:
				current.WriteByte(ch)
			}
		}
		// Add a space between lines to preserve readability
		if current.Len() > 0 {
			current.WriteByte(' ')
		}
	}

	// Collect any remaining content (no trailing semicolon)
	if stmt := strings.TrimSpace(current.String()); stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}

// ParseMigrationFile parses a migration filename and content into a MigrationFile.
func ParseMigrationFile(filename string, content []byte) (MigrationFile, error) {
	if !strings.HasPrefix(filename, "V") || !strings.HasSuffix(filename, ".cypher") {
		return MigrationFile{}, fmt.Errorf("invalid migration filename: %s", filename)
	}

	base := strings.TrimSuffix(filename, ".cypher")
	parts := strings.SplitN(base[1:], "__", 2)
	if len(parts) < 2 {
		return MigrationFile{}, fmt.Errorf("filename missing __ separator: %s", filename)
	}

	ver, err := strconv.Atoi(parts[0])
	if err != nil {
		return MigrationFile{}, fmt.Errorf("parse version from %s: %w", filename, err)
	}

	return MigrationFile{
		Version:    ver,
		Name:       base,
		Filename:   filename,
		Statements: SplitStatements(string(content)),
	}, nil
}

// DiscoverMigrations reads all migration files from the given filesystem and returns them sorted by version.
func DiscoverMigrations(fsys fs.FS) ([]MigrationFile, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var migrations []MigrationFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".cypher") || !strings.HasPrefix(e.Name(), "V") {
			continue
		}

		content, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}

		mig, err := ParseMigrationFile(e.Name(), content)
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, mig)
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// GetMigrationStatus queries the database for current schema version and computes pending migrations.
func GetMigrationStatus(ctx context.Context, driver neo4j.DriverWithContext, available []MigrationFile) (MigrationStatus, error) {
	status := MigrationStatus{}

	if len(available) > 0 {
		status.AvailableVersion = available[len(available)-1].Version
	}

	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	// Get current schema version
	result, err := sess.Run(ctx,
		"OPTIONAL MATCH (s:SchemaMeta {key:'schema'}) RETURN coalesce(s.current_version, 0) AS version",
		nil)
	if err != nil {
		return status, fmt.Errorf("query schema version: %w", err)
	}
	if result.Next(ctx) {
		v, _ := result.Record().Get("version")
		if vi, ok := v.(int64); ok {
			status.CurrentVersion = int(vi)
		}
	}

	// Get list of applied migration versions
	result2, err := sess.Run(ctx,
		"MATCH (m:Migration) RETURN m.version AS version ORDER BY m.version",
		nil)
	if err != nil {
		// Migration nodes may not exist yet — not an error
		slog.Info("could not query Migration nodes", "error", err)
	} else {
		for result2.Next(ctx) {
			v, _ := result2.Record().Get("version")
			if vi, ok := v.(int64); ok {
				status.Applied = append(status.Applied, int(vi))
			}
		}
	}

	// Compute pending migrations
	appliedSet := make(map[int]bool, len(status.Applied))
	for _, v := range status.Applied {
		appliedSet[v] = true
	}

	for _, mig := range available {
		if !appliedSet[mig.Version] {
			status.Pending = append(status.Pending, mig)
		}
	}

	return status, nil
}

// ApplyMigration executes a single migration's statements and records it.
// Each statement runs in auto-commit mode (matching cypher-shell behavior).
func ApplyMigration(ctx context.Context, driver neo4j.DriverWithContext, mig MigrationFile) error {
	// Execute each statement in auto-commit mode
	for i, stmt := range mig.Statements {
		sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		result, err := sess.Run(ctx, stmt, nil)
		if err != nil {
			sess.Close(ctx)
			return fmt.Errorf("statement %d of %s failed: %w", i+1, mig.Filename, err)
		}
		// Drain the result to ensure the statement completes
		for result.Next(ctx) {
		}
		if err := result.Err(); err != nil {
			sess.Close(ctx)
			return fmt.Errorf("statement %d of %s result error: %w", i+1, mig.Filename, err)
		}
		sess.Close(ctx)
	}

	// Record migration in SchemaMeta
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer sess.Close(ctx)

	_, err := sess.Run(ctx, `
		MERGE (m:Migration {version: $ver})
		ON CREATE SET m.name = $name, m.applied_at = datetime(), m.applied_by = 'mdemg-migrate'
	`, map[string]any{
		"ver":  mig.Version,
		"name": mig.Name,
	})
	if err != nil {
		return fmt.Errorf("record migration %s: %w", mig.Filename, err)
	}

	// Update schema version
	_, err = sess.Run(ctx, `
		MERGE (s:SchemaMeta {key: 'schema'})
		ON CREATE SET s.current_version = $ver, s.created_at = datetime(), s.updated_at = datetime()
		ON MATCH SET s.current_version = CASE WHEN s.current_version < $ver THEN $ver ELSE s.current_version END, s.updated_at = datetime()
	`, map[string]any{
		"ver": mig.Version,
	})
	if err != nil {
		return fmt.Errorf("update schema version for %s: %w", mig.Filename, err)
	}

	return nil
}

// RunMigrations applies all pending migrations from the given filesystem.
// Returns the number of migrations applied.
func RunMigrations(ctx context.Context, driver neo4j.DriverWithContext, fsys fs.FS) (int, error) {
	available, err := DiscoverMigrations(fsys)
	if err != nil {
		return 0, fmt.Errorf("discover migrations: %w", err)
	}

	status, err := GetMigrationStatus(ctx, driver, available)
	if err != nil {
		return 0, fmt.Errorf("get migration status: %w", err)
	}

	if len(status.Pending) == 0 {
		return 0, nil
	}

	for _, mig := range status.Pending {
		slog.Info("applying migration", "filename", mig.Filename, "statements", len(mig.Statements))
		if err := ApplyMigration(ctx, driver, mig); err != nil {
			return 0, err
		}
		slog.Info("applied migration", "filename", mig.Filename, "version", mig.Version)
	}

	return len(status.Pending), nil
}
