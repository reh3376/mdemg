package tsdb

import "sync"

// Writer flush-stats registry (TSDB-CONSUME-001).
//
// Every buffered event writer already tracked flush success/failure/row
// counts via atomics and exposed Stats(), but only the reinforcement writer
// had any metrics visibility — a wedged writer dropped rows silently. Each
// writer registers a stats closure at construction; the metrics pre-flush
// hook reads AllWriterStats() into the mdemg_tsdb_writer_* gauge family and
// the tsdb_writer_flush_failures evaluator rule alerts on failure growth.
//
// MetricWriter (metric_samples) is deliberately NOT registered: its stats
// would travel through the very pipeline being monitored, and its failure
// is already self-evident (every gauge goes stale and the evaluator's
// global no-rule-succeeding discriminator fires).
//
// Registration is last-wins by name, which keeps repeated construction in
// tests harmless; production constructs each writer once in serve startup.

var (
	writerStatsMu    sync.RWMutex
	writerStatsFuncs = map[string]func() FlushStats{}
)

func registerWriterStats(name string, fn func() FlushStats) {
	writerStatsMu.Lock()
	defer writerStatsMu.Unlock()
	writerStatsFuncs[name] = fn
}

// AllWriterStats snapshots the flush stats of every registered buffered
// writer, keyed by the hypertable the writer feeds.
func AllWriterStats() map[string]FlushStats {
	writerStatsMu.RLock()
	defer writerStatsMu.RUnlock()
	out := make(map[string]FlushStats, len(writerStatsFuncs))
	for name, fn := range writerStatsFuncs {
		out[name] = fn()
	}
	return out
}
