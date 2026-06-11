package retrieval

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// TemporalMode represents the type of temporal reasoning to apply.
type TemporalMode string

const (
	TemporalModeNone TemporalMode = "none"
	TemporalModeSoft TemporalMode = "soft"
	TemporalModeHard TemporalMode = "hard"
)

// TemporalConstraint defines a time range for hard-mode filtering.
type TemporalConstraint struct {
	After       *time.Time
	Before      *time.Time
	Description string // "last 7 days", "since 2026-01-15"
}

// TemporalIntent captures the parsed temporal understanding from a query.
type TemporalIntent struct {
	Mode       TemporalMode
	Constraint *TemporalConstraint // nil when mode == "none"
	Keywords   []string
	Confidence float64 // 0.0-1.0
}

// Hard-mode patterns: explicit time range references
var hardTemporalPatterns = []struct {
	re      *regexp.Regexp
	extract func(matches []string, now time.Time) *TemporalConstraint
}{
	// "in the last N days/weeks/months"
	{
		re: regexp.MustCompile(`(?i)\b(?:in\s+)?the\s+last\s+(\d+)\s+(days?|weeks?|months?)\b`),
		extract: func(m []string, now time.Time) *TemporalConstraint {
			n, _ := strconv.Atoi(m[1])
			dur := relativeDuration(n, m[2])
			after := now.Add(-dur)
			return &TemporalConstraint{After: &after, Description: "last " + m[1] + " " + m[2]}
		},
	},
	// "last N days/weeks/months" (without "in the")
	{
		re: regexp.MustCompile(`(?i)\blast\s+(\d+)\s+(days?|weeks?|months?)\b`),
		extract: func(m []string, now time.Time) *TemporalConstraint {
			n, _ := strconv.Atoi(m[1])
			dur := relativeDuration(n, m[2])
			after := now.Add(-dur)
			return &TemporalConstraint{After: &after, Description: "last " + m[1] + " " + m[2]}
		},
	},
	// "this week"
	{
		re: regexp.MustCompile(`(?i)\bthis\s+week\b`),
		extract: func(_ []string, now time.Time) *TemporalConstraint {
			weekday := int(now.Weekday())
			if weekday == 0 {
				weekday = 7 // Sunday
			}
			startOfWeek := time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, now.Location())
			return &TemporalConstraint{After: &startOfWeek, Description: "this week"}
		},
	},
	// "this month"
	{
		re: regexp.MustCompile(`(?i)\bthis\s+month\b`),
		extract: func(_ []string, now time.Time) *TemporalConstraint {
			startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
			return &TemporalConstraint{After: &startOfMonth, Description: "this month"}
		},
	},
	// "this year"
	{
		re: regexp.MustCompile(`(?i)\bthis\s+year\b`),
		extract: func(_ []string, now time.Time) *TemporalConstraint {
			startOfYear := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
			return &TemporalConstraint{After: &startOfYear, Description: "this year"}
		},
	},
	// "last week"
	{
		re: regexp.MustCompile(`(?i)\blast\s+week\b`),
		extract: func(_ []string, now time.Time) *TemporalConstraint {
			dur := 7 * 24 * time.Hour
			after := now.Add(-dur)
			return &TemporalConstraint{After: &after, Description: "last week"}
		},
	},
	// "last month"
	{
		re: regexp.MustCompile(`(?i)\blast\s+month\b`),
		extract: func(_ []string, now time.Time) *TemporalConstraint {
			after := now.AddDate(0, -1, 0)
			return &TemporalConstraint{After: &after, Description: "last month"}
		},
	},
	// "since YYYY-MM-DD" (ISO 8601)
	{
		re: regexp.MustCompile(`(?i)\bsince\s+(\d{4}-\d{2}-\d{2})\b`),
		extract: func(m []string, now time.Time) *TemporalConstraint {
			t, err := time.Parse("2006-01-02", m[1])
			if err != nil {
				return nil
			}
			return &TemporalConstraint{After: &t, Description: "since " + m[1]}
		},
	},
	// "since January 2026" / "since Jan 2026"
	{
		re: regexp.MustCompile(`(?i)\bsince\s+(` + monthPattern + `)\s+(\d{4})\b`),
		extract: func(m []string, now time.Time) *TemporalConstraint {
			month := parseMonth(m[1])
			if month == 0 {
				return nil
			}
			year, _ := strconv.Atoi(m[2])
			t := time.Date(year, month, 1, 0, 0, 0, 0, now.Location())
			return &TemporalConstraint{After: &t, Description: "since " + m[1] + " " + m[2]}
		},
	},
	// "before YYYY-MM-DD" (ISO 8601)
	{
		re: regexp.MustCompile(`(?i)\bbefore\s+(\d{4}-\d{2}-\d{2})\b`),
		extract: func(m []string, now time.Time) *TemporalConstraint {
			t, err := time.Parse("2006-01-02", m[1])
			if err != nil {
				return nil
			}
			return &TemporalConstraint{Before: &t, Description: "before " + m[1]}
		},
	},
	// "before January 2026" / "before Jan 2026"
	{
		re: regexp.MustCompile(`(?i)\bbefore\s+(` + monthPattern + `)\s+(\d{4})\b`),
		extract: func(m []string, now time.Time) *TemporalConstraint {
			month := parseMonth(m[1])
			if month == 0 {
				return nil
			}
			year, _ := strconv.Atoi(m[2])
			t := time.Date(year, month, 1, 0, 0, 0, 0, now.Location())
			return &TemporalConstraint{Before: &t, Description: "before " + m[1] + " " + m[2]}
		},
	},
	// "between YYYY-MM-DD and YYYY-MM-DD"
	{
		re: regexp.MustCompile(`(?i)\bbetween\s+(\d{4}-\d{2}-\d{2})\s+and\s+(\d{4}-\d{2}-\d{2})\b`),
		extract: func(m []string, now time.Time) *TemporalConstraint {
			after, err1 := time.Parse("2006-01-02", m[1])
			before, err2 := time.Parse("2006-01-02", m[2])
			if err1 != nil || err2 != nil {
				return nil
			}
			return &TemporalConstraint{After: &after, Before: &before, Description: "between " + m[1] + " and " + m[2]}
		},
	},
}

