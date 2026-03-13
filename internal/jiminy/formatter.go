package jiminy

import (
	"fmt"
	"strings"
)

// FormatPromptAugmentation renders guidance items as injectable markdown
// matching the existing ═══ CMS RECALL ═══ style.
func FormatPromptAugmentation(items []GuidanceItem, counts SourceCounts, confidence float64) string {
	if len(items) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("═══ JIMINY GUIDANCE ═══\n")

	// Group items by type
	constraints := filterByType(items, GuidanceConstraint)
	corrections := filterByType(items, GuidanceCorrection)
	patterns := filterByType(items, GuidancePattern, GuidanceSuggestion)
	conflicts := filterByType(items, GuidanceConflict, GuidanceRisk)
	frontiers := filterByType(items, GuidanceFrontier)

	if len(constraints) > 0 {
		sb.WriteString("CONSTRAINTS:\n")
		for _, item := range constraints {
			sb.WriteString(fmt.Sprintf("  • %s (conf: %.2f)\n", item.Content, item.Confidence))
		}
	}

	if len(corrections) > 0 {
		sb.WriteString("CORRECTIONS:\n")
		for _, item := range corrections {
			sb.WriteString(fmt.Sprintf("  • Prior correction: %s (conf: %.2f)\n", item.Content, item.Confidence))
		}
	}

	if len(patterns) > 0 {
		sb.WriteString("PATTERNS:\n")
		for _, item := range patterns {
			sb.WriteString(fmt.Sprintf("  • %s (conf: %.2f)\n", item.Content, item.Confidence))
		}
	}

	if len(conflicts) > 0 {
		sb.WriteString("CONFLICTS:\n")
		for _, item := range conflicts {
			sb.WriteString(fmt.Sprintf("  • %s (conf: %.2f)\n", item.Content, item.Confidence))
		}
	}

	if len(frontiers) > 0 {
		sb.WriteString("FRONTIERS:\n")
		for _, item := range frontiers {
			sb.WriteString(fmt.Sprintf("  • %s (conf: %.2f)\n", item.Content, item.Confidence))
		}
	}

	// Summary line
	parts := []string{}
	if counts.Constraints > 0 {
		parts = append(parts, fmt.Sprintf("%d constraints", counts.Constraints))
	}
	if counts.Corrections > 0 {
		parts = append(parts, fmt.Sprintf("%d corrections", counts.Corrections))
	}
	if counts.Patterns > 0 {
		parts = append(parts, fmt.Sprintf("%d patterns", counts.Patterns))
	}
	if counts.Conflicts > 0 {
		parts = append(parts, fmt.Sprintf("%d conflicts", counts.Conflicts))
	}
	if counts.Frontiers > 0 {
		parts = append(parts, fmt.Sprintf("%d frontiers", counts.Frontiers))
	}

	sb.WriteString(fmt.Sprintf("[confidence: %.2f | sources: %s]\n", confidence, strings.Join(parts, ", ")))
	sb.WriteString("═══ END JIMINY GUIDANCE ═══")

	return sb.String()
}

// filterByType returns items matching any of the given types.
func filterByType(items []GuidanceItem, types ...GuidanceType) []GuidanceItem {
	var result []GuidanceItem
	for _, item := range items {
		for _, t := range types {
			if item.Type == t {
				result = append(result, item)
				break
			}
		}
	}
	return result
}

// priorityRank returns a numeric rank for sorting (lower = higher priority).
func priorityRank(p string) int {
	switch p {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 2
	default:
		return 3
	}
}
