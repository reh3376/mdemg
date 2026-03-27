package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/spf13/cobra"
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
func tsdbConfigFromEnv() tsdb.Config {
	return tsdb.Config{
		Host:     tsdbEnv("TSDB_HOST", "localhost"),
		Port:     tsdbEnvInt("TSDB_PORT", 5432),
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

	return cmd
}

// newTSDBStartCmd creates the `tsdb start` subcommand.
func newTSDBStartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "start",
		Short: "Start the TimescaleDB container",
		Long: `Start the TimescaleDB Docker container via docker compose.

Uses the observability compose file at deploy/docker/docker-compose.observability.yml.

Examples:
  mdemg tsdb start`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Starting TimescaleDB container...")
			c := exec.Command("docker", "compose",
				"-f", "deploy/docker/docker-compose.observability.yml",
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
			c := exec.Command("docker", "compose",
				"-f", "deploy/docker/docker-compose.observability.yml",
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
  metric_samples, llm_interactions, ft_benchmarks,
  ft_training_cycles, ft_model_versions, ft_hitl_decisions

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
				"ft_benchmarks",
				"ft_training_cycles",
				"ft_model_versions",
				"ft_hitl_decisions",
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
