package cli

import (
	"bufio"
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/spf13/cobra"
	"mdemg/internal/api"
	"mdemg/internal/config"
	"mdemg/internal/db"
	"mdemg/migrations"
)

// resolveProjectContainer returns the project-scoped Neo4j container and volume
// names based on the current working directory.
func resolveProjectContainer() (containerName, volumeName string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("get working directory: %w", err)
	}
	return ContainerNameForProject(cwd), VolumeNameForProject(cwd), nil
}

// newDBCmd creates the parent `db` command with subcommands
func newDBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Database management commands",
		Long:  "Commands for managing the MDEMG Neo4j database",
	}

	cmd.AddCommand(newDBResetCmd())
	cmd.AddCommand(newDBMigrateCmd())
	cmd.AddCommand(newDBStartCmd())
	cmd.AddCommand(newDBStopCmd())
	cmd.AddCommand(newDBStatusCmd())
	cmd.AddCommand(newDBShellCmd())
	cmd.AddCommand(newDBBackupCmd())

	return cmd
}

// newDBResetCmd creates the `db reset` subcommand
func newDBResetCmd() *cobra.Command {
	var (
		spaceID      string
		deleteAll    bool
		skipConfirm  bool
	)

	cmd := &cobra.Command{
		Use:   "reset",
		Short: "Reset database by deleting nodes",
		Long: `Delete nodes from the database, either for a specific space or all non-protected spaces.

Protected spaces are never deleted and include:
  - mdemg-dev (conversation memory)

Examples:
  mdemg db reset --space-id my-test-space
  mdemg db reset --all --yes`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Validation
			if spaceID == "" && !deleteAll {
				fmt.Fprintln(os.Stderr, "Usage: mdemg db reset --space-id <space> [--yes]")
				fmt.Fprintln(os.Stderr, "       mdemg db reset --all [--yes]")
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, "Protected spaces (never deleted):")
				for id := range api.ProtectedSpaces {
					fmt.Fprintf(os.Stderr, "  - %s\n", id)
				}
				return fmt.Errorf("must specify either --space-id or --all")
			}

			// Reject attempts to delete protected spaces
			if spaceID != "" && api.ProtectedSpaces[spaceID] {
				return fmt.Errorf("space '%s' is protected and cannot be deleted", spaceID)
			}

			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			ctx := context.Background()
			driver, err := db.NewDriver(cfg)
			if err != nil {
				return fmt.Errorf("driver error: %w", err)
			}
			defer driver.Close(ctx)

			// Confirmation prompt
			if !skipConfirm {
				target := spaceID
				if deleteAll {
					target = "ALL non-protected spaces"
				}
				fmt.Printf("⚠ This will DELETE all nodes in: %s\n", target)
				fmt.Printf("Protected spaces (safe): %v\n", protectedSpaceList())
				fmt.Print("Type 'yes' to confirm: ")
				scanner := bufio.NewScanner(os.Stdin)
				scanner.Scan()
				if strings.TrimSpace(scanner.Text()) != "yes" {
					fmt.Println("Aborted.")
					return nil
				}
			}

			if spaceID != "" {
				fmt.Printf("[STATUS %s] Starting delete for space: %s\n", ts(), spaceID)
			} else {
				fmt.Printf("[STATUS %s] Starting database clear (all non-protected)...\n", ts())
			}

			// Get initial count
			sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
			result, err := sess.Run(ctx, "MATCH (n) RETURN count(n) as nodes", nil)
			if err != nil {
				sess.Close(ctx)
				return fmt.Errorf("count query error: %w", err)
			}
			initialCount := int64(0)
			if result.Next(ctx) {
				initialCount = result.Record().Values[0].(int64)
			}
			sess.Close(ctx)
			fmt.Printf("[STATUS %s] Initial node count: %d\n", ts(), initialCount)

			if initialCount == 0 {
				fmt.Printf("[STATUS %s] Database is already empty\n", ts())
				return nil
			}

			// Print protected spaces
			psList := protectedSpaceList()
			if len(psList) > 0 {
				fmt.Printf("[STATUS %s] PROTECTED SPACES (will NOT be deleted): %v\n", ts(), psList)
			}

			// Delete in batches to avoid transaction limits
			batchSize := 10000
			totalDeleted := int64(0)

			for {
				sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})

				var cypher string
				if spaceID != "" {
					// Targeted: delete only from specified space
					cypher = fmt.Sprintf(`
				MATCH (n)
				WHERE n.space_id = '%s'
				WITH n LIMIT %d
				DETACH DELETE n
				RETURN count(*) as deleted
			`, spaceID, batchSize)
				} else {
					// All mode: exclude protected spaces
					if len(psList) > 0 {
						exclusions := make([]string, len(psList))
						for i, sid := range psList {
							exclusions[i] = fmt.Sprintf("n.space_id <> '%s'", sid)
						}
						whereClause := strings.Join(exclusions, " AND ")
						cypher = fmt.Sprintf(`
					MATCH (n)
					WHERE (%s) OR n.space_id IS NULL
					WITH n LIMIT %d
					DETACH DELETE n
					RETURN count(*) as deleted
				`, whereClause, batchSize)
					} else {
						cypher = fmt.Sprintf(`
					MATCH (n)
					WITH n LIMIT %d
					DETACH DELETE n
					RETURN count(*) as deleted
				`, batchSize)
					}
				}

				result, err := sess.Run(ctx, cypher, nil)
				if err != nil {
					sess.Close(ctx)
					return fmt.Errorf("delete query error: %w", err)
				}

				deleted := int64(0)
				if result.Next(ctx) {
					deleted = result.Record().Values[0].(int64)
				}
				sess.Close(ctx)

				totalDeleted += deleted
				fmt.Printf("[STATUS %s] Deleted batch: %d (total: %d)\n", ts(), deleted, totalDeleted)

				if deleted == 0 {
					break
				}
			}

			// Verify
			fmt.Printf("[STATUS %s] Verifying...\n", ts())
			sess = driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
			defer sess.Close(ctx)

			result, err = sess.Run(ctx, "MATCH (n) RETURN count(n) as nodes", nil)
			if err != nil {
				return fmt.Errorf("verify query error: %w", err)
			}

			if result.Next(ctx) {
				count := result.Record().Values[0]
				fmt.Printf("[STATUS %s] Remaining nodes: %v\n", ts(), count)
				if count.(int64) == 0 {
					fmt.Printf("[STATUS %s] Database cleared successfully!\n", ts())
				} else {
					fmt.Println("[WARNING] Some nodes may remain - check for constraints or protected spaces")
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&spaceID, "space-id", "", "Delete only this space ID (required unless --all is set)")
	cmd.Flags().BoolVar(&deleteAll, "all", false, "Delete all non-protected spaces (requires confirmation)")
	cmd.Flags().BoolVarP(&skipConfirm, "yes", "y", false, "Skip confirmation prompt")

	return cmd
}

