package sidecar

import (
	"encoding/json"
	"testing"
)

func TestReportEnvelope_NilSliceSafety(t *testing.T) {
	env := NewReportEnvelope("mdemg sidecar status", StateRunning, StateRunning, ExitSuccess)

	data, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	// Verify slices serialize as [] not null
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

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
}

func TestExitCodes(t *testing.T) {
	codes := map[string]int{
		"success":    ExitSuccess,
		"validation": ExitValidation,
		"dependency": ExitDependency,
		"runtime":    ExitRuntime,
		"permission": ExitPermissions,
		"adapter":    ExitAdapterConflict,
	}
	expected := map[string]int{
		"success":    0,
		"validation": 2,
		"dependency": 3,
		"runtime":    4,
		"permission": 5,
		"adapter":    6,
	}
	for name, got := range codes {
		want := expected[name]
		if got != want {
			t.Errorf("Exit%s = %d, want %d", name, got, want)
		}
	}
}

func TestNowUTC_Format(t *testing.T) {
	ts := NowUTC()
	if len(ts) < 20 {
		t.Errorf("NowUTC() = %q, too short for RFC3339", ts)
	}
}
