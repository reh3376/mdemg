package cli

import (
	"encoding/json"
	"testing"

	"mdemg/internal/sidecar"
)

func TestQuickstartReport_NilSliceSafety(t *testing.T) {
	report := quickstartReport{
		ReportEnvelope: sidecar.NewReportEnvelope(
			"mdemg sidecar quickstart",
			sidecar.StateUninitialized,
			sidecar.StateRunning,
			sidecar.ExitSuccess,
		),
		Steps: nil,
	}

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Envelope fields must be arrays, not null
	for _, field := range []string{"changes", "issues", "next_actions"} {
		v, ok := raw[field]
		if !ok {
			t.Errorf("missing field %q in JSON", field)
			continue
		}
		arr, ok := v.([]any)
		if !ok {
			t.Errorf("field %q is not an array, got %T", field, v)
			continue
		}
		if len(arr) != 0 {
			t.Errorf("field %q should be empty, got %v", field, arr)
		}
	}

	// Steps field: nil Go slice → null JSON is acceptable here,
	// but we check the report is still valid JSON.
	if _, ok := raw["steps"]; !ok {
		t.Error("missing 'steps' field in JSON")
	}
}

func TestQuickstartStep_JSON(t *testing.T) {
	step := quickstartStep{
		Name:    "init",
		Status:  "done",
		Message: "initialized",
	}

	data, err := json.Marshal(step)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if raw["name"] != "init" {
		t.Errorf("name = %q, want init", raw["name"])
	}
	if raw["status"] != "done" {
		t.Errorf("status = %q, want done", raw["status"])
	}
	if raw["message"] != "initialized" {
		t.Errorf("message = %q, want initialized", raw["message"])
	}
}
