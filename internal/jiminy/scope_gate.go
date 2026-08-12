// Sprint LEVER-C-TIGHTEN-002 — scope gate for constraint surfacing.
//
// Phase 2 of JIMINY-CEILING-BREAK-2 arc. Addresses the ceiling data
// showing that follow rates on the top-25 canonical constraints hover
// at 10-20% because constraints get surfaced on retrieval queries that
// aren't the actual mechanism they govern (e.g. `never-direct-main-commits`
// surfaces on any query mentioning "commit" — including docs edits +
// analysis discussions ABOUT commits, not just Bash calls that ARE
// commits).
//
// Data (mdemg-dev, 7d ending 2026-08-12):
//   - never-direct-main-commits constraint: 57 events, 9 followed, 48 ignored → 15.8%
//   - never-direct-main-commits correction: 75 events, 11 followed, 62 ignored → 14.7%
//   - similarity distribution overlaps: followed p50=0.900, ignored p50=0.800
//     — similarity is NOT a discriminator, so raising sim_floor won't help.
//
// The lever that WILL help: suppress surfacing when the constraint's
// action-scope doesn't intersect the request's action surface. A
// commit-scoped rule shouldn't fire when the agent is editing a doc.
//
// Implementation: heuristic keyword-verb match. For each surfaced
// candidate, derive characteristic action-verbs from its content; check
// if any intersect the request's Context + AgentOutput + FilePath +
// Query. If the item has NO discernible action-verb scope, surface
// (safe default — constraints without a scope apply universally).
//
// Env flag: JIMINY_SCOPE_GATE_ENABLED (default false in code, true in
// .env after live smoke).

package jiminy

import (
	"strings"
)

// scopeVerbFamily maps a family label to the tokens that identify it.
// A constraint's content is scanned for these tokens; if any hit, the
// family (and its aliases) is added to the constraint's derived scope.
//
// Families intentionally overlap where domains blur (e.g. "commit"
// implies git-scope AND action-scope).
var scopeVerbFamily = map[string][]string{
	// Git operations — the biggest miss class per the ceiling data.
	"git": {
		"commit", "commits", "push", "pushes", "merge", "merged", "merging",
		"branch", "branches", "rebase", "rebased", "rebasing",
		"stash", "checkout", "cherry-pick", "cherrypick",
		"pull request", "pr comment", "auto-pr", "goreleaser",
	},
	// File-mutation ops. Note "modify" is intentionally EXCLUDED — it
	// appears too broadly (in schema/config/code rules alike) and would
	// make file_mutation fire on nearly every request, defeating the
	// scope-gate purpose. Prefer specific verbs (edit/write/delete/etc)
	// that unambiguously indicate a file-mutation action.
	"file_mutation": {
		"edit", "edited", "write", "wrote", "delete", "deleted",
		"remove", "removed", "rm -rf", "unlink",
		"hardcode", "hardcoded", "hardcoding",
	},
	// Shell / bash execution.
	"bash": {
		"bash", "shell", "exec", "docker compose", "docker run",
		"mdemg db", "kickstart", "launchctl",
	},
	// Database / schema.
	"schema": {
		"alter table", "drop table", "drop index", "drop column",
		"migrate", "migration", "schema", "cypher", "neo4j",
	},
	// Identifier / codegen.
	"identifier": {
		"uuid", "cuidv2", "cuid2", "identifier", "id generation",
		"randomuuid",
	},
	// Testing.
	"testing": {
		"test", "e2e", "unit test", "integration test", "lint",
		"golangci", "pytest",
	},
	// Sprint / process docs.
	"process_docs": {
		"sprint", "plan", "changelog", "feature doc", "12-section",
		"documents accessed",
	},
	// LLM / classifier config.
	"llm_config": {
		"llm", "classifier", "temperature", "max_tokens",
		"max_completion_tokens", "openai", "gpt-",
	},
	// CMS / memory.
	"cms": {
		"cms", "memory", "space_id", "mdemg-dev",
		"observe", "recall",
	},
}

// deriveScopeFamilies returns the set of family labels a piece of text
// implicates. A text with no matching tokens returns an empty set —
// callers should treat this as "no discernible scope; surface anywhere."
func deriveScopeFamilies(text string) map[string]bool {
	lc := strings.ToLower(text)
	families := make(map[string]bool)
	for fam, toks := range scopeVerbFamily {
		for _, t := range toks {
			if strings.Contains(lc, t) {
				families[fam] = true
				break
			}
		}
	}
	return families
}

// scopeGateApplicable returns true iff the given guidance item should be
// surfaced for the current agent action, per the scope gate heuristic.
// Returns true (surface) when:
//   - Gate is disabled (caller checks; this function assumes gate is on)
//   - Item content is empty or has no discernible scope families
//   - At least one derived family intersects the request-side scope
//
// Returns false (suppress) when the item has discernible scope families
// AND none intersect the request-side derived families.
//
// This is a heuristic: false negatives (suppressing something that would
// have been legitimately followed) are possible. The safe-defaults design
// (empty-scope-surfaces) bounds the failure mode to "suppress items with
// specific scope on requests with unrelated scope" — exactly the class
// causing the ~14% follow-rate ceiling.
func scopeGateApplicable(item GuidanceItem, req GuidanceRequest) bool {
	if item.Content == "" {
		return true
	}
	itemFams := deriveScopeFamilies(item.Content)
	if len(itemFams) == 0 {
		return true // no discernible scope — surface anywhere (safe default)
	}
	reqText := req.Context + " " + req.AgentOutput + " " + req.FilePath + " " + req.Query
	reqFams := deriveScopeFamilies(reqText)
	if len(reqFams) == 0 {
		// Request has no discernible scope either — surface (safe default;
		// prevents suppression when the caller didn't pass tool-context hints).
		return true
	}
	// Suppress iff no family intersects.
	for fam := range itemFams {
		if reqFams[fam] {
			return true
		}
	}
	return false
}

// applyScopeGate filters items in place, dropping those the scope gate
// judges non-applicable to the request. Returns the surviving items and
// a count of drops. When the gate is disabled, returns the input
// unchanged with drops=0.
func applyScopeGate(items []GuidanceItem, req GuidanceRequest, enabled bool) ([]GuidanceItem, int) {
	if !enabled || len(items) == 0 {
		return items, 0
	}
	kept := items[:0]
	drops := 0
	for _, it := range items {
		if scopeGateApplicable(it, req) {
			kept = append(kept, it)
		} else {
			drops++
		}
	}
	return kept, drops
}
