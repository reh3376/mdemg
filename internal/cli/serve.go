package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/alert"
	"mdemg/internal/api"
	"mdemg/internal/config"
	"mdemg/internal/db"
	"mdemg/internal/healthprobe"
	"mdemg/internal/llmclient"
	mlog "mdemg/internal/logging"
	"mdemg/internal/plugins"
	"mdemg/internal/tsdb"
	"mdemg/migrations"
)

func newServeCmd() *cobra.Command {
	var port int
	var dbURI string
	var autoMigrate bool
	var mcpEnabled bool
	var logLevel string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the MDEMG HTTP API server",
		Long: `Start the MDEMG HTTP API server.

Configuration is primarily read from environment variables (via .env file or ENV).
CLI flags override environment variables for common settings.

The server will:
  - Load configuration from .env file (if present)
  - Apply CLI flag overrides (--port, --db-uri)
  - Connect to Neo4j database
  - Apply pending migrations (if --auto-migrate)
  - Verify schema version
  - Initialize plugin manager (if enabled)
  - Start periodic background tasks (consolidation, sync, RSIC, pruning)
  - Start HTTP server with graceful shutdown support
  - Write port file for client discovery

See config.FromEnv() for the full list of environment variable options.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd, args, port, dbURI, autoMigrate, mcpEnabled, logLevel)
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "Listen port (overrides LISTEN_ADDR env var)")
	cmd.Flags().StringVar(&dbURI, "db-uri", "", "Neo4j URI (overrides NEO4J_URI env var)")
	cmd.Flags().BoolVar(&autoMigrate, "auto-migrate", false, "Apply pending database migrations before starting")
	cmd.Flags().BoolVar(&mcpEnabled, "mcp", false, "Start MCP server subprocess alongside HTTP server")
	cmd.Flags().StringVar(&logLevel, "log-level", "", "Log level: debug, info, warn, error (overrides LOG_LEVEL env var)")

	return cmd
}

func runServe(cmd *cobra.Command, _ []string, port int, dbURI string, autoMigrate bool, mcpEnabled bool, logLevel string) error {
	// Pass CLI build commit to config so healthz can expose it
	if os.Getenv("MDEMG_COMMIT") == "" && Commit != "" {
		_ = os.Setenv("MDEMG_COMMIT", Commit)
	}

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

	// Support AUTO_MIGRATE / TSDB_AUTO_MIGRATE env vars for Docker deployments
	// where CLI flags aren't available. AUTO_MIGRATE covers both Neo4j and TSDB.
	if !autoMigrate {
		if os.Getenv("AUTO_MIGRATE") == "true" || os.Getenv("TSDB_AUTO_MIGRATE") == "true" {
			autoMigrate = true
		}
	}

	// Resolve log level: --log-level flag > --verbose flag > LOG_LEVEL env > "info"
	effectiveLevel := cfg.LogLevel
	if logLevel != "" {
		effectiveLevel = logLevel
	} else if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
		effectiveLevel = "debug"
	}
	cfg.LogLevel = effectiveLevel

	// Initialize structured logging — bridges stdlib log through slog
	// DOCKER-P2: Capture logs in ring buffer for browser dashboard
	logBuf := api.NewLogRingBuffer(500)
	mlog.Init(cfg.LogFormat, effectiveLevel, io.MultiWriter(os.Stderr, logBuf))

	// CLI flag overrides
	if port > 0 {
		cfg.ListenAddr = fmt.Sprintf(":%d", port)
	}
	if dbURI != "" {
		cfg.Neo4jURI = dbURI
	}

	driver, err := db.NewDriver(cfg)
	if err != nil {
		return fmt.Errorf("database connection failed: %w", err)
	}
	defer driver.Close(context.Background())

	// Start pool metrics collector (10s probe interval)
	cancelPoolMetrics := db.StartPoolMetricsCollector(context.Background(), driver)
	defer cancelPoolMetrics()

	// Auto-migrate if requested
	if autoMigrate {
		applied, migrateErr := db.RunMigrations(context.Background(), driver, migrations.FS)
		if migrateErr != nil {
			return fmt.Errorf("auto-migrate failed: %w", migrateErr)
		}
		if applied > 0 {
			slog.Info("auto-migrate: applied migrations", "count", applied)
		}
	}

	// Readiness check: schema version
	if err := db.AssertSchemaVersion(context.Background(), driver, cfg.RequiredSchemaVersion); err != nil {
		return fmt.Errorf("schema version check failed: %w", err)
	}

	// TimescaleDB initialization (if enabled)
	var tsdbClient *tsdb.Client
	if cfg.TSDBEnabled {
		tsdbCfg := tsdb.Config{
			Host:     cfg.TSDBHost,
			Port:     cfg.TSDBPort,
			User:     cfg.TSDBUser,
			Password: cfg.TSDBPassword,
			Database: cfg.TSDBDatabase,
			SSLMode:  cfg.TSDBSSLMode,
			MaxConns: int32(cfg.TSDBMaxConns),
		}
		tc, tsdbErr := tsdb.NewClient(context.Background(), tsdbCfg)
		if tsdbErr != nil {
			if cfg.TSDBOptional {
				slog.Warn("TimescaleDB unavailable (TSDB_OPTIONAL=true), continuing without TSDB", "error", tsdbErr)
			} else {
				return fmt.Errorf("TimescaleDB connection failed: %w", tsdbErr)
			}
		} else {
			tsdbClient = tc
			defer tsdbClient.Close()

			// Auto-migrate TimescaleDB if requested
			if autoMigrate {
				if migrateErr := tsdbClient.Migrate(context.Background()); migrateErr != nil {
					if cfg.TSDBOptional {
						slog.Warn("TimescaleDB migration failed (TSDB_OPTIONAL=true)", "error", migrateErr)
					} else {
						return fmt.Errorf("tsdb migration: %w", migrateErr)
					}
				} else {
					slog.Info("auto-migrate: TimescaleDB migrations applied")
				}
			}

			// Backfill instance_id on pre-migration-008 records (idempotent)
			if cfg.InstanceID != "" {
				if backfillErr := tsdb.BackfillInstanceID(context.Background(), tsdbClient.Pool(), cfg.InstanceID); backfillErr != nil {
					slog.Warn("tsdb: instance_id backfill failed", "error", backfillErr)
				}
			}

			// Schema version enforcement
			tsdbVer, verErr := tsdbClient.GetSchemaVersion(context.Background())
			if verErr != nil {
				if !cfg.TSDBOptional {
					return fmt.Errorf("TimescaleDB schema check failed: %w", verErr)
				}
				slog.Warn("TimescaleDB schema check failed (TSDB_OPTIONAL=true)", "error", verErr)
			} else if tsdbVer < cfg.TSDBRequiredSchemaVersion {
				if !cfg.TSDBOptional {
					return fmt.Errorf("TimescaleDB schema version %d < required %d — run: mdemg tsdb migrate", tsdbVer, cfg.TSDBRequiredSchemaVersion)
				}
				slog.Warn("TimescaleDB schema version behind", "have", tsdbVer, "required", cfg.TSDBRequiredSchemaVersion)
			} else {
				slog.Info("TimescaleDB ready", "schema_version", tsdbVer)
			}
		}
	}

	// Initialize Plugin Manager if enabled
	var pluginMgr *plugins.Manager
	if cfg.PluginsEnabled {
		pluginMgr = plugins.NewManager(cfg.PluginsDir, cfg.PluginSocketDir, cfg.MdemgVersion)
		if err := pluginMgr.Start(); err != nil {
			slog.Warn("failed to start plugin manager", "error", err)
			// Continue without plugins - this is not fatal
		} else {
			slog.Info("plugin manager started", "dir", cfg.PluginsDir)
		}
	}

	// SR-001: Set LLM retry defaults BEFORE NewServer creates LLM clients.
	llmclient.SetDefaultRetryConfig(llmclient.RetryConfig{
		Enabled:     cfg.LLMRetryEnabled,
		MaxAttempts: cfg.LLMRetryMaxAttempts,
		BaseDelayMs: cfg.LLMRetryBaseDelayMs,
		MaxDelayMs:  cfg.LLMRetryMaxDelayMs,
		Multiplier:  cfg.LLMRetryMultiplier,
		Jitter:      cfg.LLMRetryJitter,
	})

	// Set package-level LLM recording defaults BEFORE NewServer creates LLM clients.
	// Without this, clients created during NewServer (query classifier, intent translator)
	// get recorder=nil and silently drop all TSDB recording.
	var earlyLLMWriter *tsdb.LLMInteractionWriter
	if tsdbClient != nil && cfg.LLMInteractionLogging {
		earlyLLMWriter = tsdb.NewLLMInteractionWriter(
			tsdbClient.Pool(),
			time.Duration(cfg.TSDBFlushIntervalSec)*time.Second,
			cfg.TSDBWriterBufferMaxSize,
		)
		llmclient.SetDefaultRecorder(earlyLLMWriter)
		llmclient.SetDefaultInstanceID(cfg.InstanceID)
		llmclient.SetDefaultSpaceID(cfg.RSICWatchdogSpaceID)
		llmclient.SetDefaultSessionID("") // empty default — callers provide via WithSessionID
		slog.Info("tsdb: early LLM recorder attached (pre-server init)")
	}

	// SR-001: Start health prober (probes API, Neo4j, TSDB, sidecar)
	var prober *healthprobe.Prober
	if cfg.HealthProbeEnabled {
		apiURL := fmt.Sprintf("http://localhost%s", cfg.ListenAddr)
		var sidecarArgs []string
		if cfg.J17SidecarURL != "" {
			sidecarArgs = append(sidecarArgs, cfg.J17SidecarURL)
		}
		prober = healthprobe.New(
			time.Duration(cfg.HealthProbeIntervalSec)*time.Second,
			apiURL, driver, tsdbClient, sidecarArgs...,
		)
		prober.Start()
		defer prober.Stop()
		slog.Info("health prober started", "interval_sec", cfg.HealthProbeIntervalSec)
	}

	srv := api.NewServer(cfg, driver, pluginMgr)
	srv.SetLogBuffer(logBuf)

	// Wire TimescaleDB client if available
	if tsdbClient != nil {
		if earlyLLMWriter != nil {
			srv.SetLLMWriter(earlyLLMWriter)
		}
		srv.SetTSDBClient(tsdbClient)
	}

	// SR-001: Wire alert callbacks for prober and TSDB writer
	if disp := srv.AlertDispatcher(); disp != nil {
		if prober != nil {
			prober.SetAlertCallback(func(target string, healthy bool, errMsg string) {
				sev := alert.SeverityHigh
				title := fmt.Sprintf("Health probe failed: %s", target)
				msg := errMsg
				if healthy {
					sev = alert.SeverityLow
					title = fmt.Sprintf("Health probe recovered: %s", target)
					msg = "target is healthy again"
				}
				disp.SendAlert(context.Background(), "health-"+target, title, msg, sev)
			})
		}
		if earlyLLMWriter != nil {
			earlyLLMWriter.SetAlertCallback(func(msg string) {
				disp.SendAlert(context.Background(), "tsdb-writer",
					"TSDB buffer overflow", msg, alert.SeverityMedium)
			})
		}
	}

	// Start periodic conversation memory consolidation (every 5 minutes)
	srv.StartPeriodicConsolidation("mdemg-dev", 5*time.Minute)

	// Start scheduled sync if configured (Phase 9.2)
	if cfg.SyncIntervalMinutes > 0 {
		srv.StartScheduledSync(time.Duration(cfg.SyncIntervalMinutes) * time.Minute)
	}

	// Start RSIC decay watchdog (Phase 60b)
	srv.StartRSICWatchdog()

	// Start RSIC macro cron scheduler (Phase 87)
	srv.StartMacroCronScheduler()

	// Start automatic space prune scheduler
	if cfg.SpacePruneIntervalHours > 0 {
		srv.StartSpacePruneScheduler(time.Duration(cfg.SpacePruneIntervalHours) * time.Hour)
	}

	h := &http.Server{
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       time.Duration(cfg.HTTPReadTimeout) * time.Second,
		WriteTimeout:      time.Duration(cfg.HTTPWriteTimeout) * time.Second,
	}

	// Configure TLS if enabled
	if cfg.TLSEnabled {
		tlsCfg := &tls.Config{
			MinVersion:               tls.VersionTLS12,
			CurvePreferences:         []tls.CurveID{tls.CurveP256, tls.X25519},
			PreferServerCipherSuites: true,
			CipherSuites: []uint16{
				tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
				tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
				tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
				tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			},
		}
		h.TLSConfig = tlsCfg
		slog.Info("TLS enabled", "cert", cfg.TLSCertFile, "key", cfg.TLSKeyFile)
	}

	// Dynamic port allocation: try preferred port, then scan range
	listener, err := listenWithFallback(cfg)
	if err != nil {
		return fmt.Errorf("failed to bind: %w", err)
	}

	// Extract actual port and write port file for client discovery
	_, portStr, _ := net.SplitHostPort(listener.Addr().String())
	portFile, _ := filepath.Abs(cfg.PortFilePath)
	if err := writePortFile(portFile, portStr); err != nil {
		slog.Warn("failed to write port file", "path", portFile, "error", err)
	} else {
		slog.Info("port file written", "path", portFile)
	}

	// Start MCP subprocess if requested
	var mcpCmd *exec.Cmd
	if mcpEnabled {
		mcpBin, err := os.Executable()
		if err != nil {
			mcpBin = "mdemg"
		}
		mcpCmd = exec.Command(mcpBin, "mcp")
		mcpCmd.Env = append(os.Environ(), fmt.Sprintf("MDEMG_ENDPOINT=http://localhost:%s", portStr))
		mcpCmd.Stdin = os.Stdin
		mcpCmd.Stdout = os.Stdout
		mcpCmd.Stderr = os.Stderr
		if err := mcpCmd.Start(); err != nil {
			slog.Warn("failed to start MCP subprocess", "error", err)
			mcpCmd = nil
		} else {
			slog.Info("MCP server started", "pid", mcpCmd.Process.Pid, "endpoint", "http://localhost:"+portStr)
		}
	}

	// Set up graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		var serveErr error
		if cfg.TLSEnabled {
			slog.Info("MDEMG server started", "addr", "https://localhost:"+portStr)
			serveErr = h.ServeTLS(listener, cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			slog.Info("MDEMG server started", "addr", "http://localhost:"+portStr)
			serveErr = h.Serve(listener)
		}
		if serveErr != nil && serveErr != http.ErrServerClosed {
			slog.Error("server error", "error", serveErr)
		}
	}()

	// Wait for shutdown signal
	<-shutdown
	slog.Info("shutdown signal received, starting graceful shutdown")

	// Use configurable graceful shutdown timeout
	shutdownTimeout := time.Duration(cfg.GracefulShutdownTimeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Create a channel to track shutdown completion
	shutdownComplete := make(chan struct{})

	go func() {
		defer close(shutdownComplete)

		// Step 1: Stop accepting new connections and drain in-flight requests
		slog.Info("draining in-flight requests", "timeout_sec", cfg.GracefulShutdownTimeoutSec)
		if err := h.Shutdown(ctx); err != nil {
			slog.Error("error shutting down HTTP server", "error", err)
		}

		// Step 2: Stop background services
		slog.Info("stopping background services")
		srv.Shutdown()

		// Step 3: Stop plugin manager
		if pluginMgr != nil {
			slog.Info("stopping plugin manager")
			if err := pluginMgr.Stop(); err != nil {
				slog.Error("error stopping plugin manager", "error", err)
			}
		}

		// Step 4: Stop MCP subprocess
		if mcpCmd != nil && mcpCmd.Process != nil {
			slog.Info("stopping MCP subprocess")
			_ = mcpCmd.Process.Signal(syscall.SIGTERM)
			done := make(chan error, 1)
			go func() { done <- mcpCmd.Wait() }()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				_ = mcpCmd.Process.Kill()
			}
		}

		// Step 5: Remove port file
		os.Remove(portFile)
	}()

	// Wait for shutdown to complete or timeout
	select {
	case <-shutdownComplete:
		slog.Info("graceful shutdown complete")
	case <-ctx.Done():
		slog.Warn("shutdown timeout exceeded, forcing exit")
		os.Exit(1)
	}

	return nil
}

// listenWithFallback tries the preferred address first, then scans the port range.
func listenWithFallback(cfg config.Config) (net.Listener, error) {
	// Try preferred address first
	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err == nil {
		return ln, nil
	}

	// Only fallback if the error is "address already in use"
	var opErr *net.OpError
	if !errors.As(err, &opErr) {
		return nil, fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
	}
	if !errors.Is(opErr.Err, syscall.EADDRINUSE) {
		return nil, fmt.Errorf("listen on %s: %w", cfg.ListenAddr, err)
	}

	slog.Warn("preferred address in use, scanning range",
		"addr", cfg.ListenAddr, "range_start", cfg.PortRangeStart, "range_end", cfg.PortRangeEnd)

	for port := cfg.PortRangeStart; port <= cfg.PortRangeEnd; port++ {
		addr := fmt.Sprintf(":%d", port)
		ln, err = net.Listen("tcp", addr)
		if err == nil {
			return ln, nil
		}
	}

	return nil, fmt.Errorf("no available port in range %d-%d", cfg.PortRangeStart, cfg.PortRangeEnd)
}

// writePortFile writes the port number atomically (write tmp, then rename).
func writePortFile(path, port string) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(port+"\n"), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
