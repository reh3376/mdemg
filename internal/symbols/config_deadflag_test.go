package symbols

import (
	"context"
	"testing"
	"time"
)

// CONFIG-DEADFLAG-001 tests: verify the previously-dead relationship-extraction
// config fields are actually wired (tier skips, call cap, batch size, resolver
// cross-file gate + timeout) and that zero values preserve historical behavior.

// tsInheritanceAndCallsSource exercises tier 1 (imports), tier 2
// (extends/implements) and tier 3 (calls) in one TypeScript file.
const tsInheritanceAndCallsSource = `import { Base } from './base';

class Animal {
    name: string;
}

class Dog extends Animal {
    bark() {
        console.log("woof");
    }
}

interface Flyable {
    fly(): void;
}

class Bird implements Flyable {
    fly() {
        console.log("flying");
        this.bark();
    }
}
`

func relationshipCounts(rels []Relationship) map[string]int {
	counts := make(map[string]int)
	for _, rel := range rels {
		counts[rel.Relation]++
	}
	return counts
}

func parseTSWithConfig(t *testing.T, cfg ParserConfig) map[string]int {
	t.Helper()

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	defer parser.Close()

	result, err := parser.ParseContent(context.Background(), "test.ts", LangTypeScript, []byte(tsInheritanceAndCallsSource))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}
	return relationshipCounts(result.Relationships)
}

func TestExtractRelationships_InheritanceTierSkip(t *testing.T) {
	cfg := DefaultParserConfig()
	cfg.Relationships = &RelationshipExtractionConfig{
		ExtractInheritance: false,
		ExtractCalls:       true,
	}
	counts := parseTSWithConfig(t, cfg)

	if counts[RelExtends] != 0 || counts[RelImplements] != 0 {
		t.Errorf("tier-2 skip did not skip: got %d EXTENDS, %d IMPLEMENTS", counts[RelExtends], counts[RelImplements])
	}
	if counts[RelImports] == 0 {
		t.Error("tier-1 (imports) should be unaffected by inheritance skip")
	}
	if counts[RelCalls] == 0 {
		t.Error("tier-3 (calls) should be unaffected by inheritance skip")
	}
}

func TestExtractRelationships_CallsTierSkip(t *testing.T) {
	cfg := DefaultParserConfig()
	cfg.Relationships = &RelationshipExtractionConfig{
		ExtractInheritance: true,
		ExtractCalls:       false,
	}
	counts := parseTSWithConfig(t, cfg)

	if counts[RelCalls] != 0 {
		t.Errorf("tier-3 skip did not skip: got %d CALLS", counts[RelCalls])
	}
	if counts[RelImports] == 0 {
		t.Error("tier-1 (imports) should be unaffected by calls skip")
	}
	if counts[RelExtends] == 0 || counts[RelImplements] == 0 {
		t.Errorf("tier-2 should be unaffected by calls skip: got %d EXTENDS, %d IMPLEMENTS",
			counts[RelExtends], counts[RelImplements])
	}
}

func TestExtractRelationships_NilConfigKeepsAllTiers(t *testing.T) {
	// Zero-value ParserConfig.Relationships (nil) must equal historical
	// behavior: every tier extracted.
	cfg := DefaultParserConfig()
	if cfg.Relationships != nil {
		t.Fatal("DefaultParserConfig should leave Relationships nil (defaults)")
	}
	counts := parseTSWithConfig(t, cfg)

	for _, relType := range []string{RelImports, RelExtends, RelImplements, RelCalls} {
		if counts[relType] == 0 {
			t.Errorf("nil extraction config should extract %s relationships", relType)
		}
	}
}