// ts returns a timestamp string in HH:MM:SS format
func ts() string {
	return time.Now().Format("15:04:05")
}

// protectedSpaceList returns a list of all protected space IDs
func protectedSpaceList() []string {
	var list []string
	for id := range api.ProtectedSpaces {
		list = append(list, id)
	}
	return list
}

// newDBMigrateCmd creates the `db migrate` subcommand
func newDBMigrateCmd() *cobra.Command {
	var (
		statusOnly    bool
		dryRun        bool
		migrationsDir string
	)

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Apply pending database migrations",
		Long: `Apply pending Neo4j schema migrations.

By default, uses migrations embedded in the binary. Use --migrations-dir
to override with a filesystem directory (useful during development).

Examples:
  mdemg db migrate              # Apply all pending migrations
  mdemg db migrate --status     # Show current/available versions
  mdemg db migrate --dry-run    # Show what would be applied`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return fmt.Errorf("config error: %w", err)
			}

			ctx := context.Background()
			driver, err := db.NewDriver(cfg)
			if err != nil {
				return fmt.Errorf("database connection failed: %w", err)
			}
			defer driver.Close(ctx)

			// Choose migration source
			var migFS fs.FS
			if migrationsDir != "" {
				migFS = os.DirFS(migrationsDir)
				fmt.Printf("Using migrations from: %s\n", migrationsDir)
			} else {
				migFS = migrations.FS
				fmt.Println("Using embedded migrations")
			}

			available, err := db.DiscoverMigrations(migFS)
			if err != nil {
				return fmt.Errorf("discover migrations: %w", err)
			}

			status, err := db.GetMigrationStatus(ctx, driver, available)
			if err != nil {
				return fmt.Errorf("get migration status: %w", err)
			}

			fmt.Printf("Current version:   %d\n", status.CurrentVersion)
			fmt.Printf("Available version: %d\n", status.AvailableVersion)
			fmt.Printf("Applied:           %d migrations\n", len(status.Applied))
			fmt.Printf("Pending:           %d migrations\n", len(status.Pending))

			if statusOnly {
				if len(status.Pending) > 0 {
					fmt.Println("\nPending migrations:")
					for _, m := range status.Pending {
						fmt.Printf("  %s (%d statements)\n", m.Filename, len(m.Statements))
					}
				}
				return nil
			}

			if len(status.Pending) == 0 {
				fmt.Println("\nDatabase is up to date.")
				return nil
			}

			if dryRun {
				fmt.Println("\nDry run — would apply:")
				for _, m := range status.Pending {
					fmt.Printf("  %s (%d statements)\n", m.Filename, len(m.Statements))
				}
				return nil
			}

			fmt.Println()
			for _, mig := range status.Pending {
				fmt.Printf("Applying %s (%d statements)...\n", mig.Filename, len(mig.Statements))
				if err := db.ApplyMigration(ctx, driver, mig); err != nil {
					return fmt.Errorf("migration failed: %w", err)
				}
				fmt.Printf("  Applied version %d\n", mig.Version)
			}

			fmt.Printf("\nDone. Applied %d migrations (version %d -> %d)\n",
				len(status.Pending), status.CurrentVersion, status.AvailableVersion)
			return nil
		},
	}

	cmd.Flags().BoolVar(&statusOnly, "status", false, "Show migration status only")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be applied without applying")
	cmd.Flags().StringVar(&migrationsDir, "migrations-dir", "", "Override embedded migrations with filesystem directory")

	return cmd
}

