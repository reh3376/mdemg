// JIMINY-PROCESS-OBSERVER-04 (task #163) — unit-integration-e2e-docs matcher tests.
package matchers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mdemg/internal/grader/process"
)

// ── Pure-function pins ────────────────────────────────────────────────

func TestExtractSprintCode(t *testing.T) {
	cases := []struct {
		msg    string
		want   string
		wantOK bool
	}{
		{"", "", false},
		{"chore: bump deps", "", false},
		{"feat(process): Epic 2 (JIMINY-PROCESS-OBSERVER-04 E1-E4)", "JIMINY-PROCESS-OBSERVER-04", true},
		{"docs(x): SEC-TRANCHE-3 followup", "SEC-TRANCHE-3", true},
		{"single-word issue-123 ref", "", false},         // requires hyphen; ISSUE-123 is single-word
		{"feat: HITL-REVIEW-001 shipped", "HITL-REVIEW-001", true},
		{"CVE-2026-63374 fix", "CVE-2026-63374", true},   // CVE codes match — acceptable false-positive class; would still fail-open at file-lookup
	}
	for _, c := range cases {
		got, ok := extractSprintCode(c.msg)
		if got != c.want || ok != c.wantOK {
			t.Fatalf("extractSprintCode(%q) = (%q, %v); want (%q, %v)", c.msg, got, ok, c.want, c.wantOK)
		}
	}
}

func TestKebabLowerSprintCode(t *testing.T) {
	if got := kebabLowerSprintCode("JIMINY-PROCESS-OBSERVER-04"); got != "jiminy-process-observer-04" {
		t.Fatalf("kebab-lower mismatch: %q", got)
	}
	if got := kebabLowerSprintCode("SEC-TRANCHE-3"); got != "sec-tranche-3" {
		t.Fatalf("kebab-lower mismatch: %q", got)
	}
}

func TestDetectTiers(t *testing.T) {
	all3 := strings.ToLower(`
## Testing Plan
- Tier 1 unit tests
- Tier 2 integration tests
- Tier 3 e2e
`)
	present, missing := detectTiers(all3)
	if len(missing) != 0 {
		t.Fatalf("expected all present, got missing=%v", missing)
	}
	if len(present) != 3 {
		t.Fatalf("expected 3 tiers present, got %v", present)
	}

	noE2e := strings.ToLower(`
## Testing Plan
- unit tests
- integration tests
`)
	_, missing2 := detectTiers(noE2e)
	if len(missing2) != 1 || missing2[0] != "e2e" {
		t.Fatalf("expected only e2e missing, got %v", missing2)
	}

	endToEndAlt := strings.ToLower(`
- unit tests
- integration tests
- end-to-end tests
`)
	_, missing3 := detectTiers(endToEndAlt)
	if len(missing3) != 0 {
		t.Fatalf("end-to-end should satisfy e2e; got missing=%v", missing3)
	}
}

// ── Grade-level integration with real filesystem ─────────────────────

func TestUIED_AllTiersPresent_Followed(t *testing.T) {
	root := t.TempDir()
	planDir := filepath.Join(root, "jiminy-process-observer-04")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	planBody := `# Sprint Plan
## Testing Plan (3 tiers)
- Tier 1 unit tests
- Tier 2 integration tests
- Tier 3 e2e
`
	if err := os.WriteFile(filepath.Join(planDir, "sprint_plan.md"), []byte(planBody), 0o644); err != nil {
		t.Fatal(err)
	}
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"feat(process): JIMINY-PROCESS-OBSERVER-04 E1"}}}, // commitMessage
	}}
	m := NewUIED(root, 200000)
	v, skip, err := m.Grade(context.Background(), pool, process.TerminalEvent{
		EventID: "c1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil || skip {
		t.Fatalf("unexpected err=%v skip=%v", err, skip)
	}
	if v.OutcomeType != "process_followed" {
		t.Fatalf("want process_followed, got %q reason=%q", v.OutcomeType, v.Reason)
	}
}

func TestUIED_MissingTier_Incomplete(t *testing.T) {
	root := t.TempDir()
	planDir := filepath.Join(root, "jiminy-process-observer-04")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	planBody := `# Sprint Plan
## Testing Plan
- unit tests
- integration tests
` // no e2e
	if err := os.WriteFile(filepath.Join(planDir, "sprint_plan.md"), []byte(planBody), 0o644); err != nil {
		t.Fatal(err)
	}
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"feat: JIMINY-PROCESS-OBSERVER-04 platform"}}},
	}}
	m := NewUIED(root, 200000)
	v, _, err := m.Grade(context.Background(), pool, process.TerminalEvent{
		EventID: "c1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v.OutcomeType != "process_incomplete" {
		t.Fatalf("want process_incomplete, got %q reason=%q", v.OutcomeType, v.Reason)
	}
	if !strings.Contains(v.Reason, "e2e") {
		t.Fatalf("reason should name missing tier e2e; got %q", v.Reason)
	}
}

func TestUIED_FileAbsent_Missed(t *testing.T) {
	root := t.TempDir() // empty
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"feat: JIMINY-PROCESS-OBSERVER-04 platform"}}},
	}}
	m := NewUIED(root, 200000)
	v, _, err := m.Grade(context.Background(), pool, process.TerminalEvent{
		EventID: "c1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v.OutcomeType != "process_missed" {
		t.Fatalf("want process_missed on absent file, got %q reason=%q", v.OutcomeType, v.Reason)
	}
}

func TestUIED_NoSprintCode_Skip(t *testing.T) {
	root := t.TempDir()
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"chore: bump deps"}}},
	}}
	m := NewUIED(root, 200000)
	_, skip, err := m.Grade(context.Background(), pool, process.TerminalEvent{
		EventID: "c1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !skip {
		t.Fatalf("expected skip=true on no-sprint-code commit")
	}
}

func TestUIED_OverCapFile_Skip(t *testing.T) {
	root := t.TempDir()
	planDir := filepath.Join(root, "jiminy-process-observer-04")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write a file larger than the cap
	huge := strings.Repeat("x", 8192)
	if err := os.WriteFile(filepath.Join(planDir, "sprint_plan.md"), []byte(huge), 0o644); err != nil {
		t.Fatal(err)
	}
	pool := &fakePool{responses: []fakeRows{
		{rows: [][]any{{"feat: JIMINY-PROCESS-OBSERVER-04"}}},
	}}
	m := NewUIED(root, 4096) // cap smaller than the file
	_, skip, err := m.Grade(context.Background(), pool, process.TerminalEvent{
		EventID: "c1", SpaceID: "mdemg-dev", SessionID: "s1", Time: time.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !skip {
		t.Fatalf("expected skip=true on over-cap file")
	}
}

func TestUIED_Metadata(t *testing.T) {
	m := NewUIED("", 0)
	if m.Name() != "unit_integration_e2e_docs" {
		t.Fatalf("Name mismatch: %q", m.Name())
	}
	if m.TerminalEventType() != "git_commit" {
		t.Fatalf("TerminalEventType mismatch: %q", m.TerminalEventType())
	}
	if m.ConstraintCode() != "unit-integration-e2e-docs" {
		t.Fatalf("ConstraintCode mismatch: %q", m.ConstraintCode())
	}
	// Zero-value defaults
	if m.SprintDocsRoot != "docs/development" {
		t.Fatalf("expected default SprintDocsRoot 'docs/development', got %q", m.SprintDocsRoot)
	}
	if m.MaxFileBytes != 200000 {
		t.Fatalf("expected default MaxFileBytes 200000, got %d", m.MaxFileBytes)
	}
}