// monthPattern matches month names (full and abbreviated)
const monthPattern = `January|February|March|April|May|June|July|August|September|October|November|December|Jan|Feb|Mar|Apr|Jun|Jul|Aug|Sep|Oct|Nov|Dec`

// Soft-mode trigger keywords
var softTemporalKeywords = []string{
	"recent", "recently", "latest", "newest", "what changed",
	"what's new", "whats new", "updates to", "updated",
	"new changes", "latest changes", "recent changes",
}

// ParseTemporalIntent analyzes a query string for temporal language.
// Returns TemporalModeNone when no temporal language is detected.
func ParseTemporalIntent(query string, now time.Time) TemporalIntent {
	if query == "" {
		return TemporalIntent{Mode: TemporalModeNone}
	}

	queryLower := strings.ToLower(query)

	// Check hard-mode patterns first (more specific)
	for _, p := range hardTemporalPatterns {
		matches := p.re.FindStringSubmatch(query)
		if matches != nil {
			constraint := p.extract(matches, now)
			if constraint != nil {
				return TemporalIntent{
					Mode:       TemporalModeHard,
					Constraint: constraint,
					Keywords:   []string{matches[0]},
					Confidence: 0.9,
				}
			}
		}
	}

	// Check soft-mode keywords
	var foundKeywords []string
	for _, kw := range softTemporalKeywords {
		if strings.Contains(queryLower, kw) {
			foundKeywords = append(foundKeywords, kw)
		}
	}

	if len(foundKeywords) > 0 {
		confidence := 0.7
		if len(foundKeywords) > 1 {
			confidence = 0.85
		}
		return TemporalIntent{
			Mode:       TemporalModeSoft,
			Keywords:   foundKeywords,
			Confidence: confidence,
		}
	}

	return TemporalIntent{Mode: TemporalModeNone}
}

// FilterCandidatesByTime applies hard-mode time range filtering.
// Returns only candidates within [After, Before).
// Uses CanonicalTime when available, falling back to UpdatedAt.
// If constraint is nil, returns all candidates unchanged.
func FilterCandidatesByTime(cands []Candidate, constraint *TemporalConstraint) []Candidate {
	if constraint == nil {
		return cands
	}

	filtered := make([]Candidate, 0, len(cands))
	for _, c := range cands {
		// Use canonical_time for temporal filtering when available
		ref := c.UpdatedAt
		if !c.CanonicalTime.IsZero() {
			ref = c.CanonicalTime
		}
		if constraint.After != nil && ref.Before(*constraint.After) {
			continue
		}
		if constraint.Before != nil && !ref.Before(*constraint.Before) {
			continue
		}
		filtered = append(filtered, c)
	}
	return filtered
}

