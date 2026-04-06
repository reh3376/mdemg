package jiminy

import (
	"strings"
)

// normalizeGuidanceContent transforms structured metadata content into natural language
// to improve cosine similarity with action summaries during outcome classification.
//
// The structured format (produced by generateSummary during ingestion) looks like:
//
//	"Module: error-sanitizer. Related to: caching, authentication. Key functions: f1, f2"
//
// This embeds into a different region of semantic space than action summaries like:
//
//	"Wrote handlers_test.go: func TestErrorHandling"
//
// producing a cosine similarity ceiling of ~0.33 even for matching topics.
// Natural language descriptions produce ~0.70 for matching topics.
//
// Priority: if the content includes a "SEMANTIC: ..." block (LLM-generated natural language),
// extract and return that as primary content. Otherwise, apply structural normalization.
func normalizeGuidanceContent(content string) string {
	if content == "" {
		return content
	}

	// Constraint items start with "[" (e.g., "[must_not] Never use UUID v4") —
	// these are already natural language and should pass through unchanged.
	if strings.HasPrefix(content, "[") {
		return content
	}

	// Priority path: extract SEMANTIC block if present.
	// CombineSummary() appends " SEMANTIC: <LLM natural language>" to structural summaries.
	if idx := strings.Index(content, " SEMANTIC: "); idx >= 0 {
		semantic := strings.TrimSpace(content[idx+len(" SEMANTIC: "):])
		if len(semantic) > 0 {
			return semantic
		}
	}
	// Also check at the start of the string (no leading space)
	if strings.HasPrefix(content, "SEMANTIC: ") {
		semantic := strings.TrimSpace(content[len("SEMANTIC: "):])
		if len(semantic) > 0 {
			return semantic
		}
	}

	// Structural normalization for content without SEMANTIC block.
	normalized := content

	// Strip suggestion-type prefixes added by formatProactiveSuggestionContent().
	// These add narrative framing that dilutes embedding signal.
	suggestionPrefixes := []string{
		"Previous solution to similar problem: ",
		"Risk warning: ",
		"Related pattern in this codebase: ",
		"Architectural constraint that applies: ",
		"Potential conflict with existing: ",
		"Relevant context: ",
		"Based on this codebase's patterns: ",
		"The typical workflow for this type of change: ",
		"This relates to the higher-level principle: ",
		"Caution - previous related finding: ",
	}
	for _, prefix := range suggestionPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			normalized = normalized[len(prefix):]
			break
		}
	}
	// Also strip "Related <trigger> pattern: " (dynamic prefix)
	if strings.HasPrefix(normalized, "Related ") && strings.Contains(normalized[:min(len(normalized), 60)], " pattern: ") {
		if idx := strings.Index(normalized, " pattern: "); idx >= 0 && idx < 50 {
			normalized = normalized[idx+len(" pattern: "):]
		}
	}

	// Remove duplicate name prefix from Source E assembly:
	// "error-sanitizer: Module: error-sanitizer. Related to: ..." → strip the "name: " prefix
	// when the name is repeated in the body.
	if colonIdx := strings.Index(normalized, ": "); colonIdx > 0 && colonIdx < 80 {
		name := normalized[:colonIdx]
		rest := normalized[colonIdx+2:]
		if strings.Contains(rest, name) {
			normalized = rest
		}
	}

	// Convert structural patterns to prose.
	var parts []string

	// Split on sentence-ending periods followed by a keyword
	segments := splitStructuredSegments(normalized)

	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		parts = append(parts, normalizeSegment(seg))
	}

	result := strings.Join(parts, ", ")
	if result == "" {
		return content // fallback to original
	}
	return result
}

// splitStructuredSegments splits a structured summary into its component segments.
// Input: "Module: error-sanitizer. Related to: a, b. Key functions: f1, f2"
// Output: ["Module: error-sanitizer", "Related to: a, b", "Key functions: f1, f2"]
func splitStructuredSegments(s string) []string {
	// Keywords that start new segments when preceded by ". "
	keywords := []string{
		"Related to:",
		"Key functions:",
		"Key methods:",
		"Key attributes:",
		"Defines classes:",
		"Inherits from:",
		"Description:",
	}

	var segments []string
	remaining := s

	for len(remaining) > 0 {
		// Find the earliest keyword boundary
		earliestIdx := -1
		earliestLen := 0
		for _, kw := range keywords {
			// Check for ". Keyword" or " Keyword" at segment boundary
			patterns := []string{". " + kw, "." + kw}
			for _, pat := range patterns {
				idx := strings.Index(remaining, pat)
				if idx >= 0 && (earliestIdx < 0 || idx < earliestIdx) {
					earliestIdx = idx
					earliestLen = len(pat) - len(kw) // length of separator before keyword
				}
			}
		}

		if earliestIdx < 0 {
			// No more keywords — rest is one segment
			segments = append(segments, remaining)
			break
		}

		// Split at the boundary
		segments = append(segments, remaining[:earliestIdx])
		remaining = remaining[earliestIdx+earliestLen:]
	}

	return segments
}

// normalizeSegment converts a single structured segment to natural language.
func normalizeSegment(seg string) string {
	// Entity type prefixes → natural prose
	entityPrefixes := []struct {
		prefix string
		format string
	}{
		{"Module: ", "%s module"},
		{"Class ", "%s"},
		{"Function ", "%s"},
		{"Interface ", "%s"},
		{"Struct ", "%s"},
		{"Documentation: ", "%s documentation"},
		{"device_function: ", "%s device function"},
		{"c-header: ", "%s header"},
	}
	for _, ep := range entityPrefixes {
		if strings.HasPrefix(seg, ep.prefix) {
			name := strings.TrimSpace(seg[len(ep.prefix):])
			// Strip trailing location info like "in module_name"
			return strings.Replace(ep.format, "%s", name, 1)
		}
	}

	// "Related to: a, b, c" → "for a, b, and c"
	if strings.HasPrefix(seg, "Related to: ") {
		items := seg[len("Related to: "):]
		return "for " + naturalJoin(items)
	}

	// "Key functions: f1, f2" → "with functions f1 and f2"
	if strings.HasPrefix(seg, "Key functions: ") {
		items := seg[len("Key functions: "):]
		return "with functions " + naturalJoin(items)
	}
	if strings.HasPrefix(seg, "Key methods: ") {
		items := seg[len("Key methods: "):]
		return "with methods " + naturalJoin(items)
	}
	if strings.HasPrefix(seg, "Key attributes: ") {
		items := seg[len("Key attributes: "):]
		return "with attributes " + naturalJoin(items)
	}
	if strings.HasPrefix(seg, "Defines classes: ") {
		items := seg[len("Defines classes: "):]
		return "defines " + naturalJoin(items)
	}
	if strings.HasPrefix(seg, "Inherits from: ") {
		items := seg[len("Inherits from: "):]
		return "inherits from " + naturalJoin(items)
	}
	if strings.HasPrefix(seg, "Description: ") {
		return seg[len("Description: "):]
	}

	// No recognized pattern — return as-is
	return seg
}

// naturalJoin converts "a, b, c" to "a, b, and c".
func naturalJoin(csv string) string {
	items := strings.Split(csv, ", ")
	switch len(items) {
	case 0:
		return csv
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	default:
		return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
	}
}