// neo4jPass returns the Neo4j password from the environment, falling back to the
// static default for backward compatibility. GAP-19: prefer env-sourced credentials.
func neo4jPass() string {
	if p := os.Getenv("NEO4J_PASS"); p != "" {
		return p
	}
	return "mdemg-dev" //nolint:gosec // G101: fallback for dev
}

// runDBStart is the core logic for starting a Neo4j container.
// boltPort/httpPort of 0 means "use config or auto-detect".
func runDBStart(boltPort, httpPort int, password string) error {
	if !DockerAvailable() {
		return fmt.Errorf("docker is not installed or not in PATH\nInstall Docker: https://docs.docker.com/get-docker/")
	}

	// Use defaults if not specified — read from env first, then fall back to static default (GAP-19)
	if password == "" {
		password = os.Getenv("NEO4J_PASS")
	}
	if password == "" {
		password = "mdemg-dev" //nolint:gosec // G101: default dev password, not a credential
	}
	portExplicit := boltPort > 0
	httpExplicit := httpPort > 0

	// Read config ports as fallback
	if !portExplicit || !httpExplicit {
		if cfg, cfgErr := loadConfig(); cfgErr == nil {
			if !portExplicit && cfg.Neo4jBoltPort > 0 {
				boltPort = cfg.Neo4jBoltPort
			}
			if !httpExplicit && cfg.Neo4jHTTPPort > 0 {
				httpPort = cfg.Neo4jHTTPPort
			}
		}
	}
	if boltPort == 0 {
		boltPort = neo4jDefaultPort
	}
	if httpPort == 0 {
		httpPort = neo4jDefaultHTTP
	}

	containerName, _, err := resolveProjectContainer()
	if err != nil {
		return err
	}

	// Check for volume name mismatch between legacy CLI and docker compose,
	// and migrate data if needed (GAP-06).
	cwd, _ := os.Getwd()
	volumeName := tryMigrateVolume(cwd)

	state, err := InspectContainer(containerName)
	if err != nil {
		return fmt.Errorf("inspect container: %w", err)
	}

	if state.Running {
		actualBolt, actualHTTP := ReadContainerPorts(containerName)
		if actualBolt == 0 {
			actualBolt = boltPort
		}
		if actualHTTP == 0 {
			actualHTTP = httpPort
		}
		fmt.Printf("Container '%s' is already running\n", containerName)
		fmt.Printf("  Bolt:  bolt://localhost:%d\n", actualBolt)
		fmt.Printf("  HTTP:  http://localhost:%d\n", actualHTTP)
		return nil
	}

	if state.Exists {
		fmt.Printf("Starting existing container '%s'...\n", containerName)
		_, err := RunDockerCommand("start", containerName)
		if err != nil {
			return fmt.Errorf("start container: %w", err)
		}
		actualBolt, actualHTTP := ReadContainerPorts(containerName)
		if actualBolt > 0 {
			boltPort = actualBolt
		}
		if actualHTTP > 0 {
			httpPort = actualHTTP
		}
	} else {
		// Dynamic port selection when not explicitly set
		if !portExplicit {
			found, portErr := FindFreePort(boltPort, 7687, 7787)
			if portErr != nil {
				return fmt.Errorf("find available bolt port: %w", portErr)
			}
			boltPort = found
		}
		if !httpExplicit {
			found, portErr := FindFreePort(httpPort, 7474, 7574)
			if portErr != nil {
				return fmt.Errorf("find available HTTP port: %w", portErr)
			}
			httpPort = found
		}

		fmt.Printf("Creating container '%s'...\n", containerName)
		dockerArgs := []string{
			"run", "-d",
			"--name", containerName,
			"-p", fmt.Sprintf("%d:7687", boltPort),
			"-p", fmt.Sprintf("%d:7474", httpPort),
			"-e", fmt.Sprintf("NEO4J_AUTH=neo4j/%s", password),
			"-e", "NEO4J_server_memory_heap_initial__size=512m",
			"-e", "NEO4J_server_memory_heap_max__size=1g",
			"-e", "NEO4J_server_memory_pagecache_size=512m",
			"-e", "NEO4J_PLUGINS=[\"apoc\"]",
			"-v", volumeName + ":/data",
			"-v", volumeName + "-backup:/backup",
			neo4jImage,
		}
		_, err := RunDockerCommand(dockerArgs...)
		if err != nil {
			return fmt.Errorf("create container: %w", err)
		}
	}

	fmt.Printf("Waiting for Neo4j to be ready...")
	if err := WaitForPort("localhost", boltPort, 60*time.Second); err != nil {
		fmt.Println(" timeout")
		return fmt.Errorf("neo4j did not become ready: %w", err)
	}
	fmt.Println(" ready")

	boltURI := fmt.Sprintf("bolt://localhost:%d", boltPort)
	configPath := config.FindConfigFile()
	if configPath == "" {
		cwd, _ := os.Getwd()
		configPath = filepath.Join(cwd, ".mdemg", "config.yaml")
	}
	if err := config.UpdateNeo4jURI(configPath, boltURI); err != nil {
		fmt.Printf("  Warning: could not update config: %v\n", err)
	} else {
		fmt.Printf("  Config updated: neo4j.uri = %s\n", boltURI)
	}

	fmt.Println()
	fmt.Printf("Neo4j is running:\n")
	fmt.Printf("  Container: %s\n", containerName)
	fmt.Printf("  Bolt:      bolt://localhost:%d\n", boltPort)
	fmt.Printf("  Browser:   http://localhost:%d\n", httpPort)
	fmt.Printf("  User:      neo4j\n")
	fmt.Printf("  Password:  %s\n", password)
	fmt.Printf("  Volume:    %s\n", volumeName)
	return nil
}

