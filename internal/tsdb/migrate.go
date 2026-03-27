package tsdb

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrate runs all SQL migration files in order.
func (c *Client) Migrate(ctx context.Context) error {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("tsdb: read migrations dir: %w", err)
	}

	// Sort by filename to ensure order
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		data, err := fs.ReadFile(migrationsFS, "migrations/"+entry.Name())
		if err != nil {
			return fmt.Errorf("tsdb: read %s: %w", entry.Name(), err)
		}

		slog.Info("tsdb: running migration", "file", entry.Name())
		if _, err := c.pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("tsdb: migrate %s: %w", entry.Name(), err)
		}
		slog.Info("tsdb: migration complete", "file", entry.Name())
	}

	return nil
}
