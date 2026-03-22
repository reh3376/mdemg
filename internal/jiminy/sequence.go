package jiminy

import (
	"sync"
	"sync/atomic"
	"time"
)

// SequenceTracker manages a monotonic sequence counter and ring buffer of events.
type SequenceTracker struct {
	counter    atomic.Int64
	mu         sync.RWMutex
	events     []SequenceEvent
	bufferSize int
	writeIdx   int // ring buffer write position
	count      int // total events stored (up to bufferSize)
}

// NewSequenceTracker creates a new SequenceTracker with the given ring buffer size.
func NewSequenceTracker(bufferSize int) *SequenceTracker {
	if bufferSize <= 0 {
		bufferSize = 1000
	}
	return &SequenceTracker{
		events:     make([]SequenceEvent, bufferSize),
		bufferSize: bufferSize,
	}
}

// Next returns the next monotonic sequence number.
func (st *SequenceTracker) Next() int64 {
	return st.counter.Add(1)
}

// Current returns the current sequence number (last assigned).
func (st *SequenceTracker) Current() int64 {
	return st.counter.Load()
}

// Record stores an event in the ring buffer.
func (st *SequenceTracker) Record(event SequenceEvent) {
	st.mu.Lock()
	defer st.mu.Unlock()

	st.events[st.writeIdx] = event
	st.writeIdx = (st.writeIdx + 1) % st.bufferSize
	if st.count < st.bufferSize {
		st.count++
	}
}

// RecordGuidance records a guidance event with the next sequence number.
func (st *SequenceTracker) RecordGuidance(spaceID, sessionID string, itemCount int, summary string) int64 {
	seq := st.Next()
	st.Record(SequenceEvent{
		Seq:       seq,
		Timestamp: time.Now(),
		SpaceID:   spaceID,
		SessionID: sessionID,
		ItemCount: itemCount,
		Summary:   summary,
	})
	return seq
}

// EventsSince returns all events with sequence numbers > afterSeq,
// up to maxEvents. Events are returned in order (oldest first).
func (st *SequenceTracker) EventsSince(afterSeq int64, maxEvents int) []SequenceEvent {
	st.mu.RLock()
	defer st.mu.RUnlock()

	if maxEvents <= 0 {
		maxEvents = 50
	}

	// Collect matching events from the ring buffer
	var result []SequenceEvent
	for i := 0; i < st.count; i++ {
		// Read from oldest to newest
		idx := (st.writeIdx - st.count + i + st.bufferSize) % st.bufferSize
		ev := st.events[idx]
		if ev.Seq > afterSeq {
			result = append(result, ev)
			if len(result) >= maxEvents {
				break
			}
		}
	}

	return result
}

// Count returns the number of events currently in the buffer.
func (st *SequenceTracker) Count() int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.count
}

// SetCounter sets the sequence counter to a specific value (used for restore).
func (st *SequenceTracker) SetCounter(val int64) {
	st.counter.Store(val)
}

// BufferSize returns the current ring buffer capacity.
func (st *SequenceTracker) BufferSize() int {
	st.mu.RLock()
	defer st.mu.RUnlock()
	return st.bufferSize
}

// Resize adjusts the ring buffer capacity, preserving the most recent events.
// No-op if newSize <= 0 or equal to current size.
func (st *SequenceTracker) Resize(newSize int) {
	if newSize <= 0 {
		return
	}

	st.mu.Lock()
	defer st.mu.Unlock()

	if newSize == st.bufferSize {
		return
	}

	// Collect existing events in order (oldest → newest)
	existing := make([]SequenceEvent, 0, st.count)
	for i := 0; i < st.count; i++ {
		idx := (st.writeIdx - st.count + i + st.bufferSize) % st.bufferSize
		existing = append(existing, st.events[idx])
	}

	// Keep only the most recent events that fit in the new buffer
	newBuf := make([]SequenceEvent, newSize)
	kept := len(existing)
	if kept > newSize {
		existing = existing[kept-newSize:]
		kept = newSize
	}
	copy(newBuf, existing)

	st.events = newBuf
	st.bufferSize = newSize
	st.writeIdx = kept % newSize
	st.count = kept
}