// newDBStartCmd creates the `db start` subcommand
func newDBStartCmd() *cobra.Command {
	var (
		boltPort int
		httpPort int
		password string
	)

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start a local Neo4j container for development",
		Long: `Start a lightweight Neo4j Docker container for local development.

Uses reduced memory settings (1GB heap, 512MB page cache) suitable for
development. Data is persisted in a Docker volume.

The container name and volume are scoped to the current project directory,
so multiple projects can each have their own isolated Neo4j instance.

If the default port (7687) is already in use, an available port is
automatically selected from the range 7687-7787.

Examples:
  mdemg db start
  mdemg db start --port 7688 --password mypassword`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Pass 0 for ports not explicitly set (triggers config/auto-detect)
			bp := boltPort
			if !cmd.Flags().Lookup("port").Changed {
				bp = 0
			}
			hp := httpPort
			if !cmd.Flags().Lookup("http-port").Changed {
				hp = 0
			}
			return runDBStart(bp, hp, password)
		},
	}

	cmd.Flags().IntVar(&boltPort, "port", neo4jDefaultPort, "Bolt protocol port")
	cmd.Flags().IntVar(&httpPort, "http-port", neo4jDefaultHTTP, "HTTP browser port")
	defaultPass := os.Getenv("NEO4J_PASS")
	if defaultPass == "" {
		defaultPass = "mdemg-dev" //nolint:gosec // G101: fallback for dev
	}
	cmd.Flags().StringVar(&password, "password", defaultPass, "Neo4j password")

	return cmd
}

