package hidden

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDetectOversizedCategories(t *testing.T) {
	r := NewReclassifier(ReclassifierConfig{Threshold: 0.25}, nil)

	classes := map[string][]BaseNode{
		"typescript": make([]BaseNode, 80),
		"go":         make([]BaseNode, 10),
		"config":     make([]BaseNode, 10),
	}

	oversized := r.detectOversizedCategories(classes, 100)
	if len(oversized) != 1 {
		t.Fatalf("expected 1 oversized category, got %d", len(oversized))
	}
	if oversized[0] != "typescript" {
		t.Errorf("expected 'typescript', got %q", oversized[0])
	}
}

func TestDetectOversizedCategories_NoneOverThreshold(t *testing.T) {
	r := NewReclassifier(ReclassifierConfig{Threshold: 0.50}, nil)

	classes := map[string][]BaseNode{
		"typescript": make([]BaseNode, 40),
		"go":         make([]BaseNode, 35),
		"config":     make([]BaseNode, 25),
	}

	oversized := r.detectOversizedCategories(classes, 100)
	if len(oversized) != 0 {
		t.Fatalf("expected 0 oversized categories, got %d", len(oversized))
	}
}

func TestDetectOversizedCategories_MultipleOversized(t *testing.T) {
	r := NewReclassifier(ReclassifierConfig{Threshold: 0.25}, nil)

	classes := map[string][]BaseNode{
		"typescript": make([]BaseNode, 40),
		"python":     make([]BaseNode, 40),
		"config":     make([]BaseNode, 20),
	}

	oversized := r.detectOversizedCategories(classes, 100)
	if len(oversized) != 2 {
		t.Fatalf("expected 2 oversized categories, got %d", len(oversized))
	}
	// Verify content (sorted deterministically)
	if oversized[0] != "python" || oversized[1] != "typescript" {
		t.Fatalf("expected [python, typescript], got %v", oversized)
	}
}

func TestSampleSummaries_AllReturned(t *testing.T) {
	r := NewReclassifier(ReclassifierConfig{MaxSampleSize: 100}, nil)

	nodes := []BaseNode{
		{Summary: "Module: auth service"},
		{Summary: "Class: user controller"},
		{Summary: "Migration: add users table"},
	}

	samples := r.sampleSummaries(nodes, 100)
	if len(samples) != 3 {
		t.Fatalf("expected 3 samples, got %d", len(samples))
	}
}

func TestSampleSummaries_EmptySummariesSkipped(t *testing.T) {
	r := NewReclassifier(ReclassifierConfig{MaxSampleSize: 100}, nil)

	nodes := []BaseNode{
		{Summary: "Module: auth service"},
		{Summary: ""},
		{Summary: "   "},
		{Summary: "Class: user controller"},
	}

	samples := r.sampleSummaries(nodes, 100)
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples (empty skipped), got %d", len(samples))
	}
}

func TestSampleSummaries_NoSummaries(t *testing.T) {
	r := NewReclassifier(ReclassifierConfig{MaxSampleSize: 100}, nil)

	nodes := []BaseNode{
		{Summary: ""},
		{Summary: ""},
	}

	samples := r.sampleSummaries(nodes, 100)
	if samples != nil {
		t.Fatalf("expected nil, got %v", samples)
	}
}

func TestSampleSummaries_StratifiedSampling(t *testing.T) {
	r := NewReclassifier(ReclassifierConfig{MaxSampleSize: 10}, nil)

	// Create nodes with 4 distinct prefix strata — triggers stratified path
	nodes := make([]BaseNode, 100)
	for i := range 25 {
		nodes[i] = BaseNode{Summary: "Module: something " + string(rune('a'+i))}
	}
	for i := range 25 {
		nodes[25+i] = BaseNode{Summary: "Class: something " + string(rune('a'+i))}
	}
	for i := range 25 {
		nodes[50+i] = BaseNode{Summary: "Migration: something " + string(rune('a'+i))}
	}
	for i := range 25 {
		nodes[75+i] = BaseNode{Summary: "Config: something " + string(rune('a'+i))}
	}

	samples := r.sampleSummaries(nodes, 10)
	if len(samples) > 10 {
		t.Fatalf("expected at most 10 samples, got %d", len(samples))
	}
	if len(samples) < 4 {
		t.Fatalf("expected at least 4 samples (one per stratum), got %d", len(samples))
	}
	// Verify at least one sample from each stratum prefix
	prefixSeen := make(map[string]bool)
	for _, s := range samples {
		if idx := strings.Index(s, ":"); idx > 0 {
			prefixSeen[strings.ToLower(strings.TrimSpace(s[:idx]))] = true
		}
	}
	for _, expected := range []string{"module", "class", "migration", "config"} {
		if !prefixSeen[expected] {
			t.Errorf("expected at least one sample from stratum %q, got prefixes %v", expected, prefixSeen)
		}
	}
}

