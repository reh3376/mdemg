package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/tsdb"
)

func newDataExportAutoCmd() *cobra.Command {
	var (
		outputDir string
		keep      int
	)

	cmd := &cobra.Command{
		Use:   "export-auto",
		Short: "Automated training data export with retention management",
		Long: `Export training data with defaults suited for automated/scheduled use.

Exports the last 24 hours of data to the output directory, prunes old
archives beyond the --keep limit, and maintains a latest.tar.gz symlink.

Designed for use with launchd/systemd timers (see mdemg service install).`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			spaceID := resolveSpaceID(cmd)
			if spaceID == "" {
				return fmt.Errorf("--space-id is required (or set MDEMG_SPACE_ID)")
			}

			// Resolve instance ID: MDEMG_INSTANCE_ID env > empty (all instances)
			instanceID := os.Getenv("MDEMG_INSTANCE_ID")
			// No auto-detection fallback — empty means "all instances for this space"

			// Resolve output directory
			if outputDir == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					return fmt.Errorf("get home directory: %w", err)
				}
				outputDir = filepath.Join(home, ".mdemg", "exports")
			}
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return fmt.Errorf("create output directory: %w", err)
			}

			// Generate output path
			outputPath := filepath.Join(outputDir,
				fmt.Sprintf("mdemg-export-%s-%s.tar.gz",
					instanceID, time.Now().Format("20060102-150405")))

			cfg := tsdb.ExportConfig{
				SpaceID:    spaceID,
				From:       time.Now().Add(-24 * time.Hour),
				To:         time.Now().UTC(),
				Tables:     []string{"llm_interactions", "retrieval_events", "embedding_events"},
				InstanceID: instanceID,
				OutputPath: outputPath,
			}

			// Connect to TSDB
			tsdbCfg := tsdbConfigFromEnv()
			client, err := tsdb.NewClient(ctx, tsdbCfg)
			if err != nil {
				return fmt.Errorf("connect to TSDB: %w", err)
			}
			defer client.Close()
			pool := client.Pool()

			// Run export
			result, err := tsdb.RunExport(ctx, pool, cfg, Version, Commit)
			if err != nil {
				return err
			}

			fmt.Printf("Export complete: %s\n", result.OutputPath)
			fmt.Printf("  Total rows: %d\n", result.TotalRows)
			fmt.Printf("  Privacy:    %d violations\n", result.Manifest.Quality.PrivacyScrubViolations)

			// Prune old archives
			pruned, pruneErr := pruneExports(outputDir, keep)
			if pruneErr != nil {
				fmt.Fprintf(os.Stderr, "Warning: prune failed: %v\n", pruneErr)
			} else if pruned > 0 {
				fmt.Printf("  Pruned:     %d old archive(s)\n", pruned)
			}

			// Update latest symlink
			if err := updateLatestSymlink(outputDir, result.OutputPath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: symlink update failed: %v\n", err)
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&outputDir, "output-dir", "", "Output directory (default: ~/.mdemg/exports/)")
	cmd.Flags().IntVar(&keep, "keep", 7, "Number of archives to keep (oldest pruned)")

	return cmd
}

// pruneExports removes the oldest mdemg-export-*.tar.gz archives beyond
// the keep limit. Returns the number of files removed.
func pruneExports(dir string, keep int) (int, error) {
	if keep <= 0 {
		return 0, nil
	}

	archives, err := findExportArchives(dir)
	if err != nil {
		return 0, err
	}

	if len(archives) <= keep {
		return 0, nil
	}

	// Sort by mod time, newest first
	sort.Slice(archives, func(i, j int) bool {
		return archives[i].modTime.After(archives[j].modTime)
	})

	pruned := 0
	for _, a := range archives[keep:] {
		if err := os.Remove(a.path); err != nil {
			return pruned, fmt.Errorf("remove %s: %w", a.path, err)
		}
		pruned++
	}
	return pruned, nil
}

type archiveEntry struct {
	path    string
	modTime time.Time
}

// findExportArchives returns all mdemg-export-*.tar.gz files in dir.
func findExportArchives(dir string) ([]archiveEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var archives []archiveEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "mdemg-export-") && strings.HasSuffix(name, ".tar.gz") && name != "latest.tar.gz" {
			info, infoErr := e.Info()
			if infoErr != nil {
				continue
			}
			archives = append(archives, archiveEntry{
				path:    filepath.Join(dir, name),
				modTime: info.ModTime(),
			})
		}
	}
	return archives, nil
}

// updateLatestSymlink atomically updates the latest.tar.gz symlink
// to point to the given archive path.
func updateLatestSymlink(dir, archivePath string) error {
	linkPath := filepath.Join(dir, "latest.tar.gz")
	// Remove existing symlink (ignore error if it doesn't exist)
	os.Remove(linkPath) //nolint:errcheck // best-effort removal
	return os.Symlink(archivePath, linkPath)
}
