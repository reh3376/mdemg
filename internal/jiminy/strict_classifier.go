package jiminy

import (
	"context"
	"fmt"
	"strings"
)

// StrictClassifier determines whether an agent action should be allowed or denied
// in /strict mode. Uses the Evaluator for constraint matching (Tier 1 only — no LLM).
// Graduated enforcement: only constraints at WARNED+ escalation level can trigger denial.
type StrictClassifier struct {
	evaluator  *Evaluator
	escalation *EscalationTracker
}

// NewStrictClassifier creates a new strict classifier.
func NewStrictClassifier(evaluator *Evaluator, escalation *EscalationTracker) *StrictClassifier {
	return &StrictClassifier{
		evaluator:  evaluator,
		escalation: escalation,
	}
}

// Classify determines whether an agent action should pass or be denied.
// Classification logic (graduated enforcement):
//  1. Evaluate agent output against constraints (vector similarity, no LLM)
//  2. If high-severity finding AND constraint is WARNED+ → deny
//  3. SURFACED constraints remain advisory (pass) — first-time constraints don't block
//  4. No high-severity findings → pass
func (c *StrictClassifier) Classify(ctx context.Context, req ClassifyRequest) (ClassifyResponse, error) {
	if c.evaluator == nil {
		return ClassifyResponse{Verdict: "pass", Confidence: 0}, nil
	}

	evalResp, err := c.evaluator.Evaluate(ctx, EvaluateRequest{
		SpaceID:     req.SpaceID,
		AgentOutput: req.AgentOutput,
		FilePath:    req.FilePath,
		ToolName:    req.ToolName,
		SessionID:   req.SessionID,
	})
	if err != nil {
		// Fail open — classification errors should not block the agent
		return ClassifyResponse{
			Verdict:    "pass",
			Confidence: 0,
		}, nil
	}

	// Check for high-severity findings with escalated constraints
	var violatedCodes []string
	var maxEscLevel EscalationLevel
	var denialReasons []string

	for _, item := range evalResp.Items {
		if item.Severity != "high" {
			continue
		}

		// Check escalation level for this constraint
		var escLevel EscalationLevel
		if c.escalation != nil && req.SessionID != "" && item.SourceNode != "" {
			escLevel = c.escalation.GetState(req.SessionID, item.SourceNode)
		}

		// Graduated: only WARNED+ triggers denial
		if escLevel == EscalationWarned || escLevel == EscalationEscalated || escLevel == EscalationBlocked {
			// Extract constraint code from content if available
			code := extractConstraintCode(item.Content)
			if code != "" {
				violatedCodes = append(violatedCodes, code)
			}
			denialReasons = append(denialReasons, item.Content)

			if escalationOrd(escLevel) > escalationOrd(maxEscLevel) {
				maxEscLevel = escLevel
			}
		}
	}

	if len(violatedCodes) > 0 || len(denialReasons) > 0 {
		reason := fmt.Sprintf("Constraint violation (%s): %s",
			maxEscLevel, strings.Join(denialReasons, "; "))
		if len(reason) > 500 {
			reason = reason[:500] + "..."
		}
		return ClassifyResponse{
			Verdict:         "deny",
			DenialReason:    reason,
			ViolatedCodes:   violatedCodes,
			Confidence:      0.8,
			EscalationLevel: maxEscLevel,
		}, nil
	}

	return ClassifyResponse{
		Verdict:    "pass",
		Confidence: 0.9,
	}, nil
}

// extractConstraintCode tries to extract a constraint code from content text.
// Looks for patterns like [CODE123] or CODE123: at the beginning.
func extractConstraintCode(content string) string {
	// Check for [CODE] pattern
	if len(content) > 2 && content[0] == '[' {
		end := strings.IndexByte(content, ']')
		if end > 0 && end < 30 {
			return content[1:end]
		}
	}
	// Check for CODE: pattern at start
	if idx := strings.Index(content, ": "); idx > 0 && idx < 30 {
		candidate := content[:idx]
		if len(candidate) >= 3 && !strings.Contains(candidate, " ") {
			return candidate
		}
	}
	return ""
}
