package guardrail

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

// TSDB-WRITER-UTF8-001: a mid-rune byte cut produced invalid UTF-8 that
// poisoned an llm_interactions flush batch. The cut must land on a rune
// boundary and stay within the byte budget.
func TestTruncateString_RuneSafe(t *testing.T) {
	// '→' is 3 bytes (0xe2 0x86 0x92). Build strings where the cut lands on
	// every possible mid-rune offset.
	for pad := 0; pad < 4; pad++ {
		prefix := strings.Repeat("a", 195+pad)
		s := prefix + strings.Repeat("→", 10)
		for _, maxLen := range []int{197, 198, 199, 200, 201, 202} {
			got := truncateString(s, maxLen)
			if !utf8.ValidString(got) {
				t.Errorf("pad=%d maxLen=%d: invalid UTF-8 output %q", pad, maxLen, got)
			}
			if len(got) > maxLen {
				t.Errorf("pad=%d maxLen=%d: output %d bytes exceeds budget", pad, maxLen, len(got))
			}
		}
	}
	// tiny budgets (maxLen < 4) must also stay rune-safe
	for _, maxLen := range []int{1, 2, 3} {
		got := truncateString("→→", maxLen)
		if !utf8.ValidString(got) {
			t.Errorf("maxLen=%d: invalid UTF-8 %q", maxLen, got)
		}
	}
	// no-truncate fast path unchanged
	if got := truncateString("short", 100); got != "short" {
		t.Errorf("fast path: got %q", got)
	}
}

// ============================================================
// buildResponse tests
// ============================================================

// must constraint violation → Block status
func TestBuildResponse_MustViolation_Block(t *testing.T) {
	constraints := []constraintMatch{
		{NodeID: "c-001", Name: "no-plaintext-passwords", ConstraintType: "must", Confidence: 0.9},
	}
	llmResult := &llmEvalResult{
		Violations: []llmViolation{
			{ConstraintNodeID: "c-001", Description: "Stores password in plaintext", Rationale: "diff adds password = value"},
		},
		Warnings: []llmViolation{},
	}

	resp := buildResponse(llmResult, constraints)

	if resp.Status != "Block" {
		t.Errorf("expected status Block, got %q", resp.Status)
	}
	if len(resp.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(resp.Violations))
	}
	if resp.Violations[0].ConstraintNodeID != "c-001" {
		t.Errorf("unexpected violation node_id: %q", resp.Violations[0].ConstraintNodeID)
	}
	if len(resp.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(resp.Warnings))
	}
}

// must_not constraint violation → Block status
func TestBuildResponse_MustNotViolation_Block(t *testing.T) {
	constraints := []constraintMatch{
		{NodeID: "c-002", Name: "no-debug-endpoints", ConstraintType: "must_not", Confidence: 0.85},
	}
	llmResult := &llmEvalResult{
		Violations: []llmViolation{
			{ConstraintNodeID: "c-002", Description: "Adds debug endpoint", Rationale: "route /debug added"},
		},
		Warnings: []llmViolation{},
	}

	resp := buildResponse(llmResult, constraints)

	if resp.Status != "Block" {
		t.Errorf("expected status Block, got %q", resp.Status)
	}
	if len(resp.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(resp.Violations))
	}
}

// should constraint violation from LLM → demoted to Warning
func TestBuildResponse_ShouldViolation_DemotedToWarning(t *testing.T) {
	constraints := []constraintMatch{
		{NodeID: "c-003", Name: "prefer-structured-logs", ConstraintType: "should", Confidence: 0.7},
	}
	// LLM incorrectly puts a "should" constraint in violations.
	llmResult := &llmEvalResult{
		Violations: []llmViolation{
			{ConstraintNodeID: "c-003", Description: "Using fmt.Println instead of structured logger", Rationale: "saw fmt.Println"},
		},
		Warnings: []llmViolation{},
	}

	resp := buildResponse(llmResult, constraints)

	if resp.Status != "Warning" {
		t.Errorf("expected status Warning (demoted from violation), got %q", resp.Status)
	}
	if len(resp.Violations) != 0 {
		t.Errorf("expected 0 violations (should was demoted), got %d: %v", len(resp.Violations), resp.Violations)
	}
	if len(resp.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(resp.Warnings))
	}
	// Demotion rationale annotation must be present.
	if !strings.Contains(resp.Warnings[0].Rationale, "demoted") {
		t.Errorf("expected 'demoted' in rationale, got %q", resp.Warnings[0].Rationale)
	}
}

// should_not constraint — also demoted
func TestBuildResponse_ShouldNotViolation_DemotedToWarning(t *testing.T) {
	constraints := []constraintMatch{
		{NodeID: "c-004", Name: "avoid-globals", ConstraintType: "should_not", Confidence: 0.6},
	}
	llmResult := &llmEvalResult{
		Violations: []llmViolation{
			{ConstraintNodeID: "c-004", Description: "Added global variable", Rationale: "var globalState"},
		},
		Warnings: []llmViolation{},
	}

	resp := buildResponse(llmResult, constraints)

	if resp.Status != "Warning" {
		t.Errorf("expected Warning, got %q", resp.Status)
	}
	if len(resp.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(resp.Warnings))
	}
}

