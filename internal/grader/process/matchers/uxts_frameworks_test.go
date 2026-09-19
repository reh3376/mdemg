// JIMINY-PROCESS-OBSERVER-05 (task #164) — uxts-frameworks matcher tests.
package matchers

import (
	"context"
	"strings"
	"testing"
	"time"

	"mdemg/internal/grader/process"
)

func TestUxtsFrameworksRegex(t *testing.T) {
	cases := []struct {
		path      string
		wantMatch bool
	}{
		{"docs/tests/uats/specs/x.uats.json", true},
		{"docs/tests/uvts/specs/x.uvts.json", true},
		{"docs/tests/ubench/specs/x.ubench.json", true},
		{"docs/tests/ults/specs/x.ults.json", true},
		{"internal/foo/schema.uats.json", true},   // filename shape yes; path violation caught elsewhere
		{"docs/tests/uats/specs/config.json", false}, // no u-prefix
		{"internal/foo/config.json", false},
		{"internal/foo/data.uxts.json", true},     // any u-prefix .json matches
		{"docs/tests/uats/uats.json", false},      // no dot before u — filename is uats.json, regex needs a dot before u
	}
	for _, c := range cases {
		got := uxtsFilenameRe.MatchString(c.path)
		if got != c.wantMatch {
			t.Fatalf("uxtsFilenameRe.MatchString(%q) = %v; want %v", c.path, got, c.wantMatch)
		}
	}
}

func TestUxtsFrameworks_PathOK_Followed(t *testing.T) {
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"docs/tests/uats/specs/my_test.uats.json"}}},
	}}
	m := NewUxtsFrameworks()
	v, skip, err := m.Grade(context.Background(), pool, process.TerminalEvent{
		EventID: "ev1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil || skip {
		t.Fatalf("unexpected err=%v skip=%v", err, skip)
	}
	if v.OutcomeType != "process_followed" {
		t.Fatalf("want process_followed, got %q reason=%q", v.OutcomeType, v.Reason)
	}
	if v.ConstraintCode != "must-use-uxts-frameworks-consistently" {
		t.Fatalf("ConstraintCode mismatch: %q", v.ConstraintCode)
	}
}

func TestUxtsFrameworks_PathViolation_Incomplete(t *testing.T) {
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"internal/foo/schema.uats.json"}}},
	}}
	m := NewUxtsFrameworks()
	v, skip, err := m.Grade(context.Background(), pool, process.TerminalEvent{
		EventID: "ev1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil || skip {
		t.Fatalf("unexpected err=%v skip=%v", err, skip)
	}
	if v.OutcomeType != "process_incomplete" {
		t.Fatalf("want process_incomplete, got %q reason=%q", v.OutcomeType, v.Reason)
	}
	if !strings.Contains(v.Reason, "internal/foo/schema.uats.json") {
		t.Fatalf("reason should name the violating path; got %q", v.Reason)
	}
}

func TestUxtsFrameworks_NonUxtsJson_Skip(t *testing.T) {
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"internal/foo/config.json"}}},
	}}
	m := NewUxtsFrameworks()
	_, skip, err := m.Grade(context.Background(), pool, process.TerminalEvent{
		EventID: "ev1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !skip {
		t.Fatalf("expected skip=true on non-UxTS-shape file")
	}
}

func TestUxtsFrameworks_EmptyFilePath_Skip(t *testing.T) {
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{""}}},
	}}
	m := NewUxtsFrameworks()
	_, skip, err := m.Grade(context.Background(), pool, process.TerminalEvent{
		EventID: "ev1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !skip {
		t.Fatalf("expected skip=true on empty file_path")
	}
}

func TestUxtsFrameworks_NestedDocsTests_Followed(t *testing.T) {
	// Any path containing docs/tests/u<name>/ satisfies the check —
	// including deeply-nested UxTS spec files.
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"/Users/x/mdemg/docs/tests/ubench/contracts/deep/mdemg.ubench.json"}}},
	}}
	m := NewUxtsFrameworks()
	v, skip, err := m.Grade(context.Background(), pool, process.TerminalEvent{
		EventID: "ev1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil || skip {
		t.Fatalf("unexpected err=%v skip=%v", err, skip)
	}
	if v.OutcomeType != "process_followed" {
		t.Fatalf("want process_followed on nested docs/tests path, got %q", v.OutcomeType)
	}
}

func TestUxtsFrameworks_Metadata(t *testing.T) {
	m := NewUxtsFrameworks()
	if m.Name() != "uxts_frameworks" {
		t.Fatalf("Name mismatch: %q", m.Name())
	}
	if m.TerminalEventType() != "file_write" {
		t.Fatalf("TerminalEventType mismatch: %q", m.TerminalEventType())
	}
	if m.ConstraintCode() != "must-use-uxts-frameworks-consistently" {
		t.Fatalf("ConstraintCode mismatch: %q", m.ConstraintCode())
	}
}