func TestExtractRelationships_MaxCallsPerFuncOverride(t *testing.T) {
	calls := ""
	for i := 0; i < 20; i++ {
		calls += "    fmt.Println(\"call\")\n"
	}
	source := `package main

import "fmt"

func main() {
` + calls + `}
`

	cfg := DefaultParserConfig()
	cfg.Relationships = &RelationshipExtractionConfig{
		ExtractInheritance: true,
		ExtractCalls:       true,
		MaxCallsPerFunc:    5,
	}

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("failed to create parser: %v", err)
	}
	defer parser.Close()

	result, err := parser.ParseContent(context.Background(), "test.go", LangGo, []byte(source))
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	callCount := relationshipCounts(result.Relationships)[RelCalls]
	if callCount == 0 {
		t.Fatal("expected some calls to be extracted")
	}
	if callCount > 5 {
		t.Errorf("MaxCallsPerFunc=5 not applied: got %d calls", callCount)
	}
}

func TestRelationshipExtractionConfig_NilDefaults(t *testing.T) {
	var cfg *RelationshipExtractionConfig

	if !cfg.extractInheritance() {
		t.Error("nil config should default to extracting inheritance")
	}
	if !cfg.extractCalls() {
		t.Error("nil config should default to extracting calls")
	}
	if got := cfg.maxCallsPerFunc(); got != defaultMaxCallsPerFunc {
		t.Errorf("nil config maxCallsPerFunc = %d, want %d", got, defaultMaxCallsPerFunc)
	}

	// <=0 falls back to the default cap.
	cfg = &RelationshipExtractionConfig{ExtractInheritance: true, ExtractCalls: true, MaxCallsPerFunc: 0}
	if got := cfg.maxCallsPerFunc(); got != defaultMaxCallsPerFunc {
		t.Errorf("MaxCallsPerFunc=0 should default to %d, got %d", defaultMaxCallsPerFunc, got)
	}

	cfg.MaxCallsPerFunc = 7
	if got := cfg.maxCallsPerFunc(); got != 7 {
		t.Errorf("maxCallsPerFunc = %d, want 7", got)
	}
}

func TestStore_RelationshipBatchSize(t *testing.T) {
	s := &Store{}

	// Zero-value Store keeps the historical batch size.
	if got := s.effectiveRelBatchSize(); got != defaultRelBatchSize {
		t.Errorf("zero-value batch size = %d, want %d", got, defaultRelBatchSize)
	}

	s.SetRelationshipBatchSize(100)
	if got := s.effectiveRelBatchSize(); got != 100 {
		t.Errorf("configured batch size = %d, want 100", got)
	}

	// Invalid values restore the default.
	s.SetRelationshipBatchSize(-1)
	if got := s.effectiveRelBatchSize(); got != defaultRelBatchSize {
		t.Errorf("negative batch size should fall back to %d, got %d", defaultRelBatchSize, got)
	}
}

func TestNewResolver_DefaultsPreserveBehavior(t *testing.T) {
	r := NewResolver(nil)

	if !r.config.CrossFileResolve {
		t.Error("NewResolver should default CrossFileResolve to true (historical behavior)")
	}
	if r.config.ResolutionTimeout != 0 {
		t.Errorf("NewResolver should default ResolutionTimeout to 0, got %v", r.config.ResolutionTimeout)
	}
}

func TestResolver_ResolutionContext(t *testing.T) {
	// No timeout configured: context passes through without a deadline.
	r := NewResolver(nil)
	ctx, cancel := r.resolutionContext(context.Background())
	defer cancel()
	if _, ok := ctx.Deadline(); ok {
		t.Error("no-timeout resolver should not add a deadline")
	}

	// Timeout configured: context carries a deadline.
	r = NewResolverWithConfig(nil, ResolverConfig{CrossFileResolve: true, ResolutionTimeout: 30 * time.Second})
	ctx, cancel = r.resolutionContext(context.Background())
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("timeout resolver should add a deadline")
	}
	if remaining := time.Until(deadline); remaining <= 0 || remaining > 30*time.Second {
		t.Errorf("unexpected deadline distance: %v", remaining)
	}
}
