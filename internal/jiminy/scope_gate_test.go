// Sprint LEVER-C-TIGHTEN-002 (2026-08-12) — scope gate pin tests.

package jiminy

import (
	"testing"
)

func TestScopeGate_ItemWithGitScope_SuppressedOnDocEdit(t *testing.T) {
	// The concrete failure class from JIMINY-CEILING-BREAK-2:
	// `never-direct-main-commits` gets surfaced on doc edits (which are
	// NOT commits) because retrieval matches the "commit" token. Scope
	// gate should suppress this.
	item := GuidanceItem{
		Content: "CONSTRAINT: NEVER commit directly to main branch. All development work happens on dev branches.",
	}
	req := GuidanceRequest{
		Context:     "editing sprint plan documentation",
		AgentOutput: "Wrote a section on process flow in docs/development/plan.md",
		FilePath:    "docs/development/plan.md",
	}
	if scopeGateApplicable(item, req) {
		t.Fatalf("expected git-scope constraint to be suppressed on doc-editing request; scopeGateApplicable returned true (constraint would still surface)")
	}
}

func TestScopeGate_ItemWithGitScope_SurfacedOnCommit(t *testing.T) {
	// Same constraint, but request IS a commit action → surface.
	item := GuidanceItem{
		Content: "CONSTRAINT: NEVER commit directly to main branch.",
	}
	req := GuidanceRequest{
		Context:     "committing changes to reh3376_dev01",
		AgentOutput: "git commit -m 'feat: X' && git push",
	}
	if !scopeGateApplicable(item, req) {
		t.Fatalf("expected git-scope constraint to surface on git-commit request; scopeGateApplicable returned false (constraint suppressed)")
	}
}

func TestScopeGate_ItemWithoutScope_AlwaysSurfaces(t *testing.T) {
	// Constraint with no discernible action verbs → safe default = surface.
	item := GuidanceItem{
		Content: "User preference: prefer conversational tone in responses",
	}
	req := GuidanceRequest{
		Context:     "editing config file",
		AgentOutput: "sed -i 's/foo/bar/' config.yaml",
	}
	if !scopeGateApplicable(item, req) {
		t.Fatalf("expected no-scope constraint to surface universally; got suppressed")
	}
}

func TestScopeGate_EmptyRequestScope_SurfacesAll(t *testing.T) {
	// Request with no discernible scope tokens → surface everything
	// (safe default; prevents wholesale suppression when caller didn't
	// pass tool-context hints).
	item := GuidanceItem{
		Content: "CONSTRAINT: NEVER commit directly to main branch",
	}
	req := GuidanceRequest{
		Context: "asking a question",
	}
	if !scopeGateApplicable(item, req) {
		t.Fatalf("expected surface with no-scope request; got suppressed (safe-default violated)")
	}
}

func TestScopeGate_IdentifierConstraintOnBashCommand_Suppressed(t *testing.T) {
	// `must-use-cuid2` should NOT fire on a Bash command that doesn't
	// generate identifiers.
	item := GuidanceItem{
		Content: "[must] All unique identifiers in MDEMG must use CUIDv2 (nrednav/cuid2), NOT UUID v4",
	}
	req := GuidanceRequest{
		Context:     "restarting mdemg server",
		AgentOutput: "launchctl kickstart -k gui/501/com.mdemg.server",
	}
	// Both have "id" fragments but no cuid/uuid/identifier tokens in the
	// request; both have bash-family tokens. Item scope: identifier only.
	// Request scope: bash only. No intersect → suppress.
	if scopeGateApplicable(item, req) {
		t.Fatalf("expected identifier constraint to be suppressed on bash-restart request; scopeGateApplicable returned true")
	}
}

func TestScopeGate_IdentifierConstraintOnCodegen_Surfaced(t *testing.T) {
	item := GuidanceItem{
		Content: "[must] All unique identifiers in MDEMG must use CUIDv2 (nrednav/cuid2), NOT UUID v4",
	}
	req := GuidanceRequest{
		Context:     "writing a new struct that needs an identifier field",
		AgentOutput: "type Foo struct { ID string `json:\"id\"` } // where should this UUID come from?",
	}
	if !scopeGateApplicable(item, req) {
		t.Fatalf("expected identifier constraint to surface on codegen mentioning UUID; got suppressed")
	}
}

func TestApplyScopeGate_DisabledIsNoop(t *testing.T) {
	items := []GuidanceItem{
		{Content: "CONSTRAINT: NEVER commit to main"},
		{Content: "CONSTRAINT: use CUIDv2 identifiers"},
	}
	req := GuidanceRequest{Context: "editing doc"} // both should be dropped if enabled
	kept, drops := applyScopeGate(items, req, false)
	if len(kept) != 2 || drops != 0 {
		t.Fatalf("disabled gate should be no-op; got kept=%d drops=%d", len(kept), drops)
	}
}

func TestApplyScopeGate_DropsMatchExpectation(t *testing.T) {
	// Ceiling-data-shaped fixture: retrieval surfaces 4 constraints in
	// response to a doc-editing request; 3 have out-of-scope git/id
	// families and should be dropped, 1 has no discernible scope and
	// surfaces.
	items := []GuidanceItem{
		{Content: "CONSTRAINT: NEVER commit directly to main branch"},   // git → drop
		{Content: "[must] All identifiers must use CUIDv2, NOT UUID"},   // identifier → drop
		{Content: "CONSTRAINT: never modify schema without migration"},  // schema → drop
		{Content: "User preference: mermaid diagrams in markdown docs"}, // no discernible scope → keep
	}
	req := GuidanceRequest{
		Context:     "editing feature documentation",
		AgentOutput: "writing docs/features/new-thing.md",
		FilePath:    "docs/features/new-thing.md",
	}
	kept, drops := applyScopeGate(items, req, true)
	if drops != 3 {
		t.Fatalf("expected 3 drops (git, identifier, schema constraints), got %d", drops)
	}
	if len(kept) != 1 {
		t.Fatalf("expected 1 surviving item (no-scope preference), got %d", len(kept))
	}
	if kept[0].Content != items[3].Content {
		t.Fatalf("expected preference item to survive, got %q", kept[0].Content)
	}
}

func TestDeriveScopeFamilies_ClassifiesRepresentativeCorpus(t *testing.T) {
	// Direct check on representative canonical constraints from the
	// post-JIMINY-CORPUS-003 corpus.
	cases := []struct {
		content       string
		wantFamilies  []string
		wantEmpty     bool
	}{
		{"CONSTRAINT: NEVER commit directly to main branch", []string{"git"}, false},
		{"Never use git stash as a workaround for goreleaser", []string{"git"}, false},
		{"[must] All unique identifiers must use CUIDv2", []string{"identifier"}, false},
		{"Never modify production schemas without migration", []string{"schema"}, false},
		{"MANDATORY WORKFLOW: run e2e testing after new development", []string{"testing"}, false},
		{"MANDATORY PROCESS: feature docs after new feature", []string{"process_docs"}, false},
		{"User preference: mermaid diagrams in markdown", nil, true}, // no discernible action verb
	}
	for _, c := range cases {
		got := deriveScopeFamilies(c.content)
		if c.wantEmpty {
			if len(got) != 0 {
				t.Errorf("content=%q: expected empty scope, got %v", c.content, got)
			}
			continue
		}
		for _, w := range c.wantFamilies {
			if !got[w] {
				t.Errorf("content=%q: expected family %q in scope %v", c.content, w, got)
			}
		}
	}
}
