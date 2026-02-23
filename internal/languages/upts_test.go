package languages

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// typeCompat maps type compatibility groups (mirrors Python UPTS runner).
var typeCompat = map[string]map[string]bool{
	"class":     {"class": true, "struct": true},
	"struct":    {"class": true, "struct": true},
	"interface": {"interface": true, "trait": true},
	"trait":     {"interface": true, "trait": true},
	"type":      {"type": true, "type_alias": true},
	"type_alias": {"type": true, "type_alias": true},
	"section":   {"section": true, "module": true, "namespace": true},
	"module":    {"section": true, "module": true, "namespace": true},
	"namespace": {"section": true, "module": true, "namespace": true},
}

// findSpecDir walks up from the test file to find the UPTS specs directory.
func findSpecDir() string {
	// Start from the module root (where go.mod lives)
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	// Walk up looking for go.mod to find module root
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}

	specDir := filepath.Join(dir, "docs", "lang-parser", "lang-parse-spec", "upts", "specs")
	if _, err := os.Stat(specDir); err == nil {
		return specDir
	}
	return ""
}

// loadAllSpecs loads all .upts.json files from the spec directory.
func loadAllSpecs(specDir string) ([]UPTSSpec, error) {
	entries, err := os.ReadDir(specDir)
	if err != nil {
		return nil, fmt.Errorf("reading spec dir: %w", err)
	}

	var specs []UPTSSpec
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".upts.json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(specDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}
		var spec UPTSSpec
		if err := json.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// resolveFixture resolves the fixture path relative to the spec directory.
func resolveFixture(specDir string, spec UPTSSpec) string {
	if spec.Fixture.Type == "inline" {
		return ""
	}
	return filepath.Clean(filepath.Join(specDir, spec.Fixture.Path))
}

// typesMatch checks if expected and actual types are compatible.
func typesMatch(expected, actual string) bool {
	if strings.EqualFold(expected, actual) {
		return true
	}
	e := strings.ToLower(expected)
	a := strings.ToLower(actual)
	if e == a {
		return true
	}
	if compat, ok := typeCompat[e]; ok {
		return compat[a]
	}
	return false
}

// findMatchingSymbol finds an actual symbol that matches the expected one.
func findMatchingSymbol(expected UPTSSymbol, actual []Symbol, config UPTSConfig) (Symbol, bool) {
	tolerance := config.LineTolerance
	if tolerance == 0 {
		tolerance = 2
	}

	for _, sym := range actual {
		// Name must match exactly
		if sym.Name != expected.Name {
			continue
		}
		// Type must be compatible
		if !typesMatch(expected.Type, sym.Type) {
			continue
		}
		// Line must be within tolerance
		if expected.Line > 0 && sym.Line > 0 {
			if int(math.Abs(float64(sym.Line-expected.Line))) > tolerance {
				continue
			}
		}
		return sym, true
	}
	return Symbol{}, false
}

// TestUPTS is the main UPTS test harness. It loads all specs and validates
// each registered parser against its corresponding spec.
func TestUPTS(t *testing.T) {
	specDir := findSpecDir()
	if specDir == "" {
		t.Fatal("Could not find UPTS specs directory")
	}

	specs, err := loadAllSpecs(specDir)
	if err != nil {
		t.Fatalf("Failed to load specs: %v", err)
	}

	if len(specs) == 0 {
		t.Fatal("No UPTS specs found")
	}

	t.Logf("Loaded %d UPTS specs from %s", len(specs), specDir)

	for _, spec := range specs {
		spec := spec // capture range variable
		t.Run(spec.Language, func(t *testing.T) {
			// Recover from panics so one parser crash doesn't kill the suite
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("PANIC in %s parser: %v", spec.Language, r)
				}
			}()

			// Look up the parser
			parser, ok := GetParser(spec.Language)
			if !ok {
				t.Skipf("No parser registered for language %q", spec.Language)
				return
			}

			// Resolve fixture path
			fixturePath := resolveFixture(specDir, spec)
			if fixturePath == "" {
				t.Skip("Inline fixtures not supported yet")
				return
			}
			if _, err := os.Stat(fixturePath); os.IsNotExist(err) {
				t.Fatalf("Fixture not found: %s", fixturePath)
			}

			// Parse the fixture file
			elements, err := parser.ParseFile(filepath.Dir(fixturePath), fixturePath, true)
			if err != nil {
				t.Fatalf("ParseFile failed: %v", err)
			}

			// Collect all symbols from all elements
			var actual []Symbol
			for _, elem := range elements {
				actual = append(actual, elem.Symbols...)
			}

			t.Logf("Parser produced %d symbols from fixture", len(actual))

			// Validate symbol count range (warning only — Python runner doesn't enforce this)
			minCount, maxCount := spec.Expected.SymbolCountRange()
			if len(actual) < minCount {
				t.Logf("  WARN: Symbol count %d below minimum %d", len(actual), minCount)
			}
			if len(actual) > maxCount {
				t.Logf("  WARN: Symbol count %d above maximum %d", len(actual), maxCount)
			}

			// Validate each expected symbol
			matched := 0
			failed := 0
			skipped := 0

			for _, expected := range spec.Expected.Symbols {
				sym, found := findMatchingSymbol(expected, actual, spec.Config)
				if !found {
					if expected.Optional {
						skipped++
						t.Logf("  SKIP (optional): %s [%s] line %d", expected.Name, expected.Type, expected.Line)
						continue
					}
					failed++
					t.Errorf("  MISS: %s [%s] expected at line %d (±%d)",
						expected.Name, expected.Type, expected.Line, spec.Config.LineTolerance)
					// Log nearby actual symbols to aid debugging
					for _, a := range actual {
						if a.Name == expected.Name {
							t.Logf("    found %s [%s] at line %d (type mismatch?)", a.Name, a.Type, a.Line)
						}
						if a.Line > 0 && expected.Line > 0 &&
							int(math.Abs(float64(a.Line-expected.Line))) <= spec.Config.LineTolerance+3 {
							t.Logf("    nearby: %s [%s] at line %d", a.Name, a.Type, a.Line)
						}
					}
					continue
				}
				matched++

				// Validate parent (if config enables it and expected has parent)
				if spec.Config.ValidateParent && expected.Parent != "" {
					if sym.Parent != expected.Parent {
						// Allow partial match for generics (same as Python runner)
						if strings.Contains(sym.Parent, expected.Parent) || strings.Contains(expected.Parent, sym.Parent) {
							// Partial match — OK
						} else {
							t.Logf("  PARENT MISMATCH (warn): %s expected parent=%q got=%q",
								expected.Name, expected.Parent, sym.Parent)
						}
					}
				}

				// Validate value (if config enables it and expected has value)
				if spec.Config.ValidateValue && expected.Value != "" {
					if sym.Value != expected.Value {
						t.Errorf("  VALUE MISMATCH: %s expected value=%q got=%q",
							expected.Name, expected.Value, sym.Value)
					}
				}

				// Validate signature (if config enables it and expected has signature)
				if spec.Config.ValidateSignature && expected.Signature != "" {
					if sym.Signature != expected.Signature {
						t.Errorf("  SIGNATURE MISMATCH: %s expected sig=%q got=%q",
							expected.Name, expected.Signature, sym.Signature)
					}
				}

				// Validate signature_contains (always checked if present)
				if len(expected.SignatureContains) > 0 && sym.Signature != "" {
					for _, substr := range expected.SignatureContains {
						if !strings.Contains(sym.Signature, substr) {
							t.Errorf("  SIGNATURE MISSING %q in %s: %q",
								substr, expected.Name, sym.Signature)
						}
					}
				}

				// Validate exported (if expected specifies it) — warn only
				if expected.Exported != nil {
					if sym.Exported != *expected.Exported {
						t.Logf("  EXPORTED MISMATCH (warn): %s expected exported=%v got=%v",
							expected.Name, *expected.Exported, sym.Exported)
					}
				}

				// Validate doc_comment (if expected specifies it)
				if expected.DocComment != "" {
					if sym.DocComment == "" {
						t.Errorf("  DOC_COMMENT MISSING: %s expected=%q", expected.Name, expected.DocComment)
					} else if !strings.Contains(sym.DocComment, expected.DocComment) {
						t.Errorf("  DOC_COMMENT MISMATCH: %s expected contains=%q got=%q",
							expected.Name, expected.DocComment, sym.DocComment)
					}
				}
			}

			// Check excluded symbols
			for _, excl := range spec.Expected.Excluded {
				for _, sym := range actual {
					if excl.Name != "" && sym.Name == excl.Name {
						t.Logf("  WARN: excluded symbol %q found (reason: %s)", excl.Name, excl.Reason)
					}
					if excl.NamePattern != "" {
						if matched, _ := regexp.MatchString(excl.NamePattern, sym.Name); matched {
							t.Logf("  WARN: excluded pattern %q matched symbol %q (reason: %s)",
								excl.NamePattern, sym.Name, excl.Reason)
						}
					}
				}
			}

			t.Logf("Results: %d matched, %d failed, %d skipped (optional) out of %d expected",
				matched, failed, skipped, len(spec.Expected.Symbols))

			// Run evidence validation if enabled for this parser
			if spec.Config.ValidateEvidence {
				validateEvidence(t, elements, actual, spec)
			}

			// Dump all actual symbols at verbose level for debugging
			if testing.Verbose() {
				t.Logf("\n  --- Actual symbols (%d) ---", len(actual))
				for i, s := range actual {
					parent := ""
					if s.Parent != "" {
						parent = fmt.Sprintf(" parent=%s", s.Parent)
					}
					val := ""
					if s.Value != "" {
						val = fmt.Sprintf(" value=%q", s.Value)
					}
					t.Logf("  [%d] %s [%s] line=%d exported=%v%s%s",
						i, s.Name, s.Type, s.Line, s.Exported, parent, val)
				}
			}
		})
	}
}

