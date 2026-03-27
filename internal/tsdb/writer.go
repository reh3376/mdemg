package tsdb

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// MetricWriter buffers metric samples and flushes them to TimescaleDB at configured intervals.
type MetricWriter struct {
	client    *Client
	spaceID   string
	buffer    []MetricSample
	mu        sync.Mutex
	flushTick *time.Ticker
	done      chan struct{}
}

// NewMetricWriter creates a writer that auto-flushes at the given interval.
func NewMetricWriter(client *Client, spaceID string, flushInterval time.Duration) *MetricWriter {
	w := &MetricWriter{
		client:  client,
		spaceID: spaceID,
		buffer:  make([]MetricSample, 0, 64),
		done:    make(chan struct{}),
	}

	if flushInterval <= 0 {
		flushInterval = 60 * time.Second
	}
	w.flushTick = time.NewTicker(flushInterval)

	go w.flushLoop()
	return w
}

func (w *MetricWriter) flushLoop() {
	for {
		select {
		case <-w.flushTick.C:
			if err := w.Flush(context.Background()); err != nil {
				slog.Warn("tsdb: auto-flush failed", "error", err)
			}
		case <-w.done:
			return
		}
	}
}

// Record buffers metric samples for the next flush.
func (w *MetricWriter) Record(samples ...MetricSample) {
	if len(samples) == 0 {
		return
	}
	w.mu.Lock()
	w.buffer = append(w.buffer, samples...)
	w.mu.Unlock()
}

// Flush writes all buffered samples to TimescaleDB and clears the buffer.
func (w *MetricWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	if len(w.buffer) == 0 {
		w.mu.Unlock()
		return nil
	}
	batch := w.buffer
	w.buffer = make([]MetricSample, 0, 64)
	w.mu.Unlock()

	return w.client.WriteBatch(ctx, batch)
}

// RecordGaugeSnapshot converts a map of gauge name->value into MetricSamples and buffers them.
func (w *MetricWriter) RecordGaugeSnapshot(gauges map[string]float64, source, qualityTag string) {
	now := time.Now()
	samples := make([]MetricSample, 0, len(gauges))
	for name, val := range gauges {
		samples = append(samples, MetricSample{
			Time:       now,
			SpaceID:    w.spaceID,
			MetricName: name,
			Value:      val,
			Source:     source,
			QualityTag: qualityTag,
		})
	}
	w.Record(samples...)
}

// Close stops the flush loop and performs a final flush.
func (w *MetricWriter) Close() {
	w.flushTick.Stop()
	close(w.done)
	// Final flush
	if err := w.Flush(context.Background()); err != nil {
		slog.Warn("tsdb: final flush failed", "error", err)
	}
}