// mixed: must violation + should warning → Block
func TestBuildResponse_Mixed_MustViolationAndShouldWarning_Block(t *testing.T) {
	constraints := []constraintMatch{
		{NodeID: "c-010", ConstraintType: "must", Confidence: 0.9},
		{NodeID: "c-011", ConstraintType: "should", Confidence: 0.7},
	}
	llmResult := &llmEvalResult{
		Violations: []llmViolation{
			{ConstraintNodeID: "c-010", Description: "Must violation", Rationale: "clear evidence"},
		},
		Warnings: []llmViolation{
			{ConstraintNodeID: "c-011", Description: "Should warning", Rationale: "minor deviation"},
		},
	}

	resp := buildResponse(llmResult, constraints)

	if resp.Status != "Block" {
		t.Errorf("expected Block, got %q", resp.Status)
	}
	if len(resp.Violations) != 1 {
		t.Errorf("expected 1 violation, got %d", len(resp.Violations))
	}
	if len(resp.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(resp.Warnings))
	}
}

// empty violations and warnings → Pass
func TestBuildResponse_Empty_Pass(t *testing.T) {
	constraints := []constraintMatch{
		{NodeID: "c-020", ConstraintType: "must"},
	}
	llmResult := &llmEvalResult{
		Violations: []llmViolation{},
		Warnings:   []llmViolation{},
	}

	resp := buildResponse(llmResult, constraints)

	if resp.Status != "Pass" {
		t.Errorf("expected Pass, got %q", resp.Status)
	}
	if len(resp.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(resp.Violations))
	}
	if len(resp.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(resp.Warnings))
	}
}

// Non-nil slices on Pass
func TestBuildResponse_Empty_NonNilSlices(t *testing.T) {
	resp := buildResponse(&llmEvalResult{
		Violations: []llmViolation{},
		Warnings:   []llmViolation{},
	}, []constraintMatch{})

	if resp.Violations == nil {
		t.Error("Violations must not be nil")
	}
	if resp.Warnings == nil {
		t.Error("Warnings must not be nil")
	}
}

// Unknown constraint_node_id from LLM → treated as warning
func TestBuildResponse_UnknownConstraintID_DemotedToWarning(t *testing.T) {
	constraints := []constraintMatch{
		{NodeID: "c-real", ConstraintType: "must"},
	}
	llmResult := &llmEvalResult{
		Violations: []llmViolation{
			{ConstraintNodeID: "c-hallucinated", Description: "Hallucinated violation", Rationale: "some reason"},
		},
		Warnings: []llmViolation{},
	}

	resp := buildResponse(llmResult, constraints)

	if resp.Status != "Warning" {
		t.Errorf("expected Warning (unknown ID demoted), got %q", resp.Status)
	}
	if len(resp.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(resp.Violations))
	}
	if len(resp.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(resp.Warnings))
	}
	if !strings.Contains(resp.Warnings[0].Rationale, "constraint node not found") {
		t.Errorf("expected 'constraint node not found' in rationale, got %q", resp.Warnings[0].Rationale)
	}
}

// LLM warnings are always kept as warnings, regardless of constraint type.
func TestBuildResponse_LLMWarnings_StayWarnings(t *testing.T) {
	constraints := []constraintMatch{
		{NodeID: "c-030", ConstraintType: "must"},
	}
	llmResult := &llmEvalResult{
		Violations: []llmViolation{},
		Warnings: []llmViolation{
			{ConstraintNodeID: "c-030", Description: "A warning about a must constraint", Rationale: "marginal"},
		},
	}

	resp := buildResponse(llmResult, constraints)

	if resp.Status != "Warning" {
		t.Errorf("expected Warning, got %q", resp.Status)
	}
	if len(resp.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(resp.Warnings))
	}
	if len(resp.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(resp.Violations))
	}
}

// deadline constraint violation demoted to warning
func TestBuildResponse_DeadlineConstraint_DemotedToWarning(t *testing.T) {
	constraints := []constraintMatch{
		{NodeID: "c-040", ConstraintType: "deadline", Confidence: 0.8},
	}
	llmResult := &llmEvalResult{
		Violations: []llmViolation{
			{ConstraintNodeID: "c-040", Description: "Ignores deadline", Rationale: "deadline check removed"},
		},
		Warnings: []llmViolation{},
	}

	resp := buildResponse(llmResult, constraints)

	if resp.Status != "Warning" {
		t.Errorf("expected Warning for deadline demotion, got %q", resp.Status)
	}
	if len(resp.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(resp.Violations))
	}
}

// ============================================================
// Validate method tests
// ============================================================

