// Sprint JIMINY-PROCESS-OBSERVER-02 (task #161) —
// sequential-epics process matcher.
//
// Grades the shipped `sequential-epics` Jiminy rule: "Execute sprint epics
// SEQUENTIALLY — Epic N MUST complete fully before Epic N+1 begins."
//
// Semantic:
//  1. Given a `git_commit` event on session S at time T with a commit_message
//     in metadata (added by the hook via `git log -1 --format=%B` —
//     JIMINY-PROCESS-OBSERVER-02 hook extension).
//  2. Extract the `Epic N` marker from the commit_message via
//     `\b[Ee]pic\s+(\d+)\b` (word-boundary anchored).
//  3. If no marker OR no commit_message → fail-open skip (not every commit
//     is part of a numbered sprint; a chore/docs/whitespace commit MUST NOT
//     be graded as "missed").
//  4. Query same-session prior `git_commit` events; extract each's Epic N;
//     find max prior N.
//  5. If max prior ≤ current → process_followed (monotonic sequence
//     maintained; equal N is fine — same epic, multiple commits).
//  6. If max prior > current → process_incomplete (out-of-order: an epic
//     numbered lower landed AFTER an epic numbered higher on the same
//     session, which is exactly what `sequential-epics` prohibits).
//
// Fail-open contract: missing evidence ≠ violation. Only present-event
// -with-out-of-order-outcome produces process_incomplete. Same class as
// JIMINY-CLASSIFIER-CONTEXT-002 mechanism-scope gate.
package matchers

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"mdemg/internal/grader/process"
)

var epicRegex = regexp.MustCompile(`(?i)\bepic\s+(\d+)\b`)

// SequentialEpicsMatcher grades the `sequential-epics` Jiminy rule.
type SequentialEpicsMatcher struct{}

// NewSequentialEpics constructs the matcher. No configurable state.
func NewSequentialEpics() *SequentialEpicsMatcher {
	return &SequentialEpicsMatcher{}
}

// Name returns the persisted matcher_name (process_outcomes column).
func (m *SequentialEpicsMatcher) Name() string {
	return "sequential_epics"
}

// TerminalEventType returns the anchor event the grader loop routes to us.
func (m *SequentialEpicsMatcher) TerminalEventType() string {
	return "git_commit"
}

// ConstraintCode returns the Jiminy rule code (must match a live substrate
// node for the aggregation join to resolve).
func (m *SequentialEpicsMatcher) ConstraintCode() string {
	return "sequential-epics"
}

// Grade implements process.Matcher.
//
// The grader loop hands us one terminal event at a time. To extract the
// commit message + prior events on the session we re-query process_events
// (the terminal-event struct itself carries only id/space/session/time to
// keep the shape narrow; per-matcher metadata reads live inside the matcher).
func (m *SequentialEpicsMatcher) Grade(ctx context.Context, pool process.PoolIface, ev process.TerminalEvent) (process.Verdict, bool, error) {
	// Fetch this commit's message from process_events. Fail-open skip when
	// the row can't be fetched or the message field is absent.
	currentMsg, err := m.commitMessage(ctx, pool, ev.EventID)
	if err != nil {
		return process.Verdict{}, false, err
	}
	currentN, ok := extractEpicNumber(currentMsg)
	if !ok {
		// No Epic marker on the current commit → skip (fail-open).
		return process.Verdict{}, true, nil
	}

	// Find max prior Epic number on the same session BEFORE this commit.
	priorMax, priorEventID, hasPrior, err := m.maxPriorEpicNumber(ctx, pool, ev.SpaceID, ev.SessionID, ev.EventID)
	if err != nil {
		return process.Verdict{}, false, err
	}
	if !hasPrior || priorMax <= currentN {
		return process.Verdict{
			ConstraintCode:  m.ConstraintCode(),
			OutcomeType:     "process_followed",
			EvidenceEventID: ev.EventID,
			Reason:          "monotonic epic sequence maintained (Epic " + strconv.Itoa(currentN) + ")",
		}, false, nil
	}
	// priorMax > currentN → out-of-order.
	return process.Verdict{
		ConstraintCode:  m.ConstraintCode(),
		OutcomeType:     "process_incomplete",
		EvidenceEventID: priorEventID,
		Reason: "out-of-order: prior commit on session had Epic " +
			strconv.Itoa(priorMax) + " but this commit has Epic " + strconv.Itoa(currentN),
	}, false, nil
}

// commitMessage fetches the metadata.commit_message string for one
// process_events row by event_id.
func (m *SequentialEpicsMatcher) commitMessage(ctx context.Context, pool process.PoolIface, eventID string) (string, error) {
	rows, err := pool.Query(ctx, `
		SELECT COALESCE(metadata->>'commit_message', '')
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
	var msg string
	if err := rows.Scan(&msg); err != nil {
		return "", err
	}
	return msg, rows.Err()
}

// maxPriorEpicNumber walks the same-session prior git_commit events and
// returns the maximum Epic number found (with the event_id that carried
// it). hasPrior=false when no prior commits exist OR none carry an Epic
// marker.
func (m *SequentialEpicsMatcher) maxPriorEpicNumber(ctx context.Context, pool process.PoolIface, spaceID, sessionID, currentEventID string) (int, string, bool, error) {
	rows, err := pool.Query(ctx, `
		SELECT event_id, COALESCE(metadata->>'commit_message', '')
		FROM process_events
		WHERE space_id = $1
		  AND session_id = $2
		  AND event_type = 'git_commit'
		  AND event_id <> $3
		  AND time < (SELECT time FROM process_events WHERE event_id = $3 LIMIT 1)
		ORDER BY time ASC
	`, spaceID, sessionID, currentEventID)
	if err != nil {
		return 0, "", false, err
	}
	defer rows.Close()
	maxN := -1
	maxEventID := ""
	for rows.Next() {
		var evID, msg string
		if err := rows.Scan(&evID, &msg); err != nil {
			return 0, "", false, err
		}
		if n, ok := extractEpicNumber(msg); ok && n > maxN {
			maxN = n
			maxEventID = evID
		}
	}
	if err := rows.Err(); err != nil {
		return 0, "", false, err
	}
	if maxN < 0 {
		return 0, "", false, nil
	}
	return maxN, maxEventID, true, nil
}

// extractEpicNumber returns the first `Epic N` marker in the message + true,
// or 0 + false when no marker is present.
func extractEpicNumber(msg string) (int, bool) {
	if strings.TrimSpace(msg) == "" {
		return 0, false
	}
	match := epicRegex.FindStringSubmatch(msg)
	if len(match) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	return n, true
}
