package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"mdemg/internal/api"
	"mdemg/internal/config"
	"mdemg/internal/db"
	"mdemg/internal/plugins"
)

func newServeCmd() *cobra.Command {
	var port int
	var dbURI string

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
  - Verify schema version
  - Initialize plugin manager (if enabled)
  - Start periodic background tasks (consolidation, sync, RSIC, pruning)
  - Start HTTP server with graceful shutdown support
  - Write port file for client discovery

See config.FromEnv() for the full list of environment variable options.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd, args, port, dbURI)
		},
	}

	cmd.Flags().IntVar(&port, "port", 0, "Listen port (overrides LISTEN_ADDR env var)")
	cmd.Flags().StringVar(&dbURI, "db-uri", "", "Neo4j URI (overrides NEO4J_URI env var)")

	return cmd
}

func runServe(cmd *cobra.Command, args []string, port int, dbURI string) error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("config error: %w", err)
	}

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

	// Readiness check: schema version
	if err := db.AssertSchemaVersion(context.Background(), driver, cfg.RequiredSchemaVersion); err != nil {
		return fmt.Errorf("schema version check failed: %w", err)
	}

	// Initialize Plugin Manager if enabled
	var pluginMgr *plugins.Manager
	if cfg.PluginsEnabled {
		pluginMgr = plugins.NewManager(cfg.PluginsDir, cfg.PluginSocketDir, cfg.MdemgVersion)
		if err := pluginMgr.Start(); err != nil {
			log.Printf("warning: failed to start plugin manager: %v", err)
			// Continue without plugins - this is not fatal
		} else {
			log.Printf("plugin manager started (dir=%s)", cfg.PluginsDir)
		}
	}

	srv := api.NewServer(cfg, driver, pluginMgr)

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
		log.Printf("TLS enabled (cert: %s, key: %s)", cfg.TLSCertFile, cfg.TLSKeyFile)
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
		log.Printf("warning: failed to write port file %s: %v", portFile, err)
	} else {
		log.Printf("port file written: %s", portFile)
	}

	// Set up graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		var serveErr error
		if cfg.TLSEnabled {
			log.Printf("MDEMG server started on https://localhost:%s", portStr)
			serveErr = h.ServeTLS(listener, cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			log.Printf("MDEMG server started on http://localhost:%s", portStr)
			serveErr = h.Serve(listener)
		}
		if serveErr != nil && serveErr != http.ErrServerClosed {
			log.Printf("server error: %v", serveErr)
		}
	}()

	// Wait for shutdown signal
	<-shutdown
	log.Println("shutdown signal received, starting graceful shutdown...")

	// Use configurable graceful shutdown timeout
	shutdownTimeout := time.Duration(cfg.GracefulShutdownTimeoutSec) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// Create a channel to track shutdown completion
	shutdownComplete := make(chan struct{})

	go func() {
		defer close(shutdownComplete)

		// Step 1: Stop accepting new connections and drain in-flight requests
		log.Printf("draining in-flight requests (timeout: %ds)...", cfg.GracefulShutdownTimeoutSec)
		if err := h.Shutdown(ctx); err != nil {
			log.Printf("error shutting down HTTP server: %v", err)
		}

		// Step 2: Stop background services
		log.Println("stopping background services...")
		srv.Shutdown()

		// Step 3: Stop plugin manager
		if pluginMgr != nil {
			log.Println("stopping plugin manager...")
			if err := pluginMgr.Stop(); err != nil {
				log.Printf("error stopping plugin manager: %v", err)
			}
		}

		// Step 4: Remove port file
		os.Remove(portFile)
	}()

	// Wait for shutdown to complete or timeout
	select {
	case <-shutdownComplete:
		log.Println("graceful shutdown complete")
	case <-ctx.Done():
		log.Println("shutdown timeout exceeded, forcing exit")
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

	log.Printf("preferred address %s in use, scanning range %d-%d",
		cfg.ListenAddr, cfg.PortRangeStart, cfg.PortRangeEnd)

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
