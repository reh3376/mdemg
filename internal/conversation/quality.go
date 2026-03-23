package conversation

import (
	"strings"
	"unicode"
)

// QualityScore represents the multi-factor quality assessment of an observation.
type QualityScore struct {
	Overall       float64 `json:"overall"`       // Weighted combination (0.0-1.0)
	Specificity   float64 `json:"specificity"`   // How specific vs. vague (0.0-1.0)
	Actionability float64 `json:"actionability"` // How actionable the content is (0.0-1.0)
	ContextRich   float64 `json:"context_rich"`  // How much context is provided (0.0-1.0)
}

// QualityThresholds defines quality gates.
const (
	QualityLowThreshold  = 0.3 // Below this: flag for enrichment
	QualityHighThreshold = 0.7 // Above this: high-quality observation
)

// Quality scoring weights.
const (
	specificityWeight   = 0.40
	actionabilityWeight = 0.35
	contextRichWeight   = 0.25
)

// ScoreObservationQuality computes a multi-factor quality score for an observation.
func ScoreObservationQuality(content, obsType string, tags []string, metadata map[string]any) QualityScore {
	specificity := scoreSpecificity(content)
	actionability := scoreActionability(content, obsType)
	contextRich := scoreContextRichness(content, tags, metadata)

	overall := specificity*specificityWeight +
		actionability*actionabilityWeight +
		contextRich*contextRichWeight

	if overall > 1.0 {
		overall = 1.0
	}

	return QualityScore{
		Overall:       overall,
		Specificity:   specificity,
		Actionability: actionability,
		ContextRich:   contextRich,
	}
}

// IsLowQuality returns true if the observation quality is below the low threshold.
func (q QualityScore) IsLowQuality() bool {
	return q.Overall < QualityLowThreshold
}

// scoreSpecificity measures how specific (vs. vague/generic) the content is.
// Factors: content length, presence of identifiers, numbers, file paths.
func scoreSpecificity(content string) float64 {
	if content == "" {
		return 0.0
	}

	score := 0.0
	words := strings.Fields(content)
	wordCount := len(words)

	// Length component: very short content is less specific
	switch {
	case wordCount >= 30:
		score += 0.3
	case wordCount >= 15:
		score += 0.2
	case wordCount >= 5:
		score += 0.1
	}

	// Identifiers: code identifiers indicate specificity
	identifierCount := 0
	for _, w := range words {
		if isCodeIdentifier(w) {
			identifierCount++
		}
	}
	if wordCount > 0 {
		identRatio := float64(identifierCount) / float64(wordCount)
		switch {
		case identRatio >= 0.15:
			score += 0.3
		case identRatio >= 0.08:
			score += 0.2
		case identRatio >= 0.03:
			score += 0.1
		}
	}

	// Numbers and versions: concrete values indicate specificity
	if containsNumbers(content) {
		score += 0.1
	}

	// File paths or URLs: concrete references
	if containsPath(content) {
		score += 0.15
	}

	// Quoted strings: specific references
	if strings.Contains(content, "\"") || strings.Contains(content, "'") || strings.Contains(content, "`") {
		score += 0.1
	}

	// Penalize vague filler words
	vagueRatio := countVagueWords(words, wordCount)
	if vagueRatio > 0.3 {
		score -= 0.15
	}

	if score > 1.0 {
		score = 1.0
	}
	if score < 0.0 {
		score = 0.0
	}
	return score
}

