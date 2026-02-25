package guardrail

import (
	"fmt"
	"strings"
)

// guardrailSystemPrompt defines the LLM's role as a guardrail evaluator.
// It biases toward Pass — only genuine violations should be flagged.
const guardrailSystemPrompt = `You are a code guardrail evaluator. You check proposed code changes against organizational constraints.

Rules for evaluation:
- "must" constraints: violation if the diff CLEARLY contradicts the requirement. Only flag if there is direct evidence of violation.
- "must_not" constraints: violation if the diff CLEARLY does something that is explicitly forbidden. Only flag if there is direct evidence of violation.
- "should" constraints: warning only if the diff appears to deviate from a recommendation. Use judgment — minor deviations are acceptable.
- "should_not" constraints: warning only if the diff appears to do something discouraged. Use judgment — minor deviations are acceptable.
- "deadline" constraints: warning only if the diff explicitly ignores or removes deadline-related code.

IMPORTANT: When in doubt, do NOT flag. False positives are worse than false negatives.
The diff may be partial — do not assume violations from missing context.

Respond with ONLY valid JSON (no markdown fences, no explanation):
{"violations": [...], "warnings": [...]}

Each violation/warning object: {"constraint_node_id": "...", "description": "...", "rationale": "..."}
If no violations and no warnings, respond: {"violations": [], "warnings": []}`

// buildEvalPrompt constructs the user prompt for LLM evaluation.
func buildEvalPrompt(diffCtx DiffContext, constraints []constraintMatch) string {
	var sb strings.Builder

	// Section 1: Constraints
	sb.WriteString("## Active Constraints\n\n")
	for i, c := range constraints {
		content := truncateString(c.Content, 500)
		sb.WriteString(fmt.Sprintf("[%d] node_id: %s\n", i+1, c.NodeID))
		sb.WriteString(fmt.Sprintf("    type: %s\n", c.ConstraintType))
		if c.Name != "" {
			sb.WriteString(fmt.Sprintf("    name: %s\n", c.Name))
		}
		sb.WriteString(fmt.Sprintf("    confidence: %.2f\n", c.Confidence))
		sb.WriteString(fmt.Sprintf("    content: %s\n\n", content))
	}

	// Section 2: Diff
	sb.WriteString("## Proposed Changes\n\n")
	if len(diffCtx.FilePaths) > 0 {
		sb.WriteString("Files: ")
		sb.WriteString(strings.Join(diffCtx.FilePaths, ", "))
		sb.WriteString("\n\n")
	}
	if len(diffCtx.FunctionNames) > 0 {
		sb.WriteString("Functions modified: ")
		sb.WriteString(strings.Join(diffCtx.FunctionNames, ", "))
		sb.WriteString("\n\n")
	}

	sb.WriteString("```diff\n")
	sb.WriteString(truncateString(rebuildDiff(diffCtx), 3000))
	sb.WriteString("\n```\n")

	return sb.String()
}

// rebuildDiff reconstructs a simplified diff from added/removed lines.
func rebuildDiff(ctx DiffContext) string {
	var sb strings.Builder
	for _, line := range ctx.RemovedLines {
		sb.WriteString("-")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	for _, line := range ctx.AddedLines {
		sb.WriteString("+")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	return sb.String()
}
