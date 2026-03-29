package tsdb

import (
	"strconv"
	"time"
)

// MetricSample represents a single metric data point.
type MetricSample struct {
	Time       time.Time
	SpaceID    string
	MetricName string
	Value      float64
	Source     string            // "assess", "meso", "macro", "live"
	QualityTag string            // "nominal", "cached_health", "degraded_nli", "partial", "bootstrap"
	MetricType string            // "gauge", "counter", "histogram_bucket", "histogram_sum", "histogram_count"
	Labels     map[string]string // optional additional labels
}

// MetricQuery describes a time-range query for metric data.
type MetricQuery struct {
	SpaceID     string
	MetricName  string // exact name or LIKE pattern
	From        time.Time
	To          time.Time
	Granularity string // "raw", "hourly", "daily" — auto-selected if empty
}

// MetricPoint is a single point returned from a query.
type MetricPoint struct {
	Time       time.Time `json:"time"`
	Value      float64   `json:"value"`
	MinValue   float64   `json:"min_value,omitempty"`
	MaxValue   float64   `json:"max_value,omitempty"`
	Count      int64     `json:"count,omitempty"`
	QualityTag string    `json:"quality_tag,omitempty"`
}

// Config holds TimescaleDB connection configuration.
type Config struct {
	Host                string
	Port                int
	User                string
	Password            string
	Database            string
	SSLMode             string
	MaxConns            int32
	FlushInterval       time.Duration
	RawRetentionDays    int
	HourlyRetentionDays int
}

// DSN returns the PostgreSQL connection string.
func (c Config) DSN() string {
	return "postgres://" + c.User + ":" + c.Password + "@" + c.Host + ":" +
		strconv.Itoa(c.Port) + "/" + c.Database + "?sslmode=" + c.SSLMode
}
