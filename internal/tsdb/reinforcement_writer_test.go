package tsdb

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// fakePool captures CopyFrom invocations for assertion. It is concurrent-safe
// because the writer's flushLoop fires on its own goroutine.
type fakePool struct {
	mu        sync.Mutex
	calls     []fakeCopyFromCall
	returnErr error
}

type fakeCopyFromCall struct {
	tableName pgx.Identifier
	columns   []string
	rows      [][]any
}

func (p *fakePool) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string,
	rowSrc pgx.CopyFromSource) (int64, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	var rows [][]any
	for rowSrc.Next() {
		values, err := rowSrc.Values()
		if err != nil {
			return 0, err
		}
		rows = append(rows, values)
	}
	p.calls = append(p.calls, fakeCopyFromCall{
		tableName: tableName,
		columns:   columnNames,
		rows:      rows,
	})
	if p.returnErr != nil {
		return 0, p.returnErr
	}
	return int64(len(rows)), nil
}

func (p *fakePool) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

func (p *fakePool) lastCall() fakeCopyFromCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.calls) == 0 {
		return fakeCopyFromCall{}
	}
	return p.calls[len(p.calls)-1]
}

func sampleRow() ReinforcementEventRow {
	return ReinforcementEventRow{
		SpaceID:            "mdemg-dev",
		SrcNodeID:          "node-a",
		DstNodeID:          "node-b",
		PrevWeight:         0.10,
		NewWeight:          0.18,
		DeltaWeight:        0.08,
		EvidenceCountAfter: 3,
		EtaEffective:       0.04,
		SurpriseFactor:     1.5,
		ActivationProduct:  0.6,
		PathSim:            0.7,
		RoleA:              "code",
		RoleB:              "conversation_observation",
		ObsTypeA:           "",
		ObsTypeB:           "learning",
		SessionID:          "session-42",
		Direction:          "bidirectional",
		CreatedNewEdge:     false,
		TriggerPath:        "apply_coactivation",
	}
}

func TestReinforcementWriter_RecordThenFlush_WritesAllRows(t *testing.T) {
	pool := &fakePool{}
	w := NewReinforcementEventsWriter(pool, time.Hour, 0)
	defer w.Close()

	for range 3 {
		w.Record(sampleRow())
	}
	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}
	if pool.callCount() != 1 {
		t.Fatalf("expected 1 CopyFrom call, got %d", pool.callCount())
	}
	last := pool.lastCall()
	if got := last.tableName[0]; got != "reinforcement_events" {
		t.Errorf("table name = %q, want reinforcement_events", got)
	}
	if len(last.rows) != 3 {
		t.Errorf("got %d rows, want 3", len(last.rows))
	}
	stats := w.Stats()
	if stats.SuccessCount != 1 || stats.TotalRows != 3 {
		t.Errorf("stats = %+v, want SuccessCount=1 TotalRows=3", stats)
	}
}

func TestReinforcementWriter_FlushEmptyBuffer_NoCopyFrom(t *testing.T) {
	pool := &fakePool{}
	w := NewReinforcementEventsWriter(pool, time.Hour, 0)
	defer w.Close()

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush(empty) returned error: %v", err)
	}
	if pool.callCount() != 0 {
		t.Errorf("expected 0 CopyFrom calls on empty buffer, got %d", pool.callCount())
	}
}

func TestReinforcementWriter_BufferFull_EvictsOldestAndCountsDrop(t *testing.T) {
	pool := &fakePool{}
	w := NewReinforcementEventsWriter(pool, time.Hour, 3) // cap = 3
	defer w.Close()

	// Record 5 rows with distinguishable SessionIDs.
	for i := range 5 {
		row := sampleRow()
		row.SessionID = "session-" + string(rune('0'+i))
		w.Record(row)
	}

	if got := w.Stats().DroppedRows; got != 2 {
		t.Errorf("dropped rows = %d, want 2 (evicted first 2)", got)
	}

	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}
	rows := pool.lastCall().rows
	if len(rows) != 3 {
		t.Fatalf("flushed %d rows, want 3", len(rows))
	}
	// Verify oldest two were evicted: remaining sessions are 2, 3, 4.
	// session_id is column index 17 (event_id=0, recorded_at=1, space_id=2,
	// src=3, dst=4, prev=5, new=6, delta=7, evidence=8, eta=9, surprise=10,
	// activation=11, path_sim=12, role_a=13, role_b=14, obs_type_a=15,
	// obs_type_b=16, session_id=17).
	wantSessions := []string{"session-2", "session-3", "session-4"}
	for i, want := range wantSessions {
		got, ok := rows[i][17].(string)
		if !ok {
			t.Fatalf("row %d session_id is not string: %v (%T)", i, rows[i][17], rows[i][17])
		}
		if got != want {
			t.Errorf("row %d session_id = %q, want %q", i, got, want)
		}
	}
}

