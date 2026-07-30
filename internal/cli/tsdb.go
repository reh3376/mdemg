package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/config"
	"mdemg/internal/tsdb"
)

// tsdbEnv reads a TSDB environment variable with a fallback default.
func tsdbEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// tsdbEnvInt reads a TSDB environment variable as int with a fallback default.
func tsdbEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

// tsdbConfigFromEnv builds a tsdb.Config from environment variables.
// For CLI commands running on the host (not inside Docker), TSDB_HOST_PORT
// (from .env) is the host-mapped port. Falls back to TSDB_PORT, then 5432.
func tsdbConfigFromEnv() tsdb.Config {
	port := tsdbEnvInt("TSDB_HOST_PORT", 0)
	if port == 0 {
		port = tsdbEnvInt("TSDB_PORT", 5432)
	}
	return tsdb.Config{
		Host:     tsdbEnv("TSDB_HOST", "localhost"),
		Port:     port,
		User:     tsdbEnv("TSDB_USER", "mdemg"),
		Password: tsdbEnv("TSDB_PASSWORD", "mdemg_metrics"), //nolint:gosec // G101: default dev password
		Database: tsdbEnv("TSDB_DATABASE", "mdemg_metrics"),
		SSLMode:  tsdbEnv("TSDB_SSL_MODE", "disable"),
	}
}

// newTSDBCmd creates the parent `tsdb` command with subcommands.
func newTSDBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tsdb",
		Short: "TimescaleDB management commands",
		Long:  "Commands for managing the MDEMG TimescaleDB metrics database",
	}

	cmd.AddCommand(newTSDBStartCmd())
	cmd.AddCommand(newTSDBStopCmd())
	cmd.AddCommand(newTSDBStatusCmd())
	cmd.AddCommand(newTSDBMigrateCmd())
	cmd.AddCommand(newTSDBShellCmd())
	cmd.AddCommand(newTSDBStatsCmd())
	cmd.AddCommand(newTSDBBackupCmd())

	return cmd
}

// resolveComposeFilePath returns the path to a Docker Compose file,
// checking the working directory first (Homebrew/edge install where init
// writes the embedded compose file), then the repo-relative path
// (development checkout).
func resolveComposeFilePath() string {
	if _, err := os.Stat("docker-compose.yml"); err == nil {
		return "docker-compose.yml"
	}
	if _, err := os.Stat("deploy/docker/docker-compose.observability.yml"); err == nil {
		return "deploy/docker/docker-compose.observability.yml"
	}
	return "docker-compose.yml"
}

// newTSDBStartCmd creates the `tsdb start` subcommand.
func newTSDBStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the TimescaleDB container",
		Long: `Start the TimescaleDB Docker container via docker compose.

Examples:
  mdemg tsdb start`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Starting TimescaleDB container...")
			composePath := resolveComposeFilePath()
			c := exec.Command("docker", "compose",
				"-f", composePath,
				"up", "-d", "timescaledb")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return fmt.Errorf("start timescaledb: %w", err)
			}
			fmt.Println("TimescaleDB container started")
			return nil
		},
	}
}

// newTSDBStopCmd creates the `tsdb stop` subcommand.
func newTSDBStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the TimescaleDB container",
		Long: `Stop the TimescaleDB Docker container via docker compose.

Examples:
  mdemg tsdb stop`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Stopping TimescaleDB container...")
			composePath := resolveComposeFilePath()
			c := exec.Command("docker", "compose",
				"-f", composePath,
				"stop", "timescaledb")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				return fmt.Errorf("stop timescaledb: %w", err)
			}
			fmt.Println("TimescaleDB container stopped")
			return nil
		},
	}
}

