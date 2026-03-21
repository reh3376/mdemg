package jiminy

import (
	"fmt"
	"strings"

	"mdemg/internal/sanitize"
)

// guidanceSystemPrompt is the enriched J15 system prompt for LLM synthesis.
const guidanceSystemPrompt = `You are Jiminy, the cognitive guidance system for an AI coding agent. You synthesize
evidence from the organization's knowledge graph into actionable guidance that constrains
agent behavior. You have deep expertise ONLY in the patterns, decisions, constraints,
and history stored in this organization's knowledge graph.

Rules:
1. CONSTRAINTS are organizational rules from the knowledge graph:
   - "must" constraints are NON-NEGOTIABLE directives: "You MUST..."
   - "must_not" constraints are ABSOLUTE prohibitions: "You MUST NOT..."
   - "should" constraints are RECOMMENDATIONS: "You SHOULD... (recommended)"
   - "should_not" constraints are DISCOURAGEMENTS: "You SHOULD NOT... (discouraged)"
2. CORRECTIONS are past-mistake lessons: "Previously, [mistake]. The correct approach is..."
3. PATTERNS are established conventions: "In this codebase, the convention is..."
4. CONFLICTS indicate contradictions requiring resolution before proceeding.
5. DECISIONS are past architectural choices with rationale.
6. CONCEPTS are higher-level organizational patterns from the knowledge graph's hidden layers.
7. FRONTIERS are knowledge gaps where no stored knowledge exists.

Output rules:
- Cite every claim as (Node: <node_id>)
- Do NOT invent information not present in the evidence
- Lead with the most critical constraints and corrections
- Surface risks prominently with WARNING prefix
- Be concise and directive — maximum 500 words
- Order by urgency: CONSTRAINTS > CORRECTIONS > CONFLICTS > DECISIONS > PATTERNS > CONCEPTS > FRONTIERS
- Output a short, directive narrative — not a bulleted list`

// ollamaSynthesisSchema is the JSON schema for Ollama structured synthesis output.
var ollamaSynthesisSchema = []byte(`{
	"type": "object",
	"properties": {
		"narrative": {"type": "string"},
		"critical_constraints": {"type": "integer"},
		"risk_level": {"type": "string", "enum": ["none", "low", "medium", "high"]}
	},
	"required": ["narrative"]
}`)

// buildGuidancePrompt constructs the user prompt for LLM synthesis (J15 enriched).
func buildGuidancePrompt(items []GuidanceItem, context, agentOutput string) string {
	var sb strings.Builder

	// Sanitize user context
	ctx := sanitize.SanitizeUserContext(context, 2000)
	sb.WriteString("## Agent Context\n\n")
	sb.WriteString(ctx)
	sb.WriteString("\n\n")

	if agentOutput != "" {
		sanitized := sanitize.SanitizeUserContext(agentOutput, 1000)
		sb.WriteString("## Agent Output (proposed action)\n\n")
		sb.WriteString(sanitized)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## Evidence from Knowledge Graph\n\n")

	// Group items by type for clearer presentation
	groups := map[GuidanceType][]GuidanceItem{}
	for _, item := range items {
		groups[item.Type] = append(groups[item.Type], item)
	}

	typeOrder := []GuidanceType{
		GuidanceConstraint, GuidanceCorrection, GuidanceConflict,
		GuidanceRisk, GuidanceDecision, GuidancePattern,
		GuidanceLearning, GuidancePreference, GuidanceConcept,
		GuidanceFrontier, GuidanceSuggestion,
	}

	// Track whether we have organizational concepts for a separate section
	var conceptItems []GuidanceItem

	for _, gType := range typeOrder {
		groupItems, ok := groups[gType]
		if !ok || len(groupItems) == 0 {
			continue
		}

		// Separate organizational concepts (L2+ GuidanceConcept) into their own section
		if gType == GuidanceConcept {
			conceptItems = groupItems
			continue
		}

		fmt.Fprintf(&sb, "### %s\n", strings.ToUpper(string(gType)))
		for _, item := range groupItems {
			nodeRef := ""
			if len(item.SourceNodes) > 0 {
				nodeRef = fmt.Sprintf(" (Node: %s)", item.SourceNodes[0])
			}
			// J15: Show constraint sub-type labels and confidence scores
			if gType == GuidanceConstraint {
				subType := extractConstraintSubType(item.Content)
				if subType != "" {
					fmt.Fprintf(&sb, "- [%s] [conf: %.2f] %s%s\n", subType, item.Confidence, item.Content, nodeRef)
				} else {
					fmt.Fprintf(&sb, "- [conf: %.2f] %s%s\n", item.Confidence, item.Content, nodeRef)
				}
			} else {
				fmt.Fprintf(&sb, "- [conf: %.2f] %s%s\n", item.Confidence, item.Content, nodeRef)
			}
		}
		sb.WriteString("\n")
	}

	// J15: Separate section for organizational concepts (L2+ nodes)
	if len(conceptItems) > 0 {
		sb.WriteString("### ORGANIZATIONAL CONCEPTS (from knowledge graph hidden layers)\n")
		for _, item := range conceptItems {
			nodeRef := ""
			if len(item.SourceNodes) > 0 {
				nodeRef = fmt.Sprintf(" (Node: %s)", item.SourceNodes[0])
			}
			fmt.Fprintf(&sb, "- [conf: %.2f] %s%s\n", item.Confidence, item.Content, nodeRef)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("## Task\n\n")
	sb.WriteString("Synthesize the above evidence into a concise, actionable guidance narrative for the agent. ")
	sb.WriteString("Lead with the most critical constraints and corrections. ")
	sb.WriteString("Cite node IDs parenthetically.")

	return sb.String()
}

// extractConstraintSubType extracts "must", "should" etc. from "[must] ..." formatted content.
func extractConstraintSubType(content string) string {
	if len(content) > 2 && content[0] == '[' {
		idx := strings.Index(content, "]")
		if idx > 0 && idx < 20 {
			return content[1:idx]
		}
	}
	return ""
}
