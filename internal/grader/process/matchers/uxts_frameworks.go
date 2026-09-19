// Sprint JIMINY-PROCESS-OBSERVER-05 (task #164) —
// must-use-uxts-frameworks-consistently process matcher.
//
// Grades the shipped `must-use-uxts-frameworks-consistently` Jiminy rule
// (hybrid class): "When creating a JSON schema, contract, or test spec
// that will be used repeatedly, use the UxTS framework family."
//
// Design classifier side: new JSON schema files outside `docs/tests/u*ts/`
// = classifier-verifiable violation. This observer ships the process side.
//
// Semantic:
//  1. Terminal event: `file_write` (Write or Edit tool, emitted by
//     .claude/hooks/post-tool-observe.py).
//  2. Extract `metadata.file_path`.
//  3. Filename regex: `\.u[a-z]+\.json$` (u-prefix + letters + `.json`) —
//     matches the 11 shipped UxTS framework families (uaits, uams, ubench,
//     ubts, uets, uits, ults, uobs, usts, utds, uvts) + any future
//     u-prefix framework.
//  4. No match → fail-open skip (not a UxTS-shape JSON file).
//  5. Path check: does the file_path contain the substring `docs/tests/u`?
//     - Match → `process_followed` (filename shape under UxTS dir)
//     - No match → `process_incomplete` (reason names the violating path)
//
// Fail-open contract: matcher never returns an error. No cross-table
// query, no filesystem read. Purely a metadata JSON extraction + regex
// against `metadata.file_path`.
package matchers

import (
	"context"
	"regexp"
	"strings"

	"mdemg/internal/grader/process"
)

var uxtsFilenameRe = regexp.MustCompile(`\.u[a-z]+\.json$`)

// UxtsFrameworksMatcher grades the `must-use-uxts-frameworks-consistently` rule.
type UxtsFrameworksMatcher struct{}

// NewUxtsFrameworks constructs the matcher.
func NewUxtsFrameworks() *UxtsFrameworksMatcher { return &UxtsFrameworksMatcher{} }

// Name returns the persisted matcher_name (process_outcomes column).
func (m *UxtsFrameworksMatcher) Name() string { return "uxts_frameworks" }

// TerminalEventType returns the anchor event the grader loop routes to us.
func (m *UxtsFrameworksMatcher) TerminalEventType() string { return "file_write" }

// ConstraintCode returns the Jiminy rule code.
func (m *UxtsFrameworksMatcher) ConstraintCode() string {
	return "must-use-uxts-frameworks-consistently"
}

// Grade implements process.Matcher.
func (m *UxtsFrameworksMatcher) Grade(ctx context.Context, pool process.PoolIface, ev process.TerminalEvent) (process.Verdict, bool, error) {
	filePath, err := m.filePath(ctx, pool, ev.EventID)
	if err != nil {
		return process.Verdict{}, false, err
	}
	if strings.TrimSpace(filePath) == "" {
		return process.Verdict{}, true, nil // skip — no file_path in metadata
	}
	if !uxtsFilenameRe.MatchString(filePath) {
		return process.Verdict{}, true, nil // skip — not a UxTS-shape JSON file
	}
	if strings.Contains(filePath, "docs/tests/u") {
		return process.Verdict{
			ConstraintCode:  m.ConstraintCode(),
			OutcomeType:     "process_followed",
			EvidenceEventID: ev.EventID,
			Reason:          "UxTS-shape JSON " + filePath + " under docs/tests/u* framework family dir",
		}, false, nil
	}
	return process.Verdict{
		ConstraintCode: m.ConstraintCode(),
		OutcomeType:    "process_incomplete",
		Reason:         "UxTS-shape JSON " + filePath + " outside docs/tests/u* framework family (should live under docs/tests/u<name>/)",
	}, false, nil
}

// filePath fetches metadata.file_path from one process_events row.
func (m *UxtsFrameworksMatcher) filePath(ctx context.Context, pool process.PoolIface, eventID string) (string, error) {
	rows, err := pool.Query(ctx, `
		SELECT COALESCE(metadata->>'file_path', '')
		FROM process_events
		WHERE event_id = $1
		LIMIT 1
	`, eventID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", rows.Err()
	}
	var fp string
	if err := rows.Scan(&fp); err != nil {
		return "", err
	}
	return fp, rows.Err()
}
