// JIMINY-PROCESS-OBSERVER-02 (task #161) — sequential-epics matcher tests.
package matchers

import (
	"context"
	"errors"
	"testing"
	"time"

	"mdemg/internal/grader/process"
)

// ── extractEpicNumber pin tests ──────────────────────────────────────

func TestExtractEpicNumber(t *testing.T) {
	cases := []struct {
		name    string
		msg     string
		wantN   int
		wantOK  bool
	}{
		{"empty", "", 0, false},
		{"whitespace", "   \n\t", 0, false},
		{"no marker", "feat: ship new feature", 0, false},
		{"lower epic", "feat(process): epic 3 ships", 3, true},
		{"capital Epic", "docs: Epic 1 shipped", 1, true},
		{"mixed", "chore: EPIC 7 test", 7, true},
		{"double digits", "Epic 12 done", 12, true},
		{"embedded", "fix: correct off-by-one in Epic 5 loop", 5, true},
		{"no space", "epic5 done", 0, false}, // word boundary required
		{"malformed", "Epic X", 0, false},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			gotN, gotOK := extractEpicNumber(c.msg)
			if gotN != c.wantN || gotOK != c.wantOK {
				t.Fatalf("extractEpicNumber(%q) = (%d, %v); want (%d, %v)",
					c.msg, gotN, gotOK, c.wantN, c.wantOK)
			}
		})
	}
}

// ── Grade pin tests ──────────────────────────────────────────────────

// fakePool implements process.PoolIface with a scripted response list.
// Each Query call pops the next scripted response.
type fakePool struct {
	responses []fakeRows
	calls     int
}

type fakeRows struct {
	rows [][]any
	err  error
	idx  int
}

func (p *fakePool) Query(_ context.Context, _ string, _ ...any) (process.Rows, error) {
	if p.calls >= len(p.responses) {
		return nil, errors.New("fakePool: no more scripted responses")
	}
	r := p.responses[p.calls]
	p.calls++
	if r.err != nil {
		return nil, r.err
	}
	rr := r
	return &rr, nil
}

func (r *fakeRows) Next() bool { return r.idx < len(r.rows) }
func (r *fakeRows) Scan(dest ...any) error {
	if r.idx >= len(r.rows) {
		return errors.New("no row")
	}
	row := r.rows[r.idx]
	r.idx++
	for i, v := range row {
		if i >= len(dest) {
			break
		}
		switch p := dest[i].(type) {
		case *string:
			*p = v.(string)
		case *int:
			*p = v.(int)
		default:
			return errors.New("fakeRows: unsupported dest type")
		}
	}
	return nil
}
func (r *fakeRows) Close()     {}
func (r *fakeRows) Err() error { return nil }

func makeEv(id string) process.TerminalEvent {
	return process.TerminalEvent{
		EventID:   id,
		SpaceID:   "mdemg-dev",
		SessionID: "s1",
		Time:      time.Now(),
	}
}

func TestSequentialEpics_NoMarkerSkip(t *testing.T) {
	// current commit: no Epic marker → skip
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"feat: no marker here"}}}, // commitMessage
	}}
	m := NewSequentialEpics()
	_, skip, err := m.Grade(context.Background(), pool, makeEv("c1"))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !skip {
		t.Fatalf("expected skip=true when no Epic marker")
	}
}

func TestSequentialEpics_NoPriors_Followed(t *testing.T) {
	// current: Epic 1. No prior commits.
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"feat: Epic 1 platform"}}}, // commitMessage
		{rows: nil},                                // maxPriorEpicNumber (no rows)
	}}
	m := NewSequentialEpics()
	v, skip, err := m.Grade(context.Background(), pool, makeEv("c1"))
	if err != nil || skip {
		t.Fatalf("unexpected err=%v skip=%v", err, skip)
	}
	if v.OutcomeType != "process_followed" {
		t.Fatalf("want process_followed, got %q", v.OutcomeType)
	}
	if v.ConstraintCode != "sequential-epics" {
		t.Fatalf("want constraint_code=sequential-epics, got %q", v.ConstraintCode)
	}
}

