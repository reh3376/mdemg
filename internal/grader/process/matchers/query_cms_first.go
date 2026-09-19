// Sprint JIMINY-PROCESS-OBSERVER-03 (task #162) —
// query-cms-first process matcher.
//
// Grades the shipped `query-mdemg-cms-file-paths` Jiminy rule: "When
// discovering unfamiliar code structure, query MDEMG CMS retrieval FIRST;
// glob/grep only for exact-token or CMS-miss fallback."
//
// Semantic:
//  1. Terminal event: `filesystem_search` (Glob / Grep tool, or Bash
//     containing rg/ripgrep/ag/grep -r/find — emitted by
//     .claude/hooks/post-tool-observe.py).
//  2. Query same-session prior `retrieval_call` events (agent MCP retrieve
//     tool call OR agent-side curl to /v1/memory/retrieve) within
//     `WindowSeconds` before the terminal event.
//  3. ≥1 prior retrieval → `process_followed` (evidence = the most recent).
//  4. 0 prior retrievals → `process_missed`.
//
// Fail-open contract: matcher never returns an error that breaks the
// grader loop. A missing session_id / bad SQL / empty rows all resolve to
// a defined verdict or a skip.
//
// The "exact-token / CMS-miss fallback" exception is NOT distinguished in
// this MVP — narrow, specific-string searches are graded the same as
// broad discovery searches. If FP rate is unacceptable in operator use,
// a follow-up sprint can add a search-shape heuristic (path scope +
// single-quoted literal). Rule is shipped default-OFF; operator flip
// after passive observation.
package matchers

import (
	"context"
	"strconv"
	"time"

	"mdemg/internal/grader/process"
)

// QueryCmsFirstMatcher grades the `query-mdemg-cms-file-paths` Jiminy rule.
type QueryCmsFirstMatcher struct {
	// WindowSeconds is the max age of a prior retrieval to be considered
	// as satisfying the "query first" precondition. 0 → default 300s.
	WindowSeconds int
}

// NewQueryCmsFirst constructs the matcher with an explicit window.
// windowSec ≤ 0 falls back to 300s (5 min).
func NewQueryCmsFirst(windowSec int) *QueryCmsFirstMatcher {
	if windowSec <= 0 {
		windowSec = 300
	}
	return &QueryCmsFirstMatcher{WindowSeconds: windowSec}
}

// Name returns the persisted matcher_name (process_outcomes column).
func (m *QueryCmsFirstMatcher) Name() string {
	return "query_cms_first"
}

// TerminalEventType returns the anchor event the grader loop routes to us.
func (m *QueryCmsFirstMatcher) TerminalEventType() string {
	return "filesystem_search"
}

// ConstraintCode returns the Jiminy rule code (must match a live substrate
// node for the aggregation join to resolve).
func (m *QueryCmsFirstMatcher) ConstraintCode() string {
	return "query-mdemg-cms-file-paths"
}

// Grade implements process.Matcher.
//
// Fetches the most-recent same-session `retrieval_call` event before the
// terminal filesystem_search event, within the configured window.
func (m *QueryCmsFirstMatcher) Grade(ctx context.Context, pool process.PoolIface, ev process.TerminalEvent) (process.Verdict, bool, error) {
	// Guard: session_id is required for correlation. A search event with
	// no session_id is skip-safe (fail-open per contract).
	if ev.SessionID == "" {
		return process.Verdict{}, true, nil
	}
	windowSec := m.WindowSeconds
	if windowSec <= 0 {
		windowSec = 300
	}
	priorEventID, hasPrior, err := m.mostRecentRetrieval(ctx, pool, ev.SpaceID, ev.SessionID, ev.Time, windowSec)
	if err != nil {
		return process.Verdict{}, false, err
	}
	if hasPrior {
		return process.Verdict{
			ConstraintCode:  m.ConstraintCode(),
			OutcomeType:     "process_followed",
			EvidenceEventID: priorEventID,
			Reason:          "retrieval_call within " + strconv.Itoa(windowSec) + "s preceded filesystem_search",
		}, false, nil
	}
	return process.Verdict{
		ConstraintCode: m.ConstraintCode(),
		OutcomeType:    "process_missed",
		Reason:         "no retrieval_call on this session within " + strconv.Itoa(windowSec) + "s before the filesystem_search",
	}, false, nil
}

// mostRecentRetrieval returns the newest `retrieval_call` event_id on the
// same session BEFORE `before`, no older than `windowSec` seconds.
func (m *QueryCmsFirstMatcher) mostRecentRetrieval(ctx context.Context, pool process.PoolIface, spaceID, sessionID string, before time.Time, windowSec int) (string, bool, error) {
	windowStart := before.Add(-time.Duration(windowSec) * time.Second)
	rows, err := pool.Query(ctx, `
		SELECT event_id
		FROM process_events
		WHERE space_id = $1
		  AND session_id = $2
		  AND event_type = 'retrieval_call'
		  AND time < $3
		  AND time >= $4
		ORDER BY time DESC
		LIMIT 1
	`, spaceID, sessionID, before, windowStart)
	if err != nil {
		return "", false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", false, rows.Err()
	}
	var evID string
	if err := rows.Scan(&evID); err != nil {
		return "", false, err
	}
	return evID, true, rows.Err()
}
