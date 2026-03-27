package ape

import (
	"context"
	"testing"

	"mdemg/internal/tsdb"
)

// ─── meanOfPoints ───

func TestMeanOfPoints_Empty(t *testing.T) {
	got := meanOfPoints(nil)
	if got != 0 {
		t.Errorf("got %f, want 0", got)
	}
}

func TestMeanOfPoints_Single(t *testing.T) {
	points := []tsdb.MetricPoint{{Value: 42.0}}
	got := meanOfPoints(points)
	if got != 42.0 {
		t.Errorf("got %f, want 42.0", got)
	}
}

func TestMeanOfPoints_Multiple(t *testing.T) {
	tests := []struct {
		name   string
		points []tsdb.MetricPoint
		want   float64
	}{
		{
			name: "two values",
			points: []tsdb.MetricPoint{
				{Value: 10.0},
				{Value: 20.0},
			},
			want: 15.0,
		},
		{
			name: "three values",
			points: []tsdb.MetricPoint{
				{Value: 1.0},
				{Value: 2.0},
				{Value: 3.0},
			},
			want: 2.0,
		},
		{
			name: "identical values",
			points: []tsdb.MetricPoint{
				{Value: 5.0},
				{Value: 5.0},
				{Value: 5.0},
			},
			want: 5.0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := meanOfPoints(tt.points)
			if got != tt.want {
				t.Errorf("got %f, want %f", got, tt.want)
			}
		})
	}
}

// ─── TrendAnalyzer.Query ───

func TestTrendAnalyzer_Query_NilClient(t *testing.T) {
	ta := NewTrendAnalyzer(nil)
	_, err := ta.Query(context.Background(), tsdb.MetricQuery{})
	if err == nil {
		t.Errorf("got nil error, want error for nil TSDB client")
	}
}

// ─── TrendAnalyzer.DetectRegression ───

func TestTrendAnalyzer_DetectRegression_NilClient(t *testing.T) {
	ta := NewTrendAnalyzer(nil)
	_, err := ta.DetectRegression(context.Background(), "space", "metric", 7)
	if err == nil {
		t.Errorf("got nil error, want error for nil TSDB client")
	}
}