// newDBStopCmd creates the `db stop` subcommand
func newDBStopCmd() *cobra.Command {
	var remove bool

	cmd := &cobra.Command{
		Use:   "stop",
		Short: "Stop the local Neo4j container",
		Long: `Stop the MDEMG Neo4j development container.

Use --remove to also remove the container (data volume is preserved).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !DockerAvailable() {
				return fmt.Errorf("docker is not installed or not in PATH")
			}

			containerName, _, err := resolveProjectContainer()
			if err != nil {
				return err
			}

			state, err := InspectContainer(containerName)
			if err != nil {
				return fmt.Errorf("inspect container: %w", err)
			}

			if !state.Exists {
				fmt.Printf("Container '%s' does not exist\n", containerName)
				return nil
			}

			if state.Running {
				fmt.Printf("Stopping container '%s'...\n", containerName)
				if _, err := RunDockerCommand("stop", containerName); err != nil {
					return fmt.Errorf("stop container: %w", err)
				}
				fmt.Println("Stopped")
			} else {
				fmt.Printf("Container '%s' is not running (status: %s)\n", containerName, state.Status)
			}

			if remove {
				fmt.Printf("Removing container '%s'...\n", containerName)
				if _, err := RunDockerCommand("rm", containerName); err != nil {
					return fmt.Errorf("remove container: %w", err)
				}
				fmt.Println("Removed (data volume preserved)")
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&remove, "remove", false, "Also remove the container (data volume is preserved)")

	return cmd
}

// newDBStatusCmd creates the `db status` subcommand
func newDBStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show database container and schema status",
		RunE: func(cmd *cobra.Command, args []string) error {
			containerName, _, cnErr := resolveProjectContainer()
			if cnErr != nil {
				containerName = neo4jContainerName // fallback
			}

			fmt.Println("Container:")
			if !DockerAvailable() {
				fmt.Println("  Docker: not installed")
			} else {
				state, err := InspectContainer(containerName)
				if err != nil {
					fmt.Printf("  Error: %v\n", err)
				} else if !state.Exists {
					fmt.Printf("  %s: not created\n", containerName)
				} else {
					fmt.Printf("  %s: %s\n", containerName, state.Status)
					if state.Running {
						actualBolt, actualHTTP := ReadContainerPorts(containerName)
						if actualBolt > 0 {
							fmt.Printf("  Bolt:  bolt://localhost:%d\n", actualBolt)
						}
						if actualHTTP > 0 {
							fmt.Printf("  HTTP:  http://localhost:%d\n", actualHTTP)
						}
					}
				}
			}

			fmt.Println()
			fmt.Println("Schema:")

			cfg, err := loadConfig()
			if err != nil {
				fmt.Printf("  Config error: %v\n", err)
				return nil
			}

			ctx := context.Background()
			driver, err := db.NewDriver(cfg)
			if err != nil {
				fmt.Printf("  Connection: failed (%v)\n", err)
				return nil
			}
			defer driver.Close(ctx)

			if err := db.VerifyConnectivity(ctx, driver); err != nil {
				fmt.Printf("  Connection: unreachable (%v)\n", err)
				return nil
			}
			fmt.Println("  Connection: ok")

			ver, err := db.GetSchemaVersion(ctx, driver)
			if err != nil {
				fmt.Printf("  Schema version: unknown (%v)\n", err)
			} else {
				fmt.Printf("  Schema version: %d\n", ver)
			}

			maxVer, err := migrations.MaxVersion()
			if err == nil {
				fmt.Printf("  Available version: %d\n", maxVer)
				if ver < maxVer {
					fmt.Printf("  Status: %d migration(s) pending — run 'mdemg db migrate'\n", maxVer-ver)
				} else {
					fmt.Println("  Status: up to date")
				}
			}

			return nil
		},
	}
}

// newDBShellCmd creates the `db shell` subcommand
func newDBShellCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell",
		Short: "Open an interactive cypher-shell session",
		Long: `Open an interactive cypher-shell inside the running Neo4j container.

Requires the project's Neo4j container to be running (via mdemg db start).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if !DockerAvailable() {
				return fmt.Errorf("docker is not installed or not in PATH")
			}

			containerName, _, err := resolveProjectContainer()
			if err != nil {
				return err
			}

			state, err := InspectContainer(containerName)
			if err != nil {
				return fmt.Errorf("inspect container: %w", err)
			}
			if !state.Exists || !state.Running {
				return fmt.Errorf("container '%s' is not running — run 'mdemg db start' first", containerName)
			}

			// exec into container with interactive cypher-shell
			shellPass := os.Getenv("NEO4J_PASS")
			if shellPass == "" {
				shellPass = "mdemg-dev" //nolint:gosec // G101: fallback for dev
			}
			shellCmd := exec.Command("docker", "exec", "-it", containerName,
				"cypher-shell", "-u", "neo4j", "-p", shellPass)
			shellCmd.Stdin = os.Stdin
			shellCmd.Stdout = os.Stdout
			shellCmd.Stderr = os.Stderr
			return shellCmd.Run()
		},
	}
}
