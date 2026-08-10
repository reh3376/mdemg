package jiminy

import (
	"context"
	"fmt"
	"mdemg/internal/sanitize"
	"strings"
)

// StrictClassifier determines whether an agent action should be allowed or denied
// in /strict mode. Uses the Evaluator for constraint matching (Tier 1 only — no LLM).
// Graduated enforcement: only constraints at WARNED+ escalation level can trigger denial.
//
// JIMINY-ENFORCE-003: if an OverrideManager is wired, the classifier consults it
// after computing the deny verdict. Overrides that match one or more violated
// constraint_codes remove those codes from the deny set; if the entire deny set
// is overridden, the verdict is downgraded to pass with the override reasons
// recorded in DenialReason (so operators see WHY the classifier would have
// denied, plus who overrode and why).
type StrictClassifier struct {
	evaluator  *Evaluator
	escalation *EscalationTracker
	overrides  *OverrideManager
}

// NewStrictClassifier creates a new strict classifier.
func NewStrictClassifier(evaluator *Evaluator, escalation *EscalationTracker) *StrictClassifier {
	return &StrictClassifier{
		evaluator:  evaluator,
		escalation: escalation,
	}
}

// SetOverrides wires the operator escape-hatch (JIMINY-ENFORCE-003).
// Safe to call with nil — no overrides, current behavior preserved.
func (c *StrictClassifier) SetOverrides(m *OverrideManager) {
	c.overrides = m
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
			// JIMINY-CLASSIFY-ESCALATION-INSPECT-001 (2026-08-10): read the
			// real constraint_code from the evaluator item (populated from
			// Neo4j's constraint_code property). Previously this called a
			// buggy extractConstraintCode(content) helper that pulled the
			// `[must]`/`[should]` severity marker prefix off the content
			// string and treated THAT as the code — so overrides on `must`
			// would silently apply to every must-severity rule instead of
			// the specific offending one. If the source node lacks a code
			// (legacy Neo4j rows), fall back to `node:<source_node>` so
			// the operator can still target the specific finding via the
			// pseudo-code.
			code := item.ConstraintCode
			if code == "" && item.SourceNode != "" {
				code = "node:" + item.SourceNode
			}
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
		// JIMINY-ENFORCE-003: consult the operator override manager. An active
		// override on a violated constraint_code downgrades that specific
		// finding to pass; if EVERY violated code has an active override, the
		// verdict becomes pass with the reasons annotated so the audit trail
		// records WHY the block would have fired + WHO overrode it.
		var unoverriddenCodes []string
		var unoverriddenReasons []string
		var overrideAnnotations []string
		if c.overrides != nil && req.SessionID != "" {
			for i, code := range violatedCodes {
				if code == "" {
					// Can't override a code we couldn't extract — keep the finding.
					if i < len(denialReasons) {
						unoverriddenReasons = append(unoverriddenReasons, denialReasons[i])
					}
					continue
				}
				if ovr := c.overrides.Get(req.SessionID, code); ovr != nil {
					overrideAnnotations = append(overrideAnnotations,
						fmt.Sprintf("[override:%s reason=%q]", code, ovr.Reason))
					continue
				}
				unoverriddenCodes = append(unoverriddenCodes, code)
				if i < len(denialReasons) {
					unoverriddenReasons = append(unoverriddenReasons, denialReasons[i])
				}
			}
			// Denial reasons with no matched code: keep them (can't check override
			// without a code key). This is the extract-failure branch above.
			if len(violatedCodes) == 0 {
				unoverriddenReasons = denialReasons
			}
		} else {
			unoverriddenCodes = violatedCodes
			unoverriddenReasons = denialReasons
		}

		if len(unoverriddenCodes) == 0 && len(unoverriddenReasons) == 0 {
			// Fully-overridden: pass with the override annotations so the audit
			// trail records what WOULD have been blocked and by whom.
			ann := strings.Join(overrideAnnotations, "; ")
			return ClassifyResponse{
				Verdict:      "pass",
				DenialReason: fmt.Sprintf("override-suppressed: %s", ann),
				Confidence:   0.9,
			}, nil
		}

		// JIMINY-CLASSIFY-ESCALATION-INSPECT-001: prepend [code=X] to each
		// reason so operators reading the block message can see which
		// constraint_code(s) to target with `mdemg jiminy override apply
		// --constraint <CODE>`. Codes and reasons zip pairwise when both
		// arrays are the same length; if lengths diverge (extract-failure
		// branch merged reasons without codes), fall back to naked reasons.
		annotatedReasons := make([]string, 0, len(unoverriddenReasons))
		if len(unoverriddenCodes) == len(unoverriddenReasons) {
			for i, r := range unoverriddenReasons {
				annotatedReasons = append(annotatedReasons,
					fmt.Sprintf("[code=%s] %s", unoverriddenCodes[i], r))
			}
		} else {
			annotatedReasons = unoverriddenReasons
		}
		reason := fmt.Sprintf("Constraint violation (%s): %s",
			maxEscLevel, strings.Join(annotatedReasons, "; "))
		if len(overrideAnnotations) > 0 {
			// Partial override: annotate but still deny.
			reason = fmt.Sprintf("%s (partial-override-suppressed: %s)", reason, strings.Join(overrideAnnotations, "; "))
		}
		reason = sanitize.CutRuneSafeSuffix(reason, 500, "...")
		return ClassifyResponse{
			Verdict:         "deny",
			DenialReason:    reason,
			ViolatedCodes:   unoverriddenCodes,
			Confidence:      0.8,
			EscalationLevel: maxEscLevel,
		}, nil
	}

	return ClassifyResponse{
		Verdict:    "pass",
		Confidence: 0.9,
	}, nil
}