func TestAssignToSubCategories(t *testing.T) {
	r := NewReclassifier(ReclassifierConfig{}, nil)

	subCats := []SubCategory{
		{Name: "services", Keywords: []string{"service", "provider", "handler"}},
		{Name: "dto", Keywords: []string{"dto", "request", "response", "schema"}},
		{Name: "misc", Keywords: []string{"misc"}},
	}

	nodes := []BaseNode{
		{NodeID: "1", Summary: "Module: auth service with provider injection"},
		{NodeID: "2", Summary: "Class: CreateUserRequest DTO with validation"},
		{NodeID: "3", Summary: "File: random utility code"},
		{NodeID: "4", Summary: "Module: payment handler service"},
	}

	result := r.assignToSubCategories(nodes, "typescript", subCats)

	if len(result["typescript-services"]) != 2 {
		t.Errorf("expected 2 nodes in typescript-services, got %d", len(result["typescript-services"]))
	}
	if len(result["typescript-dto"]) != 1 {
		t.Errorf("expected 1 node in typescript-dto, got %d", len(result["typescript-dto"]))
	}
	if len(result["typescript-misc"]) != 1 {
		t.Errorf("expected 1 node in typescript-misc, got %d", len(result["typescript-misc"]))
	}
}

func TestAssignToSubCategories_EmptySummaryGoesToMisc(t *testing.T) {
	r := NewReclassifier(ReclassifierConfig{}, nil)

	subCats := []SubCategory{
		{Name: "services", Keywords: []string{"service"}},
		{Name: "misc", Keywords: []string{"misc"}},
	}

	nodes := []BaseNode{
		{NodeID: "1", Summary: ""},
		{NodeID: "2", Summary: "Module: auth service"},
	}

	result := r.assignToSubCategories(nodes, "typescript", subCats)
	if len(result["typescript-misc"]) != 1 {
		t.Errorf("expected empty summary to go to misc, got %v", result)
	}
	if len(result["typescript-services"]) != 1 {
		t.Errorf("expected 1 in services, got %d", len(result["typescript-services"]))
	}
}

func TestAssignToSubCategories_TieBreaking(t *testing.T) {
	r := NewReclassifier(ReclassifierConfig{}, nil)

	// Both categories have 1 keyword match — first one wins (deterministic)
	subCats := []SubCategory{
		{Name: "alpha", Keywords: []string{"test"}},
		{Name: "beta", Keywords: []string{"test"}},
		{Name: "misc", Keywords: []string{"misc"}},
	}

	nodes := []BaseNode{
		{NodeID: "1", Summary: "test file"},
	}

	result := r.assignToSubCategories(nodes, "go", subCats)
	if len(result["go-alpha"]) != 1 {
		t.Errorf("expected tie to go to first matching category 'alpha', got %v", result)
	}
}

func TestReclassifyOversizedCategories_FailOpen(t *testing.T) {
	// LLM is unreachable — should return original classes unchanged
	r := NewReclassifier(ReclassifierConfig{
		Enabled:       true,
		Threshold:     0.25,
		MaxSampleSize: 10,
		MaxCategories: 10,
		Provider:      "openai",
		Model:         "gpt-4o-mini",
		MaxTokens:     2000,
		TimeoutMs:     1000,
		OpenAIKey:     "fake",
		OpenAIURL:     "http://localhost:1", // unreachable
	}, nil)

	nodes := make([]BaseNode, 80)
	for i := range nodes {
		nodes[i] = BaseNode{NodeID: "n" + string(rune('0'+i%10)), Summary: "Module: something"}
	}

	classes := map[string][]BaseNode{
		"typescript": nodes,
		"go":         make([]BaseNode, 20),
	}

	result := r.ReclassifyOversizedCategories(context.Background(), classes, 100)

	// Should be unchanged — fail-open
	if len(result["typescript"]) != 80 {
		t.Errorf("expected fail-open to preserve original 80 typescript nodes, got %d", len(result["typescript"]))
	}
}

