// JIMINY-PROCESS-OBSERVER-06 (task #165) — never-haiku-for-planning matcher tests.
package matchers

import (
	"context"
	"strings"
	"testing"
	"time"

	"mdemg/internal/grader/process"
)

func TestNeverHaiku_OpusPlanning_Followed(t *testing.T) {
	// Uses haikuPool (below) for two-column returns since sibling fakePool
	// only supports one-column scans.
	m := NewNeverHaikuPlanning()
	h := newHaikuPool([]haikuRow{{subtype: "planning", model: "claude-opus-4-7"}})
	v, skip, err := m.Grade(context.Background(), h, process.TerminalEvent{
		EventID: "e1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil || skip {
		t.Fatalf("unexpected err=%v skip=%v", err, skip)
	}
	if v.OutcomeType != "process_followed" {
		t.Fatalf("want process_followed on opus, got %q reason=%q", v.OutcomeType, v.Reason)
	}
	if !strings.Contains(v.Reason, "claude-opus-4-7") {
		t.Fatalf("reason should name the model; got %q", v.Reason)
	}
}

func TestNeverHaiku_HaikuPlanning_Incomplete(t *testing.T) {
	m := NewNeverHaikuPlanning()
	h := newHaikuPool([]haikuRow{{subtype: "planning", model: "claude-haiku-4-5"}})
	v, _, err := m.Grade(context.Background(), h, process.TerminalEvent{
		EventID: "e1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v.OutcomeType != "process_incomplete" {
		t.Fatalf("want process_incomplete on haiku, got %q reason=%q", v.OutcomeType, v.Reason)
	}
	if !strings.Contains(strings.ToLower(v.Reason), "haiku") {
		t.Fatalf("reason should name haiku; got %q", v.Reason)
	}
}

func TestNeverHaiku_SonnetPlanning_Followed(t *testing.T) {
	m := NewNeverHaikuPlanning()
	h := newHaikuPool([]haikuRow{{subtype: "planning", model: "claude-sonnet-5"}})
	v, _, err := m.Grade(context.Background(), h, process.TerminalEvent{
		EventID: "e1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v.OutcomeType != "process_followed" {
		t.Fatalf("want process_followed on sonnet, got %q", v.OutcomeType)
	}
}

func TestNeverHaiku_NonPlanningSubtype_Skip(t *testing.T) {
	m := NewNeverHaikuPlanning()
	h := newHaikuPool([]haikuRow{{subtype: "other", model: "claude-haiku-4-5"}})
	_, skip, err := m.Grade(context.Background(), h, process.TerminalEvent{
		EventID: "e1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !skip {
		t.Fatalf("expected skip=true on non-planning subtype")
	}
}

func TestNeverHaiku_EmptyModel_Skip(t *testing.T) {
	m := NewNeverHaikuPlanning()
	h := newHaikuPool([]haikuRow{{subtype: "planning", model: ""}})
	_, skip, err := m.Grade(context.Background(), h, process.TerminalEvent{
		EventID: "e1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !skip {
		t.Fatalf("expected skip=true on empty model")
	}
}

func TestNeverHaiku_Metadata(t *testing.T) {
	m := NewNeverHaikuPlanning()
	if m.Name() != "never_haiku_planning" {
		t.Fatalf("Name mismatch: %q", m.Name())
	}
	if m.TerminalEventType() != "model_call" {
		t.Fatalf("TerminalEventType mismatch: %q", m.TerminalEventType())
	}
	if m.ConstraintCode() != "never-haiku-for-planning" {
		t.Fatalf("ConstraintCode mismatch: %q", m.ConstraintCode())
	}
}

// ── two-column fake pool for planningModel (subtype + model) ──────────

type haikuRow struct {
	subtype string
	model   string
}

type haikuPool struct {
	rows []haikuRow
	idx  int
}

func newHaikuPool(rows []haikuRow) *haikuPool { return &haikuPool{rows: rows} }

func (p *haikuPool) Query(_ context.Context, _ string, _ ...any) (process.Rows, error) {
	if p.idx >= len(p.rows) {
		return &haikuRows{done: true}, nil
	}
	r := p.rows[p.idx]
	p.idx++
	return &haikuRows{row: r, has: true}, nil
}

type haikuRows struct {
	row    haikuRow
	has    bool
	done   bool
	served bool
}

func (r *haikuRows) Next() bool {
	if r.done || !r.has || r.served {
		return false
	}
	r.served = true
	return true
}

func (r *haikuRows) Scan(dest ...any) error {
	if len(dest) < 2 {
		return nil
	}
	if p, ok := dest[0].(*string); ok {
		*p = r.row.subtype
	}
	if p, ok := dest[1].(*string); ok {
		*p = r.row.model
	}
	return nil
}

func (r *haikuRows) Close()     {}
func (r *haikuRows) Err() error { return nil }
