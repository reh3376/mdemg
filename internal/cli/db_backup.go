package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/backup"
	"mdemg/internal/config"
	"mdemg/internal/db"
	"mdemg/internal/transfer"
)

func newDBBackupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Database backup management",
		Long: `Manage periodic Neo4j database backups.

Backups run automatically when the server is running (daily partial, weekly full).
Use these commands to trigger manual backups, list existing backups, or view
the current backup configuration.

Backup files are stored in .mdemg/backups/ (gitignored by default).
Only the 2 most recent backups per type are retained.`,
	}

	cmd.AddCommand(newDBBackupTriggerCmd())
	cmd.AddCommand(newDBBackupListCmd())
	cmd.AddCommand(newDBBackupConfigCmd())

	return cmd
}

func newDBBackupTriggerCmd() *cobra.Command {
	var backupType string

	cmd := &cobra.Command{
		Use:   "trigger",
		Short: "Trigger a manual backup",
		Long: `Trigger an immediate database backup.

Types:
  partial  — Cypher-based export (runs on live database, no downtime)
  full     — Docker neo4j-admin dump (requires stopping the database)

For most cases, use "partial" (the default).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			if !cfg.BackupEnabled {
				fmt.Println("Backups are disabled. Enable with:")
				fmt.Println("  Set backup.enabled: true in .mdemg/config.yaml")
				fmt.Println("  Or: BACKUP_ENABLED=true")
				return nil
			}

			driver, dErr := db.NewDriver(cfg)
			if dErr != nil {
				return fmt.Errorf("database connection failed: %w", dErr)
			}
			defer driver.Close(context.Background())

			backupCfg := makeBackupConfig(cfg)
			exp := transfer.NewExporter(driver)
			svc := backup.NewService(backupCfg, driver, exp)

			fmt.Printf("Triggering %s backup...\n", backupType)
			id, trigErr := svc.Trigger(context.Background(), backup.TriggerRequest{
				Type:  backupType,
				Label: "manual",
			})
			if trigErr != nil {
				return fmt.Errorf("backup trigger failed: %w", trigErr)
			}
			fmt.Printf("Backup started: %s\n", id)
			fmt.Printf("Storage: %s\n", cfg.BackupStorageDir)

			// Wait briefly for the async backup to complete
			time.Sleep(3 * time.Second)
			manifest, mErr := svc.GetBackup(id)
			if mErr == nil && manifest != nil {
				fmt.Printf("Completed: %s (%d nodes, %d edges)\n", backupFormatBytes(manifest.SizeBytes), manifest.NodeCount, manifest.EdgeCount)
			} else {
				fmt.Println("Status: running (check server logs for completion)")
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&backupType, "type", "partial", "Backup type: partial or full")

	return cmd
}

func newDBBackupListCmd() *cobra.Command {
	var limit int
	var backupType string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List existing backups",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			driver, dErr := db.NewDriver(cfg)
			if dErr != nil {
				return fmt.Errorf("database connection failed: %w", dErr)
			}
			defer driver.Close(context.Background())

			backupCfg := backup.Config{
				StorageDir: cfg.BackupStorageDir,
			}
			exp := transfer.NewExporter(driver)
			svc := backup.NewService(backupCfg, driver, exp)

			backups, lErr := svc.ListBackups(backupType, limit)
			if lErr != nil {
				return fmt.Errorf("list backups: %w", lErr)
			}

			if len(backups) == 0 {
				fmt.Println("No backups found.")
				fmt.Printf("Storage directory: %s\n", cfg.BackupStorageDir)
				return nil
			}

			fmt.Printf("%-40s  %-8s  %-10s  %s\n", "BACKUP ID", "TYPE", "SIZE", "CREATED")
			fmt.Printf("%-40s  %-8s  %-10s  %s\n", "─────────", "────", "────", "───────")
			for _, b := range backups {
				size := backupFormatBytes(b.SizeBytes)
				created := b.CreatedAt
				if t, tErr := time.Parse(time.RFC3339, b.CreatedAt); tErr == nil {
					created = t.Format("2006-01-02 15:04")
				}
				fmt.Printf("%-40s  %-8s  %-10s  %s\n", b.BackupID, b.Type, size, created)
			}
			fmt.Printf("\nTotal: %d backup(s) in %s\n", len(backups), cfg.BackupStorageDir)

			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum number of backups to show")
	cmd.Flags().StringVar(&backupType, "type", "", "Filter by type: partial or full")

	return cmd
}

func newDBBackupConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show current backup configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			fmt.Println("Backup Configuration")
			fmt.Println("====================")
			fmt.Printf("  Enabled:              %v\n", cfg.BackupEnabled)
			fmt.Printf("  Storage directory:     %s\n", cfg.BackupStorageDir)
			fmt.Printf("  Neo4j container:       %s\n", cfg.BackupNeo4jContainer)
			fmt.Println()
			fmt.Println("Schedule:")
			fmt.Printf("  Full backup:           every %dh (%s)\n", cfg.BackupFullIntervalHours, backupFormatHours(cfg.BackupFullIntervalHours))
			fmt.Printf("  Partial backup:        every %dh (%s)\n", cfg.BackupPartialIntervalHours, backupFormatHours(cfg.BackupPartialIntervalHours))
			fmt.Println()
			fmt.Println("Retention:")
			fmt.Printf("  Full backups kept:     %d\n", cfg.BackupRetentionFullCount)
			fmt.Printf("  Partial backups kept:  %d\n", cfg.BackupRetentionPartialCount)
			fmt.Printf("  Max age:               %d days\n", cfg.BackupRetentionMaxAgeDays)
			fmt.Printf("  Max storage:           %d GB\n", cfg.BackupRetentionMaxStorageGB)
			fmt.Printf("  Run after backup:      %v\n", cfg.BackupRetentionRunAfter)
			fmt.Println()
			fmt.Println("To change these values, edit .mdemg/config.yaml:")
			fmt.Println("  backup:")
			fmt.Println("    enabled: true")
			fmt.Println("    storage_dir: .mdemg/backups")
			fmt.Println("    interval_hours: 24")
			fmt.Println("    retention_count: 2")
			fmt.Println()
			fmt.Println("Or use environment variables:")
			fmt.Println("  BACKUP_ENABLED, BACKUP_STORAGE_DIR,")
			fmt.Println("  BACKUP_PARTIAL_INTERVAL_HOURS, BACKUP_RETENTION_FULL_COUNT, etc.")

			return nil
		},
	}

	return cmd
}

func makeBackupConfig(cfg config.Config) backup.Config {
	return backup.Config{
		Enabled:               cfg.BackupEnabled,
		StorageDir:            cfg.BackupStorageDir,
		FullCmd:               cfg.BackupFullCmd,
		Neo4jContainer:        cfg.BackupNeo4jContainer,
		FullIntervalHours:     cfg.BackupFullIntervalHours,
		PartialIntervalHours:  cfg.BackupPartialIntervalHours,
		RetentionFullCount:    cfg.BackupRetentionFullCount,
		RetentionPartialCount: cfg.BackupRetentionPartialCount,
		RetentionMaxAgeDays:   cfg.BackupRetentionMaxAgeDays,
		RetentionMaxStorageGB: cfg.BackupRetentionMaxStorageGB,
		RetentionRunAfter:     cfg.BackupRetentionRunAfter,
	}
}

func backupFormatBytes(b int64) string {
	switch {
	case b >= 1024*1024:
		return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
	case b >= 1024:
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func backupFormatHours(hours int) string {
	if hours >= 168 {
		weeks := hours / 168
		return fmt.Sprintf("%d week(s)", weeks)
	}
	if hours >= 24 {
		days := hours / 24
		return fmt.Sprintf("%d day(s)", days)
	}
	return fmt.Sprintf("%d hour(s)", hours)
}
