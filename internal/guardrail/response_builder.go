package guardrail

// buildResponse maps LLM evaluation results to the final ValidateResponse.
// It re-validates LLM output against actual constraint types to prevent
// the LLM from incorrectly marking a "should" constraint as a violation.
func buildResponse(llmResult *llmEvalResult, constraints []constraintMatch) *ValidateResponse {
	constraintMap := buildConstraintMap(constraints)

	var violations []GuardrailViolation
	var warnings []GuardrailViolation

	// Process violations from LLM — re-validate against constraint type
	for _, v := range llmResult.Violations {
		gv := GuardrailViolation{
			ConstraintNodeID: v.ConstraintNodeID,
			Description:      v.Description,
			Rationale:        v.Rationale,
		}

		c, found := constraintMap[v.ConstraintNodeID]
		if !found {
			// Unknown constraint_node_id from LLM → demote to warning
			gv.Rationale = gv.Rationale + " (constraint node not found in retrieved set; demoted to warning)"
			warnings = append(warnings, gv)
			continue
		}

		if isBlockingType(c.ConstraintType) {
			// must/must_not → keep as violation
			violations = append(violations, gv)
		} else {
			// should/should_not/deadline/other → demote to warning
			gv.Rationale = gv.Rationale + " (demoted: constraint type is " + c.ConstraintType + ")"
			warnings = append(warnings, gv)
		}
	}

	// Process warnings from LLM — keep as warnings regardless
	for _, w := range llmResult.Warnings {
		warnings = append(warnings, GuardrailViolation{
			ConstraintNodeID: w.ConstraintNodeID,
			Description:      w.Description,
			Rationale:        w.Rationale,
		})
	}

	// Ensure non-nil slices
	if violations == nil {
		violations = []GuardrailViolation{}
	}
	if warnings == nil {
		warnings = []GuardrailViolation{}
	}

	// Determine status
	status := "Pass"
	if len(violations) > 0 {
		status = "Block"
	} else if len(warnings) > 0 {
		status = "Warning"
	}

	return &ValidateResponse{
		Status:     status,
		Violations: violations,
		Warnings:   warnings,
	}
}
