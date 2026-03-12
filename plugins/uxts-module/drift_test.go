package main

import (
	"testing"
)

func TestCanonicalJSONBytes(t *testing.T) {
	obj := map[string]any{
		"z": "last",
		"a": "first",
		"m": map[string]any{
			"b": 2,
			"a": 1,
		},
	}

	data, err := canonicalJSONBytes(obj)
	if err != nil {
		t.Fatal(err)
	}

	got := string(data)
	// Go's json.Marshal sorts map keys alphabetically
	expected := `{"a":"first","m":{"a":1,"b":2},"z":"last"}`
	if got != expected {
		t.Errorf("canonicalJSONBytes:\n  got:  %s\n  want: %s", got, expected)
	}
}

func TestSHA256HexObj(t *testing.T) {
	obj := map[string]any{"hello": "world"}
	hash, err := sha256HexObj(obj)
	if err != nil {
		t.Fatal(err)
	}
	if len(hash) != 64 {
		t.Errorf("expected 64-char hex hash, got %d chars", len(hash))
	}
}

func TestSHA256SpecWithoutField(t *testing.T) {
	spec := map[string]any{
		"name": "test",
		"config": map[string]any{
			"timeout_ms": 5000,
			"sha256":     "placeholder",
		},
	}

	hash1, err := sha256SpecWithoutField(spec, []string{"config", "sha256"})
	if err != nil {
		t.Fatal(err)
	}

	// Original should be unchanged
	cfg := spec["config"].(map[string]any)
	if cfg["sha256"] != "placeholder" {
		t.Error("original spec was mutated")
	}

	// Hash should be deterministic
	hash2, err := sha256SpecWithoutField(spec, []string{"config", "sha256"})
	if err != nil {
		t.Fatal(err)
	}
	if hash1 != hash2 {
		t.Error("hash not deterministic")
	}
}

func TestDropPath(t *testing.T) {
	m := map[string]any{
		"a": map[string]any{
			"b": "value",
			"c": "keep",
		},
	}

	dropPath(m, []string{"a", "b"})

	inner := m["a"].(map[string]any)
	if _, exists := inner["b"]; exists {
		t.Error("field 'b' should have been removed")
	}
	if inner["c"] != "keep" {
		t.Error("field 'c' should still exist")
	}
}

func TestCheckDrift_Clean(t *testing.T) {
	spec := map[string]any{
		"name": "test",
		"config": map[string]any{
			"timeout_ms": 5000,
		},
	}

	// Compute the correct hash
	hash, err := sha256SpecWithoutField(spec, []string{"config", "sha256"})
	if err != nil {
		t.Fatal(err)
	}

	// Set it
	spec["config"].(map[string]any)["sha256"] = hash

	result := checkDrift(spec, []string{"config", "sha256"})
	if result != "clean" {
		t.Errorf("expected clean, got %s", result)
	}
}

func TestCheckDrift_Detected(t *testing.T) {
	spec := map[string]any{
		"name": "test",
		"config": map[string]any{
			"sha256": "wrong_hash_value",
		},
	}

	result := checkDrift(spec, []string{"config", "sha256"})
	if result != "drift_detected" {
		t.Errorf("expected drift_detected, got %s", result)
	}
}

func TestCheckDrift_NoHash(t *testing.T) {
	spec := map[string]any{
		"name": "test",
		"config": map[string]any{
			"timeout_ms": 5000,
		},
	}

	result := checkDrift(spec, []string{"config", "sha256"})
	if result != "clean" {
		t.Errorf("expected clean (no hash), got %s", result)
	}
}

func TestGetNestedString(t *testing.T) {
	m := map[string]any{
		"config": map[string]any{
			"sha256": "abc123",
		},
	}

	got := getNestedString(m, []string{"config", "sha256"})
	if got != "abc123" {
		t.Errorf("expected abc123, got %s", got)
	}

	got = getNestedString(m, []string{"config", "missing"})
	if got != "" {
		t.Errorf("expected empty for missing field, got %s", got)
	}

	got = getNestedString(m, []string{"missing", "sha256"})
	if got != "" {
		t.Errorf("expected empty for missing parent, got %s", got)
	}
}
