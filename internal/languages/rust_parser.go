package languages

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func init() {
	Register(&RustParser{})
}

// RustParser implements LanguageParser for Rust source files
type RustParser struct{}

func (p *RustParser) Name() string {
	return "rust"
}

func (p *RustParser) Extensions() []string {
	return []string{".rs"}
}

func (p *RustParser) CanParse(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".rs")
}

func (p *RustParser) IsTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.rs") ||
		strings.Contains(path, "/tests/") ||
		strings.Contains(path, "/test/")
}

func (p *RustParser) ParseFile(root, path string, extractSymbols bool) ([]CodeElement, error) {
	var elements []CodeElement

	content, err := ReadFileContent(path)
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(root, path)
	fileName := filepath.Base(path)

	// Extract module name from path (crate/module structure)
	moduleName := strings.TrimSuffix(fileName, ".rs")
	if moduleName == "mod" || moduleName == "lib" || moduleName == "main" {
		dir := filepath.Dir(relPath)
		if dir != "." {
			parts := strings.Split(dir, string(filepath.Separator))
			if len(parts) > 0 {
				moduleName = parts[len(parts)-1]
			}
		}
	}

	// Find structs, enums, traits, and functions
	structs := FindAllMatches(content, `(?:pub\s+)?struct\s+(\w+)`)
	enums := FindAllMatches(content, `(?:pub\s+)?enum\s+(\w+)`)
	traits := FindAllMatches(content, `(?:pub\s+)?trait\s+(\w+)`)
	functions := FindAllMatches(content, `(?:pub\s+)?(?:async\s+)?fn\s+(\w+)`)
	macros := FindAllMatches(content, `macro_rules!\s+(\w+)`)
	uses := FindAllMatches(content, `^\s*use\s+([\w:]+)`)

	// Build content for embedding
	var contentBuilder strings.Builder
	contentBuilder.WriteString(fmt.Sprintf("Rust file: %s\n", fileName))
	contentBuilder.WriteString(fmt.Sprintf("Module: %s\n", moduleName))

	if len(structs) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Structs: %s\n", strings.Join(uniqueStrings(structs), ", ")))
	}
	if len(enums) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Enums: %s\n", strings.Join(uniqueStrings(enums), ", ")))
	}
	if len(traits) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Traits: %s\n", strings.Join(uniqueStrings(traits), ", ")))
	}
	if len(functions) > 0 {
		fnList := uniqueStrings(functions)
		if len(fnList) > 15 {
			fnList = fnList[:15]
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s (and more)\n", strings.Join(fnList, ", ")))
		} else {
			contentBuilder.WriteString(fmt.Sprintf("Functions: %s\n", strings.Join(fnList, ", ")))
		}
	}
	if len(macros) > 0 {
		contentBuilder.WriteString(fmt.Sprintf("Macros: %s\n", strings.Join(uniqueStrings(macros), ", ")))
	}
	contentBuilder.WriteString(fmt.Sprintf("Imports: %d\n", len(uses)))

	// Include actual code content
	contentBuilder.WriteString("\n--- Code ---\n")
	contentBuilder.WriteString(TruncateContent(content, 4000))

	// Detect cross-cutting concerns
	concerns := DetectConcerns(relPath, content)
	tags := []string{"rust", "module"}
	tags = append(tags, concerns...)

	// Determine file kind
	rustKind := "rust-module"
	if fileName == "lib.rs" {
		rustKind = "rust-lib"
		tags = append(tags, "library")
	} else if fileName == "main.rs" {
		rustKind = "rust-main"
		tags = append(tags, "executable")
	} else if fileName == "mod.rs" {
		rustKind = "rust-mod"
		tags = append(tags, "submodule")
	}

	// Extract code symbols
	var symbols []Symbol
	if extractSymbols {
		symbols = p.extractSymbols(content)
	}

	// Add file-level element
	elements = append(elements, CodeElement{
		Name:     fileName,
		Kind:     rustKind,
		Path:     "/" + relPath,
		Content:  contentBuilder.String(),
		Package:  moduleName,
		FilePath: relPath,
		Tags:     tags,
		Concerns: concerns,
		Symbols:  symbols,
	})

	// Add structs as separate elements
	for _, st := range uniqueStrings(structs) {
		elements = append(elements, CodeElement{
			Name:     st,
			Kind:     "struct",
			Path:     fmt.Sprintf("/%s#%s", relPath, st),
			Content:  fmt.Sprintf("Rust struct '%s' in module %s (file: %s)", st, moduleName, fileName),
			Package:  moduleName,
			FilePath: relPath,
			Tags:     append([]string{"rust", "struct"}, concerns...),
			Concerns: concerns,
		})
	}

	// Add traits as separate elements
	for _, tr := range uniqueStrings(traits) {
		elements = append(elements, CodeElement{
			Name:     tr,
			Kind:     "trait",
			Path:     fmt.Sprintf("/%s#%s", relPath, tr),
			Content:  fmt.Sprintf("Rust trait '%s' in module %s (file: %s)", tr, moduleName, fileName),
			Package:  moduleName,
			FilePath: relPath,
			Tags:     append([]string{"rust", "trait"}, concerns...),
			Concerns: concerns,
		})
	}

	return elements, nil
}

