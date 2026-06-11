package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	nconfig "github.com/neo4j/neo4j-go-driver/v5/neo4j/config"
	"mdemg/internal/config"
)

// NOTE (TSDB-CONSUME-001): the former ConnectionPoolMetrics /
// StartPoolMetricsCollector machinery was deleted. The neo4j Go driver
// exposes no pool-stats API, so the "pool" numbers were a VerifyConnectivity
// probe loop in disguise: TotalAcquired counted probe successes,
// Active/Idle/Waiting were perpetual zeros. Liveness probing is the health
// prober's job (internal/health); real pool gauges now come from the TSDB
// pgx pool (metrics.CollectTSDBPoolMetrics).

func NewDriver(cfg config.Config) (neo4j.DriverWithContext, error) {
	// Configure connection pool options
	configurator := func(conf *nconfig.Config) {
		// Connection pool size (default: 100)
		if cfg.Neo4jMaxPoolSize > 0 {
			conf.MaxConnectionPoolSize = cfg.Neo4jMaxPoolSize
		} else {
			conf.MaxConnectionPoolSize = 100
		}

		// Connection acquisition timeout (default: 60s)
		if cfg.Neo4jAcquireTimeoutSec > 0 {
			conf.ConnectionAcquisitionTimeout = time.Duration(cfg.Neo4jAcquireTimeoutSec) * time.Second
		} else {
			conf.ConnectionAcquisitionTimeout = 60 * time.Second
		}

		// Maximum connection lifetime (default: 1 hour)
		if cfg.Neo4jMaxConnLifetimeSec > 0 {
			conf.MaxConnectionLifetime = time.Duration(cfg.Neo4jMaxConnLifetimeSec) * time.Second
		} else {
			conf.MaxConnectionLifetime = 1 * time.Hour
		}

		// Connection idle timeout (default: disabled)
		if cfg.Neo4jConnIdleTimeoutSec > 0 {
			conf.ConnectionLivenessCheckTimeout = time.Duration(cfg.Neo4jConnIdleTimeoutSec) * time.Second
		}

		// Socket connect timeout (default: 5s)
		conf.SocketConnectTimeout = 5 * time.Second

		// Socket keep alive (default: true)
		conf.SocketKeepalive = true
	}

	driver, err := neo4j.NewDriverWithContext(
		cfg.Neo4jURI,
		neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""),
		configurator,
	)
	if err != nil {
		return nil, err
	}

	// Log pool configuration
	slog.Info("Neo4j connection pool configured", "max_pool_size", cfg.Neo4jMaxPoolSize, "acquire_timeout_sec", cfg.Neo4jAcquireTimeoutSec)

	return driver, nil
}

func VerifyConnectivity(ctx context.Context, driver neo4j.DriverWithContext) error {
	return driver.VerifyConnectivity(ctx)
}

func AssertSchemaVersion(ctx context.Context, driver neo4j.DriverWithContext, required int) error {
	sess := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer sess.Close(ctx)

	verAny, err := sess.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		res, err := tx.Run(ctx, "MATCH (s:SchemaMeta {key:'schema'}) RETURN coalesce(s.current_version,0) AS v", nil)
		if err != nil {
			return nil, err
		}
		if !res.Next(ctx) {
			return 0, fmt.Errorf("SchemaMeta missing: run migrations")
		}
		v, _ := res.Record().Get("v")
		return v, nil
	})
	if err != nil {
		return err
	}

	ver, ok := verAny.(int64)
	if !ok {
		// Neo4j returns integer as int64
		return fmt.Errorf("unexpected schema version type: %T", verAny)
	}
	if int(ver) < required {
		return fmt.Errorf("schema version %d < required %d — run 'mdemg db migrate' to upgrade", ver, required)
	}
	return nil
}
