package tsdb

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Client wraps a pgx connection pool for TimescaleDB operations.
type Client struct {
	pool *pgxpool.Pool
	cfg  Config
}

// NewClient creates a new TimescaleDB client with a connection pool.
func NewClient(ctx context.Context, cfg Config) (*Client, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("tsdb: parse config: %w", err)
	}
	if cfg.MaxConns > 0 {
		poolCfg.MaxConns = cfg.MaxConns
	} else {
		poolCfg.MaxConns = 10
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("tsdb: connect: %w", err)
	}

	// Verify connection
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("tsdb: ping: %w", err)
	}

	return &Client{pool: pool, cfg: cfg}, nil
}

// WriteBatch inserts multiple metric samples in a single transaction using COPY.
func (c *Client) WriteBatch(ctx context.Context, samples []MetricSample) error {
	if len(samples) == 0 {
		return nil
	}

	rows := make([][]any, len(samples))
	for i, s := range samples {
		metricType := s.MetricType
		if metricType == "" {
			metricType = "gauge"
		}
		rows[i] = []any{s.Time, s.SpaceID, s.MetricName, s.Value, s.Source, s.QualityTag, s.Labels, metricType}
	}

	_, err := c.pool.CopyFrom(ctx,
		pgx.Identifier{"metric_samples"},
		[]string{"time", "space_id", "metric_name", "value", "source", "quality_tag", "labels", "metric_type"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("tsdb: copy: %w", err)
	}
	return nil
}

// Query retrieves metric points for a time range.
// Granularity auto-selects: raw (<24h), hourly (1-30d), daily (>30d).
func (c *Client) Query(ctx context.Context, q MetricQuery) ([]MetricPoint, error) {
	granularity := q.Granularity
	if granularity == "" {
		dur := q.To.Sub(q.From)
		switch {
		case dur <= 24*time.Hour:
			granularity = "raw"
		case dur <= 30*24*time.Hour:
			granularity = "hourly"
		default:
			granularity = "daily"
		}
	}

	var query string
	switch granularity {
	case "raw":
		query = `SELECT time, value, 0::float8 AS min_value, 0::float8 AS max_value, 1::bigint AS count, quality_tag
			FROM metric_samples
			WHERE metric_name = $1 AND space_id = $2 AND time >= $3 AND time <= $4
			ORDER BY time`
	case "hourly":
		query = `SELECT bucket AS time, avg_value AS value, min_value, max_value, sample_count AS count, '' AS quality_tag
			FROM metrics_hourly
			WHERE metric_name = $1 AND space_id = $2 AND bucket >= $3 AND bucket <= $4
			ORDER BY bucket`
	case "daily":
		query = `SELECT bucket AS time, avg_value AS value, min_value, max_value, sample_count AS count, '' AS quality_tag
			FROM metrics_daily
			WHERE metric_name = $1 AND space_id = $2 AND bucket >= $3 AND bucket <= $4
			ORDER BY bucket`
	default:
		return nil, fmt.Errorf("tsdb: unknown granularity %q", granularity)
	}

	rows, err := c.pool.Query(ctx, query, q.MetricName, q.SpaceID, q.From, q.To)
	if err != nil {
		return nil, fmt.Errorf("tsdb: query: %w", err)
	}
	defer rows.Close()

	var points []MetricPoint
	for rows.Next() {
		var p MetricPoint
		if err := rows.Scan(&p.Time, &p.Value, &p.MinValue, &p.MaxValue, &p.Count, &p.QualityTag); err != nil {
			return nil, fmt.Errorf("tsdb: scan: %w", err)
		}
		points = append(points, p)
	}
	return points, rows.Err()
}

// GetSchemaVersion returns the current TimescaleDB schema version.
func (c *Client) GetSchemaVersion(ctx context.Context) (int, error) {
	var version int
	err := c.pool.QueryRow(ctx,
		"SELECT CAST(value AS INTEGER) FROM tsdb_schema_meta WHERE key = 'schema_version'",
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("tsdb schema version: %w", err)
	}
	return version, nil
}

// Ping checks the database connection.
func (c *Client) Ping(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

// Close closes the connection pool.
func (c *Client) Close() {
	if c.pool != nil {
		c.pool.Close()
	}
}

// Pool returns the underlying connection pool for use by specialized writers.
func (c *Client) Pool() *pgxpool.Pool {
	return c.pool
}
