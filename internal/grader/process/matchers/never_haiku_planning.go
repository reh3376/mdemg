// Sprint JIMINY-PROCESS-OBSERVER-06 (task #165) —
// never-haiku-for-planning process matcher. FINAL observer in the arc.
//
// Grades the shipped `never-haiku-for-planning` Jiminy rule (user
// preference): "When planning complex features or fixes, ALWAYS use the
// most advanced coding model (Opus). Haiku only for simple mechanical
// tasks."
//
// Semantic:
//  1. Terminal event: `model_call` with `event_subtype='planning'`
//     (emitted by .claude/hooks/post-tool-observe.py on Write of a file
//     named `sprint_plan.md` or `plan.md`; captures ANTHROPIC_MODEL from
//     the hook's environment).
//  2. Fetch `metadata.model` from process_events.
//  3. Empty model → fail-open skip (env not set on emitter side; can't
//     grade what we can't see).
//  4. Model name contains "haiku" (case-insensitive) → `process_incomplete`
//     (reason names the model).
//  5. Else → `process_followed` (reason names the model).
//
// Fail-open contract: no cross-table query beyond process_events itself,
// no filesystem read. Purely metadata-driven verdict.
package matchers

import (
	"context"
	"strings"

	"mdemg/internal/grader/process"
)

// NeverHaikuPlanningMatcher grades the `never-haiku-for-planning` rule.
type NeverHaikuPlanningMatcher struct{}

// NewNeverHaikuPlanning constructs the matcher.
func NewNeverHaikuPlanning() *NeverHaikuPlanningMatcher { return &NeverHaikuPlanningMatcher{} }

// Name returns the persisted matcher_name (process_outcomes column).
func (m *NeverHaikuPlanningMatcher) Name() string { return "never_haiku_planning" }

// TerminalEventType returns the anchor event the grader loop routes to us.
func (m *NeverHaikuPlanningMatcher) TerminalEventType() string { return "model_call" }

// ConstraintCode returns the Jiminy rule code.
func (m *NeverHaikuPlanningMatcher) ConstraintCode() string { return "never-haiku-for-planning" }

// Grade implements process.Matcher.
func (m *NeverHaikuPlanningMatcher) Grade(ctx context.Context, pool process.PoolIface, ev process.TerminalEvent) (process.Verdict, bool, error) {
	subtype, model, err := m.planningModel(ctx, pool, ev.EventID)
	if err != nil {
		return process.Verdict{}, false, err
	}
	if subtype != "planning" {
		return process.Verdict{}, true, nil // skip — reserved for future model_call subtypes
	}
	if strings.TrimSpace(model) == "" {
		return process.Verdict{}, true, nil // skip — no model captured
	}
	lc := strings.ToLower(model)
	if strings.Contains(lc, "haiku") {
		return process.Verdict{
			ConstraintCode: m.ConstraintCode(),
			OutcomeType:    "process_incomplete",
			Reason:         "planning model was " + model + " (contains 'haiku'; rule requires Opus / Sonnet for planning)",
		}, false, nil
	}
	return process.Verdict{
		ConstraintCode:  m.ConstraintCode(),
		OutcomeType:     "process_followed",
		EvidenceEventID: ev.EventID,
		Reason:          "planning model was " + model + " (non-haiku)",
	}, false, nil
}

// planningModel fetches (event_subtype, metadata.model) for one row.
func (m *NeverHaikuPlanningMatcher) planningModel(ctx context.Context, pool process.PoolIface, eventID string) (subtype string, model string, err error) {
	rows, qerr := pool.Query(ctx, `
		SELECT event_subtype, COALESCE(metadata->>'model', '')
		FROM process_events
		WHERE event_id = $1
		LIMIT 1
	`, eventID)
	if qerr != nil {
		return "", "", qerr
	}
	defer rows.Close()
	if !rows.Next() {
		return "", "", rows.Err()
	}
	if err := rows.Scan(&subtype, &model); err != nil {
		return "", "", err
	}
	return subtype, model, rows.Err()
}
