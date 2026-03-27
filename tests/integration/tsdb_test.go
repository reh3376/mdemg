//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"mdemg/internal/tsdb"
)

// skipIfNoTSDB skips the test if no TSDB connection info is available.
func skipIfNoTSDB(t *testing.T) {
	t.Helper()
	if os.Getenv("TEST_TSDB_DSN") == "" &&
		os.Getenv("TEST_TSDB_HOST") == "" {
		t.Skip("TEST_TSDB_DSN / TEST_TSDB_HOST not set, skipping TSDB integration tests")
	}
}

// tsdbTestConfig builds a tsdb.Config from environment variables with sensible defaults.
func tsdbTestConfig(t *testing.T) tsdb.Config {
	t.Helper()
	return tsdb.Config{
		Host:     envOrDefault("TEST_TSDB_HOST", "localhost"),
		Port:     envOrDefaultInt("TEST_TSDB_PORT", 5432),
		User:     envOrDefault("TEST_TSDB_USER", "mdemg"),
		Password: envOrDefault("TEST_TSDB_PASSWORD", "mdemg_metrics"),
		Database: envOrDefault("TEST_TSDB_DATABASE", "mdemg_metrics"),
		SSLMode:  "disable",
	}
}

func envOrDefault(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func envOrDefaultInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

// newTestClient creates a tsdb.Client for integration tests and registers cleanup.
func newTestClient(t *testing.T) *tsdb.Client {
	t.Helper()
	skipIfNoTSDB(t)

	cfg := tsdbTestConfig(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := tsdb.NewClient(ctx, cfg)
	if err != nil {
		t.Fatalf("failed to create TSDB client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestTSDB_ClientConnect(t *testing.T) {
	client := newTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	// Close should not error (called via t.Cleanup, but verify explicit call is safe)
	client.Close()
}

func TestTSDB_Migrate(t *testing.T) {
	client := newTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First run
	if err := client.Migrate(ctx); err != nil {
		t.Fatalf("Migrate (first run) failed: %v", err)
	}

	// Second run — idempotent
	if err := client.Migrate(ctx); err != nil {
		t.Fatalf("Migrate (idempotent run) failed: %v", err)
	}
}

func TestTSDB_GetSchemaVersion(t *testing.T) {
	client := newTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Ensure schema exists
	if err := client.Migrate(ctx); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	version, err := client.GetSchemaVersion(ctx)
	if err != nil {
		t.Fatalf("GetSchemaVersion failed: %v", err)
	}
	if version < 1 {
		t.Errorf("expected schema version >= 1, got %d", version)
	}
}

func TestTSDB_WriteBatch_Query_Roundtrip(t *testing.T) {
	client := newTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Ensure schema exists
	if err := client.Migrate(ctx); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	testSpaceID := fmt.Sprintf("test-tsdb-%d", time.Now().UnixNano())
	metricName := "test_roundtrip_metric"

	// Cleanup after test
	t.Cleanup(func() {
		_, _ = client.Pool().Exec(context.Background(),
			"DELETE FROM metric_samples WHERE space_id = $1", testSpaceID)
	})

	now := time.Now().UTC().Truncate(time.Millisecond)
	samples := []tsdb.MetricSample{
		{Time: now.Add(-2 * time.Minute), SpaceID: testSpaceID, MetricName: metricName, Value: 1.0, Source: "test", QualityTag: "nominal"},
		{Time: now.Add(-1 * time.Minute), SpaceID: testSpaceID, MetricName: metricName, Value: 2.5, Source: "test", QualityTag: "nominal"},
		{Time: now, SpaceID: testSpaceID, MetricName: metricName, Value: 3.7, Source: "test", QualityTag: "nominal"},
	}

	if err := client.WriteBatch(ctx, samples); err != nil {
		t.Fatalf("WriteBatch failed: %v", err)
	}

	// Query back — raw granularity (< 24h range)
	query := tsdb.MetricQuery{
		SpaceID:     testSpaceID,
		MetricName:  metricName,
		From:        now.Add(-5 * time.Minute),
		To:          now.Add(1 * time.Minute),
		Granularity: "raw",
	}

	points, err := client.Query(ctx, query)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if got := len(points); got != 3 {
		t.Fatalf("expected 3 points, got %d", got)
	}

	// Verify values are in order
	expectedValues := []float64{1.0, 2.5, 3.7}
	for i, p := range points {
		if p.Value != expectedValues[i] {
			t.Errorf("point[%d].Value = %f, want %f", i, p.Value, expectedValues[i])
		}
	}
}

func TestTSDB_MetricWriter_Flush(t *testing.T) {
	client := newTestClient(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Ensure schema exists
	if err := client.Migrate(ctx); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	testSpaceID := fmt.Sprintf("test-writer-%d", time.Now().UnixNano())

	// Cleanup after test
	t.Cleanup(func() {
		_, _ = client.Pool().Exec(context.Background(),
			"DELETE FROM metric_samples WHERE space_id = $1", testSpaceID)
	})

	// Use a very long flush interval so auto-flush doesn't interfere; we flush manually.
	writer := tsdb.NewMetricWriter(client, testSpaceID, 1*time.Hour)
	t.Cleanup(func() { writer.Close() })

	gauges := map[string]float64{
		"gauge_a": 10.0,
		"gauge_b": 20.0,
		"gauge_c": 30.0,
		"gauge_d": 40.0,
		"gauge_e": 50.0,
	}

	writer.RecordGaugeSnapshot(gauges, "test", "nominal")

	if err := writer.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}

	// Query each gauge to verify it landed in the DB
	for name, expectedVal := range gauges {
		var count int
		err := client.Pool().QueryRow(ctx,
			"SELECT count(*) FROM metric_samples WHERE space_id = $1 AND metric_name = $2",
			testSpaceID, name,
		).Scan(&count)
		if err != nil {
			t.Fatalf("query count for %s failed: %v", name, err)
		}
		if count < 1 {
			t.Errorf("expected at least 1 row for metric %s, got %d", name, count)
		}

		var val float64
		err = client.Pool().QueryRow(ctx,
			"SELECT value FROM metric_samples WHERE space_id = $1 AND metric_name = $2 LIMIT 1",
			testSpaceID, name,
		).Scan(&val)
		if err != nil {
			t.Fatalf("query value for %s failed: %v", name, err)
		}
		if val != expectedVal {
			t.Errorf("metric %s: got value %f, want %f", name, val, expectedVal)
		}
	}
}
