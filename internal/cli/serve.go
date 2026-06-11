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
	"sync"
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
	"mdemg/internal/metrics"
	"mdemg/internal/mlxprobe"
	"mdemg/internal/plugins"
	"mdemg/internal/supervisor"
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

	// Hotfix 11.6.3.1 — always-on MLX policy. The framework's 16 LLM call
	// sites all depend on mlx; refusing to start when mlx is unreachable
	// is the only honest signal. Operator escape hatch: MDEMG_ALLOW_NO_MLX=1
	// (intended for Linux/Docker-only setups where the operator has wired
	// LLM_ENDPOINT to a non-mlx provider).
	if err := preflightMLXReachable(cfg); err != nil {
		return err
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

			// Backfill constraint_outcomes from Neo4j GUIDANCE_OUTCOME edges (idempotent).
			// Catches up data created before the TSDB outcome writer was wired.
			if cfg.JiminyEnabled {
				spaceID := cfg.RSICWatchdogSpaceID
				if spaceID == "" {
					spaceID = "mdemg-dev"
				}
				if backfillErr := tsdb.BackfillConstraintOutcomes(context.Background(), tsdbClient.Pool(), driver, spaceID); backfillErr != nil {
					slog.Warn("tsdb: constraint_outcomes backfill failed", "error", backfillErr)
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
		Enabled:         cfg.LLMRetryEnabled,
		MaxAttempts:     cfg.LLMRetryMaxAttempts,
		BaseDelayMs:     cfg.LLMRetryBaseDelayMs,
		MaxDelayMs:      cfg.LLMRetryMaxDelayMs,
		Multiplier:      cfg.LLMRetryMultiplier,
		Jitter:          cfg.LLMRetryJitter,
		RetryOnDeadline: cfg.LLMRetryOnDeadline,
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
		slog.Info("health prober created", "interval_sec", cfg.HealthProbeIntervalSec)
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

	// Phase 11.6.3 — MLX Watchdog. Construct + register the prober BEFORE
	// supervisor.Start so the goroutine joins the same panic-recovery loop
	// the health prober uses. SetDefault wires the singleton llmclient reads
	// at request time. Embeddings are unaffected because the gate keys on
	// baseURL == cfg.EffectiveLLMEndpoint().
	var mlxProber *mlxprobe.Prober
	var burstFlushRun func(ctx context.Context) error // SUPERVISOR-002: launched under the supervisor below
	if cfg.MLXWatchdogEnabled {
		mlxProber = mlxprobe.New(mlxprobe.Config{
			Endpoint: cfg.EffectiveLLMEndpoint(),
			Interval: time.Duration(cfg.MLXProbeIntervalSec) * time.Second,
			Timeout:  time.Duration(cfg.MLXProbeTimeoutSec) * time.Second,
		})
		mlxprobe.SetDefault(mlxProber)
		mlxprobe.SetFastFailEnabled(cfg.MLXFailFastEnabled)

		// State transition callback: metrics on every transition, alerts on
		// up→down (High) and down→up (Low). The alert dispatcher is resolved
		// lazily via srv.AlertDispatcher() so this closure works even though
		// the supervisor may start the goroutine before disp is wired. Phase
		// 13.5: also write each transition to the V0018 llm_endpoint_health
		// hypertable so Grafana panels can render historical stability over
		// time ranges that survive process restarts.
		stdMetrics := metrics.Metrics()
		mlxProber.OnTransition(func(from, to mlxprobe.State, lastErr error) {
			stdMetrics.MLXHealthState(cfg.EffectiveLLMEndpoint()).Set(float64(to))
			stdMetrics.MLXStateTransitions(from.String(), to.String()).Inc()

			// Persist to TSDB (V0018) — silent no-op if writer isn't ready.
			if w := srv.LLMEndpointHealthWriter(); w != nil {
				kind := "state_transition"
				if from == mlxprobe.StateDown && to == mlxprobe.StateUp {
					kind = "probe_recovery"
				}
				errMsg := ""
				if lastErr != nil {
					errMsg = lastErr.Error()
				}
				w.Record(tsdb.LLMEndpointHealthEvent{
					EndpointURL:  cfg.EffectiveLLMEndpoint(),
					EventKind:    kind,
					FromState:    from.String(),
					ToState:      to.String(),
					ErrorMessage: errMsg,
					StateNumeric: int(to),
				})
			}

			disp := srv.AlertDispatcher()
			if disp == nil {
				return
			}
			switch {
			case to == mlxprobe.StateDown:
				msg := fmt.Sprintf("endpoint=%s last_error=%v",
					cfg.EffectiveLLMEndpoint(), lastErr)
				disp.SendAlert(context.Background(), "mlx-server",
					"mlx unreachable — fast-fail engaged", msg, alert.SeverityHigh)
			case from == mlxprobe.StateDown && to == mlxprobe.StateUp:
				disp.SendAlert(context.Background(), "mlx-server",
					"mlx recovered",
					fmt.Sprintf("endpoint=%s back to StateUp", cfg.EffectiveLLMEndpoint()),
					alert.SeverityLow)
			}
		})
		// Initial state read so the gauge is populated before any tick.
		stdMetrics.MLXHealthState(cfg.EffectiveLLMEndpoint()).Set(float64(mlxProber.State()))

		// Wire fast-fail observer so the llmclient gate increments
		// mdemg_mlx_fast_fail_total without llmclient importing metrics.
		// Phase 13.5: also aggregate bursts into V0018 health rows.
		// Rate-limit so a 9-min outage produces O(window-count) rows, not
		// O(short-circuit-count). Window default 30s; 1 row per non-zero
		// window flushed by the goroutine below.
		var fastFailBurstMu sync.Mutex
		var fastFailBurstCount int
		mlxprobe.SetFastFailObserver(func(callerTask, _ string) {
			task := callerTask
			if task == "" {
				task = "unknown"
			}
			stdMetrics.MLXFastFailTotal(task).Inc()
			fastFailBurstMu.Lock()
			fastFailBurstCount++
			fastFailBurstMu.Unlock()
		})
		// Burst-flush loop (Phase 13.5). Persists at most 1 row per
		// 30s window when the gate is engaged, with the count of
		// short-circuits observed in the window. Silent in steady-state.
		// Lives for process lifetime; on shutdown the LLMEndpointHealthWriter
		// gets Closed() in api.Server.Shutdown which flushes any pending row.
		// SUPERVISOR-002: defined here, launched below once the supervisor
		// exists (sup.Go) so it gets panic recovery + restart budget.
		burstFlushRun = func(ctx context.Context) error {
			tick := time.NewTicker(30 * time.Second)
			defer tick.Stop()
			for {
				select {
				case <-ctx.Done():
					return nil
				case <-tick.C:
					fastFailBurstMu.Lock()
					n := fastFailBurstCount
					fastFailBurstCount = 0
					fastFailBurstMu.Unlock()
					if n == 0 {
						continue
					}
					if w := srv.LLMEndpointHealthWriter(); w != nil {
						w.Record(tsdb.LLMEndpointHealthEvent{
							EndpointURL:        cfg.EffectiveLLMEndpoint(),
							EventKind:          "fast_fail_burst",
							FromState:          mlxProber.State().String(),
							ToState:            mlxProber.State().String(),
							BurstShortCircuits: n,
							StateNumeric:       int(mlxProber.State()),
						})
					}
				}
			}
		}

		slog.Info("mlx-watchdog: enabled",
			"endpoint", cfg.EffectiveLLMEndpoint(),
			"interval_sec", cfg.MLXProbeIntervalSec,
			"timeout_sec", cfg.MLXProbeTimeoutSec,
			"fast_fail", cfg.MLXFailFastEnabled,
		)
	}

	// SNA-001: Goroutine supervisor with panic recovery and auto-restart
	var sup *supervisor.Supervisor
	if disp := srv.AlertDispatcher(); disp != nil {
		sup = supervisor.New(func(service, title, message, severity string) {
			sev := alert.Severity(severity)
			disp.SendAlert(context.Background(), service, title, message, sev)
		})
		// SUPERVISOR-002: sliding-window restart budget from config
		sup.Configure(cfg.SupervisorMaxRestarts,
			time.Duration(cfg.SupervisorRestartWindowMin)*time.Minute,
			time.Duration(cfg.SupervisorBackoffBaseSec)*time.Second)

		// Register health prober
		if prober != nil {
			sup.Register("health-prober", func(ctx context.Context) error {
				prober.Start()
				<-ctx.Done()
				prober.Stop()
				return nil
			})
		}

		// Phase 11.6.3 — register mlx watchdog under supervisor for panic
		// recovery + ctx cancellation. Probe.Run() blocks until ctx.Done().
		if mlxProber != nil {
			sup.Register("mlx-watchdog", mlxProber.Run)
		}

		// Register alert evaluator
		if tsdbClient != nil && cfg.AlertEvaluatorEnabled {
			evalInterval := time.Duration(cfg.AlertEvaluatorIntervalSec) * time.Second
			rules := alert.DefaultRules()
			// NOSILENT-001: append scheduled-job staleness/failure rules so the
			// server catches "a job failed OR never ran". Backup-staleness
			// window derives from the actual backup interval × 2 unless
			// explicitly overridden; gated on backups actually being enabled.
			if cfg.JobHealthAlertEnabled {
				staleness := cfg.JobBackupStalenessHours
				if staleness <= 0 {
					staleness = cfg.TSDBBackupIntervalHours * 2
				}
				rules = append(rules, alert.JobHealthRules(
					staleness, cfg.JobFailureLookbackMin, cfg.TSDBBackupEnabled)...)
				// BACKUP-RESTORE-VERIFY-001: same guarantee for the Neo4j
				// backup scheduler (window = partial interval × 2 unless
				// BACKUP_JOB_STALENESS_HOURS overrides).
				if cfg.BackupEnabled {
					neoStaleness := cfg.BackupJobStalenessHours
					if neoStaleness <= 0 {
						neoStaleness = cfg.BackupPartialIntervalHours * 2
					}
					rules = append(rules, alert.Neo4jBackupStalenessRule(neoStaleness))
				}
			}
			// HOOKSYNC-001: hook-channel absence detection — sessions active
			// (post-tool-observe heartbeats) but zero prompt-context fires.
			if cfg.HookHealthAlertEnabled {
				rules = append(rules, alert.HookHealthRules(
					cfg.HookSilentLookbackHours, cfg.HookActivityMinEvents)...)
			}
			// HIDDEN-WEIGHT-001: NULL-weight abstraction edges reappearing.
			rules = append(rules, alert.WeightIntegrityRules(
				cfg.NullWeightEdgeAlertThreshold)...)
			// HIDDEN-CHURN-001 PR-B: conversation coverage below floor.
			rules = append(rules, alert.CoverageRules(
				cfg.ConversationCoverageAlertFloor)...)
			// MAINT-LIVE-001: maintenance recorded but never running live.
			if cfg.MaintLiveAlertEnabled {
				rules = append(rules, alert.MaintenanceLivenessRules(
					cfg.MaintLiveLookbackDays)...)
			}
			// TSDB-CONSUME-001: retrieve-latency SLO over retrieval_audit
			// real wall-time (replaces the lifetime-cumulative HTTP
			// synthetic rules removed from DefaultRules).
			rules = append(rules, alert.RetrieveLatencyRules(
				float64(cfg.AlertRetrieveP95Ms), float64(cfg.AlertRetrieveP99Ms),
				cfg.AlertRetrieveLatencyLookbackMin)...)
			// TSDB-CONSUME-001: buffered-writer flush failures (a wedged
			// writer used to drop rows in silence).
			rules = append(rules, alert.TSDBWriterRules(
				cfg.TSDBWriterAlertLookbackMin)...)
			// TSDB-CONSUME-001: scorer-drift tripwires over retrieval_audit
			// (the RRF-SCALE-001 regression class becomes self-detecting).
			rules = append(rules, alert.ScorerDriftRules(
				cfg.ScorerChangeLookbackHours, cfg.ConsensusShiftThreshold,
				cfg.ConsensusShiftRecentHours, cfg.ConsensusShiftBaselineDays,
				cfg.ConsensusShiftMinSamples)...)
			evaluator := alert.NewEvaluator(rules, tsdbClient.Pool(), disp, evalInterval)
			// SUPERVISOR-002: meta-alert when a rule's query fails repeatedly
			evaluator.SetRuleFailureThreshold(cfg.AlertRuleFailureThreshold)
			sup.Register("alert-evaluator", func(_ context.Context) error {
				evaluator.Start() // blocks until evaluator.Stop()
				return nil
			})
		}

		go sup.Start(context.Background())
		defer sup.Stop()

		// SUPERVISOR-002: route all server background loops through the
		// supervisor (panic recovery + sliding-window restart budget).
		srv.SetSupervisor(sup.Go)
		if burstFlushRun != nil {
			sup.Go("llm-fastfail-burst-flush", burstFlushRun)
		}
	} else {
		// Fallback: start prober directly if no supervisor
		if prober != nil {
			prober.Start()
			defer prober.Stop()
		}
		if burstFlushRun != nil {
			go func() {
				_ = burstFlushRun(context.Background())
			}()
		}
	}

	// SUPERVISOR-002: start the construction-deferred loops (backup
	// schedulers, RSIC store flush, signal-learner persistence) — supervised
	// when a supervisor was injected above, bare goroutines otherwise.
	srv.StartSupervisedBackground()

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

		// G4+G11: Wire LLM consecutive failure alert callback.
		// Late binding: callback reads disp at call time, works even though
		// LLM clients were created inside NewServer before this point.
		llmclient.SetDefaultFailureThreshold(cfg.LLMConsecutiveFailureThreshold)
		llmclient.SetDefaultAlertCallback(func(taskName string, count int, lastErr error) {
			disp.SendAlert(context.Background(), "llm-"+taskName,
				fmt.Sprintf("LLM consecutive failures: %s", taskName),
				fmt.Sprintf("%d consecutive failures, last: %v", count, lastErr),
				alert.SeverityHigh)
		})

		// Phase 11.6.3 — mlx watchdog OnTransition was wired earlier (with
		// late-bound disp lookup) so it operates regardless of the order
		// supervisor.Start fires relative to alert dispatcher availability.
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

	// Start context cooler background processing (opt-in)
	if cfg.ContextCoolerEnabled {
		srv.StartContextCoolerProcessing("mdemg-dev", 10*time.Minute)
	}

	// Start weekly gap interviews (opt-in)
	if cfg.WeeklyGapInterviewsEnabled {
		srv.StartWeeklyGapInterviews(7 * 24 * time.Hour)
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
