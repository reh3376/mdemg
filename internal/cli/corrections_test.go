package cli

import (
	"encoding/json"
	"testing"
)

// JIMINY-STRUCTURED-CORRECTION-001 Epic 3 — regex + merge Tier-1 pins.

func TestParseCorrectionContent_HappyPath(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wIn     string
		wCo     string
		wCtx    string
	}{
		{
			"three_parts",
			"CORRECTION: Incorrect: X | Correct: Y | Context: Z",
			"X", "Y", "Z",
		},
		{
			"two_parts_no_context",
			"CORRECTION: Incorrect: X | Correct: Y",
			"X", "Y", "",
		},
		{
			"multi_word",
			"CORRECTION: Incorrect: committed directly to main | Correct: always use a dev branch | Context: git workflow",
			"committed directly to main", "always use a dev branch", "git workflow",
		},
		{
			"embedded_context_pipe",
			"CORRECTION: Incorrect: A | Correct: B | Context: has | pipes | in | it",
			"A", "B", "has | pipes | in | it",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inc, cor, ctx, ok := parseCorrectionContent(tc.content)
			if !ok {
				t.Fatalf("parse failed on %q", tc.content)
			}
			if inc != tc.wIn || cor != tc.wCo || ctx != tc.wCtx {
				t.Errorf("parse mismatch: got (%q,%q,%q), want (%q,%q,%q)",
					inc, cor, ctx, tc.wIn, tc.wCo, tc.wCtx)
			}
		})
	}
}

func TestParseCorrectionContent_UnparseableSkipped(t *testing.T) {
	cases := []string{
		"",
		"just a random note",
		"CORRECTION missing the colon",
		"CORRECTION: Incorrect only", // no | Correct: tail
	}
	for _, c := range cases {
		if _, _, _, ok := parseCorrectionContent(c); ok {
			t.Errorf("expected unparseable, got parsed for %q", c)
		}
	}
}

func TestMergeStructuredCorrection_FreshWhenEmpty(t *testing.T) {
	got, err := mergeStructuredCorrection("", "X", "Y", "Z")
	if err != nil {
		t.Fatalf("merge err: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	c := m["correction"].(map[string]any)
	if c["incorrect"] != "X" || c["correct"] != "Y" || c["context"] != "Z" {
		t.Errorf("correction sub-object wrong: %#v", c)
	}
}

func TestMergeStructuredCorrection_PreservesExistingKeys(t *testing.T) {
	// Constraint detector may have populated the L0 obs with these keys —
	// the merge must NOT drop them.
	existing := `{"constraint_code":"never-commit-main","detected_constraints":[{"confidence":0.9}]}`
	got, err := mergeStructuredCorrection(existing, "X", "Y", "Z")
	if err != nil {
		t.Fatalf("merge err: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal([]byte(got), &m)
	if m["constraint_code"] != "never-commit-main" {
		t.Errorf("constraint_code dropped: %#v", m)
	}
	if _, ok := m["detected_constraints"].([]any); !ok {
		t.Errorf("detected_constraints dropped: %#v", m["detected_constraints"])
	}
	if c, _ := m["correction"].(map[string]any); c == nil || c["incorrect"] != "X" {
		t.Errorf("correction not added: %#v", m["correction"])
	}
}

func TestMergeStructuredCorrection_OverwritesExistingCorrection(t *testing.T) {
	// Idempotency: re-running the backfill with different-parsed fields
	// overwrites the correction key (repair path for a mis-parsed row).
	existing := `{"correction":{"incorrect":"OLD","correct":"OLD","context":"OLD"}}`
	got, _ := mergeStructuredCorrection(existing, "NEW-I", "NEW-C", "NEW-CTX")
	var m map[string]any
	_ = json.Unmarshal([]byte(got), &m)
	c := m["correction"].(map[string]any)
	if c["incorrect"] != "NEW-I" || c["correct"] != "NEW-C" || c["context"] != "NEW-CTX" {
		t.Errorf("re-merge did not overwrite: %#v", c)
	}
}

func TestMergeStructuredCorrection_MalformedExistingStartsFresh(t *testing.T) {
	// If existing structured_data is not valid JSON, merge starts fresh
	// rather than crashing (defensive parse in the merge helper).
	got, err := mergeStructuredCorrection("not json at all {", "X", "Y", "Z")
	if err != nil {
		t.Fatalf("merge err: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("output is malformed: %v", err)
	}
	c := m["correction"].(map[string]any)
	if c["incorrect"] != "X" {
		t.Errorf("correction not populated: %#v", c)
	}
}
