package jiminy

import (
	"testing"
)

func TestSequenceTracker_BufferSize(t *testing.T) {
	st := NewSequenceTracker(500)
	if st.BufferSize() != 500 {
		t.Errorf("BufferSize() = %d, want 500", st.BufferSize())
	}
}

func TestSequenceTracker_ResizeUp(t *testing.T) {
	st := NewSequenceTracker(5)

	// Fill buffer with 5 events
	for i := 0; i < 5; i++ {
		st.RecordGuidance("space", "sess", i, "event")
	}

	if st.Count() != 5 {
		t.Fatalf("count before resize = %d, want 5", st.Count())
	}

	// Resize up to 10 — all events preserved
	st.Resize(10)

	if st.BufferSize() != 10 {
		t.Errorf("BufferSize() = %d, want 10", st.BufferSize())
	}
	if st.Count() != 5 {
		t.Errorf("Count() = %d, want 5 (all preserved)", st.Count())
	}

	// Verify events are still accessible
	events := st.EventsSince(0, 10)
	if len(events) != 5 {
		t.Errorf("EventsSince(0) = %d events, want 5", len(events))
	}
}

func TestSequenceTracker_ResizeDown(t *testing.T) {
	st := NewSequenceTracker(10)

	// Fill buffer with 10 events (seq 1-10)
	for i := 0; i < 10; i++ {
		st.RecordGuidance("space", "sess", i, "event")
	}

	if st.Count() != 10 {
		t.Fatalf("count before resize = %d, want 10", st.Count())
	}

	// Resize down to 3 — only most recent 3 events kept (seq 8, 9, 10)
	st.Resize(3)

	if st.BufferSize() != 3 {
		t.Errorf("BufferSize() = %d, want 3", st.BufferSize())
	}
	if st.Count() != 3 {
		t.Errorf("Count() = %d, want 3", st.Count())
	}

	// The oldest surviving event should be seq 8
	events := st.EventsSince(0, 10)
	if len(events) != 3 {
		t.Fatalf("EventsSince(0) = %d events, want 3", len(events))
	}
	if events[0].Seq != 8 {
		t.Errorf("oldest event seq = %d, want 8", events[0].Seq)
	}
	if events[2].Seq != 10 {
		t.Errorf("newest event seq = %d, want 10", events[2].Seq)
	}
}

func TestSequenceTracker_ResizeSameSize(t *testing.T) {
	st := NewSequenceTracker(5)
	st.RecordGuidance("space", "sess", 1, "event")

	st.Resize(5) // no-op

	if st.BufferSize() != 5 {
		t.Errorf("BufferSize() = %d, want 5", st.BufferSize())
	}
	if st.Count() != 1 {
		t.Errorf("Count() = %d, want 1", st.Count())
	}
}

func TestSequenceTracker_ResizeZero(t *testing.T) {
	st := NewSequenceTracker(5)
	st.RecordGuidance("space", "sess", 1, "event")

	st.Resize(0)  // no-op for invalid size
	st.Resize(-1) // no-op for negative

	if st.BufferSize() != 5 {
		t.Errorf("BufferSize() = %d, want 5", st.BufferSize())
	}
	if st.Count() != 1 {
		t.Errorf("Count() = %d, want 1", st.Count())
	}
}
