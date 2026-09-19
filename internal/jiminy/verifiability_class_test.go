// Sprint JIMINY-METRIC-PARTITION-001 (task #158) — pin tests for the
// VerifiabilityClass type + primaryVerifiabilityClass helper + the
// ClassPersistsToConstraintOutcomes contract.

package jiminy

import (
	"testing"
)

func TestVerifiabilityClass_Enum_Valid(t *testing.T) {
	for _, class := range []VerifiabilityClass{
		VerifiabilityClassifier,
		VerifiabilityProcess,
		VerifiabilityHybrid,
		VerifiabilityHuman,
	} {
		if !IsValidVerifiabilityClass(string(class)) {
			t.Errorf("expected %q to be valid", class)
		}
	}
}

func TestVerifiabilityClass_Enum_Invalid(t *testing.T) {
	for _, s := range []string{"", "unknown", "CLASSIFIER", " classifier ", "informational"} {
		if IsValidVerifiabilityClass(s) {
			t.Errorf("expected %q to be INVALID", s)
		}
	}
}

func TestAllVerifiabilityClasses_Order(t *testing.T) {
	got := AllVerifiabilityClasses()
	want := []VerifiabilityClass{
		VerifiabilityClassifier,
		VerifiabilityProcess,
		VerifiabilityHybrid,
		VerifiabilityHuman,
	}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestClassPersistsToConstraintOutcomes(t *testing.T) {
	cases := map[VerifiabilityClass]bool{
		VerifiabilityClassifier: true,
		VerifiabilityProcess:    false,
		VerifiabilityHybrid:     true,
		VerifiabilityHuman:      false,
	}
	for class, want := range cases {
		if got := ClassPersistsToConstraintOutcomes(class); got != want {
			t.Errorf("ClassPersistsToConstraintOutcomes(%q) = %v, want %v", class, got, want)
		}
	}
	// Unknown class defaults to non-persistent (safe: don't silently persist
	// a class we can't grade correctly).
	if got := ClassPersistsToConstraintOutcomes(VerifiabilityClass("unknown")); got {
		t.Errorf("expected unknown class NOT to persist; got true")
	}
}

func TestPrimaryVerifiabilityClass_HappyPath(t *testing.T) {
	classMap := map[string]VerifiabilityClass{
		"node_a": VerifiabilityProcess,
		"node_b": VerifiabilityClassifier,
	}
	s := &Service{}
	got := s.primaryVerifiabilityClass([]string{"node_a", "node_b"}, classMap)
	if got != VerifiabilityProcess {
		t.Errorf("got %q, want %q (first non-empty source wins)", got, VerifiabilityProcess)
	}
}

func TestPrimaryVerifiabilityClass_MissingKey_DefaultsToClassifier(t *testing.T) {
	classMap := map[string]VerifiabilityClass{}
	s := &Service{}
	got := s.primaryVerifiabilityClass([]string{"unmapped_node"}, classMap)
	if got != VerifiabilityClassifier {
		t.Errorf("got %q, want %q (unmapped → classifier default)", got, VerifiabilityClassifier)
	}
}

func TestPrimaryVerifiabilityClass_EmptySources_DefaultsToClassifier(t *testing.T) {
	classMap := map[string]VerifiabilityClass{"node_a": VerifiabilityProcess}
	s := &Service{}

	// nil sources
	if got := s.primaryVerifiabilityClass(nil, classMap); got != VerifiabilityClassifier {
		t.Errorf("nil sources: got %q, want %q", got, VerifiabilityClassifier)
	}
	// empty slice
	if got := s.primaryVerifiabilityClass([]string{}, classMap); got != VerifiabilityClassifier {
		t.Errorf("empty sources: got %q, want %q", got, VerifiabilityClassifier)
	}
	// slice of empty strings
	if got := s.primaryVerifiabilityClass([]string{"", ""}, classMap); got != VerifiabilityClassifier {
		t.Errorf("empty-string sources: got %q, want %q", got, VerifiabilityClassifier)
	}
}

func TestLoadVerifiabilityClassMap_NilDriver(t *testing.T) {
	s := &Service{driver: nil}
	got := s.loadVerifiabilityClassMap(t.Context(), []string{"n1", "n2"})
	if len(got) != 0 {
		t.Errorf("expected empty map with nil driver, got %v", got)
	}
}

func TestLoadVerifiabilityClassMap_EmptyInput(t *testing.T) {
	s := &Service{driver: nil}
	got := s.loadVerifiabilityClassMap(t.Context(), []string{})
	if len(got) != 0 {
		t.Errorf("expected empty map with empty input, got %v", got)
	}
	got = s.loadVerifiabilityClassMap(t.Context(), nil)
	if len(got) != 0 {
		t.Errorf("expected empty map with nil input, got %v", got)
	}
	got = s.loadVerifiabilityClassMap(t.Context(), []string{"", "", ""})
	if len(got) != 0 {
		t.Errorf("expected empty map with all-empty ids, got %v", got)
	}
}
