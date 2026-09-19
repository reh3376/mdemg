// Sprint JIMINY-PROCESS-OBSERVER-04 (task #163) —
// unit-integration-e2e-docs (UIED) process matcher.
//
// Grades the shipped `unit-integration-e2e-docs` Jiminy rule: "All
// development plans MUST include three testing tiers: unit tests,
// integration tests, e2e tests, plus documentation updates."
//
// Semantic:
//  1. Terminal event: `git_commit` with a `commit_message` in metadata
//     (added by OBSERVER-02 hook capture).
//  2. Extract a sprint code from the message via
//     `\b([A-Z][A-Z0-9]+(?:-[A-Z0-9]+)+-\d+)\b`
//     (word-boundary; requires at least one hyphen so ISSUE-123 style
//     single-word codes don't match).
//  3. If no code → fail-open skip (not a sprint commit).
//  4. Kebab-lower the code, resolve `<SprintDocsRoot>/<kebab-code>/sprint_plan.md`.
//  5. If file missing → `process_missed` (sprint referenced without a plan).
//  6. If file present:
//     - Case-fold search for `unit`, `integration`, and (`e2e` OR `end-to-end`).
//     - All three present → `process_followed`.
//     - Missing ≥1 → `process_incomplete` (reason names the missing tiers).
//  7. File over `MaxFileBytes` cap → skip (safety on unexpectedly-large file).
//
// Fail-open contract: filesystem-read errors return skip verdict; never
// break the grader loop. Read-only single-path lookup (no directory
// walk, no arbitrary path traversal — the sprint code is anchored to a
// kebab-lower of a UPPER-case regex match and joined under a fixed
// SprintDocsRoot).
//
// This matcher is a NEW capability class for the grader — filesystem
// read from a config-driven root. Prior matchers (lint_before_commit,
// sequential_epics, query_cms_first) only read from process_events.
// The read is bounded (path fully constructed from regex-validated
// input; max bytes capped) and fail-open on any error.
package matchers

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"mdemg/internal/grader/process"
)

var uiedSprintCodeRe = regexp.MustCompile(`\b([A-Z][A-Z0-9]+(?:-[A-Z0-9]+)+-\d+)\b`)

// UIEDMatcher grades the `unit-integration-e2e-docs` Jiminy rule.
type UIEDMatcher struct {
	// SprintDocsRoot is the filesystem root the matcher joins the
	// kebab-lower sprint code under. Zero-value → "docs/development".
	SprintDocsRoot string
	// MaxFileBytes caps the sprint_plan.md read size. Zero → 200000.
	MaxFileBytes int
}

// NewUIED constructs the matcher.
func NewUIED(sprintDocsRoot string, maxFileBytes int) *UIEDMatcher {
	if strings.TrimSpace(sprintDocsRoot) == "" {
		sprintDocsRoot = "docs/development"
	}
	if maxFileBytes <= 0 {
		maxFileBytes = 200000
	}
	return &UIEDMatcher{SprintDocsRoot: sprintDocsRoot, MaxFileBytes: maxFileBytes}
}

// Name returns the persisted matcher_name (process_outcomes column).
func (m *UIEDMatcher) Name() string { return "unit_integration_e2e_docs" }

// TerminalEventType returns the anchor event the grader loop routes to us.
func (m *UIEDMatcher) TerminalEventType() string { return "git_commit" }

// ConstraintCode returns the Jiminy rule code.
func (m *UIEDMatcher) ConstraintCode() string { return "unit-integration-e2e-docs" }

// Grade implements process.Matcher.
func (m *UIEDMatcher) Grade(ctx context.Context, pool process.PoolIface, ev process.TerminalEvent) (process.Verdict, bool, error) {
	// Fetch commit_message from process_events.
	msg, err := m.commitMessage(ctx, pool, ev.EventID)
	if err != nil {
		return process.Verdict{}, false, err
	}
	code, ok := extractSprintCode(msg)
	if !ok {
		return process.Verdict{}, true, nil // skip — no sprint code
	}
	kebab := kebabLowerSprintCode(code)
	planPath := filepath.Join(m.SprintDocsRoot, kebab, "sprint_plan.md")

	info, statErr := os.Stat(planPath)
	if statErr != nil {
		if errors.Is(statErr, fs.ErrNotExist) {
			return process.Verdict{
				ConstraintCode: m.ConstraintCode(),
				OutcomeType:    "process_missed",
				Reason:         "sprint code " + code + " referenced but no sprint_plan.md at " + planPath,
			}, false, nil
		}
		// Other filesystem error — fail-open skip
		return process.Verdict{}, true, nil
	}
	if info.Size() > int64(m.MaxFileBytes) {
		return process.Verdict{}, true, nil // over-cap skip
	}
	data, readErr := os.ReadFile(planPath) //nolint:gosec // path fully constructed from regex-validated input joined under fixed root
	if readErr != nil {
		return process.Verdict{}, true, nil
	}
	lc := strings.ToLower(string(data))
	present, missing := detectTiers(lc)
	if len(missing) == 0 {
		return process.Verdict{
			ConstraintCode:  m.ConstraintCode(),
			OutcomeType:     "process_followed",
			EvidenceEventID: ev.EventID,
			Reason:          "sprint plan has all 3 testing tiers: " + strings.Join(present, ", "),
		}, false, nil
	}
	return process.Verdict{
		ConstraintCode: m.ConstraintCode(),
		OutcomeType:    "process_incomplete",
		Reason:         "sprint plan " + planPath + " missing tier(s): " + strings.Join(missing, ", "),
	}, false, nil
}

// commitMessage fetches metadata.commit_message for one process_events row.
func (m *UIEDMatcher) commitMessage(ctx context.Context, pool process.PoolIface, eventID string) (string, error) {
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

// extractSprintCode returns the first sprint code found in the commit
// message OR ("", false) if none.
func extractSprintCode(msg string) (string, bool) {
	if strings.TrimSpace(msg) == "" {
		return "", false
	}
	match := uiedSprintCodeRe.FindStringSubmatch(msg)
	if len(match) < 2 {
		return "", false
	}
	return match[1], true
}

// kebabLowerSprintCode maps `JIMINY-PROCESS-OBSERVER-04` →
// `jiminy-process-observer-04`. Preserves hyphens; lowercases letters.
func kebabLowerSprintCode(code string) string {
	return strings.ToLower(code)
}

// detectTiers returns (present, missing) tier names. Case-fold input.
func detectTiers(lowerContent string) (present, missing []string) {
	tiers := []struct {
		name    string
		markers []string // any-match
	}{
		{"unit", []string{"unit test", "unit-test", "unit tests"}},
		{"integration", []string{"integration test", "integration tests"}},
		{"e2e", []string{"e2e", "end-to-end", "end to end"}},
	}
	for _, t := range tiers {
		found := false
		for _, mk := range t.markers {
			if strings.Contains(lowerContent, mk) {
				found = true
				break
			}
		}
		if found {
			present = append(present, t.name)
		} else {
			missing = append(missing, t.name)
		}
	}
	return
}
