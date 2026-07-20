package metrics

import "time"

// MetricSnapshot is the JSON-serializable snapshot of all registered metrics.
// Served by the /v1/metrics/snapshot endpoint (wired in A2).
type MetricSnapshot struct {
	Timestamp  time.Time                    `json:"timestamp"`
	Counters   map[string]int64             `json:"counters"`
	Gauges     map[string]float64           `json:"gauges"`
	Histograms map[string]HistogramSnapshot `json:"histograms"`
	Writer     WriterHealthSnapshot         `json:"writer_health"`
}

// HistogramSnapshot holds a point-in-time view of a single histogram.
type HistogramSnapshot struct {
	Count   int64            `json:"count"`
	Sum     float64          `json:"sum"`
	Buckets map[string]int64 `json:"buckets"` // le boundary (string) -> cumulative count
	P95     float64          `json:"p95"`
	P99     float64          `json:"p99"`
}

// WriterHealthSnapshot reports TSDB writer health for the snapshot envelope.
type WriterHealthSnapshot struct {
	LastSuccessfulWrite string `json:"last_successful_write,omitempty"`
	ConsecutiveFailures int    `json:"consecutive_failures"`
	BufferSize          int    `json:"buffer_size"`
}