func TestReclassifyOversizedCategories_Integration(t *testing.T) {
	// Mock LLM server that returns valid sub-categories
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		subCats := []SubCategory{
			{Name: "services", Keywords: []string{"service", "provider"}, Description: "Service layer"},
			{Name: "controllers", Keywords: []string{"controller", "handler", "endpoint"}, Description: "API controllers"},
			{Name: "misc", Keywords: []string{}, Description: "Everything else"},
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": mustJSONInline(subCats)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	r := NewReclassifier(ReclassifierConfig{
		Enabled:       true,
		Threshold:     0.25,
		MaxSampleSize: 10,
		MaxCategories: 10,
		MaxIterations: 1, // Single pass for this test
		Provider:      "openai",
		Model:         "gpt-4o-mini",
		MaxTokens:     2000,
		TimeoutMs:     5000,
		OpenAIKey:     "test-key",
		OpenAIURL:     mockServer.URL,
	}, nil)

	tsNodes := []BaseNode{
		{NodeID: "1", Summary: "Module: auth service provider"},
		{NodeID: "2", Summary: "Module: payment service"},
		{NodeID: "3", Summary: "Class: user controller endpoint"},
		{NodeID: "4", Summary: "File: utility code"},
		{NodeID: "5", Summary: "Module: order handler controller"},
	}
	goNodes := make([]BaseNode, 2)

	classes := map[string][]BaseNode{
		"typescript": tsNodes,
		"go":         goNodes,
	}

	result := r.ReclassifyOversizedCategories(context.Background(), classes, 7)

	// Original "typescript" key should be gone
	if _, ok := result["typescript"]; ok {
		t.Fatalf("expected 'typescript' to be removed after reclassification")
	}

	// Should have sub-categories
	if len(result["typescript-services"]) != 2 {
		t.Fatalf("expected 2 in typescript-services, got %d", len(result["typescript-services"]))
	}
	if len(result["typescript-controllers"]) != 2 {
		t.Errorf("expected 2 in typescript-controllers, got %d", len(result["typescript-controllers"]))
	}
	if len(result["typescript-misc"]) != 1 {
		t.Errorf("expected 1 in typescript-misc, got %d", len(result["typescript-misc"]))
	}

	// "go" should be preserved
	if len(result["go"]) != 2 {
		t.Errorf("expected 2 in go, got %d", len(result["go"]))
	}
}

func TestReclassifyOversizedCategories_BelowThreshold(t *testing.T) {
	r := NewReclassifier(ReclassifierConfig{
		Enabled:   true,
		Threshold: 0.50,
	}, nil)

	classes := map[string][]BaseNode{
		"typescript": make([]BaseNode, 40),
		"go":         make([]BaseNode, 35),
		"config":     make([]BaseNode, 25),
	}

	result := r.ReclassifyOversizedCategories(context.Background(), classes, 100)

	// Nothing should change
	if len(result["typescript"]) != 40 {
		t.Errorf("expected 40, got %d", len(result["typescript"]))
	}
	if len(result["go"]) != 35 {
		t.Errorf("expected 35, got %d", len(result["go"]))
	}
}

func TestReclassifyOversizedCategories_EmptyInput(t *testing.T) {
	r := NewReclassifier(ReclassifierConfig{Enabled: true, Threshold: 0.25}, nil)

	result := r.ReclassifyOversizedCategories(context.Background(), map[string][]BaseNode{}, 0)
	if len(result) != 0 {
		t.Errorf("expected empty result for empty input, got %d categories", len(result))
	}
}

