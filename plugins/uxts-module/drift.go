package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
)

// canonicalJSONBytes produces deterministic JSON: sorted keys, compact separators.
// This matches the Python implementation in uxts_runner_core.py.
func canonicalJSONBytes(obj any) ([]byte, error) {
	// json.Marshal with sorted keys is the default in Go
	return json.Marshal(obj)
}

// sha256HexObj computes the SHA256 hex digest of the canonical JSON form.
func sha256HexObj(obj any) (string, error) {
	data, err := canonicalJSONBytes(obj)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum), nil
}

// sha256SpecWithoutField computes the SHA256 of a spec after removing a nested field.
// fieldPath is like ["config", "sha256"] — removes spec["config"]["sha256"] before hashing.
func sha256SpecWithoutField(spec map[string]any, fieldPath []string) (string, error) {
	// Deep copy via JSON round-trip
	data, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	var copy map[string]any
	if err := json.Unmarshal(data, &copy); err != nil {
		return "", err
	}

	dropPath(copy, fieldPath)
	return sha256HexObj(copy)
}

// dropPath removes a nested field from a map. For example,
// dropPath(m, ["config", "sha256"]) deletes m["config"]["sha256"].
func dropPath(m map[string]any, path []string) {
	if len(path) == 0 {
		return
	}
	if len(path) == 1 {
		delete(m, path[0])
		return
	}
	if next, ok := m[path[0]].(map[string]any); ok {
		dropPath(next, path[1:])
	}
}

// checkDrift verifies a spec's embedded hash against its computed hash.
// Returns "clean" if hashes match, "drift_detected" if they don't,
// or "clean" if no hash field is present (nothing to verify).
func checkDrift(spec map[string]any, hashFieldPath []string) string {
	// Extract the stored hash
	storedHash := getNestedString(spec, hashFieldPath)
	if storedHash == "" {
		return "clean" // No hash to verify
	}

	// Compute expected hash
	computed, err := sha256SpecWithoutField(spec, hashFieldPath)
	if err != nil {
		return "drift_detected"
	}

	if computed != storedHash {
		return "drift_detected"
	}
	return "clean"
}

// getNestedString retrieves a string value at a nested path.
func getNestedString(m map[string]any, path []string) string {
	if len(path) == 0 {
		return ""
	}
	if len(path) == 1 {
		if s, ok := m[path[0]].(string); ok {
			return s
		}
		return ""
	}
	if next, ok := m[path[0]].(map[string]any); ok {
		return getNestedString(next, path[1:])
	}
	return ""
}
