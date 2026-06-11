package tsdb

import "testing"

// TSDB-CONSUME-001: every buffered writer self-registers its flush stats at
// construction so the metrics plane can surface a wedged writer (previously
// only the reinforcement writer was visible; the rest dropped rows silently).
func TestWriterStatsRegistry_RoundTrip(t *testing.T) {
	registerWriterStats("test_writer", func() FlushStats {
		return FlushStats{SuccessCount: 3, FailureCount: 1, TotalRows: 42, OverflowCount: 2}
	})
	st, ok := AllWriterStats()["test_writer"]
	if !ok {
		t.Fatal("registered writer missing from AllWriterStats")
	}
	if st.SuccessCount != 3 || st.FailureCount != 1 || st.TotalRows != 42 || st.OverflowCount != 2 {
		t.Errorf("stats = %+v", st)
	}
}

// Constructing the writers must register them under their hypertable names.
func TestWriterStatsRegistry_WritersSelfRegister(t *testing.T) {
	// nil pool is safe: registration happens before any flush, and we never
	// enqueue rows here.
	w1 := NewConstraintOutcomesWriter(nil, 0)
	defer w1.Close()
	w2 := NewRetrievalAuditWriter(nil, 0)
	defer w2.Close()

	stats := AllWriterStats()
	for _, name := range []string{"constraint_outcomes", "retrieval_audit"} {
		if _, ok := stats[name]; !ok {
			t.Errorf("writer %q not registered at construction", name)
		}
	}
}
