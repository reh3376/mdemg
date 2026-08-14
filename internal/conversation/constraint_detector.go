package conversation

import (
	"log/slog"
	"mdemg/internal/sanitize"
	"regexp"
	"strings"
)

// constraintPattern pairs a compiled regex with a base confidence score.
type constraintPattern struct {
	regex      *regexp.Regexp
	confidence float64
}

// DetectedConstraint represents a constraint found in observation text.
type DetectedConstraint struct {
	ConstraintType string  `json:"constraint_type"` // must, must_not, should, should_not, deadline
	Name           string  `json:"name"`            // Short extracted label
	Confidence     float64 `json:"confidence"`
	MatchedPattern string  `json:"matched_pattern"`
	// SkippedSuppressed is populated ONLY on the single canonical result when
	// JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001's severity-precedence gate
	// collapses a multi-severity match. It's the count of same-obs sibling
	// severities that were suppressed (N-1 where N is the raw match count).
	// Populated for downstream telemetry hooks; not persisted on the L1 node.
	SkippedSuppressed int `json:"skipped_suppressed,omitempty"`
}

// severityPrecedence orders constraint types from strongest to weakest.
// JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001 (2026-08-14): when one L0
// observation triggers multiple severity buckets, this precedence picks the
// single canonical DetectedConstraint. Rationale:
//   - `must_not` outranks `must` because a prohibition is strictly more
//     constraining than an obligation over the same substrate (the audit-004
//     twin pair `z5xgcm`/`pwa2lm` proved this — the `must_not` twin was
//     semantically correct, the `must` twin was a spurious match).
//   - `should_not` > `should` for the same reason (prohibition over
//     preference).
//   - `deadline` is temporally-oriented, not severity-oriented; it lands
//     last only as a tiebreak — content that is ONLY a deadline isn't
//     collapsed at all (len(bestByType)==1).
var severityPrecedence = map[string]int{
	"must_not":   5,
	"must":       4,
	"should_not": 3,
	"should":     2,
	"deadline":   1,
}

// ConstraintDetector scans observation content for commitment/prohibition patterns.
type ConstraintDetector struct {
	patterns      map[string][]constraintPattern
	minConfidence float64
	dedupEnabled  bool
}

// NewConstraintDetector creates a detector with compiled patterns.
// The severity-precedence collapse (JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001)
// defaults ON; the conversation service overrides via SetDedupEnabled when
// the operator sets CONSTRAINT_DETECTOR_DEDUP_ENABLED=false.
func NewConstraintDetector(minConfidence float64) *ConstraintDetector {
	if minConfidence <= 0 {
		minConfidence = 0.6
	}
	d := &ConstraintDetector{
		patterns:      make(map[string][]constraintPattern),
		minConfidence: minConfidence,
		dedupEnabled:  true, // safe default per JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001
	}
	d.initPatterns()
	return d
}

// SetDedupEnabled toggles the severity-precedence collapse. Off returns to
// pre-sprint behavior (byte-identical).
func (d *ConstraintDetector) SetDedupEnabled(enabled bool) {
	d.dedupEnabled = enabled
}

func (d *ConstraintDetector) initPatterns() {
	mustPatterns := []struct {
		pattern    string
		confidence float64
	}{
		{`\bmust\b`, 0.7},
		{`\balways\b`, 0.65},
		{`\brequired\b`, 0.75},
		{`\bhave to\b`, 0.65},
		{`\bneed to ensure\b`, 0.7},
		{`\bmandatory\b`, 0.8},
	}
	for _, p := range mustPatterns {
		d.patterns["must"] = append(d.patterns["must"], constraintPattern{
			regex:      regexp.MustCompile(`(?i)` + p.pattern),
			confidence: p.confidence,
		})
	}

	mustNotPatterns := []struct {
		pattern    string
		confidence float64
	}{
		{`\bnever\b`, 0.75},
		{`\bdon'?t\b`, 0.55},
		{`\bmust not\b`, 0.8},
		{`\bforbidden\b`, 0.85},
		{`\bavoid\b`, 0.6},
		{`\bcan'?t use\b`, 0.65},
		{`\bnot allowed\b`, 0.75},
		{`\bprohibited\b`, 0.85},
	}
	for _, p := range mustNotPatterns {
		d.patterns["must_not"] = append(d.patterns["must_not"], constraintPattern{
			regex:      regexp.MustCompile(`(?i)` + p.pattern),
			confidence: p.confidence,
		})
	}

	shouldPatterns := []struct {
		pattern    string
		confidence float64
	}{
		{`\bshould\b`, 0.55},
		{`\bprefer\b`, 0.5},
		{`\brecommended\b`, 0.6},
		{`\bbest practice\b`, 0.65},
		{`\bideally\b`, 0.5},
	}
	for _, p := range shouldPatterns {
		d.patterns["should"] = append(d.patterns["should"], constraintPattern{
			regex:      regexp.MustCompile(`(?i)` + p.pattern),
			confidence: p.confidence,
		})
	}

	shouldNotPatterns := []struct {
		pattern    string
		confidence float64
	}{
		{`\bshould not\b`, 0.6},
		{`\btry to avoid\b`, 0.55},
		{`\bdiscouraged\b`, 0.65},
		{`\bshouldn'?t\b`, 0.6},
	}
	for _, p := range shouldNotPatterns {
		d.patterns["should_not"] = append(d.patterns["should_not"], constraintPattern{
			regex:      regexp.MustCompile(`(?i)` + p.pattern),
			confidence: p.confidence,
		})
	}

	deadlinePatterns := []struct {
		pattern    string
		confidence float64
	}{
		{`\bby\s+\d{4}[-/]\d{1,2}[-/]\d{1,2}\b`, 0.8},
		{`\bbefore\s+\w+\s+\d{1,2}`, 0.7},
		{`\bdue\s+(date|by)\b`, 0.75},
		{`\bdeadline\b`, 0.8},
		{`\btarget date\b`, 0.75},
	}
	for _, p := range deadlinePatterns {
		d.patterns["deadline"] = append(d.patterns["deadline"], constraintPattern{
			regex:      regexp.MustCompile(`(?i)` + p.pattern),
			confidence: p.confidence,
		})
	}
}

