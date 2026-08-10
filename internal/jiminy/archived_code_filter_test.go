package jiminy

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestConstraintCodeCypher_AllPathsFilterArchived pins JIMINY-ARCHIVED-CODE-FILTER-001:
// every Cypher query in the guidance/effectiveness surface that MATCHes a
// MemoryNode by role_type='constraint' (or role_type IN ['constraint',
// 'correction']) MUST also filter `NOT coalesce(*.is_archived, false)`.
//
// Why: `loadSpaceConstraintCodes` originally did NOT filter archived nodes,
// so the fallback keyword matcher would assign archived constraint codes to
// fresh guidance items — dragging the follow-rate analytics against codes
// that had zero live nodes. The vector-index path (matchConstraintCodeByEmbedding
// + fetchActionableCandidates) always did filter; the fallback + several
// sibling analytical queries did NOT. Live-caught 2026-08-08 on mdemg-dev
// (auto-9f5134a1a0c3: 4 archived + 1 archived nodes, 0 live, still producing
// 17 outcomes/day → 0% follow rate contribution).
//
// This test walks the relevant source files and asserts every Cypher block
// that filters constraint nodes ALSO filters is_archived. Add new query
// sites → they get flagged if you forget the archive filter.
func TestConstraintCodeCypher_AllPathsFilterArchived(t *testing.T) {
	// Files that contain Cypher queries against constraint MemoryNodes.
	// Includes archive-related queries too (the setter that ARCHIVES nodes
	// is expected to filter NOT archived — you can't re-archive; and reads
	// that ARCHIVE are the exception via allowlist below).
	files := []string{
		"service.go",
		"persistence.go",
		"stats.go",
		"confidence_updater.go",
	}
	// Sites that are LEGITIMATELY unfiltered:
	//  - confidence_updater.go archiveNodesByCode WRITES is_archived=true
	//    (the setter itself; already gates on NOT is_archived to be idempotent).
	//  - service.go BootstrapCodes / BootstrapCorrectionCodes SETTERS: mint
	//    constraint_code on nodes that lack one. Filtering archived here
	//    would silently skip newly-un-archived nodes; and setting a code
	//    on an archived node is a harmless no-op operationally (the readers
	//    filter archived, so the code is never surfaced anyway).
	//  - Any query that specifically WANTS archived rows (none today).
	// The allowlist string must appear in the same query block as the MATCH.
	allowlistedContexts := []string{
		"SET n.is_archived = true", // the archive setter itself
		"SET n.constraint_code = $code", // BootstrapCodes / BootstrapCorrectionCodes setters
		// The Bootstrap* READ pair — finds nodes lacking a code, to be
		// SET by the sibling SET query. Kept for the same reason as the
		// setter itself (see notes above).
		"n.constraint_code IS NULL OR n.constraint_code = ''",
	}
	// The archive filter phrases we accept. Kept broad to match the various
	// ways operators have written the guard across files.
	acceptableFilters := []*regexp.Regexp{
		regexp.MustCompile(`NOT coalesce\([a-zA-Z0-9_]+\.is_archived,\s*false\)`),
		regexp.MustCompile(`is_archived\s+IS\s+NULL`),
		regexp.MustCompile(`is_archived\s*=\s*false`),
	}
	// Cypher MATCH patterns that target constraint (or constraint/correction)
	// nodes and therefore need the archive filter.
	constraintMatchPattern := regexp.MustCompile(
		`(?i)MATCH\s*\([a-zA-Z]:MemoryNode[^)]*role_type[^)]*constraint`)
	// Also match the plural form: role_type IN ['constraint', 'correction']
	// as a WHERE clause (used by 3 files).
	roleTypeInPattern := regexp.MustCompile(`role_type\s+IN\s+\['constraint',\s*'correction'\]`)

	for _, fname := range files {
		path := "./" + fname
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", fname, err)
		}
		src := string(data)
		// Find all Cypher blocks (raw-string literals with backticks). A
		// "block" is the text inside a `` ` ` `` pair. Splitting on backticks
		// alternates code/string/code/string...; odd indices are the strings.
		parts := strings.Split(src, "`")
		for i := 1; i < len(parts); i += 2 {
			block := parts[i]
			// Not a Cypher block? Skip cheaply.
			if !strings.Contains(strings.ToLower(block), "match") {
				continue
			}
			// Does this block target constraint nodes?
			hasConstraintMatch := constraintMatchPattern.MatchString(block) ||
				roleTypeInPattern.MatchString(block) ||
				(strings.Contains(block, "role_type") && strings.Contains(block, "'constraint'")) ||
				(strings.Contains(block, "constraint_code") && strings.Contains(block, "MemoryNode"))
			if !hasConstraintMatch {
				continue
			}
			// Allowlist?
			allowed := false
			for _, ctx := range allowlistedContexts {
				if strings.Contains(block, ctx) {
					allowed = true
					break
				}
			}
			if allowed {
				continue
			}
			// Assert the archive filter is present.
			filterOK := false
			for _, re := range acceptableFilters {
				if re.MatchString(block) {
					filterOK = true
					break
				}
			}
			if !filterOK {
				// Trim block to first 220 chars for readable failure.
				preview := block
				if len(preview) > 220 {
					preview = preview[:220] + "..."
				}
				t.Errorf("%s: Cypher block targeting constraint MemoryNodes is missing the is_archived filter (JIMINY-ARCHIVED-CODE-FILTER-001).\nBlock:\n%s",
					fname, preview)
			}
		}
	}
}
