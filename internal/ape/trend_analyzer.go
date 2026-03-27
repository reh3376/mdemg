package ape

import (
	"context"
	"fmt"
	"math"
	"time"

	"mdemg/internal/tsdb"
)

// TrendAnalyzer provides historical metric trend queries and regression detection
// backed by TimescaleDB continuous aggregates.
type TrendAnalyzer struct {
	tsdbClient *tsdb.Client
}

// NewTrendAnalyzer creates a TrendAnalyzer wired to the given TSDB client.
func NewTrendAnalyzer(client *tsdb.Client) *TrendAnalyzer {
	return &TrendAnalyzer{tsdbClient: client}
}

// RegressionAlert describes a detected regression in a metric's trend.
type RegressionAlert struct {
	MetricName     string  `json:"metric_name"`
	BaselineMean   float64 `json:"baseline_mean"`
	RecentMean     float64 `json:"recent_mean"`
	DeltaPercent   float64 `json:"delta_percent"`
	WindowDays     int     `json:"window_days"`
	IsRegression   bool    `json:"is_regression"`
	Severity       string  `json:"severity"` // "low", "medium", "high"
}

// Query retrieves metric points for a time range, delegating to the TSDB client.
func (t *TrendAnalyzer) Query(ctx context.Context, q tsdb.MetricQuery) ([]tsdb.MetricPoint, error) {
	if t.tsdbClient == nil {
		return nil, fmt.Errorf("trend_analyzer: TSDB client not available")
	}
	return t.tsdbClient.Query(ctx, q)
}

// DetectRegression compares the recent window mean to a historical baseline.
// It queries daily aggregates for the baseline (2x windowDays ago → windowDays ago)
// and compares to the recent window (windowDays ago → now).
// A regression is flagged when the recent mean drops by more than 5% from baseline.
func (t *TrendAnalyzer) DetectRegression(ctx context.Context, spaceID, metric string, windowDays int) (*RegressionAlert, error) {
	if t.tsdbClient == nil {
		return nil, fmt.Errorf("trend_analyzer: TSDB client not available")
	}

	now := time.Now()
	recentStart := now.AddDate(0, 0, -windowDays)
	baselineStart := now.AddDate(0, 0, -2*windowDays)

	// Query baseline window
	baselinePoints, err := t.tsdbClient.Query(ctx, tsdb.MetricQuery{
		SpaceID:     spaceID,
		MetricName:  metric,
		From:        baselineStart,
		To:          recentStart,
		Granularity: "daily",
	})
	if err != nil {
		return nil, fmt.Errorf("trend_analyzer: baseline query: %w", err)
	}

	// Query recent window
	recentPoints, err := t.tsdbClient.Query(ctx, tsdb.MetricQuery{
		SpaceID:     spaceID,
		MetricName:  metric,
		From:        recentStart,
		To:          now,
		Granularity: "daily",
	})
	if err != nil {
		return nil, fmt.Errorf("trend_analyzer: recent query: %w", err)
	}

	baselineMean := meanOfPoints(baselinePoints)
	recentMean := meanOfPoints(recentPoints)

	alert := &RegressionAlert{
		MetricName:   metric,
		BaselineMean: baselineMean,
		RecentMean:   recentMean,
		WindowDays:   windowDays,
	}

	if baselineMean > 0 {
		alert.DeltaPercent = ((recentMean - baselineMean) / baselineMean) * 100
	}

	// Flag regression if recent is >5% below baseline
	if baselineMean > 0 && alert.DeltaPercent < -5 {
		alert.IsRegression = true
		switch {
		case alert.DeltaPercent < -20:
			alert.Severity = "high"
		case alert.DeltaPercent < -10:
			alert.Severity = "medium"
		default:
			alert.Severity = "low"
		}
	}

	return alert, nil
}

// meanOfPoints computes the arithmetic mean of a slice of MetricPoints.
func meanOfPoints(points []tsdb.MetricPoint) float64 {
	if len(points) == 0 {
		return 0
	}
	var sum float64
	for _, p := range points {
		sum += p.Value
	}
	mean := sum / float64(len(points))
	// Round to 6 decimal places to avoid floating point noise
	return math.Round(mean*1e6) / 1e6
}