// newTSDBStatusCmd creates the `tsdb status` subcommand.
func newTSDBStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show TimescaleDB connection and schema status",
		Long: `Check the TimescaleDB connection and display schema version.

Reads connection details from environment variables:
  TSDB_HOST     (default: localhost)
  TSDB_PORT     (default: 5432)
  TSDB_USER     (default: mdemg)
  TSDB_PASSWORD (default: mdemg_metrics)
  TSDB_DATABASE (default: mdemg_metrics)
  TSDB_SSL_MODE (default: disable)

Examples:
  mdemg tsdb status`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := tsdbConfigFromEnv()

			fmt.Println("TimescaleDB:")
			fmt.Printf("  Host:     %s:%d\n", cfg.Host, cfg.Port)
			fmt.Printf("  Database: %s\n", cfg.Database)
			fmt.Printf("  User:     %s\n", cfg.User)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			client, err := tsdb.NewClient(ctx, cfg)
			if err != nil {
				fmt.Printf("  Connection: failed (%v)\n", err)
				return nil
			}
			defer client.Close()

			fmt.Println("  Connection: ok")

			version, err := client.GetSchemaVersion(ctx)
			if err != nil {
				fmt.Printf("  Schema version: unknown (%v)\n", err)
			} else {
				fmt.Printf("  Schema version: %d\n", version)
			}

			return nil
		},
	}
}

// newTSDBMigrateCmd creates the `tsdb migrate` subcommand.
func newTSDBMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Apply TimescaleDB schema migrations",
		Long: `Connect to TimescaleDB and run all pending schema migrations.

Examples:
  mdemg tsdb migrate`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := tsdbConfigFromEnv()

			fmt.Println("Connecting to TimescaleDB...")
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			client, err := tsdb.NewClient(ctx, cfg)
			if err != nil {
				return fmt.Errorf("tsdb connect: %w", err)
			}
			defer client.Close()

			fmt.Println("Running migrations...")
			if err := client.Migrate(ctx); err != nil {
				return fmt.Errorf("tsdb migrate: %w", err)
			}

			fmt.Println("Migrations complete")
			return nil
		},
	}
}

// newTSDBShellCmd creates the `tsdb shell` subcommand.
func newTSDBShellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "Open an interactive psql session to TimescaleDB",
		Long: `Open an interactive psql session connected to the TimescaleDB instance.

Reads TSDB_HOST, TSDB_USER, and TSDB_DATABASE from environment variables.

Examples:
  mdemg tsdb shell`,
		RunE: func(cmd *cobra.Command, args []string) error {
			host := tsdbEnv("TSDB_HOST", "localhost")
			user := tsdbEnv("TSDB_USER", "mdemg")
			database := tsdbEnv("TSDB_DATABASE", "mdemg_metrics")

			shellCmd := exec.Command("psql", "-h", host, "-U", user, database) //nolint:gosec // G204: user-controlled env vars for dev tool
			shellCmd.Stdin = os.Stdin
			shellCmd.Stdout = os.Stdout
			shellCmd.Stderr = os.Stderr
			return shellCmd.Run()
		},
	}
}

// newTSDBStatsCmd creates the `tsdb stats` subcommand.
func newTSDBStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Show TimescaleDB table row counts and compression stats",
		Long: `Query row counts for all metric tables and show compression settings.

Tables queried:
  metric_samples, llm_interactions, benchmark_runs,
  ft_training_cycles, ft_model_versions

Examples:
  mdemg tsdb stats`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := tsdbConfigFromEnv()

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			client, err := tsdb.NewClient(ctx, cfg)
			if err != nil {
				return fmt.Errorf("tsdb connect: %w", err)
			}
			defer client.Close()

			tables := []string{
				"metric_samples",
				"llm_interactions",
				"benchmark_runs", // V0012 replaced the V0002 ft_benchmarks
				"ft_training_cycles",
				"ft_model_versions",
				// ft_hitl_decisions dropped in V0032 (superseded by review_grades V0028)
			}

			fmt.Println("Row counts:")
			for _, table := range tables {
				var count int64
				err := client.Pool().QueryRow(ctx,
					fmt.Sprintf("SELECT count(*) FROM %s", table), //nolint:gosec // G201: table names are hardcoded constants
				).Scan(&count)
				if err != nil {
					fmt.Printf("  %-25s error (%v)\n", table, err)
				} else {
					fmt.Printf("  %-25s %d\n", table, count)
				}
			}

			fmt.Println()
			fmt.Println("Compression settings (top 5):")

			rows, err := client.Pool().Query(ctx,
				"SELECT * FROM timescaledb_information.compression_settings LIMIT 5")
			if err != nil {
				fmt.Printf("  Error: %v\n", err)
				return nil
			}
			defer rows.Close()

			cols := rows.FieldDescriptions()
			// Print header
			for i, col := range cols {
				if i > 0 {
					fmt.Print("\t")
				}
				fmt.Print(string(col.Name))
			}
			fmt.Println()

			// Print rows
			for rows.Next() {
				vals, err := rows.Values()
				if err != nil {
					fmt.Printf("  Scan error: %v\n", err)
					continue
				}
				for i, v := range vals {
					if i > 0 {
						fmt.Print("\t")
					}
					if v == nil {
						fmt.Print("<null>")
					} else {
						fmt.Printf("%v", v)
					}
				}
				fmt.Println()
			}

			return nil
		},
	}
}