func TestSequentialEpics_Monotonic_Followed(t *testing.T) {
	// current: Epic 3. Prior commits: Epic 1, Epic 2. In order.
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"feat: Epic 3 wire"}}},
		{rows: [][]any{
			{"c_prior1", "feat: Epic 1"},
			{"c_prior2", "feat: Epic 2"},
		}},
	}}
	m := NewSequentialEpics()
	v, skip, err := m.Grade(context.Background(), pool, makeEv("c3"))
	if err != nil || skip {
		t.Fatalf("unexpected err=%v skip=%v", err, skip)
	}
	if v.OutcomeType != "process_followed" {
		t.Fatalf("want process_followed, got %q reason=%q", v.OutcomeType, v.Reason)
	}
}

func TestSequentialEpics_EqualN_Followed(t *testing.T) {
	// current: Epic 2. Prior: Epic 2 (multiple commits on same epic).
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"fix: Epic 2 lint"}}},
		{rows: [][]any{{"c_prior", "feat: Epic 2 core"}}},
	}}
	m := NewSequentialEpics()
	v, skip, err := m.Grade(context.Background(), pool, makeEv("c2b"))
	if err != nil || skip {
		t.Fatalf("unexpected err=%v skip=%v", err, skip)
	}
	if v.OutcomeType != "process_followed" {
		t.Fatalf("want process_followed on equal N, got %q", v.OutcomeType)
	}
}

func TestSequentialEpics_OutOfOrder_Incomplete(t *testing.T) {
	// current: Epic 2. Prior: Epic 1 THEN Epic 3 (out of order — the max prior is 3, current is 2).
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"feat: Epic 2 catch-up"}}},
		{rows: [][]any{
			{"c_prior1", "feat: Epic 1"},
			{"c_prior3", "feat: Epic 3 skip-ahead"},
		}},
	}}
	m := NewSequentialEpics()
	v, skip, err := m.Grade(context.Background(), pool, makeEv("c2_late"))
	if err != nil || skip {
		t.Fatalf("unexpected err=%v skip=%v", err, skip)
	}
	if v.OutcomeType != "process_incomplete" {
		t.Fatalf("want process_incomplete, got %q reason=%q", v.OutcomeType, v.Reason)
	}
	if v.EvidenceEventID != "c_prior3" {
		t.Fatalf("expected evidence to be the out-of-order prior (c_prior3), got %q", v.EvidenceEventID)
	}
}

func TestSequentialEpics_PriorsWithoutMarkers_Followed(t *testing.T) {
	// current: Epic 5. Prior commits exist but none carry Epic markers → treated as no priors → followed.
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"feat: Epic 5"}}},
		{rows: [][]any{
			{"c_chore1", "chore: bump deps"},
			{"c_chore2", "docs: fix typo"},
		}},
	}}
	m := NewSequentialEpics()
	v, skip, err := m.Grade(context.Background(), pool, makeEv("c5"))
	if err != nil || skip {
		t.Fatalf("unexpected err=%v skip=%v", err, skip)
	}
	if v.OutcomeType != "process_followed" {
		t.Fatalf("want process_followed when priors have no markers, got %q", v.OutcomeType)
	}
}

// ── Matcher metadata pins ────────────────────────────────────────────

func TestSequentialEpics_Metadata(t *testing.T) {
	m := NewSequentialEpics()
	if m.Name() != "sequential_epics" {
		t.Fatalf("Name mismatch: %q", m.Name())
	}
	if m.TerminalEventType() != "git_commit" {
		t.Fatalf("TerminalEventType mismatch: %q", m.TerminalEventType())
	}
	if m.ConstraintCode() != "sequential-epics" {
		t.Fatalf("ConstraintCode mismatch: %q", m.ConstraintCode())
	}
}
