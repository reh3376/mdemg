// Package neo4jutil provides shared type conversion helpers for Neo4j query results.
// These functions safely convert interface{} values returned by the Neo4j Go driver
// into Go types, handling nil values and type mismatches gracefully.
package neo4jutil

import (
	"fmt"
	"time"
)

// AsString safely converts interface{} to string.
func AsString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

// AsFloat64 safely converts interface{} to float64.
func AsFloat64(v any) float64 {
	if v == nil {
		return 0.0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return 0.0
	}
}

// AsInt safely converts interface{} to int.
func AsInt(v any) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int64:
		return int(n)
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}

// AsBool safely converts interface{} to bool.
func AsBool(v any) bool {
	if v == nil {
		return false
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return false
}

// AsTime safely converts interface{} to time.Time.
// Handles time.Time directly and types with a Time() method (e.g. neo4j.LocalDateTime).
func AsTime(v any) time.Time {
	if v == nil {
		return time.Time{}
	}
	if t, ok := v.(time.Time); ok {
		return t
	}
	if dt, ok := v.(interface{ Time() time.Time }); ok {
		return dt.Time()
	}
	return time.Time{}
}

// AsStringSlice safely converts interface{} to []string.
func AsStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	if arr, ok := v.([]any); ok {
		result := make([]string, 0, len(arr))
		for _, item := range arr {
			result = append(result, AsString(item))
		}
		return result
	}
	if arr, ok := v.([]string); ok {
		return arr
	}
	return nil
}

// AsFloat32Slice safely converts interface{} to []float32.
func AsFloat32Slice(v any) []float32 {
	if v == nil {
		return nil
	}
	if arr, ok := v.([]any); ok {
		result := make([]float32, 0, len(arr))
		for _, item := range arr {
			result = append(result, float32(AsFloat64(item)))
		}
		return result
	}
	if arr, ok := v.([]float32); ok {
		return arr
	}
	if arr, ok := v.([]float64); ok {
		result := make([]float32, 0, len(arr))
		for _, f := range arr {
			result = append(result, float32(f))
		}
		return result
	}
	return nil
}

// AsFloat64Slice safely converts interface{} to []float64.
func AsFloat64Slice(v any) []float64 {
	if v == nil {
		return nil
	}
	if arr, ok := v.([]any); ok {
		result := make([]float64, 0, len(arr))
		for _, item := range arr {
			result = append(result, AsFloat64(item))
		}
		return result
	}
	if arr, ok := v.([]float64); ok {
		return arr
	}
	return nil
}
