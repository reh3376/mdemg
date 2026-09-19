// Sprint JIMINY-PROCESS-OBSERVER-01 (task #160) Epic 4 —
// lint-before-commit process matcher.
//
// Semantic (per design spec §Phase B4):
//  1. Given a `git_commit` event on session S at time T,
//  2. Find the most-recent `file_write` event on session S before T (defines
//     the "last edit" moment; if absent → fail-open skip),
//  3. Find any `lint_run` event on session S between those two moments,
//  4. If lint_run with outcome=success → process_followed,
//  5. If lint_run with outcome=failure → process_incomplete,
//  6. Else (no lint_run in window) → process_missed.
//
// The design intent: mirror JIMINY-CLASSIFIER-CONTEXT-002's mechanism-scope
// gate — missing evidence is NOT a violation. `process_missed` here means
// "the process wasn't observed on this commit"; not every commit MUST run
// lint (e.g. docs-only commits), but on sessions that DID edit code the
// missing lint is a real signal.
package matchers

import (
	"context"
	"time"

	"mdemg/internal/grader/process"
)

// LintBeforeCommitMatcher grades the `lint-before-commit` Jiminy rule.
type LintBeforeCommitMatcher struct{}

// NewLintBeforeCommit constructs the matcher. It has no configurable state.
func NewLintBeforeCommit() *LintBeforeCommitMatcher {
	return &LintBeforeCommitMatcher{}
}

// Name returns the persisted matcher_name (process_outcomes column).
func (m *LintBeforeCommitMatcher) Name() string {
	return "lint_before_commit"
}

// TerminalEventType returns the anchor event the grader loop routes to us.
func (m *LintBeforeCommitMatcher) TerminalEventType() string {
	return "git_commit"
}

// ConstraintCode returns the Jiminy rule code (must match a live substrate
// node for the aggregation join to resolve).
func (m *LintBeforeCommitMatcher) ConstraintCode() string {
	return "lint-before-commit"
}

// Grade implements process.Matcher.
func (m *LintBeforeCommitMatcher) Grade(ctx context.Context, pool process.PoolIface, ev process.TerminalEvent) (process.Verdict, bool, error) {
	// Step 2 — find the last file_write on this session BEFORE the commit.
	// If none, fail-open skip (defensive default; a commit with no observed
	// prior file_write on this session is likely a docs/whitespace-only
	// commit we don't want to grade as "missed").
	priorWriteTime, ok, err := m.mostRecentWrite(ctx, pool, ev.SpaceID, ev.SessionID, ev.Time)
	if err != nil {
		return process.Verdict{}, false, err
	}
	if !ok {
		return process.Verdict{}, true, nil // skip
	}

	// Step 3 — find the newest lint_run on this session between priorWrite
	// and the commit. Prefer the newest so re-runs win the verdict.
	lintOutcome, lintEventID, ok, err := m.newestLintInWindow(ctx, pool, ev.SpaceID, ev.SessionID, priorWriteTime, ev.Time)
	if err != nil {
		return process.Verdict{}, false, err
	}
	if !ok {
		// Step 6 — no lint_run in the window.
		return process.Verdict{
			ConstraintCode: m.ConstraintCode(),
			OutcomeType:    "process_missed",
			Reason:         "no lint_run event on this session between the last file_write and the commit",
		}, false, nil
	}

	// Step 4/5 — lint present, branch on outcome.
	switch lintOutcome {
	case "success":
		return process.Verdict{
			ConstraintCode:  m.ConstraintCode(),
			OutcomeType:     "process_followed",
			EvidenceEventID: lintEventID,
			Reason:          "lint_run succeeded before commit",
		}, false, nil
	case "failure":
		return process.Verdict{
			ConstraintCode:  m.ConstraintCode(),
			OutcomeType:     "process_incomplete",
			EvidenceEventID: lintEventID,
			Reason:          "lint_run present but exited with failure before commit",
		}, false, nil
	default:
		// unknown / missing outcome — treat as skip; the row is not clean
		// enough to grade. Fail-open per contract.
		return process.Verdict{}, true, nil
	}
}

func (m *LintBeforeCommitMatcher) mostRecentWrite(ctx context.Context, pool process.PoolIface, spaceID, sessionID string, before time.Time) (time.Time, bool, error) {
	rows, err := pool.Query(ctx, `
		SELECT time
		FROM process_events
		WHERE space_id = $1
		  AND session_id = $2
		  AND event_type = 'file_write'
		  AND time < $3
		ORDER BY time DESC
		LIMIT 1
	`, spaceID, sessionID, before)
	if err != nil {
		return time.Time{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return time.Time{}, false, rows.Err()
	}
	var t time.Time
	if err := rows.Scan(&t); err != nil {
		return time.Time{}, false, err
	}
	return t, true, rows.Err()
}

func (m *LintBeforeCommitMatcher) newestLintInWindow(ctx context.Context, pool process.PoolIface, spaceID, sessionID string, from, to time.Time) (outcome string, eventID string, ok bool, err error) {
	rows, err := pool.Query(ctx, `
		SELECT event_id, outcome
		FROM process_events
		WHERE space_id = $1
		  AND session_id = $2
		  AND event_type = 'lint_run'
		  AND time >= $3
		  AND time <= $4
		ORDER BY time DESC
		LIMIT 1
	`, spaceID, sessionID, from, to)
	if err != nil {
		return "", "", false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", "", false, rows.Err()
	}
	if err := rows.Scan(&eventID, &outcome); err != nil {
		return "", "", false, err
	}
	return outcome, eventID, true, rows.Err()
}
