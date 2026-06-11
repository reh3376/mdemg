package config

import (
	"strings"
	"testing"
)

// CONFIG-DEADFLAG-001: invalid boolean env values must fail FromEnv loudly
// instead of silently parsing as false (the FEATURE_ENABLED=ture class).
func TestFromEnv_StrictBool_InvalidValueFails(t *testing.T) {
	t.Setenv("NEO4J_URI", "bolt://localhost:7687")
	t.Setenv("NEO4J_USER", "neo4j")
	t.Setenv("NEO4J_PASS", "x")
	t.Setenv("JIMINY_ENABLED", "ture") // typo

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected FromEnv to fail on invalid boolean")
	}
	if !strings.Contains(err.Error(), "JIMINY_ENABLED") {
		t.Fatalf("error must name the offending var: %v", err)
	}
}

func TestFromEnv_StrictBool_AccumulatesAllErrors(t *testing.T) {
	t.Setenv("NEO4J_URI", "bolt://localhost:7687")
	t.Setenv("NEO4J_USER", "neo4j")
	t.Setenv("NEO4J_PASS", "x")
	t.Setenv("JIMINY_ENABLED", "ture")
	t.Setenv("BACKUP_ENABLED", "1x")

	_, err := FromEnv()
	if err == nil {
		t.Fatal("expected FromEnv to fail")
	}
	for _, want := range []string{"JIMINY_ENABLED", "BACKUP_ENABLED"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should accumulate %s: %v", want, err)
		}
	}
}

func TestFromEnv_StrictBool_AcceptsCanonicalForms(t *testing.T) {
	t.Setenv("NEO4J_URI", "bolt://localhost:7687")
	t.Setenv("NEO4J_USER", "neo4j")
	t.Setenv("NEO4J_PASS", "x")
	for _, v := range []string{"true", "1", "yes", "on", "false", "0", "no", "off", "TRUE", " On "} {
		t.Setenv("JIMINY_ENABLED", v)
		if _, err := FromEnv(); err != nil {
			t.Errorf("value %q should parse: %v", v, err)
		}
	}
}