// Detect scans content for constraint patterns and returns detected constraints.
// obsType boosts confidence: decision +0.2, correction +0.15.
func (d *ConstraintDetector) Detect(content string, obsType ObservationType) []DetectedConstraint {
	var results []DetectedConstraint

	// Observation type confidence boost
	boost := 0.0
	switch obsType {
	case ObsTypeConstraint:
		boost = 0.25
	case ObsTypeDecision:
		boost = 0.2
	case ObsTypeCorrection:
		boost = 0.15
	}

	// Track which constraint types we've already matched (take highest confidence per type)
	bestByType := make(map[string]DetectedConstraint)

	for cType, patterns := range d.patterns {
		for _, p := range patterns {
			if p.regex.MatchString(content) {
				conf := p.confidence + boost
				if conf > 1.0 {
					conf = 1.0
				}

				if conf < d.minConfidence {
					continue
				}

				existing, exists := bestByType[cType]
				if !exists || conf > existing.Confidence {
					bestByType[cType] = DetectedConstraint{
						ConstraintType: cType,
						Name:           extractConstraintName(content),
						Confidence:     conf,
						MatchedPattern: p.regex.String(),
					}
				}
			}
		}
	}

	// JIMINY-CORPUS-CONSTRAINT-DETECTOR-DEDUP-001 (2026-08-14): when the
	// dedup gate is on AND multiple severity buckets matched on the same
	// L0 observation, collapse to a single canonical DetectedConstraint via
	// severity precedence (must_not > must > should_not > should > deadline).
	// This closes the dual-severity dual-mint class where the L1 promoter
	// (`CreateConstraintNodes`) minted one node per emitted type, all sharing
	// the same constraint_code — the class Fable's JIMINY-CORPUS-AUDIT-004
	// found live on mdemg-dev.
	if d.dedupEnabled && len(bestByType) > 1 {
		var chosenType string
		var chosenPrec int
		for cType, det := range bestByType {
			prec := severityPrecedence[cType]
			if prec == 0 {
				continue // unknown type; skip from candidacy but still emit alone if it's the only one
			}
			if chosenType == "" || prec > chosenPrec ||
				(prec == chosenPrec && det.Confidence > bestByType[chosenType].Confidence) {
				chosenType = cType
				chosenPrec = prec
			}
		}
		if chosenType != "" {
			suppressed := len(bestByType) - 1
			winner := bestByType[chosenType]
			winner.SkippedSuppressed = suppressed
			results = []DetectedConstraint{winner}
			// Collect suppressed-type names for the log line.
			suppressedTypes := make([]string, 0, suppressed)
			for t := range bestByType {
				if t != chosenType {
					suppressedTypes = append(suppressedTypes, t)
				}
			}
			slog.Debug("constraint detector: multi-emit collapsed to canonical",
				"chosen_type", chosenType,
				"suppressed_types", suppressedTypes)
			return results
		}
	}

	for _, det := range bestByType {
		results = append(results, det)
	}
	return results
}

// extractConstraintName produces a short label from the first sentence of the content.
func extractConstraintName(content string) string {
	// Take first sentence (up to period, newline, or 120 chars)
	name := content
	if idx := strings.IndexAny(name, ".\n"); idx > 0 {
		name = name[:idx]
	}
	name = sanitize.CutRuneSafeSuffix(name, 120, "...")
	return strings.TrimSpace(name)
}