// Disabled config → returns Pass immediately, no LLM call needed.
func TestValidate_Disabled_ReturnsPass(t *testing.T) {
	svc := &GuardrailService{
		cfg: GuardrailConfig{
			Enabled: false,
		},
		// driver, embedder, cbRegistry intentionally nil — must not be touched
	}

	resp, err := svc.Validate(context.Background(), ValidateRequest{
		SpaceID:      "test-space",
		FilesChanged: []string{"main.go"},
		Diff:         "+func NewFeature() {}",
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Status != "Pass" {
		t.Errorf("expected Pass, got %q", resp.Status)
	}
	if len(resp.Violations) != 0 {
		t.Errorf("expected 0 violations, got %d", len(resp.Violations))
	}
	if len(resp.Warnings) != 0 {
		t.Errorf("expected 0 warnings, got %d", len(resp.Warnings))
	}
}

// Disabled config returns non-nil slices (JSON-serialisable).
func TestValidate_Disabled_NonNilSlices(t *testing.T) {
	svc := &GuardrailService{cfg: GuardrailConfig{Enabled: false}}

	resp, err := svc.Validate(context.Background(), ValidateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Violations == nil {
		t.Error("Violations must not be nil")
	}
	if resp.Warnings == nil {
		t.Error("Warnings must not be nil")
	}
}

// ============================================================
// cleanJSONResponse tests
// ============================================================

func TestCleanJSONResponse_BacktickJsonFence(t *testing.T) {
	raw := "```json\n{\"violations\": [], \"warnings\": []}\n```"
	got := cleanJSONResponse(raw)
	want := `{"violations": [], "warnings": []}`
	if got != want {
		t.Errorf("cleanJSONResponse mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

func TestCleanJSONResponse_PlainBacktickFence(t *testing.T) {
	raw := "```\n{\"violations\": []}\n```"
	got := cleanJSONResponse(raw)
	want := `{"violations": []}`
	if got != want {
		t.Errorf("cleanJSONResponse mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

func TestCleanJSONResponse_NoFence_NoOp(t *testing.T) {
	raw := `{"violations": [], "warnings": []}`
	got := cleanJSONResponse(raw)
	if got != raw {
		t.Errorf("cleanJSONResponse should be a no-op for clean JSON\ngot:  %q\nwant: %q", got, raw)
	}
}

func TestCleanJSONResponse_LeadingTrailingWhitespace(t *testing.T) {
	raw := "  \n{\"violations\": []}\n  "
	got := cleanJSONResponse(raw)
	want := `{"violations": []}`
	if got != want {
		t.Errorf("expected whitespace trimmed, got %q", got)
	}
}

func TestCleanJSONResponse_EmptyString(t *testing.T) {
	got := cleanJSONResponse("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestCleanJSONResponse_TrailingBrace(t *testing.T) {
	raw := `{"violations":[{"constraint_node_id":"9","description":"doc missing","rationale":"no docs"}],"warnings":[]}}`
	got := cleanJSONResponse(raw)
	want := `{"violations":[{"constraint_node_id":"9","description":"doc missing","rationale":"no docs"}],"warnings":[]}`
	if got != want {
		t.Errorf("trailing brace not stripped\ngot:  %q\nwant: %q", got, want)
	}
}

func TestCleanJSONResponse_TrailingText(t *testing.T) {
	raw := `{"violations":[],"warnings":[]} I hope this helps!`
	got := cleanJSONResponse(raw)
	want := `{"violations":[],"warnings":[]}`
	if got != want {
		t.Errorf("trailing text not stripped\ngot:  %q\nwant: %q", got, want)
	}
}

func TestCleanJSONResponse_LeadingText(t *testing.T) {
	raw := `Here is my analysis: {"violations":[],"warnings":[]}`
	got := cleanJSONResponse(raw)
	want := `{"violations":[],"warnings":[]}`
	if got != want {
		t.Errorf("leading text not stripped\ngot:  %q\nwant: %q", got, want)
	}
}

func TestCleanJSONResponse_FenceWithTrailingBrace(t *testing.T) {
	raw := "```json\n{\"violations\": [], \"warnings\": []}}\n```"
	got := cleanJSONResponse(raw)
	want := `{"violations": [], "warnings": []}`
	if got != want {
		t.Errorf("fenced JSON with trailing brace\ngot:  %q\nwant: %q", got, want)
	}
}

// ============================================================
// truncateString tests
// ============================================================

func TestTruncateString_NoTruncation(t *testing.T) {
	s := "hello"
	got := truncateString(s, 10)
	if got != s {
		t.Errorf("expected no truncation, got %q", got)
	}
}

func TestTruncateString_ExactLength(t *testing.T) {
	s := "hello"
	got := truncateString(s, 5)
	if got != s {
		t.Errorf("expected exact match, got %q", got)
	}
}

func TestTruncateString_Truncation(t *testing.T) {
	s := "hello world"
	got := truncateString(s, 8)
	want := "hello..."
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestTruncateString_TruncationAppendsDots(t *testing.T) {
	s := strings.Repeat("a", 100)
	got := truncateString(s, 10)
	if len(got) != 10 {
		t.Errorf("expected length 10, got %d", len(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected '...' suffix, got %q", got)
	}
}

func TestTruncateString_VeryShortMaxLen(t *testing.T) {
	// maxLen < 4: no "..." appended, just raw truncation.
	s := "hello"
	got := truncateString(s, 3)
	if len(got) != 3 {
		t.Errorf("expected length 3, got %d", len(got))
	}
}

func TestTruncateString_EmptyString(t *testing.T) {
	got := truncateString("", 10)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// ============================================================
// buildEvalPrompt tests
// ============================================================

func TestBuildEvalPrompt_IncludesConstraintDetails(t *testing.T) {
	constraints := []constraintMatch{
		{
			NodeID:         "c-100",
			Name:           "no-raw-sql",
			ConstraintType: "must_not",
			Content:        "Never use raw SQL strings; always use parameterised queries.",
			Confidence:     0.95,
		},
	}
	diffCtx := DiffContext{
		FilePaths:     []string{"db/query.go"},
		FunctionNames: []string{"GetUser"},
		AddedLines:    []string{`db.Exec("SELECT * FROM users WHERE id = " + id)`},
		RemovedLines:  []string{},
		ImportPaths:   []string{},
		Keywords:      []string{},
	}

	prompt := buildEvalPrompt(diffCtx, constraints, false)

	checks := []struct {
		label string
		want  string
	}{
		{"node_id", "c-100"},
		{"constraint type", "must_not"},
		{"constraint name", "no-raw-sql"},
		{"constraint content", "parameterised queries"},
		{"file path", "db/query.go"},
		{"function name", "GetUser"},
	}
	for _, c := range checks {
		if !strings.Contains(prompt, c.want) {
			t.Errorf("prompt missing %s (%q)", c.label, c.want)
		}
	}
}

func TestBuildEvalPrompt_TruncatesDiffTo3000(t *testing.T) {
	// Build a diffCtx with many added lines to force a long rebuilt diff.
	added := make([]string, 200)
	for i := range added {
		added[i] = strings.Repeat("x", 30)
	}

	diffCtx := DiffContext{
		AddedLines:    added,
		RemovedLines:  []string{},
		FunctionNames: []string{},
		ImportPaths:   []string{},
		FilePaths:     []string{},
		Keywords:      []string{},
	}

	constraints := []constraintMatch{
		{NodeID: "c-999", ConstraintType: "must"},
	}

	prompt := buildEvalPrompt(diffCtx, constraints, false)

	// The diff section between the ``` fences should be <= 3000 chars + "...".
	// We verify this by checking that the total prompt length is bounded
	// (rather than parsing the prompt structure exactly).
	// The rebuilt diff alone is 200*(30+1) = 6200 chars; after truncation it must be <= 3003.
	diffStart := strings.Index(prompt, "```diff\n")
	diffEnd := strings.LastIndex(prompt, "\n```")
	if diffStart < 0 || diffEnd < 0 {
		t.Fatal("prompt does not contain expected ```diff block")
	}
	diffContent := prompt[diffStart+8 : diffEnd]
	if len(diffContent) > 3003 { // 3000 + "..."
		t.Errorf("diff section in prompt exceeds 3000 chars: len=%d", len(diffContent))
	}
}

func TestBuildEvalPrompt_MultipleConstraints(t *testing.T) {
	constraints := []constraintMatch{
		{NodeID: "c-A", ConstraintType: "must", Content: "Always add tests."},
		{NodeID: "c-B", ConstraintType: "should_not", Content: "Avoid fmt.Println."},
	}
	diffCtx := DiffContext{
		AddedLines:   []string{},
		RemovedLines: []string{},
	}

	prompt := buildEvalPrompt(diffCtx, constraints, false)

	if !strings.Contains(prompt, "c-A") {
		t.Error("prompt missing constraint c-A")
	}
	if !strings.Contains(prompt, "c-B") {
		t.Error("prompt missing constraint c-B")
	}
	if !strings.Contains(prompt, "[1]") {
		t.Error("prompt missing numbered constraint index [1]")
	}
	if !strings.Contains(prompt, "[2]") {
		t.Error("prompt missing numbered constraint index [2]")
	}
}

// ============================================================
// collectKeywords tests
// ============================================================

func TestCollectKeywords_Deduplication(t *testing.T) {
	ctx := DiffContext{
		FunctionNames: []string{"Login", "login", "Login"}, // "login" deduplicates
		ImportPaths:   []string{},
		Keywords:      []string{"Login"},
		FilePaths:     []string{},
	}

	keywords := collectKeywords(ctx)

	count := 0
	for _, kw := range keywords {
		if kw == "login" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected 'login' exactly once after dedup, got %d (all: %v)", count, keywords)
	}
}

func TestCollectKeywords_MinimumLength3(t *testing.T) {
	ctx := DiffContext{
		FunctionNames: []string{"ok", "ab", "x", "foo", "go"},
		ImportPaths:   []string{},
		Keywords:      []string{"ab"},
		FilePaths:     []string{},
	}

	keywords := collectKeywords(ctx)

	for _, kw := range keywords {
		if len(kw) < 3 {
			t.Errorf("keyword %q is shorter than minimum length 3", kw)
		}
	}

	found := false
	for _, kw := range keywords {
		if kw == "foo" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'foo' (len 3) to be included, got %v", keywords)
	}
}

func TestCollectKeywords_LowercasedOutput(t *testing.T) {
	ctx := DiffContext{
		FunctionNames: []string{"MyHandler", "ProcessRequest"},
		ImportPaths:   []string{},
		Keywords:      []string{},
		FilePaths:     []string{},
	}

	keywords := collectKeywords(ctx)

	for _, kw := range keywords {
		if kw != strings.ToLower(kw) {
			t.Errorf("keyword %q is not lowercase", kw)
		}
	}
}

func TestCollectKeywords_FileBasenamesIncluded(t *testing.T) {
	ctx := DiffContext{
		FunctionNames: []string{},
		ImportPaths:   []string{},
		Keywords:      []string{},
		FilePaths:     []string{"internal/auth/handler.go", "pkg/config.yaml"},
	}

	keywords := collectKeywords(ctx)

	want := map[string]bool{"handler.go": true, "config.yaml": true}
	for _, kw := range keywords {
		delete(want, kw)
	}
	if len(want) != 0 {
		t.Errorf("expected file basenames in keywords, missing: %v (got %v)", want, keywords)
	}
}

func TestCollectKeywords_EmptyContext(t *testing.T) {
	ctx := DiffContext{
		FunctionNames: []string{},
		ImportPaths:   []string{},
		Keywords:      []string{},
		FilePaths:     []string{},
	}

	keywords := collectKeywords(ctx)

	if len(keywords) != 0 {
		t.Errorf("expected empty keywords, got %v", keywords)
	}
}

func TestCollectKeywords_ImportPathsIncluded(t *testing.T) {
	ctx := DiffContext{
		FunctionNames: []string{},
		ImportPaths:   []string{"path/to/pkg", "fmt"},
		Keywords:      []string{},
		FilePaths:     []string{},
	}

	keywords := collectKeywords(ctx)

	// "fmt" is 3 chars — should be included. "path/to/pkg" is also included.
	want := map[string]bool{"path/to/pkg": true, "fmt": true}
	for _, kw := range keywords {
		delete(want, kw)
	}
	if len(want) != 0 {
		t.Errorf("expected import paths in keywords, missing: %v (got %v)", want, keywords)
	}
}

// ============================================================
// FmtConstraintID helper tests
// ============================================================

func TestFmtConstraintID_WithName(t *testing.T) {
	got := FmtConstraintID("c-001", "no-raw-sql")
	want := "no-raw-sql (c-001)"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestFmtConstraintID_WithoutName(t *testing.T) {
	got := FmtConstraintID("c-001", "")
	want := "c-001"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

// ============================================================
// isBlockingType tests
// ============================================================

func TestIsBlockingType(t *testing.T) {
	tests := []struct {
		constraintType string
		want           bool
	}{
		{"must", true},
		{"must_not", true},
		{"should", false},
		{"should_not", false},
		{"deadline", false},
		{"", false},
		{"MUST", false}, // case-sensitive
	}
	for _, tt := range tests {
		t.Run(tt.constraintType, func(t *testing.T) {
			got := isBlockingType(tt.constraintType)
			if got != tt.want {
				t.Errorf("isBlockingType(%q) = %v, want %v", tt.constraintType, got, tt.want)
			}
		})
	}
}

// --- asString / asFloat64 tests ---

func TestAsString(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{nil, ""},
		{"hello", "hello"},
		{42, "42"},
		{3.14, "3.14"},
		{true, "true"},
		{[]int{1, 2}, "[1 2]"},
	}
	for _, tt := range tests {
		got := asString(tt.input)
		if got != tt.want {
			t.Errorf("asString(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAsFloat64(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  float64
	}{
		{"nil", nil, 0.0},
		{"float64", float64(3.14), 3.14},
		{"float32", float32(2.5), 2.5},
		{"int64", int64(42), 42.0},
		{"int", int(7), 7.0},
		{"string", "not a number", 0.0},
		{"bool", true, 0.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := asFloat64(tt.input)
			if got != tt.want {
				t.Errorf("asFloat64(%v) = %f, want %f", tt.input, got, tt.want)
			}
		})
	}
}

// --- buildConstraintMap tests ---

func TestBuildConstraintMap(t *testing.T) {
	constraints := []constraintMatch{
		{NodeID: "c1", Name: "auth-required", ConstraintType: "must"},
		{NodeID: "c2", Name: "no-eval", ConstraintType: "must_not"},
	}

	m := buildConstraintMap(constraints)
	if len(m) != 2 {
		t.Errorf("expected 2 entries, got %d", len(m))
	}
	if m["c1"].Name != "auth-required" {
		t.Errorf("c1 name = %q, want %q", m["c1"].Name, "auth-required")
	}
	if m["c2"].ConstraintType != "must_not" {
		t.Errorf("c2 type = %q, want %q", m["c2"].ConstraintType, "must_not")
	}
}

func TestBuildConstraintMap_Empty(t *testing.T) {
	m := buildConstraintMap(nil)
	if len(m) != 0 {
		t.Errorf("expected 0 entries, got %d", len(m))
	}
}

func TestBuildConstraintMap_DuplicateNodeID(t *testing.T) {
	constraints := []constraintMatch{
		{NodeID: "c1", Name: "first"},
		{NodeID: "c1", Name: "second"},
	}
	m := buildConstraintMap(constraints)
	// Last one wins
	if m["c1"].Name != "second" {
		t.Errorf("duplicate NodeID should use last value, got %q", m["c1"].Name)
	}
}

// --- NewGuardrailService tests ---

func TestNewGuardrailService(t *testing.T) {
	cfg := GuardrailConfig{
		Enabled:   true,
		Provider:  "openai",
		Model:     "gpt-4o",
		MaxTokens: 2048,
		TimeoutMs: 5000,
	}
	svc := NewGuardrailService(cfg, nil, nil, nil, nil)
	if svc == nil {
		t.Fatal("NewGuardrailService returned nil")
	}
	if !svc.cfg.Enabled {
		t.Error("expected cfg.Enabled to be true")
	}
	if svc.cfg.Provider != "openai" {
		t.Errorf("expected Provider %q, got %q", "openai", svc.cfg.Provider)
	}
	if svc.cfg.MaxTokens != 2048 {
		t.Errorf("expected MaxTokens 2048, got %d", svc.cfg.MaxTokens)
	}
}

func TestNewGuardrailService_DisabledConfig(t *testing.T) {
	svc := NewGuardrailService(GuardrailConfig{Enabled: false}, nil, nil, nil, nil)
	if svc == nil {
		t.Fatal("NewGuardrailService returned nil")
	}
	if svc.cfg.Enabled {
		t.Error("expected cfg.Enabled to be false")
	}
}

// --- buildDiffSummary direct tests ---

func TestBuildDiffSummary_Empty(t *testing.T) {
	ctx := DiffContext{}
	got := buildDiffSummary(ctx)
	if got != "" {
		t.Errorf("expected empty summary, got %q", got)
	}
}

func TestBuildDiffSummary_FilesOnly(t *testing.T) {
	ctx := DiffContext{FilePaths: []string{"main.go", "handler.go"}}
	got := buildDiffSummary(ctx)
	if !strings.Contains(got, "Files: main.go, handler.go") {
		t.Errorf("expected Files line, got %q", got)
	}
}

func TestBuildDiffSummary_FunctionsOnly(t *testing.T) {
	ctx := DiffContext{FunctionNames: []string{"Foo", "Bar"}}
	got := buildDiffSummary(ctx)
	if !strings.Contains(got, "Functions: Foo, Bar") {
		t.Errorf("expected Functions line, got %q", got)
	}
}

func TestBuildDiffSummary_ImportsOnly(t *testing.T) {
	ctx := DiffContext{ImportPaths: []string{"fmt", "os"}}
	got := buildDiffSummary(ctx)
	if !strings.Contains(got, "Imports: fmt, os") {
		t.Errorf("expected Imports line, got %q", got)
	}
}

func TestBuildDiffSummary_AddedLines(t *testing.T) {
	ctx := DiffContext{AddedLines: []string{"line1", "line2"}}
	got := buildDiffSummary(ctx)
	if !strings.Contains(got, "Added:") {
		t.Errorf("expected Added: header, got %q", got)
	}
	if !strings.Contains(got, "  + line1") {
		t.Errorf("expected '  + line1', got %q", got)
	}
	if !strings.Contains(got, "  + line2") {
		t.Errorf("expected '  + line2', got %q", got)
	}
}

func TestBuildDiffSummary_AddedLinesMax20(t *testing.T) {
	lines := make([]string, 30)
	for i := range lines {
		lines[i] = "line"
	}
	ctx := DiffContext{AddedLines: lines}
	got := buildDiffSummary(ctx)
	// Should only include 20 "  + line" entries
	count := strings.Count(got, "  + line")
	if count != 20 {
		t.Errorf("expected 20 added lines in summary, got %d", count)
	}
}

func TestBuildDiffSummary_Truncation(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = strings.Repeat("x", 120)
	}
	ctx := DiffContext{AddedLines: lines}
	got := buildDiffSummary(ctx)
	if len(got) > 2000 {
		t.Errorf("summary not truncated to 2000 chars: len=%d", len(got))
	}
}

func TestBuildDiffSummary_AllSections(t *testing.T) {
	ctx := DiffContext{
		FilePaths:     []string{"a.go"},
		FunctionNames: []string{"Init"},
		ImportPaths:   []string{"fmt"},
		AddedLines:    []string{"return nil"},
	}
	got := buildDiffSummary(ctx)
	if !strings.Contains(got, "Files:") {
		t.Error("missing Files section")
	}
	if !strings.Contains(got, "Functions:") {
		t.Error("missing Functions section")
	}
	if !strings.Contains(got, "Imports:") {
		t.Error("missing Imports section")
	}
	if !strings.Contains(got, "Added:") {
		t.Error("missing Added section")
	}
}

// --- rebuildDiff tests ---

func TestRebuildDiff(t *testing.T) {
	ctx := DiffContext{
		RemovedLines: []string{"old line 1", "old line 2"},
		AddedLines:   []string{"new line 1"},
	}

	got := rebuildDiff(ctx)
	want := "-old line 1\n-old line 2\n+new line 1\n"
	if got != want {
		t.Errorf("rebuildDiff = %q, want %q", got, want)
	}
}

func TestRebuildDiff_Empty(t *testing.T) {
	got := rebuildDiff(DiffContext{})
	if got != "" {
		t.Errorf("rebuildDiff(empty) = %q, want empty", got)
	}
}

func TestRebuildDiff_OnlyAdded(t *testing.T) {
	ctx := DiffContext{AddedLines: []string{"added"}}
	got := rebuildDiff(ctx)
	if got != "+added\n" {
		t.Errorf("rebuildDiff = %q, want %q", got, "+added\n")
	}
}

func TestRebuildDiff_OnlyRemoved(t *testing.T) {
	ctx := DiffContext{RemovedLines: []string{"removed"}}
	got := rebuildDiff(ctx)
	if got != "-removed\n" {
		t.Errorf("rebuildDiff = %q, want %q", got, "-removed\n")
	}
}

// ============================================================
// buildEvalPrompt compressed mode tests
// ============================================================

func TestBuildEvalPrompt_Compressed(t *testing.T) {
	diffCtx := DiffContext{
		FilePaths:  []string{"main.go"},
		AddedLines: []string{"fmt.Println(secret)"},
	}
	constraints := []constraintMatch{
		{NodeID: "c-1", Name: "no-secrets", ConstraintType: "must_not", Content: "Never log secrets", Confidence: 0.92},
	}

	prompt := buildEvalPrompt(diffCtx, constraints, true)

	// Compressed mode uses pipe-separated format with conf:
	if !strings.Contains(prompt, "|") {
		t.Error("compressed prompt should use pipe-separated format")
	}
	if !strings.Contains(prompt, "conf:") {
		t.Error("compressed prompt should include 'conf:' confidence label")
	}
}

func TestBuildEvalPrompt_Compressed_ShorterOutput(t *testing.T) {
	diffCtx := DiffContext{
		FilePaths:     []string{"auth.go", "handler.go"},
		FunctionNames: []string{"Login", "Validate"},
		AddedLines:    []string{"password := input", "db.Exec(query)"},
		RemovedLines:  []string{"// old code"},
	}
	constraints := []constraintMatch{
		{NodeID: "c-1", Name: "no-raw-sql", ConstraintType: "must_not", Content: "Never use raw SQL strings; always use parameterised queries.", Confidence: 0.95},
		{NodeID: "c-2", Name: "prefer-structured-logs", ConstraintType: "should", Content: "Use structured logging instead of fmt.Println for all log output.", Confidence: 0.7},
	}

	compressed := buildEvalPrompt(diffCtx, constraints, true)
	uncompressed := buildEvalPrompt(diffCtx, constraints, false)

	if len(compressed) >= len(uncompressed) {
		t.Errorf("compressed prompt (%d chars) should be shorter than uncompressed (%d chars)", len(compressed), len(uncompressed))
	}
}

func TestBuildEvalPrompt_Compressed_DiffVerbatim(t *testing.T) {
	diffCtx := DiffContext{
		AddedLines:   []string{"+password := getInput()"},
		RemovedLines: []string{"-// placeholder"},
	}
	constraints := []constraintMatch{
		{NodeID: "c-1", ConstraintType: "must", Content: "test constraint", Confidence: 0.8},
	}

	compressed := buildEvalPrompt(diffCtx, constraints, true)
	uncompressed := buildEvalPrompt(diffCtx, constraints, false)

	// Extract the diff block from both prompts — should be identical
	extractDiffBlock := func(prompt string) string {
		start := strings.Index(prompt, "```diff\n")
		end := strings.LastIndex(prompt, "\n```")
		if start < 0 || end < 0 {
			return ""
		}
		return prompt[start : end+4]
	}

	compressedDiff := extractDiffBlock(compressed)
	uncompressedDiff := extractDiffBlock(uncompressed)

	if compressedDiff == "" {
		t.Fatal("compressed prompt should contain a ```diff block")
	}
	if compressedDiff != uncompressedDiff {
		t.Errorf("diff block should be identical in both modes.\ncompressed:   %q\nuncompressed: %q", compressedDiff, uncompressedDiff)
	}
}

// ============================================================
// scopeClause tests (pure — no Neo4j)
// ============================================================

// When ConstraintScopeFilteringEnabled is false, scopeClause must return "".
func TestScopeClause_Disabled(t *testing.T) {
	svc := &GuardrailService{
		cfg: GuardrailConfig{ConstraintScopeFilteringEnabled: false},
	}
	got := svc.scopeClause()
	if got != "" {
		t.Errorf("expected empty string when scope filtering disabled, got %q", got)
	}
}

// When ConstraintScopeFilteringEnabled is true, the clause must contain the
// regex match condition used in the Cypher WHERE fragment.
func TestScopeClause_Enabled(t *testing.T) {
	svc := &GuardrailService{
		cfg: GuardrailConfig{ConstraintScopeFilteringEnabled: true},
	}
	got := svc.scopeClause()
	if got == "" {
		t.Fatal("expected non-empty scope clause when scope filtering enabled")
	}
	if !strings.Contains(got, "$filePath =~ c.scope") {
		t.Errorf("scope clause missing '$filePath =~ c.scope', got %q", got)
	}
}

// ============================================================
// authorityClause tests (pure — no Neo4j)
// ============================================================

// When ConstraintAuthorityEnabled is false, authorityClause must return ""
// for every trust level.
func TestAuthorityClause_Disabled(t *testing.T) {
	svc := &GuardrailService{
		cfg: GuardrailConfig{ConstraintAuthorityEnabled: false},
	}
	for _, level := range []string{"restricted", "standard", "elevated", "", "unknown"} {
		got := svc.authorityClause(level)
		if got != "" {
			t.Errorf("authorityClause(%q) with authority disabled: expected \"\", got %q", level, got)
		}
	}
}

// "standard" trust level must include both org_policy and team_standard.
func TestAuthorityClause_Standard(t *testing.T) {
	svc := &GuardrailService{
		cfg: GuardrailConfig{ConstraintAuthorityEnabled: true},
	}
	got := svc.authorityClause("standard")
	if got == "" {
		t.Fatal("expected non-empty clause for standard trust level")
	}
	if !strings.Contains(got, "org_policy") {
		t.Errorf("standard clause missing 'org_policy', got %q", got)
	}
	if !strings.Contains(got, "team_standard") {
		t.Errorf("standard clause missing 'team_standard', got %q", got)
	}
}

// "elevated" trust level must filter for org_policy only (equality, not IN list).
// team_standard may appear as a coalesce default but must NOT appear in the
// permitted-authority list.
func TestAuthorityClause_Elevated(t *testing.T) {
	svc := &GuardrailService{
		cfg: GuardrailConfig{ConstraintAuthorityEnabled: true},
	}
	got := svc.authorityClause("elevated")
	if got == "" {
		t.Fatal("expected non-empty clause for elevated trust level")
	}
	if !strings.Contains(got, "org_policy") {
		t.Errorf("elevated clause missing 'org_policy', got %q", got)
	}
	// Elevated uses = 'org_policy' (single equality), not an IN list with team_standard.
	if strings.Contains(got, "IN [") {
		t.Errorf("elevated clause must use equality (= 'org_policy'), not an IN list, got %q", got)
	}
	if !strings.Contains(got, "= 'org_policy'") {
		t.Errorf("elevated clause must use equality = 'org_policy', got %q", got)
	}
}

// "restricted" trust level (sees all constraints) must return "" even when
// authority filtering is enabled.
func TestAuthorityClause_Restricted(t *testing.T) {
	svc := &GuardrailService{
		cfg: GuardrailConfig{ConstraintAuthorityEnabled: true},
	}
	got := svc.authorityClause("restricted")
	if got != "" {
		t.Errorf("restricted trust level should return \"\", got %q", got)
	}
}

// ============================================================
// primaryFilePath tests (pure — no Neo4j)
// ============================================================

func TestPrimaryFilePath(t *testing.T) {
	tests := []struct {
		name     string
		diffCtx  DiffContext
		wantPath string
	}{
		{
			name:     "first element returned",
			diffCtx:  DiffContext{FilePaths: []string{"internal/auth/handler.go", "pkg/config.go"}},
			wantPath: "internal/auth/handler.go",
		},
		{
			name:     "single element",
			diffCtx:  DiffContext{FilePaths: []string{"main.go"}},
			wantPath: "main.go",
		},
		{
			name:     "empty slice returns empty string",
			diffCtx:  DiffContext{FilePaths: []string{}},
			wantPath: "",
		},
		{
			name:     "nil slice returns empty string",
			diffCtx:  DiffContext{FilePaths: nil},
			wantPath: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := primaryFilePath(tt.diffCtx)
			if got != tt.wantPath {
				t.Errorf("primaryFilePath() = %q, want %q", got, tt.wantPath)
			}
		})
	}
}

// GUARDRAIL-CORRECTIONS-001: role set follows the flag; type coalesce caps
// corrections at Warning via isBlockingType.
func TestRetrievalRoleTypes_Flag(t *testing.T) {
	off := &GuardrailService{cfg: GuardrailConfig{}}
	if got := off.retrievalRoleTypes(); len(got) != 1 || got[0] != "constraint" {
		t.Errorf("flag off: got %v, want [constraint]", got)
	}
	on := &GuardrailService{cfg: GuardrailConfig{IncludeCorrections: true}}
	if got := on.retrievalRoleTypes(); len(got) != 2 || got[1] != "correction" {
		t.Errorf("flag on: got %v, want [constraint correction]", got)
	}
	if isBlockingType("correction") {
		t.Error("correction must NOT be a blocking type (Warning tier)")
	}
}