// StaleReferencePenalty computes a penalty for candidates that reference
// significantly newer content (suggesting the candidate may be outdated).
// Returns map[NodeID]penalty. Penalty is capped at maxPenalty.
// Returns nil map if staleDays <= 0.
func StaleReferencePenalty(cands []Candidate, edges []Edge, staleDays float64, maxPenalty float64) map[string]float64 {
	if staleDays <= 0 {
		return nil
	}

	// Build map of node → time reference (canonical_time or fallback to UpdatedAt)
	nodeTime := make(map[string]time.Time, len(cands))
	for _, c := range cands {
		ref := c.UpdatedAt
		if !c.CanonicalTime.IsZero() {
			ref = c.CanonicalTime
		}
		nodeTime[c.NodeID] = ref
	}

	penalties := make(map[string]float64)

	for _, e := range edges {
		srcTime, srcOK := nodeTime[e.Src]
		dstTime, dstOK := nodeTime[e.Dst]
		if !srcOK || !dstOK {
			continue
		}

		// If dst is significantly newer than src, penalize src
		daysDiff := dstTime.Sub(srcTime).Hours() / 24.0
		if daysDiff > staleDays {
			penalty := 0.05 * (daysDiff - staleDays) / staleDays
			if penalty > maxPenalty {
				penalty = maxPenalty
			}
			// Keep the largest penalty for each node
			if penalty > penalties[e.Src] {
				penalties[e.Src] = penalty
			}
		}
	}

	return penalties
}

// CleanTemporalKeywords strips detected temporal phrases from the query text
// for cleaner semantic/BM25 search. Returns original query if no keywords found.
func CleanTemporalKeywords(query string, keywords []string) string {
	if len(keywords) == 0 {
		return query
	}

	cleaned := query
	for _, kw := range keywords {
		// Case-insensitive replacement
		re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(kw))
		cleaned = re.ReplaceAllString(cleaned, "")
	}

	// Clean up extra whitespace
	cleaned = strings.TrimSpace(cleaned)
	cleaned = regexp.MustCompile(`\s{2,}`).ReplaceAllString(cleaned, " ")

	if cleaned == "" {
		return query // Don't return empty string
	}
	return cleaned
}

// BuildExplicitTemporalIntent constructs a hard-mode TemporalIntent from
// explicit ISO8601 date strings (used for API override fields).
func BuildExplicitTemporalIntent(afterStr, beforeStr string) TemporalIntent {
	constraint := &TemporalConstraint{}
	parts := []string{}

	if afterStr != "" {
		t, err := time.Parse(time.RFC3339, afterStr)
		if err != nil {
			// Try date-only format
			t, err = time.Parse("2006-01-02", afterStr)
		}
		if err == nil {
			constraint.After = &t
			parts = append(parts, "after "+afterStr)
		}
	}

	if beforeStr != "" {
		t, err := time.Parse(time.RFC3339, beforeStr)
		if err != nil {
			// Try date-only format
			t, err = time.Parse("2006-01-02", beforeStr)
		}
		if err == nil {
			constraint.Before = &t
			parts = append(parts, "before "+beforeStr)
		}
	}

	if constraint.After == nil && constraint.Before == nil {
		return TemporalIntent{Mode: TemporalModeNone}
	}

	constraint.Description = strings.Join(parts, ", ")

	return TemporalIntent{
		Mode:       TemporalModeHard,
		Constraint: constraint,
		Keywords:   nil,
		Confidence: 1.0, // Explicit override = full confidence
	}
}

// relativeDuration converts a count and unit to a time.Duration
func relativeDuration(n int, unit string) time.Duration {
	unit = strings.ToLower(unit)
	switch {
	case strings.HasPrefix(unit, "day"):
		return time.Duration(n) * 24 * time.Hour
	case strings.HasPrefix(unit, "week"):
		return time.Duration(n) * 7 * 24 * time.Hour
	case strings.HasPrefix(unit, "month"):
		return time.Duration(n) * 30 * 24 * time.Hour // approximate
	default:
		return time.Duration(n) * 24 * time.Hour
	}
}

// parseMonth converts a month name (full or abbreviated) to time.Month
func parseMonth(name string) time.Month {
	months := map[string]time.Month{
		"january": time.January, "jan": time.January,
		"february": time.February, "feb": time.February,
		"march": time.March, "mar": time.March,
		"april": time.April, "apr": time.April,
		"may":  time.May,
		"june": time.June, "jun": time.June,
		"july": time.July, "jul": time.July,
		"august": time.August, "aug": time.August,
		"september": time.September, "sep": time.September,
		"october": time.October, "oct": time.October,
		"november": time.November, "nov": time.November,
		"december": time.December, "dec": time.December,
	}
	return months[strings.ToLower(name)]
}