func (p *RustParser) extractSymbols(content string) []Symbol {
	var symbols []Symbol
	lines := strings.Split(content, "\n")

	// Patterns for Rust constructs
	// pub, pub(crate), pub(super), pub(self), pub(in path) are all visibility modifiers
	pubPattern := `(pub(?:\s*\([^)]*\))?\s+)?`
	constPattern := regexp.MustCompile(`^\s*` + pubPattern + `const\s+([A-Z][A-Z0-9_]*)\s*:\s*([^=]+)\s*=\s*(.+?);`)
	typeAliasPattern := regexp.MustCompile(`^\s*` + pubPattern + `type\s+(\w+)\s*=\s*(.+?);`)
	structPattern := regexp.MustCompile(`^\s*` + pubPattern + `struct\s+(\w+)(?:<[^>]*>)?`)
	enumPattern := regexp.MustCompile(`^\s*` + pubPattern + `enum\s+(\w+)`)
	traitPattern := regexp.MustCompile(`^\s*` + pubPattern + `trait\s+(\w+)`)
	implPattern := regexp.MustCompile(`^\s*impl(?:<[^>]*>)?\s+(?:(\w+)(?:<[^>]*>)?\s+for\s+)?(\w+)(?:<[^>]*>)?`)
	fnPattern := regexp.MustCompile(`^\s*` + pubPattern + `(?:async\s+)?fn\s+(\w+)\s*(?:<[^>]*>)?\s*\(([^)]*)\)(?:\s*->\s*([^\{]+))?`)
	modPattern := regexp.MustCompile(`^\s*` + pubPattern + `mod\s+(\w+)`)
	macroPattern := regexp.MustCompile(`^\s*macro_rules!\s+(\w+)`)

	// Track current impl block for method parent assignment
	var currentImpl string
	var braceDepth int

	for i, line := range lines {
		lineNum := i + 1
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Track brace depth to know when we exit impl blocks
		braceDepth += strings.Count(line, "{") - strings.Count(line, "}")
		if braceDepth == 0 {
			currentImpl = ""
		}

		// Extract constants
		if matches := constPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:           matches[2],
				Type:           "constant",
				TypeAnnotation: strings.TrimSpace(matches[3]),
				Value:          CleanValue(matches[4]),
				RawValue:       matches[4],
				Line:           lineNum,
				Exported:       matches[1] != "",
				Language:       "rust",
			})
			continue
		}

		// Extract type aliases
		if matches := typeAliasPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:           matches[2],
				Type:           "type",
				TypeAnnotation: strings.TrimSpace(matches[3]),
				Line:           lineNum,
				Exported:       matches[1] != "",
				Language:       "rust",
			})
			continue
		}

		// Extract struct definitions
		if matches := structPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[2],
				Type:     "struct",
				Line:     lineNum,
				Exported: matches[1] != "",
				Language: "rust",
			})
			continue
		}

		// Extract enum definitions
		if matches := enumPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[2],
				Type:     "enum",
				Line:     lineNum,
				Exported: matches[1] != "",
				Language: "rust",
			})
			continue
		}

		// Extract trait definitions
		if matches := traitPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[2],
				Type:     "trait",
				Line:     lineNum,
				Exported: matches[1] != "",
				Language: "rust",
			})
			continue
		}

		// Track impl blocks for method parent assignment
		if matches := implPattern.FindStringSubmatch(line); matches != nil {
			// matches[1] is trait name (if "impl Trait for Type")
			// matches[2] is the type being implemented
			currentImpl = matches[2]
			if matches[1] != "" {
				// This is a trait implementation, include trait info
				currentImpl = matches[2] + " (impl " + matches[1] + ")"
			}
			continue
		}

		// Extract module definitions
		if matches := modPattern.FindStringSubmatch(line); matches != nil {
			symbols = append(symbols, Symbol{
				Name:     matches[2],
				Type:     "module",
				Line:     lineNum,
				Exported: matches[1] != "",
				Language: "rust",
			})
			continue
		}

		// Extract macro definitions
		if matches := macroPattern.FindStringSubmatch(line); matches != nil {
			// Check if previous line has #[macro_export]
			exported := false
			if i > 0 && strings.Contains(lines[i-1], "#[macro_export]") {
				exported = true
			}
			symbols = append(symbols, Symbol{
				Name:     matches[1],
				Type:     "macro",
				Line:     lineNum,
				Exported: exported,
				Language: "rust",
			})
			continue
		}

		// Extract functions and methods
		if matches := fnPattern.FindStringSubmatch(line); matches != nil {
			returnType := ""
			if len(matches) > 4 && matches[4] != "" {
				returnType = strings.TrimSpace(matches[4])
			}

			symType := "function"
			parent := ""
			if currentImpl != "" {
				symType = "method"
				parent = currentImpl
			}

			sig := fmt.Sprintf("(%s)%s", matches[3], formatReturn(returnType))

			symbols = append(symbols, Symbol{
				Name:           matches[2],
				Type:           symType,
				Parent:         parent,
				Signature:      sig,
				TypeAnnotation: returnType,
				Line:           lineNum,
				Exported:       matches[1] != "",
				Language:       "rust",
			})
			continue
		}
	}

	return symbols
}

// formatReturn formats a return type for display
func formatReturn(returnType string) string {
	if returnType == "" {
		return ""
	}
	return " -> " + returnType
}