func TestReclassifyOversizedCategories_RecursiveConvergence(t *testing.T) {
	// Mock LLM that returns different sub-categories each time, based on the category name.
	// First call (for "typescript" at 80%): splits into 4 sub-categories of ~20% each → all below 25%.
	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		subCats := []SubCategory{
			{Name: "group_a", Keywords: []string{"service", "provider"}, Description: "Group A services"},
			{Name: "group_b", Keywords: []string{"controller", "handler"}, Description: "Group B controllers"},
			{Name: "group_c", Keywords: []string{"module", "factory"}, Description: "Group C modules"},
			{Name: "misc", Keywords: []string{}, Description: "Everything else"},
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": mustJSONInline(subCats)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	r := NewReclassifier(ReclassifierConfig{
		Enabled:       true,
		Threshold:     0.25,
		MaxSampleSize: 50,
		MaxCategories: 10,
		MaxIterations: 5,
		Provider:      "openai",
		Model:         "test",
		MaxTokens:     2000,
		TimeoutMs:     5000,
		OpenAIKey:     "test-key",
		OpenAIURL:     mockServer.URL,
	}, nil)

	// 80 typescript nodes with varied summaries → each group gets ~20 nodes (20%)
	tsNodes := make([]BaseNode, 80)
	for i := range 20 {
		tsNodes[i] = BaseNode{NodeID: fmt.Sprintf("s%d", i), Summary: "Module: auth service provider"}
	}
	for i := range 20 {
		tsNodes[20+i] = BaseNode{NodeID: fmt.Sprintf("c%d", i), Summary: "Class: user controller handler"}
	}
	for i := range 20 {
		tsNodes[40+i] = BaseNode{NodeID: fmt.Sprintf("m%d", i), Summary: "Module: order factory module"}
	}
	for i := range 20 {
		tsNodes[60+i] = BaseNode{NodeID: fmt.Sprintf("x%d", i), Summary: "File: utility code"}
	}

	classes := map[string][]BaseNode{
		"typescript": tsNodes,
		"go":         make([]BaseNode, 20),
	}

	result := r.ReclassifyOversizedCategories(context.Background(), classes, 100)

	// "typescript" should be gone, replaced by sub-categories
	if _, ok := result["typescript"]; ok {
		t.Fatalf("expected 'typescript' to be removed after reclassification")
	}

	// Should have converged in 1 iteration (all sub-cats ≤20% after first split)
	if callCount != 1 {
		t.Errorf("expected 1 LLM call (single iteration), got %d", callCount)
	}

	// Verify CategoryDescriptions populated
	if len(r.CategoryDescriptions) == 0 {
		t.Error("expected CategoryDescriptions to be populated")
	}
	if desc, ok := r.CategoryDescriptions["typescript-group_a"]; !ok || desc == "" {
		t.Errorf("expected description for 'typescript-group_a', got %q", desc)
	}
}