// ── TSDB Backup Commands ──

func newTSDBBackupCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "TimescaleDB backup management",
		Long: `Manage TimescaleDB backups using pg_dump via docker compose exec.

Backups run automatically when enabled. Use these commands to trigger manual
backups, list existing backups, restore from a dump, or view configuration.

Backup files are stored in .mdemg/backups/tsdb/ (gitignored by default).`,
	}

	cmd.AddCommand(newTSDBBackupTriggerCmd())
	cmd.AddCommand(newTSDBBackupListCmd())
	cmd.AddCommand(newTSDBBackupConfigCmd())
	cmd.AddCommand(newTSDBBackupRestoreCmd())

	return cmd
}

func makeTSDBBackupConfig(cfg config.Config) tsdb.TSDBBackupConfig {
	return tsdb.TSDBBackupConfig{
		Enabled:             cfg.TSDBBackupEnabled,
		StorageDir:          cfg.TSDBBackupStorageDir,
		ComposeFile:         cfg.TSDBBackupComposeFile,
		ServiceName:         cfg.TSDBBackupServiceName,
		Database:            tsdbEnv("TSDB_DATABASE", "mdemg_metrics"),
		User:                tsdbEnv("TSDB_USER", "mdemg"),
		IntervalHours:       cfg.TSDBBackupIntervalHours,
		RetentionCount:      cfg.TSDBBackupRetentionCount,
		RetentionMaxAgeDays: cfg.TSDBBackupRetentionMaxAgeDays,
	}
}

func newTSDBBackupTriggerCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "trigger",
		Short: "Trigger a manual TSDB backup",
		Long: `Trigger an immediate TimescaleDB backup using pg_dump.

The backup uses docker compose exec to run pg_dump inside the TimescaleDB
container, producing a compressed custom-format dump file.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			if !cfg.TSDBBackupEnabled {
				fmt.Println("TSDB backups are disabled. Enable with:")
				fmt.Println("  TSDB_BACKUP_ENABLED=true")
				return nil
			}

			backupCfg := makeTSDBBackupConfig(cfg)
			svc := tsdb.NewTSDBBackupService(backupCfg)

			fmt.Println("Triggering TSDB backup...")
			rec, err := svc.Trigger("manual")
			if err != nil {
				return fmt.Errorf("backup trigger failed: %w", err)
			}

			fmt.Printf("Backup ID:   %s\n", rec.BackupID)
			fmt.Printf("Status:      %s\n", rec.Status)
			fmt.Printf("Path:        %s\n", rec.Path)
			if rec.SizeBytes > 0 {
				fmt.Printf("Size:        %s\n", backupFormatBytes(rec.SizeBytes))
			}
			if rec.Checksum != "" {
				fmt.Printf("Checksum:    %s\n", rec.Checksum)
			}
			return nil
		},
	}
}

func newTSDBBackupListCmd() *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List existing TSDB backups",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			backupCfg := tsdb.TSDBBackupConfig{
				StorageDir: cfg.TSDBBackupStorageDir,
			}
			svc := tsdb.NewTSDBBackupService(backupCfg)

			backups, err := svc.ListBackups(limit)
			if err != nil {
				return fmt.Errorf("list backups: %w", err)
			}

			if len(backups) == 0 {
				fmt.Println("No TSDB backups found.")
				fmt.Printf("Storage directory: %s\n", cfg.TSDBBackupStorageDir)
				return nil
			}

			fmt.Printf("%-35s  %-10s  %-10s  %s\n", "BACKUP ID", "SIZE", "LABEL", "CREATED")
			fmt.Printf("%-35s  %-10s  %-10s  %s\n", "─────────", "────", "─────", "───────")
			for _, b := range backups {
				size := backupFormatBytes(b.SizeBytes)
				created := b.CreatedAt
				if t, tErr := time.Parse(time.RFC3339, b.CreatedAt); tErr == nil {
					created = t.Format("2006-01-02 15:04")
				}
				label := b.Label
				if label == "" {
					label = "-"
				}
				fmt.Printf("%-35s  %-10s  %-10s  %s\n", b.BackupID, size, label, created)
			}
			fmt.Printf("\nTotal: %d backup(s) in %s\n", len(backups), cfg.TSDBBackupStorageDir)
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum number of backups to show")

	return cmd
}

func newTSDBBackupConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Show current TSDB backup configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			fmt.Println("TSDB Backup Configuration")
			fmt.Println("=========================")
			fmt.Printf("  Enabled:              %v\n", cfg.TSDBBackupEnabled)
			fmt.Printf("  Storage directory:     %s\n", cfg.TSDBBackupStorageDir)
			fmt.Printf("  Compose file:          %s\n", displayOrDefault(cfg.TSDBBackupComposeFile, "(auto-detect)"))
			fmt.Printf("  Service name:          %s\n", cfg.TSDBBackupServiceName)
			fmt.Println()
			fmt.Println("Schedule:")
			fmt.Printf("  Backup interval:       every %dh (%s)\n", cfg.TSDBBackupIntervalHours, backupFormatHours(cfg.TSDBBackupIntervalHours))
			fmt.Println()
			fmt.Println("Retention:")
			fmt.Printf("  Backups kept:          %d\n", cfg.TSDBBackupRetentionCount)
			fmt.Printf("  Max age:               %d days\n", cfg.TSDBBackupRetentionMaxAgeDays)
			fmt.Println()
			fmt.Println("Environment variables:")
			fmt.Println("  TSDB_BACKUP_ENABLED, TSDB_BACKUP_STORAGE_DIR,")
			fmt.Println("  TSDB_BACKUP_COMPOSE_FILE, TSDB_BACKUP_SERVICE,")
			fmt.Println("  TSDB_BACKUP_INTERVAL_HOURS, TSDB_BACKUP_RETENTION_COUNT,")
			fmt.Println("  TSDB_BACKUP_RETENTION_MAX_AGE_DAYS")
			return nil
		},
	}
}

func newTSDBBackupRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <file>",
		Short: "Restore TSDB from a backup dump",
		Long: `Restore TimescaleDB from a pg_dump backup file.

The restore uses pg_restore with --clean --if-exists to drop and recreate
objects before restoring. This is a destructive operation.

Examples:
  mdemg tsdb backup restore .mdemg/backups/tsdb/tsdb-bk-20260328-120000.dump`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			backupCfg := makeTSDBBackupConfig(cfg)
			svc := tsdb.NewTSDBBackupService(backupCfg)

			dumpPath := args[0]
			fmt.Printf("Restoring TSDB from: %s\n", dumpPath)
			fmt.Println("WARNING: This will overwrite the current database contents.")

			if err := svc.Restore(dumpPath); err != nil {
				return fmt.Errorf("restore failed: %w", err)
			}

			fmt.Println("Restore completed successfully.")
			return nil
		},
	}
}

func displayOrDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}