func TestReinforcementWriter_UnlimitedBuffer_NoDrop(t *testing.T) {
	pool := &fakePool{}
	w := NewReinforcementEventsWriter(pool, time.Hour, 0) // maxBufferSize=0 → unlimited
	defer w.Close()
	for range 100 {
		w.Record(sampleRow())
	}
	if got := w.Stats().DroppedRows; got != 0 {
		t.Errorf("dropped rows = %d, want 0 (unlimited buffer)", got)
	}
}

func TestReinforcementWriter_NullableSerialization(t *testing.T) {
	pool := &fakePool{}
	w := NewReinforcementEventsWriter(pool, time.Hour, 0)
	defer w.Close()

	// Row with zero-valued optional fields → expect NULL serialization.
	row := ReinforcementEventRow{
		SpaceID:            "mdemg-dev",
		SrcNodeID:          "a",
		DstNodeID:          "b",
		PrevWeight:         0.1,
		NewWeight:          0.2,
		DeltaWeight:        0.1,
		EvidenceCountAfter: 1,
		// All optional float / string fields left at zero value.
		CreatedNewEdge: true,
		TriggerPath:    "apply_coactivation",
	}
	w.Record(row)
	if err := w.Flush(context.Background()); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}

	last := pool.lastCall()
	if len(last.rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(last.rows))
	}
	r := last.rows[0]

	// Indices for the nullable optional fields:
	// eta_effective=9, surprise_factor=10, activation_product=11, path_sim=12
	// role_a=13, role_b=14, obs_type_a=15, obs_type_b=16, session_id=17, direction=18
	nullable := []int{9, 10, 11, 12, 13, 14, 15, 16, 17, 18}
	for _, idx := range nullable {
		if r[idx] != nil {
			t.Errorf("column index %d should be nil for zero-valued input, got %v (%T)", idx, r[idx], r[idx])
		}
	}

	// Required fields must NOT be nil.
	if r[5] == nil || r[6] == nil || r[7] == nil || r[8] == nil {
		t.Errorf("required weight/evidence fields should not be nil: prev=%v new=%v delta=%v evidence=%v",
			r[5], r[6], r[7], r[8])
	}
	if r[19] != true {
		t.Errorf("created_new_edge = %v, want true", r[19])
	}
	if r[20] != "apply_coactivation" {
		t.Errorf("trigger_path = %v, want apply_coactivation", r[20])
	}
}

func TestReinforcementWriter_FlushError_IncrementsFailureCounter(t *testing.T) {
	pool := &fakePool{returnErr: errors.New("pg conn refused")}
	w := NewReinforcementEventsWriter(pool, time.Hour, 0)
	defer w.Close()
	w.Record(sampleRow())
	err := w.Flush(context.Background())
	if err == nil {
		t.Fatal("expected Flush to return the underlying error")
	}
	stats := w.Stats()
	if stats.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", stats.FailureCount)
	}
	if stats.SuccessCount != 0 || stats.TotalRows != 0 {
		t.Errorf("SuccessCount/TotalRows should remain 0 on flush failure, got %+v", stats)
	}
}

func TestReinforcementWriter_Close_DrainsBuffer(t *testing.T) {
	pool := &fakePool{}
	w := NewReinforcementEventsWriter(pool, time.Hour, 0)
	for range 5 {
		w.Record(sampleRow())
	}
	w.Close()
	if pool.callCount() != 1 {
		t.Fatalf("Close should have triggered a final flush; CopyFrom calls = %d", pool.callCount())
	}
	if len(pool.lastCall().rows) != 5 {
		t.Errorf("final-flush rows = %d, want 5", len(pool.lastCall().rows))
	}
}

func TestReinforcementWriter_Close_IsIdempotent(t *testing.T) {
	pool := &fakePool{}
	w := NewReinforcementEventsWriter(pool, time.Hour, 0)
	w.Record(sampleRow())
	w.Close()
	// Second Close should not panic and should not double-flush an empty buffer
	// into a CopyFrom call.
	w.Close()
	if pool.callCount() != 1 {
		t.Errorf("Close()×2 caused %d CopyFrom calls; want 1", pool.callCount())
	}
}

func TestReinforcementWriter_FlushTickerFires(t *testing.T) {
	pool := &fakePool{}
	w := NewReinforcementEventsWriter(pool, 50*time.Millisecond, 0)
	defer w.Close()
	w.Record(sampleRow())

	// Wait for at least one ticker fire.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pool.callCount() >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if pool.callCount() < 1 {
		t.Errorf("expected ≥1 auto-flush CopyFrom call within deadline, got %d", pool.callCount())
	}
}