// scoreActionability measures how actionable the observation is.
// Corrections, decisions, and errors are inherently more actionable.
func scoreActionability(content, obsType string) float64 {
	score := 0.0

	// Observation type base scores (some types are inherently actionable)
	switch obsType {
	case "correction":
		score += 0.5
	case "decision":
		score += 0.4
	case "error", "blocker":
		score += 0.4
	case "task":
		score += 0.35
	case "preference":
		score += 0.3
	case "learning", "insight":
		score += 0.2
	case "technical_note":
		score += 0.15
	case "progress":
		score += 0.1
	case "context":
		score += 0.1
	case "constraint":
		score += 0.5
	case "note":
		score += 0.1
	case "context_signal":
		score += 0.0
	case "self_improvement":
		score += 0.15
	}

	lower := strings.ToLower(content)

	// Action verbs boost actionability
	actionVerbs := []string{
		"should", "must", "need to", "requires", "use ", "don't use",
		"always", "never", "instead of", "prefer", "avoid",
		"changed to", "switched to", "migrated", "fixed",
	}
	for _, verb := range actionVerbs {
		if strings.Contains(lower, verb) {
			score += 0.1
			break
		}
	}

	// Imperative statements
	imperativeStarters := []string{
		"use ", "add ", "remove ", "fix ", "update ", "change ",
		"set ", "configure ", "run ", "create ", "delete ",
	}
	for _, starter := range imperativeStarters {
		if strings.HasPrefix(lower, starter) {
			score += 0.15
			break
		}
	}

	// Contains a "because" or "reason" — contextual why
	if strings.Contains(lower, "because") || strings.Contains(lower, "reason") {
		score += 0.1
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// scoreContextRichness measures how much useful context the observation provides.
// Factors: tags, metadata, cross-references, structured content.
func scoreContextRichness(content string, tags []string, metadata map[string]any) float64 {
	score := 0.0

	// Tags provide categorical context
	switch {
	case len(tags) >= 3:
		score += 0.3
	case len(tags) >= 1:
		score += 0.15
	}

	// Metadata provides structured context
	switch {
	case len(metadata) >= 3:
		score += 0.25
	case len(metadata) >= 1:
		score += 0.1
	}

	// Content structure: lists, bullets, colons indicate structured thinking
	if strings.Contains(content, "\n") {
		score += 0.1
	}
	if strings.Contains(content, ": ") || strings.Contains(content, " -> ") || strings.Contains(content, " → ") {
		score += 0.1
	}

	// Cross-references to other entities
	lower := strings.ToLower(content)
	referencePatterns := []string{
		"see ", "ref ", "related to", "depends on", "blocks ",
		"from session", "previously", "as discussed",
	}
	for _, pat := range referencePatterns {
		if strings.Contains(lower, pat) {
			score += 0.1
			break
		}
	}

	// Content length provides more context
	if len(content) >= 200 {
		score += 0.15
	} else if len(content) >= 100 {
		score += 0.1
	}

	if score > 1.0 {
		score = 1.0
	}
	return score
}

// isCodeIdentifier detects code-like identifiers (PascalCase, camelCase, snake_case, SCREAMING_CASE).
func isCodeIdentifier(word string) bool {
	if len(word) < 2 {
		return false
	}
	// Strip trailing punctuation
	word = strings.TrimRight(word, ".,;:!?()[]{}\"'`")
	if len(word) < 2 {
		return false
	}

	hasUpper := false
	hasLower := false
	hasUnderscore := strings.Contains(word, "_")
	hasDot := strings.Contains(word, ".")

	for _, r := range word {
		if unicode.IsUpper(r) {
			hasUpper = true
		}
		if unicode.IsLower(r) {
			hasLower = true
		}
	}

	// PascalCase or camelCase: mixed case without spaces
	if hasUpper && hasLower && !strings.Contains(word, " ") {
		// Check for at least one case transition
		prev := rune(0)
		for _, r := range word {
			if prev != 0 {
				if (unicode.IsLower(prev) && unicode.IsUpper(r)) ||
					(unicode.IsUpper(prev) && unicode.IsLower(r) && prev != rune(word[0])) {
					return true
				}
			}
			prev = r
		}
	}

	// snake_case
	if hasUnderscore && (hasLower || hasUpper) {
		return true
	}

	// SCREAMING_CASE (all upper + underscores)
	if hasUpper && !hasLower && hasUnderscore {
		return true
	}

	// dotted path (e.g., internal.api.server)
	if hasDot && hasLower && len(word) > 5 {
		return true
	}

	return false
}

// containsNumbers checks if the content has numeric values.
func containsNumbers(content string) bool {
	for _, r := range content {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

// containsPath checks if content has file paths or URLs.
func containsPath(content string) bool {
	return strings.Contains(content, "/") && (strings.Contains(content, ".go") ||
		strings.Contains(content, ".py") ||
		strings.Contains(content, ".ts") ||
		strings.Contains(content, ".js") ||
		strings.Contains(content, ".md") ||
		strings.Contains(content, ".yaml") ||
		strings.Contains(content, ".yml") ||
		strings.Contains(content, ".json") ||
		strings.Contains(content, "http://") ||
		strings.Contains(content, "https://") ||
		strings.HasPrefix(content, "/") ||
		strings.Contains(content, "internal/") ||
		strings.Contains(content, "cmd/"))
}

// vagueWords are common filler/vague words that indicate low specificity.
var vagueWords = map[string]bool{
	"thing": true, "things": true, "stuff": true, "something": true,
	"somehow": true, "somewhere": true, "sometime": true,
	"maybe": true, "probably": true, "might": true,
	"basically": true, "essentially": true, "generally": true,
	"etc": true, "etc.": true, "whatever": true,
}

// countVagueWords returns the ratio of vague words in the content.
func countVagueWords(words []string, total int) float64 {
	if total == 0 {
		return 0.0
	}
	count := 0
	for _, w := range words {
		if vagueWords[strings.ToLower(strings.TrimRight(w, ".,;:!?"))] {
			count++
		}
	}
	return float64(count) / float64(total)
}