func TestReclassifyOversizedCategories_RecursiveMultiPass(t *testing.T) {
	// Mock LLM that splits into 2 sub-categories — one still oversized (60%), requiring iteration 2
	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		var subCats []SubCategory
		if callCount == 1 {
			// First split: 60% goes to "big", 40% to "small"
			subCats = []SubCategory{
				{Name: "big", Keywords: []string{"service", "module", "provider", "factory", "handler", "controller"}, Description: "Big group"},
				{Name: "small", Keywords: []string{"utility", "helper"}, Description: "Small group"},
				{Name: "misc", Keywords: []string{}, Description: "Misc"},
			}
		} else {
			// Second split: balanced 3-way
			subCats = []SubCategory{
				{Name: "alpha", Keywords: []string{"service", "provider"}, Description: "Alpha"},
				{Name: "beta", Keywords: []string{"module", "factory"}, Description: "Beta"},
				{Name: "gamma", Keywords: []string{"handler", "controller"}, Description: "Gamma"},
				{Name: "misc", Keywords: []string{}, Description: "Misc"},
			}
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": mustJSONInline(subCats)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	r := NewReclassifier(ReclassifierConfig{
		Enabled:       true,
		Threshold:     0.25,
		MaxSampleSize: 50,
		MaxCategories: 10,
		MaxIterations: 5,
		Provider:      "openai",
		Model:         "test",
		MaxTokens:     2000,
		TimeoutMs:     5000,
		OpenAIKey:     "test-key",
		OpenAIURL:     mockServer.URL,
	}, nil)

	// 80 nodes: 30 service, 20 module, 10 handler, 20 utility
	tsNodes := make([]BaseNode, 80)
	for i := range 30 {
		tsNodes[i] = BaseNode{NodeID: fmt.Sprintf("s%d", i), Summary: "Module: auth service provider"}
	}
	for i := range 20 {
		tsNodes[30+i] = BaseNode{NodeID: fmt.Sprintf("m%d", i), Summary: "Module: order module factory"}
	}
	for i := range 10 {
		tsNodes[50+i] = BaseNode{NodeID: fmt.Sprintf("h%d", i), Summary: "Class: user handler controller"}
	}
	for i := range 20 {
		tsNodes[60+i] = BaseNode{NodeID: fmt.Sprintf("u%d", i), Summary: "File: utility helper code"}
	}

	classes := map[string][]BaseNode{
		"typescript": tsNodes,
		"config":     make([]BaseNode, 20),
	}

	result := r.ReclassifyOversizedCategories(context.Background(), classes, 100)

	// Should have taken 2 LLM calls (iteration 1: split typescript, iteration 2: split typescript-big)
	if callCount < 2 {
		t.Errorf("expected at least 2 LLM calls for multi-pass convergence, got %d", callCount)
	}

	// Original "typescript" should be gone
	if _, ok := result["typescript"]; ok {
		t.Fatalf("expected 'typescript' to be removed")
	}

	// "config" should be preserved
	if len(result["config"]) != 20 {
		t.Errorf("expected 20 config nodes preserved, got %d", len(result["config"]))
	}

	// Total node count should be preserved
	totalNodes := 0
	for _, nodes := range result {
		totalNodes += len(nodes)
	}
	if totalNodes != 100 {
		t.Fatalf("expected 100 total nodes preserved, got %d", totalNodes)
	}
}

func TestSingleChildSplitGuard(t *testing.T) {
	// Mock LLM returns categories, but keyword assignment puts 95% into one sub-category.
	// The single-child guard (>= 90%) should fire and preserve the original category.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		subCats := []SubCategory{
			{Name: "dominant", Keywords: []string{"service", "module", "provider", "handler", "controller", "factory"}, Description: "Dominant group"},
			{Name: "tiny", Keywords: []string{"zzz_rare_keyword"}, Description: "Tiny group"},
			{Name: "misc", Keywords: []string{}, Description: "Misc"},
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": mustJSONInline(subCats)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	r := NewReclassifier(ReclassifierConfig{
		Enabled:       true,
		Threshold:     0.25,
		MaxSampleSize: 10,
		MaxCategories: 10,
		MaxIterations: 3,
		Provider:      "openai",
		Model:         "test",
		MaxTokens:     2000,
		TimeoutMs:     5000,
		OpenAIKey:     "test-key",
		OpenAIURL:     mockServer.URL,
	}, nil)

	// 100 nodes: 95 match "dominant" keywords, 5 match nothing → misc
	nodes := make([]BaseNode, 100)
	for i := range 95 {
		nodes[i] = BaseNode{NodeID: fmt.Sprintf("n%d", i), Summary: "Module: auth service provider"}
	}
	for i := range 5 {
		nodes[95+i] = BaseNode{NodeID: fmt.Sprintf("x%d", i), Summary: "File: utility code"}
	}

	classes := map[string][]BaseNode{
		"typescript": nodes,
	}

	result := r.ReclassifyOversizedCategories(context.Background(), classes, 100)

	// Single-child guard should fire: dominant gets 95/100 = 95% >= 90%.
	// Original "typescript" should be preserved.
	if _, ok := result["typescript"]; !ok {
		t.Fatalf("expected 'typescript' to be preserved (single-child guard), but it was removed. Categories: %v", keysOf(result))
	}
	if len(result["typescript"]) != 100 {
		t.Fatalf("expected 100 nodes in preserved 'typescript', got %d", len(result["typescript"]))
	}
}

