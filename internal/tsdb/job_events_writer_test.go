package tsdb

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// fakeJobPool captures the args of the last Exec for assertion.
type fakeJobPool struct {
	lastArgs []any
	calls    int
	err      error
}

func (f *fakeJobPool) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	f.calls++
	f.lastArgs = args
	return pgconn.CommandTag{}, f.err
}

func TestRecordJobEvent_NilPoolNoOp(t *testing.T) {
	if err := RecordJobEvent(context.Background(), nil, JobEventRow{JobName: "x"}); err != nil {
		t.Fatalf("nil pool should no-op, got %v", err)
	}
}

func TestRecordJobEvent_MapsFields(t *testing.T) {
	p := &fakeJobPool{}
	err := RecordJobEvent(context.Background(), p, JobEventRow{
		JobName: "tsdb-backup", SpaceID: "mdemg-dev", InstanceID: "host-1",
		Success: true, LatencyMS: 1234,
		Metadata: map[string]any{"size_bytes": 99},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if p.calls != 1 {
		t.Fatalf("expected 1 Exec, got %d", p.calls)
	}
	// args: event_id, recorded_at, job_name, space_id, instance_id, success, latency_ms, error_message, metadata
	if p.lastArgs[2] != "tsdb-backup" {
		t.Errorf("job_name arg = %v", p.lastArgs[2])
	}
	if p.lastArgs[3] != "mdemg-dev" {
		t.Errorf("space_id arg = %v", p.lastArgs[3])
	}
	if p.lastArgs[5] != true {
		t.Errorf("success arg = %v", p.lastArgs[5])
	}
	if p.lastArgs[6] != int64(1234) {
		t.Errorf("latency arg = %v", p.lastArgs[6])
	}
	// metadata is JSON-encoded string
	md, ok := p.lastArgs[8].(string)
	if !ok || !strings.Contains(md, "size_bytes") {
		t.Errorf("metadata arg should be JSON containing size_bytes, got %v", p.lastArgs[8])
	}
}

func TestRecordJobEvent_OptionalNullsWhenUnset(t *testing.T) {
	p := &fakeJobPool{}
	_ = RecordJobEvent(context.Background(), p, JobEventRow{JobName: "maintenance", Success: false})
	// space_id, instance_id, latency, error_message, metadata all unset → nil
	for _, idx := range []int{3, 4, 6, 7, 8} {
		if p.lastArgs[idx] != nil {
			t.Errorf("arg[%d] should be nil when unset, got %v", idx, p.lastArgs[idx])
		}
	}
}

func TestRecordJobEvent_TruncatesError(t *testing.T) {
	p := &fakeJobPool{}
	long := strings.Repeat("e", jobErrMessageMaxLen+500)
	_ = RecordJobEvent(context.Background(), p, JobEventRow{JobName: "export-auto", Success: false, ErrorMessage: long})
	got, _ := p.lastArgs[7].(string)
	if len(got) != jobErrMessageMaxLen {
		t.Errorf("error_message should be truncated to %d, got %d", jobErrMessageMaxLen, len(got))
	}
}

func TestRecordJobEvent_PropagatesInsertError(t *testing.T) {
	p := &fakeJobPool{err: errors.New("boom")}
	if err := RecordJobEvent(context.Background(), p, JobEventRow{JobName: "x", Success: true}); err == nil {
		t.Fatal("expected insert error to propagate")
	}
}