// validateEvidence performs structural consistency checks on parser output.
// These validate internal consistency of parser output, not against spec expectations.
//
// Checks performed:
//  1. Symbol.LineEnd consistency: LineEnd >= Line when both > 0
//  2. CodeElement range consistency: StartLine <= EndLine when both > 0
//  3. Symbol containment: symbol Line is within element [StartLine, EndLine] when populated
//  4. UPTSSymbol.LineEnd matching: when spec has line_end and actual has LineEnd, they match within tolerance
func validateEvidence(t *testing.T, elements []CodeElement, actual []Symbol, spec UPTSSpec) {
	t.Helper()

	tolerance := spec.Config.LineTolerance
	if tolerance == 0 {
		tolerance = 2
	}

	evidenceErrors := 0
	evidenceWarnings := 0

	// Check 1: Symbol.LineEnd consistency
	for _, sym := range actual {
		if sym.LineEnd > 0 && sym.Line > 0 {
			if sym.LineEnd < sym.Line {
				t.Errorf("  EVIDENCE: symbol %q [%s] has LineEnd=%d < Line=%d",
					sym.Name, sym.Type, sym.LineEnd, sym.Line)
				evidenceErrors++
			}
		}
	}

	// Check 2: CodeElement range consistency
	for _, elem := range elements {
		if elem.StartLine > 0 && elem.EndLine > 0 {
			if elem.StartLine > elem.EndLine {
				t.Errorf("  EVIDENCE: element %q has StartLine=%d > EndLine=%d",
					elem.Name, elem.StartLine, elem.EndLine)
				evidenceErrors++
			}
		}
	}

	// Check 3: Symbol containment within element range
	for _, elem := range elements {
		if elem.StartLine <= 0 || elem.EndLine <= 0 {
			continue
		}
		for _, sym := range elem.Symbols {
			if sym.Line <= 0 {
				continue
			}
			if sym.Line < elem.StartLine || sym.Line > elem.EndLine {
				// Warning only — some parsers don't populate element ranges yet
				t.Logf("  EVIDENCE WARN: symbol %q [%s] line=%d outside element %q range [%d, %d]",
					sym.Name, sym.Type, sym.Line, elem.Name, elem.StartLine, elem.EndLine)
				evidenceWarnings++
			}
		}
	}

	// Check 4: UPTSSymbol.LineEnd matching against spec
	for _, expected := range spec.Expected.Symbols {
		if expected.LineEnd <= 0 {
			continue
		}
		sym, found := findMatchingSymbol(expected, actual, spec.Config)
		if !found || sym.LineEnd <= 0 {
			continue
		}
		if int(math.Abs(float64(sym.LineEnd-expected.LineEnd))) > tolerance {
			t.Errorf("  EVIDENCE: symbol %q LineEnd=%d expected=%d (±%d)",
				expected.Name, sym.LineEnd, expected.LineEnd, tolerance)
			evidenceErrors++
		}
	}

	t.Logf("Evidence validation: %d errors, %d warnings", evidenceErrors, evidenceWarnings)
}