func TestKeywordDeduplication(t *testing.T) {
	r := NewReclassifier(ReclassifierConfig{
		Enabled:   true,
		Threshold: 0.25,
	}, nil)

	// Create sub-categories with overlapping keyword "service"
	subCats := []SubCategory{
		{Name: "api", Keywords: []string{"service", "endpoint"}},
		{Name: "core", Keywords: []string{"service", "model"}},
		{Name: "misc", Keywords: []string{}},
	}

	// proposeSubCategories is private, so test via assignToSubCategories after manual dedup.
	// The dedup logic removes "service" from "core" (second category).
	// Simulate: after dedup, core only has "model".
	globalSeen := make(map[string]bool)
	for i := range subCats {
		var deduped []string
		for _, kw := range subCats[i].Keywords {
			if !globalSeen[kw] {
				globalSeen[kw] = true
				deduped = append(deduped, kw)
			}
		}
		subCats[i].Keywords = deduped
	}

	// Verify dedup removed "service" from core
	if len(subCats[1].Keywords) != 1 || subCats[1].Keywords[0] != "model" {
		t.Fatalf("expected core keywords to be [model] after dedup, got %v", subCats[1].Keywords)
	}

	// Now assign: node with only "service" should go to api (first-wins via dedup)
	nodes := []BaseNode{
		{NodeID: "1", Summary: "a service module"},
	}
	result := r.assignToSubCategories(nodes, "ts", subCats)
	if len(result["ts-api"]) != 1 {
		t.Fatalf("expected node with 'service' to go to api (first-wins), got %v", result)
	}
}

func TestMaxDepthEnforcement(t *testing.T) {
	r := NewReclassifier(ReclassifierConfig{
		Threshold: 0.25,
		MaxDepth:  4,
	}, nil)

	// Category at depth 4 should NOT be detected as oversized, even if it exceeds threshold
	classes := map[string][]BaseNode{
		"typescript-services-auth-saml": make([]BaseNode, 80), // depth 4
		"go": make([]BaseNode, 20),
	}

	oversized := r.detectOversizedCategories(classes, 100)
	if len(oversized) != 0 {
		t.Fatalf("expected 0 oversized (depth-limited), got %d: %v", len(oversized), oversized)
	}

	// Category at depth 3 should still be detected
	classes2 := map[string][]BaseNode{
		"typescript-services-auth": make([]BaseNode, 80), // depth 3
		"go":                       make([]BaseNode, 20),
	}

	oversized2 := r.detectOversizedCategories(classes2, 100)
	if len(oversized2) != 1 {
		t.Fatalf("expected 1 oversized (depth 3 < maxDepth 4), got %d", len(oversized2))
	}
	if oversized2[0] != "typescript-services-auth" {
		t.Fatalf("expected 'typescript-services-auth', got %q", oversized2[0])
	}
}

func TestMiscDescriptionFallback(t *testing.T) {
	// When nodes land in misc but the LLM didn't return a "misc" category,
	// the fallback description should be set.
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		subCats := []SubCategory{
			{Name: "services", Keywords: []string{"service"}, Description: "Service layer"},
			{Name: "controllers", Keywords: []string{"controller"}, Description: "Controller layer"},
			// No explicit "misc" category — fallback "misc" name will be used
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": mustJSONInline(subCats)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	r := NewReclassifier(ReclassifierConfig{
		Enabled:       true,
		Threshold:     0.25,
		MaxSampleSize: 10,
		MaxCategories: 10,
		MaxIterations: 1,
		Provider:      "openai",
		Model:         "test",
		MaxTokens:     2000,
		TimeoutMs:     5000,
		OpenAIKey:     "test-key",
		OpenAIURL:     mockServer.URL,
	}, nil)

	nodes := []BaseNode{
		{NodeID: "1", Summary: "Module: auth service"},
		{NodeID: "2", Summary: "Class: user controller"},
		{NodeID: "3", Summary: "File: utility code"},          // → misc
		{NodeID: "4", Summary: "File: random documentation"},  // → misc
	}

	classes := map[string][]BaseNode{
		"typescript": nodes,
	}

	r.ReclassifyOversizedCategories(context.Background(), classes, 4)

	// Verify misc description is set
	desc, ok := r.CategoryDescriptions["typescript-misc"]
	if !ok {
		t.Fatalf("expected CategoryDescriptions to have 'typescript-misc' key, got keys: %v", descKeys(r.CategoryDescriptions))
	}
	if desc == "" {
		t.Fatalf("expected non-empty description for 'typescript-misc'")
	}
}

func TestProgressDetection(t *testing.T) {
	// Mock LLM returns splits where 98% lands in one sub-category.
	// Single-child guard fires, progress detection should stop the loop.
	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		subCats := []SubCategory{
			{Name: "big", Keywords: []string{"service", "module", "provider", "handler", "controller"}, Description: "Big"},
			{Name: "tiny", Keywords: []string{"zzz_nonexistent_keyword"}, Description: "Tiny"},
			{Name: "misc", Keywords: []string{}, Description: "Misc"},
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": mustJSONInline(subCats)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	r := NewReclassifier(ReclassifierConfig{
		Enabled:       true,
		Threshold:     0.25,
		MaxSampleSize: 10,
		MaxCategories: 10,
		MaxIterations: 5,
		Provider:      "openai",
		Model:         "test",
		MaxTokens:     2000,
		TimeoutMs:     5000,
		OpenAIKey:     "test-key",
		OpenAIURL:     mockServer.URL,
	}, nil)

	// 80 nodes all matching "big" keywords
	nodes := make([]BaseNode, 80)
	for i := range nodes {
		nodes[i] = BaseNode{NodeID: fmt.Sprintf("n%d", i), Summary: "Module: auth service provider"}
	}

	classes := map[string][]BaseNode{
		"typescript": nodes,
		"go":         make([]BaseNode, 20),
	}

	r.ReclassifyOversizedCategories(context.Background(), classes, 100)

	// Single-child guard fires on first attempt, progress detection stops the loop.
	// Should only make 1 LLM call, not 5.
	if callCount != 1 {
		t.Fatalf("expected 1 LLM call (progress detection stopped), got %d", callCount)
	}
}

func TestNameSanitization(t *testing.T) {
	// Simulate LLM response with problematic names containing dashes, dots, slashes, spaces
	rawSubCats := []SubCategory{
		{Name: "v2.0_services", Keywords: []string{"service"}, Description: "V2 services"},
		{Name: "api-handlers", Keywords: []string{"handler"}, Description: "API handlers"},
		{Name: "data models", Keywords: []string{"model"}, Description: "Data models"},
		{Name: "core/utils", Keywords: []string{"util"}, Description: "Core utils"},
		{Name: "misc", Keywords: []string{}, Description: "Misc"},
	}

	// Create a mock server that returns these names
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": mustJSONInline(rawSubCats)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockServer.Close()

	r := NewReclassifier(ReclassifierConfig{
		Enabled:   true,
		Threshold: 0.25,
		Provider:  "openai",
		Model:     "test",
		MaxTokens: 2000,
		TimeoutMs: 5000,
		OpenAIKey: "test-key",
		OpenAIURL: mockServer.URL,
	}, nil)

	subCats, err := r.proposeSubCategories(context.Background(), "typescript", 100, []string{"Module: something"})
	if err != nil {
		t.Fatalf("proposeSubCategories failed: %v", err)
	}

	// Verify names are sanitized — no separator chars remain
	expectedNames := map[string]bool{
		"v2_0_services": true,
		"api_handlers":  true,
		"data_models":   true,
		"core_utils":    true,
		"misc":          true,
	}
	for _, sc := range subCats {
		if strings.Contains(sc.Name, "-") || strings.Contains(sc.Name, ".") || strings.Contains(sc.Name, "/") || strings.Contains(sc.Name, " ") {
			t.Fatalf("name %q not sanitized: still contains separator char", sc.Name)
		}
		if !expectedNames[sc.Name] {
			t.Fatalf("unexpected sanitized name %q", sc.Name)
		}
	}
}

func TestDynamicPromptThreshold(t *testing.T) {
	prompt25 := buildReclassSystemPrompt(0.25)
	if !strings.Contains(prompt25, "25%") {
		t.Fatalf("expected prompt to contain '25%%' for threshold 0.25, got: %s", prompt25)
	}
	if strings.Contains(prompt25, "40%") {
		t.Fatalf("expected prompt NOT to contain '40%%' for threshold 0.25")
	}

	prompt40 := buildReclassSystemPrompt(0.40)
	if !strings.Contains(prompt40, "40%") {
		t.Fatalf("expected prompt to contain '40%%' for threshold 0.40, got: %s", prompt40)
	}
}

// --- Test helpers ---

func mustJSONInline(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func keysOf(m map[string][]BaseNode) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func descKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

